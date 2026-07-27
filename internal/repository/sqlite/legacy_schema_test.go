package sqlite_test

import (
	"bytes"
	"context"
	"database/sql"
	"errors"
	"log/slog"
	"os"
	"strings"
	"testing"

	"github.com/google/uuid"

	"github.com/cacack/my-family/internal/domain"
	"github.com/cacack/my-family/internal/repository"
	"github.com/cacack/my-family/internal/repository/sqlite"
)

// setupLegacyReadModelDB builds a database that mimics a pre-#669 read model: the
// slice tables carry the old single-column `id` PRIMARY KEY. SQLite cannot alter a
// PK in place, so NewReadModelStore's CREATE TABLE IF NOT EXISTS leaves these
// definitions untouched and the store must detect that it cannot hold branch rows.
func setupLegacyReadModelDB(t *testing.T) (*sqlite.ReadModelStore, func()) {
	t.Helper()

	tmpFile, err := os.CreateTemp("", "myfamily-legacy-readmodel-*.db")
	if err != nil {
		t.Fatalf("create temp file: %v", err)
	}
	tmpFile.Close()

	db, err := sqlite.OpenDB(tmpFile.Name())
	if err != nil {
		os.Remove(tmpFile.Name())
		t.Fatalf("open database: %v", err)
	}

	// Pre-#669 shape: the real persons columns, but the lone `id` PRIMARY KEY and
	// no branch_id/deleted. This is what an existing deployment's database looks
	// like; SQLite cannot alter that PK in place, so the store must detect it.
	if _, err := db.Exec(`
		CREATE TABLE persons (
			id TEXT PRIMARY KEY,
			given_name TEXT NOT NULL,
			surname TEXT NOT NULL,
			full_name TEXT GENERATED ALWAYS AS (given_name || ' ' || surname) STORED,
			gender TEXT,
			birth_date_raw TEXT,
			birth_date_sort TEXT,
			birth_place TEXT,
			death_date_raw TEXT,
			death_date_sort TEXT,
			death_place TEXT,
			notes TEXT,
			research_status TEXT,
			version INTEGER NOT NULL DEFAULT 1,
			updated_at TEXT NOT NULL DEFAULT (datetime('now'))
		)`); err != nil {
		db.Close()
		os.Remove(tmpFile.Name())
		t.Fatalf("create legacy persons table: %v", err)
	}

	store, err := sqlite.NewReadModelStore(db)
	if err != nil {
		db.Close()
		os.Remove(tmpFile.Name())
		t.Fatalf("create read model store: %v", err)
	}
	return store, func() {
		db.Close()
		os.Remove(tmpFile.Name())
	}
}

// TestLegacySchemaRefusesBranchWrites verifies the ADR-005 / issue #680 guard: a
// read model whose schema predates branch support must refuse branch-scoped writes
// with repository.ErrBranchesUnsupported instead of appearing branch-capable and
// then failing on an opaque PRIMARY KEY constraint violation. Mainline writes and
// reads must keep working so an un-rebuilt deployment stays usable.
func TestLegacySchemaRefusesBranchWrites(t *testing.T) {
	store, cleanup := setupLegacyReadModelDB(t)
	defer cleanup()
	ctx := context.Background()

	branch := domain.BranchID(uuid.New())
	personID := uuid.New()

	err := store.SavePerson(ctx, branch, &repository.PersonReadModel{
		ID: personID, GivenName: "Branch", Surname: "Write", Version: 1,
	})
	if !errors.Is(err, repository.ErrBranchesUnsupported) {
		t.Fatalf("branch SavePerson on legacy schema: want ErrBranchesUnsupported, got %v", err)
	}

	// Every branch-scoped write is guarded, not just SavePerson.
	if err := store.DeletePerson(ctx, branch, personID); !errors.Is(err, repository.ErrBranchesUnsupported) {
		t.Fatalf("branch DeletePerson on legacy schema: want ErrBranchesUnsupported, got %v", err)
	}
	if err := store.PurgeBranch(ctx, branch); !errors.Is(err, repository.ErrBranchesUnsupported) {
		t.Fatalf("PurgeBranch on legacy schema: want ErrBranchesUnsupported, got %v", err)
	}

	// Mainline stays fully usable on the un-rebuilt database.
	if err := store.SavePerson(ctx, domain.MainBranchID, &repository.PersonReadModel{
		ID: personID, GivenName: "Main", Surname: "Write", Version: 1,
	}); err != nil {
		t.Fatalf("main SavePerson on legacy schema: %v", err)
	}
	got, err := store.GetPerson(ctx, domain.MainBranchID, personID)
	if err != nil {
		t.Fatalf("main GetPerson on legacy schema: %v", err)
	}
	if got == nil || got.GivenName != "Main" {
		t.Fatalf("main GetPerson on legacy schema: want Main, got %+v", got)
	}
}

