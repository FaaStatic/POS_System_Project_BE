package handler

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"shop_project_be/internal/domain"
	requestdto "shop_project_be/internal/dto/request_dto"
	responsedto "shop_project_be/internal/dto/response_dto"

	"github.com/gofiber/fiber/v3"
	"go.uber.org/zap"
)

type fakeAuthUsecase struct {
	domain.UserUsecase
	login        *requestdto.UserLoginRequest
	refreshToken string
}

func (f *fakeAuthUsecase) UserLogin(ctx context.Context, dto *requestdto.UserLoginRequest) (*responsedto.UserLoginResponse, error) {
	f.login = dto
	return &responsedto.UserLoginResponse{Token: "access-1", RefreshToken: "refresh-1"}, nil
}

func (f *fakeAuthUsecase) RefreshToken(ctx context.Context, dto *requestdto.UserRefreshTokenRequest) (*responsedto.UserLoginResponse, error) {
	f.refreshToken = dto.RefreshToken
	return &responsedto.UserLoginResponse{Token: "access-2", RefreshToken: "refresh-2"}, nil
}

func newAuthApp(uc *fakeAuthUsecase) *fiber.App {
	h := NewUserHandler(uc, nil, zap.NewNop(), WebCookieConfig{Secure: true, RefreshTTL: time.Hour})
	app := fiber.New()
	app.Post("/auth/login", h.Login)
	app.Post("/auth/refresh", h.Refresh)
	return app
}

func send(t *testing.T, app *fiber.App, path, body string, header map[string]string) (*http.Response, string) {
	t.Helper()
	req := httptest.NewRequest(http.MethodPost, path, strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	for k, v := range header {
		req.Header.Set(k, v)
	}
	resp, err := app.Test(req)
	if err != nil {
		t.Fatalf("%s: %v", path, err)
	}
	raw, _ := io.ReadAll(resp.Body)
	return resp, string(raw)
}

func TestLogin_WebClientGetsHttpOnlyCookieInsteadOfBodyToken(t *testing.T) {
	uc := &fakeAuthUsecase{}
	app := newAuthApp(uc)
	creds := `{"username":"owner","password":"secret1"}`

	resp, body := send(t, app, "/auth/login", creds, map[string]string{"X-Client": "web"})
	cookie := resp.Header.Get("Set-Cookie")
	for _, want := range []string{"refresh_token=refresh-1", "HttpOnly", "SameSite=Strict", "secure"} {
		if !strings.Contains(strings.ToLower(cookie), strings.ToLower(want)) {
			t.Errorf("Set-Cookie %q lacks %q", cookie, want)
		}
	}
	if strings.Contains(body, "refresh-1") {
		t.Errorf("web login leaked the refresh token in the body: %s", body)
	}
	if !uc.login.AdminOnly {
		t.Error("web login must be restricted to superadmin")
	}

	resp, body = send(t, app, "/auth/login", creds, nil)
	if resp.Header.Get("Set-Cookie") != "" || !strings.Contains(body, "refresh-1") || uc.login.AdminOnly {
		t.Errorf("mobile login changed: cookie=%q body=%s adminOnly=%v", resp.Header.Get("Set-Cookie"), body, uc.login.AdminOnly)
	}
}

func TestRefresh_CookieOnlyHonouredWithWebHeader(t *testing.T) {
	uc := &fakeAuthUsecase{}
	app := newAuthApp(uc)
	cookie := map[string]string{"Cookie": "refresh_token=from-cookie", "X-Client": "web"}

	resp, body := send(t, app, "/auth/refresh", "", cookie)
	if resp.StatusCode != fiber.StatusOK || uc.refreshToken != "from-cookie" || strings.Contains(body, "refresh-2") {
		t.Fatalf("web refresh: status=%d token=%q body=%s", resp.StatusCode, uc.refreshToken, body)
	}

	// Tanpa X-Client (mis. form dari situs lain) cookie tidak boleh dipakai.
	uc.refreshToken = ""
	resp, _ = send(t, app, "/auth/refresh", "{}", map[string]string{"Cookie": "refresh_token=from-cookie"})
	if resp.StatusCode != fiber.StatusBadRequest || uc.refreshToken != "" {
		t.Errorf("cookie used without X-Client: status=%d token=%q", resp.StatusCode, uc.refreshToken)
	}
}
