package query

import (
	"context"
	"sort"

	"github.com/google/uuid"

	"github.com/cacack/my-family/internal/domain"
	"github.com/cacack/my-family/internal/repository"
)

// DescendancyService provides descendancy (descendant tree) queries.
type DescendancyService struct {
	readStore repository.ReadModelStore
}

// NewDescendancyService creates a new descendancy query service.
func NewDescendancyService(readStore repository.ReadModelStore) *DescendancyService {
	return &DescendancyService{readStore: readStore}
}

// SpouseInfo represents spouse information in a descendancy node.
type SpouseInfo struct {
	ID           uuid.UUID       `json:"id"`
	FamilyID     uuid.UUID       `json:"family_id"`
	Name         string          `json:"name"`
	MarriageDate *domain.GenDate `json:"marriage_date,omitempty"`
}

// DescendancyNode represents a person in the descendancy tree.
type DescendancyNode struct {
	ID         uuid.UUID          `json:"id"`
	GivenName  string             `json:"given_name"`
	Surname    string             `json:"surname"`
	Gender     string             `json:"gender,omitempty"`
	BirthDate  *domain.GenDate    `json:"birth_date,omitempty"`
	DeathDate  *domain.GenDate    `json:"death_date,omitempty"`
	Spouses    []SpouseInfo       `json:"spouses,omitempty"`
	Children   []*DescendancyNode `json:"children,omitempty"`
	Generation int                `json:"generation"`
	// FamilyID is the family through which this node descends from its parent
	// node, matching one of the parent's Spouses when that family has a second
	// partner; nil on the root.
	FamilyID *uuid.UUID `json:"family_id,omitempty"`
}

// DescendancyResult contains the descendancy tree for a person.
type DescendancyResult struct {
	Root             *DescendancyNode `json:"root"`
	TotalDescendants int              `json:"total_descendants"`
	MaxGeneration    int              `json:"max_generation"`
}

// GetDescendancyInput contains options for retrieving a descendancy.
type GetDescendancyInput struct {
	PersonID       uuid.UUID
	MaxGenerations int             // Maximum generations to traverse (default 4)
	BranchID       domain.BranchID // Branch scope; zero value = MainBranchID (main line)
}

// GetDescendancy returns the descendant tree for a person on input.BranchID's
// view of the tree (#829): the root, every family, every child link and every
// descendant resolve through the branch overlay.
func (s *DescendancyService) GetDescendancy(ctx context.Context, input GetDescendancyInput) (*DescendancyResult, error) {
	// Set default max generations
	maxGen := input.MaxGenerations
	if maxGen <= 0 {
		maxGen = 4
	}
	if maxGen > 10 {
		maxGen = 10 // Hard limit to prevent excessive recursion
	}

	// Get the root person
	person, err := s.readStore.GetPerson(ctx, input.BranchID, input.PersonID)
	if err != nil {
		return nil, err
	}
	if person == nil {
		return nil, ErrNotFound
	}

	root, err := s.buildDescendancyTree(ctx, input.BranchID, *person, maxGen)
	if err != nil {
		return nil, err
	}

	// Count total descendants and max generation
	totalDescendants := 0
	maxGenReached := 0
	countDescendants(root, &totalDescendants, &maxGenReached)

	return &DescendancyResult{
		Root:             root,
		TotalDescendants: totalDescendants,
		MaxGeneration:    maxGenReached,
	}, nil
}

