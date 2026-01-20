package query_test

import (
	"context"
	"testing"

	"github.com/google/uuid"

	"github.com/cacack/my-family/internal/command"
	"github.com/cacack/my-family/internal/domain"
	"github.com/cacack/my-family/internal/query"
	"github.com/cacack/my-family/internal/repository"
	"github.com/cacack/my-family/internal/repository/memory"
)

// setupAssociationTestData creates test persons and associations for query tests.
func setupAssociationTestData(t *testing.T, cmdHandler *command.Handler, ctx context.Context) (person1ID, person2ID, person3ID, assoc1ID, assoc2ID uuid.UUID) {
	// Create persons
	person1Result, err := cmdHandler.CreatePerson(ctx, command.CreatePersonInput{
		GivenName: "John",
		Surname:   "Smith",
		Gender:    "male",
	})
	if err != nil {
		t.Fatalf("Failed to create person 1: %v", err)
	}
	person1ID = person1Result.ID

	person2Result, err := cmdHandler.CreatePerson(ctx, command.CreatePersonInput{
		GivenName: "Mary",
		Surname:   "Jones",
		Gender:    "female",
	})
	if err != nil {
		t.Fatalf("Failed to create person 2: %v", err)
	}
	person2ID = person2Result.ID

	person3Result, err := cmdHandler.CreatePerson(ctx, command.CreatePersonInput{
		GivenName: "Bob",
		Surname:   "Brown",
		Gender:    "male",
	})
	if err != nil {
		t.Fatalf("Failed to create person 3: %v", err)
	}
	person3ID = person3Result.ID

	// Create associations
	assoc1Result, err := cmdHandler.CreateAssociation(ctx, command.CreateAssociationInput{
		PersonID:    person1ID,
		AssociateID: person2ID,
		Role:        domain.RoleGodparent,
		Phrase:      "John was Mary's godparent",
	})
	if err != nil {
		t.Fatalf("Failed to create association 1: %v", err)
	}
	assoc1ID = assoc1Result.ID

	assoc2Result, err := cmdHandler.CreateAssociation(ctx, command.CreateAssociationInput{
		PersonID:    person1ID,
		AssociateID: person3ID,
		Role:        domain.RoleWitness,
		Notes:       "Witnessed Bob's marriage",
	})
	if err != nil {
		t.Fatalf("Failed to create association 2: %v", err)
	}
	assoc2ID = assoc2Result.ID

	return
}

// TestGetAssociation tests getting an association by ID.
func TestGetAssociation(t *testing.T) {
	eventStore := memory.NewEventStore()
	readStore := memory.NewReadModelStore()
	cmdHandler := command.NewHandler(eventStore, readStore)
	queryService := query.NewAssociationService(readStore)
	ctx := context.Background()

	person1ID, person2ID, _, assoc1ID, _ := setupAssociationTestData(t, cmdHandler, ctx)

	// Get association
	assoc, err := queryService.GetAssociation(ctx, assoc1ID)
	if err != nil {
		t.Fatalf("GetAssociation failed: %v", err)
	}

	if assoc.ID != assoc1ID {
		t.Errorf("ID = %v, want %v", assoc.ID, assoc1ID)
	}
	if assoc.PersonID != person1ID {
		t.Errorf("PersonID = %v, want %v", assoc.PersonID, person1ID)
	}
	if assoc.AssociateID != person2ID {
		t.Errorf("AssociateID = %v, want %v", assoc.AssociateID, person2ID)
	}
	if assoc.Role != domain.RoleGodparent {
		t.Errorf("Role = %s, want %s", assoc.Role, domain.RoleGodparent)
	}
	if assoc.Phrase != "John was Mary's godparent" {
		t.Errorf("Phrase = %s, want 'John was Mary's godparent'", assoc.Phrase)
	}
	// Verify denormalized names
	if assoc.PersonName != "John Smith" {
		t.Errorf("PersonName = %s, want 'John Smith'", assoc.PersonName)
	}
	if assoc.AssociateName != "Mary Jones" {
		t.Errorf("AssociateName = %s, want 'Mary Jones'", assoc.AssociateName)
	}
}

// TestGetAssociation_NotFound tests getting a non-existent association.
func TestGetAssociation_NotFound(t *testing.T) {
	readStore := memory.NewReadModelStore()
	queryService := query.NewAssociationService(readStore)
	ctx := context.Background()

	assoc, err := queryService.GetAssociation(ctx, uuid.New())
	if err != nil {
		t.Fatalf("GetAssociation should not error, got: %v", err)
	}
	if assoc != nil {
		t.Error("GetAssociation should return nil for non-existent association")
	}
}

// TestListAssociations tests listing associations with pagination.
func TestListAssociations(t *testing.T) {
	eventStore := memory.NewEventStore()
	readStore := memory.NewReadModelStore()
	cmdHandler := command.NewHandler(eventStore, readStore)
	queryService := query.NewAssociationService(readStore)
	ctx := context.Background()

	setupAssociationTestData(t, cmdHandler, ctx)

	tests := []struct {
		name      string
		opts      repository.ListOptions
		wantCount int
		wantTotal int
	}{
		{
			name: "list all",
			opts: repository.ListOptions{
				Limit: 10,
			},
			wantCount: 2,
			wantTotal: 2,
		},
		{
			name: "with pagination limit 1",
			opts: repository.ListOptions{
				Limit:  1,
				Offset: 0,
			},
			wantCount: 1,
			wantTotal: 2,
		},
		{
			name: "second page",
			opts: repository.ListOptions{
				Limit:  1,
				Offset: 1,
			},
			wantCount: 1,
			wantTotal: 2,
		},
		{
			name: "beyond last page",
			opts: repository.ListOptions{
				Limit:  10,
				Offset: 10,
			},
			wantCount: 0,
			wantTotal: 2,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			associations, total, err := queryService.ListAssociations(ctx, tt.opts)
			if err != nil {
				t.Fatalf("ListAssociations failed: %v", err)
			}

			if len(associations) != tt.wantCount {
				t.Errorf("Got %d associations, want %d", len(associations), tt.wantCount)
			}

			if total != tt.wantTotal {
				t.Errorf("Total = %d, want %d", total, tt.wantTotal)
			}
		})
	}
}

