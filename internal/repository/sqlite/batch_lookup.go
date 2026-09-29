package sqlite

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"

	"github.com/google/uuid"

	"github.com/cacack/my-family/internal/domain"
	"github.com/cacack/my-family/internal/repository"
)

// Batched id lookups (#697). Each resolves the branch overlay for a whole set of
// ids in ONE statement — the same factOverlaySubquery the list reads use, narrowed
// by idSetFilter — so the answer matches N single-row Get calls without N round
// trips. The ids travel as a single JSON-array parameter unpacked by json_each, so
// the SQL text is a package constant whatever the set size and no bind-variable
// limit applies.

const (
	// personSelectCols is scanPerson's column order (unaliased).
	personSelectCols = `id, given_name, surname, full_name, gender,
		birth_date_raw, birth_date_sort, birth_place, birth_place_lat, birth_place_long,
		death_date_raw, death_date_sort, death_place, death_place_lat, death_place_long,
		notes, research_status, brick_wall_note, brick_wall_since, brick_wall_resolved_at,
		version, updated_at`

	// familySelectCols is scanFamily's column order (unaliased).
	familySelectCols = `id, partner1_id, partner1_given_name, partner1_surname,
		partner2_id, partner2_given_name, partner2_surname,
		relationship_type, marriage_date_raw, marriage_date_sort, marriage_place,
		marriage_place_lat, marriage_place_long,
		child_count, version, updated_at`

	// idSetFilter narrows an overlay subquery to the ids in its one bound JSON
	// array. Off main factOverlaySubquery applies it both to the candidate ids and
	// to the winning row; an id never changes across branches, so both agree.
	idSetFilter = `id IN (SELECT value FROM json_each(?))`
)

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
	idsJSON, err := json.Marshal(strs)
	if err != nil {
		return nil, fmt.Errorf("encode %s ids: %w", table, err)
	}
	sub, args := factOverlaySubquery(table, idSetFilter, []any{string(idsJSON)}, branchID)
	// #nosec G202 -- table, cols and sub are built from package constants; every value is a bound ? placeholder
	// nosemgrep: go.lang.security.audit.database.string-formatted-query.string-formatted-query
	rows, err := s.db.QueryContext(ctx, "SELECT "+cols+" FROM "+sub+" r ORDER BY id", args...)
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
	return queryByIDs(ctx, s, "persons", personSelectCols, branchID, ids, scanPersonRows)
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
// JSON array (see idSetFilter).
const sourceSetFilter = `source_id IN (SELECT value FROM json_each(?))`

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
	idsJSON, err := json.Marshal(strs)
	if err != nil {
		return nil, fmt.Errorf("encode source ids: %w", err)
	}
	sub, args := overlayColsSubquery("citations", "id, source_id", sourceSetFilter, []any{string(idsJSON)}, branchID)
	// #nosec G202 -- sub is built from package constants; every value is a bound ? placeholder
	// nosemgrep: go.lang.security.audit.database.string-formatted-query.string-formatted-query
	rows, err := s.db.QueryContext(ctx, "SELECT source_id, COUNT(*) FROM "+sub+" c GROUP BY source_id", args...)
	if err != nil {
		return nil, fmt.Errorf("count citations by source: %w", err)
	}
	defer rows.Close()
	for rows.Next() {
		var (
			raw   string
			count int
		)
		if err := rows.Scan(&raw, &count); err != nil {
			return nil, fmt.Errorf("scan citation count: %w", err)
		}
		sourceID, err := uuid.Parse(raw)
		if err != nil {
			return nil, fmt.Errorf("parse citation source id %q: %w", raw, err)
		}
		counts[sourceID] = count
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("count citations by source: %w", err)
	}
	return counts, nil
}
