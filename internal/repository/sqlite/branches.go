package sqlite

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

// BranchStore is a SQLite implementation of repository.BranchStore.
type BranchStore struct {
	db *sql.DB
}

// NewBranchStore creates a new SQLite branch store.
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
			id TEXT PRIMARY KEY,
			name TEXT NOT NULL,
			description TEXT,
			base_position INTEGER NOT NULL,
			status TEXT NOT NULL,
			created_at TEXT NOT NULL,
			merged_at TEXT,
			merge_note TEXT,
			hypothesis TEXT,
			subjects TEXT,
			outcome TEXT NOT NULL DEFAULT 'open',
			proof_summary_ids TEXT,
			closed_at TEXT,
			close_reason TEXT
		);

		CREATE INDEX IF NOT EXISTS idx_branches_created_at ON branches(created_at DESC);
	`)
	if err != nil {
		return err
	}
	s.migrateMergeColumns()
	s.migrateResearchColumns()
	return s.migrateCloseColumns()
}

// migrateCloseColumns adds the close-record columns (#836) to a branches table
// created before they existed. As in migrateMergeColumns, the duplicate-column
// error on an already-migrated database is the only failure and is
// intentionally ignored. A branch archived before #836 keeps NULLs; one still
// reading "open" is backfilled as abandoned, matching MarkClosed for a close
// that recorded no outcome.
func (s *BranchStore) migrateCloseColumns() error {
	_, _ = s.db.Exec(`ALTER TABLE branches ADD COLUMN closed_at TEXT`)
	_, _ = s.db.Exec(`ALTER TABLE branches ADD COLUMN close_reason TEXT`)
	if _, err := s.db.Exec(`UPDATE branches SET outcome = ? WHERE status = ? AND outcome IN ('', ?)`,
		string(domain.BranchOutcomeAbandoned), string(domain.BranchStatusArchived), string(domain.BranchOutcomeOpen)); err != nil {
		return fmt.Errorf("backfill abandoned outcome: %w", err)
	}
	return nil
}

// migrateMergeColumns adds the merge-record columns (issue #55) to a branches
// table created before they existed, so a pre-#55 database gains them on open.
// SQLite lacks ADD COLUMN IF NOT EXISTS, so the duplicate-column error on an
// already-migrated database is intentionally swallowed — the same approach as
// EventStore.migrateBranchID. A plain ADD COLUMN is all that is needed here;
// no table rebuild.
func (s *BranchStore) migrateMergeColumns() {
	_, _ = s.db.Exec(`ALTER TABLE branches ADD COLUMN merged_at TEXT`)
	_, _ = s.db.Exec(`ALTER TABLE branches ADD COLUMN merge_note TEXT`)
}

// migrateResearchColumns adds the research-record columns (#835) to a
// branches table created before they existed. As in migrateMergeColumns, the
// duplicate-column error on an already-migrated database is the only failure
// and is intentionally ignored. Existing rows read as an open branch with no
// hypothesis, subjects or proof summaries — exactly what their pre-#835
// BranchCreated events replay to.
func (s *BranchStore) migrateResearchColumns() {
	_, _ = s.db.Exec(`ALTER TABLE branches ADD COLUMN hypothesis TEXT`)
	_, _ = s.db.Exec(`ALTER TABLE branches ADD COLUMN subjects TEXT`)
	_, _ = s.db.Exec(`ALTER TABLE branches ADD COLUMN outcome TEXT NOT NULL DEFAULT 'open'`)
	_, _ = s.db.Exec(`ALTER TABLE branches ADD COLUMN proof_summary_ids TEXT`)
}

// branchScanner is the Scan method shared by *sql.Row and *sql.Rows.
type branchScanner interface {
	Scan(dest ...any) error
}

// scanBranch reads one row selected in Get/List's column order.
func scanBranch(row branchScanner) (*domain.Branch, error) {
	var (
		idStr, name, status, createdAtStr string
		description                       sql.NullString
		basePosition                      int64
		mergedAtStr, mergeNote            sql.NullString
		hypothesis, outcome               sql.NullString
		subjects, proofSummaryIDs         sql.NullString
		closedAtStr, closeReason          sql.NullString
	)
	if err := row.Scan(&idStr, &name, &description, &basePosition, &status, &createdAtStr,
		&mergedAtStr, &mergeNote, &hypothesis, &outcome, &subjects, &proofSummaryIDs,
		&closedAtStr, &closeReason); err != nil {
		return nil, err
	}

	parsedID, err := uuid.Parse(idStr)
	if err != nil {
		return nil, fmt.Errorf("parse branch id: %w", err)
	}

	createdAt, err := parseTimestamp(createdAtStr)
	if err != nil {
		return nil, fmt.Errorf("parse branch created_at %q: %w", createdAtStr, err)
	}

	mergedAt, err := parseNullableTimestamp(mergedAtStr)
	if err != nil {
		return nil, fmt.Errorf("parse branch merged_at %q: %w", mergedAtStr.String, err)
	}

	closedAt, err := parseNullableTimestamp(closedAtStr)
	if err != nil {
		return nil, fmt.Errorf("parse branch closed_at %q: %w", closedAtStr.String, err)
	}

	research, err := repository.DecodeBranchResearch(hypothesis.String, outcome.String, repository.BranchListsJSON{
		Subjects:        subjects.String,
		ProofSummaryIDs: proofSummaryIDs.String,
	})
	if err != nil {
		return nil, fmt.Errorf("branch %s: %w", parsedID, err)
	}

	branch := &domain.Branch{
		ID:           parsedID,
		Name:         name,
		Description:  description.String,
		BasePosition: basePosition,
		Status:       domain.BranchStatus(status),
		CreatedAt:    createdAt,
		MergedAt:     mergedAt,
		MergeNote:    mergeNote.String,
		ClosedAt:     closedAt,
		CloseReason:  closeReason.String,
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
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
	`,
		branch.ID.String(),
		branch.Name,
		nullableString(branch.Description),
		branch.BasePosition,
		string(branch.Status),
		formatTimestamp(branch.CreatedAt),
		nullableTimestamp(branch.MergedAt),
		nullableString(branch.MergeNote),
		nullableString(branch.Hypothesis),
		string(branch.Outcome.OrDefault()),
		nullableString(lists.Subjects),
		nullableString(lists.ProofSummaryIDs),
		nullableTimestamp(branch.ClosedAt),
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
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT (id) DO UPDATE SET
			name = excluded.name,
			description = excluded.description,
			base_position = excluded.base_position,
			status = excluded.status,
			created_at = excluded.created_at,
			merged_at = excluded.merged_at,
			merge_note = excluded.merge_note,
			hypothesis = excluded.hypothesis,
			outcome = excluded.outcome,
			subjects = excluded.subjects,
			proof_summary_ids = excluded.proof_summary_ids,
			closed_at = excluded.closed_at,
			close_reason = excluded.close_reason
	`,
		branch.ID.String(),
		branch.Name,
		nullableString(branch.Description),
		branch.BasePosition,
		string(branch.Status),
		formatTimestamp(branch.CreatedAt),
		nullableTimestamp(branch.MergedAt),
		nullableString(branch.MergeNote),
		nullableString(branch.Hypothesis),
		string(branch.Outcome.OrDefault()),
		nullableString(lists.Subjects),
		nullableString(lists.ProofSummaryIDs),
		nullableTimestamp(branch.ClosedAt),
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
		       hypothesis, outcome, subjects, proof_summary_ids,
		       closed_at, close_reason
		FROM branches
		WHERE id = ?
	`, id.String())
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
		       hypothesis, outcome, subjects, proof_summary_ids,
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
		DELETE FROM branches WHERE id = ?
	`, id.String())
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
		UPDATE branches SET status = ? WHERE id = ?
	`, string(status), id.String())
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
		UPDATE branches SET description = ?, hypothesis = ?, outcome = ?, subjects = ?, proof_summary_ids = ?
		WHERE id = ?
	`, nullableString(description), nullableString(research.Hypothesis), string(research.Outcome.OrDefault()),
		nullableString(lists.Subjects), nullableString(lists.ProofSummaryIDs), id.String())
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
		UPDATE branches SET status = ?, merged_at = ?, merge_note = ? WHERE id = ?
	`, string(domain.BranchStatusMerged), formatTimestamp(mergedAt), nullableString(note), id.String())
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
		UPDATE branches SET status = ?, closed_at = ?, close_reason = ?,
			outcome = CASE
				WHEN ? <> '' THEN ?
				WHEN outcome IN ('', ?) THEN ?
				ELSE outcome
			END
		WHERE id = ?
	`, string(domain.BranchStatusArchived), formatTimestamp(closedAt), nullableString(reason),
		string(outcome), string(outcome), string(domain.BranchOutcomeOpen), string(domain.BranchOutcomeAbandoned),
		id.String())
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
