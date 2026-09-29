package integration_test

import (
	"fmt"
	"net/http"
	"strings"
	"testing"

	"github.com/cacack/my-family/internal/api"
)

// Person merge on a research branch (#834), driven through the real HTTP API
// against every storage backend.
//
// A PersonMerged lands on the survivor's stream but re-links rows of many other
// aggregates (the merged person's names, families, child edges, citations, GPS
// artifacts) and deletes the merged person. On a branch every one of those
// writes has to go through the overlay — main must not move until the branch
// merges — and the branch merge then has to carry the person merge onto main
// correctly. Both halves read their rows back out of each backend here.
//
// Fixtures use neutral placeholder names only (public repo - no real PII).

// mergeFixture is the mainline tree a person merge is made over: the survivor,
// the duplicate to merge away (a father with a family, a child, an alternate
// name, a citation and a research log), and the duplicate's family.
type mergeFixture struct {
	survivor, merged, partner, child, family string
	name, citation, researchLog              string
}

// createFullNamedPerson creates a person on the mainline with a gender, which
// decides whether they are a child's father or mother in the pedigree.
func createFullNamedPerson(t *testing.T, server *api.Server, givenName, surname, gender string) string {
	t.Helper()
	resp := mustDo(t, server, http.MethodPost, "/api/v1/persons",
		fmt.Sprintf(`{"given_name":%q,"surname":%q,"gender":%q}`, givenName, surname, gender),
		http.StatusCreated)
	return mustString(t, resp, "id")
}

// seedMergeFixture builds the mainline tree for the scenarios below.
func seedMergeFixture(t *testing.T, server *api.Server) mergeFixture {
	t.Helper()
	var f mergeFixture
	f.survivor = createFullNamedPerson(t, server, "Morgan", "Duplicate", "male")
	f.merged = createFullNamedPerson(t, server, "Morgan", "Duplicat", "male")
	mustDo(t, server, http.MethodPut, "/api/v1/persons/"+f.merged,
		fmt.Sprintf(`{"birth_place":"Riverton","version":%d}`, entityVersion(t, server, "/api/v1/persons/"+f.merged, "")),
		http.StatusOK)
	f.partner = createFullNamedPerson(t, server, "Jordan", "Partner", "female")
	f.child = createFullNamedPerson(t, server, "Quinn", "Duplicat", "unknown")
	f.family = createFamily(t, server, f.merged, f.partner)
	mustDo(t, server, http.MethodPost, "/api/v1/families/"+f.family+"/children",
		fmt.Sprintf(`{"person_id":%q}`, f.child), http.StatusCreated)
	name := mustDo(t, server, http.MethodPost, "/api/v1/persons/"+f.merged+"/names",
		`{"given_name":"Morgen","surname":"Duplicat","name_type":"aka"}`, http.StatusCreated)
	f.name = mustString(t, name, "id")
	f.citation = createCitation(t, server, "", createSource(t, server, "", "Parish register"), f.merged)
	f.researchLog = createResearchLog(t, server, "", f.merged, "County archive")
	return f
}

// mergePersonsOn merges f.merged into f.survivor on the given scope, reading
// both versions from that scope, keeping the merged person's birth place.
func mergePersonsOn(t *testing.T, server *api.Server, f mergeFixture, branchID string, wantStatus int) map[string]any {
	t.Helper()
	body := fmt.Sprintf(
		`{"survivor_id":%q,"merged_id":%q,"survivor_version":%d,"merged_version":%d,"field_resolution":{"birth_place":"merged"}}`,
		f.survivor, f.merged,
		entityVersion(t, server, "/api/v1/persons/"+f.survivor, branchID),
		entityVersion(t, server, "/api/v1/persons/"+f.merged, branchID))
	return mustDo(t, server, http.MethodPost, scoped("/api/v1/persons/merge", branchID), body, wantStatus)
}

