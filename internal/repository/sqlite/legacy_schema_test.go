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
	"time"

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
	// PurgeBranch is deliberately unguarded: it only deletes, and a lone-id table
	// holds no branch rows, so on a pre-#669 database it is a harmless no-op.
	if err := store.PurgeBranch(ctx, branch); err != nil {
		t.Fatalf("PurgeBranch on legacy schema: want no-op success, got %v", err)
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

// TestPreFactBranchSchemaRefusesBranchWrites covers a database built between
// #669 and #757: persons already carry the composite (id, branch_id) key, but
// life_events still has its lone-id key (SQLite cannot alter it in place). Such a
// database must refuse EVERY branch write with repository.ErrBranchesUnsupported —
// a branch DeletePerson would need to tombstone the person's life events, which
// the old key cannot hold — while mainline fact writes, including upserts of an
// existing row, keep working.
func TestPreFactBranchSchemaRefusesBranchWrites(t *testing.T) {
	tmpFile, err := os.CreateTemp("", "myfamily-prefact-readmodel-*.db")
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

	// Post-#733, pre-#757 life_events: owner_type present (so it is not mistaken
	// for a pre-#733 events table), lone-id PRIMARY KEY, no branch columns.
	if _, err := db.Exec(`
		CREATE TABLE life_events (
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
			is_negated INTEGER NOT NULL DEFAULT 0,
			version INTEGER NOT NULL DEFAULT 1,
			created_at TEXT NOT NULL DEFAULT (datetime('now'))
		)`); err != nil {
		t.Fatalf("create pre-#757 life_events: %v", err)
	}

	store, err := sqlite.NewReadModelStore(db)
	if err != nil {
		t.Fatalf("create read model store: %v", err)
	}
	ctx := context.Background()
	branch := domain.BranchID(uuid.New())
	personID := uuid.New()
	event := &repository.EventReadModel{
		ID: uuid.New(), OwnerType: "person", OwnerID: personID,
		FactType: domain.FactPersonBurial, Place: "Restland", Version: 1, CreatedAt: time.Now(),
	}

	// Branch writes are refused up front, fact and slice alike.
	if err := store.SaveEvent(ctx, branch, event); !errors.Is(err, repository.ErrBranchesUnsupported) {
		t.Errorf("branch SaveEvent: want ErrBranchesUnsupported, got %v", err)
	}
	if err := store.DeleteEvent(ctx, branch, event.ID); !errors.Is(err, repository.ErrBranchesUnsupported) {
		t.Errorf("branch DeleteEvent: want ErrBranchesUnsupported, got %v", err)
	}
	if err := store.SavePerson(ctx, branch, branchPersonRM(personID, "Branch", "Row")); !errors.Is(err, repository.ErrBranchesUnsupported) {
		t.Errorf("branch SavePerson: want ErrBranchesUnsupported, got %v", err)
	}

	// Mainline keeps working: insert, then upsert the same row, then delete it.
	if err := store.SaveEvent(ctx, domain.MainBranchID, event); err != nil {
		t.Fatalf("main SaveEvent: %v", err)
	}
	event.Place = "Oak Grove"
	if err := store.SaveEvent(ctx, domain.MainBranchID, event); err != nil {
		t.Fatalf("main SaveEvent upsert: %v", err)
	}
	got, err := store.GetEvent(ctx, domain.MainBranchID, event.ID)
	if err != nil || got == nil || got.Place != "Oak Grove" {
		t.Fatalf("main GetEvent = %+v (err=%v), want the upserted Oak Grove", got, err)
	}
	if err := store.DeleteEvent(ctx, domain.MainBranchID, event.ID); err != nil {
		t.Fatalf("main DeleteEvent: %v", err)
	}
	if got, err := store.GetEvent(ctx, domain.MainBranchID, event.ID); err != nil || got != nil {
		t.Errorf("main GetEvent after delete = %+v (err=%v), want absent", got, err)
	}
}

// TestPreFactBranchSchemaPurgesExistingBranch covers a database built between
// #669 and #757 that ALREADY holds a branch: its persons table has the composite
// key, so the branch's shadow rows were written before the fact tables gained
// theirs. The schema is no longer branch-capable, but deleting or merging that
// branch must still purge its overlay — otherwise projectBranchDeleted and
// projectBranchMerged would fail after their event is appended and the rows
// would linger forever.
func TestPreFactBranchSchemaPurgesExistingBranch(t *testing.T) {
	tmpFile, err := os.CreateTemp("", "myfamily-prefact-purge-*.db")
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

	// Pre-#757 attributes: lone-id key, no branch columns.
	if _, err := db.Exec(`
		CREATE TABLE attributes (
			id TEXT PRIMARY KEY,
			person_id TEXT NOT NULL,
			fact_type TEXT NOT NULL,
			value TEXT NOT NULL DEFAULT '',
			date_raw TEXT,
			date_sort TEXT,
			place TEXT,
			version INTEGER NOT NULL DEFAULT 1,
			created_at TEXT NOT NULL DEFAULT (datetime('now'))
		)`); err != nil {
		t.Fatalf("create pre-#757 attributes: %v", err)
	}

	store, err := sqlite.NewReadModelStore(db)
	if err != nil {
		t.Fatalf("create read model store: %v", err)
	}
	ctx := context.Background()
	branch := domain.BranchID(uuid.New())
	personID := uuid.New()

	// A main row and a branch shadow row that existed before the upgrade. The
	// branch row is inserted directly: the store now (correctly) refuses new
	// branch writes on this schema.
	if err := store.SavePerson(ctx, domain.MainBranchID, branchPersonRM(personID, "Main", "Row")); err != nil {
		t.Fatalf("main SavePerson: %v", err)
	}
	if _, err := db.Exec(
		`INSERT INTO persons (id, branch_id, given_name, surname) VALUES (?, ?, 'Branch', 'Row')`,
		personID.String(), branch.String()); err != nil {
		t.Fatalf("seed pre-existing branch row: %v", err)
	}
	if err := store.SavePerson(ctx, branch, branchPersonRM(personID, "New", "Write")); !errors.Is(err, repository.ErrBranchesUnsupported) {
		t.Fatalf("new branch SavePerson: want ErrBranchesUnsupported, got %v", err)
	}

	if err := store.PurgeBranch(ctx, branch); err != nil {
		t.Fatalf("PurgeBranch of pre-existing branch: %v", err)
	}

	var n int
	if err := db.QueryRow(`SELECT COUNT(*) FROM persons WHERE branch_id = ?`, branch.String()).Scan(&n); err != nil {
		t.Fatalf("count branch rows: %v", err)
	}
	if n != 0 {
		t.Errorf("branch rows after purge = %d, want 0", n)
	}
	got, err := store.GetPerson(ctx, domain.MainBranchID, personID)
	if err != nil || got == nil || got.GivenName != "Main" {
		t.Errorf("main GetPerson after purge = %+v (err=%v), want the untouched Main row", got, err)
	}
}

// preEvidenceSQLiteDDL is the evidence schema as it stood before #758: sources,
// citations and notes keyed by a lone id, source_external_ids keyed by
// (source_id, sequence), and both source children holding a foreign key to
// sources(id). SQLite cannot re-key these tables in place.
const preEvidenceSQLiteDDL = `
	CREATE TABLE sources (
		id TEXT PRIMARY KEY,
		source_type TEXT NOT NULL,
		title TEXT NOT NULL,
		author TEXT,
		publisher TEXT,
		publish_date_raw TEXT,
		publish_date_sort TEXT,
		url TEXT,
		repository_id TEXT,
		repository_name TEXT,
		collection_name TEXT,
		call_number TEXT,
		notes TEXT,
		gedcom_xref TEXT,
		citation_count INTEGER NOT NULL DEFAULT 0,
		version INTEGER NOT NULL DEFAULT 1,
		updated_at TEXT NOT NULL DEFAULT (datetime('now'))
	);
	CREATE TABLE citations (
		id TEXT PRIMARY KEY,
		source_id TEXT NOT NULL,
		source_title TEXT,
		fact_type TEXT NOT NULL,
		fact_owner_id TEXT NOT NULL,
		page TEXT,
		volume TEXT,
		source_quality TEXT,
		informant_type TEXT,
		evidence_type TEXT,
		quoted_text TEXT,
		analysis TEXT,
		template_id TEXT,
		fields_data TEXT,
		gedcom_xref TEXT,
		version INTEGER NOT NULL DEFAULT 1,
		created_at TEXT NOT NULL DEFAULT (datetime('now')),
		FOREIGN KEY (source_id) REFERENCES sources(id)
	);
	CREATE TABLE notes (
		id TEXT PRIMARY KEY,
		text TEXT NOT NULL,
		mime TEXT,
		language TEXT,
		translations TEXT,
		gedcom_xref TEXT,
		version INTEGER NOT NULL DEFAULT 1,
		updated_at TEXT NOT NULL DEFAULT (datetime('now'))
	);
	CREATE TABLE source_external_ids (
		source_id TEXT NOT NULL,
		sequence INTEGER NOT NULL,
		value TEXT NOT NULL,
		type TEXT NOT NULL DEFAULT '',
		PRIMARY KEY (source_id, sequence),
		FOREIGN KEY (source_id) REFERENCES sources(id) ON DELETE CASCADE
	);
`

// TestPreEvidenceBranchSchemaRefusesBranchWrites covers a database built between
// #757 and #758: the slice and fact tables carry branch keys, but the evidence
// tables keep their pre-branch keys (and foreign keys to sources(id)). Such a
// database must refuse every evidence branch write with
// repository.ErrBranchesUnsupported — and, like any partially branch-keyed
// schema, every other branch write too — while mainline evidence reads and
// writes, including upserts and the DeleteSource cascade, keep working.
func TestPreEvidenceBranchSchemaRefusesBranchWrites(t *testing.T) {
	tmpFile, err := os.CreateTemp("", "myfamily-preevidence-readmodel-*.db")
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

	if _, err := db.Exec(preEvidenceSQLiteDDL); err != nil {
		t.Fatalf("create pre-#758 evidence tables: %v", err)
	}

	store, err := sqlite.NewReadModelStore(db)
	if err != nil {
		t.Fatalf("create read model store: %v", err)
	}
	ctx := context.Background()
	main := domain.MainBranchID
	branch := domain.BranchID(uuid.New())
	now := time.Now()
	src := &repository.SourceReadModel{ID: uuid.New(), SourceType: domain.SourceCensus, Title: "Census 1880", Version: 1, UpdatedAt: now}
	cit := &repository.CitationReadModel{ID: uuid.New(), SourceID: src.ID, SourceTitle: src.Title, FactType: domain.FactPersonBirth, FactOwnerID: uuid.New(), Version: 1, CreatedAt: now}
	note := &repository.NoteReadModel{ID: uuid.New(), Text: "Seen in the register", Version: 1, UpdatedAt: now}
	extIDs := []repository.SourceExternalIDReadModel{{Value: "MAIN-1", Type: "http://example.org/ids"}}

	// Branch writes are refused up front.
	for name, err := range map[string]error{
		"SaveSource":               store.SaveSource(ctx, branch, src),
		"DeleteSource":             store.DeleteSource(ctx, branch, src.ID),
		"ReplaceSourceExternalIDs": store.ReplaceSourceExternalIDs(ctx, branch, src.ID, extIDs),
		"SaveCitation":             store.SaveCitation(ctx, branch, cit),
		"DeleteCitation":           store.DeleteCitation(ctx, branch, cit.ID),
		"SaveNote":                 store.SaveNote(ctx, branch, note),
		"DeleteNote":               store.DeleteNote(ctx, branch, note.ID),
		"SavePerson":               store.SavePerson(ctx, branch, branchPersonRM(uuid.New(), "Branch", "Row")),
	} {
		if !errors.Is(err, repository.ErrBranchesUnsupported) {
			t.Errorf("branch %s on pre-#758 schema: want ErrBranchesUnsupported, got %v", name, err)
		}
	}

	// Mainline keeps working: insert, upsert, read, then the DeleteSource cascade
	// (citations first, so the legacy foreign key never trips).
	if err := store.SaveSource(ctx, main, src); err != nil {
		t.Fatalf("main SaveSource: %v", err)
	}
	src.Title = "Census 1880 (Revised)"
	if err := store.SaveSource(ctx, main, src); err != nil {
		t.Fatalf("main SaveSource upsert: %v", err)
	}
	if err := store.ReplaceSourceExternalIDs(ctx, main, src.ID, extIDs); err != nil {
		t.Fatalf("main ReplaceSourceExternalIDs: %v", err)
	}
	if err := store.SaveCitation(ctx, main, cit); err != nil {
		t.Fatalf("main SaveCitation: %v", err)
	}
	if err := store.SaveCitation(ctx, main, cit); err != nil {
		t.Fatalf("main SaveCitation upsert: %v", err)
	}
	if err := store.SaveNote(ctx, main, note); err != nil {
		t.Fatalf("main SaveNote: %v", err)
	}
	if err := store.SaveNote(ctx, main, note); err != nil {
		t.Fatalf("main SaveNote upsert: %v", err)
	}
	if got, err := store.GetSource(ctx, main, src.ID); err != nil || got == nil || got.Title != "Census 1880 (Revised)" {
		t.Fatalf("main GetSource = %+v (err=%v), want the upserted title", got, err)
	}
	if got, err := store.SearchSources(ctx, main, "revised", 10); err != nil || len(got) != 1 {
		t.Errorf("main SearchSources = %+v (err=%v), want 1 hit", got, err)
	}
	if got, err := store.GetSourceExternalIDs(ctx, main, src.ID); err != nil || len(got) != 1 {
		t.Errorf("main GetSourceExternalIDs = %+v (err=%v), want 1", got, err)
	}
	if got, err := store.GetNote(ctx, main, note.ID); err != nil || got == nil {
		t.Errorf("main GetNote = %+v (err=%v), want the note", got, err)
	}
	if err := store.DeleteSource(ctx, main, src.ID); err != nil {
		t.Fatalf("main DeleteSource: %v", err)
	}
	if got, err := store.GetCitation(ctx, main, cit.ID); err != nil || got != nil {
		t.Errorf("main GetCitation after DeleteSource = %+v (err=%v), want cascaded", got, err)
	}
	if got, err := store.GetSourceExternalIDs(ctx, main, src.ID); err != nil || len(got) != 0 {
		t.Errorf("main GetSourceExternalIDs after DeleteSource = %+v (err=%v), want cascaded", got, err)
	}
	if err := store.DeleteNote(ctx, main, note.ID); err != nil {
		t.Fatalf("main DeleteNote: %v", err)
	}
	if got, err := store.GetNote(ctx, main, note.ID); err != nil || got != nil {
		t.Errorf("main GetNote after delete = %+v (err=%v), want absent", got, err)
	}
}

// TestPreEvidenceExternalIDKeyIsNotBranchCapable pins the detail that made
// detectBranchCapable look for branch_id itself: source_external_ids was already
// keyed by the composite (source_id, sequence) before #758, so "more than one
// key column" would wrongly call it branch-capable. With only that table left on
// its old key, branch writes must still be refused.
func TestPreEvidenceExternalIDKeyIsNotBranchCapable(t *testing.T) {
	tmpFile, err := os.CreateTemp("", "myfamily-preevidence-exid-*.db")
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

	if _, err := db.Exec(`
		CREATE TABLE source_external_ids (
			source_id TEXT NOT NULL,
			sequence INTEGER NOT NULL,
			value TEXT NOT NULL,
			type TEXT NOT NULL DEFAULT '',
			PRIMARY KEY (source_id, sequence)
		)`); err != nil {
		t.Fatalf("create pre-#758 source_external_ids: %v", err)
	}

	store, err := sqlite.NewReadModelStore(db)
	if err != nil {
		t.Fatalf("create read model store: %v", err)
	}
	ctx := context.Background()
	branch := domain.BranchID(uuid.New())
	if err := store.ReplaceSourceExternalIDs(ctx, branch, uuid.New(), nil); !errors.Is(err, repository.ErrBranchesUnsupported) {
		t.Errorf("branch ReplaceSourceExternalIDs: want ErrBranchesUnsupported, got %v", err)
	}
	// Mainline external IDs keep working on the old key.
	sourceID := uuid.New()
	ids := []repository.SourceExternalIDReadModel{{Value: "A"}, {Value: "B"}}
	if err := store.ReplaceSourceExternalIDs(ctx, domain.MainBranchID, sourceID, ids); err != nil {
		t.Fatalf("main ReplaceSourceExternalIDs: %v", err)
	}
	if got, err := store.GetSourceExternalIDs(ctx, domain.MainBranchID, sourceID); err != nil || len(got) != 2 {
		t.Errorf("main GetSourceExternalIDs = %+v (err=%v), want 2", got, err)
	}
}

// preMediaSQLiteDDL is the media table as it stood before #759: keyed by a lone
// id, with file_data NOT NULL. SQLite cannot re-key it (or relax the NOT NULL)
// in place.
const preMediaSQLiteDDL = `
	CREATE TABLE media (
		id TEXT PRIMARY KEY,
		entity_type TEXT NOT NULL,
		entity_id TEXT NOT NULL,
		title TEXT NOT NULL,
		description TEXT,
		mime_type TEXT NOT NULL,
		media_type TEXT NOT NULL,
		filename TEXT NOT NULL,
		file_size INTEGER NOT NULL,
		file_data BLOB NOT NULL,
		thumbnail_data BLOB,
		crop_left INTEGER,
		crop_top INTEGER,
		crop_width INTEGER,
		crop_height INTEGER,
		gedcom_xref TEXT,
		version INTEGER NOT NULL DEFAULT 1,
		created_at TEXT NOT NULL DEFAULT (datetime('now')),
		updated_at TEXT NOT NULL DEFAULT (datetime('now')),
		files TEXT,
		format TEXT,
		translations TEXT
	);
`

// TestPreMediaBranchSchemaRefusesBranchWrites covers a database built between
// #758 and #759: every other branch-scoped table carries its branch key, but
// media keeps its lone-id key and NOT NULL file_data. Such a database must
// refuse media branch writes (and, like any partially branch-keyed schema,
// every other branch write) with repository.ErrBranchesUnsupported, while
// mainline media keeps working — including a metadata-only update, which hands
// SaveMedia no bytes and must keep the stored ones rather than trip NOT NULL —
// and PurgeBranch still runs.
func TestPreMediaBranchSchemaRefusesBranchWrites(t *testing.T) {
	tmpFile, err := os.CreateTemp("", "myfamily-premedia-readmodel-*.db")
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

	if _, err := db.Exec(preMediaSQLiteDDL); err != nil {
		t.Fatalf("create pre-#759 media table: %v", err)
	}

	store, err := sqlite.NewReadModelStore(db)
	if err != nil {
		t.Fatalf("create read model store: %v", err)
	}
	ctx := context.Background()
	main := domain.MainBranchID
	branch := domain.BranchID(uuid.New())
	now := time.Now()
	owner := uuid.New()
	media := &repository.MediaReadModel{
		ID: uuid.New(), EntityType: "person", EntityID: owner, Title: "Portrait",
		MimeType: "image/jpeg", MediaType: domain.MediaPhoto, Filename: "portrait.jpg",
		FileSize: 4, FileData: []byte("FILE"), ThumbnailData: []byte("THUMB"),
		Version: 1, CreatedAt: now, UpdatedAt: now,
	}

	for name, err := range map[string]error{
		"SaveMedia":   store.SaveMedia(ctx, branch, media),
		"DeleteMedia": store.DeleteMedia(ctx, branch, media.ID),
		"SavePerson":  store.SavePerson(ctx, branch, branchPersonRM(uuid.New(), "Branch", "Row")),
	} {
		if !errors.Is(err, repository.ErrBranchesUnsupported) {
			t.Errorf("branch %s on pre-#759 schema: want ErrBranchesUnsupported, got %v", name, err)
		}
	}

	// Mainline keeps working: insert, a metadata-only update, reads, delete.
	if err := store.SaveMedia(ctx, main, media); err != nil {
		t.Fatalf("main SaveMedia: %v", err)
	}
	meta, err := store.GetMedia(ctx, main, media.ID)
	if err != nil || meta == nil {
		t.Fatalf("main GetMedia = %+v (err=%v)", meta, err)
	}
	meta.Title = "Portrait (retitled)"
	if err := store.SaveMedia(ctx, main, meta); err != nil {
		t.Fatalf("main metadata-only SaveMedia on NOT NULL file_data: %v", err)
	}
	got, err := store.GetMediaWithData(ctx, main, media.ID)
	if err != nil || got == nil || got.Title != "Portrait (retitled)" || string(got.FileData) != "FILE" || string(got.ThumbnailData) != "THUMB" {
		t.Fatalf("main GetMediaWithData after update = %+v (err=%v), want the new title and the kept bytes", got, err)
	}
	if items, total, err := store.ListMediaForEntity(ctx, "person", owner, repository.ListOptions{Limit: 10}); err != nil || total != 1 || len(items) != 1 {
		t.Errorf("main ListMediaForEntity = %d/%d (err=%v), want 1", len(items), total, err)
	}
	if err := store.PurgeBranch(ctx, branch); err != nil {
		t.Errorf("PurgeBranch on pre-#759 schema: %v", err)
	}
	if err := store.DeleteMedia(ctx, main, media.ID); err != nil {
		t.Fatalf("main DeleteMedia: %v", err)
	}
	if got, err := store.GetMedia(ctx, main, media.ID); err != nil || got != nil {
		t.Errorf("main GetMedia after delete = %+v (err=%v), want absent", got, err)
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

	// ...and a branch write now succeeds, seeded from main's current version.
	branch := repository.AppendScope{BranchID: domain.BranchID(uuid.New())}
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
	other := repository.AppendScope{BranchID: domain.BranchID(uuid.New())}
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

// setupPreMergeRecordBranchesDB builds a database carrying the pre-#55 branches
// DDL: no merged_at / merge_note columns. CREATE TABLE IF NOT EXISTS leaves the
// existing definition alone, so NewBranchStore must ADD COLUMN to migrate it.
func setupPreMergeRecordBranchesDB(t *testing.T) (*sql.DB, func()) {
	t.Helper()

	tmpFile, err := os.CreateTemp("", "myfamily-legacy-branches-*.db")
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
		CREATE TABLE branches (
			id TEXT PRIMARY KEY,
			name TEXT NOT NULL,
			description TEXT,
			base_position INTEGER NOT NULL,
			status TEXT NOT NULL,
			created_at TEXT NOT NULL
		)`); err != nil {
		cleanup()
		t.Fatalf("create pre-#55 branches table: %v", err)
	}

	return db, cleanup
}

// TestLegacyBranchesGainMergeColumns verifies the issue #55 migration: an
// existing deployment's branches table gains merged_at / merge_note on open,
// keeps the rows it already had, and can then record a merge. Opening twice
// must not error — the duplicate-column failure is swallowed.
func TestLegacyBranchesGainMergeColumns(t *testing.T) {
	db, cleanup := setupPreMergeRecordBranchesDB(t)
	defer cleanup()
	ctx := context.Background()

	// A branch that predates the migration.
	existing := uuid.New()
	if _, err := db.Exec(`
		INSERT INTO branches (id, name, description, base_position, status, created_at)
		VALUES (?, 'Pre-existing', NULL, 7, 'active', '2026-01-01T00:00:00Z')`,
		existing.String()); err != nil {
		t.Fatalf("seed pre-#55 branch: %v", err)
	}

	// Opening twice must both succeed: the second ADD COLUMN is a duplicate.
	var store *sqlite.BranchStore
	for i := 0; i < 2; i++ {
		var err error
		store, err = sqlite.NewBranchStore(db)
		if err != nil {
			t.Fatalf("NewBranchStore pass %d on pre-#55 database: %v", i, err)
		}
	}

	// The pre-existing row survived and reads back with no merge record.
	got, err := store.Get(ctx, existing)
	if err != nil {
		t.Fatalf("Get pre-existing branch after migration: %v", err)
	}
	if got.Name != "Pre-existing" || got.BasePosition != 7 {
		t.Errorf("migrated branch = %+v, want name=Pre-existing base=7", got)
	}
	if got.MergedAt != nil || got.MergeNote != "" {
		t.Errorf("migrated branch merge fields = %v/%q, want nil/empty", got.MergedAt, got.MergeNote)
	}

	// And the new columns are writable.
	mergedAt := time.Date(2026, 7, 26, 12, 0, 0, 0, time.UTC)
	if err := store.MarkMerged(ctx, existing, mergedAt, "migrated then merged"); err != nil {
		t.Fatalf("MarkMerged after migration: %v", err)
	}
	got, err = store.Get(ctx, existing)
	if err != nil {
		t.Fatalf("Get after MarkMerged: %v", err)
	}
	if got.Status != domain.BranchStatusMerged {
		t.Errorf("status = %s, want merged", got.Status)
	}
	if got.MergedAt == nil || !got.MergedAt.Equal(mergedAt) {
		t.Errorf("MergedAt = %v, want %v", got.MergedAt, mergedAt)
	}
	if got.MergeNote != "migrated then merged" {
		t.Errorf("MergeNote = %q, want the note", got.MergeNote)
	}
}

// preGPSSQLiteDDL is the four GPS artifact tables as they stood before #760:
// keyed by a lone id. SQLite cannot re-key them in place.
const preGPSSQLiteDDL = `
	CREATE TABLE evidence_analyses (
		id TEXT PRIMARY KEY,
		fact_type TEXT NOT NULL,
		subject_id TEXT NOT NULL,
		citation_ids TEXT,
		conclusion TEXT NOT NULL,
		research_status TEXT,
		notes TEXT,
		version INTEGER NOT NULL DEFAULT 1,
		created_at TEXT NOT NULL DEFAULT (datetime('now')),
		updated_at TEXT NOT NULL DEFAULT (datetime('now'))
	);
	CREATE TABLE evidence_conflicts (
		id TEXT PRIMARY KEY,
		fact_type TEXT NOT NULL,
		subject_id TEXT NOT NULL,
		analysis_ids TEXT,
		description TEXT NOT NULL,
		resolution TEXT,
		status TEXT NOT NULL,
		version INTEGER NOT NULL DEFAULT 1,
		created_at TEXT NOT NULL DEFAULT (datetime('now')),
		updated_at TEXT NOT NULL DEFAULT (datetime('now'))
	);
	CREATE TABLE research_logs (
		id TEXT PRIMARY KEY,
		subject_id TEXT NOT NULL,
		subject_type TEXT NOT NULL,
		repository TEXT NOT NULL,
		search_description TEXT NOT NULL,
		outcome TEXT NOT NULL,
		notes TEXT,
		search_date TEXT NOT NULL,
		version INTEGER NOT NULL DEFAULT 1,
		created_at TEXT NOT NULL DEFAULT (datetime('now')),
		updated_at TEXT NOT NULL DEFAULT (datetime('now'))
	);
	CREATE TABLE proof_summaries (
		id TEXT PRIMARY KEY,
		fact_type TEXT NOT NULL,
		subject_id TEXT NOT NULL,
		conclusion TEXT NOT NULL,
		argument TEXT NOT NULL,
		analysis_ids TEXT,
		research_status TEXT,
		version INTEGER NOT NULL DEFAULT 1,
		created_at TEXT NOT NULL DEFAULT (datetime('now')),
		updated_at TEXT NOT NULL DEFAULT (datetime('now'))
	);
`

// TestPreGPSBranchSchemaRefusesBranchWrites covers a database built between
// #759 and #760: every other branch-scoped table carries its branch key, but the
// four GPS artifact tables keep their lone-id keys. Such a database must refuse
// GPS branch writes (and, like any partially branch-keyed schema, every other
// branch write) with repository.ErrBranchesUnsupported, while mainline GPS
// artifacts keep working — insert, update, the filtered lists and the
// DeletePerson cascade — and PurgeBranch still runs.
func TestPreGPSBranchSchemaRefusesBranchWrites(t *testing.T) {
	tmpFile, err := os.CreateTemp("", "myfamily-pregps-readmodel-*.db")
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

	if _, err := db.Exec(preGPSSQLiteDDL); err != nil {
		t.Fatalf("create pre-#760 GPS tables: %v", err)
	}

	store, err := sqlite.NewReadModelStore(db)
	if err != nil {
		t.Fatalf("create read model store: %v", err)
	}
	ctx := context.Background()
	main := domain.MainBranchID
	branch := domain.BranchID(uuid.New())
	now := time.Now().UTC().Truncate(time.Second)
	subject := uuid.New()
	analysis := &repository.EvidenceAnalysisReadModel{ID: uuid.New(), FactType: domain.FactPersonBirth, SubjectID: subject,
		Conclusion: "Born 1815", Version: 1, CreatedAt: now, UpdatedAt: now}
	conflict := &repository.EvidenceConflictReadModel{ID: uuid.New(), FactType: domain.FactPersonBirth, SubjectID: subject,
		Description: "Disagreement", Status: domain.ConflictStatusOpen, Version: 1, CreatedAt: now, UpdatedAt: now}
	log := &repository.ResearchLogReadModel{ID: uuid.New(), SubjectID: subject, SubjectType: "person", Repository: "Archive",
		SearchDescription: "Baptisms", Outcome: domain.ResearchOutcomeFound, SearchDate: now, Version: 1, CreatedAt: now, UpdatedAt: now}
	proof := &repository.ProofSummaryReadModel{ID: uuid.New(), FactType: domain.FactPersonBirth, SubjectID: subject,
		Conclusion: "Born 1815", Argument: "Census", Version: 1, CreatedAt: now, UpdatedAt: now}

	for name, err := range map[string]error{
		"SaveEvidenceAnalysis":   store.SaveEvidenceAnalysis(ctx, branch, analysis),
		"DeleteEvidenceAnalysis": store.DeleteEvidenceAnalysis(ctx, branch, analysis.ID),
		"SaveEvidenceConflict":   store.SaveEvidenceConflict(ctx, branch, conflict),
		"SaveResearchLog":        store.SaveResearchLog(ctx, branch, log),
		"SaveProofSummary":       store.SaveProofSummary(ctx, branch, proof),
		"SavePerson":             store.SavePerson(ctx, branch, branchPersonRM(uuid.New(), "Branch", "Row")),
	} {
		if !errors.Is(err, repository.ErrBranchesUnsupported) {
			t.Errorf("branch %s on pre-#760 schema: want ErrBranchesUnsupported, got %v", name, err)
		}
	}

	// Mainline keeps working: inserts, an update (an upsert on the legacy key),
	// the filtered lists and the subject cascade.
	for name, err := range map[string]error{
		"SaveEvidenceAnalysis": store.SaveEvidenceAnalysis(ctx, main, analysis),
		"SaveEvidenceConflict": store.SaveEvidenceConflict(ctx, main, conflict),
		"SaveResearchLog":      store.SaveResearchLog(ctx, main, log),
		"SaveProofSummary":     store.SaveProofSummary(ctx, main, proof),
	} {
		if err != nil {
			t.Fatalf("main %s on pre-#760 schema: %v", name, err)
		}
	}
	resolved := *conflict
	resolved.Status, resolved.Resolution = domain.ConflictStatusResolved, "Register wins"
	if err := store.SaveEvidenceConflict(ctx, main, &resolved); err != nil {
		t.Fatalf("main update of a conflict on pre-#760 schema: %v", err)
	}
	if got, err := store.ListUnresolvedConflicts(ctx, main); err != nil || len(got) != 0 {
		t.Errorf("main ListUnresolvedConflicts after resolving = %d (err=%v), want 0", len(got), err)
	}
	if got, err := store.GetAnalysesForFact(ctx, main, domain.FactPersonBirth, subject); err != nil || len(got) != 1 {
		t.Errorf("main GetAnalysesForFact = %d (err=%v), want 1", len(got), err)
	}
	if err := store.PurgeBranch(ctx, branch); err != nil {
		t.Errorf("PurgeBranch on pre-#760 schema: %v", err)
	}
	if err := store.DeletePerson(ctx, main, subject); err != nil {
		t.Fatalf("main DeletePerson on pre-#760 schema: %v", err)
	}
	if got, err := store.GetResearchLogsForSubject(ctx, main, subject); err != nil || len(got) != 0 {
		t.Errorf("main research logs after the subject cascade = %d (err=%v), want 0", len(got), err)
	}
	if got, err := store.GetProofSummary(ctx, main, proof.ID); err != nil || got != nil {
		t.Errorf("main proof after the subject cascade = %+v (err=%v), want absent", got, err)
	}
}
