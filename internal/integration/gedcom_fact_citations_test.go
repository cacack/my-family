package integration_test

import (
	"bytes"
	"context"
	"fmt"
	"net/http"
	"slices"
	"strings"
	"testing"

	"github.com/google/uuid"

	"github.com/cacack/my-family/internal/command"
	"github.com/cacack/my-family/internal/domain"
	"github.com/cacack/my-family/internal/gedcom"
	"github.com/cacack/my-family/internal/repository"
)

// Citations of one life event or attribute among several of the same fact
// type (issue #1294).
//
// A SOUR under one of two CENS, OCCU or family EVEN cites that one: the
// citation carries the life event's or attribute's id in CitationCreated, so a
// rebuild and a replay keep the link, and an export writes each SOUR back
// under its own event only, not under every event of the fact type.

const factCitationsGedcom = `0 HEAD
1 GEDC
2 VERS 5.5.1
1 CHAR UTF-8
0 @S1@ SOUR
1 TITL 1850 census
0 @S2@ SOUR
1 TITL 1860 census
0 @S3@ SOUR
1 TITL Farm ledger
0 @S4@ SOUR
1 TITL City directory
0 @S5@ SOUR
1 TITL Deed book
0 @S6@ SOUR
1 TITL Probate file
0 @S7@ SOUR
1 TITL Draft card
0 @S8@ SOUR
1 TITL Passport
0 @I1@ INDI
1 NAME A /B/
1 CENS
2 DATE 1850
2 SOUR @S1@
3 PAGE p. 1
1 CENS
2 DATE 1860
2 SOUR @S2@
3 PAGE p. 2
1 OCCU Farmer
2 SOUR @S3@
1 OCCU Blacksmith
2 SOUR @S4@
1 IDNO 111
2 SOUR @S7@
1 IDNO 222
2 SOUR @S8@
0 @I2@ INDI
1 NAME C /D/
0 @F1@ FAM
1 HUSB @I1@
1 WIFE @I2@
1 EVEN
2 TYPE Land purchase
2 DATE 1855
2 SOUR @S5@
1 EVEN
2 TYPE Bequest
2 DATE 1870
2 SOUR @S6@
0 TRLR
`

// factCitationsWant is each life event and attribute with the sources that
// cite it, as factCitationSummaries renders them.
var factCitationsWant = []string{
	"family_generic_event 1855 <- Deed book",
	"family_generic_event 1870 <- Probate file",
	"person_census 1850 <- 1850 census p. 1",
	"person_census 1860 <- 1860 census p. 2",
	"person_identification_number 111 <- Draft card",
	"person_identification_number 222 <- Passport",
	"person_occupation Blacksmith <- City directory",
	"person_occupation Farmer <- Farm ledger",
}

func TestGedcomImport_CitationsStayWithTheirOwnFact(t *testing.T) {
	for _, backend := range backends {
		t.Run(backend.name, func(t *testing.T) {
			ctx := context.Background()
			st := backend.setup(t)
			h := command.NewHandlerWithBranches(st.events, st.read, st.branches, st.snapshots)
			if _, err := h.ImportGedcom(ctx, command.ImportGedcomInput{Filename: "t.ged", Reader: strings.NewReader(factCitationsGedcom)}); err != nil {
				t.Fatalf("import: %v", err)
			}
			if got := factCitationSummaries(t, ctx, st.read); !slices.Equal(got, factCitationsWant) {
				t.Fatalf("imported fact citations = %q, want %q", got, factCitationsWant)
			}

			// A rebuild finds the links in the log: nothing to backfill.
			result, err := repository.RebuildReadModel(ctx, st.events, st.read, st.branches, st.snapshots)
			if err != nil || result.Backfilled != 0 || result.DroppedImported != 0 {
				t.Errorf("rebuild = %+v, %v; want nothing backfilled or dropped", result, err)
			}
			if got := factCitationSummaries(t, ctx, st.read); !slices.Equal(got, factCitationsWant) {
				t.Errorf("rebuilt fact citations = %q, want %q", got, factCitationsWant)
			}

			// Replay into an empty read model: the links must come from the log.
			fresh := backend.setup(t).read
			replayLog(t, ctx, st.events, fresh)
			if got := factCitationSummaries(t, ctx, fresh); !slices.Equal(got, factCitationsWant) {
				t.Errorf("replayed fact citations = %q, want %q", got, factCitationsWant)
			}

			var buf bytes.Buffer
			if _, err := gedcom.NewExporter(fresh).Export(ctx, &buf); err != nil {
				t.Fatalf("export: %v", err)
			}
			wantExport := []string{
				"CENS 1850 <- @S1@",
				"CENS 1860 <- @S2@",
				"EVEN 1855 <- @S5@",
				"EVEN 1870 <- @S6@",
				"IDNO 111 <- @S7@",
				"IDNO 222 <- @S8@",
				"OCCU Blacksmith <- @S4@",
				"OCCU Farmer <- @S3@",
			}
			if got := exportedFactSources(buf.String(), "CENS", "OCCU", "EVEN", "IDNO"); !slices.Equal(got, wantExport) {
				t.Errorf("exported fact sources = %q, want %q; export:\n%s", got, wantExport, buf.String())
			}
		})
	}
}

