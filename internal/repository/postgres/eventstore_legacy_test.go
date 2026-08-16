package postgres_test

import (
	"context"
	"database/sql"
	"errors"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/cacack/my-family/internal/domain"
	"github.com/cacack/my-family/internal/repository"
	pgstore "github.com/cacack/my-family/internal/repository/postgres"
)

// legacyEventLogDDL is the event log's pre-ADR-005 schema: no branch_id, and the
// per-stream UNIQUE(stream_id, version) that createTables must swap for the
// composite unique index.
const legacyEventLogDDL = `
	CREATE TABLE streams (
		id UUID PRIMARY KEY,
		type VARCHAR(50) NOT NULL,
		created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
		metadata JSONB
	);

	CREATE TABLE events (
		id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
		stream_id UUID NOT NULL REFERENCES streams(id),
		stream_type VARCHAR(50) NOT NULL,
		version BIGINT NOT NULL,
		event_type VARCHAR(100) NOT NULL,
		data JSONB NOT NULL,
		metadata JSONB,
		timestamp TIMESTAMPTZ NOT NULL,
		position BIGSERIAL UNIQUE,
		UNIQUE (stream_id, version)
	);

	CREATE INDEX idx_events_stream_version ON events(stream_id, version);
`

// hasColumn reports whether a table in the current schema carries a column.
func hasColumn(t *testing.T, db *sql.DB, table, column string) bool {
	t.Helper()
	var exists bool
	if err := db.QueryRow(`
		SELECT EXISTS (
			SELECT 1 FROM information_schema.columns
			WHERE table_schema = CURRENT_SCHEMA() AND table_name = $1 AND column_name = $2
		)`, table, column).Scan(&exists); err != nil {
		t.Fatalf("inspect %s.%s: %v", table, column, err)
	}
	return exists
}

// seedLegacyReadModelEvents builds a database carrying only the read model's
// pre-#733 events table, with one row, and returns that row's id.
func seedLegacyReadModelEvents(t *testing.T, db *sql.DB) uuid.UUID {
	t.Helper()

	if _, err := db.Exec(legacyEventsDDL); err != nil {
		t.Fatalf("create legacy read model events table: %v", err)
	}

	eventID := uuid.New()
	if _, err := db.Exec(`
		INSERT INTO events (id, owner_type, owner_id, fact_type, date_raw, place, version, created_at)
		VALUES ($1, 'person', $2, $3, '1 JAN 1850', 'Springfield, IL', 1, NOW())
	`, eventID, uuid.New(), string(domain.FactPersonBirth)); err != nil {
		t.Fatalf("insert legacy read model row: %v", err)
	}
	return eventID
}

// TestEventStore_RefusesLegacyReadModelEventsTable is the regression test for the
// guard at the top of createTables (issue #733). Opening the event store against
// a pre-#733 read model table used to add a branch_id column to it before failing
// on the missing stream_id; it must now refuse without touching anything.
func TestEventStore_RefusesLegacyReadModelEventsTable(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test")
	}

	db, cleanup := setupPostgres(t)
	defer cleanup()

	eventID := seedLegacyReadModelEvents(t, db)

	store, err := pgstore.NewEventStore(db)
	if !errors.Is(err, pgstore.ErrReadModelEventsTable) {
		t.Fatalf("expected ErrReadModelEventsTable, got: %v", err)
	}
	if store != nil {
		t.Error("expected a nil store alongside the error")
	}

	// Nothing from the DDL batch may have run.
	if hasColumn(t, db, "events", "branch_id") {
		t.Error("the read model's events table was polluted with a branch_id column")
	}
	if hasColumn(t, db, "events", "stream_id") {
		t.Error("the read model's events table was polluted with event log columns")
	}
	if tableExists(t, db, "streams") {
		t.Error("event log DDL ran despite the guard")
	}

	var count int
	var place string
	if err := db.QueryRow(`SELECT COUNT(*), MAX(place) FROM events WHERE id = $1`, eventID).Scan(&count, &place); err != nil {
		t.Fatalf("read legacy row back: %v", err)
	}
	if count != 1 || place != "Springfield, IL" {
		t.Errorf("legacy read model row damaged: count=%d place=%q", count, place)
	}
}

