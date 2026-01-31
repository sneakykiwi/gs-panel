package middleware

import (
	"github.com/sneakykiwi/gs-panel/views/components"

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

		return Render(c, components.ErrorModal(code, message))
	}
}
