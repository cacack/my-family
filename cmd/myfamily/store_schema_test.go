package main

import (
	"errors"
	"path/filepath"
	"strings"
	"testing"

	"github.com/cacack/my-family/internal/repository"
	"github.com/cacack/my-family/internal/repository/sqlite"
)

// TestCommands_RefuseNewerStoreSchema: `serve`, `rebuild` and `user` all
// refuse a database a newer release migrated, naming both versions (ADR-011 §4).
func TestCommands_RefuseNewerStoreSchema(t *testing.T) {
	path := filepath.Join(t.TempDir(), "myfamily.db")
	db, err := sqlite.OpenDB(path)
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	if err := sqlite.NewStoreMigrator(db).Run(t.Context()); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	if _, err := db.Exec(`UPDATE store_schema SET version = 99, name = 'from_the_future'`); err != nil {
		t.Fatalf("stamp: %v", err)
	}
	if err := db.Close(); err != nil {
		t.Fatalf("close: %v", err)
	}
	t.Setenv("SQLITE_PATH", path)
	t.Setenv("DATABASE_URL", "")
	t.Setenv("DEMO_MODE", "")
	t.Setenv("AUTH_MODE", "")

	for name, run := range map[string]func() error{
		"serve":   runServer,
		"rebuild": runRebuild,
		"user":    func() error { return runUser([]string{"reset-password", "--username", "ada"}) },
	} {
		t.Run(name, func(t *testing.T) {
			err := run()
			if !errors.Is(err, repository.ErrStoreSchemaNewer) {
				t.Fatalf("err = %v, want ErrStoreSchemaNewer", err)
			}
			for _, want := range []string{"version 99", "up to 1"} {
				if !strings.Contains(err.Error(), want) {
					t.Errorf("error %q does not say %q", err, want)
				}
			}
		})
	}
}
