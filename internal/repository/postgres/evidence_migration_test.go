package postgres_test

import (
	"context"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/cacack/my-family/internal/domain"
	"github.com/cacack/my-family/internal/repository"
	pgstore "github.com/cacack/my-family/internal/repository/postgres"
)

// preEvidenceBranchDDL is the evidence schema as it stood before #758: sources,
// citations and notes keyed by a lone id, source_external_ids keyed by
// (source_id, sequence), and both source children holding a foreign key to
// sources(id). runBranchMigration must drop those foreign keys (sources(id) stops
// being unique) and bring every table to a branch_id-bearing key in place,
// keeping its rows.
const preEvidenceBranchDDL = `
	CREATE TABLE sources (
		id UUID PRIMARY KEY,
		source_type VARCHAR(50) NOT NULL,
		title VARCHAR(500) NOT NULL,
		author VARCHAR(200),
		publisher VARCHAR(200),
		publish_date_raw VARCHAR(100),
		publish_date_sort DATE,
		url VARCHAR(500),
		repository_id UUID,
		repository_name VARCHAR(200),
		collection_name VARCHAR(200),
		call_number VARCHAR(100),
		notes TEXT,
		gedcom_xref VARCHAR(50),
		citation_count INTEGER NOT NULL DEFAULT 0,
		version BIGINT NOT NULL DEFAULT 1,
		updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
	);
	CREATE TABLE citations (
		id UUID PRIMARY KEY,
		source_id UUID NOT NULL REFERENCES sources(id),
		source_title VARCHAR(500),
		fact_type VARCHAR(100) NOT NULL,
		fact_owner_id UUID NOT NULL,
		page VARCHAR(100),
		volume VARCHAR(50),
		source_quality VARCHAR(20),
		informant_type VARCHAR(20),
		evidence_type VARCHAR(20),
		quoted_text TEXT,
		analysis TEXT,
		template_id VARCHAR(100),
		fields_data JSONB,
		gedcom_xref VARCHAR(50),
		version BIGINT NOT NULL DEFAULT 1,
		created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
	);
	CREATE TABLE notes (
		id UUID PRIMARY KEY,
		text TEXT NOT NULL,
		mime VARCHAR(100),
		language VARCHAR(35),
		translations JSONB,
		gedcom_xref VARCHAR(50),
		version BIGINT NOT NULL DEFAULT 1,
		updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
	);
	CREATE TABLE source_external_ids (
		source_id UUID NOT NULL REFERENCES sources(id) ON DELETE CASCADE,
		sequence INTEGER NOT NULL,
		value TEXT NOT NULL,
		type TEXT NOT NULL DEFAULT '',
		PRIMARY KEY (source_id, sequence)
	);
`

