package middleware

import (
	"strings"
	"time"

	"shop_project_be/pkg/response"

	"github.com/gofiber/fiber/v3"
	"github.com/gofiber/fiber/v3/middleware/limiter"
)

func GetLoginLimiter(store fiber.Storage) limiter.Config {
	return limiter.Config{
		Max:               5,
		Expiration:        2 * time.Minute,
		Storage:           store,
		KeyGenerator:      func(c fiber.Ctx) string { return "login:" + c.IP() },
		LimiterMiddleware: limiter.SlidingWindow{},
		LimitReached:      tooManyRequests("too many login attempts, please retry in 2 minutes"),
	}
}

func GetRefreshLimiter(store fiber.Storage) limiter.Config {
	return limiter.Config{
		Max:               20,
		Expiration:        2 * time.Minute,
		Storage:           store,
		KeyGenerator:      func(c fiber.Ctx) string { return "refresh:" + c.IP() },
		LimiterMiddleware: limiter.SlidingWindow{},
		LimitReached:      tooManyRequests("too many token refreshes, please retry in 2 minutes"),
	}
}

func GetGlobalLimiter(store fiber.Storage) limiter.Config {
	return limiter.Config{
		Max:               50,
		Expiration:        time.Minute,
		Storage:           store,
		KeyGenerator:      func(c fiber.Ctx) string { return "global:" + c.IP() },
		LimiterMiddleware: limiter.SlidingWindow{},
		LimitReached:      tooManyRequests("too many requests, please retry in a minute"),
		Next: func(c fiber.Ctx) bool {
			p := c.Path()
			return p == "/" || strings.HasPrefix(p, "/api/") || p == "/payments/notification"
		},
	}
}

func GetUserLimiter(store fiber.Storage) limiter.Config {
	return limiter.Config{
		Max:               300,
		Expiration:        time.Minute,
		Storage:           store,
		KeyGenerator:      func(c fiber.Ctx) string { return "user:" + GetUserID(c) },
		LimiterMiddleware: limiter.SlidingWindow{},
		LimitReached:      tooManyRequests("too many requests, please retry in a minute"),
	}
}

func GetWebhookLimiter(store fiber.Storage) limiter.Config {
	return limiter.Config{
		Max:               60,
		Expiration:        time.Minute,
		Storage:           store,
		KeyGenerator:      func(c fiber.Ctx) string { return "webhook:" + c.IP() },
		LimiterMiddleware: limiter.SlidingWindow{},
		LimitReached:      tooManyRequests("too many requests, please retry later"),
	}
}

func tooManyRequests(msg string) fiber.Handler {
	return func(c fiber.Ctx) error {
		return response.Error(c, fiber.StatusTooManyRequests, msg, nil)
	}
}
