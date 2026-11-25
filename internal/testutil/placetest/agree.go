package placetest

import (
	"context"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/cacack/my-family/internal/domain"
	"github.com/cacack/my-family/internal/repository"
)

// RunPlaceReadsAgree is the #1812 scenario: the place browser and the list of
// places walk one hierarchy, explicit parents included (ADR-009 §4), for every
// principal. Every place the browser shows at any level is on the list, which
// is what a GET of a place reads, with the same count while each person has
// one place and no other record; and an explicit parent that only hidden
// records name is shown by both, because a visible place stated under it,
// named by its key and counting only visible records (ADR-009 §10, PL-006).
// store must start empty.
func RunPlaceReadsAgree(t *testing.T, store repository.ReadModelStore) {
	t.Helper()
	ctx := context.Background()
	now := time.Now().UTC().Truncate(time.Second)
	main := domain.MainBranchID
	id := repository.PlaceID

	for _, p := range []struct {
		name, birth string
		restriction domain.Restriction
		dead        bool
	}{
		{"Ann", "Springfield, Sangamon", "", true},
		// Pam is private and the only record that names Illinois itself.
		{"Pam", "Illinois, USA", "confidential", true},
		{"Liv", "Chicago, Illinois, USA", "", false},
		{"Bo", "Boston, MA", "", true},
	} {
		if err := store.SavePerson(ctx, main, &repository.PersonReadModel{
			ID: uuid.New(), GivenName: p.name, Surname: "Agree", FullName: p.name + " Agree",
			BirthPlace: p.birth, Restriction: p.restriction, DeathEvidence: p.dead, Version: 1, UpdatedAt: now,
		}); err != nil {
			t.Fatalf("SavePerson(%s): %v", p.name, err)
		}
	}
	if err := repository.NewProjector(store, nil).Project(ctx, domain.NewPlaceUpdated(id("springfield,sangamon"),
		"springfield,sangamon", "Springfield, Sangamon", map[string]any{"parent": "illinois,usa"}), 1, main); err != nil {
		t.Fatalf("project the parent: %v", err)
	}

	everyone := repository.Privacy{}
	viewer := repository.Privacy{HidePrivate: true, HideLiving: true, LivingCutoff: time.Now()}
	for _, tt := range []struct {
		label    string
		privacy  repository.Privacy
		illinois string // Illinois's name on the list
		counts   map[string]int
	}{
		{"everyone", everyone, "Illinois, USA", map[string]int{
			"usa": 3, "illinois,usa": 3, "chicago,illinois,usa": 1, "springfield,sangamon": 1,
			"ma": 1, "boston,ma": 1,
		}},
		// Only Ann and Bo: Illinois is above Ann's Springfield and no visible
		// record spells it.
		{"viewer", viewer, "illinois,usa", map[string]int{
			"usa": 1, "illinois,usa": 1, "springfield,sangamon": 1, "ma": 1, "boston,ma": 1,
		}},
	} {
		places, err := store.ListPlaces(ctx, main, tt.privacy)
		if err != nil {
			t.Fatalf("%s: ListPlaces: %v", tt.label, err)
		}
		listed := make(map[uuid.UUID]repository.PlaceReadModel, len(places))
		got := make(map[string]int, len(places))
		for _, p := range places {
			listed[p.ID] = p
			got[p.Key] = p.Count
		}
		if len(got) != len(tt.counts) {
			t.Errorf("%s: ListPlaces counts = %v, want %v", tt.label, got, tt.counts)
		}
		for key, want := range tt.counts {
			if got[key] != want {
				t.Errorf("%s: ListPlaces counts %s %d, want %d (all: %v)", tt.label, key, got[key], want, got)
			}
		}
		if name := listed[id("illinois,usa")].Name; name != tt.illinois {
			t.Errorf("%s: Illinois is named %q on the list, want %q", tt.label, name, tt.illinois)
		}

		shown := 0
		var walk func(parent uuid.UUID)
		walk = func(parent uuid.UUID) {
			level, err := store.GetPlaceHierarchy(ctx, main, parent, tt.privacy)
			if err != nil {
				t.Fatalf("%s: GetPlaceHierarchy(%s): %v", tt.label, parent, err)
			}
			for _, e := range level.Places {
				shown++
				p, ok := listed[e.ID]
				if !ok {
					t.Errorf("%s: the browser shows %s, which the list does not hold", tt.label, e.FullName)
				} else if p.Count != e.Count {
					t.Errorf("%s: %s counts %d in the browser and %d on the list", tt.label, e.FullName, e.Count, p.Count)
				}
				if e.HasChildren {
					walk(e.ID)
				}
			}
		}
		walk(uuid.Nil)
		if shown != len(places) {
			t.Errorf("%s: the browser shows %d places and the list holds %d", tt.label, shown, len(places))
		}
	}
}
