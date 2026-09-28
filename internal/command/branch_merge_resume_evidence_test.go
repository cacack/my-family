package command_test

import (
	"context"
	"errors"
	"slices"
	"testing"

	"github.com/google/uuid"

	"github.com/cacack/my-family/internal/command"
	"github.com/cacack/my-family/internal/domain"
	"github.com/cacack/my-family/internal/repository"
)

// evidenceResume is a fixture whose event and read-model stores can fail on
// demand, with one main person to hang citations on.
type evidenceResume struct {
	f      *branchFixture
	faulty *faultyReplayStore
	reads  *faultyReadStore
	person uuid.UUID
}

func newEvidenceResume(t *testing.T) evidenceResume {
	t.Helper()
	var e evidenceResume
	e.f = newBranchFixtureWith(branchFixtureDeps{
		wrapEvents: func(inner repository.EventStore) repository.EventStore {
			e.faulty = &faultyReplayStore{EventStore: inner}
			return e.faulty
		},
		wrapReads: func(inner repository.ReadModelStore) repository.ReadModelStore {
			e.reads = &faultyReadStore{ReadModelStore: inner}
			return e.reads
		},
	})
	person, err := e.f.handler.CreatePerson(context.Background(), command.CreatePersonInput{GivenName: "Ada", Surname: "Lovelace"})
	if err != nil {
		t.Fatalf("CreatePerson failed: %v", err)
	}
	e.person = person.ID
	return e
}

func (e evidenceResume) source(t *testing.T, h *command.Handler, title string) *command.CreateSourceResult {
	t.Helper()
	src, err := h.CreateSource(context.Background(), command.CreateSourceInput{SourceType: "census", Title: title})
	if err != nil {
		t.Fatalf("CreateSource(%q) failed: %v", title, err)
	}
	return src
}

func (e evidenceResume) cite(t *testing.T, h *command.Handler, sourceID uuid.UUID) *command.CreateCitationResult {
	t.Helper()
	cit, err := h.CreateCitation(context.Background(), command.CreateCitationInput{
		SourceID: sourceID, FactType: string(domain.FactPersonBirth), FactOwnerID: e.person, Page: "12",
	})
	if err != nil {
		t.Fatalf("CreateCitation failed: %v", err)
	}
	return cit
}

func (e evidenceResume) branch(t *testing.T, name string) (*domain.Branch, *command.Handler) {
	t.Helper()
	branch, err := e.f.handler.CreateBranch(context.Background(), name, "")
	if err != nil {
		t.Fatalf("CreateBranch failed: %v", err)
	}
	return branch, e.f.handler.WithBranch(branch)
}

// interrupt runs a merge whose replay fails on its failAt-th stream.
func (e evidenceResume) interrupt(t *testing.T, branch *domain.Branch, failAt int) {
	t.Helper()
	e.faulty.armed, e.faulty.failAt, e.faulty.mainAppends = true, failAt, 0
	_, err := e.f.handler.MergeBranch(context.Background(), command.MergeBranchInput{BranchID: branch.ID})
	e.faulty.armed = false
	if !errors.Is(err, command.ErrMergePartiallyApplied) {
		t.Fatalf("MergeBranch error = %v, want ErrMergePartiallyApplied", err)
	}
}

func (e evidenceResume) mainSource(t *testing.T, id uuid.UUID) *repository.SourceReadModel {
	t.Helper()
	src, err := e.f.readStore.GetSource(context.Background(), domain.MainBranchID, id)
	if err != nil {
		t.Fatalf("GetSource(%s) failed: %v", id, err)
	}
	return src
}

func (e evidenceResume) mainCitation(t *testing.T, id uuid.UUID) *repository.CitationReadModel {
	t.Helper()
	cit, err := e.f.readStore.GetCitation(context.Background(), domain.MainBranchID, id)
	if err != nil {
		t.Fatalf("GetCitation(%s) failed: %v", id, err)
	}
	return cit
}

func (e evidenceResume) mainEventCount(t *testing.T) int {
	t.Helper()
	events, err := e.f.eventStore.ReadBranch(context.Background(), domain.MainBranchID, 0, 100000)
	if err != nil {
		t.Fatalf("ReadBranch failed: %v", err)
	}
	return len(events)
}

