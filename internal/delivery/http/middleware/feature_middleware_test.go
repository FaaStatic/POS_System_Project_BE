package middleware

import (
	"net/http/httptest"
	"testing"

	"github.com/gofiber/fiber/v3"
)

func TestRequireFeature_GatesRoutesWhenUnconfigured(t *testing.T) {
	tests := []struct {
		name        string
		configured  bool
		wantStatus  int
		wantHandler bool
	}{
		{
			name:        "keys absent from config: routes disabled",
			configured:  false,
			wantStatus:  fiber.StatusServiceUnavailable,
			wantHandler: false,
		},
		{
			name:        "keys present: routes work normally",
			configured:  true,
			wantStatus:  fiber.StatusOK,
			wantHandler: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			for _, path := range []string{"/api/payments/qris", "/api/payments/va", "/payments/notification"} {
				handlerRan := false
				app := fiber.New()
				gate := RequireFeature(tt.configured, "online payment")
				app.Post(path, gate, func(c fiber.Ctx) error {
					handlerRan = true
					return c.SendStatus(fiber.StatusOK)
				})

				resp, err := app.Test(httptest.NewRequest(fiber.MethodPost, path, nil))
				if err != nil {
					t.Fatalf("%s: request failed: %v", path, err)
				}
				defer resp.Body.Close()

				if resp.StatusCode != tt.wantStatus {
					t.Errorf("%s: status = %d; want %d", path, resp.StatusCode, tt.wantStatus)
				}
				if handlerRan != tt.wantHandler {
					t.Errorf("%s: handler ran = %v; want %v", path, handlerRan, tt.wantHandler)
				}
			}
		})
	}
}
