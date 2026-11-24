package api_test

import (
	"fmt"
	"net/http"
	"testing"

	"github.com/cacack/my-family/internal/api"
	"github.com/cacack/my-family/internal/config"
	"github.com/cacack/my-family/internal/repository"
	"github.com/cacack/my-family/internal/repository/memory"
)

// The owner previews the tree as a viewer or a visitor (#1413): with the
// preview cookie, the API answers the owner as it answers that principal.

func previewCookie(as string) *http.Cookie {
	return &http.Cookie{Name: "myfamily_preview", Value: as}
}

// previewPaths are reads across the read-surface table: lists, searches,
// records, sub-resources, charts, browse indexes, the map, aggregates, media
// bytes and the family pages.
func previewPaths(tr redactionTree) []string {
	return []string{
		"/api/v1/persons",
		"/api/v1/search?q=Smith",
		"/api/v1/persons/" + tr.dead,
		"/api/v1/persons/" + tr.living,
		"/api/v1/persons/" + tr.private,
		"/api/v1/persons/" + tr.living + "/names",
		"/api/v1/persons/" + tr.dead + "/media",
		"/api/v1/media/" + tr.livingMedia + "/content",
		"/api/v1/media/" + tr.deadPrivateMedia,
		"/api/v1/families",
		"/api/v1/families/" + tr.family,
		"/api/v1/families/" + tr.family + "/group-sheet",
		"/api/v1/pedigree/" + tr.kid,
		"/api/v1/descendancy/" + tr.dead,
		"/api/v1/relationship/" + tr.dead + "/" + tr.kid,
		"/api/v1/browse/surnames",
		"/api/v1/browse/places/" + springfieldPlaceID + "/persons",
		"/api/v1/map/locations",
		"/api/v1/statistics",
		"/api/v1/settings/home-person",
	}
}

func TestPrivacyPreview_ShowsExactlyWhatThePrincipalGets(t *testing.T) {
	for _, viewersSeeLiving := range []bool{true, false} {
		t.Run(fmt.Sprintf("viewers see living %v", viewersSeeLiving), func(t *testing.T) {
			f, tr, principals := redactionFixture(t, viewersSeeLiving)
			owner := principals[0].cookie
			// The owner's home person is the living Lena, so a viewer
			// preview that kept the owner's account would show her.
			if rec := f.do(t, request{method: http.MethodPut, path: "/api/v1/settings/home-person", cookie: owner,
				body: fmt.Sprintf(`{"person_id":%q}`, tr.living)}); rec.Code != http.StatusOK {
				t.Fatalf("set home person: %d %s", rec.Code, rec.Body)
			}
			for _, tt := range []struct {
				as        string
				principal privacyPrincipal
			}{
				{"viewer", principals[2]},
				{"visitor", principals[3]},
			} {
				t.Run(tt.as, func(t *testing.T) {
					for _, path := range previewPaths(tr) {
						if path == "/api/v1/settings/home-person" && tt.as == "viewer" {
							// Each account has its own home person: the
							// viewer's is not the owner's.
							continue
						}
						want := tt.principal.get(t, f, path)
						got := f.do(t, request{method: http.MethodGet, path: path, cookie: owner, more: []*http.Cookie{previewCookie(tt.as)}})
						if got.Code != want.Code || got.Body.String() != want.Body.String() {
							t.Errorf("GET %s previewed as %s: %d %s\nwant %d %s", path, tt.as, got.Code, got.Body, want.Code, want.Body)
						}
					}
				})
			}
		})
	}
}

