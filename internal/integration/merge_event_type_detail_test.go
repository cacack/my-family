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

// A person merge keeps the GEDCOM TYPE under BIRT and DEAT with the date and
// place it describes (#1352).

const mergeTypeDetailGedcom = `0 HEAD
1 GEDC
2 VERS 5.5.1
1 CHAR UTF-8
0 @I1@ INDI
1 NAME John /Doe/
1 BIRT
2 TYPE Hospital birth
2 DATE 1850
1 DEAT
2 TYPE Accident
2 DATE 1920
0 @I2@ INDI
1 NAME Johnny /Doe/
1 BIRT
2 TYPE Home birth
2 DATE 1851
1 DEAT
2 TYPE Illness
2 DATE 1921
0 @I3@ INDI
1 NAME Ann /Roe/
1 BIRT
2 TYPE Twin
2 DATE 1800
1 DEAT
2 TYPE Fever
2 PLAC Salem
0 @I4@ INDI
1 NAME Annie /Roe/
1 BIRT
2 DATE 1801
1 DEAT
2 TYPE Drowning
2 PLAC Boston
0 TRLR
`

func TestMergePersons_BirthAndDeathTypeFollowResolvedEvent(t *testing.T) {
	for _, backend := range backends {
		t.Run(backend.name, func(t *testing.T) {
			ctx := context.Background()
			st := backend.setup(t)
			h := command.NewHandlerWithBranches(st.events, st.read, st.branches, st.snapshots)
			if _, err := h.ImportGedcom(ctx, command.ImportGedcomInput{Filename: "t.ged", Reader: strings.NewReader(mergeTypeDetailGedcom)}); err != nil {
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
			merge := func(survivor, merged repository.PersonReadModel, resolution map[string]string) {
				t.Helper()
				if _, err := h.MergePersons(ctx, command.MergePersonsInput{
					SurvivorID: survivor.ID, MergedID: merged.ID,
					SurvivorVersion: survivor.Version, MergedVersion: merged.Version,
					FieldResolution: resolution,
				}); err != nil {
					t.Fatalf("merge %s into %s: %v", merged.GivenName, survivor.GivenName, err)
				}
			}
			// John takes Johnny's birth date and keeps his own death; Ann takes
			// Annie's death place and keeps her own birth.
			merge(byName["John"], byName["Johnny"], map[string]string{"birth_date": "merged"})
			merge(byName["Ann"], byName["Annie"], map[string]string{"death_place": "merged"})

			// The snapshot records the merged person's TYPE for the audit trail.
			snapshots := map[string]map[string]any{}
			page, err := st.events.ReadAll(ctx, 0, 1000)
			if err != nil {
				t.Fatal(err)
			}
			for _, stored := range page {
				decoded, err := stored.DecodeEvent()
				if err != nil {
					t.Fatal(err)
				}
				if e, ok := decoded.(domain.PersonMerged); ok {
					snapshots[e.MergedPersonSnapshot["given_name"].(string)] = e.MergedPersonSnapshot
				}
			}
			for name, want := range map[string][2]any{"Johnny": {"Home birth", "Illness"}, "Annie": {nil, "Drowning"}} {
				snap := snapshots[name]
				if snap == nil {
					t.Fatalf("no PersonMerged snapshot for %s", name)
				}
				if got := [2]any{snap["birth_type_detail"], snap["death_type_detail"]}; got != want {
					t.Errorf("%s snapshot birth/death type = %v, want %v", name, got, want)
				}
			}

			fresh := backend.setup(t).read
			replayLog(t, ctx, st.events, fresh)
			for rsName, rs := range map[string]repository.ReadModelStore{"live": st.read, "replayed": fresh} {
				for _, tc := range []struct {
					survivor             string
					birthType, deathType string
				}{
					{"John", "Home birth", "Accident"},
					{"Ann", "Twin", "Drowning"},
				} {
					got, err := rs.GetPerson(ctx, domain.MainBranchID, byName[tc.survivor].ID)
					if err != nil || got == nil {
						t.Fatalf("%s: get %s: %v, %v", rsName, tc.survivor, got, err)
					}
					if got.BirthTypeDetail != tc.birthType || got.DeathTypeDetail != tc.deathType {
						t.Errorf("%s: %s birth/death type = %q/%q, want %q/%q", rsName, tc.survivor,
							got.BirthTypeDetail, got.DeathTypeDetail, tc.birthType, tc.deathType)
					}
				}
			}

			var buf bytes.Buffer
			if _, err := gedcom.NewExporter(fresh).Export(ctx, &buf); err != nil {
				t.Fatalf("export: %v", err)
			}
			out := buf.String()
			for _, tc := range []struct {
				record, event string
				want          []string
			}{
				{"1 NAME John /Doe/", "BIRT", []string{"2 TYPE Home birth"}},
				{"1 NAME John /Doe/", "DEAT", []string{"2 TYPE Accident"}},
				{"1 NAME Ann /Roe/", "BIRT", []string{"2 TYPE Twin"}},
				{"1 NAME Ann /Roe/", "DEAT", []string{"2 TYPE Drowning"}},
			} {
				if got := eventTypeLines(out, tc.record, tc.event); !slices.Equal(got, tc.want) {
					t.Errorf("TYPE lines under %s of the record with %q = %q, want %q; export:\n%s", tc.event, tc.record, got, tc.want, out)
				}
			}
		})
	}
}

// eventTypeLines returns the "2 TYPE" lines directly under the level-1 event
// tag of the record that has a line starting with recordLine.
func eventTypeLines(out, recordLine, event string) []string {
	var types []string
	inRecord, inEvent := false, false
	for _, line := range strings.Split(out, "\n") {
		line = strings.TrimRight(line, "\r")
		switch {
		case strings.HasPrefix(line, "0 "):
			inRecord, inEvent = false, false
		case strings.HasPrefix(line, recordLine):
			inRecord = true
			inEvent = false
		case strings.HasPrefix(line, "1 "):
			inEvent = inRecord && line == "1 "+event
		case inEvent && strings.HasPrefix(line, "2 TYPE"):
			types = append(types, line)
		}
	}
	return types
}
