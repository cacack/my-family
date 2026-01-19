package query

import (
	"context"
	"fmt"
	"strings"

	"github.com/google/uuid"

	"github.com/cacack/my-family/internal/repository"
)

// RelationshipService provides relationship calculation queries between two people.
type RelationshipService struct {
	readStore       repository.ReadModelStore
	pedigreeService *PedigreeService
}

// NewRelationshipService creates a new relationship query service.
func NewRelationshipService(readStore repository.ReadModelStore) *RelationshipService {
	return &RelationshipService{
		readStore:       readStore,
		pedigreeService: NewPedigreeService(readStore),
	}
}

// RelationshipPath represents a single path of relationship through a common ancestor.
type RelationshipPath struct {
	Name                string      `json:"name"`                  // Human-readable relationship name (e.g., "1st cousin")
	PathFromA           []uuid.UUID `json:"path_from_a"`           // Path from PersonA to common ancestor
	PathFromB           []uuid.UUID `json:"path_from_b"`           // Path from PersonB to common ancestor
	CommonAncestor      *Person     `json:"common_ancestor"`       // The lowest common ancestor
	GenerationDistanceA int         `json:"generation_distance_a"` // Generations from A to common ancestor
	GenerationDistanceB int         `json:"generation_distance_b"` // Generations from B to common ancestor
}

// RelationshipResult contains the complete relationship analysis between two people.
type RelationshipResult struct {
	PersonA   *Person            `json:"person_a"`
	PersonB   *Person            `json:"person_b"`
	Paths     []RelationshipPath `json:"paths"`
	IsRelated bool               `json:"is_related"`
	Summary   string             `json:"summary"` // Human-readable summary
}

// ancestorInfo stores information about an ancestor for LCA calculation.
type ancestorInfo struct {
	person     Person
	generation int
	path       []uuid.UUID // Path from the starting person to this ancestor
}

// maxGenerations is the limit for ancestor search to prevent excessive recursion.
const maxRelationshipGenerations = 15

// GetRelationship calculates the relationship between two people.
func (s *RelationshipService) GetRelationship(ctx context.Context, personID1, personID2 uuid.UUID) (*RelationshipResult, error) {
	// Get person A
	personARM, err := s.readStore.GetPerson(ctx, personID1)
	if err != nil {
		return nil, err
	}
	if personARM == nil {
		return nil, ErrNotFound
	}
	personA := convertReadModelToPerson(*personARM)

	// Get person B
	personBRM, err := s.readStore.GetPerson(ctx, personID2)
	if err != nil {
		return nil, err
	}
	if personBRM == nil {
		return nil, ErrNotFound
	}
	personB := convertReadModelToPerson(*personBRM)

	result := &RelationshipResult{
		PersonA: &personA,
		PersonB: &personB,
		Paths:   []RelationshipPath{},
	}

	// Handle same person case
	if personID1 == personID2 {
		result.IsRelated = true
		result.Summary = "same person"
		result.Paths = []RelationshipPath{{
			Name:                "self",
			PathFromA:           []uuid.UUID{personID1},
			PathFromB:           []uuid.UUID{personID2},
			GenerationDistanceA: 0,
			GenerationDistanceB: 0,
		}}
		return result, nil
	}

	// Build ancestor maps for both persons with paths
	ancestorsA := s.buildAncestorMap(ctx, personID1)
	ancestorsB := s.buildAncestorMap(ctx, personID2)

	// Check if A is an ancestor of B (direct line down from A's perspective)
	if info, ok := ancestorsB[personID1]; ok {
		path := RelationshipPath{
			PathFromA:           []uuid.UUID{personID1},
			PathFromB:           info.path,
			CommonAncestor:      &personA,
			GenerationDistanceA: 0,
			GenerationDistanceB: info.generation,
		}
		path.Name = s.getRelationshipName(0, info.generation)
		result.Paths = append(result.Paths, path)
	}

	// Check if B is an ancestor of A (direct line up from A's perspective)
	if info, ok := ancestorsA[personID2]; ok {
		path := RelationshipPath{
			PathFromA:           info.path,
			PathFromB:           []uuid.UUID{personID2},
			CommonAncestor:      &personB,
			GenerationDistanceA: info.generation,
			GenerationDistanceB: 0,
		}
		path.Name = s.getRelationshipName(info.generation, 0)
		result.Paths = append(result.Paths, path)
	}

	// Find common ancestors (excluding the case where A or B are direct ancestors)
	commonAncestors := s.findCommonAncestors(ancestorsA, ancestorsB)

	// For each common ancestor, create a relationship path
	for _, ca := range commonAncestors {
		infoA := ancestorsA[ca.person.ID]
		infoB := ancestorsB[ca.person.ID]

		path := RelationshipPath{
			PathFromA:           infoA.path,
			PathFromB:           infoB.path,
			CommonAncestor:      &ca.person,
			GenerationDistanceA: infoA.generation,
			GenerationDistanceB: infoB.generation,
		}
		path.Name = s.getRelationshipName(infoA.generation, infoB.generation)
		result.Paths = append(result.Paths, path)
	}

	// Set overall result
	result.IsRelated = len(result.Paths) > 0
	if result.IsRelated {
		result.Summary = s.buildSummary(result.Paths)
	} else {
		result.Summary = "not related"
	}

	return result, nil
}

