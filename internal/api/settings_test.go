package api_test

import (
	"net/http"
	"testing"

	"github.com/cacack/my-family/internal/api"
	"github.com/cacack/my-family/internal/config"
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
	if got := get(); got != (api.PrivacySettings{LivingThresholdYears: 110, ViewersSeeLiving: true}) {
		t.Errorf("default settings = %+v", got)
	}

	rec := f.do(t, request{method: http.MethodPut, path: "/api/v1/settings/privacy", cookie: owner,
		body: `{"living_threshold_years":90,"viewers_see_living":false}`})
	if rec.Code != http.StatusOK {
		t.Fatalf("PUT settings: %d %s", rec.Code, rec.Body)
	}
	want := api.PrivacySettings{LivingThresholdYears: 90, ViewersSeeLiving: false}
	if got := decode[api.PrivacySettings](t, rec); got != want {
		t.Errorf("PUT response = %+v, want %+v", got, want)
	}
	if got := get(); got != want {
		t.Errorf("settings after PUT = %+v, want %+v", got, want)
	}

	for _, body := range []string{
		`{"living_threshold_years":49,"viewers_see_living":true}`,
		`{"living_threshold_years":151,"viewers_see_living":true}`,
		`{"viewers_see_living":true}`,
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
			{method: http.MethodPut, path: "/api/v1/settings/privacy", cookie: cookie, body: `{"living_threshold_years":90,"viewers_see_living":true}`},
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
		body: `{"living_threshold_years":90,"viewers_see_living":true}`})
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
		body: `{"living_threshold_years":90,"viewers_see_living":true}`})
	if rec.Code != http.StatusServiceUnavailable {
		t.Errorf("PUT settings without a store: %d, want 503", rec.Code)
	}
}