// What the previewed principal may not call is refused with 403, so the
// owner stays signed in; reads they may call are served.
func TestPrivacyPreview_RefusesWhatThePrincipalMayNot(t *testing.T) {
	f, tr, principals := redactionFixture(t, true)
	owner := principals[0].cookie
	for _, tt := range []struct {
		as, method, path, body string
		want                   int
	}{
		{"viewer", http.MethodPost, "/api/v1/persons", `{"given_name":"Ann"}`, http.StatusForbidden},
		{"viewer", http.MethodGet, "/api/v1/settings/privacy", "", http.StatusForbidden},
		{"viewer", http.MethodGet, "/api/v1/history", "", http.StatusOK},
		{"viewer", http.MethodGet, "/api/v1/auth/me", "", http.StatusOK},
		{"visitor", http.MethodPost, "/api/v1/persons", `{"given_name":"Ann"}`, http.StatusForbidden},
		{"visitor", http.MethodGet, "/api/v1/history", "", http.StatusForbidden},
		{"visitor", http.MethodGet, "/api/v1/persons/" + tr.dead + "/history", "", http.StatusForbidden},
		{"visitor", http.MethodGet, "/api/v1/auth/me", "", http.StatusForbidden},
		{"visitor", http.MethodGet, "/api/v1/persons", "", http.StatusOK},
		{"visitor", http.MethodGet, "/api/v1/persons?branch=" + tr.dead, "", http.StatusForbidden},
		{"visitor", http.MethodGet, "/api/v1/branches/" + tr.dead + "/health", "", http.StatusForbidden},
		{"stranger", http.MethodGet, "/api/v1/persons", "", http.StatusBadRequest},
	} {
		t.Run(tt.as+" "+tt.method+" "+tt.path, func(t *testing.T) {
			rec := f.do(t, request{method: tt.method, path: tt.path, body: tt.body, cookie: owner, more: []*http.Cookie{previewCookie(tt.as)}})
			if rec.Code != tt.want {
				t.Errorf("%d %s, want %d", rec.Code, rec.Body, tt.want)
			}
		})
	}
	// Public routes are not previewed: the auth status still names the owner.
	status := decode[api.AuthStatus](t, f.do(t, request{method: http.MethodGet, path: "/api/v1/auth/status", cookie: owner,
		more: []*http.Cookie{previewCookie("visitor")}}))
	if status.Account == nil || status.Account.Role != api.AccountRoleOwner {
		t.Errorf("auth status while previewing = %+v, want the owner's account", status.Account)
	}
}

// Only the owner previews: the cookie never changes what anyone else sees.
func TestPrivacyPreview_OnlyTheOwner(t *testing.T) {
	f, tr, principals := redactionFixture(t, false)
	for _, p := range principals[1:3] {
		for _, as := range []string{"visitor", "viewer", "stranger"} {
			rec := f.do(t, request{method: http.MethodGet, path: "/api/v1/persons/" + tr.living, cookie: p.cookie, more: []*http.Cookie{previewCookie(as)}})
			if want := p.get(t, f, "/api/v1/persons/"+tr.living); rec.Code != want.Code || rec.Body.String() != want.Body.String() {
				t.Errorf("%s with preview %s: %d %s, want %d %s", p.name, as, rec.Code, rec.Body, want.Code, want.Body)
			}
		}
	}
	// A visitor's preview cookie changes nothing either.
	rec := f.do(t, request{method: http.MethodGet, path: "/api/v1/history", more: []*http.Cookie{previewCookie("viewer")}})
	if !unauthenticated(rec) {
		t.Errorf("visitor with preview viewer GET /history: %d %s, want 401", rec.Code, rec.Body)
	}
}

// In open mode everyone is the owner, so anyone may preview.
func TestPrivacyPreview_OpenMode(t *testing.T) {
	events := memory.NewEventStore()
	server := api.NewServer(&config.Config{AuthMode: config.AuthModeOpen}, events, memory.NewReadModelStore(),
		memory.NewSnapshotStore(events), nil, api.WithBranchStore(memory.NewBranchStore()))
	f := &authFixture{server: server, events: events}
	rec := f.do(t, request{method: http.MethodPost, path: "/api/v1/persons", body: `{"given_name":"Lena","surname":"Smith","birth_date":"1990"}`})
	if rec.Code != http.StatusCreated {
		t.Fatalf("create: %d %s", rec.Code, rec.Body)
	}
	id := decode[map[string]any](t, rec)["id"].(string)

	if body := decode[map[string]any](t, f.do(t, request{method: http.MethodGet, path: "/api/v1/persons/" + id})); body["given_name"] != "Lena" {
		t.Errorf("owner sees %v, want Lena", body)
	}
	visitor := []*http.Cookie{previewCookie("visitor")}
	if body := decode[map[string]any](t, f.do(t, request{method: http.MethodGet, path: "/api/v1/persons/" + id, more: visitor})); body["given_name"] != repository.PlaceholderGivenName {
		t.Errorf("previewed as a visitor: %v, want the placeholder", body)
	}
	if rec := f.do(t, request{method: http.MethodGet, path: "/api/v1/history", more: visitor}); rec.Code != http.StatusForbidden {
		t.Errorf("previewed as a visitor GET /history: %d, want 403", rec.Code)
	}
	if rec := f.do(t, request{method: http.MethodGet, path: "/api/v1/health", more: []*http.Cookie{previewCookie("stranger")}}); rec.Code != http.StatusOK {
		t.Errorf("health with a bad preview: %d, want 200 (public routes are not previewed)", rec.Code)
	}
}

