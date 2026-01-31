package validators

import (
	"gs-panel/internal/forms"
	"gs-panel/internal/models"
	"gs-panel/internal/services"
)

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
	}

	if form.MemoryLimit < 512 || form.MemoryLimit > 16384 {
		errors["memory_limit"] = "Memory must be between 512 and 16384 MB"
	}

	if form.Port < 1024 || form.Port > 65535 {
		errors["port"] = "Port must be between 1024 and 65535"
	}

	if form.Port != currentServer.Port {
		if err := v.serverService.ValidatePort(form.Port, currentServer.ID); err != nil {
			errors["port"] = err.Error()
		}
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
