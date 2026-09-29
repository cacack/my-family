package integration_test

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"reflect"
	"sort"
	"strings"
	"testing"

	"github.com/google/uuid"

	"github.com/cacack/my-family/internal/api"
	"github.com/cacack/my-family/internal/command"
)

// TestBranchMerge_Blockers drives, on every backend, a branch whose merge is
// blocked four ways at once (#831) — a family naming an excluded person, a
// citation of a source main deleted, a photo of a person main deleted, and a
// research log about a person main deleted — and checks that:
//
//   - the precheck names every blocker, with the one-step fix for each, and
//     writes nothing;
//   - the merge with the same resolutions is refused with exactly the same
//     blockers (and its message is the first blocker's, as before);
//   - applying the suggested fixes clears the precheck, and the merge then
//     goes through.
func TestBranchMerge_Blockers(t *testing.T) {
	for _, backend := range backends {
		t.Run(backend.name, func(t *testing.T) {
			st := backend.setup(t)
			runMergeBlockers(t, newServer(t, st), st)
		})
	}
}

func precheckPath(branchID string) string { return "/api/v1/branches/" + branchID + "/merge/precheck" }

// blockerRow is the part of a blocker the scenario pins exactly.
type blockerRow struct {
	stream, entityType, entityName string
	referenced, refType, refName   string
	kind, fix                      string
}

func blockerRows(t *testing.T, resp map[string]any) []blockerRow {
	t.Helper()
	var rows []blockerRow
	for _, raw := range jsonArray(t, resp, "blockers") {
		b, _ := raw.(map[string]any)
		get := func(field string) string {
			value, ok := b[field].(string)
			if !ok {
				t.Fatalf("blocker field %s = %v, want a string (blocker %v)", field, b[field], b)
			}
			return value
		}
		if get("message") == "" {
			t.Errorf("blocker %v has no message", b)
		}
		rows = append(rows, blockerRow{
			stream: get("stream_id"), entityType: get("entity_type"), entityName: get("entity_name"),
			referenced: get("referenced_id"), refType: get("referenced_type"), refName: get("referenced_name"),
			kind: get("kind"), fix: get("suggested_resolution"),
		})
	}
	return rows
}

