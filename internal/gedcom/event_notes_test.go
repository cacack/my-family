package gedcom_test

import (
	"bytes"
	"context"
	"fmt"
	"slices"
	"strings"
	"testing"

	"github.com/cacack/my-family/internal/command"
	"github.com/cacack/my-family/internal/gedcom"
	"github.com/cacack/my-family/internal/repository/memory"
)

// importAndExport imports data through the command handler into an in-memory
// store and returns the GEDCOM export of that store.
func importAndExport(t *testing.T, data string) string {
	t.Helper()
	ctx := context.Background()
	readStore := memory.NewReadModelStore()
	if _, err := command.NewHandler(memory.NewEventStore(), readStore).ImportGedcom(ctx, command.ImportGedcomInput{
		Filename: "t.ged", Reader: strings.NewReader(data),
	}); err != nil {
		t.Fatalf("import: %v", err)
	}
	buf := &bytes.Buffer{}
	if _, err := gedcom.NewExporter(readStore).Export(ctx, buf); err != nil {
		t.Fatalf("export: %v", err)
	}
	return buf.String()
}

// subNoteLines returns the level-2 NOTE lines, with their CONT/CONC lines,
// under the level-1 line event of each record that has a line starting with
// marker.
func subNoteLines(out, marker, event string) []string {
	var notes []string
	for _, record := range strings.Split(out, "\n0 ") {
		lines := strings.Split(record, "\n")
		if !slices.ContainsFunc(lines, func(l string) bool { return strings.HasPrefix(l, marker) }) {
			continue
		}
		inEvent, inNote := false, false
		for _, line := range lines {
			switch {
			case strings.HasPrefix(line, "1 "):
				inEvent, inNote = line == event, false
			case inEvent && strings.HasPrefix(line, "2 "):
				inNote = strings.HasPrefix(line, "2 NOTE")
				if inNote {
					notes = append(notes, line)
				}
			case inNote && (strings.HasPrefix(line, "3 CONT") || strings.HasPrefix(line, "3 CONC")):
				notes = append(notes, line)
			}
		}
	}
	return notes
}

const eventNotesGedcom = `0 HEAD
1 GEDC
2 VERS 5.5.1
1 CHAR UTF-8
0 @N1@ NOTE From the death certificate.
0 @I1@ INDI
1 NAME John /Doe/
1 SEX M
1 BIRT
2 NOTE Born at home
1 DEAT
2 DATE 1 JAN 1920
2 NOTE @N1@
2 NOTE Died of a fever,
3 CONT after a week in bed.
1 BURI
2 PLAC Salem
2 NOTE Buried beside his wife
0 @I2@ INDI
1 NAME Mary /Roe/
1 SEX F
0 @F1@ FAM
1 HUSB @I1@
1 WIFE @I2@
1 MARR
2 DATE 1875
2 NOTE Married in the parlour
1 DIV
2 NOTE @N1@
0 TRLR
`

func TestEventNotes_RoundTrip(t *testing.T) {
	// A NOTE under BIRT, DEAT, a life event and a family event is kept on that
	// event and written back under it; a BIRT with only a NOTE still exports
	// (#1250).
	first := importAndExport(t, eventNotesGedcom)
	// The export re-imports to the same notes: the round trip is stable.
	for i, out := range []string{first, importAndExport(t, first)} {
		for _, tc := range []struct {
			marker, event string
			want          []string
		}{
			{"1 NAME John /Doe/", "1 BIRT", []string{"2 NOTE Born at home"}},
			{"1 NAME John /Doe/", "1 DEAT", []string{"2 NOTE @N1@", "2 NOTE Died of a fever,", "3 CONT after a week in bed."}},
			{"1 NAME John /Doe/", "1 BURI", []string{"2 NOTE Buried beside his wife"}},
			{"1 HUSB ", "1 MARR", []string{"2 NOTE Married in the parlour"}},
			{"1 HUSB ", "1 DIV", []string{"2 NOTE @N1@"}},
		} {
			if got := subNoteLines(out, tc.marker, tc.event); !slices.Equal(got, tc.want) {
				t.Errorf("export %d: NOTE lines under %q = %q, want %q; export:\n%s", i+1, tc.event, got, tc.want, out)
			}
		}
		if !strings.Contains(out, "0 @N1@ NOTE From the death certificate.") {
			t.Errorf("export %d should keep the NOTE record the event notes point to; got:\n%s", i+1, out)
		}
		// No event note leaks into a record's own notes.
		if strings.Contains(out, "\n1 NOTE") {
			t.Errorf("export %d has a record-level NOTE; got:\n%s", i+1, out)
		}
	}
}

