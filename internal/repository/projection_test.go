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

	// Test with a GedcomImported event which is handled gracefully
	// (no projection action needed for this event type)
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

// Media Projection Tests

func TestProjector_MediaCreated(t *testing.T) {
	readStore := memory.NewReadModelStore()
	projector := repository.NewProjector(readStore)
	ctx := context.Background()

	entityID := uuid.New()
	media := domain.NewMedia("Family Photo", "person", entityID)
	media.Description = "A photo from 1950"
	media.MimeType = "image/jpeg"
	media.MediaType = domain.MediaPhoto
	media.Filename = "family.jpg"
	media.FileSize = 1024
	media.FileData = []byte("fake data")
	media.ThumbnailData = []byte("fake thumbnail")
	media.GedcomXref = "@M1@"

	event := domain.NewMediaCreated(media)
	err := projector.Project(ctx, event, 1)
	if err != nil {
		t.Fatalf("Project MediaCreated failed: %v", err)
	}

	// Verify media was created
	retrieved, err := readStore.GetMediaWithData(ctx, media.ID)
	if err != nil {
		t.Fatalf("GetMediaWithData failed: %v", err)
	}
	if retrieved == nil {
		t.Fatal("Media not found after projection")
	}
	if retrieved.Title != "Family Photo" {
		t.Errorf("Title = %s, want Family Photo", retrieved.Title)
	}
	if retrieved.EntityType != "person" {
		t.Errorf("EntityType = %s, want person", retrieved.EntityType)
	}
	if retrieved.Version != 1 {
		t.Errorf("Version = %d, want 1", retrieved.Version)
	}
	if len(retrieved.FileData) == 0 {
		t.Error("FileData should be present")
	}
	if len(retrieved.ThumbnailData) == 0 {
		t.Error("ThumbnailData should be present")
	}
}

func TestProjector_MediaUpdated(t *testing.T) {
	readStore := memory.NewReadModelStore()
	projector := repository.NewProjector(readStore)
	ctx := context.Background()

	// First create the media
	entityID := uuid.New()
	media := domain.NewMedia("Original Title", "person", entityID)
	media.MimeType = "image/jpeg"
	media.FileSize = 1024
	media.FileData = []byte("fake data")

	createEvent := domain.NewMediaCreated(media)
	_ = projector.Project(ctx, createEvent, 1)

	// Update media
	changes := map[string]any{
		"title":       "Updated Title",
		"description": "New description",
		"media_type":  "document",
		"crop_left":   10,
		"crop_top":    20,
		"crop_width":  100,
		"crop_height": 150,
	}
	updateEvent := domain.NewMediaUpdated(media.ID, changes)
	err := projector.Project(ctx, updateEvent, 2)
	if err != nil {
		t.Fatalf("Project MediaUpdated failed: %v", err)
	}

	// Verify changes
	retrieved, _ := readStore.GetMedia(ctx, media.ID)
	if retrieved == nil {
		t.Fatal("Media not found after update")
	}
	if retrieved.Title != "Updated Title" {
		t.Errorf("Title = %s, want Updated Title", retrieved.Title)
	}
	if retrieved.Description != "New description" {
		t.Errorf("Description = %s, want New description", retrieved.Description)
	}
	if retrieved.MediaType != domain.MediaDocument {
		t.Errorf("MediaType = %s, want document", retrieved.MediaType)
	}
	if retrieved.Version != 2 {
		t.Errorf("Version = %d, want 2", retrieved.Version)
	}
	if retrieved.CropLeft == nil || *retrieved.CropLeft != 10 {
		t.Error("CropLeft not set correctly")
	}
}

func TestProjector_MediaDeleted(t *testing.T) {
	readStore := memory.NewReadModelStore()
	projector := repository.NewProjector(readStore)
	ctx := context.Background()

	// First create the media
	entityID := uuid.New()
	media := domain.NewMedia("To Delete", "person", entityID)
	createEvent := domain.NewMediaCreated(media)
	_ = projector.Project(ctx, createEvent, 1)

	// Delete media
	deleteEvent := domain.NewMediaDeleted(media.ID, "test deletion")
	err := projector.Project(ctx, deleteEvent, 2)
	if err != nil {
		t.Fatalf("Project MediaDeleted failed: %v", err)
	}

	// Verify deletion
	retrieved, _ := readStore.GetMedia(ctx, media.ID)
	if retrieved != nil {
		t.Error("Media should be deleted")
	}
}

func TestProjector_MediaUpdated_NonExistent(t *testing.T) {
	readStore := memory.NewReadModelStore()
	projector := repository.NewProjector(readStore)
	ctx := context.Background()

	// Try to update a non-existent media (should not error, just skip)
	updateEvent := domain.NewMediaUpdated(uuid.New(), map[string]any{"title": "Test"})
	err := projector.Project(ctx, updateEvent, 1)
	if err != nil {
		t.Fatalf("Project update should not fail for non-existent media: %v", err)
	}
}

// LifeEvent Projection Tests

func TestProjector_LifeEventCreated_ForPerson(t *testing.T) {
	readStore := memory.NewReadModelStore()
	projector := repository.NewProjector(readStore)
	ctx := context.Background()

	// Create a person first
	person := domain.NewPerson("John", "Doe")
	projector.Project(ctx, domain.NewPersonCreated(person), 1)

	// Create a life event for this person
	lifeEvent := domain.NewLifeEvent(person.ID, domain.FactPersonBirth)
	gd := domain.ParseGenDate("1 JAN 1850")
	lifeEvent.Date = &gd
	lifeEvent.Place = "Springfield, IL"
	lifeEvent.Description = "Born at home"
	lifeEvent.Age = "0"

	event := domain.NewLifeEventCreatedFromModel(lifeEvent)
	err := projector.Project(ctx, event, 1)
	if err != nil {
		t.Fatalf("Project LifeEventCreated failed: %v", err)
	}

	// Verify life event was created
	rm, err := readStore.GetEvent(ctx, lifeEvent.ID)
	if err != nil {
		t.Fatalf("GetEvent failed: %v", err)
	}
	if rm == nil {
		t.Fatal("Life event not found in read model")
	}
	if rm.OwnerType != "person" {
		t.Errorf("OwnerType = %s, want person", rm.OwnerType)
	}
	if rm.OwnerID != person.ID {
		t.Errorf("OwnerID = %v, want %v", rm.OwnerID, person.ID)
	}
	if rm.FactType != domain.FactPersonBirth {
		t.Errorf("FactType = %s, want person_birth", rm.FactType)
	}
	if rm.DateRaw != "1 JAN 1850" {
		t.Errorf("DateRaw = %s, want '1 JAN 1850'", rm.DateRaw)
	}
	if rm.DateSort == nil {
		t.Error("DateSort should not be nil for valid date")
	}
	if rm.Place != "Springfield, IL" {
		t.Errorf("Place = %s, want Springfield, IL", rm.Place)
	}
	if rm.Description != "Born at home" {
		t.Errorf("Description = %s, want 'Born at home'", rm.Description)
	}
	if rm.Age != "0" {
		t.Errorf("Age = %s, want '0'", rm.Age)
	}
	if rm.Version != 1 {
		t.Errorf("Version = %d, want 1", rm.Version)
	}
}

func TestProjector_LifeEventCreated_ForFamily(t *testing.T) {
	readStore := memory.NewReadModelStore()
	projector := repository.NewProjector(readStore)
	ctx := context.Background()

	// Create a family
	family := domain.NewFamily()
	projector.Project(ctx, domain.NewFamilyCreated(family), 1)

	// Create a life event for this family (e.g., marriage)
	lifeEvent := domain.NewFamilyLifeEvent(family.ID, domain.FactFamilyMarriage)
	gd := domain.ParseGenDate("15 JUN 1870")
	lifeEvent.Date = &gd
	lifeEvent.Place = "New York, NY"

	event := domain.NewLifeEventCreatedFromModel(lifeEvent)
	err := projector.Project(ctx, event, 1)
	if err != nil {
		t.Fatalf("Project LifeEventCreated for family failed: %v", err)
	}

	// Verify life event was created
	rm, err := readStore.GetEvent(ctx, lifeEvent.ID)
	if err != nil {
		t.Fatalf("GetEvent failed: %v", err)
	}
	if rm == nil {
		t.Fatal("Life event not found in read model")
	}
	if rm.OwnerType != "family" {
		t.Errorf("OwnerType = %s, want family", rm.OwnerType)
	}
	if rm.OwnerID != family.ID {
		t.Errorf("OwnerID = %v, want %v", rm.OwnerID, family.ID)
	}
	if rm.FactType != domain.FactFamilyMarriage {
		t.Errorf("FactType = %s, want family_marriage", rm.FactType)
	}
	if rm.DateRaw != "15 JUN 1870" {
		t.Errorf("DateRaw = %s, want '15 JUN 1870'", rm.DateRaw)
	}
	if rm.Place != "New York, NY" {
		t.Errorf("Place = %s, want 'New York, NY'", rm.Place)
	}
}

