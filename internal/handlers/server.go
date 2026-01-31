package handlers

import (
	"gs-panel/internal/forms"
	"gs-panel/internal/middleware"
	"gs-panel/internal/services"
	"gs-panel/views/pages/servers"
	"gs-panel/views/partials"

	"github.com/gofiber/fiber/v3"
)

type ServerHandler struct {
	serverService   *services.ServerService
	templateService *services.TemplateService
}

func NewServerHandler(serverService *services.ServerService, templateService *services.TemplateService) *ServerHandler {
	return &ServerHandler{serverService: serverService, templateService: templateService}
}

func (h *ServerHandler) Dashboard(c fiber.Ctx) error {
	user := middleware.GetUser(c)
	var servers interface{}
	var err error

	if user.IsAdmin {
		servers, err = h.serverService.List()
	} else {
		servers, err = h.serverService.ListForUser(user.ID)
	}
	if err != nil {
		return err
	}

	return c.Render("dashboard", fiber.Map{"Title": "Dashboard", "User": user, "Servers": servers})
}

func (h *ServerHandler) CreatePage(c fiber.Ctx) error {
	return c.Render("servers/create", fiber.Map{
		"Title":     "Create Server",
		"User":      middleware.GetUser(c),
		"Templates": h.templateService.List(),
	})
}

func (h *ServerHandler) Create(c fiber.Ctx) error {
	var form forms.CreateServer
	if err := c.Bind().Form(&form); err != nil {
		return c.Render("servers/create", fiber.Map{
			"Title":     "Create Server",
			"User":      middleware.GetUser(c),
			"Templates": h.templateService.List(),
			"Error":     "Invalid form data",
		})
	}

	_, err := h.serverService.Create(services.CreateServerRequest{
		Name:        form.Name,
		GameType:    form.GameType,
		MemoryLimit: form.MemoryLimit,
		Port:        form.Port,
	})
	if err != nil {
		return c.Render("servers/create", fiber.Map{
			"Title":     "Create Server",
			"User":      middleware.GetUser(c),
			"Templates": h.templateService.List(),
			"Error":     err.Error(),
		})
	}

	return c.Redirect().To("/")
}

func (h *ServerHandler) View(c fiber.Ctx) error {
	id := c.Params("id")
	if id == "" {
		return fiber.NewError(fiber.StatusBadRequest, "Invalid server ID")
	}

	server, err := h.serverService.Get(id)
	if err != nil {
		return fiber.NewError(fiber.StatusNotFound, "Server not found")
	}

	_ = h.serverService.SyncStatus(server.ID)
	server, _ = h.serverService.Get(id)

	user := middleware.GetUser(c)
	return Render(c, servers.ViewPage(server, user))
}

func (h *ServerHandler) Delete(c fiber.Ctx) error {
	id := c.Params("id")
	if id == "" {
		return fiber.NewError(fiber.StatusBadRequest, "Invalid server ID")
	}
	if err := h.serverService.Delete(id); err != nil {
		return fiber.NewError(fiber.StatusInternalServerError, "Failed to delete server: "+err.Error())
	}
	return c.Redirect().To("/")
}

func (h *ServerHandler) Start(c fiber.Ctx) error {
	id := c.Params("id")
	if id == "" {
		return fiber.NewError(fiber.StatusBadRequest, "Invalid server ID")
	}
	if err := h.serverService.Start(id); err != nil {
		return fiber.NewError(fiber.StatusInternalServerError, "Failed to start server: "+err.Error())
	}
	server, _ := h.serverService.Get(id)
	return c.Render("partials/server_status", fiber.Map{"Server": server})
}

func (h *ServerHandler) Stop(c fiber.Ctx) error {
	id := c.Params("id")
	if id == "" {
		return fiber.NewError(fiber.StatusBadRequest, "Invalid server ID")
	}
	if err := h.serverService.Stop(id); err != nil {
		return fiber.NewError(fiber.StatusInternalServerError, "Failed to stop server: "+err.Error())
	}
	server, _ := h.serverService.Get(id)
	return c.Render("partials/server_status", fiber.Map{"Server": server})
}

