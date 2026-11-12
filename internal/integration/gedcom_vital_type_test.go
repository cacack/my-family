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

// TYPE under BIRT, DEAT and MARR (issue #1284).
//
// Birth, death and marriage live on the person and family, not as life events,
// so their TYPE travels in PersonCreated and FamilyCreated. Replaying the log
// into an empty read model reproduces it and an export writes it back.

const vitalTypeGedcom = `0 HEAD
1 GEDC
2 VERS 5.5.1
1 CHAR UTF-8
0 @I1@ INDI
1 NAME John /Doe/
1 SEX M
1 BIRT
2 TYPE Stillborn
2 DATE 1 JAN 1850
1 DEAT
2 TYPE Infant death
2 DATE 1 JAN 1850
0 @I2@ INDI
1 NAME Mary /Roe/
1 SEX F
0 @F1@ FAM
1 HUSB @I1@
1 WIFE @I2@
1 MARR
2 TYPE Civil
2 DATE 1875
0 TRLR
`

func TestGedcomImport_VitalEventTypeSurvivesReplayAndExport(t *testing.T) {
	for _, backend := range backends {
		t.Run(backend.name, func(t *testing.T) {
			ctx := context.Background()
			st := backend.setup(t)
			h := command.NewHandlerWithBranches(st.events, st.read, st.branches, st.snapshots)
			if _, err := h.ImportGedcom(ctx, command.ImportGedcomInput{Filename: "t.ged", Reader: strings.NewReader(vitalTypeGedcom)}); err != nil {
				t.Fatalf("import: %v", err)
			}

			// Replay into an empty read model: the TYPEs must come from the log.
			fresh := backend.setup(t).read
			replayLog(t, ctx, st.events, fresh)

			var buf bytes.Buffer
			if _, err := gedcom.NewExporter(fresh).Export(ctx, &buf); err != nil {
				t.Fatalf("export: %v", err)
			}
			out := buf.String()

			for _, tc := range []struct{ event, want string }{
				{"1 BIRT", "2 TYPE Stillborn"},
				{"1 DEAT", "2 TYPE Infant death"},
				{"1 MARR", "2 TYPE Civil"},
			} {
				got, ok := eventBlockLines(out, tc.event)
				if !ok {
					t.Errorf("no %q line in export:\n%s", tc.event, out)
					continue
				}
				if !slices.Contains(got, tc.want) {
					t.Errorf("lines under %q = %q, want %q among them; export:\n%s", tc.event, got, tc.want, out)
				}
			}
		})
	}
}