func TestProjector_LifeEventCreated_WithCause(t *testing.T) {
	readStore := memory.NewReadModelStore()
	projector := repository.NewProjector(readStore)
	ctx := context.Background()

	// Create a person
	person := domain.NewPerson("John", "Doe")
	projector.Project(ctx, domain.NewPersonCreated(person), 1)

	// Create a death event with cause
	lifeEvent := domain.NewLifeEvent(person.ID, domain.FactPersonDeath)
	gd := domain.ParseGenDate("15 DEC 1920")
	lifeEvent.Date = &gd
	lifeEvent.Place = "Chicago, IL"
	lifeEvent.Cause = "Natural causes"
	lifeEvent.Age = "70"

	event := domain.NewLifeEventCreatedFromModel(lifeEvent)
	err := projector.Project(ctx, event, 1)
	if err != nil {
		t.Fatalf("Project LifeEventCreated failed: %v", err)
	}

	// Verify
	rm, _ := readStore.GetEvent(ctx, lifeEvent.ID)
	if rm == nil {
		t.Fatal("Life event not found")
	}
	if rm.Cause != "Natural causes" {
		t.Errorf("Cause = %s, want 'Natural causes'", rm.Cause)
	}
	if rm.Age != "70" {
		t.Errorf("Age = %s, want '70'", rm.Age)
	}
}

func TestProjector_LifeEventCreated_WithoutDate(t *testing.T) {
	readStore := memory.NewReadModelStore()
	projector := repository.NewProjector(readStore)
	ctx := context.Background()

	// Create a person
	person := domain.NewPerson("John", "Doe")
	projector.Project(ctx, domain.NewPersonCreated(person), 1)

	// Create a life event without a date
	lifeEvent := domain.NewLifeEvent(person.ID, domain.FactPersonBirth)
	lifeEvent.Place = "Unknown location"

	event := domain.NewLifeEventCreatedFromModel(lifeEvent)
	err := projector.Project(ctx, event, 1)
	if err != nil {
		t.Fatalf("Project LifeEventCreated failed: %v", err)
	}

	// Verify
	rm, _ := readStore.GetEvent(ctx, lifeEvent.ID)
	if rm == nil {
		t.Fatal("Life event not found")
	}
	if rm.DateRaw != "" {
		t.Errorf("DateRaw = %s, want empty string", rm.DateRaw)
	}
	if rm.DateSort != nil {
		t.Error("DateSort should be nil when no date provided")
	}
}

func TestProjector_LifeEventDeleted(t *testing.T) {
	readStore := memory.NewReadModelStore()
	projector := repository.NewProjector(readStore)
	ctx := context.Background()

	// Create a person and life event
	person := domain.NewPerson("John", "Doe")
	projector.Project(ctx, domain.NewPersonCreated(person), 1)

	lifeEvent := domain.NewLifeEvent(person.ID, domain.FactPersonBirth)
	createEvent := domain.NewLifeEventCreatedFromModel(lifeEvent)
	if err := projector.Project(ctx, createEvent, 1); err != nil {
		t.Fatalf("Project create failed: %v", err)
	}

	// Verify event exists
	rm, _ := readStore.GetEvent(ctx, lifeEvent.ID)
	if rm == nil {
		t.Fatal("Life event should exist before deletion")
	}

	// Delete life event
	deleteEvent := domain.NewLifeEventDeleted(lifeEvent.ID, "test deletion")
	err := projector.Project(ctx, deleteEvent, 2)
	if err != nil {
		t.Fatalf("Project delete failed: %v", err)
	}

	// Verify deletion
	rm, _ = readStore.GetEvent(ctx, lifeEvent.ID)
	if rm != nil {
		t.Error("Life event should be deleted")
	}
}

func TestProjector_LifeEventsList(t *testing.T) {
	readStore := memory.NewReadModelStore()
	projector := repository.NewProjector(readStore)
	ctx := context.Background()

	// Create a person
	person := domain.NewPerson("John", "Doe")
	projector.Project(ctx, domain.NewPersonCreated(person), 1)

	// Create multiple life events for this person
	lifeEvent1 := domain.NewLifeEvent(person.ID, domain.FactPersonBirth)
	gd1 := domain.ParseGenDate("1 JAN 1850")
	lifeEvent1.Date = &gd1

	lifeEvent2 := domain.NewLifeEvent(person.ID, domain.FactPersonDeath)
	gd2 := domain.ParseGenDate("15 DEC 1920")
	lifeEvent2.Date = &gd2

	projector.Project(ctx, domain.NewLifeEventCreatedFromModel(lifeEvent1), 1)
	projector.Project(ctx, domain.NewLifeEventCreatedFromModel(lifeEvent2), 2)

	// List events for person
	events, err := readStore.ListEventsForPerson(ctx, person.ID)
	if err != nil {
		t.Fatalf("ListEventsForPerson failed: %v", err)
	}
	if len(events) != 2 {
		t.Errorf("Expected 2 events, got %d", len(events))
	}
}

// Attribute Projection Tests

func TestProjector_AttributeCreated(t *testing.T) {
	readStore := memory.NewReadModelStore()
	projector := repository.NewProjector(readStore)
	ctx := context.Background()

	// Create a person first
	person := domain.NewPerson("John", "Doe")
	projector.Project(ctx, domain.NewPersonCreated(person), 1)

	// Create an attribute for this person
	attribute := domain.NewAttribute(person.ID, domain.FactPersonOccupation, "Blacksmith")
	gd := domain.ParseGenDate("1875")
	attribute.Date = &gd
	attribute.Place = "Springfield, IL"

	event := domain.NewAttributeCreatedFromModel(attribute)
	err := projector.Project(ctx, event, 1)
	if err != nil {
		t.Fatalf("Project AttributeCreated failed: %v", err)
	}

	// Verify attribute was created
	rm, err := readStore.GetAttribute(ctx, attribute.ID)
	if err != nil {
		t.Fatalf("GetAttribute failed: %v", err)
	}
	if rm == nil {
		t.Fatal("Attribute not found in read model")
	}
	if rm.PersonID != person.ID {
		t.Errorf("PersonID = %v, want %v", rm.PersonID, person.ID)
	}
	if rm.FactType != domain.FactPersonOccupation {
		t.Errorf("FactType = %s, want person_occupation", rm.FactType)
	}
	if rm.Value != "Blacksmith" {
		t.Errorf("Value = %s, want Blacksmith", rm.Value)
	}
	if rm.DateRaw != "1875" {
		t.Errorf("DateRaw = %s, want '1875'", rm.DateRaw)
	}
	if rm.DateSort == nil {
		t.Error("DateSort should not be nil for valid date")
	}
	if rm.Place != "Springfield, IL" {
		t.Errorf("Place = %s, want Springfield, IL", rm.Place)
	}
	if rm.Version != 1 {
		t.Errorf("Version = %d, want 1", rm.Version)
	}
}

