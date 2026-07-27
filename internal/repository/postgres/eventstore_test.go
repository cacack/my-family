// Package postgres_test provides integration tests using testcontainers.
package postgres_test

import (
	"context"
	"database/sql"
	"errors"
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

	err = store.Append(ctx, streamID, "Person", []domain.Event{event1}, -1, repository.MainScope)
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

	err = store.Append(ctx, streamID, "Person", []domain.Event{event2}, 1, repository.MainScope)
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

	err = store.Append(ctx, streamID, "Person", []domain.Event{event1}, -1, repository.MainScope)
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

	err = store.Append(ctx, streamID, "Person", []domain.Event{event2}, 0, repository.MainScope)
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
		err := store.Append(ctx, streamID, "Person", []domain.Event{event}, -1, repository.MainScope)
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
	version, err := store.GetStreamVersion(ctx, streamID, domain.MainBranchID)
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
	err = store.Append(ctx, streamID, "Person", []domain.Event{event}, -1, repository.MainScope)
	if err != nil {
		t.Fatalf("append: %v", err)
	}

	// Version should now be 1
	version, err = store.GetStreamVersion(ctx, streamID, domain.MainBranchID)
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

	err = store.Append(ctx, streamID, "Person", []domain.Event{event}, -1, repository.MainScope)
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

	err = store.Append(ctx, streamID, "Person", events, -1, repository.MainScope)
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

func TestEventStore_ReadByStream_EmptyResults(t *testing.T) {
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

	// Query non-existent stream
	page, err := store.ReadByStream(ctx, streamID, domain.MainBranchID, 10, 0)
	if err != nil {
		t.Fatalf("read by stream: %v", err)
	}

	if page.TotalCount != 0 {
		t.Errorf("expected total count 0, got %d", page.TotalCount)
	}
	if len(page.Events) != 0 {
		t.Errorf("expected 0 events, got %d", len(page.Events))
	}
	if page.HasMore {
		t.Errorf("expected HasMore false for empty results")
	}
}

func TestEventStore_ReadByStream_SinglePage(t *testing.T) {
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

	// Create 3 events
	for i := 0; i < 3; i++ {
		event := domain.PersonUpdated{
			BaseEvent: domain.BaseEvent{
				ID:        uuid.New(),
				Timestamp: time.Now().Add(time.Duration(i) * time.Second),
			},
			PersonID: streamID,
			Changes:  map[string]any{"update": i},
		}
		expectedVersion := int64(i)
		if i == 0 {
			expectedVersion = -1 // First event
		}
		err := store.Append(ctx, streamID, "Person", []domain.Event{event}, expectedVersion, repository.MainScope)
		if err != nil {
			t.Fatalf("append event %d: %v", i, err)
		}
		time.Sleep(10 * time.Millisecond) // Ensure different timestamps
	}

	// Read all events in one page
	page, err := store.ReadByStream(ctx, streamID, domain.MainBranchID, 10, 0)
	if err != nil {
		t.Fatalf("read by stream: %v", err)
	}

	if page.TotalCount != 3 {
		t.Errorf("expected total count 3, got %d", page.TotalCount)
	}
	if len(page.Events) != 3 {
		t.Errorf("expected 3 events, got %d", len(page.Events))
	}
	if page.HasMore {
		t.Errorf("expected HasMore false")
	}

	// Verify events are ordered by version ASC
	for i, event := range page.Events {
		expectedVersion := int64(i + 1)
		if event.Version != expectedVersion {
			t.Errorf("event %d: expected version %d, got %d", i, expectedVersion, event.Version)
		}
	}
}

func TestEventStore_ReadByStream_Pagination(t *testing.T) {
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

	// Create 5 events
	for i := 0; i < 5; i++ {
		event := domain.PersonUpdated{
			BaseEvent: domain.BaseEvent{
				ID:        uuid.New(),
				Timestamp: time.Now().Add(time.Duration(i) * time.Second),
			},
			PersonID: streamID,
			Changes:  map[string]any{"update": i},
		}
		expectedVersion := int64(i)
		if i == 0 {
			expectedVersion = -1
		}
		err := store.Append(ctx, streamID, "Person", []domain.Event{event}, expectedVersion, repository.MainScope)
		if err != nil {
			t.Fatalf("append event %d: %v", i, err)
		}
		time.Sleep(10 * time.Millisecond)
	}

	// First page (limit 2, offset 0)
	page1, err := store.ReadByStream(ctx, streamID, domain.MainBranchID, 2, 0)
	if err != nil {
		t.Fatalf("read page 1: %v", err)
	}
	if page1.TotalCount != 5 {
		t.Errorf("page 1: expected total count 5, got %d", page1.TotalCount)
	}
	if len(page1.Events) != 2 {
		t.Errorf("page 1: expected 2 events, got %d", len(page1.Events))
	}
	if !page1.HasMore {
		t.Errorf("page 1: expected HasMore true")
	}
	if page1.Events[0].Version != 1 {
		t.Errorf("page 1: expected first event version 1, got %d", page1.Events[0].Version)
	}

	// Second page (limit 2, offset 2)
	page2, err := store.ReadByStream(ctx, streamID, domain.MainBranchID, 2, 2)
	if err != nil {
		t.Fatalf("read page 2: %v", err)
	}
	if page2.TotalCount != 5 {
		t.Errorf("page 2: expected total count 5, got %d", page2.TotalCount)
	}
	if len(page2.Events) != 2 {
		t.Errorf("page 2: expected 2 events, got %d", len(page2.Events))
	}
	if !page2.HasMore {
		t.Errorf("page 2: expected HasMore true")
	}
	if page2.Events[0].Version != 3 {
		t.Errorf("page 2: expected first event version 3, got %d", page2.Events[0].Version)
	}

	// Third page (limit 2, offset 4)
	page3, err := store.ReadByStream(ctx, streamID, domain.MainBranchID, 2, 4)
	if err != nil {
		t.Fatalf("read page 3: %v", err)
	}
	if page3.TotalCount != 5 {
		t.Errorf("page 3: expected total count 5, got %d", page3.TotalCount)
	}
	if len(page3.Events) != 1 {
		t.Errorf("page 3: expected 1 event, got %d", len(page3.Events))
	}
	if page3.HasMore {
		t.Errorf("page 3: expected HasMore false")
	}
	if page3.Events[0].Version != 5 {
		t.Errorf("page 3: expected first event version 5, got %d", page3.Events[0].Version)
	}
}

func TestEventStore_ReadGlobalByTime_EmptyResults(t *testing.T) {
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
	fromTime := time.Now()
	toTime := fromTime.Add(1 * time.Hour)

	// Query empty time range
	page, err := store.ReadGlobalByTime(ctx, fromTime, toTime, nil, 10, 0)
	if err != nil {
		t.Fatalf("read global by time: %v", err)
	}

	if page.TotalCount != 0 {
		t.Errorf("expected total count 0, got %d", page.TotalCount)
	}
	if len(page.Events) != 0 {
		t.Errorf("expected 0 events, got %d", len(page.Events))
	}
	if page.HasMore {
		t.Errorf("expected HasMore false")
	}
}

func TestEventStore_ReadGlobalByTime_TimeFiltering(t *testing.T) {
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
	baseTime := time.Now()

	// Create events across different times and streams
	events := []struct {
		streamID  uuid.UUID
		eventType string
		offset    time.Duration
	}{
		{uuid.New(), "PersonCreated", 0},
		{uuid.New(), "FamilyCreated", 1 * time.Hour},
		{uuid.New(), "PersonUpdated", 2 * time.Hour},
		{uuid.New(), "FamilyUpdated", 3 * time.Hour},
		{uuid.New(), "PersonDeleted", 4 * time.Hour},
	}

	for i, e := range events {
		var event domain.Event
		timestamp := baseTime.Add(e.offset)
		switch e.eventType {
		case "PersonCreated":
			event = domain.PersonCreated{
				BaseEvent: domain.BaseEvent{ID: uuid.New(), Timestamp: timestamp},
				PersonID:  e.streamID,
				GivenName: "Person",
				Surname:   "Test",
			}
		case "FamilyCreated":
			event = domain.FamilyCreated{
				BaseEvent: domain.BaseEvent{ID: uuid.New(), Timestamp: timestamp},
				FamilyID:  e.streamID,
			}
		case "PersonUpdated":
			event = domain.PersonUpdated{
				BaseEvent: domain.BaseEvent{ID: uuid.New(), Timestamp: timestamp},
				PersonID:  e.streamID,
				Changes:   map[string]any{"update": i},
			}
		case "FamilyUpdated":
			event = domain.FamilyUpdated{
				BaseEvent: domain.BaseEvent{ID: uuid.New(), Timestamp: timestamp},
				FamilyID:  e.streamID,
				Changes:   map[string]any{"update": i},
			}
		case "PersonDeleted":
			event = domain.PersonDeleted{
				BaseEvent: domain.BaseEvent{ID: uuid.New(), Timestamp: timestamp},
				PersonID:  e.streamID,
			}
		}
		err := store.Append(ctx, e.streamID, "Person", []domain.Event{event}, -1, repository.MainScope)
		if err != nil {
			t.Fatalf("append event %d: %v", i, err)
		}
	}

	// Query middle time range (1-3 hours)
	fromTime := baseTime.Add(1 * time.Hour)
	toTime := baseTime.Add(3 * time.Hour)
	page, err := store.ReadGlobalByTime(ctx, fromTime, toTime, nil, 10, 0)
	if err != nil {
		t.Fatalf("read global by time: %v", err)
	}

	if page.TotalCount != 3 {
		t.Errorf("expected total count 3, got %d", page.TotalCount)
	}
	if len(page.Events) != 3 {
		t.Errorf("expected 3 events, got %d", len(page.Events))
	}
	if page.HasMore {
		t.Errorf("expected HasMore false")
	}

	// Verify events are in time order
	expectedTypes := []string{"FamilyCreated", "PersonUpdated", "FamilyUpdated"}
	for i, event := range page.Events {
		if event.EventType != expectedTypes[i] {
			t.Errorf("event %d: expected type %s, got %s", i, expectedTypes[i], event.EventType)
		}
	}
}

func TestEventStore_ReadGlobalByTime_EventTypeFiltering(t *testing.T) {
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
	baseTime := time.Now()

	// Create mixed event types
	events := []struct {
		streamID  uuid.UUID
		eventType string
		offset    time.Duration
	}{
		{uuid.New(), "PersonCreated", 0},
		{uuid.New(), "FamilyCreated", 1 * time.Second},
		{uuid.New(), "PersonUpdated", 2 * time.Second},
		{uuid.New(), "FamilyUpdated", 3 * time.Second},
		{uuid.New(), "PersonCreated", 4 * time.Second},
	}

	for i, e := range events {
		var event domain.Event
		timestamp := baseTime.Add(e.offset)
		switch e.eventType {
		case "PersonCreated":
			event = domain.PersonCreated{
				BaseEvent: domain.BaseEvent{ID: uuid.New(), Timestamp: timestamp},
				PersonID:  e.streamID,
				GivenName: "Person",
				Surname:   "Test",
			}
		case "FamilyCreated":
			event = domain.FamilyCreated{
				BaseEvent: domain.BaseEvent{ID: uuid.New(), Timestamp: timestamp},
				FamilyID:  e.streamID,
			}
		case "PersonUpdated":
			event = domain.PersonUpdated{
				BaseEvent: domain.BaseEvent{ID: uuid.New(), Timestamp: timestamp},
				PersonID:  e.streamID,
				Changes:   map[string]any{"update": i},
			}
		case "FamilyUpdated":
			event = domain.FamilyUpdated{
				BaseEvent: domain.BaseEvent{ID: uuid.New(), Timestamp: timestamp},
				FamilyID:  e.streamID,
				Changes:   map[string]any{"update": i},
			}
		}
		err := store.Append(ctx, e.streamID, "Person", []domain.Event{event}, -1, repository.MainScope)
		if err != nil {
			t.Fatalf("append event %d: %v", i, err)
		}
		time.Sleep(10 * time.Millisecond)
	}

	// Query only PersonCreated events
	fromTime := baseTime.Add(-1 * time.Second)
	toTime := baseTime.Add(5 * time.Second)
	page, err := store.ReadGlobalByTime(ctx, fromTime, toTime, []string{"PersonCreated"}, 10, 0)
	if err != nil {
		t.Fatalf("read global by time: %v", err)
	}

	if page.TotalCount != 2 {
		t.Errorf("expected total count 2, got %d", page.TotalCount)
	}
	if len(page.Events) != 2 {
		t.Errorf("expected 2 events, got %d", len(page.Events))
	}
	for i, event := range page.Events {
		if event.EventType != "PersonCreated" {
			t.Errorf("event %d: expected PersonCreated, got %s", i, event.EventType)
		}
	}
}

func TestEventStore_ReadGlobalByTime_Pagination(t *testing.T) {
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
	baseTime := time.Now()

	// Create 5 events
	for i := 0; i < 5; i++ {
		streamID := uuid.New()
		event := domain.PersonCreated{
			BaseEvent: domain.BaseEvent{
				ID:        uuid.New(),
				Timestamp: baseTime.Add(time.Duration(i) * time.Second),
			},
			PersonID:  streamID,
			GivenName: "Person",
			Surname:   "Test",
		}
		err := store.Append(ctx, streamID, "Person", []domain.Event{event}, -1, repository.MainScope)
		if err != nil {
			t.Fatalf("append event %d: %v", i, err)
		}
		time.Sleep(10 * time.Millisecond)
	}

	fromTime := baseTime.Add(-1 * time.Second)
	toTime := baseTime.Add(10 * time.Second)

	// First page
	page1, err := store.ReadGlobalByTime(ctx, fromTime, toTime, nil, 2, 0)
	if err != nil {
		t.Fatalf("read page 1: %v", err)
	}
	if page1.TotalCount != 5 {
		t.Errorf("page 1: expected total count 5, got %d", page1.TotalCount)
	}
	if len(page1.Events) != 2 {
		t.Errorf("page 1: expected 2 events, got %d", len(page1.Events))
	}
	if !page1.HasMore {
		t.Errorf("page 1: expected HasMore true")
	}

	// Second page
	page2, err := store.ReadGlobalByTime(ctx, fromTime, toTime, nil, 2, 2)
	if err != nil {
		t.Fatalf("read page 2: %v", err)
	}
	if page2.TotalCount != 5 {
		t.Errorf("page 2: expected total count 5, got %d", page2.TotalCount)
	}
	if len(page2.Events) != 2 {
		t.Errorf("page 2: expected 2 events, got %d", len(page2.Events))
	}
	if !page2.HasMore {
		t.Errorf("page 2: expected HasMore true")
	}

	// Third page
	page3, err := store.ReadGlobalByTime(ctx, fromTime, toTime, nil, 2, 4)
	if err != nil {
		t.Fatalf("read page 3: %v", err)
	}
	if page3.TotalCount != 5 {
		t.Errorf("page 3: expected total count 5, got %d", page3.TotalCount)
	}
	if len(page3.Events) != 1 {
		t.Errorf("page 3: expected 1 event, got %d", len(page3.Events))
	}
	if page3.HasMore {
		t.Errorf("page 3: expected HasMore false")
	}
}

func TestEventStore_BranchIDRoundTrip(t *testing.T) {
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
	branchID := domain.BranchID(uuid.New())
	streamID := uuid.New()

	event := domain.PersonCreated{
		BaseEvent: domain.BaseEvent{ID: uuid.New(), Timestamp: time.Now()},
		PersonID:  streamID,
		GivenName: "Jane",
		Surname:   "Roe",
	}

	if err := store.Append(ctx, streamID, "Person", []domain.Event{event}, -1, repository.AppendScope{BranchID: branchID}); err != nil {
		t.Fatalf("Append() failed: %v", err)
	}

	stream, err := store.ReadStream(ctx, streamID)
	if err != nil {
		t.Fatalf("ReadStream() failed: %v", err)
	}
	if len(stream) != 1 {
		t.Fatalf("ReadStream() returned %d events, want 1", len(stream))
	}
	if stream[0].BranchID != branchID {
		t.Errorf("ReadStream BranchID = %v, want %v", stream[0].BranchID, branchID)
	}

	all, err := store.ReadAll(ctx, 0, 10)
	if err != nil {
		t.Fatalf("ReadAll() failed: %v", err)
	}
	if len(all) != 1 {
		t.Fatalf("ReadAll() returned %d events, want 1", len(all))
	}
	if all[0].BranchID != branchID {
		t.Errorf("ReadAll BranchID = %v, want %v", all[0].BranchID, branchID)
	}
}

func TestEventStore_MainBranchDefault(t *testing.T) {
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

	event := domain.PersonCreated{
		BaseEvent: domain.BaseEvent{ID: uuid.New(), Timestamp: time.Now()},
		PersonID:  streamID,
		GivenName: "John",
		Surname:   "Doe",
	}

	if err := store.Append(ctx, streamID, "Person", []domain.Event{event}, -1, repository.MainScope); err != nil {
		t.Fatalf("Append() failed: %v", err)
	}

	stream, err := store.ReadStream(ctx, streamID)
	if err != nil {
		t.Fatalf("ReadStream() failed: %v", err)
	}
	if len(stream) != 1 {
		t.Fatalf("ReadStream() returned %d events, want 1", len(stream))
	}
	if stream[0].BranchID != domain.MainBranchID {
		t.Errorf("BranchID = %v, want MainBranchID %v", stream[0].BranchID, domain.MainBranchID)
	}
}

// TestEventStore_BranchVersioning drives the ADR-005 per-branch versioning
// scenario against postgres.
func TestEventStore_BranchVersioning(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test")
	}

	db, cleanup := setupPostgres(t)
	defer cleanup()

	store, err := pgstore.NewEventStore(db)
	if err != nil {
		t.Fatalf("create event store: %v", err)
	}

	runBranchVersioningScenario(t, store)
}

