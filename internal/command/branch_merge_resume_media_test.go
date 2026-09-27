package command_test

// Resuming a merge that carries media (#759 on top of #685): landed detection,
// the media-owner rule with resume's pending semantics, and the read-model
// repair, which must never copy file bytes per branch or lose shared ones.

import (
	"bytes"
	"context"
	"errors"
	"slices"
	"strings"
	"testing"

	"github.com/google/uuid"

	"github.com/cacack/my-family/internal/command"
	"github.com/cacack/my-family/internal/domain"
	"github.com/cacack/my-family/internal/repository"
)

func (e evidenceResume) upload(t *testing.T, h *command.Handler, owner uuid.UUID, title string) uuid.UUID {
	t.Helper()
	res, err := h.UploadMedia(context.Background(), command.UploadMediaInput{
		EntityType: "person", EntityID: owner, Title: title,
		MediaType: "photo", Filename: "scan.jpg", FileData: createTestJPEG(),
	})
	if err != nil {
		t.Fatalf("UploadMedia(%q) failed: %v", title, err)
	}
	return res.ID
}

// media reads a media item with its bytes on the given branch.
func (e evidenceResume) media(t *testing.T, branchID domain.BranchID, id uuid.UUID) *repository.MediaReadModel {
	t.Helper()
	m, err := e.f.readStore.GetMediaWithData(context.Background(), branchID, id)
	if err != nil {
		t.Fatalf("GetMediaWithData(%s) failed: %v", id, err)
	}
	return m
}

func (e evidenceResume) retitleMedia(t *testing.T, h *command.Handler, branchID domain.BranchID, id uuid.UUID, title string) {
	t.Helper()
	current := e.media(t, branchID, id)
	if current == nil {
		t.Fatalf("media %s missing on %s", id, branchID)
	}
	if _, err := h.UpdateMedia(context.Background(), command.UpdateMediaInput{ID: id, Title: &title, Version: current.Version}); err != nil {
		t.Fatalf("UpdateMedia(%s) failed: %v", id, err)
	}
}

func (e evidenceResume) mainPerson(t *testing.T, given string) uuid.UUID {
	t.Helper()
	p, err := e.f.handler.CreatePerson(context.Background(), command.CreatePersonInput{GivenName: given, Surname: "Owner"})
	if err != nil {
		t.Fatalf("CreatePerson failed: %v", err)
	}
	return p.ID
}

func (e evidenceResume) deleteMainPerson(t *testing.T, id uuid.UUID) {
	t.Helper()
	p, err := e.f.readStore.GetPerson(context.Background(), domain.MainBranchID, id)
	if err != nil || p == nil {
		t.Fatalf("GetPerson(%s) = %v, %v", id, p, err)
	}
	if err := e.f.handler.DeletePerson(context.Background(), command.DeletePersonInput{ID: id, Version: p.Version, Reason: "duplicate"}); err != nil {
		t.Fatalf("main DeletePerson failed: %v", err)
	}
}

func (e evidenceResume) renameOnBranch(t *testing.T, h *command.Handler, branchID domain.BranchID, id uuid.UUID) {
	t.Helper()
	p, err := e.f.readStore.GetPerson(context.Background(), branchID, id)
	if err != nil || p == nil {
		t.Fatalf("GetPerson(%s) = %v, %v", id, p, err)
	}
	surname := "Revised"
	if _, err := h.UpdatePerson(context.Background(), command.UpdatePersonInput{ID: id, Surname: &surname, Version: p.Version}); err != nil {
		t.Fatalf("branch UpdatePerson failed: %v", err)
	}
}

// failProjection runs a merge whose replay appends every stream but whose
// projection of one media item's mainline save fails.
func (e evidenceResume) failProjection(t *testing.T, branch *domain.Branch, mediaID uuid.UUID) {
	t.Helper()
	e.reads.armed, e.reads.failMedia = true, mediaID
	_, err := e.f.handler.MergeBranch(context.Background(), command.MergeBranchInput{BranchID: branch.ID})
	e.reads.armed, e.reads.failMedia = false, uuid.Nil
	if !errors.Is(err, command.ErrMergePartiallyApplied) {
		t.Fatalf("MergeBranch error = %v, want ErrMergePartiallyApplied", err)
	}
}

func (e evidenceResume) resume(t *testing.T, branchID uuid.UUID, resolutions map[uuid.UUID]command.MergeResolution) *command.ResumeMergeResult {
	t.Helper()
	res, err := e.f.handler.ResumeMerge(context.Background(), command.ResumeMergeInput{BranchID: branchID, Resolutions: resolutions})
	if err != nil {
		t.Fatalf("ResumeMerge failed: %v", err)
	}
	return res
}

