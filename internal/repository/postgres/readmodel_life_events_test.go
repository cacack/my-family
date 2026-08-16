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

// legacyEventsDDL is the read model's pre-#733 events table, verbatim enough to
// exercise the rename: owner_type is the column the migration guard keys on, and
// id UUID PRIMARY KEY is what produces the events_pkey that must be renamed.
const legacyEventsDDL = `
	CREATE TABLE events (
		id UUID PRIMARY KEY,
		owner_type VARCHAR(10) NOT NULL,
		owner_id UUID NOT NULL,
		fact_type VARCHAR(100) NOT NULL,
		date_raw VARCHAR(100),
		date_sort DATE,
		place VARCHAR(255),
		place_lat VARCHAR(20),
		place_long VARCHAR(20),
		address JSONB,
		description TEXT,
		cause TEXT,
		age VARCHAR(50),
		research_status VARCHAR(20),
		is_negated BOOLEAN NOT NULL DEFAULT FALSE,
		version BIGINT NOT NULL DEFAULT 1,
		created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
	);
	CREATE INDEX idx_events_owner ON events(owner_type, owner_id);
	CREATE INDEX idx_events_fact_type ON events(fact_type);
`

// tableExists reports whether a relation of the given name is visible in the
// current schema.
func tableExists(t *testing.T, db *sql.DB, name string) bool {
	t.Helper()
	var exists bool
	if err := db.QueryRow(`SELECT to_regclass($1) IS NOT NULL`, name).Scan(&exists); err != nil {
		t.Fatalf("check table %s: %v", name, err)
	}
	return exists
}

// primaryKeyName returns the primary key constraint name for a table.
func primaryKeyName(t *testing.T, db *sql.DB, table string) string {
	t.Helper()
	var name string
	err := db.QueryRow(`SELECT conname FROM pg_constraint WHERE conrelid = $1::regclass AND contype = 'p'`, table).Scan(&name)
	if err != nil {
		t.Fatalf("resolve primary key of %s: %v", table, err)
	}
	return name
}

// TestReadModelStore_MigratesLegacyEventsTable covers the in-place upgrade of a
// database created before the events → life_events rename (issue #733).
func TestReadModelStore_MigratesLegacyEventsTable(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test")
	}

	db, cleanup := setupPostgres(t)
	defer cleanup()

	if _, err := db.Exec(legacyEventsDDL); err != nil {
		t.Fatalf("create legacy events table: %v", err)
	}

	eventID := uuid.New()
	ownerID := uuid.New()
	if _, err := db.Exec(`
		INSERT INTO events (id, owner_type, owner_id, fact_type, date_raw, place, version, created_at)
		VALUES ($1, 'person', $2, $3, '1 JAN 1850', 'Springfield, IL', 1, NOW())
	`, eventID, ownerID, string(domain.FactPersonBirth)); err != nil {
		t.Fatalf("insert legacy row: %v", err)
	}

	store, err := pgstore.NewReadModelStore(db)
	if err != nil {
		t.Fatalf("create read model store: %v", err)
	}

	if !tableExists(t, db, "life_events") {
		t.Fatal("expected life_events table after migration")
	}
	if tableExists(t, db, "events") {
		t.Fatal("expected legacy events table to be gone after migration")
	}
	if pk := primaryKeyName(t, db, "life_events"); pk != "life_events_pkey" {
		t.Errorf("expected primary key life_events_pkey, got %s", pk)
	}

	ctx := context.Background()
	got, err := store.GetEvent(ctx, eventID)
	if err != nil {
		t.Fatalf("get migrated event: %v", err)
	}
	if got.Place != "Springfield, IL" || got.OwnerID != ownerID {
		t.Errorf("migrated row corrupted: %+v", got)
	}

	// Reopening must be a no-op that preserves the row.
	store2, err := pgstore.NewReadModelStore(db)
	if err != nil {
		t.Fatalf("reopen read model store: %v", err)
	}
	events, total, err := store2.ListEvents(ctx, repository.ListOptions{Limit: 10})
	if err != nil {
		t.Fatalf("list events after reopen: %v", err)
	}
	if total != 1 || len(events) != 1 || events[0].ID != eventID {
		t.Errorf("expected the migrated row to survive reopen, got total=%d rows=%d", total, len(events))
	}
	if pk := primaryKeyName(t, db, "life_events"); pk != "life_events_pkey" {
		t.Errorf("primary key changed on reopen: %s", pk)
	}
}

