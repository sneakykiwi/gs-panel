package services

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"sync"
	"time"

	"github.com/sneakykiwi/gs-panel/internal/logger"
	"gopkg.in/yaml.v3"
)

type PortConfig struct {
	Port     int    `json:"port" yaml:"port"`
	Protocol string `json:"protocol" yaml:"protocol"`
	Purpose  string `json:"purpose" yaml:"purpose"`
}

type VolumeConfig struct {
	Host      string `json:"host" yaml:"host"`
	Container string `json:"container" yaml:"container"`
	Mode      string `json:"mode" yaml:"mode"`
}

type HealthCheckConfig struct {
	Test        []string      `json:"test" yaml:"test"`
	Interval    time.Duration `json:"interval" yaml:"interval"`
	Timeout     time.Duration `json:"timeout" yaml:"timeout"`
	Retries     int           `json:"retries" yaml:"retries"`
	StartPeriod time.Duration `json:"start_period" yaml:"start_period"`
}

type GameTemplate struct {
	ID              string             `json:"id" yaml:"id"`
	Name            string             `json:"name" yaml:"name"`
	Version         string             `json:"version" yaml:"version"`
	DockerImage     string             `json:"docker_image" yaml:"docker_image"`
	DefaultPort     int                `json:"default_port" yaml:"default_port"`
	DefaultMemory   int                `json:"default_memory" yaml:"default_memory"`
	Protocol        string             `json:"protocol" yaml:"protocol"`
	Environment     map[string]string  `json:"environment" yaml:"environment"`
	StartupCommand  string             `json:"startup_command" yaml:"startup_command"`
	StopCommand     string             `json:"stop_command" yaml:"stop_command"`
	SaveCommand     string             `json:"save_command" yaml:"save_command"`
	AdditionalPorts []PortConfig       `json:"additional_ports" yaml:"additional_ports"`
	Volumes         []VolumeConfig     `json:"volumes" yaml:"volumes"`
	Entrypoint      []string           `json:"entrypoint" yaml:"entrypoint"`
	Cmd             []string           `json:"cmd" yaml:"cmd"`
	HealthCheck     *HealthCheckConfig `json:"health_check" yaml:"health_check"`
	User            string             `json:"user" yaml:"user"`
	CapAdd          []string           `json:"cap_add" yaml:"cap_add"`
	NetworkMode     string             `json:"network_mode" yaml:"network_mode"`
	Labels          map[string]string  `json:"labels" yaml:"labels"`
	StopSignal      string             `json:"stop_signal" yaml:"stop_signal"`
	StopTimeout     int                `json:"stop_timeout" yaml:"stop_timeout"`
	RequiredVars    []string           `json:"required_vars" yaml:"required_vars"`
	VarDescriptions map[string]string  `json:"var_descriptions" yaml:"var_descriptions"`
	IsBuiltIn       bool               `json:"-" yaml:"-"`
}

var defaultTemplates = []GameTemplate{
	{
		ID:            "minecraft-java",
		Name:          "Minecraft: Java Edition",
		Version:       "1.0.0",
		DockerImage:   "itzg/minecraft-server:latest",
		DefaultPort:   25565,
		DefaultMemory: 2048,
		Protocol:      "both",
		Environment: map[string]string{
			"EULA":       "TRUE",
			"TYPE":       "VANILLA",
			"MAX_MEMORY": "{{MEMORY}}M",
		},
		StopCommand: "stop",
		SaveCommand: "save-all",
		StopTimeout: 30,
		IsBuiltIn:   true,
	},
	{
		ID:            "minecraft-bedrock",
		Name:          "Minecraft: Bedrock Edition",
		Version:       "1.0.0",
		DockerImage:   "itzg/minecraft-bedrock-server:latest",
		DefaultPort:   19132,
		DefaultMemory: 1024,
		Protocol:      "udp",
		Environment: map[string]string{
			"EULA":       "TRUE",
			"GAMEMODE":   "survival",
			"DIFFICULTY": "normal",
		},
		StopCommand: "stop",
		StopTimeout: 30,
		IsBuiltIn:   true,
	},
	{
		ID:            "terraria",
		Name:          "Terraria",
		Version:       "1.0.0",
		DockerImage:   "ryshe/terraria:latest",
		DefaultPort:   7777,
		DefaultMemory: 1024,
		Protocol:      "tcp",
		Environment:   map[string]string{},
		StopCommand:   "exit",
		SaveCommand:   "save",
		StopTimeout:   30,
		IsBuiltIn:     true,
	},
	{
		ID:            "valheim",
		Name:          "Valheim",
		Version:       "1.0.0",
		DockerImage:   "lloesche/valheim-server:latest",
		DefaultPort:   2456,
		DefaultMemory: 4096,
		Protocol:      "udp",
		Environment: map[string]string{
			"SERVER_NAME": "My Valheim Server",
			"WORLD_NAME":  "Dedicated",
			"SERVER_PASS": "secret",
		},
		AdditionalPorts: []PortConfig{
			{Port: 2457, Protocol: "udp", Purpose: "query"},
		},
		StopTimeout: 30,
		IsBuiltIn:   true,
	},
	{
		ID:            "palworld",
		Name:          "Palworld",
		Version:       "1.0.0",
		DockerImage:   "thijsvanloef/palworld-server-docker:latest",
		DefaultPort:   8211,
		DefaultMemory: 8192,
		Protocol:      "udp",
		Environment: map[string]string{
			"PLAYERS":        "16",
			"MULTITHREADING": "true",
			"COMMUNITY":      "false",
		},
		StopTimeout: 30,
		IsBuiltIn:   true,
	},
}