// assertMergedView checks one scope's view of the tree after the merge: the
// merged person is gone and everything they carried is the survivor's.
func assertMergedView(t *testing.T, server *api.Server, f mergeFixture, branchID string) {
	t.Helper()
	mustDo(t, server, http.MethodGet, scoped("/api/v1/persons/"+f.merged, branchID), "", http.StatusNotFound)

	survivor := getEntity(t, server, "/api/v1/persons/"+f.survivor, branchID)
	if got := survivor["birth_place"]; got != "Riverton" {
		t.Errorf("survivor birth_place = %v, want Riverton (resolved from the merged person)", got)
	}

	family := getEntity(t, server, "/api/v1/families/"+f.family, branchID)
	if got := family["partner1_id"]; got != f.survivor {
		t.Errorf("family partner1_id = %v, want the survivor %s", got, f.survivor)
	}

	pedigree := getEntity(t, server, "/api/v1/pedigree/"+f.child, branchID)
	root, _ := pedigree["root"].(map[string]any)
	father, _ := root["father"].(map[string]any)
	if father == nil || father["id"] != f.survivor {
		t.Errorf("child's father = %v, want the survivor %s", father, f.survivor)
	}

	names := getEntity(t, server, "/api/v1/persons/"+f.survivor+"/names", branchID)
	if ids := entryField(t, jsonArray(t, names, "items"), "id"); !contains(ids, f.name) {
		t.Errorf("survivor names = %v, want the merged person's name %s", ids, f.name)
	}

	citations := getEntity(t, server, "/api/v1/persons/"+f.survivor+"/citations", branchID)
	if ids := entryField(t, jsonArray(t, citations, "citations"), "id"); !contains(ids, f.citation) {
		t.Errorf("survivor citations = %v, want the merged person's citation %s", ids, f.citation)
	}

	if got := getEntity(t, server, "/api/v1/research-logs/"+f.researchLog, branchID)["subject_id"]; got != f.survivor {
		t.Errorf("research log subject_id = %v, want the survivor %s", got, f.survivor)
	}
}

// assertUnmergedMain checks that main still has both persons as they were.
func assertUnmergedMain(t *testing.T, server *api.Server, f mergeFixture) {
	t.Helper()
	if got := getEntity(t, server, "/api/v1/persons/"+f.merged, "")["birth_place"]; got != "Riverton" {
		t.Errorf("main merged person birth_place = %v, want Riverton", got)
	}
	if got := getEntity(t, server, "/api/v1/persons/"+f.survivor, "")["birth_place"]; got != nil && got != "" {
		t.Errorf("main survivor birth_place = %v, want it unset — the branch merge leaked onto main", got)
	}
	if got := getEntity(t, server, "/api/v1/families/"+f.family, "")["partner1_id"]; got != f.merged {
		t.Errorf("main family partner1_id = %v, want the merged person %s", got, f.merged)
	}
	if got := getEntity(t, server, "/api/v1/research-logs/"+f.researchLog, "")["subject_id"]; got != f.merged {
		t.Errorf("main research log subject_id = %v, want the merged person %s", got, f.merged)
	}
	citations := getEntity(t, server, "/api/v1/persons/"+f.merged+"/citations", "")
	if ids := entryField(t, jsonArray(t, citations, "citations"), "id"); !contains(ids, f.citation) {
		t.Errorf("main citations of the merged person = %v, want %s", ids, f.citation)
	}
}

// TestBranchPersonMerge_EndToEnd: two persons merged on a branch leave main
// untouched; the branch shows the survivor with the merged person's data and
// no merged person; merging the branch makes main match.
func TestBranchPersonMerge_EndToEnd(t *testing.T) {
	forEachBackend(t, func(t *testing.T, server *api.Server) {
		f := seedMergeFixture(t, server)
		branchID := createBranch(t, server, "same-person-hypothesis")

		result := mergePersonsOn(t, server, f, branchID, http.StatusOK)
		if person, _ := result["person"].(map[string]any); person["birth_place"] != "Riverton" {
			t.Errorf("merge response person = %v, want the branch's survivor with Riverton", person)
		}

		assertMergedView(t, server, f, branchID)
		assertUnmergedMain(t, server, f)

		// The branch's compare shows the merge, and nothing conflicts.
		compare := mustDo(t, server, http.MethodGet, comparePath(branchID), "", http.StatusOK)
		if conflicts := jsonArray(t, compare, "conflicts"); len(conflicts) != 0 {
			t.Fatalf("conflicts = %v, want none - main did not move", conflicts)
		}

		merged := mustDo(t, server, http.MethodPost, mergePath(branchID), mergeBody("same person"), http.StatusOK)
		if count := merged["replayed_event_count"]; count != float64(1) {
			t.Errorf("replayed_event_count = %v, want 1 (the PersonMerged)", count)
		}
		assertMergedView(t, server, f, "")
	})
}

