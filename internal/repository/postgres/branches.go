package postgres

import (
	"context"
	"database/sql"
	"fmt"
	"time"

	"github.com/google/uuid"

	"github.com/cacack/my-family/internal/domain"
	"github.com/cacack/my-family/internal/repository"
)

// Compile-time assertion that BranchStore satisfies the interface.
var _ repository.BranchStore = (*BranchStore)(nil)

// BranchStore is a PostgreSQL implementation of repository.BranchStore.
type BranchStore struct {
	db *sql.DB
}

// NewBranchStore creates a new PostgreSQL branch store.
func NewBranchStore(db *sql.DB) (*BranchStore, error) {
	store := &BranchStore{db: db}
	if err := store.createTables(); err != nil {
		return nil, fmt.Errorf("failed to create tables: %w", err)
	}
	return store, nil
}

// createTables creates the branches table if it doesn't exist and brings an
// existing one up to the current shape.
func (s *BranchStore) createTables() error {
	_, err := s.db.Exec(`
		CREATE TABLE IF NOT EXISTS branches (
			id UUID PRIMARY KEY,
			name VARCHAR(100) NOT NULL,
			description VARCHAR(500),
			base_position BIGINT NOT NULL,
			status VARCHAR(20) NOT NULL,
			created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
			merged_at TIMESTAMPTZ,
			merge_note TEXT,
			hypothesis TEXT,
			subjects JSONB,
			outcome VARCHAR(20) NOT NULL DEFAULT 'open',
			proof_summary_ids JSONB,
			closed_at TIMESTAMPTZ,
			close_reason TEXT
		);

		CREATE INDEX IF NOT EXISTS idx_branches_created_at ON branches(created_at DESC);
	`)
	if err != nil {
		return err
	}
	if err := s.migrateMergeColumns(); err != nil {
		return err
	}
	if err := s.migrateResearchColumns(); err != nil {
		return err
	}
	return s.migrateCloseColumns()
}

// migrateCloseColumns adds the close-record columns (#836) to a branches table
// created before they existed. A branch archived before #836 keeps NULLs: its
// BranchDeleted event carried no close record, and a projection rebuild sets
// closed_at from the event's timestamp. Such a branch still reading "open"
// is backfilled as abandoned, matching MarkClosed.
func (s *BranchStore) migrateCloseColumns() error {
	if _, err := s.db.Exec(`
		ALTER TABLE branches ADD COLUMN IF NOT EXISTS closed_at TIMESTAMPTZ;
		ALTER TABLE branches ADD COLUMN IF NOT EXISTS close_reason TEXT;
	`); err != nil {
		return err
	}
	// A branch archived before #836 with no verdict was abandoned: that is
	// what MarkClosed now records for a close without an outcome.
	if _, err := s.db.Exec(`UPDATE branches SET outcome = $1 WHERE status = $2 AND outcome IN ('', $3)`,
		string(domain.BranchOutcomeAbandoned), string(domain.BranchStatusArchived), string(domain.BranchOutcomeOpen)); err != nil {
		return fmt.Errorf("backfill abandoned outcome: %w", err)
	}
	return nil
}

// migrateMergeColumns adds the merge-record columns (issue #55) to a branches
// table created before they existed, so a pre-#55 database gains them on open.
// ADD COLUMN IF NOT EXISTS makes this idempotent across repeated opens.
func (s *BranchStore) migrateMergeColumns() error {
	_, err := s.db.Exec(`
		ALTER TABLE branches ADD COLUMN IF NOT EXISTS merged_at TIMESTAMPTZ;
		ALTER TABLE branches ADD COLUMN IF NOT EXISTS merge_note TEXT;
	`)
	return err
}

// migrateResearchColumns adds the research-record columns (#835) to a
// branches table created before they existed. Existing rows read as an open
// branch with no hypothesis, subjects or proof summaries — exactly what their
// pre-#835 BranchCreated events replay to.
func (s *BranchStore) migrateResearchColumns() error {
	_, err := s.db.Exec(`
		ALTER TABLE branches ADD COLUMN IF NOT EXISTS hypothesis TEXT;
		ALTER TABLE branches ADD COLUMN IF NOT EXISTS subjects JSONB;
		ALTER TABLE branches ADD COLUMN IF NOT EXISTS outcome VARCHAR(20) NOT NULL DEFAULT 'open';
		ALTER TABLE branches ADD COLUMN IF NOT EXISTS proof_summary_ids JSONB;
	`)
	return err
}

// branchScanner is the Scan method shared by *sql.Row and *sql.Rows.
type branchScanner interface {
	Scan(dest ...any) error
}

