package api_test

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/cacack/my-family/internal/api"
	"github.com/cacack/my-family/internal/config"
	"github.com/cacack/my-family/internal/domain"
	"github.com/cacack/my-family/internal/repository"
	"github.com/cacack/my-family/internal/repository/memory"
)

// ============================================================================
// Branch scope on media (sub-issue D of #676, issue #759)
// ============================================================================

// uploadMedia uploads a JPEG for personID at path suffix (e.g. "?branch=…")
// and returns the created media's id and version.
func uploadMedia(t *testing.T, server *api.Server, personID, suffix string, jpegData []byte) (string, int64) {
	t.Helper()
	req, err := createMultipartRequest(fmt.Sprintf("/api/v1/persons/%s/media%s", personID, suffix),
		"file", "portrait.jpg", jpegData, nil)
	if err != nil {
		t.Fatalf("build upload request: %v", err)
	}
	rec := httptest.NewRecorder()
	server.Echo().ServeHTTP(rec, req)
	if rec.Code != http.StatusCreated {
		t.Fatalf("Upload media%s: status = %d, want 201. Body: %s", suffix, rec.Code, rec.Body.String())
	}
	body := decodeJSON(t, rec)
	id, _ := body["id"].(string)
	version, _ := body["version"].(float64)
	return id, int64(version)
}

// TestMedia_BranchScope walks the media surface on a branch: a metadata edit
// lands on the branch only while content and thumbnail keep serving the shared
// bytes there, a branch upload is branch-only, and a branch delete hides the
// item on the branch without touching the mainline item or its file.
func TestMedia_BranchScope(t *testing.T) {
	server := setupBranchTestServer()
	personID := createPerson(t, server, "Ada", "Lovelace")
	jpegData := createTestJPEGImage()
	mediaID, version := uploadMedia(t, server, personID, "", jpegData)

	branchID := createBranch(t, server, "Portrait reading")
	onBranch := "?branch=" + branchID

	getJSON := func(path string) map[string]any {
		t.Helper()
		rec := do(t, server, http.MethodGet, path, "")
		if rec.Code != http.StatusOK {
			t.Fatalf("GET %s: status = %d, want 200. Body: %s", path, rec.Code, rec.Body.String())
		}
		return decodeJSON(t, rec)
	}
	total := func(path string) int {
		t.Helper()
		n, _ := getJSON(path)["total"].(float64)
		return int(n)
	}
	status := func(method, path string) int {
		t.Helper()
		return do(t, server, method, path, "").Code
	}

	// Retitle on the branch only.
	rec := do(t, server, http.MethodPut, "/api/v1/media/"+mediaID+onBranch,
		fmt.Sprintf(`{"title":"Portrait (branch reading)","version":%d}`, version))
	if rec.Code != http.StatusOK {
		t.Fatalf("Update media on branch: status = %d, want 200. Body: %s", rec.Code, rec.Body.String())
	}
	if got := decodeJSON(t, rec)["title"]; got != "Portrait (branch reading)" {
		t.Errorf("Branch update response title = %v, want the branch title", got)
	}
	if got := getJSON("/api/v1/media/" + mediaID)["title"]; got != "portrait.jpg" {
		t.Errorf("Mainline media title = %v, want portrait.jpg", got)
	}
	if got := getJSON("/api/v1/media/" + mediaID + onBranch)["title"]; got != "Portrait (branch reading)" {
		t.Errorf("Branch media title = %v, want the branch title", got)
	}

	// The file and thumbnail are served on the branch from the shared bytes.
	rec = do(t, server, http.MethodGet, "/api/v1/media/"+mediaID+"/content"+onBranch, "")
	if rec.Code != http.StatusOK || rec.Body.Len() != len(jpegData) {
		t.Errorf("Branch download: status = %d, %d bytes, want 200 and the %d uploaded bytes", rec.Code, rec.Body.Len(), len(jpegData))
	}
	if got := status(http.MethodGet, "/api/v1/media/"+mediaID+"/thumbnail"+onBranch); got != http.StatusOK {
		t.Errorf("Branch thumbnail: status = %d, want 200", got)
	}

	// A branch upload is branch-only.
	branchMediaID, _ := uploadMedia(t, server, personID, onBranch, jpegData)
	if got := status(http.MethodGet, "/api/v1/media/"+branchMediaID); got != http.StatusNotFound {
		t.Errorf("Mainline GET of a branch upload: status = %d, want 404", got)
	}
	if got := total("/api/v1/persons/" + personID + "/media"); got != 1 {
		t.Errorf("Mainline person media = %d, want 1", got)
	}
	if got := total("/api/v1/persons/" + personID + "/media" + onBranch); got != 2 {
		t.Errorf("Branch person media = %d, want 2", got)
	}

	// Delete the mainline item on the branch only.
	branchVersion, _ := getJSON("/api/v1/media/" + mediaID + onBranch)["version"].(float64)
	if got := status(http.MethodDelete, fmt.Sprintf("/api/v1/media/%s?branch=%s&version=%d", mediaID, branchID, int64(branchVersion))); got != http.StatusNoContent {
		t.Fatalf("Delete media on branch: status = %d, want 204", got)
	}
	for _, path := range []string{"", "/content", "/thumbnail"} {
		if got := status(http.MethodGet, "/api/v1/media/"+mediaID+path+onBranch); got != http.StatusNotFound {
			t.Errorf("Branch GET media%s after branch delete: status = %d, want 404", path, got)
		}
	}
	// Updating or deleting it again on the branch reports not found, not 500.
	if got := do(t, server, http.MethodPut, "/api/v1/media/"+mediaID+onBranch,
		fmt.Sprintf(`{"title":"x","version":%d}`, int64(branchVersion))).Code; got != http.StatusNotFound {
		t.Errorf("Branch update of a branch-deleted item: status = %d, want 404", got)
	}
	if got := status(http.MethodDelete, fmt.Sprintf("/api/v1/media/%s?branch=%s&version=%d", mediaID, branchID, int64(branchVersion))); got != http.StatusNotFound {
		t.Errorf("Branch delete of a branch-deleted item: status = %d, want 404", got)
	}
	rec = do(t, server, http.MethodGet, "/api/v1/media/"+mediaID+"/content", "")
	if rec.Code != http.StatusOK || rec.Body.Len() != len(jpegData) {
		t.Errorf("Mainline download after branch delete: status = %d, %d bytes, want the original file", rec.Code, rec.Body.Len())
	}
	if got := total("/api/v1/persons/" + personID + "/media" + onBranch); got != 1 {
		t.Errorf("Branch person media after delete = %d, want 1 (the branch upload)", got)
	}
}

