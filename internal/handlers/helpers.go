package handlers

import (
	"github.com/a-h/templ"
	"github.com/gofiber/fiber/v3"
)

// Render renders a templ component to the response
func Render(c fiber.Ctx, component templ.Component, status ...int) error {
	c.Set("Content-Type", "text/html; charset=utf-8")

	// Optional status code
	if len(status) > 0 {
		c.Status(status[0])
	}

	return component.Render(c.Context(), c.Response().BodyWriter())
}

// RenderFragment checks if request is HTMX and renders accordingly
func RenderFragment(c fiber.Ctx, fragment templ.Component, fullPage templ.Component) error {
	if c.Get("HX-Request") == "true" {
		return Render(c, fragment)
	}
	return Render(c, fullPage)
}
