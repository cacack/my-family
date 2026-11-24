package placetest

import (
	"context"
	"reflect"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/cacack/my-family/internal/domain"
	"github.com/cacack/my-family/internal/repository"
)

// RunPlaceList is the #1734 scenario: ListPlaces derives the places in use
// from all five place-bearing rows with IDs and counts, the same on every
// store; a viewer's list holds nothing that only hidden records name; and a
// place named only on a research branch is listed on that branch alone, with
// the ID it will have after a merge (ADR-009 §2, §6, §10). store must start
// empty.
func RunPlaceList(t *testing.T, store repository.ReadModelStore) {
	t.Helper()
	branch := seedPlaceList(t, store)
	place := func(key, name string, count int) repository.PlaceReadModel {
		return repository.PlaceReadModel{ID: repository.PlaceID(key), Key: key, Name: name, Spelling: name, Count: count}
	}

	// Everyone: every use counts, toward its place and each place above it.
	// "Boston, MA" (birth and burial) outweighs " BOSTON ,ma " (death).
	all := []repository.PlaceReadModel{
		place("boston,ma", "Boston, MA", 3),
		place("hidden chapel,ma", "Hidden Chapel, MA", 1),
		place("hidden court,ma", "Hidden Court, MA", 1),
		place("livingston,ma", "Livingston, MA", 1),
		place("ma", "MA", 13),
		place("oak hill,boston,ma", "Oak Hill, Boston, MA", 1),
		place("private hall,ma", "Private Hall, MA", 1),
		place("salem court,ma", "Salem Court, MA", 1),
		place("salem,ma", "Salem, MA", 1),
		place("secret cemetery,ma", "Secret Cemetery, MA", 1),
		place("secret farm,ma", "Secret Farm, MA", 1),
		place("secret,ma", "Secret, MA", 1),
		place("worcester,ma", "Worcester, MA", 1),
	}
	checkPlaces(t, store, domain.MainBranchID, repository.Privacy{}, all)

	// A viewer who sees neither private nor living records: only Ann, her
	// own family and their records remain, and the counts and names come from
	// those alone.
	viewer := repository.Privacy{HidePrivate: true, HideLiving: true, LivingCutoff: time.Now()}
	checkPlaces(t, store, domain.MainBranchID, viewer, []repository.PlaceReadModel{
		place("boston,ma", "Boston, MA", 3),
		place("ma", "MA", 6),
		place("oak hill,boston,ma", "Oak Hill, Boston, MA", 1),
		place("salem court,ma", "Salem Court, MA", 1),
		place("salem,ma", "Salem, MA", 1),
		place("worcester,ma", "Worcester, MA", 1),
	})

	// The branch sees main's places and its own; main does not see the
	// branch's, which has the ID a merge will give it.
	onBranch := append([]repository.PlaceReadModel{all[0], place("branchville,ma", "Branchville, MA", 1)}, all[1:]...)
	onBranch[5] = place("ma", "MA", 14)
	checkPlaces(t, store, branch, repository.Privacy{}, onBranch)
}

// checkPlaces compares ListPlaces on branch under privacy with want.
func checkPlaces(t *testing.T, store repository.ReadModelStore, branch domain.BranchID, privacy repository.Privacy, want []repository.PlaceReadModel) {
	t.Helper()
	got, err := store.ListPlaces(context.Background(), branch, privacy)
	if err != nil {
		t.Fatalf("ListPlaces(%s, %+v): %v", branch, privacy, err)
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("ListPlaces(%s, %+v) =\n  %+v\nwant\n  %+v", branch, privacy, got, want)
	}
}

// seedPlaceList saves RunPlaceList's records on main, and one person on a
// new branch, which it returns. Ann is dead and shown; Pam is private; Liv is
// living. Ann's families are one of her own, one with Pam and one marked
// private.
func seedPlaceList(t *testing.T, store repository.ReadModelStore) domain.BranchID {
	t.Helper()
	ctx := context.Background()
	now := time.Now().UTC().Truncate(time.Second)
	main := domain.MainBranchID

	person := func(branch domain.BranchID, name, birth, death string, restriction domain.Restriction, dead bool) uuid.UUID {
		id := uuid.New()
		if err := store.SavePerson(ctx, branch, &repository.PersonReadModel{
			ID: id, GivenName: name, Surname: "Placetest", FullName: name + " Placetest",
			BirthPlace: birth, DeathPlace: death, Restriction: restriction, DeathEvidence: dead,
			Version: 1, UpdatedAt: now,
		}); err != nil {
			t.Fatalf("SavePerson(%s): %v", name, err)
		}
		return id
	}
	event := func(ownerType string, owner uuid.UUID, fact domain.FactType, place string) {
		if err := store.SaveEvent(ctx, main, &repository.EventReadModel{
			ID: uuid.New(), OwnerType: ownerType, OwnerID: owner, FactType: fact,
			Place: place, Version: 1, CreatedAt: now,
		}); err != nil {
			t.Fatalf("SaveEvent(%s): %v", place, err)
		}
	}
	attribute := func(owner uuid.UUID, place string) {
		if err := store.SaveAttribute(ctx, main, &repository.AttributeReadModel{
			ID: uuid.New(), PersonID: owner, FactType: domain.FactPersonResidence,
			Place: place, Version: 1, CreatedAt: now,
		}); err != nil {
			t.Fatalf("SaveAttribute(%s): %v", place, err)
		}
	}
	family := func(partner1 uuid.UUID, partner2 *uuid.UUID, marriage string, restriction domain.Restriction) uuid.UUID {
		id := uuid.New()
		if err := store.SaveFamily(ctx, main, &repository.FamilyReadModel{
			ID: id, Partner1ID: &partner1, Partner2ID: partner2, MarriagePlace: marriage,
			Restriction: restriction, Version: 1, UpdatedAt: now,
		}); err != nil {
			t.Fatalf("SaveFamily(%s): %v", marriage, err)
		}
		return id
	}

	ann := person(main, "Ann", "Boston, MA", " BOSTON ,ma ", "", true)
	event("person", ann, domain.FactPersonBurial, "Oak Hill, Boston, MA")
	attribute(ann, "Worcester, MA")
	pam := person(main, "Pam", "Secret, MA", "", "confidential", true)
	event("person", pam, domain.FactPersonBurial, "Secret Cemetery, MA")
	attribute(pam, "Secret Farm, MA")
	person(main, "Liv", "Livingston, MA", "", "", false)

	own := family(ann, nil, "Salem, MA", "")
	event("family", own, domain.FactFamilyDivorce, "Salem Court, MA")
	withPam := family(ann, &pam, "Hidden Chapel, MA", "")
	event("family", withPam, domain.FactFamilyDivorce, "Hidden Court, MA")
	family(ann, nil, "Private Hall, MA", "confidential")

	branch := domain.BranchID(uuid.New())
	person(branch, "Bea", "Branchville, MA", "", "", true)
	return branch
}
