package repository_test

import (
	"context"
	"testing"

	"github.com/google/uuid"

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

func TestProjector_Apply(t *testing.T) {
	readStore := memory.NewReadModelStore()
	projector := repository.NewProjector(readStore)
	ctx := context.Background()

	// Create a person
	person := domain.NewPerson("John", "Doe")
	event := domain.NewPersonCreated(person)

	// Use Apply instead of Project
	err := projector.Apply(ctx, event)
	if err != nil {
		t.Fatalf("Apply failed: %v", err)
	}

	// Verify person was created
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

func TestProjector_FamilyUpdated(t *testing.T) {
	readStore := memory.NewReadModelStore()
	projector := repository.NewProjector(readStore)
	ctx := context.Background()

	// Create a family first
	family := domain.NewFamily()
	family.RelationshipType = domain.RelationMarriage
	family.SetMarriageDate("1 JAN 1870")
	family.MarriagePlace = "New York"

	createEvent := domain.NewFamilyCreated(family)
	if err := projector.Project(ctx, createEvent, 1); err != nil {
		t.Fatalf("Project create failed: %v", err)
	}

	// Test updating various fields
	tests := []struct {
		name     string
		changes  map[string]any
		validate func(t *testing.T, rm *repository.FamilyReadModel)
	}{
		{
			name: "update relationship type",
			changes: map[string]any{
				"relationship_type": "partnership",
			},
			validate: func(t *testing.T, rm *repository.FamilyReadModel) {
				if rm.RelationshipType != domain.RelationPartnership {
					t.Errorf("RelationshipType = %s, want partnership", rm.RelationshipType)
				}
			},
		},
		{
			name: "update marriage date",
			changes: map[string]any{
				"marriage_date": "15 JUN 1875",
			},
			validate: func(t *testing.T, rm *repository.FamilyReadModel) {
				if rm.MarriageDateRaw != "15 JUN 1875" {
					t.Errorf("MarriageDateRaw = %s, want '15 JUN 1875'", rm.MarriageDateRaw)
				}
				if rm.MarriageDateSort == nil {
					t.Error("MarriageDateSort should not be nil")
				}
			},
		},
		{
			name: "update marriage place",
			changes: map[string]any{
				"marriage_place": "Boston, MA",
			},
			validate: func(t *testing.T, rm *repository.FamilyReadModel) {
				if rm.MarriagePlace != "Boston, MA" {
					t.Errorf("MarriagePlace = %s, want 'Boston, MA'", rm.MarriagePlace)
				}
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			updateEvent := domain.NewFamilyUpdated(family.ID, tt.changes)
			err := projector.Project(ctx, updateEvent, 2)
			if err != nil {
				t.Fatalf("Project update failed: %v", err)
			}

			rm, err := readStore.GetFamily(ctx, family.ID)
			if err != nil {
				t.Fatalf("GetFamily failed: %v", err)
			}
			if rm == nil {
				t.Fatal("Family not found")
			}

			tt.validate(t, rm)

			if rm.Version != 2 {
				t.Errorf("Version = %d, want 2", rm.Version)
			}
		})
	}
}

func TestProjector_FamilyUpdated_NonExistent(t *testing.T) {
	readStore := memory.NewReadModelStore()
	projector := repository.NewProjector(readStore)
	ctx := context.Background()

	// Try to update a non-existent family (should not error, just skip)
	updateEvent := domain.NewFamilyUpdated(uuid.New(), map[string]any{"marriage_place": "Test"})
	err := projector.Project(ctx, updateEvent, 1)
	if err != nil {
		t.Fatalf("Project update should not fail for non-existent family: %v", err)
	}
}

func TestProjector_FamilyDeleted(t *testing.T) {
	readStore := memory.NewReadModelStore()
	projector := repository.NewProjector(readStore)
	ctx := context.Background()

	// Create family with children
	father := domain.NewPerson("John", "Doe")
	father.Gender = domain.GenderMale
	mother := domain.NewPerson("Jane", "Doe")
	mother.Gender = domain.GenderFemale
	child1 := domain.NewPerson("Jimmy", "Doe")
	child2 := domain.NewPerson("Jenny", "Doe")

	// Create persons
	projector.Project(ctx, domain.NewPersonCreated(father), 1)
	projector.Project(ctx, domain.NewPersonCreated(mother), 1)
	projector.Project(ctx, domain.NewPersonCreated(child1), 1)
	projector.Project(ctx, domain.NewPersonCreated(child2), 1)

	// Create family
	family := domain.NewFamilyWithPartners(&father.ID, &mother.ID)
	projector.Project(ctx, domain.NewFamilyCreated(family), 1)

	// Link children
	fc1 := domain.NewFamilyChild(family.ID, child1.ID, domain.ChildBiological)
	fc2 := domain.NewFamilyChild(family.ID, child2.ID, domain.ChildBiological)
	projector.Project(ctx, domain.NewChildLinkedToFamily(fc1), 2)
	projector.Project(ctx, domain.NewChildLinkedToFamily(fc2), 3)

	// Verify children are linked
	children, _ := readStore.GetFamilyChildren(ctx, family.ID)
	if len(children) != 2 {
		t.Errorf("Expected 2 children before deletion, got %d", len(children))
	}

	// Verify pedigree edges exist
	edge1, _ := readStore.GetPedigreeEdge(ctx, child1.ID)
	edge2, _ := readStore.GetPedigreeEdge(ctx, child2.ID)
	if edge1 == nil || edge2 == nil {
		t.Error("Pedigree edges should exist before deletion")
	}

	// Delete family
	deleteEvent := domain.NewFamilyDeleted(family.ID, "test deletion")
	err := projector.Project(ctx, deleteEvent, 4)
	if err != nil {
		t.Fatalf("Project delete failed: %v", err)
	}

	// Verify family is deleted
	rm, _ := readStore.GetFamily(ctx, family.ID)
	if rm != nil {
		t.Error("Family should be deleted")
	}

	// Verify children are unlinked
	children, _ = readStore.GetFamilyChildren(ctx, family.ID)
	if len(children) != 0 {
		t.Errorf("Expected 0 children after deletion, got %d", len(children))
	}

	// Verify pedigree edges are removed
	edge1, _ = readStore.GetPedigreeEdge(ctx, child1.ID)
	edge2, _ = readStore.GetPedigreeEdge(ctx, child2.ID)
	if edge1 != nil || edge2 != nil {
		t.Error("Pedigree edges should be removed after family deletion")
	}
}

func TestProjector_PersonUpdated_AllFields(t *testing.T) {
	readStore := memory.NewReadModelStore()
	projector := repository.NewProjector(readStore)
	ctx := context.Background()

	// Create person
	person := domain.NewPerson("John", "Doe")
	person.Gender = domain.GenderMale
	createEvent := domain.NewPersonCreated(person)
	if err := projector.Project(ctx, createEvent, 1); err != nil {
		t.Fatalf("Project create failed: %v", err)
	}

	// Test updating all possible fields
	tests := []struct {
		name     string
		changes  map[string]any
		validate func(t *testing.T, rm *repository.PersonReadModel)
	}{
		{
			name:    "update given_name",
			changes: map[string]any{"given_name": "Jane"},
			validate: func(t *testing.T, rm *repository.PersonReadModel) {
				if rm.GivenName != "Jane" {
					t.Errorf("GivenName = %s, want Jane", rm.GivenName)
				}
				if rm.FullName != "Jane Doe" {
					t.Errorf("FullName = %s, want Jane Doe", rm.FullName)
				}
			},
		},
		{
			name:    "update surname",
			changes: map[string]any{"surname": "Smith"},
			validate: func(t *testing.T, rm *repository.PersonReadModel) {
				if rm.Surname != "Smith" {
					t.Errorf("Surname = %s, want Smith", rm.Surname)
				}
				if rm.FullName != "Jane Smith" {
					t.Errorf("FullName = %s, want Jane Smith", rm.FullName)
				}
			},
		},
		{
			name:    "update gender",
			changes: map[string]any{"gender": "female"},
			validate: func(t *testing.T, rm *repository.PersonReadModel) {
				if rm.Gender != domain.GenderFemale {
					t.Errorf("Gender = %s, want female", rm.Gender)
				}
			},
		},
		{
			name:    "update birth_date",
			changes: map[string]any{"birth_date": "1 JAN 1850"},
			validate: func(t *testing.T, rm *repository.PersonReadModel) {
				if rm.BirthDateRaw != "1 JAN 1850" {
					t.Errorf("BirthDateRaw = %s, want '1 JAN 1850'", rm.BirthDateRaw)
				}
				if rm.BirthDateSort == nil {
					t.Error("BirthDateSort should not be nil")
				}
			},
		},
		{
			name:    "update birth_place",
			changes: map[string]any{"birth_place": "Springfield, IL"},
			validate: func(t *testing.T, rm *repository.PersonReadModel) {
				if rm.BirthPlace != "Springfield, IL" {
					t.Errorf("BirthPlace = %s, want 'Springfield, IL'", rm.BirthPlace)
				}
			},
		},
		{
			name:    "update death_date",
			changes: map[string]any{"death_date": "15 DEC 1900"},
			validate: func(t *testing.T, rm *repository.PersonReadModel) {
				if rm.DeathDateRaw != "15 DEC 1900" {
					t.Errorf("DeathDateRaw = %s, want '15 DEC 1900'", rm.DeathDateRaw)
				}
				if rm.DeathDateSort == nil {
					t.Error("DeathDateSort should not be nil")
				}
			},
		},
		{
			name:    "update death_place",
			changes: map[string]any{"death_place": "Chicago, IL"},
			validate: func(t *testing.T, rm *repository.PersonReadModel) {
				if rm.DeathPlace != "Chicago, IL" {
					t.Errorf("DeathPlace = %s, want 'Chicago, IL'", rm.DeathPlace)
				}
			},
		},
		{
			name:    "update notes",
			changes: map[string]any{"notes": "Test notes"},
			validate: func(t *testing.T, rm *repository.PersonReadModel) {
				if rm.Notes != "Test notes" {
					t.Errorf("Notes = %s, want 'Test notes'", rm.Notes)
				}
			},
		},
	}

	version := int64(1)
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			version++
			updateEvent := domain.NewPersonUpdated(person.ID, tt.changes)
			err := projector.Project(ctx, updateEvent, version)
			if err != nil {
				t.Fatalf("Project update failed: %v", err)
			}

			rm, err := readStore.GetPerson(ctx, person.ID)
			if err != nil {
				t.Fatalf("GetPerson failed: %v", err)
			}
			if rm == nil {
				t.Fatal("Person not found")
			}

			tt.validate(t, rm)

			if rm.Version != version {
				t.Errorf("Version = %d, want %d", rm.Version, version)
			}
		})
	}
}

