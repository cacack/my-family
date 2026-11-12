package command_test

import (
	"errors"
	"strings"
	"testing"

	"github.com/google/uuid"

	"github.com/cacack/my-family/internal/command"
)

// A gedcom_xref entered through a create or update command must be empty or
// one @-delimited pointer of at most 64 characters (#527 item 6).
func TestGedcomXref_ValidatedOnInput(t *testing.T) {
	valid := []string{"@R1@", "@I_123@", "@" + strings.Repeat("x", 62) + "@"}
	invalid := []string{
		"R1",
		"@R1",
		"@R 1@",
		"@R1@\n0 INDI",
		"@a@b@",
		"@@",
		"@" + strings.Repeat("x", 63) + "@",
	}

	// Each submits xref through one command that accepts it.
	submit := map[string]func(xref string) error{
		"CreateRepository": func(xref string) error {
			ctx, _, _, h := newTestHandler()
			_, err := h.CreateRepository(ctx, command.CreateRepositoryInput{Name: "Archives", GedcomXref: xref})
			return err
		},
		"UpdateRepository": func(xref string) error {
			ctx, _, _, h := newTestHandler()
			created, err := h.CreateRepository(ctx, command.CreateRepositoryInput{Name: "Archives"})
			if err != nil {
				return err
			}
			_, err = h.UpdateRepository(ctx, command.UpdateRepositoryInput{ID: created.ID, GedcomXref: &xref, Version: created.Version})
			return err
		},
		"CreateNote": func(xref string) error {
			ctx, _, _, h := newTestHandler()
			_, err := h.CreateNote(ctx, command.CreateNoteInput{Text: "note", GedcomXref: xref})
			return err
		},
		"CreateSubmitter": func(xref string) error {
			ctx, _, _, h := newTestHandler()
			_, err := h.CreateSubmitter(ctx, command.CreateSubmitterInput{Name: "Jane", GedcomXref: xref})
			return err
		},
		"CreateCitation": func(xref string) error {
			ctx, _, _, h := newTestHandler()
			source, err := h.CreateSource(ctx, command.CreateSourceInput{SourceType: "census", Title: "1880 Census"})
			if err != nil {
				return err
			}
			_, err = h.CreateCitation(ctx, command.CreateCitationInput{
				SourceID: source.ID, FactType: "person_birth", FactOwnerID: uuid.New(), GedcomXref: xref,
			})
			return err
		},
	}

	for name, fn := range submit {
		t.Run(name, func(t *testing.T) {
			for _, xref := range valid {
				if err := fn(xref); err != nil {
					t.Errorf("xref %q: unexpected error %v", xref, err)
				}
			}
			for _, xref := range invalid {
				if err := fn(xref); !errors.Is(err, command.ErrInvalidInput) {
					t.Errorf("xref %q: err = %v, want ErrInvalidInput", xref, err)
				}
			}
		})
	}
}

// A stored xref outside the accepted shape (GEDCOM 5.5.1 imports keep `@R 1@`
// verbatim) does not block an edit that leaves it unchanged, and can be cleared.
func TestUpdateRepository_KeepsStoredLegacyXref(t *testing.T) {
	ctx, _, read, h := newTestHandler()
	created, err := h.CreateRepository(ctx, command.CreateRepositoryInput{Name: "Archives"})
	if err != nil {
		t.Fatalf("CreateRepository: %v", err)
	}
	stored, err := read.GetRepository(ctx, created.ID)
	if err != nil || stored == nil {
		t.Fatalf("GetRepository: %v", err)
	}
	stored.GedcomXref = "@R 1@"
	if err := read.SaveRepository(ctx, stored); err != nil {
		t.Fatalf("SaveRepository: %v", err)
	}

	legacy, renamed := "@R 1@", "Archive"
	result, err := h.UpdateRepository(ctx, command.UpdateRepositoryInput{
		ID: created.ID, Name: &renamed, GedcomXref: &legacy, Version: created.Version,
	})
	if err != nil {
		t.Fatalf("rename with the stored xref unchanged: %v", err)
	}

	cleared := ""
	if _, err := h.UpdateRepository(ctx, command.UpdateRepositoryInput{
		ID: created.ID, GedcomXref: &cleared, Version: result.Version,
	}); err != nil {
		t.Fatalf("clear the stored xref: %v", err)
	}
}