func (e evidenceResume) deleteMainSource(t *testing.T, id uuid.UUID) {
	t.Helper()
	src := e.mainSource(t, id)
	if src == nil {
		t.Fatalf("main source %s missing", id)
	}
	if err := e.f.handler.DeleteSource(context.Background(), id, src.Version, "withdrawn"); err != nil {
		t.Fatalf("main DeleteSource failed: %v", err)
	}
}

// TestResumeMerge_EvidenceReplaysInEvidenceOrder interrupts a merge carrying
// sources and citations — a re-pointed citation and a source deletion
// included — after its first stream, then resumes it. The branch first
// touched the doomed source, so a resume replaying in first-touch order would
// delete it (cascading onto main's citation) before the re-point lands; the
// resume must continue the merge's evidence order instead.
func TestResumeMerge_EvidenceReplaysInEvidenceOrder(t *testing.T) {
	e := newEvidenceResume(t)
	ctx := context.Background()
	doomed := e.source(t, e.f.handler, "1880 Census")
	mainCit := e.cite(t, e.f.handler, doomed.ID)
	branch, scoped := e.branch(t, "consolidate")

	retitle := "1880 Census (duplicate)"
	if _, err := scoped.UpdateSource(ctx, command.UpdateSourceInput{ID: doomed.ID, Title: &retitle, Version: doomed.Version}); err != nil {
		t.Fatalf("branch UpdateSource failed: %v", err)
	}
	later := e.source(t, scoped, "1900 Census")
	if _, err := scoped.UpdateCitation(ctx, command.UpdateCitationInput{ID: mainCit.ID, SourceID: &later.ID, Version: mainCit.Version}); err != nil {
		t.Fatalf("branch UpdateCitation failed: %v", err)
	}
	newCit := e.cite(t, scoped, later.ID)
	src, err := e.f.readStore.GetSource(ctx, domain.BranchID(branch.ID), doomed.ID)
	if err != nil || src == nil {
		t.Fatalf("branch GetSource = %v, %v", src, err)
	}
	if err := scoped.DeleteSource(ctx, doomed.ID, src.Version, "duplicate"); err != nil {
		t.Fatalf("branch DeleteSource failed: %v", err)
	}

	// Evidence order is later, mainCit, newCit, doomed: only later lands.
	e.interrupt(t, branch, 2)
	if e.mainSource(t, later.ID) == nil || e.mainCitation(t, mainCit.ID).SourceID != doomed.ID {
		t.Fatal("interrupted merge should have landed the new source and nothing else")
	}

	result, err := e.f.handler.ResumeMerge(ctx, command.ResumeMergeInput{BranchID: branch.ID})
	if err != nil {
		t.Fatalf("ResumeMerge failed: %v", err)
	}
	if !slices.Equal(result.AlreadyReplayedStreamIDs, []uuid.UUID{later.ID}) {
		t.Errorf("AlreadyReplayedStreamIDs = %v, want [%s]", result.AlreadyReplayedStreamIDs, later.ID)
	}
	if result.ReplayedEventCount != 4 {
		t.Errorf("ReplayedEventCount = %d, want 4 (re-point, new citation, retitle, delete)", result.ReplayedEventCount)
	}

	moved := e.mainCitation(t, mainCit.ID)
	if moved == nil || moved.SourceID != later.ID || moved.SourceTitle != "1900 Census" {
		t.Fatalf("main's citation = %+v, want it re-pointed at %s and kept", moved, later.ID)
	}
	if added := e.mainCitation(t, newCit.ID); added == nil || added.SourceTitle != "1900 Census" {
		t.Errorf("branch citation on main = %+v, want it citing %q", added, "1900 Census")
	}
	if got := e.mainSource(t, later.ID); got == nil || got.CitationCount != 2 {
		t.Errorf("main source %q = %+v, want citation_count 2", "1900 Census", got)
	}
	if got := e.mainSource(t, doomed.ID); got != nil {
		t.Errorf("main still has deleted source %+v", got)
	}

	before := e.mainEventCount(t)
	again, err := e.f.handler.ResumeMerge(ctx, command.ResumeMergeInput{BranchID: branch.ID})
	if err != nil {
		t.Fatalf("second ResumeMerge failed: %v", err)
	}
	if again.ReplayedEventCount != 0 || len(again.ReprojectedStreamIDs) != 0 || e.mainEventCount(t) != before {
		t.Errorf("second resume = %+v, want a no-op", again)
	}
}

