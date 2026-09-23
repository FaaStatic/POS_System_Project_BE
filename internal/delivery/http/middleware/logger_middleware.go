package middleware

import (
	"time"

	"github.com/gofiber/fiber/v3"
	"go.uber.org/zap"
)

func LoggerMiddleware(log *zap.Logger) fiber.Handler {
	return func(c fiber.Ctx) error {
		start := time.Now()
		err := c.Next()
		duration := time.Since(start)
		statusCode := c.Response().StatusCode()
		log.Info("HTTP Request",
			zap.String("method", c.Method()),
			zap.String("path", c.Path()),
			zap.Int("status", statusCode),
			zap.String("latency", duration.String()),
			zap.String("ip", c.IP()),
			zap.String("user_agent", string(c.Request().Header.UserAgent())),
		)
		return err
	}

}
