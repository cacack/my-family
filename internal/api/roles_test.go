package api_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"regexp"
	"strings"
	"testing"

	"github.com/google/uuid"

	"github.com/cacack/my-family/internal/api"
	"github.com/cacack/my-family/internal/auth"
	"github.com/cacack/my-family/internal/config"
	"github.com/cacack/my-family/internal/repository"
	"github.com/cacack/my-family/internal/repository/memory"
)

// The role table (ADR-007), written out independently of openapi.yaml so the
// spec is checked against it: a GET reads the tree and needs a viewer, any
// other method changes it and needs an editor, except for the routes below.
// A new route outside those defaults needs an entry here as well as its
// x-min-role in openapi.yaml (or handRouteRoles in roles.go for a route
// registered by hand).
var (
	publicRoutes = routeSet(
		"GET /api/v1/health",
		"GET /api/v1/auth/status",
		"POST /api/v1/auth/setup",
		"POST /api/v1/auth/sign-in",
		"POST /api/v1/auth/sign-out",
		"POST /api/v1/auth/invites/accept",
		"POST /api/v1/auth/password-reset",
	)
	ownerRoutes = routeSet(
		"POST /api/v1/gedcom/import",
		"POST /api/v1/gedcom/import/stream",
		"POST /api/v1/demo/reset",
		"POST /api/v1/persons/:id/rollback",
		"POST /api/v1/families/:id/rollback",
		"POST /api/v1/sources/:id/rollback",
		"POST /api/v1/citations/:id/rollback",
		"GET /api/v1/users",
		"PATCH /api/v1/users/:id",
		"POST /api/v1/users/:id/password-reset",
		"POST /api/v1/invites",
		"GET /api/v1/invites",
		"DELETE /api/v1/invites/:linkId",
		"GET /api/v1/password-resets",
		"DELETE /api/v1/password-resets/:linkId",
		"GET /api/v1/auth-audit",
		"GET /api/v1/settings/privacy",
		"PUT /api/v1/settings/privacy",
		"GET /api/v1/backup",
		"GET /api/v1/backup/check",
	)
	// Exports are reads an editor needs: a viewer may not take the tree away.
	editorReads = routeSet(
		"GET /api/v1/gedcom/export",
		"GET /api/v1/gedcom/export/preview",
		"GET /api/v1/export/tree",
		"GET /api/v1/export/persons",
		"GET /api/v1/export/families",
		"GET /api/v1/export/sources",
		"GET /api/v1/export/citations",
		"GET /api/v1/export/events",
		"GET /api/v1/export/attributes",
		"GET /api/v1/export/estimate",
	)
	// Posts that change nothing in the tree, and a user's own account.
	viewerWrites = routeSet(
		"POST /api/v1/citation-templates/:id/preview",
		"PATCH /api/v1/auth/me",
		// Each account's own API tokens (#1416).
		"POST /api/v1/auth/me/tokens",
		"DELETE /api/v1/auth/me/tokens/:tokenId",
		// Each account's own home person (#1443).
		"PUT /api/v1/settings/home-person",
		"DELETE /api/v1/settings/home-person",
	)
)

func routeSet(keys ...string) map[string]bool {
	set := make(map[string]bool, len(keys))
	for _, key := range keys {
		set[key] = true
	}
	return set
}

// expectedRole is the role table's entry for a route.
func expectedRole(key string) repository.Role {
	switch {
	case publicRoutes[key]:
		return "public"
	case ownerRoutes[key]:
		return repository.RoleOwner
	case editorReads[key]:
		return repository.RoleEditor
	case viewerWrites[key]:
		return repository.RoleViewer
	case strings.HasPrefix(key, http.MethodGet+" "):
		return repository.RoleViewer
	default:
		return repository.RoleEditor
	}
}

// addMember creates a user named after role holding it, and signs them in.
func (f *authFixture) addMember(t *testing.T, role repository.Role) *http.Cookie {
	t.Helper()
	hash, err := auth.HashPassword(ownerPassword)
	if err != nil {
		t.Fatalf("HashPassword: %v", err)
	}
	user := &repository.User{ID: uuid.New(), Username: string(role), DisplayName: string(role), PasswordHash: hash}
	if err := f.accounts.CreateUser(context.Background(), user, repository.DefaultTreeID, role); err != nil {
		t.Fatalf("CreateUser(%s): %v", role, err)
	}
	return f.signInAs(t, string(role))
}

