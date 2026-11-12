package api_test

import (
	"fmt"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/cacack/my-family/internal/api"
)

// updateTarget is one record behind an update endpoint (#1359).
type updateTarget struct {
	update    string // PUT or PATCH path
	get       string // GET path that reads the record back
	method    string // http.MethodPut unless set
	versioned bool   // the body carries the record's version
	item      string // when set, get lists records and this is the one's id
}

// nullClearSetup builds one record for every update endpoint, with what each
// needs (a person, a source, a citation...), on a fresh server.
func nullClearSetup(t *testing.T) (*api.Server, map[string]updateTarget) {
	t.Helper()
	server := setupBranchTestServer()
	person := createPerson(t, server, "Ada", "Lovelace")
	partner := createPerson(t, server, "William", "King")
	family := createFamily(t, server, person, partner)
	source := createSource(t, server, "Parish register")
	citation := createCitation(t, server, source, person)
	mediaID, _ := uploadMedia(t, server, person, "", createTestJPEGImage())
	branch := createBranch(t, server, "Clearing")
	note, _ := createNote(t, server, "A note")
	submitter, _ := createSubmitter(t, server, "Researcher")
	repo, _ := createRepository(t, server, "Archive")
	association, _ := createAssociation(t, server, person, partner, "godparent")
	ordinance, _ := createIndividualOrdinance(t, server, person)

	created := func(path, body string) string {
		t.Helper()
		rec := do(t, server, http.MethodPost, path, body)
		if rec.Code != http.StatusCreated {
			t.Fatalf("POST %s: status = %d. Body: %s", path, rec.Code, rec.Body.String())
		}
		id, _ := decodeJSON(t, rec)["id"].(string)
		return id
	}
	name := created("/api/v1/persons/"+person+"/names", `{"given_name":"Augusta","surname":"Byron","name_type":"birth"}`)
	analysis := created("/api/v1/evidence-analyses", fmt.Sprintf(
		`{"fact_type":"person_birth","subject_id":%q,"conclusion":"Born 1815"}`, person))
	researchLog := created("/api/v1/research-logs", fmt.Sprintf(
		`{"subject_id":%q,"subject_type":"person","repository":"Archive","search_description":"Baptisms",`+
			`"outcome":"found","search_date":%q}`, person, time.Now().UTC().Format(time.RFC3339)))
	proof := created("/api/v1/proof-summaries", fmt.Sprintf(
		`{"fact_type":"person_birth","subject_id":%q,"conclusion":"Born 1815","argument":"Register agrees"}`, person))

	at := func(path string) updateTarget {
		return updateTarget{update: path, get: path, method: http.MethodPut, versioned: true}
	}
	targets := map[string]updateTarget{
		"person":            at("/api/v1/persons/" + person),
		"family":            at("/api/v1/families/" + family),
		"media":             at("/api/v1/media/" + mediaID),
		"source":            at("/api/v1/sources/" + source),
		"citation":          at("/api/v1/citations/" + citation),
		"name":              {update: "/api/v1/persons/" + person + "/names/" + name, get: "/api/v1/persons/" + person + "/names", method: http.MethodPut, item: name},
		"branch":            {update: "/api/v1/branches/" + branch, get: "/api/v1/branches/" + branch, method: http.MethodPatch},
		"note":              at("/api/v1/notes/" + note),
		"submitter":         at("/api/v1/submitters/" + submitter),
		"repository":        at("/api/v1/repositories/" + repo),
		"association":       at("/api/v1/associations/" + association),
		"lds_ordinance":     at("/api/v1/lds-ordinances/" + ordinance),
		"evidence_analysis": at("/api/v1/evidence-analyses/" + analysis),
		"research_log":      at("/api/v1/research-logs/" + researchLog),
		"proof_summary":     at("/api/v1/proof-summaries/" + proof),
	}
	return server, targets
}

// send issues an update with the given body members, adding the record's
// current version when the endpoint takes one, and fails unless wantStatus.
func (tg updateTarget) send(t *testing.T, server *api.Server, members string, wantStatus int) {
	t.Helper()
	if tg.versioned {
		version := tg.read(t, server)["version"]
		if members != "" {
			members += ","
		}
		members += fmt.Sprintf(`"version":%v`, version)
	}
	rec := do(t, server, tg.method, tg.update, "{"+members+"}")
	if rec.Code != wantStatus {
		t.Fatalf("%s %s {%s}: status = %d, want %d. Body: %s", tg.method, tg.update, members, rec.Code, wantStatus, rec.Body.String())
	}
}

func (tg updateTarget) read(t *testing.T, server *api.Server) map[string]any {
	t.Helper()
	rec := do(t, server, http.MethodGet, tg.get, "")
	if rec.Code != http.StatusOK {
		t.Fatalf("GET %s: status = %d. Body: %s", tg.get, rec.Code, rec.Body.String())
	}
	body := decodeJSON(t, rec)
	if tg.item == "" {
		return body
	}
	items, _ := body["items"].([]any)
	for _, item := range items {
		if record, _ := item.(map[string]any); record["id"] == tg.item {
			return record
		}
	}
	t.Fatalf("GET %s: no item %s. Body: %s", tg.get, tg.item, rec.Body.String())
	return nil
}

// isCleared reports whether a field read back is absent, null or empty.
func isCleared(v any) bool {
	switch x := v.(type) {
	case nil:
		return true
	case string:
		return x == ""
	case []any:
		return len(x) == 0
	case map[string]any:
		return len(x) == 0
	}
	return false
}

