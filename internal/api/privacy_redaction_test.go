package api_test

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"slices"
	"strings"
	"testing"

	"github.com/cacack/my-family/internal/repository"
)

// The read-surface table of ADR-008 for people, families, charts, search and
// media, driven through the API as every principal of ADR-008 §4. The tree
// holds a dead person, a private one and two living ones; a visitor, and a
// viewer once the owner turns "viewers see living" off, see the living as
// placeholders and the private one not at all.

// redactionTree is the tree's IDs.
type redactionTree struct {
	dead, private, living, kid, family       string
	livingMedia, deadMedia, deadPrivateMedia string
}

// privacyPrincipal is one way of calling the API, and what it may see.
type privacyPrincipal struct {
	name                    string
	cookie                  *http.Cookie
	seesLiving, seesPrivate bool
}

func buildRedactionTree(t *testing.T, f *authFixture, owner *http.Cookie) redactionTree {
	t.Helper()
	create := func(path, body string) string {
		t.Helper()
		rec := f.do(t, request{method: http.MethodPost, path: path, body: body, cookie: owner})
		if rec.Code != http.StatusCreated {
			t.Fatalf("POST %s: %d %s", path, rec.Code, rec.Body)
		}
		return decode[map[string]any](t, rec)["id"].(string)
	}
	setPrivate := func(path string) {
		t.Helper()
		rec := f.do(t, request{method: http.MethodGet, path: path, cookie: owner})
		version := decode[map[string]any](t, rec)["version"]
		rec = f.do(t, request{method: http.MethodPut, path: path, cookie: owner, body: fmt.Sprintf(`{"private":true,"version":%v}`, version)})
		if rec.Code != http.StatusOK {
			t.Fatalf("mark %s private: %d %s", path, rec.Code, rec.Body)
		}
	}
	upload := func(personID string) string {
		t.Helper()
		req, err := createMultipartRequest("/api/v1/persons/"+personID+"/media", "file", "photo.jpg", createTestJPEGImage(),
			map[string]string{"title": "Photo", "media_type": "photo"})
		if err != nil {
			t.Fatal(err)
		}
		req.AddCookie(owner)
		rec := httptest.NewRecorder()
		f.server.Echo().ServeHTTP(rec, req)
		if rec.Code != http.StatusCreated {
			t.Fatalf("upload: %d %s", rec.Code, rec.Body)
		}
		return decode[map[string]any](t, rec)["id"].(string)
	}

	var tr redactionTree
	tr.dead = create("/api/v1/persons", `{"given_name":"Dora","surname":"Smith","gender":"female","birth_date":"1850","death_date":"1920","birth_place":"Springfield"}`)
	tr.private = create("/api/v1/persons", `{"given_name":"Petra","surname":"Smith","gender":"female","birth_date":"1850","death_date":"1920","birth_place":"Springfield"}`)
	setPrivate("/api/v1/persons/" + tr.private)
	tr.living = create("/api/v1/persons", `{"given_name":"Lena","surname":"Smith","gender":"female","birth_date":"1990","birth_place":"Springfield"}`)
	tr.kid = create("/api/v1/persons", `{"given_name":"Kit","surname":"Smith","gender":"male","birth_date":"2015","birth_place":"Springfield"}`)
	create("/api/v1/persons/"+tr.living+"/names", `{"given_name":"Lena","surname":"Maiden","name_type":"birth"}`)
	tr.family = create("/api/v1/families", fmt.Sprintf(
		`{"partner1_id":%q,"partner2_id":%q,"relationship_type":"marriage","marriage_date":"2012","marriage_place":"Chapel"}`, tr.dead, tr.living))
	if rec := f.do(t, request{method: http.MethodPost, path: "/api/v1/families/" + tr.family + "/children", cookie: owner,
		body: fmt.Sprintf(`{"person_id":%q,"relationship_type":"biological"}`, tr.kid)}); rec.Code != http.StatusCreated {
		t.Fatalf("add child: %d %s", rec.Code, rec.Body)
	}
	tr.livingMedia = upload(tr.living)
	tr.deadMedia = upload(tr.dead)
	tr.deadPrivateMedia = upload(tr.dead)
	setPrivate("/api/v1/media/" + tr.deadPrivateMedia)
	return tr
}