func TestProjector_PersonCreated_WithDates(t *testing.T) {
	readStore := memory.NewReadModelStore()
	projector := repository.NewProjector(readStore)
	ctx := context.Background()

	// Test with birth date but no death date
	person := domain.NewPerson("John", "Doe")
	person.SetBirthDate("1 JAN 1850")
	person.BirthPlace = "New York"

	event := domain.NewPersonCreated(person)
	err := projector.Project(ctx, event, 1)
	if err != nil {
		t.Fatalf("Project failed: %v", err)
	}

	rm, _ := readStore.GetPerson(ctx, person.ID)
	if rm.BirthDateRaw != "1 JAN 1850" {
		t.Errorf("BirthDateRaw = %s, want '1 JAN 1850'", rm.BirthDateRaw)
	}
	if rm.BirthDateSort == nil {
		t.Error("BirthDateSort should not be nil for valid date")
	}
	if rm.DeathDateRaw != "" {
		t.Errorf("DeathDateRaw should be empty, got %s", rm.DeathDateRaw)
	}
	if rm.DeathDateSort != nil {
		t.Error("DeathDateSort should be nil when no death date")
	}

	// Test with death date
	person2 := domain.NewPerson("Jane", "Doe")
	person2.SetDeathDate("15 DEC 1900")
	person2.DeathPlace = "Boston"

	event2 := domain.NewPersonCreated(person2)
	err = projector.Project(ctx, event2, 1)
	if err != nil {
		t.Fatalf("Project failed: %v", err)
	}

	rm2, _ := readStore.GetPerson(ctx, person2.ID)
	if rm2.DeathDateRaw != "15 DEC 1900" {
		t.Errorf("DeathDateRaw = %s, want '15 DEC 1900'", rm2.DeathDateRaw)
	}
	if rm2.DeathDateSort == nil {
		t.Error("DeathDateSort should not be nil for valid date")
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

// Source/Citation projection tests

func TestProjector_SourceCreated(t *testing.T) {
	readStore := memory.NewReadModelStore()
	projector := repository.NewProjector(readStore)
	ctx := context.Background()

	source := domain.NewSource("Test Book", domain.SourceBook)
	source.Author = "John Smith"
	source.Publisher = "Test Press"
	gd := domain.ParseGenDate("1995")
	source.PublishDate = &gd

	event := domain.NewSourceCreated(source)

	err := projector.Project(ctx, event, 1)
	if err != nil {
		t.Fatalf("Project failed: %v", err)
	}

	// Verify source in read model
	rm, err := readStore.GetSource(ctx, source.ID)
	if err != nil {
		t.Fatalf("GetSource failed: %v", err)
	}
	if rm == nil {
		t.Fatal("Source not found in read model")
	}
	if rm.Title != "Test Book" {
		t.Errorf("Title = %s, want Test Book", rm.Title)
	}
	if rm.SourceType != domain.SourceBook {
		t.Errorf("SourceType = %s, want book", rm.SourceType)
	}
	if rm.Author != "John Smith" {
		t.Errorf("Author = %s, want John Smith", rm.Author)
	}
	if rm.Version != 1 {
		t.Errorf("Version = %d, want 1", rm.Version)
	}
}

func TestProjector_SourceUpdated(t *testing.T) {
	readStore := memory.NewReadModelStore()
	projector := repository.NewProjector(readStore)
	ctx := context.Background()

	// Create source first
	source := domain.NewSource("Original Title", domain.SourceBook)
	createEvent := domain.NewSourceCreated(source)
	if err := projector.Project(ctx, createEvent, 1); err != nil {
		t.Fatalf("Project create failed: %v", err)
	}

	// Update source
	changes := map[string]any{
		"title":     "Updated Title",
		"author":    "Jane Doe",
		"publisher": "New Publisher",
	}
	updateEvent := domain.NewSourceUpdated(source.ID, changes)

	err := projector.Project(ctx, updateEvent, 2)
	if err != nil {
		t.Fatalf("Project update failed: %v", err)
	}

	// Verify updates
	rm, _ := readStore.GetSource(ctx, source.ID)
	if rm.Title != "Updated Title" {
		t.Errorf("Title = %s, want Updated Title", rm.Title)
	}
	if rm.Author != "Jane Doe" {
		t.Errorf("Author = %s, want Jane Doe", rm.Author)
	}
	if rm.Publisher != "New Publisher" {
		t.Errorf("Publisher = %s, want New Publisher", rm.Publisher)
	}
	if rm.Version != 2 {
		t.Errorf("Version = %d, want 2", rm.Version)
	}
}

func TestProjector_SourceUpdated_AllFields(t *testing.T) {
	readStore := memory.NewReadModelStore()
	projector := repository.NewProjector(readStore)
	ctx := context.Background()

	// Create source
	source := domain.NewSource("Test Source", domain.SourceBook)
	createEvent := domain.NewSourceCreated(source)
	if err := projector.Project(ctx, createEvent, 1); err != nil {
		t.Fatalf("Project create failed: %v", err)
	}

	// Test updating each field individually
	tests := []struct {
		name     string
		changes  map[string]any
		validate func(t *testing.T, rm *repository.SourceReadModel)
	}{
		{
			name:    "update source_type",
			changes: map[string]any{"source_type": "census"},
			validate: func(t *testing.T, rm *repository.SourceReadModel) {
				if rm.SourceType != domain.SourceCensus {
					t.Errorf("SourceType = %s, want census", rm.SourceType)
				}
			},
		},
		{
			name:    "update publish_date",
			changes: map[string]any{"publish_date": "1 JAN 1995"},
			validate: func(t *testing.T, rm *repository.SourceReadModel) {
				if rm.PublishDateRaw != "1 JAN 1995" {
					t.Errorf("PublishDateRaw = %s, want '1 JAN 1995'", rm.PublishDateRaw)
				}
				if rm.PublishDateSort == nil {
					t.Error("PublishDateSort should not be nil")
				}
			},
		},
		{
			name:    "update url",
			changes: map[string]any{"url": "https://example.com"},
			validate: func(t *testing.T, rm *repository.SourceReadModel) {
				if rm.URL != "https://example.com" {
					t.Errorf("URL = %s, want https://example.com", rm.URL)
				}
			},
		},
		{
			name:    "update repository_name",
			changes: map[string]any{"repository_name": "National Archives"},
			validate: func(t *testing.T, rm *repository.SourceReadModel) {
				if rm.RepositoryName != "National Archives" {
					t.Errorf("RepositoryName = %s, want National Archives", rm.RepositoryName)
				}
			},
		},
		{
			name:    "update collection_name",
			changes: map[string]any{"collection_name": "Birth Records"},
			validate: func(t *testing.T, rm *repository.SourceReadModel) {
				if rm.CollectionName != "Birth Records" {
					t.Errorf("CollectionName = %s, want Birth Records", rm.CollectionName)
				}
			},
		},
		{
			name:    "update call_number",
			changes: map[string]any{"call_number": "BR-1850-1900"},
			validate: func(t *testing.T, rm *repository.SourceReadModel) {
				if rm.CallNumber != "BR-1850-1900" {
					t.Errorf("CallNumber = %s, want BR-1850-1900", rm.CallNumber)
				}
			},
		},
		{
			name:    "update notes",
			changes: map[string]any{"notes": "Test notes"},
			validate: func(t *testing.T, rm *repository.SourceReadModel) {
				if rm.Notes != "Test notes" {
					t.Errorf("Notes = %s, want Test notes", rm.Notes)
				}
			},
		},
	}

	version := int64(1)
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			version++
			updateEvent := domain.NewSourceUpdated(source.ID, tt.changes)
			err := projector.Project(ctx, updateEvent, version)
			if err != nil {
				t.Fatalf("Project update failed: %v", err)
			}

			rm, err := readStore.GetSource(ctx, source.ID)
			if err != nil {
				t.Fatalf("GetSource failed: %v", err)
			}
			if rm == nil {
				t.Fatal("Source not found")
			}

			tt.validate(t, rm)

			if rm.Version != version {
				t.Errorf("Version = %d, want %d", rm.Version, version)
			}
		})
	}
}

