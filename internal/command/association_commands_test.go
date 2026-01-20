package command_test

import (
	"context"
	"testing"

	"github.com/google/uuid"

	"github.com/cacack/my-family/internal/command"
	"github.com/cacack/my-family/internal/domain"
	"github.com/cacack/my-family/internal/repository"
	"github.com/cacack/my-family/internal/repository/memory"
)

// TestCreateAssociation tests creating a new association.
func TestCreateAssociation(t *testing.T) {
	eventStore := memory.NewEventStore()
	readStore := memory.NewReadModelStore()
	handler := command.NewHandler(eventStore, readStore)
	ctx := context.Background()

	// Create two persons first (required for associations)
	person1Result, _ := handler.CreatePerson(ctx, command.CreatePersonInput{
		GivenName: "John",
		Surname:   "Smith",
		Gender:    "male",
	})
	person2Result, _ := handler.CreatePerson(ctx, command.CreatePersonInput{
		GivenName: "Mary",
		Surname:   "Jones",
		Gender:    "female",
	})

	tests := []struct {
		name    string
		input   command.CreateAssociationInput
		wantErr bool
	}{
		{
			name: "valid association with godparent role",
			input: command.CreateAssociationInput{
				PersonID:    person1Result.ID,
				AssociateID: person2Result.ID,
				Role:        domain.RoleGodparent,
			},
			wantErr: false,
		},
		{
			name: "valid association with witness role",
			input: command.CreateAssociationInput{
				PersonID:    person1Result.ID,
				AssociateID: person2Result.ID,
				Role:        domain.RoleWitness,
			},
			wantErr: false,
		},
		{
			name: "valid association with custom role",
			input: command.CreateAssociationInput{
				PersonID:    person1Result.ID,
				AssociateID: person2Result.ID,
				Role:        "mentor",
			},
			wantErr: false,
		},
		{
			name: "valid association with phrase",
			input: command.CreateAssociationInput{
				PersonID:    person1Result.ID,
				AssociateID: person2Result.ID,
				Role:        domain.RoleGodparent,
				Phrase:      "John was the godparent at baptism",
			},
			wantErr: false,
		},
		{
			name: "valid association with notes",
			input: command.CreateAssociationInput{
				PersonID:    person1Result.ID,
				AssociateID: person2Result.ID,
				Role:        domain.RoleWitness,
				Notes:       "Witnessed the marriage",
			},
			wantErr: false,
		},
		{
			name: "valid association with gedcom xref",
			input: command.CreateAssociationInput{
				PersonID:    person1Result.ID,
				AssociateID: person2Result.ID,
				Role:        domain.RoleGodparent,
				GedcomXref:  "@I2@",
			},
			wantErr: false,
		},
		{
			name: "invalid - person not found",
			input: command.CreateAssociationInput{
				PersonID:    uuid.New(),
				AssociateID: person2Result.ID,
				Role:        domain.RoleGodparent,
			},
			wantErr: true,
		},
		{
			name: "invalid - associate not found",
			input: command.CreateAssociationInput{
				PersonID:    person1Result.ID,
				AssociateID: uuid.New(),
				Role:        domain.RoleGodparent,
			},
			wantErr: true,
		},
		{
			name: "invalid - empty role",
			input: command.CreateAssociationInput{
				PersonID:    person1Result.ID,
				AssociateID: person2Result.ID,
				Role:        "",
			},
			wantErr: true,
		},
		{
			name: "invalid - self association",
			input: command.CreateAssociationInput{
				PersonID:    person1Result.ID,
				AssociateID: person1Result.ID,
				Role:        domain.RoleGodparent,
			},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := handler.CreateAssociation(ctx, tt.input)
			if (err != nil) != tt.wantErr {
				t.Errorf("CreateAssociation() error = %v, wantErr %v", err, tt.wantErr)
				return
			}

			if !tt.wantErr {
				if result.ID == uuid.Nil {
					t.Error("Expected non-nil ID")
				}
				if result.Version != 1 {
					t.Errorf("Version = %d, want 1", result.Version)
				}

				// Verify association in read model
				assoc, _ := readStore.GetAssociation(ctx, result.ID)
				if assoc == nil {
					t.Fatal("Association not found in read model")
				}
				if assoc.PersonID != tt.input.PersonID {
					t.Errorf("PersonID = %v, want %v", assoc.PersonID, tt.input.PersonID)
				}
				if assoc.AssociateID != tt.input.AssociateID {
					t.Errorf("AssociateID = %v, want %v", assoc.AssociateID, tt.input.AssociateID)
				}
				if assoc.Role != tt.input.Role {
					t.Errorf("Role = %s, want %s", assoc.Role, tt.input.Role)
				}
			}
		})
	}
}

