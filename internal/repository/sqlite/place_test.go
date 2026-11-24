package sqlite_test

import (
	"testing"

	"github.com/cacack/my-family/internal/testutil/placetest"
)

// TestPlaceHierarchy_Parity, TestPlaceMatching and TestPlaceList run the
// shared place suites against the sqlite backend; the memory and postgres
// packages run the same bodies (DB-001).
func TestPlaceHierarchy_Parity(t *testing.T) {
	store, cleanup := setupTestReadModelDB(t)
	defer cleanup()
	placetest.RunHierarchy(t, store)
}

func TestPlaceMatching(t *testing.T) {
	store, cleanup := setupTestReadModelDB(t)
	defer cleanup()
	placetest.RunMatching(t, store)
}

func TestPlaceList(t *testing.T) {
	store, cleanup := setupTestReadModelDB(t)
	defer cleanup()
	placetest.RunPlaceList(t, store)
}