func TestProjector_AttributeCreated_WithoutDate(t *testing.T) {
	readStore := memory.NewReadModelStore()
	projector := repository.NewProjector(readStore)
	ctx := context.Background()

	// Create a person
	person := domain.NewPerson("John", "Doe")
	projector.Project(ctx, domain.NewPersonCreated(person), 1)

	// Create an attribute without a date
	attribute := domain.NewAttribute(person.ID, domain.FactPersonOccupation, "Farmer")

	event := domain.NewAttributeCreatedFromModel(attribute)
	err := projector.Project(ctx, event, 1)
	if err != nil {
		t.Fatalf("Project AttributeCreated failed: %v", err)
	}

	// Verify
	rm, _ := readStore.GetAttribute(ctx, attribute.ID)
	if rm == nil {
		t.Fatal("Attribute not found")
	}
	if rm.DateRaw != "" {
		t.Errorf("DateRaw = %s, want empty string", rm.DateRaw)
	}
	if rm.DateSort != nil {
		t.Error("DateSort should be nil when no date provided")
	}
	if rm.Value != "Farmer" {
		t.Errorf("Value = %s, want Farmer", rm.Value)
	}
}

func TestProjector_AttributeDeleted(t *testing.T) {
	readStore := memory.NewReadModelStore()
	projector := repository.NewProjector(readStore)
	ctx := context.Background()

	// Create a person and attribute
	person := domain.NewPerson("John", "Doe")
	projector.Project(ctx, domain.NewPersonCreated(person), 1)

	attribute := domain.NewAttribute(person.ID, domain.FactPersonOccupation, "Blacksmith")
	createEvent := domain.NewAttributeCreatedFromModel(attribute)
	if err := projector.Project(ctx, createEvent, 1); err != nil {
		t.Fatalf("Project create failed: %v", err)
	}

	// Verify attribute exists
	rm, _ := readStore.GetAttribute(ctx, attribute.ID)
	if rm == nil {
		t.Fatal("Attribute should exist before deletion")
	}

	// Delete attribute
	deleteEvent := domain.NewAttributeDeleted(attribute.ID, "test deletion")
	err := projector.Project(ctx, deleteEvent, 2)
	if err != nil {
		t.Fatalf("Project delete failed: %v", err)
	}

	// Verify deletion
	rm, _ = readStore.GetAttribute(ctx, attribute.ID)
	if rm != nil {
		t.Error("Attribute should be deleted")
	}
}

func TestProjector_AttributesList(t *testing.T) {
	readStore := memory.NewReadModelStore()
	projector := repository.NewProjector(readStore)
	ctx := context.Background()

	// Create a person
	person := domain.NewPerson("John", "Doe")
	projector.Project(ctx, domain.NewPersonCreated(person), 1)

	// Create multiple attributes for this person
	attr1 := domain.NewAttribute(person.ID, domain.FactPersonOccupation, "Blacksmith")
	attr2 := domain.NewAttribute(person.ID, domain.FactPersonOccupation, "Farmer")

	projector.Project(ctx, domain.NewAttributeCreatedFromModel(attr1), 1)
	projector.Project(ctx, domain.NewAttributeCreatedFromModel(attr2), 2)

	// List attributes for person
	attrs, err := readStore.ListAttributesForPerson(ctx, person.ID)
	if err != nil {
		t.Fatalf("ListAttributesForPerson failed: %v", err)
	}
	if len(attrs) != 2 {
		t.Errorf("Expected 2 attributes, got %d", len(attrs))
	}
}

// Edge cases and error paths

func TestProjector_PersonUpdated_NonExistent(t *testing.T) {
	readStore := memory.NewReadModelStore()
	projector := repository.NewProjector(readStore)
	ctx := context.Background()

	// Try to update a non-existent person (should not error, just skip)
	updateEvent := domain.NewPersonUpdated(uuid.New(), map[string]any{"given_name": "Test"})
	err := projector.Project(ctx, updateEvent, 1)
	if err != nil {
		t.Fatalf("Project update should not fail for non-existent person: %v", err)
	}
}

func TestProjector_PersonUpdated_DateClearing(t *testing.T) {
	readStore := memory.NewReadModelStore()
	projector := repository.NewProjector(readStore)
	ctx := context.Background()

	// Create person with dates
	person := domain.NewPerson("John", "Doe")
	person.SetBirthDate("1 JAN 1850")
	person.SetDeathDate("15 DEC 1920")
	createEvent := domain.NewPersonCreated(person)
	if err := projector.Project(ctx, createEvent, 1); err != nil {
		t.Fatalf("Project create failed: %v", err)
	}

	// Verify dates are set
	rm, _ := readStore.GetPerson(ctx, person.ID)
	if rm.BirthDateSort == nil {
		t.Fatal("BirthDateSort should be set initially")
	}
	if rm.DeathDateSort == nil {
		t.Fatal("DeathDateSort should be set initially")
	}

	// Update with invalid dates (should clear sort fields)
	changes := map[string]any{
		"birth_date": "UNKNOWN",
		"death_date": "ABOUT 1920",
	}
	updateEvent := domain.NewPersonUpdated(person.ID, changes)
	err := projector.Project(ctx, updateEvent, 2)
	if err != nil {
		t.Fatalf("Project update failed: %v", err)
	}

	// Verify raw dates updated but sort may be nil for unparseable dates
	rm, _ = readStore.GetPerson(ctx, person.ID)
	if rm.BirthDateRaw != "UNKNOWN" {
		t.Errorf("BirthDateRaw = %s, want UNKNOWN", rm.BirthDateRaw)
	}
	// "UNKNOWN" should result in nil DateSort
	if rm.BirthDateSort != nil {
		t.Error("BirthDateSort should be nil for unparseable date 'UNKNOWN'")
	}
	if rm.DeathDateRaw != "ABOUT 1920" {
		t.Errorf("DeathDateRaw = %s, want 'ABOUT 1920'", rm.DeathDateRaw)
	}
}

func TestProjector_FamilyUpdated_DateClearing(t *testing.T) {
	readStore := memory.NewReadModelStore()
	projector := repository.NewProjector(readStore)
	ctx := context.Background()

	// Create family with marriage date
	family := domain.NewFamily()
	family.SetMarriageDate("1 JAN 1870")
	createEvent := domain.NewFamilyCreated(family)
	if err := projector.Project(ctx, createEvent, 1); err != nil {
		t.Fatalf("Project create failed: %v", err)
	}

	// Verify date is set
	rm, _ := readStore.GetFamily(ctx, family.ID)
	if rm.MarriageDateSort == nil {
		t.Fatal("MarriageDateSort should be set initially")
	}

	// Update with invalid date
	changes := map[string]any{
		"marriage_date": "UNKNOWN",
	}
	updateEvent := domain.NewFamilyUpdated(family.ID, changes)
	err := projector.Project(ctx, updateEvent, 2)
	if err != nil {
		t.Fatalf("Project update failed: %v", err)
	}

	// Verify raw date updated but sort is nil for unparseable dates
	rm, _ = readStore.GetFamily(ctx, family.ID)
	if rm.MarriageDateRaw != "UNKNOWN" {
		t.Errorf("MarriageDateRaw = %s, want UNKNOWN", rm.MarriageDateRaw)
	}
	if rm.MarriageDateSort != nil {
		t.Error("MarriageDateSort should be nil for unparseable date 'UNKNOWN'")
	}
}

func TestProjector_SourceUpdated_DateClearing(t *testing.T) {
	readStore := memory.NewReadModelStore()
	projector := repository.NewProjector(readStore)
	ctx := context.Background()

	// Create source with publish date
	source := domain.NewSource("Test Source", domain.SourceBook)
	gd := domain.ParseGenDate("1995")
	source.PublishDate = &gd
	createEvent := domain.NewSourceCreated(source)
	if err := projector.Project(ctx, createEvent, 1); err != nil {
		t.Fatalf("Project create failed: %v", err)
	}

	// Verify date is set
	rm, _ := readStore.GetSource(ctx, source.ID)
	if rm.PublishDateSort == nil {
		t.Fatal("PublishDateSort should be set initially")
	}

	// Update with invalid date
	changes := map[string]any{
		"publish_date": "UNKNOWN",
	}
	updateEvent := domain.NewSourceUpdated(source.ID, changes)
	err := projector.Project(ctx, updateEvent, 2)
	if err != nil {
		t.Fatalf("Project update failed: %v", err)
	}

	// Verify raw date updated but sort is nil for unparseable dates
	rm, _ = readStore.GetSource(ctx, source.ID)
	if rm.PublishDateRaw != "UNKNOWN" {
		t.Errorf("PublishDateRaw = %s, want UNKNOWN", rm.PublishDateRaw)
	}
	if rm.PublishDateSort != nil {
		t.Error("PublishDateSort should be nil for unparseable date 'UNKNOWN'")
	}
}

