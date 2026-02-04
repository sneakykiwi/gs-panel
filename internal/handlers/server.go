package handlers

import (
	"errors"
	"fmt"
	"path/filepath"
	"strings"

	"github.com/sneakykiwi/gs-panel/internal/config"
	"github.com/sneakykiwi/gs-panel/internal/forms"
	"github.com/sneakykiwi/gs-panel/internal/middleware"
	"github.com/sneakykiwi/gs-panel/internal/models"
	"github.com/sneakykiwi/gs-panel/internal/services"
	"github.com/sneakykiwi/gs-panel/internal/validators"
	"github.com/sneakykiwi/gs-panel/views/pages"
	"github.com/sneakykiwi/gs-panel/views/pages/servers"
	"github.com/sneakykiwi/gs-panel/views/partials"

	"github.com/gofiber/fiber/v3"
)

type ServerHandler struct {
	serverService   *services.ServerService
	templateService *services.TemplateService
	cfg             *config.Config
}

func NewServerHandler(serverService *services.ServerService, templateService *services.TemplateService, cfg *config.Config) *ServerHandler {
	return &ServerHandler{
		serverService:   serverService,
		templateService: templateService,
		cfg:             cfg,
	}
}

func (h *ServerHandler) Dashboard(c fiber.Ctx) error {
	user := middleware.GetUser(c)
	var serverList []models.Server
	var err error

	if user.IsAdmin {
		serverList, err = h.serverService.List()
	} else {
		serverList, err = h.serverService.ListForUser(user.ID)
	}
	if err != nil {
		return err
	}

	return Render(c, pages.DashboardPage(user, serverList))
}

func (h *ServerHandler) CreatePage(c fiber.Ctx) error {
	user := middleware.GetUser(c)
	return Render(c, servers.CreatePage(user, h.templateService.List(), ""))
}

func (h *ServerHandler) Create(c fiber.Ctx) error {
	user := middleware.GetUser(c)
	templates := h.templateService.List()

	var form forms.CreateServer
	if err := c.Bind().Form(&form); err != nil {
		return Render(c, servers.CreatePage(user, templates, "Invalid form data"))
	}

	validator := validators.NewCreateServerValidator(h.serverService)
	validationErrors := validator.Validate(form)
	if len(validationErrors) > 0 {
		errorMsg := ""
		for _, msg := range validationErrors {
			errorMsg = msg
			break
		}
		return Render(c, servers.CreatePage(user, templates, errorMsg))
	}

	template, _ := h.templateService.Get(form.GameType)
	customVars := make(map[string]string)

	editMode := c.FormValue("edit_mode")
	if editMode == "yaml" {
		envYaml := c.FormValue("env_yaml")
		if envYaml != "" {
			customVars = parseEnvYAML(envYaml)
		}
	} else {
		for key := range template.Environment {
			if val := c.FormValue("env_" + key); val != "" {
				customVars[key] = val
			}
		}
	}

	_, err := h.serverService.Create(services.CreateServerRequest{
		Name:        form.Name,
		GameType:    form.GameType,
		MemoryLimit: form.MemoryLimit,
		Port:        form.Port,
		CustomVars:  customVars,
	})
	if err != nil {
		return Render(c, servers.CreatePage(user, templates, err.Error()))
	}

	return c.Redirect().To("/")
}

func parseEnvYAML(yaml string) map[string]string {
	result := make(map[string]string)
	lines := strings.Split(yaml, "\n")
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		parts := strings.SplitN(line, ":", 2)
		if len(parts) == 2 {
			key := strings.TrimSpace(parts[0])
			value := strings.TrimSpace(parts[1])
			if key != "" {
				result[key] = value
			}
		}
	}
	return result
}

func (h *ServerHandler) CreateAdvancedPage(c fiber.Ctx) error {
	user := middleware.GetUser(c)
	templateID := c.Query("template")

	if templateID == "" {
		return c.Redirect().To("/servers/new")
	}

	template, ok := h.templateService.Get(templateID)
	if !ok {
		return c.Redirect().To("/servers/new")
	}

	yamlContent := h.templateService.TemplateToYAML(template)
	return Render(c, servers.CreateAdvancedPage(user, template, yamlContent, ""))
}

