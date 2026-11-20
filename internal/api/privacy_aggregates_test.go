package api_test

import (
	"context"
	"fmt"
	"net/http"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/cacack/my-family/internal/domain"
	"github.com/cacack/my-family/internal/repository"
)

// The browse indexes, the map, brick walls and the home page widgets
// (statistics, discovery, quality) of ADR-008's read-surface table, driven
// through the API as every principal of ADR-008 §4. They count and name
// people, so they leave protected people out, counts included (ADR-008 §5,
// "Counts" and "Home widgets").

// placeTree gives each person of the redaction tree a birthplace of their
// own under Springfield, pinned on the map, a burial in a churchyard of their
// own and a brick wall. The places are written to the read model: the API
// takes no coordinates and writes no burials.
func placeTree(t *testing.T, f *authFixture, owner *http.Cookie, tr redactionTree) {
	t.Helper()
	ctx := context.Background()
	lat, long := "N42.3601", "W71.0589"
	for _, id := range []string{tr.dead, tr.private, tr.living, tr.kid} {
		if rec := f.do(t, request{method: http.MethodPut, path: "/api/v1/persons/" + id + "/brick-wall", cookie: owner,
			body: `{"note":"no record"}`}); rec.Code != http.StatusNoContent {
			t.Fatalf("set brick wall: %d %s", rec.Code, rec.Body)
		}
		p, err := f.read.GetPerson(ctx, domain.MainBranchID, uuid.MustParse(id))
		if err != nil || p == nil {
			t.Fatalf("GetPerson %s: %v", id, err)
		}
		p.BirthPlace, p.BirthPlaceLat, p.BirthPlaceLong = p.GivenName+"ville, Springfield", &lat, &long
		if err := f.read.SavePerson(ctx, domain.MainBranchID, p); err != nil {
			t.Fatalf("SavePerson: %v", err)
		}
		burial := &repository.EventReadModel{ID: uuid.New(), OwnerType: "person", OwnerID: p.ID,
			FactType: domain.FactPersonBurial, Place: p.GivenName + " Churchyard", Version: 1, CreatedAt: time.Now()}
		if err := f.read.SaveEvent(ctx, domain.MainBranchID, burial); err != nil {
			t.Fatalf("SaveEvent: %v", err)
		}
	}
}

// hiddenPeople is the given names of the redaction tree p may not see.
func (p privacyPrincipal) hiddenPeople() []string {
	var hidden []string
	for _, name := range []string{"Dora", "Kit", "Lena", "Petra"} {
		if !slices.Contains(p.shownPeople(), name) {
			hidden = append(hidden, name)
		}
	}
	return hidden
}

// suffixed returns each of names followed by suffix.
func suffixed(names []string, suffix string) []string {
	out := make([]string, len(names))
	for i, n := range names {
		out[i] = n + suffix
	}
	return out
}

// listed returns the sorted values of key in the objects under list in body.
func listed(body map[string]any, list, key string) []string {
	var out []string
	items, _ := body[list].([]any)
	for _, item := range items {
		if v, ok := item.(map[string]any)[key].(string); ok {
			out = append(out, v)
		}
	}
	slices.Sort(out)
	return out
}

// countOf returns the count of the object under list in body whose key is
// value, or 0.
func countOf(body map[string]any, list, key, value string) float64 {
	items, _ := body[list].([]any)
	for _, item := range items {
		if m := item.(map[string]any); m[key] == value {
			n, _ := m["count"].(float64)
			return n
		}
	}
	return 0
}