func TestProjector_CitationUpdated_SourceChange(t *testing.T) {
	readStore := memory.NewReadModelStore()
	projector := repository.NewProjector(readStore)
	ctx := context.Background()

	// Create two sources
	source1 := domain.NewSource("Source 1", domain.SourceBook)
	source2 := domain.NewSource("Source 2", domain.SourceBook)
	projector.Project(ctx, domain.NewSourceCreated(source1), 1)
	projector.Project(ctx, domain.NewSourceCreated(source2), 1)

	// Create citation on source1
	citation := domain.NewCitation(source1.ID, domain.FactPersonBirth, uuid.New())
	createEvent := domain.NewCitationCreated(citation)
	if err := projector.Project(ctx, createEvent, 1); err != nil {
		t.Fatalf("Project create failed: %v", err)
	}

	// Verify source1 has citation count of 1
	s1, _ := readStore.GetSource(ctx, source1.ID)
	if s1.CitationCount != 1 {
		t.Errorf("Source1 CitationCount = %d, want 1", s1.CitationCount)
	}

	// Move citation to source2
	changes := map[string]any{
		"source_id": source2.ID.String(),
	}
	updateEvent := domain.NewCitationUpdated(citation.ID, changes)
	err := projector.Project(ctx, updateEvent, 2)
	if err != nil {
		t.Fatalf("Project update failed: %v", err)
	}

	// Verify source1 count decremented and source2 incremented
	s1, _ = readStore.GetSource(ctx, source1.ID)
	s2, _ := readStore.GetSource(ctx, source2.ID)
	if s1.CitationCount != 0 {
		t.Errorf("Source1 CitationCount after move = %d, want 0", s1.CitationCount)
	}
	if s2.CitationCount != 1 {
		t.Errorf("Source2 CitationCount after move = %d, want 1", s2.CitationCount)
	}

	// Verify citation has new source title
	c, _ := readStore.GetCitation(ctx, citation.ID)
	if c.SourceID != source2.ID {
		t.Errorf("Citation SourceID = %v, want %v", c.SourceID, source2.ID)
	}
	if c.SourceTitle != "Source 2" {
		t.Errorf("Citation SourceTitle = %s, want 'Source 2'", c.SourceTitle)
	}
}

func TestProjector_CitationUpdated_FactOwnerChange(t *testing.T) {
	readStore := memory.NewReadModelStore()
	projector := repository.NewProjector(readStore)
	ctx := context.Background()

	// Create source and citation
	source := domain.NewSource("Test Source", domain.SourceBook)
	projector.Project(ctx, domain.NewSourceCreated(source), 1)

	originalOwner := uuid.New()
	newOwner := uuid.New()

	citation := domain.NewCitation(source.ID, domain.FactPersonBirth, originalOwner)
	createEvent := domain.NewCitationCreated(citation)
	if err := projector.Project(ctx, createEvent, 1); err != nil {
		t.Fatalf("Project create failed: %v", err)
	}

	// Update fact owner and fact type
	changes := map[string]any{
		"fact_type":     "person_death",
		"fact_owner_id": newOwner.String(),
	}
	updateEvent := domain.NewCitationUpdated(citation.ID, changes)
	err := projector.Project(ctx, updateEvent, 2)
	if err != nil {
		t.Fatalf("Project update failed: %v", err)
	}

	// Verify updates
	c, _ := readStore.GetCitation(ctx, citation.ID)
	if c.FactType != domain.FactPersonDeath {
		t.Errorf("FactType = %s, want person_death", c.FactType)
	}
	if c.FactOwnerID != newOwner {
		t.Errorf("FactOwnerID = %v, want %v", c.FactOwnerID, newOwner)
	}
}

func TestProjector_ChildLinked_WithSingleParent(t *testing.T) {
	readStore := memory.NewReadModelStore()
	projector := repository.NewProjector(readStore)
	ctx := context.Background()

	// Create family with only one parent
	mother := domain.NewPerson("Jane", "Doe")
	mother.Gender = domain.GenderFemale
	child := domain.NewPerson("Jimmy", "Doe")

	projector.Project(ctx, domain.NewPersonCreated(mother), 1)
	projector.Project(ctx, domain.NewPersonCreated(child), 1)

	family := domain.NewFamilyWithPartners(nil, &mother.ID)
	projector.Project(ctx, domain.NewFamilyCreated(family), 1)

	// Link child
	fc := domain.NewFamilyChild(family.ID, child.ID, domain.ChildBiological)
	event := domain.NewChildLinkedToFamily(fc)

	err := projector.Project(ctx, event, 2)
	if err != nil {
		t.Fatalf("Project child link failed: %v", err)
	}

	// Verify pedigree edge has only mother set
	edge, _ := readStore.GetPedigreeEdge(ctx, child.ID)
	if edge == nil {
		t.Fatal("Pedigree edge not created")
	}
	if edge.FatherID != nil {
		t.Error("FatherID should be nil when only mother is in family")
	}
	if edge.MotherID == nil || *edge.MotherID != mother.ID {
		t.Error("MotherID not set correctly")
	}
	if edge.MotherName != "Jane Doe" {
		t.Errorf("MotherName = %s, want 'Jane Doe'", edge.MotherName)
	}
}

func TestProjector_ChildUnlinked_NonExistentFamily(t *testing.T) {
	readStore := memory.NewReadModelStore()
	projector := repository.NewProjector(readStore)
	ctx := context.Background()

	// Try to unlink a child from a non-existent family
	event := domain.NewChildUnlinkedFromFamily(uuid.New(), uuid.New())
	err := projector.Project(ctx, event, 1)
	// Should not error, just skip (family doesn't exist in read model)
	if err != nil {
		t.Fatalf("Project child unlink should not fail for non-existent family: %v", err)
	}
}

func TestProjector_FamilyDeleted_NoChildren(t *testing.T) {
	readStore := memory.NewReadModelStore()
	projector := repository.NewProjector(readStore)
	ctx := context.Background()

	// Create a family without children
	father := domain.NewPerson("John", "Doe")
	father.Gender = domain.GenderMale
	projector.Project(ctx, domain.NewPersonCreated(father), 1)

	family := domain.NewFamilyWithPartners(&father.ID, nil)
	projector.Project(ctx, domain.NewFamilyCreated(family), 1)

	// Verify no children
	children, _ := readStore.GetFamilyChildren(ctx, family.ID)
	if len(children) != 0 {
		t.Errorf("Expected 0 children, got %d", len(children))
	}

	// Delete family without children
	deleteEvent := domain.NewFamilyDeleted(family.ID, "test deletion")
	err := projector.Project(ctx, deleteEvent, 2)
	if err != nil {
		t.Fatalf("Project delete failed: %v", err)
	}

	// Verify family is deleted
	rm, _ := readStore.GetFamily(ctx, family.ID)
	if rm != nil {
		t.Error("Family should be deleted")
	}
}

func TestProjector_ChildLinked_NoFamily(t *testing.T) {
	readStore := memory.NewReadModelStore()
	projector := repository.NewProjector(readStore)
	ctx := context.Background()

	// Create a child but no family
	child := domain.NewPerson("Jimmy", "Doe")
	projector.Project(ctx, domain.NewPersonCreated(child), 1)

	// Link child to non-existent family
	// (This tests the path where family is nil)
	fc := domain.NewFamilyChild(uuid.New(), child.ID, domain.ChildBiological)
	event := domain.NewChildLinkedToFamily(fc)

	err := projector.Project(ctx, event, 1)
	if err != nil {
		t.Fatalf("Project child link failed: %v", err)
	}

	// Should succeed but not create pedigree edge (no family)
	edge, _ := readStore.GetPedigreeEdge(ctx, child.ID)
	if edge != nil {
		t.Error("Pedigree edge should not be created when family doesn't exist")
	}
}

