package sqlite_test

import (
	"context"
	"errors"
	"os"
	"testing"

	"github.com/google/uuid"

	"github.com/cacack/my-family/internal/domain"
	"github.com/cacack/my-family/internal/repository"
	"github.com/cacack/my-family/internal/repository/sqlite"
)

// setupLegacyReadModelDB builds a database that mimics a pre-#669 read model: the
// slice tables carry the old single-column `id` PRIMARY KEY. SQLite cannot alter a
// PK in place, so NewReadModelStore's CREATE TABLE IF NOT EXISTS leaves these
// definitions untouched and the store must detect that it cannot hold branch rows.
func setupLegacyReadModelDB(t *testing.T) (*sqlite.ReadModelStore, func()) {
	t.Helper()

	tmpFile, err := os.CreateTemp("", "myfamily-legacy-readmodel-*.db")
	if err != nil {
		t.Fatalf("create temp file: %v", err)
	}
	tmpFile.Close()

	db, err := sqlite.OpenDB(tmpFile.Name())
	if err != nil {
		os.Remove(tmpFile.Name())
		t.Fatalf("open database: %v", err)
	}

	// Pre-#669 shape: the real persons columns, but the lone `id` PRIMARY KEY and
	// no branch_id/deleted. This is what an existing deployment's database looks
	// like; SQLite cannot alter that PK in place, so the store must detect it.
	if _, err := db.Exec(`
		CREATE TABLE persons (
			id TEXT PRIMARY KEY,
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
			updated_at TEXT NOT NULL DEFAULT (datetime('now'))
		)`); err != nil {
		db.Close()
		os.Remove(tmpFile.Name())
		t.Fatalf("create legacy persons table: %v", err)
	}

	store, err := sqlite.NewReadModelStore(db)
	if err != nil {
		db.Close()
		os.Remove(tmpFile.Name())
		t.Fatalf("create read model store: %v", err)
	}
	return store, func() {
		db.Close()
		os.Remove(tmpFile.Name())
	}
}

// TestLegacySchemaRefusesBranchWrites verifies the ADR-005 / issue #680 guard: a
// read model whose schema predates branch support must refuse branch-scoped writes
// with repository.ErrBranchesUnsupported instead of appearing branch-capable and
// then failing on an opaque PRIMARY KEY constraint violation. Mainline writes and
// reads must keep working so an un-rebuilt deployment stays usable.
func TestLegacySchemaRefusesBranchWrites(t *testing.T) {
	store, cleanup := setupLegacyReadModelDB(t)
	defer cleanup()
	ctx := context.Background()

	branch := domain.BranchID(uuid.New())
	personID := uuid.New()

	err := store.SavePerson(ctx, branch, &repository.PersonReadModel{
		ID: personID, GivenName: "Branch", Surname: "Write", Version: 1,
	})
	if !errors.Is(err, repository.ErrBranchesUnsupported) {
		t.Fatalf("branch SavePerson on legacy schema: want ErrBranchesUnsupported, got %v", err)
	}

	// Every branch-scoped write is guarded, not just SavePerson.
	if err := store.DeletePerson(ctx, branch, personID); !errors.Is(err, repository.ErrBranchesUnsupported) {
		t.Fatalf("branch DeletePerson on legacy schema: want ErrBranchesUnsupported, got %v", err)
	}
	if err := store.PurgeBranch(ctx, branch); !errors.Is(err, repository.ErrBranchesUnsupported) {
		t.Fatalf("PurgeBranch on legacy schema: want ErrBranchesUnsupported, got %v", err)
	}

	// Mainline stays fully usable on the un-rebuilt database.
	if err := store.SavePerson(ctx, domain.MainBranchID, &repository.PersonReadModel{
		ID: personID, GivenName: "Main", Surname: "Write", Version: 1,
	}); err != nil {
		t.Fatalf("main SavePerson on legacy schema: %v", err)
	}
	got, err := store.GetPerson(ctx, domain.MainBranchID, personID)
	if err != nil {
		t.Fatalf("main GetPerson on legacy schema: %v", err)
	}
	if got == nil || got.GivenName != "Main" {
		t.Fatalf("main GetPerson on legacy schema: want Main, got %+v", got)
	}
}

// TestFreshSchemaAllowsBranchWrites is the control: a freshly created read model
// has the composite (id, branch_id) key and must NOT be flagged legacy.
func TestFreshSchemaAllowsBranchWrites(t *testing.T) {
	store, cleanup := setupTestReadModelDB(t)
	defer cleanup()
	ctx := context.Background()

	branch := domain.BranchID(uuid.New())
	personID := uuid.New()

	if err := store.SavePerson(ctx, domain.MainBranchID, branchPersonRM(personID, "Main", "Row")); err != nil {
		t.Fatalf("main SavePerson: %v", err)
	}
	if err := store.SavePerson(ctx, branch, branchPersonRM(personID, "Branch", "Row")); err != nil {
		t.Fatalf("branch SavePerson on fresh schema: want success, got %v", err)
	}
}
