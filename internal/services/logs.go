package services

import (
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/sneakykiwi/gs-panel/internal/logger"

	"github.com/moby/moby/client"
)

type LogFileInfo struct {
	Filename   string    `json:"filename"`
	ServerName string    `json:"server_name"`
	CreatedAt  time.Time `json:"created_at"`
	Size       int64     `json:"size"`
}

type LogService struct {
	basePath string
	docker   *client.Client
}

func NewLogService(basePath string, docker *client.Client) *LogService {
	return &LogService{
		basePath: basePath,
		docker:   docker,
	}
}

func (s *LogService) getLogsDir(serverID string) string {
	return filepath.Join(s.basePath, serverID, "logs")
}

func (s *LogService) SaveContainerLogs(ctx context.Context, serverID, serverName, containerID string, timestamp time.Time) error {
	logsDir := s.getLogsDir(serverID)
	if err := os.MkdirAll(logsDir, 0755); err != nil {
		return fmt.Errorf("failed to create logs directory: %w", err)
	}

	safeName := strings.ReplaceAll(serverName, " ", "_")
	safeName = strings.ReplaceAll(safeName, "/", "_")
	safeName = strings.ReplaceAll(safeName, "\\", "_")

	filename := fmt.Sprintf("%s-%s.log", safeName, timestamp.Format("2006-01-02-15-04"))
	filePath := filepath.Join(logsDir, filename)

	resp, err := s.docker.ContainerLogs(ctx, containerID, client.ContainerLogsOptions{
		ShowStdout: true,
		ShowStderr: true,
		Timestamps: false,
		Tail:       "all",
	})
	if err != nil {
		return fmt.Errorf("failed to fetch container logs: %w", err)
	}
	defer resp.Close()

	file, err := os.Create(filePath)
	if err != nil {
		return fmt.Errorf("failed to create log file: %w", err)
	}
	defer file.Close()

	written, err := io.Copy(file, resp)
	if err != nil {
		return fmt.Errorf("failed to write logs to file: %w", err)
	}

	logger.Info().
		Str("server_id", serverID).
		Str("container_id", containerID).
		Str("filename", filename).
		Int64("size", written).
		Msg("Saved container logs to file")

	return nil
}

func (s *LogService) ListSavedLogs(serverID string) ([]LogFileInfo, error) {
	logsDir := s.getLogsDir(serverID)

	entries, err := os.ReadDir(logsDir)
	if err != nil {
		if os.IsNotExist(err) {
			return []LogFileInfo{}, nil // No logs yet
		}
		return nil, fmt.Errorf("failed to read logs directory: %w", err)
	}

	var logs []LogFileInfo
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}

		if !strings.HasSuffix(entry.Name(), ".log") {
			continue
		}

		info, err := entry.Info()
		if err != nil {
			logger.Warn().Str("filename", entry.Name()).Err(err).Msg("Failed to get file info")
			continue
		}

		logFile := LogFileInfo{
			Filename:  entry.Name(),
			CreatedAt: info.ModTime(),
			Size:      info.Size(),
		}

		parts := strings.Split(entry.Name(), "-")
		if len(parts) >= 2 {
			logFile.ServerName = strings.Join(parts[:len(parts)-4], "-")
		}

		logs = append(logs, logFile)
	}

	sort.Slice(logs, func(i, j int) bool {
		return logs[i].CreatedAt.After(logs[j].CreatedAt)
	})

	return logs, nil
}

func (s *LogService) ReadLogFile(serverID, filename string) (string, error) {
	if !strings.HasSuffix(filename, ".log") {
		return "", fmt.Errorf("invalid log filename")
	}

	filePath := filepath.Join(s.getLogsDir(serverID), filename)

	content, err := os.ReadFile(filePath)
	if err != nil {
		if os.IsNotExist(err) {
			return "", fmt.Errorf("log file not found")
		}
		return "", fmt.Errorf("failed to read log file: %w", err)
	}

	return string(content), nil
}

func (s *LogService) GetLogFilePath(serverID, filename string) (string, error) {
	if !strings.HasSuffix(filename, ".log") {
		return "", fmt.Errorf("invalid log filename")
	}

	filePath := filepath.Join(s.getLogsDir(serverID), filename)

	if _, err := os.Stat(filePath); err != nil {
		if os.IsNotExist(err) {
			return "", fmt.Errorf("log file not found")
		}
		return "", fmt.Errorf("failed to access log file: %w", err)
	}

	return filePath, nil
}
