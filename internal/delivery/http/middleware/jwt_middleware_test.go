package middleware

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"shop_project_be/internal/domain"
	"shop_project_be/pkg/jwt"

	"github.com/gofiber/fiber/v3"
	"go.uber.org/zap"
)

type fakeSessionRepo struct {
	domain.SessionRepository
	exists bool
	err    error
}

func (f *fakeSessionRepo) Exists(ctx context.Context, key string) (bool, error) {
	return f.exists, f.err
}

func newTestApp(mw *JWTMiddleware) *fiber.App {
	app := fiber.New()
	log := zap.NewNop()
	app.Get("/protected", mw.Auth(log), func(c fiber.Ctx) error {
		return c.SendString(GetUserID(c))
	})
	app.Delete("/superadmin-only", mw.Auth(log), mw.RequireRole("superadmin"), func(c fiber.Ctx) error {
		return c.SendStatus(fiber.StatusOK)
	})
	return app
}

func TestAuth_RejectsMissingToken(t *testing.T) {
	svc := jwt.NewJWTService("secret", 15*time.Minute, 24*time.Hour)
	mw := NewJwtMiddleware(svc, &fakeSessionRepo{exists: true})
	app := newTestApp(mw)

	req := httptest.NewRequest(http.MethodGet, "/protected", nil)
	resp, err := app.Test(req)
	if err != nil {
		t.Fatalf("request failed: %v", err)
	}
	if resp.StatusCode != fiber.StatusUnauthorized {
		t.Errorf("status = %d, want 401 when no Authorization header is sent", resp.StatusCode)
	}
}

func TestAuth_RejectsInvalidToken(t *testing.T) {
	svc := jwt.NewJWTService("secret", 15*time.Minute, 24*time.Hour)
	mw := NewJwtMiddleware(svc, &fakeSessionRepo{exists: true})
	app := newTestApp(mw)

	req := httptest.NewRequest(http.MethodGet, "/protected", nil)
	req.Header.Set("Authorization", "Bearer not-a-valid-jwt")
	resp, err := app.Test(req)
	if err != nil {
		t.Fatalf("request failed: %v", err)
	}
	if resp.StatusCode != fiber.StatusUnauthorized {
		t.Errorf("status = %d, want 401 for a malformed token", resp.StatusCode)
	}
}

func TestAuth_RejectsRefreshTokenOnAccessRoute(t *testing.T) {
	svc := jwt.NewJWTService("secret", 15*time.Minute, 24*time.Hour)
	mw := NewJwtMiddleware(svc, &fakeSessionRepo{exists: true})
	app := newTestApp(mw)

	pair, err := svc.GenerateTokenPair(uuidLikeID, "staff")
	if err != nil {
		t.Fatalf("generate token pair: %v", err)
	}

	req := httptest.NewRequest(http.MethodGet, "/protected", nil)
	req.Header.Set("Authorization", "Bearer "+pair.RefreshToken)
	resp, err := app.Test(req)
	if err != nil {
		t.Fatalf("request failed: %v", err)
	}
	if resp.StatusCode != fiber.StatusUnauthorized {
		t.Errorf("status = %d, want 401 when a refresh token is used where an access token is required", resp.StatusCode)
	}
}

func TestAuth_RejectsRevokedSession(t *testing.T) {
	svc := jwt.NewJWTService("secret", 15*time.Minute, 24*time.Hour)
	mw := NewJwtMiddleware(svc, &fakeSessionRepo{exists: false})
	app := newTestApp(mw)

	pair, err := svc.GenerateTokenPair(uuidLikeID, "staff")
	if err != nil {
		t.Fatalf("generate token pair: %v", err)
	}

	req := httptest.NewRequest(http.MethodGet, "/protected", nil)
	req.Header.Set("Authorization", "Bearer "+pair.AccessToken)
	resp, err := app.Test(req)
	if err != nil {
		t.Fatalf("request failed: %v", err)
	}
	if resp.StatusCode != fiber.StatusUnauthorized {
		t.Errorf("status = %d, want 401 when the Redis session no longer exists (revoked/logged out)", resp.StatusCode)
	}
}