// TestCreateAssociation_WithNoteIDs tests creating associations with linked notes.
func TestCreateAssociation_WithNoteIDs(t *testing.T) {
	eventStore := memory.NewEventStore()
	readStore := memory.NewReadModelStore()
	handler := command.NewHandler(eventStore, readStore)
	ctx := context.Background()

	// Create persons
	person1Result, _ := handler.CreatePerson(ctx, command.CreatePersonInput{
		GivenName: "John",
		Surname:   "Smith",
		Gender:    "male",
	})
	person2Result, _ := handler.CreatePerson(ctx, command.CreatePersonInput{
		GivenName: "Mary",
		Surname:   "Jones",
		Gender:    "female",
	})

	// Create notes
	note1Result, _ := handler.CreateNote(ctx, command.CreateNoteInput{
		Text: "First note about the association",
	})
	note2Result, _ := handler.CreateNote(ctx, command.CreateNoteInput{
		Text: "Second note about the association",
	})

	// Create association with note IDs
	result, err := handler.CreateAssociation(ctx, command.CreateAssociationInput{
		PersonID:    person1Result.ID,
		AssociateID: person2Result.ID,
		Role:        domain.RoleGodparent,
		NoteIDs:     []uuid.UUID{note1Result.ID, note2Result.ID},
	})
	if err != nil {
		t.Fatalf("CreateAssociation failed: %v", err)
	}

	// Verify note IDs in read model
	assoc, _ := readStore.GetAssociation(ctx, result.ID)
	if len(assoc.NoteIDs) != 2 {
		t.Errorf("Expected 2 note IDs, got %d", len(assoc.NoteIDs))
	}
}

// TestUpdateAssociation tests updating an association.
func TestUpdateAssociation(t *testing.T) {
	eventStore := memory.NewEventStore()
	readStore := memory.NewReadModelStore()
	handler := command.NewHandler(eventStore, readStore)
	ctx := context.Background()

	// Create persons and association
	person1Result, _ := handler.CreatePerson(ctx, command.CreatePersonInput{
		GivenName: "John",
		Surname:   "Smith",
		Gender:    "male",
	})
	person2Result, _ := handler.CreatePerson(ctx, command.CreatePersonInput{
		GivenName: "Mary",
		Surname:   "Jones",
		Gender:    "female",
	})

	createResult, _ := handler.CreateAssociation(ctx, command.CreateAssociationInput{
		PersonID:    person1Result.ID,
		AssociateID: person2Result.ID,
		Role:        domain.RoleGodparent,
	})

	tests := []struct {
		name    string
		input   command.UpdateAssociationInput
		wantErr bool
	}{
		{
			name: "update role",
			input: command.UpdateAssociationInput{
				ID:      createResult.ID,
				Role:    strPtr(domain.RoleWitness),
				Version: createResult.Version,
			},
			wantErr: false,
		},
		{
			name: "wrong version (optimistic locking)",
			input: command.UpdateAssociationInput{
				ID:      createResult.ID,
				Role:    strPtr("mentor"),
				Version: 999,
			},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := handler.UpdateAssociation(ctx, tt.input)
			if (err != nil) != tt.wantErr {
				t.Errorf("UpdateAssociation() error = %v, wantErr %v", err, tt.wantErr)
				return
			}

			if !tt.wantErr && result.Version <= tt.input.Version {
				t.Errorf("Version not incremented: got %d, want > %d", result.Version, tt.input.Version)
			}
		})
	}
}

// TestUpdateAssociation_AllFields tests updating all association fields.
func TestUpdateAssociation_AllFields(t *testing.T) {
	eventStore := memory.NewEventStore()
	readStore := memory.NewReadModelStore()
	handler := command.NewHandler(eventStore, readStore)
	ctx := context.Background()

	// Create persons and association
	person1Result, _ := handler.CreatePerson(ctx, command.CreatePersonInput{
		GivenName: "John",
		Surname:   "Smith",
		Gender:    "male",
	})
	person2Result, _ := handler.CreatePerson(ctx, command.CreatePersonInput{
		GivenName: "Mary",
		Surname:   "Jones",
		Gender:    "female",
	})

	createResult, _ := handler.CreateAssociation(ctx, command.CreateAssociationInput{
		PersonID:    person1Result.ID,
		AssociateID: person2Result.ID,
		Role:        domain.RoleGodparent,
	})

	// Update all fields
	newRole := "mentor"
	newPhrase := "John was Mary's mentor"
	newNotes := "Met at university"
	noteIDs := []uuid.UUID{}

	_, err := handler.UpdateAssociation(ctx, command.UpdateAssociationInput{
		ID:      createResult.ID,
		Role:    &newRole,
		Phrase:  &newPhrase,
		Notes:   &newNotes,
		NoteIDs: &noteIDs,
		Version: createResult.Version,
	})
	if err != nil {
		t.Fatalf("UpdateAssociation failed: %v", err)
	}

	// Verify changes in read model
	assoc, _ := readStore.GetAssociation(ctx, createResult.ID)
	if assoc.Role != newRole {
		t.Errorf("Role = %s, want %s", assoc.Role, newRole)
	}
	if assoc.Phrase != newPhrase {
		t.Errorf("Phrase = %s, want %s", assoc.Phrase, newPhrase)
	}
	if assoc.Notes != newNotes {
		t.Errorf("Notes = %s, want %s", assoc.Notes, newNotes)
	}
}

