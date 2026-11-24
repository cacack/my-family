package postgres_test

import (
	"testing"

	pgstore "github.com/cacack/my-family/internal/repository/postgres"
)

// TestPlaceFactTables_NoBranchID is PL-005's schema half: place facts are
// main-only, so no place fact table carries a branch_id column.
func TestPlaceFactTables_NoBranchID(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test")
	}
	db, cleanup := setupPostgres(t)
	defer cleanup()
	if _, err := pgstore.NewReadModelStore(db); err != nil {
		t.Fatal(err)
	}
	for _, table := range []string{"places"} {
		if !hasColumn(t, db, table, "id") {
			t.Fatalf("place fact table %s does not exist", table)
		}
		if hasColumn(t, db, table, "branch_id") {
			t.Errorf("place fact table %s has a branch_id column; place facts are main-only (PL-005)", table)
		}
	}
}
