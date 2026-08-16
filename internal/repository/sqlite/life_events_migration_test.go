package sqlite_test

import (
	"context"
	"database/sql"
	"errors"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/cacack/my-family/internal/domain"
	"github.com/cacack/my-family/internal/repository"
	"github.com/cacack/my-family/internal/repository/sqlite"
)

// newTempDB opens a file-backed SQLite database with no schema at all, so a test
// can hand-build a legacy shape before any store touches it.
func newTempDB(t *testing.T) (*sql.DB, func()) {
	t.Helper()

	tmpFile, err := os.CreateTemp("", "myfamily-life-events-*.db")
	if err != nil {
		t.Fatalf("create temp file: %v", err)
	}
	tmpFile.Close()

	db, err := sqlite.OpenDB(tmpFile.Name())
	if err != nil {
		os.Remove(tmpFile.Name())
		t.Fatalf("open database: %v", err)
	}
	return db, func() {
		db.Close()
		os.Remove(tmpFile.Name())
	}
}

// tableExists reports whether a table of that name is in the schema.
func tableExists(t *testing.T, db *sql.DB, name string) bool {
	t.Helper()

	var n int
	if err := db.QueryRow(
		"SELECT COUNT(*) FROM sqlite_master WHERE type = 'table' AND name = ?", name).Scan(&n); err != nil {
		t.Fatalf("look up table %s: %v", name, err)
	}
	return n > 0
}

// setupLegacyLifeEventsDB builds a database carrying the pre-#733 read-model
// `events` table — the read-model shape, not the event log's — seeded with one
// row, plus the old idx_events_* indexes. is_negated is deliberately absent: this
// is the pre-#222 shape, so the test also proves the rename runs early enough for
// runMigrations' ADD COLUMN to land on the renamed table.
func setupLegacyLifeEventsDB(t *testing.T, eventID, ownerID uuid.UUID) (*sql.DB, func()) {
	t.Helper()

	db, cleanup := newTempDB(t)

	if _, err := db.Exec(`
		CREATE TABLE events (
			id TEXT PRIMARY KEY,
			owner_type TEXT NOT NULL,
			owner_id TEXT NOT NULL,
			fact_type TEXT NOT NULL,
			date_raw TEXT,
			date_sort TEXT,
			place TEXT,
			place_lat TEXT,
			place_long TEXT,
			address TEXT,
			description TEXT,
			cause TEXT,
			age TEXT,
			research_status TEXT,
			version INTEGER NOT NULL DEFAULT 1,
			created_at TEXT NOT NULL DEFAULT (datetime('now'))
		);

		CREATE INDEX idx_events_owner ON events(owner_type, owner_id);
		CREATE INDEX idx_events_fact_type ON events(fact_type);
	`); err != nil {
		cleanup()
		t.Fatalf("create legacy read-model events table: %v", err)
	}

	if _, err := db.Exec(`
		INSERT INTO events (id, owner_type, owner_id, fact_type, date_raw, place, version, created_at)
		VALUES (?, 'person', ?, ?, '12 MAR 1888', 'Athens, Ohio', 1, '2026-01-01T00:00:00Z')`,
		eventID.String(), ownerID.String(), string(domain.FactPersonBirth)); err != nil {
		cleanup()
		t.Fatalf("seed legacy life event: %v", err)
	}

	return db, cleanup
}