// hasColumnInSchema reports whether a table in a named schema carries a column.
// Distinct from hasColumn, which is fixed to CURRENT_SCHEMA().
func hasColumnInSchema(t *testing.T, db *sql.DB, schema, table, column string) bool {
	t.Helper()
	var exists bool
	if err := db.QueryRow(`
		SELECT EXISTS (
			SELECT 1 FROM information_schema.columns
			WHERE table_schema = $1 AND table_name = $2 AND column_name = $3
		)`, schema, table, column).Scan(&exists); err != nil {
		t.Fatalf("inspect %s.%s.%s: %v", schema, table, column, err)
	}
	return exists
}

// countRows returns the row count of a (possibly schema-qualified) relation.
func countRows(t *testing.T, db *sql.DB, relation string) int {
	t.Helper()
	var n int
	if err := db.QueryRow(`SELECT COUNT(*) FROM ` + relation).Scan(&n); err != nil {
		t.Fatalf("count rows in %s: %v", relation, err)
	}
	return n
}

// TestReadModelStore_RefusesConflictingEventsTables covers the ambiguous schema: a
// pre-#733 read-model events table sitting beside a life_events table. The store
// cannot tell which one holds the live life facts, so it must refuse rather than
// serve one of them, and must leave both exactly as it found them (issue #733).
func TestReadModelStore_RefusesConflictingEventsTables(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test")
	}

	db, cleanup := setupPostgres(t)
	defer cleanup()

	legacyID := seedLegacyReadModelEvents(t, db)
	if _, err := db.Exec(`
		CREATE TABLE life_events (id UUID PRIMARY KEY, owner_type VARCHAR(10) NOT NULL);
		INSERT INTO life_events (id, owner_type) VALUES (gen_random_uuid(), 'family');
	`); err != nil {
		t.Fatalf("create conflicting life_events table: %v", err)
	}

	store, err := pgstore.NewReadModelStore(db)
	if !errors.Is(err, pgstore.ErrConflictingEventsTables) {
		t.Fatalf("expected ErrConflictingEventsTables, got: %v", err)
	}
	if store != nil {
		t.Error("expected a nil store alongside the error")
	}

	// Neither table may have been touched: no rename, no new columns, no lost rows.
	if !tableExists(t, db, "events") {
		t.Fatal("the legacy events table was renamed despite the conflict")
	}
	if n := countRows(t, db, "events"); n != 1 {
		t.Errorf("expected the legacy row to survive, got %d rows", n)
	}
	var place string
	if err := db.QueryRow(`SELECT place FROM events WHERE id = $1`, legacyID).Scan(&place); err != nil {
		t.Fatalf("read legacy row back: %v", err)
	}
	if place != "Springfield, IL" {
		t.Errorf("legacy row damaged: place=%q", place)
	}
	if n := countRows(t, db, "life_events"); n != 1 {
		t.Errorf("expected the pre-existing life_events row to survive, got %d rows", n)
	}
	if hasColumn(t, db, "life_events", "fact_type") {
		t.Error("the DDL batch ran against the conflicting life_events table")
	}
	if tableExists(t, db, "persons") {
		t.Error("the read model DDL batch ran despite the refusal")
	}
}