func TestAuth_AllowsValidAccessTokenWithLiveSession(t *testing.T) {
	svc := jwt.NewJWTService("secret", 15*time.Minute, 24*time.Hour)
	mw := NewJwtMiddleware(svc, &fakeSessionRepo{exists: true})
	app := newTestApp(mw)

	pair, err := svc.GenerateTokenPair(uuidLikeID, "staff")
	if err != nil {
		t.Fatalf("generate token pair: %v", err)
	}

	req := httptest.NewRequest(http.MethodGet, "/protected", nil)
	req.Header.Set("Authorization", "Bearer "+pair.AccessToken)
	resp, err := app.Test(req)
	if err != nil {
		t.Fatalf("request failed: %v", err)
	}
	if resp.StatusCode != fiber.StatusOK {
		t.Errorf("status = %d, want 200 for a valid access token with a live session", resp.StatusCode)
	}
}

func TestAuth_RejectsTokenSignedWithDifferentSecret(t *testing.T) {
	issuer := jwt.NewJWTService("secret-a", 15*time.Minute, 24*time.Hour)
	verifier := jwt.NewJWTService("secret-b", 15*time.Minute, 24*time.Hour)
	mw := NewJwtMiddleware(verifier, &fakeSessionRepo{exists: true})
	app := newTestApp(mw)

	pair, err := issuer.GenerateTokenPair(uuidLikeID, "staff")
	if err != nil {
		t.Fatalf("generate token pair: %v", err)
	}

	req := httptest.NewRequest(http.MethodGet, "/protected", nil)
	req.Header.Set("Authorization", "Bearer "+pair.AccessToken)
	resp, err := app.Test(req)
	if err != nil {
		t.Fatalf("request failed: %v", err)
	}
	if resp.StatusCode != fiber.StatusUnauthorized {
		t.Errorf("status = %d, want 401 for a token signed with a secret this server does not recognize", resp.StatusCode)
	}
}

func TestAuth_RejectsExpiredAccessToken(t *testing.T) {
	svc := jwt.NewJWTService("secret", 15*time.Minute, 24*time.Hour)
	mw := NewJwtMiddleware(svc, &fakeSessionRepo{exists: true})
	app := newTestApp(mw)

	svcShortTTL := jwt.NewJWTService("secret", 0, 24*time.Hour)
	time.Sleep(2 * time.Millisecond)
	pair, err := svcShortTTL.GenerateTokenPair(uuidLikeID, "staff")
	if err != nil {
		t.Fatalf("generate token pair: %v", err)
	}
	time.Sleep(2 * time.Millisecond)

	req := httptest.NewRequest(http.MethodGet, "/protected", nil)
	req.Header.Set("Authorization", "Bearer "+pair.AccessToken)
	resp, err := app.Test(req)
	if err != nil {
		t.Fatalf("request failed: %v", err)
	}
	if resp.StatusCode != fiber.StatusUnauthorized {
		t.Errorf("status = %d, want 401 for an expired access token", resp.StatusCode)
	}
}

func TestRequireRole_AllowsMatchingRole(t *testing.T) {
	svc := jwt.NewJWTService("secret", 15*time.Minute, 24*time.Hour)
	mw := NewJwtMiddleware(svc, &fakeSessionRepo{exists: true})
	app := newTestApp(mw)

	pair, err := svc.GenerateTokenPair(uuidLikeID, "superadmin")
	if err != nil {
		t.Fatalf("generate token pair: %v", err)
	}

	req := httptest.NewRequest(http.MethodDelete, "/superadmin-only", nil)
	req.Header.Set("Authorization", "Bearer "+pair.AccessToken)
	resp, err := app.Test(req)
	if err != nil {
		t.Fatalf("request failed: %v", err)
	}
	if resp.StatusCode != fiber.StatusOK {
		t.Errorf("status = %d, want 200 for a superadmin hitting a superadmin-only route", resp.StatusCode)
	}
}

func TestRequireRole_RejectsNonMatchingRole(t *testing.T) {
	svc := jwt.NewJWTService("secret", 15*time.Minute, 24*time.Hour)
	mw := NewJwtMiddleware(svc, &fakeSessionRepo{exists: true})
	app := newTestApp(mw)

	pair, err := svc.GenerateTokenPair(uuidLikeID, "staff")
	if err != nil {
		t.Fatalf("generate token pair: %v", err)
	}

	req := httptest.NewRequest(http.MethodDelete, "/superadmin-only", nil)
	req.Header.Set("Authorization", "Bearer "+pair.AccessToken)
	resp, err := app.Test(req)
	if err != nil {
		t.Fatalf("request failed: %v", err)
	}
	if resp.StatusCode != fiber.StatusForbidden {
		t.Errorf("status = %d, want 403 for staff hitting a superadmin-only route", resp.StatusCode)
	}
}

const uuidLikeID = "11111111-1111-1111-1111-111111111111"