// scanBranch reads one row selected in Get/List's column order.
func scanBranch(row branchScanner) (*domain.Branch, error) {
	var (
		branchID                  uuid.UUID
		name                      string
		description               sql.NullString
		basePosition              int64
		status                    string
		createdAt                 time.Time
		mergedAt                  sql.NullTime
		mergeNote                 sql.NullString
		hypothesis, outcome       sql.NullString
		subjects, proofSummaryIDs sql.NullString
		closedAt                  sql.NullTime
		closeReason               sql.NullString
	)
	if err := row.Scan(&branchID, &name, &description, &basePosition, &status, &createdAt,
		&mergedAt, &mergeNote, &hypothesis, &outcome, &subjects, &proofSummaryIDs,
		&closedAt, &closeReason); err != nil {
		return nil, err
	}

	research, err := repository.DecodeBranchResearch(hypothesis.String, outcome.String, repository.BranchListsJSON{
		Subjects:        subjects.String,
		ProofSummaryIDs: proofSummaryIDs.String,
	})
	if err != nil {
		return nil, fmt.Errorf("branch %s: %w", branchID, err)
	}

	branch := &domain.Branch{
		ID:           branchID,
		Name:         name,
		Description:  description.String,
		BasePosition: basePosition,
		Status:       domain.BranchStatus(status),
		CreatedAt:    createdAt,
		MergeNote:    mergeNote.String,
		CloseReason:  closeReason.String,
	}
	if mergedAt.Valid {
		branch.MergedAt = &mergedAt.Time
	}
	if closedAt.Valid {
		branch.ClosedAt = &closedAt.Time
	}
	branch.ApplyResearch(research)
	return branch, nil
}

// Create stores a new branch.
func (s *BranchStore) Create(ctx context.Context, branch *domain.Branch) error {
	lists, err := repository.EncodeBranchLists(branch.Research())
	if err != nil {
		return err
	}
	_, err = s.db.ExecContext(ctx, `
		INSERT INTO branches (id, name, description, base_position, status, created_at, merged_at, merge_note,
		                      hypothesis, outcome, subjects, proof_summary_ids, closed_at, close_reason)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, NULLIF($11, '')::JSONB, NULLIF($12, '')::JSONB, $13, $14)
	`,
		branch.ID,
		branch.Name,
		nullableString(branch.Description),
		branch.BasePosition,
		string(branch.Status),
		branch.CreatedAt,
		nullableTime(branch.MergedAt),
		nullableString(branch.MergeNote),
		nullableString(branch.Hypothesis),
		string(branch.Outcome.OrDefault()),
		lists.Subjects,
		lists.ProofSummaryIDs,
		nullableTime(branch.ClosedAt),
		nullableString(branch.CloseReason),
	)
	if err != nil {
		return fmt.Errorf("insert branch: %w", err)
	}
	return nil
}

// Upsert stores a branch, inserting or updating on ID conflict.
func (s *BranchStore) Upsert(ctx context.Context, branch *domain.Branch) error {
	lists, err := repository.EncodeBranchLists(branch.Research())
	if err != nil {
		return err
	}
	_, err = s.db.ExecContext(ctx, `
		INSERT INTO branches (id, name, description, base_position, status, created_at, merged_at, merge_note,
		                      hypothesis, outcome, subjects, proof_summary_ids, closed_at, close_reason)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, NULLIF($11, '')::JSONB, NULLIF($12, '')::JSONB, $13, $14)
		ON CONFLICT (id) DO UPDATE SET
			name = EXCLUDED.name,
			description = EXCLUDED.description,
			base_position = EXCLUDED.base_position,
			status = EXCLUDED.status,
			created_at = EXCLUDED.created_at,
			merged_at = EXCLUDED.merged_at,
			merge_note = EXCLUDED.merge_note,
			hypothesis = EXCLUDED.hypothesis,
			outcome = EXCLUDED.outcome,
			subjects = EXCLUDED.subjects,
			proof_summary_ids = EXCLUDED.proof_summary_ids,
			closed_at = EXCLUDED.closed_at,
			close_reason = EXCLUDED.close_reason
	`,
		branch.ID,
		branch.Name,
		nullableString(branch.Description),
		branch.BasePosition,
		string(branch.Status),
		branch.CreatedAt,
		nullableTime(branch.MergedAt),
		nullableString(branch.MergeNote),
		nullableString(branch.Hypothesis),
		string(branch.Outcome.OrDefault()),
		lists.Subjects,
		lists.ProofSummaryIDs,
		nullableTime(branch.ClosedAt),
		nullableString(branch.CloseReason),
	)
	if err != nil {
		return fmt.Errorf("upsert branch: %w", err)
	}
	return nil
}

// Get retrieves a branch by ID.
func (s *BranchStore) Get(ctx context.Context, id uuid.UUID) (*domain.Branch, error) {
	row := s.db.QueryRowContext(ctx, `
		SELECT id, name, description, base_position, status, created_at, merged_at, merge_note,
		       hypothesis, outcome, subjects::TEXT, proof_summary_ids::TEXT,
		       closed_at, close_reason
		FROM branches
		WHERE id = $1
	`, id)
	branch, err := scanBranch(row)
	if err == sql.ErrNoRows {
		return nil, repository.ErrBranchNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("query branch: %w", err)
	}
	return branch, nil
}

