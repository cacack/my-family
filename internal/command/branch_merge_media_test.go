package command_test

import (
	"context"
	"testing"

	"github.com/google/uuid"

	"github.com/cacack/my-family/internal/command"
	"github.com/cacack/my-family/internal/domain"
)

func (e evidenceFixture) upload(t *testing.T, h *command.Handler, entityType string, owner uuid.UUID) uuid.UUID {
	t.Helper()
	res, err := h.UploadMedia(context.Background(), command.UploadMediaInput{
		EntityType: entityType, EntityID: owner, Title: "Scan",
		MediaType: "photo", Filename: "scan.jpg", FileData: createTestJPEG(),
	})
	if err != nil {
		t.Fatalf("UploadMedia failed: %v", err)
	}
	return res.ID
}

func (e evidenceFixture) deletePerson(t *testing.T, h *command.Handler, branchID domain.BranchID, personID uuid.UUID) {
	t.Helper()
	ctx := context.Background()
	p, err := e.f.readStore.GetPerson(ctx, branchID, personID)
	if err != nil || p == nil {
		t.Fatalf("GetPerson(%s) = %v, %v", personID, p, err)
	}
	if err := h.DeletePerson(ctx, command.DeletePersonInput{ID: personID, Version: p.Version, Reason: "theory"}); err != nil {
		t.Fatalf("DeletePerson failed: %v", err)
	}
}

func (e evidenceFixture) assertNoMainMedia(t *testing.T, mediaID uuid.UUID) {
	t.Helper()
	got, err := e.f.readStore.GetMediaWithData(context.Background(), domain.MainBranchID, mediaID)
	if err != nil {
		t.Fatalf("GetMediaWithData failed: %v", err)
	}
	if got != nil {
		t.Errorf("main has media %s after the merge; want it absent", mediaID)
	}
}

// A branch uploads a photo of a person main deletes after the fork. Replaying
// the upload would leave main with a media item attached to nobody (#759).
func TestMergeBranch_MediaOfMainDeletedPersonIsRefused(t *testing.T) {
	e := newEvidenceFixture(t)
	ctx := context.Background()

	mediaID := e.upload(t, e.f.handler.WithBranch(e.branch), "person", e.person)
	e.deletePerson(t, e.f.handler, domain.MainBranchID, e.person)

	_, err := e.f.handler.MergeBranch(ctx, command.MergeBranchInput{BranchID: e.branch.ID})
	e.assertRefusedBeforeClaim(t, err)
	e.assertNoMainMedia(t, mediaID)
}

// Same shape with a source owner: main deletes the source the branch attached
// a scan to.
func TestMergeBranch_MediaOfMainDeletedSourceIsRefused(t *testing.T) {
	e := newEvidenceFixture(t)
	ctx := context.Background()

	mediaID := e.upload(t, e.f.handler.WithBranch(e.branch), "source", e.source)
	e.deleteSource(t, e.f.handler, domain.MainBranchID, e.source)

	_, err := e.f.handler.MergeBranch(ctx, command.MergeBranchInput{BranchID: e.branch.ID})
	e.assertRefusedBeforeClaim(t, err)
	e.assertNoMainMedia(t, mediaID)
}

// Same shape with a family owner.
func TestMergeBranch_MediaOfMainDeletedFamilyIsRefused(t *testing.T) {
	e := newEvidenceFixture(t)
	ctx := context.Background()

	fam, err := e.f.handler.CreateFamily(ctx, command.CreateFamilyInput{Partner1ID: &e.person})
	if err != nil {
		t.Fatalf("CreateFamily failed: %v", err)
	}
	// Re-fork so the branch sees the family.
	branch, err := e.f.handler.CreateBranch(ctx, "family-scan", "")
	if err != nil {
		t.Fatalf("CreateBranch failed: %v", err)
	}
	e.branch = branch
	mediaID := e.upload(t, e.f.handler.WithBranch(branch), "family", fam.ID)
	f, err := e.f.readStore.GetFamily(ctx, domain.MainBranchID, fam.ID)
	if err != nil || f == nil {
		t.Fatalf("GetFamily = %v, %v", f, err)
	}
	if err := e.f.handler.DeleteFamily(ctx, command.DeleteFamilyInput{ID: fam.ID, Version: f.Version}); err != nil {
		t.Fatalf("DeleteFamily failed: %v", err)
	}

	_, err = e.f.handler.MergeBranch(ctx, command.MergeBranchInput{BranchID: branch.ID})
	e.assertRefusedBeforeClaim(t, err)
	e.assertNoMainMedia(t, mediaID)
}

// A branch creates a person, uploads a photo of them, then deletes them. The
// person's stream is touched first, so its delete would replay before the
// upload and the upload would land on a person main no longer has.
func TestMergeBranch_MediaOfPersonDeletedEarlierInReplayIsRefused(t *testing.T) {
	e := newEvidenceFixture(t)
	ctx := context.Background()
	scoped := e.f.handler.WithBranch(e.branch)
	branchID := domain.BranchID(e.branch.ID)

	p, err := scoped.CreatePerson(ctx, command.CreatePersonInput{GivenName: "Brief", Surname: "Hypothesis"})
	if err != nil {
		t.Fatalf("CreatePerson failed: %v", err)
	}
	mediaID := e.upload(t, scoped, "person", p.ID)
	e.deletePerson(t, scoped, branchID, p.ID)

	_, err = e.f.handler.MergeBranch(ctx, command.MergeBranchInput{BranchID: e.branch.ID})
	e.assertRefusedBeforeClaim(t, err)
	e.assertNoMainMedia(t, mediaID)
}

// The branch uploads a photo of main's person and later deletes that person.
// The media stream replays first and the person's delete then cascades it on
// main, so nothing dangles and the merge goes through.
func TestMergeBranch_MediaThenOwnerDeleteOnBranchMerges(t *testing.T) {
	e := newEvidenceFixture(t)
	ctx := context.Background()
	scoped := e.f.handler.WithBranch(e.branch)

	mediaID := e.upload(t, scoped, "person", e.person)
	e.deletePerson(t, scoped, domain.BranchID(e.branch.ID), e.person)

	if _, err := e.f.handler.MergeBranch(ctx, command.MergeBranchInput{BranchID: e.branch.ID}); err != nil {
		t.Fatalf("MergeBranch failed: %v", err)
	}
	if p, err := e.f.readStore.GetPerson(ctx, domain.MainBranchID, e.person); err != nil || p != nil {
		t.Errorf("main person = %v (err=%v), want deleted by the merge", p, err)
	}
	e.assertNoMainMedia(t, mediaID)
}

// A branch upload to an owner that main still has merges, bytes and all.
func TestMergeBranch_MediaOfLiveOwnerMerges(t *testing.T) {
	e := newEvidenceFixture(t)
	ctx := context.Background()

	mediaID := e.upload(t, e.f.handler.WithBranch(e.branch), "source", e.source)
	if _, err := e.f.handler.MergeBranch(ctx, command.MergeBranchInput{BranchID: e.branch.ID}); err != nil {
		t.Fatalf("MergeBranch failed: %v", err)
	}
	got, err := e.f.readStore.GetMediaWithData(ctx, domain.MainBranchID, mediaID)
	if err != nil || got == nil || len(got.FileData) == 0 {
		t.Fatalf("main GetMediaWithData = %+v (err=%v), want the merged upload with its bytes", got, err)
	}
}
