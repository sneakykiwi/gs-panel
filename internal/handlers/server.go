package handlers

import (
	"strconv"

	"gs-panel/internal/middleware"
	"gs-panel/internal/services"

	"github.com/gofiber/fiber/v3"
)

type ServerHandler struct {
	serverService   *services.ServerService
	templateService *services.TemplateService
}

func NewServerHandler(serverService *services.ServerService, templateService *services.TemplateService) *ServerHandler {
	return &ServerHandler{
		serverService:   serverService,
		templateService: templateService,
	}
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

	return c.Render("dashboard", fiber.Map{
		"Title":   "Dashboard",
		"User":    user,
		"Servers": servers,
	})
}

func (h *ServerHandler) CreatePage(c fiber.Ctx) error {
	user := middleware.GetUser(c)
	templates := h.templateService.List()

	return c.Render("servers/create", fiber.Map{
		"Title":     "Create Server",
		"User":      user,
		"Templates": templates,
	})
}

func (h *ServerHandler) Create(c fiber.Ctx) error {
	name := c.FormValue("name")
	gameType := c.FormValue("game_type")
	memoryLimit, _ := strconv.Atoi(c.FormValue("memory_limit"))
	port, _ := strconv.Atoi(c.FormValue("port"))

	_, err := h.serverService.Create(services.CreateServerRequest{
		Name:        name,
		GameType:    gameType,
		MemoryLimit: memoryLimit,
		Port:        port,
	})

	if err != nil {
		user := middleware.GetUser(c)
		templates := h.templateService.List()
		return c.Render("servers/create", fiber.Map{
			"Title":     "Create Server",
			"User":      user,
			"Templates": templates,
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

	h.serverService.SyncStatus(server.ID)
	server, _ = h.serverService.Get(uint(id))

	user := middleware.GetUser(c)

	return c.Render("servers/view", fiber.Map{
		"Title":  server.Name,
		"User":   user,
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
	return c.Render("partials/server_status", fiber.Map{
		"Server": server,
	})
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
	return c.Render("partials/server_status", fiber.Map{
		"Server": server,
	})
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
	return c.Render("partials/server_status", fiber.Map{
		"Server": server,
	})
}

func (h *ServerHandler) Status(c fiber.Ctx) error {
	id, err := strconv.ParseUint(c.Params("id"), 10, 32)
	if err != nil {
		return fiber.ErrBadRequest
	}

	h.serverService.SyncStatus(uint(id))
	server, err := h.serverService.Get(uint(id))
	if err != nil {
		return fiber.ErrNotFound
	}

	return c.Render("partials/server_card", fiber.Map{
		"Server": server,
	})
}
