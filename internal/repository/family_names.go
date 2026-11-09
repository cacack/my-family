package repository

import (
	"cmp"
	"context"
	"strings"

	"github.com/google/uuid"

	"github.com/cacack/my-family/internal/domain"
)

// Family member names are resolved at read time (#1207). A family's partner
// names, its child links' names and a pedigree edge's father/mother names are
// not stored: every backend's family, child-link and pedigree-edge read fills
// them in from the person overlay of the branch it reads, through one batched
// person lookup per call. So a family shows each member's name as that branch
// sees the person, whether or not the branch shadows the family, and a rename
// writes no family row on any branch (ADR-005 live overlay).
//
// A member the branch cannot see (deleted on it, or not projected yet) reads
// with an empty name.

// PersonsByIDs is the batched person-overlay lookup the resolvers read names
// through; ReadModelStore.GetPersonsByIDs satisfies it.
type PersonsByIDs func(ctx context.Context, branchID domain.BranchID, ids []uuid.UUID) ([]PersonReadModel, error)

// ResolveFamilyNames sets the partner names of families to the partners'
// names as branchID sees them.
func ResolveFamilyNames(ctx context.Context, lookup PersonsByIDs, branchID domain.BranchID, families []FamilyReadModel) error {
	var ids []uuid.UUID
	for _, f := range families {
		ids = appendID(ids, f.Partner1ID)
		ids = appendID(ids, f.Partner2ID)
	}
	persons, err := personsByID(ctx, lookup, branchID, ids)
	if err != nil {
		return err
	}
	for i := range families {
		f := &families[i]
		f.Partner1GivenName, f.Partner1Surname = persons.names(f.Partner1ID)
		f.Partner2GivenName, f.Partner2Surname = persons.names(f.Partner2ID)
	}
	return nil
}

// ResolveFamilyChildNames sets the names of child links to the children's
// names as branchID sees them.
func ResolveFamilyChildNames(ctx context.Context, lookup PersonsByIDs, branchID domain.BranchID, children []FamilyChildReadModel) error {
	ids := make([]uuid.UUID, 0, len(children))
	for _, c := range children {
		ids = append(ids, c.PersonID)
	}
	persons, err := personsByID(ctx, lookup, branchID, ids)
	if err != nil {
		return err
	}
	for i := range children {
		c := &children[i]
		c.PersonGivenName, c.PersonSurname = persons.names(&c.PersonID)
	}
	return nil
}

// ResolvePedigreeEdgeNames sets the father and mother names of edges to those
// parents' full names as branchID sees them.
func ResolvePedigreeEdgeNames(ctx context.Context, lookup PersonsByIDs, branchID domain.BranchID, edges []PedigreeEdge) error {
	var ids []uuid.UUID
	for _, e := range edges {
		ids = appendID(ids, e.FatherID)
		ids = appendID(ids, e.MotherID)
	}
	persons, err := personsByID(ctx, lookup, branchID, ids)
	if err != nil {
		return err
	}
	for i := range edges {
		e := &edges[i]
		e.FatherName = domain.JoinFullName(persons.names(e.FatherID))
		e.MotherName = domain.JoinFullName(persons.names(e.MotherID))
	}
	return nil
}

// CompareFamilyChildren is the order GetFamilyChildrenByFamilyIDs returns
// child links in on every backend: by family id, then by sequence with
// unsequenced children last, then bytewise by the resolved surname, given name
// and person id.
func CompareFamilyChildren(a, b FamilyChildReadModel) int {
	if c := strings.Compare(a.FamilyID.String(), b.FamilyID.String()); c != 0 {
		return c
	}
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

// personNames maps a person id to its resolved row.
type personNames map[uuid.UUID]PersonReadModel

// names returns the given name and surname of id, or empty names for a nil
// id or a person the lookup did not return.
func (n personNames) names(id *uuid.UUID) (givenName, surname string) {
	if id == nil {
		return "", ""
	}
	p := n[*id]
	return p.GivenName, p.Surname
}

func personsByID(ctx context.Context, lookup PersonsByIDs, branchID domain.BranchID, ids []uuid.UUID) (personNames, error) {
	if len(ids) == 0 {
		return nil, nil
	}
	persons, err := lookup(ctx, branchID, ids)
	if err != nil {
		return nil, err
	}
	byID := make(personNames, len(persons))
	for _, p := range persons {
		byID[p.ID] = p
	}
	return byID, nil
}

func appendID(ids []uuid.UUID, id *uuid.UUID) []uuid.UUID {
	if id == nil {
		return ids
	}
	return append(ids, *id)
}
