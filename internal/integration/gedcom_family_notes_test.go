package integration_test

import (
	"bytes"
	"context"
	"strings"
	"testing"

	"github.com/cacack/my-family/internal/command"
	"github.com/cacack/my-family/internal/gedcom"
)

// A family record's own NOTEs (issue #1267).
//
// Import keeps a FAM's level-1 NOTEs in FamilyCreated, so replaying the log
// into an empty read model reproduces them, a NOTE pointer follows a renamed
// NOTE record, and an export writes them back on the same FAM.

// familyNotesFirstGedcom gives the tree a @N1@ so the second file's @N1@ is
// renamed on import (#1228).
const familyNotesFirstGedcom = `0 HEAD
1 GEDC
2 VERS 5.5.1
1 CHAR UTF-8
0 @N1@ NOTE An unrelated note already in the tree.
0 @I1@ INDI
1 NAME Ann /Smith/
0 TRLR
`

const familyNotesGedcom = `0 HEAD
1 GEDC
2 VERS 5.5.1
1 CHAR UTF-8
0 @N1@ NOTE From the family bible.
0 @I1@ INDI
1 NAME John /Doe/
1 SEX M
0 @I2@ INDI
1 NAME Mary /Roe/
1 SEX F
0 @F1@ FAM
1 HUSB @I1@
1 WIFE @I2@
1 NOTE @N1@
1 NOTE Lived on the family farm
2 CONT until 1890.
0 TRLR
`

func TestGedcomImport_FamilyNotesSurviveReplayAndExport(t *testing.T) {
	for _, backend := range backends {
		t.Run(backend.name, func(t *testing.T) {
			ctx := context.Background()
			st := backend.setup(t)
			h := command.NewHandlerWithBranches(st.events, st.read, st.branches, st.snapshots)
			for _, data := range []string{familyNotesFirstGedcom, familyNotesGedcom} {
				result, err := h.ImportGedcom(ctx, command.ImportGedcomInput{Filename: "t.ged", Reader: strings.NewReader(data)})
				if err != nil {
					t.Fatalf("import: %v", err)
				}
				for _, w := range result.Warnings {
					if strings.Contains(w, "not imported") {
						t.Errorf("unexpected dropped-note warning: %s", w)
					}
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

			got := familyNoteLines(out)
			if len(got) != 3 || !strings.HasPrefix(got[0], "1 NOTE @") ||
				got[1] != "1 NOTE Lived on the family farm" || got[2] != "2 CONT until 1890." {
				t.Fatalf("FAM NOTE lines = %q, want a pointer then the inline note; export:\n%s", got, out)
			}
			// The pointer names the exported NOTE record of the file's @N1@, not
			// the note the tree already held under that xref.
			pointer := strings.TrimPrefix(got[0], "1 NOTE ")
			if !strings.Contains(out, "0 "+pointer+" NOTE From the family bible.") {
				t.Errorf("FAM NOTE pointer %s does not name the family bible note; export:\n%s", pointer, out)
			}
		})
	}
}

// familyNoteLines returns the level-1 NOTE lines, with their CONT/CONC lines,
// of the FAM records in out.
func familyNoteLines(out string) []string {
	var notes []string
	inFam, inNote := false, false
	for _, line := range strings.Split(out, "\n") {
		line = strings.TrimRight(line, "\r")
		switch {
		case strings.HasPrefix(line, "0 "):
			inFam, inNote = strings.HasSuffix(line, " FAM"), false
		case inFam && strings.HasPrefix(line, "1 "):
			inNote = strings.HasPrefix(line, "1 NOTE")
			if inNote {
				notes = append(notes, line)
			}
		case inNote && (strings.HasPrefix(line, "2 CONT") || strings.HasPrefix(line, "2 CONC")):
			notes = append(notes, line)
		}
	}
	return notes
}