// buildDescendancyTree builds the descendant tree under root one generation at a
// time. Each generation costs three set-based reads whatever its size — the
// generation's families, their child links, and the children not yet seen
// (#829) — rather than a chain of reads per person.
//
// A person reachable along more than one line (pedigree collapse) appears once,
// at the shallowest generation that reaches them, under the first parent (in
// tree order) to claim them.
func (s *DescendancyService) buildDescendancyTree(ctx context.Context, branchID domain.BranchID, root repository.PersonReadModel, maxGen int) (*DescendancyNode, error) {
	rootNode := newDescendancyNode(root, 0)
	visited := map[uuid.UUID]bool{root.ID: true}
	level := []*DescendancyNode{rootNode}

	for generation := 0; len(level) > 0; generation++ {
		levelIDs := make([]uuid.UUID, len(level))
		for i, node := range level {
			levelIDs[i] = node.ID
		}
		families, err := s.readStore.GetFamiliesForPersons(ctx, branchID, levelIDs)
		if err != nil {
			return nil, err
		}

		// Each node's families, by marriage date (undated last), and its spouses
		// in that order (#1099).
		familiesOf := make(map[uuid.UUID][]repository.FamilyReadModel, len(level))
		for _, family := range families {
			for _, partnerID := range []*uuid.UUID{family.Partner1ID, family.Partner2ID} {
				if partnerID != nil {
					familiesOf[*partnerID] = append(familiesOf[*partnerID], family)
				}
			}
		}
		for _, node := range level {
			sortFamiliesByMarriage(familiesOf[node.ID])
			for _, family := range familiesOf[node.ID] {
				if spouse := s.getSpouseInfo(family, node.ID); spouse != nil {
					node.Spouses = append(node.Spouses, *spouse)
				}
			}
		}

		// Don't descend beyond max generations
		if generation >= maxGen {
			break
		}

		next, err := s.nextGeneration(ctx, branchID, level, familiesOf, families, visited, generation+1)
		if err != nil {
			return nil, err
		}
		level = next
	}

	return rootNode, nil
}

// nextGeneration resolves the children of every family in families and attaches
// each not-yet-visited child, in tree order, to the level node whose family
// claims them first. A node's children are grouped by family, in the node's
// family order, and ordered by birth date (undated last) within each family.
// It returns the new nodes, which form the next level.
func (s *DescendancyService) nextGeneration(
	ctx context.Context,
	branchID domain.BranchID,
	level []*DescendancyNode,
	familiesOf map[uuid.UUID][]repository.FamilyReadModel,
	families []repository.FamilyReadModel,
	visited map[uuid.UUID]bool,
	generation int,
) ([]*DescendancyNode, error) {
	familyIDs := make([]uuid.UUID, len(families))
	for i, family := range families {
		familyIDs[i] = family.ID
	}
	links, err := s.readStore.GetFamilyChildrenByFamilyIDs(ctx, branchID, familyIDs)
	if err != nil {
		return nil, err
	}
	childrenOf := make(map[uuid.UUID][]uuid.UUID, len(families))
	for _, link := range links {
		childrenOf[link.FamilyID] = append(childrenOf[link.FamilyID], link.PersonID)
	}

	// Claim each child for its first parent in tree order.
	type claim struct {
		parent   *DescendancyNode
		familyID uuid.UUID
		childID  uuid.UUID
	}
	var claims []claim
	var childIDs []uuid.UUID
	for _, node := range level {
		for _, family := range familiesOf[node.ID] {
			for _, childID := range childrenOf[family.ID] {
				if visited[childID] {
					continue
				}
				visited[childID] = true
				claims = append(claims, claim{parent: node, familyID: family.ID, childID: childID})
				childIDs = append(childIDs, childID)
			}
		}
	}

	persons, err := s.readStore.GetPersonsByIDs(ctx, branchID, childIDs)
	if err != nil {
		return nil, err
	}
	personByID := make(map[uuid.UUID]repository.PersonReadModel, len(persons))
	for _, p := range persons {
		personByID[p.ID] = p
	}

	next := make([]*DescendancyNode, 0, len(claims))
	for _, c := range claims {
		person, ok := personByID[c.childID]
		if !ok {
			// A child link to a person this view cannot see: skip it, as the
			// family detail does.
			continue
		}
		child := newDescendancyNode(person, generation)
		familyID := c.familyID
		child.FamilyID = &familyID
		c.parent.Children = append(c.parent.Children, child)
		next = append(next, child)
	}
	for _, node := range level {
		sortChildrenWithinFamilies(node.Children)
	}
	return next, nil
}

