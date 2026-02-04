package services

import (
	"bufio"
	"bytes"
	"context"
	"encoding/binary"
	"fmt"
	"io"
	"net"
	"strings"
	"sync"
	"time"

	"github.com/sneakykiwi/gs-panel/internal/logger"

	"github.com/moby/moby/client"
)

type ConsoleSession struct {
	serverID    string
	containerID string
	docker      *client.Client

	// Connection
	conn   net.Conn
	reader *bufio.Reader
	writer io.Writer

	// State
	mu             sync.RWMutex
	attached       bool
	stopChan       chan struct{}
	reconnectCount int

	// Callbacks
	onOutput func(string)
	onStatus func(string)
}

func NewConsoleSession(serverID, containerID string, docker *client.Client, onOutput func(string), onStatus func(string)) *ConsoleSession {
	return &ConsoleSession{
		serverID:    serverID,
		containerID: containerID,
		docker:      docker,
		onOutput:    onOutput,
		onStatus:    onStatus,
		stopChan:    make(chan struct{}),
	}
}

func (s *ConsoleSession) Start(ctx context.Context) {
	go s.run(ctx)
}

func (s *ConsoleSession) run(ctx context.Context) {
	defer func() {
		s.mu.Lock()
		s.attached = false
		s.mu.Unlock()
	}()

	backoff := time.Second
	maxBackoff := 30 * time.Second

	for {
		select {
		case <-ctx.Done():
			return
		case <-s.stopChan:
			return
		default:
		}

		inspect, err := s.docker.ContainerInspect(ctx, s.containerID, client.ContainerInspectOptions{})
		if err != nil {
			logger.Error().Str("server_id", s.serverID).Err(err).Msg("Failed to inspect container")
			s.onStatus("error: container not found")
			s.sleepWithBackoff(&backoff, maxBackoff)
			continue
		}

		if !inspect.Container.State.Running {
			s.mu.Lock()
			wasAttached := s.attached
			s.attached = false
			s.mu.Unlock()

			if wasAttached {
				s.onStatus("disconnected")
			} else {
				s.onStatus("waiting")
			}

			s.sleepWithBackoff(&backoff, maxBackoff)
			continue
		}

		s.onStatus("connecting")
		if err := s.attach(ctx); err != nil {
			logger.Error().Str("server_id", s.serverID).Err(err).Msg("Failed to attach to container")
			s.onStatus("error: attach failed")
			s.sleepWithBackoff(&backoff, maxBackoff)
			continue
		}

		s.mu.Lock()
		s.attached = true
		s.reconnectCount = 0
		s.mu.Unlock()

		backoff = time.Second
		s.onStatus("connected")
		logger.Info().Str("server_id", s.serverID).Str("container_id", s.containerID).Msg("Console attached to container")

		s.stream(ctx)

		s.mu.Lock()
		s.attached = false
		if s.conn != nil {
			s.conn.Close()
			s.conn = nil
		}
		s.mu.Unlock()

		s.onStatus("disconnected")
		logger.Info().Str("server_id", s.serverID).Msg("Console disconnected from container")
	}
}

func (s *ConsoleSession) attach(ctx context.Context) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.conn != nil {
		s.conn.Close()
	}

	inspect, err := s.docker.ContainerInspect(ctx, s.containerID, client.ContainerInspectOptions{})
	if err != nil {
		return err
	}
	usesTTY := inspect.Container.Config.Tty

	resp, err := s.docker.ContainerAttach(ctx, s.containerID, client.ContainerAttachOptions{
		Stream: true,
		Stdin:  true,
		Stdout: true,
		Stderr: true,
		Logs:   false,
	})
	if err != nil {
		return fmt.Errorf("container attach failed: %w", err)
	}

	s.conn = resp.Conn
	s.reader = resp.Reader

	// If TTY is enabled, we can write directly to the connection
	if usesTTY {
		s.writer = resp.Conn
	} else {
		//TODO implement attach protocol for non-TTY
		s.writer = resp.Conn
	}

	return nil
}

