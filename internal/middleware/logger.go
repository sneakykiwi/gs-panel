package middleware

import (
	"time"

	"github.com/sneakykiwi/gs-panel/internal/logger"

	"github.com/gofiber/fiber/v3"
)

func Logger() fiber.Handler {
	return func(c fiber.Ctx) error {
		start := time.Now()

		err := c.Next()

		status := c.Response().StatusCode()
		logEvent := logger.Info()

		if status >= 400 {
			logEvent = logger.Warn()
		}
		if status >= 500 {
			logEvent = logger.Error()
		}

		logEvent.
			Str("method", c.Method()).
			Str("path", c.Path()).
			Int("status", status).
			Dur("latency", time.Since(start)).
			Str("ip", c.IP()).
			Msg("request")

		return err
	}
}