func assertResumeNoop(t *testing.T, res *command.ResumeMergeResult) {
	t.Helper()
	if res.ReplayedEventCount != 0 || len(res.ReprojectedStreamIDs) != 0 {
		t.Errorf("repeat resume replayed %d event(s), re-projected %v; want a no-op", res.ReplayedEventCount, res.ReprojectedStreamIDs)
	}
}

// assertSharedBytes checks that a media item has the given bytes on main and,
// through the overlay, on the branch too.
func (e evidenceResume) assertSharedBytes(t *testing.T, branchID domain.BranchID, id uuid.UUID, want []byte) {
	t.Helper()
	for _, scope := range []domain.BranchID{domain.MainBranchID, branchID} {
		m := e.media(t, scope, id)
		if m == nil {
			t.Fatalf("media %s missing on %s", id, scope)
		}
		if !bytes.Equal(m.FileData, want) {
			t.Errorf("media %s bytes on %s = %d byte(s), want the %d uploaded", id, scope, len(m.FileData), len(want))
		}
	}
}

// A merge carrying a metadata edit of main's photo and a new upload (itself
// edited) is interrupted after the edit lands. The resume replays the upload,
// main keeps its original bytes for the edited photo, the new item's bytes
// reach main, and a second resume does nothing.
func TestResumeMerge_MediaUploadsAndEdits(t *testing.T) {
	e := newEvidenceResume(t)
	photo := e.upload(t, e.f.handler, e.person, "Portrait")
	original := e.media(t, domain.MainBranchID, photo).FileData
	branch, scoped := e.branch(t, "media-work")
	scope := domain.BranchID(branch.ID)
	e.retitleMedia(t, scoped, scope, photo, "Portrait, 1901")
	scan := e.upload(t, scoped, e.person, "Census scan")
	e.retitleMedia(t, scoped, scope, scan, "Census scan (cropped)")
	scanBytes := e.media(t, scope, scan).FileData

	e.interrupt(t, branch, 2)
	if got := e.media(t, domain.MainBranchID, photo); got == nil || got.Title != "Portrait, 1901" {
		t.Fatalf("main photo after the interruption = %+v, want the landed retitle", got)
	}
	if got := e.media(t, domain.MainBranchID, scan); got != nil {
		t.Fatalf("main has the new scan before the resume")
	}

	res := e.resume(t, branch.ID, nil)
	if !slices.Equal(res.AlreadyReplayedStreamIDs, []uuid.UUID{photo}) {
		t.Errorf("AlreadyReplayedStreamIDs = %v, want [%s]", res.AlreadyReplayedStreamIDs, photo)
	}
	if res.ReplayedEventCount != 2 {
		t.Errorf("ReplayedEventCount = %d, want 2 (the upload and its edit)", res.ReplayedEventCount)
	}
	if got := e.media(t, domain.MainBranchID, scan); got == nil || got.Title != "Census scan (cropped)" {
		t.Errorf("main scan = %+v, want the branch's edited upload", got)
	}
	e.assertSharedBytes(t, scope, photo, original)
	e.assertSharedBytes(t, scope, scan, scanBytes)

	assertResumeNoop(t, e.resume(t, branch.ID, nil))
	e.assertSharedBytes(t, scope, photo, original)
}

// The metadata edit's append lands but its projection fails: main's photo row
// is behind the log. The resume re-projects it forward (a metadata-only save
// that keeps main's bytes) and replays the rest.
func TestResumeMerge_RepairsMediaEditProjection(t *testing.T) {
	e := newEvidenceResume(t)
	photo := e.upload(t, e.f.handler, e.person, "Portrait")
	original := e.media(t, domain.MainBranchID, photo).FileData
	branch, scoped := e.branch(t, "retitle")
	scope := domain.BranchID(branch.ID)
	e.retitleMedia(t, scoped, scope, photo, "Portrait, 1901")
	scan := e.upload(t, scoped, e.person, "Census scan")

	e.failProjection(t, branch, photo)
	if got := e.media(t, domain.MainBranchID, photo); got == nil || got.Title != "Portrait" {
		t.Fatalf("main photo after the failed projection = %+v, want it still behind", got)
	}

	res := e.resume(t, branch.ID, nil)
	if !slices.Equal(res.ReprojectedStreamIDs, []uuid.UUID{photo}) {
		t.Errorf("ReprojectedStreamIDs = %v, want [%s]", res.ReprojectedStreamIDs, photo)
	}
	if got := e.media(t, domain.MainBranchID, photo); got == nil || got.Title != "Portrait, 1901" {
		t.Errorf("main photo = %+v, want the repaired retitle", got)
	}
	e.assertSharedBytes(t, scope, photo, original)
	if e.media(t, domain.MainBranchID, scan) == nil {
		t.Errorf("main lacks the new scan after the resume")
	}
	assertResumeNoop(t, e.resume(t, branch.ID, nil))
}

