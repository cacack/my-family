package postgres

import (
	"context"
	"database/sql"
	"fmt"

	"github.com/google/uuid"
	"github.com/lib/pq"

	"github.com/cacack/my-family/internal/domain"
	"github.com/cacack/my-family/internal/repository"
)

// Batched id lookups (#697). Each resolves the branch overlay for a whole set of
// ids in ONE statement — the same overlaySrc the list reads use, narrowed by
// idSetFilter — so the answer matches N single-row Get calls without N round
// trips. The ids travel as a single uuid[] parameter, so the SQL text does not
// grow with the set and no bind-parameter limit applies.

// idSetFilter narrows an overlay source to the ids in one bound uuid[]. %[1]d is
// its placeholder number, which the caller binds after overlayArgs. Off main
// overlaySrc applies it both to the candidate ids and to the winning row; an id
// never changes across branches, so both agree.
const idSetFilter = `id = ANY($%[1]d::uuid[])`

// queryByIDs runs the batched overlay lookup of table for ids on branchID,
// projected to cols and ordered by id, and hands the rows to scan. table and cols
// must be package constants. An empty id set returns nil without querying.
func queryByIDs[T any](ctx context.Context, s *ReadModelStore, table, cols string, branchID domain.BranchID, ids []uuid.UUID, scan func(*sql.Rows) ([]T, error)) ([]T, error) {
	if len(ids) == 0 {
		return nil, nil
	}
	strs := make([]string, len(ids))
	for i, id := range ids {
		strs[i] = id.String()
	}
	args, n := overlayArgs(branchID)
	src := overlaySrc(table, cols, fmt.Sprintf(idSetFilter, n), branchID)
	// #nosec G202 -- table, cols and src are built from package constants carrying only $-placeholders
	// nosemgrep: go.lang.security.audit.database.string-formatted-query.string-formatted-query
	rows, err := s.db.QueryContext(ctx, "SELECT "+cols+" FROM "+src+" r ORDER BY id", append(args, pq.Array(strs))...)
	if err != nil {
		return nil, fmt.Errorf("query %s by ids: %w", table, err)
	}
	defer rows.Close()
	result, err := scan(rows)
	if err != nil {
		return nil, fmt.Errorf("scan %s by ids: %w", table, err)
	}
	return result, nil
}

// drainRows scans every row with scanOne.
func drainRows[T any](rows *sql.Rows, scanOne func(*sql.Rows) (*T, error)) ([]T, error) {
	var result []T
	for rows.Next() {
		row, err := scanOne(rows)
		if err != nil {
			return nil, err
		}
		result = append(result, *row)
	}
	return result, rows.Err()
}

// GetPersonsByIDs retrieves every visible person among ids on branchID (#697).
func (s *ReadModelStore) GetPersonsByIDs(ctx context.Context, branchID domain.BranchID, ids []uuid.UUID) ([]repository.PersonReadModel, error) {
	return queryByIDs(ctx, s, "persons", personSelectCols, branchID, ids, func(rows *sql.Rows) ([]repository.PersonReadModel, error) {
		return drainRows(rows, scanPersonRow)
	})
}

// GetFamiliesByIDs retrieves every visible family among ids on branchID (#697).
func (s *ReadModelStore) GetFamiliesByIDs(ctx context.Context, branchID domain.BranchID, ids []uuid.UUID) ([]repository.FamilyReadModel, error) {
	return queryByIDs(ctx, s, "families", familySelectCols, branchID, ids, func(rows *sql.Rows) ([]repository.FamilyReadModel, error) {
		return drainRows(rows, scanFamilyRow)
	})
}

// GetSourcesByIDs retrieves every visible source among ids on branchID (#697).
func (s *ReadModelStore) GetSourcesByIDs(ctx context.Context, branchID domain.BranchID, ids []uuid.UUID) ([]repository.SourceReadModel, error) {
	return queryByIDs(ctx, s, "sources", sourceSelectCols, branchID, ids, scanSources)
}

// GetCitationsByIDs retrieves every visible citation among ids on branchID (#697).
func (s *ReadModelStore) GetCitationsByIDs(ctx context.Context, branchID domain.BranchID, ids []uuid.UUID) ([]repository.CitationReadModel, error) {
	return queryByIDs(ctx, s, "citations", citationSelectCols, branchID, ids, scanCitations)
}

// sourceSetFilter narrows the citations overlay to the sources in one bound
// uuid[]; %[1]d is its placeholder number (see idSetFilter).
const sourceSetFilter = `source_id = ANY($%[1]d::uuid[])`

// CountCitationsBySource counts the visible citations of each of sourceIDs on
// branchID in one grouped statement (see the interface).
func (s *ReadModelStore) CountCitationsBySource(ctx context.Context, branchID domain.BranchID, sourceIDs []uuid.UUID) (map[uuid.UUID]int, error) {
	counts := make(map[uuid.UUID]int)
	if len(sourceIDs) == 0 {
		return counts, nil
	}
	strs := make([]string, len(sourceIDs))
	for i, id := range sourceIDs {
		strs[i] = id.String()
	}
	args, n := overlayArgs(branchID)
	src := overlaySrc("citations", "id, source_id", fmt.Sprintf(sourceSetFilter, n), branchID)
	// #nosec G202 -- src is built from package constants carrying only $-placeholders
	// nosemgrep: go.lang.security.audit.database.string-formatted-query.string-formatted-query
	rows, err := s.db.QueryContext(ctx, "SELECT source_id, COUNT(*) FROM "+src+" c GROUP BY source_id", append(args, pq.Array(strs))...)
	if err != nil {
		return nil, fmt.Errorf("count citations by source: %w", err)
	}
	defer rows.Close()
	for rows.Next() {
		var (
			sourceID uuid.UUID
			count    int
		)
		if err := rows.Scan(&sourceID, &count); err != nil {
			return nil, fmt.Errorf("scan citation count: %w", err)
		}
		counts[sourceID] = count
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("count citations by source: %w", err)
	}
	return counts, nil
}
