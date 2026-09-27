package command_test

// Resuming a merge that carries media (#759 on top of #685): landed detection,
// the media-owner rule with resume's pending semantics, and the read-model
// repair, which must never copy file bytes per branch or lose shared ones.

import (
	"bytes"
	"context"
	"errors"
	"slices"
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

// The crop edit's append lands but its projection fails. The repair
// re-projects the event decoded from main's log, where the crop numbers are
// float64, and must still land the crop box on main (#759).
func TestResumeMerge_RepairsMediaCropProjection(t *testing.T) {
	e := newEvidenceResume(t)
	photo := e.upload(t, e.f.handler, e.person, "Portrait")
	branch, scoped := e.branch(t, "crop")
	scope := domain.BranchID(branch.ID)
	cropMedia(t, scoped, e.media(t, scope, photo), "Portrait (cropped)", 12)

	e.failProjection(t, branch, photo)
	if got := e.media(t, domain.MainBranchID, photo); got == nil || got.CropLeft != nil {
		t.Fatalf("main photo after the failed projection = %+v, want it still uncropped", got)
	}

	res := e.resume(t, branch.ID, nil)
	if !slices.Equal(res.ReprojectedStreamIDs, []uuid.UUID{photo}) {
		t.Errorf("ReprojectedStreamIDs = %v, want [%s]", res.ReprojectedStreamIDs, photo)
	}
	got := e.media(t, domain.MainBranchID, photo)
	if got == nil || got.Title != "Portrait (cropped)" {
		t.Fatalf("main photo = %+v, want the repaired edit", got)
	}
	assertCrop(t, got, 12)
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

// mergeMainPersons merges merged into survivor on main.
func (e evidenceResume) mergeMainPersons(t *testing.T, survivor, merged uuid.UUID) {
	t.Helper()
	ctx := context.Background()
	s, err := e.f.readStore.GetPerson(ctx, domain.MainBranchID, survivor)
	if err != nil || s == nil {
		t.Fatalf("GetPerson survivor = %v, %v", s, err)
	}
	m, err := e.f.readStore.GetPerson(ctx, domain.MainBranchID, merged)
	if err != nil || m == nil {
		t.Fatalf("GetPerson merged = %v, %v", m, err)
	}
	if _, err := e.f.handler.MergePersons(ctx, command.MergePersonsInput{
		SurvivorID: survivor, MergedID: merged, SurvivorVersion: s.Version, MergedVersion: m.Version,
	}); err != nil {
		t.Fatalf("main MergePersons failed: %v", err)
	}
}

// The upload lands in main's log but its projection fails, and main then
// merges its owner into another person — and that one into a third. The
// person merges would have moved the item to the final survivor, and that
// transfer is not in the media stream, so the resume re-projects the item
// from main's log and then re-links it to the final survivor, exactly as the
// merges would have: bytes and version as the log has them, nothing appended,
// and a second resume does nothing.
func TestResumeMerge_MediaOfMergedAwayOwnerIsRelinked(t *testing.T) {
	e := newEvidenceResume(t)
	owner := e.mainPerson(t, "Owen")
	final := e.mainPerson(t, "Fay")
	branch, scoped := e.branch(t, "upload-then-merge")
	scope := domain.BranchID(branch.ID)
	scan := e.upload(t, scoped, owner, "Portrait of Owen")
	scanBytes := e.media(t, scope, scan).FileData

	e.failProjection(t, branch, scan)
	e.mergeMainPersons(t, e.person, owner)
	e.mergeMainPersons(t, final, e.person)
	before := e.mainEventCount(t)

	res := e.resume(t, branch.ID, nil)
	if res.ReplayedEventCount != 0 || !slices.Equal(res.ReprojectedStreamIDs, []uuid.UUID{scan}) {
		t.Errorf("resume replayed %d, re-projected %v; want 0 and [%s]", res.ReplayedEventCount, res.ReprojectedStreamIDs, scan)
	}
	if got := e.mainEventCount(t); got != before {
		t.Errorf("main event count = %d, want %d (a repair appends nothing)", got, before)
	}
	row := e.media(t, domain.MainBranchID, scan)
	if row == nil {
		t.Fatalf("main has no row for the repaired media")
	}
	if row.EntityType != "person" || row.EntityID != final {
		t.Errorf("repaired media is attached to %s %s, want the final merge survivor %s", row.EntityType, row.EntityID, final)
	}
	head, err := e.f.eventStore.GetStreamVersion(context.Background(), scan, domain.MainBranchID)
	if err != nil {
		t.Fatalf("GetStreamVersion failed: %v", err)
	}
	if row.Version != head {
		t.Errorf("repaired media version = %d, want main's stream version %d", row.Version, head)
	}
	if !bytes.Equal(row.FileData, scanBytes) {
		t.Errorf("repaired media bytes = %d byte(s), want the %d uploaded", len(row.FileData), len(scanBytes))
	}
	listed, _, err := e.f.readStore.ListMediaForEntity(context.Background(), "person", final, repository.ListOptions{Limit: 10, BranchID: domain.MainBranchID})
	if err != nil || len(listed) != 1 || listed[0].ID != scan {
		t.Errorf("survivor's media = %v, %v; want the repaired item", listed, err)
	}
	assertResumeNoop(t, e.resume(t, branch.ID, nil))
}

// As above, but main deletes the survivor before the resume: the item is gone
// either way (the survivor's delete cascade), so nothing is re-projected.
func TestResumeMerge_MediaOfMergedThenDeletedOwnerStaysGone(t *testing.T) {
	e := newEvidenceResume(t)
	owner := e.mainPerson(t, "Owen")
	branch, scoped := e.branch(t, "upload-merge-delete")
	scan := e.upload(t, scoped, owner, "Portrait of Owen")

	e.failProjection(t, branch, scan)
	e.mergeMainPersons(t, e.person, owner)
	e.deleteMainPerson(t, e.person)

	res := e.resume(t, branch.ID, nil)
	if len(res.ReprojectedStreamIDs) != 0 || e.media(t, domain.MainBranchID, scan) != nil {
		t.Errorf("resume re-projected %v; want the cascaded media left gone", res.ReprojectedStreamIDs)
	}
}

// The branch also edits the upload's owner, and main merges that owner away
// during the interruption, so the owner's edit is pending and can only be
// resolved to main. That does not orphan the landed upload: its repair
// re-links it to the survivor main still has.
func TestResumeMerge_MergedAwayOwnerResolvedToMainKeepsRelinkedMedia(t *testing.T) {
	e := newEvidenceResume(t)
	owner := e.mainPerson(t, "Owen")
	branch, scoped := e.branch(t, "upload-and-edit-then-merge")
	scan := e.upload(t, scoped, owner, "Portrait of Owen")
	e.renameOnBranch(t, scoped, domain.BranchID(branch.ID), owner)

	e.failProjection(t, branch, scan)
	e.mergeMainPersons(t, e.person, owner)

	res := e.resume(t, branch.ID, map[uuid.UUID]command.MergeResolution{owner: command.ResolveMain})
	if !slices.Contains(res.ReprojectedStreamIDs, scan) {
		t.Errorf("resume re-projected %v; want the media %s", res.ReprojectedStreamIDs, scan)
	}
	row := e.media(t, domain.MainBranchID, scan)
	if row == nil || row.EntityID != e.person {
		t.Fatalf("repaired media = %+v, want it attached to the survivor %s", row, e.person)
	}
	assertResumeNoop(t, e.resume(t, branch.ID, nil))
}

// A branch creates a person, uploads a photo of them and deletes them. The
// replay moves the upload ahead of the person's stream; the merge is
// interrupted after the upload lands, so main briefly holds a photo of a
// person it does not have yet. The resume replays the person's create and
// delete, whose cascade removes the photo, and ends with neither on main.
func TestResumeMerge_MediaBeforeOwnerDeleteFinishes(t *testing.T) {
	e := newEvidenceResume(t)
	ctx := context.Background()
	branch, scoped := e.branch(t, "self-cancelling")
	scope := domain.BranchID(branch.ID)
	p, err := scoped.CreatePerson(ctx, command.CreatePersonInput{GivenName: "Brief", Surname: "Hypothesis"})
	if err != nil {
		t.Fatalf("CreatePerson failed: %v", err)
	}
	scan := e.upload(t, scoped, p.ID, "Portrait of Brief")
	current, err := e.f.readStore.GetPerson(ctx, scope, p.ID)
	if err != nil || current == nil {
		t.Fatalf("GetPerson = %v, %v", current, err)
	}
	if err := scoped.DeletePerson(ctx, command.DeletePersonInput{ID: p.ID, Version: current.Version, Reason: "theory"}); err != nil {
		t.Fatalf("DeletePerson failed: %v", err)
	}

	e.interrupt(t, branch, 2)
	if e.media(t, domain.MainBranchID, scan) == nil {
		t.Fatalf("main lacks the upload after the interruption; want it replayed first")
	}

	res := e.resume(t, branch.ID, nil)
	if !slices.Equal(res.AlreadyReplayedStreamIDs, []uuid.UUID{scan}) {
		t.Errorf("AlreadyReplayedStreamIDs = %v, want [%s]", res.AlreadyReplayedStreamIDs, scan)
	}
	if got, err := e.f.readStore.GetPerson(ctx, domain.MainBranchID, p.ID); err != nil || got != nil {
		t.Errorf("main person = %v (err=%v), want absent", got, err)
	}
	if e.media(t, domain.MainBranchID, scan) != nil {
		t.Errorf("main still has the photo after its owner's delete replayed")
	}
	assertResumeNoop(t, e.resume(t, branch.ID, nil))
}

// The branch deletes a person; the replay stops before that delete lands, and
// main then uploads a photo of the person. Replaying the delete would cascade
// the photo away on main with no record, so the delete is pending, "branch" is
// refused as a dangling reference, and "main" keeps both.
func TestResumeMerge_OwnerDeleteOntoLaterMainMediaIsPending(t *testing.T) {
	e := newEvidenceResume(t)
	ctx := context.Background()
	owner := e.mainPerson(t, "Owen")
	branch, scoped := e.branch(t, "tidy-owner")
	scope := domain.BranchID(branch.ID)
	e.renameOnBranch(t, scoped, scope, e.person)
	current, err := e.f.readStore.GetPerson(ctx, scope, owner)
	if err != nil || current == nil {
		t.Fatalf("GetPerson = %v, %v", current, err)
	}
	if err := scoped.DeletePerson(ctx, command.DeletePersonInput{ID: owner, Version: current.Version, Reason: "theory"}); err != nil {
		t.Fatalf("branch DeletePerson failed: %v", err)
	}

	e.interrupt(t, branch, 2)
	scan := e.upload(t, e.f.handler, owner, "Portrait of Owen")
	before := e.mainEventCount(t)

	res, err := e.f.handler.ResumeMerge(ctx, command.ResumeMergeInput{BranchID: branch.ID})
	if !errors.Is(err, command.ErrMergeResumeNeedsResolution) {
		t.Fatalf("ResumeMerge error = %v, want ErrMergeResumeNeedsResolution", err)
	}
	if res == nil || !slices.Equal(res.PendingStreamIDs, []uuid.UUID{owner}) {
		t.Fatalf("PendingStreamIDs = %v, want [%s]", res, owner)
	}
	_, err = e.f.handler.ResumeMerge(ctx, command.ResumeMergeInput{
		BranchID: branch.ID, Resolutions: map[uuid.UUID]command.MergeResolution{owner: command.ResolveBranch},
	})
	if !errors.Is(err, command.ErrMergeDanglingReference) {
		t.Fatalf("branch resolution error = %v, want ErrMergeDanglingReference", err)
	}
	if got := e.mainEventCount(t); got != before {
		t.Fatalf("refused resumes wrote %d event(s) to main", got-before)
	}

	done := e.resume(t, branch.ID, map[uuid.UUID]command.MergeResolution{owner: command.ResolveMain})
	if !slices.Equal(done.SkippedStreamIDs, []uuid.UUID{owner}) {
		t.Errorf("SkippedStreamIDs = %v, want [%s]", done.SkippedStreamIDs, owner)
	}
	if e.media(t, domain.MainBranchID, scan) == nil {
		t.Error("main lost the photo to a delete the resume rolled forward without")
	}
	if p, err := e.f.readStore.GetPerson(ctx, domain.MainBranchID, owner); err != nil || p == nil {
		t.Errorf("main person = %v (err=%v), want kept", p, err)
	}
	assertResumeNoop(t, e.resume(t, branch.ID, nil))
}

// uploadTo uploads a photo attached to any media owner type.
func (e evidenceResume) uploadTo(t *testing.T, h *command.Handler, entityType string, owner uuid.UUID, title string) uuid.UUID {
	t.Helper()
	res, err := h.UploadMedia(context.Background(), command.UploadMediaInput{
		EntityType: entityType, EntityID: owner, Title: title,
		MediaType: "photo", Filename: "scan.jpg", FileData: createTestJPEG(),
	})
	if err != nil {
		t.Fatalf("UploadMedia(%s %q) failed: %v", entityType, title, err)
	}
	return res.ID
}

// refuseMainResolution asserts that resolving owner to main is refused as a
// dangling reference and records nothing.
func (e evidenceResume) refuseMainResolution(t *testing.T, branch *domain.Branch, owner uuid.UUID) {
	t.Helper()
	before := e.mainEventCount(t)
	_, err := e.f.handler.ResumeMerge(context.Background(), command.ResumeMergeInput{
		BranchID: branch.ID, Resolutions: map[uuid.UUID]command.MergeResolution{owner: command.ResolveMain},
	})
	if !errors.Is(err, command.ErrMergeDanglingReference) {
		t.Fatalf("ResumeMerge(%s: main) error = %v, want ErrMergeDanglingReference", owner, err)
	}
	if after := e.mainEventCount(t); after != before {
		t.Errorf("the refused resume wrote %d main event(s), want none", after-before)
	}
	if records := branchResumeRecords(t, e.f, branch); len(records) != 0 {
		t.Errorf("the refused resume recorded %d decision(s), want none", len(records))
	}
}

// The branch creates family F with main's person Q, uploads a photo of F and
// deletes F. The replay lands the photo ahead of F's stream (whose delete is
// to cascade it away) and is interrupted before F's stream; main then deletes
// Q, so F's stream is pending. Resolving it to "main" would skip the delete and
// leave main's photo attached to a family main never had, so it is refused.
// Once main deletes the photo itself, "main" finishes with nothing orphaned.
func TestResumeMerge_RefusesSkippingOwnerCreateDeleteUnderLandedMedia(t *testing.T) {
	e := newEvidenceResume(t)
	ctx := context.Background()
	partner := e.mainPerson(t, "Quinn")
	branch, scoped := e.branch(t, "short-lived-family")
	scope := domain.BranchID(branch.ID)
	fam, err := scoped.CreateFamily(ctx, command.CreateFamilyInput{Partner1ID: &partner})
	if err != nil {
		t.Fatalf("branch CreateFamily failed: %v", err)
	}
	scan := e.uploadTo(t, scoped, "family", fam.ID, "Wedding portrait")
	current, err := e.f.readStore.GetFamily(ctx, scope, fam.ID)
	if err != nil || current == nil {
		t.Fatalf("GetFamily = %v, %v", current, err)
	}
	if err := scoped.DeleteFamily(ctx, command.DeleteFamilyInput{ID: fam.ID, Version: current.Version}); err != nil {
		t.Fatalf("branch DeleteFamily failed: %v", err)
	}

	e.interrupt(t, branch, 2)
	if e.media(t, domain.MainBranchID, scan) == nil {
		t.Fatalf("main lacks the upload after the interruption; want it replayed ahead of the family")
	}
	e.deleteMainPerson(t, partner)

	res, err := e.f.handler.ResumeMerge(ctx, command.ResumeMergeInput{BranchID: branch.ID})
	if !errors.Is(err, command.ErrMergeResumeNeedsResolution) {
		t.Fatalf("bare ResumeMerge error = %v, want ErrMergeResumeNeedsResolution", err)
	}
	if res == nil || !slices.Equal(res.PendingStreamIDs, []uuid.UUID{fam.ID}) {
		t.Fatalf("PendingStreamIDs = %v, want [%s]", res, fam.ID)
	}
	e.refuseMainResolution(t, branch, fam.ID)
	if e.media(t, domain.MainBranchID, scan) == nil {
		t.Fatalf("the refused resume removed main's photo")
	}

	// Main deletes the photo itself; nothing is left to orphan.
	photo := e.media(t, domain.MainBranchID, scan)
	if err := e.f.handler.DeleteMedia(ctx, scan, photo.Version, "stray"); err != nil {
		t.Fatalf("main DeleteMedia failed: %v", err)
	}
	done := e.resume(t, branch.ID, map[uuid.UUID]command.MergeResolution{fam.ID: command.ResolveMain})
	if !slices.Equal(done.SkippedStreamIDs, []uuid.UUID{fam.ID}) {
		t.Errorf("SkippedStreamIDs = %v, want [%s]", done.SkippedStreamIDs, fam.ID)
	}
	if got, err := e.f.readStore.GetFamily(ctx, domain.MainBranchID, fam.ID); err != nil || got != nil {
		t.Errorf("main family = %v (err=%v), want absent", got, err)
	}
	if e.media(t, domain.MainBranchID, scan) != nil {
		t.Errorf("main still has the photo")
	}
	assertResumeNoop(t, e.resume(t, branch.ID, nil))
}

// legacyClaimWithLanded writes a pre-#685 claim (no recorded plan, so every
// unreplayed stream is left to the caller) and then replays the given streams'
// branch events onto main by hand, as that merge would have before it was
// interrupted. The replayed streams are not projected: the resume's repair
// brings main's read model level with them.
func (e evidenceResume) legacyClaimWithLanded(t *testing.T, branch *domain.Branch, landed ...uuid.UUID) {
	t.Helper()
	ctx := context.Background()
	f := e.f
	claim := domain.BranchMerged{
		BaseEvent:        domain.NewBaseEvent(),
		BranchID:         branch.ID,
		BasePosition:     branch.BasePosition,
		MergedAtPosition: logHead(t, f),
	}
	scope := repository.AppendScope{BranchID: domain.BranchID(branch.ID), BasePosition: branch.BasePosition}
	version, err := f.eventStore.GetStreamVersion(ctx, branch.ID, scope.BranchID)
	if err != nil {
		t.Fatalf("GetStreamVersion failed: %v", err)
	}
	if err := f.eventStore.Append(ctx, branch.ID, "branch", []domain.Event{claim}, version, scope); err != nil {
		t.Fatalf("appending legacy claim failed: %v", err)
	}
	for _, streamID := range landed {
		stored := branchEventsFor(t, f, streamID, domain.BranchID(branch.ID))
		var events []domain.Event
		for i := range stored {
			decoded, err := stored[i].DecodeEvent()
			if err != nil {
				t.Fatalf("DecodeEvent failed: %v", err)
			}
			events = append(events, decoded)
		}
		if err := f.eventStore.Append(ctx, streamID, stored[0].StreamType, events, 0, repository.MainScope); err != nil {
			t.Fatalf("replaying stream %s by hand failed: %v", streamID, err)
		}
	}
}

// The person and source variants of the case above: the branch creates an
// owner, uploads a photo of it and deletes it; a pre-#685 claim lands the
// upload and leaves the owner's create+delete stream to the caller. "main"
// would leave the photo attached to an owner main never had, so it is
// refused; "branch" replays the delete, which cascades the photo away.
func TestResumeMerge_LegacyClaimRefusesSkippingOwnerCreateDeleteUnderLandedMedia(t *testing.T) {
	cases := []struct {
		entityType string
		create     func(t *testing.T, e evidenceResume, h *command.Handler) uuid.UUID
		remove     func(t *testing.T, e evidenceResume, h *command.Handler, scope domain.BranchID, id uuid.UUID)
		onMain     func(t *testing.T, e evidenceResume, id uuid.UUID) bool
	}{
		{
			entityType: "person",
			create: func(t *testing.T, _ evidenceResume, h *command.Handler) uuid.UUID {
				p, err := h.CreatePerson(context.Background(), command.CreatePersonInput{GivenName: "Brief", Surname: "Hypothesis"})
				if err != nil {
					t.Fatalf("CreatePerson failed: %v", err)
				}
				return p.ID
			},
			remove: func(t *testing.T, e evidenceResume, h *command.Handler, scope domain.BranchID, id uuid.UUID) {
				p, err := e.f.readStore.GetPerson(context.Background(), scope, id)
				if err != nil || p == nil {
					t.Fatalf("GetPerson = %v, %v", p, err)
				}
				if err := h.DeletePerson(context.Background(), command.DeletePersonInput{ID: id, Version: p.Version, Reason: "theory"}); err != nil {
					t.Fatalf("DeletePerson failed: %v", err)
				}
			},
			onMain: func(t *testing.T, e evidenceResume, id uuid.UUID) bool {
				p, err := e.f.readStore.GetPerson(context.Background(), domain.MainBranchID, id)
				if err != nil {
					t.Fatalf("GetPerson failed: %v", err)
				}
				return p != nil
			},
		},
		{
			entityType: "source",
			create: func(t *testing.T, e evidenceResume, h *command.Handler) uuid.UUID {
				return e.source(t, h, "Parish register").ID
			},
			remove: func(t *testing.T, e evidenceResume, h *command.Handler, scope domain.BranchID, id uuid.UUID) {
				s, err := e.f.readStore.GetSource(context.Background(), scope, id)
				if err != nil || s == nil {
					t.Fatalf("GetSource = %v, %v", s, err)
				}
				if err := h.DeleteSource(context.Background(), id, s.Version, "theory"); err != nil {
					t.Fatalf("DeleteSource failed: %v", err)
				}
			},
			onMain: func(_ *testing.T, e evidenceResume, id uuid.UUID) bool {
				return e.mainSource(t, id) != nil
			},
		},
	}
	for _, tc := range cases {
		t.Run(tc.entityType, func(t *testing.T) {
			e := newEvidenceResume(t)
			branch, scoped := e.branch(t, "short-lived-"+tc.entityType)
			scope := domain.BranchID(branch.ID)
			owner := tc.create(t, e, scoped)
			scan := e.uploadTo(t, scoped, tc.entityType, owner, "Scan")
			tc.remove(t, e, scoped, scope, owner)
			e.legacyClaimWithLanded(t, branch, scan)

			res, err := e.f.handler.ResumeMerge(context.Background(), command.ResumeMergeInput{BranchID: branch.ID})
			if !errors.Is(err, command.ErrMergeResumeNeedsResolution) {
				t.Fatalf("bare ResumeMerge error = %v, want ErrMergeResumeNeedsResolution", err)
			}
			if res == nil || !slices.Equal(res.PendingStreamIDs, []uuid.UUID{owner}) {
				t.Fatalf("PendingStreamIDs = %v, want [%s]", res, owner)
			}
			e.refuseMainResolution(t, branch, owner)

			done := e.resume(t, branch.ID, map[uuid.UUID]command.MergeResolution{owner: command.ResolveBranch})
			if done.ReplayedEventCount == 0 {
				t.Errorf("ReplayedEventCount = 0, want the owner's stream replayed")
			}
			if tc.onMain(t, e, owner) {
				t.Errorf("main has the %s after its delete replayed", tc.entityType)
			}
			if e.media(t, domain.MainBranchID, scan) != nil {
				t.Errorf("main still has the photo after its owner's delete replayed")
			}
			assertResumeNoop(t, e.resume(t, branch.ID, nil))
		})
	}
}