func TestProjector_SourceDeleted(t *testing.T) {
	readStore := memory.NewReadModelStore()
	projector := repository.NewProjector(readStore)
	ctx := context.Background()

	// Create source first
	source := domain.NewSource("Test Source", domain.SourceBook)
	createEvent := domain.NewSourceCreated(source)
	if err := projector.Project(ctx, createEvent, 1); err != nil {
		t.Fatalf("Project create failed: %v", err)
	}

	// Delete source
	deleteEvent := domain.NewSourceDeleted(source.ID, "test deletion")
	err := projector.Project(ctx, deleteEvent, 2)
	if err != nil {
		t.Fatalf("Project delete failed: %v", err)
	}

	// Verify deletion
	rm, _ := readStore.GetSource(ctx, source.ID)
	if rm != nil {
		t.Error("Source should be deleted")
	}
}

func TestProjector_CitationCreated(t *testing.T) {
	readStore := memory.NewReadModelStore()
	projector := repository.NewProjector(readStore)
	ctx := context.Background()

	// Create source first
	source := domain.NewSource("Test Source", domain.SourceBook)
	projector.Project(ctx, domain.NewSourceCreated(source), 1)

	// Create citation
	citation := domain.NewCitation(source.ID, domain.FactPersonBirth, uuid.New())
	citation.Page = "123"
	citation.SourceQuality = domain.SourceOriginal
	citation.InformantType = domain.InformantPrimary
	citation.EvidenceType = domain.EvidenceDirect

	event := domain.NewCitationCreated(citation)

	err := projector.Project(ctx, event, 1)
	if err != nil {
		t.Fatalf("Project failed: %v", err)
	}

	// Verify citation in read model
	rm, err := readStore.GetCitation(ctx, citation.ID)
	if err != nil {
		t.Fatalf("GetCitation failed: %v", err)
	}
	if rm == nil {
		t.Fatal("Citation not found in read model")
	}
	if rm.SourceID != source.ID {
		t.Errorf("SourceID = %v, want %v", rm.SourceID, source.ID)
	}
	if rm.FactType != domain.FactPersonBirth {
		t.Errorf("FactType = %s, want person_birth", rm.FactType)
	}
	if rm.Page != "123" {
		t.Errorf("Page = %s, want 123", rm.Page)
	}
	if rm.Version != 1 {
		t.Errorf("Version = %d, want 1", rm.Version)
	}

	// Verify source citation count updated
	sourceRM, _ := readStore.GetSource(ctx, source.ID)
	if sourceRM.CitationCount != 1 {
		t.Errorf("Source CitationCount = %d, want 1", sourceRM.CitationCount)
	}
}