// redactionFixture builds the tree and signs in each principal. settings
// are the owner's privacy settings; visitors are let in.
func redactionFixture(t *testing.T, viewersSeeLiving bool) (*authFixture, redactionTree, []privacyPrincipal) {
	t.Helper()
	f, cookies := rolesFixture(t)
	tr := buildRedactionTree(t, f, cookies[repository.RoleOwner])
	f.server.ProtectVisitorsForTest()
	settings := repository.DefaultTreeSettings()
	settings.AnonymousAccess = repository.AnonymousAccessViewer
	settings.ViewersSeeLiving = viewersSeeLiving
	if err := f.accounts.SaveTreeSettings(context.Background(), repository.DefaultTreeID, settings); err != nil {
		t.Fatalf("SaveTreeSettings: %v", err)
	}
	return f, tr, []privacyPrincipal{
		{"owner", cookies[repository.RoleOwner], true, true},
		{"editor", cookies[repository.RoleEditor], true, true},
		{"viewer", cookies[repository.RoleViewer], viewersSeeLiving, false},
		{"anonymous", nil, false, false},
	}
}

func (p privacyPrincipal) get(t *testing.T, f *authFixture, path string) *httptest.ResponseRecorder {
	t.Helper()
	return f.do(t, request{method: http.MethodGet, path: path, cookie: p.cookie})
}

// getJSON GETs path as p and decodes the body, failing unless the status is
// 200.
func (p privacyPrincipal) getJSON(t *testing.T, f *authFixture, path string) map[string]any {
	t.Helper()
	rec := p.get(t, f, path)
	if rec.Code != http.StatusOK {
		t.Fatalf("%s GET %s: %d %s", p.name, path, rec.Code, rec.Body)
	}
	var out map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &out); err != nil {
		t.Fatalf("decode %s: %v", rec.Body, err)
	}
	return out
}

// itemNames returns the sorted given names of the items of a list response,
// and its total.
func itemNames(body map[string]any) ([]string, float64) {
	var names []string
	items, _ := body["items"].([]any)
	for _, item := range items {
		names = append(names, item.(map[string]any)["given_name"].(string))
	}
	slices.Sort(names)
	total, _ := body["total"].(float64)
	return names, total
}

// shownPeople is the given names p's lists and searches return.
func (p privacyPrincipal) shownPeople() []string {
	names := []string{"Dora"}
	if p.seesLiving {
		names = append(names, "Kit", "Lena")
	}
	if p.seesPrivate {
		names = append(names, "Petra")
	}
	slices.Sort(names)
	return names
}