func (s *ConsoleSession) stream(ctx context.Context) {
	s.mu.RLock()
	reader := s.reader
	conn := s.conn
	s.mu.RUnlock()

	if reader == nil || conn == nil {
		return
	}

	buf := make([]byte, 4096)
	var lineBuf strings.Builder

	const readTimeout = 2 * time.Second

	for {
		select {
		case <-ctx.Done():
			if line := strings.TrimRight(lineBuf.String(), "\r\n"); line != "" {
				s.onOutput(line)
			}
			return
		case <-s.stopChan:
			if line := strings.TrimRight(lineBuf.String(), "\r\n"); line != "" {
				s.onOutput(line)
			}
			return
		default:
		}

		if err := conn.SetReadDeadline(time.Now().Add(readTimeout)); err != nil {
			logger.Error().Str("server_id", s.serverID).Err(err).Msg("Failed to set read deadline")
			if line := strings.TrimRight(lineBuf.String(), "\r\n"); line != "" {
				s.onOutput(line)
			}
			return
		}

		n, err := reader.Read(buf)
		if err != nil {
			if netErr, ok := err.(net.Error); ok && netErr.Timeout() {
				continue
			}
			if err != io.EOF {
				logger.Error().Str("server_id", s.serverID).Err(err).Msg("Console stream read error")
			}
			if line := strings.TrimRight(lineBuf.String(), "\r\n"); line != "" {
				s.onOutput(line)
			}
			return
		}

		if n > 0 {
			data := string(buf[:n])
			for _, ch := range data {
				if ch == '\n' || ch == '\r' {
					line := strings.TrimRight(lineBuf.String(), "\r\n")
					if line != "" {
						s.onOutput(line)
					}
					lineBuf.Reset()
				} else {
					lineBuf.WriteRune(ch)
				}
			}
		}
	}
}

func (s *ConsoleSession) WriteCommand(command string) error {
	s.mu.RLock()
	attached := s.attached
	writer := s.writer
	s.mu.RUnlock()

	if !attached || writer == nil {
		return fmt.Errorf("server is not running")
	}

	command = strings.TrimSpace(command)
	if command == "" {
		return fmt.Errorf("command is empty")
	}

	cmdBytes := []byte(command + "\n")

	_, err := writer.Write(cmdBytes)
	if err != nil {
		return fmt.Errorf("failed to write command: %w", err)
	}

	return nil
}

func (s *ConsoleSession) IsAttached() bool {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.attached
}

func (s *ConsoleSession) Stop() {
	close(s.stopChan)

	s.mu.Lock()
	if s.conn != nil {
		s.conn.Close()
	}
	s.mu.Unlock()
}

func (s *ConsoleSession) sleepWithBackoff(backoff *time.Duration, maxBackoff time.Duration) {
	time.Sleep(*backoff)

	*backoff = *backoff * 2
	if *backoff > maxBackoff {
		*backoff = maxBackoff
	}
}

type ConsoleService struct {
	docker *client.Client

	ctx    context.Context
	cancel context.CancelFunc

	sessions    map[StreamKey]*ConsoleSession
	connections map[StreamKey][]chan string
	mu          sync.RWMutex
}

func NewConsoleService(docker *client.Client) *ConsoleService {
	ctx, cancel := context.WithCancel(context.Background())
	return &ConsoleService{
		docker:      docker,
		ctx:         ctx,
		cancel:      cancel,
		sessions:    make(map[StreamKey]*ConsoleSession),
		connections: make(map[StreamKey][]chan string),
	}
}

func (s *ConsoleService) Subscribe(serverID string) chan string {
	s.mu.Lock()
	defer s.mu.Unlock()

	ch := make(chan string, 200)
	key := StreamKey(serverID)
	s.connections[key] = append(s.connections[key], ch)
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
		if session, ok := s.sessions[key]; ok {
			session.Stop()
			delete(s.sessions, key)
		}
	}
}