// TestReadModelStore_MigratesEvidenceTablesToBranchKeys covers the in-place
// upgrade of a database created before #758: the four evidence tables gain
// branch_id + deleted and a branch_id-bearing primary key, the foreign keys to
// sources(id) are dropped, their rows land on main, branch shadow rows become
// writable, and reopening is a no-op.
func TestReadModelStore_MigratesEvidenceTablesToBranchKeys(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test")
	}

	db, cleanup := setupPostgres(t)
	defer cleanup()

	if _, err := db.Exec(preEvidenceBranchDDL); err != nil {
		t.Fatalf("create pre-#758 evidence tables: %v", err)
	}
	sourceID, citationID, noteID, ownerID := uuid.New(), uuid.New(), uuid.New(), uuid.New()
	if _, err := db.Exec(`
		INSERT INTO sources (id, source_type, title, citation_count) VALUES ($1, 'census', 'Census 1880', 1)
	`, sourceID); err != nil {
		t.Fatalf("seed source: %v", err)
	}
	if _, err := db.Exec(`
		INSERT INTO citations (id, source_id, source_title, fact_type, fact_owner_id, page)
		VALUES ($1, $2, 'Census 1880', $3, $4, '12')
	`, citationID, sourceID, string(domain.FactPersonBirth), ownerID); err != nil {
		t.Fatalf("seed citation: %v", err)
	}
	if _, err := db.Exec(`INSERT INTO notes (id, text) VALUES ($1, 'Seen in the register')`, noteID); err != nil {
		t.Fatalf("seed note: %v", err)
	}
	if _, err := db.Exec(`
		INSERT INTO source_external_ids (source_id, sequence, value, type) VALUES ($1, 0, 'MAIN-1', 'http://example.org/ids')
	`, sourceID); err != nil {
		t.Fatalf("seed source external id: %v", err)
	}

	store, err := pgstore.NewReadModelStore(db)
	if err != nil {
		t.Fatalf("create read model store: %v", err)
	}
	for _, table := range []string{"sources", "source_external_ids", "citations", "notes"} {
		if !primaryKeyHasBranch(t, db, table) {
			t.Errorf("%s primary key was not migrated to include branch_id", table)
		}
		if !hasColumn(t, db, table, "deleted") {
			t.Errorf("%s gained no deleted column", table)
		}
	}
	var fks int
	if err := db.QueryRow(`
		SELECT COUNT(*) FROM pg_constraint
		WHERE contype = 'f' AND confrelid = 'sources'::regclass
	`).Scan(&fks); err != nil {
		t.Fatalf("count foreign keys to sources: %v", err)
	}
	if fks != 0 {
		t.Errorf("%d foreign keys still reference sources(id), want 0", fks)
	}

	ctx := context.Background()
	main := domain.MainBranchID
	src, err := store.GetSource(ctx, main, sourceID)
	if err != nil || src == nil || src.Title != "Census 1880" || src.CitationCount != 1 {
		t.Fatalf("migrated source on main = %+v (err=%v)", src, err)
	}
	if cit, err := store.GetCitation(ctx, main, citationID); err != nil || cit == nil || cit.Page != "12" {
		t.Fatalf("migrated citation on main = %+v (err=%v)", cit, err)
	}
	if note, err := store.GetNote(ctx, main, noteID); err != nil || note == nil || note.Text != "Seen in the register" {
		t.Fatalf("migrated note on main = %+v (err=%v)", note, err)
	}
	if ids, err := store.GetSourceExternalIDs(ctx, main, sourceID); err != nil || len(ids) != 1 || ids[0].Value != "MAIN-1" {
		t.Fatalf("migrated source external ids on main = %+v (err=%v)", ids, err)
	}

	// Branch shadows of the migrated rows are now representable, including a
	// branch-only citation of the migrated source (no foreign key to trip over).
	branch := domain.BranchID(uuid.New())
	shadow := *src
	shadow.Title = "Census 1880 (Revised)"
	shadow.UpdatedAt = time.Now()
	if err := store.SaveSource(ctx, branch, &shadow); err != nil {
		t.Fatalf("branch SaveSource on migrated table: %v", err)
	}
	if err := store.ReplaceSourceExternalIDs(ctx, branch, sourceID, []repository.SourceExternalIDReadModel{{Value: "BRANCH-1"}}); err != nil {
		t.Fatalf("branch ReplaceSourceExternalIDs on migrated table: %v", err)
	}
	if err := store.SaveCitation(ctx, branch, &repository.CitationReadModel{
		ID: uuid.New(), SourceID: sourceID, SourceTitle: shadow.Title, FactType: domain.FactPersonDeath,
		FactOwnerID: ownerID, Version: 1, CreatedAt: time.Now(),
	}); err != nil {
		t.Fatalf("branch SaveCitation on migrated table: %v", err)
	}
	if got, err := store.GetSource(ctx, branch, sourceID); err != nil || got == nil || got.Title != shadow.Title {
		t.Errorf("branch GetSource = %+v (err=%v), want the revised shadow", got, err)
	}
	if got, err := store.GetSource(ctx, main, sourceID); err != nil || got == nil || got.Title != "Census 1880" {
		t.Errorf("main GetSource after branch write = %+v (err=%v), want Census 1880", got, err)
	}

	// Deleting the source on main now cascades in code (the FK is gone).
	if err := store.DeleteSource(ctx, main, sourceID); err != nil {
		t.Fatalf("main DeleteSource on migrated table: %v", err)
	}
	if got, err := store.GetCitation(ctx, main, citationID); err != nil || got != nil {
		t.Errorf("main citation after DeleteSource = %+v (err=%v), want cascaded", got, err)
	}

	// Reopening is a no-op that keeps the branch rows.
	store2, err := pgstore.NewReadModelStore(db)
	if err != nil {
		t.Fatalf("reopen read model store: %v", err)
	}
	if _, total, err := store2.ListCitations(ctx, repository.ListOptions{Limit: 10, BranchID: branch}); err != nil || total != 1 {
		t.Errorf("branch ListCitations after reopen: total=%d err=%v, want 1 (the branch-only citation)", total, err)
	}
	if ids, err := store2.GetSourceExternalIDs(ctx, branch, sourceID); err != nil || len(ids) != 1 || ids[0].Value != "BRANCH-1" {
		t.Errorf("branch GetSourceExternalIDs after reopen = %+v (err=%v), want [BRANCH-1]", ids, err)
	}
}
