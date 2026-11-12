package gedcom_test

import (
	"context"
	"slices"
	"strings"
	"testing"

	"github.com/cacack/my-family/internal/gedcom"
)

// A family record's own NOTEs (issue #1267).

const familyNotesGedcom = `0 HEAD
1 GEDC
2 VERS 5.5.1
1 CHAR UTF-8
0 @N1@ NOTE From the family bible.
0 @I1@ INDI
1 NAME John /Doe/
0 @I2@ INDI
1 NAME Mary /Roe/
0 @F1@ FAM
1 HUSB @I1@
1 WIFE @I2@
1 MARR
2 NOTE Married in the parlour
1 NOTE @N1@
1 NOTE Lived on the family farm
2 CONT until 1890.
0 TRLR
`

// recordNoteLines returns the level-1 NOTE lines, with their CONT/CONC lines,
// of each record that has a line starting with marker.
func recordNoteLines(out, marker string) []string {
	var notes []string
	for _, record := range strings.Split(out, "\n0 ") {
		lines := strings.Split(record, "\n")
		if !slices.ContainsFunc(lines, func(l string) bool { return strings.HasPrefix(l, marker) }) {
			continue
		}
		inNote := false
		for _, line := range lines {
			switch {
			case strings.HasPrefix(line, "1 "):
				inNote = strings.HasPrefix(line, "1 NOTE")
				if inNote {
					notes = append(notes, line)
				}
			case inNote && (strings.HasPrefix(line, "2 CONT") || strings.HasPrefix(line, "2 CONC")):
				notes = append(notes, line)
			}
		}
	}
	return notes
}

func TestFamilyNotes_RoundTrip(t *testing.T) {
	// A FAM's own NOTEs, pointer and text, are written back on the FAM and do
	// not mix with the MARR notes; the round trip is stable. (The encoder writes
	// pointers before inline text, so the fixture lists them in that order.)
	first := importAndExport(t, familyNotesGedcom)
	for i, out := range []string{first, importAndExport(t, first)} {
		want := []string{"1 NOTE @N1@", "1 NOTE Lived on the family farm", "2 CONT until 1890."}
		if got := recordNoteLines(out, "1 HUSB "); !slices.Equal(got, want) {
			t.Errorf("export %d: FAM NOTE lines = %q, want %q; export:\n%s", i+1, got, want, out)
		}
		if got, want := subNoteLines(out, "1 HUSB ", "1 MARR"), []string{"2 NOTE Married in the parlour"}; !slices.Equal(got, want) {
			t.Errorf("export %d: MARR NOTE lines = %q, want %q", i+1, got, want)
		}
		if !strings.Contains(out, "0 @N1@ NOTE From the family bible.") {
			t.Errorf("export %d should keep the NOTE record the family points to; got:\n%s", i+1, out)
		}
	}
}

func TestFamilyNotes_FollowRenamedNoteXref(t *testing.T) {
	// When the tree already holds @N1@, the file's @N1@ is renamed and the
	// family's NOTE pointer follows (#1228); the FAM NOTEs raise no warning.
	result, _, families, _, _, _, _, _, notes, _, _, _, _, err := gedcom.NewImporter().ImportWithOptions(context.Background(),
		strings.NewReader(familyNotesGedcom), gedcom.ImportOptions{ExistingXrefs: []string{"@N1@"}})
	if err != nil {
		t.Fatalf("import: %v", err)
	}
	if len(notes) != 1 || notes[0].GedcomXref == "@N1@" {
		t.Fatalf("notes = %+v, want the one note renamed", notes)
	}
	want := notes[0].GedcomXref + "\n\nLived on the family farm\nuntil 1890."
	if len(families) != 1 || families[0].Notes != want {
		t.Errorf("families = %+v, want Notes %q", families, want)
	}
	for _, w := range result.Warnings {
		if strings.Contains(w, "not imported") {
			t.Errorf("unexpected dropped-note warning: %s", w)
		}
	}
}