// TestUpdateAssociation_NoChanges tests that updating without changes returns current version.
func TestUpdateAssociation_NoChanges(t *testing.T) {
	eventStore := memory.NewEventStore()
	readStore := memory.NewReadModelStore()
	handler := command.NewHandler(eventStore, readStore)
	ctx := context.Background()

	// Create persons and association
	person1Result, _ := handler.CreatePerson(ctx, command.CreatePersonInput{
		GivenName: "John",
		Surname:   "Smith",
		Gender:    "male",
	})
	person2Result, _ := handler.CreatePerson(ctx, command.CreatePersonInput{
		GivenName: "Mary",
		Surname:   "Jones",
		Gender:    "female",
	})

	createResult, _ := handler.CreateAssociation(ctx, command.CreateAssociationInput{
		PersonID:    person1Result.ID,
		AssociateID: person2Result.ID,
		Role:        domain.RoleGodparent,
	})

	// Update with no changes
	result, err := handler.UpdateAssociation(ctx, command.UpdateAssociationInput{
		ID:      createResult.ID,
		Version: createResult.Version,
	})
	if err != nil {
		t.Fatalf("UpdateAssociation failed: %v", err)
	}

	if result.Version != createResult.Version {
		t.Errorf("Version changed without updates: got %d, want %d", result.Version, createResult.Version)
	}
}

// TestUpdateAssociation_NotFound tests updating a non-existent association.
func TestUpdateAssociation_NotFound(t *testing.T) {
	eventStore := memory.NewEventStore()
	readStore := memory.NewReadModelStore()
	handler := command.NewHandler(eventStore, readStore)
	ctx := context.Background()

	// Try to update non-existent association
	_, err := handler.UpdateAssociation(ctx, command.UpdateAssociationInput{
		ID:      uuid.New(),
		Role:    strPtr(domain.RoleWitness),
		Version: 1,
	})
	if err != command.ErrAssociationNotFound {
		t.Errorf("UpdateAssociation should fail with ErrAssociationNotFound, got: %v", err)
	}
}

// TestUpdateAssociation_EmptyRole tests that updating role to empty string fails.
func TestUpdateAssociation_EmptyRole(t *testing.T) {
	eventStore := memory.NewEventStore()
	readStore := memory.NewReadModelStore()
	handler := command.NewHandler(eventStore, readStore)
	ctx := context.Background()

	// Create persons and association
	person1Result, _ := handler.CreatePerson(ctx, command.CreatePersonInput{
		GivenName: "John",
		Surname:   "Smith",
		Gender:    "male",
	})
	person2Result, _ := handler.CreatePerson(ctx, command.CreatePersonInput{
		GivenName: "Mary",
		Surname:   "Jones",
		Gender:    "female",
	})

	createResult, _ := handler.CreateAssociation(ctx, command.CreateAssociationInput{
		PersonID:    person1Result.ID,
		AssociateID: person2Result.ID,
		Role:        domain.RoleGodparent,
	})

	// Try to update role to empty string
	emptyRole := ""
	_, err := handler.UpdateAssociation(ctx, command.UpdateAssociationInput{
		ID:      createResult.ID,
		Role:    &emptyRole,
		Version: createResult.Version,
	})
	if err == nil {
		t.Error("UpdateAssociation should fail when setting role to empty string")
	}
}