func TestProjector_ChildLinked_WithTwoMaleParents(t *testing.T) {
	readStore := memory.NewReadModelStore()
	projector := repository.NewProjector(readStore)
	ctx := context.Background()

	// Create two male parents (covers the edge case)
	father1 := domain.NewPerson("John", "Doe")
	father1.Gender = domain.GenderMale
	father2 := domain.NewPerson("Bob", "Smith")
	father2.Gender = domain.GenderMale
	child := domain.NewPerson("Jimmy", "Doe")

	projector.Project(ctx, domain.NewPersonCreated(father1), 1)
	projector.Project(ctx, domain.NewPersonCreated(father2), 1)
	projector.Project(ctx, domain.NewPersonCreated(child), 1)

	family := domain.NewFamilyWithPartners(&father1.ID, &father2.ID)
	projector.Project(ctx, domain.NewFamilyCreated(family), 1)

	// Link child
	fc := domain.NewFamilyChild(family.ID, child.ID, domain.ChildBiological)
	event := domain.NewChildLinkedToFamily(fc)

	err := projector.Project(ctx, event, 2)
	if err != nil {
		t.Fatalf("Project child link failed: %v", err)
	}

	// Verify pedigree edge - second male should overwrite first as father
	edge, _ := readStore.GetPedigreeEdge(ctx, child.ID)
	if edge == nil {
		t.Fatal("Pedigree edge not created")
	}
	// Both are male, so father2 should be the father (last one wins)
	if edge.FatherID == nil || *edge.FatherID != father2.ID {
		t.Error("FatherID should be father2")
	}
	if edge.MotherID != nil {
		t.Error("MotherID should be nil (no female parent)")
	}
}

func TestProjector_ChildLinked_WithTwoFemaleParents(t *testing.T) {
	readStore := memory.NewReadModelStore()
	projector := repository.NewProjector(readStore)
	ctx := context.Background()

	// Create two female parents
	mother1 := domain.NewPerson("Jane", "Doe")
	mother1.Gender = domain.GenderFemale
	mother2 := domain.NewPerson("Mary", "Smith")
	mother2.Gender = domain.GenderFemale
	child := domain.NewPerson("Jimmy", "Doe")

	projector.Project(ctx, domain.NewPersonCreated(mother1), 1)
	projector.Project(ctx, domain.NewPersonCreated(mother2), 1)
	projector.Project(ctx, domain.NewPersonCreated(child), 1)

	family := domain.NewFamilyWithPartners(&mother1.ID, &mother2.ID)
	projector.Project(ctx, domain.NewFamilyCreated(family), 1)

	// Link child
	fc := domain.NewFamilyChild(family.ID, child.ID, domain.ChildBiological)
	event := domain.NewChildLinkedToFamily(fc)

	err := projector.Project(ctx, event, 2)
	if err != nil {
		t.Fatalf("Project child link failed: %v", err)
	}

	// Verify pedigree edge - second female should overwrite first as mother
	edge, _ := readStore.GetPedigreeEdge(ctx, child.ID)
	if edge == nil {
		t.Fatal("Pedigree edge not created")
	}
	if edge.FatherID != nil {
		t.Error("FatherID should be nil (no male parent)")
	}
	// Both are female, so mother2 should be the mother (last one wins)
	if edge.MotherID == nil || *edge.MotherID != mother2.ID {
		t.Error("MotherID should be mother2")
	}
}

func TestProjector_CitationCreated_NoSource(t *testing.T) {
	readStore := memory.NewReadModelStore()
	projector := repository.NewProjector(readStore)
	ctx := context.Background()

	// Create citation without creating source first
	citation := domain.NewCitation(uuid.New(), domain.FactPersonBirth, uuid.New())
	event := domain.NewCitationCreated(citation)

	err := projector.Project(ctx, event, 1)
	if err != nil {
		t.Fatalf("Project CitationCreated should succeed even without source: %v", err)
	}

	// Verify citation was created (without source title)
	rm, _ := readStore.GetCitation(ctx, citation.ID)
	if rm == nil {
		t.Fatal("Citation not found")
	}
	if rm.SourceTitle != "" {
		t.Errorf("SourceTitle = %s, want empty string", rm.SourceTitle)
	}
}

func TestProjector_CitationDeleted_NoSource(t *testing.T) {
	readStore := memory.NewReadModelStore()
	projector := repository.NewProjector(readStore)
	ctx := context.Background()

	// Create citation without source
	sourceID := uuid.New()
	citation := domain.NewCitation(sourceID, domain.FactPersonBirth, uuid.New())
	createEvent := domain.NewCitationCreated(citation)
	projector.Project(ctx, createEvent, 1)

	// Delete citation (source doesn't exist, so citation count update should be skipped)
	deleteEvent := domain.NewCitationDeleted(citation.ID, "test deletion")
	err := projector.Project(ctx, deleteEvent, 2)
	if err != nil {
		t.Fatalf("Project delete should succeed even without source: %v", err)
	}

	// Verify citation is deleted
	rm, _ := readStore.GetCitation(ctx, citation.ID)
	if rm != nil {
		t.Error("Citation should be deleted")
	}
}

func TestProjector_CitationDeleted_NonExistent(t *testing.T) {
	readStore := memory.NewReadModelStore()
	projector := repository.NewProjector(readStore)
	ctx := context.Background()

	// Try to delete non-existent citation
	deleteEvent := domain.NewCitationDeleted(uuid.New(), "test deletion")
	err := projector.Project(ctx, deleteEvent, 1)
	if err != nil {
		t.Fatalf("Project delete should succeed for non-existent citation: %v", err)
	}
}

func TestProjector_FamilyCreated_NoPartners(t *testing.T) {
	readStore := memory.NewReadModelStore()
	projector := repository.NewProjector(readStore)
	ctx := context.Background()

	// Create a family without partners
	family := domain.NewFamily()
	event := domain.NewFamilyCreated(family)

	err := projector.Project(ctx, event, 1)
	if err != nil {
		t.Fatalf("Project FamilyCreated failed: %v", err)
	}

	// Verify family was created
	rm, _ := readStore.GetFamily(ctx, family.ID)
	if rm == nil {
		t.Fatal("Family not found")
	}
	if rm.Partner1Name != "" {
		t.Errorf("Partner1Name = %s, want empty string", rm.Partner1Name)
	}
	if rm.Partner2Name != "" {
		t.Errorf("Partner2Name = %s, want empty string", rm.Partner2Name)
	}
}

func TestProjector_ChildUnlinked_WithChildCount(t *testing.T) {
	readStore := memory.NewReadModelStore()
	projector := repository.NewProjector(readStore)
	ctx := context.Background()

	// Create family with a child
	father := domain.NewPerson("John", "Doe")
	father.Gender = domain.GenderMale
	child := domain.NewPerson("Jimmy", "Doe")

	projector.Project(ctx, domain.NewPersonCreated(father), 1)
	projector.Project(ctx, domain.NewPersonCreated(child), 1)

	family := domain.NewFamilyWithPartners(&father.ID, nil)
	projector.Project(ctx, domain.NewFamilyCreated(family), 1)

	// Link child
	fc := domain.NewFamilyChild(family.ID, child.ID, domain.ChildBiological)
	projector.Project(ctx, domain.NewChildLinkedToFamily(fc), 2)

	// Verify child count is 1
	rm, _ := readStore.GetFamily(ctx, family.ID)
	if rm.ChildCount != 1 {
		t.Errorf("ChildCount = %d, want 1", rm.ChildCount)
	}

	// Unlink child
	unlinkEvent := domain.NewChildUnlinkedFromFamily(family.ID, child.ID)
	err := projector.Project(ctx, unlinkEvent, 3)
	if err != nil {
		t.Fatalf("Project unlink failed: %v", err)
	}

	// Verify child count is 0
	rm, _ = readStore.GetFamily(ctx, family.ID)
	if rm.ChildCount != 0 {
		t.Errorf("ChildCount after unlink = %d, want 0", rm.ChildCount)
	}
}

// PersonMerged Projection Tests

