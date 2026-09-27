package integration_test

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"image"
	"image/color"
	"image/png"
	"net/http"
	"testing"

	"github.com/google/uuid"

	"github.com/cacack/my-family/internal/api"
	"github.com/cacack/my-family/internal/command"
	"github.com/cacack/my-family/internal/domain"
	"github.com/cacack/my-family/internal/repository"
)

// TestBranchMergeResume_Media drives, on every backend, a resume of a merge
// that carries media (#759 on top of #685): uploads and metadata edits
// interrupted mid-replay, an owner main deletes after the interruption, and a
// media projection that failed after its append. File bytes are shared, so
// every scenario also checks that main and the branch read the same bytes.
func TestBranchMergeResume_Media(t *testing.T) {
	for _, backend := range backends {
		t.Run(backend.name, func(t *testing.T) {
			t.Run("resumes uploads and edits", func(t *testing.T) {
				st := backend.setup(t)
				faulty := &faultyReplayStore{EventStore: st.events}
				wrapped := st
				wrapped.events = faulty
				runResumeMediaUploadsAndEdits(t, newServer(t, wrapped), wrapped, faulty)
			})
			t.Run("main deletes a media owner after the interruption", func(t *testing.T) {
				st := backend.setup(t)
				faulty := &faultyReplayStore{EventStore: st.events}
				wrapped := st
				wrapped.events = faulty
				runResumeMediaOwnerDeletedOnMain(t, newServer(t, wrapped), wrapped, faulty)
			})
			t.Run("repairs a failed media projection", func(t *testing.T) {
				st := backend.setup(t)
				reads := &faultyReadStore{ReadModelStore: st.read}
				wrapped := st
				wrapped.read = reads
				runResumeRepairsMediaProjection(t, newServer(t, wrapped), wrapped, reads)
			})
		})
	}
}

func (s *faultyReadStore) failMediaFor(id uuid.UUID) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.failMedia = id
}

func (s *faultyReadStore) SaveMedia(ctx context.Context, branchID domain.BranchID, media *repository.MediaReadModel) error {
	s.mu.Lock()
	fail := branchID.IsMain() && media.ID == s.failMedia
	s.mu.Unlock()
	if fail {
		return errors.New("injected media read-model failure during projection")
	}
	return s.ReadModelStore.SaveMedia(ctx, branchID, media)
}

// testPNG returns a small PNG whose pixels depend on seed, so two uploads carry
// different bytes.
func testPNG(t *testing.T, seed uint8) []byte {
	t.Helper()
	img := image.NewRGBA(image.Rect(0, 0, 8, 8))
	for y := 0; y < 8; y++ {
		for x := 0; x < 8; x++ {
			img.Set(x, y, color.RGBA{R: seed, G: uint8(x * 16), B: uint8(y * 16), A: 255})
		}
	}
	var buf bytes.Buffer
	if err := png.Encode(&buf, img); err != nil {
		t.Fatalf("png.Encode failed: %v", err)
	}
	return buf.Bytes()
}

// uploadMedia attaches a photo to a person on the given scope ("" for main)
// through the command layer on the server's stores — the upload endpoint is
// multipart, which the JSON harness does not speak — and returns its id.
func uploadMedia(t *testing.T, st stores, branchID, personID, title string, data []byte) string {
	t.Helper()
	ctx := context.Background()
	handler := command.NewHandlerWithBranches(st.events, st.read, st.branches, st.snapshots)
	if branchID != "" {
		branch, err := st.branches.Get(ctx, uuid.MustParse(branchID))
		if err != nil {
			t.Fatalf("Get branch failed: %v", err)
		}
		handler = handler.WithBranch(branch)
	}
	res, err := handler.UploadMedia(ctx, command.UploadMediaInput{
		EntityType: "person", EntityID: uuid.MustParse(personID), Title: title,
		MediaType: "photo", Filename: "photo.png", FileData: data,
	})
	if err != nil {
		t.Fatalf("UploadMedia(%q) failed: %v", title, err)
	}
	return res.ID.String()
}

// retitleMedia edits a media item's title on the given scope over HTTP.
func retitleMedia(t *testing.T, server *api.Server, branchID, mediaID, title string) {
	t.Helper()
	path := "/api/v1/media/" + mediaID
	mustDo(t, server, http.MethodPut, scoped(path, branchID),
		fmt.Sprintf(`{"title":%q,"version":%d}`, title, entityVersion(t, server, path, branchID)), http.StatusOK)
}