// TestLegacyEventsTableRenamedToLifeEvents verifies the #733 in-place migration: a
// read model whose life-fact table is still named `events` comes out of
// NewReadModelStore with `life_events`, every row intact and readable, and opening
// the store again is a no-op.
func TestLegacyEventsTableRenamedToLifeEvents(t *testing.T) {
	eventID, ownerID := uuid.New(), uuid.New()
	db, cleanup := setupLegacyLifeEventsDB(t, eventID, ownerID)
	defer cleanup()
	ctx := context.Background()

	store, err := sqlite.NewReadModelStore(db)
	if err != nil {
		t.Fatalf("NewReadModelStore on pre-#733 database: %v", err)
	}

	if !tableExists(t, db, "life_events") {
		t.Fatal("life_events table missing after migration")
	}
	if tableExists(t, db, "events") {
		t.Fatal("legacy events table still present after migration")
	}

	// The seeded row survived the rename and reads back through the store.
	got, err := store.GetEvent(ctx, eventID)
	if err != nil {
		t.Fatalf("GetEvent after migration: %v", err)
	}
	if got == nil {
		t.Fatal("GetEvent after migration returned nil; the legacy row was lost")
	}
	if got.OwnerID != ownerID || got.FactType != domain.FactPersonBirth || got.Place != "Athens, Ohio" {
		t.Errorf("migrated event = %+v, want owner %s / %s / Athens, Ohio", got, ownerID, domain.FactPersonBirth)
	}

	events, total, err := store.ListEvents(ctx, repository.ListOptions{Limit: 10})
	if err != nil {
		t.Fatalf("ListEvents after migration: %v", err)
	}
	if total != 1 || len(events) != 1 {
		t.Fatalf("ListEvents after migration = %d rows (total %d), want 1", len(events), total)
	}

	// The stale idx_events_* indexes SQLite would otherwise carry across the rename
	// are gone, and the current ones exist.
	for _, idx := range []struct {
		name string
		want bool
	}{
		{"idx_events_owner", false},
		{"idx_events_fact_type", false},
		{"idx_life_events_owner", true},
		{"idx_life_events_fact_type", true},
	} {
		var n int
		if err := db.QueryRow(
			"SELECT COUNT(*) FROM sqlite_master WHERE type = 'index' AND name = ?", idx.name).Scan(&n); err != nil {
			t.Fatalf("look up index %s: %v", idx.name, err)
		}
		if (n > 0) != idx.want {
			t.Errorf("index %s present = %v, want %v", idx.name, n > 0, idx.want)
		}
	}

	// Re-opening is a no-op: the rename is skipped and the row is still there.
	reopened, err := sqlite.NewReadModelStore(db)
	if err != nil {
		t.Fatalf("NewReadModelStore second pass: %v", err)
	}
	again, err := reopened.GetEvent(ctx, eventID)
	if err != nil || again == nil {
		t.Fatalf("GetEvent after re-open = %+v (err %v), want the migrated row", again, err)
	}

	// And the migrated table is writable through the current schema, including the
	// is_negated column the legacy fixture never had.
	if err := reopened.SaveEvent(ctx, &repository.EventReadModel{
		ID: uuid.New(), OwnerType: "person", OwnerID: ownerID,
		FactType: domain.FactPersonBurial, IsNegated: true, Version: 1, CreatedAt: time.Now(),
	}); err != nil {
		t.Fatalf("SaveEvent on migrated table: %v", err)
	}
}

// TestConflictingEventsTablesRefused covers the second guard. A database holding
// BOTH a pre-#733 read-model `events` table and a `life_events` table is ambiguous —
// either could hold the live life facts — so the store refuses to open rather than
// silently preferring one. Preferring life_events is precisely the data-stranding
// path #733 exists to prevent, and it would be unrecoverable: every later open would
// take the same already-migrated branch.
func TestConflictingEventsTablesRefused(t *testing.T) {
	eventID, ownerID := uuid.New(), uuid.New()
	db, cleanup := setupLegacyLifeEventsDB(t, eventID, ownerID)
	defer cleanup()

	// First open migrates events -> life_events.
	if _, err := sqlite.NewReadModelStore(db); err != nil {
		t.Fatalf("NewReadModelStore first pass: %v", err)
	}
	if _, err := db.Exec(`
		CREATE TABLE events (id TEXT PRIMARY KEY, owner_type TEXT NOT NULL);
		INSERT INTO events (id, owner_type) VALUES ('stray', 'person');
	`); err != nil {
		t.Fatalf("recreate a stray events table: %v", err)
	}

	_, err := sqlite.NewReadModelStore(db)
	if !errors.Is(err, sqlite.ErrConflictingEventsTables) {
		t.Fatalf("NewReadModelStore with both tables present: err = %v, want ErrConflictingEventsTables", err)
	}

	// Refusing means refusing to touch anything: an operator has to be able to
	// inspect both tables and decide which one holds the current rows.
	if !tableExists(t, db, "events") {
		t.Error("the stray events table was renamed away despite the refusal")
	}
	if !tableExists(t, db, "life_events") {
		t.Error("life_events table disappeared")
	}
	var stray int
	if err := db.QueryRow(`SELECT COUNT(*) FROM events`).Scan(&stray); err != nil {
		t.Fatalf("count stray events rows: %v", err)
	}
	if stray != 1 {
		t.Errorf("stray events row count = %d, want 1 (the refusal must not mutate either table)", stray)
	}
}

