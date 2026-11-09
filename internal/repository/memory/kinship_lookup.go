package memory

import (
	"context"
	"slices"
	"strings"

	"github.com/google/uuid"

	"github.com/cacack/my-family/internal/domain"
	"github.com/cacack/my-family/internal/repository"
)

// Batched kinship lookups (#829): the set-based reads the descendancy and
// relationship services walk a tree with, one call per generation rather than
// one per person. Each resolves the branch overlay exactly as the matching
// single-row read does, and orders its rows the way the SQL backends do.

// GetPedigreeEdgesByPersonIDs retrieves the edges visible on branchID for
// personIDs, ordered by person id.
func (s *ReadModelStore) GetPedigreeEdgesByPersonIDs(ctx context.Context, branchID domain.BranchID, personIDs []uuid.UUID) ([]repository.PedigreeEdge, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	edges := resolveRowsByIDs(s.pedigreeEdges, branchID, personIDs)
	if err := repository.ResolvePedigreeEdgeNames(ctx, s.personsByIDsLocked, branchID, edges); err != nil {
		return nil, err
	}
	return edges, nil
}

// GetFamiliesForPersons retrieves every family visible on branchID in which any
// of personIDs is a partner, each once, ordered by id.
func (s *ReadModelStore) GetFamiliesForPersons(ctx context.Context, branchID domain.BranchID, personIDs []uuid.UUID) ([]repository.FamilyReadModel, error) {
	if len(personIDs) == 0 {
		return nil, nil
	}
	s.mu.RLock()
	defer s.mu.RUnlock()

	wanted := make(map[uuid.UUID]bool, len(personIDs))
	for _, id := range personIDs {
		wanted[id] = true
	}
	isWanted := func(id *uuid.UUID) bool { return id != nil && wanted[*id] }

	var results []repository.FamilyReadModel
	for _, f := range resolveAllRows(s.families, branchID) {
		if isWanted(f.Partner1ID) || isWanted(f.Partner2ID) {
			results = append(results, *f)
		}
	}
	slices.SortFunc(results, func(a, b repository.FamilyReadModel) int {
		return strings.Compare(a.ID.String(), b.ID.String())
	})
	return s.withFamilyNames(ctx, branchID, results)
}

// GetFamilyChildrenByFamilyIDs retrieves the child links visible on branchID of
// every family in familyIDs, ordered by family id and then compareFamilyChild.
func (s *ReadModelStore) GetFamilyChildrenByFamilyIDs(ctx context.Context, branchID domain.BranchID, familyIDs []uuid.UUID) ([]repository.FamilyChildReadModel, error) {
	if len(familyIDs) == 0 {
		return nil, nil
	}
	s.mu.RLock()
	defer s.mu.RUnlock()

	sorted := slices.Clone(familyIDs)
	slices.SortFunc(sorted, func(a, b uuid.UUID) int { return strings.Compare(a.String(), b.String()) })
	sorted = slices.Compact(sorted)

	var results []repository.FamilyChildReadModel
	for _, familyID := range sorted {
		children, _ := resolveBucket(s.familyChildren, branchID, familyID)
		results = append(results, children...)
	}
	if err := repository.ResolveFamilyChildNames(ctx, s.personsByIDsLocked, branchID, results); err != nil {
		return nil, err
	}
	slices.SortStableFunc(results, repository.CompareFamilyChildren)
	return results, nil
}