func TestProjector_PersonMerged_Basic(t *testing.T) {
	readStore := memory.NewReadModelStore()
	projector := repository.NewProjector(readStore)
	ctx := context.Background()

	// Create survivor and merged person
	survivor := domain.NewPerson("John", "Doe")
	survivor.Gender = domain.GenderMale
	survivor.SetBirthDate("1 JAN 1850")

	merged := domain.NewPerson("Johnny", "Doe")
	merged.Gender = domain.GenderMale
	merged.SetBirthDate("ABT 1850")
	merged.BirthPlace = "Springfield, IL"

	projector.Project(ctx, domain.NewPersonCreated(survivor), 1)
	projector.Project(ctx, domain.NewPersonCreated(merged), 1)

	// Create PersonMerged event with resolved fields from merged
	event := domain.NewPersonMerged(
		survivor.ID,
		merged.ID,
		map[string]any{"full_name": "Johnny Doe"},        // merged snapshot
		map[string]any{"birth_place": "Springfield, IL"}, // resolved fields
		[]uuid.UUID{}, // affected families
		[]uuid.UUID{}, // affected citations
		[]uuid.UUID{}, // transferred names
		[]uuid.UUID{}, // transferred events
		[]uuid.UUID{}, // transferred media
	)

	err := projector.Project(ctx, event, 2)
	if err != nil {
		t.Fatalf("Project PersonMerged failed: %v", err)
	}

	// Verify survivor was updated
	survivorRM, _ := readStore.GetPerson(ctx, survivor.ID)
	if survivorRM == nil {
		t.Fatal("Survivor should exist after merge")
	}
	if survivorRM.BirthPlace != "Springfield, IL" {
		t.Errorf("BirthPlace = %s, want 'Springfield, IL'", survivorRM.BirthPlace)
	}
	if survivorRM.Version != 2 {
		t.Errorf("Version = %d, want 2", survivorRM.Version)
	}

	// Verify merged person was deleted
	mergedRM, _ := readStore.GetPerson(ctx, merged.ID)
	if mergedRM != nil {
		t.Error("Merged person should be deleted")
	}
}

func TestProjector_PersonMerged_WithResolvedFields(t *testing.T) {
	readStore := memory.NewReadModelStore()
	projector := repository.NewProjector(readStore)
	ctx := context.Background()

	// Create survivor with some fields
	survivor := domain.NewPerson("John", "Doe")
	survivor.Gender = domain.GenderMale

	// Create merged person with more fields
	merged := domain.NewPerson("Johnny", "Smith")
	merged.Gender = domain.GenderMale
	merged.SetBirthDate("1 JAN 1850")
	merged.BirthPlace = "Boston, MA"
	merged.SetDeathDate("15 DEC 1920")
	merged.DeathPlace = "New York, NY"
	merged.Notes = "Important notes"

	projector.Project(ctx, domain.NewPersonCreated(survivor), 1)
	projector.Project(ctx, domain.NewPersonCreated(merged), 1)

	// Resolve multiple fields from merged
	resolvedFields := map[string]any{
		"given_name":      "Johnny",
		"surname":         "Smith",
		"birth_date":      "1 JAN 1850",
		"birth_place":     "Boston, MA",
		"death_date":      "15 DEC 1920",
		"death_place":     "New York, NY",
		"notes":           "Important notes",
		"research_status": "verified",
	}

	event := domain.NewPersonMerged(
		survivor.ID,
		merged.ID,
		map[string]any{},
		resolvedFields,
		[]uuid.UUID{},
		[]uuid.UUID{},
		[]uuid.UUID{},
		[]uuid.UUID{},
		[]uuid.UUID{},
	)

	err := projector.Project(ctx, event, 2)
	if err != nil {
		t.Fatalf("Project PersonMerged failed: %v", err)
	}

	// Verify all fields were updated
	survivorRM, _ := readStore.GetPerson(ctx, survivor.ID)
	if survivorRM.GivenName != "Johnny" {
		t.Errorf("GivenName = %s, want Johnny", survivorRM.GivenName)
	}
	if survivorRM.Surname != "Smith" {
		t.Errorf("Surname = %s, want Smith", survivorRM.Surname)
	}
	if survivorRM.FullName != "Johnny Smith" {
		t.Errorf("FullName = %s, want 'Johnny Smith'", survivorRM.FullName)
	}
	if survivorRM.BirthDateRaw != "1 JAN 1850" {
		t.Errorf("BirthDateRaw = %s, want '1 JAN 1850'", survivorRM.BirthDateRaw)
	}
	if survivorRM.BirthDateSort == nil {
		t.Error("BirthDateSort should not be nil")
	}
	if survivorRM.BirthPlace != "Boston, MA" {
		t.Errorf("BirthPlace = %s, want 'Boston, MA'", survivorRM.BirthPlace)
	}
	if survivorRM.DeathDateRaw != "15 DEC 1920" {
		t.Errorf("DeathDateRaw = %s, want '15 DEC 1920'", survivorRM.DeathDateRaw)
	}
	if survivorRM.DeathDateSort == nil {
		t.Error("DeathDateSort should not be nil")
	}
	if survivorRM.DeathPlace != "New York, NY" {
		t.Errorf("DeathPlace = %s, want 'New York, NY'", survivorRM.DeathPlace)
	}
	if survivorRM.Notes != "Important notes" {
		t.Errorf("Notes = %s, want 'Important notes'", survivorRM.Notes)
	}
	if survivorRM.ResearchStatus != domain.ParseResearchStatus("verified") {
		t.Errorf("ResearchStatus = %s, want verified", survivorRM.ResearchStatus)
	}
}

func TestProjector_PersonMerged_FamilyPartnerUpdate(t *testing.T) {
	readStore := memory.NewReadModelStore()
	projector := repository.NewProjector(readStore)
	ctx := context.Background()

	// Create survivor, merged, and spouse
	survivor := domain.NewPerson("John", "Doe")
	survivor.Gender = domain.GenderMale
	merged := domain.NewPerson("Johnny", "Doe")
	merged.Gender = domain.GenderMale
	spouse := domain.NewPerson("Jane", "Doe")
	spouse.Gender = domain.GenderFemale

	projector.Project(ctx, domain.NewPersonCreated(survivor), 1)
	projector.Project(ctx, domain.NewPersonCreated(merged), 1)
	projector.Project(ctx, domain.NewPersonCreated(spouse), 1)

	// Create family where merged person is partner1
	family := domain.NewFamilyWithPartners(&merged.ID, &spouse.ID)
	family.RelationshipType = domain.RelationMarriage
	projector.Project(ctx, domain.NewFamilyCreated(family), 1)

	// Verify initial family state
	familyRM, _ := readStore.GetFamily(ctx, family.ID)
	if familyRM.Partner1Name != "Johnny Doe" {
		t.Errorf("Initial Partner1Name = %s, want 'Johnny Doe'", familyRM.Partner1Name)
	}

	// Merge merged into survivor
	event := domain.NewPersonMerged(
		survivor.ID,
		merged.ID,
		map[string]any{},
		map[string]any{},
		[]uuid.UUID{family.ID},
		[]uuid.UUID{},
		[]uuid.UUID{},
		[]uuid.UUID{},
		[]uuid.UUID{},
	)

	err := projector.Project(ctx, event, 2)
	if err != nil {
		t.Fatalf("Project PersonMerged failed: %v", err)
	}

	// Verify family was updated with survivor as partner
	familyRM, _ = readStore.GetFamily(ctx, family.ID)
	if familyRM.Partner1ID == nil || *familyRM.Partner1ID != survivor.ID {
		t.Error("Partner1ID should be updated to survivor ID")
	}
	if familyRM.Partner1Name != "John Doe" {
		t.Errorf("Partner1Name = %s, want 'John Doe'", familyRM.Partner1Name)
	}
}

func TestProjector_PersonMerged_FamilyPartner2Update(t *testing.T) {
	readStore := memory.NewReadModelStore()
	projector := repository.NewProjector(readStore)
	ctx := context.Background()

	// Create survivor, merged, and spouse
	survivor := domain.NewPerson("Jane", "Doe")
	survivor.Gender = domain.GenderFemale
	merged := domain.NewPerson("Janet", "Doe")
	merged.Gender = domain.GenderFemale
	spouse := domain.NewPerson("John", "Doe")
	spouse.Gender = domain.GenderMale

	projector.Project(ctx, domain.NewPersonCreated(survivor), 1)
	projector.Project(ctx, domain.NewPersonCreated(merged), 1)
	projector.Project(ctx, domain.NewPersonCreated(spouse), 1)

	// Create family where merged person is partner2
	family := domain.NewFamilyWithPartners(&spouse.ID, &merged.ID)
	family.RelationshipType = domain.RelationMarriage
	projector.Project(ctx, domain.NewFamilyCreated(family), 1)

	// Merge merged into survivor
	event := domain.NewPersonMerged(
		survivor.ID,
		merged.ID,
		map[string]any{},
		map[string]any{},
		[]uuid.UUID{family.ID},
		[]uuid.UUID{},
		[]uuid.UUID{},
		[]uuid.UUID{},
		[]uuid.UUID{},
	)

	err := projector.Project(ctx, event, 2)
	if err != nil {
		t.Fatalf("Project PersonMerged failed: %v", err)
	}

	// Verify family was updated with survivor as partner2
	familyRM, _ := readStore.GetFamily(ctx, family.ID)
	if familyRM.Partner2ID == nil || *familyRM.Partner2ID != survivor.ID {
		t.Error("Partner2ID should be updated to survivor ID")
	}
	if familyRM.Partner2Name != "Jane Doe" {
		t.Errorf("Partner2Name = %s, want 'Jane Doe'", familyRM.Partner2Name)
	}
}