func (h *ServerHandler) Restart(c fiber.Ctx) error {
	id := c.Params("id")
	if id == "" {
		return fiber.NewError(fiber.StatusBadRequest, "Invalid server ID")
	}
	if err := h.serverService.Restart(id); err != nil {
		return fiber.NewError(fiber.StatusInternalServerError, "Failed to restart server: "+err.Error())
	}
	server, _ := h.serverService.Get(id)
	return c.Render("partials/server_status", fiber.Map{"Server": server})
}

func (h *ServerHandler) Status(c fiber.Ctx) error {
	id := c.Params("id")
	if id == "" {
		return fiber.NewError(fiber.StatusBadRequest, "Invalid server ID")
	}
	_ = h.serverService.SyncStatus(id)
	server, err := h.serverService.Get(id)
	if err != nil {
		return fiber.NewError(fiber.StatusNotFound, "Server not found")
	}
	return c.Render("partials/server_card", fiber.Map{"Server": server})
}

func (h *ServerHandler) Controls(c fiber.Ctx) error {
	id := c.Params("id")
	if id == "" {
		return fiber.NewError(fiber.StatusBadRequest, "Invalid server ID")
	}

	user := middleware.GetUser(c)
	_ = h.serverService.SyncStatus(id)
	server, err := h.serverService.Get(id)
	if err != nil {
		return fiber.NewError(fiber.StatusNotFound, "Server not found")
	}

	return Render(c, partials.ServerControls(server, user.IsAdmin))
}

func (h *ServerHandler) StatusBadge(c fiber.Ctx) error {
	id := c.Params("id")
	if id == "" {
		return fiber.NewError(fiber.StatusBadRequest, "Invalid server ID")
	}

	_ = h.serverService.SyncStatus(id)
	server, err := h.serverService.Get(id)
	if err != nil {
		return fiber.NewError(fiber.StatusNotFound, "Server not found")
	}

	return Render(c, partials.ServerStatusBadge(server.Status))
}

func (h *ServerHandler) EditPage(c fiber.Ctx) error {
	user := middleware.GetUser(c)
	serverID := c.Params("id")

	server, err := h.serverService.Get(serverID)
	if err != nil {
		return fiber.NewError(fiber.StatusNotFound, "Server not found")
	}

	// Check permissions
	if !user.IsAdmin && !h.serverService.HasAccess(user.ID, serverID) {
		return fiber.ErrForbidden
	}

	return Render(c, servers.EditPage(server, user, nil))
}

func (h *ServerHandler) Update(c fiber.Ctx) error {
	user := middleware.GetUser(c)
	serverID := c.Params("id")

	// Get server
	server, err := h.serverService.Get(serverID)
	if err != nil {
		return fiber.NewError(fiber.StatusNotFound, "Server not found")
	}

	// Check permissions
	if !user.IsAdmin && !h.serverService.HasAccess(user.ID, serverID) {
		return fiber.ErrForbidden
	}

	// Parse form
	var form forms.UpdateServer
	if err := c.Bind().Form(&form); err != nil {
		errors := map[string]string{"_general": "Invalid form data"}
		return Render(c, servers.EditForm(server, errors), fiber.StatusBadRequest)
	}

	// Validate
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

	// Check port conflicts
	if form.Port != server.Port {
		if err := h.serverService.ValidatePort(form.Port, serverID); err != nil {
			errors["port"] = err.Error()
		}
	}

	if len(errors) > 0 {
		return Render(c, servers.EditForm(server, errors), fiber.StatusBadRequest)
	}

	// Update server
	req := services.UpdateServerRequest{
		Name:        form.Name,
		MemoryLimit: form.MemoryLimit,
		Port:        form.Port,
	}

	if err := h.serverService.Update(serverID, req); err != nil {
		errors["_general"] = "Failed to update server: " + err.Error()
		return Render(c, servers.EditForm(server, errors), fiber.StatusInternalServerError)
	}

	// Set success message header
	c.Set("X-Success-Message", "Server updated successfully")

	// Redirect to server view
	c.Set("HX-Redirect", "/servers/"+serverID)
	return c.SendStatus(fiber.StatusOK)
}
