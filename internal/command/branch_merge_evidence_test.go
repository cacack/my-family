package command_test

import (
	"context"
	"errors"
	"testing"

	"github.com/google/uuid"

	"github.com/cacack/my-family/internal/command"
	"github.com/cacack/my-family/internal/domain"
)

// evidenceFixture is a main person and source, plus a branch forked after both.
type evidenceFixture struct {
	f      *branchFixture
	branch *domain.Branch
	person uuid.UUID
	source uuid.UUID
}

func newEvidenceFixture(t *testing.T) evidenceFixture {
	t.Helper()
	f := newBranchFixture()
	ctx := context.Background()
	person, err := f.handler.CreatePerson(ctx, command.CreatePersonInput{GivenName: "Ada", Surname: "Lovelace"})
	if err != nil {
		t.Fatalf("CreatePerson failed: %v", err)
	}
	source, err := f.handler.CreateSource(ctx, command.CreateSourceInput{SourceType: "census", Title: "1880 Census"})
	if err != nil {
		t.Fatalf("CreateSource failed: %v", err)
	}
	branch, err := f.handler.CreateBranch(ctx, "evidence-theory", "")
	if err != nil {
		t.Fatalf("CreateBranch failed: %v", err)
	}
	return evidenceFixture{f: f, branch: branch, person: person.ID, source: source.ID}
}

func (e evidenceFixture) cite(t *testing.T, h *command.Handler, sourceID uuid.UUID) *command.CreateCitationResult {
	t.Helper()
	cit, err := h.CreateCitation(context.Background(), command.CreateCitationInput{
		SourceID: sourceID, FactType: string(domain.FactPersonBirth), FactOwnerID: e.person, Page: "12",
	})
	if err != nil {
		t.Fatalf("CreateCitation failed: %v", err)
	}
	return cit
}

func (e evidenceFixture) deleteSource(t *testing.T, h *command.Handler, branchID domain.BranchID, sourceID uuid.UUID) {
	t.Helper()
	ctx := context.Background()
	src, err := e.f.readStore.GetSource(ctx, branchID, sourceID)
	if err != nil || src == nil {
		t.Fatalf("GetSource(%s) = %v, %v", sourceID, src, err)
	}
	if err := h.DeleteSource(ctx, sourceID, src.Version, "theory"); err != nil {
		t.Fatalf("DeleteSource failed: %v", err)
	}
}

func (e evidenceFixture) assertRefusedBeforeClaim(t *testing.T, err error) {
	t.Helper()
	if !errors.Is(err, command.ErrMergeDanglingReference) {
		t.Fatalf("MergeBranch error = %v, want ErrMergeDanglingReference", err)
	}
	after, err := e.f.branchStore.Get(context.Background(), e.branch.ID)
	if err != nil {
		t.Fatalf("Get branch failed: %v", err)
	}
	if after.Status != domain.BranchStatusActive {
		t.Errorf("branch status = %s, want active (refused before the claim)", after.Status)
	}
}

// A branch deletes a source it sees no citations of; main then cites that
// source. Neither stream conflicts, but replaying the delete would cascade onto
// main's citation and delete it with no CitationDeleted event.
func TestMergeBranch_SourceDeleteOrphaningMainCitationIsRefused(t *testing.T) {
	e := newEvidenceFixture(t)
	ctx := context.Background()
	branchID := domain.BranchID(e.branch.ID)

	e.deleteSource(t, e.f.handler.WithBranch(e.branch), branchID, e.source)
	mainCit := e.cite(t, e.f.handler, e.source)

	_, err := e.f.handler.MergeBranch(ctx, command.MergeBranchInput{BranchID: e.branch.ID})
	e.assertRefusedBeforeClaim(t, err)

	cit, err := e.f.readStore.GetCitation(ctx, domain.MainBranchID, mainCit.ID)
	if err != nil {
		t.Fatalf("GetCitation failed: %v", err)
	}
	if cit == nil {
		t.Fatal("main lost its citation on a refused merge")
	}
	src, err := e.f.readStore.GetSource(ctx, domain.MainBranchID, e.source)
	if err != nil || src == nil {
		t.Fatalf("main source = %v, %v; want it untouched", src, err)
	}
}

