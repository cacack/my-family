package postgres_test

import (
	"context"
	"testing"

	"github.com/google/uuid"

	"github.com/cacack/my-family/internal/domain"
	"github.com/cacack/my-family/internal/repository"
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

	// The migration drops and re-adds full_name, which gives the column a new
	// attnum, so an unchanged attnum shows a reopen left the column alone.
	attnum := func(t *testing.T) int {
		t.Helper()
		var n int
		if err := db.QueryRow(`SELECT attnum FROM pg_attribute
			WHERE attrelid = 'persons'::regclass AND attname = 'full_name' AND NOT attisdropped`).Scan(&n); err != nil {
			t.Fatalf("read full_name attnum: %v", err)
		}
		return n
	}
	legacy := attnum(t)

	if _, err := pgstore.NewReadModelStore(db); err != nil {
		t.Fatalf("open read model over legacy schema: %v", err)
	}
	check(t)
	migrated := attnum(t)
	if migrated == legacy {
		t.Fatalf("full_name attnum = %d after migration, want the column re-added", migrated)
	}

	if _, err := pgstore.NewReadModelStore(db); err != nil {
		t.Fatalf("reopen migrated read model: %v", err)
	}
	check(t)
	if got := attnum(t); got != migrated {
		t.Errorf("full_name attnum = %d after reopen, want %d: the reopen rewrote the column again", got, migrated)
	}
}

// TestReadModelStore_MigratesPersonNameFullName opens a database whose
// person_names.full_name is still the generated given_name || ' ' || surname
// column and checks that the store recomputes existing rows by the one rule
// (#1130, #985), that a name saved afterwards follows it, and that reopening is
// a no-op.
func TestReadModelStore_MigratesPersonNameFullName(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test")
	}

	db, cleanup := setupPostgres(t)
	defer cleanup()

	if _, err := db.Exec(`
		CREATE TABLE person_names (
			id UUID NOT NULL,
			branch_id UUID NOT NULL DEFAULT '00000000-0000-0000-0000-000000000000',
			person_id UUID NOT NULL,
			given_name VARCHAR(100) NOT NULL,
			surname VARCHAR(100) NOT NULL,
			full_name VARCHAR(200) GENERATED ALWAYS AS (given_name || ' ' || surname) STORED,
			name_prefix VARCHAR(50),
			name_suffix VARCHAR(50),
			surname_prefix VARCHAR(50),
			nickname VARCHAR(100),
			name_type VARCHAR(20) NOT NULL DEFAULT '',
			is_primary BOOLEAN NOT NULL DEFAULT FALSE,
			search_vector TSVECTOR,
			updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
			deleted BOOLEAN NOT NULL DEFAULT FALSE,
			PRIMARY KEY (id, branch_id)
		)`); err != nil {
		t.Fatalf("create legacy person_names table: %v", err)
	}

	want := map[uuid.UUID]string{}
	personID := uuid.New()
	for _, n := range []struct{ prefix, given, surnamePrefix, surname, suffix, full string }{
		{"", "Alpha", "", "", "", "Alpha"},
		{"Rev.", "", "", "Placeholder", "", "Rev. Placeholder"},
		{"Dr.", "Avery", "van", "Original", "Jr.", "Dr. Avery van Original Jr."},
	} {
		id := uuid.New()
		if _, err := db.Exec(`INSERT INTO person_names (id, person_id, given_name, surname, name_prefix, surname_prefix, name_suffix)
			VALUES ($1, $2, $3, $4, NULLIF($5, ''), NULLIF($6, ''), NULLIF($7, ''))`,
			id, personID, n.given, n.surname, n.prefix, n.surnamePrefix, n.suffix); err != nil {
			t.Fatalf("seed person name: %v", err)
		}
		want[id] = n.full
	}

	check := func(t *testing.T) {
		t.Helper()
		for id, wantName := range want {
			var got string
			if err := db.QueryRow(`SELECT full_name FROM person_names WHERE id = $1`, id).Scan(&got); err != nil {
				t.Fatalf("read full_name for %s: %v", id, err)
			}
			if got != wantName {
				t.Errorf("full_name for %s = %q, want %q", id, got, wantName)
			}
		}
	}

	store, err := pgstore.NewReadModelStore(db)
	if err != nil {
		t.Fatalf("open read model over legacy schema: %v", err)
	}
	check(t)

	saved := &repository.PersonNameReadModel{ID: uuid.New(), PersonID: personID, GivenName: "Beta", NameSuffix: "III"}
	if err := store.SavePersonName(context.Background(), domain.MainBranchID, saved); err != nil {
		t.Fatalf("save name after migration: %v", err)
	}
	want[saved.ID] = "Beta III"

	if _, err := pgstore.NewReadModelStore(db); err != nil {
		t.Fatalf("reopen migrated read model: %v", err)
	}
	check(t)
}

