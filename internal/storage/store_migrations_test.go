package storage

import (
	"database/sql"
	"errors"
	"path/filepath"
	"strings"
	"testing"

	"github.com/cacack/my-family/internal/config"
	"github.com/cacack/my-family/internal/repository"
	"github.com/cacack/my-family/internal/repository/postgres"
	"github.com/cacack/my-family/internal/repository/sqlite"
	"github.com/cacack/my-family/internal/testutil/pgtest"
)

// TestStoreMigrations_SameOnBothBackends holds the two lists to one length and
// one set of names in one order (ADR-011 §2, DB-010).
func TestStoreMigrations_SameOnBothBackends(t *testing.T) {
	names := func(list []repository.StoreMigration) []string {
		out := make([]string, len(list))
		for i, m := range list {
			out[i] = m.Name
		}
		return out
	}
	lite, pg := names(sqlite.StoreMigrations()), names(postgres.StoreMigrations())
	if strings.Join(lite, ",") != strings.Join(pg, ",") {
		t.Errorf("store migrations differ:\n sqlite:   %v\n postgres: %v", lite, pg)
	}
	if len(lite) == 0 || lite[0] != "baseline" {
		t.Errorf("first migration = %v, want baseline", lite)
	}
}

// stampFuture marks db as migrated by a release this binary predates.
func stampFuture(t *testing.T, db *sql.DB) {
	t.Helper()
	if _, err := db.Exec(`UPDATE store_schema SET version = 99, name = 'from_the_future'`); err != nil {
		t.Fatalf("stamp: %v", err)
	}
}

// assertRefusesNewer opens cfg for every access and expects a refusal naming
// both versions, before any store is constructed.
func assertRefusesNewer(t *testing.T, cfg *config.Config, readModelCreated func() bool) {
	t.Helper()
	for _, access := range []Access{AccessServe, AccessRebuild} {
		stores, err := Open(cfg, access)
		if err == nil {
			_ = stores.Close()
			t.Fatalf("Open(%v) on a newer database succeeded", access)
		}
		if !errors.Is(err, repository.ErrStoreSchemaNewer) {
			t.Fatalf("Open(%v): err = %v, want ErrStoreSchemaNewer", access, err)
		}
		for _, want := range []string{"version 99 (from_the_future)", "up to 1 (baseline)"} {
			if !strings.Contains(err.Error(), want) {
				t.Errorf("Open(%v): error %q does not say %q", access, err, want)
			}
		}
	}
	if readModelCreated() {
		t.Error("the read model was constructed before the refusal")
	}
}

func TestSQLite_RefusesNewerStoreSchema(t *testing.T) {
	path := filepath.Join(t.TempDir(), "myfamily.db")
	db, err := sqlite.OpenDB(path)
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	defer db.Close()
	if err := sqlite.NewStoreMigrator(db).Run(t.Context()); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	stampFuture(t, db)

	assertRefusesNewer(t, &config.Config{SQLitePath: path}, func() bool {
		var n int
		if err := db.QueryRow(`SELECT COUNT(*) FROM sqlite_master WHERE name = 'persons'`).Scan(&n); err != nil {
			t.Fatalf("look for persons: %v", err)
		}
		return n > 0
	})
}

func TestPostgres_RefusesNewerStoreSchema(t *testing.T) {
	dsn := pgtest.NewDatabase(t)
	db, err := postgres.OpenDB(dsn)
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	defer db.Close()
	if err := postgres.NewStoreMigrator(db).Run(t.Context()); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	stampFuture(t, db)

	assertRefusesNewer(t, &config.Config{DatabaseURL: dsn}, func() bool {
		var exists bool
		if err := db.QueryRow(`SELECT to_regclass('persons') IS NOT NULL`).Scan(&exists); err != nil {
			t.Fatalf("look for persons: %v", err)
		}
		return exists
	})
}

// TestSQLite_OpenStampsTheBaseline is the first start of the runner's release
// over a store: it applies the baseline and stamps it.
func TestSQLite_OpenStampsTheBaseline(t *testing.T) {
	path := filepath.Join(t.TempDir(), "myfamily.db")
	stores, err := OpenSQLite(path, AccessServe)
	if err != nil {
		t.Fatalf("OpenSQLite: %v", err)
	}
	defer stores.Close()
	var version int
	var name string
	if err := stores.db.QueryRow(`SELECT version, name FROM store_schema WHERE id = 1`).Scan(&version, &name); err != nil {
		t.Fatalf("read stamp: %v", err)
	}
	if version != 1 || name != "baseline" {
		t.Errorf("stamp = %d %q, want 1 baseline", version, name)
	}
}