// TestMedia_BranchScopeRejectsUnknownBranch pins the shared branchScope
// contract on the media operations: an unknown branch is 404 for reads and
// writes alike.
func TestMedia_BranchScopeRejectsUnknownBranch(t *testing.T) {
	server := setupBranchTestServer()
	personID := createPerson(t, server, "Ada", "Lovelace")
	mediaID, version := uploadMedia(t, server, personID, "", createTestJPEGImage())
	unknown := "?branch=00000000-0000-0000-0000-00000000beef"

	for _, tc := range []struct {
		method, path, body string
	}{
		{http.MethodGet, "/api/v1/media/" + mediaID + unknown, ""},
		{http.MethodGet, "/api/v1/media/" + mediaID + "/content" + unknown, ""},
		{http.MethodGet, "/api/v1/media/" + mediaID + "/thumbnail" + unknown, ""},
		{http.MethodGet, "/api/v1/persons/" + personID + "/media" + unknown, ""},
		{http.MethodPut, "/api/v1/media/" + mediaID + unknown, fmt.Sprintf(`{"title":"x","version":%d}`, version)},
		{http.MethodDelete, fmt.Sprintf("/api/v1/media/%s%s&version=%d", mediaID, unknown, version), ""},
	} {
		if got := do(t, server, tc.method, tc.path, tc.body).Code; got != http.StatusNotFound {
			t.Errorf("%s %s: status = %d, want 404", tc.method, tc.path, got)
		}
	}
	req, err := createMultipartRequest("/api/v1/persons/"+personID+"/media"+unknown, "file", "p.jpg", createTestJPEGImage(), nil)
	if err != nil {
		t.Fatalf("build upload request: %v", err)
	}
	rec := httptest.NewRecorder()
	server.Echo().ServeHTTP(rec, req)
	if rec.Code != http.StatusNotFound {
		t.Errorf("Upload to an unknown branch: status = %d, want 404", rec.Code)
	}
}

