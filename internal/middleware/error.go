package middleware

import (
	"fmt"

	"github.com/sneakykiwi/gs-panel/internal/logger"

	"github.com/gofiber/fiber/v3"
)

func ErrorHandler() fiber.ErrorHandler {
	return func(c fiber.Ctx, err error) error {
		code := fiber.StatusInternalServerError
		message := "Internal Server Error"

		if e, ok := err.(*fiber.Error); ok {
			code = e.Code
			message = e.Message
		} else {
			message = err.Error()
		}

		logger.Error().
			Err(err).
			Int("status", code).
			Str("method", c.Method()).
			Str("path", c.Path()).
			Str("ip", c.IP()).
			Msg("request error")

		isHTMX := c.Get("HX-Request") == "true"
		wantsJSON := c.Get("Accept") == "application/json"

		if wantsJSON {
			return c.Status(code).JSON(fiber.Map{
				"error":   true,
				"message": message,
				"code":    code,
			})
		}

		if isHTMX {
			c.Set("HX-Retarget", "#error-toast")
			c.Set("HX-Reswap", "innerHTML")
			return c.Status(code).SendString(`<div class="bg-red-500/20 border border-red-500 text-red-300 px-4 py-3 rounded" role="alert">` + message + `</div>`)
		}

		// Return simple error page for non-HTMX requests
		return c.Status(code).SendString(`<!DOCTYPE html>
<html>
<head><title>Error</title></head>
<body style="background:#1f2937;color:#fff;font-family:sans-serif;display:flex;align-items:center;justify-content:center;height:100vh;margin:0;">
<div style="text-align:center;">
<div style="font-size:4rem;font-weight:bold;color:#ef4444;margin-bottom:1rem;">` + fmt.Sprintf("%d", code) + `</div>
<h1 style="margin-bottom:1rem;">Something went wrong</h1>
<p style="color:#9ca3af;margin-bottom:2rem;">` + message + `</p>
<a href="/" style="background:#3b82f6;color:#fff;padding:0.75rem 1.5rem;text-decoration:none;border-radius:0.25rem;">Go to Dashboard</a>
</div>
</body>
</html>`)
	}
}
