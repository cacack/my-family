// Package postgres_test provides integration tests using testcontainers.
package postgres_test

import (
	"context"
	"database/sql"
	"os/exec"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/testcontainers/testcontainers-go"
	"github.com/testcontainers/testcontainers-go/modules/postgres"
	"github.com/testcontainers/testcontainers-go/wait"

	"github.com/cacack/my-family/internal/domain"
	"github.com/cacack/my-family/internal/repository"
	pgstore "github.com/cacack/my-family/internal/repository/postgres"
)

// isDockerAvailable checks if Docker is available and running.
func isDockerAvailable() bool {
	cmd := exec.Command("docker", "info")
	return cmd.Run() == nil
}

// setupPostgres creates a PostgreSQL testcontainer and returns a connected database.
func setupPostgres(t *testing.T) (*sql.DB, func()) {
	t.Helper()

	if !isDockerAvailable() {
		t.Skip("Docker is not available, skipping PostgreSQL integration test")
	}

	ctx := context.Background()

	container, err := postgres.Run(ctx,
		"postgres:16-alpine",
		postgres.WithDatabase("testdb"),
		postgres.WithUsername("test"),
		postgres.WithPassword("test"),
		testcontainers.WithWaitStrategy(
			wait.ForLog("database system is ready to accept connections").
				WithOccurrence(2).
				WithStartupTimeout(60*time.Second)),
	)
	if err != nil {
		t.Fatalf("failed to start postgres container: %v", err)
	}

	connStr, err := container.ConnectionString(ctx, "sslmode=disable")
	if err != nil {
		container.Terminate(ctx)
		t.Fatalf("failed to get connection string: %v", err)
	}

	db, err := sql.Open("postgres", connStr)
	if err != nil {
		container.Terminate(ctx)
		t.Fatalf("failed to connect to postgres: %v", err)
	}

	// Wait for database to be ready
	for i := 0; i < 30; i++ {
		if err := db.Ping(); err == nil {
			break
		}
		time.Sleep(100 * time.Millisecond)
	}

	cleanup := func() {
		db.Close()
		container.Terminate(ctx)
	}

	return db, cleanup
}

func TestEventStore_AppendAndRead(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test")
	}

	db, cleanup := setupPostgres(t)
	defer cleanup()

	store, err := pgstore.NewEventStore(db)
	if err != nil {
		t.Fatalf("create event store: %v", err)
	}

	ctx := context.Background()
	streamID := uuid.New()

	// Append first event
	event1 := domain.PersonCreated{
		BaseEvent: domain.BaseEvent{
			ID:        uuid.New(),
			Timestamp: time.Now(),
		},
		PersonID:  streamID,
		GivenName: "John",
		Surname:   "Doe",
	}

	err = store.Append(ctx, streamID, "Person", []domain.Event{event1}, -1)
	if err != nil {
		t.Fatalf("append first event: %v", err)
	}

	// Read stream
	events, err := store.ReadStream(ctx, streamID)
	if err != nil {
		t.Fatalf("read stream: %v", err)
	}

	if len(events) != 1 {
		t.Fatalf("expected 1 event, got %d", len(events))
	}

	if events[0].EventType != "PersonCreated" {
		t.Errorf("expected PersonCreated, got %s", events[0].EventType)
	}
	if events[0].Version != 1 {
		t.Errorf("expected version 1, got %d", events[0].Version)
	}

	// Append second event
	event2 := domain.PersonUpdated{
		BaseEvent: domain.BaseEvent{
			ID:        uuid.New(),
			Timestamp: time.Now(),
		},
		PersonID: streamID,
		Changes:  map[string]any{"given_name": "Jane"},
	}

	err = store.Append(ctx, streamID, "Person", []domain.Event{event2}, 1)
	if err != nil {
		t.Fatalf("append second event: %v", err)
	}

	// Read stream again
	events, err = store.ReadStream(ctx, streamID)
	if err != nil {
		t.Fatalf("read stream: %v", err)
	}

	if len(events) != 2 {
		t.Fatalf("expected 2 events, got %d", len(events))
	}

	if events[1].EventType != "PersonUpdated" {
		t.Errorf("expected PersonUpdated, got %s", events[1].EventType)
	}
	if events[1].Version != 2 {
		t.Errorf("expected version 2, got %d", events[1].Version)
	}
}