// buildAncestorMap builds a map of all ancestors with their generation distance and path.
func (s *RelationshipService) buildAncestorMap(ctx context.Context, personID uuid.UUID) map[uuid.UUID]ancestorInfo {
	ancestors := make(map[uuid.UUID]ancestorInfo)
	visited := make(map[uuid.UUID]bool)

	s.collectAncestorsWithPath(ctx, personID, 0, []uuid.UUID{personID}, visited, ancestors)

	return ancestors
}

// collectAncestorsWithPath recursively collects ancestors with their generation and path.
func (s *RelationshipService) collectAncestorsWithPath(
	ctx context.Context,
	personID uuid.UUID,
	generation int,
	currentPath []uuid.UUID,
	visited map[uuid.UUID]bool,
	ancestors map[uuid.UUID]ancestorInfo,
) {
	if generation >= maxRelationshipGenerations {
		return
	}
	if visited[personID] {
		return
	}
	visited[personID] = true

	edge, err := s.readStore.GetPedigreeEdge(ctx, personID)
	if err != nil || edge == nil {
		return
	}

	// Process father
	if edge.FatherID != nil {
		fatherPath := make([]uuid.UUID, len(currentPath))
		copy(fatherPath, currentPath)
		fatherPath = append(fatherPath, *edge.FatherID)

		father, err := s.readStore.GetPerson(ctx, *edge.FatherID)
		if err == nil && father != nil {
			// Only add if not already present or if this path is shorter
			if existing, ok := ancestors[*edge.FatherID]; !ok || generation+1 < existing.generation {
				ancestors[*edge.FatherID] = ancestorInfo{
					person:     convertReadModelToPerson(*father),
					generation: generation + 1,
					path:       fatherPath,
				}
			}
			s.collectAncestorsWithPath(ctx, *edge.FatherID, generation+1, fatherPath, visited, ancestors)
		}
	}

	// Process mother
	if edge.MotherID != nil {
		motherPath := make([]uuid.UUID, len(currentPath))
		copy(motherPath, currentPath)
		motherPath = append(motherPath, *edge.MotherID)

		mother, err := s.readStore.GetPerson(ctx, *edge.MotherID)
		if err == nil && mother != nil {
			// Only add if not already present or if this path is shorter
			if existing, ok := ancestors[*edge.MotherID]; !ok || generation+1 < existing.generation {
				ancestors[*edge.MotherID] = ancestorInfo{
					person:     convertReadModelToPerson(*mother),
					generation: generation + 1,
					path:       motherPath,
				}
			}
			s.collectAncestorsWithPath(ctx, *edge.MotherID, generation+1, motherPath, visited, ancestors)
		}
	}
}

