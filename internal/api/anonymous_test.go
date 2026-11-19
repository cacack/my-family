package api_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/google/uuid"

	"github.com/cacack/my-family/internal/api"
	"github.com/cacack/my-family/internal/repository"
)

// openToVisitors stores anonymous access viewer directly: the settings API
// refuses it until visitors are protected (repository.AnonymousViewingProtected).
func (f *authFixture) openToVisitors(t *testing.T) {
	t.Helper()
	settings := repository.DefaultTreeSettings()
	settings.AnonymousAccess = repository.AnonymousAccessViewer
	if err := f.accounts.SaveTreeSettings(context.Background(), repository.DefaultTreeID, settings); err != nil {
		t.Fatalf("SaveTreeSettings: %v", err)
	}
}

// unauthenticated reports whether the sign-in check refused the request.
func unauthenticated(rec *httptest.ResponseRecorder) bool {
	return rec.Code == http.StatusUnauthorized && errorCodeOf(rec) == api.CodeUnauthenticated
}

// accountRoutes act on one's own account, which a visitor does not have.
var accountRoutes = routeSet("GET /api/v1/auth/me", "PATCH /api/v1/auth/me")

// TestAnonymous_EveryRoute calls every API route without a session, with
// anonymous access none and then viewer. With none, every route that is not
// public is refused with 401. With viewer, exactly the viewer routes (but the
// account ones) are served; the rest are still refused with 401.
func TestAnonymous_EveryRoute(t *testing.T) {
	for _, access := range []repository.AnonymousAccess{repository.AnonymousAccessNone, repository.AnonymousAccessViewer} {
		t.Run(string(access), func(t *testing.T) {
			f := newAuthFixture(t)
			f.createOwner(t)
			if access == repository.AnonymousAccessViewer {
				f.openToVisitors(t)
			}
			id := uuid.New().String()
			for _, route := range f.server.Echo().Routes() {
				if !strings.HasPrefix(route.Path, "/api/") {
					continue
				}
				key := route.Method + " " + route.Path
				role := expectedRole(key)
				if role == "public" {
					continue
				}
				body := ""
				if route.Method != http.MethodGet {
					body = "{}"
				}
				rec := f.do(t, request{method: route.Method, path: routeParam.ReplaceAllString(route.Path, id), body: body})
				wantServed := access == repository.AnonymousAccessViewer && role == repository.RoleViewer && !accountRoutes[key]
				if got := !unauthenticated(rec) && !forbidden(rec); got != wantServed {
					t.Errorf("%s anonymously (needs %s): %d %s; served = %v, want %v", key, role, rec.Code, rec.Body, got, wantServed)
				}
				if !wantServed && !unauthenticated(rec) {
					t.Errorf("%s anonymously: %d %s, want 401 UNAUTHENTICATED", key, rec.Code, rec.Body)
				}
			}
		})
	}
}

// A visitor reads the tree, and a signed-in session still works as itself.
func TestAnonymous_VisitorReadsTheTree(t *testing.T) {
	f := newAuthFixture(t)
	f.createOwner(t)
	owner := f.signIn(t)
	rec := f.do(t, request{method: http.MethodPost, path: "/api/v1/persons", cookie: owner,
		body: `{"given_name":"Ada","surname":"Byron","birth_date":"1815"}`})
	if rec.Code != http.StatusCreated {
		t.Fatalf("create person: %d %s", rec.Code, rec.Body)
	}
	f.openToVisitors(t)

	rec = f.do(t, request{method: http.MethodGet, path: "/api/v1/persons"})
	if rec.Code != http.StatusOK || decode[api.PersonList](t, rec).Total != 1 {
		t.Errorf("visitor lists persons: %d %s", rec.Code, rec.Body)
	}
	if rec := f.do(t, request{method: http.MethodPost, path: "/api/v1/persons", body: `{"given_name":"Eve","surname":"X"}`}); rec.Code != http.StatusUnauthorized {
		t.Errorf("visitor creates a person: %d %s, want 401", rec.Code, rec.Body)
	}
	if rec := f.do(t, request{method: http.MethodPost, path: "/api/v1/persons", cookie: owner, body: `{"given_name":"Eve","surname":"X"}`}); rec.Code != http.StatusCreated {
		t.Errorf("owner creates a person while visitors are let in: %d %s", rec.Code, rec.Body)
	}
}