func (f *authFixture) signInAs(t *testing.T, username string) *http.Cookie {
	t.Helper()
	rec := f.do(t, request{method: http.MethodPost, path: "/api/v1/auth/sign-in", body: `{"username":"` + username + `","password":"` + ownerPassword + `"}`})
	if rec.Code != http.StatusOK {
		t.Fatalf("sign-in as %s: status %d, body %s", username, rec.Code, rec.Body)
	}
	return sessionCookieOf(t, rec)
}

// rolesFixture is an accounts-mode server with a signed-in owner, editor and
// viewer.
func rolesFixture(t *testing.T) (*authFixture, map[repository.Role]*http.Cookie) {
	t.Helper()
	f := newAuthFixture(t)
	f.createOwner(t)
	return f, map[repository.Role]*http.Cookie{
		repository.RoleOwner:  f.signIn(t),
		repository.RoleEditor: f.addMember(t, repository.RoleEditor),
		repository.RoleViewer: f.addMember(t, repository.RoleViewer),
	}
}

// forbidden reports whether the role check refused the request.
func forbidden(rec *httptest.ResponseRecorder) bool {
	return rec.Code == http.StatusForbidden && errorCodeOf(rec) == api.CodeForbidden
}

// errorCodeOf is the code of an API error body, or "" for any other body.
func errorCodeOf(rec *httptest.ResponseRecorder) string {
	var body struct {
		Code string `json:"code"`
	}
	_ = json.Unmarshal(rec.Body.Bytes(), &body)
	return body.Code
}

var routeParam = regexp.MustCompile(`:[A-Za-z0-9]+`)

// specPathParam matches an OpenAPI path parameter, `{id}`.
var specPathParam = regexp.MustCompile(`\{([^}/]+)\}`)

// TestRoles_EveryRouteForEveryRole calls every API route the server registers
// as a viewer, an editor and an owner, and checks that the role check refuses
// exactly the roles below the route's entry in the role table. Path
// parameters are fresh IDs and bodies empty: handlers may answer 400 or 404,
// but only the role check answers 403 FORBIDDEN.
func TestRoles_EveryRouteForEveryRole(t *testing.T) {
	f, cookies := rolesFixture(t)
	id := uuid.New().String()

	visited := make(map[string]bool)
	for _, route := range f.server.Echo().Routes() {
		if !strings.HasPrefix(route.Path, "/api/") {
			continue
		}
		key := route.Method + " " + route.Path
		visited[key] = true
		want := expectedRole(key)
		path := routeParam.ReplaceAllString(route.Path, id)
		body := ""
		if route.Method != http.MethodGet {
			body = "{}"
		}

		if want == "public" {
			rec := f.do(t, request{method: route.Method, path: path, body: body})
			if code := errorCodeOf(rec); code == api.CodeUnauthenticated || code == api.CodeForbidden {
				t.Errorf("%s without a session: %d %s, want it served", key, rec.Code, rec.Body)
			}
			continue
		}
		for _, role := range []repository.Role{repository.RoleViewer, repository.RoleEditor, repository.RoleOwner} {
			rec := f.do(t, request{method: route.Method, path: path, body: body, cookie: cookies[role]})
			if got, wantRefused := forbidden(rec), !role.Includes(want); got != wantRefused {
				t.Errorf("%s as %s (route needs %s): %d %s; refused = %v, want %v",
					key, role, want, rec.Code, rec.Body, got, wantRefused)
			}
		}
	}
	// Every operation in openapi.yaml must have been called above, or the loop
	// would prove nothing about it.
	operations := 0
	for path, item := range apiSpec.Paths.Map() {
		echoPath := "/api/v1" + specPathParam.ReplaceAllString(path, ":$1")
		for method := range item.Operations() {
			operations++
			if key := method + " " + echoPath; !visited[key] {
				t.Errorf("%s is in openapi.yaml but was not called as a registered route", key)
			}
		}
	}
	if operations == 0 {
		t.Error("openapi.yaml has no operations")
	}
}

