package handlers

import (
	"context"

	"github.com/a-h/templ"
	"github.com/gofiber/fiber/v3"
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
	if token, ok := c.Locals(CsrfTokenKey).(string); ok {
		ctx = context.WithValue(ctx, CsrfTokenKey, token)
	}

	return component.Render(ctx, c.Response().BodyWriter())
}