// TestEventStore_AfterReadModelMigratesLegacyTable is the remedy the refusal
// message prescribes: open the read model first, then the event store.
func TestEventStore_AfterReadModelMigratesLegacyTable(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test")
	}

	db, cleanup := setupPostgres(t)
	defer cleanup()

	lifeEventID := seedLegacyReadModelEvents(t, db)

	readModel, err := pgstore.NewReadModelStore(db)
	if err != nil {
		t.Fatalf("create read model store: %v", err)
	}
	eventStore, err := pgstore.NewEventStore(db)
	if err != nil {
		t.Fatalf("create event store after read model migration: %v", err)
	}

	ctx := context.Background()

	// The migrated life event survived and is still queryable.
	got, err := readModel.GetEvent(ctx, lifeEventID)
	if err != nil {
		t.Fatalf("get migrated life event: %v", err)
	}
	if got.Place != "Springfield, IL" {
		t.Errorf("migrated life event corrupted: %+v", got)
	}

	// And the event log now owns its own events table.
	personID := uuid.New()
	created := domain.PersonCreated{
		BaseEvent: domain.BaseEvent{ID: uuid.New(), Timestamp: time.Now()},
		PersonID:  personID,
		GivenName: "Ada",
		Surname:   "Lovelace",
	}
	if err := eventStore.Append(ctx, personID, "Person", []domain.Event{created}, -1, repository.MainScope); err != nil {
		t.Fatalf("append event: %v", err)
	}
	stream, err := eventStore.ReadStream(ctx, personID)
	if err != nil {
		t.Fatalf("read stream: %v", err)
	}
	if len(stream) != 1 || stream[0].EventType != "PersonCreated" {
		t.Errorf("event log round-trip failed: %+v", stream)
	}
}

// TestEventStore_MigratesLegacyEventLogTable guards the other side of the #733
// check: a genuine pre-ADR-005 event log table (no owner_type) must still be
// migrated in place rather than refused.
func TestEventStore_MigratesLegacyEventLogTable(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test")
	}

	db, cleanup := setupPostgres(t)
	defer cleanup()

	if _, err := db.Exec(legacyEventLogDDL); err != nil {
		t.Fatalf("create legacy event log: %v", err)
	}

	streamID := uuid.New()
	if _, err := db.Exec(`INSERT INTO streams (id, type) VALUES ($1, 'Person')`, streamID); err != nil {
		t.Fatalf("insert legacy stream: %v", err)
	}
	if _, err := db.Exec(`
		INSERT INTO events (id, stream_id, stream_type, version, event_type, data, timestamp)
		VALUES ($1, $2, 'Person', 1, 'PersonCreated', '{"given_name":"Ada"}'::jsonb, NOW())
	`, uuid.New(), streamID); err != nil {
		t.Fatalf("insert legacy event: %v", err)
	}

	eventStore, err := pgstore.NewEventStore(db)
	if err != nil {
		t.Fatalf("create event store on legacy event log: %v", err)
	}

	// branch_id was added and the pre-existing row backfilled to main.
	if !hasColumn(t, db, "events", "branch_id") {
		t.Fatal("expected branch_id column after migration")
	}
	var backfilled int
	if err := db.QueryRow(`SELECT COUNT(*) FROM events WHERE branch_id = $1`, domain.MainBranchID.UUID()).Scan(&backfilled); err != nil {
		t.Fatalf("check branch_id backfill: %v", err)
	}
	if backfilled != 1 {
		t.Errorf("expected 1 row backfilled to main, got %d", backfilled)
	}

	// The per-stream uniqueness was swapped for the composite index.
	var hasOldConstraint bool
	if err := db.QueryRow(`
		SELECT EXISTS (
			SELECT 1 FROM pg_constraint
			WHERE conrelid = 'events'::regclass AND conname = 'events_stream_id_version_key'
		)`).Scan(&hasOldConstraint); err != nil {
		t.Fatalf("inspect constraints: %v", err)
	}
	if hasOldConstraint {
		t.Error("pre-branch UNIQUE(stream_id, version) survived the migration")
	}
	if tableExists(t, db, "idx_events_stream_version") {
		t.Error("superseded idx_events_stream_version survived the migration")
	}
	if !tableExists(t, db, "idx_events_stream_branch_version") {
		t.Error("expected idx_events_stream_branch_version after migration")
	}

	// The migrated log still reads, and accepts new appends after the old row.
	ctx := context.Background()
	stream, err := eventStore.ReadStream(ctx, streamID)
	if err != nil {
		t.Fatalf("read migrated stream: %v", err)
	}
	if len(stream) != 1 || stream[0].BranchID != domain.MainBranchID {
		t.Fatalf("migrated event lost its identity: %+v", stream)
	}

	next := domain.PersonCreated{
		BaseEvent: domain.BaseEvent{ID: uuid.New(), Timestamp: time.Now()},
		PersonID:  streamID,
		GivenName: "Grace",
	}
	if err := eventStore.Append(ctx, streamID, "Person", []domain.Event{next}, 1, repository.MainScope); err != nil {
		t.Fatalf("append onto migrated stream: %v", err)
	}
}
