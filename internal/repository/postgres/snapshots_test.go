package postgres_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/cacack/my-family/internal/domain"
	"github.com/cacack/my-family/internal/repository"
	pgstore "github.com/cacack/my-family/internal/repository/postgres"
)

func setupSnapshotStore(t *testing.T) (*pgstore.SnapshotStore, func()) {
	t.Helper()

	db, cleanup := setupPostgres(t)

	store, err := pgstore.NewSnapshotStore(db)
	if err != nil {
		cleanup()
		t.Fatalf("create snapshot store: %v", err)
	}

	return store, cleanup
}

// TestSnapshotStore_CRUD is the PostgreSQL half of the dual-database parity the
// project requires (DB-001): it mirrors the SQLite snapshot store tests.
func TestSnapshotStore_CRUD(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test")
	}

	store, cleanup := setupSnapshotStore(t)
	defer cleanup()

	ctx := context.Background()
	snapshot := &domain.Snapshot{
		ID:          uuid.New(),
		Name:        "Pre-DNA results",
		Description: "before the test came back",
		Position:    42,
		CreatedAt:   time.Now().UTC().Truncate(time.Microsecond),
	}

	if err := store.Create(ctx, snapshot); err != nil {
		t.Fatalf("Create() error = %v", err)
	}

	retrieved, err := store.Get(ctx, snapshot.ID)
	if err != nil {
		t.Fatalf("Get() error = %v", err)
	}
	if retrieved.Name != snapshot.Name || retrieved.Description != snapshot.Description {
		t.Errorf("row = %+v, want %+v", retrieved, snapshot)
	}
	if retrieved.Position != snapshot.Position {
		t.Errorf("Position = %d, want %d", retrieved.Position, snapshot.Position)
	}

	all, err := store.List(ctx, domain.MainBranchID)
	if err != nil {
		t.Fatalf("List() error = %v", err)
	}
	if len(all) != 1 {
		t.Fatalf("List() returned %d snapshots, want 1", len(all))
	}

	if err := store.Delete(ctx, snapshot.ID); err != nil {
		t.Fatalf("Delete() error = %v", err)
	}
	if _, err := store.Get(ctx, snapshot.ID); !errors.Is(err, repository.ErrSnapshotNotFound) {
		t.Errorf("Get() after delete = %v, want ErrSnapshotNotFound", err)
	}
	if err := store.Delete(ctx, snapshot.ID); !errors.Is(err, repository.ErrSnapshotNotFound) {
		t.Errorf("Delete() of a missing row = %v, want ErrSnapshotNotFound", err)
	}
}

// TestSnapshotStore_Upsert covers the projection's write path (issue #624):
// inserting when absent, overwriting when present, so replaying SnapshotCreated
// is idempotent. Mirrors TestSQLiteSnapshotStore_Upsert.
func TestSnapshotStore_Upsert(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test")
	}

	store, cleanup := setupSnapshotStore(t)
	defer cleanup()

	ctx := context.Background()
	snapshot := &domain.Snapshot{
		ID:          uuid.New(),
		Name:        "Pre-DNA results",
		Description: "before",
		Position:    42,
		CreatedAt:   time.Now().UTC().Truncate(time.Microsecond),
	}

	if err := store.Upsert(ctx, snapshot); err != nil {
		t.Fatalf("Upsert() insert error = %v", err)
	}

	retrieved, err := store.Get(ctx, snapshot.ID)
	if err != nil {
		t.Fatalf("Get() error = %v", err)
	}
	if retrieved.Name != snapshot.Name || retrieved.Position != snapshot.Position {
		t.Errorf("row = %+v, want %+v", retrieved, snapshot)
	}

	updated := *snapshot
	updated.Name = "After courthouse trip"
	updated.Description = ""
	updated.Position = 99
	if err := store.Upsert(ctx, &updated); err != nil {
		t.Fatalf("Upsert() overwrite error = %v", err)
	}

	retrieved, err = store.Get(ctx, snapshot.ID)
	if err != nil {
		t.Fatalf("Get() after overwrite error = %v", err)
	}
	if retrieved.Name != "After courthouse trip" || retrieved.Position != 99 {
		t.Errorf("row after overwrite = %+v, want the updated values", retrieved)
	}
	// A cleared description must overwrite as NULL, not linger from the insert.
	if retrieved.Description != "" {
		t.Errorf("description = %q, want it cleared by the overwrite", retrieved.Description)
	}

	all, err := store.List(ctx, domain.MainBranchID)
	if err != nil {
		t.Fatalf("List() error = %v", err)
	}
	if len(all) != 1 {
		t.Errorf("List() returned %d snapshots, want 1", len(all))
	}
}