// The upload's append lands but its projection fails: main has the item in
// its log and no row. The resume re-projects MediaCreated from main's own log,
// bytes included, without appending anything.
func TestResumeMerge_RepairsMediaUploadProjection(t *testing.T) {
	e := newEvidenceResume(t)
	branch, scoped := e.branch(t, "upload")
	scope := domain.BranchID(branch.ID)
	scan := e.upload(t, scoped, e.person, "Census scan")
	scanBytes := e.media(t, scope, scan).FileData

	e.failProjection(t, branch, scan)
	if e.media(t, domain.MainBranchID, scan) != nil {
		t.Fatalf("main has the scan's row after its projection failed")
	}
	before := e.mainEventCount(t)

	res := e.resume(t, branch.ID, nil)
	if res.ReplayedEventCount != 0 || !slices.Equal(res.ReprojectedStreamIDs, []uuid.UUID{scan}) {
		t.Errorf("resume replayed %d, re-projected %v; want 0 and [%s]", res.ReplayedEventCount, res.ReprojectedStreamIDs, scan)
	}
	if got := e.mainEventCount(t); got != before {
		t.Errorf("main event count = %d, want %d (a repair appends nothing)", got, before)
	}
	e.assertSharedBytes(t, scope, scan, scanBytes)
	assertResumeNoop(t, e.resume(t, branch.ID, nil))
}

// Main deletes the person a not-yet-replayed upload is attached to after the
// interruption. The upload is pending; "branch" would orphan it and is
// refused; "main" rolls the merge forward without it.
func TestResumeMerge_MediaOwnerDeletedOnMainAfterInterruption(t *testing.T) {
	e := newEvidenceResume(t)
	owner := e.mainPerson(t, "Owen")
	branch, scoped := e.branch(t, "owner-lost")
	scope := domain.BranchID(branch.ID)
	e.renameOnBranch(t, scoped, scope, e.person)
	scan := e.upload(t, scoped, owner, "Portrait of Owen")

	e.interrupt(t, branch, 2)
	e.deleteMainPerson(t, owner)
	before := e.mainEventCount(t)

	res, err := e.f.handler.ResumeMerge(context.Background(), command.ResumeMergeInput{BranchID: branch.ID})
	if !errors.Is(err, command.ErrMergeResumeNeedsResolution) {
		t.Fatalf("ResumeMerge error = %v, want ErrMergeResumeNeedsResolution", err)
	}
	if res == nil || !slices.Equal(res.PendingStreamIDs, []uuid.UUID{scan}) {
		t.Fatalf("PendingStreamIDs = %v, want [%s]", res, scan)
	}
	_, err = e.f.handler.ResumeMerge(context.Background(), command.ResumeMergeInput{
		BranchID: branch.ID, Resolutions: map[uuid.UUID]command.MergeResolution{scan: command.ResolveBranch},
	})
	if !errors.Is(err, command.ErrMergeDanglingReference) {
		t.Fatalf("branch resolution error = %v, want ErrMergeDanglingReference", err)
	}
	if got := e.mainEventCount(t); got != before {
		t.Fatalf("refused resumes wrote %d event(s) to main", got-before)
	}

	done := e.resume(t, branch.ID, map[uuid.UUID]command.MergeResolution{scan: command.ResolveMain})
	if !slices.Equal(done.SkippedStreamIDs, []uuid.UUID{scan}) {
		t.Errorf("SkippedStreamIDs = %v, want [%s]", done.SkippedStreamIDs, scan)
	}
	if e.media(t, domain.MainBranchID, scan) != nil {
		t.Errorf("main has the orphaned upload after the resume")
	}
	assertResumeNoop(t, e.resume(t, branch.ID, nil))
}