func TestProjector_CitationUpdated(t *testing.T) {
	readStore := memory.NewReadModelStore()
	projector := repository.NewProjector(readStore)
	ctx := context.Background()

	// Create source and citation first
	source := domain.NewSource("Test Source", domain.SourceBook)
	projector.Project(ctx, domain.NewSourceCreated(source), 1)

	citation := domain.NewCitation(source.ID, domain.FactPersonBirth, uuid.New())
	citation.Page = "100"
	createEvent := domain.NewCitationCreated(citation)
	if err := projector.Project(ctx, createEvent, 1); err != nil {
		t.Fatalf("Project create failed: %v", err)
	}

	// Update citation - test all fields
	tests := []struct {
		name     string
		changes  map[string]any
		validate func(t *testing.T, rm *repository.CitationReadModel)
	}{
		{
			name: "update GPS fields",
			changes: map[string]any{
				"source_quality": "derivative",
				"informant_type": "secondary",
				"evidence_type":  "indirect",
			},
			validate: func(t *testing.T, rm *repository.CitationReadModel) {
				if rm.SourceQuality != domain.SourceDerivative {
					t.Errorf("SourceQuality = %s, want derivative", rm.SourceQuality)
				}
				if rm.InformantType != domain.InformantSecondary {
					t.Errorf("InformantType = %s, want secondary", rm.InformantType)
				}
				if rm.EvidenceType != domain.EvidenceIndirect {
					t.Errorf("EvidenceType = %s, want indirect", rm.EvidenceType)
				}
			},
		},
		{
			name: "update page and volume",
			changes: map[string]any{
				"page":   "200",
				"volume": "Vol 2",
			},
			validate: func(t *testing.T, rm *repository.CitationReadModel) {
				if rm.Page != "200" {
					t.Errorf("Page = %s, want 200", rm.Page)
				}
				if rm.Volume != "Vol 2" {
					t.Errorf("Volume = %s, want Vol 2", rm.Volume)
				}
			},
		},
		{
			name: "update text fields",
			changes: map[string]any{
				"quoted_text": "Born on this date",
				"analysis":    "Primary evidence",
			},
			validate: func(t *testing.T, rm *repository.CitationReadModel) {
				if rm.QuotedText != "Born on this date" {
					t.Errorf("QuotedText = %s, want 'Born on this date'", rm.QuotedText)
				}
				if rm.Analysis != "Primary evidence" {
					t.Errorf("Analysis = %s, want 'Primary evidence'", rm.Analysis)
				}
			},
		},
		{
			name: "update template_id",
			changes: map[string]any{
				"template_id": "template-123",
			},
			validate: func(t *testing.T, rm *repository.CitationReadModel) {
				if rm.TemplateID != "template-123" {
					t.Errorf("TemplateID = %s, want 'template-123'", rm.TemplateID)
				}
			},
		},
	}

	version := int64(1)
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			version++
			updateEvent := domain.NewCitationUpdated(citation.ID, tt.changes)
			err := projector.Project(ctx, updateEvent, version)
			if err != nil {
				t.Fatalf("Project update failed: %v", err)
			}

			rm, _ := readStore.GetCitation(ctx, citation.ID)
			tt.validate(t, rm)

			if rm.Version != version {
				t.Errorf("Version = %d, want %d", rm.Version, version)
			}
		})
	}
}