func TestPrivacyRedaction_PersonSurfaces(t *testing.T) {
	for _, viewersSeeLiving := range []bool{true, false} {
		t.Run(fmt.Sprintf("viewers see living %v", viewersSeeLiving), func(t *testing.T) {
			f, tr, principals := redactionFixture(t, viewersSeeLiving)
			for _, p := range principals {
				t.Run(p.name, func(t *testing.T) {
					want := p.shownPeople()
					for _, path := range []string{
						"/api/v1/persons",                           // listPersons
						"/api/v1/search?q=Smith",                    // searchPersons
						"/api/v1/search?q=Smith&fuzzy=true",         // searchPersons, fuzzy
						"/api/v1/browse/surnames/Smith/persons",     // getPersonsBySurname
						"/api/v1/browse/places/Springfield/persons", // getPersonsByPlace
					} {
						names, total := itemNames(p.getJSON(t, f, path))
						if !slices.Equal(names, want) || int(total) != len(want) {
							t.Errorf("%s: %v (total %v), want %v", path, names, total, want)
						}
					}
					// A hidden name matches nothing, not even its total.
					for _, q := range []string{"Lena", "Maiden", "Petra"} {
						_, total := itemNames(p.getJSON(t, f, "/api/v1/search?q="+q))
						hidden := (q == "Petra" && !p.seesPrivate) || (q != "Petra" && !p.seesLiving)
						if hidden != (total == 0) {
							t.Errorf("search %q: total %v; hidden %v", q, total, hidden)
						}
					}

					// getPerson: the dead person in full, the living one a
					// placeholder, the private one not found.
					if body := p.getJSON(t, f, "/api/v1/persons/"+tr.dead); body["given_name"] != "Dora" || body["redacted"] != nil {
						t.Errorf("dead person = %v", body)
					}
					living := p.getJSON(t, f, "/api/v1/persons/"+tr.living)
					if p.seesLiving {
						if living["given_name"] != "Lena" || living["redacted"] != nil {
							t.Errorf("living person = %v, want her in full", living)
						}
					} else {
						for _, key := range []string{"surname", "birth_date", "birth_place", "names", "notes"} {
							if v, ok := living[key]; ok && v != "" && v != nil {
								t.Errorf("living placeholder shows %s = %v", key, v)
							}
						}
						if living["given_name"] != "Living" || living["redacted"] != true || living["gender"] != "female" {
							t.Errorf("living person = %v, want the placeholder", living)
						}
					}
					if rec := p.get(t, f, "/api/v1/persons/"+tr.private); (rec.Code == http.StatusOK) != p.seesPrivate {
						t.Errorf("private person: %d, want shown %v", rec.Code, p.seesPrivate)
					}

					// Sub-resources of a protected person are empty; its quality is not found.
					names := p.getJSON(t, f, "/api/v1/persons/"+tr.living+"/names")
					if items, _ := names["items"].([]any); (len(items) > 0) != p.seesLiving {
						t.Errorf("living person's names = %v, want shown %v", names, p.seesLiving)
					}
					media := p.getJSON(t, f, "/api/v1/persons/"+tr.living+"/media")
					if items, _ := media["items"].([]any); (len(items) > 0) != p.seesLiving {
						t.Errorf("living person's media = %v, want shown %v", media, p.seesLiving)
					}
					if rec := p.get(t, f, "/api/v1/quality/persons/"+tr.living); (rec.Code == http.StatusOK) != p.seesLiving {
						t.Errorf("living person's quality: %d, want shown %v", rec.Code, p.seesLiving)
					}

					// The event log is not redacted yet: a protected person's
					// is empty, and a private person's sub-resources are not
					// found, as their page is not. Visitors never reach the
					// change log (TestAnonymous_ChangeLogNeedsSignIn).
					logSubs := []string{"history", "restore-points"}
					if p.cookie == nil {
						logSubs = nil
					}
					for _, sub := range logSubs {
						log := p.getJSON(t, f, "/api/v1/persons/"+tr.living+"/"+sub)
						if items, _ := log["items"].([]any); (len(items) > 0) != p.seesLiving {
							t.Errorf("living person's %s = %v, want shown %v", sub, log, p.seesLiving)
						}
					}
					for _, sub := range append(logSubs, "media", "associations") {
						if rec := p.get(t, f, "/api/v1/persons/"+tr.private+"/"+sub); (rec.Code == http.StatusOK) != p.seesPrivate {
							t.Errorf("private person's %s: %d, want shown %v", sub, rec.Code, p.seesPrivate)
						}
					}
				})
			}
		})
	}
}

