package domain

import (
	"strings"
	"testing"

	"github.com/google/uuid"
)

func TestNewAssociation(t *testing.T) {
	personID := uuid.New()
	associateID := uuid.New()
	role := RoleGodparent

	a := NewAssociation(personID, associateID, role)

	if a.ID == uuid.Nil {
		t.Error("Expected non-nil UUID")
	}
	if a.PersonID != personID {
		t.Errorf("PersonID = %v, want %v", a.PersonID, personID)
	}
	if a.AssociateID != associateID {
		t.Errorf("AssociateID = %v, want %v", a.AssociateID, associateID)
	}
	if a.Role != role {
		t.Errorf("Role = %v, want %v", a.Role, role)
	}
	if a.Version != 1 {
		t.Errorf("Version = %v, want 1", a.Version)
	}
	if a.Phrase != "" {
		t.Errorf("Phrase should be empty, got %v", a.Phrase)
	}
	if a.Notes != "" {
		t.Errorf("Notes should be empty, got %v", a.Notes)
	}
	if len(a.NoteIDs) != 0 {
		t.Errorf("NoteIDs should be empty, got %v", a.NoteIDs)
	}
	if a.GedcomXref != "" {
		t.Errorf("GedcomXref should be empty, got %v", a.GedcomXref)
	}
}

func TestNewAssociationWithID(t *testing.T) {
	id := uuid.New()
	personID := uuid.New()
	associateID := uuid.New()
	role := RoleWitness

	a := NewAssociationWithID(id, personID, associateID, role)

	if a.ID != id {
		t.Errorf("ID = %v, want %v", a.ID, id)
	}
	if a.PersonID != personID {
		t.Errorf("PersonID = %v, want %v", a.PersonID, personID)
	}
	if a.AssociateID != associateID {
		t.Errorf("AssociateID = %v, want %v", a.AssociateID, associateID)
	}
	if a.Role != role {
		t.Errorf("Role = %v, want %v", a.Role, role)
	}
	if a.Version != 1 {
		t.Errorf("Version = %v, want 1", a.Version)
	}
}

func TestAssociation_Validate(t *testing.T) {
	validPersonID := uuid.New()
	validAssociateID := uuid.New()

	tests := []struct {
		name       string
		assoc      *Association
		wantErr    bool
		errMessage string
	}{
		{
			name:    "valid association with godparent role",
			assoc:   NewAssociation(validPersonID, validAssociateID, RoleGodparent),
			wantErr: false,
		},
		{
			name:    "valid association with witness role",
			assoc:   NewAssociation(validPersonID, validAssociateID, RoleWitness),
			wantErr: false,
		},
		{
			name:    "valid association with custom role",
			assoc:   NewAssociation(validPersonID, validAssociateID, "mentor"),
			wantErr: false,
		},
		{
			name: "valid association with phrase",
			assoc: func() *Association {
				a := NewAssociation(validPersonID, validAssociateID, RoleGodparent)
				a.SetPhrase("John was the godparent at the baptism")
				return a
			}(),
			wantErr: false,
		},
		{
			name: "valid association with notes",
			assoc: func() *Association {
				a := NewAssociation(validPersonID, validAssociateID, RoleWitness)
				a.SetNotes("Witnessed the marriage ceremony")
				return a
			}(),
			wantErr: false,
		},
		{
			name: "valid association with note IDs",
			assoc: func() *Association {
				a := NewAssociation(validPersonID, validAssociateID, RoleWitness)
				a.AddNoteID(uuid.New())
				a.AddNoteID(uuid.New())
				return a
			}(),
			wantErr: false,
		},
		{
			name: "valid association with gedcom xref",
			assoc: func() *Association {
				a := NewAssociation(validPersonID, validAssociateID, RoleGodparent)
				a.SetGedcomXref("@I1@")
				return a
			}(),
			wantErr: false,
		},
		{
			name:       "invalid - nil person ID",
			assoc:      NewAssociation(uuid.Nil, validAssociateID, RoleGodparent),
			wantErr:    true,
			errMessage: "person_id",
		},
		{
			name:       "invalid - nil associate ID",
			assoc:      NewAssociation(validPersonID, uuid.Nil, RoleGodparent),
			wantErr:    true,
			errMessage: "associate_id",
		},
		{
			name:       "invalid - empty role",
			assoc:      NewAssociation(validPersonID, validAssociateID, ""),
			wantErr:    true,
			errMessage: "role",
		},
		{
			name:       "invalid - self association",
			assoc:      NewAssociation(validPersonID, validPersonID, RoleGodparent),
			wantErr:    true,
			errMessage: "cannot associate with self",
		},
		{
			name: "invalid - role too long",
			assoc: func() *Association {
				return NewAssociation(validPersonID, validAssociateID, strings.Repeat("a", 101))
			}(),
			wantErr:    true,
			errMessage: "role",
		},
		{
			name: "invalid - phrase too long",
			assoc: func() *Association {
				a := NewAssociation(validPersonID, validAssociateID, RoleGodparent)
				a.SetPhrase(strings.Repeat("a", 501))
				return a
			}(),
			wantErr:    true,
			errMessage: "phrase",
		},
		{
			name: "invalid - multiple errors (nil person and associate)",
			assoc: &Association{
				ID:          uuid.New(),
				PersonID:    uuid.Nil,
				AssociateID: uuid.Nil,
				Role:        "",
			},
			wantErr:    true,
			errMessage: "person_id",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.assoc.Validate()
			if (err != nil) != tt.wantErr {
				t.Errorf("Validate() error = %v, wantErr %v", err, tt.wantErr)
			}
			if tt.wantErr && tt.errMessage != "" && !strings.Contains(err.Error(), tt.errMessage) {
				t.Errorf("Validate() error = %v, should contain %v", err, tt.errMessage)
			}
		})
	}
}

