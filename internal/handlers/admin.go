package handlers

import (
	"strconv"

	"gs-panel/internal/middleware"
	"gs-panel/internal/services"

	"github.com/gofiber/fiber/v3"
)

type AdminHandler struct {
	authService   *services.AuthService
	serverService *services.ServerService
}

func NewAdminHandler(authService *services.AuthService, serverService *services.ServerService) *AdminHandler {
	return &AdminHandler{
		authService:   authService,
		serverService: serverService,
	}
}

func (h *AdminHandler) UsersPage(c fiber.Ctx) error {
	user := middleware.GetUser(c)
	users, err := h.authService.ListUsers()
	if err != nil {
		return err
	}

	return c.Render("admin/users", fiber.Map{
		"Title": "User Management",
		"User":  user,
		"Users": users,
	})
}

func (h *AdminHandler) CreateUserPage(c fiber.Ctx) error {
	user := middleware.GetUser(c)

	return c.Render("admin/user_create", fiber.Map{
		"Title": "Create User",
		"User":  user,
	})
}

func (h *AdminHandler) CreateUser(c fiber.Ctx) error {
	email := c.FormValue("email")
	password := c.FormValue("password")
	isAdmin := c.FormValue("is_admin") == "on"

	if len(password) < 8 {
		user := middleware.GetUser(c)
		return c.Render("admin/user_create", fiber.Map{
			"Title": "Create User",
			"User":  user,
			"Error": "Password must be at least 8 characters",
		})
	}

	_, err := h.authService.CreateUser(email, password, isAdmin)
	if err != nil {
		user := middleware.GetUser(c)
		return c.Render("admin/user_create", fiber.Map{
			"Title": "Create User",
			"User":  user,
			"Error": "Failed to create user: " + err.Error(),
		})
	}

	return c.Redirect().To("/admin/users")
}

func (h *AdminHandler) DeleteUser(c fiber.Ctx) error {
	id, err := strconv.ParseUint(c.Params("id"), 10, 32)
	if err != nil {
		return fiber.ErrBadRequest
	}

	currentUser := middleware.GetUser(c)
	if currentUser.ID == uint(id) {
		return c.Status(fiber.StatusBadRequest).SendString("Cannot delete yourself")
	}

	if err := h.authService.DeleteUser(uint(id)); err != nil {
		return err
	}

	return c.SendStatus(fiber.StatusOK)
}

func (h *AdminHandler) UserServersPage(c fiber.Ctx) error {
	id, err := strconv.ParseUint(c.Params("id"), 10, 32)
	if err != nil {
		return fiber.ErrBadRequest
	}

	user := middleware.GetUser(c)
	targetUser, err := h.authService.GetUserByID(uint(id))
	if err != nil {
		return fiber.ErrNotFound
	}

	allServers, err := h.serverService.List()
	if err != nil {
		return err
	}

	userServers, err := h.serverService.ListForUser(uint(id))
	if err != nil {
		return err
	}

	userServerIDs := make(map[uint]bool)
	for _, s := range userServers {
		userServerIDs[s.ID] = true
	}

	return c.Render("admin/user_servers", fiber.Map{
		"Title":         "Manage User Servers",
		"User":          user,
		"TargetUser":    targetUser,
		"AllServers":    allServers,
		"UserServerIDs": userServerIDs,
	})
}

func (h *AdminHandler) AssignServer(c fiber.Ctx) error {
	userID, err := strconv.ParseUint(c.Params("id"), 10, 32)
	if err != nil {
		return fiber.ErrBadRequest
	}

	serverID, err := strconv.ParseUint(c.FormValue("server_id"), 10, 32)
	if err != nil {
		return fiber.ErrBadRequest
	}

	if err := h.serverService.AssignUser(uint(serverID), uint(userID)); err != nil {
		return err
	}

	return c.Redirect().To("/admin/users/" + c.Params("id") + "/servers")
}

func (h *AdminHandler) UnassignServer(c fiber.Ctx) error {
	userID, err := strconv.ParseUint(c.Params("id"), 10, 32)
	if err != nil {
		return fiber.ErrBadRequest
	}

	serverID, err := strconv.ParseUint(c.Params("serverId"), 10, 32)
	if err != nil {
		return fiber.ErrBadRequest
	}

	if err := h.serverService.UnassignUser(uint(serverID), uint(userID)); err != nil {
		return err
	}

	return c.SendStatus(fiber.StatusOK)
}