func TestRoles_EditorWorksInBranchesAndExports(t *testing.T) {
	f, cookies := rolesFixture(t)
	editor := cookies[repository.RoleEditor]

	newBranch := func(name string) string {
		t.Helper()
		rec := f.do(t, request{method: http.MethodPost, path: "/api/v1/branches", body: `{"name":"` + name + `"}`, cookie: editor})
		if rec.Code != http.StatusCreated {
			t.Fatalf("editor creates a branch: %d %s", rec.Code, rec.Body)
		}
		return decode[struct {
			ID string `json:"id"`
		}](t, rec).ID
	}
	merged := newBranch("merged")
	if rec := f.do(t, request{method: http.MethodPost, path: "/api/v1/persons?branch=" + merged, body: `{"given_name":"Ada","surname":"Byron"}`, cookie: editor}); rec.Code != http.StatusCreated {
		t.Fatalf("editor adds a person in the branch: %d %s", rec.Code, rec.Body)
	}
	if rec := f.do(t, request{method: http.MethodPost, path: "/api/v1/branches/" + merged + "/merge", body: `{}`, cookie: editor}); rec.Code != http.StatusOK {
		t.Errorf("editor merges a branch: %d %s", rec.Code, rec.Body)
	}
	if rec := f.do(t, request{method: http.MethodPost, path: "/api/v1/branches/" + newBranch("closed") + "/close", body: `{"outcome":"disproved"}`, cookie: editor}); rec.Code != http.StatusOK {
		t.Errorf("editor closes a branch: %d %s", rec.Code, rec.Body)
	}
	if rec := f.do(t, request{method: http.MethodGet, path: "/api/v1/gedcom/export", cookie: editor}); rec.Code != http.StatusOK {
		t.Errorf("editor exports: %d %s", rec.Code, rec.Body)
	}
	if rec := f.do(t, request{method: http.MethodGet, path: "/api/v1/gedcom/export", cookie: cookies[repository.RoleViewer]}); !forbidden(rec) {
		t.Errorf("viewer exports: %d %s, want 403 FORBIDDEN", rec.Code, rec.Body)
	}
}

func TestRoles_AccountCarriesTheRole(t *testing.T) {
	f, cookies := rolesFixture(t)
	for role, cookie := range cookies {
		status := decode[api.AuthStatus](t, f.do(t, request{method: http.MethodGet, path: "/api/v1/auth/status", cookie: cookie}))
		if status.Account == nil || status.Account.Role != api.AccountRole(role) {
			t.Errorf("auth status as %s: account %+v", role, status.Account)
		}
		me := decode[api.Account](t, f.do(t, request{method: http.MethodGet, path: "/api/v1/auth/me", cookie: cookie}))
		if me.Role != api.AccountRole(role) {
			t.Errorf("auth/me as %s: role %q", role, me.Role)
		}
	}
	rec := f.do(t, request{method: http.MethodPost, path: "/api/v1/auth/sign-in", body: `{"username":"editor","password":"` + ownerPassword + `"}`})
	if got := decode[api.Account](t, rec); got.Role != api.AccountRoleEditor {
		t.Errorf("sign-in as editor: role %q", got.Role)
	}
}

func TestRoles_ForbiddenSaysWhy(t *testing.T) {
	f, cookies := rolesFixture(t)
	rec := f.do(t, request{method: http.MethodPost, path: "/api/v1/persons", body: `{"given_name":"A","surname":"B"}`, cookie: cookies[repository.RoleViewer]})
	if !forbidden(rec) || !strings.Contains(rec.Body.String(), "viewer") || !strings.Contains(rec.Body.String(), "editor") {
		t.Errorf("viewer creates a person: %d %s, want 403 FORBIDDEN naming both roles", rec.Code, rec.Body)
	}

	// A user with no membership of the tree may do nothing that needs a role.
	hash, err := auth.HashPassword(ownerPassword)
	if err != nil {
		t.Fatal(err)
	}
	stranger := &repository.User{ID: uuid.New(), Username: "stranger", DisplayName: "Stranger", PasswordHash: hash}
	if err := f.accounts.CreateUser(context.Background(), stranger, uuid.New(), repository.RoleOwner); err != nil {
		t.Fatal(err)
	}
	rec = f.do(t, request{method: http.MethodGet, path: "/api/v1/persons", cookie: f.signInAs(t, "stranger")})
	if !forbidden(rec) || !strings.Contains(rec.Body.String(), "no role") {
		t.Errorf("non-member lists persons: %d %s, want 403 FORBIDDEN", rec.Code, rec.Body)
	}
}