// TestResumeMerge_MainDeletesCitedSourceAfterInterruption: the branch cites
// main's source; the replay stops before the citation lands, and main then
// deletes the source (allowed: main has no citation of it). Replaying the
// citation would orphan it, so it is pending, "branch" is refused as a
// dangling reference, and "main" rolls the merge forward without it.
func TestResumeMerge_MainDeletesCitedSourceAfterInterruption(t *testing.T) {
	e := newEvidenceResume(t)
	ctx := context.Background()
	cited := e.source(t, e.f.handler, "1880 Census")
	branch, scoped := e.branch(t, "cite-then-lose")
	surname := "Byron"
	if _, err := scoped.UpdatePerson(ctx, command.UpdatePersonInput{ID: e.person, Surname: &surname, Version: 1}); err != nil {
		t.Fatalf("branch UpdatePerson failed: %v", err)
	}
	cit := e.cite(t, scoped, cited.ID)

	e.interrupt(t, branch, 2)
	e.deleteMainSource(t, cited.ID)

	before := e.mainEventCount(t)
	result, err := e.f.handler.ResumeMerge(ctx, command.ResumeMergeInput{BranchID: branch.ID})
	if !errors.Is(err, command.ErrMergeResumeNeedsResolution) {
		t.Fatalf("ResumeMerge error = %v, want ErrMergeResumeNeedsResolution", err)
	}
	if !slices.Equal(result.PendingStreamIDs, []uuid.UUID{cit.ID}) {
		t.Fatalf("PendingStreamIDs = %v, want [%s]", result.PendingStreamIDs, cit.ID)
	}

	_, err = e.f.handler.ResumeMerge(ctx, command.ResumeMergeInput{
		BranchID:    branch.ID,
		Resolutions: map[uuid.UUID]command.MergeResolution{cit.ID: command.ResolveBranch},
	})
	if !errors.Is(err, command.ErrMergeDanglingReference) {
		t.Fatalf("ResumeMerge(branch) error = %v, want ErrMergeDanglingReference", err)
	}
	if got := e.mainEventCount(t); got != before {
		t.Fatalf("refused resumes wrote %d main events", got-before)
	}

	done, err := e.f.handler.ResumeMerge(ctx, command.ResumeMergeInput{
		BranchID:    branch.ID,
		Resolutions: map[uuid.UUID]command.MergeResolution{cit.ID: command.ResolveMain},
	})
	if err != nil {
		t.Fatalf("ResumeMerge(main) failed: %v", err)
	}
	if !slices.Equal(done.SkippedStreamIDs, []uuid.UUID{cit.ID}) || done.ReplayedEventCount != 0 {
		t.Errorf("resume = skipped %v replayed %d, want [%s] and 0", done.SkippedStreamIDs, done.ReplayedEventCount, cit.ID)
	}
	if got := e.mainCitation(t, cit.ID); got != nil {
		t.Errorf("main gained orphaned citation %+v", got)
	}
}

