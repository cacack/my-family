package main

import (
	"context"
	"io"
	"log"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/cacack/my-family/internal/config"
	"github.com/cacack/my-family/internal/storage"
)

// TestNewAPIServer_Geocoder checks that `serve` wires the geocoder only when
// GEOCODER_ENABLED is set, at GEOCODER_URL and identified by GEOCODER_CONTACT.
func TestNewAPIServer_Geocoder(t *testing.T) {
	for _, enabled := range []bool{false, true} {
		var requests atomic.Int32
		var userAgent atomic.Value
		provider := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			requests.Add(1)
			userAgent.Store(r.UserAgent())
			_, _ = io.WriteString(w, `[{"display_name":"Boston","lat":"42.36","lon":"-71.06"}]`)
		}))
		cfg := &config.Config{
			SQLitePath:      filepath.Join(t.TempDir(), "myfamily.db"),
			AuthMode:        config.AuthModeOpen,
			GeocoderEnabled: enabled,
			GeocoderURL:     provider.URL,
			GeocoderContact: "ada@example.com",
		}
		stores, err := storage.Open(cfg, storage.AccessServe)
		if err != nil {
			t.Fatal(err)
		}
		_ = stores.ProjectionFailureWarning(context.Background())
		server, err := newAPIServer(cfg, stores, nil, log.New(io.Discard, "", 0))
		if err != nil {
			t.Fatal(err)
		}
		get := func(path string) *httptest.ResponseRecorder {
			rec := httptest.NewRecorder()
			server.Echo().ServeHTTP(rec, httptest.NewRequest(http.MethodGet, path, http.NoBody))
			return rec
		}

		wantFlag := `"geocoder_enabled":false`
		wantStatus := http.StatusConflict
		var wantRequests int32
		if enabled {
			wantFlag, wantStatus, wantRequests = `"geocoder_enabled":true`, http.StatusOK, 1
		}
		if rec := get("/api/v1/config"); !strings.Contains(rec.Body.String(), wantFlag) {
			t.Errorf("enabled=%v: GET /config = %s, want %s", enabled, rec.Body, wantFlag)
		}
		if rec := get("/api/v1/geocode?q=Boston"); rec.Code != wantStatus {
			t.Errorf("enabled=%v: GET /geocode = %d, want %d: %s", enabled, rec.Code, wantStatus, rec.Body)
		}
		if got := requests.Load(); got != wantRequests {
			t.Errorf("enabled=%v: provider requests = %d, want %d", enabled, got, wantRequests)
		}
		if ua, _ := userAgent.Load().(string); enabled && !strings.Contains(ua, "ada@example.com") {
			t.Errorf("User-Agent = %q, want the contact", ua)
		}
		provider.Close()
		if err := stores.Close(); err != nil {
			t.Error(err)
		}
	}
}