func TestEventNotes_NegatedEventRoundTrip(t *testing.T) {
	// A NOTE under a 7.0 negative assertion is kept on its life event.
	const data = `0 HEAD
1 GEDC
2 VERS 7.0
0 @I1@ INDI
1 NAME John /Doe/
0 @I2@ INDI
1 NAME Mary /Roe/
0 @F1@ FAM
1 HUSB @I1@
1 WIFE @I2@
1 NO MARR
2 NOTE Never married, per the family bible
0 TRLR
`
	out := importAndExport(t, data)
	if got, want := subNoteLines(out, "1 HUSB ", "1 NO MARR"), []string{"2 NOTE Never married, per the family bible"}; !slices.Equal(got, want) {
		t.Errorf("NOTE lines under NO MARR = %q, want %q; export:\n%s", got, want, out)
	}
}

func TestEventNotes_FollowRenamedNoteXref(t *testing.T) {
	// When the tree already holds @N1@, the file's @N1@ is renamed and every
	// event NOTE pointer to it follows (#1228, #1250).
	ctx := context.Background()
	_, persons, families, _, _, _, events, _, notes, _, _, _, _, err := gedcom.NewImporter().ImportWithOptions(ctx,
		strings.NewReader(eventNotesGedcom), gedcom.ImportOptions{ExistingXrefs: []string{"@N1@"}})
	if err != nil {
		t.Fatalf("import: %v", err)
	}
	if len(notes) != 1 || notes[0].GedcomXref == "@N1@" {
		t.Fatalf("notes = %+v, want the one note renamed", notes)
	}
	renamed := notes[0].GedcomXref
	var john gedcom.PersonData
	for _, p := range persons {
		if p.GivenName == "John" {
			john = p
		}
	}
	if want := renamed + "\n\nDied of a fever,\nafter a week in bed."; john.DeathNotes != want {
		t.Errorf("DeathNotes = %q, want %q", john.DeathNotes, want)
	}
	if john.BirthNotes != "Born at home" {
		t.Errorf("BirthNotes = %q, want %q", john.BirthNotes, "Born at home")
	}
	if len(families) != 1 || families[0].MarriageNotes != "Married in the parlour" {
		t.Errorf("families = %+v, want MarriageNotes %q", families, "Married in the parlour")
	}
	var divorceNotes []string
	for _, e := range events {
		if e.FactType == "family_divorce" {
			divorceNotes = append(divorceNotes, e.Notes)
		}
	}
	if !slices.Equal(divorceNotes, []string{renamed}) {
		t.Errorf("divorce event notes = %q, want [%q]", divorceNotes, renamed)
	}
}

// droppedNoteWarnings imports data and returns its dropped-note warnings and
// persons.
func droppedNoteWarnings(t *testing.T, data string) ([]string, []gedcom.PersonData) {
	t.Helper()
	result, persons, _, _, _, _, _, _, _, _, _, _, _, err := gedcom.NewImporter().Import(context.Background(), strings.NewReader(data))
	if err != nil {
		t.Fatalf("import: %v", err)
	}
	var got []string
	for _, w := range result.Warnings {
		if strings.Contains(w, "not imported") {
			got = append(got, w)
		}
	}
	return got, persons
}

