package integration_test

import (
	"bytes"
	"context"
	"os"
	"slices"
	"strings"
	"testing"

	"github.com/cacack/my-family/internal/command"
	"github.com/cacack/my-family/internal/domain"
	"github.com/cacack/my-family/internal/gedcom"
	"github.com/cacack/my-family/internal/repository"
)

// The SOURs and NOTEs under each NAME are imported against that name and
// exported under it again on every backend, after a rebuild and a replay
// (#1165).
func TestGedcomRoundTrip_NameSourcesAndNotes(t *testing.T) {
	data, err := os.ReadFile("../../testdata/gedcom-5.5/name-sources-notes.ged")
	if err != nil {
		t.Fatalf("read fixture: %v", err)
	}
	in := string(data)
	want := nameSubLines(in)

	for _, backend := range backends {
		t.Run(backend.name, func(t *testing.T) {
			ctx := context.Background()
			st := backend.setup(t)
			h := command.NewHandlerWithBranches(st.events, st.read, st.branches, st.snapshots)
			if _, err := h.ImportGedcom(ctx, command.ImportGedcomInput{Filename: "t.ged", Reader: strings.NewReader(in)}); err != nil {
				t.Fatalf("import: %v", err)
			}
			check := func(stage string, rs repository.ReadModelStore) {
				t.Helper()
				var buf bytes.Buffer
				if _, err := gedcom.NewExporter(rs).Export(ctx, &buf); err != nil {
					t.Fatalf("%s: export: %v", stage, err)
				}
				got := nameSubLines(buf.String())
				if len(got) != len(want) {
					t.Fatalf("%s: NAME lines = %d, want %d; export:\n%s", stage, len(got), len(want), buf.String())
				}
				for name, lines := range want {
					if !slices.Equal(got[name], lines) {
						t.Errorf("%s: %s: SOUR/NOTE lines = %q, want %q", stage, name, got[name], lines)
					}
				}

				persons, _, err := rs.ListPersons(ctx, repository.ListOptions{Limit: 10})
				if err != nil || len(persons) != 1 {
					t.Fatalf("%s: persons = %d, %v; want 1", stage, len(persons), err)
				}
				names, err := rs.GetPersonNames(ctx, domain.MainBranchID, persons[0].ID)
				if err != nil {
					t.Fatalf("%s: get names: %v", stage, err)
				}
				var notes []string
				for _, n := range names {
					notes = append(notes, n.Surname+": "+n.Notes)
				}
				slices.Sort(notes)
				if wantNotes := []string{"de Vermandois: Spelling varies.", "of Vermandois: @N1@\n\nUsed in the charter."}; !slices.Equal(notes, wantNotes) {
					t.Errorf("%s: name notes = %q, want %q", stage, notes, wantNotes)
				}
			}
			check("imported", st.read)

			if _, err := repository.RebuildReadModel(ctx, st.events, st.read, st.branches, st.snapshots); err != nil {
				t.Fatalf("rebuild: %v", err)
			}
			check("rebuilt", st.read)

			fresh := backend.setup(t).read
			replayLog(t, ctx, st.events, fresh)
			check("replayed", fresh)
		})
	}
}

// nameSubLines maps each "1 NAME" line of out to its SOUR and NOTE lines,
// with their own subordinate lines, in file order.
func nameSubLines(out string) map[string][]string {
	blocks := map[string][]string{}
	name, keep := "", false
	for _, line := range strings.Split(strings.ReplaceAll(out, "\r\n", "\n"), "\n") {
		switch {
		case strings.HasPrefix(line, "0 ") || strings.HasPrefix(line, "1 "):
			name, keep = "", false
			if strings.HasPrefix(line, "1 NAME ") {
				name = line
				blocks[name] = []string{}
			}
		case name == "":
		case strings.HasPrefix(line, "2 "):
			keep = strings.HasPrefix(line, "2 SOUR ") || strings.HasPrefix(line, "2 NOTE ")
			if keep {
				blocks[name] = append(blocks[name], line)
			}
		case keep:
			blocks[name] = append(blocks[name], line)
		}
	}
	return blocks
}
