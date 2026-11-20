package command_test

import (
	"context"
	"io"
	"slices"
	"strings"
	"testing"

	"github.com/google/uuid"

	"github.com/cacack/my-family/internal/command"
	"github.com/cacack/my-family/internal/domain"
	"github.com/cacack/my-family/internal/repository"
	"github.com/cacack/my-family/internal/repository/memory"
)

// mediaGedcom has an OBJE record linked from a person, a family and a source,
// and one linked from nothing (#1435).
const mediaGedcom = `0 HEAD
1 GEDC
2 VERS 7.0
0 @M1@ OBJE
1 RESN CONFIDENTIAL
1 FILE photos/wedding.jpg
2 FORM image/jpeg
3 MEDI PHOTO
2 TITL Wedding
0 @M2@ OBJE
1 FILE scans/unused.pdf
2 FORM application/pdf
2 TITL Unused
0 @S1@ SOUR
1 TITL Register
1 OBJE @M1@
0 @I1@ INDI
1 NAME John /Doe/
1 OBJE @M1@
1 OBJE @M1@
0 @F1@ FAM
1 HUSB @I1@
1 OBJE @M1@
0 TRLR
`

func TestImportGedcom_MediaOneItemPerOwner(t *testing.T) {
	ctx := context.Background()
	readStore := memory.NewReadModelStore()
	handler := command.NewHandler(memory.NewEventStore(), readStore)

	result, err := handler.ImportGedcom(ctx, command.ImportGedcomInput{Filename: "m.ged", Reader: strings.NewReader(mediaGedcom)})
	if err != nil {
		t.Fatalf("ImportGedcom: %v", err)
	}
	if result.MediaImported != 3 {
		t.Errorf("MediaImported = %d, want 3 (one per owner of @M1@)", result.MediaImported)
	}
	if !slices.ContainsFunc(result.Warnings, func(w string) bool { return strings.Contains(w, "@M2@") }) {
		t.Errorf("warnings %q lack the unlinked @M2@", result.Warnings)
	}

	persons, _, _ := readStore.ListPersons(ctx, repository.DefaultListOptions())
	families, _, _ := readStore.ListFamilies(ctx, repository.DefaultListOptions())
	sources, _, _ := readStore.ListSources(ctx, repository.DefaultListOptions())
	if len(persons) != 1 || len(families) != 1 || len(sources) != 1 {
		t.Fatalf("imported %d persons, %d families, %d sources; want 1 each", len(persons), len(families), len(sources))
	}
	for _, owner := range []struct {
		entityType string
		id         uuid.UUID
	}{{"person", persons[0].ID}, {"family", families[0].ID}, {"source", sources[0].ID}} {
		media, _, err := readStore.ListMediaForEntity(ctx, owner.entityType, owner.id, repository.DefaultListOptions())
		if err != nil || len(media) != 1 {
			t.Fatalf("%s media = %d, %v; want 1", owner.entityType, len(media), err)
		}
		m := media[0]
		if m.Title != "Wedding" || m.MediaType != domain.MediaPhoto || m.GedcomXref != "@M1@" ||
			len(m.Files) != 1 || m.Files[0].Path != "photos/wedding.jpg" || !m.Restriction.IsPrivate() {
			t.Errorf("%s media = %+v, want the private Wedding photo of @M1@", owner.entityType, m)
		}
	}
}

func TestImportGedcom_MediaAppendFailureWarns(t *testing.T) {
	ctx := context.Background()
	readStore := memory.NewReadModelStore()
	failing := newMockEventStore()
	failing.appendError = io.ErrUnexpectedEOF
	handler := command.NewHandler(failing, readStore)

	result, err := handler.ImportGedcom(ctx, command.ImportGedcomInput{Filename: "m.ged", Reader: strings.NewReader(mediaGedcom)})
	if err != nil {
		t.Fatalf("ImportGedcom: %v", err)
	}
	if result.MediaImported != 0 {
		t.Errorf("MediaImported = %d, want 0", result.MediaImported)
	}
	if !slices.ContainsFunc(result.Warnings, func(w string) bool { return strings.Contains(w, "Failed to import media @M1@") }) {
		t.Errorf("warnings %q lack the failed media", result.Warnings)
	}
}