func TestEventStore_ConcurrencyConflict(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test")
	}

	db, cleanup := setupPostgres(t)
	defer cleanup()

	store, err := pgstore.NewEventStore(db)
	if err != nil {
		t.Fatalf("create event store: %v", err)
	}

	ctx := context.Background()
	streamID := uuid.New()

	// Append first event
	event1 := domain.PersonCreated{
		BaseEvent: domain.BaseEvent{
			ID:        uuid.New(),
			Timestamp: time.Now(),
		},
		PersonID:  streamID,
		GivenName: "John",
		Surname:   "Doe",
	}

	err = store.Append(ctx, streamID, "Person", []domain.Event{event1}, -1)
	if err != nil {
		t.Fatalf("append first event: %v", err)
	}

	// Try to append with wrong version
	event2 := domain.PersonUpdated{
		BaseEvent: domain.BaseEvent{
			ID:        uuid.New(),
			Timestamp: time.Now(),
		},
		PersonID: streamID,
		Changes:  map[string]any{"given_name": "Jane"},
	}

	err = store.Append(ctx, streamID, "Person", []domain.Event{event2}, 0)
	if err != repository.ErrConcurrencyConflict {
		t.Errorf("expected ErrConcurrencyConflict, got %v", err)
	}
}

func TestEventStore_ReadAll(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test")
	}

	db, cleanup := setupPostgres(t)
	defer cleanup()

	store, err := pgstore.NewEventStore(db)
	if err != nil {
		t.Fatalf("create event store: %v", err)
	}

	ctx := context.Background()

	// Create multiple streams with events
	for i := 0; i < 3; i++ {
		streamID := uuid.New()
		event := domain.PersonCreated{
			BaseEvent: domain.BaseEvent{
				ID:        uuid.New(),
				Timestamp: time.Now(),
			},
			PersonID:  streamID,
			GivenName: "Person",
			Surname:   "Test",
		}
		err := store.Append(ctx, streamID, "Person", []domain.Event{event}, -1)
		if err != nil {
			t.Fatalf("append event %d: %v", i, err)
		}
	}

	// Read all events
	events, err := store.ReadAll(ctx, 0, 10)
	if err != nil {
		t.Fatalf("read all: %v", err)
	}

	if len(events) != 3 {
		t.Errorf("expected 3 events, got %d", len(events))
	}

	// Verify positions are sequential
	for i, e := range events {
		expectedPosition := int64(i + 1)
		if e.Position != expectedPosition {
			t.Errorf("event %d: expected position %d, got %d", i, expectedPosition, e.Position)
		}
	}
}

func TestEventStore_GetStreamVersion(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test")
	}

	db, cleanup := setupPostgres(t)
	defer cleanup()

	store, err := pgstore.NewEventStore(db)
	if err != nil {
		t.Fatalf("create event store: %v", err)
	}

	ctx := context.Background()
	streamID := uuid.New()

	// Non-existent stream should return 0
	version, err := store.GetStreamVersion(ctx, streamID)
	if err != nil {
		t.Fatalf("get version: %v", err)
	}
	if version != 0 {
		t.Errorf("expected version 0 for non-existent stream, got %d", version)
	}

	// Append event
	event := domain.PersonCreated{
		BaseEvent: domain.BaseEvent{
			ID:        uuid.New(),
			Timestamp: time.Now(),
		},
		PersonID:  streamID,
		GivenName: "John",
		Surname:   "Doe",
	}
	err = store.Append(ctx, streamID, "Person", []domain.Event{event}, -1)
	if err != nil {
		t.Fatalf("append: %v", err)
	}

	// Version should now be 1
	version, err = store.GetStreamVersion(ctx, streamID)
	if err != nil {
		t.Fatalf("get version: %v", err)
	}
	if version != 1 {
		t.Errorf("expected version 1, got %d", version)
	}
}