// Media content and thumbnails of a protected person, and private media,
// are not found (ADR-008 §5).
func TestPrivacyRedaction_MediaBytes(t *testing.T) {
	for _, viewersSeeLiving := range []bool{true, false} {
		t.Run(fmt.Sprintf("viewers see living %v", viewersSeeLiving), func(t *testing.T) {
			f, tr, principals := redactionFixture(t, viewersSeeLiving)
			for _, p := range principals {
				for media, shown := range map[string]bool{
					tr.deadMedia:        true,
					tr.livingMedia:      p.seesLiving,
					tr.deadPrivateMedia: p.seesPrivate,
				} {
					for _, path := range []string{"/api/v1/media/" + media, "/api/v1/media/" + media + "/content", "/api/v1/media/" + media + "/thumbnail"} {
						rec := p.get(t, f, path)
						if shown && rec.Code != http.StatusOK || !shown && rec.Code != http.StatusNotFound {
							t.Errorf("%s GET %s: %d, want shown %v", p.name, path, rec.Code, shown)
						}
					}
				}
				dead := p.getJSON(t, f, "/api/v1/persons/"+tr.dead+"/media")
				want := 1
				if p.seesPrivate {
					want = 2
				}
				if items, _ := dead["items"].([]any); len(items) != want || dead["total"] != float64(want) {
					t.Errorf("%s: the dead person's media = %v, want %d", p.name, dead, want)
				}
			}
		})
	}
}

// A family with a living partner keeps its shape; the partner is the
// placeholder and the family's own facts are hidden. Charts keep the slot.
func TestPrivacyRedaction_FamiliesAndCharts(t *testing.T) {
	f, tr, principals := redactionFixture(t, false)
	for _, p := range principals {
		t.Run(p.name, func(t *testing.T) {
			family := p.getJSON(t, f, "/api/v1/families/"+tr.family)
			if p.seesLiving {
				if family["marriage_place"] != "Chapel" {
					t.Errorf("family = %v, want it in full", family)
				}
			} else if family["partner1_id"] != tr.dead || family["partner2_id"] != tr.living ||
				family["marriage_date"] != nil || family["marriage_place"] != nil {
				t.Errorf("family = %v, want its partners kept and its facts hidden", family)
			}
			// The event log is not redacted yet: a family with a protected
			// partner has none to show.
			for _, sub := range []string{"history", "restore-points"} {
				if rec := p.get(t, f, "/api/v1/families/"+tr.family+"/"+sub); (rec.Code == http.StatusOK) != p.seesLiving {
					t.Errorf("family %s: %d, want shown %v", sub, rec.Code, p.seesLiving)
				}
			}
			for _, path := range []string{
				"/api/v1/families/" + tr.family,
				"/api/v1/families/" + tr.family + "/group-sheet",
				"/api/v1/pedigree/" + tr.kid,
				"/api/v1/descendancy/" + tr.dead,
				"/api/v1/families",
				"/api/v1/relationship/" + tr.dead + "/" + tr.kid,
			} {
				rec := p.get(t, f, path)
				if rec.Code != http.StatusOK {
					t.Fatalf("GET %s: %d %s", path, rec.Code, rec.Body)
				}
				if !p.seesLiving && (containsAny(rec.Body.String(), "Lena", "Kit", "1990", "2015", "Chapel")) {
					t.Errorf("GET %s shows a living person: %s", path, rec.Body)
				}
			}
		})
	}
}
func containsAny(s string, subs ...string) bool {
	return slices.ContainsFunc(subs, func(sub string) bool { return strings.Contains(s, sub) })
}

