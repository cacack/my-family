package api

import (
	"testing"

	"github.com/cacack/my-family/internal/query"
)

// TestHandleRollbackErrorStrict_NoEventsIsNotFound pins the status a rollback
// against an entity with no events maps to: 404, not 400 (issue #683).
func TestHandleRollbackErrorStrict_NoEventsIsNotFound(t *testing.T) {
	got, err := handleRollbackErrorStrict[string](query.ErrNoEvents,
		func(Error) string { return "400" },
		func(Error) string { return "404" },
		func(Error) string { return "409" },
	)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got != "404" {
		t.Errorf("query.ErrNoEvents mapped to %s, want 404", got)
	}
}
