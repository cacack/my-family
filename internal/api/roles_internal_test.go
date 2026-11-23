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

// TestAnonymousRouteSets fails when a hand-kept visitor route set names a
// route that does not exist or is not a viewer read, so a renamed or
// re-roled route cannot silently drop out of it.
func TestAnonymousRouteSets(t *testing.T) {
	for name, set := range map[string]map[string]bool{"anonymousCostlyRoutes": anonymousCostlyRoutes, "signedInRoutes": signedInRoutes} {
		for key := range set {
			if role, ok := routeRoles[key]; !ok || role != repository.RoleViewer {
				t.Errorf("%s: %s has role %q (declared %v), want a viewer route", name, key, role, ok)
			}
		}
	}
}

// TestOpenAPI_SignedInRoutesSaySo fails when an operation refused to visitors
// (signedInRoutes) does not say so in its openapi.yaml description, so an API
// client can tell a 401 there means "sign in" (#1469). /auth/me is the
// signed-in user's own account, which says it in its summary.
func TestOpenAPI_SignedInRoutesSaySo(t *testing.T) {
	doc, err := openapi3.NewLoader().LoadFromData(openapiSpec)
	if err != nil {
		t.Fatal(err)
	}
	documented := make(map[string]bool)
	for path, item := range doc.Paths.Map() {
		echoPath := apiBase + specParam.ReplaceAllString(path, ":$1")
		for method, op := range item.Operations() {
			description := strings.Join(strings.Fields(op.Description), " ")
			documented[routeKey(method, echoPath)] = strings.Contains(description, "Requires a signed-in user")
		}
	}
	for key := range signedInRoutes {
		if strings.HasSuffix(key, " "+apiBase+"/auth/me") {
			continue
		}
		if !documented[key] {
			t.Errorf("%s is refused to visitors, but its openapi.yaml description does not say \"Requires a signed-in user\"", key)
		}
	}
}

// anonymousCheapRoutes are the viewer routes a visitor may call unlimited:
// each reads one record, one record's own sub-lists, or a fixed document, so
// its cost does not grow with the tree.
var anonymousCheapRoutes = []string{
	"GET /api/v1/associations/:id",
	"GET /api/v1/branches/:id",
	"GET /api/v1/citation-templates",
	"GET /api/v1/citation-templates/:id",
	"POST /api/v1/citation-templates/:id/preview",
	"GET /api/v1/citations/:id",
	"GET /api/v1/citations/:id/format",
	"GET /api/v1/config",
	"GET /api/v1/docs",
	"GET /api/v1/evidence-analyses/:id",
	"GET /api/v1/evidence-analyses/by-fact",
	"GET /api/v1/evidence-conflicts/:id",
	"GET /api/v1/evidence-conflicts/by-subject/:subjectId",
	"GET /api/v1/families/:id",
	"GET /api/v1/families/:id/lds-ordinances",
	"GET /api/v1/families/:id/media",
	"GET /api/v1/lds-ordinances/:id",
	"GET /api/v1/media/:id",
	"GET /api/v1/notes/:id",
	"GET /api/v1/openapi.yaml",
	"GET /api/v1/persons/:id",
	"GET /api/v1/persons/:id/associations",
	"GET /api/v1/persons/:id/citations",
	"GET /api/v1/persons/:id/lds-ordinances",
	"GET /api/v1/persons/:id/media",
	"GET /api/v1/persons/:id/names",
	"GET /api/v1/proof-summaries/:id",
	"GET /api/v1/proof-summaries/by-fact",
	"GET /api/v1/quality/persons/:id",
	"GET /api/v1/repositories/:id",
	"GET /api/v1/research-logs/:id",
	"GET /api/v1/research-logs/by-subject/:subjectId",
	"GET /api/v1/settings/home-person",
	"GET /api/v1/sources/:id",
	"GET /api/v1/sources/:id/media",
	"GET /api/v1/submitters/:id",
}

// TestAnonymousRouteClassification puts every viewer route in exactly one
// class: refused to visitors (signedInRoutes), limited for them
// (anonymousCostlyRoutes), or cheap. A new viewer route fails it until it is
// classified, so a list whose cost grows with the tree cannot be left
// unlimited by accident.
func TestAnonymousRouteClassification(t *testing.T) {
	cheap := make(map[string]bool, len(anonymousCheapRoutes))
	for _, key := range anonymousCheapRoutes {
		cheap[key] = true
	}
	for key, role := range routeRoles {
		if role != repository.RoleViewer {
			if cheap[key] {
				t.Errorf("%s is listed cheap but needs %s, not viewer", key, role)
			}
			continue
		}
		classes := 0
		for _, in := range []bool{signedInRoutes[key], anonymousCostlyRoutes[key], cheap[key]} {
			if in {
				classes++
			}
		}
		if classes != 1 {
			t.Errorf("%s is in %d of signedInRoutes, anonymousCostlyRoutes and anonymousCheapRoutes, want exactly 1", key, classes)
		}
	}
	for key := range cheap {
		if _, ok := routeRoles[key]; !ok {
			t.Errorf("cheap route %s is not a route", key)
		}
	}
}