func (h *ServerHandler) CreateAdvanced(c fiber.Ctx) error {
	user := middleware.GetUser(c)
	sourceTemplateID := c.FormValue("source_template")

	sourceTemplate, ok := h.templateService.Get(sourceTemplateID)
	if !ok {
		return c.Redirect().To("/servers/new")
	}

	serverName := c.FormValue("server_name")
	yamlContent := c.FormValue("yaml_content")
	memoryLimit := c.FormValue("memory_limit")
	port := c.FormValue("port")
	saveTemplate := c.FormValue("save_template") == "true"

	if serverName == "" {
		return Render(c, servers.CreateAdvancedPage(user, sourceTemplate, yamlContent, "Server name is required"))
	}

	var template services.GameTemplate
	if err := h.templateService.ParseYAML(yamlContent, &template); err != nil {
		return Render(c, servers.CreateAdvancedPage(user, sourceTemplate, yamlContent, "Invalid YAML: "+err.Error()))
	}

	template.IsBuiltIn = false

	if template.ID == "" {
		return Render(c, servers.CreateAdvancedPage(user, sourceTemplate, yamlContent, "Template ID is required in YAML"))
	}

	if template.DockerImage == "" {
		return Render(c, servers.CreateAdvancedPage(user, sourceTemplate, yamlContent, "Docker image is required in template"))
	}

	tempServerPath := filepath.Join(h.cfg.Storage.Servers, "validation-temp")
	if err := h.templateService.ValidateTemplate(&template, tempServerPath); err != nil {
		return Render(c, servers.CreateAdvancedPage(user, sourceTemplate, yamlContent,
			fmt.Sprintf("Security validation failed: %s. Contact an administrator if you need additional capabilities or mount permissions.", err.Error())))
	}

	if saveTemplate {
		var err error
		template, err = h.templateService.AddWithUniqueID(template)
		if err != nil {
			return Render(c, servers.CreateAdvancedPage(user, sourceTemplate, yamlContent, "Failed to save template: "+err.Error()))
		}
		if err := h.templateService.SaveToFile(template); err != nil {
			return Render(c, servers.CreateAdvancedPage(user, sourceTemplate, yamlContent, "Template saved to memory but failed to save file: "+err.Error()))
		}
	} else {
		var err error
		template, err = h.templateService.AddWithUniqueID(template)
		if err != nil {
			return Render(c, servers.CreateAdvancedPage(user, sourceTemplate, yamlContent, "Failed to register template: "+err.Error()))
		}
	}

	var memLimit int
	if memoryLimit != "" {
		if _, err := fmt.Sscanf(memoryLimit, "%d", &memLimit); err != nil {
			return Render(c, servers.CreateAdvancedPage(user, sourceTemplate, yamlContent, "Invalid memory limit: must be a number"))
		}
	}
	var serverPort int
	if port != "" {
		if _, err := fmt.Sscanf(port, "%d", &serverPort); err != nil {
			return Render(c, servers.CreateAdvancedPage(user, sourceTemplate, yamlContent, "Invalid port: must be a number"))
		}
	}

	_, err := h.serverService.Create(services.CreateServerRequest{
		Name:        serverName,
		GameType:    template.ID,
		MemoryLimit: memLimit,
		Port:        serverPort,
	})
	if err != nil {
		return Render(c, servers.CreateAdvancedPage(user, sourceTemplate, yamlContent, "Template saved but failed to create server: "+err.Error()))
	}

	return c.Redirect().To("/")
}

func (h *ServerHandler) View(c fiber.Ctx) error {
	server, err := h.syncAndGetServer(c)
	if err != nil {
		return err
	}

	user := middleware.GetUser(c)
	isOutdated := h.serverService.IsTemplateOutdated(server)
	newVersion := h.serverService.GetTemplateVersion(server.GameType)
	return Render(c, servers.ViewPage(server, user, isOutdated, newVersion))
}

func (h *ServerHandler) ViewTemplateConfig(c fiber.Ctx) error {
	server, err := h.syncAndGetServer(c)
	if err != nil {
		return err
	}

	user := middleware.GetUser(c)
	yamlContent, err := h.serverService.GetServerTemplateYAML(server)
	if err != nil {
		return fiber.NewError(fiber.StatusInternalServerError, "Failed to get template config: "+err.Error())
	}

	return Render(c, servers.TemplateConfigPage(server, user, yamlContent))
}

func (h *ServerHandler) Delete(c fiber.Ctx) error {
	id, err := GetServerID(c)
	if err != nil {
		return err
	}
	if err := h.serverService.Delete(id); err != nil {
		return fiber.NewError(fiber.StatusInternalServerError, "Failed to delete server: "+err.Error())
	}
	return c.Redirect().To("/")
}

func (h *ServerHandler) Start(c fiber.Ctx) error {
	id, err := GetServerID(c)
	if err != nil {
		return err
	}
	if err := h.serverService.Start(id); err != nil {
		return fiber.NewError(fiber.StatusInternalServerError, "Failed to start server: "+err.Error())
	}
	server, _ := h.serverService.Get(id)
	return Render(c, partials.ServerStatusBadge(server.Status))
}

func (h *ServerHandler) Stop(c fiber.Ctx) error {
	id, err := GetServerID(c)
	if err != nil {
		return err
	}
	var logSaveErr *services.ErrLogSaveFailed
	if err := h.serverService.Stop(id); err != nil {
		if errors.As(err, &logSaveErr) {
			c.Set("HX-Trigger", `{"showWarning": "Server stopped but logs could not be saved"}`)
		} else {
			return fiber.NewError(fiber.StatusInternalServerError, "Failed to stop server: "+err.Error())
		}
	}
	server, _ := h.serverService.Get(id)
	return Render(c, partials.ServerStatusBadge(server.Status))
}

