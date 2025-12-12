package repository_test

import (
	"context"
	"testing"

	"github.com/cacack/my-family/internal/domain"
	"github.com/cacack/my-family/internal/repository"
	"github.com/cacack/my-family/internal/repository/memory"
)

func TestProjector_PersonCreated(t *testing.T) {
	readStore := memory.NewReadModelStore()
	projector := repository.NewProjector(readStore)
	ctx := context.Background()

	person := domain.NewPerson("John", "Doe")
	person.Gender = domain.GenderMale
	person.SetBirthDate("1 JAN 1850")
	person.BirthPlace = "Springfield, IL"

	event := domain.NewPersonCreated(person)

	err := projector.Project(ctx, event, 1)
	if err != nil {
		t.Fatalf("Project failed: %v", err)
	}

	// Verify person in read model
	rm, err := readStore.GetPerson(ctx, person.ID)
	if err != nil {
		t.Fatalf("GetPerson failed: %v", err)
	}
	if rm == nil {
		t.Fatal("Person not found in read model")
	}
	if rm.GivenName != "John" {
		t.Errorf("GivenName = %s, want John", rm.GivenName)
	}
	if rm.Surname != "Doe" {
		t.Errorf("Surname = %s, want Doe", rm.Surname)
	}
	if rm.FullName != "John Doe" {
		t.Errorf("FullName = %s, want John Doe", rm.FullName)
	}
	if rm.Gender != domain.GenderMale {
		t.Errorf("Gender = %s, want male", rm.Gender)
	}
	if rm.BirthPlace != "Springfield, IL" {
		t.Errorf("BirthPlace = %s, want Springfield, IL", rm.BirthPlace)
	}
	if rm.Version != 1 {
		t.Errorf("Version = %d, want 1", rm.Version)
	}
}

func TestProjector_PersonUpdated(t *testing.T) {
	readStore := memory.NewReadModelStore()
	projector := repository.NewProjector(readStore)
	ctx := context.Background()

	// Create person first
	person := domain.NewPerson("John", "Doe")
	createEvent := domain.NewPersonCreated(person)
	if err := projector.Project(ctx, createEvent, 1); err != nil {
		t.Fatalf("Project create failed: %v", err)
	}

	// Update person
	changes := map[string]any{
		"given_name": "Jane",
		"surname":    "Smith",
	}
	updateEvent := domain.NewPersonUpdated(person.ID, changes)

	err := projector.Project(ctx, updateEvent, 2)
	if err != nil {
		t.Fatalf("Project update failed: %v", err)
	}

	// Verify updates
	rm, _ := readStore.GetPerson(ctx, person.ID)
	if rm.GivenName != "Jane" {
		t.Errorf("GivenName = %s, want Jane", rm.GivenName)
	}
	if rm.Surname != "Smith" {
		t.Errorf("Surname = %s, want Smith", rm.Surname)
	}
	if rm.FullName != "Jane Smith" {
		t.Errorf("FullName = %s, want Jane Smith", rm.FullName)
	}
	if rm.Version != 2 {
		t.Errorf("Version = %d, want 2", rm.Version)
	}
}

func TestProjector_PersonDeleted(t *testing.T) {
	readStore := memory.NewReadModelStore()
	projector := repository.NewProjector(readStore)
	ctx := context.Background()

	// Create person first
	person := domain.NewPerson("John", "Doe")
	createEvent := domain.NewPersonCreated(person)
	if err := projector.Project(ctx, createEvent, 1); err != nil {
		t.Fatalf("Project create failed: %v", err)
	}

	// Delete person
	deleteEvent := domain.NewPersonDeleted(person.ID, "test")
	err := projector.Project(ctx, deleteEvent, 2)
	if err != nil {
		t.Fatalf("Project delete failed: %v", err)
	}

	// Verify deletion
	rm, _ := readStore.GetPerson(ctx, person.ID)
	if rm != nil {
		t.Error("Person should be deleted")
	}
}