// failingMainMediaStore fails every mainline SaveMedia while armed, so a merge
// appends a branch upload to main's log without projecting its row.
type failingMainMediaStore struct {
	repository.ReadModelStore
	armed bool
}

func (s *failingMainMediaStore) SaveMedia(ctx context.Context, branchID domain.BranchID, media *repository.MediaReadModel) error {
	if s.armed && branchID.IsMain() {
		return errors.New("simulated media projection failure")
	}
	return s.ReadModelStore.SaveMedia(ctx, branchID, media)
}

// TestResumeBranchMerge_MediaRepairUnsound: a branch upload lands in main's
// log but not in main's read model, then main merges its owner into another
// person. Repairing the item from its own stream would orphan it, so the
// resume refuses with a 409 merge_resume_repair_unsound — a permanent refusal
// the client can tell apart from a retryable 500 — and writes nothing.
func TestResumeBranchMerge_MediaRepairUnsound(t *testing.T) {
	cfg := &config.Config{Port: 8080, LogFormat: "text"}
	events := memory.NewEventStore()
	reads := &failingMainMediaStore{ReadModelStore: memory.NewReadModelStore()}
	server := api.NewServer(cfg, events, reads, memory.NewSnapshotStore(events), nil,
		api.WithBranchStore(memory.NewBranchStore()))

	survivorID, survivorVersion := createMergeTestPerson(t, server, "Ada", "Lovelace")
	ownerID, _ := createMergeTestPerson(t, server, "Owen", "Lovelace")
	branchID := createBranch(t, server, "Portrait of Owen")
	mediaID, _ := uploadMedia(t, server, ownerID, "?branch="+branchID, createTestJPEGImage())

	reads.armed = true
	rec := do(t, server, http.MethodPost, "/api/v1/branches/"+branchID+"/merge", `{}`)
	reads.armed = false
	if rec.Code != http.StatusInternalServerError {
		t.Fatalf("Merge: status = %d, want 500. Body: %s", rec.Code, rec.Body.String())
	}

	rec = do(t, server, http.MethodGet, "/api/v1/persons/"+ownerID, "")
	if rec.Code != http.StatusOK {
		t.Fatalf("GET owner: status = %d. Body: %s", rec.Code, rec.Body.String())
	}
	ownerVersion, _ := decodeJSON(t, rec)["version"].(float64)
	rec = do(t, server, http.MethodPost, "/api/v1/persons/merge", fmt.Sprintf(
		`{"survivor_id":%q,"merged_id":%q,"survivor_version":%d,"merged_version":%d}`,
		survivorID, ownerID, survivorVersion, int64(ownerVersion)))
	if rec.Code != http.StatusOK {
		t.Fatalf("MergePersons: status = %d. Body: %s", rec.Code, rec.Body.String())
	}

	for attempt := 1; attempt <= 2; attempt++ {
		rec = do(t, server, http.MethodPost, "/api/v1/branches/"+branchID+"/merge/resume", "")
		if rec.Code != http.StatusConflict {
			t.Fatalf("Resume #%d: status = %d, want 409. Body: %s", attempt, rec.Code, rec.Body.String())
		}
		resp := decodeJSON(t, rec)
		if resp["code"] != "merge_resume_repair_unsound" {
			t.Errorf("Resume #%d: code = %v, want merge_resume_repair_unsound", attempt, resp["code"])
		}
		if msg, _ := resp["message"].(string); !strings.Contains(msg, "rebuild main's read model") {
			t.Errorf("Resume #%d: message = %q, want the rebuild remedy", attempt, msg)
		}
	}
	if rec := do(t, server, http.MethodGet, "/api/v1/media/"+mediaID, ""); rec.Code != http.StatusNotFound {
		t.Errorf("main GET media after refused resume: status = %d, want 404", rec.Code)
	}
}