var (
	ErrTemplateNotFound  = fmt.Errorf("template not found")
	ErrInvalidTemplate   = fmt.Errorf("invalid template")
	ErrInvalidMountPath  = fmt.Errorf("invalid mount path")
	ErrInvalidCapability = fmt.Errorf("invalid capability")
	mountPathRegex       = regexp.MustCompile(`^[a-zA-Z0-9_/.-]+$`)
)

type SecurityConfig struct {
	AllowedMountPrefixes []string
	AllowedCapAdds       []string
	EnablePerUserMounts  bool
}

type TemplateService struct {
	templates    map[string]GameTemplate
	templatesDir string
	security     *SecurityConfig
	mu           sync.RWMutex
}

func NewTemplateService(templatesDir string, security *SecurityConfig) *TemplateService {
	if security == nil {
		security = &SecurityConfig{
			AllowedCapAdds: []string{"SYS_NICE", "NET_BIND_SERVICE"},
		}
	}

	ts := &TemplateService{
		templates:    make(map[string]GameTemplate),
		templatesDir: templatesDir,
		security:     security,
	}

	for _, t := range defaultTemplates {
		ts.templates[t.ID] = t
	}

	if templatesDir != "" {
		ts.loadTemplatesFromDir()
	}

	return ts
}

func (s *TemplateService) loadTemplatesFromDir() {
	if s.templatesDir == "" {
		return
	}

	entries, err := os.ReadDir(s.templatesDir)
	if err != nil {
		if !os.IsNotExist(err) {
			logger.Error().Err(err).Str("dir", s.templatesDir).Msg("Failed to read templates directory")
		}
		return
	}

	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}

		ext := strings.ToLower(filepath.Ext(entry.Name()))
		if ext != ".yaml" && ext != ".yml" && ext != ".json" {
			continue
		}

		filePath := filepath.Join(s.templatesDir, entry.Name())
		template, err := s.loadTemplateFile(filePath)
		if err != nil {
			logger.Error().Err(err).Str("file", filePath).Msg("Failed to load template file")
			continue
		}

		if err := s.validateTemplateFields(&template); err != nil {
			logger.Error().Err(err).Str("file", filePath).Msg("Invalid template")
			continue
		}

		s.templates[template.ID] = template
		logger.Info().Str("id", template.ID).Str("name", template.Name).Msg("Loaded template")
	}
}

func (s *TemplateService) loadTemplateFile(filePath string) (GameTemplate, error) {
	data, err := os.ReadFile(filePath)
	if err != nil {
		return GameTemplate{}, err
	}

	var template GameTemplate
	ext := strings.ToLower(filepath.Ext(filePath))

	switch ext {
	case ".yaml", ".yml":
		err = yaml.Unmarshal(data, &template)
	case ".json":
		err = json.Unmarshal(data, &template)
	default:
		return GameTemplate{}, fmt.Errorf("unsupported format: %s", ext)
	}

	if err != nil {
		return GameTemplate{}, err
	}

	return template, nil
}

