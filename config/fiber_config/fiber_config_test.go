package fiberconfig

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"

	envconfig "shop_project_be/config/env_config"
	"shop_project_be/internal/constant/apipath"

	"github.com/gofiber/fiber/v3"
	"go.uber.org/zap"
)

// TestInitFiberVersionedDocsAndHints menjalankan InitFiber sungguhan dengan versi v2:
// UI Swagger di /v2, spec berawalan /api/v2, "/" dialihkan, dan klien yang masih
// memakai URL lama (tanpa versi atau v1) mendapat petunjuk versi.
func TestInitFiberVersionedDocsAndHints(t *testing.T) {
	spec := `{"swagger":"2.0","info":{"title":"x","version":"1"},"paths":{"/api/v1/catalog/products":{"get":{}}}}`
	dir := t.TempDir()
	if err := os.WriteFile(dir+"/swagger.json", []byte(spec), 0o600); err != nil {
		t.Fatal(err)
	}
	t.Chdir(dir)

	cfg := &envconfig.Config{App: envconfig.AppConfig{Name: "shop", APIVersion: "v2"}}
	app := InitFiber("development", cfg, zap.NewNop(), nil, func(fiber.Router) {})
	get := func(path string) (*http.Response, string) {
		t.Helper()
		resp, err := app.Test(httptest.NewRequest(http.MethodGet, path, nil))
		if err != nil {
			t.Fatalf("GET %s: %v", path, err)
		}
		defer resp.Body.Close()
		body, _ := io.ReadAll(resp.Body)
		return resp, string(body)
	}

	resp, _ := get("/")
	if resp.StatusCode != http.StatusSeeOther || resp.Header.Get("Location") != "/v2" {
		t.Errorf("GET / = %d -> %q, want 303 -> /v2", resp.StatusCode, resp.Header.Get("Location"))
	}

	if resp, _ = get("/v2"); resp.StatusCode != http.StatusOK {
		t.Errorf("GET /v2 = %d, want the Swagger UI", resp.StatusCode)
	}

	resp, body := get("/v2/swagger.json")
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("GET /v2/swagger.json = %d", resp.StatusCode)
	}
	var served struct {
		Paths map[string]json.RawMessage `json:"paths"`
	}
	if err := json.Unmarshal([]byte(body), &served); err != nil {
		t.Fatal(err)
	}
	if _, ok := served.Paths["/api/v2/catalog/products"]; !ok || len(served.Paths) != 1 {
		t.Errorf("served spec paths = %v, want only /api/v2/catalog/products", served.Paths)
	}

	for _, path := range []string{"/api/products", "/api/v1/catalog/products"} {
		resp, body = get(path)
		if resp.StatusCode != http.StatusNotFound || !strings.Contains(body, "api version not supported, use /api/v2") {
			t.Errorf("GET %s = %d %s, want 404 with the supported version", path, resp.StatusCode, body)
		}
	}

	resp, body = get("/api/v2/nowhere")
	if resp.StatusCode != http.StatusNotFound || strings.Contains(body, "version") {
		t.Errorf("GET /api/v2/nowhere = %d %s, want a plain 404", resp.StatusCode, body)
	}
}

func TestIsOtherAPIVersion(t *testing.T) {
	tests := []struct {
		path string
		want bool
	}{
		{"/api/v2/catalog/products", false},
		{"/api/v1/catalog/products", true},
		{"/api/products", true},
		{"/api/v3/x", true},
		{"/api/v2", false},
		{"/v2", false},
		{"/", false},
		{"/unknown", false},
	}
	for _, tt := range tests {
		if got := isOtherAPIVersion(tt.path, "v2"); got != tt.want {
			t.Errorf("isOtherAPIVersion(%q, v2) = %v, want %v", tt.path, got, tt.want)
		}
	}
}

func TestWithVersionRewritesOnlyTheVersionPrefix(t *testing.T) {
	paths := map[string]interface{}{
		"/api/v1/sales/payments/qris":   "qris",
		"/api/v1/catalog/products/{id}": "product",
		"/other":                        "kept",
	}

	got := withVersion(paths, "v3").(map[string]interface{})
	for _, want := range []string{"/api/v3/sales/payments/qris", "/api/v3/catalog/products/{id}", "/other"} {
		if _, ok := got[want]; !ok {
			t.Errorf("missing %q in %v", want, got)
		}
	}
	if len(got) != len(paths) {
		t.Errorf("got %d paths, want %d", len(got), len(paths))
	}

	same := withVersion(paths, apipath.DefaultVersion).(map[string]interface{})
	if _, ok := same["/api/v1/sales/payments/qris"]; !ok {
		t.Error("default version must leave paths untouched")
	}
	if withVersion(nil, "v2") != nil {
		t.Error("a spec without paths must stay without paths")
	}
}

func TestSwaggerServedUnderTheVersion(t *testing.T) {
	cfg := GetSwaggerConfig("shop", true, "v4")
	if cfg.BasePath != "/v4" {
		t.Errorf("BasePath = %q, want /v4", cfg.BasePath)
	}
}
