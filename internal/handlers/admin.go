package handlers

import (
	"strconv"

	"gs-panel/internal/forms"
	"gs-panel/internal/middleware"
	"gs-panel/internal/services"

	"github.com/gofiber/fiber/v3"
)

type AdminHandler struct {
	authService   *services.AuthService
	serverService *services.ServerService
}

func NewAdminHandler(authService *services.AuthService, serverService *services.ServerService) *AdminHandler {
	return &AdminHandler{authService: authService, serverService: serverService}
}

func (h *AdminHandler) UsersPage(c fiber.Ctx) error {
	users, err := h.authService.ListUsers()
	if err != nil {
		return err
	}
	return c.Render("admin/users", fiber.Map{
		"Title": "User Management",
		"User":  middleware.GetUser(c),
		"Users": users,
	})
}

func (h *AdminHandler) CreateUserPage(c fiber.Ctx) error {
	return c.Render("admin/user_create", fiber.Map{
		"Title": "Create User",
		"User":  middleware.GetUser(c),
	})
}

func (h *AdminHandler) CreateUser(c fiber.Ctx) error {
	var form forms.CreateUser
	if err := c.Bind().Form(&form); err != nil {
		return c.Render("admin/user_create", fiber.Map{
			"Title": "Create User",
			"User":  middleware.GetUser(c),
			"Error": "Invalid form data",
		})
	}

	if len(form.Password) < 8 {
		return c.Render("admin/user_create", fiber.Map{
			"Title": "Create User",
			"User":  middleware.GetUser(c),
			"Error": "Password must be at least 8 characters",
		})
	}

	if _, err := h.authService.CreateUser(form.Email, form.Password, form.IsAdmin == "on"); err != nil {
		return c.Render("admin/user_create", fiber.Map{
			"Title": "Create User",
			"User":  middleware.GetUser(c),
			"Error": "Failed to create user: " + err.Error(),
		})
	}

	return c.Redirect().To("/admin/users")
}

func (h *AdminHandler) DeleteUser(c fiber.Ctx) error {
	id, err := strconv.ParseUint(c.Params("id"), 10, 32)
	if err != nil {
		return fiber.NewError(fiber.StatusBadRequest, "Invalid user ID")
	}

	if currentUser := middleware.GetUser(c); currentUser != nil && currentUser.ID == uint(id) {
		return fiber.NewError(fiber.StatusBadRequest, "Cannot delete yourself")
	}

	if err := h.authService.DeleteUser(uint(id)); err != nil {
		return fiber.NewError(fiber.StatusInternalServerError, "Failed to delete user: "+err.Error())
	}
	return c.SendStatus(fiber.StatusOK)
}

func (h *AdminHandler) UserServersPage(c fiber.Ctx) error {
	id, err := strconv.ParseUint(c.Params("id"), 10, 32)
	if err != nil {
		return fiber.NewError(fiber.StatusBadRequest, "Invalid user ID")
	}

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
		"User":          middleware.GetUser(c),
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

	var form forms.AssignServer
	if err := c.Bind().Form(&form); err != nil {
		return fiber.ErrBadRequest
	}

	if err := h.serverService.AssignUser(form.ServerID, uint(userID)); err != nil {
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
