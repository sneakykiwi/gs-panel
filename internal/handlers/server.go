package handlers

import (
	"gs-panel/internal/forms"
	"gs-panel/internal/middleware"
	"gs-panel/internal/services"

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

	return c.Render("servers/view", fiber.Map{
		"Title":  server.Name,
		"User":   middleware.GetUser(c),
		"Server": server,
	})
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
