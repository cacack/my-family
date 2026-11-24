package memory_test

import (
	"testing"

	"github.com/cacack/my-family/internal/repository/memory"
	"github.com/cacack/my-family/internal/testutil/placetest"
)

// TestPlaceHierarchy_Parity, TestPlaceMatching and TestPlaceList run the
// shared place suites against the memory backend; the sqlite and postgres
// packages run the same bodies (DB-001).
func TestPlaceHierarchy_Parity(t *testing.T) {
	placetest.RunHierarchy(t, memory.NewReadModelStore())
}

func TestPlaceMatching(t *testing.T) {
	placetest.RunMatching(t, memory.NewReadModelStore())
}

func TestPlaceList(t *testing.T) {
	placetest.RunPlaceList(t, memory.NewReadModelStore())
}

// TestCoordinateOrigins runs the shared #1563 suite against the memory
// backend; the sqlite and postgres packages run the same body (DB-001).
func TestCoordinateOrigins(t *testing.T) {
	placetest.RunCoordinateOrigins(t, memory.NewReadModelStore())
}

// TestPlaceFacts runs the shared #1735 suite against the memory backend; the
// sqlite and postgres packages run the same body (DB-001).
func TestPlaceFacts(t *testing.T) {
	placetest.RunPlaceFacts(t, memory.NewReadModelStore())
}

// TestPlaceAliases runs the shared #1736 suite against the memory backend;
// the sqlite and postgres packages run the same body (DB-001).
func TestPlaceAliases(t *testing.T) {
	placetest.RunPlaceAliases(t, memory.NewReadModelStore())
}
