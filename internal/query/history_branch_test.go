package query

import (
	"context"
	"database/sql"
	"path/filepath"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/cacack/my-family/internal/domain"
	"github.com/cacack/my-family/internal/repository"
	"github.com/cacack/my-family/internal/repository/memory"
	"github.com/cacack/my-family/internal/repository/postgres"
	"github.com/cacack/my-family/internal/repository/sqlite"
)

// historyBackend opens a fresh event store and read model on one backend.
type historyBackend struct {
	name string
	open func(t *testing.T) (repository.EventStore, repository.ReadModelStore)
}

func historyBackends() []historyBackend {
	return []historyBackend{
		{name: "memory", open: func(*testing.T) (repository.EventStore, repository.ReadModelStore) {
			return memory.NewEventStore(), memory.NewReadModelStore()
		}},
		{name: "sqlite", open: openHistorySQLite},
		{name: "postgres", open: openHistoryPostgres},
	}
}

func openHistorySQLite(t *testing.T) (repository.EventStore, repository.ReadModelStore) {
	t.Helper()
	db, err := sql.Open("sqlite3", filepath.Join(t.TempDir(), "history.db")+"?_foreign_keys=on&_busy_timeout=5000")
	require.NoError(t, err)
	db.SetMaxOpenConns(1)
	t.Cleanup(func() { _ = db.Close() })
	es, err := sqlite.NewEventStore(db)
	require.NoError(t, err)
	rs, err := sqlite.NewReadModelStore(db)
	require.NoError(t, err)
	return es, rs
}

func openHistoryPostgres(t *testing.T) (repository.EventStore, repository.ReadModelStore) {
	t.Helper()
	db, err := sql.Open("postgres", createTestPostgresDatabase(t))
	require.NoError(t, err)
	t.Cleanup(func() { _ = db.Close() })
	es, err := postgres.NewEventStore(db)
	require.NoError(t, err)
	rs, err := postgres.NewReadModelStore(db)
	require.NoError(t, err)
	return es, rs
}

// originsOf returns the (action, origin) pairs of entries.
func originsOf(entries []ChangeEntry) [][2]string {
	out := make([][2]string, len(entries))
	for i, e := range entries {
		out[i] = [2]string{e.Action, e.Origin}
	}
	return out
}