// runBranchVersioningScenario exercises per-(stream, branch) optimistic versioning
// and ReadBranch (ADR-005). Each backend package carries an identical copy of this
// body (there is no shared test harness in this repo, see branch_scenario_test.go);
// keeping the assertions byte-identical is the DB-001 parity guarantee. Fixtures
// use neutral placeholder names only (public repo -- no real PII).
func runBranchVersioningScenario(t *testing.T, store repository.EventStore) {
	t.Helper()
	ctx := context.Background()

	person := domain.NewPerson("Alex", "Placeholder")
	streamID := person.ID
	branchA := repository.AppendScope{BranchID: domain.BranchID(uuid.New())}
	branchB := repository.AppendScope{BranchID: domain.BranchID(uuid.New())}

	// --- Main reaches version 3. ---
	mainEvents := []domain.Event{
		domain.NewPersonCreated(person),
		domain.NewPersonUpdated(streamID, map[string]any{"birth_place": "Placeville"}),
		domain.NewPersonUpdated(streamID, map[string]any{"notes": "seeded on main"}),
	}
	for i, ev := range mainEvents {
		expected := int64(i) // -1 for the create, then the version it follows
		if i == 0 {
			expected = -1
		}
		if err := store.Append(ctx, streamID, "Person", []domain.Event{ev}, expected, repository.MainScope); err != nil {
			t.Fatalf("main append %d: %v", i, err)
		}
	}
	if v, err := store.GetStreamVersion(ctx, streamID, domain.MainBranchID); err != nil || v != 3 {
		t.Fatalf("main version after seeding = %d (err %v), want 3", v, err)
	}

	// The branches fork from main's tip.
	all, err := store.ReadAll(ctx, 0, 100)
	if err != nil {
		t.Fatalf("ReadAll: %v", err)
	}
	if len(all) != 3 {
		t.Fatalf("ReadAll after seeding returned %d events, want 3", len(all))
	}
	basePosition := all[len(all)-1].Position
	branchA.BasePosition = basePosition
	branchB.BasePosition = basePosition

	// --- Seeding: a branch's first write continues main's version line at 4. ---
	branchEdit := domain.NewPersonUpdated(streamID, map[string]any{"surname": "Revised-A"})
	if err := store.Append(ctx, streamID, "Person", []domain.Event{branchEdit}, 3, branchA); err != nil {
		t.Fatalf("branch A append (seeded from main v3): %v", err)
	}
	if v, err := store.GetStreamVersion(ctx, streamID, branchA.BranchID); err != nil || v != 4 {
		t.Fatalf("branch A version = %d (err %v), want 4", v, err)
	}
	// The branch write did not advance main.
	if v, err := store.GetStreamVersion(ctx, streamID, domain.MainBranchID); err != nil || v != 3 {
		t.Fatalf("main version after branch A write = %d (err %v), want 3 (unchanged)", v, err)
	}

	// --- No cross-branch contention: a second branch writes the SAME stream from
	// the same base without either side seeing ErrConcurrencyConflict. ---
	if err := store.Append(ctx, streamID, "Person",
		[]domain.Event{domain.NewPersonUpdated(streamID, map[string]any{"surname": "Revised-B"})}, 3, branchB); err != nil {
		t.Fatalf("branch B append to the same stream: %v", err)
	}
	if v, err := store.GetStreamVersion(ctx, streamID, branchB.BranchID); err != nil || v != 4 {
		t.Fatalf("branch B version = %d (err %v), want 4", v, err)
	}
	if v, err := store.GetStreamVersion(ctx, streamID, branchA.BranchID); err != nil || v != 4 {
		t.Fatalf("branch A version after branch B write = %d (err %v), want 4 (unchanged)", v, err)
	}

	// --- Main advances afterwards without contending with either branch. ---
	if err := store.Append(ctx, streamID, "Person",
		[]domain.Event{domain.NewPersonUpdated(streamID, map[string]any{"notes": "main moved on"})}, 3, repository.MainScope); err != nil {
		t.Fatalf("main append after branch writes: %v", err)
	}
	if v, err := store.GetStreamVersion(ctx, streamID, domain.MainBranchID); err != nil || v != 4 {
		t.Fatalf("main version after its own 4th write = %d (err %v), want 4", v, err)
	}
	if v, err := store.GetStreamVersion(ctx, streamID, branchA.BranchID); err != nil || v != 4 {
		t.Fatalf("branch A version after main advanced = %d (err %v), want 4 (unchanged)", v, err)
	}

	// --- Optimistic concurrency still bites WITHIN a branch. ---
	err = store.Append(ctx, streamID, "Person",
		[]domain.Event{domain.NewPersonUpdated(streamID, map[string]any{"notes": "stale"})}, 3, branchA)
	if !errors.Is(err, repository.ErrConcurrencyConflict) {
		t.Fatalf("stale branch A append: want ErrConcurrencyConflict, got %v", err)
	}

	// --- expectedVersion -1 means "no prior events for this stream ON THIS BRANCH":
	// an aggregate created on a branch starts its own line at 1. ---
	branchOnly := domain.NewPerson("Sam", "Hypothesis")
	if err := store.Append(ctx, branchOnly.ID, "Person",
		[]domain.Event{domain.NewPersonCreated(branchOnly)}, -1, branchA); err != nil {
		t.Fatalf("branch A create of a new aggregate: %v", err)
	}
	if v, err := store.GetStreamVersion(ctx, branchOnly.ID, branchA.BranchID); err != nil || v != 1 {
		t.Fatalf("branch A version of branch-only aggregate = %d (err %v), want 1", v, err)
	}
	if v, err := store.GetStreamVersion(ctx, branchOnly.ID, domain.MainBranchID); err != nil || v != 0 {
		t.Fatalf("main version of branch-only aggregate = %d (err %v), want 0", v, err)
	}

	// --- ReadBranch returns a branch's OWN events, in position order. ---
	aEvents, err := store.ReadBranch(ctx, branchA.BranchID, 0, 100)
	if err != nil {
		t.Fatalf("ReadBranch(A): %v", err)
	}
	if len(aEvents) != 2 {
		t.Fatalf("ReadBranch(A) returned %d events, want 2 (its own deltas only)", len(aEvents))
	}
	for i, ev := range aEvents {
		if ev.BranchID != branchA.BranchID {
			t.Fatalf("ReadBranch(A) event %d has branch %v, want %v", i, ev.BranchID, branchA.BranchID)
		}
		if i > 0 && ev.Position <= aEvents[i-1].Position {
			t.Fatalf("ReadBranch(A) not ordered by position: %d after %d", ev.Position, aEvents[i-1].Position)
		}
	}
	// fromPosition is exclusive (same convention as ReadAll).
	rest, err := store.ReadBranch(ctx, branchA.BranchID, aEvents[0].Position, 100)
	if err != nil {
		t.Fatalf("ReadBranch(A, from tip of first): %v", err)
	}
	if len(rest) != 1 || rest[0].Position != aEvents[1].Position {
		t.Fatalf("ReadBranch(A) exclusive fromPosition: got %d events, want the 1 after position %d", len(rest), aEvents[0].Position)
	}
	// limit caps the result.
	limited, err := store.ReadBranch(ctx, branchA.BranchID, 0, 1)
	if err != nil {
		t.Fatalf("ReadBranch(A, limit 1): %v", err)
	}
	if len(limited) != 1 {
		t.Fatalf("ReadBranch(A, limit 1) returned %d events, want 1", len(limited))
	}
	// Main's own events are exactly the four main appends -- no branch deltas.
	mainOwn, err := store.ReadBranch(ctx, domain.MainBranchID, 0, 100)
	if err != nil {
		t.Fatalf("ReadBranch(main): %v", err)
	}
	if len(mainOwn) != 4 {
		t.Fatalf("ReadBranch(main) returned %d events, want 4", len(mainOwn))
	}
	for _, ev := range mainOwn {
		if !ev.BranchID.IsMain() {
			t.Fatalf("ReadBranch(main) returned a %v event", ev.BranchID)
		}
	}
}