// TestFailedRenameIsReportedNotSwallowed is the regression test for the panel's
// blocking finding: a rename that fails must surface as an error from
// NewReadModelStore, not be swallowed so the DDL batch can create an empty
// life_events beside the still-populated legacy table.
//
// The failure is forced with a VIEW named life_events. The guard's existence check
// looks for type = 'table', so the view slips past it, but SQLite still refuses
// ALTER TABLE ... RENAME TO against a name a view already occupies.
//
// The assertion is on the error's PROVENANCE, not merely on err != nil. If the
// rename's error were swallowed, control would reach the DDL batch, whose
// CREATE TABLE IF NOT EXISTS life_events also fails against the view — so a bare
// err != nil check passes against the very bug this test exists to catch. Only the
// wrapped rename message distinguishes "reported the rename failure" from
// "happened to fail later for an unrelated reason". Verified by mutation: swallowing
// the rename error makes this test fail.
func TestFailedRenameIsReportedNotSwallowed(t *testing.T) {
	eventID, ownerID := uuid.New(), uuid.New()
	db, cleanup := setupLegacyLifeEventsDB(t, eventID, ownerID)
	defer cleanup()

	if _, err := db.Exec(`CREATE VIEW life_events AS SELECT 1 AS id`); err != nil {
		t.Fatalf("create blocking life_events view: %v", err)
	}

	store, err := sqlite.NewReadModelStore(db)
	if err == nil {
		t.Fatal("NewReadModelStore returned nil error despite the rename failing; " +
			"the legacy rows would be stranded behind an empty life_events table")
	}
	if store != nil {
		t.Error("NewReadModelStore returned a non-nil store alongside an error")
	}
	const want = "rename the read model's legacy events table to life_events"
	if !strings.Contains(err.Error(), want) {
		t.Errorf("error = %v\nwant it to report the rename failure (%q); a later DDL error "+
			"instead means the rename's own error was swallowed", err, want)
	}

	// The legacy table and its row must still be intact, so the operator can recover.
	if !tableExists(t, db, "events") {
		t.Fatal("the legacy events table disappeared after the failed rename")
	}
	var got string
	if err := db.QueryRow(`SELECT place FROM events WHERE id = ?`, eventID.String()).Scan(&got); err != nil {
		t.Fatalf("read the seeded row back from the legacy table: %v", err)
	}
	if got != "Athens, Ohio" {
		t.Errorf("seeded row place = %q, want %q", got, "Athens, Ohio")
	}

	// And no empty life_events TABLE may have been created behind the view.
	if tableExists(t, db, "life_events") {
		t.Error("an empty life_events table was created despite the rename failing")
	}
}

// TestReadModelLeavesEventLogTableAlone is the regression test for the owner_type
// guard in renameLegacyEventsTable: on a database that holds only the event log's
// `events` table, opening the read model must not rename the log's source of truth.
func TestReadModelLeavesEventLogTableAlone(t *testing.T) {
	db, cleanup := newTempDB(t)
	defer cleanup()
	ctx := context.Background()

	eventStore, err := sqlite.NewEventStore(db)
	if err != nil {
		t.Fatalf("NewEventStore: %v", err)
	}

	streamID := uuid.New()
	if err := eventStore.Append(ctx, streamID, "Person",
		[]domain.Event{domain.NewPersonUpdated(streamID, map[string]any{"surname": "Guarded"})},
		0, repository.MainScope); err != nil {
		t.Fatalf("append before read model: %v", err)
	}

	if _, err := sqlite.NewReadModelStore(db); err != nil {
		t.Fatalf("NewReadModelStore alongside the event log: %v", err)
	}

	if !tableExists(t, db, "events") {
		t.Fatal("the event log's events table was renamed away; the owner_type guard failed")
	}
	stored, err := eventStore.ReadStream(ctx, streamID)
	if err != nil {
		t.Fatalf("ReadStream after opening the read model: %v", err)
	}
	if len(stored) != 1 {
		t.Fatalf("event log holds %d events after opening the read model, want 1", len(stored))
	}
}

