package memory_test

import (
	"testing"

	"github.com/cacack/my-family/internal/repository/memory"
	"github.com/cacack/my-family/internal/testutil/placetest"
)

// TestPlaceHierarchy_Parity and TestPlaceMatching run the shared place suites
// against the memory backend; the sqlite and postgres packages run the same
// bodies (DB-001).
func TestPlaceHierarchy_Parity(t *testing.T) {
	placetest.RunHierarchy(t, memory.NewReadModelStore())
}

func TestPlaceMatching(t *testing.T) {
	placetest.RunMatching(t, memory.NewReadModelStore())
}
