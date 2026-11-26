package query_test

import (
	"context"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/cacack/my-family/internal/domain"
	"github.com/cacack/my-family/internal/repository"
	"github.com/cacack/my-family/internal/repository/memory"
)

// addMarriedRecord adds one record of a woman, married to husband with the
// given children, on scope.
func addMarriedRecord(t *testing.T, store *memory.ReadModelStore, scope domain.BranchID, surname string, husband uuid.UUID, children []uuid.UUID) uuid.UUID {
	t.Helper()
	ctx := context.Background()
	wife := uuid.New()
	require.NoError(t, store.SavePerson(ctx, scope, &repository.PersonReadModel{
		ID: wife, GivenName: "Mary", Surname: surname, FullName: "Mary " + surname,
		Gender: domain.GenderFemale, BirthDateRaw: "1850", Version: 1,
	}))
	family := uuid.New()
	require.NoError(t, store.SaveFamily(ctx, scope, &repository.FamilyReadModel{ID: family, Partner1ID: &husband, Partner2ID: &wife, Version: 1}))
	for _, child := range children {
		require.NoError(t, store.SaveFamilyChild(ctx, scope, &repository.FamilyChildReadModel{FamilyID: family, PersonID: child}))
	}
	return wife
}

// savePlainPerson saves a person with a name and sex only.
func savePlainPerson(t *testing.T, store *memory.ReadModelStore, scope domain.BranchID, given, surname string, gender domain.Gender) uuid.UUID {
	t.Helper()
	id := uuid.New()
	require.NoError(t, store.SavePerson(context.Background(), scope, &repository.PersonReadModel{
		ID: id, GivenName: given, Surname: surname, FullName: given + " " + surname, Gender: gender, Version: 1,
	}))
	return id
}

// One woman recorded twice, under her maiden and her married name, wife of
// the same husband and mother of the same two children (#1880).
func TestFindDuplicates_SharedSpouseAndChildren(t *testing.T) {
	tests := []struct {
		name          string
		maiden        string
		withRelatives bool
	}{
		{"maiden and married name, shared family", "Brown", true},
		{"maiden and married name, no shared family", "Brown", false},
		{"same surname, shared family", "Doe", true},
		{"same surname, no shared family", "Doe", false},
	}
	confidence := map[string]float64{}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			service, store := setupValidationService()
			ctx := context.Background()
			main := domain.MainBranchID

			var married, maiden uuid.UUID
			if tt.withRelatives {
				husband := savePlainPerson(t, store, main, "John", "Doe", domain.GenderMale)
				kids := []uuid.UUID{
					savePlainPerson(t, store, main, "Anna", "Doe", domain.GenderFemale),
					savePlainPerson(t, store, main, "Peter", "Doe", domain.GenderMale),
				}
				married = addMarriedRecord(t, store, main, "Doe", husband, kids)
				maiden = addMarriedRecord(t, store, main, tt.maiden, husband, kids)
			} else {
				married = addMarriedRecord(t, store, main, "Doe", savePlainPerson(t, store, main, "John", "Doe", domain.GenderMale), nil)
				maiden = addMarriedRecord(t, store, main, tt.maiden, savePlainPerson(t, store, main, "Carl", "Ross", domain.GenderMale), nil)
			}

			results, _, err := service.FindDuplicates(ctx, 100, 0)
			require.NoError(t, err)
			for _, r := range results {
				if (r.Person1ID == married && r.Person2ID == maiden) || (r.Person1ID == maiden && r.Person2ID == married) {
					confidence[tt.name] = r.Confidence
					if tt.withRelatives {
						assert.Contains(t, r.MatchReasons, "same spouse name")
						assert.Contains(t, r.MatchReasons, "2 shared child names")
					}
				}
			}
		})
	}

	// Different surnames: paired only through the shared family.
	assert.Greater(t, confidence["maiden and married name, shared family"], 0.0, "maiden/married pair reported")
	assert.Zero(t, confidence["maiden and married name, no shared family"], "no surname and no family: not a pair")
	assert.Greater(t, confidence["maiden and married name, shared family"], confidence["maiden and married name, no shared family"])
	// Same surname: reported either way, higher with the shared family.
	require.Greater(t, confidence["same surname, no shared family"], 0.0)
	assert.Greater(t, confidence["same surname, shared family"], confidence["same surname, no shared family"])
}

// Branch health finds the same maiden/married pair a branch introduces, with
// the confidence FindDuplicatesOn reports (#1880).
func TestBranchHealth_DuplicatesUseRelatives(t *testing.T) {
	f := newHealthFixture()
	husband := savePlainPerson(t, f.store, domain.MainBranchID, "John", "Doe", domain.GenderMale)
	kids := []uuid.UUID{
		savePlainPerson(t, f.store, domain.MainBranchID, "Anna", "Doe", domain.GenderFemale),
		savePlainPerson(t, f.store, domain.MainBranchID, "Peter", "Doe", domain.GenderMale),
	}
	married := addMarriedRecord(t, f.store, domain.MainBranchID, "Doe", husband, kids)
	// The branch adds the same woman under her maiden name.
	maiden := addMarriedRecord(t, f.store, f.branch, "Brown", husband, kids)

	health, err := f.health.BranchHealth(f.ctx, f.branch)
	require.NoError(t, err)
	require.Len(t, health.Duplicates, 1, "duplicates: %+v", health.Duplicates)
	pair := health.Duplicates[0]
	assert.ElementsMatch(t, []uuid.UUID{married, maiden}, []uuid.UUID{pair.Person1ID, pair.Person2ID})
	assert.Contains(t, pair.MatchReasons, "same spouse name")

	found, _, err := f.validation.FindDuplicatesOn(f.ctx, f.branch, 10, 0)
	require.NoError(t, err)
	require.Len(t, found, 1)
	assert.Equal(t, found[0].Confidence, pair.Confidence)
	assert.Equal(t, found[0].MatchReasons, pair.MatchReasons)
}
