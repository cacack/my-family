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

// snapshotBackend opens a fresh event store, read model and snapshot registry
// on one backend.
type snapshotBackend struct {
	name string
	open func(t *testing.T) (repository.EventStore, repository.ReadModelStore, repository.SnapshotStore)
}

func snapshotBackends() []snapshotBackend {
	return []snapshotBackend{
		{name: "memory", open: func(*testing.T) (repository.EventStore, repository.ReadModelStore, repository.SnapshotStore) {
			es := memory.NewEventStore()
			return es, memory.NewReadModelStore(), memory.NewSnapshotStore(es)
		}},
		{name: "sqlite", open: func(t *testing.T) (repository.EventStore, repository.ReadModelStore, repository.SnapshotStore) {
			t.Helper()
			db, err := sql.Open("sqlite3", filepath.Join(t.TempDir(), "snapshots.db")+"?_foreign_keys=on&_busy_timeout=5000")
			require.NoError(t, err)
			db.SetMaxOpenConns(1)
			t.Cleanup(func() { _ = db.Close() })
			es, err := sqlite.NewEventStore(db)
			require.NoError(t, err)
			rs, err := sqlite.NewReadModelStore(db)
			require.NoError(t, err)
			ss, err := sqlite.NewSnapshotStore(db)
			require.NoError(t, err)
			return es, rs, ss
		}},
		{name: "postgres", open: func(t *testing.T) (repository.EventStore, repository.ReadModelStore, repository.SnapshotStore) {
			t.Helper()
			db, err := sql.Open("postgres", createTestPostgresDatabase(t))
			require.NoError(t, err)
			t.Cleanup(func() { _ = db.Close() })
			es, err := postgres.NewEventStore(db)
			require.NoError(t, err)
			rs, err := postgres.NewReadModelStore(db)
			require.NoError(t, err)
			ss, err := postgres.NewSnapshotStore(db)
			require.NoError(t, err)
			return es, rs, ss
		}},
	}
}

// logWriter appends person events on the mainline or a branch, tracking the
// per-(stream, branch) versions the event store expects. A branch's first
// write to a stream seeds from the mainline's version at the branch's base.
type logWriter struct {
	t        *testing.T
	es       repository.EventStore
	base     int64
	versions map[string]int64
	atBase   map[uuid.UUID]int64
}

func (w *logWriter) key(stream uuid.UUID, branch domain.BranchID) string {
	return stream.String() + "/" + branch.String()
}

func (w *logWriter) append(branch domain.BranchID, stream uuid.UUID, event domain.Event) {
	w.t.Helper()
	ctx := context.Background()
	k := w.key(stream, branch)
	expected, seen := w.versions[k]
	switch {
	case seen:
	case branch.IsMain():
		expected = -1
	default:
		expected = w.atBase[stream]
	}
	scope := repository.MainScope
	if !branch.IsMain() {
		scope = repository.AppendScope{BranchID: branch, BasePosition: w.base}
	}
	require.NoError(w.t, w.es.Append(ctx, stream, "Person", []domain.Event{event}, expected, scope))
	if expected < 0 {
		expected = 0
	}
	w.versions[k] = expected + 1
}

func (w *logWriter) update(branch domain.BranchID, stream uuid.UUID, surname string) {
	w.t.Helper()
	w.append(branch, stream, domain.NewPersonUpdated(stream, map[string]any{"surname": surname}))
}

// changeKeys summarizes changes as (entity, action, origin) triples.
func changeKeys(changes []ChangeEntry) [][3]string {
	out := make([][3]string, len(changes))
	for i, c := range changes {
		out[i] = [3]string{c.EntityID.String(), c.Action, c.Origin}
	}
	return out
}