// TestPersonNameFullName mirrors repository_test.TestReadModel_PersonNameFullName
// for PostgreSQL: every backend returns the same person_names.full_name (#1130).
func TestPersonNameFullName(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test")
	}

	store, cleanup := setupReadModelStore(t)
	defer cleanup()
	ctx := context.Background()

	tests := []struct {
		name string
		pn   repository.PersonNameReadModel
		want string
	}{
		{"every part", repository.PersonNameReadModel{NamePrefix: "Dr.", GivenName: "Avery", SurnamePrefix: "van",
			Surname: "Original", NameSuffix: "Jr."}, "Dr. Avery van Original Jr."},
		{"no surname", repository.PersonNameReadModel{GivenName: "Alpha"}, "Alpha"},
		{"no given name", repository.PersonNameReadModel{Surname: "Placeholder"}, "Placeholder"},
		{"prefix and surname", repository.PersonNameReadModel{NamePrefix: "Rev.", Surname: "Placeholder"}, "Rev. Placeholder"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			pn := tt.pn
			pn.ID, pn.PersonID = uuid.New(), uuid.New()
			if err := store.SavePersonName(ctx, domain.MainBranchID, &pn); err != nil {
				t.Fatalf("SavePersonName: %v", err)
			}
			got, err := store.GetPersonName(ctx, domain.MainBranchID, pn.ID)
			if err != nil || got == nil {
				t.Fatalf("GetPersonName = %v, %v", got, err)
			}
			if got.FullName != tt.want {
				t.Errorf("FullName = %q, want %q", got.FullName, tt.want)
			}
		})
	}
}

// TestReadModelStore_PersonNameFullNameMigrationFailureIsFatal checks that a
// failed person_names.full_name migration refuses the store instead of leaving
// the column GENERATED, which every later SavePersonName would hit (#1130).
func TestReadModelStore_PersonNameFullNameMigrationFailureIsFatal(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test")
	}

	db, cleanup := setupPostgres(t)
	defer cleanup()

	if _, err := db.Exec(`
		CREATE TABLE person_names (
			id UUID NOT NULL,
			branch_id UUID NOT NULL DEFAULT '00000000-0000-0000-0000-000000000000',
			person_id UUID NOT NULL,
			given_name VARCHAR(100) NOT NULL,
			surname VARCHAR(100) NOT NULL,
			full_name VARCHAR(200) GENERATED ALWAYS AS (given_name || ' ' || surname) STORED,
			name_prefix VARCHAR(50),
			name_suffix VARCHAR(50),
			surname_prefix VARCHAR(50),
			nickname VARCHAR(100),
			name_type VARCHAR(20) NOT NULL DEFAULT '',
			is_primary BOOLEAN NOT NULL DEFAULT FALSE,
			search_vector TSVECTOR,
			updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
			deleted BOOLEAN NOT NULL DEFAULT FALSE,
			PRIMARY KEY (id, branch_id)
		)`); err != nil {
		t.Fatalf("create legacy person_names table: %v", err)
	}
	// A dependent view makes the migration's DROP COLUMN fail.
	if _, err := db.Exec(`CREATE VIEW person_name_full_names AS SELECT full_name FROM person_names`); err != nil {
		t.Fatalf("create dependent view: %v", err)
	}

	if _, err := pgstore.NewReadModelStore(db); err == nil {
		t.Fatal("NewReadModelStore succeeded although person_names.full_name could not be migrated")
	}

	var generated string
	if err := db.QueryRow(`SELECT is_generated FROM information_schema.columns
		WHERE table_schema = CURRENT_SCHEMA() AND table_name = 'person_names' AND column_name = 'full_name'`).
		Scan(&generated); err != nil {
		t.Fatalf("inspect full_name: %v", err)
	}
	if generated != "ALWAYS" {
		t.Errorf("full_name is_generated = %q, want the failed migration rolled back to ALWAYS", generated)
	}
}