// A branch cites a source that main deletes after the fork. Replaying the
// citation would leave main with a citation of a source it does not have.
func TestMergeBranch_CitationOfMainDeletedSourceIsRefused(t *testing.T) {
	e := newEvidenceFixture(t)
	ctx := context.Background()

	branchCit := e.cite(t, e.f.handler.WithBranch(e.branch), e.source)
	e.deleteSource(t, e.f.handler, domain.MainBranchID, e.source)

	_, err := e.f.handler.MergeBranch(ctx, command.MergeBranchInput{BranchID: e.branch.ID})
	e.assertRefusedBeforeClaim(t, err)

	cit, err := e.f.readStore.GetCitation(ctx, domain.MainBranchID, branchCit.ID)
	if err != nil {
		t.Fatalf("GetCitation failed: %v", err)
	}
	if cit != nil {
		t.Errorf("main gained orphaned citation %+v on a refused merge", cit)
	}
}

// Re-pointing a branch citation at a source main deleted is caught too: the
// check follows source_id through CitationUpdated, not only CitationCreated.
func TestMergeBranch_CitationRepointedAtMainDeletedSourceIsRefused(t *testing.T) {
	e := newEvidenceFixture(t)
	ctx := context.Background()

	other, err := e.f.handler.CreateSource(ctx, command.CreateSourceInput{SourceType: "book", Title: "Parish Register"})
	if err != nil {
		t.Fatalf("CreateSource failed: %v", err)
	}
	// Fork a fresh branch that sees both sources.
	branch, err := e.f.handler.CreateBranch(ctx, "repoint-theory", "")
	if err != nil {
		t.Fatalf("CreateBranch failed: %v", err)
	}
	e.branch = branch
	scoped := e.f.handler.WithBranch(branch)

	cit := e.cite(t, scoped, e.source)
	if _, err := scoped.UpdateCitation(ctx, command.UpdateCitationInput{
		ID: cit.ID, SourceID: &other.ID, Version: cit.Version,
	}); err != nil {
		t.Fatalf("branch UpdateCitation failed: %v", err)
	}
	e.deleteSource(t, e.f.handler, domain.MainBranchID, other.ID)

	_, err = e.f.handler.MergeBranch(ctx, command.MergeBranchInput{BranchID: branch.ID})
	e.assertRefusedBeforeClaim(t, err)
}

// Only the citation's FINAL source matters: a citation created on a source
// main has since deleted, then re-pointed at a live one, lands somewhere real.
func TestMergeBranch_CitationFinallyCitingLiveSourceMerges(t *testing.T) {
	e := newEvidenceFixture(t)
	ctx := context.Background()

	live, err := e.f.handler.CreateSource(ctx, command.CreateSourceInput{SourceType: "book", Title: "Parish Register"})
	if err != nil {
		t.Fatalf("CreateSource failed: %v", err)
	}
	branch, err := e.f.handler.CreateBranch(ctx, "final-source", "")
	if err != nil {
		t.Fatalf("CreateBranch failed: %v", err)
	}
	scoped := e.f.handler.WithBranch(branch)

	cit := e.cite(t, scoped, e.source)
	if _, err := scoped.UpdateCitation(ctx, command.UpdateCitationInput{
		ID: cit.ID, SourceID: &live.ID, Version: cit.Version,
	}); err != nil {
		t.Fatalf("branch UpdateCitation failed: %v", err)
	}
	e.deleteSource(t, e.f.handler, domain.MainBranchID, e.source)

	if _, err := e.f.handler.MergeBranch(ctx, command.MergeBranchInput{BranchID: branch.ID}); err != nil {
		t.Fatalf("MergeBranch failed: %v", err)
	}
	got, err := e.f.readStore.GetCitation(ctx, domain.MainBranchID, cit.ID)
	if err != nil || got == nil {
		t.Fatalf("main GetCitation = %v, %v", got, err)
	}
	if got.SourceID != live.ID || got.SourceTitle != "Parish Register" {
		t.Errorf("main citation source = %s %q, want %s %q", got.SourceID, got.SourceTitle, live.ID, "Parish Register")
	}
}

