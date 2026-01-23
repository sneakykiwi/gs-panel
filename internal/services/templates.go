package services

import (
	"encoding/json"
)

type GameTemplate struct {
	ID             string            `json:"id"`
	Name           string            `json:"name"`
	DockerImage    string            `json:"docker_image"`
	DefaultPort    int               `json:"default_port"`
	DefaultMemory  int               `json:"default_memory"`
	Environment    map[string]string `json:"environment"`
	StartupCommand string            `json:"startup_command"`
	StopCommand    string            `json:"stop_command"`
	SaveCommand    string            `json:"save_command"`
}

var defaultTemplates = []GameTemplate{
	{
		ID:            "minecraft-java",
		Name:          "Minecraft: Java Edition",
		DockerImage:   "itzg/minecraft-server:latest",
		DefaultPort:   25565,
		DefaultMemory: 2048,
		Environment: map[string]string{
			"EULA":   "TRUE",
			"TYPE":   "VANILLA",
			"MEMORY": "{{MEMORY}}M",
		},
		StopCommand: "stop",
		SaveCommand: "save-all",
	},
	{
		ID:            "minecraft-bedrock",
		Name:          "Minecraft: Bedrock Edition",
		DockerImage:   "itzg/minecraft-bedrock-server:latest",
		DefaultPort:   19132,
		DefaultMemory: 1024,
		Environment: map[string]string{
			"EULA":       "TRUE",
			"GAMEMODE":   "survival",
			"DIFFICULTY": "normal",
		},
		StopCommand: "stop",
		SaveCommand: "",
	},
	{
		ID:            "terraria",
		Name:          "Terraria",
		DockerImage:   "ryshe/terraria:latest",
		DefaultPort:   7777,
		DefaultMemory: 1024,
		Environment:   map[string]string{},
		StopCommand:   "exit",
		SaveCommand:   "save",
	},
	{
		ID:            "valheim",
		Name:          "Valheim",
		DockerImage:   "lloesche/valheim-server:latest",
		DefaultPort:   2456,
		DefaultMemory: 4096,
		Environment: map[string]string{
			"SERVER_NAME": "My Valheim Server",
			"WORLD_NAME":  "Dedicated",
			"SERVER_PASS": "secret",
		},
		StopCommand: "",
		SaveCommand: "",
	},
	{
		ID:            "palworld",
		Name:          "Palworld",
		DockerImage:   "thijsvanloef/palworld-server-docker:latest",
		DefaultPort:   8211,
		DefaultMemory: 8192,
		Environment: map[string]string{
			"PLAYERS":        "16",
			"MULTITHREADING": "true",
			"COMMUNITY":      "false",
		},
		StopCommand: "",
		SaveCommand: "",
	},
	{
		ID:            "cs2",
		Name:          "Counter-Strike 2",
		DockerImage:   "joedwards32/cs2:latest",
		DefaultPort:   27015,
		DefaultMemory: 4096,
		Environment: map[string]string{
			"CS2_SERVERNAME": "CS2 Server",
			"CS2_PORT":       "27015",
		},
		StopCommand: "quit",
		SaveCommand: "",
	},
	{
		ID:            "zomboid",
		Name:          "Project Zomboid",
		DockerImage:   "renegademaster/zomboid-dedicated-server:latest",
		DefaultPort:   16261,
		DefaultMemory: 4096,
		Environment: map[string]string{
			"SERVER_NAME": "ZomboidServer",
		},
		StopCommand: "quit",
		SaveCommand: "save",
	},
}

type TemplateService struct {
	templates map[string]GameTemplate
}

func NewTemplateService() *TemplateService {
	ts := &TemplateService{
		templates: make(map[string]GameTemplate),
	}
	for _, t := range defaultTemplates {
		ts.templates[t.ID] = t
	}
	return ts
}

func (s *TemplateService) Get(id string) (GameTemplate, bool) {
	t, ok := s.templates[id]
	return t, ok
}

func (s *TemplateService) List() []GameTemplate {
	result := make([]GameTemplate, 0, len(s.templates))
	for _, t := range s.templates {
		result = append(result, t)
	}
	return result
}

func (s *TemplateService) EncodeEnvironment(env map[string]string) string {
	data, _ := json.Marshal(env)
	return string(data)
}

func (s *TemplateService) DecodeEnvironment(data string) map[string]string {
	var env map[string]string
	json.Unmarshal([]byte(data), &env)
	return env
}
