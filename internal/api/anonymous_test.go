package api_test

import (
	"bytes"
	"context"
	"errors"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"slices"
	"strconv"
	"strings"
	"testing"

	"github.com/google/uuid"

	"github.com/cacack/my-family/internal/api"
	"github.com/cacack/my-family/internal/auth"
	"github.com/cacack/my-family/internal/config"
	"github.com/cacack/my-family/internal/repository"
	"github.com/cacack/my-family/internal/repository/memory"
)

// openToVisitors lets visitors in by storing anonymous access viewer
// directly, without an owner session or an audit entry.
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
var accountRoutes = routeSet("GET /api/v1/auth/me", "PATCH /api/v1/auth/me",
	"PUT /api/v1/settings/home-person", "DELETE /api/v1/settings/home-person",
	"GET /api/v1/auth/me/tokens", "POST /api/v1/auth/me/tokens", "DELETE /api/v1/auth/me/tokens/:tokenId")

// changeLogRoutes read change records, which name who made each change, so
// a visitor may not call them (ADR-008 §4). With accountRoutes, it restates
// the server's signedInRoutes on purpose, as an oracle independent of the code
// under test: change both together, and do not derive one from the other.
var changeLogRoutes = routeSet(
	"GET /api/v1/history",
	"GET /api/v1/persons/:id/history",
	"GET /api/v1/families/:id/history",
	"GET /api/v1/sources/:id/history",
	"GET /api/v1/persons/:id/restore-points",
	"GET /api/v1/families/:id/restore-points",
	"GET /api/v1/sources/:id/restore-points",
	"GET /api/v1/citations/:id/restore-points",
	"GET /api/v1/snapshots",
	"GET /api/v1/snapshots/:id",
	"GET /api/v1/snapshots/:id/compare-current",
	"GET /api/v1/snapshots/:id1/compare/:id2",
	"GET /api/v1/branches/:id/compare",
)

// branchViewRoutes read a research branch's view, which a visitor may not:
// a visitor reads the mainline only (ADR-008 §4). With accountRoutes and
// changeLogRoutes it restates the server's signedInRoutes.
var branchViewRoutes = routeSet(
	"GET /api/v1/branches/:id/evidence-coverage",
	"GET /api/v1/branches/:id/health",
	"GET /api/v1/branches/:id/research",
)

// TestAnonymous_EveryRoute calls every API route without a session, with
// anonymous access none and then viewer. With none, every route that is not
// public is refused with 401. With viewer, exactly the viewer routes (but the
// account and change-log ones) are served; the rest are still refused with
// 401.
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
				wantServed := access == repository.AnonymousAccessViewer && role == repository.RoleViewer && !accountRoutes[key] && !changeLogRoutes[key] && !branchViewRoutes[key]
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

// The owner lets visitors in and closes the tree again through the settings
// API: each change takes effect on the next request and writes one audit
// entry, and saving the same value again writes none.
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
	byAda := func(e api.AuthAuditEntry, detail string) bool {
		return e.Detail == detail && e.ActorName != nil && *e.ActorName == "Ada Lovelace"
	}

	if code := put("none"); code != http.StatusOK {
		t.Fatalf("PUT none: %d", code)
	}
	if got := audited(); len(got) != 0 {
		t.Errorf("audit after changing nothing: %+v", got)
	}

	if code := put("viewer"); code != http.StatusOK {
		t.Fatalf("PUT viewer: %d, want 200", code)
	}
	if got := audited(); len(got) != 1 || !byAda(got[0], "viewer") {
		t.Errorf("audit after opening = %+v, want one change to viewer by Ada", got)
	}
	if rec := f.do(t, request{method: http.MethodGet, path: "/api/v1/persons"}); rec.Code != http.StatusOK {
		t.Fatalf("visitor after opening: %d %s, want 200", rec.Code, rec.Body)
	}

	if code := put("none"); code != http.StatusOK {
		t.Fatalf("PUT none: %d", code)
	}
	if rec := f.do(t, request{method: http.MethodGet, path: "/api/v1/persons"}); rec.Code != http.StatusUnauthorized {
		t.Errorf("visitor after closing: %d %s, want 401", rec.Code, rec.Body)
	}
	closed := slices.ContainsFunc(audited(), func(e api.AuthAuditEntry) bool { return byAda(e, "none") })
	if got := audited(); len(got) != 2 || !closed {
		t.Errorf("audit after closing = %+v, want a change to viewer and one to none by Ada", got)
	}
}