// TestResumeMerge_SourceDeleteOntoLaterMainCitationIsPending: the branch
// deletes a source nothing cites; the replay stops before the delete lands,
// and main then cites the source. Replaying the delete would cascade onto
// main's new citation with no record, so the delete is pending, and "main"
// keeps both.
func TestResumeMerge_SourceDeleteOntoLaterMainCitationIsPending(t *testing.T) {
	e := newEvidenceResume(t)
	ctx := context.Background()
	unused := e.source(t, e.f.handler, "Unused register")
	branch, scoped := e.branch(t, "tidy")
	surname := "Byron"
	if _, err := scoped.UpdatePerson(ctx, command.UpdatePersonInput{ID: e.person, Surname: &surname, Version: 1}); err != nil {
		t.Fatalf("branch UpdatePerson failed: %v", err)
	}
	if err := scoped.DeleteSource(ctx, unused.ID, unused.Version, "unused"); err != nil {
		t.Fatalf("branch DeleteSource failed: %v", err)
	}

	e.interrupt(t, branch, 2)
	mainCit := e.cite(t, e.f.handler, unused.ID)

	result, err := e.f.handler.ResumeMerge(ctx, command.ResumeMergeInput{BranchID: branch.ID})
	if !errors.Is(err, command.ErrMergeResumeNeedsResolution) {
		t.Fatalf("ResumeMerge error = %v, want ErrMergeResumeNeedsResolution", err)
	}
	if !slices.Equal(result.PendingStreamIDs, []uuid.UUID{unused.ID}) {
		t.Fatalf("PendingStreamIDs = %v, want [%s]", result.PendingStreamIDs, unused.ID)
	}
	if _, err := e.f.handler.ResumeMerge(ctx, command.ResumeMergeInput{
		BranchID:    branch.ID,
		Resolutions: map[uuid.UUID]command.MergeResolution{unused.ID: command.ResolveBranch},
	}); !errors.Is(err, command.ErrMergeDanglingReference) {
		t.Fatalf("ResumeMerge(branch) error = %v, want ErrMergeDanglingReference", err)
	}
	if _, err := e.f.handler.ResumeMerge(ctx, command.ResumeMergeInput{
		BranchID:    branch.ID,
		Resolutions: map[uuid.UUID]command.MergeResolution{unused.ID: command.ResolveMain},
	}); err != nil {
		t.Fatalf("ResumeMerge(main) failed: %v", err)
	}
	if e.mainCitation(t, mainCit.ID) == nil || e.mainSource(t, unused.ID) == nil {
		t.Error("main lost its citation or its source to a delete the resume rolled forward without")
	}
}

// TestResumeMerge_RepairsEvidenceProjections: a citation whose projection
// saved the citation but failed to bump its source's count (level by version,
// so only a recount sees it), then — during the resume — a note whose
// projection failed after its append. Each resume repairs what the previous
// attempt left behind without re-appending anything.
func TestResumeMerge_RepairsEvidenceProjections(t *testing.T) {
	e := newEvidenceResume(t)
	ctx := context.Background()
	cited := e.source(t, e.f.handler, "1880 Census")
	branch, scoped := e.branch(t, "annotate")
	cit := e.cite(t, scoped, cited.ID)
	note, err := scoped.CreateNote(ctx, command.CreateNoteInput{Text: "Enumerator misspelled the surname"})
	if err != nil {
		t.Fatalf("branch CreateNote failed: %v", err)
	}

	e.reads.armed, e.reads.failSourceCount = true, cited.ID
	_, err = e.f.handler.MergeBranch(ctx, command.MergeBranchInput{BranchID: branch.ID})
	e.reads.failSourceCount = uuid.Nil
	if !errors.Is(err, command.ErrMergePartiallyApplied) {
		t.Fatalf("MergeBranch error = %v, want ErrMergePartiallyApplied", err)
	}
	if got := e.mainSource(t, cited.ID); got.CitationCount != 0 || e.mainCitation(t, cit.ID) == nil {
		t.Fatalf("after the failed projection source = %+v, want the citation saved and the count not bumped", got)
	}

	e.reads.failNote = note.ID
	_, err = e.f.handler.ResumeMerge(ctx, command.ResumeMergeInput{BranchID: branch.ID})
	e.reads.armed, e.reads.failNote = false, uuid.Nil
	if !errors.Is(err, command.ErrMergePartiallyApplied) {
		t.Fatalf("first ResumeMerge error = %v, want ErrMergePartiallyApplied from the note's projection", err)
	}

	before := e.mainEventCount(t)
	result, err := e.f.handler.ResumeMerge(ctx, command.ResumeMergeInput{BranchID: branch.ID})
	if err != nil {
		t.Fatalf("second ResumeMerge failed: %v", err)
	}
	if result.ReplayedEventCount != 0 || e.mainEventCount(t) != before {
		t.Errorf("second resume replayed %d events, want 0 (everything is in the log)", result.ReplayedEventCount)
	}
	if !slices.Equal(result.ReprojectedStreamIDs, []uuid.UUID{note.ID, cit.ID}) {
		t.Errorf("ReprojectedStreamIDs = %v, want [%s %s] (the note re-projected, the citation's count recounted)",
			result.ReprojectedStreamIDs, note.ID, cit.ID)
	}
	if got := e.mainSource(t, cited.ID); got.CitationCount != 1 {
		t.Errorf("source citation_count = %d, want 1 (recounted)", got.CitationCount)
	}
	got, err := e.f.readStore.GetNote(ctx, domain.MainBranchID, note.ID)
	if err != nil || got == nil || got.Text != "Enumerator misspelled the surname" || got.Version != 1 {
		t.Errorf("main note = %+v, %v; want it repaired from the log", got, err)
	}
}