// columnExists reports whether a table carries a column of that name.
func columnExists(t *testing.T, db *sql.DB, table, column string) bool {
	t.Helper()

	var n int
	if err := db.QueryRow(
		`SELECT COUNT(*) FROM pragma_table_info(?) WHERE name = ?`, table, column).Scan(&n); err != nil {
		t.Fatalf("look up column %s.%s: %v", table, column, err)
	}
	return n > 0
}

// TestEventStoreRefusesReadModelEventsTable is the mirror of the read model's
// owner_type guard (#733): on a pre-#733 database opened event-store-first, the
// event log must refuse rather than mutate a table it does not own. Without the
// guard, migrateBranchID silently ALTERs branch_id onto the read model's table and
// the composite-key rebuild then dies on "copy events: no such column: stream_id".
func TestEventStoreRefusesReadModelEventsTable(t *testing.T) {
	eventID, ownerID := uuid.New(), uuid.New()
	db, cleanup := setupLegacyLifeEventsDB(t, eventID, ownerID)
	defer cleanup()

	_, err := sqlite.NewEventStore(db)
	if err == nil {
		t.Fatal("NewEventStore accepted a database whose events table belongs to the read model")
	}
	if !errors.Is(err, sqlite.ErrReadModelEventsTable) {
		t.Fatalf("NewEventStore error = %v, want ErrReadModelEventsTable", err)
	}

	// The message has to tell an operator what happened and what to do about it.
	msg := err.Error()
	for _, want := range []string{"read model", "life_events", "733"} {
		if !strings.Contains(msg, want) {
			t.Errorf("error message %q does not mention %q", msg, want)
		}
	}

	// Nothing was mutated: no branch_id column, the table keeps its name, and the
	// seeded row is untouched.
	if columnExists(t, db, "events", "branch_id") {
		t.Error("the read model's events table gained a branch_id column; the guard ran too late")
	}
	if !tableExists(t, db, "events") {
		t.Fatal("the read model's events table disappeared")
	}
	if tableExists(t, db, "events_new") {
		t.Error("a stray events_new table was left behind by an aborted rebuild")
	}

	var gotOwner, gotPlace string
	if err := db.QueryRow(
		"SELECT owner_id, place FROM events WHERE id = ?", eventID.String()).Scan(&gotOwner, &gotPlace); err != nil {
		t.Fatalf("read the seeded life event back: %v", err)
	}
	if gotOwner != ownerID.String() || gotPlace != "Athens, Ohio" {
		t.Errorf("seeded row = (%s, %s), want (%s, Athens, Ohio)", gotOwner, gotPlace, ownerID)
	}
}

