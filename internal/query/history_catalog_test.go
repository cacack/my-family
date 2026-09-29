package query

import (
	"context"
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"sort"
	"strconv"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/cacack/my-family/internal/domain"
	"github.com/cacack/my-family/internal/repository"
)

// decodableEventTypes returns the event types repository.StoredEvent.DecodeEvent
// handles, read from the case clauses of its switch — the store's own list of
// every event type it can hold (invariant ES-007), so the history catalog is
// checked against the source of truth rather than a copy of it.
func decodableEventTypes(t *testing.T) []string {
	t.Helper()
	file, err := parser.ParseFile(token.NewFileSet(), "../repository/eventstore.go", nil, 0)
	require.NoError(t, err)

	var types []string
	ast.Inspect(file, func(n ast.Node) bool {
		fn, ok := n.(*ast.FuncDecl)
		if !ok || fn.Name.Name != "DecodeEvent" {
			return true
		}
		ast.Inspect(fn.Body, func(n ast.Node) bool {
			clause, ok := n.(*ast.CaseClause)
			if !ok {
				return true
			}
			for _, expr := range clause.List {
				if lit, ok := expr.(*ast.BasicLit); ok && lit.Kind == token.STRING {
					value, err := strconv.Unquote(lit.Value)
					require.NoError(t, err)
					types = append(types, value)
				}
			}
			return true
		})
		return false
	})
	require.NotEmpty(t, types, "found no case clauses in DecodeEvent")
	sort.Strings(types)
	return types
}

// TestHistoryCatalog_CoversEveryEventType is the #739 guarantee: every event
// type the store can decode is either mapped to a valid ChangeEntry type and
// action or deliberately excluded — none renders as "unknown" — and the
// catalog lists nothing the store cannot hold.
func TestHistoryCatalog_CoversEveryEventType(t *testing.T) {
	decodable := decodableEventTypes(t)
	for _, eventType := range decodable {
		class, ok := classifyHistoryEvent(eventType)
		if !assert.True(t, ok, "event type %s is not classified in historyEventCatalog: map it or exclude it with a reason", eventType) {
			continue
		}
		if class.Excluded() {
			assert.Empty(t, class.EntityType, "%s is excluded but also mapped", eventType)
			continue
		}
		assert.True(t, validEntityTypes[class.EntityType], "%s maps to entity type %q outside the ChangeEntry enum", eventType, class.EntityType)
		assert.True(t, validActions[class.Action], "%s maps to action %q outside the ChangeEntry enum", eventType, class.Action)
	}

	known := make(map[string]bool, len(decodable))
	for _, eventType := range decodable {
		known[eventType] = true
	}
	for eventType := range historyEventCatalog {
		assert.True(t, known[eventType], "historyEventCatalog lists %s, which DecodeEvent does not handle", eventType)
	}
}

// Every research-metadata event the branch diff strips is either excluded
// from the change log or, for a branch's lifecycle (#832), reported as a
// change to the branch itself — never as a change to genealogy data.
func TestHistoryCatalog_ExcludesResearchMetadata(t *testing.T) {
	for eventType := range researchMetadataEventTypes {
		class, ok := classifyHistoryEvent(eventType)
		require.True(t, ok, eventType)
		assert.True(t, class.Excluded() || class.EntityType == entityTypeBranch,
			"%s is research metadata but the history catalog maps it to %q", eventType, class.EntityType)
	}
	assert.Equal(t, []string{"BranchCreated", "BranchDeleted", "BranchMerged"}, HistoryBranchLifecycleEventTypes())
}

func TestHistoryCatalog_Lookups(t *testing.T) {
	assert.Equal(t, []string{"BranchMergeResumed", "BranchUpdated", "GedcomImported", "SnapshotCreated", "SnapshotDeleted"}, HistoryExcludedEventTypes())
	assert.Equal(t, []string{"ChildLinkedToFamily", "ChildUnlinkedFromFamily", "FamilyCreated", "FamilyDeleted", "FamilyUpdated"}, HistoryEventTypesForEntity("family"))
	assert.Equal(t, []string{"NameAdded", "NameRemoved", "NameUpdated", "PersonCreated", "PersonDeleted", "PersonMerged", "PersonUpdated"}, HistoryEventTypesForEntity("person"))
	assert.Nil(t, HistoryEventTypesForEntity("unknown"))

	entityTypes := HistoryEntityTypes()
	assert.Len(t, entityTypes, len(validEntityTypes))
	for _, entityType := range entityTypes {
		assert.True(t, validEntityTypes[entityType], entityType)
		assert.NotEmpty(t, HistoryEventTypesForEntity(entityType), entityType)
	}
}

