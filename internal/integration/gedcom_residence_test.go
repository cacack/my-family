package integration_test

import (
	"bytes"
	"context"
	"net/http"
	"slices"
	"strings"
	"testing"

	"github.com/cacack/my-family/internal/command"
	"github.com/cacack/my-family/internal/domain"
	"github.com/cacack/my-family/internal/gedcom"
)

// A RESI is imported once (issue #1266).
//
// It used to be stored both as a generic life event and as a residence
// attribute, and exported as both `1 EVEN` and `1 RESI`. It is now one
// residence life event, which keeps its date, place, coordinates, address,
// note and citation through a replay of the log and back out as one RESI.

const residenceGedcom = `0 HEAD
1 GEDC
2 VERS 5.5.1
1 CHAR UTF-8
0 @S1@ SOUR
1 TITL Census 1880
0 @I1@ INDI
1 NAME John /Doe/
1 SEX M
1 RESI
2 DATE 1880
2 PLAC Lowell
3 MAP
4 LATI N42.6334
4 LONG W71.3162
2 ADDR 12 Mill St
3 CITY Lowell
2 SOUR @S1@
3 PAGE Sheet 4
2 NOTE Boarded with the Smiths
0 TRLR
`

func TestGedcomImport_ResidenceImportedAndExportedOnce(t *testing.T) {
	for _, backend := range backends {
		t.Run(backend.name, func(t *testing.T) {
			ctx := context.Background()
			st := backend.setup(t)
			h := command.NewHandlerWithBranches(st.events, st.read, st.branches, st.snapshots)
			if _, err := h.ImportGedcom(ctx, command.ImportGedcomInput{Filename: "t.ged", Reader: strings.NewReader(residenceGedcom)}); err != nil {
				t.Fatalf("import: %v", err)
			}

			// Replay into an empty read model: what follows comes from the log.
			fresh := backend.setup(t)
			replayLog(t, ctx, st.events, fresh.read)

			john := findPerson(t, ctx, fresh.read, "John")
			events, err := fresh.read.ListEventsForPerson(ctx, domain.MainBranchID, john.ID)
			if err != nil {
				t.Fatal(err)
			}
			if len(events) != 1 || events[0].FactType != domain.FactPersonResidence {
				t.Fatalf("life events = %+v, want one person_residence", events)
			}
			resi := events[0]
			if resi.DateRaw != "1880" || resi.Place != "Lowell" || resi.Notes != "Boarded with the Smiths" ||
				resi.PlaceLat == nil || *resi.PlaceLat != "N42.6334" || resi.Address == nil || resi.Address.City != "Lowell" {
				t.Errorf("residence = %+v, want its date, place, coordinates, address and note", resi)
			}
			if attrs, err := fresh.read.ListAttributesForPerson(ctx, domain.MainBranchID, john.ID); err != nil || len(attrs) != 0 {
				t.Errorf("attributes = %+v, %v; want none", attrs, err)
			}

			// The API lists the residence once: in the events, not the attributes.
			server := newServer(t, fresh)
			mustDo(t, server, http.MethodGet, "/api/v1/persons/"+john.ID.String(), "", http.StatusOK)
			apiEvents := jsonArray(t, mustDo(t, server, http.MethodGet, "/api/v1/export/events", "", http.StatusOK), "events")
			if got := entryField(t, apiEvents, "fact_type"); !slices.Equal(got, []string{string(domain.FactPersonResidence)}) {
				t.Errorf("exported event fact types = %q, want one person_residence", got)
			}
			if got := jsonArray(t, mustDo(t, server, http.MethodGet, "/api/v1/export/attributes", "", http.StatusOK), "attributes"); len(got) != 0 {
				t.Errorf("exported attributes = %v, want none", got)
			}

			var buf bytes.Buffer
			if _, err := gedcom.NewExporter(fresh.read).Export(ctx, &buf); err != nil {
				t.Fatalf("export: %v", err)
			}
			out := buf.String()
			if n := strings.Count(out, "\n1 RESI"); n != 1 || strings.Contains(out, "\n1 EVEN") {
				t.Fatalf("export has %d RESI lines (want 1) or an EVEN:\n%s", n, out)
			}
			want := []string{"2 DATE 1880", "2 PLAC Lowell", "4 LATI N42.6334", "4 LONG W71.3162", "3 CITY Lowell", "3 PAGE Sheet 4", "2 NOTE Boarded with the Smiths"}
			block := residenceBlock(out)
			for _, line := range want {
				if !slices.Contains(block, line) {
					t.Errorf("RESI block lacks %q; block = %q", line, block)
				}
			}
		})
	}
}

// residenceBlock returns the lines of the first `1 RESI` structure, from the
// RESI line up to the next level-0 or level-1 line.
func residenceBlock(out string) []string {
	var block []string
	for _, line := range strings.Split(out, "\n") {
		line = strings.TrimRight(line, "\r")
		switch {
		case strings.HasPrefix(line, "1 RESI"):
			block = []string{line}
		case block != nil && (strings.HasPrefix(line, "0 ") || strings.HasPrefix(line, "1 ")):
			return block
		case block != nil:
			block = append(block, line)
		}
	}
	return block
}
