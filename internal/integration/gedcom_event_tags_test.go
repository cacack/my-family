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

// Individual event tags and EVEN descriptors (issue #1276).
//
// Import keeps each named individual event tag as its own fact type, and an
// event's TYPE and an EVEN's descriptor in LifeEventCreated, so replaying the
// log into an empty read model reproduces them and an export writes them back.

const eventTagsGedcom = `0 HEAD
1 GEDC
2 VERS 5.5.1
1 CHAR UTF-8
0 @I1@ INDI
1 NAME John /Doe/
1 SEX M
1 ADOP
2 DATE 1851
1 BARM
2 DATE 1863
1 BASM
2 DATE 1864
1 BLES
2 DATE 1865
1 CHRA
2 DATE 1870
1 CONF
2 DATE 1866
1 FCOM
2 DATE 1867
1 GRAD
2 DATE 1900
1 RETI
2 TYPE Early retirement
2 DATE 1910
1 ORDN
2 DATE 1880
1 PROB
2 DATE 1921
1 WILL
2 DATE 1919
1 EVEN Moved west
2 TYPE Migration
2 DATE 1890
0 TRLR
`

func TestGedcomImport_EventTagsSurviveReplayAndExport(t *testing.T) {
	for _, backend := range backends {
		t.Run(backend.name, func(t *testing.T) {
			ctx := context.Background()
			st := backend.setup(t)
			h := command.NewHandlerWithBranches(st.events, st.read, st.branches, st.snapshots)
			if _, err := h.ImportGedcom(ctx, command.ImportGedcomInput{Filename: "t.ged", Reader: strings.NewReader(eventTagsGedcom)}); err != nil {
				t.Fatalf("import: %v", err)
			}

			// Replay into an empty read model: the tags must come from the log.
			fresh := backend.setup(t).read
			replayLog(t, ctx, st.events, fresh)

			var buf bytes.Buffer
			if _, err := gedcom.NewExporter(fresh).Export(ctx, &buf); err != nil {
				t.Fatalf("export: %v", err)
			}
			out := buf.String()

			for _, tc := range []struct {
				event string
				want  []string
			}{
				{"1 ADOP", []string{"2 DATE 1851"}},
				{"1 BARM", []string{"2 DATE 1863"}},
				{"1 BASM", []string{"2 DATE 1864"}},
				{"1 BLES", []string{"2 DATE 1865"}},
				{"1 CHRA", []string{"2 DATE 1870"}},
				{"1 CONF", []string{"2 DATE 1866"}},
				{"1 FCOM", []string{"2 DATE 1867"}},
				{"1 GRAD", []string{"2 DATE 1900"}},
				{"1 RETI", []string{"2 DATE 1910", "2 TYPE Early retirement"}},
				{"1 ORDN", []string{"2 DATE 1880"}},
				{"1 PROB", []string{"2 DATE 1921"}},
				{"1 WILL", []string{"2 DATE 1919"}},
				{"1 EVEN Moved west", []string{"2 DATE 1890", "2 TYPE Migration"}},
			} {
				got, ok := eventBlockLines(out, tc.event)
				if !ok {
					t.Errorf("no %q line in export:\n%s", tc.event, out)
					continue
				}
				for _, line := range tc.want {
					if !slices.Contains(got, line) {
						t.Errorf("lines under %q = %q, want %q among them; export:\n%s", tc.event, got, line, out)
					}
				}
			}
			if _, ok := eventBlockLines(out, "1 EVEN"); ok {
				t.Errorf("export has a bare 1 EVEN:\n%s", out)
			}
		})
	}
}

// eventBlockLines returns the level-2 lines under the first line equal to
// eventLine, and whether that line was found.
func eventBlockLines(out, eventLine string) ([]string, bool) {
	var block []string
	found, in := false, false
	for _, line := range strings.Split(out, "\n") {
		line = strings.TrimRight(line, "\r")
		switch {
		case line == eventLine && !found:
			found, in = true, true
		case in && strings.HasPrefix(line, "2 "):
			block = append(block, line)
		case in && (strings.HasPrefix(line, "0 ") || strings.HasPrefix(line, "1 ")):
			in = false
		}
	}
	return block, found
}