// assertMediaBytes checks a media item's bytes on main (downloaded over HTTP)
// and on the branch. A merged branch no longer serves a view over HTTP, so the
// branch side reads the store: the overlay must still resolve the shared
// bytes there, not a copy and not nothing.
func assertMediaBytes(t *testing.T, server *api.Server, st stores, branchID, mediaID string, want []byte) {
	t.Helper()
	rec := do(t, server, http.MethodGet, "/api/v1/media/"+mediaID+"/content", "")
	if rec.Code != http.StatusOK {
		t.Errorf("main content of %s: status = %d, want 200", mediaID, rec.Code)
	} else if !bytes.Equal(rec.Body.Bytes(), want) {
		t.Errorf("main content of %s = %d byte(s), want the %d uploaded", mediaID, rec.Body.Len(), len(want))
	}
	onBranch, err := st.read.GetMediaWithData(context.Background(), domain.BranchID(uuid.MustParse(branchID)), uuid.MustParse(mediaID))
	if err != nil {
		t.Fatalf("GetMediaWithData on the branch failed: %v", err)
	}
	if onBranch == nil || !bytes.Equal(onBranch.FileData, want) {
		t.Errorf("branch bytes of %s = %v, want the %d uploaded", mediaID, onBranch, len(want))
	}
}

// runResumeMediaUploadsAndEdits: the branch retitles main's photo P and
// uploads a new scan S (then retitles it). The merge is interrupted after P's
// edit lands; the resume must replay S — bytes included — leave P's shared
// bytes alone, and a second resume must do nothing.
func runResumeMediaUploadsAndEdits(t *testing.T, server *api.Server, st stores, faulty *faultyReplayStore) {
	t.Helper()
	person := createPerson(t, server, "Alex", "Original")
	photoBytes, scanBytes := testPNG(t, 1), testPNG(t, 2)
	photo := uploadMedia(t, st, "", person, "Portrait", photoBytes)
	branchID := createBranch(t, server, "media-work")
	branchPath := "/api/v1/branches/" + branchID
	retitleMedia(t, server, branchID, photo, "Portrait, 1901")
	scan := uploadMedia(t, st, branchID, person, "Census scan", scanBytes)
	retitleMedia(t, server, branchID, scan, "Census scan (cropped)")

	faulty.arm(2)
	failed := mustDo(t, server, http.MethodPost, branchPath+"/merge", `{}`, http.StatusInternalServerError)
	faulty.disarm()
	if failed["code"] != "merge_partially_applied" {
		t.Fatalf("merge code = %v, want merge_partially_applied", failed["code"])
	}
	if got := mustString(t, getEntity(t, server, "/api/v1/media/"+photo, ""), "title"); got != "Portrait, 1901" {
		t.Fatalf("main photo title after the interruption = %q, want the landed edit", got)
	}
	mustDo(t, server, http.MethodGet, "/api/v1/media/"+scan, "", http.StatusNotFound)

	resumed := mustDo(t, server, http.MethodPost, branchPath+"/merge/resume", "", http.StatusOK)
	if already := entryStrings(t, jsonArray(t, resumed, "already_replayed_stream_ids")); len(already) != 1 || already[0] != photo {
		t.Errorf("already_replayed_stream_ids = %v, want [%s]", already, photo)
	}
	if got := resumed["replayed_event_count"]; got != float64(2) {
		t.Errorf("replayed_event_count = %v, want 2 (the upload and its edit)", got)
	}
	if got := mustString(t, getEntity(t, server, "/api/v1/media/"+scan, ""), "title"); got != "Census scan (cropped)" {
		t.Errorf("main scan title = %q, want the branch's edit", got)
	}
	assertMediaBytes(t, server, st, branchID, photo, photoBytes)
	assertMediaBytes(t, server, st, branchID, scan, scanBytes)

	again := mustDo(t, server, http.MethodPost, branchPath+"/merge/resume", "", http.StatusOK)
	if got := again["replayed_event_count"]; got != float64(0) {
		t.Errorf("second resume replayed_event_count = %v, want 0", got)
	}
	if got := jsonArray(t, again, "reprojected_stream_ids"); len(got) != 0 {
		t.Errorf("second resume reprojected_stream_ids = %v, want none", got)
	}
	assertMediaBytes(t, server, st, branchID, photo, photoBytes)
}

