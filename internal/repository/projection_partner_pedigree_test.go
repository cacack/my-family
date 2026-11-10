package repository_test

import (
	"context"
	"testing"

	"github.com/google/uuid"

	"github.com/cacack/my-family/internal/domain"
	"github.com/cacack/my-family/internal/repository"
	"github.com/cacack/my-family/internal/repository/memory"
)

// TestProjector_FamilyPartnerChange_RefreshesChildEdges (#826): a partner
// change re-derives the pedigree edge of the family's children, but leaves an
// edge that came from another family the child belongs to.
func TestProjector_FamilyPartnerChange_RefreshesChildEdges(t *testing.T) {
	ctx := context.Background()
	readStore := memory.NewReadModelStore()
	projector := repository.NewProjector(readStore, nil)
	project := func(e domain.Event, version int64) {
		t.Helper()
		if err := projector.Project(ctx, e, version, domain.MainBranchID); err != nil {
			t.Fatalf("project %T: %v", e, err)
		}
	}
	person := func(name string, g domain.Gender) *domain.Person {
		p := domain.NewPerson(name, "Placeholder")
		p.Gender = g
		project(domain.NewPersonCreated(p), 1)
		return p
	}
	father := person("Alden", domain.GenderMale)
	mother := person("Briar", domain.GenderFemale)
	stepMother := person("Dana", domain.GenderFemale)
	child := person("Wren", domain.GenderFemale)
	adopted := person("Ellis", domain.GenderMale)

	birth := domain.NewFamilyWithPartners(&father.ID, &mother.ID)
	project(domain.NewFamilyCreated(birth), 1)
	project(domain.NewChildLinkedToFamily(domain.NewFamilyChild(birth.ID, child.ID, domain.ChildBiological)), 2)
	project(domain.NewChildLinkedToFamily(domain.NewFamilyChild(birth.ID, adopted.ID, domain.ChildBiological)), 3)

	// Ellis's edge now comes from a second family.
	other := domain.NewFamilyWithPartners(&stepMother.ID, nil)
	project(domain.NewFamilyCreated(other), 1)
	project(domain.NewChildLinkedToFamily(domain.NewFamilyChild(other.ID, adopted.ID, domain.ChildAdopted)), 2)

	project(domain.NewFamilyUpdated(birth.ID, map[string]any{"partner2_id": nil}), 4)

	edge := mustEdge(t, readStore, child.ID)
	if edge.FatherID == nil || *edge.FatherID != father.ID || edge.MotherID != nil {
		t.Errorf("Wren edge = father %v mother %v, want father %s and no mother", edge.FatherID, edge.MotherID, father.ID)
	}
	edge = mustEdge(t, readStore, adopted.ID)
	if edge.MotherID == nil || *edge.MotherID != stepMother.ID || edge.FatherID != nil {
		t.Errorf("Ellis edge = father %v mother %v, want the other family's mother %s", edge.FatherID, edge.MotherID, stepMother.ID)
	}

	// A change that does not touch partners leaves edges alone.
	project(domain.NewFamilyUpdated(birth.ID, map[string]any{"marriage_place": "Hall"}), 5)
	if edge := mustEdge(t, readStore, child.ID); edge.FatherID == nil || *edge.FatherID != father.ID {
		t.Errorf("Wren father after non-partner change = %v, want %s", edge.FatherID, father.ID)
	}

	// A swap re-derives father/mother from the new partners' genders.
	project(domain.NewFamilyUpdated(birth.ID, map[string]any{"partner1_id": mother.ID.String(), "partner2_id": father.ID.String()}), 6)
	edge = mustEdge(t, readStore, child.ID)
	if edge.FatherID == nil || *edge.FatherID != father.ID || edge.MotherID == nil || *edge.MotherID != mother.ID {
		t.Errorf("Wren edge after swap = father %v mother %v, want %s / %s", edge.FatherID, edge.MotherID, father.ID, mother.ID)
	}
}

