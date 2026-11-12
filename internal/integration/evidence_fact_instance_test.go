package integration_test

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"slices"
	"strings"
	"testing"

	"github.com/cacack/my-family/internal/api"
	"github.com/cacack/my-family/internal/command"
	"github.com/cacack/my-family/internal/domain"
	"github.com/cacack/my-family/internal/repository"
)

// Evidence analyses, conflicts and proof summaries of one life event or
// attribute among several of the same fact type (issue #1304).
//
// An analysis or proof may name the census it is about (fact_id); the API
// rejects a fact_id of another fact type or subject. Listing one census's
// analyses returns its own plus those of the census fact as a whole, not the
// other census's. Analyses of different censuses do not conflict. The link
// travels in the Created and Updated events, so it survives a rebuild.

const evidenceFactsGedcom = `0 HEAD
1 GEDC
2 VERS 5.5.1
1 CHAR UTF-8
0 @I1@ INDI
1 NAME A /B/
1 CENS
2 DATE 1850
1 CENS
2 DATE 1860
1 OCCU Farmer
0 @I2@ INDI
1 NAME C /D/
1 CENS
2 DATE 1850
0 TRLR
`

func TestEvidence_FactInstance(t *testing.T) {
	for _, backend := range backends {
		t.Run(backend.name, func(t *testing.T) {
			ctx := context.Background()
			st := backend.setup(t)
			h := command.NewHandlerWithBranches(st.events, st.read, st.branches, st.snapshots)
			if _, err := h.ImportGedcom(ctx, command.ImportGedcomInput{Filename: "t.ged", Reader: strings.NewReader(evidenceFactsGedcom)}); err != nil {
				t.Fatalf("import: %v", err)
			}
			server := newServer(t, st)

			// The imported facts, by owner name and date or value.
			facts := map[string]string{}
			persons, err := repository.ListAll(ctx, 10, st.read.ListPersons)
			if err != nil {
				t.Fatalf("list persons: %v", err)
			}
			for _, p := range persons {
				facts[p.GivenName] = p.ID.String()
				events, err := st.read.ListEventsForPerson(ctx, domain.MainBranchID, p.ID)
				if err != nil {
					t.Fatalf("list events: %v", err)
				}
				for _, e := range events {
					facts[p.GivenName+" "+e.DateRaw] = e.ID.String()
				}
				attrs, err := st.read.ListAttributesForPerson(ctx, domain.MainBranchID, p.ID)
				if err != nil {
					t.Fatalf("list attributes: %v", err)
				}
				for _, a := range attrs {
					facts[p.GivenName+" "+a.Value] = a.ID.String()
				}
			}
			a, census1850, census1860 := facts["A"], facts["A 1850"], facts["A 1860"]
			if a == "" || census1850 == "" || census1860 == "" || facts["A Farmer"] == "" || facts["C 1850"] == "" {
				t.Fatalf("imported facts = %v", facts)
			}

			analysis := func(conclusion, factID string) string {
				body := fmt.Sprintf(`{"fact_type":"person_census","subject_id":%q,"conclusion":%q`, a, conclusion)
				if factID != "" {
					body += fmt.Sprintf(`,"fact_id":%q`, factID)
				}
				return body + "}"
			}

			// A fact_id of another subject, another fact type, or no fact.
			for name, body := range map[string]string{
				"census of another person": analysis("x", facts["C 1850"]),
				"no such fact":             analysis("x", "00000000-0000-0000-0000-000000000001"),
				"attribute of another fact type": fmt.Sprintf(
					`{"fact_type":"person_census","subject_id":%q,"conclusion":"x","fact_id":%q}`, a, facts["A Farmer"]),
			} {
				if rec := do(t, server, http.MethodPost, "/api/v1/evidence-analyses", body); rec.Code != http.StatusBadRequest {
					t.Errorf("analysis of %s: status = %d, want 400. Body: %s", name, rec.Code, rec.Body.String())
				}
				proofBody := strings.Replace(body, `"conclusion":"x"`, `"conclusion":"x","argument":"y"`, 1)
				if rec := do(t, server, http.MethodPost, "/api/v1/proof-summaries", proofBody); rec.Code != http.StatusBadRequest {
					t.Errorf("proof of %s: status = %d, want 400. Body: %s", name, rec.Code, rec.Body.String())
				}
			}

			ohio := mustDo(t, server, http.MethodPost, "/api/v1/evidence-analyses", analysis("Ohio", census1850), http.StatusCreated)
			if got := ohio["fact_id"]; got != census1850 {
				t.Errorf("created analysis fact_id = %v, want %s", got, census1850)
			}
			// Another census's analysis disagrees with Ohio, but about another fact.
			indiana := mustDo(t, server, http.MethodPost, "/api/v1/evidence-analyses", analysis("Indiana", census1860), http.StatusCreated)
			if got, ok := indiana["conflict_id"]; ok {
				t.Errorf("analysis of the 1860 census raised conflict %v with the 1850 one", got)
			}
			// A second analysis of the 1850 census does conflict, about that census.
			kentucky := mustDo(t, server, http.MethodPost, "/api/v1/evidence-analyses", analysis("Kentucky", census1850), http.StatusCreated)
			conflictID, _ := kentucky["conflict_id"].(string)
			if conflictID == "" {
				t.Fatalf("conflicting analysis of the 1850 census raised no conflict: %v", kentucky)
			}
			// An analysis of the census fact as a whole has no fact_id.
			whole := mustDo(t, server, http.MethodPost, "/api/v1/evidence-analyses", analysis("Midwest", ""), http.StatusCreated)
			if got, ok := whole["fact_id"]; ok {
				t.Errorf("whole-fact analysis fact_id = %v, want none", got)
			}

			proof := func(conclusion, factID string) string {
				return strings.Replace(analysis(conclusion, factID), `"conclusion"`, `"argument":"because","conclusion"`, 1)
			}
			proof1850 := mustDo(t, server, http.MethodPost, "/api/v1/proof-summaries", proof("Ohio", census1850), http.StatusCreated)
			wholeProof := mustDo(t, server, http.MethodPost, "/api/v1/proof-summaries", proof("Midwest", ""), http.StatusCreated)

			ids := func(resp ...map[string]any) []string {
				var out []string
				for _, r := range resp {
					out = append(out, r["id"].(string))
				}
				slices.Sort(out)
				return out
			}
			check := func(stage string) {
				t.Helper()
				for _, tc := range []struct {
					path, factID string
					want         []string
				}{
					{"evidence-analyses", census1850, ids(ohio, kentucky, whole)},
					{"evidence-analyses", census1860, ids(indiana, whole)},
					{"evidence-analyses", "", ids(ohio, indiana, kentucky, whole)},
					{"proof-summaries", census1850, ids(proof1850, wholeProof)},
					{"proof-summaries", census1860, ids(wholeProof)},
				} {
					if got := listByFact(t, server, tc.path, a, tc.factID); !slices.Equal(got, tc.want) {
						t.Errorf("%s: %s by fact %q = %v, want %v", stage, tc.path, tc.factID, got, tc.want)
					}
				}
				conflict := mustDo(t, server, http.MethodGet, "/api/v1/evidence-conflicts/"+conflictID, "", http.StatusOK)
				if got := conflict["fact_id"]; got != census1850 {
					t.Errorf("%s: conflict fact_id = %v, want %s", stage, got, census1850)
				}
				got := mustDo(t, server, http.MethodGet, "/api/v1/evidence-analyses/"+ohio["id"].(string), "", http.StatusOK)
				if got["fact_id"] != census1850 {
					t.Errorf("%s: analysis fact_id = %v, want %s", stage, got["fact_id"], census1850)
				}
			}
			check("before rebuild")

			// Re-pointing the proof at the 1860 census is checked like a create.
			proofPath := "/api/v1/proof-summaries/" + proof1850["id"].(string)
			mustDo(t, server, http.MethodPut, proofPath,
				fmt.Sprintf(`{"fact_id":%q,"version":1}`, facts["C 1850"]), http.StatusBadRequest)
			moved := mustDo(t, server, http.MethodPut, proofPath,
				fmt.Sprintf(`{"fact_id":%q,"version":1}`, census1860), http.StatusOK)
			if got := moved["fact_id"]; got != census1860 {
				t.Errorf("updated proof fact_id = %v, want %s", got, census1860)
			}
			if got, want := listByFact(t, server, "proof-summaries", a, census1860), ids(proof1850, wholeProof); !slices.Equal(got, want) {
				t.Errorf("proofs of the 1860 census after update = %v, want %v", got, want)
			}
			// Put it back, so check() holds again.
			mustDo(t, server, http.MethodPut, proofPath, fmt.Sprintf(`{"fact_id":%q,"version":2}`, census1850), http.StatusOK)

			// A rebuild finds every link in the log.
			if _, err := repository.RebuildReadModel(ctx, st.events, st.read, st.branches, st.snapshots); err != nil {
				t.Fatalf("rebuild: %v", err)
			}
			check("after rebuild")
		})
	}
}

// listByFact returns the sorted ids a /by-fact list of a person's census facts
// returns, narrowed to factID when it is not empty.
func listByFact(t *testing.T, server *api.Server, resource, subjectID, factID string) []string {
	t.Helper()
	path := fmt.Sprintf("/api/v1/%s/by-fact?factType=person_census&subjectId=%s", resource, subjectID)
	if factID != "" {
		path += "&factId=" + factID
	}
	rec := do(t, server, http.MethodGet, path, "")
	if rec.Code != http.StatusOK {
		t.Fatalf("GET %s: status = %d. Body: %s", path, rec.Code, rec.Body.String())
	}
	var items []struct {
		ID string `json:"id"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &items); err != nil {
		t.Fatalf("parse %s: %v", path, err)
	}
	out := make([]string, 0, len(items))
	for _, item := range items {
		out = append(out, item.ID)
	}
	slices.Sort(out)
	return out
}
