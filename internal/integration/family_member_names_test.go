package integration_test

import (
	"fmt"
	"net/http"
	"testing"

	"github.com/cacack/my-family/internal/api"
)

// TestFamilyMemberNames_FollowTheBranch covers #1207 on every backend: a
// family shows its partners' and children's names as the requested scope sees
// those persons. A branch rename shows in the branch's view of every family
// naming the person, whether or not the branch edited the family, and leaves
// main's view alone; a main rename shows in a branch's shadow of a family; and
// the rename writes no branch copy of a family, so main's later edits to it
// stay visible on the branch.
func TestFamilyMemberNames_FollowTheBranch(t *testing.T) {
	forEachBackend(t, runFamilyMemberNames)
}

func runFamilyMemberNames(t *testing.T, server *api.Server) {
	renamed := createPerson(t, server, "Rowan", "Elder")
	spouse := createPerson(t, server, "Lane", "Elder")
	child := createPerson(t, server, "Kit", "Elder")
	partner := createPerson(t, server, "Ash", "Other")
	unedited := createFamily(t, server, renamed, spouse)
	mustDo(t, server, http.MethodPost, "/api/v1/families/"+unedited+"/children",
		fmt.Sprintf(`{"person_id":%q}`, child), http.StatusCreated)
	shadowed := createFamily(t, server, renamed, partner)
	uneditedPath, shadowedPath := "/api/v1/families/"+unedited, "/api/v1/families/"+shadowed

	branch := createBranch(t, server, "family names")
	setMarriagePlace(t, server, shadowedPath, branch, "Branchton")

	// Rename the partner and the child on the branch.
	renamePerson(t, server, renamed, branch, `"surname":"Branchname"`)
	renamePerson(t, server, child, branch, `"given_name":"Kestrel"`)

	assertMember(t, server, uneditedPath, branch, "partner1", "Rowan Branchname")
	assertChild(t, server, uneditedPath, branch, "Kestrel Elder")
	assertMember(t, server, shadowedPath, branch, "partner1", "Rowan Branchname")
	assertMember(t, server, uneditedPath, "", "partner1", "Rowan Elder")
	assertChild(t, server, uneditedPath, "", "Kit Elder")
	assertMember(t, server, shadowedPath, "", "partner1", "Rowan Elder")

	// The rename wrote no branch row of the unedited family: main's later
	// edit to it shows on the branch.
	setMarriagePlace(t, server, uneditedPath, "", "Mainville")
	if got := getEntity(t, server, uneditedPath, branch)["marriage_place"]; got != "Mainville" {
		t.Errorf("branch view of main's family: marriage_place = %v, want main's later edit", got)
	}

	// A family the branch edits after the rename still shows the branch name.
	setMarriagePlace(t, server, uneditedPath, branch, "Branchville")
	assertMember(t, server, uneditedPath, branch, "partner1", "Rowan Branchname")

	// A main rename shows in the branch's shadow of a family naming the person.
	renamePerson(t, server, partner, "", `"surname":"Mainname"`)
	assertMember(t, server, shadowedPath, branch, "partner2", "Ash Mainname")
	assertMember(t, server, shadowedPath, "", "partner2", "Ash Mainname")
}

// setMarriagePlace edits a family's marriage place on the given scope.
func setMarriagePlace(t *testing.T, server *api.Server, familyPath, branchID, place string) {
	t.Helper()
	mustDo(t, server, http.MethodPut, scoped(familyPath, branchID),
		fmt.Sprintf(`{"marriage_place":%q,"version":%d}`, place, entityVersion(t, server, familyPath, branchID)),
		http.StatusOK)
}

// renamePerson applies the name fields (a JSON fragment) to a person on the
// given scope.
func renamePerson(t *testing.T, server *api.Server, personID, branchID, fields string) {
	t.Helper()
	path := "/api/v1/persons/" + personID
	mustDo(t, server, http.MethodPut, scoped(path, branchID),
		fmt.Sprintf(`{%s,"version":%d}`, fields, entityVersion(t, server, path, branchID)), http.StatusOK)
}

// assertMember checks the name a family shows for one partner slot.
func assertMember(t *testing.T, server *api.Server, familyPath, branchID, slot, want string) {
	t.Helper()
	summary, _ := getEntity(t, server, familyPath, branchID)[slot].(map[string]any)
	if got := personName(summary); got != want {
		t.Errorf("GET %s %s = %q, want %q", scoped(familyPath, branchID), slot, got, want)
	}
}

// assertChild checks the name a family shows for its only child.
func assertChild(t *testing.T, server *api.Server, familyPath, branchID, want string) {
	t.Helper()
	children := jsonArray(t, getEntity(t, server, familyPath, branchID), "children")
	if len(children) != 1 {
		t.Fatalf("GET %s children = %v, want one", scoped(familyPath, branchID), children)
	}
	summary, _ := children[0].(map[string]any)["person"].(map[string]any)
	if got := personName(summary); got != want {
		t.Errorf("GET %s child = %q, want %q", scoped(familyPath, branchID), got, want)
	}
}

// personName joins a person summary's given name and surname.
func personName(summary map[string]any) string {
	given, _ := summary["given_name"].(string)
	surname, _ := summary["surname"].(string)
	return given + " " + surname
}