// TestResumeMerge_RecountReportsTheCitation: the first resume after a failed
// count bump reports the citation stream as repaired.
func TestResumeMerge_RecountReportsTheCitation(t *testing.T) {
	e := newEvidenceResume(t)
	ctx := context.Background()
	cited := e.source(t, e.f.handler, "1880 Census")
	branch, scoped := e.branch(t, "count")
	cit := e.cite(t, scoped, cited.ID)

	e.reads.armed, e.reads.failSourceCount = true, cited.ID
	if _, err := e.f.handler.MergeBranch(ctx, command.MergeBranchInput{BranchID: branch.ID}); !errors.Is(err, command.ErrMergePartiallyApplied) {
		t.Fatalf("MergeBranch error = %v, want ErrMergePartiallyApplied", err)
	}
	e.reads.armed, e.reads.failSourceCount = false, uuid.Nil

	result, err := e.f.handler.ResumeMerge(ctx, command.ResumeMergeInput{BranchID: branch.ID})
	if err != nil {
		t.Fatalf("ResumeMerge failed: %v", err)
	}
	if !slices.Equal(result.ReprojectedStreamIDs, []uuid.UUID{cit.ID}) {
		t.Errorf("ReprojectedStreamIDs = %v, want [%s]", result.ReprojectedStreamIDs, cit.ID)
	}
	if got := e.mainSource(t, cited.ID); got.CitationCount != 1 {
		t.Errorf("citation_count = %d, want 1", got.CitationCount)
	}
}

// TestResumeMerge_RecountNeverRollsBackARacingSourceEdit: a mainline retitle
// of the source projects between the recount's read and its save, so the save
// writes the old title and version back. The recount must notice the row is
// behind main's log and re-project the retitle, keeping the corrected count.
func TestResumeMerge_RecountNeverRollsBackARacingSourceEdit(t *testing.T) {
	e := newEvidenceResume(t)
	ctx := context.Background()
	cited := e.source(t, e.f.handler, "1880 Census")
	branch, scoped := e.branch(t, "count-race")
	e.cite(t, scoped, cited.ID)

	e.reads.armed, e.reads.failSourceCount = true, cited.ID
	if _, err := e.f.handler.MergeBranch(ctx, command.MergeBranchInput{BranchID: branch.ID}); !errors.Is(err, command.ErrMergePartiallyApplied) {
		t.Fatalf("MergeBranch error = %v, want ErrMergePartiallyApplied", err)
	}
	e.reads.armed, e.reads.failSourceCount = false, uuid.Nil

	e.reads.beforeMainSaveSource = func() {
		title := "1880 Federal Census"
		if _, err := e.f.handler.UpdateSource(ctx, command.UpdateSourceInput{ID: cited.ID, Title: &title, Version: cited.Version}); err != nil {
			t.Errorf("racing main UpdateSource failed: %v", err)
		}
	}
	if _, err := e.f.handler.ResumeMerge(ctx, command.ResumeMergeInput{BranchID: branch.ID}); err != nil {
		t.Fatalf("ResumeMerge failed: %v", err)
	}
	got := e.mainSource(t, cited.ID)
	if got == nil || got.Title != "1880 Federal Census" || got.Version != 2 || got.CitationCount != 1 {
		t.Errorf("main source = %+v, want the racing retitle (version 2) kept and citation_count 1", got)
	}
}

