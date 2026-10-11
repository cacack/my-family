package sqlite_test

import (
	"os"
	"testing"

	"github.com/cacack/my-family/internal/repository/sqlite"
)

// TestReadModel_MigratesPersonFullName opens a database whose persons.full_name
// still uses the old always-join-with-a-space expression and checks that the
// store recomputes existing rows without a stray space (#977), and that
// reopening the migrated database is a no-op.
func TestReadModel_MigratesPersonFullName(t *testing.T) {
	tmpFile, err := os.CreateTemp("", "myfamily-full-name-migration-*.db")
	if err != nil {
		t.Fatalf("create temp file: %v", err)
	}
	tmpFile.Close()
	t.Cleanup(func() { os.Remove(tmpFile.Name()) })

	db, err := sqlite.OpenDB(tmpFile.Name())
	if err != nil {
		t.Fatalf("open database: %v", err)
	}
	t.Cleanup(func() { db.Close() })

	if _, err := db.Exec(`
		CREATE TABLE persons (
			id TEXT NOT NULL,
			given_name TEXT NOT NULL,
			surname TEXT NOT NULL,
			full_name TEXT GENERATED ALWAYS AS (given_name || ' ' || surname) STORED,
			gender TEXT,
			birth_date_raw TEXT,
			birth_date_sort TEXT,
			birth_place TEXT,
			death_date_raw TEXT,
			death_date_sort TEXT,
			death_place TEXT,
			notes TEXT,
			research_status TEXT,
			version INTEGER NOT NULL DEFAULT 1,
			updated_at TEXT NOT NULL DEFAULT (datetime('now')),
			branch_id TEXT NOT NULL DEFAULT '00000000-0000-0000-0000-000000000000',
			deleted INTEGER NOT NULL DEFAULT 0,
			PRIMARY KEY (id, branch_id)
		);
		CREATE INDEX idx_persons_full_name ON persons(full_name);
		INSERT INTO persons (id, given_name, surname) VALUES
			('a', 'Alpha', ''), ('b', '', 'Placeholder'), ('c', 'Alpha', 'Placeholder');
	`); err != nil {
		t.Fatalf("create legacy persons table: %v", err)
	}

	want := map[string]string{"a": "Alpha", "b": "Placeholder", "c": "Alpha Placeholder"}
	check := func(t *testing.T) {
		t.Helper()
		for id, wantName := range want {
			var got string
			if err := db.QueryRow(`SELECT full_name FROM persons WHERE id = ?`, id).Scan(&got); err != nil {
				t.Fatalf("read full_name for %s: %v", id, err)
			}
			if got != wantName {
				t.Errorf("full_name for %s = %q, want %q", id, got, wantName)
			}
		}
		var indexes int
		if err := db.QueryRow(
			`SELECT COUNT(*) FROM sqlite_master WHERE type = 'index' AND name = 'idx_persons_full_name'`,
		).Scan(&indexes); err != nil {
			t.Fatalf("look up full_name index: %v", err)
		}
		if indexes != 1 {
			t.Errorf("idx_persons_full_name count = %d, want 1", indexes)
		}
	}

	if _, err := sqlite.NewReadModelStore(db); err != nil {
		t.Fatalf("open read model over legacy schema: %v", err)
	}
	check(t)

	if _, err := sqlite.NewReadModelStore(db); err != nil {
		t.Fatalf("reopen migrated read model: %v", err)
	}
	check(t)
}
