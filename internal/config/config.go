package config

import (
	"os"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
)

type Config struct {
	Server   ServerConfig
	Database DatabaseConfig
	Storage  StorageConfig
	Docker   DockerConfig
	Security SecurityConfig
}

type ServerConfig struct {
	Host string
	Port int
}

type DatabaseConfig struct {
	Path string
}

type StorageConfig struct {
	Servers   string
	Backups   string
	Logs      string
	Templates string
}

type DockerConfig struct {
	Socket  string
	Network string
}

type SecurityConfig struct {
	AllowedMountPrefixes []string
	AllowedCapAdds       []string
	EnableImageScanning  bool
	Scanner              string
	VulnThreshold        string
	EnablePerUserMounts  bool
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

	serversDir := getEnv("GS_PANEL_SERVERS_DIR", filepath.Join(baseDir, "servers"))

	return &Config{
		Server: ServerConfig{
			Host: getEnv("GS_PANEL_HOST", "0.0.0.0"),
			Port: getEnvInt("GS_PANEL_PORT", 8080),
		},
		Database: DatabaseConfig{
			Path: getEnv("GS_PANEL_DB_PATH", filepath.Join(baseDir, "panel.db")),
		},
		Storage: StorageConfig{
			Servers:   serversDir,
			Backups:   getEnv("GS_PANEL_BACKUPS_DIR", filepath.Join(baseDir, "backups")),
			Logs:      getEnv("GS_PANEL_LOGS_DIR", filepath.Join(baseDir, "logs")),
			Templates: getEnv("GS_PANEL_TEMPLATES_DIR", filepath.Join(baseDir, "templates")),
		},
		Docker: DockerConfig{
			Socket:  getEnv("GS_PANEL_DOCKER_SOCKET", defaultSocket),
			Network: getEnv("GS_PANEL_DOCKER_NETWORK", "gs-panel"),
		},
		Security: SecurityConfig{
			AllowedMountPrefixes: getEnvList("GS_PANEL_ALLOWED_MOUNT_PREFIXES", []string{serversDir}),
			AllowedCapAdds:       getEnvList("GS_PANEL_ALLOWED_CAP_ADDS", []string{"SYS_NICE", "NET_BIND_SERVICE"}),
			EnableImageScanning:  getEnvBool("GS_PANEL_ENABLE_IMAGE_SCANNING", false),
			Scanner:              getEnv("GS_PANEL_SCANNER", "trivy"),
			VulnThreshold:        getEnv("GS_PANEL_VULN_THRESHOLD", "HIGH"),
			EnablePerUserMounts:  getEnvBool("GS_PANEL_ENABLE_PER_USER_MOUNTS", true),
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
		value = strings.TrimSpace(value)
		if i, err := strconv.Atoi(value); err == nil {
			return i
		}
	}
	return defaultValue
}

func getEnvBool(key string, defaultValue bool) bool {
	if value := os.Getenv(key); value != "" {
		value = strings.ToLower(strings.TrimSpace(value))
		return value == "true" || value == "1" || value == "yes"
	}
	return defaultValue
}

func getEnvList(key string, defaultValue []string) []string {
	if value := os.Getenv(key); value != "" {
		parts := strings.Split(value, ",")
		result := make([]string, 0, len(parts))
		for _, p := range parts {
			p = strings.TrimSpace(p)
			if p != "" {
				result = append(result, p)
			}
		}
		if len(result) > 0 {
			return result
		}
	}
	return defaultValue
}