// TestUpdate_NullClearsOptionalField: on every update endpoint, an optional
// field left out of the body stays as it is, and the same field sent as
// JSON null is cleared (#1359).
func TestUpdate_NullClearsOptionalField(t *testing.T) {
	server, targets := nullClearSetup(t)
	citation := strings.TrimPrefix(targets["citation"].get, "/api/v1/citations/")
	person := strings.TrimPrefix(targets["person"].get, "/api/v1/persons/")
	partner, _ := targets["family"].read(t, server)["partner2_id"].(string)

	for _, tc := range []struct {
		target, field, value string
	}{
		{"person", "surname", `"Byron"`},
		{"person", "gender", `"female"`},
		{"person", "birth_date", `"10 DEC 1815"`},
		{"person", "notes", `"Mathematician"`},
		{"person", "research_status", `"probable"`},
		{"family", "partner2_id", fmt.Sprintf("%q", partner)},
		{"family", "marriage_date", `"1835"`},
		{"family", "marriage_place", `"London"`},
		{"media", "description", `"Portrait"`},
		{"media", "crop_left", `10`},
		{"source", "author", `"Clerk"`},
		{"source", "publish_date", `"1900"`},
		{"citation", "page", `"12"`},
		{"citation", "source_quality", `"original"`},
		{"citation", "fields", `{"line":"5"}`},
		{"name", "nickname", `"Ada"`},
		{"branch", "hypothesis", `"Born in London"`},
		{"branch", "subjects", fmt.Sprintf(`[{"type":"person","id":%q}]`, person)},
		{"submitter", "address", `{"city":"London"}`},
		{"submitter", "phone", `["555-0100"]`},
		{"submitter", "language", `"English"`},
		{"repository", "address", `{"city":"Kew","country":"England"}`},
		{"repository", "notes", `"National archive"`},
		{"association", "phrase", `"stood as godparent"`},
		{"lds_ordinance", "place", `"Logan"`},
		{"lds_ordinance", "date", `"1900"`},
		{"evidence_analysis", "citation_ids", fmt.Sprintf("[%q]", citation)},
		{"evidence_analysis", "notes", `"Reasoning"`},
		{"research_log", "notes", `"Searched twice"`},
		{"proof_summary", "research_status", `"probable"`},
	} {
		t.Run(tc.target+"/"+tc.field, func(t *testing.T) {
			tg := targets[tc.target]
			tg.send(t, server, fmt.Sprintf("%q:%s", tc.field, tc.value), http.StatusOK)
			if got := tg.read(t, server)[tc.field]; isCleared(got) {
				t.Fatalf("%s after set = %v, want a value", tc.field, got)
			}

			// Left out: unchanged. (A branch PATCH must name a field.)
			omit := ""
			if tc.target == "branch" {
				omit = `"outcome":"open"`
			}
			tg.send(t, server, omit, http.StatusOK)
			if got := tg.read(t, server)[tc.field]; isCleared(got) {
				t.Errorf("%s after an update that left it out = %v, want it kept", tc.field, got)
			}

			tg.send(t, server, fmt.Sprintf("%q:null", tc.field), http.StatusOK)
			if got := tg.read(t, server)[tc.field]; !isCleared(got) {
				t.Errorf("%s after null = %v, want it cleared", tc.field, got)
			}
		})
	}
}

// TestUpdate_FamilyPartnerNullClears: partner1_id null removes the partner,
// as the deprecated clear_partner1 flag does, and the two together with a
// value are refused.
func TestUpdate_FamilyPartnerNullClears(t *testing.T) {
	server, targets := nullClearSetup(t)
	tg := targets["family"]
	partner1, _ := tg.read(t, server)["partner1_id"].(string)

	tg.send(t, server, fmt.Sprintf(`"partner1_id":%q,"clear_partner1":true`, partner1), http.StatusBadRequest)
	tg.send(t, server, `"partner1_id":null`, http.StatusOK)
	if got := tg.read(t, server)["partner1_id"]; got != nil {
		t.Errorf("partner1_id after null = %v, want none", got)
	}
}

// TestUpdate_NullOnRequiredFieldRejected: a field that cannot be cleared
// refuses JSON null with 400 rather than reading it as left out.
func TestUpdate_NullOnRequiredFieldRejected(t *testing.T) {
	server, targets := nullClearSetup(t)
	for _, tc := range []struct{ target, field string }{
		{"person", "given_name"},
		{"person", "version"},
		{"media", "title"},
		{"source", "title"},
		{"source", "source_type"},
		{"citation", "version"},
		{"name", "given_name"},
		{"name", "is_primary"},
		{"branch", "outcome"},
		{"note", "text"},
		{"submitter", "name"},
		{"repository", "name"},
		{"association", "role"},
		{"lds_ordinance", "version"},
		{"evidence_analysis", "conclusion"},
		{"research_log", "repository"},
		{"research_log", "search_date"},
		{"proof_summary", "argument"},
	} {
		t.Run(tc.target+"/"+tc.field, func(t *testing.T) {
			tg := targets[tc.target]
			want := http.StatusBadRequest
			body := fmt.Sprintf("%q:null", tc.field)
			if tc.field == "version" {
				// The version is added by send; send it as null directly.
				rec := do(t, server, tg.method, tg.update, "{"+body+"}")
				if rec.Code != want {
					t.Fatalf("status = %d, want %d. Body: %s", rec.Code, want, rec.Body.String())
				}
				return
			}
			tg.send(t, server, body, want)
		})
	}
}
