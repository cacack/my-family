package postgres_test

import (
	"testing"

	"github.com/cacack/my-family/internal/testutil/placetest"
)

// TestPlaceHierarchy_Parity, TestPlaceMatching and TestPlaceList run the
// shared place suites against the postgres backend; the memory and sqlite
// packages run the same bodies (DB-001).
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

func TestPlaceList(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test")
	}
	store, cleanup := setupReadModelStore(t)
	defer cleanup()
	placetest.RunPlaceList(t, store)
}

// TestCoordinateOrigins runs the shared #1563 suite against the postgres
// backend; the memory and sqlite packages run the same body (DB-001).
func TestCoordinateOrigins(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test")
	}
	store, cleanup := setupReadModelStore(t)
	defer cleanup()
	placetest.RunCoordinateOrigins(t, store)
}

// TestPlaceFacts runs the shared #1735 suite against the postgres backend;
// the memory and sqlite packages run the same body (DB-001).
func TestPlaceFacts(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test")
	}
	store, cleanup := setupReadModelStore(t)
	defer cleanup()
	placetest.RunPlaceFacts(t, store)
}

// TestPlaceAliases runs the shared #1736 suite against the postgres backend;
// the memory and sqlite packages run the same body (DB-001).
func TestPlaceAliases(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test")
	}
	store, cleanup := setupReadModelStore(t)
	defer cleanup()
	placetest.RunPlaceAliases(t, store)
}

// TestUnmappedPlaces runs the shared #1561 suite against the postgres
// backend; the memory and sqlite packages run the same body (DB-001).
func TestUnmappedPlaces(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test")
	}
	store, cleanup := setupReadModelStore(t)
	defer cleanup()
	placetest.RunUnmappedPlaces(t, store)
}
