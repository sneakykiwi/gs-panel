package services

import (
	"bufio"
	"context"
	"sync"

	"github.com/moby/moby/client"
)

type ConsoleService struct {
	docker      *client.Client
	connections map[uint][]chan string
	mu          sync.RWMutex
}

func NewConsoleService(docker *client.Client) *ConsoleService {
	return &ConsoleService{
		docker:      docker,
		connections: make(map[uint][]chan string),
	}
}

func (s *ConsoleService) Subscribe(serverID uint) chan string {
	s.mu.Lock()
	defer s.mu.Unlock()

	ch := make(chan string, 100)
	s.connections[serverID] = append(s.connections[serverID], ch)
	return ch
}

func (s *ConsoleService) Unsubscribe(serverID uint, ch chan string) {
	s.mu.Lock()
	defer s.mu.Unlock()

	channels := s.connections[serverID]
	for i, c := range channels {
		if c == ch {
			s.connections[serverID] = append(channels[:i], channels[i+1:]...)
			close(ch)
			break
		}
	}
}

func (s *ConsoleService) Broadcast(serverID uint, message string) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	for _, ch := range s.connections[serverID] {
		select {
		case ch <- message:
		default:
		}
	}
}

func (s *ConsoleService) StreamLogs(ctx context.Context, containerID string, serverID uint) error {
	reader, err := s.docker.ContainerLogs(ctx, containerID, client.ContainerLogsOptions{
		ShowStdout: true,
		ShowStderr: true,
		Follow:     true,
		Tail:       "100",
	})
	if err != nil {
		return err
	}
	defer reader.Close()

	scanner := bufio.NewScanner(reader)
	for scanner.Scan() {
		select {
		case <-ctx.Done():
			return nil
		default:
			line := scanner.Text()
			if len(line) > 8 {
				line = line[8:]
			}
			s.Broadcast(serverID, line)
		}
	}

	return scanner.Err()
}