// findCommonAncestors finds common ancestors between two ancestor maps.
// Returns the lowest common ancestors (smallest total generation distance).
func (s *RelationshipService) findCommonAncestors(ancestorsA, ancestorsB map[uuid.UUID]ancestorInfo) []ancestorInfo {
	var common []ancestorInfo

	for id, infoA := range ancestorsA {
		if infoB, ok := ancestorsB[id]; ok {
			// This is a common ancestor
			common = append(common, ancestorInfo{
				person:     infoA.person,
				generation: infoA.generation + infoB.generation, // Total distance for sorting
			})
		}
	}

	// Sort by total generation distance (lowest first)
	for i := 0; i < len(common)-1; i++ {
		for j := i + 1; j < len(common); j++ {
			if common[j].generation < common[i].generation {
				common[i], common[j] = common[j], common[i]
			}
		}
	}

	// Keep only the lowest common ancestors (filter out ancestors of common ancestors)
	return s.filterToLowestCommonAncestors(common, ancestorsA, ancestorsB)
}

// filterToLowestCommonAncestors removes common ancestors that are ancestors of other common ancestors.
func (s *RelationshipService) filterToLowestCommonAncestors(common []ancestorInfo, ancestorsA, ancestorsB map[uuid.UUID]ancestorInfo) []ancestorInfo {
	if len(common) <= 1 {
		return common
	}

	// Build set of common ancestor IDs
	commonIDs := make(map[uuid.UUID]bool)
	for _, ca := range common {
		commonIDs[ca.person.ID] = true
	}

	// Filter out ancestors that have common ancestors as descendants
	// A common ancestor X is "lower" than Y if X is an ancestor of Y
	filtered := make([]ancestorInfo, 0, len(common))

	for _, ca := range common {
		isLowest := true
		caInfoA := ancestorsA[ca.person.ID]
		caInfoB := ancestorsB[ca.person.ID]

		// Check if any other common ancestor is a descendant of this one
		// (i.e., this one has higher generation numbers for both paths)
		for _, other := range common {
			if other.person.ID == ca.person.ID {
				continue
			}
			otherInfoA := ancestorsA[other.person.ID]
			otherInfoB := ancestorsB[other.person.ID]

			// If other has lower generation distances on both sides, it's closer to the people
			if otherInfoA.generation < caInfoA.generation && otherInfoB.generation < caInfoB.generation {
				isLowest = false
				break
			}
		}

		if isLowest {
			filtered = append(filtered, ca)
		}
	}

	return filtered
}

// getRelationshipName returns the human-readable relationship name based on generation distances.
// The name describes what PersonB is to PersonA (e.g., "PersonB is PersonA's parent").
// genA = generations from PersonA to common ancestor
// genB = generations from PersonB to common ancestor
func (s *RelationshipService) getRelationshipName(genA, genB int) string {
	// Direct line cases
	if genA == 0 && genB == 0 {
		return "self"
	}

	// A is an ancestor of B (genA=0): PersonB is PersonA's descendant
	if genA == 0 {
		return s.getDescendantName(genB)
	}

	// B is an ancestor of A (genB=0): PersonB is PersonA's ancestor
	if genB == 0 {
		return s.getAncestorName(genA)
	}

	// Siblings: both at generation 1 from common ancestor
	if genA == 1 && genB == 1 {
		return "sibling"
	}

	// Uncle/Aunt: PersonB is 1 gen from LCA, PersonA is 2 gens (PersonB is PersonA's uncle/aunt)
	// Nephew/Niece: PersonB is 2 gens from LCA, PersonA is 1 gen (PersonB is PersonA's nephew/niece)
	if genA == 2 && genB == 1 {
		return "uncle/aunt"
	}
	if genA == 1 && genB == 2 {
		return "nephew/niece"
	}

	// Grand-uncle/aunt: PersonB is 1 gen from LCA, PersonA is 3+ gens
	// genA=3, genB=1 -> grand-uncle/aunt (grandparent's sibling)
	// genA=4, genB=1 -> great-grand-uncle/aunt (great-grandparent's sibling)
	// Grand-nephew/niece: PersonB is 3+ gens from LCA, PersonA is 1 gen
	if genB == 1 && genA > 2 {
		return s.getGreatPrefix(genA-3) + "grand-uncle/aunt"
	}
	if genA == 1 && genB > 2 {
		return s.getGreatPrefix(genB-3) + "grand-nephew/niece"
	}

	// Cousins
	// Cousin degree = min(genA, genB) - 1
	// Removed = |genA - genB|
	minGen := genA
	if genB < minGen {
		minGen = genB
	}

	degree := minGen - 1
	removed := genA - genB
	if removed < 0 {
		removed = -removed
	}

	return s.getCousinName(degree, removed)
}

