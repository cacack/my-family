package sqlite

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"slices"

	"github.com/google/uuid"

	"github.com/cacack/my-family/internal/domain"
	"github.com/cacack/my-family/internal/repository"
)

// Batched kinship lookups (#829): the set-based reads the descendancy and
// relationship services walk a tree with, one statement per generation rather
// than one per person. Each resolves the branch overlay in SQL exactly as the
// matching single-row read does. The ids travel as one JSON-array parameter
// unpacked by json_each (see batch_lookup.go), so every statement's text is a
// package constant whatever the set size.

const (
	// pedigreeEdgesByPersonIDsQuery binds (branch, ids JSON, branch, main).
	pedigreeEdgesByPersonIDsQuery = `
		SELECT person_id, father_id, mother_id
		FROM (
			SELECT *, ROW_NUMBER() OVER (PARTITION BY person_id ORDER BY (branch_id = ?) DESC) AS rn
			FROM pedigree_edges
			WHERE person_id IN (SELECT value FROM json_each(?)) AND branch_id IN (?, ?)
		)
		WHERE rn = 1 AND deleted = 0
		ORDER BY person_id`

	// familyChildrenByFamilyIDsQuery binds (branch, ids JSON, branch, main). The
	// rows are put in the cross-backend order of GetFamilyChildrenByFamilyIDs
	// once their names are resolved (repository.CompareFamilyChildren).
	familyChildrenByFamilyIDsQuery = `
		SELECT ` + familyChildSelectCols + `
		FROM (
			SELECT *, ROW_NUMBER() OVER (PARTITION BY family_id, person_id ORDER BY (branch_id = ?) DESC) AS rn
			FROM family_children
			WHERE family_id IN (SELECT value FROM json_each(?)) AND branch_id IN (?, ?)
		)
		WHERE rn = 1 AND deleted = 0`

	// partnerSetFilter narrows the families overlay to those with a partner in
	// the one bound JSON array (bound twice: once per partner column).
	partnerSetFilter = `(partner1_id IN (SELECT value FROM json_each(?)) OR partner2_id IN (SELECT value FROM json_each(?)))`
)

// idsJSON encodes ids as the JSON array json_each unpacks.
func idsJSON(ids []uuid.UUID) (string, error) {
	strs := make([]string, len(ids))
	for i, id := range ids {
		strs[i] = id.String()
	}
	encoded, err := json.Marshal(strs)
	if err != nil {
		return "", err
	}
	return string(encoded), nil
}

// GetPedigreeEdgesByPersonIDs retrieves the edges visible on branchID for
// personIDs, ordered by person id (#829).
func (s *ReadModelStore) GetPedigreeEdgesByPersonIDs(ctx context.Context, branchID domain.BranchID, personIDs []uuid.UUID) ([]repository.PedigreeEdge, error) {
	if len(personIDs) == 0 {
		return nil, nil
	}
	ids, err := idsJSON(personIDs)
	if err != nil {
		return nil, fmt.Errorf("encode pedigree edge ids: %w", err)
	}
	rows, err := s.db.QueryContext(ctx, pedigreeEdgesByPersonIDsQuery,
		branchID.String(), ids, branchID.String(), mainBranchID)
	if err != nil {
		return nil, fmt.Errorf("query pedigree edges by person ids: %w", err)
	}
	defer rows.Close()

	var edges []repository.PedigreeEdge
	for rows.Next() {
		var personID, fatherID, motherID sql.NullString
		if err := rows.Scan(&personID, &fatherID, &motherID); err != nil {
			return nil, fmt.Errorf("scan pedigree edge: %w", err)
		}
		pID, err := uuid.Parse(personID.String)
		if err != nil {
			return nil, fmt.Errorf("parse pedigree edge person id: %w", err)
		}
		edge := repository.PedigreeEdge{PersonID: pID}
		if edge.FatherID, err = parseNullUUID(fatherID); err != nil {
			return nil, fmt.Errorf("parse pedigree edge father id: %w", err)
		}
		if edge.MotherID, err = parseNullUUID(motherID); err != nil {
			return nil, fmt.Errorf("parse pedigree edge mother id: %w", err)
		}
		edges = append(edges, edge)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	if err := repository.ResolvePedigreeEdgeNames(ctx, s.GetPersonsByIDs, branchID, edges); err != nil {
		return nil, err
	}
	return edges, nil
}

// GetFamiliesForPersons retrieves every family visible on branchID in which any
// of personIDs is a partner, each once, ordered by id (#829).
func (s *ReadModelStore) GetFamiliesForPersons(ctx context.Context, branchID domain.BranchID, personIDs []uuid.UUID) ([]repository.FamilyReadModel, error) {
	if len(personIDs) == 0 {
		return nil, nil
	}
	ids, err := idsJSON(personIDs)
	if err != nil {
		return nil, fmt.Errorf("encode family partner ids: %w", err)
	}
	sub, args := factOverlaySubquery("families", partnerSetFilter, []any{ids, ids}, branchID)
	// #nosec G202 -- familySelectCols and sub are built from package constants; every value is a bound ? placeholder
	// nosemgrep: go.lang.security.audit.database.string-formatted-query.string-formatted-query
	rows, err := s.db.QueryContext(ctx, "SELECT "+familySelectCols+" FROM "+sub+" r ORDER BY id", args...)
	if err != nil {
		return nil, fmt.Errorf("query families for persons: %w", err)
	}
	defer rows.Close()
	families, err := drainRows(rows, scanFamilyRow)
	if err != nil {
		return nil, fmt.Errorf("scan families for persons: %w", err)
	}
	return s.withFamilyNames(ctx, branchID, families)
}

// GetFamilyChildrenByFamilyIDs retrieves the child links visible on branchID of
// every family in familyIDs (#829), in the order the interface documents.
func (s *ReadModelStore) GetFamilyChildrenByFamilyIDs(ctx context.Context, branchID domain.BranchID, familyIDs []uuid.UUID) ([]repository.FamilyChildReadModel, error) {
	if len(familyIDs) == 0 {
		return nil, nil
	}
	ids, err := idsJSON(familyIDs)
	if err != nil {
		return nil, fmt.Errorf("encode family child ids: %w", err)
	}
	rows, err := s.db.QueryContext(ctx, familyChildrenByFamilyIDsQuery,
		branchID.String(), ids, branchID.String(), mainBranchID)
	if err != nil {
		return nil, fmt.Errorf("query family children by family ids: %w", err)
	}
	defer rows.Close()
	children, err := scanFamilyChildRows(rows)
	if err != nil {
		return nil, err
	}
	if children, err = s.withFamilyChildNames(ctx, branchID, children); err != nil {
		return nil, err
	}
	slices.SortStableFunc(children, repository.CompareFamilyChildren)
	return children, nil
}

// parseNullUUID parses an optional uuid column; NULL is nil.
func parseNullUUID(v sql.NullString) (*uuid.UUID, error) {
	if !v.Valid {
		return nil, nil
	}
	id, err := uuid.Parse(v.String)
	if err != nil {
		return nil, err
	}
	return &id, nil
}