// TestListAssociations_Empty tests listing when there are no associations.
func TestListAssociations_Empty(t *testing.T) {
	readStore := memory.NewReadModelStore()
	queryService := query.NewAssociationService(readStore)
	ctx := context.Background()

	associations, total, err := queryService.ListAssociations(ctx, repository.ListOptions{
		Limit: 10,
	})
	if err != nil {
		t.Fatalf("ListAssociations failed: %v", err)
	}

	if len(associations) != 0 {
		t.Errorf("Got %d associations, want 0", len(associations))
	}
	if total != 0 {
		t.Errorf("Total = %d, want 0", total)
	}
}

// TestListAssociationsForPerson tests listing associations for a specific person.
func TestListAssociationsForPerson(t *testing.T) {
	eventStore := memory.NewEventStore()
	readStore := memory.NewReadModelStore()
	cmdHandler := command.NewHandler(eventStore, readStore)
	queryService := query.NewAssociationService(readStore)
	ctx := context.Background()

	person1ID, person2ID, person3ID, _, _ := setupAssociationTestData(t, cmdHandler, ctx)

	tests := []struct {
		name      string
		personID  uuid.UUID
		wantCount int
	}{
		{
			name:      "person with 2 associations",
			personID:  person1ID,
			wantCount: 2, // John has 2 associations (with Mary and Bob)
		},
		{
			name:      "person2 as associate only",
			personID:  person2ID,
			wantCount: 1, // Mary is associate in 1 association
		},
		{
			name:      "person3 as associate only",
			personID:  person3ID,
			wantCount: 1, // Bob is associate in 1 association
		},
		{
			name:      "non-existent person",
			personID:  uuid.New(),
			wantCount: 0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			associations, err := queryService.ListAssociationsForPerson(ctx, tt.personID)
			if err != nil {
				t.Fatalf("ListAssociationsForPerson failed: %v", err)
			}

			if len(associations) != tt.wantCount {
				t.Errorf("Got %d associations, want %d", len(associations), tt.wantCount)
			}
		})
	}
}

// TestNewAssociationService tests creating a new AssociationService.
func TestNewAssociationService(t *testing.T) {
	readStore := memory.NewReadModelStore()
	service := query.NewAssociationService(readStore)

	if service == nil {
		t.Error("NewAssociationService should not return nil")
	}
}

// TestListAssociationsForPerson_BothDirections tests that associations are found
// regardless of whether the person is PersonID or AssociateID.
func TestListAssociationsForPerson_BothDirections(t *testing.T) {
	eventStore := memory.NewEventStore()
	readStore := memory.NewReadModelStore()
	cmdHandler := command.NewHandler(eventStore, readStore)
	queryService := query.NewAssociationService(readStore)
	ctx := context.Background()

	// Create persons
	person1Result, _ := cmdHandler.CreatePerson(ctx, command.CreatePersonInput{
		GivenName: "Alice",
		Surname:   "First",
		Gender:    "female",
	})
	person2Result, _ := cmdHandler.CreatePerson(ctx, command.CreatePersonInput{
		GivenName: "Bob",
		Surname:   "Second",
		Gender:    "male",
	})
	person3Result, _ := cmdHandler.CreatePerson(ctx, command.CreatePersonInput{
		GivenName: "Carol",
		Surname:   "Third",
		Gender:    "female",
	})

	// Create association: Alice -> Bob (Alice is godparent)
	_, _ = cmdHandler.CreateAssociation(ctx, command.CreateAssociationInput{
		PersonID:    person1Result.ID,
		AssociateID: person2Result.ID,
		Role:        domain.RoleGodparent,
	})

	// Create association: Carol -> Alice (Carol witnessed Alice's event)
	_, _ = cmdHandler.CreateAssociation(ctx, command.CreateAssociationInput{
		PersonID:    person3Result.ID,
		AssociateID: person1Result.ID,
		Role:        domain.RoleWitness,
	})

	// Alice should have 2 associations: one where she is PersonID, one where she is AssociateID
	associations, err := queryService.ListAssociationsForPerson(ctx, person1Result.ID)
	if err != nil {
		t.Fatalf("ListAssociationsForPerson failed: %v", err)
	}

	if len(associations) != 2 {
		t.Errorf("Expected Alice to have 2 associations (one as person, one as associate), got %d", len(associations))
	}

	// Bob should have 1 association (as associate)
	bobAssocs, _ := queryService.ListAssociationsForPerson(ctx, person2Result.ID)
	if len(bobAssocs) != 1 {
		t.Errorf("Expected Bob to have 1 association, got %d", len(bobAssocs))
	}

	// Carol should have 1 association (as person)
	carolAssocs, _ := queryService.ListAssociationsForPerson(ctx, person3Result.ID)
	if len(carolAssocs) != 1 {
		t.Errorf("Expected Carol to have 1 association, got %d", len(carolAssocs))
	}
}
