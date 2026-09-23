package middleware

import (
	"shop_project_be/pkg/response"

	"github.com/gofiber/fiber/v3"
)

func RequireFeature(enabled bool, feature string) fiber.Handler {
	return func(c fiber.Ctx) error {
		if !enabled {
			return response.Error(c, fiber.StatusServiceUnavailable, feature+" is not configured on this server", nil)
		}
		return c.Next()
	}
}