// TestGetGlobalHistory_PaginatesOverShownEntries is #739's acceptance test:
// over a log interleaving shown events with excluded ones (snapshot markers, an
// import record, a merge-resume record) and a branch's own edits, every page holds
// exactly min(limit, total-offset) entries, has_more agrees, and the pages
// together are the whole mainline change log — on every backend.
func TestGetGlobalHistory_PaginatesOverShownEntries(t *testing.T) {
	for _, backend := range historyBackends() {
		t.Run(backend.name, func(t *testing.T) {
			ctx := context.Background()
			es, rs := backend.open(t)
			service := NewHistoryService(es, rs)
			branch := domain.BranchID(uuid.New())
			appendOn := func(scope repository.AppendScope, streamType string, streamID uuid.UUID, events ...domain.Event) {
				t.Helper()
				require.NoError(t, es.Append(ctx, streamID, streamType, events, -1, scope))
			}

			var shownPeople []uuid.UUID
			for i := 0; i < 5; i++ {
				person := uuid.New()
				shownPeople = append(shownPeople, person)
				appendOn(repository.MainScope, "Person", person, domain.NewPersonCreated(&domain.Person{ID: person, GivenName: fmt.Sprintf("Person%d", i), Surname: "Shown"}))

				snapshot := uuid.New()
				appendOn(repository.MainScope, "snapshot", snapshot,
					domain.SnapshotCreated{BaseEvent: domain.NewBaseEvent(), SnapshotID: snapshot, Name: "Milestone"},
					domain.SnapshotDeleted{BaseEvent: domain.NewBaseEvent(), SnapshotID: snapshot})
				appendOn(repository.MainScope, "import", uuid.New(), domain.GedcomImported{BaseEvent: domain.NewBaseEvent(), Filename: "tree.ged"})
				appendOn(repository.AppendScope{BranchID: branch}, "branch", branch.UUID(), domain.BranchMergeResumed{BaseEvent: domain.NewBaseEvent(), BranchID: branch.UUID()})
				// A research branch's edit of the same person is not mainline history.
				appendOn(repository.AppendScope{BranchID: branch}, "Person", person, domain.NewPersonUpdated(person, map[string]any{"surname": "Branch"}))
			}
			const shown = 5

			for limit := 1; limit <= shown+1; limit++ {
				var seen []uuid.UUID
				for offset := 0; offset <= shown+1; offset += limit {
					page, err := service.GetGlobalHistory(ctx, GetGlobalHistoryInput{Limit: limit, Offset: offset, ToTime: time.Now().Add(time.Hour)})
					require.NoError(t, err)
					assert.Equal(t, shown, page.TotalCount, "limit %d offset %d: total", limit, offset)
					want := min(limit, max(shown-offset, 0))
					require.Len(t, page.Entries, want, "limit %d offset %d: len(items) == min(limit, total-offset)", limit, offset)
					assert.Equal(t, offset+len(page.Entries) < page.TotalCount, page.HasMore, "limit %d offset %d: has_more", limit, offset)
					for _, e := range page.Entries {
						assert.Equal(t, "person", e.EntityType)
						assert.Equal(t, "created", e.Action)
						seen = append(seen, e.EntityID)
					}
				}
				assert.ElementsMatch(t, shownPeople, seen, "limit %d: the pages together are the whole log", limit)
			}

			// The entity_type filter is applied in the store too.
			filtered, err := service.GetGlobalHistory(ctx, GetGlobalHistoryInput{EventTypes: HistoryEventTypesForEntity("family"), Limit: 10})
			require.NoError(t, err)
			assert.Zero(t, filtered.TotalCount)
			assert.Empty(t, filtered.Entries)
		})
	}
}