// TestSnapshotStore_GetMaxPosition is what pins a snapshot (and a new branch's
// base) to the head of the event log.
func TestSnapshotStore_GetMaxPosition(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test")
	}

	db, cleanup := setupPostgres(t)
	defer cleanup()

	// The event store owns the events table GetMaxPosition reads.
	eventStore, err := pgstore.NewEventStore(db)
	if err != nil {
		t.Fatalf("create event store: %v", err)
	}
	store, err := pgstore.NewSnapshotStore(db)
	if err != nil {
		t.Fatalf("create snapshot store: %v", err)
	}

	ctx := context.Background()

	position, err := store.GetMaxPosition(ctx)
	if err != nil {
		t.Fatalf("GetMaxPosition() on an empty log error = %v", err)
	}
	if position != 0 {
		t.Errorf("GetMaxPosition() on an empty log = %d, want 0", position)
	}

	person := domain.NewPerson("Ada", "Lovelace")
	event := domain.NewPersonCreated(person)
	if err := eventStore.Append(ctx, person.ID, "Person", []domain.Event{event}, -1, repository.MainScope); err != nil {
		t.Fatalf("Append() error = %v", err)
	}

	position, err = store.GetMaxPosition(ctx)
	if err != nil {
		t.Fatalf("GetMaxPosition() error = %v", err)
	}
	if position <= 0 {
		t.Errorf("GetMaxPosition() after one append = %d, want it past 0", position)
	}
}

// TestSnapshotStore_BranchScoped: a snapshot keeps its branch, and List answers
// one scope at a time (issue #839).
func TestSnapshotStore_BranchScoped(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test")
	}
	store, cleanup := setupSnapshotStore(t)
	defer cleanup()
	ctx := context.Background()
	branch := domain.BranchID(uuid.New())
	now := time.Now().UTC().Truncate(time.Microsecond)
	mainSnap := &domain.Snapshot{ID: uuid.New(), Name: "Mainline", Position: 5, CreatedAt: now}
	branchSnap := &domain.Snapshot{ID: uuid.New(), BranchID: branch, Name: "On branch", Position: 7, CreatedAt: now.Add(time.Second)}
	if err := store.Create(ctx, mainSnap); err != nil {
		t.Fatalf("Create(main) error = %v", err)
	}
	if err := store.Upsert(ctx, branchSnap); err != nil {
		t.Fatalf("Upsert(branch) error = %v", err)
	}

	got, err := store.Get(ctx, branchSnap.ID)
	if err != nil {
		t.Fatalf("Get() error = %v", err)
	}
	if got.BranchID != branch || got.Position != 7 {
		t.Errorf("Get() = (%v, %d), want (%v, 7)", got.BranchID, got.Position, branch)
	}
	got, err = store.Get(ctx, mainSnap.ID)
	if err != nil {
		t.Fatalf("Get() error = %v", err)
	}
	if !got.BranchID.IsMain() {
		t.Errorf("mainline snapshot BranchID = %v, want main", got.BranchID)
	}

	for _, tc := range []struct {
		scope domain.BranchID
		want  uuid.UUID
	}{{domain.MainBranchID, mainSnap.ID}, {branch, branchSnap.ID}} {
		list, err := store.List(ctx, tc.scope)
		if err != nil {
			t.Fatalf("List(%v) error = %v", tc.scope, err)
		}
		if len(list) != 1 || list[0].ID != tc.want {
			t.Errorf("List(%v) = %v, want only %v", tc.scope, list, tc.want)
		}
	}
	if list, err := store.List(ctx, domain.BranchID(uuid.New())); err != nil || len(list) != 0 {
		t.Errorf("List(unknown branch) = %v, %v; want empty", list, err)
	}

	// An upsert replay keeps the branch.
	branchSnap.Name = "Renamed"
	if err := store.Upsert(ctx, branchSnap); err != nil {
		t.Fatalf("Upsert(replay) error = %v", err)
	}
	if got, err := store.Get(ctx, branchSnap.ID); err != nil || got.BranchID != branch || got.Name != "Renamed" {
		t.Errorf("after replay Get() = %+v, %v", got, err)
	}
}

// TestSnapshotStore_MigratesBranchID: a snapshots table from before #839 gains
// branch_id, and its existing rows become mainline snapshots. Opening the store
// again is a no-op.
func TestSnapshotStore_MigratesBranchID(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test")
	}
	db, cleanup := setupPostgres(t)
	defer cleanup()
	ctx := context.Background()

	if _, err := db.Exec(`
		CREATE TABLE snapshots (
			id UUID PRIMARY KEY,
			name VARCHAR(100) NOT NULL,
			description VARCHAR(500),
			position BIGINT NOT NULL,
			created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
		)`); err != nil {
		t.Fatalf("create legacy table: %v", err)
	}
	legacyID := uuid.New()
	if _, err := db.Exec(`INSERT INTO snapshots (id, name, position) VALUES ($1, $2, $3)`,
		legacyID, "Legacy", 3); err != nil {
		t.Fatalf("insert legacy row: %v", err)
	}

	for range 2 {
		store, err := pgstore.NewSnapshotStore(db)
		if err != nil {
			t.Fatalf("NewSnapshotStore() error = %v", err)
		}
		list, err := store.List(ctx, domain.MainBranchID)
		if err != nil {
			t.Fatalf("List() error = %v", err)
		}
		if len(list) != 1 || list[0].ID != legacyID || !list[0].BranchID.IsMain() {
			t.Fatalf("List(main) = %+v, want the legacy row as a mainline snapshot", list)
		}
	}
}
