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

// Batched kinship lookups (#829): the set-based reads the descendancy and
// relationship services walk a tree with, one statement per generation rather
// than one per person. Each resolves the branch overlay in SQL exactly as the
// matching single-row read does. The ids travel as one uuid[] parameter (see
// batch_lookup.go), so every statement's text is a package constant whatever
// the set size.

const (
	// pedigreeEdgesByPersonIDsQuery binds (branch, main, ids). On main the two
	// branch parameters are equal and the query degenerates to main's rows.
	pedigreeEdgesByPersonIDsQuery = `
		SELECT ` + pedigreeSelectCols + ` FROM (
			SELECT DISTINCT ON (person_id) ` + pedigreeSelectCols + `, deleted
			FROM pedigree_edges WHERE person_id = ANY($3::uuid[]) AND branch_id IN ($1, $2)
			ORDER BY person_id, (branch_id = $1) DESC
		) o WHERE NOT deleted
		ORDER BY person_id`

	// familyChildrenByFamilyIDsQuery binds (branch, main, ids). The order is the
	// cross-backend contract of GetFamilyChildrenByFamilyIDs; the "C" collation
	// makes the name tie-break bytewise, as it is on SQLite and in memory.
	familyChildrenByFamilyIDsQuery = `
		SELECT ` + familyChildSelectCols + ` FROM (
			SELECT DISTINCT ON (family_id, person_id) ` + familyChildSelectCols + `, deleted
			FROM family_children WHERE family_id = ANY($3::uuid[]) AND branch_id IN ($1, $2)
			ORDER BY family_id, person_id, (branch_id = $1) DESC
		) o WHERE NOT deleted
		ORDER BY family_id, sequence NULLS LAST,
			COALESCE(person_surname, '') COLLATE "C", COALESCE(person_given_name, '') COLLATE "C", person_id`

	// partnerSetFilter narrows the families overlay to those with a partner in
	// one bound uuid[]. %[1]d is its placeholder number (see idSetFilter).
	partnerSetFilter = `(partner1_id = ANY($%[1]d::uuid[]) OR partner2_id = ANY($%[1]d::uuid[]))`
)

// uuidArray binds ids as one uuid[] parameter.
func uuidArray(ids []uuid.UUID) any {
	strs := make([]string, len(ids))
	for i, id := range ids {
		strs[i] = id.String()
	}
	return pq.Array(strs)
}

// GetPedigreeEdgesByPersonIDs retrieves the edges visible on branchID for
// personIDs, ordered by person id (#829).
func (s *ReadModelStore) GetPedigreeEdgesByPersonIDs(ctx context.Context, branchID domain.BranchID, personIDs []uuid.UUID) ([]repository.PedigreeEdge, error) {
	if len(personIDs) == 0 {
		return nil, nil
	}
	rows, err := s.db.QueryContext(ctx, pedigreeEdgesByPersonIDsQuery,
		branchID.UUID(), domain.MainBranchID.UUID(), uuidArray(personIDs))
	if err != nil {
		return nil, fmt.Errorf("query pedigree edges by person ids: %w", err)
	}
	defer rows.Close()

	var edges []repository.PedigreeEdge
	for rows.Next() {
		var (
			edge               repository.PedigreeEdge
			fatherID, motherID uuid.NullUUID
			fatherName         sql.NullString
			motherName         sql.NullString
		)
		if err := rows.Scan(&edge.PersonID, &fatherID, &motherID, &fatherName, &motherName); err != nil {
			return nil, fmt.Errorf("scan pedigree edge: %w", err)
		}
		if fatherID.Valid {
			edge.FatherID = &fatherID.UUID
		}
		if motherID.Valid {
			edge.MotherID = &motherID.UUID
		}
		edge.FatherName = fatherName.String
		edge.MotherName = motherName.String
		edges = append(edges, edge)
	}
	return edges, rows.Err()
}

// GetFamiliesForPersons retrieves every family visible on branchID in which any
// of personIDs is a partner, each once, ordered by id (#829).
func (s *ReadModelStore) GetFamiliesForPersons(ctx context.Context, branchID domain.BranchID, personIDs []uuid.UUID) ([]repository.FamilyReadModel, error) {
	if len(personIDs) == 0 {
		return nil, nil
	}
	args, n := overlayArgs(branchID)
	src := overlaySrc("families", familySelectCols, fmt.Sprintf(partnerSetFilter, n), branchID)
	// #nosec G202 -- familySelectCols and src are built from package constants carrying only $-placeholders
	// nosemgrep: go.lang.security.audit.database.string-formatted-query.string-formatted-query
	rows, err := s.db.QueryContext(ctx, "SELECT "+familySelectCols+" FROM "+src+" r ORDER BY id",
		append(args, uuidArray(personIDs))...)
	if err != nil {
		return nil, fmt.Errorf("query families for persons: %w", err)
	}
	defer rows.Close()
	families, err := drainRows(rows, scanFamilyRow)
	if err != nil {
		return nil, fmt.Errorf("scan families for persons: %w", err)
	}
	return families, nil
}

// GetFamilyChildrenByFamilyIDs retrieves the child links visible on branchID of
// every family in familyIDs (#829), in the order the interface documents.
func (s *ReadModelStore) GetFamilyChildrenByFamilyIDs(ctx context.Context, branchID domain.BranchID, familyIDs []uuid.UUID) ([]repository.FamilyChildReadModel, error) {
	if len(familyIDs) == 0 {
		return nil, nil
	}
	rows, err := s.db.QueryContext(ctx, familyChildrenByFamilyIDsQuery,
		branchID.UUID(), domain.MainBranchID.UUID(), uuidArray(familyIDs))
	if err != nil {
		return nil, fmt.Errorf("query family children by family ids: %w", err)
	}
	defer rows.Close()

	var children []repository.FamilyChildReadModel
	for rows.Next() {
		var (
			child                          repository.FamilyChildReadModel
			personGivenName, personSurname sql.NullString
			relType                        string
			sequence                       sql.NullInt64
		)
		if err := rows.Scan(&child.FamilyID, &child.PersonID, &personGivenName, &personSurname, &relType, &sequence); err != nil {
			return nil, fmt.Errorf("scan family child: %w", err)
		}
		child.PersonGivenName = personGivenName.String
		child.PersonSurname = personSurname.String
		child.RelationshipType = domain.ChildRelationType(relType)
		if sequence.Valid {
			seq := int(sequence.Int64)
			child.Sequence = &seq
		}
		children = append(children, child)
	}
	return children, rows.Err()
}
