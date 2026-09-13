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

	all, err := store.List(ctx)
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

	all, err := store.List(ctx)
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
