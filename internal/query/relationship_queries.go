package query

import (
	"context"
	"fmt"
	"strings"

	"github.com/google/uuid"

	"github.com/cacack/my-family/internal/domain"
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

// RelationshipPathNode represents a person in a relationship path with their display name.
type RelationshipPathNode struct {
	ID   uuid.UUID `json:"id"`
	Name string    `json:"name"` // Display name (e.g., "John Smith")
}

// RelationshipPath represents a single path of relationship through a common ancestor.
type RelationshipPath struct {
	Name                string                 `json:"name"`                  // Human-readable relationship name (e.g., "1st cousin")
	PathFromA           []RelationshipPathNode `json:"path_from_a"`           // Path from PersonA to common ancestor
	PathFromB           []RelationshipPathNode `json:"path_from_b"`           // Path from PersonB to common ancestor
	CommonAncestor      *Person                `json:"common_ancestor"`       // The lowest common ancestor
	GenerationDistanceA int                    `json:"generation_distance_a"` // Generations from A to common ancestor
	GenerationDistanceB int                    `json:"generation_distance_b"` // Generations from B to common ancestor
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
	path       []RelationshipPathNode // Path from the starting person to this ancestor
}

// maxGenerations is the limit for ancestor search to prevent excessive recursion.
const maxRelationshipGenerations = 15

// GetRelationship calculates the relationship between two people on branchID's
// view of the tree (#829): both persons and every pedigree edge walked resolve
// through the branch overlay, so a relationship the branch asserts (or removes)
// is the one reported. The zero value (MainBranchID) reads the mainline.
func (s *RelationshipService) GetRelationship(ctx context.Context, branchID domain.BranchID, personID1, personID2 uuid.UUID) (*RelationshipResult, error) {
	// Get person A
	personARM, err := s.readStore.GetPerson(ctx, branchID, personID1)
	if err != nil {
		return nil, err
	}
	if personARM == nil {
		return nil, ErrNotFound
	}
	personA := convertReadModelToPerson(*personARM)

	// Get person B
	personBRM, err := s.readStore.GetPerson(ctx, branchID, personID2)
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
		node := RelationshipPathNode{ID: personID1, Name: personDisplayName(personA)}
		result.IsRelated = true
		result.Summary = "same person"
		result.Paths = []RelationshipPath{{
			Name:                "self",
			PathFromA:           []RelationshipPathNode{node},
			PathFromB:           []RelationshipPathNode{node},
			GenerationDistanceA: 0,
			GenerationDistanceB: 0,
		}}
		return result, nil
	}

	// Build ancestor maps for both persons with paths
	ancestorsA, err := s.buildAncestorMap(ctx, branchID, personA)
	if err != nil {
		return nil, err
	}
	ancestorsB, err := s.buildAncestorMap(ctx, branchID, personB)
	if err != nil {
		return nil, err
	}

	// Check if A is an ancestor of B (direct line down from A's perspective)
	if info, ok := ancestorsB[personID1]; ok {
		nodeA := RelationshipPathNode{ID: personID1, Name: personDisplayName(personA)}
		path := RelationshipPath{
			PathFromA:           []RelationshipPathNode{nodeA},
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
		nodeB := RelationshipPathNode{ID: personID2, Name: personDisplayName(personB)}
		path := RelationshipPath{
			PathFromA:           info.path,
			PathFromB:           []RelationshipPathNode{nodeB},
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

// personDisplayName returns a display name for a person.
func personDisplayName(p Person) string {
	name := fullName(p.GivenName, p.Surname)
	if name == "" {
		return "Unknown"
	}
	return name
}

// buildAncestorMap builds a map of all ancestors with their generation distance
// and path, walking branchID's pedigree one generation at a time.
//
// Each generation costs two set-based reads whatever its size — the frontier's
// pedigree edges, then the parents not yet seen (#829) — rather than two reads
// per ancestor. Breadth-first order also means every ancestor is recorded at its
// shortest distance; within a generation the frontier keeps father-before-mother
// order, so where two equally short paths reach one ancestor (pedigree collapse)
// the paternal one is kept.
func (s *RelationshipService) buildAncestorMap(ctx context.Context, branchID domain.BranchID, person Person) (map[uuid.UUID]ancestorInfo, error) {
	ancestors := make(map[uuid.UUID]ancestorInfo)
	seen := map[uuid.UUID]bool{person.ID: true}

	type frontierNode struct {
		id   uuid.UUID
		path []RelationshipPathNode
	}
	frontier := []frontierNode{{id: person.ID, path: []RelationshipPathNode{{ID: person.ID, Name: personDisplayName(person)}}}}

	for generation := 0; generation < maxRelationshipGenerations && len(frontier) > 0; generation++ {
		frontierIDs := make([]uuid.UUID, len(frontier))
		for i, node := range frontier {
			frontierIDs[i] = node.id
		}
		edges, err := s.readStore.GetPedigreeEdgesByPersonIDs(ctx, branchID, frontierIDs)
		if err != nil {
			return nil, err
		}
		edgeOf := make(map[uuid.UUID]repository.PedigreeEdge, len(edges))
		for _, edge := range edges {
			edgeOf[edge.PersonID] = edge
		}

		// The parents this generation discovers, in frontier order (father first).
		type discovery struct {
			parentID uuid.UUID
			child    frontierNode
		}
		var found []discovery
		var parentIDs []uuid.UUID
		for _, node := range frontier {
			edge, ok := edgeOf[node.id]
			if !ok {
				continue
			}
			for _, parentID := range []*uuid.UUID{edge.FatherID, edge.MotherID} {
				if parentID == nil || seen[*parentID] {
					continue
				}
				seen[*parentID] = true
				found = append(found, discovery{parentID: *parentID, child: node})
				parentIDs = append(parentIDs, *parentID)
			}
		}

		parents, err := s.readStore.GetPersonsByIDs(ctx, branchID, parentIDs)
		if err != nil {
			return nil, err
		}
		parentByID := make(map[uuid.UUID]Person, len(parents))
		for _, p := range parents {
			parentByID[p.ID] = convertReadModelToPerson(p)
		}

		next := make([]frontierNode, 0, len(found))
		for _, d := range found {
			parent, ok := parentByID[d.parentID]
			if !ok {
				// The edge names a person this view cannot see (deleted on the
				// branch, say): the line stops here, as it does in the pedigree.
				continue
			}
			path := make([]RelationshipPathNode, len(d.child.path), len(d.child.path)+1)
			copy(path, d.child.path)
			path = append(path, RelationshipPathNode{ID: parent.ID, Name: personDisplayName(parent)})
			ancestors[parent.ID] = ancestorInfo{person: parent, generation: generation + 1, path: path}
			next = append(next, frontierNode{id: parent.ID, path: path})
		}
		frontier = next
	}

	return ancestors, nil
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