func (s *TemplateService) validateTemplateFields(t *GameTemplate) error {
	if t.ID == "" {
		return fmt.Errorf("%w: missing id", ErrInvalidTemplate)
	}
	if t.DockerImage == "" {
		return fmt.Errorf("%w: missing docker_image", ErrInvalidTemplate)
	}
	if t.Version == "" {
		t.Version = "1.0.0"
	}
	if t.Protocol == "" {
		t.Protocol = "both"
	}
	if t.StopTimeout <= 0 {
		t.StopTimeout = 30
	}
	return nil
}

func (s *TemplateService) ValidateTemplate(t *GameTemplate, serverPath string) error {
	for _, vol := range t.Volumes {
		if !mountPathRegex.MatchString(vol.Host) || !mountPathRegex.MatchString(vol.Container) {
			return fmt.Errorf("%w: %s", ErrInvalidMountPath, vol.Host)
		}

		hostPath := vol.Host
		if !filepath.IsAbs(hostPath) {
			hostPath = filepath.Join(serverPath, hostPath)
		}
		hostPath = filepath.Clean(hostPath)

		allowed := false
		for _, prefix := range s.security.AllowedMountPrefixes {
			if strings.HasPrefix(hostPath, prefix) {
				allowed = true
				break
			}
		}
		if !allowed && len(s.security.AllowedMountPrefixes) > 0 {
			return fmt.Errorf("%w: %s not in allowed prefixes", ErrInvalidMountPath, hostPath)
		}
	}

	for _, capName := range t.CapAdd {
		capUpper := strings.ToUpper(capName)
		if capUpper == "ALL" || capUpper == "SYS_ADMIN" {
			return fmt.Errorf("%w: %s is not allowed", ErrInvalidCapability, capName)
		}

		allowed := false
		for _, allowedCap := range s.security.AllowedCapAdds {
			if capUpper == strings.ToUpper(allowedCap) {
				allowed = true
				break
			}
		}
		if !allowed && len(s.security.AllowedCapAdds) > 0 {
			return fmt.Errorf("%w: %s not in allowed list", ErrInvalidCapability, capName)
		}
	}

	return nil
}

func (s *TemplateService) ReloadTemplates() error {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.templates = make(map[string]GameTemplate)
	for _, t := range defaultTemplates {
		s.templates[t.ID] = t
	}

	s.loadTemplatesFromDir()
	return nil
}

func (s *TemplateService) Get(id string) (GameTemplate, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	t, ok := s.templates[id]
	return t, ok
}

func (s *TemplateService) List() []GameTemplate {
	s.mu.RLock()
	defer s.mu.RUnlock()
	result := make([]GameTemplate, 0, len(s.templates))
	for _, t := range s.templates {
		result = append(result, t)
	}
	return result
}

func (s *TemplateService) Add(t GameTemplate) error {
	if err := s.validateTemplateFields(&t); err != nil {
		return err
	}

	s.mu.Lock()
	defer s.mu.Unlock()
	s.templates[t.ID] = t
	return nil
}

func (s *TemplateService) Delete(id string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	t, ok := s.templates[id]
	if !ok {
		return ErrTemplateNotFound
	}
	if t.IsBuiltIn {
		return fmt.Errorf("cannot delete built-in template")
	}

	delete(s.templates, id)
	return nil
}

func (s *TemplateService) SaveToFile(t GameTemplate) error {
	if s.templatesDir == "" {
		return fmt.Errorf("templates directory not configured")
	}

	if err := os.MkdirAll(s.templatesDir, 0755); err != nil {
		return err
	}

	filePath := filepath.Join(s.templatesDir, t.ID+".yaml")
	data, err := yaml.Marshal(t)
	if err != nil {
		return err
	}

	return os.WriteFile(filePath, data, 0644)
}

func (s *TemplateService) DeleteFile(id string) error {
	if s.templatesDir == "" {
		return fmt.Errorf("templates directory not configured")
	}

	for _, ext := range []string{".yaml", ".yml", ".json"} {
		filePath := filepath.Join(s.templatesDir, id+ext)
		if _, err := os.Stat(filePath); err == nil {
			return os.Remove(filePath)
		}
	}

	return nil
}

func (s *TemplateService) EncodeEnvironment(env map[string]string) string {
	data, _ := json.Marshal(env)
	return string(data)
}

func (s *TemplateService) DecodeEnvironment(data string) map[string]string {
	var env map[string]string
	if err := json.Unmarshal([]byte(data), &env); err != nil {
		return make(map[string]string)
	}
	return env
}
