package integration_test

import (
	"context"
	"fmt"
	"net/http"
	"testing"

	"github.com/cacack/my-family/internal/api"
)

// TestPedigreeEdge_PartnerCreatedAfterFamilyReplay covers #1259 on every
// backend. A merge replays the branch's streams in the order it first touched
// them, so a family the branch touched before creating a person replays its
// re-partnering to that person before the person's PersonCreated. The child's
// pedigree on main must still name the person as a parent after the merge, and
// a rebuild of the read model from the log must give the same pedigree.
func TestPedigreeEdge_PartnerCreatedAfterFamilyReplay(t *testing.T) {
	scenarios := []struct {
		name string
		run  func(t *testing.T, server *api.Server) (child, father, mother string)
	}{
		{"FamilyCreatedOnBranchBeforePerson", seedFamilyCreatedBeforePartner},
		{"FamilyTouchedBeforePersonThenRepartnered", seedFamilyTouchedBeforePartner},
	}
	for _, sc := range scenarios {
		t.Run(sc.name, func(t *testing.T) {
			for _, backend := range backends {
				t.Run(backend.name, func(t *testing.T) {
					st := backend.setup(t)
					server := newServer(t, st)
					child, father, mother := sc.run(t, server)
					assertPedigreeParents(t, server, child, "", father, mother)

					fresh := backend.setup(t)
					replayLog(t, context.Background(), st.events, fresh.read)
					assertPedigreeParents(t, newServer(t, fresh), child, "", father, mother)
				})
			}
		})
	}
}

// seedFamilyCreatedBeforePartner: the branch creates a family with the mother
// alone and links the child, then creates the father and adds him as the
// family's second partner, and merges.
func seedFamilyCreatedBeforePartner(t *testing.T, server *api.Server) (child, father, mother string) {
	t.Helper()
	mother = createGenderedPerson(t, server, "Briar", "female")
	child = createGenderedPerson(t, server, "Wren", "female")

	branchID := createBranch(t, server, "late father")
	resp := mustDo(t, server, http.MethodPost, scoped("/api/v1/families", branchID),
		fmt.Sprintf(`{"partner1_id":%q,"relationship_type":"marriage"}`, mother), http.StatusCreated)
	familyPath := "/api/v1/families/" + mustString(t, resp, "id")
	mustDo(t, server, http.MethodPost, scoped(familyPath+"/children", branchID),
		fmt.Sprintf(`{"person_id":%q}`, child), http.StatusCreated)

	father = createGenderedPersonOn(t, server, branchID, "Alden", "male")
	setPartner(t, server, familyPath, branchID, "partner2_id", father)
	assertPedigreeParents(t, server, child, branchID, father, mother)

	mustDo(t, server, http.MethodPost, mergePath(branchID), mergeBody("late father"), http.StatusOK)
	return child, father, mother
}

// seedFamilyTouchedBeforePartner: main has the family and child; the branch
// edits the family, then creates the father, then swaps him in for the old
// father, and merges. The person exists before the re-partnering, but the
// family's stream still replays first.
func seedFamilyTouchedBeforePartner(t *testing.T, server *api.Server) (child, father, mother string) {
	t.Helper()
	oldFather := createGenderedPerson(t, server, "Corin", "male")
	mother = createGenderedPerson(t, server, "Briar", "female")
	child = createGenderedPerson(t, server, "Wren", "female")
	familyPath := "/api/v1/families/" + createFamily(t, server, oldFather, mother)
	mustDo(t, server, http.MethodPost, familyPath+"/children",
		fmt.Sprintf(`{"person_id":%q}`, child), http.StatusCreated)

	branchID := createBranch(t, server, "father swap")
	setMarriagePlace(t, server, familyPath, branchID, "Parish Hall")
	father = createGenderedPersonOn(t, server, branchID, "Alden", "male")
	setPartner(t, server, familyPath, branchID, "partner1_id", father)
	assertPedigreeParents(t, server, child, branchID, father, mother)

	mustDo(t, server, http.MethodPost, mergePath(branchID), mergeBody("father swap"), http.StatusOK)
	return child, father, mother
}

// createGenderedPersonOn creates a person of the given gender on a scope.
func createGenderedPersonOn(t *testing.T, server *api.Server, branchID, givenName, gender string) string {
	t.Helper()
	resp := mustDo(t, server, http.MethodPost, scoped("/api/v1/persons", branchID),
		fmt.Sprintf(`{"given_name":%q,"surname":"Placeholder","gender":%q}`, givenName, gender),
		http.StatusCreated)
	return mustString(t, resp, "id")
}

// setPartner sets one partner slot of a family on a scope.
func setPartner(t *testing.T, server *api.Server, familyPath, branchID, slot, personID string) {
	t.Helper()
	mustDo(t, server, http.MethodPut, scoped(familyPath, branchID),
		fmt.Sprintf(`{%q:%q,"version":%d}`, slot, personID, entityVersion(t, server, familyPath, branchID)),
		http.StatusOK)
}