// TestEventStoreAfterReadModelMigrationOnLegacyDB is the remedy the refusal points
// at: open the read model first so it renames itself to life_events, and the event
// store then builds its own events table on the same database. Both round-trip.
func TestEventStoreAfterReadModelMigrationOnLegacyDB(t *testing.T) {
	lifeEventID, ownerID := uuid.New(), uuid.New()
	db, cleanup := setupLegacyLifeEventsDB(t, lifeEventID, ownerID)
	defer cleanup()
	ctx := context.Background()

	readModel, err := sqlite.NewReadModelStore(db)
	if err != nil {
		t.Fatalf("NewReadModelStore on pre-#733 database: %v", err)
	}
	eventStore, err := sqlite.NewEventStore(db)
	if err != nil {
		t.Fatalf("NewEventStore after the read model migrated: %v", err)
	}

	// The event store's own table now owns the name, complete with branch_id.
	if !columnExists(t, db, "events", "branch_id") {
		t.Error("the event log's events table is missing branch_id")
	}
	if columnExists(t, db, "events", "owner_type") {
		t.Error("the events table still carries the read model's owner_type column")
	}

	streamID := uuid.New()
	if err := eventStore.Append(ctx, streamID, "Person",
		[]domain.Event{domain.NewPersonUpdated(streamID, map[string]any{"surname": "Migrated"})},
		0, repository.MainScope); err != nil {
		t.Fatalf("append after the migration: %v", err)
	}
	stored, err := eventStore.ReadStream(ctx, streamID)
	if err != nil {
		t.Fatalf("ReadStream after the migration: %v", err)
	}
	if len(stored) != 1 {
		t.Fatalf("event log holds %d events, want 1", len(stored))
	}

	// And the read model's pre-existing life event is still readable from life_events.
	got, err := readModel.GetEvent(ctx, lifeEventID)
	if err != nil || got == nil {
		t.Fatalf("GetEvent after both stores opened = %+v (err %v), want the migrated row", got, err)
	}
	if got.Place != "Athens, Ohio" {
		t.Errorf("life event place = %q, want Athens, Ohio", got.Place)
	}
}

// TestEventStoreAndReadModelShareOneDatabase covers the #733 goal: with the read
// model's life-fact table renamed, both stores build on a single *sql.DB in either
// construction order and round-trip their own data independently.
func TestEventStoreAndReadModelShareOneDatabase(t *testing.T) {
	orders := []struct {
		name  string
		build func(*testing.T, *sql.DB) (*sqlite.EventStore, *sqlite.ReadModelStore)
	}{
		{"event store first", func(t *testing.T, db *sql.DB) (*sqlite.EventStore, *sqlite.ReadModelStore) {
			t.Helper()
			es, err := sqlite.NewEventStore(db)
			if err != nil {
				t.Fatalf("NewEventStore: %v", err)
			}
			rm, err := sqlite.NewReadModelStore(db)
			if err != nil {
				t.Fatalf("NewReadModelStore: %v", err)
			}
			return es, rm
		}},
		{"read model first", func(t *testing.T, db *sql.DB) (*sqlite.EventStore, *sqlite.ReadModelStore) {
			t.Helper()
			rm, err := sqlite.NewReadModelStore(db)
			if err != nil {
				t.Fatalf("NewReadModelStore: %v", err)
			}
			es, err := sqlite.NewEventStore(db)
			if err != nil {
				t.Fatalf("NewEventStore: %v", err)
			}
			return es, rm
		}},
	}

	for _, order := range orders {
		t.Run(order.name, func(t *testing.T) {
			db, cleanup := newTempDB(t)
			defer cleanup()
			ctx := context.Background()

			eventStore, readModel := order.build(t, db)

			streamID := uuid.New()
			if err := eventStore.Append(ctx, streamID, "Person",
				[]domain.Event{domain.NewPersonUpdated(streamID, map[string]any{"surname": "Shared"})},
				0, repository.MainScope); err != nil {
				t.Fatalf("append to the shared database: %v", err)
			}

			lifeEventID := uuid.New()
			if err := readModel.SaveEvent(ctx, &repository.EventReadModel{
				ID: lifeEventID, OwnerType: "person", OwnerID: streamID,
				FactType: domain.FactPersonBirth, DateRaw: "1888", Place: "Athens, Ohio",
				Version: 1, CreatedAt: time.Now(),
			}); err != nil {
				t.Fatalf("SaveEvent on the shared database: %v", err)
			}

			// Each store reads back its own row, unaffected by the other.
			stored, err := eventStore.ReadStream(ctx, streamID)
			if err != nil {
				t.Fatalf("ReadStream on the shared database: %v", err)
			}
			if len(stored) != 1 {
				t.Fatalf("event log holds %d events, want 1", len(stored))
			}

			got, err := readModel.GetEvent(ctx, lifeEventID)
			if err != nil || got == nil {
				t.Fatalf("GetEvent on the shared database = %+v (err %v), want the saved life event", got, err)
			}
			if got.Place != "Athens, Ohio" {
				t.Errorf("life event place = %q, want Athens, Ohio", got.Place)
			}
		})
	}
}