// TestReadModelStore_FailedRenameIsFatal is the mutation-sensitive test for the
// #733 panel finding that the rename must not be best-effort. The failure is forced
// by parking a relation of the wrong kind on the name DROP INDEX needs: the drop
// step then errors with `"idx_events_owner" is not an index`, aborting the rename
// sequence at its first statement. The blocker is synthetic, but the code path is
// the real one — any failure inside the sequence has to reach the caller.
//
// If the error were swallowed (the pre-fix behaviour), createTables' DDL batch would
// go on to create an EMPTY life_events beside the still-populated legacy table,
// NewReadModelStore would return success, and every life-event query would answer
// zero rows forever — the next open takes the already-migrated path, so restarting
// never recovers. All three assertions below fail under that mutation.
func TestReadModelStore_FailedRenameIsFatal(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test")
	}

	db, cleanup := setupPostgres(t)
	defer cleanup()

	legacyID := seedLegacyReadModelEvents(t, db)

	// seedLegacyReadModelEvents built idx_events_owner as an index; replace it with
	// a table of that name so DROP INDEX refuses to touch it.
	if _, err := db.Exec(`
		DROP INDEX idx_events_owner;
		CREATE TABLE idx_events_owner (id UUID PRIMARY KEY);
	`); err != nil {
		t.Fatalf("plant the rename blocker: %v", err)
	}

	store, err := pgstore.NewReadModelStore(db)
	if err == nil {
		t.Fatal("expected NewReadModelStore to fail when the rename cannot complete")
	}
	if store != nil {
		t.Error("expected a nil store alongside the error")
	}

	// The legacy rows must still be reachable under their original name...
	if !tableExists(t, db, "events") {
		t.Fatal("legacy events table disappeared after the failed rename")
	}
	var place string
	if err := db.QueryRow(`SELECT place FROM events WHERE id = $1`, legacyID).Scan(&place); err != nil {
		t.Fatalf("read legacy row back: %v", err)
	}
	if place != "Springfield, IL" {
		t.Errorf("legacy row damaged: place=%q", place)
	}

	// ...and no empty life_events may have been created to shadow them.
	if tableExists(t, db, "life_events") {
		t.Errorf("an empty life_events table was created beside the un-migrated legacy table (%d rows)",
			countRows(t, db, "life_events"))
	}
}

// TestReadModelStore_MigratesWithinCurrentSchemaOnly pins the schema-resolution fix.
// The rename used to test for life_events with to_regclass, which searches the WHOLE
// search_path, while its owner_type probe was scoped to CURRENT_SCHEMA(). With a DSN
// carrying search_path=app,other — lib/pq forwards it, so DATABASE_URL can express
// it — an unrelated other.life_events made to_regclass report "already migrated" and
// app.events was left behind, stranding every life-event row (issue #733).
func TestReadModelStore_MigratesWithinCurrentSchemaOnly(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test")
	}

	db, cleanup := setupPostgres(t)
	defer cleanup()

	// SET search_path is session state, so the pool must hand out one connection.
	db.SetMaxOpenConns(1)

	if _, err := db.Exec(`
		CREATE SCHEMA app;
		CREATE SCHEMA other;
		CREATE TABLE other.life_events (id UUID PRIMARY KEY, decoy_marker TEXT);
		SET search_path = app, other;
	`); err != nil {
		t.Fatalf("set up the two-schema search_path: %v", err)
	}

	// Unqualified DDL now lands in app, which is what CURRENT_SCHEMA() reports.
	eventID := seedLegacyReadModelEvents(t, db)

	store, err := pgstore.NewReadModelStore(db)
	if err != nil {
		t.Fatalf("create read model store under a multi-schema search_path: %v", err)
	}

	if !tableExists(t, db, "app.life_events") {
		t.Fatal("app.events was not renamed; the decoy other.life_events suppressed the migration")
	}
	if tableExists(t, db, "app.events") {
		t.Error("app.events survived the rename")
	}
	if pk := primaryKeyName(t, db, "app.life_events"); pk != "life_events_pkey" {
		t.Errorf("expected primary key life_events_pkey, got %s", pk)
	}

	got, err := store.GetEvent(context.Background(), eventID)
	if err != nil {
		t.Fatalf("get migrated event: %v", err)
	}
	if got.Place != "Springfield, IL" {
		t.Errorf("migrated row corrupted: %+v", got)
	}

	// The other schema's table is none of the read model's business.
	if !tableExists(t, db, "other.life_events") {
		t.Fatal("other.life_events was destroyed")
	}
	if n := countRows(t, db, "other.life_events"); n != 0 {
		t.Errorf("other.life_events was written to: %d rows", n)
	}
	if !hasColumnInSchema(t, db, "other", "life_events", "decoy_marker") {
		t.Error("other.life_events lost its shape; the migration reached across schemas")
	}
}

