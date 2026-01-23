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

type loginForm struct {
	Email    string `form:"email"`
	Password string `form:"password"`
}

func (h *AuthHandler) Login(c fiber.Ctx) error {
	var form loginForm
	if err := c.Bind().Form(&form); err != nil {
		return c.Render("login", fiber.Map{
			"Title": "Login",
			"Error": "Invalid form data",
		})
	}

	session, err := h.authService.Login(form.Email, form.Password)
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

type setupForm struct {
	Email           string `form:"email"`
	Password        string `form:"password"`
	ConfirmPassword string `form:"confirm_password"`
}

func (h *AuthHandler) Setup(c fiber.Ctx) error {
	if h.authService.HasAdminUser() {
		return c.Redirect().To("/login")
	}

	var form setupForm
	if err := c.Bind().Form(&form); err != nil {
		return c.Render("setup", fiber.Map{
			"Title": "Initial Setup",
			"Error": "Invalid form data",
		})
	}

	if form.Password != form.ConfirmPassword {
		return c.Render("setup", fiber.Map{
			"Title": "Initial Setup",
			"Error": "Passwords do not match",
		})
	}

	if len(form.Password) < 8 {
		return c.Render("setup", fiber.Map{
			"Title": "Initial Setup",
			"Error": "Password must be at least 8 characters",
		})
	}

	_, err := h.authService.CreateUser(form.Email, form.Password, true)
	if err != nil {
		return c.Render("setup", fiber.Map{
			"Title": "Initial Setup",
			"Error": "Failed to create admin user",
		})
	}

	return c.Redirect().To("/login")
}