// TestBranchPersonMerge_MainEditedMergedPerson: main edits the merged person
// after the fork. The branch never wrote the merged person's stream, but its
// merge ends that person, so compare reports a delete_edit conflict on the
// survivor's stream. "main" keeps both persons; "branch" completes the merge.
func TestBranchPersonMerge_MainEditedMergedPerson(t *testing.T) {
	forEachBackend(t, func(t *testing.T, server *api.Server) {
		f := seedMergeFixture(t, server)
		keepMain := createBranch(t, server, "merge-then-main-wins")
		takeBranch := createBranch(t, server, "merge-then-branch-wins")
		mergePersonsOn(t, server, f, keepMain, http.StatusOK)
		mergePersonsOn(t, server, f, takeBranch, http.StatusOK)

		updateSurname(t, server, "/api/v1/persons/"+f.merged, "", "Duplicatte")

		conflict := conflictFor(t, mustDo(t, server, http.MethodGet, comparePath(keepMain), "", http.StatusOK), f.survivor)
		if kind := conflict["kind"]; kind != "delete_edit" {
			t.Errorf("conflict kind = %v, want delete_edit", kind)
		}
		supported := stringValues(t, conflict, "supported_resolutions")
		if !contains(supported, "branch") || !contains(supported, "main") {
			t.Errorf("supported_resolutions = %v, want both", supported)
		}

		rec := do(t, server, http.MethodPost, mergePath(keepMain), mergeBody("undecided"))
		if rec.Code != http.StatusConflict {
			t.Fatalf("merge with no resolution: status = %d, want 409. Body: %s", rec.Code, rec.Body.String())
		}

		mustDo(t, server, http.MethodPost, mergePath(keepMain),
			mergeBody("main's record stands", resolution(f.survivor, "main")), http.StatusOK)
		if got := personSurname(t, server, f.merged, ""); got != "Duplicatte" {
			t.Errorf("main merged person surname = %q, want Duplicatte - the main resolution was not honored", got)
		}
		if got := getEntity(t, server, "/api/v1/families/"+f.family, "")["partner1_id"]; got != f.merged {
			t.Errorf("main family partner1_id = %v, want still the merged person", got)
		}

		mustDo(t, server, http.MethodPost, mergePath(takeBranch),
			mergeBody("same person after all", resolution(f.survivor, "branch")), http.StatusOK)
		assertMergedView(t, server, f, "")
	})
}

// TestBranchPersonMerge_MainDeletedSurvivor: main deletes the survivor after
// the fork. Replaying the merge onto a missing survivor would re-link nothing,
// so compare reports the survivor's delete and offers only "main".
func TestBranchPersonMerge_MainDeletedSurvivor(t *testing.T) {
	forEachBackend(t, func(t *testing.T, server *api.Server) {
		f := seedMergeFixture(t, server)
		branchID := createBranch(t, server, "merge-into-a-deleted-person")
		mergePersonsOn(t, server, f, branchID, http.StatusOK)

		mustDo(t, server, http.MethodDelete, "/api/v1/persons/"+f.survivor, "", http.StatusNoContent)

		conflict := conflictFor(t, mustDo(t, server, http.MethodGet, comparePath(branchID), "", http.StatusOK), f.survivor)
		if kind := conflict["kind"]; kind != "delete_edit" {
			t.Errorf("conflict kind = %v, want delete_edit", kind)
		}
		if supported := stringValues(t, conflict, "supported_resolutions"); len(supported) != 1 || supported[0] != "main" {
			t.Errorf("supported_resolutions = %v, want only main", supported)
		}
		mustDo(t, server, http.MethodPost, mergePath(branchID),
			mergeBody("survivor is gone", resolution(f.survivor, "main")), http.StatusOK)
		if got := getEntity(t, server, "/api/v1/families/"+f.family, "")["partner1_id"]; got != f.merged {
			t.Errorf("main family partner1_id = %v, want still the merged person", got)
		}
	})
}