func TestPrivacyRedaction_AggregatesAndHomeWidgets(t *testing.T) {
	for _, viewersSeeLiving := range []bool{true, false} {
		t.Run(fmt.Sprintf("viewers see living %v", viewersSeeLiving), func(t *testing.T) {
			f, tr, principals := redactionFixture(t, viewersSeeLiving)
			placeTree(t, f, principals[0].cookie, tr)
			for _, p := range principals {
				t.Run(p.name, func(t *testing.T) {
					shown, hidden := p.shownPeople(), p.hiddenPeople()
					n := float64(len(shown))

					// No surface names a hidden person or their places.
					for _, path := range []string{
						"/api/v1/browse/surnames",          // browseSurnames
						"/api/v1/browse/surnames?letter=S", // browseSurnames by letter
						"/api/v1/browse/places",            // browsePlaces
						"/api/v1/browse/places?parent=Springfield",
						"/api/v1/browse/cemeteries",   // browseCemeteries
						"/api/v1/map/locations",       // getMapLocations
						"/api/v1/browse/brick-walls",  // getBrickWalls
						"/api/v1/statistics",          // getStatistics
						"/api/v1/analytics/discovery", // getDiscoveryFeed
						"/api/v1/quality/overview",    // getQualityOverview
						"/api/v1/quality/report",      // getQualityReport
					} {
						rec := p.get(t, f, path)
						if rec.Code != http.StatusOK {
							t.Fatalf("GET %s: %d %s", path, rec.Code, rec.Body)
						}
						if len(hidden) > 0 && containsAny(rec.Body.String(), hidden...) {
							t.Errorf("GET %s names one of %v: %s", path, hidden, rec.Body)
						}
					}

					// Counts leave them out too, and the shown people are there.
					for _, path := range []string{"/api/v1/browse/surnames", "/api/v1/browse/surnames?letter=S"} {
						if got := countOf(p.getJSON(t, f, path), "items", "surname", "Smith"); got != n {
							t.Errorf("%s: Smith counts %v, want %v", path, got, n)
						}
					}
					// The memory store behind the API lists every place in full,
					// so the places under Springfield are told apart by full name.
					var places []string
					for _, full := range listed(p.getJSON(t, f, "/api/v1/browse/places?parent=Springfield"), "items", "full_name") {
						if place, ok := strings.CutSuffix(full, ", Springfield"); ok {
							places = append(places, place)
						}
					}
					if want := suffixed(shown, "ville"); !slices.Equal(places, want) {
						t.Errorf("browsePlaces under Springfield: %v, want %v", places, want)
					}
					if got, want := listed(p.getJSON(t, f, "/api/v1/browse/cemeteries"), "items", "place"), suffixed(shown, " Churchyard"); !slices.Equal(got, want) {
						t.Errorf("browseCemeteries: %v, want %v", got, want)
					}
					if got, want := listed(p.getJSON(t, f, "/api/v1/map/locations"), "items", "place"), suffixed(shown, "ville, Springfield"); !slices.Equal(got, want) {
						t.Errorf("getMapLocations: %v, want %v", got, want)
					}
					walls := p.getJSON(t, f, "/api/v1/browse/brick-walls")
					if got, want := listed(walls, "items", "person_name"), suffixed(shown, " Smith"); !slices.Equal(got, want) || walls["active_count"] != n {
						t.Errorf("getBrickWalls: %v (active %v), want %v", got, walls["active_count"], want)
					}

					stats := p.getJSON(t, f, "/api/v1/statistics")
					latest := "1850"
					if p.seesLiving {
						latest = "2015"
					}
					if got := stats["date_range"].(map[string]any)["latest_birth"]; got != latest {
						t.Errorf("getStatistics: latest birth %v, want %v", got, latest)
					}
					if got := countOf(stats, "top_surnames", "surname", "Smith"); got != n {
						t.Errorf("getStatistics: Smith counts %v, want %v", got, n)
					}
					if got := p.getJSON(t, f, "/api/v1/quality/report")["total_individuals"]; got != n {
						t.Errorf("getQualityReport: %v individuals, want %v", got, n)
					}
					if got := listed(p.getJSON(t, f, "/api/v1/analytics/discovery"), "items", "person_name"); !slices.Contains(got, "Dora Smith") {
						t.Errorf("getDiscoveryFeed names %v, want Dora Smith among them", got)
					}
				})
			}
		})
	}
}
