package sqlite_test

import (
	"os"
	"testing"

	"github.com/cacack/my-family/internal/repository/sqlite"
)

// TestPlaceFactTables_NoBranchID is PL-005's schema half: place facts are
// main-only, so no place fact table carries a branch_id column.
func TestPlaceFactTables_NoBranchID(t *testing.T) {
	tmp, err := os.CreateTemp("", "myfamily-place-schema-*.db")
	if err != nil {
		t.Fatal(err)
	}
	tmp.Close()
	defer os.Remove(tmp.Name())
	db, err := sqlite.OpenDB(tmp.Name())
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	if _, err := sqlite.NewReadModelStore(db); err != nil {
		t.Fatal(err)
	}
	for table, column := range map[string]string{"places": "key", "place_aliases": "key", "place_forms": "positional_key", "place_external_ids": "place_id"} {
		if !columnExists(t, db, table, column) {
			t.Fatalf("place fact table %s does not exist", table)
		}
		if columnExists(t, db, table, "branch_id") {
			t.Errorf("place fact table %s has a branch_id column; place facts are main-only (PL-005)", table)
		}
	}
}