func TestAnonymous_AuthStatusSaysWhetherVisitorsMayLook(t *testing.T) {
	f := newAuthFixture(t)
	f.createOwner(t)
	status := func() api.AuthStatus {
		t.Helper()
		return decode[api.AuthStatus](t, f.do(t, request{method: http.MethodGet, path: "/api/v1/auth/status"}))
	}
	if got := status(); got.AnonymousAccess == nil || *got.AnonymousAccess != api.AnonymousAccessNone || got.Account != nil {
		t.Errorf("status = %+v, want anonymous access none and no account", got)
	}
	f.openToVisitors(t)
	if got := status(); got.AnonymousAccess == nil || *got.AnonymousAccess != api.AnonymousAccessViewer {
		t.Errorf("status after opening = %+v, want anonymous access viewer", got)
	}
}

// Closing the tree takes effect on the next request and is audited; turning
// visitors in is refused while they would see everything.
func TestAnonymous_OwnerChangesAccess(t *testing.T) {
	f := newAuthFixture(t)
	f.createOwner(t)
	owner := f.signIn(t)
	put := func(access string) int {
		t.Helper()
		return f.do(t, request{method: http.MethodPut, path: "/api/v1/settings/privacy", cookie: owner,
			body: `{"living_threshold_years":110,"viewers_see_living":true,"anonymous_access":"` + access + `"}`}).Code
	}
	audited := func() []api.AuthAuditEntry {
		t.Helper()
		var out []api.AuthAuditEntry
		for _, e := range decode[api.AuthAuditList](t, f.do(t, request{method: http.MethodGet, path: "/api/v1/auth-audit", cookie: owner})).Items {
			if e.Action == api.AuthAuditEntryActionAnonymousAccessChanged {
				out = append(out, e)
			}
		}
		return out
	}

	if code := put("viewer"); repository.AnonymousViewingProtected != (code == http.StatusOK) {
		t.Errorf("PUT viewer: %d, want 200 only once visitors are protected", code)
	}
	if code := put("none"); code != http.StatusOK {
		t.Fatalf("PUT none: %d", code)
	}
	if got := audited(); len(got) != 0 {
		t.Errorf("audit after changing nothing: %+v", got)
	}

	f.openToVisitors(t)
	if rec := f.do(t, request{method: http.MethodGet, path: "/api/v1/persons"}); rec.Code != http.StatusOK {
		t.Fatalf("visitor before closing: %d %s", rec.Code, rec.Body)
	}
	if code := put("none"); code != http.StatusOK {
		t.Fatalf("PUT none: %d", code)
	}
	if rec := f.do(t, request{method: http.MethodGet, path: "/api/v1/persons"}); rec.Code != http.StatusUnauthorized {
		t.Errorf("visitor after closing: %d %s, want 401", rec.Code, rec.Body)
	}
	got := audited()
	if len(got) != 1 || got[0].Detail != "none" || got[0].ActorName == nil || *got[0].ActorName != "Ada Lovelace" {
		t.Errorf("audit = %+v, want one change to none by Ada", got)
	}
}

// Costly reads are limited per client address for visitors only; cheap
// reads and signed-in users are not.
func TestAnonymous_CostlyReadsAreRateLimitedPerClient(t *testing.T) {
	f := newAuthFixture(t)
	f.createOwner(t)
	owner := f.signIn(t)
	f.openToVisitors(t)
	const first, second = "198.51.100.1:1000", "198.51.100.2:1000"

	limited := 0
	for range 40 {
		if rec := f.do(t, request{method: http.MethodGet, path: "/api/v1/search?q=ada", remoteAddr: first}); rec.Code == http.StatusTooManyRequests {
			limited++
			if code := errorCodeOf(rec); code != api.CodeRateLimited {
				t.Fatalf("limited search: code %q", code)
			}
		}
	}
	if limited == 0 {
		t.Fatal("40 anonymous searches at once: none limited")
	}
	if rec := f.do(t, request{method: http.MethodGet, path: "/api/v1/search?q=ada", remoteAddr: second}); rec.Code != http.StatusOK {
		t.Errorf("another client's search: %d %s", rec.Code, rec.Body)
	}
	if rec := f.do(t, request{method: http.MethodGet, path: "/api/v1/persons", remoteAddr: first}); rec.Code != http.StatusOK {
		t.Errorf("limited client's cheap read: %d %s", rec.Code, rec.Body)
	}
	if rec := f.do(t, request{method: http.MethodGet, path: "/api/v1/search?q=ada", remoteAddr: first, cookie: owner}); rec.Code != http.StatusOK {
		t.Errorf("signed-in search from the limited address: %d %s", rec.Code, rec.Body)
	}
}
