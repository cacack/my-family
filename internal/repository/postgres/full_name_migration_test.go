package postgres_test

import (
	"testing"

	"github.com/google/uuid"

	pgstore "github.com/cacack/my-family/internal/repository/postgres"
)

// TestReadModelStore_MigratesPersonFullName opens a database whose
// persons.full_name still uses the old always-join-with-a-space expression and
// checks that the store recomputes existing rows without a stray space (#977),
// and that reopening the migrated database is a no-op.
func TestReadModelStore_MigratesPersonFullName(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test")
	}

	db, cleanup := setupPostgres(t)
	defer cleanup()

	if _, err := db.Exec(`
		CREATE TABLE persons (
			id UUID NOT NULL,
			branch_id UUID NOT NULL DEFAULT '00000000-0000-0000-0000-000000000000',
			given_name VARCHAR(100) NOT NULL,
			surname VARCHAR(100) NOT NULL,
			full_name VARCHAR(200) GENERATED ALWAYS AS (given_name || ' ' || surname) STORED,
			gender VARCHAR(10),
			birth_date_raw VARCHAR(100),
			birth_date_sort DATE,
			birth_place VARCHAR(255),
			death_date_raw VARCHAR(100),
			death_date_sort DATE,
			death_place VARCHAR(255),
			notes TEXT,
			research_status VARCHAR(20),
			search_vector TSVECTOR,
			version BIGINT NOT NULL DEFAULT 1,
			updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
			deleted BOOLEAN NOT NULL DEFAULT FALSE,
			PRIMARY KEY (id, branch_id)
		)`); err != nil {
		t.Fatalf("create legacy persons table: %v", err)
	}

	want := map[uuid.UUID]string{}
	for _, p := range []struct{ given, surname, full string }{
		{"Alpha", "", "Alpha"},
		{"", "Placeholder", "Placeholder"},
		{"Alpha", "Placeholder", "Alpha Placeholder"},
	} {
		id := uuid.New()
		if _, err := db.Exec(`INSERT INTO persons (id, given_name, surname) VALUES ($1, $2, $3)`, id, p.given, p.surname); err != nil {
			t.Fatalf("seed person: %v", err)
		}
		want[id] = p.full
	}

	check := func(t *testing.T) {
		t.Helper()
		for id, wantName := range want {
			var got string
			if err := db.QueryRow(`SELECT full_name FROM persons WHERE id = $1`, id).Scan(&got); err != nil {
				t.Fatalf("read full_name for %s: %v", id, err)
			}
			if got != wantName {
				t.Errorf("full_name for %s = %q, want %q", id, got, wantName)
			}
		}
	}

	if _, err := pgstore.NewReadModelStore(db); err != nil {
		t.Fatalf("open read model over legacy schema: %v", err)
	}
	check(t)

	if _, err := pgstore.NewReadModelStore(db); err != nil {
		t.Fatalf("reopen migrated read model: %v", err)
	}
	check(t)
}