// Media bytes are never kept by the browser (#1578): the same URL serves the
// owner and, during a preview, the principal they preview as, so a kept copy
// would cross from one to the other when the preview starts or stops.
func TestPrivacyPreview_MediaBytesAreNotCached(t *testing.T) {
	f, tr, principals := redactionFixture(t, false)
	owner := principals[0].cookie
	for _, path := range []string{
		"/api/v1/media/" + tr.deadMedia + "/content",
		"/api/v1/media/" + tr.deadMedia + "/thumbnail",
		"/api/v1/media/" + tr.livingMedia + "/content",
		"/api/v1/media/" + tr.livingMedia + "/thumbnail",
		"/api/v1/media/" + tr.deadPrivateMedia + "/content",
		"/api/v1/media/" + tr.deadPrivateMedia + "/thumbnail",
	} {
		for _, as := range []string{"", "viewer", "visitor"} {
			var more []*http.Cookie
			if as != "" {
				more = []*http.Cookie{previewCookie(as)}
			}
			rec := f.do(t, request{method: http.MethodGet, path: path, cookie: owner, more: more})
			// The owner is served the bytes, so the header is checked on
			// what is kept, not only on refusals.
			if as == "" && rec.Code != http.StatusOK {
				t.Errorf("GET %s as the owner: %d %s, want 200", path, rec.Code, rec.Body)
			}
			if got := rec.Header().Get("Cache-Control"); got != "no-store" {
				t.Errorf("GET %s previewed as %q: %d with Cache-Control %q, want no-store", path, as, rec.Code, got)
			}
		}
	}
	// Other reads keep the default.
	if got := f.do(t, request{method: http.MethodGet, path: "/api/v1/media/" + tr.deadMedia, cookie: owner}).Header().Get("Cache-Control"); got != "" {
		t.Errorf("GET media metadata: Cache-Control %q, want none", got)
	}
}

// Choosing a living home person during a viewer preview is answered as the
// viewer's own choice is (#1578): while viewers do not see the living, both
// get her placeholder back, never her details.
func TestPrivacyPreview_HomePersonAsAViewer(t *testing.T) {
	f, tr, principals := redactionFixture(t, false)
	owner, viewer := principals[0].cookie, principals[2].cookie
	body := fmt.Sprintf(`{"person_id":%q}`, tr.living)
	want := f.do(t, request{method: http.MethodPut, path: "/api/v1/settings/home-person", cookie: viewer, body: body})
	got := f.do(t, request{method: http.MethodPut, path: "/api/v1/settings/home-person", cookie: owner, body: body,
		more: []*http.Cookie{previewCookie("viewer")}})
	if got.Code != want.Code || got.Body.String() != want.Body.String() {
		t.Errorf("PUT home person previewed as viewer: %d %s\nwant %d %s", got.Code, got.Body, want.Code, want.Body)
	}
	if want.Code != http.StatusOK {
		t.Fatalf("viewer PUT home person: %d %s", want.Code, want.Body)
	}
	if person := decode[api.HomePerson](t, got).Person; person == nil || person.GivenName != repository.PlaceholderGivenName {
		t.Errorf("home person previewed as viewer = %+v, want the placeholder", person)
	}
}