// A branch that moves main's citation off a source and then deletes the source
// takes the citation with it deliberately: the replay re-points it first.
func TestMergeBranch_SourceDeleteAfterRepointingMainCitationMerges(t *testing.T) {
	f := newBranchFixture()
	ctx := context.Background()
	person, err := f.handler.CreatePerson(ctx, command.CreatePersonInput{GivenName: "Ada", Surname: "Lovelace"})
	if err != nil {
		t.Fatalf("CreatePerson failed: %v", err)
	}
	doomed, err := f.handler.CreateSource(ctx, command.CreateSourceInput{SourceType: "census", Title: "1880 Census"})
	if err != nil {
		t.Fatalf("CreateSource failed: %v", err)
	}
	keeper, err := f.handler.CreateSource(ctx, command.CreateSourceInput{SourceType: "book", Title: "Parish Register"})
	if err != nil {
		t.Fatalf("CreateSource failed: %v", err)
	}
	cit, err := f.handler.CreateCitation(ctx, command.CreateCitationInput{
		SourceID: doomed.ID, FactType: string(domain.FactPersonBirth), FactOwnerID: person.ID,
	})
	if err != nil {
		t.Fatalf("CreateCitation failed: %v", err)
	}
	branch, err := f.handler.CreateBranch(ctx, "consolidate", "")
	if err != nil {
		t.Fatalf("CreateBranch failed: %v", err)
	}
	e := evidenceFixture{f: f, branch: branch, person: person.ID, source: doomed.ID}
	scoped := f.handler.WithBranch(branch)

	if _, err := scoped.UpdateCitation(ctx, command.UpdateCitationInput{
		ID: cit.ID, SourceID: &keeper.ID, Version: cit.Version,
	}); err != nil {
		t.Fatalf("branch UpdateCitation failed: %v", err)
	}
	e.deleteSource(t, scoped, domain.BranchID(branch.ID), doomed.ID)

	if _, err := f.handler.MergeBranch(ctx, command.MergeBranchInput{BranchID: branch.ID}); err != nil {
		t.Fatalf("MergeBranch failed: %v", err)
	}
	got, err := f.readStore.GetCitation(ctx, domain.MainBranchID, cit.ID)
	if err != nil || got == nil {
		t.Fatalf("main GetCitation = %v, %v; want the re-pointed citation", got, err)
	}
	if got.SourceID != keeper.ID {
		t.Errorf("main citation source = %s, want %s", got.SourceID, keeper.ID)
	}
	src, err := f.readStore.GetSource(ctx, domain.MainBranchID, doomed.ID)
	if err != nil {
		t.Fatalf("GetSource failed: %v", err)
	}
	if src != nil {
		t.Errorf("main still has deleted source %s", doomed.ID)
	}
}

// A branch citing a source the branch itself created merges: the replay
// creates the source too.
func TestMergeBranch_CitationOfBranchCreatedSourceMerges(t *testing.T) {
	e := newEvidenceFixture(t)
	ctx := context.Background()
	scoped := e.f.handler.WithBranch(e.branch)

	src, err := scoped.CreateSource(ctx, command.CreateSourceInput{SourceType: "book", Title: "Family letter"})
	if err != nil {
		t.Fatalf("branch CreateSource failed: %v", err)
	}
	cit := e.cite(t, scoped, src.ID)

	if _, err := e.f.handler.MergeBranch(ctx, command.MergeBranchInput{BranchID: e.branch.ID}); err != nil {
		t.Fatalf("MergeBranch failed: %v", err)
	}
	got, err := e.f.readStore.GetCitation(ctx, domain.MainBranchID, cit.ID)
	if err != nil || got == nil || got.SourceTitle != "Family letter" {
		t.Fatalf("main GetCitation = %+v, %v; want it citing %q", got, err, "Family letter")
	}
}