// TestResumeMerge_CitationCascadedAwayIsNotResurrected: a branch citation's
// append lands but its projection fails, so main's read model never shows it;
// main then deletes the source (its guard sees no citation). The citation's
// row is missing because the source's delete cascade would have removed it,
// which main's log explains — re-projecting it would resurrect an orphan.
func TestResumeMerge_CitationCascadedAwayIsNotResurrected(t *testing.T) {
	e := newEvidenceResume(t)
	ctx := context.Background()
	cited := e.source(t, e.f.handler, "1880 Census")
	branch, scoped := e.branch(t, "lost-source")
	cit := e.cite(t, scoped, cited.ID)

	e.reads.armed, e.reads.failCitation = true, cit.ID
	if _, err := e.f.handler.MergeBranch(ctx, command.MergeBranchInput{BranchID: branch.ID}); !errors.Is(err, command.ErrMergePartiallyApplied) {
		t.Fatalf("MergeBranch error = %v, want ErrMergePartiallyApplied", err)
	}
	e.reads.armed, e.reads.failCitation = false, uuid.Nil
	e.deleteMainSource(t, cited.ID)

	result, err := e.f.handler.ResumeMerge(ctx, command.ResumeMergeInput{BranchID: branch.ID})
	if err != nil {
		t.Fatalf("ResumeMerge failed: %v", err)
	}
	if len(result.ReprojectedStreamIDs) != 0 || result.ReplayedEventCount != 0 {
		t.Errorf("resume = reprojected %v replayed %d, want nothing", result.ReprojectedStreamIDs, result.ReplayedEventCount)
	}
	if got := e.mainCitation(t, cit.ID); got != nil {
		t.Errorf("resume resurrected orphaned citation %+v", got)
	}
}

// TestResumeMerge_CascadedCitationsAreDetectedInOneScan: two landed citations
// whose projections failed, each citing a source main then deleted. Deciding
// that both were cascaded away must read the two sources' histories in one
// set-based scan, not one scan per citation.
func TestResumeMerge_CascadedCitationsAreDetectedInOneScan(t *testing.T) {
	e := newEvidenceResume(t)
	ctx := context.Background()
	first := e.source(t, e.f.handler, "1880 Census")
	second := e.source(t, e.f.handler, "1900 Census")
	branch, scoped := e.branch(t, "lost-sources")
	cit1 := e.cite(t, scoped, first.ID)
	cit2 := e.cite(t, scoped, second.ID)

	e.reads.armed, e.reads.failCitation = true, cit1.ID
	if _, err := e.f.handler.MergeBranch(ctx, command.MergeBranchInput{BranchID: branch.ID}); !errors.Is(err, command.ErrMergePartiallyApplied) {
		t.Fatalf("MergeBranch error = %v, want ErrMergePartiallyApplied", err)
	}
	if e.mainCitation(t, cit2.ID) != nil {
		// cit2 landed before cit1 and projected; drop its row as a failed
		// projection would have left it.
		if err := e.f.readStore.DeleteCitation(ctx, domain.MainBranchID, cit2.ID); err != nil {
			t.Fatalf("DeleteCitation failed: %v", err)
		}
	} else {
		e.reads.failCitation = cit2.ID
		if _, err := e.f.handler.ResumeMerge(ctx, command.ResumeMergeInput{BranchID: branch.ID}); !errors.Is(err, command.ErrMergePartiallyApplied) {
			t.Fatalf("first ResumeMerge error = %v, want ErrMergePartiallyApplied", err)
		}
		if err := e.f.readStore.DeleteCitation(ctx, domain.MainBranchID, cit1.ID); err != nil {
			t.Fatalf("DeleteCitation failed: %v", err)
		}
	}
	e.reads.armed, e.reads.failCitation = false, uuid.Nil
	if e.mainCitation(t, cit1.ID) != nil || e.mainCitation(t, cit2.ID) != nil {
		t.Fatal("setup: both citations must be missing from main's read model")
	}
	e.deleteMainSource(t, first.ID)
	e.deleteMainSource(t, second.ID)

	e.faulty.recordScans = true
	result, err := e.f.handler.ResumeMerge(ctx, command.ResumeMergeInput{BranchID: branch.ID})
	e.faulty.recordScans = false
	if err != nil {
		t.Fatalf("ResumeMerge failed: %v", err)
	}
	if len(result.ReprojectedStreamIDs) != 0 || result.ReplayedEventCount != 0 {
		t.Errorf("resume = reprojected %v replayed %d, want nothing", result.ReprojectedStreamIDs, result.ReplayedEventCount)
	}
	if e.mainCitation(t, cit1.ID) != nil || e.mainCitation(t, cit2.ID) != nil {
		t.Error("resume resurrected an orphaned citation")
	}
	sourceScans := 0
	for _, scan := range e.faulty.mainScans {
		hasFirst, hasSecond := slices.Contains(scan, first.ID), slices.Contains(scan, second.ID)
		if hasFirst != hasSecond {
			t.Errorf("main scan %v reads one cascaded citation's source without the other (a scan per citation)", scan)
		}
		if hasFirst {
			sourceScans++
		}
	}
	if sourceScans == 0 {
		t.Error("no main scan read the deleted sources' histories")
	}
}

