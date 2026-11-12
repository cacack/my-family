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

// A birth, death or marriage and its negated counterpart (NO BIRT, NO DEAT,
// NO MARR) each export their own citations only, after a rebuild and a
// replay; citations with no fact_id (imported before #1294) export under the
// record's own event when it has one, else under the negated event (#1300).

const negatedFactCitationsGedcom = `0 HEAD
1 GEDC
2 VERS 7.0
0 @S1@ SOUR
1 TITL Baptismal register
0 @S2@ SOUR
1 TITL Parish census
0 @S3@ SOUR
1 TITL Burial register
0 @S4@ SOUR
1 TITL Later letter
0 @S5@ SOUR
1 TITL Marriage bond
0 @S6@ SOUR
1 TITL Church minutes
0 @S7@ SOUR
1 TITL Family bible
0 @I1@ INDI
1 NAME A /B/
1 BIRT
2 DATE 1830
2 SOUR @S1@
1 NO BIRT
2 SOUR @S2@
1 DEAT
2 DATE 1890
2 SOUR @S3@
1 NO DEAT
2 SOUR @S4@
0 @I2@ INDI
1 NAME C /D/
1 NO BIRT
2 SOUR @S7@
0 @F1@ FAM
1 HUSB @I1@
1 WIFE @I2@
1 MARR
2 DATE 1855
2 SOUR @S5@
1 NO MARR
2 SOUR @S6@
0 TRLR
`

func TestGedcomExport_NegatedEventsKeepTheirOwnCitations(t *testing.T) {
	for _, backend := range backends {
		t.Run(backend.name, func(t *testing.T) {
			ctx := context.Background()
			st := backend.setup(t)
			h := command.NewHandlerWithBranches(st.events, st.read, st.branches, st.snapshots)
			if _, err := h.ImportGedcom(ctx, command.ImportGedcomInput{Filename: "t.ged", Reader: strings.NewReader(negatedFactCitationsGedcom)}); err != nil {
				t.Fatalf("import: %v", err)
			}
			// exportedFactSources labels a NO line by its value (BIRT, DEAT, MARR).
			tags := []string{"BIRT", "DEAT", "MARR", "NO"}
			check := func(stage string, rs repository.ReadModelStore, want []string) {
				t.Helper()
				var buf bytes.Buffer
				if _, err := gedcom.NewExporter(rs).Export(ctx, &buf); err != nil {
					t.Fatalf("%s: export: %v", stage, err)
				}
				got := exportedFactSources(buf.String(), tags...)
				for i, line := range got { // the order of one event's citations is not part of the contract
					head, sours, _ := strings.Cut(line, " <- ")
					xrefs := strings.Fields(sours)
					slices.Sort(xrefs)
					got[i] = head + " <- " + strings.Join(xrefs, " ")
				}
				if !slices.Equal(got, want) {
					t.Errorf("%s: exported sources = %q, want %q; export:\n%s", stage, got, want, buf.String())
				}
			}
			want := []string{
				"BIRT 1830 <- @S1@",
				"DEAT 1890 <- @S3@",
				"MARR 1855 <- @S5@",
				"NO BIRT <- @S2@",
				"NO BIRT <- @S7@",
				"NO DEAT <- @S4@",
				"NO MARR <- @S6@",
			}
			check("imported", st.read, want)

			if _, err := repository.RebuildReadModel(ctx, st.events, st.read, st.branches, st.snapshots); err != nil {
				t.Fatalf("rebuild: %v", err)
			}
			check("rebuilt", st.read, want)

			fresh := backend.setup(t).read
			replayLog(t, ctx, st.events, fresh)
			check("replayed", fresh, want)

			// Citations as imported before #1294 carry no fact_id. A NO event
			// keeps them only when its owner holds no BIRT, DEAT or MARR of
			// its own, as C /D/'s NO BIRT.
			citations, err := repository.ListAll(ctx, 100, fresh.ListCitations)
			if err != nil {
				t.Fatalf("list citations: %v", err)
			}
			for _, c := range citations {
				c.FactID = nil
				if err := fresh.SaveCitation(ctx, domain.MainBranchID, &c); err != nil {
					t.Fatalf("save citation: %v", err)
				}
			}
			check("unlinked", fresh, []string{
				"BIRT 1830 <- @S1@ @S2@",
				"DEAT 1890 <- @S3@ @S4@",
				"MARR 1855 <- @S5@ @S6@",
				"NO BIRT <- ",
				"NO BIRT <- @S7@",
				"NO DEAT <- ",
				"NO MARR <- ",
			})
		})
	}
}
