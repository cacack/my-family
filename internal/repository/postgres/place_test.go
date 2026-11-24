package postgres_test

import (
	"testing"

	"github.com/cacack/my-family/internal/testutil/placetest"
)

// TestPlaceHierarchy_Parity and TestPlaceMatching run the shared place suites
// against the postgres backend; the memory and sqlite packages run the same
// bodies (DB-001).
func TestPlaceHierarchy_Parity(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test")
	}
	store, cleanup := setupReadModelStore(t)
	defer cleanup()
	placetest.RunHierarchy(t, store)
}

func TestPlaceMatching(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test")
	}
	store, cleanup := setupReadModelStore(t)
	defer cleanup()
	placetest.RunMatching(t, store)
}
