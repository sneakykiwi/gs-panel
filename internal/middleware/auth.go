package middleware

import (
	"gs-panel/internal/models"
	"gs-panel/internal/services"

	"github.com/gofiber/fiber/v3"
)

const (
	SessionCookieName = "gs_session"
	UserContextKey    = "user"
)

func Auth(authService *services.AuthService) fiber.Handler {
	return func(c fiber.Ctx) error {
		token := c.Cookies(SessionCookieName)
		if token == "" {
			return c.Redirect().To("/login")
		}

		user, err := authService.ValidateSession(token)
		if err != nil {
			c.Cookie(&fiber.Cookie{
				Name:   SessionCookieName,
				Value:  "",
				MaxAge: -1,
			})
			return c.Redirect().To("/login")
		}

		c.Locals(UserContextKey, user)
		return c.Next()
	}
}

func AdminOnly() fiber.Handler {
	return func(c fiber.Ctx) error {
		user := GetUser(c)
		if user == nil || !user.IsAdmin {
			return c.Status(fiber.StatusForbidden).SendString("Forbidden")
		}
		return c.Next()
	}
}

func GetUser(c fiber.Ctx) *models.User {
	user, ok := c.Locals(UserContextKey).(*models.User)
	if !ok {
		return nil
	}
	return user
}