// TestGetEntityHistoryOn_AllBackends pins the branch view of an entity's
// history on every event-store backend: mainline events until the branch's
// first write are inherited, the branch's own events follow, and later
// mainline events and other branches' events are excluded.
func TestGetEntityHistoryOn_AllBackends(t *testing.T) {
	for _, backend := range historyBackends() {
		t.Run(backend.name, func(t *testing.T) {
			ctx := context.Background()
			es, rs := backend.open(t)
			service := NewHistoryService(es, rs)

			person := domain.NewPerson("Ada", "Lovelace")
			mainUpdate := func(expected int64, surname string) {
				t.Helper()
				require.NoError(t, es.Append(ctx, person.ID, "Person",
					[]domain.Event{domain.NewPersonUpdated(person.ID, map[string]any{"surname": surname})},
					expected, repository.MainScope))
			}
			require.NoError(t, es.Append(ctx, person.ID, "Person",
				[]domain.Event{domain.NewPersonCreated(person)}, -1, repository.MainScope))
			mainUpdate(1, "King")

			base := int64(0)
			all, err := es.ReadAll(ctx, 0, 100)
			require.NoError(t, err)
			for _, e := range all {
				base = max(base, e.Position)
			}
			branch := domain.BranchID(uuid.New())
			other := domain.BranchID(uuid.New())

			// Untouched by the branch: the whole mainline history is inherited.
			res, err := service.GetEntityHistoryOn(ctx, branch, "person", person.ID, 20, 0)
			require.NoError(t, err)
			assert.Equal(t, [][2]string{{"created", "main"}, {"updated", "main"}}, originsOf(res.Entries))

			// The branch's own edit, another branch's edit, then a mainline edit.
			require.NoError(t, es.Append(ctx, person.ID, "Person",
				[]domain.Event{domain.NewPersonUpdated(person.ID, map[string]any{"surname": "Byron"})},
				2, repository.AppendScope{BranchID: branch}))
			require.NoError(t, es.Append(ctx, person.ID, "Person",
				[]domain.Event{domain.NewPersonUpdated(person.ID, map[string]any{"surname": "Other"})},
				2, repository.AppendScope{BranchID: other}))
			mainUpdate(2, "Later")

			res, err = service.GetEntityHistoryOn(ctx, branch, "person", person.ID, 20, 0)
			require.NoError(t, err)
			assert.Equal(t, [][2]string{{"created", "main"}, {"updated", "main"}, {"updated", "branch"}}, originsOf(res.Entries))
			assert.Equal(t, 3, res.TotalCount)
			assert.False(t, res.HasMore)

			// Paged after the filter.
			res, err = service.GetEntityHistoryOn(ctx, branch, "person", person.ID, 2, 0)
			require.NoError(t, err)
			assert.Len(t, res.Entries, 2)
			assert.Equal(t, 3, res.TotalCount)
			assert.True(t, res.HasMore)
			res, err = service.GetEntityHistoryOn(ctx, branch, "person", person.ID, 2, 2)
			require.NoError(t, err)
			assert.Equal(t, [][2]string{{"updated", "branch"}}, originsOf(res.Entries))
			assert.False(t, res.HasMore)

			// An offset past the end is an empty page, not an error.
			res, err = service.GetEntityHistoryOn(ctx, branch, "person", person.ID, 2, 50)
			require.NoError(t, err)
			assert.Empty(t, res.Entries)
			assert.Equal(t, 3, res.TotalCount)

			// The mainline is GetEntityHistory, with no origin set.
			res, err = service.GetEntityHistoryOn(ctx, domain.MainBranchID, "person", person.ID, 20, 0)
			require.NoError(t, err)
			assert.Equal(t, [][2]string{{"created", ""}, {"updated", ""}, {"updated", ""}}, originsOf(res.Entries))
		})
	}
}

// TestGetEntityHistoryOn_ClampsPaging: limits and offsets are clamped the way
// GetEntityHistory clamps them.
func TestGetEntityHistoryOn_ClampsPaging(t *testing.T) {
	ctx := context.Background()
	es := memory.NewEventStore()
	service := NewHistoryService(es, memory.NewReadModelStore())
	branch := domain.BranchID(uuid.New())
	person := domain.NewPerson("Hypothetical", "Ancestor")
	require.NoError(t, es.Append(ctx, person.ID, "Person",
		[]domain.Event{domain.NewPersonCreated(person)}, -1, repository.AppendScope{BranchID: branch}))

	res, err := service.GetEntityHistoryOn(ctx, branch, "person", person.ID, 0, -3)
	require.NoError(t, err)
	assert.Equal(t, 20, res.Limit)
	assert.Equal(t, 0, res.Offset)
	assert.Equal(t, [][2]string{{"created", "branch"}}, originsOf(res.Entries))

	res, err = service.GetEntityHistoryOn(ctx, branch, "person", person.ID, 500, 0)
	require.NoError(t, err)
	assert.Equal(t, 100, res.Limit)
}

// TestBranchVisibleStreamEvents_OrdersByPosition: ReadStream orders by version,
// which interleaves branches; the filter re-orders by global position.
func TestBranchVisibleStreamEvents_OrdersByPosition(t *testing.T) {
	branch := domain.BranchID(uuid.New())
	other := domain.BranchID(uuid.New())
	events := []repository.StoredEvent{
		{Position: 1, Version: 1, BranchID: domain.MainBranchID},
		{Position: 5, Version: 2, BranchID: branch},
		{Position: 3, Version: 2, BranchID: domain.MainBranchID},
		{Position: 4, Version: 2, BranchID: other},
		{Position: 6, Version: 3, BranchID: domain.MainBranchID},
	}
	got := branchVisibleStreamEvents(events, branch)
	positions := make([]int64, len(got))
	for i, e := range got {
		positions[i] = e.Position
	}
	assert.Equal(t, []int64{1, 3, 5}, positions)
}