// TestBranchPersonMerge_FollowsTheBranchMergeChain: the branch links the
// duplicate into a new family and then merges them away; main then deletes
// the duplicate. The link names a person main no longer has, but the replayed
// merge re-links it to the survivor, so the merge is accepted and main's new
// family ends up with the survivor as partner. A child the branch linked to
// that family must name the survivor as father on main too: the replayed link
// lands while main has no row for the duplicate, so the merge has to fill the
// parent the link left empty (#834).
func TestBranchPersonMerge_FollowsTheBranchMergeChain(t *testing.T) {
	forEachBackend(t, func(t *testing.T, server *api.Server) {
		// A duplicate with no family on main, so main is free to delete them.
		f := mergeFixture{
			survivor: createFullNamedPerson(t, server, "Avery", "Twin", "male"),
			merged:   createFullNamedPerson(t, server, "Avery", "Twinn", "male"),
		}
		branchID := createBranch(t, server, "link-then-merge")
		other := createFullNamedPerson(t, server, "Avery", "Second", "female")

		resp := mustDo(t, server, http.MethodPost, scoped("/api/v1/families", branchID),
			fmt.Sprintf(`{"partner1_id":%q,"partner2_id":%q,"relationship_type":"marriage"}`, f.merged, other),
			http.StatusCreated)
		secondFamily := mustString(t, resp, "id")
		child := createFullNamedPerson(t, server, "Kit", "Twin", "unknown")
		mustDo(t, server, http.MethodPost, scoped("/api/v1/families/"+secondFamily+"/children", branchID),
			fmt.Sprintf(`{"person_id":%q}`, child), http.StatusCreated)
		mergePersonsOn(t, server, f, branchID, http.StatusOK)

		if got := getEntity(t, server, "/api/v1/families/"+secondFamily, branchID)["partner1_id"]; got != f.survivor {
			t.Fatalf("branch second family partner1_id = %v, want the survivor", got)
		}
		assertPedigreeParents(t, server, child, branchID, f.survivor, other)

		mustDo(t, server, http.MethodDelete, "/api/v1/persons/"+f.merged, "", http.StatusNoContent)

		mustDo(t, server, http.MethodPost, mergePath(branchID), mergeBody("linked, then merged"), http.StatusOK)
		if got := getEntity(t, server, "/api/v1/families/"+secondFamily, "")["partner1_id"]; got != f.survivor {
			t.Errorf("main second family partner1_id = %v, want the survivor %s", got, f.survivor)
		}
		mustDo(t, server, http.MethodGet, "/api/v1/persons/"+f.merged, "", http.StatusNotFound)
		assertPedigreeParents(t, server, child, "", f.survivor, other)
	})
}

// assertPedigreeParents checks a person's father and mother in one scope's
// pedigree.
func assertPedigreeParents(t *testing.T, server *api.Server, personID, branchID, fatherID, motherID string) {
	t.Helper()
	pedigree := getEntity(t, server, "/api/v1/pedigree/"+personID, branchID)
	root, _ := pedigree["root"].(map[string]any)
	father, _ := root["father"].(map[string]any)
	if father == nil || father["id"] != fatherID {
		t.Errorf("pedigree of %s on %q: father = %v, want %s", personID, branchID, father, fatherID)
	}
	mother, _ := root["mother"].(map[string]any)
	if mother == nil || mother["id"] != motherID {
		t.Errorf("pedigree of %s on %q: mother = %v, want %s", personID, branchID, mother, motherID)
	}
}

// TestBranchPersonMerge_MainMergedSurvivorAway: main merges the branch's
// survivor into a third person after the fork. PersonMerged writes nothing to
// the survivor's stream, so there is no conflict to show, but replaying the
// branch's merge would target a person main no longer has: it is refused as a
// dangling reference, with nothing written.
func TestBranchPersonMerge_MainMergedSurvivorAway(t *testing.T) {
	forEachBackend(t, func(t *testing.T, server *api.Server) {
		f := seedMergeFixture(t, server)
		branchID := createBranch(t, server, "survivor-merged-on-main")
		mergePersonsOn(t, server, f, branchID, http.StatusOK)

		third := createFullNamedPerson(t, server, "Morgan", "Duplicates", "male")
		mustDo(t, server, http.MethodPost, "/api/v1/persons/merge", fmt.Sprintf(
			`{"survivor_id":%q,"merged_id":%q,"survivor_version":%d,"merged_version":%d}`,
			third, f.survivor,
			entityVersion(t, server, "/api/v1/persons/"+third, ""),
			entityVersion(t, server, "/api/v1/persons/"+f.survivor, "")), http.StatusOK)

		rec := do(t, server, http.MethodPost, mergePath(branchID), mergeBody("survivor is merged away"))
		if rec.Code != http.StatusConflict {
			t.Fatalf("merge: status = %d, want 409. Body: %s", rec.Code, rec.Body.String())
		}
		if code := decodeJSON(t, rec)["code"]; code != "merge_dangling_reference" {
			t.Errorf("refusal code = %v, want merge_dangling_reference", code)
		}
		if got := getEntity(t, server, "/api/v1/branches/"+branchID, "")["status"]; got != "active" {
			t.Errorf("branch status = %v, want active - the refusal must write nothing", got)
		}
		assertUnmergedMainFamily(t, server, f)
	})
}

