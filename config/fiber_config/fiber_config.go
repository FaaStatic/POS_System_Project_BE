package fiberconfig

import (
	"context"
	"crypto/tls"
	"encoding/json"
	"os"
	"strings"
	"time"

	envconfig "shop_project_be/config/env_config"
	"shop_project_be/internal/constant/apipath"
	middleware "shop_project_be/internal/delivery/http/middleware"
	"shop_project_be/pkg/response"

	"github.com/bytedance/sonic"
	swagger "github.com/gofiber/contrib/v3/swaggerui"
	"github.com/gofiber/fiber/v3"
	"github.com/gofiber/fiber/v3/middleware/compress"
	"github.com/gofiber/fiber/v3/middleware/cors"
	"github.com/gofiber/fiber/v3/middleware/helmet"
	"github.com/gofiber/fiber/v3/middleware/limiter"
	"github.com/gofiber/fiber/v3/middleware/recover"
	"github.com/gofiber/fiber/v3/middleware/requestid"
	"go.uber.org/zap"
)

func GetFiberConfig(logger *zap.Logger, appName string, trustedProxies []string) fiber.Config {
	return fiber.Config{
		JSONEncoder:        sonic.Marshal,
		JSONDecoder:        sonic.Unmarshal,
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
				zap.String("request_id", requestid.FromContext(c)),
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

func GetFiberConfigListener(env string, prefork bool, gracefulCtx context.Context) fiber.ListenConfig {
	return fiber.ListenConfig{
		EnablePrefork:     prefork,
		EnablePrintRoutes: env == "development",
		TLSMinVersion:     tls.VersionTLS12,
		GracefulContext:   gracefulCtx,
		ShutdownTimeout:   10 * time.Second,
	}
}

func loadSwaggerSpec(nameApp, version string) []byte {
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
	spec["paths"] = withVersion(spec["paths"], version)

	delete(spec, "host")
	delete(spec, "schemes")

	patched, err := json.Marshal(spec)
	if err != nil {
		return raw
	}
	return patched
}

// withVersion mengganti awalan versi bawaan (yang ditulis anotasi @Router) pada tiap
// path spec dengan versi aktif, jadi swagger.json tidak perlu dibuat ulang per versi.
func withVersion(paths interface{}, version string) interface{} {
	items, ok := paths.(map[string]interface{})
	if !ok || version == apipath.DefaultVersion {
		return paths
	}
	from, to := apipath.Base(apipath.DefaultVersion)+"/", apipath.Base(version)+"/"
	renamed := make(map[string]interface{}, len(items))
	for path, item := range items {
		if rest, found := strings.CutPrefix(path, from); found {
			path = to + rest
		}
		renamed[path] = item
	}
	return renamed
}

// GetSwaggerConfig: UI di /{version} (mis. /v1) dan spec di /{version}/swagger.json.
func GetSwaggerConfig(nameApp string, isDev bool, version string) swagger.Config {
	cacheAge := 3600
	if isDev {
		cacheAge = 1
	}
	return swagger.Config{
		BasePath:    "/" + version,
		FilePath:    "./swagger.json",
		FileContent: loadSwaggerSpec(nameApp, version),
		Path:        "/",
		Title:       nameApp + " API documentation",
		CacheAge:    cacheAge,
	}
}

func InitFiber(env string, envData *envconfig.Config, logger *zap.Logger, redisClient fiber.Storage, routes func(router fiber.Router)) *fiber.App {
	app := fiber.New(GetFiberConfig(logger, envData.App.Name, envData.App.TrustedProxies))
	// Logger paling luar: ia yang menjalankan ErrorHandler, jadi 429 dari limiter
	// dan panic yang dipulihkan recover ikut tercatat dengan status aslinya.
	app.Use(requestid.New())
	app.Use(middleware.LoggerMiddleware(logger))
	app.Use(recover.New(recover.Config{
		EnableStackTrace: true,
	}))
	app.Use(helmet.New(middleware.GetXSSConfig()))
	app.Use(compress.New())
	app.Use(cors.New(middleware.GetCorsConfig(envData.App.CMSOrigins)))
	version := envData.App.APIVersion
	app.Use(limiter.New(middleware.GetGlobalLimiter(redisClient, version)))

	app.Get("/", func(c fiber.Ctx) error {
		return c.Redirect().To("/" + version)
	})
	app.Use(swagger.New(GetSwaggerConfig(envData.App.Name, env != "production", version)))

	routes(app)

	app.Use(func(c fiber.Ctx) error {
		if isOtherAPIVersion(c.Path(), version) {
			return response.Error(c, fiber.StatusNotFound, "api version not supported, use "+apipath.Base(version), nil)
		}
		return c.Status(fiber.StatusNotFound).JSON(fiber.Map{
			"error": "Endpoint Not Found!",
		})
	})
	return app
}

// isOtherAPIVersion: path API yang bukan versi aktif, termasuk URL lama tanpa versi
// (/api/products). Klien yang versinya tertinggal mendapat petunjuk, bukan 404 polos.
func isOtherAPIVersion(path, version string) bool {
	base := apipath.Base(version)
	return strings.HasPrefix(path, apipath.Root+"/") && path != base && !strings.HasPrefix(path, base+"/")
}
