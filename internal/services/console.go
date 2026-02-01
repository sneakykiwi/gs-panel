package services

import (
	"bufio"
	"context"
	"fmt"
	"io"
	"net"
	"strings"
	"sync"
	"time"

	"github.com/sneakykiwi/gs-panel/internal/logger"

	"github.com/moby/moby/client"
)

// ConsoleSession maintains a persistent attach connection to a container
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

// NewConsoleSession creates a new console session
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

// Start begins the session with infinite reconnection
func (s *ConsoleSession) Start(ctx context.Context) {
	go s.run(ctx)
}

// run is the main loop that handles attachment and reconnection
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

		// Check if container is running before attempting attach
		inspect, err := s.docker.ContainerInspect(ctx, s.containerID, client.ContainerInspectOptions{})
		if err != nil {
			logger.Error().Str("server_id", s.serverID).Err(err).Msg("Failed to inspect container")
			s.onStatus("error: container not found")
			s.sleepWithBackoff(&backoff, maxBackoff)
			continue
		}

		if !inspect.Container.State.Running {
			// Container exists but is not running
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

		// Container is running, try to attach
		s.onStatus("connecting")
		if err := s.attach(ctx); err != nil {
			logger.Error().Str("server_id", s.serverID).Err(err).Msg("Failed to attach to container")
			s.onStatus("error: attach failed")
			s.sleepWithBackoff(&backoff, maxBackoff)
			continue
		}

		// Successfully attached
		s.mu.Lock()
		s.attached = true
		s.reconnectCount = 0
		s.mu.Unlock()

		backoff = time.Second // Reset backoff on successful connection
		s.onStatus("connected")
		logger.Info().Str("server_id", s.serverID).Str("container_id", s.containerID).Msg("Console attached to container")

		// Stream until disconnected
		s.stream(ctx)

		// Connection lost
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

// attach establishes the ContainerAttach connection
func (s *ConsoleSession) attach(ctx context.Context) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	// Clean up any existing connection
	if s.conn != nil {
		s.conn.Close()
	}

	// Check if container uses TTY
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
		Logs:   false, // Only new logs, not historical
	})
	if err != nil {
		return fmt.Errorf("container attach failed: %w", err)
	}

	s.conn = resp.Conn
	s.reader = resp.Reader

	// If TTY is enabled, we can write directly to the connection
	// Otherwise we need to handle the multiplexed stream
	if usesTTY {
		s.writer = resp.Conn
	} else {
		// For non-TTY, we need to handle stdin differently
		// The Docker attach protocol for non-TTY is more complex
		// For now, we'll use the connection directly
		s.writer = resp.Conn
	}

	return nil
}

