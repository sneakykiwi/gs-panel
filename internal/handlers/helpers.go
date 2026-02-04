package handlers

import (
	"context"

	"github.com/a-h/templ"
	"github.com/gofiber/fiber/v3"
	"github.com/gofiber/fiber/v3/middleware/csrf"
)

// CsrfTokenKey is the context key for CSRF token
const CsrfTokenKey = "csrf_token"

// Render renders a templ component to the response
func Render(c fiber.Ctx, component templ.Component, status ...int) error {
	c.Set("Content-Type", "text/html; charset=utf-8")

	if len(status) > 0 {
		c.Status(status[0])
	}

	// Inject CSRF token into template context
	ctx := c.Context()
	if token := csrf.TokenFromContext(c); token != "" {
		ctx = context.WithValue(ctx, CsrfTokenKey, token)
	}

	return component.Render(ctx, c.Response().BodyWriter())
}
