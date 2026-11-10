package integration_test

import (
	"context"
	"fmt"
	"net/http"
	"testing"

	"github.com/google/uuid"

	"github.com/cacack/my-family/internal/api"
	"github.com/cacack/my-family/internal/domain"
	"github.com/cacack/my-family/internal/repository"
)

// genderChangeFixture is a family whose partner's gender changes after the
// child was linked, and a second child whose edge comes from another family.
type genderChangeFixture struct {
	child, father, mother             string // the child and the parents it should end with
	adopted, otherFather, otherMother string
}

// TestPedigreeEdge_PartnerGenderChange covers #1263 on every backend. A
// partner's gender change moves them to the side of their children's pedigree
// the new gender names, leaves a child whose edge came from another family
// alone, and a rebuild of the read model from the log gives the same pedigree.
func TestPedigreeEdge_PartnerGenderChange(t *testing.T) {
	scenarios := []struct {
		name string
		run  func(t *testing.T, server *api.Server, st stores) genderChangeFixture
	}{
		{"UnknownToFemale", seedUnknownToFemale},
		{"MaleToFemale", seedMaleToFemale},
		{"OnBranchThenMerged", seedGenderChangeOnBranch},
	}
	for _, sc := range scenarios {
		t.Run(sc.name, func(t *testing.T) {
			for _, backend := range backends {
				t.Run(backend.name, func(t *testing.T) {
					st := backend.setup(t)
					server := newServer(t, st)
					f := sc.run(t, server, st)
					assertGenderChangeFixture(t, server, f)

					fresh := backend.setup(t)
					replayLog(t, context.Background(), st.events, fresh.read)
					assertGenderChangeFixture(t, newServer(t, fresh), f)
				})
			}
		})
	}
}

func assertGenderChangeFixture(t *testing.T, server *api.Server, f genderChangeFixture) {
	t.Helper()
	assertPedigreeParents(t, server, f.child, "", f.father, f.mother)
	assertPedigreeParents(t, server, f.adopted, "", f.otherFather, f.otherMother)
}

// seedGenderFamily creates a family of the two partners with a child, and a
// second child of the family whose edge then comes from another family. The
// API links a child to one family only, so the second link is appended to the
// log directly, as a GEDCOM file or a merge can leave a child in two families.
func seedGenderFamily(t *testing.T, server *api.Server, st stores, partner1, partner2 string) (f genderChangeFixture) {
	t.Helper()
	f.child = createGenderedPerson(t, server, "Wren", "female")
	f.adopted = createGenderedPerson(t, server, "Ellis", "male")
	f.otherFather = createGenderedPerson(t, server, "Corin", "male")
	f.otherMother = createGenderedPerson(t, server, "Dana", "female")

	familyPath := "/api/v1/families/" + createFamily(t, server, partner1, partner2)
	for _, child := range []string{f.child, f.adopted} {
		mustDo(t, server, http.MethodPost, familyPath+"/children",
			fmt.Sprintf(`{"person_id":%q}`, child), http.StatusCreated)
	}
	other := uuid.MustParse(createFamily(t, server, f.otherFather, f.otherMother))
	appendAndProject(t, context.Background(), st, other, "family", repository.MainScope,
		domain.NewChildLinkedToFamily(domain.NewFamilyChild(other, uuid.MustParse(f.adopted), domain.ChildAdopted)))
	assertPedigreeParents(t, server, f.adopted, "", f.otherFather, f.otherMother)
	return f
}

// setGender sets a person's gender on a scope.
func setGender(t *testing.T, server *api.Server, personID, branchID, gender string) {
	t.Helper()
	renamePerson(t, server, personID, branchID, fmt.Sprintf(`"gender":%q`, gender))
}

// seedUnknownToFemale: both partners start unknown, so the first takes the
// mother slot; the second is then set to female and becomes the mother.
func seedUnknownToFemale(t *testing.T, server *api.Server, st stores) genderChangeFixture {
	t.Helper()
	first := createGenderedPerson(t, server, "Alden", "unknown")
	second := createGenderedPerson(t, server, "Briar", "unknown")
	f := seedGenderFamily(t, server, st, first, second)
	assertPedigreeParents(t, server, f.child, "", second, first)

	setGender(t, server, second, "", "female")
	f.father, f.mother = first, second
	return f
}

// seedMaleToFemale: a male partner and an unknown one; the male partner is
// then set to female, which moves the unknown partner to the father slot.
func seedMaleToFemale(t *testing.T, server *api.Server, st stores) genderChangeFixture {
	t.Helper()
	first := createGenderedPerson(t, server, "Alden", "male")
	second := createGenderedPerson(t, server, "Briar", "unknown")
	f := seedGenderFamily(t, server, st, first, second)
	assertPedigreeParents(t, server, f.child, "", first, second)

	setGender(t, server, first, "", "female")
	f.father, f.mother = second, first
	return f
}

// seedGenderChangeOnBranch: the gender change happens on a branch. The
// branch's pedigree follows it while main's stays as it was, until the merge.
func seedGenderChangeOnBranch(t *testing.T, server *api.Server, st stores) genderChangeFixture {
	t.Helper()
	first := createGenderedPerson(t, server, "Alden", "unknown")
	second := createGenderedPerson(t, server, "Briar", "unknown")
	f := seedGenderFamily(t, server, st, first, second)

	branchID := createBranch(t, server, "gender fix")
	setGender(t, server, second, branchID, "female")
	assertPedigreeParents(t, server, f.child, branchID, first, second)
	assertPedigreeParents(t, server, f.adopted, branchID, f.otherFather, f.otherMother)
	assertPedigreeParents(t, server, f.child, "", second, first)

	mustDo(t, server, http.MethodPost, mergePath(branchID), mergeBody("gender fix"), http.StatusOK)
	f.father, f.mother = first, second
	return f
}
