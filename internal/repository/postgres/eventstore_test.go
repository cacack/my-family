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
	page, err := store.ReadByStream(ctx, streamID, 10, 0)
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
		err := store.Append(ctx, streamID, "Person", []domain.Event{event}, expectedVersion)
		if err != nil {
			t.Fatalf("append event %d: %v", i, err)
		}
		time.Sleep(10 * time.Millisecond) // Ensure different timestamps
	}

	// Read all events in one page
	page, err := store.ReadByStream(ctx, streamID, 10, 0)
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
		err := store.Append(ctx, streamID, "Person", []domain.Event{event}, expectedVersion)
		if err != nil {
			t.Fatalf("append event %d: %v", i, err)
		}
		time.Sleep(10 * time.Millisecond)
	}

	// First page (limit 2, offset 0)
	page1, err := store.ReadByStream(ctx, streamID, 2, 0)
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
	page2, err := store.ReadByStream(ctx, streamID, 2, 2)
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
	page3, err := store.ReadByStream(ctx, streamID, 2, 4)
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
		err := store.Append(ctx, e.streamID, "Person", []domain.Event{event}, -1)
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
		err := store.Append(ctx, e.streamID, "Person", []domain.Event{event}, -1)
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
		err := store.Append(ctx, streamID, "Person", []domain.Event{event}, -1)
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