// TestFreshSchemaAllowsBranchWrites is the control: a freshly created read model
// has the composite (id, branch_id) key and must NOT be flagged legacy.
func TestFreshSchemaAllowsBranchWrites(t *testing.T) {
	store, cleanup := setupTestReadModelDB(t)
	defer cleanup()
	ctx := context.Background()

	branch := domain.BranchID(uuid.New())
	personID := uuid.New()

	if err := store.SavePerson(ctx, domain.MainBranchID, branchPersonRM(personID, "Main", "Row")); err != nil {
		t.Fatalf("main SavePerson: %v", err)
	}
	if err := store.SavePerson(ctx, branch, branchPersonRM(personID, "Branch", "Row")); err != nil {
		t.Fatalf("branch SavePerson on fresh schema: want success, got %v", err)
	}
}

// legacyEventFixture is one row of the pre-ADR-005 events table. Ids and
// positions are asserted to survive the rebuild byte-for-byte.
type legacyEventFixture struct {
	id       string
	position int64
	version  int64
}

// setupLegacyEventStoreDB builds a database carrying the pre-ADR-005 events DDL:
// UNIQUE(stream_id, version) and no branch_id column. SQLite cannot alter a table
// constraint, so NewEventStore must rebuild the table in place.
func setupLegacyEventStoreDB(t *testing.T, streamID uuid.UUID) (*sql.DB, []legacyEventFixture, func()) {
	t.Helper()

	tmpFile, err := os.CreateTemp("", "myfamily-legacy-eventstore-*.db")
	if err != nil {
		t.Fatalf("create temp file: %v", err)
	}
	tmpFile.Close()

	db, err := sqlite.OpenDB(tmpFile.Name())
	if err != nil {
		os.Remove(tmpFile.Name())
		t.Fatalf("open database: %v", err)
	}
	cleanup := func() {
		db.Close()
		os.Remove(tmpFile.Name())
	}

	if _, err := db.Exec(`
		CREATE TABLE streams (
			id TEXT PRIMARY KEY,
			type TEXT NOT NULL,
			created_at TEXT NOT NULL DEFAULT (datetime('now')),
			metadata TEXT
		);

		CREATE TABLE events (
			id TEXT PRIMARY KEY,
			stream_id TEXT NOT NULL,
			stream_type TEXT NOT NULL,
			version INTEGER NOT NULL,
			event_type TEXT NOT NULL,
			data TEXT NOT NULL,
			metadata TEXT,
			timestamp TEXT NOT NULL,
			position INTEGER NOT NULL,
			FOREIGN KEY (stream_id) REFERENCES streams(id),
			UNIQUE(stream_id, version)
		);

		CREATE INDEX idx_events_stream_version ON events(stream_id, version);
		CREATE INDEX idx_events_position ON events(position);
	`); err != nil {
		cleanup()
		t.Fatalf("create legacy event schema: %v", err)
	}

	if _, err := db.Exec("INSERT INTO streams (id, type) VALUES (?, ?)", streamID.String(), "Person"); err != nil {
		cleanup()
		t.Fatalf("seed legacy stream: %v", err)
	}

	fixtures := []legacyEventFixture{
		{id: uuid.New().String(), position: 1, version: 1},
		{id: uuid.New().String(), position: 2, version: 2},
		{id: uuid.New().String(), position: 3, version: 3},
	}
	for _, f := range fixtures {
		if _, err := db.Exec(`
			INSERT INTO events (id, stream_id, stream_type, version, event_type, data, timestamp, position)
			VALUES (?, ?, 'Person', ?, 'PersonUpdated', '{}', '2026-01-01T00:00:00Z', ?)`,
			f.id, streamID.String(), f.version, f.position); err != nil {
			cleanup()
			t.Fatalf("seed legacy event: %v", err)
		}
	}

	return db, fixtures, cleanup
}

