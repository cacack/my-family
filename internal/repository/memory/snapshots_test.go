package memory_test

import (
	"context"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/cacack/my-family/internal/domain"
	"github.com/cacack/my-family/internal/repository"
	"github.com/cacack/my-family/internal/repository/memory"
)

func TestSnapshotStore_Create(t *testing.T) {
	eventStore := memory.NewEventStore()
	store := memory.NewSnapshotStore(eventStore)
	ctx := context.Background()

	snapshot := &domain.Snapshot{
		ID:          uuid.New(),
		Name:        "Test Snapshot",
		Description: "Test description",
		Position:    42,
		CreatedAt:   time.Now().UTC(),
	}

	err := store.Create(ctx, snapshot)
	if err != nil {
		t.Fatalf("Create() error = %v", err)
	}

	// Verify we can retrieve it
	retrieved, err := store.Get(ctx, snapshot.ID)
	if err != nil {
		t.Fatalf("Get() error = %v", err)
	}

	if retrieved.ID != snapshot.ID {
		t.Errorf("ID = %v, want %v", retrieved.ID, snapshot.ID)
	}
	if retrieved.Name != snapshot.Name {
		t.Errorf("Name = %v, want %v", retrieved.Name, snapshot.Name)
	}
	if retrieved.Description != snapshot.Description {
		t.Errorf("Description = %v, want %v", retrieved.Description, snapshot.Description)
	}
	if retrieved.Position != snapshot.Position {
		t.Errorf("Position = %v, want %v", retrieved.Position, snapshot.Position)
	}
}

func TestSnapshotStore_Get_NotFound(t *testing.T) {
	eventStore := memory.NewEventStore()
	store := memory.NewSnapshotStore(eventStore)
	ctx := context.Background()

	_, err := store.Get(ctx, uuid.New())
	if err != repository.ErrSnapshotNotFound {
		t.Errorf("Get() error = %v, want %v", err, repository.ErrSnapshotNotFound)
	}
}

func TestSnapshotStore_List(t *testing.T) {
	eventStore := memory.NewEventStore()
	store := memory.NewSnapshotStore(eventStore)
	ctx := context.Background()

	// Create multiple snapshots with different times
	snapshots := []*domain.Snapshot{
		{
			ID:        uuid.New(),
			Name:      "First",
			Position:  1,
			CreatedAt: time.Now().UTC().Add(-2 * time.Hour),
		},
		{
			ID:        uuid.New(),
			Name:      "Second",
			Position:  2,
			CreatedAt: time.Now().UTC().Add(-1 * time.Hour),
		},
		{
			ID:        uuid.New(),
			Name:      "Third",
			Position:  3,
			CreatedAt: time.Now().UTC(),
		},
	}

	for _, s := range snapshots {
		if err := store.Create(ctx, s); err != nil {
			t.Fatalf("Create() error = %v", err)
		}
	}

	// List should return snapshots ordered by created_at DESC
	list, err := store.List(ctx, domain.MainBranchID)
	if err != nil {
		t.Fatalf("List() error = %v", err)
	}

	if len(list) != 3 {
		t.Fatalf("List() returned %d items, want 3", len(list))
	}

	// Verify order (newest first)
	if list[0].Name != "Third" {
		t.Errorf("First item Name = %v, want Third", list[0].Name)
	}
	if list[1].Name != "Second" {
		t.Errorf("Second item Name = %v, want Second", list[1].Name)
	}
	if list[2].Name != "First" {
		t.Errorf("Third item Name = %v, want First", list[2].Name)
	}
}

func TestSnapshotStore_List_Empty(t *testing.T) {
	eventStore := memory.NewEventStore()
	store := memory.NewSnapshotStore(eventStore)
	ctx := context.Background()

	list, err := store.List(ctx, domain.MainBranchID)
	if err != nil {
		t.Fatalf("List() error = %v", err)
	}

	if list == nil {
		t.Error("List() returned nil, want empty slice")
	}
	if len(list) != 0 {
		t.Errorf("List() returned %d items, want 0", len(list))
	}
}

func TestSnapshotStore_Delete(t *testing.T) {
	eventStore := memory.NewEventStore()
	store := memory.NewSnapshotStore(eventStore)
	ctx := context.Background()

	snapshot := &domain.Snapshot{
		ID:        uuid.New(),
		Name:      "To Delete",
		Position:  1,
		CreatedAt: time.Now().UTC(),
	}

	if err := store.Create(ctx, snapshot); err != nil {
		t.Fatalf("Create() error = %v", err)
	}

	// Delete the snapshot
	err := store.Delete(ctx, snapshot.ID)
	if err != nil {
		t.Fatalf("Delete() error = %v", err)
	}

	// Verify it's gone
	_, err = store.Get(ctx, snapshot.ID)
	if err != repository.ErrSnapshotNotFound {
		t.Errorf("Get() after delete error = %v, want %v", err, repository.ErrSnapshotNotFound)
	}
}

func TestSnapshotStore_Delete_NotFound(t *testing.T) {
	eventStore := memory.NewEventStore()
	store := memory.NewSnapshotStore(eventStore)
	ctx := context.Background()

	err := store.Delete(ctx, uuid.New())
	if err != repository.ErrSnapshotNotFound {
		t.Errorf("Delete() error = %v, want %v", err, repository.ErrSnapshotNotFound)
	}
}