func TestProjector_PersonMerged_CitationTransfer(t *testing.T) {
	readStore := memory.NewReadModelStore()
	projector := repository.NewProjector(readStore)
	ctx := context.Background()

	// Create persons
	survivor := domain.NewPerson("John", "Doe")
	merged := domain.NewPerson("Johnny", "Doe")

	projector.Project(ctx, domain.NewPersonCreated(survivor), 1)
	projector.Project(ctx, domain.NewPersonCreated(merged), 1)

	// Create source and citation for merged person
	source := domain.NewSource("Test Source", domain.SourceBook)
	projector.Project(ctx, domain.NewSourceCreated(source), 1)

	citation := domain.NewCitation(source.ID, domain.FactPersonBirth, merged.ID)
	citation.Page = "123"
	projector.Project(ctx, domain.NewCitationCreated(citation), 1)

	// Verify citation is for merged person
	citationRM, _ := readStore.GetCitation(ctx, citation.ID)
	if citationRM.FactOwnerID != merged.ID {
		t.Error("Citation should be for merged person initially")
	}

	// Merge
	event := domain.NewPersonMerged(
		survivor.ID,
		merged.ID,
		map[string]any{},
		map[string]any{},
		[]uuid.UUID{},
		[]uuid.UUID{citation.ID},
		[]uuid.UUID{},
		[]uuid.UUID{},
		[]uuid.UUID{},
	)

	err := projector.Project(ctx, event, 2)
	if err != nil {
		t.Fatalf("Project PersonMerged failed: %v", err)
	}

	// Verify citation was transferred to survivor
	citationRM, _ = readStore.GetCitation(ctx, citation.ID)
	if citationRM.FactOwnerID != survivor.ID {
		t.Errorf("Citation FactOwnerID = %v, want %v", citationRM.FactOwnerID, survivor.ID)
	}
}

func TestProjector_PersonMerged_NameTransfer(t *testing.T) {
	readStore := memory.NewReadModelStore()
	projector := repository.NewProjector(readStore)
	ctx := context.Background()

	// Create persons
	survivor := domain.NewPerson("John", "Doe")
	merged := domain.NewPerson("Johnny", "Doe")

	projector.Project(ctx, domain.NewPersonCreated(survivor), 1)
	projector.Project(ctx, domain.NewPersonCreated(merged), 1)

	// Add alternate name to merged person
	personName := domain.NewPersonName(merged.ID, "Jonathan", "Doe")
	personName.IsPrimary = true
	nameEvent := domain.NewNameAdded(personName)
	projector.Project(ctx, nameEvent, 2)

	// Verify name is for merged person
	names, _ := readStore.GetPersonNames(ctx, merged.ID)
	if len(names) != 1 {
		t.Fatalf("Expected 1 name for merged person, got %d", len(names))
	}
	if names[0].IsPrimary != true {
		t.Error("Name should be primary before merge")
	}

	// Merge
	event := domain.NewPersonMerged(
		survivor.ID,
		merged.ID,
		map[string]any{},
		map[string]any{},
		[]uuid.UUID{},
		[]uuid.UUID{},
		[]uuid.UUID{personName.ID},
		[]uuid.UUID{},
		[]uuid.UUID{},
	)

	err := projector.Project(ctx, event, 3)
	if err != nil {
		t.Fatalf("Project PersonMerged failed: %v", err)
	}

	// Verify name was transferred to survivor (and is no longer primary)
	survivorNames, _ := readStore.GetPersonNames(ctx, survivor.ID)
	if len(survivorNames) != 1 {
		t.Fatalf("Expected 1 name for survivor, got %d", len(survivorNames))
	}
	if survivorNames[0].IsPrimary != false {
		t.Error("Transferred name should not be primary")
	}
	if survivorNames[0].GivenName != "Jonathan" {
		t.Errorf("GivenName = %s, want Jonathan", survivorNames[0].GivenName)
	}
}

func TestProjector_PersonMerged_EventTransfer(t *testing.T) {
	readStore := memory.NewReadModelStore()
	projector := repository.NewProjector(readStore)
	ctx := context.Background()

	// Create persons
	survivor := domain.NewPerson("John", "Doe")
	merged := domain.NewPerson("Johnny", "Doe")

	projector.Project(ctx, domain.NewPersonCreated(survivor), 1)
	projector.Project(ctx, domain.NewPersonCreated(merged), 1)

	// Create life event for merged person
	lifeEvent := domain.NewLifeEvent(merged.ID, domain.FactPersonBirth)
	gd := domain.ParseGenDate("1 JAN 1850")
	lifeEvent.Date = &gd
	lifeEvent.Place = "Springfield, IL"
	projector.Project(ctx, domain.NewLifeEventCreatedFromModel(lifeEvent), 2)

	// Verify event is for merged person
	events, _ := readStore.ListEventsForPerson(ctx, merged.ID)
	if len(events) != 1 {
		t.Fatalf("Expected 1 event for merged person, got %d", len(events))
	}

	// Merge
	event := domain.NewPersonMerged(
		survivor.ID,
		merged.ID,
		map[string]any{},
		map[string]any{},
		[]uuid.UUID{},
		[]uuid.UUID{},
		[]uuid.UUID{},
		[]uuid.UUID{lifeEvent.ID},
		[]uuid.UUID{},
	)

	err := projector.Project(ctx, event, 3)
	if err != nil {
		t.Fatalf("Project PersonMerged failed: %v", err)
	}

	// Verify event was transferred to survivor
	survivorEvents, _ := readStore.ListEventsForPerson(ctx, survivor.ID)
	if len(survivorEvents) != 1 {
		t.Fatalf("Expected 1 event for survivor, got %d", len(survivorEvents))
	}
	if survivorEvents[0].OwnerID != survivor.ID {
		t.Errorf("Event OwnerID = %v, want %v", survivorEvents[0].OwnerID, survivor.ID)
	}
}

func TestProjector_PersonMerged_MediaTransfer(t *testing.T) {
	readStore := memory.NewReadModelStore()
	projector := repository.NewProjector(readStore)
	ctx := context.Background()

	// Create persons
	survivor := domain.NewPerson("John", "Doe")
	merged := domain.NewPerson("Johnny", "Doe")

	projector.Project(ctx, domain.NewPersonCreated(survivor), 1)
	projector.Project(ctx, domain.NewPersonCreated(merged), 1)

	// Create media for merged person
	media := domain.NewMedia("Photo", "person", merged.ID)
	media.MimeType = "image/jpeg"
	media.FileData = []byte("fake data")
	projector.Project(ctx, domain.NewMediaCreated(media), 2)

	// Verify media is for merged person
	mediaList, _, _ := readStore.ListMediaForEntity(ctx, "person", merged.ID, repository.ListOptions{Limit: 100})
	if len(mediaList) != 1 {
		t.Fatalf("Expected 1 media for merged person, got %d", len(mediaList))
	}

	// Merge
	event := domain.NewPersonMerged(
		survivor.ID,
		merged.ID,
		map[string]any{},
		map[string]any{},
		[]uuid.UUID{},
		[]uuid.UUID{},
		[]uuid.UUID{},
		[]uuid.UUID{},
		[]uuid.UUID{media.ID},
	)

	err := projector.Project(ctx, event, 3)
	if err != nil {
		t.Fatalf("Project PersonMerged failed: %v", err)
	}

	// Verify media was transferred to survivor
	survivorMedia, _, _ := readStore.ListMediaForEntity(ctx, "person", survivor.ID, repository.ListOptions{Limit: 100})
	if len(survivorMedia) != 1 {
		t.Fatalf("Expected 1 media for survivor, got %d", len(survivorMedia))
	}
	if survivorMedia[0].EntityID != survivor.ID {
		t.Errorf("Media EntityID = %v, want %v", survivorMedia[0].EntityID, survivor.ID)
	}
}

