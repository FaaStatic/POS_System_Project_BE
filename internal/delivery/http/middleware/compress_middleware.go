package middleware

import "github.com/gofiber/fiber/v3/middleware/compress"

func GetCompressConfig() compress.Config {
	return compress.Config{
		Level: compress.LevelDefault,
	}
}
