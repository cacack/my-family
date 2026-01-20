package command_test

import (
	"context"
	"testing"

	"github.com/google/uuid"

	"github.com/cacack/my-family/internal/command"
	"github.com/cacack/my-family/internal/repository"
	"github.com/cacack/my-family/internal/repository/memory"
)

// TestCreateNote tests creating a new note.
func TestCreateNote(t *testing.T) {
	eventStore := memory.NewEventStore()
	readStore := memory.NewReadModelStore()
	handler := command.NewHandler(eventStore, readStore)
	ctx := context.Background()

	tests := []struct {
		name    string
		input   command.CreateNoteInput
		wantErr bool
	}{
		{
			name: "valid note with text",
			input: command.CreateNoteInput{
				Text: "This is a test note with some important information.",
			},
			wantErr: false,
		},
		{
			name: "valid note with empty text",
			input: command.CreateNoteInput{
				Text: "",
			},
			wantErr: false,
		},
		{
			name: "valid note with gedcom xref",
			input: command.CreateNoteInput{
				Text:       "Note imported from GEDCOM",
				GedcomXref: "@N1@",
			},
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := handler.CreateNote(ctx, tt.input)
			if (err != nil) != tt.wantErr {
				t.Errorf("CreateNote() error = %v, wantErr %v", err, tt.wantErr)
				return
			}

			if !tt.wantErr {
				if result.ID == uuid.Nil {
					t.Error("Expected non-nil ID")
				}
				if result.Version != 1 {
					t.Errorf("Version = %d, want 1", result.Version)
				}

				// Verify note in read model
				note, _ := readStore.GetNote(ctx, result.ID)
				if note == nil {
					t.Fatal("Note not found in read model")
				}
				if note.Text != tt.input.Text {
					t.Errorf("Text = %s, want %s", note.Text, tt.input.Text)
				}
				if note.GedcomXref != tt.input.GedcomXref {
					t.Errorf("GedcomXref = %s, want %s", note.GedcomXref, tt.input.GedcomXref)
				}
			}
		})
	}
}

// TestUpdateNote tests updating a note.
func TestUpdateNote(t *testing.T) {
	eventStore := memory.NewEventStore()
	readStore := memory.NewReadModelStore()
	handler := command.NewHandler(eventStore, readStore)
	ctx := context.Background()

	// Create note first
	createResult, err := handler.CreateNote(ctx, command.CreateNoteInput{
		Text: "Original note text",
	})
	if err != nil {
		t.Fatalf("CreateNote failed: %v", err)
	}

	tests := []struct {
		name    string
		input   command.UpdateNoteInput
		wantErr bool
	}{
		{
			name: "update text",
			input: command.UpdateNoteInput{
				ID:      createResult.ID,
				Text:    strPtr("Updated note text"),
				Version: createResult.Version,
			},
			wantErr: false,
		},
		{
			name: "wrong version (optimistic locking)",
			input: command.UpdateNoteInput{
				ID:      createResult.ID,
				Text:    strPtr("Should Fail"),
				Version: 999,
			},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := handler.UpdateNote(ctx, tt.input)
			if (err != nil) != tt.wantErr {
				t.Errorf("UpdateNote() error = %v, wantErr %v", err, tt.wantErr)
				return
			}

			if !tt.wantErr && result.Version <= tt.input.Version {
				t.Errorf("Version not incremented: got %d, want > %d", result.Version, tt.input.Version)
			}
		})
	}
}

// TestUpdateNote_NoChanges tests that updating without changes returns current version.
func TestUpdateNote_NoChanges(t *testing.T) {
	eventStore := memory.NewEventStore()
	readStore := memory.NewReadModelStore()
	handler := command.NewHandler(eventStore, readStore)
	ctx := context.Background()

	// Create note
	createResult, _ := handler.CreateNote(ctx, command.CreateNoteInput{
		Text: "Test note",
	})

	// Update with no changes
	result, err := handler.UpdateNote(ctx, command.UpdateNoteInput{
		ID:      createResult.ID,
		Version: createResult.Version,
	})
	if err != nil {
		t.Fatalf("UpdateNote failed: %v", err)
	}

	if result.Version != createResult.Version {
		t.Errorf("Version changed without updates: got %d, want %d", result.Version, createResult.Version)
	}
}

