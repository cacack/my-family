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

// Person attributes and their DATE/PLAC in the event log (issue #1275).
//
// Import reads attributes from the level-1 lines only, so a TITL under OBJE or
// a RELI under BAPM is not a person attribute, and an attribute's DATE and
// PLAC travel in AttributeCreated: replaying the log into an empty read model
// reproduces them and an export writes them back under the attribute.

const attributeSubordinatesGedcom = `0 HEAD
1 GEDC
2 VERS 5.5.1
1 CHAR UTF-8
0 @I1@ INDI
1 NAME A /B/
1 OBJE
2 FILE x.jpg
2 TITL Portrait
1 BAPM
2 RELI Catholic
1 OCCU Blacksmith
2 DATE 1860
2 PLAC Springfield
0 TRLR
`

func TestGedcomImport_AttributeSubordinatesSurviveReplayAndExport(t *testing.T) {
	for _, backend := range backends {
		t.Run(backend.name, func(t *testing.T) {
			ctx := context.Background()
			st := backend.setup(t)
			h := command.NewHandlerWithBranches(st.events, st.read, st.branches, st.snapshots)
			if _, err := h.ImportGedcom(ctx, command.ImportGedcomInput{Filename: "t.ged", Reader: strings.NewReader(attributeSubordinatesGedcom)}); err != nil {
				t.Fatalf("import: %v", err)
			}
			want := []string{"person_occupation|Blacksmith|1860|Springfield"}
			if got := attributeSummaries(t, ctx, st.read); !slices.Equal(got, want) {
				t.Fatalf("imported attributes = %q, want %q", got, want)
			}

			// Replay into an empty read model: date and place must come from the log.
			fresh := backend.setup(t).read
			replayLog(t, ctx, st.events, fresh)
			if got := attributeSummaries(t, ctx, fresh); !slices.Equal(got, want) {
				t.Errorf("replayed attributes = %q, want %q", got, want)
			}

			var buf bytes.Buffer
			if _, err := gedcom.NewExporter(fresh).Export(ctx, &buf); err != nil {
				t.Fatalf("export: %v", err)
			}
			out := buf.String()
			wantLines := []string{"1 OCCU Blacksmith", "2 DATE 1860", "2 PLAC Springfield"}
			if got := attributeLines(out, "1 OCCU"); !slices.Equal(got, wantLines) {
				t.Errorf("OCCU lines = %q, want %q; export:\n%s", got, wantLines, out)
			}
			for _, bad := range []string{"1 TITL", "1 RELI"} {
				if strings.Contains(out, bad) {
					t.Errorf("export contains %q from a nested tag; export:\n%s", bad, out)
				}
			}
		})
	}
}

// attributeSummaries returns each main-branch attribute as
// "fact_type|value|date|place".
func attributeSummaries(t *testing.T, ctx context.Context, rs repository.ReadModelStore) []string {
	t.Helper()
	persons, err := repository.ListAll(ctx, 100, rs.ListPersons)
	if err != nil || len(persons) != 1 {
		t.Fatalf("list persons = %d, %v; want one", len(persons), err)
	}
	attrs, err := rs.ListAttributesForPerson(ctx, domain.MainBranchID, persons[0].ID)
	if err != nil {
		t.Fatalf("list attributes: %v", err)
	}
	got := []string{}
	for _, a := range attrs {
		got = append(got, strings.Join([]string{string(a.FactType), a.Value, a.DateRaw, a.Place}, "|"))
	}
	return got
}

// attributeLines returns the line starting with head and the level-2 lines
// nested under it, in export order.
func attributeLines(out, head string) []string {
	var lines []string
	in := false
	for _, line := range strings.Split(out, "\n") {
		line = strings.TrimRight(line, "\r")
		switch {
		case strings.HasPrefix(line, "0 ") || strings.HasPrefix(line, "1 "):
			in = strings.HasPrefix(line, head)
			if in {
				lines = append(lines, line)
			}
		case in && strings.HasPrefix(line, "2 "):
			lines = append(lines, line)
		}
	}
	return lines
}
