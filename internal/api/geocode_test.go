package api_test

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/cacack/my-family/internal/api"
	"github.com/cacack/my-family/internal/geocode"
	"github.com/cacack/my-family/internal/repository"
)

// geocodeStub is a Nominatim stand-in that counts its requests.
type geocodeStub struct {
	*httptest.Server
	requests atomic.Int32
}

func newGeocodeStub(t *testing.T, handler http.HandlerFunc) *geocodeStub {
	t.Helper()
	s := &geocodeStub{}
	s.Server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		s.requests.Add(1)
		handler(w, r)
	}))
	t.Cleanup(s.Close)
	return s
}

// geocodeFixture is an accounts-mode server with a signed-in owner, editor
// and viewer, and a geocoder at stub unless stub is nil.
func geocodeFixture(t *testing.T, stub *geocodeStub) (*authFixture, map[repository.Role]*http.Cookie) {
	t.Helper()
	var opts []api.ServerOption
	if stub != nil {
		lookup := geocode.NewLookup(geocode.NewNominatim(stub.URL, "ada@example.com", "test", stub.Client()),
			geocode.WithInterval(time.Millisecond), geocode.WithTimeout(200*time.Millisecond))
		opts = append(opts, api.WithGeocoder(lookup))
	}
	f := newAuthFixtureWith(t, accountsConfig, opts...)
	f.createOwner(t)
	return f, map[repository.Role]*http.Cookie{
		repository.RoleOwner:  f.signIn(t),
		repository.RoleEditor: f.addMember(t, repository.RoleEditor),
		repository.RoleViewer: f.addMember(t, repository.RoleViewer),
	}
}

func geocodePath(q string) string {
	return "/api/v1/geocode?q=" + url.QueryEscape(q)
}

func appConfigOf(t *testing.T, f *authFixture, cookie *http.Cookie) map[string]any {
	t.Helper()
	rec := f.do(t, request{method: http.MethodGet, path: "/api/v1/config", cookie: cookie})
	if rec.Code != http.StatusOK {
		t.Fatalf("GET /config = %d: %s", rec.Code, rec.Body)
	}
	var cfg map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &cfg); err != nil {
		t.Fatal(err)
	}
	return cfg
}

func TestGeocode_Disabled(t *testing.T) {
	// The stub exists to show nothing reaches it; the server is not given it.
	stub := newGeocodeStub(t, func(w http.ResponseWriter, _ *http.Request) { _, _ = io.WriteString(w, `[]`) })
	f, cookies := geocodeFixture(t, nil)
	rec := f.do(t, request{method: http.MethodGet, path: geocodePath("Boston"), cookie: cookies[repository.RoleEditor]})
	if rec.Code != http.StatusConflict || errorCodeOf(rec) != "geocoder_disabled" {
		t.Errorf("GET /geocode = %d %s, want 409 geocoder_disabled", rec.Code, rec.Body)
	}
	if got := appConfigOf(t, f, cookies[repository.RoleViewer])["geocoder_enabled"]; got != false {
		t.Errorf("geocoder_enabled = %v, want false", got)
	}
	if n := stub.requests.Load(); n != 0 {
		t.Errorf("provider requests = %d, want 0", n)
	}
}