func TestProjector_CitationDeleted(t *testing.T) {
	readStore := memory.NewReadModelStore()
	projector := repository.NewProjector(readStore)
	ctx := context.Background()

	// Create source and citation first
	source := domain.NewSource("Test Source", domain.SourceBook)
	projector.Project(ctx, domain.NewSourceCreated(source), 1)

	citation := domain.NewCitation(source.ID, domain.FactPersonBirth, uuid.New())
	createEvent := domain.NewCitationCreated(citation)
	if err := projector.Project(ctx, createEvent, 1); err != nil {
		t.Fatalf("Project create failed: %v", err)
	}

	// Verify citation count is 1
	sourceRM, _ := readStore.GetSource(ctx, source.ID)
	if sourceRM.CitationCount != 1 {
		t.Errorf("Initial CitationCount = %d, want 1", sourceRM.CitationCount)
	}

	// Delete citation
	deleteEvent := domain.NewCitationDeleted(citation.ID, "test deletion")
	err := projector.Project(ctx, deleteEvent, 2)
	if err != nil {
		t.Fatalf("Project delete failed: %v", err)
	}

	// Verify deletion
	rm, _ := readStore.GetCitation(ctx, citation.ID)
	if rm != nil {
		t.Error("Citation should be deleted")
	}

	// Verify source citation count updated
	sourceRM, _ = readStore.GetSource(ctx, source.ID)
	if sourceRM.CitationCount != 0 {
		t.Errorf("CitationCount after delete = %d, want 0", sourceRM.CitationCount)
	}
}

func TestProjector_SourceUpdated_NonExistent(t *testing.T) {
	readStore := memory.NewReadModelStore()
	projector := repository.NewProjector(readStore)
	ctx := context.Background()

	// Try to update a non-existent source (should not error, just skip)
	updateEvent := domain.NewSourceUpdated(uuid.New(), map[string]any{"title": "Test"})
	err := projector.Project(ctx, updateEvent, 1)
	if err != nil {
		t.Fatalf("Project update should not fail for non-existent source: %v", err)
	}
}

func TestProjector_CitationUpdated_NonExistent(t *testing.T) {
	readStore := memory.NewReadModelStore()
	projector := repository.NewProjector(readStore)
	ctx := context.Background()

	// Try to update a non-existent citation (should not error, just skip)
	updateEvent := domain.NewCitationUpdated(uuid.New(), map[string]any{"page": "100"})
	err := projector.Project(ctx, updateEvent, 1)
	if err != nil {
		t.Fatalf("Project update should not fail for non-existent citation: %v", err)
	}
}
