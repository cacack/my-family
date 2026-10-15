package integration_test

import (
	"context"
	"fmt"
	"net/http"
	"net/url"
	"testing"

	"github.com/google/uuid"

	"github.com/cacack/my-family/internal/api"
	"github.com/cacack/my-family/internal/domain"
	"github.com/cacack/my-family/internal/repository"
)

// A person's given name and surname mirror its primary name, whichever side is
// edited (issue #1093; docs/CONVENTIONS.md "Person Name and Primary Name").
// Editing the person once left the primary name behind, and editing the primary
// name left the person (page header, search) behind.
//
// The scenario drives both edits through the HTTP API on every backend, checks
// GET and search after each, then replays the log into a fresh read model and
// requires the person and its names to come out identical.
//
// Fixtures use neutral placeholder names only (public repo - no real PII).

const primaryNameGedcom = `0 HEAD
1 GEDC
2 VERS 5.5.1
1 CHAR UTF-8
0 @I1@ INDI
1 NAME Quinn /Original/
1 NAME Sky /Alias/
2 TYPE aka
1 SEX F
0 TRLR
`

func TestPrimaryNameSync(t *testing.T) {
	for _, backend := range backends {
		t.Run(backend.name, func(t *testing.T) {
			st := backend.setup(t)
			server := newServer(t, st)

			importGedcom(t, server, primaryNameGedcom)
			personID := onlySearchHit(t, server, "Quinn")
			personPath := "/api/v1/persons/" + personID

			// 1. Edit the person: the primary name follows; the alternate does not.
			mustDo(t, server, http.MethodPut, personPath,
				fmt.Sprintf(`{"given_name":"Robin","surname":"Renamed","version":%d}`, entityVersion(t, server, personPath, "")),
				http.StatusOK)
			primaryID := assertPersonNames(t, server, personPath, "Robin", "Renamed")
			assertSearch(t, server, personID, "Quinn", "Robin")

			// 2. Edit the primary name: the person follows.
			mustDo(t, server, http.MethodPut, personPath+"/names/"+primaryID,
				`{"given_name":"Taylor","surname":"Final"}`, http.StatusOK)
			assertPersonNames(t, server, personPath, "Taylor", "Final")
			assertSearch(t, server, personID, "Robin", "Taylor")

			// 3. A rebuild from the log gives the same rows.
			ctx := context.Background()
			fresh := backend.setup(t).read
			replayLog(t, ctx, st.events, fresh)
			id := uuid.MustParse(personID)
			for name, get := range map[string]rowGetter{
				"person": func(ctx context.Context, rs repository.ReadModelStore) (any, error) {
					return rs.GetPerson(ctx, domain.MainBranchID, id)
				},
				"names": func(ctx context.Context, rs repository.ReadModelStore) (any, error) {
					return rs.GetPersonNames(ctx, domain.MainBranchID, id)
				},
			} {
				live, err := get(ctx, st.read)
				if err != nil {
					t.Fatalf("read live %s: %v", name, err)
				}
				replayed, err := get(ctx, fresh)
				if err != nil {
					t.Fatalf("read replayed %s: %v", name, err)
				}
				if l, r := canonicalRowJSON(t, live), canonicalRowJSON(t, replayed); l != r {
					t.Errorf("%s: live projection differs from replay\n live:   %s\n replay: %s", name, l, r)
				}
			}
		})
	}
}

// assertPersonNames checks that GET person reports the given name and surname
// and that its primary name matches them, the alternate name untouched. It
// returns the primary name's id.
func assertPersonNames(t *testing.T, server *api.Server, personPath, given, surname string) string {
	t.Helper()
	person := getEntity(t, server, personPath, "")
	if person["given_name"] != given || person["surname"] != surname {
		t.Errorf("person = %v %v, want %s %s", person["given_name"], person["surname"], given, surname)
	}
	var primaryID string
	for _, raw := range jsonArray(t, person, "names") {
		name := raw.(map[string]any)
		if name["is_primary"] == true {
			primaryID, _ = name["id"].(string)
			if name["given_name"] != given || name["surname"] != surname || name["full_name"] != given+" "+surname {
				t.Errorf("primary name = %v / %v / %v, want %s %s", name["given_name"], name["surname"], name["full_name"], given, surname)
			}
		} else if name["given_name"] != "Sky" || name["surname"] != "Alias" {
			t.Errorf("alternate name = %v %v, want Sky Alias", name["given_name"], name["surname"])
		}
	}
	if primaryID == "" {
		t.Fatalf("person has no primary name: %v", person["names"])
	}
	return primaryID
}

// assertSearch checks that search no longer finds the person by its old given
// name and does find it by the new one.
func assertSearch(t *testing.T, server *api.Server, personID, oldName, newName string) {
	t.Helper()
	if hits := searchIDs(t, server, oldName); contains(hits, personID) {
		t.Errorf("search %q still returns the person: %v", oldName, hits)
	}
	if hits := searchIDs(t, server, newName); !contains(hits, personID) {
		t.Errorf("search %q does not return the person: %v", newName, hits)
	}
}

// onlySearchHit returns the id of the single person a search finds.
func onlySearchHit(t *testing.T, server *api.Server, query string) string {
	t.Helper()
	hits := searchIDs(t, server, query)
	if len(hits) != 1 {
		t.Fatalf("search %q = %v, want one hit", query, hits)
	}
	return hits[0]
}

func searchIDs(t *testing.T, server *api.Server, query string) []string {
	t.Helper()
	return itemIDs(t, mustDo(t, server, http.MethodGet, "/api/v1/search?q="+url.QueryEscape(query), "", http.StatusOK))
}
