package query_test

import (
	"context"
	"testing"

	"github.com/google/uuid"

	"github.com/cacack/my-family/internal/command"
	"github.com/cacack/my-family/internal/query"
	"github.com/cacack/my-family/internal/repository/memory"
)

// TestListNotes tests listing notes with pagination.
func TestListNotes(t *testing.T) {
	eventStore := memory.NewEventStore()
	readStore := memory.NewReadModelStore()
	cmdHandler := command.NewHandler(eventStore, readStore)
	queryService := query.NewNoteService(readStore)
	ctx := context.Background()

	// Create test notes
	notes := []string{"First note", "Second note", "Third note"}
	for _, text := range notes {
		_, err := cmdHandler.CreateNote(ctx, command.CreateNoteInput{
			Text: text,
		})
		if err != nil {
			t.Fatalf("Failed to create note: %v", err)
		}
	}

	tests := []struct {
		name      string
		input     query.ListNotesInput
		wantCount int
		wantTotal int
	}{
		{
			name: "list all",
			input: query.ListNotesInput{
				Limit: 10,
			},
			wantCount: 3,
			wantTotal: 3,
		},
		{
			name: "with pagination",
			input: query.ListNotesInput{
				Limit:  2,
				Offset: 0,
			},
			wantCount: 2,
			wantTotal: 3,
		},
		{
			name: "second page",
			input: query.ListNotesInput{
				Limit:  2,
				Offset: 2,
			},
			wantCount: 1,
			wantTotal: 3,
		},
		{
			name: "default limit when not specified",
			input: query.ListNotesInput{
				Limit: 0, // should default to 20
			},
			wantCount: 3,
			wantTotal: 3,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := queryService.ListNotes(ctx, tt.input)
			if err != nil {
				t.Fatalf("ListNotes failed: %v", err)
			}

			if len(result.Notes) != tt.wantCount {
				t.Errorf("Got %d notes, want %d", len(result.Notes), tt.wantCount)
			}

			if result.Total != tt.wantTotal {
				t.Errorf("Total = %d, want %d", result.Total, tt.wantTotal)
			}
		})
	}
}

// TestListNotes_LimitEnforcement tests that limits are enforced.
func TestListNotes_LimitEnforcement(t *testing.T) {
	eventStore := memory.NewEventStore()
	readStore := memory.NewReadModelStore()
	cmdHandler := command.NewHandler(eventStore, readStore)
	queryService := query.NewNoteService(readStore)
	ctx := context.Background()

	// Create 5 notes
	for i := 0; i < 5; i++ {
		_, err := cmdHandler.CreateNote(ctx, command.CreateNoteInput{
			Text: "Test note",
		})
		if err != nil {
			t.Fatalf("Failed to create note: %v", err)
		}
	}

	// Request with limit > 100 should be capped at 100
	result, err := queryService.ListNotes(ctx, query.ListNotesInput{
		Limit: 150, // Should be capped at 100
	})
	if err != nil {
		t.Fatalf("ListNotes failed: %v", err)
	}

	// Since we only have 5 notes, we should get 5
	if len(result.Notes) != 5 {
		t.Errorf("Got %d notes, want 5", len(result.Notes))
	}

	// But the limit should be set to 100
	if result.Limit != 100 {
		t.Errorf("Limit = %d, want 100", result.Limit)
	}
}

// TestListNotes_SortOrder tests sorting.
func TestListNotes_SortOrder(t *testing.T) {
	readStore := memory.NewReadModelStore()
	queryService := query.NewNoteService(readStore)
	ctx := context.Background()

	// Test default order (desc)
	result, err := queryService.ListNotes(ctx, query.ListNotesInput{
		Limit: 10,
	})
	if err != nil {
		t.Fatalf("ListNotes failed: %v", err)
	}

	// Just verify it doesn't error with empty results
	if result == nil {
		t.Error("Result should not be nil")
	}

	// Test explicit ascending order
	result, err = queryService.ListNotes(ctx, query.ListNotesInput{
		Limit:     10,
		SortOrder: "asc",
	})
	if err != nil {
		t.Fatalf("ListNotes with asc order failed: %v", err)
	}
	if result == nil {
		t.Error("Result should not be nil")
	}
}

// TestGetNote tests getting a note by ID.
func TestGetNote(t *testing.T) {
	eventStore := memory.NewEventStore()
	readStore := memory.NewReadModelStore()
	cmdHandler := command.NewHandler(eventStore, readStore)
	queryService := query.NewNoteService(readStore)
	ctx := context.Background()

	// Create note
	createResult, err := cmdHandler.CreateNote(ctx, command.CreateNoteInput{
		Text:       "Test note content",
		GedcomXref: "@N1@",
	})
	if err != nil {
		t.Fatalf("CreateNote failed: %v", err)
	}

	// Get note
	result, err := queryService.GetNote(ctx, createResult.ID)
	if err != nil {
		t.Fatalf("GetNote failed: %v", err)
	}

	if result.ID != createResult.ID {
		t.Errorf("ID = %v, want %v", result.ID, createResult.ID)
	}
	if result.Text != "Test note content" {
		t.Errorf("Text = %s, want 'Test note content'", result.Text)
	}
	if result.GedcomXref == nil || *result.GedcomXref != "@N1@" {
		t.Error("GedcomXref not set correctly")
	}
	if result.Version != 1 {
		t.Errorf("Version = %d, want 1", result.Version)
	}
}

// TestGetNote_NotFound tests getting a non-existent note.
func TestGetNote_NotFound(t *testing.T) {
	readStore := memory.NewReadModelStore()
	queryService := query.NewNoteService(readStore)
	ctx := context.Background()

	_, err := queryService.GetNote(ctx, uuid.New())
	if err != query.ErrNotFound {
		t.Errorf("Expected ErrNotFound, got %v", err)
	}
}

// TestGetNote_NoGedcomXref tests that notes without GedcomXref have nil pointer.
func TestGetNote_NoGedcomXref(t *testing.T) {
	eventStore := memory.NewEventStore()
	readStore := memory.NewReadModelStore()
	cmdHandler := command.NewHandler(eventStore, readStore)
	queryService := query.NewNoteService(readStore)
	ctx := context.Background()

	// Create note without GedcomXref
	createResult, err := cmdHandler.CreateNote(ctx, command.CreateNoteInput{
		Text: "Note without xref",
	})
	if err != nil {
		t.Fatalf("CreateNote failed: %v", err)
	}

	// Get note
	result, err := queryService.GetNote(ctx, createResult.ID)
	if err != nil {
		t.Fatalf("GetNote failed: %v", err)
	}

	if result.GedcomXref != nil {
		t.Errorf("GedcomXref should be nil, got %v", result.GedcomXref)
	}
}

// TestListNotes_Empty tests listing with no notes.
func TestListNotes_Empty(t *testing.T) {
	readStore := memory.NewReadModelStore()
	queryService := query.NewNoteService(readStore)
	ctx := context.Background()

	result, err := queryService.ListNotes(ctx, query.ListNotesInput{
		Limit: 10,
	})
	if err != nil {
		t.Fatalf("ListNotes failed: %v", err)
	}

	if len(result.Notes) != 0 {
		t.Errorf("Got %d notes, want 0", len(result.Notes))
	}
	if result.Total != 0 {
		t.Errorf("Total = %d, want 0", result.Total)
	}
}