// TestEventStore_ReadByStream_BranchScoped proves the ADR-005 isolation rule for
// entity history: branch and main share a stream id, so an unfiltered read would
// present a branch's in-progress edits as part of main's audit trail. The filter
// is in the SQL, alongside the COUNT(*) OVER(), so TotalCount and HasMore
// describe the requested branch only.
func TestEventStore_ReadByStream_BranchScoped(t *testing.T) {
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
	branchID := domain.BranchID(uuid.New())

	mainEvents := []domain.Event{
		domain.NewPersonCreated(domain.NewPerson("Main", "Person")),
		domain.NewPersonUpdated(streamID, map[string]any{"notes": "main edit"}),
	}
	if err := store.Append(ctx, streamID, "Person", mainEvents, -1, repository.MainScope); err != nil {
		t.Fatalf("append main: %v", err)
	}

	branchScope := repository.AppendScope{BranchID: branchID, BasePosition: 2}
	branchEvents := []domain.Event{
		domain.NewPersonUpdated(streamID, map[string]any{"notes": "branch edit 1"}),
		domain.NewPersonUpdated(streamID, map[string]any{"notes": "branch edit 2"}),
		domain.NewPersonUpdated(streamID, map[string]any{"notes": "branch edit 3"}),
	}
	if err := store.Append(ctx, streamID, "Person", branchEvents, -1, branchScope); err != nil {
		t.Fatalf("append branch: %v", err)
	}

	mainPage, err := store.ReadByStream(ctx, streamID, domain.MainBranchID, 10, 0)
	if err != nil {
		t.Fatalf("read by stream (main): %v", err)
	}
	if mainPage.TotalCount != 2 {
		t.Errorf("main TotalCount = %d, want 2 (branch events must not be counted)", mainPage.TotalCount)
	}
	if len(mainPage.Events) != 2 {
		t.Fatalf("main len(Events) = %d, want 2", len(mainPage.Events))
	}
	if mainPage.HasMore {
		t.Errorf("main HasMore = true, want false")
	}
	for _, evt := range mainPage.Events {
		if !evt.BranchID.IsMain() {
			t.Errorf("branch event %s leaked into main history", evt.ID)
		}
	}

	branchPage, err := store.ReadByStream(ctx, streamID, branchID, 10, 0)
	if err != nil {
		t.Fatalf("read by stream (branch): %v", err)
	}
	if branchPage.TotalCount != 3 {
		t.Errorf("branch TotalCount = %d, want 3", branchPage.TotalCount)
	}
	for _, evt := range branchPage.Events {
		if evt.BranchID != branchID {
			t.Errorf("event %s on branch page has branch %v, want %v", evt.ID, evt.BranchID, branchID)
		}
	}

	// Pagination is computed over the filtered set: one main event of two left.
	paged, err := store.ReadByStream(ctx, streamID, domain.MainBranchID, 1, 0)
	if err != nil {
		t.Fatalf("read by stream (paged): %v", err)
	}
	if paged.TotalCount != 2 || !paged.HasMore {
		t.Errorf("paged TotalCount/HasMore = %d/%v, want 2/true", paged.TotalCount, paged.HasMore)
	}
}

