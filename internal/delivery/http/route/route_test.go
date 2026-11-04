package route_test

import (
	"net/http"
	"net/http/httptest"
	"regexp"
	"slices"
	"strings"
	"testing"

	"shop_project_be/internal/constant/apipath"
	"shop_project_be/internal/delivery/http/middleware"
	"shop_project_be/internal/delivery/http/route"

	"github.com/gofiber/fiber/v3"
	"go.uber.org/zap"
)

var (
	groups   = []string{apipath.GroupAccount, apipath.GroupCatalog, apipath.GroupSales, apipath.GroupInsight}
	paramRe  = regexp.MustCompile(`:[A-Za-z_]+`)
	crudVerb = map[string]bool{"GET": true, "POST": true, "PUT": true, "PATCH": true, "DELETE": true}
)

// Handlers nil cukup: method value dari pointer nil tidak panik sampai dipanggil,
// dan test ini tidak memanggil rute publik.
func newApp(version string) *fiber.App {
	app := fiber.New()
	route.New(route.Handlers{}, middleware.NewJwtMiddleware(nil, nil), nil, zap.NewNop(), true, version)(app)
	return app
}

// publicRoutes: satu-satunya rute yang boleh dipanggil tanpa JWT.
func publicRoutes(version string) map[string]bool {
	base := apipath.Group(version, apipath.GroupAccount)
	return map[string]bool{
		"POST " + base + "/auth/login":              true,
		"POST " + base + "/auth/refresh":            true,
		"POST " + apipath.NotificationPath(version): true,
	}
}

func TestRoutesFollowVersionGroupService(t *testing.T) {
	app := newApp("v2")
	seen := 0
	for _, r := range app.GetRoutes(true) {
		if !crudVerb[r.Method] {
			continue
		}
		seen++
		rest, ok := strings.CutPrefix(r.Path, apipath.Base("v2")+"/")
		if !ok {
			t.Errorf("%s %s is outside %s", r.Method, r.Path, apipath.Base("v2"))
			continue
		}
		group, service, found := strings.Cut(rest, "/")
		if !found || service == "" {
			t.Errorf("%s %s has no service after the group", r.Method, r.Path)
		}
		if !slices.Contains(groups, group) {
			t.Errorf("%s %s uses unknown group %q", r.Method, r.Path, group)
		}
	}
	if seen == 0 {
		t.Fatal("no routes were registered")
	}
}

func TestOnlyLoginRefreshAndWebhookSkipJWT(t *testing.T) {
	const version = "v1"
	app := newApp(version)
	public := publicRoutes(version)

	registered := map[string]bool{}
	for _, r := range app.GetRoutes(true) {
		if !crudVerb[r.Method] {
			continue
		}
		key := r.Method + " " + r.Path
		registered[key] = true
		if public[key] {
			continue
		}
		req := httptest.NewRequest(r.Method, paramRe.ReplaceAllString(r.Path, "x"), nil)
		resp, err := app.Test(req)
		if err != nil {
			t.Fatalf("%s: %v", key, err)
		}
		resp.Body.Close()
		if resp.StatusCode != http.StatusUnauthorized {
			t.Errorf("%s without a token = %d, want 401", key, resp.StatusCode)
		}
	}
	for key := range public {
		if !registered[key] {
			t.Errorf("public route %s is not registered", key)
		}
	}
}

func TestUnversionedPathsAreGone(t *testing.T) {
	app := newApp("v1")
	for _, path := range []string{"/api/products", "/api/auth/logout", "/auth/login", "/payments/notification", "/api/v1/products"} {
		resp, err := app.Test(httptest.NewRequest(http.MethodGet, path, nil))
		if err != nil {
			t.Fatalf("%s: %v", path, err)
		}
		resp.Body.Close()
		if resp.StatusCode != http.StatusNotFound {
			t.Errorf("GET %s = %d, want 404", path, resp.StatusCode)
		}
	}
}

func TestVersionComesFromConfig(t *testing.T) {
	app := newApp("v7")
	resp, err := app.Test(httptest.NewRequest(http.MethodGet, "/api/v7/catalog/products", nil))
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusUnauthorized {
		t.Errorf("v7 route = %d, want 401 (registered, JWT required)", resp.StatusCode)
	}

	resp, err = app.Test(httptest.NewRequest(http.MethodGet, "/api/v1/catalog/products", nil))
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusNotFound {
		t.Errorf("v1 route while serving v7 = %d, want 404", resp.StatusCode)
	}
}