// TestUpdateNote_NotFound tests updating a non-existent note.
func TestUpdateNote_NotFound(t *testing.T) {
	eventStore := memory.NewEventStore()
	readStore := memory.NewReadModelStore()
	handler := command.NewHandler(eventStore, readStore)
	ctx := context.Background()

	// Try to update non-existent note
	_, err := handler.UpdateNote(ctx, command.UpdateNoteInput{
		ID:      uuid.New(),
		Text:    strPtr("Should Fail"),
		Version: 1,
	})
	if err != command.ErrNoteNotFound {
		t.Errorf("UpdateNote should fail with ErrNoteNotFound, got: %v", err)
	}
}

// TestDeleteNote tests deleting a note.
func TestDeleteNote(t *testing.T) {
	eventStore := memory.NewEventStore()
	readStore := memory.NewReadModelStore()
	handler := command.NewHandler(eventStore, readStore)
	ctx := context.Background()

	// Create note
	createResult, _ := handler.CreateNote(ctx, command.CreateNoteInput{
		Text: "Test note",
	})

	// Delete note
	err := handler.DeleteNote(ctx, createResult.ID, createResult.Version, "Test deletion")
	if err != nil {
		t.Errorf("DeleteNote failed: %v", err)
	}

	// Verify deleted from read model
	note, _ := readStore.GetNote(ctx, createResult.ID)
	if note != nil {
		t.Error("Note should be deleted from read model")
	}
}

// TestDeleteNote_NotFound tests deleting a non-existent note.
func TestDeleteNote_NotFound(t *testing.T) {
	eventStore := memory.NewEventStore()
	readStore := memory.NewReadModelStore()
	handler := command.NewHandler(eventStore, readStore)
	ctx := context.Background()

	// Try to delete non-existent note
	err := handler.DeleteNote(ctx, uuid.New(), 1, "Should fail")
	if err != command.ErrNoteNotFound {
		t.Errorf("DeleteNote should fail with ErrNoteNotFound, got: %v", err)
	}
}

// TestDeleteNote_WrongVersion tests optimistic locking on delete.
func TestDeleteNote_WrongVersion(t *testing.T) {
	eventStore := memory.NewEventStore()
	readStore := memory.NewReadModelStore()
	handler := command.NewHandler(eventStore, readStore)
	ctx := context.Background()

	// Create note
	createResult, _ := handler.CreateNote(ctx, command.CreateNoteInput{
		Text: "Test note",
	})

	// Try to delete with wrong version
	err := handler.DeleteNote(ctx, createResult.ID, 999, "Should fail")
	if err != repository.ErrConcurrencyConflict {
		t.Errorf("DeleteNote should fail with ErrConcurrencyConflict, got: %v", err)
	}
}

// TestUpdateNote_TextChange tests updating note text and verifying it in read model.
func TestUpdateNote_TextChange(t *testing.T) {
	eventStore := memory.NewEventStore()
	readStore := memory.NewReadModelStore()
	handler := command.NewHandler(eventStore, readStore)
	ctx := context.Background()

	// Create note
	createResult, _ := handler.CreateNote(ctx, command.CreateNoteInput{
		Text: "Original text",
	})

	// Update text
	newText := "Updated text content"
	result, err := handler.UpdateNote(ctx, command.UpdateNoteInput{
		ID:      createResult.ID,
		Text:    &newText,
		Version: createResult.Version,
	})
	if err != nil {
		t.Fatalf("UpdateNote failed: %v", err)
	}

	if result.Version <= createResult.Version {
		t.Errorf("Version not incremented: got %d, want > %d", result.Version, createResult.Version)
	}

	// Verify change in read model
	note, _ := readStore.GetNote(ctx, createResult.ID)
	if note.Text != newText {
		t.Errorf("Text = %s, want %s", note.Text, newText)
	}
}
