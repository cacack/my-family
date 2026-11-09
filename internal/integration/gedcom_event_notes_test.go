package integration_test

import (
	"bytes"
	"context"
	"slices"
	"strings"
	"testing"

	"github.com/cacack/my-family/internal/command"
	"github.com/cacack/my-family/internal/gedcom"
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
