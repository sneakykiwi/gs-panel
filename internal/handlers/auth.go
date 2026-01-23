package handlers

import (
	"gs-panel/internal/middleware"
	"gs-panel/internal/services"

	"github.com/gofiber/fiber/v3"
)

type AuthHandler struct {
	authService *services.AuthService
}

func NewAuthHandler(authService *services.AuthService) *AuthHandler {
	return &AuthHandler{authService: authService}
}

func (h *AuthHandler) LoginPage(c fiber.Ctx) error {
	needsSetup := !h.authService.HasAdminUser()
	return c.Render("login", fiber.Map{
		"Title":      "Login",
		"NeedsSetup": needsSetup,
	})
}

func (h *AuthHandler) Login(c fiber.Ctx) error {
	email := c.FormValue("email")
	password := c.FormValue("password")

	session, err := h.authService.Login(email, password)
	if err != nil {
		return c.Render("login", fiber.Map{
			"Title": "Login",
			"Error": "Invalid email or password",
		})
	}

	c.Cookie(&fiber.Cookie{
		Name:     middleware.SessionCookieName,
		Value:    session.Token,
		HTTPOnly: true,
		Secure:   false,
		SameSite: "Lax",
		MaxAge:   86400,
	})

	return c.Redirect().To("/")
}

func (h *AuthHandler) Logout(c fiber.Ctx) error {
	token := c.Cookies(middleware.SessionCookieName)
	if token != "" {
		h.authService.Logout(token)
	}

	c.Cookie(&fiber.Cookie{
		Name:   middleware.SessionCookieName,
		Value:  "",
		MaxAge: -1,
	})

	return c.Redirect().To("/login")
}

func (h *AuthHandler) SetupPage(c fiber.Ctx) error {
	if h.authService.HasAdminUser() {
		return c.Redirect().To("/login")
	}

	return c.Render("setup", fiber.Map{
		"Title": "Initial Setup",
	})
}

func (h *AuthHandler) Setup(c fiber.Ctx) error {
	if h.authService.HasAdminUser() {
		return c.Redirect().To("/login")
	}

	email := c.FormValue("email")
	password := c.FormValue("password")
	confirmPassword := c.FormValue("confirm_password")

	if password != confirmPassword {
		return c.Render("setup", fiber.Map{
			"Title": "Initial Setup",
			"Error": "Passwords do not match",
		})
	}

	if len(password) < 8 {
		return c.Render("setup", fiber.Map{
			"Title": "Initial Setup",
			"Error": "Password must be at least 8 characters",
		})
	}

	_, err := h.authService.CreateUser(email, password, true)
	if err != nil {
		return c.Render("setup", fiber.Map{
			"Title": "Initial Setup",
			"Error": "Failed to create admin user",
		})
	}

	return c.Redirect().To("/login")
}