func runMergeBlockers(t *testing.T, server *api.Server, st stores) {
	t.Helper()
	ctx := context.Background()

	owner := createPerson(t, server, "Owen", "Owner")
	subject := createPerson(t, server, "Sam", "Subject")
	anchor := createPerson(t, server, "Ada", "Anchor")
	source := createSource(t, server, "", "Parish Register")
	branchID := createBranch(t, server, "blocked-four-ways")

	newcomer := mustString(t, mustDo(t, server, http.MethodPost, scoped("/api/v1/persons", branchID),
		`{"given_name":"Pat","surname":"Newcomer","gender":"unknown"}`, http.StatusCreated), "id")
	family := mustString(t, mustDo(t, server, http.MethodPost, scoped("/api/v1/families", branchID),
		fmt.Sprintf(`{"partner1_id":%q,"relationship_type":"marriage"}`, newcomer), http.StatusCreated), "id")
	citation := createCitation(t, server, branchID, source, anchor)
	scan := uploadMedia(t, st, branchID, owner, "Portrait of Owen", testPNG(t, 7))
	researchLog := createResearchLog(t, server, branchID, subject, "County Archive")

	sourcePath := "/api/v1/sources/" + source
	mustDo(t, server, http.MethodDelete,
		fmt.Sprintf("%s?version=%d", sourcePath, entityVersion(t, server, sourcePath, "")), "", http.StatusNoContent)
	for _, person := range []string{owner, subject} {
		path := "/api/v1/persons/" + person
		mustDo(t, server, http.MethodDelete,
			fmt.Sprintf("%s?version=%d", path, entityVersion(t, server, path, "")), "", http.StatusNoContent)
	}

	excludeNewcomer := fmt.Sprintf(`{"resolutions":[%s]}`, resolution(newcomer, "main"))
	want := []blockerRow{
		{family, "family", "Pat Newcomer", newcomer, "person", "Pat Newcomer", "missing_person", "include_referenced"},
		{citation, "citation", "Parish Register (Birth)", source, "source", "Parish Register", "missing_source", "leave_out"},
		{scan, "media", "Portrait of Owen", owner, "person", "Owen Owner", "missing_media_owner", "leave_out"},
		{researchLog, "research_log", "Baptisms (County Archive)", subject, "person", "Sam Subject", "missing_gps_subject", "leave_out"},
	}

	// --- The precheck names every blocker and writes nothing -----------------
	before, err := st.snapshots.GetMaxPosition(ctx)
	if err != nil {
		t.Fatalf("GetMaxPosition failed: %v", err)
	}
	precheck := mustDo(t, server, http.MethodPost, precheckPath(branchID), excludeNewcomer, http.StatusOK)
	after, err := st.snapshots.GetMaxPosition(ctx)
	if err != nil {
		t.Fatalf("GetMaxPosition failed: %v", err)
	}
	if after != before {
		t.Errorf("precheck moved the log head from %d to %d; it must write nothing", before, after)
	}
	if got := blockerRows(t, precheck); !reflect.DeepEqual(got, want) {
		t.Fatalf("precheck blockers =\n  %+v\nwant\n  %+v", got, want)
	}
	if branch := getEntity(t, server, "/api/v1/branches/"+branchID, ""); branch["status"] != "active" {
		t.Errorf("branch status after precheck = %v, want active", branch["status"])
	}

	// --- The merge refuses with the very same blockers -----------------------
	refused := mustDo(t, server, http.MethodPost, mergePath(branchID), excludeNewcomer, http.StatusConflict)
	if refused["code"] != "merge_dangling_reference" {
		t.Fatalf("merge code = %v, want merge_dangling_reference", refused["code"])
	}
	if !reflect.DeepEqual(refused["blockers"], precheck["blockers"]) {
		t.Errorf("merge blockers differ from the precheck's:\n  merge    %v\n  precheck %v", refused["blockers"], precheck["blockers"])
	}
	message, _ := refused["message"].(string)
	if !strings.Contains(message, "merge would leave a reference pointing at an entity main does not have") ||
		!strings.Contains(message, newcomer) || !strings.Contains(message, "3 more blocker") {
		t.Errorf("merge message = %q, want the first blocker's text and a count of the rest", message)
	}

	// --- Each suggested fix, applied, clears its blocker ----------------------
	fixes := []string{resolution(citation, "main"), resolution(scan, "main"), resolution(researchLog, "main")}
	partial := fmt.Sprintf(`{"resolutions":[%s,%s]}`, resolution(newcomer, "main"), strings.Join(fixes, ","))
	if got := blockerRows(t, mustDo(t, server, http.MethodPost, precheckPath(branchID), partial, http.StatusOK)); !reflect.DeepEqual(got, want[:1]) {
		t.Errorf("precheck after leaving three out = %+v, want only the family's blocker", got)
	}
	fixed := fmt.Sprintf(`{"resolutions":[%s]}`, strings.Join(fixes, ","))
	cleared := mustDo(t, server, http.MethodPost, precheckPath(branchID), fixed, http.StatusOK)
	if got := jsonArray(t, cleared, "blockers"); len(got) != 0 {
		t.Fatalf("precheck with every fix = %v, want []", got)
	}
	merged := mustDo(t, server, http.MethodPost, mergePath(branchID), fixed, http.StatusOK)
	skipped := entryStrings(t, jsonArray(t, merged, "skipped_stream_ids"))
	for _, id := range []string{citation, scan, researchLog} {
		if !contains(skipped, id) {
			t.Errorf("skipped_stream_ids = %v, want it to include %s", skipped, id)
		}
	}
	if got := mustString(t, getEntity(t, server, "/api/v1/persons/"+newcomer, ""), "given_name"); got != "Pat" {
		t.Errorf("included person on main given_name = %q, want Pat", got)
	}

	// --- A precheck of a merged branch is refused like the merge -------------
	gone := mustDo(t, server, http.MethodPost, precheckPath(branchID), "", http.StatusConflict)
	if gone["code"] != "branch_not_active" {
		t.Errorf("precheck of a merged branch code = %v, want branch_not_active", gone["code"])
	}
}

// TestBranchMerge_PrecheckRefusals covers the precheck's non-200 answers on one
// backend: they are the merge's, mapped the same way.
func TestBranchMerge_PrecheckRefusals(t *testing.T) {
	server := newServer(t, setupMemory(t))
	empty := createBranch(t, server, "nothing-yet")
	if got := mustDo(t, server, http.MethodPost, precheckPath(empty), "", http.StatusConflict)["code"]; got != "merge_empty" {
		t.Errorf("empty branch precheck code = %v, want merge_empty", got)
	}
	mustDo(t, server, http.MethodPost, precheckPath("00000000-0000-0000-0000-000000000001"), "", http.StatusNotFound)

	person := createPerson(t, server, "Lee", "Lone")
	branchID := createBranch(t, server, "one-edit")
	personPath := "/api/v1/persons/" + person
	mustDo(t, server, http.MethodPut, scoped(personPath, branchID),
		fmt.Sprintf(`{"surname":"Edited","version":%d}`, entityVersion(t, server, personPath, branchID)), http.StatusOK)

	unknown := fmt.Sprintf(`{"resolutions":[%s]}`, resolution("00000000-0000-0000-0000-000000000002", "main"))
	if got := mustDo(t, server, http.MethodPost, precheckPath(branchID), unknown, http.StatusBadRequest)["code"]; got != "invalid_resolution" {
		t.Errorf("unknown stream precheck code = %v, want invalid_resolution", got)
	}
	twice := fmt.Sprintf(`{"resolutions":[%s,%s]}`, resolution(person, "main"), resolution(person, "branch"))
	mustDo(t, server, http.MethodPost, precheckPath(branchID), twice, http.StatusBadRequest)

	cleared := mustDo(t, server, http.MethodPost, precheckPath(branchID), "", http.StatusOK)
	raw, err := json.Marshal(cleared["blockers"])
	if err != nil || string(raw) != "[]" {
		t.Errorf("clean branch blockers = %s (%v), want []", raw, err)
	}
}

