package services

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
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

func (v *VolumeConfig) UnmarshalYAML(unmarshal func(interface{}) error) error {
	var str string
	if err := unmarshal(&str); err == nil {
		parts := strings.Split(str, ":")
		if len(parts) >= 2 {
			v.Host = parts[0]
			v.Container = parts[1]
			v.Mode = "rw"
			if len(parts) == 3 {
				v.Mode = parts[2]
			}
			return nil
		}
		return fmt.Errorf("invalid volume format: %s", str)
	}

	var obj struct {
		Host      string `yaml:"host"`
		Container string `yaml:"container"`
		Mode      string `yaml:"mode"`
	}
	if err := unmarshal(&obj); err != nil {
		return err
	}
	v.Host = obj.Host
	v.Container = obj.Container
	v.Mode = obj.Mode
	if v.Mode == "" {
		v.Mode = "rw"
	}
	return nil
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

var (
	ErrTemplateNotFound  = fmt.Errorf("template not found")
	ErrInvalidTemplate   = fmt.Errorf("invalid template")
	ErrInvalidMountPath  = fmt.Errorf("invalid mount path")
	ErrInvalidCapability = fmt.Errorf("invalid capability")
	ErrInvalidTemplateID = fmt.Errorf("invalid template ID")
	mountPathRegex       = regexp.MustCompile(`^[a-zA-Z0-9_\\/.:-]+$`)
	templateIDRegex      = regexp.MustCompile(`^[a-zA-Z0-9_-]+$`)
)

type SecurityConfig struct {
	AllowedMountPrefixes []string
	AllowedCapAdds       []string
	EnablePerUserMounts  bool
}

type TemplateService struct {
	templates           map[string]GameTemplate
	defaultTemplatesDir string
	userTemplatesDir    string
	security            *SecurityConfig
	mu                  sync.RWMutex
}

func NewTemplateService(defaultTemplatesDir, userTemplatesDir string, security *SecurityConfig) *TemplateService {
	if security == nil {
		security = &SecurityConfig{
			AllowedCapAdds: []string{"SYS_NICE", "NET_BIND_SERVICE"},
		}
	}

	ts := &TemplateService{
		templates:           make(map[string]GameTemplate),
		defaultTemplatesDir: defaultTemplatesDir,
		userTemplatesDir:    userTemplatesDir,
		security:            security,
	}

	ts.loadTemplatesFromDir(defaultTemplatesDir, true)
	ts.loadTemplatesFromDir(userTemplatesDir, false)

	return ts
}

func (s *TemplateService) loadTemplatesFromDir(dir string, isBuiltIn bool) {
	if dir == "" {
		return
	}

	entries, err := os.ReadDir(dir)
	if err != nil {
		if !os.IsNotExist(err) {
			logger.Error().Err(err).Str("dir", dir).Msg("Failed to read templates directory")
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

		filePath := filepath.Join(dir, entry.Name())
		template, err := s.loadTemplateFile(filePath)
		if err != nil {
			logger.Error().Err(err).Str("file", filePath).Msg("Failed to load template file")
			continue
		}

		if err := s.validateTemplateFields(&template); err != nil {
			logger.Error().Err(err).Str("file", filePath).Msg("Invalid template")
			continue
		}

		template.IsBuiltIn = isBuiltIn
		s.templates[template.ID] = template
		logger.Info().Str("id", template.ID).Str("name", template.Name).Bool("builtin", isBuiltIn).Msg("Loaded template")
	}
}

func (s *TemplateService) loadTemplateFile(filePath string) (GameTemplate, error) {
	data, err := os.ReadFile(filePath)
	if err != nil {
		return GameTemplate{}, err
	}

	var template GameTemplate
	var raw map[string]interface{}
	ext := strings.ToLower(filepath.Ext(filePath))

	switch ext {
	case ".yaml", ".yml":
		err = yaml.Unmarshal(data, &template)
		if err == nil {
			err = yaml.Unmarshal(data, &raw)
		}
	case ".json":
		err = json.Unmarshal(data, &template)
		if err == nil {
			err = json.Unmarshal(data, &raw)
		}
	default:
		return GameTemplate{}, fmt.Errorf("unsupported format: %s", ext)
	}

	if err != nil {
		return GameTemplate{}, err
	}

	// Parse volumes using helper if present
	if raw != nil {
		if v, ok := raw["volumes"]; ok {
			template.Volumes = ParseVolumes(v)
		}
	}

	return template, nil
}

func (s *TemplateService) validateTemplateFields(t *GameTemplate) error {
	if t.ID == "" {
		return fmt.Errorf("%w: missing id", ErrInvalidTemplate)
	}
	// Validate template ID to prevent XSS attacks
	if !templateIDRegex.MatchString(t.ID) {
		return fmt.Errorf("%w: template ID must only contain alphanumeric characters, hyphens, and underscores", ErrInvalidTemplateID)
	}
	if len(t.ID) > 64 {
		return fmt.Errorf("%w: template ID must not exceed 64 characters", ErrInvalidTemplateID)
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

// Template variable replacement helper
func ReplaceTemplateVars(input, serverID, serverName, serverPath, backupPath, logsPath string, memory, port int) string {
	input = strings.ReplaceAll(input, "{{SERVER_ID}}", serverID)
	input = strings.ReplaceAll(input, "{{SERVER_NAME}}", serverName)
	input = strings.ReplaceAll(input, "{{SERVER_DIR}}", serverPath)
	input = strings.ReplaceAll(input, "{{BACKUP_DIR}}", backupPath)
	input = strings.ReplaceAll(input, "{{LOGS_DIR}}", logsPath)
	input = strings.ReplaceAll(input, "{{MEMORY}}", strconv.Itoa(memory))
	input = strings.ReplaceAll(input, "{{PORT}}", strconv.Itoa(port))
	return input
}

func ParseVolumes(raw interface{}) []VolumeConfig {
	var volumes []VolumeConfig
	switch v := raw.(type) {
	case []interface{}:
		for _, item := range v {
			switch val := item.(type) {
			case string:
				// Short syntax: "host:container[:mode]"
				parts := strings.Split(val, ":")
				if len(parts) >= 2 {
					vol := VolumeConfig{
						Host:      parts[0],
						Container: parts[1],
						Mode:      "rw",
					}
					if len(parts) == 3 {
						vol.Mode = parts[2]
					}
					volumes = append(volumes, vol)
				}
			case map[string]interface{}:
				host, _ := val["host"].(string)
				container, _ := val["container"].(string)
				mode := "rw"
				if m, ok := val["mode"].(string); ok {
					mode = m
				}
				vol := VolumeConfig{
					Host:      host,
					Container: container,
					Mode:      mode,
				}
				volumes = append(volumes, vol)
			}
		}
	case []VolumeConfig:
		volumes = v
	}
	return volumes
}

func (s *TemplateService) ValidateTemplate(t *GameTemplate, serverPath string) error {
	backupPath := filepath.Join(serverPath, "../backups")
	logsPath := filepath.Join(serverPath, "../logs")
	for _, vol := range t.Volumes {
		hostPath := ReplaceTemplateVars(vol.Host, t.ID, t.Name, serverPath, backupPath, logsPath, t.DefaultMemory, t.DefaultPort)
		if !filepath.IsAbs(hostPath) {
			hostPath = filepath.Join(serverPath, hostPath)
		}
		hostPath = filepath.Clean(hostPath)

		if !mountPathRegex.MatchString(hostPath) || !mountPathRegex.MatchString(vol.Container) {
			return fmt.Errorf("%w: host=%s container=%s", ErrInvalidMountPath, hostPath, vol.Container)
		}

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
	s.loadTemplatesFromDir(s.defaultTemplatesDir, true)
	s.loadTemplatesFromDir(s.userTemplatesDir, false)
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
	sort.Slice(result, func(i, j int) bool {
		if result[i].IsBuiltIn != result[j].IsBuiltIn {
			return result[i].IsBuiltIn
		}
		return result[i].Name < result[j].Name
	})
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

func (s *TemplateService) AddWithUniqueID(t GameTemplate) (GameTemplate, error) {
	if err := s.validateTemplateFields(&t); err != nil {
		return t, err
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	baseID := t.ID
	counter := 1
	for {
		if _, exists := s.templates[t.ID]; !exists {
			break
		}
		t.ID = fmt.Sprintf("%s-%d", baseID, counter)
		counter++
	}

	s.templates[t.ID] = t
	return t, nil
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
	if s.userTemplatesDir == "" {
		return fmt.Errorf("user templates directory not configured")
	}

	// prevent path traversal (e.g., "../../../etc/passwd")
	if !templateIDRegex.MatchString(t.ID) {
		return fmt.Errorf("%w: template ID contains invalid characters", ErrInvalidTemplateID)
	}
	if len(t.ID) > 64 {
		return fmt.Errorf("%w: template ID must not exceed 64 characters", ErrInvalidTemplateID)
	}

	if err := os.MkdirAll(s.userTemplatesDir, 0755); err != nil {
		return err
	}

	filePath := filepath.Join(s.userTemplatesDir, t.ID+".yaml")
	data, err := yaml.Marshal(t)
	if err != nil {
		return err
	}

	return os.WriteFile(filePath, data, 0644)
}

func (s *TemplateService) DeleteFile(id string) error {
	if s.userTemplatesDir == "" {
		return fmt.Errorf("user templates directory not configured")
	}

	for _, ext := range []string{".yaml", ".yml", ".json"} {
		filePath := filepath.Join(s.userTemplatesDir, id+ext)
		if _, err := os.Stat(filePath); err == nil {
			return os.Remove(filePath)
		}
	}

	return fmt.Errorf("no template file found for id: %s", id)
}

func (s *TemplateService) EncodeEnvironment(env map[string]string) string {
	data, err := json.Marshal(env)
	if err != nil {
		logger.Warn().Err(err).Msg("Failed to marshal environment variables")
		return "{}"
	}
	return string(data)
}

func (s *TemplateService) DecodeEnvironment(data string) map[string]string {
	var env map[string]string
	if err := json.Unmarshal([]byte(data), &env); err != nil {
		return make(map[string]string)
	}
	return env
}

func (s *TemplateService) TemplateToYAML(t GameTemplate) string {
	t.IsBuiltIn = false
	data, err := yaml.Marshal(t)
	if err != nil {
		return ""
	}
	return string(data)
}

func (s *TemplateService) ParseYAML(yamlContent string, t *GameTemplate) error {
	var raw map[string]interface{}
	if err := yaml.Unmarshal([]byte(yamlContent), t); err != nil {
		return err
	}
	if err := yaml.Unmarshal([]byte(yamlContent), &raw); err == nil {
		if v, ok := raw["volumes"]; ok {
			t.Volumes = ParseVolumes(v)
		}
	}
	return nil
}
