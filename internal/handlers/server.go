package handlers

import (
	"strconv"

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
	id, err := strconv.ParseUint(c.Params("id"), 10, 32)
	if err != nil {
		return fiber.ErrBadRequest
	}

	server, err := h.serverService.Get(uint(id))
	if err != nil {
		return fiber.ErrNotFound
	}

	_ = h.serverService.SyncStatus(server.ID)
	server, _ = h.serverService.Get(uint(id))

	return c.Render("servers/view", fiber.Map{
		"Title":  server.Name,
		"User":   middleware.GetUser(c),
		"Server": server,
	})
}

func (h *ServerHandler) Delete(c fiber.Ctx) error {
	id, err := strconv.ParseUint(c.Params("id"), 10, 32)
	if err != nil {
		return fiber.ErrBadRequest
	}
	if err := h.serverService.Delete(uint(id)); err != nil {
		return err
	}
	return c.Redirect().To("/")
}

func (h *ServerHandler) Start(c fiber.Ctx) error {
	id, err := strconv.ParseUint(c.Params("id"), 10, 32)
	if err != nil {
		return fiber.ErrBadRequest
	}
	if err := h.serverService.Start(uint(id)); err != nil {
		return c.Status(fiber.StatusInternalServerError).SendString(err.Error())
	}
	server, _ := h.serverService.Get(uint(id))
	return c.Render("partials/server_status", fiber.Map{"Server": server})
}

func (h *ServerHandler) Stop(c fiber.Ctx) error {
	id, err := strconv.ParseUint(c.Params("id"), 10, 32)
	if err != nil {
		return fiber.ErrBadRequest
	}
	if err := h.serverService.Stop(uint(id)); err != nil {
		return c.Status(fiber.StatusInternalServerError).SendString(err.Error())
	}
	server, _ := h.serverService.Get(uint(id))
	return c.Render("partials/server_status", fiber.Map{"Server": server})
}

func (h *ServerHandler) Restart(c fiber.Ctx) error {
	id, err := strconv.ParseUint(c.Params("id"), 10, 32)
	if err != nil {
		return fiber.ErrBadRequest
	}
	if err := h.serverService.Restart(uint(id)); err != nil {
		return c.Status(fiber.StatusInternalServerError).SendString(err.Error())
	}
	server, _ := h.serverService.Get(uint(id))
	return c.Render("partials/server_status", fiber.Map{"Server": server})
}

func (h *ServerHandler) Status(c fiber.Ctx) error {
	id, err := strconv.ParseUint(c.Params("id"), 10, 32)
	if err != nil {
		return fiber.ErrBadRequest
	}
	_ = h.serverService.SyncStatus(uint(id))
	server, err := h.serverService.Get(uint(id))
	if err != nil {
		return fiber.ErrNotFound
	}
	return c.Render("partials/server_card", fiber.Map{"Server": server})
}
