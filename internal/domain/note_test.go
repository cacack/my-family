package domain

import (
	"testing"

	"github.com/google/uuid"
)

func TestNewNote(t *testing.T) {
	text := "This is a test note with some text content."
	n := NewNote(text)

	if n.ID == uuid.Nil {
		t.Error("Expected non-nil UUID")
	}
	if n.Text != text {
		t.Errorf("Text = %v, want %v", n.Text, text)
	}
	if n.Version != 1 {
		t.Errorf("Version = %v, want 1", n.Version)
	}
	if n.GedcomXref != "" {
		t.Errorf("GedcomXref should be empty, got %v", n.GedcomXref)
	}
}

func TestNewNote_EmptyText(t *testing.T) {
	n := NewNote("")

	if n.ID == uuid.Nil {
		t.Error("Expected non-nil UUID")
	}
	if n.Text != "" {
		t.Errorf("Text = %v, want empty string", n.Text)
	}
	if n.Version != 1 {
		t.Errorf("Version = %v, want 1", n.Version)
	}
}

func TestNewNoteWithID(t *testing.T) {
	id := uuid.New()
	text := "Note with specific ID"
	n := NewNoteWithID(id, text)

	if n.ID != id {
		t.Errorf("ID = %v, want %v", n.ID, id)
	}
	if n.Text != text {
		t.Errorf("Text = %v, want %v", n.Text, text)
	}
	if n.Version != 1 {
		t.Errorf("Version = %v, want 1", n.Version)
	}
}

func TestNote_Validate(t *testing.T) {
	tests := []struct {
		name    string
		note    *Note
		wantErr bool
	}{
		{
			name:    "valid note with text",
			note:    NewNote("This is a valid note"),
			wantErr: false,
		},
		{
			name:    "valid note with empty text",
			note:    NewNote(""),
			wantErr: false,
		},
		{
			name: "valid note with gedcom xref",
			note: func() *Note {
				n := NewNote("Note with xref")
				n.SetGedcomXref("@N1@")
				return n
			}(),
			wantErr: false,
		},
		{
			name:    "invalid - nil ID",
			note:    &Note{ID: uuid.Nil, Text: "Some text"},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.note.Validate()
			if (err != nil) != tt.wantErr {
				t.Errorf("Validate() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestNote_SetText(t *testing.T) {
	n := NewNote("Original text")

	newText := "Updated text"
	n.SetText(newText)

	if n.Text != newText {
		t.Errorf("Text = %v, want %v", n.Text, newText)
	}
}

func TestNote_SetGedcomXref(t *testing.T) {
	n := NewNote("Test note")

	xref := "@N123@"
	n.SetGedcomXref(xref)

	if n.GedcomXref != xref {
		t.Errorf("GedcomXref = %v, want %v", n.GedcomXref, xref)
	}
}

func TestNoteValidationError_Error(t *testing.T) {
	err := NoteValidationError{Field: "id", Message: "id is required"}
	expected := "id: id is required"
	if err.Error() != expected {
		t.Errorf("Error() = %v, want %v", err.Error(), expected)
	}
}
