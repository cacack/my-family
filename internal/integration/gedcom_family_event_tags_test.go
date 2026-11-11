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

// Family EVEN, DIVF, CENS and RESI (issue #1285).
//
// Import keeps each as a family life event, with an EVEN's descriptor and an
// event's TYPE in LifeEventCreated, so replaying the log into an empty read
// model reproduces them and an export writes them back under the FAM.

const familyEventTagsGedcom = `0 HEAD
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
1 EVEN Moved to Ohio
2 TYPE Relocation
2 DATE 1850
1 DIVF
2 DATE 1860
1 CENS
2 DATE 1860
1 RESI
2 DATE 1855
0 TRLR
`

func TestGedcomImport_FamilyEventTagsSurviveReplayAndExport(t *testing.T) {
	for _, backend := range backends {
		t.Run(backend.name, func(t *testing.T) {
			ctx := context.Background()
			st := backend.setup(t)
			h := command.NewHandlerWithBranches(st.events, st.read, st.branches, st.snapshots)
			if _, err := h.ImportGedcom(ctx, command.ImportGedcomInput{Filename: "t.ged", Reader: strings.NewReader(familyEventTagsGedcom)}); err != nil {
				t.Fatalf("import: %v", err)
			}

			// Replay into an empty read model: the events must come from the log.
			fresh := backend.setup(t).read
			replayLog(t, ctx, st.events, fresh)

			families, _, err := fresh.ListFamilies(ctx, repository.ListOptions{Limit: 10})
			if err != nil || len(families) != 1 {
				t.Fatalf("ListFamilies = %d families, err %v", len(families), err)
			}
			events, err := fresh.ListEventsForFamily(ctx, domain.MainBranchID, families[0].ID)
			if err != nil {
				t.Fatalf("ListEventsForFamily: %v", err)
			}
			i := slices.IndexFunc(events, func(e repository.EventReadModel) bool {
				return e.FactType == domain.FactFamilyGenericEvent
			})
			if i < 0 {
				t.Fatalf("no %s event after replay; events = %+v", domain.FactFamilyGenericEvent, events)
			}
			if e := events[i]; e.Description != "Moved to Ohio" || e.TypeDetail != "Relocation" || e.DateRaw != "1850" {
				t.Errorf("family EVEN after replay = %+v", e)
			}

			var buf bytes.Buffer
			if _, err := gedcom.NewExporter(fresh).Export(ctx, &buf); err != nil {
				t.Fatalf("export: %v", err)
			}
			out := buf.String()

			for _, tc := range []struct {
				event string
				want  []string
			}{
				{"1 EVEN Moved to Ohio", []string{"2 DATE 1850", "2 TYPE Relocation"}},
				{"1 DIVF", []string{"2 DATE 1860"}},
				{"1 CENS", []string{"2 DATE 1860"}},
				{"1 RESI", []string{"2 DATE 1855"}},
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
		})
	}
}
