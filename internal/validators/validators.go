package validators

import (
	"regexp"
	"strings"

	"github.com/sneakykiwi/gs-panel/internal/forms"
	"github.com/sneakykiwi/gs-panel/internal/models"
	"github.com/sneakykiwi/gs-panel/internal/services"
)

// serverNamePattern allows alphanumeric, spaces, hyphens, underscores, and periods.
// This prevents shell metacharacters and other potentially dangerous characters
// from being injected into environment variables.
var serverNamePattern = regexp.MustCompile(`^[a-zA-Z0-9 _\-\.]+$`)

// ValidateServerName checks if the server name contains only safe characters.
func ValidateServerName(name string) bool {
	return serverNamePattern.MatchString(name)
}

// SanitizeServerName removes any characters that are not alphanumeric, spaces,
// hyphens, underscores, or periods. Use this when you need to clean a name
// rather than reject it outright.
func SanitizeServerName(name string) string {
	var result strings.Builder
	for _, r := range name {
		if (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') ||
			(r >= '0' && r <= '9') || r == ' ' || r == '_' || r == '-' || r == '.' {
			result.WriteRune(r)
		}
	}
	return result.String()
}

type ServerUpdateValidator struct {
	serverService *services.ServerService
}

func NewServerUpdateValidator(serverService *services.ServerService) *ServerUpdateValidator {
	return &ServerUpdateValidator{serverService: serverService}
}

func (v *ServerUpdateValidator) Validate(form forms.UpdateServer, currentServer *models.Server) map[string]string {
	errors := make(map[string]string)

	if len(form.Name) < 3 || len(form.Name) > 50 {
		errors["name"] = "Name must be between 3 and 50 characters"
	} else if !ValidateServerName(form.Name) {
		errors["name"] = "Name can only contain letters, numbers, spaces, hyphens, underscores, and periods"
	}

	if form.MemoryLimit < 512 || form.MemoryLimit > 16384 {
		errors["memory_limit"] = "Memory must be between 512 and 16384 MB"
	}

	if form.Port < 1024 || form.Port > 65535 {
		errors["port"] = "Port must be between 1024 and 65535"
	}

	return errors
}

type CreateServerValidator struct {
	serverService *services.ServerService
}

func NewCreateServerValidator(serverService *services.ServerService) *CreateServerValidator {
	return &CreateServerValidator{serverService: serverService}
}

func (v *CreateServerValidator) Validate(form forms.CreateServer) map[string]string {
	errors := make(map[string]string)

	if len(form.Name) < 3 || len(form.Name) > 50 {
		errors["name"] = "Name must be between 3 and 50 characters"
	} else if !ValidateServerName(form.Name) {
		errors["name"] = "Name can only contain letters, numbers, spaces, hyphens, underscores, and periods"
	}

	if form.GameType == "" {
		errors["game_type"] = "Game type is required"
	}

	if form.MemoryLimit != 0 && (form.MemoryLimit < 512 || form.MemoryLimit > 16384) {
		errors["memory_limit"] = "Memory must be between 512 and 16384 MB"
	}

	if form.Port != 0 && (form.Port < 1024 || form.Port > 65535) {
		errors["port"] = "Port must be between 1024 and 65535"
	}

	return errors
}
