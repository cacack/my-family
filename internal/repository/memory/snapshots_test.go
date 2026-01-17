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
	list, err := store.List(ctx)
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

	list, err := store.List(ctx)
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
	err = eventStore.Append(ctx, streamID, "test", []domain.Event{testEvent}, -1)
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
	err = eventStore.Append(ctx, streamID, "test", []domain.Event{testEvent}, 1)
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
	list, err := store.List(ctx)
	if err != nil {
		t.Fatalf("List() error = %v", err)
	}
	if len(list) != 0 {
		t.Errorf("List() after Reset() returned %d items, want 0", len(list))
	}
}