// TestResumeMerge_LegacyClaimRefusesExcludingALandedCitationsSource: a claim
// with no recorded plan leaves every unreplayed stream to the caller. When a
// citation of a branch-created source is on main and the source is not, a
// "main" resolution for the source would orphan the landed citation, so it is
// refused; "branch" lands the source, and the recount after the replay gives
// it the landed citation.
func TestResumeMerge_LegacyClaimRefusesExcludingALandedCitationsSource(t *testing.T) {
	e := newEvidenceResume(t)
	f := e.f
	ctx := context.Background()
	branch, scoped := e.branch(t, "legacy-evidence")
	created := e.source(t, scoped, "Family letter")
	cit := e.cite(t, scoped, created.ID)

	// A pre-#685 claim (no recorded plan), then the citation's stream
	// replayed by hand — a shape the evidence order never produces, but the
	// log alone cannot rule out.
	claim := domain.BranchMerged{
		BaseEvent:        domain.NewBaseEvent(),
		BranchID:         branch.ID,
		BasePosition:     branch.BasePosition,
		MergedAtPosition: logHead(t, f),
	}
	scope := repository.AppendScope{BranchID: domain.BranchID(branch.ID)}
	version, err := f.eventStore.GetStreamVersion(ctx, branch.ID, scope.BranchID)
	if err != nil {
		t.Fatalf("GetStreamVersion failed: %v", err)
	}
	if err := f.eventStore.Append(ctx, branch.ID, "branch", []domain.Event{claim}, version, scope); err != nil {
		t.Fatalf("appending legacy claim failed: %v", err)
	}
	var citationEvents []domain.Event
	for _, stored := range branchEventsFor(t, f, cit.ID, domain.BranchID(branch.ID)) {
		decoded, err := stored.DecodeEvent()
		if err != nil {
			t.Fatalf("DecodeEvent failed: %v", err)
		}
		citationEvents = append(citationEvents, decoded)
	}
	if err := f.eventStore.Append(ctx, cit.ID, "Citation", citationEvents, 0, repository.MainScope); err != nil {
		t.Fatalf("replaying the citation by hand failed: %v", err)
	}

	result, err := f.handler.ResumeMerge(ctx, command.ResumeMergeInput{BranchID: branch.ID})
	if !errors.Is(err, command.ErrMergeResumeNeedsResolution) {
		t.Fatalf("bare ResumeMerge error = %v, want ErrMergeResumeNeedsResolution", err)
	}
	if !slices.Equal(result.PendingStreamIDs, []uuid.UUID{created.ID}) {
		t.Fatalf("PendingStreamIDs = %v, want [%s]", result.PendingStreamIDs, created.ID)
	}
	_, err = f.handler.ResumeMerge(ctx, command.ResumeMergeInput{
		BranchID:    branch.ID,
		Resolutions: map[uuid.UUID]command.MergeResolution{created.ID: command.ResolveMain},
	})
	if !errors.Is(err, command.ErrMergeDanglingReference) {
		t.Fatalf("ResumeMerge(main) error = %v, want ErrMergeDanglingReference", err)
	}
	if records := branchResumeRecords(t, f, branch); len(records) != 0 {
		t.Errorf("the refused resume recorded %d decision(s), want none", len(records))
	}

	done, err := f.handler.ResumeMerge(ctx, command.ResumeMergeInput{
		BranchID:    branch.ID,
		Resolutions: map[uuid.UUID]command.MergeResolution{created.ID: command.ResolveBranch},
	})
	if err != nil {
		t.Fatalf("ResumeMerge(branch) failed: %v", err)
	}
	if done.ReplayedEventCount != 1 {
		t.Errorf("ReplayedEventCount = %d, want 1 (the source)", done.ReplayedEventCount)
	}
	if e.mainCitation(t, cit.ID) == nil {
		t.Error("the landed citation is missing from main's read model")
	}
	if got := e.mainSource(t, created.ID); got == nil || got.CitationCount != 1 {
		t.Errorf("main source = %+v, want it created with citation_count 1", got)
	}
}