// A branch that deletes main's citation and then its source removes both on
// merge: the citation delete is itself replayed, so nothing is lost silently.
func TestMergeBranch_SourceDeleteAfterDeletingMainCitationMerges(t *testing.T) {
	f := newBranchFixture()
	ctx := context.Background()
	person, err := f.handler.CreatePerson(ctx, command.CreatePersonInput{GivenName: "Ada", Surname: "Lovelace"})
	if err != nil {
		t.Fatalf("CreatePerson failed: %v", err)
	}
	src, err := f.handler.CreateSource(ctx, command.CreateSourceInput{SourceType: "census", Title: "1880 Census"})
	if err != nil {
		t.Fatalf("CreateSource failed: %v", err)
	}
	cit, err := f.handler.CreateCitation(ctx, command.CreateCitationInput{
		SourceID: src.ID, FactType: string(domain.FactPersonBirth), FactOwnerID: person.ID,
	})
	if err != nil {
		t.Fatalf("CreateCitation failed: %v", err)
	}
	branch, err := f.handler.CreateBranch(ctx, "retract", "")
	if err != nil {
		t.Fatalf("CreateBranch failed: %v", err)
	}
	e := evidenceFixture{f: f, branch: branch, person: person.ID, source: src.ID}
	scoped := f.handler.WithBranch(branch)

	if err := scoped.DeleteCitation(ctx, cit.ID, cit.Version, "retracted"); err != nil {
		t.Fatalf("branch DeleteCitation failed: %v", err)
	}
	e.deleteSource(t, scoped, domain.BranchID(branch.ID), src.ID)

	if _, err := f.handler.MergeBranch(ctx, command.MergeBranchInput{BranchID: branch.ID}); err != nil {
		t.Fatalf("MergeBranch failed: %v", err)
	}
	if got, err := f.readStore.GetCitation(ctx, domain.MainBranchID, cit.ID); err != nil || got != nil {
		t.Errorf("main GetCitation = %+v, %v; want deleted", got, err)
	}
	if got, err := f.readStore.GetSource(ctx, domain.MainBranchID, src.ID); err != nil || got != nil {
		t.Errorf("main GetSource = %+v, %v; want deleted", got, err)
	}
}

// A branch cites main's source, then creates a second source and re-points the
// citation at it. The citation stream is touched first, so a first-touch replay
// would move the citation before the new source exists on main: the citation
// would keep the old title and the new source's count would never be bumped.
func TestMergeBranch_CitationRepointedAtLaterBranchSourceKeepsTitleAndCounts(t *testing.T) {
	e := newEvidenceFixture(t)
	ctx := context.Background()
	scoped := e.f.handler.WithBranch(e.branch)

	cit := e.cite(t, scoped, e.source)
	later, err := scoped.CreateSource(ctx, command.CreateSourceInput{SourceType: "census", Title: "1900 Census"})
	if err != nil {
		t.Fatalf("branch CreateSource failed: %v", err)
	}
	if _, err := scoped.UpdateCitation(ctx, command.UpdateCitationInput{
		ID: cit.ID, SourceID: &later.ID, Version: cit.Version,
	}); err != nil {
		t.Fatalf("branch UpdateCitation failed: %v", err)
	}

	if _, err := e.f.handler.MergeBranch(ctx, command.MergeBranchInput{BranchID: e.branch.ID}); err != nil {
		t.Fatalf("MergeBranch failed: %v", err)
	}
	got, err := e.f.readStore.GetCitation(ctx, domain.MainBranchID, cit.ID)
	if err != nil || got == nil {
		t.Fatalf("main GetCitation = %v, %v", got, err)
	}
	if got.SourceID != later.ID || got.SourceTitle != "1900 Census" {
		t.Errorf("main citation = source %s %q, want %s %q", got.SourceID, got.SourceTitle, later.ID, "1900 Census")
	}
	for id, want := range map[uuid.UUID]int{e.source: 0, later.ID: 1} {
		src, err := e.f.readStore.GetSource(ctx, domain.MainBranchID, id)
		if err != nil || src == nil {
			t.Fatalf("main GetSource(%s) = %v, %v", id, src, err)
		}
		if src.CitationCount != want {
			t.Errorf("main source %q citation_count = %d, want %d", src.Title, src.CitationCount, want)
		}
	}
}

