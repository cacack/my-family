package command_test

import (
	"context"
	"testing"

	"github.com/google/uuid"

	"github.com/cacack/my-family/internal/command"
	"github.com/cacack/my-family/internal/domain"
	"github.com/cacack/my-family/internal/repository"
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
// person's stream is touched first, but the replay moves the upload ahead of
// the person's delete — as it happened on the branch — so the delete cascades
// it on main and the self-cancelling branch merges cleanly with nothing left.
func TestMergeBranch_MediaOfPersonCreatedAndDeletedOnBranchMerges(t *testing.T) {
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

	if _, err := e.f.handler.MergeBranch(ctx, command.MergeBranchInput{BranchID: e.branch.ID}); err != nil {
		t.Fatalf("MergeBranch failed: %v", err)
	}
	if got, err := e.f.readStore.GetPerson(ctx, domain.MainBranchID, p.ID); err != nil || got != nil {
		t.Errorf("main person = %v (err=%v), want absent", got, err)
	}
	e.assertNoMainMedia(t, mediaID)
}

// Same shape with main's person, edited on the branch before the upload so
// its stream is touched first, then deleted on the branch.
func TestMergeBranch_MediaOfMainPersonEditedThenDeletedOnBranchMerges(t *testing.T) {
	e := newEvidenceFixture(t)
	ctx := context.Background()
	scoped := e.f.handler.WithBranch(e.branch)
	branchID := domain.BranchID(e.branch.ID)

	current, err := e.f.readStore.GetPerson(ctx, branchID, e.person)
	if err != nil || current == nil {
		t.Fatalf("GetPerson = %v, %v", current, err)
	}
	surname := "Revised"
	if _, err := scoped.UpdatePerson(ctx, command.UpdatePersonInput{ID: e.person, Surname: &surname, Version: current.Version}); err != nil {
		t.Fatalf("UpdatePerson failed: %v", err)
	}
	mediaID := e.upload(t, scoped, "person", e.person)
	e.deletePerson(t, scoped, branchID, e.person)

	if _, err := e.f.handler.MergeBranch(ctx, command.MergeBranchInput{BranchID: e.branch.ID}); err != nil {
		t.Fatalf("MergeBranch failed: %v", err)
	}
	if got, err := e.f.readStore.GetPerson(ctx, domain.MainBranchID, e.person); err != nil || got != nil {
		t.Errorf("main person = %v (err=%v), want deleted by the merge", got, err)
	}
	e.assertNoMainMedia(t, mediaID)
}

// Same shape with a family the branch creates, photographs and deletes.
func TestMergeBranch_MediaOfFamilyCreatedAndDeletedOnBranchMerges(t *testing.T) {
	e := newEvidenceFixture(t)
	ctx := context.Background()
	scoped := e.f.handler.WithBranch(e.branch)
	branchID := domain.BranchID(e.branch.ID)

	fam, err := scoped.CreateFamily(ctx, command.CreateFamilyInput{Partner1ID: &e.person})
	if err != nil {
		t.Fatalf("CreateFamily failed: %v", err)
	}
	mediaID := e.upload(t, scoped, "family", fam.ID)
	f, err := e.f.readStore.GetFamily(ctx, branchID, fam.ID)
	if err != nil || f == nil {
		t.Fatalf("GetFamily = %v, %v", f, err)
	}
	if err := scoped.DeleteFamily(ctx, command.DeleteFamilyInput{ID: fam.ID, Version: f.Version}); err != nil {
		t.Fatalf("DeleteFamily failed: %v", err)
	}

	if _, err := e.f.handler.MergeBranch(ctx, command.MergeBranchInput{BranchID: e.branch.ID}); err != nil {
		t.Fatalf("MergeBranch failed: %v", err)
	}
	if got, err := e.f.readStore.GetFamily(ctx, domain.MainBranchID, fam.ID); err != nil || got != nil {
		t.Errorf("main family = %v (err=%v), want absent", got, err)
	}
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

// cropMedia sets a crop box (and a new title) on a media item through h.
func cropMedia(t *testing.T, h *command.Handler, current *repository.MediaReadModel, title string, crop int) {
	t.Helper()
	if _, err := h.UpdateMedia(context.Background(), command.UpdateMediaInput{
		ID: current.ID, Title: &title, Version: current.Version,
		CropLeft: &crop, CropTop: &crop, CropWidth: &crop, CropHeight: &crop,
	}); err != nil {
		t.Fatalf("UpdateMedia(%s) failed: %v", current.ID, err)
	}
}

// assertCrop checks every crop field of m equals want.
func assertCrop(t *testing.T, m *repository.MediaReadModel, want int) {
	t.Helper()
	if m == nil {
		t.Fatal("media missing")
	}
	for name, got := range map[string]*int{"CropLeft": m.CropLeft, "CropTop": m.CropTop, "CropWidth": m.CropWidth, "CropHeight": m.CropHeight} {
		if got == nil || *got != want {
			t.Errorf("%s = %v, want %d", name, got, want)
		}
	}
}

// A branch crop edit reaches main through the merge. The merge projects the
// event decoded from its stored JSON, where the crop numbers are float64, so
// this guards against the projection dropping them (#759).
func TestMergeBranch_MediaCropEditMerges(t *testing.T) {
	e := newEvidenceFixture(t)
	ctx := context.Background()
	mediaID := e.upload(t, e.f.handler, "source", e.source)
	branch, err := e.f.handler.CreateBranch(ctx, "crop", "")
	if err != nil {
		t.Fatalf("CreateBranch failed: %v", err)
	}
	scope := domain.BranchID(branch.ID)
	current, err := e.f.readStore.GetMedia(ctx, scope, mediaID)
	if err != nil || current == nil {
		t.Fatalf("branch GetMedia = %v, %v", current, err)
	}
	cropMedia(t, e.f.handler.WithBranch(branch), current, "Cropped", 10)
	onBranch, err := e.f.readStore.GetMedia(ctx, scope, mediaID)
	if err != nil {
		t.Fatalf("branch GetMedia failed: %v", err)
	}
	assertCrop(t, onBranch, 10)

	if _, err := e.f.handler.MergeBranch(ctx, command.MergeBranchInput{BranchID: branch.ID}); err != nil {
		t.Fatalf("MergeBranch failed: %v", err)
	}
	onMain, err := e.f.readStore.GetMedia(ctx, domain.MainBranchID, mediaID)
	if err != nil {
		t.Fatalf("main GetMedia failed: %v", err)
	}
	if onMain == nil || onMain.Title != "Cropped" {
		t.Fatalf("main media = %+v, want the merged title", onMain)
	}
	assertCrop(t, onMain, 10)
}
