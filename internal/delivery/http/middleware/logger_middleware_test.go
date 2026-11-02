package middleware

import (
	"errors"
	"net/http/httptest"
	"testing"

	"github.com/gofiber/fiber/v3"
	"github.com/gofiber/fiber/v3/middleware/recover"
	"github.com/gofiber/fiber/v3/middleware/requestid"
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
	"go.uber.org/zap/zaptest/observer"
)

func loggedApp(t *testing.T) (*fiber.App, *observer.ObservedLogs) {
	t.Helper()
	core, logs := observer.New(zapcore.InfoLevel)
	app := fiber.New(fiber.Config{
		// Observer menyimpan string; tanpa ini c.Path() menunjuk buffer yang dipakai ulang.
		Immutable: true,
		ErrorHandler: func(c fiber.Ctx, err error) error {
			code := fiber.StatusInternalServerError
			var fe *fiber.Error
			if errors.As(err, &fe) {
				code = fe.Code
			}
			return c.Status(code).SendString("error")
		},
	})
	app.Use(requestid.New())
	app.Use(LoggerMiddleware(zap.New(core)))
	app.Use(recover.New())
	return app, logs
}

func TestLoggerMiddleware_LogsTheStatusTheClientReceives(t *testing.T) {
	app, logs := loggedApp(t)
	app.Get("/ok", func(c fiber.Ctx) error { return c.SendString("ok") })
	app.Get("/reject", func(c fiber.Ctx) error { return fiber.NewError(fiber.StatusTeapot, "no") })
	app.Get("/panic", func(c fiber.Ctx) error { panic("boom") })

	tests := []struct {
		target     string
		wantStatus int
		wantPath   string
	}{
		{"/ok", 200, "/ok"},
		// ErrorHandler berjalan di dalam logger: tanpa itu baris log ini 200.
		{"/reject?token=secret", fiber.StatusTeapot, "/reject"},
		{"/panic", 500, "/panic"},
	}
	for _, tt := range tests {
		resp, err := app.Test(httptest.NewRequest(fiber.MethodGet, tt.target, nil))
		if err != nil {
			t.Fatalf("%s: %v", tt.target, err)
		}
		resp.Body.Close()
		if resp.StatusCode != tt.wantStatus {
			t.Fatalf("%s: response status = %d; want %d", tt.target, resp.StatusCode, tt.wantStatus)
		}
	}

	entries := logs.All()
	if len(entries) != len(tests) {
		t.Fatalf("got %d log lines; want %d", len(entries), len(tests))
	}
	for i, tt := range tests {
		fields := entries[i].ContextMap()
		if fields["status"] != int64(tt.wantStatus) {
			t.Errorf("%s: logged status = %v; want %d", tt.target, fields["status"], tt.wantStatus)
		}
		if fields["path"] != tt.wantPath {
			t.Errorf("%s: logged path = %v; want %q without the query string", tt.target, fields["path"], tt.wantPath)
		}
		if id, _ := fields["request_id"].(string); id == "" {
			t.Errorf("%s: request_id missing", tt.target)
		}
		if _, ok := fields["latency_ms"].(float64); !ok {
			t.Errorf("%s: latency_ms = %T; want a number", tt.target, fields["latency_ms"])
		}
	}
}