// getAncestorName returns the name for an ancestor at the given generation.
func (s *RelationshipService) getAncestorName(gen int) string {
	switch gen {
	case 1:
		return "parent"
	case 2:
		return "grandparent"
	default:
		return s.getGreatPrefix(gen-2) + "grandparent"
	}
}

// getDescendantName returns the name for a descendant at the given generation.
func (s *RelationshipService) getDescendantName(gen int) string {
	switch gen {
	case 1:
		return "child"
	case 2:
		return "grandchild"
	default:
		return s.getGreatPrefix(gen-2) + "grandchild"
	}
}

// getGreatPrefix returns the "great-" prefix for a given count.
func (s *RelationshipService) getGreatPrefix(count int) string {
	if count <= 0 {
		return ""
	}
	if count == 1 {
		return "great-"
	}
	if count == 2 {
		return "great-great-"
	}
	// For 3+, use ordinal: "3rd great-", "4th great-", etc.
	return fmt.Sprintf("%s great-", s.ordinal(count))
}

// getCousinName returns the name for a cousin relationship.
func (s *RelationshipService) getCousinName(degree, removed int) string {
	if degree <= 0 {
		return "related"
	}

	ordinalDegree := s.ordinal(degree)

	if removed == 0 {
		return ordinalDegree + " cousin"
	}

	removedStr := "once"
	if removed == 2 {
		removedStr = "twice"
	} else if removed == 3 {
		removedStr = "thrice"
	} else if removed > 3 {
		removedStr = fmt.Sprintf("%d times", removed)
	}

	return fmt.Sprintf("%s cousin %s removed", ordinalDegree, removedStr)
}

// ordinal returns the ordinal string for a number (1st, 2nd, 3rd, etc.).
func (s *RelationshipService) ordinal(n int) string {
	suffix := "th"
	switch n % 10 {
	case 1:
		if n%100 != 11 {
			suffix = "st"
		}
	case 2:
		if n%100 != 12 {
			suffix = "nd"
		}
	case 3:
		if n%100 != 13 {
			suffix = "rd"
		}
	}
	return fmt.Sprintf("%d%s", n, suffix)
}

// buildSummary creates a human-readable summary of the relationship.
func (s *RelationshipService) buildSummary(paths []RelationshipPath) string {
	if len(paths) == 0 {
		return "not related"
	}

	if len(paths) == 1 {
		return paths[0].Name
	}

	// Multiple paths - summarize uniquely
	names := make([]string, 0, len(paths))
	seen := make(map[string]bool)
	for _, p := range paths {
		if !seen[p.Name] {
			names = append(names, p.Name)
			seen[p.Name] = true
		}
	}

	if len(names) == 1 {
		return fmt.Sprintf("%s (via %d paths)", names[0], len(paths))
	}

	return strings.Join(names, "; ")
}