// The citation template preview is a POST a viewer may call because it
// changes nothing: it renders the posted fields against a built-in template
// and writes no event.
func TestRoles_ViewerCitationPreviewChangesNothing(t *testing.T) {
	f, cookies := rolesFixture(t)
	before := f.events.EventCount()
	rec := f.do(t, request{method: http.MethodPost, path: "/api/v1/citation-templates/census.us.federal/preview", cookie: cookies[repository.RoleViewer],
		body: `{"fields":{"year":"1850","state":"Ohio","county":"Adams","person_name":"John Doe"}}`})
	if rec.Code != http.StatusOK {
		t.Fatalf("viewer preview: %d %s", rec.Code, rec.Body)
	}
	if got := decode[api.FormattedCitation](t, rec).Full; !strings.Contains(got, "1850 U.S. Census, Ohio, Adams County") {
		t.Errorf("preview = %q, want the rendered census citation", got)
	}
	if after := f.events.EventCount(); after != before {
		t.Errorf("events after a preview = %d, want %d: a preview must not write", after, before)
	}
}

func TestRoles_GedcomExportsAreRateLimitedPerUser(t *testing.T) {
	f, cookies := rolesFixture(t)
	export := func(path string, cookie *http.Cookie) int {
		return f.do(t, request{method: http.MethodGet, path: path, cookie: cookie}).Code
	}
	paths := []string{"/api/v1/gedcom/export", "/api/v1/gedcom/export/preview"}
	for i := range 5 {
		if code := export(paths[i%2], cookies[repository.RoleEditor]); code != http.StatusOK {
			t.Fatalf("export %d by the editor: %d", i+1, code)
		}
	}
	for _, path := range paths {
		rec := f.do(t, request{method: http.MethodGet, path: path, cookie: cookies[repository.RoleEditor]})
		if rec.Code != http.StatusTooManyRequests || errorCode(t, rec) != api.CodeRateLimited {
			t.Errorf("%s after the burst: %d %s, want 429 RATE_LIMITED", path, rec.Code, rec.Body)
		}
	}
	if code := export(paths[0], cookies[repository.RoleOwner]); code != http.StatusOK {
		t.Errorf("another user's export after the editor's burst: %d, want 200", code)
	}
	if code := export("/api/v1/export/tree", cookies[repository.RoleEditor]); code != http.StatusOK {
		t.Errorf("a JSON export after the GEDCOM burst: %d, want 200 (only GEDCOM is limited)", code)
	}
}

func TestRoles_OpenModeIsOwnerAndUnlimited(t *testing.T) {
	cfg := &config.Config{Port: 8080, LogFormat: "text", AuthMode: config.AuthModeOpen}
	events := memory.NewEventStore()
	server := api.NewServer(cfg, events, memory.NewReadModelStore(), memory.NewSnapshotStore(events), nil)
	f := &authFixture{server: server}
	for i := range 10 {
		if rec := f.do(t, request{method: http.MethodGet, path: "/api/v1/gedcom/export/preview"}); rec.Code != http.StatusOK {
			t.Fatalf("open-mode export %d: %d", i+1, rec.Code)
		}
	}
	if rec := f.do(t, request{method: http.MethodPost, path: "/api/v1/persons/" + uuid.New().String() + "/rollback", body: `{}`}); forbidden(rec) {
		t.Errorf("open-mode rollback: %d %s, want the owner let through", rec.Code, rec.Body)
	}
}