// sortFamiliesByMarriage orders one person's families by marriage date,
// undated last; ties keep the store's (id) order.
func sortFamiliesByMarriage(families []repository.FamilyReadModel) {
	sort.SliceStable(families, func(i, j int) bool {
		return genDateBefore(parseRawDate(families[i].MarriageDateRaw), parseRawDate(families[j].MarriageDateRaw))
	})
}

// sortChildrenWithinFamilies orders children by birth date (undated last)
// inside each run of one family, leaving the family groups where they are.
func sortChildrenWithinFamilies(children []*DescendancyNode) {
	for start := 0; start < len(children); {
		end := start + 1
		for end < len(children) && *children[end].FamilyID == *children[start].FamilyID {
			end++
		}
		group := children[start:end]
		sort.SliceStable(group, func(i, j int) bool {
			return genDateBefore(group[i].BirthDate, group[j].BirthDate)
		})
		start = end
	}
}

// parseRawDate parses a raw date, or returns nil when there is none.
func parseRawDate(raw string) *domain.GenDate {
	if raw == "" {
		return nil
	}
	d := domain.ParseGenDate(raw)
	return &d
}

// genDateBefore reports whether a sorts before b, with dates that cannot be
// placed on the timeline (no year, or an unconvertible calendar) last.
func genDateBefore(a, b *domain.GenDate) bool {
	ta, tb := GenDateToSortTime(a), GenDateToSortTime(b)
	if (ta == nil) != (tb == nil) {
		return ta != nil
	}
	return ta != nil && ta.Before(*tb)
}

// newDescendancyNode builds the node for person at generation, without spouses
// or children.
func newDescendancyNode(person repository.PersonReadModel, generation int) *DescendancyNode {
	node := &DescendancyNode{
		ID:         person.ID,
		GivenName:  person.GivenName,
		Surname:    person.Surname,
		Generation: generation,
	}
	if person.Gender != "" {
		node.Gender = string(person.Gender)
	}
	if person.BirthDateRaw != "" {
		bd := domain.ParseGenDate(person.BirthDateRaw)
		node.BirthDate = &bd
	}
	if person.DeathDateRaw != "" {
		dd := domain.ParseGenDate(person.DeathDateRaw)
		node.DeathDate = &dd
	}
	return node
}

// getSpouseInfo extracts spouse information from a family record.
func (s *DescendancyService) getSpouseInfo(family repository.FamilyReadModel, personID uuid.UUID) *SpouseInfo {
	var spouseID *uuid.UUID
	var spouseName string

	// Find the other partner
	if family.Partner1ID != nil && *family.Partner1ID != personID {
		spouseID = family.Partner1ID
		spouseName = fullName(family.Partner1GivenName, family.Partner1Surname)
	} else if family.Partner2ID != nil && *family.Partner2ID != personID {
		spouseID = family.Partner2ID
		spouseName = fullName(family.Partner2GivenName, family.Partner2Surname)
	}

	if spouseID == nil {
		return nil
	}

	info := &SpouseInfo{
		ID:       *spouseID,
		FamilyID: family.ID,
		Name:     spouseName,
	}

	// Add marriage date if available
	if family.MarriageDateRaw != "" {
		md := domain.ParseGenDate(family.MarriageDateRaw)
		info.MarriageDate = &md
	}

	return info
}

// countDescendants counts total descendants and finds max generation in the tree.
func countDescendants(node *DescendancyNode, total *int, maxGen *int) {
	if node == nil {
		return
	}

	if node.Generation > *maxGen {
		*maxGen = node.Generation
	}

	for _, child := range node.Children {
		*total++
		countDescendants(child, total, maxGen)
	}
}