// Citations created through the API for one life event are not shown on
// another of the same fact type, on the mainline or on a branch merged into
// it (#1294).

const apiFactCitationsGedcom = `0 HEAD
1 GEDC
2 VERS 5.5.1
1 CHAR UTF-8
0 @S1@ SOUR
1 TITL 1850 census
0 @S2@ SOUR
1 TITL 1860 census
0 @I1@ INDI
1 NAME A /B/
1 CENS
2 DATE 1850
1 CENS
2 DATE 1860
0 TRLR
`

func TestAPI_CitationOfOneLifeEvent(t *testing.T) {
	for _, backend := range backends {
		t.Run(backend.name, func(t *testing.T) {
			ctx := context.Background()
			st := backend.setup(t)
			h := command.NewHandlerWithBranches(st.events, st.read, st.branches, st.snapshots)
			if _, err := h.ImportGedcom(ctx, command.ImportGedcomInput{Filename: "t.ged", Reader: strings.NewReader(apiFactCitationsGedcom)}); err != nil {
				t.Fatalf("import: %v", err)
			}
			server := newServer(t, st)

			persons, err := repository.ListAll(ctx, 100, st.read.ListPersons)
			if err != nil || len(persons) != 1 {
				t.Fatalf("persons = %v, %v; want one", persons, err)
			}
			personID := persons[0].ID
			census := map[string]uuid.UUID{}
			events, err := st.read.ListEventsForPerson(ctx, domain.MainBranchID, personID)
			if err != nil {
				t.Fatalf("list events: %v", err)
			}
			for _, e := range events {
				census[e.DateRaw] = e.ID
			}
			sources := map[string]uuid.UUID{}
			allSources, err := repository.ListAll(ctx, 100, st.read.ListSources)
			if err != nil {
				t.Fatalf("list sources: %v", err)
			}
			for _, s := range allSources {
				sources[s.Title] = s.ID
			}
			body := func(source string, factType string, factID uuid.UUID) string {
				return fmt.Sprintf(`{"source_id":%q,"fact_type":%q,"fact_owner_id":%q,"fact_id":%q}`,
					sources[source], factType, personID, factID)
			}

			// The mainline cites the 1850 census; the response names it.
			created := mustDo(t, server, http.MethodPost, "/api/v1/citations",
				body("1850 census", "person_census", census["1850"]), http.StatusCreated)
			if got := created["fact_id"]; got != census["1850"].String() {
				t.Errorf("created fact_id = %v, want %s", got, census["1850"])
			}

			// A branch cites the 1860 census, then merges into the mainline.
			branchID := createBranch(t, server, "census-1860")
			onBranch := mustDo(t, server, http.MethodPost, scoped("/api/v1/citations", branchID),
				body("1860 census", "person_census", census["1860"]), http.StatusCreated)
			citationPath := "/api/v1/citations/" + mustString(t, onBranch, "id")
			if got := getEntity(t, server, citationPath, branchID)["fact_id"]; got != census["1860"].String() {
				t.Errorf("branch citation fact_id = %v, want %s", got, census["1860"])
			}
			mustDo(t, server, http.MethodPost, mergePath(branchID), mergeBody("1860 census found"), http.StatusOK)
			if got := getEntity(t, server, citationPath, "")["fact_id"]; got != census["1860"].String() {
				t.Errorf("merged citation fact_id = %v, want %s", got, census["1860"])
			}

			// A fact_id must be a life event or attribute of the citation's fact
			// type and owner.
			for name, bad := range map[string]string{
				"other fact type": body("1850 census", "person_occupation", census["1850"]),
				"unknown fact":    body("1850 census", "person_census", uuid.New()),
			} {
				if rec := do(t, server, http.MethodPost, "/api/v1/citations", bad); rec.Code != http.StatusBadRequest {
					t.Errorf("%s: status = %d, want 400. Body: %s", name, rec.Code, rec.Body.String())
				}
			}

			rec := do(t, server, http.MethodGet, "/api/v1/gedcom/export", "")
			if rec.Code != http.StatusOK {
				t.Fatalf("export: status = %d. Body: %s", rec.Code, rec.Body.String())
			}
			want := []string{"CENS 1850 <- @S1@", "CENS 1860 <- @S2@"}
			if got := exportedFactSources(rec.Body.String(), "CENS"); !slices.Equal(got, want) {
				t.Errorf("exported census sources = %q, want %q; export:\n%s", got, want, rec.Body.String())
			}
		})
	}
}

