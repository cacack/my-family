package api

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/getkin/kin-openapi/openapi3"
	"github.com/labstack/echo/v4"

	"github.com/cacack/my-family/internal/config"
	"github.com/cacack/my-family/internal/repository"
	"github.com/cacack/my-family/internal/repository/memory"
)

// TestOpenAPI_EveryOperationDeclaresARole fails when an operation in
// openapi.yaml has no x-min-role (AC-004): the server would refuse it.
func TestOpenAPI_EveryOperationDeclaresARole(t *testing.T) {
	doc, err := openapi3.NewLoader().LoadFromData(openapiSpec)
	if err != nil {
		t.Fatal(err)
	}
	ops := 0
	for path, item := range doc.Paths.Map() {
		for method, op := range item.Operations() {
			ops++
			if _, ok := op.Extensions["x-min-role"]; !ok {
				t.Errorf("%s %s (%s) declares no x-min-role", method, path, op.OperationID)
			}
		}
	}
	if ops == 0 {
		t.Fatal("openapi.yaml has no operations")
	}
}

func TestSpecRoles(t *testing.T) {
	spec := func(ext string) []byte {
		return []byte(`openapi: 3.0.3
info: {title: t, version: "1"}
paths:
  /things/{id}:
    get:
` + ext + `
      responses: {'200': {description: ok}}
    delete:
      responses: {'204': {description: ok}}
`)
	}
	roles, err := specRoles(spec("      x-min-role: editor"))
	if err != nil {
		t.Fatal(err)
	}
	if got := roles["GET /api/v1/things/:id"]; got != "editor" {
		t.Errorf("GET role = %q, want editor (keyed by the Echo path)", got)
	}
	if role, ok := roles["DELETE /api/v1/things/:id"]; ok {
		t.Errorf("an operation without x-min-role got role %q, want it left out", role)
	}
	if _, err := specRoles(spec("      x-min-role: admin")); err == nil || !strings.Contains(err.Error(), "admin") {
		t.Errorf("x-min-role admin: err = %v, want it refused", err)
	}
}

func newOpenModeServer(t *testing.T) *Server {
	t.Helper()
	events := memory.NewEventStore()
	return NewServer(&config.Config{AuthMode: config.AuthModeOpen}, events, memory.NewReadModelStore(),
		memory.NewSnapshotStore(events), nil, WithBranchStore(memory.NewBranchStore()))
}

// TestRouteRoles_CoverEveryRoute fails when an API route, generated or
// registered by hand, has no entry in routeRoles.
func TestRouteRoles_CoverEveryRoute(t *testing.T) {
	for _, route := range newOpenModeServer(t).echo.Routes() {
		if !strings.HasPrefix(route.Path, "/api/") {
			continue
		}
		if _, ok := routeRoles[routeKey(route.Method, route.Path)]; !ok {
			t.Errorf("%s %s declares no role: add x-min-role to openapi.yaml or an entry to handRouteRoles", route.Method, route.Path)
		}
	}
	for key := range handRouteRoles {
		if _, ok := routeRoles[key]; !ok {
			t.Errorf("handRouteRoles entry %s missing from routeRoles", key)
		}
	}
}

// TestAuthenticate_RefusesAnUndeclaredRoute shows the default deny: a route
// with no role is refused even to the owner in open mode.
func TestAuthenticate_RefusesAnUndeclaredRoute(t *testing.T) {
	s := newOpenModeServer(t)
	s.echo.GET("/api/v1/undeclared", func(c echo.Context) error { return c.NoContent(http.StatusOK) })
	s.routes[routeKey(http.MethodGet, "/api/v1/undeclared")] = true

	rec := httptest.NewRecorder()
	s.echo.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/v1/undeclared", http.NoBody))
	if rec.Code != http.StatusForbidden || !strings.Contains(rec.Body.String(), CodeForbidden) {
		t.Errorf("undeclared route: %d %s, want 403 FORBIDDEN", rec.Code, rec.Body)
	}

	rec = httptest.NewRecorder()
	s.echo.ServeHTTP(rec, httptest.NewRequest(http.MethodPatch, "/api/v1/persons", http.NoBody))
	if rec.Code != http.StatusMethodNotAllowed {
		t.Errorf("a method a declared path lacks: %d %s, want the router's 405", rec.Code, rec.Body)
	}
}

// Every costly route limitAnonymousReads names is a route a visitor may call,
// so a typo or a renamed path cannot silently leave it unlimited.
func TestAnonymousCostlyRoutes_AreViewerRoutes(t *testing.T) {
	for key := range anonymousCostlyRoutes {
		if role, ok := routeRoles[key]; !ok || role != repository.RoleViewer {
			t.Errorf("%s: role %q (declared %v), want a viewer route", key, role, ok)
		}
	}
	for key := range accountRoutes {
		if _, ok := routeRoles[key]; !ok {
			t.Errorf("account route %s is not a route", key)
		}
	}
}

// TestAnonymousRouteSets fails when a hand-kept visitor route set names a
// route that does not exist or is not a viewer read, so a renamed or
// re-roled route cannot silently drop out of it.
func TestAnonymousRouteSets(t *testing.T) {
	for name, set := range map[string]map[string]bool{"anonymousCostlyRoutes": anonymousCostlyRoutes, "accountRoutes": accountRoutes} {
		for key := range set {
			if role, ok := routeRoles[key]; !ok || role != repository.RoleViewer {
				t.Errorf("%s: %s has role %q (declared %v), want a viewer route", name, key, role, ok)
			}
		}
	}
}
