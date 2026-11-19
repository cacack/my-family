package api_test

import (
	"context"
	"errors"
	"net/http"
	"testing"

	"github.com/google/uuid"

	"github.com/cacack/my-family/internal/api"
	"github.com/cacack/my-family/internal/auth"
	"github.com/cacack/my-family/internal/config"
	"github.com/cacack/my-family/internal/repository"
	"github.com/cacack/my-family/internal/repository/memory"
)

func TestPrivacySettings_OwnerReadsAndChangesThem(t *testing.T) {
	f := newAuthFixture(t)
	f.createOwner(t)
	owner := f.signIn(t)

	get := func() api.PrivacySettings {
		t.Helper()
		rec := f.do(t, request{method: http.MethodGet, path: "/api/v1/settings/privacy", cookie: owner})
		if rec.Code != http.StatusOK {
			t.Fatalf("GET settings: %d %s", rec.Code, rec.Body)
		}
		return decode[api.PrivacySettings](t, rec)
	}
	if got := get(); got != (api.PrivacySettings{LivingThresholdYears: 110, ViewersSeeLiving: true, AnonymousAccess: api.AnonymousAccessNone}) {
		t.Errorf("default settings = %+v", got)
	}

	rec := f.do(t, request{method: http.MethodPut, path: "/api/v1/settings/privacy", cookie: owner,
		body: `{"living_threshold_years":90,"viewers_see_living":false,"anonymous_access":"none"}`})
	if rec.Code != http.StatusOK {
		t.Fatalf("PUT settings: %d %s", rec.Code, rec.Body)
	}
	want := api.PrivacySettings{LivingThresholdYears: 90, ViewersSeeLiving: false, AnonymousAccess: api.AnonymousAccessNone}
	if got := decode[api.PrivacySettings](t, rec); got != want {
		t.Errorf("PUT response = %+v, want %+v", got, want)
	}
	if got := get(); got != want {
		t.Errorf("settings after PUT = %+v, want %+v", got, want)
	}

	for _, body := range []string{
		`{"living_threshold_years":49,"viewers_see_living":true,"anonymous_access":"none"}`,
		`{"living_threshold_years":151,"viewers_see_living":true,"anonymous_access":"none"}`,
		`{"viewers_see_living":true,"anonymous_access":"none"}`,
		`{"living_threshold_years":90,"viewers_see_living":true}`,
		`{"living_threshold_years":90,"viewers_see_living":true,"anonymous_access":"editor"}`,
	} {
		rec := f.do(t, request{method: http.MethodPut, path: "/api/v1/settings/privacy", cookie: owner, body: body})
		if rec.Code != http.StatusBadRequest {
			t.Errorf("PUT %s: %d %s, want 400", body, rec.Code, rec.Body)
		}
	}
	if got := get(); got != want {
		t.Errorf("settings after refused PUTs = %+v, want %+v", got, want)
	}
}

func TestPrivacySettings_OwnersOnly(t *testing.T) {
	f := newAuthFixture(t)
	f.createOwner(t)
	owner := f.signIn(t)
	for _, role := range []string{"viewer", "editor"} {
		cookie := f.inviteAs(t, owner, role, role)
		for _, r := range []request{
			{method: http.MethodGet, path: "/api/v1/settings/privacy", cookie: cookie},
			{method: http.MethodPut, path: "/api/v1/settings/privacy", cookie: cookie, body: `{"living_threshold_years":90,"viewers_see_living":true,"anonymous_access":"none"}`},
		} {
			if rec := f.do(t, r); rec.Code != http.StatusForbidden {
				t.Errorf("%s %s as %s: %d, want 403", r.method, r.path, role, rec.Code)
			}
		}
	}
}