// stream reads output from the container and broadcasts it
func (s *ConsoleSession) stream(ctx context.Context) {
	s.mu.RLock()
	reader := s.reader
	s.mu.RUnlock()

	if reader == nil {
		return
	}

	// Read bytes directly for minimal latency (no line buffering)
	buf := make([]byte, 4096)
	var lineBuf strings.Builder

	for {
		select {
		case <-ctx.Done():
			return
		case <-s.stopChan:
			return
		default:
		}

		n, err := reader.Read(buf)
		if err != nil {
			if err != io.EOF {
				logger.Error().Str("server_id", s.serverID).Err(err).Msg("Console stream read error")
			}
			return
		}

		if n > 0 {
			// Process the bytes, preserving partial lines in buffer
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

// WriteCommand writes a command directly to the container's stdin
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

	// Add newline since we're writing to TTY stdin
	cmdBytes := []byte(command + "\n")

	_, err := writer.Write(cmdBytes)
	if err != nil {
		return fmt.Errorf("failed to write command: %w", err)
	}

	return nil
}

// IsAttached returns whether the session is currently attached
func (s *ConsoleSession) IsAttached() bool {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.attached
}

// Stop gracefully stops the session
func (s *ConsoleSession) Stop() {
	close(s.stopChan)

	s.mu.Lock()
	if s.conn != nil {
		s.conn.Close()
	}
	s.mu.Unlock()
}

// sleepWithBackoff sleeps with exponential backoff
func (s *ConsoleSession) sleepWithBackoff(backoff *time.Duration, maxBackoff time.Duration) {
	time.Sleep(*backoff)

	// Increase backoff for next time (exponential, capped at max)
	*backoff = *backoff * 2
	if *backoff > maxBackoff {
		*backoff = maxBackoff
	}
}

// ConsoleService manages console sessions and subscriptions
type ConsoleService struct {
	docker *client.Client

	sessions    map[StreamKey]*ConsoleSession
	connections map[StreamKey][]chan string
	mu          sync.RWMutex
}

// NewConsoleService creates a new console service
func NewConsoleService(docker *client.Client) *ConsoleService {
	return &ConsoleService{
		docker:      docker,
		sessions:    make(map[StreamKey]*ConsoleSession),
		connections: make(map[StreamKey][]chan string),
	}
}

// Subscribe adds a subscriber for a server's console output
func (s *ConsoleService) Subscribe(serverID string) chan string {
	s.mu.Lock()
	defer s.mu.Unlock()

	ch := make(chan string, 200)
	key := StreamKey(serverID)
	s.connections[key] = append(s.connections[key], ch)
	return ch
}

// Unsubscribe removes a subscriber
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

	// Clean up session if no more subscribers
	if len(s.connections[key]) == 0 {
		delete(s.connections, key)
		if session, ok := s.sessions[key]; ok {
			session.Stop()
			delete(s.sessions, key)
		}
	}
}

// EnsureSession creates or returns an existing console session for a server
func (s *ConsoleService) EnsureSession(serverID, containerID string) {
	s.mu.Lock()
	defer s.mu.Unlock()

	key := StreamKey(serverID)

	// Check if session already exists
	if session, ok := s.sessions[key]; ok {
		// Update container ID if it changed (rare, but possible)
		if session.containerID != containerID {
			session.Stop()
			delete(s.sessions, key)
		} else {
			// Session already exists with correct container ID
			return
		}
	}

	// Only create session if there are subscribers
	if len(s.connections[key]) == 0 {
		return
	}

	// Create new session
	onOutput := func(line string) {
		s.Broadcast(serverID, line)
	}

	onStatus := func(status string) {
		s.BroadcastStatus(serverID, status)
	}

	session := NewConsoleSession(serverID, containerID, s.docker, onOutput, onStatus)
	s.sessions[key] = session

	// Start the session
	ctx := context.Background()
	session.Start(ctx)
}

// StopSession stops a console session (called when server stops)
func (s *ConsoleService) StopSession(serverID string) {
	s.mu.Lock()
	defer s.mu.Unlock()

	key := StreamKey(serverID)
	if session, ok := s.sessions[key]; ok {
		session.Stop()
		delete(s.sessions, key)
	}
}

// SendCommand sends a command to the server's console (direct stdin)
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

// Broadcast sends a message to all subscribers
func (s *ConsoleService) Broadcast(serverID string, message string) {
	logger.Info().Str("server_id", serverID).Str("log", message).Msg("console")

	s.mu.RLock()
	defer s.mu.RUnlock()

	key := StreamKey(serverID)
	for _, ch := range s.connections[key] {
		select {
		case ch <- message:
		default:
			// Channel is full, skip this message
		}
	}
}

// BroadcastStatus sends a status message to all subscribers
func (s *ConsoleService) BroadcastStatus(serverID string, status string) {
	logger.Info().Str("server_id", serverID).Str("status", status).Msg("console status")

	s.mu.RLock()
	defer s.mu.RUnlock()

	key := StreamKey(serverID)
	for _, ch := range s.connections[key] {
		select {
		case ch <- "[status]" + status:
		default:
		}
	}
}
