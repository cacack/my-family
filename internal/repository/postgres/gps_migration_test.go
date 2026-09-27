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

// preGPSBranchDDL is the four GPS artifact tables as they stood before #760:
// keyed by a lone id, with no branch_id or deleted column. runBranchMigration
// must bring each to an (id, branch_id) key with a deleted tombstone, keeping
// its rows on main.
const preGPSBranchDDL = `
	CREATE TABLE evidence_analyses (
		id UUID PRIMARY KEY,
		fact_type VARCHAR(50) NOT NULL,
		subject_id UUID NOT NULL,
		citation_ids JSONB,
		conclusion TEXT NOT NULL,
		research_status VARCHAR(20),
		notes TEXT,
		version BIGINT NOT NULL DEFAULT 1,
		created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
		updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
	);
	CREATE TABLE evidence_conflicts (
		id UUID PRIMARY KEY,
		fact_type VARCHAR(50) NOT NULL,
		subject_id UUID NOT NULL,
		analysis_ids JSONB,
		description TEXT NOT NULL,
		resolution TEXT,
		status VARCHAR(20) NOT NULL,
		version BIGINT NOT NULL DEFAULT 1,
		created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
		updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
	);
	CREATE TABLE research_logs (
		id UUID PRIMARY KEY,
		subject_id UUID NOT NULL,
		subject_type VARCHAR(20) NOT NULL,
		repository VARCHAR(255) NOT NULL,
		search_description TEXT NOT NULL,
		outcome VARCHAR(20) NOT NULL,
		notes TEXT,
		search_date TIMESTAMPTZ NOT NULL,
		version BIGINT NOT NULL DEFAULT 1,
		created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
		updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
	);
	CREATE TABLE proof_summaries (
		id UUID PRIMARY KEY,
		fact_type VARCHAR(50) NOT NULL,
		subject_id UUID NOT NULL,
		conclusion TEXT NOT NULL,
		argument TEXT NOT NULL,
		analysis_ids JSONB,
		research_status VARCHAR(20),
		version BIGINT NOT NULL DEFAULT 1,
		created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
		updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
	);
`

// TestReadModelStore_MigratesGPSArtifactsToBranchKeys covers the in-place
// upgrade of a database created before #760: the four GPS tables gain branch_id
// + deleted and a branch_id-bearing primary key, their rows land on main, a
// branch resolution of a migrated conflict then shadows it on the branch only
// (ListUnresolvedConflicts resolves before it filters), and reopening is a no-op.
func TestReadModelStore_MigratesGPSArtifactsToBranchKeys(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test")
	}

	db, cleanup := setupPostgres(t)
	defer cleanup()

	if _, err := db.Exec(preGPSBranchDDL); err != nil {
		t.Fatalf("create pre-#760 GPS tables: %v", err)
	}
	subject := uuid.New()
	analysisID, conflictID, logID, proofID := uuid.New(), uuid.New(), uuid.New(), uuid.New()
	for _, seed := range []struct {
		sql  string
		args []any
	}{
		{`INSERT INTO evidence_analyses (id, fact_type, subject_id, conclusion) VALUES ($1, 'person_birth', $2, 'Born 1815')`, []any{analysisID, subject}},
		{`INSERT INTO evidence_conflicts (id, fact_type, subject_id, description, status) VALUES ($1, 'person_birth', $2, 'Disagreement', 'open')`, []any{conflictID, subject}},
		{`INSERT INTO research_logs (id, subject_id, subject_type, repository, search_description, outcome, search_date)
		  VALUES ($1, $2, 'person', 'Archive', 'Baptisms', 'not_found', NOW())`, []any{logID, subject}},
		{`INSERT INTO proof_summaries (id, fact_type, subject_id, conclusion, argument) VALUES ($1, 'person_birth', $2, 'Born 1815', 'Census')`, []any{proofID, subject}},
	} {
		if _, err := db.Exec(seed.sql, seed.args...); err != nil {
			t.Fatalf("seed: %v", err)
		}
	}

	store, err := pgstore.NewReadModelStore(db)
	if err != nil {
		t.Fatalf("create read model store: %v", err)
	}
	for _, table := range []string{"evidence_analyses", "evidence_conflicts", "research_logs", "proof_summaries"} {
		if !primaryKeyHasBranch(t, db, table) {
			t.Errorf("%s primary key was not migrated to include branch_id", table)
		}
		if !hasColumn(t, db, table, "deleted") {
			t.Errorf("%s gained no deleted column", table)
		}
	}

	ctx := context.Background()
	main := domain.MainBranchID
	if got, err := store.GetEvidenceAnalysis(ctx, main, analysisID); err != nil || got == nil || got.Conclusion != "Born 1815" {
		t.Errorf("migrated analysis on main = %+v (err=%v)", got, err)
	}
	if got, err := store.GetResearchLog(ctx, main, logID); err != nil || got == nil || got.Repository != "Archive" {
		t.Errorf("migrated research log on main = %+v (err=%v)", got, err)
	}
	if got, err := store.GetProofSummary(ctx, main, proofID); err != nil || got == nil || got.Argument != "Census" {
		t.Errorf("migrated proof summary on main = %+v (err=%v)", got, err)
	}
	conflict, err := store.GetEvidenceConflict(ctx, main, conflictID)
	if err != nil || conflict == nil {
		t.Fatalf("migrated conflict on main = %+v (err=%v)", conflict, err)
	}

	// A branch resolution of the migrated conflict shadows it on the branch only.
	branch := domain.BranchID(uuid.New())
	resolved := *conflict
	resolved.Status, resolved.Resolution, resolved.UpdatedAt = domain.ConflictStatusResolved, "Register wins", time.Now()
	if err := store.SaveEvidenceConflict(ctx, branch, &resolved); err != nil {
		t.Fatalf("branch SaveEvidenceConflict on migrated table: %v", err)
	}
	openOn := func(b domain.BranchID) []repository.EvidenceConflictReadModel {
		t.Helper()
		got, err := store.ListUnresolvedConflicts(ctx, b)
		if err != nil {
			t.Fatalf("ListUnresolvedConflicts: %v", err)
		}
		return got
	}
	if got := openOn(main); len(got) != 1 {
		t.Errorf("main open conflicts = %d, want 1", len(got))
	}
	if got := openOn(branch); len(got) != 0 {
		t.Errorf("branch open conflicts = %d, want 0", len(got))
	}

	// Reopening is a no-op that keeps the branch row.
	store2, err := pgstore.NewReadModelStore(db)
	if err != nil {
		t.Fatalf("reopen read model store: %v", err)
	}
	if got, err := store2.GetEvidenceConflict(ctx, branch, conflictID); err != nil || got == nil || got.Status != domain.ConflictStatusResolved {
		t.Errorf("branch GetEvidenceConflict after reopen = %+v (err=%v), want the resolved shadow", got, err)
	}
}
