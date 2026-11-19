package gedcom_test

import (
	"bytes"
	"context"
	"strings"
	"testing"

	"github.com/google/uuid"

	"github.com/cacack/my-family/internal/domain"
	"github.com/cacack/my-family/internal/gedcom"
	"github.com/cacack/my-family/internal/repository"
	"github.com/cacack/my-family/internal/repository/memory"
)

// A GEDCOM 7.0 shared note keeps its _RESN through import and an export that
// writes it with its SNOTE metadata (ADR-008 §2).
func TestRestriction_SharedNoteRoundTrips(t *testing.T) {
	const input = `0 HEAD
1 GEDC
2 VERS 7.0
0 @N1@ SNOTE <p>Private</p>
1 MIME text/html
1 _RESN CONFIDENTIAL
0 TRLR
`
	_, _, _, _, _, _, _, _, notes, _, _, _, _, err := gedcom.NewImporter().Import(context.Background(), strings.NewReader(input))
	if err != nil {
		t.Fatalf("import: %v", err)
	}
	if len(notes) != 1 || notes[0].Restriction != "CONFIDENTIAL" {
		t.Fatalf("notes = %+v, want one with restriction CONFIDENTIAL", notes)
	}

	readStore := memory.NewReadModelStore()
	ctx := context.Background()
	note := &repository.NoteReadModel{ID: uuid.New(), Text: "<p>Private</p>", MIME: "text/html", Restriction: domain.Restriction(notes[0].Restriction)}
	if err := readStore.SaveNote(ctx, domain.MainBranchID, note); err != nil {
		t.Fatalf("SaveNote: %v", err)
	}
	var buf bytes.Buffer
	if _, err := gedcom.NewExporter(readStore).Export(ctx, &buf); err != nil {
		t.Fatalf("export: %v", err)
	}
	if !strings.Contains(buf.String(), "1 _RESN CONFIDENTIAL") {
		t.Errorf("export lacks the note's _RESN:\n%s", buf.String())
	}
}
