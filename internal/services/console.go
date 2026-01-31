package services

import (
	"bufio"
	"context"
	"fmt"
	"io"
	"strings"
	"sync"

	"github.com/sneakykiwi/gs-panel/internal/logger"

	"github.com/moby/moby/api/pkg/stdcopy"
	"github.com/moby/moby/client"
)

type ConsoleService struct {
	docker *client.Client

	connections map[StreamKey][]chan string
	streams     map[StreamKey]context.CancelFunc
	mu          sync.RWMutex
}

func NewConsoleService(docker *client.Client) *ConsoleService {
	return &ConsoleService{
		docker:      docker,
		connections: make(map[StreamKey][]chan string),
		streams:     make(map[StreamKey]context.CancelFunc),
	}
}

func (s *ConsoleService) Subscribe(serverID string) chan string {
	s.mu.Lock()
	defer s.mu.Unlock()

	ch := make(chan string, 200)
	s.connections[StreamKey(serverID)] = append(s.connections[StreamKey(serverID)], ch)
	return ch
}

func (s *ConsoleService) Unsubscribe(serverID string, ch chan string) {
	s.mu.Lock()
	defer s.mu.Unlock()

	key := StreamKey(serverID)
	channels := s.connections[key]
	for i, c := range channels {
		if c == ch {
			s.connections[key] = append(channels[:i], channels[i+1:]...)
			close(ch)
			break
		}
	}

	if len(s.connections[key]) == 0 {
		delete(s.connections, key)
		// no more listeners, stop docker log streaming
		if cancel, ok := s.streams[key]; ok {
			cancel()
			delete(s.streams, key)
		}
	}
}

func (s *ConsoleService) Broadcast(serverID string, message string) {
	logger.Info().Str("server_id", serverID).Str("log", message).Msg("console")
	s.mu.RLock()
	defer s.mu.RUnlock()

	for _, ch := range s.connections[StreamKey(serverID)] {
		select {
		case ch <- message:
		default:
		}
	}
}

// StartLogStream ensures there is exactly one docker log follower per server.
// It stops automatically when the last subscriber disconnects.
func (s *ConsoleService) StartLogStream(serverID, containerID string) {
	s.mu.Lock()
	key := StreamKey(serverID)
	if _, exists := s.streams[key]; exists {
		s.mu.Unlock()
		return
	}
	ctx, cancel := context.WithCancel(context.Background())
	s.streams[key] = cancel
	s.mu.Unlock()

	go func() {
		defer func() {
			s.mu.Lock()
			if c, ok := s.streams[key]; ok {
				c()
				delete(s.streams, key)
			}
			s.mu.Unlock()
		}()

		_ = s.StreamLogs(ctx, containerID, serverID)
	}()
}