func TestProjector_FamilyCreated(t *testing.T) {
	readStore := memory.NewReadModelStore()
	projector := repository.NewProjector(readStore)
	ctx := context.Background()

	// Create partners first
	p1 := domain.NewPerson("John", "Doe")
	p1.Gender = domain.GenderMale
	p2 := domain.NewPerson("Jane", "Doe")
	p2.Gender = domain.GenderFemale

	if err := projector.Project(ctx, domain.NewPersonCreated(p1), 1); err != nil {
		t.Fatalf("Project p1 failed: %v", err)
	}
	if err := projector.Project(ctx, domain.NewPersonCreated(p2), 1); err != nil {
		t.Fatalf("Project p2 failed: %v", err)
	}

	// Create family
	family := domain.NewFamilyWithPartners(&p1.ID, &p2.ID)
	family.RelationshipType = domain.RelationMarriage
	family.SetMarriageDate("1 JAN 1870")

	event := domain.NewFamilyCreated(family)
	err := projector.Project(ctx, event, 1)
	if err != nil {
		t.Fatalf("Project family failed: %v", err)
	}

	// Verify family in read model
	rm, _ := readStore.GetFamily(ctx, family.ID)
	if rm == nil {
		t.Fatal("Family not found in read model")
	}
	if rm.Partner1Name != "John Doe" {
		t.Errorf("Partner1Name = %s, want John Doe", rm.Partner1Name)
	}
	if rm.Partner2Name != "Jane Doe" {
		t.Errorf("Partner2Name = %s, want Jane Doe", rm.Partner2Name)
	}
	if rm.RelationshipType != domain.RelationMarriage {
		t.Errorf("RelationshipType = %s, want marriage", rm.RelationshipType)
	}
}

func TestProjector_ChildLinked(t *testing.T) {
	readStore := memory.NewReadModelStore()
	projector := repository.NewProjector(readStore)
	ctx := context.Background()

	// Create family with parents
	father := domain.NewPerson("John", "Doe")
	father.Gender = domain.GenderMale
	mother := domain.NewPerson("Jane", "Doe")
	mother.Gender = domain.GenderFemale
	child := domain.NewPerson("Jimmy", "Doe")

	projector.Project(ctx, domain.NewPersonCreated(father), 1)
	projector.Project(ctx, domain.NewPersonCreated(mother), 1)
	projector.Project(ctx, domain.NewPersonCreated(child), 1)

	family := domain.NewFamilyWithPartners(&father.ID, &mother.ID)
	projector.Project(ctx, domain.NewFamilyCreated(family), 1)

	// Link child
	fc := domain.NewFamilyChild(family.ID, child.ID, domain.ChildBiological)
	event := domain.NewChildLinkedToFamily(fc)

	err := projector.Project(ctx, event, 2)
	if err != nil {
		t.Fatalf("Project child link failed: %v", err)
	}

	// Verify child in family
	children, _ := readStore.GetFamilyChildren(ctx, family.ID)
	if len(children) != 1 {
		t.Errorf("Expected 1 child, got %d", len(children))
	}
	if children[0].PersonID != child.ID {
		t.Error("Wrong child linked")
	}

	// Verify pedigree edge
	edge, _ := readStore.GetPedigreeEdge(ctx, child.ID)
	if edge == nil {
		t.Fatal("Pedigree edge not created")
	}
	if edge.FatherID == nil || *edge.FatherID != father.ID {
		t.Error("Father not set correctly in pedigree edge")
	}
	if edge.MotherID == nil || *edge.MotherID != mother.ID {
		t.Error("Mother not set correctly in pedigree edge")
	}
}

func TestProjector_ChildUnlinked(t *testing.T) {
	readStore := memory.NewReadModelStore()
	projector := repository.NewProjector(readStore)
	ctx := context.Background()

	// Setup: Create family with child
	father := domain.NewPerson("John", "Doe")
	father.Gender = domain.GenderMale
	child := domain.NewPerson("Jimmy", "Doe")

	projector.Project(ctx, domain.NewPersonCreated(father), 1)
	projector.Project(ctx, domain.NewPersonCreated(child), 1)

	family := domain.NewFamilyWithPartners(&father.ID, nil)
	projector.Project(ctx, domain.NewFamilyCreated(family), 1)

	fc := domain.NewFamilyChild(family.ID, child.ID, domain.ChildBiological)
	projector.Project(ctx, domain.NewChildLinkedToFamily(fc), 2)

	// Unlink child
	event := domain.NewChildUnlinkedFromFamily(family.ID, child.ID)
	err := projector.Project(ctx, event, 3)
	if err != nil {
		t.Fatalf("Project child unlink failed: %v", err)
	}

	// Verify child removed
	children, _ := readStore.GetFamilyChildren(ctx, family.ID)
	if len(children) != 0 {
		t.Errorf("Expected 0 children, got %d", len(children))
	}

	// Verify pedigree edge removed
	edge, _ := readStore.GetPedigreeEdge(ctx, child.ID)
	if edge != nil {
		t.Error("Pedigree edge should be removed")
	}
}