// A branch edits a source first, then re-points main's only citation of it
// elsewhere and deletes it. The source stream is touched first, so a
// first-touch replay would delete the source — cascading onto main's citation —
// before the re-point lands, losing the citation.
func TestMergeBranch_SourceTouchedBeforeRepointIsDeletedAfterIt(t *testing.T) {
	f := newBranchFixture()
	ctx := context.Background()
	person, err := f.handler.CreatePerson(ctx, command.CreatePersonInput{GivenName: "Ada", Surname: "Lovelace"})
	if err != nil {
		t.Fatalf("CreatePerson failed: %v", err)
	}
	doomed, err := f.handler.CreateSource(ctx, command.CreateSourceInput{SourceType: "census", Title: "1880 Census"})
	if err != nil {
		t.Fatalf("CreateSource failed: %v", err)
	}
	keeper, err := f.handler.CreateSource(ctx, command.CreateSourceInput{SourceType: "book", Title: "Parish Register"})
	if err != nil {
		t.Fatalf("CreateSource failed: %v", err)
	}
	cit, err := f.handler.CreateCitation(ctx, command.CreateCitationInput{
		SourceID: doomed.ID, FactType: string(domain.FactPersonBirth), FactOwnerID: person.ID,
	})
	if err != nil {
		t.Fatalf("CreateCitation failed: %v", err)
	}
	branch, err := f.handler.CreateBranch(ctx, "consolidate-late", "")
	if err != nil {
		t.Fatalf("CreateBranch failed: %v", err)
	}
	e := evidenceFixture{f: f, branch: branch, person: person.ID, source: doomed.ID}
	scoped := f.handler.WithBranch(branch)

	retitle := "1880 Census (dup)"
	if _, err := scoped.UpdateSource(ctx, command.UpdateSourceInput{
		ID: doomed.ID, Title: &retitle, Version: doomed.Version,
	}); err != nil {
		t.Fatalf("branch UpdateSource failed: %v", err)
	}
	if _, err := scoped.UpdateCitation(ctx, command.UpdateCitationInput{
		ID: cit.ID, SourceID: &keeper.ID, Version: cit.Version,
	}); err != nil {
		t.Fatalf("branch UpdateCitation failed: %v", err)
	}
	e.deleteSource(t, scoped, domain.BranchID(branch.ID), doomed.ID)

	if _, err := f.handler.MergeBranch(ctx, command.MergeBranchInput{BranchID: branch.ID}); err != nil {
		t.Fatalf("MergeBranch failed: %v", err)
	}
	got, err := f.readStore.GetCitation(ctx, domain.MainBranchID, cit.ID)
	if err != nil || got == nil {
		t.Fatalf("main GetCitation = %v, %v; want the re-pointed citation to survive", got, err)
	}
	if got.SourceID != keeper.ID || got.SourceTitle != "Parish Register" {
		t.Errorf("main citation = source %s %q, want %s %q", got.SourceID, got.SourceTitle, keeper.ID, "Parish Register")
	}
	kept, err := f.readStore.GetSource(ctx, domain.MainBranchID, keeper.ID)
	if err != nil || kept == nil || kept.CitationCount != 1 {
		t.Fatalf("main keeper source = %+v, %v; want citation_count 1", kept, err)
	}
	gone, err := f.readStore.GetSource(ctx, domain.MainBranchID, doomed.ID)
	if err != nil || gone != nil {
		t.Errorf("main GetSource(doomed) = %+v, %v; want deleted", gone, err)
	}
}