// runResumeMediaOwnerDeletedOnMain: the branch renames a person and uploads a
// photo of main's person O; the replay stops after the rename, and main then
// deletes O. Replaying the upload would orphan it, so it is pending, "branch"
// is refused, and "main" rolls forward without it.
func runResumeMediaOwnerDeletedOnMain(t *testing.T, server *api.Server, st stores, faulty *faultyReplayStore) {
	t.Helper()
	person := createPerson(t, server, "Alex", "Original")
	owner := createPerson(t, server, "Owen", "Owner")
	branchID := createBranch(t, server, "owner-lost")
	branchPath := "/api/v1/branches/" + branchID
	personPath := "/api/v1/persons/" + person
	mustDo(t, server, http.MethodPut, scoped(personPath, branchID),
		fmt.Sprintf(`{"surname":"Revised","version":%d}`, entityVersion(t, server, personPath, branchID)), http.StatusOK)
	scan := uploadMedia(t, st, branchID, owner, "Portrait of Owen", testPNG(t, 3))

	faulty.arm(2)
	mustDo(t, server, http.MethodPost, branchPath+"/merge", `{}`, http.StatusInternalServerError)
	faulty.disarm()
	mustDo(t, server, http.MethodDelete, "/api/v1/persons/"+owner, "", http.StatusNoContent)

	refused := mustDo(t, server, http.MethodPost, branchPath+"/merge/resume", "", http.StatusConflict)
	if pending := entryStrings(t, jsonArray(t, refused, "pending_stream_ids")); len(pending) != 1 || pending[0] != scan {
		t.Fatalf("pending_stream_ids = %v, want [%s]", pending, scan)
	}
	dangling := mustDo(t, server, http.MethodPost, branchPath+"/merge/resume",
		fmt.Sprintf(`{"resolutions":[{"stream_id":%q,"resolution":"branch"}]}`, scan), http.StatusConflict)
	if dangling["code"] != "merge_dangling_reference" {
		t.Errorf("branch resolution code = %v, want merge_dangling_reference", dangling["code"])
	}
	done := mustDo(t, server, http.MethodPost, branchPath+"/merge/resume",
		fmt.Sprintf(`{"resolutions":[{"stream_id":%q,"resolution":"main"}]}`, scan), http.StatusOK)
	if skipped := entryStrings(t, jsonArray(t, done, "skipped_stream_ids")); len(skipped) != 1 || skipped[0] != scan {
		t.Errorf("skipped_stream_ids = %v, want [%s]", skipped, scan)
	}
	mustDo(t, server, http.MethodGet, "/api/v1/media/"+scan, "", http.StatusNotFound)
	if got := mustString(t, getEntity(t, server, personPath, ""), "surname"); got != "Revised" {
		t.Errorf("person surname on main = %q, want the landed rename", got)
	}

	again := mustDo(t, server, http.MethodPost, branchPath+"/merge/resume", "", http.StatusOK)
	if got := again["replayed_event_count"]; got != float64(0) {
		t.Errorf("second resume replayed_event_count = %v, want 0", got)
	}
}

// runResumeRepairsMediaProjection: the branch uploads a photo; the merge
// appends it but its main projection fails, so main's log has the item and its
// read model does not. The resume re-projects it from main's own log — bytes
// included — without appending, and a second resume finds nothing to do.
func runResumeRepairsMediaProjection(t *testing.T, server *api.Server, st stores, reads *faultyReadStore) {
	t.Helper()
	person := createPerson(t, server, "Alex", "Original")
	branchID := createBranch(t, server, "upload-line")
	branchPath := "/api/v1/branches/" + branchID
	data := testPNG(t, 4)
	scan := uploadMedia(t, st, branchID, person, "Census scan", data)

	reads.failMediaFor(uuid.MustParse(scan))
	failed := mustDo(t, server, http.MethodPost, branchPath+"/merge", `{}`, http.StatusInternalServerError)
	reads.failMediaFor(uuid.Nil)
	if failed["code"] != "merge_partially_applied" {
		t.Fatalf("merge code = %v, want merge_partially_applied", failed["code"])
	}
	mustDo(t, server, http.MethodGet, "/api/v1/media/"+scan, "", http.StatusNotFound)
	logBefore := mainEventCount(t, context.Background(), st.events, scan)

	resumed := mustDo(t, server, http.MethodPost, branchPath+"/merge/resume", "", http.StatusOK)
	if got := resumed["replayed_event_count"]; got != float64(0) {
		t.Errorf("replayed_event_count = %v, want 0 (the upload is already in the log)", got)
	}
	if got := entryStrings(t, jsonArray(t, resumed, "reprojected_stream_ids")); len(got) != 1 || got[0] != scan {
		t.Errorf("reprojected_stream_ids = %v, want [%s]", got, scan)
	}
	if got := mainEventCount(t, context.Background(), st.events, scan); got != logBefore {
		t.Errorf("media has %d main events after resume, want %d (no re-append)", got, logBefore)
	}
	assertMediaBytes(t, server, st, branchID, scan, data)

	again := mustDo(t, server, http.MethodPost, branchPath+"/merge/resume", "", http.StatusOK)
	if got := jsonArray(t, again, "reprojected_stream_ids"); len(got) != 0 {
		t.Errorf("second resume reprojected_stream_ids = %v, want none", got)
	}
}
