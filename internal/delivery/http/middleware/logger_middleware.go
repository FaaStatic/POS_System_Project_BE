package middleware

import (
	"time"

	"github.com/gofiber/fiber/v3"
	"github.com/gofiber/fiber/v3/middleware/requestid"
	"go.uber.org/zap"
)

// LoggerMiddleware mencatat satu baris JSON per request; Alloy membacanya dari
// stdout container dan mengirimnya ke Loki. Pasang sebagai middleware paling
// luar (setelah requestid, sebelum recover): error dari lapisan di bawahnya
// diproses ErrorHandler di sini, seperti middleware logger bawaan Fiber, supaya
// status yang dicatat sama dengan yang diterima client, dan panic yang
// dipulihkan recover tetap tercatat sebagai 500.
func LoggerMiddleware(log *zap.Logger) fiber.Handler {
	return func(c fiber.Ctx) error {
		start := time.Now()
		if err := c.Next(); err != nil {
			if handlerErr := c.App().ErrorHandler(c, err); handlerErr != nil {
				_ = c.SendStatus(fiber.StatusInternalServerError)
			}
		}
		// Path tanpa query string (bisa memuat token). route = pola ("/api/x/:id")
		// untuk mengelompokkan per endpoint tanpa meledakkan jumlah nilai unik.
		log.Info("HTTP Request",
			zap.String("request_id", requestid.FromContext(c)),
			zap.String("method", c.Method()),
			zap.String("path", c.Path()),
			zap.String("route", c.Route().Path),
			zap.Int("status", c.Response().StatusCode()),
			zap.Float64("latency_ms", float64(time.Since(start))/float64(time.Millisecond)),
			zap.String("ip", c.IP()),
			zap.ByteString("user_agent", c.Request().Header.UserAgent()),
		)
		return nil
	}
}