// A placeholder is marked redacted wherever a chart or family shows it, a
// private parent reached through a visible family keeps its slot as the
// placeholder, and a private family's parentage is a gap in the pedigree
// (ADR-008 §3).
func TestPrivacyRedaction_ChartPlaceholders(t *testing.T) {
	f, tr, principals := redactionFixture(t, false)
	owner := principals[0].cookie
	send := func(method, path, body string, want int) map[string]any {
		t.Helper()
		rec := f.do(t, request{method: method, path: path, body: body, cookie: owner})
		if rec.Code != want {
			t.Fatalf("%s %s: %d %s", method, path, rec.Code, rec.Body)
		}
		return decode[map[string]any](t, rec)
	}
	family := func(p1, p2, child string) string {
		t.Helper()
		id := send(http.MethodPost, "/api/v1/families", fmt.Sprintf(`{"partner1_id":%q,"partner2_id":%q}`, p1, p2), http.StatusCreated)["id"].(string)
		send(http.MethodPost, "/api/v1/families/"+id+"/children", fmt.Sprintf(`{"person_id":%q}`, child), http.StatusCreated)
		return id
	}
	person := func(given string) string {
		return send(http.MethodPost, "/api/v1/persons", fmt.Sprintf(`{"given_name":%q,"surname":"Jones","gender":"male","birth_date":"1880","death_date":"1950"}`, given), http.StatusCreated)["id"].(string)
	}
	// Gus's mother is the private Petra; Hal's parents are recorded in a
	// private family.
	gus, hal := person("Gus"), person("Hal")
	family(tr.dead, tr.private, gus)
	secret := family(tr.dead, gus, hal)
	version := send(http.MethodGet, "/api/v1/families/"+secret, "", http.StatusOK)["version"]
	send(http.MethodPut, "/api/v1/families/"+secret, fmt.Sprintf(`{"private":true,"version":%v}`, version), http.StatusOK)

	redacted := func(node any) bool {
		m, _ := node.(map[string]any)
		return m["redacted"] == true
	}
	// parent returns the father or mother of a pedigree node that is id: the
	// slots follow sex, and the tree's women share one.
	parent := func(node map[string]any, id string) map[string]any {
		for _, slot := range []string{"father", "mother"} {
			if m, _ := node[slot].(map[string]any); m != nil && m["id"] == id {
				return m
			}
		}
		return nil
	}
	for _, p := range principals {
		t.Run(p.name, func(t *testing.T) {
			kid := p.getJSON(t, f, "/api/v1/pedigree/"+tr.kid)["root"].(map[string]any)
			mother := parent(kid, tr.living)
			if mother == nil || redacted(mother) == p.seesLiving || redacted(parent(kid, tr.dead)) || (!p.seesLiving && mother["birth_date"] != nil) {
				t.Errorf("pedigree(kid) = %v; want the living mother redacted %v, the dead one in full", kid, !p.seesLiving)
			}

			gus := p.getJSON(t, f, "/api/v1/pedigree/"+gus)["root"].(map[string]any)
			if mother := parent(gus, tr.private); mother == nil || redacted(mother) == p.seesPrivate {
				t.Errorf("pedigree(gus) = %v; want the private mother, redacted %v", gus, !p.seesPrivate)
			}

			hal := p.getJSON(t, f, "/api/v1/pedigree/"+hal)["root"].(map[string]any)
			if hasParents := hal["father"] != nil || hal["mother"] != nil; hasParents != p.seesPrivate {
				t.Errorf("pedigree(hal) = %v; want parents shown %v", hal, p.seesPrivate)
			}

			root := p.getJSON(t, f, "/api/v1/descendancy/"+tr.dead)["root"].(map[string]any)
			for _, s := range root["spouses"].([]any) {
				spouse := s.(map[string]any)
				if spouse["id"] == tr.living && redacted(spouse) == p.seesLiving {
					t.Errorf("descendancy spouse = %v; want redacted %v", spouse, !p.seesLiving)
				}
			}
			for _, c := range root["children"].([]any) {
				child := c.(map[string]any)
				if child["id"] == tr.kid && redacted(child) == p.seesLiving {
					t.Errorf("descendancy child = %v; want redacted %v", child, !p.seesLiving)
				}
			}

			fam := p.getJSON(t, f, "/api/v1/families/"+tr.family)
			child := fam["children"].([]any)[0].(map[string]any)["person"]
			if redacted(fam["partner2"]) == p.seesLiving || redacted(child) == p.seesLiving || redacted(fam["partner1"]) {
				t.Errorf("family = %v; want the living partner and kid redacted %v", fam, !p.seesLiving)
			}

			sheet := p.getJSON(t, f, "/api/v1/families/"+tr.family+"/group-sheet")
			if child := sheet["children"].([]any)[0]; redacted(child) == p.seesLiving {
				t.Errorf("group sheet child = %v; want redacted %v", child, !p.seesLiving)
			}
		})
	}
}
