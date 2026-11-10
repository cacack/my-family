package query_test

import (
	"context"
	"testing"

	"github.com/google/uuid"

	"github.com/cacack/my-family/internal/domain"
	"github.com/cacack/my-family/internal/query"
	"github.com/cacack/my-family/internal/repository"
	"github.com/cacack/my-family/internal/repository/memory"
)

// Place coordinates from the read model reach person and family query results,
// and unset or empty ones stay nil so the API omits them (#1278).

func TestGetPerson_PlaceCoordinates(t *testing.T) {
	ctx := context.Background()
	readStore := memory.NewReadModelStore()
	lat, long, empty := "N42.3601", "W71.0589", ""
	mapped := &repository.PersonReadModel{ID: uuid.New(), GivenName: "John", Surname: "Mapped",
		BirthPlaceLat: &lat, BirthPlaceLong: &long, DeathPlaceLat: &long, DeathPlaceLong: &lat}
	plain := &repository.PersonReadModel{ID: uuid.New(), GivenName: "Mary", Surname: "Plain",
		BirthPlaceLat: &empty}
	for _, p := range []*repository.PersonReadModel{mapped, plain} {
		if err := readStore.SavePerson(ctx, domain.MainBranchID, p); err != nil {
			t.Fatalf("SavePerson: %v", err)
		}
	}
	service := query.NewPersonService(readStore)

	got, err := service.GetPerson(ctx, domain.MainBranchID, mapped.ID)
	if err != nil {
		t.Fatalf("GetPerson: %v", err)
	}
	assertStrings(t, "mapped person",
		[]*string{got.BirthPlaceLat, got.BirthPlaceLong, got.DeathPlaceLat, got.DeathPlaceLong},
		[]*string{&lat, &long, &long, &lat})

	got, err = service.GetPerson(ctx, domain.MainBranchID, plain.ID)
	if err != nil {
		t.Fatalf("GetPerson: %v", err)
	}
	assertStrings(t, "plain person",
		[]*string{got.BirthPlaceLat, got.BirthPlaceLong, got.DeathPlaceLat, got.DeathPlaceLong},
		[]*string{nil, nil, nil, nil})
}

func TestGetFamily_PlaceCoordinates(t *testing.T) {
	ctx := context.Background()
	readStore := memory.NewReadModelStore()
	lat, long, empty := "N39.7817", "W89.6501", ""
	mapped := &repository.FamilyReadModel{ID: uuid.New(), MarriagePlaceLat: &lat, MarriagePlaceLong: &long}
	plain := &repository.FamilyReadModel{ID: uuid.New(), MarriagePlaceLong: &empty}
	for _, f := range []*repository.FamilyReadModel{mapped, plain} {
		if err := readStore.SaveFamily(ctx, domain.MainBranchID, f); err != nil {
			t.Fatalf("SaveFamily: %v", err)
		}
	}
	service := query.NewFamilyService(readStore)

	got, err := service.GetFamily(ctx, domain.MainBranchID, mapped.ID)
	if err != nil {
		t.Fatalf("GetFamily: %v", err)
	}
	assertStrings(t, "mapped family", []*string{got.MarriagePlaceLat, got.MarriagePlaceLong}, []*string{&lat, &long})

	got, err = service.GetFamily(ctx, domain.MainBranchID, plain.ID)
	if err != nil {
		t.Fatalf("GetFamily: %v", err)
	}
	assertStrings(t, "plain family", []*string{got.MarriagePlaceLat, got.MarriagePlaceLong}, []*string{nil, nil})
}

// assertStrings compares optional strings by value, nil matching only nil.
func assertStrings(t *testing.T, label string, got, want []*string) {
	t.Helper()
	for i := range want {
		switch {
		case want[i] == nil && got[i] != nil:
			t.Errorf("%s[%d] = %q, want nil", label, i, *got[i])
		case want[i] != nil && (got[i] == nil || *got[i] != *want[i]):
			t.Errorf("%s[%d] = %v, want %q", label, i, got[i], *want[i])
		}
	}
}