// factCitationSummaries returns, sorted, each main-branch life event as
// "<fact type> <date> <- <source> <page>..." and each attribute as
// "<fact type> <value> <- ...", from the citations of that one fact.
func factCitationSummaries(t *testing.T, ctx context.Context, rs repository.ReadModelStore) []string {
	t.Helper()
	cites := func(factType domain.FactType, ownerID, factID uuid.UUID) string {
		citations, err := rs.GetCitationsForFactInstance(ctx, domain.MainBranchID, factType, ownerID, factID)
		if err != nil {
			t.Fatalf("citations for %s: %v", factID, err)
		}
		var parts []string
		for _, c := range citations {
			parts = append(parts, strings.TrimSpace(c.SourceTitle+" "+c.Page))
		}
		return strings.Join(parts, ", ")
	}
	events, err := repository.ListAll(ctx, 100, rs.ListEvents)
	if err != nil {
		t.Fatalf("list events: %v", err)
	}
	attrs, err := repository.ListAll(ctx, 100, rs.ListAttributes)
	if err != nil {
		t.Fatalf("list attributes: %v", err)
	}
	var got []string
	for _, e := range events {
		got = append(got, fmt.Sprintf("%s %s <- %s", e.FactType, e.DateRaw, cites(e.FactType, e.OwnerID, e.ID)))
	}
	for _, a := range attrs {
		got = append(got, fmt.Sprintf("%s %s <- %s", a.FactType, a.Value, cites(a.FactType, a.PersonID, a.ID)))
	}
	slices.Sort(got)
	return got
}

// exportedFactSources returns, sorted, each exported level-1 record line whose
// tag is one of tags as "<tag> <value or DATE> <- <SOUR xrefs>".
func exportedFactSources(out string, tags ...string) []string {
	var got []string
	var tag, label string
	var sours []string
	flush := func() {
		if tag != "" {
			got = append(got, fmt.Sprintf("%s %s <- %s", tag, label, strings.Join(sours, " ")))
		}
		tag, label, sours = "", "", nil
	}
	for _, line := range strings.Split(out, "\n") {
		line = strings.TrimRight(line, "\r")
		fields := strings.SplitN(line, " ", 3)
		switch {
		case len(fields) >= 2 && (fields[0] == "0" || fields[0] == "1"):
			flush()
			if fields[0] == "1" && slices.Contains(tags, fields[1]) {
				tag = fields[1]
				if len(fields) == 3 {
					label = fields[2]
				}
			}
		case tag != "" && len(fields) == 3 && fields[0] == "2" && fields[1] == "DATE" && label == "":
			label = fields[2]
		case tag != "" && len(fields) == 3 && fields[0] == "2" && fields[1] == "SOUR":
			sours = append(sours, fields[2])
		}
	}
	flush()
	slices.Sort(got)
	return got
}