// failingAuditStore is an account store whose audit log cannot be written.
type failingAuditStore struct{ *memory.AccountStore }

func (failingAuditStore) AppendAuthAudit(context.Context, *repository.AuthAuditEntry) error {
	return errors.New("auth_audit is locked")
}

// A change to anonymous access whose audit entry cannot be written still takes
// effect and answers 200, as every account action does: the failure is logged
// at error level, naming the action, and the change is not undone. Closing the
// tree to visitors must never wait on the audit log.
func TestAnonymous_AccessChangeOutlivesAFailedAuditWrite(t *testing.T) {
	var logged bytes.Buffer
	prev := slog.Default()
	slog.SetDefault(slog.New(slog.NewTextHandler(&logged, nil)))
	t.Cleanup(func() { slog.SetDefault(prev) })

	accounts := memory.NewAccountStore()
	svc := auth.NewService(failingAuditStore{accounts})
	events := memory.NewEventStore()
	server := api.NewServer(&config.Config{Port: 8080, LogFormat: "text", AuthMode: config.AuthModeAccounts},
		events, memory.NewReadModelStore(), memory.NewSnapshotStore(events), nil,
		api.WithAuth(svc), api.WithBranchStore(memory.NewBranchStore()), api.WithTreeSettings(accounts))
	f := &authFixture{server: server, auth: svc, accounts: accounts, events: events}
	f.createOwner(t)
	owner := f.signIn(t)
	f.openToVisitors(t)

	rec := f.do(t, request{method: http.MethodPut, path: "/api/v1/settings/privacy", cookie: owner,
		body: `{"living_threshold_years":110,"viewers_see_living":true,"anonymous_access":"none"}`})
	if rec.Code != http.StatusOK {
		t.Fatalf("PUT none with the audit log down: %d %s, want 200", rec.Code, rec.Body)
	}
	if got, err := accounts.GetTreeSettings(context.Background(), repository.DefaultTreeID); err != nil || got.AnonymousAccess != repository.AnonymousAccessNone {
		t.Errorf("stored settings = %+v, %v; want anonymous access none", got, err)
	}
	if rec := f.do(t, request{method: http.MethodGet, path: "/api/v1/persons"}); !unauthenticated(rec) {
		t.Errorf("visitor after closing: %d %s, want 401", rec.Code, rec.Body)
	}
	if out := logged.String(); !strings.Contains(out, "level=ERROR") || !strings.Contains(out, string(repository.AuthActionAnonymousAccessChanged)) {
		t.Errorf("log = %q, want an error naming %s", out, repository.AuthActionAnonymousAccessChanged)
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
	if rec := f.do(t, request{method: http.MethodGet, path: "/api/v1/config", remoteAddr: first}); rec.Code != http.StatusOK {
		t.Errorf("limited client's cheap read: %d %s", rec.Code, rec.Body)
	}
	if rec := f.do(t, request{method: http.MethodGet, path: "/api/v1/search?q=ada", remoteAddr: first, cookie: owner}); rec.Code != http.StatusOK {
		t.Errorf("signed-in search from the limited address: %d %s", rec.Code, rec.Body)
	}
}

// The visitor limit keys on the connection's address, which the server's
// IPExtractor (echo.ExtractIPDirect) takes from the TCP peer: a visitor cannot
// leave its bucket by sending a fresh X-Forwarded-For or X-Real-IP header each
// time, nor reach another client's bucket by naming that client's address.
func TestAnonymous_RateLimitIgnoresForwardingHeaders(t *testing.T) {
	f := newAuthFixture(t)
	f.createOwner(t)
	f.openToVisitors(t)
	const visitor, other = "198.51.100.1:1000", "198.51.100.2:1000"

	limited := 0
	for i := range 40 {
		forwarded := "203.0.113." + strconv.Itoa(i)
		rec := f.do(t, request{method: http.MethodGet, path: "/api/v1/search?q=ada", remoteAddr: visitor,
			header: map[string]string{"X-Forwarded-For": forwarded, "X-Real-IP": forwarded}})
		if rec.Code == http.StatusTooManyRequests {
			limited++
		}
	}
	if limited == 0 {
		t.Fatal("40 searches from one connection address, each forwarded for another: none limited")
	}
	rec := f.do(t, request{method: http.MethodGet, path: "/api/v1/search?q=ada", remoteAddr: other,
		header: map[string]string{"X-Forwarded-For": "198.51.100.1", "X-Real-IP": "198.51.100.1"}})
	if rec.Code != http.StatusOK {
		t.Errorf("another connection forwarded for the limited address: %d %s, want 200", rec.Code, rec.Body)
	}
}

// With the kill switch off (repository.AnonymousViewingProtected false), a
// viewer setting already in the store (an older build, a restored backup, a
// hand edit) lets no visitor in, and the auth status says so.
func TestAnonymous_UnprotectedServerRefusesVisitors(t *testing.T) {
	f := newAuthFixture(t)
	f.createOwner(t)
	f.server.UnprotectVisitorsForTest()
	f.openToVisitors(t)

	if rec := f.do(t, request{method: http.MethodGet, path: "/api/v1/persons"}); !unauthenticated(rec) {
		t.Errorf("visitor lists persons: %d %s, want 401 UNAUTHENTICATED", rec.Code, rec.Body)
	}
	status := decode[api.AuthStatus](t, f.do(t, request{method: http.MethodGet, path: "/api/v1/auth/status"}))
	if status.AnonymousAccess == nil || *status.AnonymousAccess != api.AnonymousAccessNone {
		t.Errorf("status = %+v, want anonymous access none", status)
	}
}

// The change log names who made each change, so a visitor is refused every
// history, restore-point, snapshot and compare read with 401, while a
// signed-in viewer reaches the handler.
func TestAnonymous_ChangeLogNeedsSignIn(t *testing.T) {
	f := newAuthFixture(t)
	f.createOwner(t)
	owner := f.signIn(t)
	f.openToVisitors(t)
	id := uuid.New().String()
	for key := range changeLogRoutes {
		path := routeParam.ReplaceAllString(strings.TrimPrefix(key, "GET "), id)
		if rec := f.do(t, request{method: http.MethodGet, path: path}); !unauthenticated(rec) {
			t.Errorf("%s as a visitor: %d %s, want 401 UNAUTHENTICATED", key, rec.Code, rec.Body)
		}
		if rec := f.do(t, request{method: http.MethodGet, path: path, cookie: owner}); unauthenticated(rec) || forbidden(rec) {
			t.Errorf("%s signed in: %d %s, want the handler's answer", key, rec.Code, rec.Body)
		}
	}
}

// A visitor reads the mainline only (ADR-008 §4): a research branch's view,
// by a `?branch=` scope on any read or by the branch's own view routes, is
// refused with 401, while a signed-in viewer reaches the handler.
func TestAnonymous_BranchViewNeedsSignIn(t *testing.T) {
	f := newAuthFixture(t)
	f.createOwner(t)
	owner := f.signIn(t)
	f.openToVisitors(t)
	id := uuid.New().String()
	paths := []string{
		"/api/v1/persons?branch=" + id,
		"/api/v1/persons?branch=",
		"/api/v1/persons/" + id + "?branch=" + id,
		"/api/v1/map/locations?branch=" + id,
	}
	for key := range branchViewRoutes {
		paths = append(paths, routeParam.ReplaceAllString(strings.TrimPrefix(key, "GET "), id))
	}
	for _, path := range paths {
		if rec := f.do(t, request{method: http.MethodGet, path: path}); !unauthenticated(rec) {
			t.Errorf("GET %s as a visitor: %d %s, want 401 UNAUTHENTICATED", path, rec.Code, rec.Body)
		}
		if rec := f.do(t, request{method: http.MethodGet, path: path, cookie: owner}); unauthenticated(rec) || forbidden(rec) {
			t.Errorf("GET %s signed in: %d %s, want the handler's answer", path, rec.Code, rec.Body)
		}
	}
	if rec := f.do(t, request{method: http.MethodGet, path: "/api/v1/persons"}); rec.Code != http.StatusOK {
		t.Errorf("GET /api/v1/persons as a visitor: %d %s, want 200", rec.Code, rec.Body)
	}
}