func (s *ConsoleService) SendCommand(ctx context.Context, serverID string, containerID string, command string) error {
	command = strings.TrimSpace(command)
	if command == "" {
		return fmt.Errorf("command is required")
	}

	inspect, err := s.docker.ContainerInspect(ctx, containerID, client.ContainerInspectOptions{})
	if err != nil {
		return err
	}
	tty := inspect.Container.Config.Tty

	execResp, err := s.docker.ExecCreate(ctx, containerID, client.ExecCreateOptions{
		AttachStdout: true,
		AttachStderr: true,
		TTY:          tty,
		Cmd:          []string{"/bin/sh", "-lc", command},
	})
	if err != nil {
		return err
	}

	attach, err := s.docker.ExecAttach(ctx, execResp.ID, client.ExecAttachOptions{})
	if err != nil {
		return err
	}
	defer attach.Close()

	if tty {
		scanner := bufio.NewScanner(attach.Reader)
		buf := make([]byte, 0, 64*1024)
		scanner.Buffer(buf, 1024*1024)
		for scanner.Scan() {
			select {
			case <-ctx.Done():
				return ctx.Err()
			default:
				line := strings.TrimRight(scanner.Text(), "\r\n")
				if line != "" {
					s.Broadcast(serverID, line)
				}
			}
		}
		return scanner.Err()
	}

	stdoutR, stdoutW := io.Pipe()
	stderrR, stderrW := io.Pipe()
	copyDone := make(chan error, 1)
	go func() {
		_, err := stdcopy.StdCopy(stdoutW, stderrW, attach.Reader)
		stdoutW.Close()
		stderrW.Close()
		copyDone <- err
	}()

	stdoutDone := make(chan struct{})
	go func() {
		defer close(stdoutDone)
		scanner := bufio.NewScanner(stdoutR)
		buf := make([]byte, 0, 64*1024)
		scanner.Buffer(buf, 1024*1024)
		for scanner.Scan() {
			select {
			case <-ctx.Done():
				return
			default:
				line := strings.TrimRight(scanner.Text(), "\r\n")
				if line != "" {
					s.Broadcast(serverID, line)
				}
			}
		}
	}()

	stderrDone := make(chan struct{})
	go func() {
		defer close(stderrDone)
		scanner := bufio.NewScanner(stderrR)
		buf := make([]byte, 0, 64*1024)
		scanner.Buffer(buf, 1024*1024)
		for scanner.Scan() {
			select {
			case <-ctx.Done():
				return
			default:
				line := strings.TrimRight(scanner.Text(), "\r\n")
				if line != "" {
					s.Broadcast(serverID, "[stderr] "+line)
				}
			}
		}
	}()

	<-copyDone
	<-stdoutDone
	<-stderrDone
	return nil
}

func (s *ConsoleService) StreamLogs(ctx context.Context, containerID string, serverID string) error {
	inspect, err := s.docker.ContainerInspect(ctx, containerID, client.ContainerInspectOptions{})
	if err != nil {
		return err
	}
	tty := inspect.Container.Config.Tty

	reader, err := s.docker.ContainerLogs(ctx, containerID, client.ContainerLogsOptions{
		ShowStdout: true,
		ShowStderr: true,
		Follow:     true,
		Tail:       "all", // Show all logs since container start
		Timestamps: false,
	})
	if err != nil {
		return err
	}
	defer reader.Close()

	if tty {
		scanner := bufio.NewScanner(reader)
		buf := make([]byte, 0, 64*1024)
		scanner.Buffer(buf, 1024*1024)
		for scanner.Scan() {
			select {
			case <-ctx.Done():
				return ctx.Err()
			default:
				line := strings.TrimRight(scanner.Text(), "\r\n")
				if line != "" {
					s.Broadcast(serverID, line)
				}
			}
		}
		return scanner.Err()
	}

	stdoutR, stdoutW := io.Pipe()
	stderrR, stderrW := io.Pipe()
	copyDone := make(chan error, 1)
	go func() {
		_, err := stdcopy.StdCopy(stdoutW, stderrW, reader)
		stdoutW.Close()
		stderrW.Close()
		copyDone <- err
	}()

	stdoutDone := make(chan struct{})
	go func() {
		defer close(stdoutDone)
		scanner := bufio.NewScanner(stdoutR)
		buf := make([]byte, 0, 64*1024)
		scanner.Buffer(buf, 1024*1024)
		for scanner.Scan() {
			select {
			case <-ctx.Done():
				return
			default:
				line := strings.TrimRight(scanner.Text(), "\r\n")
				if line != "" {
					s.Broadcast(serverID, line)
				}
			}
		}
	}()

	stderrDone := make(chan struct{})
	go func() {
		defer close(stderrDone)
		scanner := bufio.NewScanner(stderrR)
		buf := make([]byte, 0, 64*1024)
		scanner.Buffer(buf, 1024*1024)
		for scanner.Scan() {
			select {
			case <-ctx.Done():
				return
			default:
				line := strings.TrimRight(scanner.Text(), "\r\n")
				if line != "" {
					s.Broadcast(serverID, "[stderr] "+line)
				}
			}
		}
	}()

	select {
	case <-ctx.Done():
		return ctx.Err()
	case err := <-copyDone:
		<-stdoutDone
		<-stderrDone
		if err != nil && err != io.EOF {
			return err
		}
		return nil
	}
}
