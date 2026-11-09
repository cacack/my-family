package integration_test

import (
	"bytes"
	"context"
	"slices"
	"strings"
	"testing"

	"github.com/cacack/my-family/internal/command"
	"github.com/cacack/my-family/internal/gedcom"
	"github.com/cacack/my-family/internal/repository"
)

// Marriage place coordinates in the event log (issue #1265).
//
// Import keeps MARR PLAC MAP LATI/LONG in FamilyCreated, as it does person and
// life-event coordinates (#1225), so replaying the log into an empty read
// model reproduces them and an export writes them back under MARR.

const marriageCoordinatesGedcom = `0 HEAD
1 GEDC
2 VERS 5.5.1
1 CHAR UTF-8
0 @I1@ INDI
1 NAME John /Doe/
1 SEX M
0 @I2@ INDI
1 NAME Mary /Roe/
1 SEX F
0 @F1@ FAM
1 HUSB @I1@
1 WIFE @I2@
1 MARR
2 DATE 1875
2 PLAC Springfield
3 MAP
4 LATI N39.7817
4 LONG W89.6501
0 TRLR
`

func TestGedcomImport_MarriageCoordinatesSurviveReplayAndExport(t *testing.T) {
	for _, backend := range backends {
		t.Run(backend.name, func(t *testing.T) {
			ctx := context.Background()
			st := backend.setup(t)
			h := command.NewHandlerWithBranches(st.events, st.read, st.branches, st.snapshots)
			if _, err := h.ImportGedcom(ctx, command.ImportGedcomInput{Filename: "t.ged", Reader: strings.NewReader(marriageCoordinatesGedcom)}); err != nil {
				t.Fatalf("import: %v", err)
			}
			want := []string{"N39.7817", "W89.6501"}
			if got := marriageCoordinates(t, ctx, st.read); !slices.Equal(got, want) {
				t.Fatalf("imported marriage coordinates = %q, want %q", got, want)
			}

			// A rebuild finds them in the log: nothing to backfill, nothing dropped.
			result, err := repository.RebuildReadModel(ctx, st.events, st.read, st.branches, st.snapshots)
			if err != nil || result.Backfilled != 0 || result.DroppedImported != 0 {
				t.Errorf("rebuild = %+v, %v; want nothing backfilled or dropped", result, err)
			}
			if got := marriageCoordinates(t, ctx, st.read); !slices.Equal(got, want) {
				t.Errorf("rebuilt marriage coordinates = %q, want %q", got, want)
			}

			// Replay into an empty read model: the coordinates must come from the log.
			fresh := backend.setup(t).read
			replayLog(t, ctx, st.events, fresh)
			if got := marriageCoordinates(t, ctx, fresh); !slices.Equal(got, want) {
				t.Errorf("replayed marriage coordinates = %q, want %q", got, want)
			}

			var buf bytes.Buffer
			if _, err := gedcom.NewExporter(fresh).Export(ctx, &buf); err != nil {
				t.Fatalf("export: %v", err)
			}
			out := buf.String()
			wantLines := []string{"2 PLAC Springfield", "3 MAP", "4 LATI N39.7817", "4 LONG W89.6501"}
			if got := marriagePlaceLines(out); !slices.Equal(got, wantLines) {
				t.Errorf("PLAC lines under MARR = %q, want %q; export:\n%s", got, wantLines, out)
			}
		})
	}
}

// marriageCoordinates returns the one main-branch family's marriage latitude
// and longitude, with "<nil>" for an unset value.
func marriageCoordinates(t *testing.T, ctx context.Context, rs repository.ReadModelStore) []string {
	t.Helper()
	families, err := repository.ListAll(ctx, 100, rs.ListFamilies)
	if err != nil || len(families) != 1 {
		t.Fatalf("list families = %d, %v; want one", len(families), err)
	}
	got := []string{}
	for _, v := range []*string{families[0].MarriagePlaceLat, families[0].MarriagePlaceLong} {
		if v == nil {
			got = append(got, "<nil>")
		} else {
			got = append(got, *v)
		}
	}
	return got
}

// marriagePlaceLines returns the PLAC line under 1 MARR and the lines nested
// under it, in export order.
func marriagePlaceLines(out string) []string {
	var lines []string
	inMarr, inPlac := false, false
	for _, line := range strings.Split(out, "\n") {
		line = strings.TrimRight(line, "\r")
		switch {
		case strings.HasPrefix(line, "0 ") || strings.HasPrefix(line, "1 "):
			inMarr, inPlac = line == "1 MARR", false
		case inMarr && strings.HasPrefix(line, "2 "):
			inPlac = strings.HasPrefix(line, "2 PLAC")
			if inPlac {
				lines = append(lines, line)
			}
		case inPlac:
			lines = append(lines, line)
		}
	}
	return lines
}
