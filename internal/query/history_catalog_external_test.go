package query_test

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/cacack/my-family/internal/command"
	"github.com/cacack/my-family/internal/query"
)

// TestHistoryCatalog_MapsEveryBranchAwareEventType is #827's guard: every
// event a branch-scoped handler may write (command.BranchAwareEventTypes) is
// mapped to a ChangeEntry type — so branch compare and merge review never show
// one as "unknown" — and the every-type compare fixture exercises each one.
func TestHistoryCatalog_MapsEveryBranchAwareEventType(t *testing.T) {
	mapped := map[string]bool{}
	for _, entityType := range query.HistoryEntityTypes() {
		for _, eventType := range query.HistoryEventTypesForEntity(entityType) {
			mapped[eventType] = true
		}
	}
	fixture := query.BranchCompareFixtureEventTypes(t)

	for _, eventType := range command.BranchAwareEventTypes() {
		assert.True(t, mapped[eventType], "branch-aware event type %s is not mapped in the history catalog", eventType)
		assert.True(t, fixture[eventType], "branch-aware event type %s is not written by the every-type compare fixture (history_every_type_test.go)", eventType)
	}
}
