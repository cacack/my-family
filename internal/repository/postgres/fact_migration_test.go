package postgres_test

import (
	"context"
	"database/sql"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/cacack/my-family/internal/domain"
	"github.com/cacack/my-family/internal/repository"
	pgstore "github.com/cacack/my-family/internal/repository/postgres"
)

// preFactBranchDDL is the person/family fact schema as it stood after #733 but
// before #757: life_events, attributes and associations keyed by a lone id with
// no branch_id or deleted column. runBranchMigration must bring each to the
// composite (id, branch_id) key in place, keeping its rows.
const preFactBranchDDL = `
	CREATE TABLE life_events (
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
	CREATE TABLE attributes (
		id UUID PRIMARY KEY,
		person_id UUID NOT NULL,
		fact_type VARCHAR(100) NOT NULL,
		value TEXT NOT NULL DEFAULT '',
		date_raw VARCHAR(100),
		date_sort DATE,
		place VARCHAR(255),
		version BIGINT NOT NULL DEFAULT 1,
		created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
	);
	CREATE TABLE associations (
		id UUID PRIMARY KEY,
		person_id UUID NOT NULL,
		person_name VARCHAR(200),
		associate_id UUID NOT NULL,
		associate_name VARCHAR(200),
		role VARCHAR(100) NOT NULL,
		phrase VARCHAR(500),
		notes TEXT,
		note_ids JSONB,
		gedcom_xref VARCHAR(50),
		version BIGINT NOT NULL DEFAULT 1,
		updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
	);
`

// primaryKeyHasBranch reports whether table's primary key includes branch_id.
func primaryKeyHasBranch(t *testing.T, db *sql.DB, table string) bool {
	t.Helper()
	var has bool
	if err := db.QueryRow(`
		SELECT EXISTS (
			SELECT 1 FROM pg_index i
			JOIN pg_attribute a ON a.attrelid = i.indrelid AND a.attnum = ANY(i.indkey)
			WHERE i.indrelid = $1::regclass AND i.indisprimary AND a.attname = 'branch_id'
		)`, table).Scan(&has); err != nil {
		t.Fatalf("inspect primary key of %s: %v", table, err)
	}
	return has
}

// TestReadModelStore_MigratesFactTablesToBranchKeys covers the in-place upgrade of
// a database created before #757: the three fact tables gain branch_id + deleted
// and the composite (id, branch_id) key, their rows land on main, branch shadow
// rows become writable, and reopening is a no-op.
func TestReadModelStore_MigratesFactTablesToBranchKeys(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test")
	}

	db, cleanup := setupPostgres(t)
	defer cleanup()

	if _, err := db.Exec(preFactBranchDDL); err != nil {
		t.Fatalf("create pre-#757 fact tables: %v", err)
	}
	eventID, attrID, assocID := uuid.New(), uuid.New(), uuid.New()
	personID, associateID := uuid.New(), uuid.New()
	if _, err := db.Exec(`
		INSERT INTO life_events (id, owner_type, owner_id, fact_type, place) VALUES ($1, 'person', $2, $3, 'Restland')
	`, eventID, personID, string(domain.FactPersonBurial)); err != nil {
		t.Fatalf("seed life event: %v", err)
	}
	if _, err := db.Exec(`
		INSERT INTO attributes (id, person_id, fact_type, value) VALUES ($1, $2, $3, 'Farmer')
	`, attrID, personID, string(domain.FactPersonOccupation)); err != nil {
		t.Fatalf("seed attribute: %v", err)
	}
	if _, err := db.Exec(`
		INSERT INTO associations (id, person_id, associate_id, role) VALUES ($1, $2, $3, 'witness')
	`, assocID, personID, associateID); err != nil {
		t.Fatalf("seed association: %v", err)
	}

	store, err := pgstore.NewReadModelStore(db)
	if err != nil {
		t.Fatalf("create read model store: %v", err)
	}
	for _, table := range []string{"life_events", "attributes", "associations"} {
		if !primaryKeyHasBranch(t, db, table) {
			t.Errorf("%s primary key was not migrated to (id, branch_id)", table)
		}
		if !hasColumn(t, db, table, "deleted") {
			t.Errorf("%s gained no deleted column", table)
		}
	}

	ctx := context.Background()
	main := domain.MainBranchID
	event, err := store.GetEvent(ctx, main, eventID)
	if err != nil || event == nil || event.Place != "Restland" {
		t.Fatalf("migrated life event on main = %+v (err=%v)", event, err)
	}
	if attr, err := store.GetAttribute(ctx, main, attrID); err != nil || attr == nil || attr.Value != "Farmer" {
		t.Fatalf("migrated attribute on main = %+v (err=%v)", attr, err)
	}
	if assoc, err := store.GetAssociation(ctx, main, assocID); err != nil || assoc == nil || assoc.Role != "witness" {
		t.Fatalf("migrated association on main = %+v (err=%v)", assoc, err)
	}

	// A branch shadow of the migrated row is now representable.
	branch := domain.BranchID(uuid.New())
	shadow := *event
	shadow.Place = "Westland"
	shadow.CreatedAt = time.Now()
	if err := store.SaveEvent(ctx, branch, &shadow); err != nil {
		t.Fatalf("branch SaveEvent on migrated table: %v", err)
	}
	if got, err := store.GetEvent(ctx, branch, eventID); err != nil || got == nil || got.Place != "Westland" {
		t.Errorf("branch GetEvent = %+v (err=%v), want the Westland shadow", got, err)
	}
	if got, err := store.GetEvent(ctx, main, eventID); err != nil || got == nil || got.Place != "Restland" {
		t.Errorf("main GetEvent after branch write = %+v (err=%v), want Restland", got, err)
	}

	// Reopening is a no-op that keeps both rows.
	store2, err := pgstore.NewReadModelStore(db)
	if err != nil {
		t.Fatalf("reopen read model store: %v", err)
	}
	if _, total, err := store2.ListEvents(ctx, repository.ListOptions{Limit: 10, BranchID: branch}); err != nil || total != 1 {
		t.Errorf("branch ListEvents after reopen: total=%d err=%v, want 1", total, err)
	}
	var rows int
	if err := db.QueryRow(`SELECT COUNT(*) FROM life_events`).Scan(&rows); err != nil {
		t.Fatalf("count life_events: %v", err)
	}
	if rows != 2 {
		t.Errorf("life_events holds %d rows after reopen, want 2 (main + branch shadow)", rows)
	}
}
