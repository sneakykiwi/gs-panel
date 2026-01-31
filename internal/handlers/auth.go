package handlers

import (
	"github.com/sneakykiwi/gs-panel/internal/forms"
	"github.com/sneakykiwi/gs-panel/internal/middleware"
	"github.com/sneakykiwi/gs-panel/internal/services"

	"github.com/gofiber/fiber/v3"
)

type AuthHandler struct {
	authService *services.AuthService
}

func NewAuthHandler(authService *services.AuthService) *AuthHandler {
	return &AuthHandler{authService: authService}
}

func (h *AuthHandler) LoginPage(c fiber.Ctx) error {
	if !h.authService.HasAdminUser() {
		return c.Redirect().To("/setup")
	}
	return c.Render("login", fiber.Map{"Title": "Login"})
}

func (h *AuthHandler) Login(c fiber.Ctx) error {
	var form forms.Login
	if err := c.Bind().Form(&form); err != nil {
		return c.Render("login", fiber.Map{"Title": "Login", "Error": "Invalid form data"})
	}

	session, err := h.authService.Login(form.Email, form.Password)
	if err != nil {
		return c.Render("login", fiber.Map{"Title": "Login", "Error": "Invalid email or password"})
	}

	c.Cookie(&fiber.Cookie{
		Name:     middleware.SessionCookieName,
		Value:    session.Token,
		HTTPOnly: true,
		SameSite: "Lax",
		MaxAge:   86400,
	})

	return c.Redirect().To("/")
}

func (h *AuthHandler) Logout(c fiber.Ctx) error {
	if token := c.Cookies(middleware.SessionCookieName); token != "" {
		_ = h.authService.Logout(token)
	}
	c.Cookie(&fiber.Cookie{Name: middleware.SessionCookieName, Value: "", MaxAge: -1})
	return c.Redirect().To("/login")
}

func (h *AuthHandler) SetupPage(c fiber.Ctx) error {
	if h.authService.HasAdminUser() {
		return c.Redirect().To("/login")
	}
	return c.Render("setup", fiber.Map{"Title": "Initial Setup"})
}

func (h *AuthHandler) Setup(c fiber.Ctx) error {
	if h.authService.HasAdminUser() {
		return c.Redirect().To("/login")
	}

	var form forms.Setup
	if err := c.Bind().Form(&form); err != nil {
		return RenderError(c, "setup", "Initial Setup", nil, "Invalid form data")
	}

	if form.Password != form.ConfirmPassword {
		return RenderError(c, "setup", "Initial Setup", nil, "Passwords do not match")
	}

	if err := ValidatePassword(form.Password); err != nil {
		return RenderError(c, "setup", "Initial Setup", nil, err.Error())
	}

	if _, err := h.authService.CreateUser(form.Email, form.Password, true); err != nil {
		return RenderError(c, "setup", "Initial Setup", nil, "Failed to create admin user: "+err.Error())
	}

	return c.Redirect().To("/login")
}
