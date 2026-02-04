package services

import (
	"gopkg.in/yaml.v3"
	"testing"
)

func TestVolumeConfig_UnmarshalYAML(t *testing.T) {
	tests := []struct {
		name      string
		yaml      string
		wantHost  string
		wantCont  string
		wantMode  string
		wantError bool
	}{
		{
			name:     "short syntax without mode",
			yaml:     `"{{SERVER_DIR}}:/config"`,
			wantHost: "{{SERVER_DIR}}",
			wantCont: "/config",
			wantMode: "rw",
		},
		{
			name:     "short syntax with mode",
			yaml:     `"/host:/container:ro"`,
			wantHost: "/host",
			wantCont: "/container",
			wantMode: "ro",
		},
		{
			name:     "object format",
			yaml:     `{"host": "/host", "container": "/container", "mode": "rw"}`,
			wantHost: "/host",
			wantCont: "/container",
			wantMode: "rw",
		},
		{
			name:     "object format without mode",
			yaml:     `{"host": "/host", "container": "/container"}`,
			wantHost: "/host",
			wantCont: "/container",
			wantMode: "rw",
		},
		{
			name:      "invalid short syntax",
			yaml:      `"onlyonepart"`,
			wantError: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var vol VolumeConfig
			err := yaml.Unmarshal([]byte(tt.yaml), &vol)

			if tt.wantError {
				if err == nil {
					t.Errorf("expected error but got none")
				}
				return
			}

			if err != nil {
				t.Errorf("unexpected error: %v", err)
				return
			}

			if vol.Host != tt.wantHost {
				t.Errorf("Host = %v, want %v", vol.Host, tt.wantHost)
			}
			if vol.Container != tt.wantCont {
				t.Errorf("Container = %v, want %v", vol.Container, tt.wantCont)
			}
			if vol.Mode != tt.wantMode {
				t.Errorf("Mode = %v, want %v", vol.Mode, tt.wantMode)
			}
		})
	}
}

func TestGameTemplate_UnmarshalVolumes(t *testing.T) {
	yamlContent := `
id: satisfactory
name: Satisfactory Server
version: "1.0.0"
docker_image: wolveix/satisfactory-server:latest
default_port: 7777
default_memory: 8192
protocol: both

environment:
  SERVERGAMEPORT: "{{PORT}}"

volumes:
  - "{{SERVER_DIR}}:/config"
  - "/data:/container:ro"

additional_ports:
  - port: 8888
    protocol: tcp

stop_command: quit
stop_timeout: 60
`

	var template GameTemplate
	err := yaml.Unmarshal([]byte(yamlContent), &template)

	if err != nil {
		t.Fatalf("Failed to unmarshal template: %v", err)
	}

	if len(template.Volumes) != 2 {
		t.Fatalf("Expected 2 volumes, got %d", len(template.Volumes))
	}

	if template.Volumes[0].Host != "{{SERVER_DIR}}" {
		t.Errorf("Volume 0 Host = %v, want {{SERVER_DIR}}", template.Volumes[0].Host)
	}
	if template.Volumes[0].Container != "/config" {
		t.Errorf("Volume 0 Container = %v, want /config", template.Volumes[0].Container)
	}
	if template.Volumes[0].Mode != "rw" {
		t.Errorf("Volume 0 Mode = %v, want rw", template.Volumes[0].Mode)
	}

	if template.Volumes[1].Host != "/data" {
		t.Errorf("Volume 1 Host = %v, want /data", template.Volumes[1].Host)
	}
	if template.Volumes[1].Container != "/container" {
		t.Errorf("Volume 1 Container = %v, want /container", template.Volumes[1].Container)
	}
	if template.Volumes[1].Mode != "ro" {
		t.Errorf("Volume 1 Mode = %v, want ro", template.Volumes[1].Mode)
	}
}
