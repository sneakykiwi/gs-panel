package config

import (
	"os"
	"path/filepath"
	"runtime"
	"strconv"
)

type Config struct {
	Server   ServerConfig
	Database DatabaseConfig
	Storage  StorageConfig
	Docker   DockerConfig
}

type ServerConfig struct {
	Host string
	Port int
}

type DatabaseConfig struct {
	Path string
}

type StorageConfig struct {
	Servers string
	Backups string
}

type DockerConfig struct {
	Socket  string
	Network string
}

func Load() *Config {
	var defaultBaseDir string
	if runtime.GOOS == "windows" {
		defaultBaseDir = filepath.Join(".", "data")
	} else {
		defaultBaseDir = "/var/lib/gs-panel"
	}
	baseDir := getEnv("GS_PANEL_DATA_DIR", defaultBaseDir)

	var defaultSocket string
	if runtime.GOOS == "windows" {
		defaultSocket = "npipe:////./pipe/docker_engine"
	} else {
		defaultSocket = "/var/run/docker.sock"
	}

	return &Config{
		Server: ServerConfig{
			Host: getEnv("GS_PANEL_HOST", "0.0.0.0"),
			Port: getEnvInt("GS_PANEL_PORT", 8080),
		},
		Database: DatabaseConfig{
			Path: getEnv("GS_PANEL_DB_PATH", filepath.Join(baseDir, "panel.db")),
		},
		Storage: StorageConfig{
			Servers: getEnv("GS_PANEL_SERVERS_DIR", filepath.Join(baseDir, "servers")),
			Backups: getEnv("GS_PANEL_BACKUPS_DIR", filepath.Join(baseDir, "backups")),
		},
		Docker: DockerConfig{
			Socket:  getEnv("GS_PANEL_DOCKER_SOCKET", defaultSocket),
			Network: getEnv("GS_PANEL_DOCKER_NETWORK", "gs-panel"),
		},
	}
}

func getEnv(key, defaultValue string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return defaultValue
}

func getEnvInt(key string, defaultValue int) int {
	if value := os.Getenv(key); value != "" {
		if i, err := strconv.Atoi(value); err == nil {
			return i
		}
	}
	return defaultValue
}