func TestGeocode_Lookup(t *testing.T) {
	var query url.Values
	stub := newGeocodeStub(t, func(w http.ResponseWriter, r *http.Request) {
		query = r.URL.Query()
		_, _ = io.WriteString(w, `[{"display_name":"Boston, Suffolk County, Massachusetts, United States","lat":"42.3554334","lon":"-71.060511"}]`)
	})
	f, cookies := geocodeFixture(t, stub)
	if got := appConfigOf(t, f, cookies[repository.RoleViewer])["geocoder_enabled"]; got != true {
		t.Errorf("geocoder_enabled = %v, want true", got)
	}

	rec := f.do(t, request{method: http.MethodGet, path: geocodePath("Boston, MA"), cookie: cookies[repository.RoleViewer]})
	if !forbidden(rec) {
		t.Errorf("viewer: GET /geocode = %d %s, want 403", rec.Code, rec.Body)
	}
	if n := stub.requests.Load(); n != 0 {
		t.Fatalf("provider requests after the viewer = %d, want 0", n)
	}

	for _, role := range []repository.Role{repository.RoleEditor, repository.RoleOwner} {
		rec := f.do(t, request{method: http.MethodGet, path: geocodePath("Boston, MA"), cookie: cookies[role]})
		if rec.Code != http.StatusOK {
			t.Fatalf("%s: GET /geocode = %d %s", role, rec.Code, rec.Body)
		}
		var list api.GeocodeCandidateList
		if err := json.Unmarshal(rec.Body.Bytes(), &list); err != nil {
			t.Fatal(err)
		}
		want := api.GeocodeCandidate{
			DisplayName: "Boston, Suffolk County, Massachusetts, United States",
			Latitude:    42.3554334, Longitude: -71.060511, Query: "Boston, MA", Provider: "Nominatim",
			Attribution: "Data © OpenStreetMap contributors, ODbL 1.0. https://osm.org/copyright",
		}
		if list.Total != 1 || len(list.Items) != 1 || list.Items[0] != want {
			t.Errorf("%s: candidates = %+v, want [%+v]", role, list, want)
		}
	}
	// The second lookup was answered from the cache.
	if n := stub.requests.Load(); n != 1 {
		t.Errorf("provider requests = %d, want 1", n)
	}
	if query.Get("q") != "Boston, MA" {
		t.Errorf("q = %q", query.Get("q"))
	}
}

func TestGeocode_Responses(t *testing.T) {
	block := make(chan struct{})
	t.Cleanup(func() { close(block) })
	tests := []struct {
		name       string
		handler    http.HandlerFunc
		q          string
		wantStatus int
		wantCode   string
	}{
		{name: "no match", handler: func(w http.ResponseWriter, _ *http.Request) { _, _ = io.WriteString(w, `[]`) },
			q: "Nowhere", wantStatus: http.StatusOK},
		{name: "provider error", handler: func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(http.StatusInternalServerError) },
			q: "Boston", wantStatus: http.StatusBadGateway, wantCode: "geocoder_error"},
		{name: "timeout", handler: func(_ http.ResponseWriter, r *http.Request) {
			select {
			case <-block:
			case <-r.Context().Done():
			}
		}, q: "Boston", wantStatus: http.StatusGatewayTimeout, wantCode: "geocoder_timeout"},
		{name: "blank name", handler: func(w http.ResponseWriter, _ *http.Request) { _, _ = io.WriteString(w, `[]`) },
			q: "   ", wantStatus: http.StatusBadRequest, wantCode: api.CodeValidation},
		{name: "long name", handler: func(w http.ResponseWriter, _ *http.Request) { _, _ = io.WriteString(w, `[]`) },
			q: strings.Repeat("é", geocode.MaxQueryLength+1), wantStatus: http.StatusBadRequest},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			f, cookies := geocodeFixture(t, newGeocodeStub(t, tt.handler))
			rec := f.do(t, request{method: http.MethodGet, path: geocodePath(tt.q), cookie: cookies[repository.RoleEditor]})
			if rec.Code != tt.wantStatus {
				t.Fatalf("GET /geocode = %d %s, want %d", rec.Code, rec.Body, tt.wantStatus)
			}
			if tt.wantCode != "" && errorCodeOf(rec) != tt.wantCode {
				t.Errorf("code = %q, want %q", errorCodeOf(rec), tt.wantCode)
			}
			if tt.wantStatus == http.StatusOK && !strings.Contains(rec.Body.String(), `"items":[]`) {
				t.Errorf("body = %s, want no items", rec.Body)
			}
		})
	}
}