// TestReadModelStore_LeavesEventLogTableAlone is the regression test for the
// owner_type guard: the event log's events table must never be renamed.
func TestReadModelStore_LeavesEventLogTableAlone(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test")
	}

	db, cleanup := setupPostgres(t)
	defer cleanup()

	if _, err := pgstore.NewEventStore(db); err != nil {
		t.Fatalf("create event store: %v", err)
	}

	if _, err := pgstore.NewReadModelStore(db); err != nil {
		t.Fatalf("create read model store: %v", err)
	}

	if !tableExists(t, db, "events") {
		t.Fatal("event log's events table was renamed away")
	}
	var hasOwnerType bool
	if err := db.QueryRow(`
		SELECT EXISTS (
			SELECT 1 FROM information_schema.columns
			WHERE table_schema = CURRENT_SCHEMA() AND table_name = 'events' AND column_name = 'owner_type'
		)`).Scan(&hasOwnerType); err != nil {
		t.Fatalf("inspect events table: %v", err)
	}
	if hasOwnerType {
		t.Error("events table gained read model columns; the guard let the migration through")
	}
}

// TestStores_ShareSingleDatabase covers ADR-002's single-DATABASE_URL deployment:
// both stores must build on one database in either construction order and
// round-trip their own data independently. Before #733 this collided on
// events_pkey.
func TestStores_ShareSingleDatabase(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test")
	}

	orders := []struct {
		name           string
		eventStoreLast bool
	}{
		{"event store first", false},
		{"read model first", true},
	}

	for _, order := range orders {
		t.Run(order.name, func(t *testing.T) {
			db, cleanup := setupPostgres(t)
			defer cleanup()

			var (
				eventStore *pgstore.EventStore
				readModel  *pgstore.ReadModelStore
				err        error
			)
			build := []func() error{
				func() error {
					eventStore, err = pgstore.NewEventStore(db)
					return err
				},
				func() error {
					readModel, err = pgstore.NewReadModelStore(db)
					return err
				},
			}
			if order.eventStoreLast {
				build[0], build[1] = build[1], build[0]
			}
			for _, step := range build {
				if err := step(); err != nil {
					t.Fatalf("build stores on shared database: %v", err)
				}
			}

			ctx := context.Background()
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

			lifeEvent := &repository.EventReadModel{
				ID:        uuid.New(),
				OwnerType: "person",
				OwnerID:   personID,
				FactType:  domain.FactPersonBirth,
				DateRaw:   "10 DEC 1815",
				Place:     "London, England",
				Version:   1,
				CreatedAt: time.Now().Truncate(time.Microsecond),
			}
			if err := readModel.SaveEvent(ctx, lifeEvent); err != nil {
				t.Fatalf("save life event: %v", err)
			}

			stream, err := eventStore.ReadStream(ctx, personID)
			if err != nil {
				t.Fatalf("read stream: %v", err)
			}
			if len(stream) != 1 || stream[0].EventType != "PersonCreated" {
				t.Errorf("event log round-trip failed: %+v", stream)
			}

			got, err := readModel.GetEvent(ctx, lifeEvent.ID)
			if err != nil {
				t.Fatalf("get life event: %v", err)
			}
			if got.Place != lifeEvent.Place || got.OwnerID != personID {
				t.Errorf("read model round-trip failed: %+v", got)
			}

			// The two tables are independent: the log's row must not appear as a
			// life event, and vice versa.
			_, total, err := readModel.ListEvents(ctx, repository.ListOptions{Limit: 10})
			if err != nil {
				t.Fatalf("list life events: %v", err)
			}
			if total != 1 {
				t.Errorf("expected exactly 1 life event, got %d", total)
			}
		})
	}
}