// TestSnapshotComparison_Branch_AllBackends pins the comparison semantics of
// issue #839 on every backend. On a branch the range is read through the
// branch's view (ADR-005, the same definition #824 gave entity history): the
// branch's own events, plus the mainline events on entities the branch had not
// yet written; never another branch's. Snapshots from different scopes are
// refused, and a snapshot compares to the current log head.
func TestSnapshotComparison_Branch_AllBackends(t *testing.T) {
	for _, backend := range snapshotBackends() {
		t.Run(backend.name, func(t *testing.T) {
			ctx := context.Background()
			es, rs, ss := backend.open(t)
			service := NewSnapshotService(ss, es, NewHistoryService(es, rs))
			w := &logWriter{t: t, es: es, versions: map[string]int64{}, atBase: map[uuid.UUID]int64{}}

			newPerson := func(given string) uuid.UUID {
				p := domain.NewPerson(given, "Lovelace")
				w.append(domain.MainBranchID, p.ID, domain.NewPersonCreated(p))
				w.atBase[p.ID] = 1
				return p.ID
			}
			mark := func(branch domain.BranchID, name string) *domain.Snapshot {
				head, err := ss.GetMaxPosition(ctx)
				require.NoError(t, err)
				s, err := domain.NewSnapshotOn(branch, name, "", head)
				require.NoError(t, err)
				require.NoError(t, ss.Upsert(ctx, s))
				return s
			}

			touchedEarly := newPerson("Early") // branch writes it before the first snapshot
			edited := newPerson("Ada")         // branch writes it inside the range
			untouched := newPerson("Bob")      // branch writes it only after the range
			base, err := ss.GetMaxPosition(ctx)
			require.NoError(t, err)
			w.base = base

			branch := domain.BranchID(uuid.New())
			other := domain.BranchID(uuid.New())

			w.update(branch, touchedEarly, "BranchEarly")
			mainBefore := mark(domain.MainBranchID, "Main before")
			s1 := mark(branch, "Branch before")

			w.update(branch, edited, "Byron")                 // own
			w.update(domain.MainBranchID, untouched, "Later") // inherited
			w.update(domain.MainBranchID, edited, "MainEdit") // after the branch's write: not in its view
			w.update(domain.MainBranchID, touchedEarly, "X")  // branch wrote it before s1: not in its view
			w.update(other, untouched, "OtherBranch")         // another branch: never
			created := domain.NewPerson("Cara", "New")
			w.append(domain.MainBranchID, created.ID, domain.NewPersonCreated(created)) // inherited

			s2 := mark(branch, "Branch after")
			mainAfter := mark(domain.MainBranchID, "Main after")

			w.update(branch, untouched, "BranchLate")          // own, after s2
			w.update(domain.MainBranchID, untouched, "Hidden") // after the branch's write

			wantRange := [][3]string{
				{edited.String(), "updated", ChangeOriginBranch},
				{untouched.String(), "updated", ChangeOriginMain},
				{created.ID.String(), "created", ChangeOriginMain},
			}

			res, err := service.CompareSnapshots(ctx, branch, s1.ID, s2.ID)
			require.NoError(t, err)
			assert.Equal(t, wantRange, changeKeys(res.Changes))
			assert.Equal(t, 3, res.TotalCount)
			assert.False(t, res.HasMore)
			assert.True(t, res.OlderFirst)

			// Either order: changes are still oldest first.
			res, err = service.CompareSnapshots(ctx, branch, s2.ID, s1.ID)
			require.NoError(t, err)
			assert.Equal(t, wantRange, changeKeys(res.Changes))
			assert.False(t, res.OlderFirst)

			// Compare to now: the range plus the branch's later write; the
			// mainline edit after that write is not in the branch's view.
			cur, err := service.CompareSnapshotToCurrent(ctx, branch, s1.ID)
			require.NoError(t, err)
			assert.Equal(t, append(append([][3]string{}, wantRange...),
				[3]string{untouched.String(), "updated", ChangeOriginBranch}), changeKeys(cur.Changes))
			head, err := ss.GetMaxPosition(ctx)
			require.NoError(t, err)
			assert.Equal(t, head, cur.HeadPosition)
			assert.Equal(t, s1.ID, cur.Snapshot.ID)

			// The mainline's own view: mainline events only, no origin.
			mres, err := service.CompareSnapshots(ctx, domain.MainBranchID, mainBefore.ID, mainAfter.ID)
			require.NoError(t, err)
			assert.Equal(t, [][3]string{
				{untouched.String(), "updated", ""},
				{edited.String(), "updated", ""},
				{touchedEarly.String(), "updated", ""},
				{created.ID.String(), "created", ""},
			}, changeKeys(mres.Changes))
			mcur, err := service.CompareSnapshotToCurrent(ctx, domain.MainBranchID, mainAfter.ID)
			require.NoError(t, err)
			assert.Equal(t, [][3]string{{untouched.String(), "updated", ""}}, changeKeys(mcur.Changes))

			// A snapshot compared to now with nothing after it has no changes.
			latest := mark(branch, "Latest")
			empty, err := service.CompareSnapshotToCurrent(ctx, branch, latest.ID)
			require.NoError(t, err)
			assert.Empty(t, empty.Changes)

			// Scopes do not mix.
			_, err = service.CompareSnapshots(ctx, domain.MainBranchID, s1.ID, s2.ID)
			assert.ErrorIs(t, err, ErrSnapshotBranchMismatch)
			_, err = service.CompareSnapshots(ctx, branch, mainBefore.ID, s2.ID)
			assert.ErrorIs(t, err, ErrSnapshotBranchMismatch)
			_, err = service.CompareSnapshotToCurrent(ctx, other, s1.ID)
			assert.ErrorIs(t, err, ErrSnapshotBranchMismatch)
			_, err = service.CompareSnapshotToCurrent(ctx, branch, uuid.New())
			assert.ErrorIs(t, err, repository.ErrSnapshotNotFound)

			_, err = service.GetSnapshot(ctx, domain.MainBranchID, s1.ID)
			assert.ErrorIs(t, err, repository.ErrSnapshotNotFound)
			got, err := service.GetSnapshot(ctx, branch, s1.ID)
			require.NoError(t, err)
			assert.Equal(t, branch, got.BranchID)

			branchList, err := service.ListSnapshots(ctx, branch)
			require.NoError(t, err)
			assert.Len(t, branchList, 3)
			mainList, err := service.ListSnapshots(ctx, domain.MainBranchID)
			require.NoError(t, err)
			assert.Len(t, mainList, 2)
		})
	}
}

