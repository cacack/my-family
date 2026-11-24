package integration_test

import (
	"context"
	"strings"
	"testing"

	"github.com/cacack/my-family/internal/command"
	"github.com/cacack/my-family/internal/domain"
	"github.com/cacack/my-family/internal/repository"
)

// A person merge keeps a place's imported coordinate pair with the place it
// locates (#1799), and a rebuild reproduces it from the log.

const mergePlaceCoordinatesGedcom = `0 HEAD
1 GEDC
2 VERS 5.5.1
1 CHAR UTF-8
0 @I1@ INDI
1 NAME John /Doe/
1 BIRT
2 PLAC Boston
3 MAP
4 LATI N42.3601
4 LONG W71.0589
1 DEAT
2 PLAC Salem
3 MAP
4 LATI N42.5195
4 LONG W70.8967
0 @I2@ INDI
1 NAME Johnny /Doe/
1 BIRT
2 PLAC Springfield
3 MAP
4 LATI N39.7817
4 LONG W89.6501
1 DEAT
2 PLAC Concord
0 TRLR
`

func TestMergePersons_PlaceCoordinatesFollowResolvedPlace(t *testing.T) {
	for _, backend := range backends {
		t.Run(backend.name, func(t *testing.T) {
			ctx := context.Background()
			st := backend.setup(t)
			h := command.NewHandlerWithBranches(st.events, st.read, st.branches, st.snapshots)
			if _, err := h.ImportGedcom(ctx, command.ImportGedcomInput{Filename: "t.ged", Reader: strings.NewReader(mergePlaceCoordinatesGedcom)}); err != nil {
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
				FieldResolution: map[string]string{"birth_place": "merged", "death_place": "merged"},
			}); err != nil {
				t.Fatalf("merge: %v", err)
			}

			check := func(label string) {
				t.Helper()
				got, err := st.read.GetPerson(ctx, domain.MainBranchID, survivor.ID)
				if err != nil || got == nil {
					t.Fatalf("%s: get survivor: %v, %v", label, got, err)
				}
				text := func(p *string) string {
					if p == nil {
						return ""
					}
					return *p
				}
				birth := [3]string{got.BirthPlace, text(got.BirthPlaceLat), text(got.BirthPlaceLong)}
				if want := [3]string{"Springfield", "N39.7817", "W89.6501"}; birth != want {
					t.Errorf("%s: birth = %q, want %q", label, birth, want)
				}
				death := [3]string{got.DeathPlace, text(got.DeathPlaceLat), text(got.DeathPlaceLong)}
				if want := [3]string{"Concord", "", ""}; death != want {
					t.Errorf("%s: death = %q, want %q", label, death, want)
				}
			}
			check("live")

			result, err := repository.RebuildReadModel(ctx, st.events, st.read, st.branches, st.snapshots)
			if err != nil || result.Backfilled != 0 || result.DroppedImported != 0 {
				t.Errorf("rebuild = %+v, %v; want nothing backfilled or dropped", result, err)
			}
			check("rebuilt")
		})
	}
}
