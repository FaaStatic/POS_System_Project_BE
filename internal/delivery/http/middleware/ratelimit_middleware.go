package middleware

import (
	"strings"
	"time"

	"shop_project_be/pkg/response"

	"github.com/gofiber/fiber/v3"
	"github.com/gofiber/fiber/v3/middleware/limiter"
)

func slidingLimit(store fiber.Storage, max int, exp time.Duration, key func(fiber.Ctx) string, msg string) limiter.Config {
	return limiter.Config{
		Max:               max,
		Expiration:        exp,
		Storage:           store,
		KeyGenerator:      key,
		LimiterMiddleware: limiter.SlidingWindow{},
		LimitReached:      tooManyRequests(msg),
	}
}

func GetLoginLimiter(store fiber.Storage) limiter.Config {
	return slidingLimit(store, 5, 2*time.Minute, func(c fiber.Ctx) string { return "login:" + c.IP() },
		"too many login attempts, please retry in 2 minutes")
}

func GetRefreshLimiter(store fiber.Storage) limiter.Config {
	return slidingLimit(store, 20, 2*time.Minute, func(c fiber.Ctx) string { return "refresh:" + c.IP() },
		"too many token refreshes, please retry in 2 minutes")
}

func GetGlobalLimiter(store fiber.Storage) limiter.Config {
	cfg := slidingLimit(store, 50, time.Minute, func(c fiber.Ctx) string { return "global:" + c.IP() },
		"too many requests, please retry in a minute")
	cfg.Next = func(c fiber.Ctx) bool {
		p := c.Path()
		return p == "/" || strings.HasPrefix(p, "/api/") || p == "/payments/notification"
	}
	return cfg
}

func GetUserLimiter(store fiber.Storage) limiter.Config {
	return slidingLimit(store, 300, time.Minute, func(c fiber.Ctx) string { return "user:" + GetUserID(c) },
		"too many requests, please retry in a minute")
}

func GetWebhookLimiter(store fiber.Storage) limiter.Config {
	return slidingLimit(store, 60, time.Minute, func(c fiber.Ctx) string { return "webhook:" + c.IP() },
		"too many requests, please retry later")
}

func tooManyRequests(msg string) fiber.Handler {
	return func(c fiber.Ctx) error {
		return response.Error(c, fiber.StatusTooManyRequests, msg, nil)
	}
}