// assertUnmergedMainFamily checks main's family still names the merged person.
func assertUnmergedMainFamily(t *testing.T, server *api.Server, f mergeFixture) {
	t.Helper()
	if got := getEntity(t, server, "/api/v1/families/"+f.family, "")["partner1_id"]; got != f.merged {
		t.Errorf("main family partner1_id = %v, want still the merged person %s", got, f.merged)
	}
}

// TestBranchPersonMerge_BranchCreatedSurvivor: the branch creates the
// survivor, a family naming them as partner, and a family naming the
// duplicate, then merges the duplicate into the new survivor. The survivor's
// stream carries both its creation and the merge, so the replay must land the
// duplicate's family before it and the survivor's family after it; main must
// then show what the branch showed, partner names included.
func TestBranchPersonMerge_BranchCreatedSurvivor(t *testing.T) {
	forEachBackend(t, func(t *testing.T, server *api.Server) {
		merged := createFullNamedPerson(t, server, "Sam", "Survivr", "male")
		spouse := createFullNamedPerson(t, server, "Robin", "Spouse", "female")
		other := createFullNamedPerson(t, server, "Casey", "Other", "female")
		branchID := createBranch(t, server, "created-survivor")

		resp := mustDo(t, server, http.MethodPost, scoped("/api/v1/persons", branchID),
			`{"given_name":"Sam","surname":"Survivor","gender":"male"}`, http.StatusCreated)
		survivor := mustString(t, resp, "id")
		familyOfSurvivor := mustString(t, mustDo(t, server, http.MethodPost, scoped("/api/v1/families", branchID),
			fmt.Sprintf(`{"partner1_id":%q,"partner2_id":%q,"relationship_type":"marriage"}`, survivor, spouse),
			http.StatusCreated), "id")
		familyOfMerged := mustString(t, mustDo(t, server, http.MethodPost, scoped("/api/v1/families", branchID),
			fmt.Sprintf(`{"partner1_id":%q,"partner2_id":%q,"relationship_type":"marriage"}`, merged, other),
			http.StatusCreated), "id")
		mergePersonsOn(t, server, mergeFixture{survivor: survivor, merged: merged}, branchID, http.StatusOK)

		mustDo(t, server, http.MethodPost, mergePath(branchID), mergeBody("created, then merged into"), http.StatusOK)
		for _, familyID := range []string{familyOfSurvivor, familyOfMerged} {
			family := getEntity(t, server, "/api/v1/families/"+familyID, "")
			partner, _ := family["partner1"].(map[string]any)
			if family["partner1_id"] != survivor || partner == nil ||
				partner["given_name"] != "Sam" || partner["surname"] != "Survivor" {
				t.Errorf("main family %s = partner1_id %v, partner1 %v; want the survivor Sam Survivor",
					familyID, family["partner1_id"], partner)
			}
		}
		mustDo(t, server, http.MethodGet, "/api/v1/persons/"+merged, "", http.StatusNotFound)
	})
}

// TestBranchPersonMerge_MainMergedMergedPersonElsewhere: the branch merges the
// duplicate into the survivor while main merges the same duplicate into a
// third person. Main's merge writes only to the third person's stream, so the
// two identity hypotheses would otherwise both land; the branch merge is
// refused as a dangling reference, and a "main" resolution of the survivor's
// stream keeps main's merge.
func TestBranchPersonMerge_MainMergedMergedPersonElsewhere(t *testing.T) {
	forEachBackend(t, func(t *testing.T, server *api.Server) {
		f := seedMergeFixture(t, server)
		branchID := createBranch(t, server, "merged-person-merged-on-main")
		mergePersonsOn(t, server, f, branchID, http.StatusOK)

		third := createFullNamedPerson(t, server, "Morgan", "Duplicates", "male")
		mustDo(t, server, http.MethodPost, "/api/v1/persons/merge", fmt.Sprintf(
			`{"survivor_id":%q,"merged_id":%q,"survivor_version":%d,"merged_version":%d}`,
			third, f.merged,
			entityVersion(t, server, "/api/v1/persons/"+third, ""),
			entityVersion(t, server, "/api/v1/persons/"+f.merged, "")), http.StatusOK)

		rec := do(t, server, http.MethodPost, mergePath(branchID), mergeBody("two hypotheses"))
		if rec.Code != http.StatusConflict {
			t.Fatalf("merge: status = %d, want 409. Body: %s", rec.Code, rec.Body.String())
		}
		if code := decodeJSON(t, rec)["code"]; code != "merge_dangling_reference" {
			t.Errorf("refusal code = %v, want merge_dangling_reference", code)
		}
		if body := rec.Body.String(); !strings.Contains(body, "merged person "+f.merged+" into another person") {
			t.Errorf("refusal = %s, want it to name main's different merge", body)
		}
		if got := getEntity(t, server, "/api/v1/persons/"+f.survivor, "")["birth_place"]; got != nil && got != "" {
			t.Errorf("main survivor birth_place = %v, want untouched by the refused merge", got)
		}

		mustDo(t, server, http.MethodPost, mergePath(branchID),
			mergeBody("main's merge stands", resolution(f.survivor, "main")), http.StatusOK)
		if got := getEntity(t, server, "/api/v1/families/"+f.family, "")["partner1_id"]; got != third {
			t.Errorf("main family partner1_id = %v, want main's survivor %s", got, third)
		}
	})
}

