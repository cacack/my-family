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
// NO MARR) each export their own citations only, after a rebuild, a replay and
// a re-import (#1300): a citation of the record's own event names the record
// (fact_id = the owner's ID, #1319), and is exported even under a bare
// BIRT. A citation with no fact_id, created before #1319, is taken to be the
// record's own when the record holds that event.

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
0 @S8@ SOUR
1 TITL Christening note
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
0 @I3@ INDI
1 NAME E /F/
1 BIRT
2 SOUR @S8@
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
				"BIRT  <- @S8@", // a bare BIRT
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

			// The replayed record-level citations still name their owner.
			citations, err := repository.ListAll(ctx, 100, fresh.ListCitations)
			if err != nil {
				t.Fatalf("list citations: %v", err)
			}
			recordLevel := map[string]bool{"Baptismal register": true, "Burial register": true, "Marriage bond": true, "Christening note": true}
			for _, c := range citations {
				if recordLevel[c.SourceTitle] != (c.FactID != nil && *c.FactID == c.FactOwnerID) {
					t.Errorf("replayed citation of %q: fact_id = %v, owner %s", c.SourceTitle, c.FactID, c.FactOwnerID)
				}
			}

			// A re-import of the export keeps every citation with its event.
			var exported bytes.Buffer
			if _, err := gedcom.NewExporter(st.read).Export(ctx, &exported); err != nil {
				t.Fatalf("export for re-import: %v", err)
			}
			again := backend.setup(t)
			if _, err := command.NewHandlerWithBranches(again.events, again.read, again.branches, again.snapshots).ImportGedcom(ctx, command.ImportGedcomInput{Filename: "again.ged", Reader: &exported}); err != nil {
				t.Fatalf("re-import: %v", err)
			}
			check("re-imported", again.read, want)

			// Citations created before #1319 carry no fact_id: each is written
			// under the record's own event when the record holds it, and
			// otherwise under the NO event of its fact type.
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