// assertBlockers checks a merge_dangling_reference refusal carries exactly the
// given blockers, in order.
func assertBlockers(t *testing.T, resp map[string]any, want ...blockerRow) {
	t.Helper()
	if resp["code"] != "merge_dangling_reference" {
		t.Fatalf("code = %v, want merge_dangling_reference", resp["code"])
	}
	if got := blockerRows(t, resp); !reflect.DeepEqual(got, want) {
		t.Errorf("blockers =\n  %+v\nwant\n  %+v", got, want)
	}
}

// TestBranchMerge_BlockerFixesConverge: the one-click fixes never cycle
// (#831). The branch creates a family with a partner main then deletes, plus a
// research log and a photo about that family. Leaving the family out (its
// fix) strands the log and the photo; their fix must be to leave them out
// too, not to include the family again — that would bring the family's own
// blocker straight back. Following the suggested fixes must reach a mergeable
// state.
func TestBranchMerge_BlockerFixesConverge(t *testing.T) {
	for _, backend := range backends {
		t.Run(backend.name, func(t *testing.T) {
			st := backend.setup(t)
			runBlockerFixesConverge(t, newServer(t, st), st)
		})
	}
}

func runBlockerFixesConverge(t *testing.T, server *api.Server, st stores) {
	t.Helper()
	ctx := context.Background()
	partner := createPerson(t, server, "Pat", "Partner")
	branchID := createBranch(t, server, "family-cycle")
	family := mustString(t, mustDo(t, server, http.MethodPost, scoped("/api/v1/families", branchID),
		fmt.Sprintf(`{"partner1_id":%q,"relationship_type":"marriage"}`, partner), http.StatusCreated), "id")
	researchLog := mustString(t, mustDo(t, server, http.MethodPost, scoped("/api/v1/research-logs", branchID),
		fmt.Sprintf(`{"subject_id":%q,"subject_type":"family","repository":"Parish chest","search_description":"Banns","outcome":"not_found","search_date":"2024-03-01T00:00:00Z"}`,
			family), http.StatusCreated), "id")
	branch, err := st.branches.Get(ctx, uuid.MustParse(branchID))
	if err != nil {
		t.Fatalf("Get branch failed: %v", err)
	}
	upload, err := command.NewHandlerWithBranches(st.events, st.read, st.branches, st.snapshots).WithBranch(branch).
		UploadMedia(ctx, command.UploadMediaInput{
			EntityType: "family", EntityID: uuid.MustParse(family), Title: "Wedding photo",
			MediaType: "photo", Filename: "wedding.png", FileData: testPNG(t, 3),
		})
	if err != nil {
		t.Fatalf("UploadMedia failed: %v", err)
	}
	photo := upload.ID.String()
	partnerPath := "/api/v1/persons/" + partner
	mustDo(t, server, http.MethodDelete,
		fmt.Sprintf("%s?version=%d", partnerPath, entityVersion(t, server, partnerPath, "")), "", http.StatusNoContent)

	// Step 1: the family names a partner main no longer has; nothing can bring
	// the partner back, so the fix is to leave the family out.
	step1 := blockerRows(t, mustDo(t, server, http.MethodPost, precheckPath(branchID), "", http.StatusOK))
	if len(step1) != 1 || step1[0].stream != family || step1[0].kind != "missing_person" || step1[0].fix != "leave_out" {
		t.Fatalf("step 1 blockers = %+v, want the family's missing partner, fixed by leaving the family out", step1)
	}

	// Step 2: with the family left out, the log and the photo are stranded.
	// Including the family would bring step 1 back, so both are fixed by
	// leaving them out.
	withoutFamily := fmt.Sprintf(`{"resolutions":[%s]}`, resolution(family, "main"))
	step2 := blockerRows(t, mustDo(t, server, http.MethodPost, precheckPath(branchID), withoutFamily, http.StatusOK))
	want := []blockerRow{
		{photo, "media", "Wedding photo", family, "family", "Pat Partner", "missing_media_owner", "leave_out"},
		{researchLog, "research_log", "Banns (Parish chest)", family, "family", "Pat Partner", "missing_gps_subject", "leave_out"},
	}
	sort.Slice(step2, func(i, j int) bool { return step2[i].entityType < step2[j].entityType })
	if !reflect.DeepEqual(step2, want) {
		t.Fatalf("step 2 blockers =\n  %+v\nwant\n  %+v", step2, want)
	}

	// Step 3: the suggested fixes, applied, clear every blocker.
	fixed := fmt.Sprintf(`{"resolutions":[%s,%s,%s]}`,
		resolution(family, "main"), resolution(researchLog, "main"), resolution(photo, "main"))
	if got := jsonArray(t, mustDo(t, server, http.MethodPost, precheckPath(branchID), fixed, http.StatusOK), "blockers"); len(got) != 0 {
		t.Fatalf("precheck after following the fixes = %v, want []", got)
	}
	mustDo(t, server, http.MethodPost, mergePath(branchID), fixed, http.StatusOK)
}
