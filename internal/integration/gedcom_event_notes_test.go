package integration_test

import (
	"bytes"
	"context"
	"slices"
	"strings"
	"testing"

	"github.com/cacack/my-family/internal/command"
	"github.com/cacack/my-family/internal/domain"
	"github.com/cacack/my-family/internal/gedcom"
	"github.com/cacack/my-family/internal/repository"
)

// NOTEs nested under events (issue #1250).
//
// Import keeps a NOTE under BIRT, DEAT, MARR and a life event in the entity's
// created event, so replaying the log into an empty read model reproduces it
// and an export writes it back under the same event.

const eventNotesGedcom = `0 HEAD
1 GEDC
2 VERS 5.5.1
1 CHAR UTF-8
0 @N1@ NOTE Death certificate transcribed.
0 @I1@ INDI
1 NAME John /Doe/
1 SEX M
1 BIRT
2 DATE 1 JAN 1850
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
1 ENGA
2 NOTE Engaged at Easter
0 TRLR
`

func TestGedcomImport_EventNotesSurviveReplayAndExport(t *testing.T) {
	for _, backend := range backends {
		t.Run(backend.name, func(t *testing.T) {
			ctx := context.Background()
			st := backend.setup(t)
			h := command.NewHandlerWithBranches(st.events, st.read, st.branches, st.snapshots)
			result, err := h.ImportGedcom(ctx, command.ImportGedcomInput{Filename: "t.ged", Reader: strings.NewReader(eventNotesGedcom)})
			if err != nil {
				t.Fatalf("import: %v", err)
			}
			for _, w := range result.Warnings {
				if strings.Contains(w, "not imported") {
					t.Errorf("unexpected dropped-note warning: %s", w)
				}
			}

			// Replay into an empty read model: the notes must come from the log.
			fresh := backend.setup(t).read
			replayLog(t, ctx, st.events, fresh)

			var buf bytes.Buffer
			if _, err := gedcom.NewExporter(fresh).Export(ctx, &buf); err != nil {
				t.Fatalf("export: %v", err)
			}
			out := buf.String()

			for _, tc := range []struct {
				record, event string
				want          []string
			}{
				{"1 NAME John /Doe/", "BIRT", []string{"2 NOTE Born at home"}},
				{"1 NAME John /Doe/", "DEAT", []string{"2 NOTE @N1@", "2 NOTE Died of a fever,", "3 CONT after a week in bed."}},
				{"1 NAME John /Doe/", "BURI", []string{"2 NOTE Buried beside his wife"}},
				{"1 HUSB ", "MARR", []string{"2 NOTE Married in the parlour"}},
				{"1 HUSB ", "ENGA", []string{"2 NOTE Engaged at Easter"}},
			} {
				if got := eventNoteLines(out, tc.record, tc.event); !slices.Equal(got, tc.want) {
					t.Errorf("NOTE lines under %s of the record with %q = %q, want %q; export:\n%s", tc.event, tc.record, got, tc.want, out)
				}
			}
		})
	}
}

// eventNoteLines returns the NOTE lines (and their CONT/CONC lines) directly
// under the level-1 event tag of the record that has a line starting with
// recordLine, in export order.
func eventNoteLines(out, recordLine, event string) []string {
	var record [][]string
	var current []string
	for _, line := range strings.Split(out, "\n") {
		line = strings.TrimRight(line, "\r")
		if strings.HasPrefix(line, "0 ") {
			record = append(record, current)
			current = nil
		}
		current = append(current, line)
	}
	record = append(record, current)

	var notes []string
	for _, lines := range record {
		if !slices.ContainsFunc(lines, func(l string) bool { return strings.HasPrefix(l, recordLine) }) {
			continue
		}
		inEvent, inNote := false, false
		for _, line := range lines {
			switch {
			case strings.HasPrefix(line, "1 "):
				inEvent = line == "1 "+event
				inNote = false
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

const mergeNotesGedcom = `0 HEAD
1 GEDC
2 VERS 5.5.1
1 CHAR UTF-8
0 @I1@ INDI
1 NAME John /Doe/
1 BIRT
2 DATE 1850
2 NOTE Born at home
0 @I2@ INDI
1 NAME Johnny /Doe/
1 BIRT
2 DATE 1851
2 NOTE Baptised the same week
1 DEAT
2 NOTE Died abroad
0 TRLR
`

func TestMergePersons_KeepsBothPersonsEventNotes(t *testing.T) {
	// A merge adds the merged person's birth and death notes to the
	// survivor's, even when the birth date resolves to the merged person, and
	// a replay of the log reproduces the result (#1250).
	for _, backend := range backends {
		t.Run(backend.name, func(t *testing.T) {
			ctx := context.Background()
			st := backend.setup(t)
			h := command.NewHandlerWithBranches(st.events, st.read, st.branches, st.snapshots)
			if _, err := h.ImportGedcom(ctx, command.ImportGedcomInput{Filename: "t.ged", Reader: strings.NewReader(mergeNotesGedcom)}); err != nil {
				t.Fatalf("import: %v", err)
			}
			persons, err := repository.ListAll(ctx, 100, st.read.ListPersons)
			if err != nil {
				t.Fatal(err)
			}
			byName := map[string]repository.PersonReadModel{}
			for _, p := range persons {
				byName[p.GivenName] = p
			}
			survivor, merged := byName["John"], byName["Johnny"]
			if _, err := h.MergePersons(ctx, command.MergePersonsInput{
				SurvivorID: survivor.ID, MergedID: merged.ID,
				SurvivorVersion: survivor.Version, MergedVersion: merged.Version,
				FieldResolution: map[string]string{"birth_date": "merged"},
			}); err != nil {
				t.Fatalf("merge: %v", err)
			}

			fresh := backend.setup(t).read
			replayLog(t, ctx, st.events, fresh)
			for name, rs := range map[string]repository.ReadModelStore{"live": st.read, "replayed": fresh} {
				got, err := rs.GetPerson(ctx, domain.MainBranchID, survivor.ID)
				if err != nil || got == nil {
					t.Fatalf("%s: get survivor: %v, %v", name, got, err)
				}
				if want := "Born at home\n\nBaptised the same week"; got.BirthNotes != want || got.BirthDateRaw != "1851" {
					t.Errorf("%s: survivor birth = %q, notes %q; want 1851, %q", name, got.BirthDateRaw, got.BirthNotes, want)
				}
				if got.DeathNotes != "Died abroad" {
					t.Errorf("%s: survivor DeathNotes = %q, want %q", name, got.DeathNotes, "Died abroad")
				}
			}
		})
	}
}