// The living verdict a person carries follows the owner's threshold.
func TestPrivacySettings_ThresholdDecidesWhoIsLiving(t *testing.T) {
	f := newAuthFixture(t)
	f.createOwner(t)
	owner := f.signIn(t)
	rec := f.do(t, request{method: http.MethodPost, path: "/api/v1/persons", cookie: owner,
		body: `{"given_name":"Edith","surname":"Holt","birth_date":"1930"}`})
	if rec.Code != http.StatusCreated {
		t.Fatalf("create person: %d %s", rec.Code, rec.Body)
	}
	id := decode[api.Person](t, rec).Id.String()

	living := func() bool {
		t.Helper()
		rec := f.do(t, request{method: http.MethodGet, path: "/api/v1/persons/" + id, cookie: owner})
		if rec.Code != http.StatusOK {
			t.Fatalf("GET person: %d %s", rec.Code, rec.Body)
		}
		p := decode[api.PersonDetail](t, rec)
		return p.Living != nil && *p.Living
	}
	if !living() {
		t.Error("born 1930 at the default 110-year threshold: not living")
	}
	rec = f.do(t, request{method: http.MethodPut, path: "/api/v1/settings/privacy", cookie: owner,
		body: `{"living_threshold_years":90,"viewers_see_living":true,"anonymous_access":"none"}`})
	if rec.Code != http.StatusOK {
		t.Fatalf("PUT settings: %d %s", rec.Code, rec.Body)
	}
	if living() {
		t.Error("born 1930 at a 90-year threshold: still living")
	}
}

func TestPrivacySettings_WithoutAStoreTheDefaultsApply(t *testing.T) {
	events := memory.NewEventStore()
	server := api.NewServer(&config.Config{AuthMode: config.AuthModeOpen}, events, memory.NewReadModelStore(),
		memory.NewSnapshotStore(events), nil)
	f := &authFixture{server: server}
	rec := f.do(t, request{method: http.MethodGet, path: "/api/v1/settings/privacy"})
	if rec.Code != http.StatusOK || decode[api.PrivacySettings](t, rec).LivingThresholdYears != 110 {
		t.Errorf("GET settings without a store: %d %s", rec.Code, rec.Body)
	}
	rec = f.do(t, request{method: http.MethodPut, path: "/api/v1/settings/privacy",
		body: `{"living_threshold_years":90,"viewers_see_living":true,"anonymous_access":"none"}`})
	if rec.Code != http.StatusServiceUnavailable {
		t.Errorf("PUT settings without a store: %d, want 503", rec.Code)
	}
}

// brokenSettings is a settings store whose reads fail.
type brokenSettings struct{ repository.TreeSettingsStore }

func (brokenSettings) GetTreeSettings(context.Context, uuid.UUID) (repository.TreeSettings, error) {
	return repository.TreeSettings{}, errors.New("settings table is locked")
}

// A failing settings store fails only the routes that read the tree, and
// requests without a session, whose answer depends on the anonymous-access
// setting: health and sign-in keep working.
func TestPrivacySettings_StoreFailureSparesPublicRoutes(t *testing.T) {
	cfg := &config.Config{Port: 8080, LogFormat: "text", AuthMode: config.AuthModeAccounts}
	events := memory.NewEventStore()
	accounts := memory.NewAccountStore()
	svc := auth.NewService(accounts)
	server := api.NewServer(cfg, events, memory.NewReadModelStore(), memory.NewSnapshotStore(events), nil,
		api.WithAuth(svc), api.WithBranchStore(memory.NewBranchStore()), api.WithTreeSettings(brokenSettings{accounts}))
	f := &authFixture{server: server, auth: svc, accounts: accounts, events: events}
	f.createOwner(t)

	if rec := f.do(t, request{method: http.MethodGet, path: "/api/v1/health"}); rec.Code != http.StatusOK {
		t.Errorf("health: %d %s", rec.Code, rec.Body)
	}
	if rec := f.do(t, request{method: http.MethodGet, path: "/api/v1/persons"}); rec.Code != http.StatusInternalServerError {
		t.Errorf("persons signed out: %d, want 500", rec.Code)
	}
	owner := f.signIn(t)
	if rec := f.do(t, request{method: http.MethodGet, path: "/api/v1/persons", cookie: owner}); rec.Code != http.StatusInternalServerError {
		t.Errorf("persons signed in: %d, want 500", rec.Code)
	}
}