func TestImport_WarnsOnDroppedNotes(t *testing.T) {
	// A NOTE the import has nowhere to keep is reported, naming the record,
	// the substructure and the note; the notes it keeps are not (#1250).
	const data = `0 HEAD
1 GEDC
2 VERS 5.5.1
1 CHAR UTF-8
0 @R1@ REPO
1 NAME State Archive
1 ADDR 1 Main St
2 CITY Boston
2 NOTE Entrance on the side street
0 @S1@ SOUR
1 TITL Census 1880
1 REPO @R1@
2 CALN 42
2 NOTE Microfilm only
0 @I1@ INDI
1 NAME John /Doe/
2 NOTE Spelling varies
1 BIRT
2 DATE 1849
2 NOTE An earlier claim
1 BIRT
2 DATE 1850
2 NOTE Born at home
2 SOUR @S1@
3 NOTE Page is torn
1 ASSO @I2@
2 RELA Godfather
2 NOTE Stood in by proxy
1 NOTE A record note
0 @I2@ INDI
1 NAME Mary /Roe/
0 @F1@ FAM
1 HUSB @I1@
1 WIFE @I2@
1 MARR
2 NOTE Married in the parlour
1 NOTE Lived on the farm
0 @M1@ OBJE
1 FILE photo.jpg
2 FORM jpg
1 NOTE A family photo
0 @U1@ SUBM
1 NAME Ann Smith
1 NOTE Contact by mail
0 TRLR
`
	got, persons := droppedNoteWarnings(t, data)
	want := []string{
		`Repository @R1@: NOTE under ADDR not imported: "Entrance on the side street"`,
		`Source @S1@: NOTE under REPO not imported: "Microfilm only"`,
		`Individual @I1@: NOTE under BIRT not imported: "An earlier claim"`,
		`Individual @I1@: NOTE under BIRT.SOUR not imported: "Page is torn"`,
		`Family @F1@: NOTE not imported: "Lived on the farm"`,
		`Media @M1@: NOTE not imported: "A family photo"`,
		`Submitter @U1@: NOTE not imported: "Contact by mail"`,
	}
	if !slices.Equal(got, want) {
		t.Errorf("dropped-note warnings =\n%q\nwant\n%q", got, want)
	}

	// Of two BIRT blocks the last supplies the date, place and notes.
	for _, p := range persons {
		if p.GivenName == "John" && (p.BirthDate != "1850" || p.BirthNotes != "Born at home") {
			t.Errorf("John's birth = %q, notes %q; want 1850, %q", p.BirthDate, p.BirthNotes, "Born at home")
		}
	}
}

func TestImport_CapsDroppedNoteWarningsPerPath(t *testing.T) {
	// Past five dropped notes under the same record type and path, the rest
	// are counted in one summary warning (#1250).
	var b strings.Builder
	b.WriteString("0 HEAD\n1 GEDC\n2 VERS 5.5.1\n1 CHAR UTF-8\n0 @S1@ SOUR\n1 TITL Census\n")
	for i := 1; i <= 7; i++ {
		fmt.Fprintf(&b, "0 @I%d@ INDI\n1 NAME P%d /Doe/\n1 BIRT\n2 SOUR @S1@\n3 NOTE Citation note %d\n", i, i, i)
	}
	b.WriteString("0 @I8@ INDI\n1 NAME P8 /Doe/\n2 SOUR @S1@\n3 NOTE Name note\n0 TRLR\n")

	got, _ := droppedNoteWarnings(t, b.String())
	var want []string
	for i := 1; i <= 5; i++ {
		want = append(want, fmt.Sprintf(`Individual @I%d@: NOTE under BIRT.SOUR not imported: "Citation note %d"`, i, i))
	}
	want = append(want,
		`Individual @I8@: NOTE under NAME.SOUR not imported: "Name note"`,
		"2 more notes under INDI.BIRT.SOUR not imported",
	)
	if !slices.Equal(got, want) {
		t.Errorf("dropped-note warnings =\n%q\nwant\n%q", got, want)
	}
}