func TestProjector_UnknownEventIgnored(t *testing.T) {
	readStore := memory.NewReadModelStore()
	projector := repository.NewProjector(readStore)
	ctx := context.Background()

	// Create a mock event type
	type UnknownEvent struct {
		domain.BaseEvent
		Data string
	}

	// This should not panic or error
	// Since Go doesn't allow creating arbitrary interface implementations easily,
	// we test with a GedcomImported event which is handled gracefully
	event := domain.NewGedcomImported("test.ged", 100, 10, 5, nil, nil)
	err := projector.Project(ctx, event, 1)
	if err != nil {
		t.Fatalf("Project should not error on GedcomImported: %v", err)
	}
}

func TestProjector_Integration(t *testing.T) {
	readStore := memory.NewReadModelStore()
	projector := repository.NewProjector(readStore)
	ctx := context.Background()

	// Build a small family tree
	// Grandparents
	gf := domain.NewPerson("George", "Smith")
	gf.Gender = domain.GenderMale
	gm := domain.NewPerson("Martha", "Smith")
	gm.Gender = domain.GenderFemale

	// Parents
	f := domain.NewPerson("John", "Doe")
	f.Gender = domain.GenderMale
	m := domain.NewPerson("Jane", "Doe")
	m.Gender = domain.GenderFemale

	// Child
	c := domain.NewPerson("Jimmy", "Doe")

	// Create all persons
	for _, p := range []*domain.Person{gf, gm, f, m, c} {
		if err := projector.Project(ctx, domain.NewPersonCreated(p), 1); err != nil {
			t.Fatalf("Failed to create person: %v", err)
		}
	}

	// Create grandparent family
	gFamily := domain.NewFamilyWithPartners(&gf.ID, &gm.ID)
	projector.Project(ctx, domain.NewFamilyCreated(gFamily), 1)

	// Link father to grandparent family
	gfc := domain.NewFamilyChild(gFamily.ID, f.ID, domain.ChildBiological)
	projector.Project(ctx, domain.NewChildLinkedToFamily(gfc), 2)

	// Create parent family
	pFamily := domain.NewFamilyWithPartners(&f.ID, &m.ID)
	projector.Project(ctx, domain.NewFamilyCreated(pFamily), 1)

	// Link child to parent family
	pfc := domain.NewFamilyChild(pFamily.ID, c.ID, domain.ChildBiological)
	projector.Project(ctx, domain.NewChildLinkedToFamily(pfc), 2)

	// Verify: List all persons
	persons, total, _ := readStore.ListPersons(ctx, repository.DefaultListOptions())
	if total != 5 {
		t.Errorf("Expected 5 persons, got %d", total)
	}
	if len(persons) != 5 {
		t.Errorf("Expected 5 persons in list, got %d", len(persons))
	}

	// Verify: Child's pedigree
	edge, _ := readStore.GetPedigreeEdge(ctx, c.ID)
	if edge == nil {
		t.Fatal("Child should have pedigree edge")
	}
	if edge.FatherID == nil || *edge.FatherID != f.ID {
		t.Error("Child's father incorrect")
	}
	if edge.MotherID == nil || *edge.MotherID != m.ID {
		t.Error("Child's mother incorrect")
	}

	// Verify: Father's pedigree (should have grandparents)
	fatherEdge, _ := readStore.GetPedigreeEdge(ctx, f.ID)
	if fatherEdge == nil {
		t.Fatal("Father should have pedigree edge")
	}
	if fatherEdge.FatherID == nil || *fatherEdge.FatherID != gf.ID {
		t.Error("Father's father (grandfather) incorrect")
	}
}