func TestAssociation_SetPhrase(t *testing.T) {
	a := NewAssociation(uuid.New(), uuid.New(), RoleGodparent)

	phrase := "John was the godparent at the baptism ceremony in 1850"
	a.SetPhrase(phrase)

	if a.Phrase != phrase {
		t.Errorf("Phrase = %v, want %v", a.Phrase, phrase)
	}
}

func TestAssociation_SetNotes(t *testing.T) {
	a := NewAssociation(uuid.New(), uuid.New(), RoleWitness)

	notes := "Witnessed the marriage ceremony on March 15, 1852"
	a.SetNotes(notes)

	if a.Notes != notes {
		t.Errorf("Notes = %v, want %v", a.Notes, notes)
	}
}

func TestAssociation_AddNoteID(t *testing.T) {
	a := NewAssociation(uuid.New(), uuid.New(), RoleGodparent)

	noteID1 := uuid.New()
	noteID2 := uuid.New()

	a.AddNoteID(noteID1)
	if len(a.NoteIDs) != 1 {
		t.Errorf("Expected 1 note ID, got %d", len(a.NoteIDs))
	}
	if a.NoteIDs[0] != noteID1 {
		t.Errorf("NoteID[0] = %v, want %v", a.NoteIDs[0], noteID1)
	}

	a.AddNoteID(noteID2)
	if len(a.NoteIDs) != 2 {
		t.Errorf("Expected 2 note IDs, got %d", len(a.NoteIDs))
	}
	if a.NoteIDs[1] != noteID2 {
		t.Errorf("NoteID[1] = %v, want %v", a.NoteIDs[1], noteID2)
	}
}

func TestAssociation_AddNoteID_Nil(t *testing.T) {
	a := NewAssociation(uuid.New(), uuid.New(), RoleGodparent)

	// Adding nil UUID should be ignored
	a.AddNoteID(uuid.Nil)
	if len(a.NoteIDs) != 0 {
		t.Errorf("Expected 0 note IDs (nil should be ignored), got %d", len(a.NoteIDs))
	}
}

func TestAssociation_SetGedcomXref(t *testing.T) {
	a := NewAssociation(uuid.New(), uuid.New(), RoleGodparent)

	xref := "@I123@"
	a.SetGedcomXref(xref)

	if a.GedcomXref != xref {
		t.Errorf("GedcomXref = %v, want %v", a.GedcomXref, xref)
	}
}

func TestAssociationValidationError_Error(t *testing.T) {
	err := AssociationValidationError{Field: "role", Message: "cannot be empty"}
	expected := "role: cannot be empty"
	if err.Error() != expected {
		t.Errorf("Error() = %v, want %v", err.Error(), expected)
	}
}

func TestRoleConstants(t *testing.T) {
	// Verify role constants match expected values
	if RoleGodparent != "godparent" {
		t.Errorf("RoleGodparent = %v, want 'godparent'", RoleGodparent)
	}
	if RoleWitness != "witness" {
		t.Errorf("RoleWitness = %v, want 'witness'", RoleWitness)
	}
}
