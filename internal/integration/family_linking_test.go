package integration_test

import (
	"context"
	"fmt"
	"net/http"
	"testing"

	"github.com/google/uuid"

	"github.com/cacack/my-family/internal/api"
	"github.com/cacack/my-family/internal/domain"
)

// TestFamilyLinking_OnBranchMergesToMain covers issue #826 over the real HTTP
// API on every backend, with the exact calls the family UI makes: on a branch,
// two branch-only persons become the partners of a new family, a third is
// added as a child, a mainline person is set as partner and then cleared
// again, and one child is removed. The branch compare lists every touched
// stream, the merge carries the result to main, and a replay of the log gives
// the same family row.
func TestFamilyLinking_OnBranchMergesToMain(t *testing.T) {
	for _, backend := range backends {
		t.Run(backend.name, func(t *testing.T) {
			st := backend.setup(t)
			runFamilyLinking(t, newServer(t, st), st, backend)
		})
	}
}

func createPersonOn(t *testing.T, server *api.Server, branchID, given, surname string) string {
	t.Helper()
	resp := mustDo(t, server, http.MethodPost, scoped("/api/v1/persons", branchID),
		fmt.Sprintf(`{"given_name":%q,"surname":%q,"gender":"unknown"}`, given, surname),
		http.StatusCreated)
	return mustString(t, resp, "id")
}

func runFamilyLinking(t *testing.T, server *api.Server, st stores, backend backend) {
	t.Helper()
	ctx := context.Background()

	mainPerson := createPerson(t, server, "Mainline", "Elder")
	branchID := createBranch(t, server, "Mary is John's daughter")

	john := createPersonOn(t, server, branchID, "John", "Smith")
	ann := createPersonOn(t, server, branchID, "Ann", "Smith")
	mary := createPersonOn(t, server, branchID, "Mary", "Smith")
	tom := createPersonOn(t, server, branchID, "Tom", "Smith")

	// The add-family form: partners picked, then the family is created.
	created := mustDo(t, server, http.MethodPost, scoped("/api/v1/families", branchID),
		fmt.Sprintf(`{"partner1_id":%q,"partner2_id":%q,"relationship_type":"marriage"}`, john, ann),
		http.StatusCreated)
	familyID := mustString(t, created, "id")
	familyPath := "/api/v1/families/" + familyID
	childrenPath := familyPath + "/children"

	// The Add child dialog, twice; then Remove child for one of them.
	mustDo(t, server, http.MethodPost, scoped(childrenPath, branchID),
		fmt.Sprintf(`{"person_id":%q,"relationship_type":"biological"}`, mary), http.StatusCreated)
	mustDo(t, server, http.MethodPost, scoped(childrenPath, branchID),
		fmt.Sprintf(`{"person_id":%q,"relationship_type":"adopted"}`, tom), http.StatusCreated)
	mustDo(t, server, http.MethodDelete, scoped(childrenPath+"/"+tom, branchID), "", http.StatusNoContent)

	// The edit form: swap in a mainline person as partner2, then clear it.
	mustDo(t, server, http.MethodPut, scoped(familyPath, branchID),
		fmt.Sprintf(`{"partner2_id":%q,"version":%d}`, mainPerson, entityVersion(t, server, familyPath, branchID)),
		http.StatusOK)
	mustDo(t, server, http.MethodPut, scoped(familyPath, branchID),
		fmt.Sprintf(`{"clear_partner2":true,"version":%d}`, entityVersion(t, server, familyPath, branchID)),
		http.StatusOK)
	// ...and put Ann back.
	mustDo(t, server, http.MethodPut, scoped(familyPath, branchID),
		fmt.Sprintf(`{"partner2_id":%q,"version":%d}`, ann, entityVersion(t, server, familyPath, branchID)),
		http.StatusOK)

	assertLinkedFamily(t, getEntity(t, server, familyPath, branchID), john, ann, mary, tom)

	// The mainline does not have the family yet.
	if rec := do(t, server, http.MethodGet, familyPath, ""); rec.Code != http.StatusNotFound {
		t.Fatalf("mainline family before merge: %d, want 404", rec.Code)
	}

	// Compare: every stream the UI touched is on the branch side.
	compare := mustDo(t, server, http.MethodGet, comparePath(branchID), "", http.StatusOK)
	changed := entryField(t, jsonArray(t, compare, "branch_changes"), "entity_id")
	for _, id := range []string{familyID, john, ann, mary, tom} {
		if !contains(changed, id) {
			t.Errorf("branch_changes entity_ids = %v, want %s", changed, id)
		}
	}

	mustDo(t, server, http.MethodPost, mergePath(branchID), mergeBody("Mary is John's daughter"), http.StatusOK)
	assertLinkedFamily(t, getEntity(t, server, familyPath, ""), john, ann, mary, tom)

	// Mary's person page reads her parents' family from the child link.
	maryDetail := getEntity(t, server, "/api/v1/persons/"+mary, "")
	if parents, _ := maryDetail["family_as_child"].(map[string]any); parents == nil || parents["id"] != familyID {
		t.Errorf("mary family_as_child = %v, want %s", maryDetail["family_as_child"], familyID)
	}

	// Replay gives the same family row on main.
	fresh := backend.setup(t).read
	replayLog(t, ctx, st.events, fresh)
	id := uuid.MustParse(familyID)
	live, err := st.read.GetFamily(ctx, domain.MainBranchID, id)
	if err != nil {
		t.Fatalf("read live family: %v", err)
	}
	replayed, err := fresh.GetFamily(ctx, domain.MainBranchID, id)
	if err != nil {
		t.Fatalf("read replayed family: %v", err)
	}
	if a, b := canonicalRowJSON(t, live), canonicalRowJSON(t, replayed); a != b {
		t.Errorf("live family row differs from replay\n live:   %s\n replay: %s", a, b)
	}
}

// assertLinkedFamily checks the partners and that exactly the kept child is linked.
func assertLinkedFamily(t *testing.T, family map[string]any, partner1, partner2, keptChild, removedChild string) {
	t.Helper()
	if family["partner1_id"] != partner1 || family["partner2_id"] != partner2 {
		t.Errorf("partners = %v / %v, want %s / %s", family["partner1_id"], family["partner2_id"], partner1, partner2)
	}
	children := entryField(t, optionalArray(t, family, "children"), "person_id")
	if !contains(children, keptChild) || contains(children, removedChild) || len(children) != 1 {
		t.Errorf("children = %v, want only %s", children, keptChild)
	}
}