func TestSnapshotStore_GetMaxPosition(t *testing.T) {
	eventStore := memory.NewEventStore()
	store := memory.NewSnapshotStore(eventStore)
	ctx := context.Background()

	// Initially should be 0
	pos, err := store.GetMaxPosition(ctx)
	if err != nil {
		t.Fatalf("GetMaxPosition() error = %v", err)
	}
	if pos != 0 {
		t.Errorf("GetMaxPosition() = %d, want 0", pos)
	}

	// Append an event to the event store
	testEvent := &testDomainEvent{
		id:   uuid.New(),
		time: time.Now(),
	}
	streamID := uuid.New()
	err = eventStore.Append(ctx, streamID, "test", []domain.Event{testEvent}, -1, repository.MainScope)
	if err != nil {
		t.Fatalf("Append() error = %v", err)
	}

	// Now max position should be 1
	pos, err = store.GetMaxPosition(ctx)
	if err != nil {
		t.Fatalf("GetMaxPosition() error = %v", err)
	}
	if pos != 1 {
		t.Errorf("GetMaxPosition() = %d, want 1", pos)
	}

	// Append another event
	err = eventStore.Append(ctx, streamID, "test", []domain.Event{testEvent}, 1, repository.MainScope)
	if err != nil {
		t.Fatalf("Append() error = %v", err)
	}

	// Now max position should be 2
	pos, err = store.GetMaxPosition(ctx)
	if err != nil {
		t.Fatalf("GetMaxPosition() error = %v", err)
	}
	if pos != 2 {
		t.Errorf("GetMaxPosition() = %d, want 2", pos)
	}
}

// testDomainEvent is a minimal domain event for testing
type testDomainEvent struct {
	id   uuid.UUID
	time time.Time
}

func (e *testDomainEvent) EventType() string      { return "TestEvent" }
func (e *testDomainEvent) AggregateID() uuid.UUID { return e.id }
func (e *testDomainEvent) OccurredAt() time.Time  { return e.time }

func TestSnapshotStore_Reset(t *testing.T) {
	eventStore := memory.NewEventStore()
	store := memory.NewSnapshotStore(eventStore)
	ctx := context.Background()

	// Create a snapshot
	snapshot := &domain.Snapshot{
		ID:        uuid.New(),
		Name:      "Test",
		Position:  1,
		CreatedAt: time.Now().UTC(),
	}
	if err := store.Create(ctx, snapshot); err != nil {
		t.Fatalf("Create() error = %v", err)
	}

	// Reset
	store.Reset()

	// Verify it's gone
	list, err := store.List(ctx, domain.MainBranchID)
	if err != nil {
		t.Fatalf("List() error = %v", err)
	}
	if len(list) != 0 {
		t.Errorf("List() after Reset() returned %d items, want 0", len(list))
	}
}

// TestSnapshotStore_Upsert covers the projection's write path (issue #624):
// inserting when absent, overwriting when present, so replaying SnapshotCreated
// is idempotent.
func TestSnapshotStore_Upsert(t *testing.T) {
	store := memory.NewSnapshotStore(memory.NewEventStore())
	ctx := context.Background()

	snapshot := &domain.Snapshot{
		ID:          uuid.New(),
		Name:        "Pre-DNA results",
		Description: "before",
		Position:    42,
		CreatedAt:   time.Now().UTC(),
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

	// Second Upsert of the same ID overwrites rather than failing or duplicating.
	updated := *snapshot
	updated.Name = "After courthouse trip"
	updated.Description = "after"
	updated.Position = 99
	if err := store.Upsert(ctx, &updated); err != nil {
		t.Fatalf("Upsert() overwrite error = %v", err)
	}

	retrieved, err = store.Get(ctx, snapshot.ID)
	if err != nil {
		t.Fatalf("Get() after overwrite error = %v", err)
	}
	if retrieved.Name != "After courthouse trip" || retrieved.Description != "after" || retrieved.Position != 99 {
		t.Errorf("row after overwrite = %+v, want the updated values", retrieved)
	}

	all, err := store.List(ctx, domain.MainBranchID)
	if err != nil {
		t.Fatalf("List() error = %v", err)
	}
	if len(all) != 1 {
		t.Errorf("List() returned %d snapshots, want 1", len(all))
	}
}

// TestSnapshotStore_UpsertDoesNotAliasCaller guards the copy-on-write the other
// memory-store methods make: mutating the argument afterwards must not change
// the stored row.
func TestSnapshotStore_UpsertDoesNotAliasCaller(t *testing.T) {
	store := memory.NewSnapshotStore(memory.NewEventStore())
	ctx := context.Background()

	snapshot := &domain.Snapshot{
		ID:        uuid.New(),
		Name:      "original",
		Position:  1,
		CreatedAt: time.Now().UTC(),
	}
	if err := store.Upsert(ctx, snapshot); err != nil {
		t.Fatalf("Upsert() error = %v", err)
	}

	snapshot.Name = "mutated after the write"

	retrieved, err := store.Get(ctx, snapshot.ID)
	if err != nil {
		t.Fatalf("Get() error = %v", err)
	}
	if retrieved.Name != "original" {
		t.Errorf("stored name = %q, want it unaffected by the caller's mutation", retrieved.Name)
	}
}

// TestSnapshotStore_BranchScoped: a snapshot keeps its branch, and List answers
// one scope at a time (issue #839).
func TestSnapshotStore_BranchScoped(t *testing.T) {
	store := memory.NewSnapshotStore(memory.NewEventStore())
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
