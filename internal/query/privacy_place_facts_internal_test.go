package query

import (
	"context"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/cacack/my-family/internal/domain"
	"github.com/cacack/my-family/internal/repository"
	"github.com/cacack/my-family/internal/repository/memory"
)

// TestPrivacyStore_PlaceFacts is PL-006 at the decorator: a principal gets
// the facts of a place only when a record they may see names it, both from
// ListPlaceFacts and GetPlaceFacts, and the resolver built on it resolves a
// hidden place's text to its ID alone.
func TestPrivacyStore_PlaceFacts(t *testing.T) {
	ctx := context.Background()
	raw := memory.NewReadModelStore()
	now := time.Now().UTC()
	lat, long := "N1", "E1"
	for _, p := range []*repository.PersonReadModel{
		{ID: uuid.New(), GivenName: "Dora", BirthPlace: "Boston, MA", DeathEvidence: true, Version: 1, UpdatedAt: now},
		{ID: uuid.New(), GivenName: "Lena", BirthPlace: "Livingston, MA", Version: 1, UpdatedAt: now},
	} {
		if err := raw.SavePerson(ctx, domain.MainBranchID, p); err != nil {
			t.Fatal(err)
		}
	}
	store := NewPrivacyStore(raw)
	for _, key := range []string{"boston,ma", "livingston,ma"} {
		if err := store.SavePlaceFacts(ctx, &repository.PlaceFactsReadModel{ID: repository.PlaceID(key), Key: key,
			Name: key + " named", Latitude: &lat, Longitude: &long, Version: 1, UpdatedAt: now}); err != nil {
			t.Fatal(err)
		}
	}
	boston, livingston := repository.PlaceID("boston,ma"), repository.PlaceID("livingston,ma")

	keys := func(ctx context.Context) []string {
		t.Helper()
		facts, err := store.ListPlaceFacts(ctx, domain.MainBranchID)
		if err != nil {
			t.Fatal(err)
		}
		var out []string
		for _, f := range facts {
			out = append(out, f.Key)
		}
		return out
	}
	owner := repository.ContextWithPrincipal(ctx, repository.Principal{Role: repository.RoleOwner})
	if got := keys(owner); len(got) != 2 {
		t.Errorf("owner ListPlaceFacts = %v, want both places", got)
	}
	viewer := strictViewerCtx(false)
	if got := keys(viewer); len(got) != 1 || got[0] != "boston,ma" {
		t.Errorf("viewer ListPlaceFacts = %v, want boston,ma only", got)
	}
	if f, err := store.GetPlaceFacts(viewer, boston); err != nil || f == nil {
		t.Errorf("viewer GetPlaceFacts(boston) = %v, %v; want its facts", f, err)
	}
	if f, err := store.GetPlaceFacts(viewer, livingston); err != nil || f != nil {
		t.Errorf("viewer GetPlaceFacts(livingston) = %+v, %v; want none", f, err)
	}
	if f, err := store.GetPlaceFacts(owner, livingston); err != nil || f == nil {
		t.Errorf("owner GetPlaceFacts(livingston) = %v, %v; want its facts", f, err)
	}

	resolver, err := NewPlaceResolver(viewer, store, domain.MainBranchID)
	if err != nil {
		t.Fatal(err)
	}
	text := "Livingston, MA"
	if got := resolver.Resolve(&text); got == nil || got.ID != livingston || got.Name != "" || got.Latitude != nil {
		t.Errorf("viewer Resolve(Livingston) = %+v, want its ID alone", got)
	}
	text = "Boston,MA"
	if got := resolver.Resolve(&text); got == nil || got.Name != "boston,ma named" || got.Latitude == nil || got.Origin == nil {
		t.Errorf("viewer Resolve(Boston) = %+v, want its name, pair and origin", got)
	}
	blank := " , "
	if got := resolver.Resolve(&blank); got != nil {
		t.Errorf("Resolve(blank) = %+v, want nil", got)
	}
}