// Main deletes the owner of a photo whose not-yet-replayed metadata edit the
// plan pinned. The owner's cascade removed the photo without writing to its
// stream, so the stream counts as removed: pending, and only "main" resolves
// it.
func TestResumeMerge_MediaEditOfCascadedItemIsRemoved(t *testing.T) {
	e := newEvidenceResume(t)
	owner := e.mainPerson(t, "Owen")
	photo := e.upload(t, e.f.handler, owner, "Portrait")
	branch, scoped := e.branch(t, "edit-lost")
	scope := domain.BranchID(branch.ID)
	e.renameOnBranch(t, scoped, scope, e.person)
	e.retitleMedia(t, scoped, scope, photo, "Portrait, 1901")

	e.interrupt(t, branch, 2)
	e.deleteMainPerson(t, owner)

	res, err := e.f.handler.ResumeMerge(context.Background(), command.ResumeMergeInput{BranchID: branch.ID})
	if !errors.Is(err, command.ErrMergeResumeNeedsResolution) || !slices.Equal(res.PendingStreamIDs, []uuid.UUID{photo}) {
		t.Fatalf("ResumeMerge = %v, %v; want the photo pending", res, err)
	}
	_, err = e.f.handler.ResumeMerge(context.Background(), command.ResumeMergeInput{
		BranchID: branch.ID, Resolutions: map[uuid.UUID]command.MergeResolution{photo: command.ResolveBranch},
	})
	if !errors.Is(err, command.ErrUnknownResolution) {
		t.Fatalf("branch resolution error = %v, want ErrUnknownResolution (removed on main)", err)
	}
	e.resume(t, branch.ID, map[uuid.UUID]command.MergeResolution{photo: command.ResolveMain})
	if e.media(t, domain.MainBranchID, photo) != nil {
		t.Errorf("main photo resurrected")
	}
}

// The upload lands in main's log but its projection fails, and main then
// deletes its owner. The owner's cascade would have removed the item, which
// main's log explains, so the resume must not re-project it into an orphan.
func TestResumeMerge_CascadedMediaIsNotResurrected(t *testing.T) {
	e := newEvidenceResume(t)
	owner := e.mainPerson(t, "Owen")
	branch, scoped := e.branch(t, "upload-then-cascade")
	scan := e.upload(t, scoped, owner, "Portrait of Owen")

	e.failProjection(t, branch, scan)
	e.deleteMainPerson(t, owner)

	res := e.resume(t, branch.ID, nil)
	if res.ReplayedEventCount != 0 || len(res.ReprojectedStreamIDs) != 0 {
		t.Errorf("resume replayed %d, re-projected %v; want nothing", res.ReplayedEventCount, res.ReprojectedStreamIDs)
	}
	if e.media(t, domain.MainBranchID, scan) != nil {
		t.Errorf("main resurrected media whose owner it deleted")
	}
}

// The upload lands in main's log but its projection fails, and main then
// merges its owner into another person. The person merge would have moved the
// item to the survivor, and that transfer is not in the media stream: the
// repair is unsound, so the resume refuses before writing anything. Once the
// survivor is deleted too the item is gone either way, and the resume
// finishes.
func TestResumeMerge_MediaOfMergedAwayOwnerIsRefused(t *testing.T) {
	e := newEvidenceResume(t)
	ctx := context.Background()
	owner := e.mainPerson(t, "Owen")
	branch, scoped := e.branch(t, "upload-then-merge")
	scan := e.upload(t, scoped, owner, "Portrait of Owen")

	e.failProjection(t, branch, scan)
	survivor, err := e.f.readStore.GetPerson(ctx, domain.MainBranchID, e.person)
	if err != nil || survivor == nil {
		t.Fatalf("GetPerson survivor = %v, %v", survivor, err)
	}
	merged, err := e.f.readStore.GetPerson(ctx, domain.MainBranchID, owner)
	if err != nil || merged == nil {
		t.Fatalf("GetPerson merged = %v, %v", merged, err)
	}
	if _, err := e.f.handler.MergePersons(ctx, command.MergePersonsInput{
		SurvivorID: e.person, MergedID: owner, SurvivorVersion: survivor.Version, MergedVersion: merged.Version,
	}); err != nil {
		t.Fatalf("main MergePersons failed: %v", err)
	}
	before := e.mainEventCount(t)

	_, err = e.f.handler.ResumeMerge(ctx, command.ResumeMergeInput{BranchID: branch.ID})
	if err == nil || !strings.Contains(err.Error(), "cannot be repaired from its own stream") {
		t.Fatalf("ResumeMerge error = %v, want the unsound-repair refusal", err)
	}
	if got := e.mainEventCount(t); got != before {
		t.Errorf("refused resume wrote %d event(s) to main", got-before)
	}
	if e.media(t, domain.MainBranchID, scan) != nil {
		t.Errorf("refused resume re-projected the media")
	}

	e.deleteMainPerson(t, e.person)
	res := e.resume(t, branch.ID, nil)
	if len(res.ReprojectedStreamIDs) != 0 || e.media(t, domain.MainBranchID, scan) != nil {
		t.Errorf("resume re-projected %v; want the cascaded media left gone", res.ReprojectedStreamIDs)
	}
}