func TestProjector_PersonMerged_AttributeTransfer(t *testing.T) {
	readStore := memory.NewReadModelStore()
	projector := repository.NewProjector(readStore)
	ctx := context.Background()

	// Create persons
	survivor := domain.NewPerson("John", "Doe")
	merged := domain.NewPerson("Johnny", "Doe")

	projector.Project(ctx, domain.NewPersonCreated(survivor), 1)
	projector.Project(ctx, domain.NewPersonCreated(merged), 1)

	// Create attribute for merged person
	attr := domain.NewAttribute(merged.ID, domain.FactPersonOccupation, "Blacksmith")
	projector.Project(ctx, domain.NewAttributeCreatedFromModel(attr), 2)

	// Verify attribute is for merged person
	attrs, _ := readStore.ListAttributesForPerson(ctx, merged.ID)
	if len(attrs) != 1 {
		t.Fatalf("Expected 1 attribute for merged person, got %d", len(attrs))
	}

	// Merge
	event := domain.NewPersonMerged(
		survivor.ID,
		merged.ID,
		map[string]any{},
		map[string]any{},
		[]uuid.UUID{},
		[]uuid.UUID{},
		[]uuid.UUID{},
		[]uuid.UUID{},
		[]uuid.UUID{},
	)

	err := projector.Project(ctx, event, 3)
	if err != nil {
		t.Fatalf("Project PersonMerged failed: %v", err)
	}

	// Verify attribute was transferred to survivor
	survivorAttrs, _ := readStore.ListAttributesForPerson(ctx, survivor.ID)
	if len(survivorAttrs) != 1 {
		t.Fatalf("Expected 1 attribute for survivor, got %d", len(survivorAttrs))
	}
	if survivorAttrs[0].PersonID != survivor.ID {
		t.Errorf("Attribute PersonID = %v, want %v", survivorAttrs[0].PersonID, survivor.ID)
	}
	if survivorAttrs[0].Value != "Blacksmith" {
		t.Errorf("Attribute Value = %s, want 'Blacksmith'", survivorAttrs[0].Value)
	}
}

func TestProjector_PersonMerged_PedigreeEdgeTransfer(t *testing.T) {
	readStore := memory.NewReadModelStore()
	projector := repository.NewProjector(readStore)
	ctx := context.Background()

	// Create grandparents
	grandfather := domain.NewPerson("George", "Doe")
	grandfather.Gender = domain.GenderMale
	grandmother := domain.NewPerson("Martha", "Doe")
	grandmother.Gender = domain.GenderFemale

	projector.Project(ctx, domain.NewPersonCreated(grandfather), 1)
	projector.Project(ctx, domain.NewPersonCreated(grandmother), 1)

	// Create family for grandparents
	grandparentFamily := domain.NewFamilyWithPartners(&grandfather.ID, &grandmother.ID)
	projector.Project(ctx, domain.NewFamilyCreated(grandparentFamily), 1)

	// Create survivor (has no parents)
	survivor := domain.NewPerson("John", "Doe")
	survivor.Gender = domain.GenderMale
	projector.Project(ctx, domain.NewPersonCreated(survivor), 1)

	// Create merged person as child of grandparents
	merged := domain.NewPerson("Johnny", "Doe")
	merged.Gender = domain.GenderMale
	projector.Project(ctx, domain.NewPersonCreated(merged), 1)

	// Link merged person to grandparent family
	fc := domain.NewFamilyChild(grandparentFamily.ID, merged.ID, domain.ChildBiological)
	projector.Project(ctx, domain.NewChildLinkedToFamily(fc), 2)

	// Verify merged person has pedigree edge
	mergedEdge, _ := readStore.GetPedigreeEdge(ctx, merged.ID)
	if mergedEdge == nil {
		t.Fatal("Merged person should have pedigree edge before merge")
	}

	// Verify survivor has no pedigree edge
	survivorEdge, _ := readStore.GetPedigreeEdge(ctx, survivor.ID)
	if survivorEdge != nil {
		t.Fatal("Survivor should not have pedigree edge before merge")
	}

	// Merge
	event := domain.NewPersonMerged(
		survivor.ID,
		merged.ID,
		map[string]any{},
		map[string]any{},
		[]uuid.UUID{},
		[]uuid.UUID{},
		[]uuid.UUID{},
		[]uuid.UUID{},
		[]uuid.UUID{},
	)

	err := projector.Project(ctx, event, 3)
	if err != nil {
		t.Fatalf("Project PersonMerged failed: %v", err)
	}

	// Verify survivor now has pedigree edge with grandparents
	survivorEdge, _ = readStore.GetPedigreeEdge(ctx, survivor.ID)
	if survivorEdge == nil {
		t.Fatal("Survivor should have pedigree edge after merge")
	}
	if survivorEdge.FatherID == nil || *survivorEdge.FatherID != grandfather.ID {
		t.Error("Survivor's father should be grandfather")
	}
	if survivorEdge.MotherID == nil || *survivorEdge.MotherID != grandmother.ID {
		t.Error("Survivor's mother should be grandmother")
	}

	// Verify merged person's pedigree edge is removed
	mergedEdge, _ = readStore.GetPedigreeEdge(ctx, merged.ID)
	if mergedEdge != nil {
		t.Error("Merged person's pedigree edge should be removed")
	}
}

func TestProjector_PersonMerged_SurvivorNotFound(t *testing.T) {
	readStore := memory.NewReadModelStore()
	projector := repository.NewProjector(readStore)
	ctx := context.Background()

	// Create only merged person (survivor doesn't exist)
	merged := domain.NewPerson("Johnny", "Doe")
	projector.Project(ctx, domain.NewPersonCreated(merged), 1)

	// Try to merge (survivor doesn't exist)
	event := domain.NewPersonMerged(
		uuid.New(), // non-existent survivor
		merged.ID,
		map[string]any{},
		map[string]any{},
		[]uuid.UUID{},
		[]uuid.UUID{},
		[]uuid.UUID{},
		[]uuid.UUID{},
		[]uuid.UUID{},
	)

	err := projector.Project(ctx, event, 2)
	// Should not error, just skip
	if err != nil {
		t.Fatalf("Project should not fail for non-existent survivor: %v", err)
	}

	// Merged person should still exist (merge didn't proceed)
	mergedRM, _ := readStore.GetPerson(ctx, merged.ID)
	if mergedRM == nil {
		t.Error("Merged person should still exist when survivor is not found")
	}
}

func TestProjector_PersonMerged_GenderUpdate(t *testing.T) {
	readStore := memory.NewReadModelStore()
	projector := repository.NewProjector(readStore)
	ctx := context.Background()

	// Create survivor without gender
	survivor := domain.NewPerson("John", "Doe")
	merged := domain.NewPerson("Johnny", "Doe")
	merged.Gender = domain.GenderMale

	projector.Project(ctx, domain.NewPersonCreated(survivor), 1)
	projector.Project(ctx, domain.NewPersonCreated(merged), 1)

	// Merge with gender from merged
	event := domain.NewPersonMerged(
		survivor.ID,
		merged.ID,
		map[string]any{},
		map[string]any{"gender": "male"},
		[]uuid.UUID{},
		[]uuid.UUID{},
		[]uuid.UUID{},
		[]uuid.UUID{},
		[]uuid.UUID{},
	)

	err := projector.Project(ctx, event, 2)
	if err != nil {
		t.Fatalf("Project PersonMerged failed: %v", err)
	}

	// Verify gender was updated
	survivorRM, _ := readStore.GetPerson(ctx, survivor.ID)
	if survivorRM.Gender != domain.GenderMale {
		t.Errorf("Gender = %s, want male", survivorRM.Gender)
	}
}