func TestEventStore_DecodeEvents(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test")
	}

	db, cleanup := setupPostgres(t)
	defer cleanup()

	store, err := pgstore.NewEventStore(db)
	if err != nil {
		t.Fatalf("create event store: %v", err)
	}

	ctx := context.Background()
	streamID := uuid.New()

	// Create a person with all fields
	birthDate := domain.ParseGenDate("1 JAN 1850")
	event := domain.PersonCreated{
		BaseEvent: domain.BaseEvent{
			ID:        uuid.New(),
			Timestamp: time.Now(),
		},
		PersonID:   streamID,
		GivenName:  "John",
		Surname:    "Doe",
		Gender:     domain.GenderMale,
		BirthDate:  &birthDate,
		BirthPlace: "Springfield, IL, USA",
	}

	err = store.Append(ctx, streamID, "Person", []domain.Event{event}, -1)
	if err != nil {
		t.Fatalf("append: %v", err)
	}

	// Read and decode
	events, err := store.ReadStream(ctx, streamID)
	if err != nil {
		t.Fatalf("read stream: %v", err)
	}

	decoded, err := events[0].DecodeEvent()
	if err != nil {
		t.Fatalf("decode event: %v", err)
	}

	personCreated, ok := decoded.(domain.PersonCreated)
	if !ok {
		t.Fatalf("expected PersonCreated, got %T", decoded)
	}

	if personCreated.GivenName != "John" {
		t.Errorf("expected GivenName John, got %s", personCreated.GivenName)
	}
	if personCreated.Surname != "Doe" {
		t.Errorf("expected Surname Doe, got %s", personCreated.Surname)
	}
	if personCreated.Gender != domain.GenderMale {
		t.Errorf("expected Gender male, got %s", personCreated.Gender)
	}
	if personCreated.BirthPlace != "Springfield, IL, USA" {
		t.Errorf("expected BirthPlace Springfield, IL, USA, got %s", personCreated.BirthPlace)
	}
}

func TestEventStore_MultipleEventsInBatch(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test")
	}

	db, cleanup := setupPostgres(t)
	defer cleanup()

	store, err := pgstore.NewEventStore(db)
	if err != nil {
		t.Fatalf("create event store: %v", err)
	}

	ctx := context.Background()
	streamID := uuid.New()

	// Append multiple events in one call
	events := []domain.Event{
		domain.PersonCreated{
			BaseEvent: domain.BaseEvent{ID: uuid.New(), Timestamp: time.Now()},
			PersonID:  streamID,
			GivenName: "John",
			Surname:   "Doe",
		},
		domain.PersonUpdated{
			BaseEvent: domain.BaseEvent{ID: uuid.New(), Timestamp: time.Now()},
			PersonID:  streamID,
			Changes:   map[string]any{"notes": "First update"},
		},
		domain.PersonUpdated{
			BaseEvent: domain.BaseEvent{ID: uuid.New(), Timestamp: time.Now()},
			PersonID:  streamID,
			Changes:   map[string]any{"notes": "Second update"},
		},
	}

	err = store.Append(ctx, streamID, "Person", events, -1)
	if err != nil {
		t.Fatalf("append batch: %v", err)
	}

	// Read and verify
	storedEvents, err := store.ReadStream(ctx, streamID)
	if err != nil {
		t.Fatalf("read stream: %v", err)
	}

	if len(storedEvents) != 3 {
		t.Fatalf("expected 3 events, got %d", len(storedEvents))
	}

	// Verify versions
	for i, e := range storedEvents {
		expectedVersion := int64(i + 1)
		if e.Version != expectedVersion {
			t.Errorf("event %d: expected version %d, got %d", i, expectedVersion, e.Version)
		}
	}
}