// TestProjector_PartnerCreatedAfterFamily_FillsChildEdges (#1259): a family
// projected before one of its partners exists, as a merge replay can do,
// gets that partner into its children's edges once the person is created,
// but an edge that came from another family is left alone.
func TestProjector_PartnerCreatedAfterFamily_FillsChildEdges(t *testing.T) {
	ctx := context.Background()
	readStore := memory.NewReadModelStore()
	projector := repository.NewProjector(readStore, nil)
	project := func(e domain.Event, version int64) {
		t.Helper()
		if err := projector.Project(ctx, e, version, domain.MainBranchID); err != nil {
			t.Fatalf("project %T: %v", e, err)
		}
	}
	newPerson := func(name string, g domain.Gender) *domain.Person {
		p := domain.NewPerson(name, "Placeholder")
		p.Gender = g
		return p
	}
	mother := newPerson("Briar", domain.GenderFemale)
	stepMother := newPerson("Dana", domain.GenderFemale)
	child := newPerson("Wren", domain.GenderFemale)
	adopted := newPerson("Ellis", domain.GenderMale)
	lateFather := newPerson("Alden", domain.GenderMale)
	for _, p := range []*domain.Person{mother, stepMother, child, adopted} {
		project(domain.NewPersonCreated(p), 1)
	}

	birth := domain.NewFamilyWithPartners(&mother.ID, &lateFather.ID)
	project(domain.NewFamilyCreated(birth), 1)
	project(domain.NewChildLinkedToFamily(domain.NewFamilyChild(birth.ID, child.ID, domain.ChildBiological)), 2)
	project(domain.NewChildLinkedToFamily(domain.NewFamilyChild(birth.ID, adopted.ID, domain.ChildBiological)), 3)
	other := domain.NewFamilyWithPartners(&stepMother.ID, nil)
	project(domain.NewFamilyCreated(other), 1)
	project(domain.NewChildLinkedToFamily(domain.NewFamilyChild(other.ID, adopted.ID, domain.ChildAdopted)), 2)

	if edge := mustEdge(t, readStore, child.ID); edge.FatherID != nil {
		t.Fatalf("Wren father before the father exists = %v, want none", edge.FatherID)
	}

	project(domain.NewPersonCreated(lateFather), 1)

	edge := mustEdge(t, readStore, child.ID)
	if edge.FatherID == nil || *edge.FatherID != lateFather.ID || edge.MotherID == nil || *edge.MotherID != mother.ID {
		t.Errorf("Wren edge = father %v mother %v, want %s / %s", edge.FatherID, edge.MotherID, lateFather.ID, mother.ID)
	}
	edge = mustEdge(t, readStore, adopted.ID)
	if edge.MotherID == nil || *edge.MotherID != stepMother.ID || edge.FatherID != nil {
		t.Errorf("Ellis edge = father %v mother %v, want the other family's mother %s", edge.FatherID, edge.MotherID, stepMother.ID)
	}
}

// TestProjector_PartnerGenderChange_RefreshesChildEdges (#1263): a partner's
// gender change re-derives the edges of the family's children, so the partner
// moves to the side the new gender names, but an edge that came from another
// family is left alone and a change to anything else leaves edges as they are.
func TestProjector_PartnerGenderChange_RefreshesChildEdges(t *testing.T) {
	ctx := context.Background()
	readStore := memory.NewReadModelStore()
	projector := repository.NewProjector(readStore, nil)
	project := func(e domain.Event, version int64) {
		t.Helper()
		if err := projector.Project(ctx, e, version, domain.MainBranchID); err != nil {
			t.Fatalf("project %T: %v", e, err)
		}
	}
	person := func(name string, g domain.Gender) *domain.Person {
		p := domain.NewPerson(name, "Placeholder")
		p.Gender = g
		project(domain.NewPersonCreated(p), 1)
		return p
	}
	first := person("Alden", domain.GenderUnknown)
	second := person("Briar", domain.GenderUnknown)
	stepMother := person("Dana", domain.GenderFemale)
	child := person("Wren", domain.GenderFemale)
	adopted := person("Ellis", domain.GenderMale)

	birth := domain.NewFamilyWithPartners(&first.ID, &second.ID)
	project(domain.NewFamilyCreated(birth), 1)
	project(domain.NewChildLinkedToFamily(domain.NewFamilyChild(birth.ID, child.ID, domain.ChildBiological)), 2)
	project(domain.NewChildLinkedToFamily(domain.NewFamilyChild(birth.ID, adopted.ID, domain.ChildBiological)), 3)
	other := domain.NewFamilyWithPartners(&stepMother.ID, nil)
	project(domain.NewFamilyCreated(other), 1)
	project(domain.NewChildLinkedToFamily(domain.NewFamilyChild(other.ID, adopted.ID, domain.ChildAdopted)), 2)

	// Both unknown: the first takes the free mother slot.
	edge := mustEdge(t, readStore, child.ID)
	if edge.MotherID == nil || *edge.MotherID != first.ID || edge.FatherID == nil || *edge.FatherID != second.ID {
		t.Fatalf("Wren edge before = father %v mother %v, want %s / %s", edge.FatherID, edge.MotherID, second.ID, first.ID)
	}

	project(domain.NewPersonUpdated(second.ID, map[string]any{"notes": "x"}), 2)
	if edge := mustEdge(t, readStore, child.ID); edge.MotherID == nil || *edge.MotherID != first.ID {
		t.Errorf("Wren mother after non-gender change = %v, want %s", edge.MotherID, first.ID)
	}

	project(domain.NewPersonUpdated(second.ID, map[string]any{"gender": string(domain.GenderFemale)}), 3)
	edge = mustEdge(t, readStore, child.ID)
	if edge.MotherID == nil || *edge.MotherID != second.ID || edge.FatherID == nil || *edge.FatherID != first.ID {
		t.Errorf("Wren edge after = father %v mother %v, want %s / %s", edge.FatherID, edge.MotherID, first.ID, second.ID)
	}
	edge = mustEdge(t, readStore, adopted.ID)
	if edge.MotherID == nil || *edge.MotherID != stepMother.ID || edge.FatherID != nil {
		t.Errorf("Ellis edge = father %v mother %v, want the other family's mother %s", edge.FatherID, edge.MotherID, stepMother.ID)
	}
}

func mustEdge(t *testing.T, rs repository.ReadModelStore, personID uuid.UUID) *repository.PedigreeEdge {
	t.Helper()
	edge, err := rs.GetPedigreeEdge(context.Background(), domain.MainBranchID, personID)
	if err != nil {
		t.Fatalf("GetPedigreeEdge: %v", err)
	}
	if edge == nil {
		t.Fatalf("no pedigree edge for %s", personID)
	}
	return edge
}