func (h *ServerHandler) Restart(c fiber.Ctx) error {
	id, err := GetServerID(c)
	if err != nil {
		return err
	}
	var logSaveErr *services.ErrLogSaveFailed
	if err := h.serverService.Restart(id); err != nil {
		if errors.As(err, &logSaveErr) {
			c.Set("HX-Trigger", `{"showWarning": "Server restarted but logs could not be saved"}`)
		} else {
			return fiber.NewError(fiber.StatusInternalServerError, "Failed to restart server: "+err.Error())
		}
	}
	server, _ := h.serverService.Get(id)
	return Render(c, partials.ServerStatusBadge(server.Status))
}

func (h *ServerHandler) Status(c fiber.Ctx) error {
	server, err := h.syncAndGetServer(c)
	if err != nil {
		return err
	}
	return Render(c, pages.ServerCard(server))
}

func (h *ServerHandler) Controls(c fiber.Ctx) error {
	server, err := h.syncAndGetServer(c)
	if err != nil {
		return err
	}

	user := middleware.GetUser(c)
	return Render(c, partials.ServerControls(server, user.IsAdmin))
}

func (h *ServerHandler) StatusBadge(c fiber.Ctx) error {
	server, err := h.syncAndGetServer(c)
	if err != nil {
		return err
	}

	return Render(c, partials.ServerStatusBadge(server.Status))
}

func (h *ServerHandler) syncAndGetServer(c fiber.Ctx) (*models.Server, error) {
	id, err := GetServerID(c)
	if err != nil {
		return nil, err
	}

	_ = h.serverService.SyncStatus(id)
	server, err := h.serverService.Get(id)
	if err != nil {
		return nil, fiber.NewError(fiber.StatusNotFound, "Server not found")
	}

	return server, nil
}

func (h *ServerHandler) EditPage(c fiber.Ctx) error {
	user := middleware.GetUser(c)
	serverID := c.Params("id")

	server, err := h.serverService.Get(serverID)
	if err != nil {
		return fiber.NewError(fiber.StatusNotFound, "Server not found")
	}

	if !user.IsAdmin && !h.serverService.HasAccess(user.ID, serverID) {
		return fiber.ErrForbidden
	}

	template, _ := h.templateService.Get(server.GameType)
	customEnv := h.templateService.DecodeEnvironment(server.CustomEnvironment)
	return Render(c, servers.EditPage(server, user, nil, template.Environment, customEnv, template.VarDescriptions))
}

func (h *ServerHandler) Update(c fiber.Ctx) error {
	user := middleware.GetUser(c)
	serverID := c.Params("id")

	server, err := h.serverService.Get(serverID)
	if err != nil {
		return fiber.NewError(fiber.StatusNotFound, "Server not found")
	}

	if !user.IsAdmin && !h.serverService.HasAccess(user.ID, serverID) {
		return fiber.ErrForbidden
	}

	template, _ := h.templateService.Get(server.GameType)
	currentCustomEnv := h.templateService.DecodeEnvironment(server.CustomEnvironment)

	var form forms.UpdateServer
	if err := c.Bind().Form(&form); err != nil {
		formErrors := map[string]string{"_general": "Invalid form data"}
		return Render(c, servers.EditForm(server, formErrors, template.Environment, currentCustomEnv, template.VarDescriptions), fiber.StatusBadRequest)
	}

	validator := validators.NewServerUpdateValidator(h.serverService)
	formErrors := validator.Validate(form, server)

	if len(formErrors) > 0 {
		return Render(c, servers.EditForm(server, formErrors, template.Environment, currentCustomEnv, template.VarDescriptions), fiber.StatusBadRequest)
	}

	customVars := make(map[string]string)
	for key := range template.Environment {
		if val := c.FormValue("env_" + key); val != "" {
			customVars[key] = val
		}
	}

	req := services.UpdateServerRequest{
		Name:        form.Name,
		MemoryLimit: form.MemoryLimit,
		Port:        form.Port,
		CustomVars:  customVars,
	}

	if err := h.serverService.Update(serverID, req); err != nil {
		updateErrors := map[string]string{"_general": "Failed to update server: " + err.Error()}
		return Render(c, servers.EditForm(server, updateErrors, template.Environment, currentCustomEnv, template.VarDescriptions), fiber.StatusInternalServerError)
	}

	c.Set("X-Success-Message", "Server updated successfully")
	c.Set("HX-Redirect", "/servers/"+serverID)
	return c.SendStatus(fiber.StatusOK)
}

func (h *ServerHandler) Upgrade(c fiber.Ctx) error {
	id, err := GetServerID(c)
	if err != nil {
		return err
	}

	if err := h.serverService.UpgradeTemplate(id); err != nil {
		return fiber.NewError(fiber.StatusInternalServerError, "Failed to upgrade server: "+err.Error())
	}

	return c.Redirect().To("/servers/" + id)
}