func (s *ConsoleService) EnsureSession(serverID, containerID string) {
	s.mu.Lock()

	key := StreamKey(serverID)

	if session, ok := s.sessions[key]; ok {
		if session.containerID != containerID {
			session.Stop()
			delete(s.sessions, key)
		} else {
			s.mu.Unlock()
			return
		}
	}

	if len(s.connections[key]) == 0 {
		s.mu.Unlock()
		return
	}

	onOutput := func(line string) {
		s.Broadcast(serverID, line)
	}

	onStatus := func(status string) {
		s.BroadcastStatus(serverID, status)
	}

	session := NewConsoleSession(serverID, containerID, s.docker, onOutput, onStatus)
	s.sessions[key] = session

	s.mu.Unlock()

	session.Start(s.ctx)
}

func (s *ConsoleService) StopSession(serverID string) {
	s.mu.Lock()
	defer s.mu.Unlock()

	key := StreamKey(serverID)
	if session, ok := s.sessions[key]; ok {
		session.Stop()
		delete(s.sessions, key)
	}
}

func (s *ConsoleService) Shutdown() {
	s.cancel()

	s.mu.Lock()
	defer s.mu.Unlock()

	for key, session := range s.sessions {
		session.Stop()
		delete(s.sessions, key)
	}

	for key, channels := range s.connections {
		for _, ch := range channels {
			close(ch)
		}
		delete(s.connections, key)
	}
}

func (s *ConsoleService) SendCommand(serverID string, command string) error {
	s.mu.RLock()
	session, exists := s.sessions[StreamKey(serverID)]
	s.mu.RUnlock()

	if !exists {
		return fmt.Errorf("server is not running")
	}

	if !session.IsAttached() {
		return fmt.Errorf("server is not running")
	}

	return session.WriteCommand(command)
}

func (s *ConsoleService) Broadcast(serverID string, message string) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	key := StreamKey(serverID)
	for _, ch := range s.connections[key] {
		select {
		case ch <- message:
		default:
			// channel full, skip message
		}
	}
}

func (s *ConsoleService) BroadcastStatus(serverID string, status string) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	key := StreamKey(serverID)
	for _, ch := range s.connections[key] {
		select {
		case ch <- "[status]" + status + "\n":
		default:
		}
	}
}

func (s *ConsoleService) FetchHistoricalLogs(ctx context.Context, containerID string, lines int) ([]string, error) {
	inspect, err := s.docker.ContainerInspect(ctx, containerID, client.ContainerInspectOptions{})
	if err != nil {
		return nil, fmt.Errorf("failed to inspect container: %w", err)
	}
	usesTTY := inspect.Container.Config.Tty

	reader, err := s.docker.ContainerLogs(ctx, containerID, client.ContainerLogsOptions{
		ShowStdout: true,
		ShowStderr: true,
		Tail:       fmt.Sprintf("%d", lines),
		Timestamps: false,
	})
	if err != nil {
		return nil, fmt.Errorf("failed to fetch container logs: %w", err)
	}
	defer reader.Close()

	var logData io.Reader
	if usesTTY {
		logData = reader
	} else {
		demuxed, err := demultiplexDockerStream(reader)
		if err != nil {
			return nil, fmt.Errorf("failed to demultiplex container logs: %w", err)
		}
		logData = bytes.NewReader(demuxed)
	}

	var logs []string
	scanner := bufio.NewScanner(logData)
	buf := make([]byte, 0, 64*1024)
	scanner.Buffer(buf, 1024*1024)

	for scanner.Scan() {
		line := strings.TrimRight(scanner.Text(), "\r\n")
		logs = append(logs, line)
	}

	if err := scanner.Err(); err != nil {
		return nil, fmt.Errorf("failed to read container logs: %w", err)
	}

	return logs, nil
}

func demultiplexDockerStream(reader io.Reader) ([]byte, error) {
	var result bytes.Buffer
	header := make([]byte, 8)

	for {
		_, err := io.ReadFull(reader, header)
		if err == io.EOF {
			break
		}
		if err != nil {
			return nil, err
		}

		size := binary.BigEndian.Uint32(header[4:8])
		if size == 0 {
			continue
		}

		payload := make([]byte, size)
		_, err = io.ReadFull(reader, payload)
		if err != nil {
			return nil, err
		}

		result.Write(payload)
	}

	return result.Bytes(), nil
}