// List retrieves all branches ordered by created_at DESC.
func (s *BranchStore) List(ctx context.Context) ([]*domain.Branch, error) {
	rows, err := s.db.QueryContext(ctx, `
		SELECT id, name, description, base_position, status, created_at, merged_at, merge_note,
		       hypothesis, outcome, subjects::TEXT, proof_summary_ids::TEXT,
		       closed_at, close_reason
		FROM branches
		ORDER BY created_at DESC
	`)
	if err != nil {
		return nil, fmt.Errorf("query branches: %w", err)
	}
	defer rows.Close()

	branches := []*domain.Branch{}
	for rows.Next() {
		branch, err := scanBranch(rows)
		if err != nil {
			return nil, fmt.Errorf("scan branch: %w", err)
		}
		branches = append(branches, branch)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate branches: %w", err)
	}

	return branches, nil
}

// Delete removes a branch by ID.
func (s *BranchStore) Delete(ctx context.Context, id uuid.UUID) error {
	result, err := s.db.ExecContext(ctx, `
		DELETE FROM branches WHERE id = $1
	`, id)
	if err != nil {
		return fmt.Errorf("delete branch: %w", err)
	}

	rowsAffected, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("get rows affected: %w", err)
	}

	if rowsAffected == 0 {
		return repository.ErrBranchNotFound
	}

	return nil
}

// UpdateStatus changes a branch's status.
func (s *BranchStore) UpdateStatus(ctx context.Context, id uuid.UUID, status domain.BranchStatus) error {
	result, err := s.db.ExecContext(ctx, `
		UPDATE branches SET status = $1 WHERE id = $2
	`, string(status), id)
	if err != nil {
		return fmt.Errorf("update branch status: %w", err)
	}

	rowsAffected, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("get rows affected: %w", err)
	}

	if rowsAffected == 0 {
		return repository.ErrBranchNotFound
	}

	return nil
}

// UpdateDetails overwrites the description and research record.
func (s *BranchStore) UpdateDetails(ctx context.Context, id uuid.UUID, description string, research domain.BranchResearch) error {
	lists, err := repository.EncodeBranchLists(research)
	if err != nil {
		return err
	}
	result, err := s.db.ExecContext(ctx, `
		UPDATE branches SET description = $1, hypothesis = $2, outcome = $3,
			subjects = NULLIF($4, '')::JSONB, proof_summary_ids = NULLIF($5, '')::JSONB
		WHERE id = $6
	`, nullableString(description), nullableString(research.Hypothesis), string(research.Outcome.OrDefault()),
		lists.Subjects, lists.ProofSummaryIDs, id)
	if err != nil {
		return fmt.Errorf("update branch details: %w", err)
	}

	rowsAffected, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("get rows affected: %w", err)
	}

	if rowsAffected == 0 {
		return repository.ErrBranchNotFound
	}

	return nil
}

// MarkMerged records the merge: status, timestamp and note in one statement.
func (s *BranchStore) MarkMerged(ctx context.Context, id uuid.UUID, mergedAt time.Time, note string) error {
	result, err := s.db.ExecContext(ctx, `
		UPDATE branches SET status = $1, merged_at = $2, merge_note = $3 WHERE id = $4
	`, string(domain.BranchStatusMerged), mergedAt, nullableString(note), id)
	if err != nil {
		return fmt.Errorf("mark branch merged: %w", err)
	}

	rowsAffected, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("get rows affected: %w", err)
	}

	if rowsAffected == 0 {
		return repository.ErrBranchNotFound
	}

	return nil
}

// MarkClosed records a close: status, timestamp, reason and the outcome in one
// statement. An empty outcome (a pre-#836 close) keeps a stored verdict and
// turns a stored "open" into abandoned.
func (s *BranchStore) MarkClosed(ctx context.Context, id uuid.UUID, closedAt time.Time, outcome domain.BranchOutcome, reason string) error {
	result, err := s.db.ExecContext(ctx, `
		UPDATE branches SET status = $1, closed_at = $2, close_reason = $3,
			outcome = CASE
				WHEN $4::TEXT <> '' THEN $4::TEXT
				WHEN outcome IN ('', $6::TEXT) THEN $7::TEXT
				ELSE outcome
			END
		WHERE id = $5
	`, string(domain.BranchStatusArchived), closedAt, nullableString(reason), string(outcome), id,
		string(domain.BranchOutcomeOpen), string(domain.BranchOutcomeAbandoned))
	if err != nil {
		return fmt.Errorf("mark branch closed: %w", err)
	}

	rowsAffected, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("get rows affected: %w", err)
	}

	if rowsAffected == 0 {
		return repository.ErrBranchNotFound
	}

	return nil
}
