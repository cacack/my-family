package memory

import (
	"cmp"
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
	return resolveRowsByIDs(s.pedigreeEdges, branchID, personIDs), nil
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
	return results, nil
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
		bucket := slices.Clone(children)
		slices.SortStableFunc(bucket, compareFamilyChild)
		results = append(results, bucket...)
	}
	return results, nil
}

// compareFamilyChild orders child links within one family: by sequence with
// unsequenced children last, then surname, given name and person id.
func compareFamilyChild(a, b repository.FamilyChildReadModel) int {
	switch {
	case a.Sequence == nil && b.Sequence != nil:
		return 1
	case a.Sequence != nil && b.Sequence == nil:
		return -1
	case a.Sequence != nil && b.Sequence != nil && *a.Sequence != *b.Sequence:
		return cmp.Compare(*a.Sequence, *b.Sequence)
	}
	if c := strings.Compare(a.PersonSurname, b.PersonSurname); c != 0 {
		return c
	}
	if c := strings.Compare(a.PersonGivenName, b.PersonGivenName); c != 0 {
		return c
	}
	return strings.Compare(a.PersonID.String(), b.PersonID.String())
}
