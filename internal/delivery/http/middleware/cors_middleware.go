package middleware

import "github.com/gofiber/fiber/v3/middleware/cors"

func GetCorsConfig() cors.Config {
	return cors.Config{
		AllowOrigins: []string{"*"},
		AllowHeaders: []string{"Origin", "Content-Type", "Accept", "Authorization"},
		AllowMethods: []string{"GET", "POST", "PUT", "PATCH", "DELETE"},
	}
}
