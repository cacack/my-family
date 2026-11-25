package main

import (
	"bytes"
	"context"
	"errors"
	"io"
	"log"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"

	"github.com/cacack/my-family/internal/config"
	"github.com/cacack/my-family/internal/storage"
	"github.com/cacack/my-family/internal/testutil/pgtest"
)

func TestRestoreBackup(t *testing.T) {
	source := servedDatabase(t)
	archive := filepath.Join(t.TempDir(), "backup.zip")
	if err := writeBackup(context.Background(), source, archive, &bytes.Buffer{}); err != nil {
		t.Fatal(err)
	}

	target := &config.Config{SQLitePath: filepath.Join(t.TempDir(), "restored.db")}
	var out bytes.Buffer
	if err := restoreBackup(context.Background(), target, archive, &out); err != nil {
		t.Fatalf("restoreBackup: %v", err)
	}
	if !strings.Contains(out.String(), "into SQLite ("+target.SQLitePath+")") ||
		!strings.Contains(out.String(), ": 1 events,") || !strings.Contains(out.String(), "Everyone signs in again") {
		t.Errorf("output = %q", out.String())
	}

	// A second restore finds the database holds data, and the source is
	// still held by its server.
	if err := restoreBackup(context.Background(), target, archive, &bytes.Buffer{}); err == nil {
		t.Error("restore into the restored database: want error")
	}
	if err := restoreBackup(context.Background(), source, archive, &bytes.Buffer{}); !errors.Is(err, storage.ErrServerRunning) {
		t.Errorf("restore into a served database = %v, want ErrServerRunning", err)
	}
}

func TestRunRestore(t *testing.T) {
	for _, args := range [][]string{nil, {""}, {"a", "b"}} {
		if err := runRestore(args, &bytes.Buffer{}); err == nil || err.Error() != restoreUsage {
			t.Errorf("runRestore(%q) = %v, want usage", args, err)
		}
	}
	t.Setenv("SQLITE_PATH", filepath.Join(t.TempDir(), "restored.db"))
	t.Setenv("DATABASE_URL", "")
	t.Setenv("DEMO_MODE", "")
	if err := runRestore([]string{filepath.Join(t.TempDir(), "missing.zip")}, &bytes.Buffer{}); err == nil {
		t.Error("runRestore of a missing archive: want error")
	}
}

// TestRestoreAfterFirstStart walks the recovery on a new machine (#1835):
// install, start the server once on an empty database and open the web UI,
// stop it, then restore. Starting and the setup page's first requests must
// write nothing restore counts as data, in either sign-in mode.
func TestRestoreAfterFirstStart(t *testing.T) {
	source := servedDatabase(t)
	archive := filepath.Join(t.TempDir(), "backup.zip")
	if err := writeBackup(context.Background(), source, archive, &bytes.Buffer{}); err != nil {
		t.Fatal(err)
	}

	backends := []struct {
		name   string
		target func(t *testing.T) *config.Config
	}{
		{"SQLite", func(t *testing.T) *config.Config {
			return &config.Config{SQLitePath: filepath.Join(t.TempDir(), "fresh.db")}
		}},
		{"PostgreSQL", func(t *testing.T) *config.Config {
			return &config.Config{DatabaseURL: pgtest.NewDatabase(t)}
		}},
	}
	modes := []struct {
		authMode config.AuthMode
		// requests are the first ones the web UI makes, with the status each
		// gets on a server with no owner yet.
		requests map[string]int
	}{
		{config.AuthModeAccounts, map[string]int{
			"/api/v1/auth/status":          http.StatusOK,
			"/api/v1/config":               http.StatusServiceUnavailable,
			"/api/v1/settings/privacy":     http.StatusServiceUnavailable,
			"/api/v1/settings/home-person": http.StatusServiceUnavailable,
		}},
		{config.AuthModeOpen, map[string]int{
			"/api/v1/auth/status":          http.StatusOK,
			"/api/v1/config":               http.StatusOK,
			"/api/v1/settings/privacy":     http.StatusOK,
			"/api/v1/settings/home-person": http.StatusOK,
		}},
	}
	for _, backend := range backends {
		for _, mode := range modes {
			t.Run(backend.name+"/"+string(mode.authMode), func(t *testing.T) {
				target := backend.target(t)
				target.AuthMode = mode.authMode
				firstStart(t, target, mode.requests)
				if err := restoreBackup(context.Background(), target, archive, &bytes.Buffer{}); err != nil {
					t.Fatalf("restore into a database started once: %v", err)
				}
			})
		}
	}
}

// firstStart starts the server on cfg's database as `serve` does, answers
// requests (path to wanted status) and stops it.
func firstStart(t *testing.T, cfg *config.Config, requests map[string]int) {
	t.Helper()
	stores, err := storage.Open(cfg, storage.AccessServe)
	if err != nil {
		t.Fatal(err)
	}
	defer func() {
		if err := stores.Close(); err != nil {
			t.Error(err)
		}
	}()
	_ = stores.ProjectionFailureWarning(context.Background())
	server, err := newAPIServer(cfg, stores, nil, log.New(io.Discard, "", 0))
	if err != nil {
		t.Fatal(err)
	}
	for path, want := range requests {
		rec := httptest.NewRecorder()
		server.Echo().ServeHTTP(rec, httptest.NewRequest(http.MethodGet, path, http.NoBody))
		if rec.Code != want {
			t.Errorf("GET %s = %d, want %d: %s", path, rec.Code, want, rec.Body.String())
		}
	}
}