// TestBranchPersonMerge_SameMergeOnBothSides: main and the branch both merge
// the duplicate into the same survivor after the fork. The refusal must say
// so rather than claim main chose another person, and resolving the
// survivor's stream to main keeps main's merge.
func TestBranchPersonMerge_SameMergeOnBothSides(t *testing.T) {
	forEachBackend(t, func(t *testing.T, server *api.Server) {
		f := seedMergeFixture(t, server)
		branchID := createBranch(t, server, "same-merge-both-sides")
		mergePersonsOn(t, server, f, branchID, http.StatusOK)
		mergePersonsOn(t, server, f, "", http.StatusOK)

		rec := do(t, server, http.MethodPost, mergePath(branchID), mergeBody("same merge twice"))
		if rec.Code != http.StatusConflict {
			t.Fatalf("merge: status = %d, want 409. Body: %s", rec.Code, rec.Body.String())
		}
		body := rec.Body.String()
		if !strings.Contains(body, "main has since made the same merge") || strings.Contains(body, "another person") {
			t.Errorf("refusal = %s, want it to say both sides made the same merge", body)
		}

		mustDo(t, server, http.MethodPost, mergePath(branchID),
			mergeBody("main's merge stands", resolution(f.survivor, "main")), http.StatusOK)
		assertMergedView(t, server, f, "")
	})
}

// TestBranchPersonMerge_MainLinkedMergedPersonAsChild: the survivor is a child
// on main; after the fork main makes the duplicate the child of another
// family. A person has one child family, so the replayed merge would drop the
// duplicate's new parentage and leave that family counting a child it no
// longer lists: it is refused, and nothing is written.
func TestBranchPersonMerge_MainLinkedMergedPersonAsChild(t *testing.T) {
	forEachBackend(t, func(t *testing.T, server *api.Server) {
		f := seedMergeFixture(t, server)
		parent := createFullNamedPerson(t, server, "Pat", "Elder", "female")
		survivorParents := createFamily(t, server, createFullNamedPerson(t, server, "Dale", "Elder", "male"), parent)
		mustDo(t, server, http.MethodPost, "/api/v1/families/"+survivorParents+"/children",
			fmt.Sprintf(`{"person_id":%q}`, f.survivor), http.StatusCreated)
		branchID := createBranch(t, server, "merge-over-new-parentage")
		mergePersonsOn(t, server, f, branchID, http.StatusOK)

		otherParent := createFullNamedPerson(t, server, "Lee", "Elder", "male")
		mergedParents := createFamily(t, server, otherParent, createFullNamedPerson(t, server, "Ash", "Elder", "female"))
		mustDo(t, server, http.MethodPost, "/api/v1/families/"+mergedParents+"/children",
			fmt.Sprintf(`{"person_id":%q}`, f.merged), http.StatusCreated)

		rec := do(t, server, http.MethodPost, mergePath(branchID), mergeBody("parentage changed"))
		if rec.Code != http.StatusConflict {
			t.Fatalf("merge: status = %d, want 409. Body: %s", rec.Code, rec.Body.String())
		}
		if code := decodeJSON(t, rec)["code"]; code != "merge_dangling_reference" {
			t.Errorf("refusal code = %v, want merge_dangling_reference", code)
		}
		pedigree := getEntity(t, server, "/api/v1/pedigree/"+f.merged, "")
		root, _ := pedigree["root"].(map[string]any)
		father, _ := root["father"].(map[string]any)
		if father == nil || father["id"] != otherParent {
			t.Errorf("main merged person's father = %v, want %s - the refusal must write nothing", father, otherParent)
		}
	})
}
