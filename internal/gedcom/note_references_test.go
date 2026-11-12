package gedcom_test

import (
	"context"
	"strings"
	"testing"

	"github.com/cacack/my-family/internal/command"
	"github.com/cacack/my-family/internal/domain"
	"github.com/cacack/my-family/internal/gedcom"
	"github.com/cacack/my-family/internal/repository"
	"github.com/cacack/my-family/internal/repository/memory"
)

// noteReferencesGedcom points @N1@ from every kind of record that can hold a
// NOTE pointer, among other notes; @N2@ is pointed to by nothing, and @N11@
// only resembles @N1@.
const noteReferencesGedcom = `0 HEAD
1 GEDC
2 VERS 5.5.1
1 CHAR UTF-8
0 @N1@ NOTE Shared.
0 @N2@ NOTE Unused.
0 @N11@ NOTE Lookalike.
0 @I1@ INDI
1 NAME John /Doe/
2 NOTE @N1@
1 NOTE Inline text first
1 NOTE @N1@
1 BIRT
2 NOTE @N11@
1 BURI
2 NOTE @N1@
1 OCCU Farmer
2 NOTE @N1@
1 ASSO @I2@
2 RELA Godfather
2 NOTE @N11@
2 NOTE @N1@
0 @I2@ INDI
1 NAME Mary /Roe/
1 NOTE @N11@
0 @F1@ FAM
1 HUSB @I1@
1 WIFE @I2@
1 MARR
2 NOTE @N1@
0 @R1@ REPO
1 NAME Archive
1 NOTE @N1@
0 @S1@ SOUR
1 TITL Register
1 NOTE @N1@
0 TRLR
`

func TestNoteReferences(t *testing.T) {
	ctx := context.Background()
	readStore := memory.NewReadModelStore()
	if _, err := command.NewHandler(memory.NewEventStore(), readStore).ImportGedcom(ctx, command.ImportGedcomInput{
		Filename: "t.ged", Reader: strings.NewReader(noteReferencesGedcom),
	}); err != nil {
		t.Fatalf("import: %v", err)
	}
	notes, err := repository.ListAll(ctx, 100, readStore.ListNotes)
	if err != nil {
		t.Fatalf("list notes: %v", err)
	}

	want := map[string]string{
		"@N1@":  "association attribute event family name person repository source",
		"@N2@":  "",
		"@N11@": "association person person",
	}
	for i := range notes {
		refs, err := gedcom.NoteReferences(ctx, readStore, domain.MainBranchID, &notes[i])
		if err != nil {
			t.Fatalf("NoteReferences(%s): %v", notes[i].GedcomXref, err)
		}
		types := make([]string, len(refs))
		for j, r := range refs {
			types[j] = r.Type
		}
		if got := strings.Join(types, " "); got != want[notes[i].GedcomXref] {
			t.Errorf("NoteReferences(%s) types = %q, want %q", notes[i].GedcomXref, got, want[notes[i].GedcomXref])
		}
	}
}