// TestDeleteAssociation tests deleting an association.
func TestDeleteAssociation(t *testing.T) {
	eventStore := memory.NewEventStore()
	readStore := memory.NewReadModelStore()
	handler := command.NewHandler(eventStore, readStore)
	ctx := context.Background()

	// Create persons and association
	person1Result, _ := handler.CreatePerson(ctx, command.CreatePersonInput{
		GivenName: "John",
		Surname:   "Smith",
		Gender:    "male",
	})
	person2Result, _ := handler.CreatePerson(ctx, command.CreatePersonInput{
		GivenName: "Mary",
		Surname:   "Jones",
		Gender:    "female",
	})

	createResult, _ := handler.CreateAssociation(ctx, command.CreateAssociationInput{
		PersonID:    person1Result.ID,
		AssociateID: person2Result.ID,
		Role:        domain.RoleGodparent,
	})

	// Delete association
	err := handler.DeleteAssociation(ctx, createResult.ID, createResult.Version, "Test deletion")
	if err != nil {
		t.Errorf("DeleteAssociation failed: %v", err)
	}

	// Verify deleted from read model
	assoc, _ := readStore.GetAssociation(ctx, createResult.ID)
	if assoc != nil {
		t.Error("Association should be deleted from read model")
	}
}

// TestDeleteAssociation_NotFound tests deleting a non-existent association.
func TestDeleteAssociation_NotFound(t *testing.T) {
	eventStore := memory.NewEventStore()
	readStore := memory.NewReadModelStore()
	handler := command.NewHandler(eventStore, readStore)
	ctx := context.Background()

	// Try to delete non-existent association
	err := handler.DeleteAssociation(ctx, uuid.New(), 1, "Should fail")
	if err != command.ErrAssociationNotFound {
		t.Errorf("DeleteAssociation should fail with ErrAssociationNotFound, got: %v", err)
	}
}

// TestDeleteAssociation_WrongVersion tests optimistic locking on delete.
func TestDeleteAssociation_WrongVersion(t *testing.T) {
	eventStore := memory.NewEventStore()
	readStore := memory.NewReadModelStore()
	handler := command.NewHandler(eventStore, readStore)
	ctx := context.Background()

	// Create persons and association
	person1Result, _ := handler.CreatePerson(ctx, command.CreatePersonInput{
		GivenName: "John",
		Surname:   "Smith",
		Gender:    "male",
	})
	person2Result, _ := handler.CreatePerson(ctx, command.CreatePersonInput{
		GivenName: "Mary",
		Surname:   "Jones",
		Gender:    "female",
	})

	createResult, _ := handler.CreateAssociation(ctx, command.CreateAssociationInput{
		PersonID:    person1Result.ID,
		AssociateID: person2Result.ID,
		Role:        domain.RoleGodparent,
	})

	// Try to delete with wrong version
	err := handler.DeleteAssociation(ctx, createResult.ID, 999, "Should fail")
	if err != repository.ErrConcurrencyConflict {
		t.Errorf("DeleteAssociation should fail with ErrConcurrencyConflict, got: %v", err)
	}
}

// TestUpdateAssociation_NoteIDs tests updating association note IDs.
func TestUpdateAssociation_NoteIDs(t *testing.T) {
	eventStore := memory.NewEventStore()
	readStore := memory.NewReadModelStore()
	handler := command.NewHandler(eventStore, readStore)
	ctx := context.Background()

	// Create persons
	person1Result, _ := handler.CreatePerson(ctx, command.CreatePersonInput{
		GivenName: "John",
		Surname:   "Smith",
		Gender:    "male",
	})
	person2Result, _ := handler.CreatePerson(ctx, command.CreatePersonInput{
		GivenName: "Mary",
		Surname:   "Jones",
		Gender:    "female",
	})

	// Create notes
	note1Result, _ := handler.CreateNote(ctx, command.CreateNoteInput{
		Text: "First note",
	})
	note2Result, _ := handler.CreateNote(ctx, command.CreateNoteInput{
		Text: "Second note",
	})

	// Create association with no note IDs
	createResult, _ := handler.CreateAssociation(ctx, command.CreateAssociationInput{
		PersonID:    person1Result.ID,
		AssociateID: person2Result.ID,
		Role:        domain.RoleGodparent,
	})

	// Update with note IDs
	noteIDs := []uuid.UUID{note1Result.ID, note2Result.ID}
	_, err := handler.UpdateAssociation(ctx, command.UpdateAssociationInput{
		ID:      createResult.ID,
		NoteIDs: &noteIDs,
		Version: createResult.Version,
	})
	if err != nil {
		t.Fatalf("UpdateAssociation failed: %v", err)
	}

	// Verify note IDs in read model
	assoc, _ := readStore.GetAssociation(ctx, createResult.ID)
	if len(assoc.NoteIDs) != 2 {
		t.Errorf("Expected 2 note IDs, got %d", len(assoc.NoteIDs))
	}
}