// TestEventStore_ReadStreamsForBranch covers the set-based accessor branch
// compare relies on: one call for many streams, filtered to a branch and to
// positions after a fork point, ordered by position, capped by SQL.
func TestEventStore_ReadStreamsForBranch(t *testing.T) {
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
	first := uuid.New()
	second := uuid.New()
	unrelated := uuid.New()
	branchID := domain.BranchID(uuid.New())

	seed := func(streamID uuid.UUID, scope repository.AppendScope, note string) {
		t.Helper()
		evt := domain.NewPersonUpdated(streamID, map[string]any{"notes": note})
		if err := store.Append(ctx, streamID, "Person", []domain.Event{evt}, -1, scope); err != nil {
			t.Fatalf("append %s: %v", note, err)
		}
	}

	seed(first, repository.MainScope, "pre-fork")   // position 1
	seed(second, repository.MainScope, "pre-fork")  // position 2
	basePosition := int64(2)                        // the fork point
	seed(second, repository.MainScope, "post-1")    // position 3
	seed(first, repository.MainScope, "post-2")     // position 4
	seed(unrelated, repository.MainScope, "post-3") // position 5
	seed(first, repository.AppendScope{BranchID: branchID, BasePosition: basePosition}, "branch")

	t.Run("empty stream set reads nothing", func(t *testing.T) {
		events, err := store.ReadStreamsForBranch(ctx, nil, domain.MainBranchID, 0, 100)
		if err != nil {
			t.Fatalf("read streams for branch: %v", err)
		}
		if len(events) != 0 {
			t.Errorf("len(events) = %d, want 0", len(events))
		}
	})

	t.Run("scoped to branch, position and stream set", func(t *testing.T) {
		events, err := store.ReadStreamsForBranch(ctx, []uuid.UUID{first, second}, domain.MainBranchID, basePosition, 100)
		if err != nil {
			t.Fatalf("read streams for branch: %v", err)
		}
		if len(events) != 2 {
			t.Fatalf("len(events) = %d, want 2", len(events))
		}
		if events[0].Position != 3 || events[1].Position != 4 {
			t.Errorf("positions = %d,%d, want 3,4", events[0].Position, events[1].Position)
		}
		for _, evt := range events {
			if !evt.BranchID.IsMain() {
				t.Errorf("event %s is not on main", evt.ID)
			}
			if evt.StreamID == unrelated {
				t.Errorf("event from a stream outside the set leaked in")
			}
		}
	})

	t.Run("branch side sees only its own events", func(t *testing.T) {
		events, err := store.ReadStreamsForBranch(ctx, []uuid.UUID{first, second}, branchID, basePosition, 100)
		if err != nil {
			t.Fatalf("read streams for branch: %v", err)
		}
		if len(events) != 1 {
			t.Fatalf("len(events) = %d, want 1", len(events))
		}
		if events[0].BranchID != branchID {
			t.Errorf("BranchID = %v, want %v", events[0].BranchID, branchID)
		}
	})

	t.Run("limit is applied by the store", func(t *testing.T) {
		events, err := store.ReadStreamsForBranch(ctx, []uuid.UUID{first, second}, domain.MainBranchID, basePosition, 1)
		if err != nil {
			t.Fatalf("read streams for branch: %v", err)
		}
		if len(events) != 1 {
			t.Fatalf("len(events) = %d, want 1", len(events))
		}
		if events[0].Position != 3 {
			t.Errorf("Position = %d, want 3 (oldest first)", events[0].Position)
		}
	})
}
