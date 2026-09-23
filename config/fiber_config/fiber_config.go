package fiberconfig

import (
	"context"
	"crypto/tls"
	"encoding/json"
	"os"
	"time"

	envconfig "shop_project_be/config/env_config"
	middleware "shop_project_be/internal/delivery/http/middleware"
	appvalidator "shop_project_be/pkg/validator"

	"github.com/bytedance/sonic"
	swagger "github.com/gofiber/contrib/v3/swaggerui"
	"github.com/gofiber/fiber/v3"
	"github.com/gofiber/fiber/v3/middleware/compress"
	"github.com/gofiber/fiber/v3/middleware/cors"
	"github.com/gofiber/fiber/v3/middleware/helmet"
	"github.com/gofiber/fiber/v3/middleware/limiter"
	"github.com/gofiber/fiber/v3/middleware/recover"
	"go.uber.org/zap"
)

func GetFiberConfig(logger *zap.Logger, appName string, trustedProxies []string) fiber.Config {
	return fiber.Config{
		JSONEncoder:        sonic.Marshal,
		JSONDecoder:        sonic.Unmarshal,
		StructValidator:    appvalidator.New(),
		ServerHeader:       "Fiber",
		AppName:            appName,
		ReadTimeout:        30 * time.Second,
		WriteTimeout:       60 * time.Second,
		IdleTimeout:        10 * time.Minute,
		BodyLimit:          25 * 1024 * 1024,
		CaseSensitive:      true,
		StrictRouting:      true,
		ProxyHeader:        fiber.HeaderXForwardedFor,
		EnableIPValidation: true,
		TrustProxy:         true,
		TrustProxyConfig: fiber.TrustProxyConfig{
			Proxies:  trustedProxies,
			Loopback: true,
		},
		ErrorHandler: func(c fiber.Ctx, err error) error {
			code := fiber.StatusInternalServerError
			message := "Something went wrong, please try again later"
			if e, ok := err.(*fiber.Error); ok {
				code = e.Code
				message = e.Message
			}
			fields := []zap.Field{
				zap.Error(err),
				zap.Int("status_code", code),
				zap.String("path", c.Path()),
				zap.String("method", c.Method()),
				zap.String("ip", c.IP()),
			}
			if code >= fiber.StatusInternalServerError {
				logger.Error("unhandled request error", fields...)
			} else {
				logger.Warn("request rejected", fields...)
			}

			return c.Status(code).JSON(fiber.Map{
				"success": false,
				"code":    code,
				"message": message,
			})
		},
	}
}

func GetFiberConfigListener(env string, gracefulCtx context.Context) fiber.ListenConfig {
	return fiber.ListenConfig{
		EnablePrefork:     true,
		EnablePrintRoutes: env == "development",
		TLSMinVersion:     tls.VersionTLS12,
		GracefulContext:   gracefulCtx,
		ShutdownTimeout:   10 * time.Second,
	}
}

func loadSwaggerSpec(nameApp string) []byte {
	raw, err := os.ReadFile("./swagger.json")
	if err != nil {
		return nil
	}

	var spec map[string]interface{}
	if err := json.Unmarshal(raw, &spec); err != nil {
		return raw
	}

	if info, ok := spec["info"].(map[string]interface{}); ok {
		info["title"] = nameApp + " API"
	}

	delete(spec, "host")
	delete(spec, "schemes")

	patched, err := json.Marshal(spec)
	if err != nil {
		return raw
	}
	return patched
}

func GetSwaggerConfig(nameApp string, isDev bool) swagger.Config {
	cacheAge := 1
	if !isDev {
		cacheAge = 3600
	} else {
		cacheAge = 1
	}
	return swagger.Config{
		Next:        nil,
		BasePath:    "/",
		FilePath:    "./swagger.json",
		FileContent: loadSwaggerSpec(nameApp),
		Path:        "/",
		Title:       nameApp + " API documentation",
		CacheAge:    cacheAge,
	}
}

func InitFiber(env string, envData *envconfig.Config, logger *zap.Logger, redisClient fiber.Storage, routes ...func(router fiber.Router)) *fiber.App {
	app := fiber.New(GetFiberConfig(logger, envData.App.Name, envData.App.TrustedProxies))
	app.Use(recover.New(recover.Config{
		EnableStackTrace: true,
	}))
	app.Use(helmet.New(middleware.GetXSSConfig()))
	app.Use(compress.New(middleware.GetCompressConfig()))
	app.Use(cors.New(middleware.GetCorsConfig()))
	app.Use(limiter.New(middleware.GetGlobalLimiter(redisClient)))
	app.Use(middleware.LoggerMiddleware(logger))

	if env != "production" {
		app.Use(swagger.New(GetSwaggerConfig(envData.App.Name, true)))
	} else {
		app.Use(swagger.New(GetSwaggerConfig(envData.App.Name, false)))
	}

	for _, register := range routes {
		register(app)
	}

	app.Use(func(c fiber.Ctx) error {
		return c.Status(fiber.StatusNotFound).JSON(fiber.Map{
			"error": "Endpoint Not Found!",
		})
	})
	return app
}