// TestLegacyEventStoreRebuild verifies the one-time in-place migration of the
// event log's source of truth: a database carrying UNIQUE(stream_id, version)
// must come out of NewEventStore with the composite UNIQUE(stream_id, branch_id,
// version), with every row, id and position preserved, and branch writes working.
func TestLegacyEventStoreRebuild(t *testing.T) {
	streamID := uuid.New()
	db, fixtures, cleanup := setupLegacyEventStoreDB(t, streamID)
	defer cleanup()
	ctx := context.Background()

	// Capture the migration log line: exactly one is expected for the rebuild.
	var logs bytes.Buffer
	restore := slog.Default()
	slog.SetDefault(slog.New(slog.NewTextHandler(&logs, nil)))
	defer slog.SetDefault(restore)

	store, err := sqlite.NewEventStore(db)
	if err != nil {
		t.Fatalf("NewEventStore on legacy database: %v", err)
	}

	if n := strings.Count(logs.String(), "rebuilding sqlite events table"); n != 1 {
		t.Fatalf("rebuild log lines = %d, want exactly 1; log was:\n%s", n, logs.String())
	}

	// The constraint is composite afterwards.
	var ddl string
	if err := db.QueryRow(
		"SELECT sql FROM sqlite_master WHERE type = 'table' AND name = 'events'").Scan(&ddl); err != nil {
		t.Fatalf("read migrated events DDL: %v", err)
	}
	normalized := strings.ToLower(strings.Join(strings.Fields(ddl), ""))
	if !strings.Contains(normalized, "unique(stream_id,branch_id,version)") {
		t.Fatalf("migrated events DDL lacks the composite constraint:\n%s", ddl)
	}

	// Row count is unchanged and every position and id survived, in order.
	rows, err := db.Query("SELECT id, position, version, branch_id FROM events ORDER BY position ASC")
	if err != nil {
		t.Fatalf("read migrated events: %v", err)
	}
	defer rows.Close()

	var got []legacyEventFixture
	for rows.Next() {
		var f legacyEventFixture
		var branchID string
		if err := rows.Scan(&f.id, &f.position, &f.version, &branchID); err != nil {
			t.Fatalf("scan migrated event: %v", err)
		}
		if branchID != domain.MainBranchID.String() {
			t.Errorf("migrated event %s has branch_id %s, want MainBranchID", f.id, branchID)
		}
		got = append(got, f)
	}
	if err := rows.Err(); err != nil {
		t.Fatalf("iterate migrated events: %v", err)
	}
	if len(got) != len(fixtures) {
		t.Fatalf("migrated row count = %d, want %d", len(got), len(fixtures))
	}
	for i, want := range fixtures {
		if got[i] != want {
			t.Errorf("migrated event %d = %+v, want %+v", i, got[i], want)
		}
	}

	// Mainline versioning still reads the legacy history...
	if v, err := store.GetStreamVersion(ctx, streamID, domain.MainBranchID); err != nil || v != 3 {
		t.Fatalf("main version after rebuild = %d (err %v), want 3", v, err)
	}

	// ...and a branch write now succeeds, seeded from main's version at the base.
	branch := repository.AppendScope{BranchID: domain.BranchID(uuid.New()), BasePosition: 3}
	if err := store.Append(ctx, streamID, "Person",
		[]domain.Event{domain.NewPersonUpdated(streamID, map[string]any{"surname": "Revised"})}, 3, branch); err != nil {
		t.Fatalf("branch append after rebuild: %v", err)
	}
	if v, err := store.GetStreamVersion(ctx, streamID, branch.BranchID); err != nil || v != 4 {
		t.Fatalf("branch version after rebuild = %d (err %v), want 4", v, err)
	}
	if v, err := store.GetStreamVersion(ctx, streamID, domain.MainBranchID); err != nil || v != 3 {
		t.Fatalf("main version after branch write = %d (err %v), want 3 (unchanged)", v, err)
	}

	// A second branch takes version 4 of the SAME stream — impossible under the
	// legacy UNIQUE(stream_id, version), so this is the constraint swap proving
	// itself rather than just the DDL text.
	other := repository.AppendScope{BranchID: domain.BranchID(uuid.New()), BasePosition: 3}
	if err := store.Append(ctx, streamID, "Person",
		[]domain.Event{domain.NewPersonUpdated(streamID, map[string]any{"surname": "Alternate"})}, 3, other); err != nil {
		t.Fatalf("second branch append at the same version after rebuild: %v", err)
	}
}

// TestFreshEventStoreSkipsRebuild is the control: a database created by the
// current code already carries the composite constraint, so opening it (twice)
// must never trigger the rebuild.
func TestFreshEventStoreSkipsRebuild(t *testing.T) {
	tmpFile, err := os.CreateTemp("", "myfamily-fresh-eventstore-*.db")
	if err != nil {
		t.Fatalf("create temp file: %v", err)
	}
	tmpFile.Close()
	defer os.Remove(tmpFile.Name())

	db, err := sqlite.OpenDB(tmpFile.Name())
	if err != nil {
		t.Fatalf("open database: %v", err)
	}
	defer db.Close()

	var logs bytes.Buffer
	restore := slog.Default()
	slog.SetDefault(slog.New(slog.NewTextHandler(&logs, nil)))
	defer slog.SetDefault(restore)

	for i := 0; i < 2; i++ {
		if _, err := sqlite.NewEventStore(db); err != nil {
			t.Fatalf("NewEventStore pass %d: %v", i, err)
		}
	}

	if n := strings.Count(logs.String(), "rebuilding sqlite events table"); n != 0 {
		t.Fatalf("rebuild log lines on a fresh database = %d, want 0; log was:\n%s", n, logs.String())
	}
}
