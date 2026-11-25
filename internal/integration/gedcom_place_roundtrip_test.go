package integration_test

import (
	"context"
	"fmt"
	"slices"
	"strings"
	"testing"

	"github.com/cacack/my-family/internal/command"
	"github.com/cacack/my-family/internal/domain"
	"github.com/cacack/my-family/internal/repository"
)

// placeRoundTripFile has a place mapped only once it is given a pair (Boston,
// in two spellings), a record with a pair of its own there, a place with a
// form and EXIDs (Darmstadt, in two part layouts) and a place with no pair.
const placeRoundTripFile = `0 HEAD
1 GEDC
2 VERS 7.0
0 @I1@ INDI
1 NAME Ann /Roe/
1 BIRT
2 DATE 1801
2 PLAC Boston, MA
1 DEAT
2 DATE 1870
2 PLAC Darmstadt,,,Germany
3 FORM City, County, State, Country
3 EXID Q2973
4 TYPE https://www.wikidata.org/entity
0 @I2@ INDI
1 NAME Bob /Roe/
1 BIRT
2 DATE 1803
2 PLAC boston,ma
1 DEAT
2 DATE 1871
2 PLAC Boston, MA
3 MAP
4 LATI N42.3
4 LONG W71.1
0 @I3@ INDI
1 NAME Cy /Roe/
1 BIRT
2 DATE 1805
2 PLAC Darmstadt, Germany
3 MAP
4 LATI N49.87
4 LONG E8.65
1 DEAT
2 DATE 1880
2 PLAC Salem, MA
0 TRLR
`

// A file imported, given a pair on one place, exported and imported again
// shows the same map: the pair set once on the place is written under every
// event that inherits it, and each re-imported record holds it as its own.
// Every PLAC line is written as the file had it, and the form and EXIDs come
// back (ADR-009 §8, #1738).
func TestGedcomRoundTrip_PlacePairsFormsAndExternalIDs(t *testing.T) {
	for _, backend := range backends {
		t.Run(backend.name, func(t *testing.T) {
			ctx := context.Background()
			first := backend.setup(t)
			importInto(t, ctx, first, []byte(placeRoundTripFile))
			h := command.NewHandlerWithBranches(first.events, first.read, first.branches, first.snapshots)
			if _, err := h.UpdatePlace(ctx, command.UpdatePlaceInput{ID: repository.PlaceID("boston,ma"),
				Coordinates: command.CoordinatesPatch{Latitude: command.Set("N42.36"), Longitude: command.Set("W71.06")}}); err != nil {
				t.Fatalf("UpdatePlace: %v", err)
			}
			want := mapPins(t, ctx, first.read)
			if !slices.Contains(want, "Boston, MA birth 42.36,-71.06 [Ann Bob]") {
				t.Fatalf("map before export = %q, want Ann and Bob at Boston's pair", want)
			}

			out := exportAt(t, ctx, first.read, "")
			if got, wantLines := placLines(out), placLines(placeRoundTripFile); !slices.Equal(got, wantLines) {
				t.Errorf("exported PLAC lines = %q, want %q", got, wantLines)
			}
			if !strings.Contains(out, "2 PLAC Darmstadt,,,Germany\n3 FORM City, County, State, Country\n") {
				t.Errorf("export lacks Darmstadt's form:\n%s", out)
			}

			second := backend.setup(t)
			importInto(t, ctx, second, []byte(out))
			if got := mapPins(t, ctx, second.read); !slices.Equal(got, want) {
				t.Errorf("map after round trip = %q\nwant %q", got, want)
			}
			darmstadt := repository.PlaceID("darmstadt,germany")
			forms, err := second.read.ListPlaceForms(ctx)
			if err != nil {
				t.Fatal(err)
			}
			if f := forms[darmstadt]; len(f) != 1 || f[0].PositionalKey != "darmstadt,,,germany" || f[0].Form != "City, County, State, Country" {
				t.Errorf("re-imported Darmstadt forms = %+v", f)
			}
			ids, err := second.read.ListPlaceExternalIDs(ctx)
			if err != nil {
				t.Fatal(err)
			}
			if x := ids[darmstadt]; len(x) != 1 || x[0].Value != "Q2973" || x[0].Type != "https://www.wikidata.org/entity" {
				t.Errorf("re-imported Darmstadt external IDs = %+v", x)
			}
		})
	}
}

// mapPins describes each map location as "place event lat,long [given
// names]", in the map's order, leaving out IDs and origins, which a round
// trip changes.
func mapPins(t *testing.T, ctx context.Context, rs repository.ReadModelStore) []string {
	t.Helper()
	locations, err := rs.GetMapLocations(ctx, domain.MainBranchID, repository.Privacy{})
	if err != nil {
		t.Fatalf("GetMapLocations: %v", err)
	}
	pins := make([]string, 0, len(locations))
	for _, l := range locations {
		names := make([]string, 0, len(l.Persons))
		for _, p := range l.Persons {
			names = append(names, p.GivenName)
		}
		slices.Sort(names)
		pins = append(pins, fmt.Sprintf("%s %s %g,%g %v", l.Place, l.EventType, l.Latitude, l.Longitude, names))
	}
	return pins
}

// placLines returns the file's PLAC lines, sorted.
func placLines(data string) []string {
	var lines []string
	for _, line := range strings.Split(data, "\n") {
		if strings.HasPrefix(line, "2 PLAC") {
			lines = append(lines, line)
		}
	}
	slices.Sort(lines)
	return lines
}