// TestSnapshotComparison_Branch_Truncation: on a branch the window ends where
// the first of the two capped reads ran out, and a stream the branch wrote many
// times before the range is still recognised as already written (the
// set-based pre-range read pages rather than stopping at its cap).
func TestSnapshotComparison_Branch_Truncation(t *testing.T) {
	ctx := context.Background()
	es := memory.NewEventStore()
	ss := memory.NewSnapshotStore(es)
	service := NewSnapshotService(ss, es, NewHistoryService(es, memory.NewReadModelStore()))
	w := &logWriter{t: t, es: es, versions: map[string]int64{}, atBase: map[uuid.UUID]int64{}}

	busy := domain.NewPerson("Busy", "Branch")
	w.append(domain.MainBranchID, busy.ID, domain.NewPersonCreated(busy))
	w.atBase[busy.ID] = 1
	w.base = 1
	branch := domain.BranchID(uuid.New())

	// More branch writes to one stream before the range than one page holds.
	for i := 0; i < maxComparisonEvents+5; i++ {
		w.update(branch, busy.ID, "early")
	}
	head, err := ss.GetMaxPosition(ctx)
	require.NoError(t, err)
	s1, err := domain.NewSnapshotOn(branch, "Start", "", head)
	require.NoError(t, err)
	require.NoError(t, ss.Upsert(ctx, s1))

	// A mainline edit to that stream is not in the branch's view.
	w.update(domain.MainBranchID, busy.ID, "main-after")
	cur, err := service.CompareSnapshotToCurrent(ctx, branch, s1.ID)
	require.NoError(t, err)
	assert.Empty(t, cur.Changes)
	assert.False(t, cur.HasMore)

	// More branch events in the range than one read holds: truncated.
	for i := 0; i < maxComparisonEvents+5; i++ {
		w.update(branch, busy.ID, "late")
	}
	cur, err = service.CompareSnapshotToCurrent(ctx, branch, s1.ID)
	require.NoError(t, err)
	assert.True(t, cur.HasMore)
	assert.Len(t, cur.Changes, maxComparisonEvents)
}
