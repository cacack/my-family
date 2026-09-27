package command_test

// Regression tests for GPS merge/resume guard gaps (#760): a decided edit_edit
// conflict on an artifact main's subject delete cascaded, a landed re-point
// whose projection failed, and research about a subject id no person or
// family ever had.

import (
	"context"
	"errors"
	"slices"
	"testing"

	"github.com/google/uuid"

	"github.com/cacack/my-family/internal/command"
	"github.com/cacack/my-family/internal/domain"
)

// Both sides change an analysis's conclusion (an edit_edit conflict); main
// then deletes the analysis's subject, whose cascade removes the analysis with
// no event on its stream. The conflict is reported first; resolving it
// "branch" would replay the edit onto nothing, so the merge is refused, and
// "main" goes through.
func TestMergeBranch_GPSEditEditResolvedBranchOntoCascadedArtifactIsRefused(t *testing.T) {
	e := newEvidenceFixture(t)
	ctx := context.Background()

	analysis := e.analyse(t, e.f.handler, e.person, "Born 1815")
	scoped := e.refork(t, "edit-analysis")
	branchConclusion, mainConclusion := "Born 1816", "Born 1817"
	if _, err := scoped.UpdateEvidenceAnalysis(ctx, command.UpdateEvidenceAnalysisInput{
		ID: analysis.ID, Conclusion: &branchConclusion, Version: analysis.Version,
	}); err != nil {
		t.Fatalf("UpdateEvidenceAnalysis on branch failed: %v", err)
	}
	if _, err := e.f.handler.UpdateEvidenceAnalysis(ctx, command.UpdateEvidenceAnalysisInput{
		ID: analysis.ID, Conclusion: &mainConclusion, Version: analysis.Version,
	}); err != nil {
		t.Fatalf("UpdateEvidenceAnalysis on main failed: %v", err)
	}
	e.deletePerson(t, e.f.handler, domain.MainBranchID, e.person)

	res, err := e.f.handler.MergeBranch(ctx, command.MergeBranchInput{BranchID: e.branch.ID})
	if !errors.Is(err, command.ErrMergeConflicts) {
		t.Fatalf("MergeBranch error = %v, want ErrMergeConflicts", err)
	}
	if res == nil || len(res.Conflicts) != 1 || res.Conflicts[0].StreamID != analysis.ID {
		t.Fatalf("merge conflicts = %+v, want one on the analysis stream", res)
	}

	_, err = e.f.handler.MergeBranch(ctx, command.MergeBranchInput{
		BranchID:    e.branch.ID,
		Resolutions: map[uuid.UUID]command.MergeResolution{analysis.ID: command.ResolveBranch},
	})
	e.assertRefusedBeforeClaim(t, err)

	if _, err := e.f.handler.MergeBranch(ctx, command.MergeBranchInput{
		BranchID:    e.branch.ID,
		Resolutions: map[uuid.UUID]command.MergeResolution{analysis.ID: command.ResolveMain},
	}); err != nil {
		t.Fatalf("MergeBranch with a main resolution failed: %v", err)
	}
}

// The branch records an analysis about a subject id no person or family ever
// had. The write path accepts that on main as on the branch, so the merge
// does too.
func TestMergeBranch_GPSAboutUnknownSubjectMerges(t *testing.T) {
	e := newEvidenceFixture(t)
	ctx := context.Background()

	unknown := uuid.New()
	analysis := e.analyse(t, e.f.handler.WithBranch(e.branch), unknown, "Born 1815")
	if _, err := e.f.handler.MergeBranch(ctx, command.MergeBranchInput{BranchID: e.branch.ID}); err != nil {
		t.Fatalf("MergeBranch failed: %v", err)
	}
	got, err := e.f.readStore.GetEvidenceAnalysis(ctx, domain.MainBranchID, analysis.ID)
	if err != nil || got == nil || got.SubjectID != unknown {
		t.Errorf("main GetEvidenceAnalysis = %+v (err=%v), want the branch's analysis", got, err)
	}
}

// The branch still may not bring research about a person main deleted: the
// subject existed, so the analysis would be about nothing.
func TestMergeBranch_GPSAboutMainDeletedSubjectIsStillRefused(t *testing.T) {
	e := newEvidenceFixture(t)
	e.analyse(t, e.f.handler.WithBranch(e.branch), e.person, "Born 1815")
	e.deletePerson(t, e.f.handler, domain.MainBranchID, e.person)

	_, err := e.f.handler.MergeBranch(context.Background(), command.MergeBranchInput{BranchID: e.branch.ID})
	e.assertRefusedBeforeClaim(t, err)
}

// repointThenDeleteOnBranch has the branch re-point main's analysis of p to q
// and then delete p.
func repointThenDeleteOnBranch(t *testing.T, e evidenceResume) (branch *domain.Branch, p, q, analysis uuid.UUID) {
	t.Helper()
	ctx := context.Background()
	p = e.mainPerson(t, "Pat")
	q = e.mainPerson(t, "Quinn")
	analysis = e.analysis(t, e.f.handler, p, "Born 1850")
	branch, scoped := e.branch(t, "repoint")
	if _, err := scoped.UpdateEvidenceAnalysis(ctx, command.UpdateEvidenceAnalysisInput{ID: analysis, SubjectID: &q, Version: 1}); err != nil {
		t.Fatalf("branch UpdateEvidenceAnalysis failed: %v", err)
	}
	person, err := e.f.readStore.GetPerson(ctx, domain.BranchID(branch.ID), p)
	if err != nil || person == nil {
		t.Fatalf("GetPerson = %v, %v", person, err)
	}
	if err := scoped.DeletePerson(ctx, command.DeletePersonInput{ID: p, Version: person.Version, Reason: "duplicate"}); err != nil {
		t.Fatalf("branch DeletePerson failed: %v", err)
	}
	return branch, p, q, analysis
}

// The branch's re-point of an analysis lands on main but its projection fails,
// so main's row still says the analysis is about the person the branch then
// deletes. The resume judges the delete from main's log, not the stale row:
// the delete lands, and the repaired analysis is about the new subject.
func TestResumeMerge_SubjectDeleteAfterStaleLandedRepointFinishes(t *testing.T) {
	e := newEvidenceResume(t)
	ctx := context.Background()
	branch, p, q, analysis := repointThenDeleteOnBranch(t, e)

	e.failGPSProjection(t, branch, analysis)
	if got := e.mainAnalysis(t, analysis); got == nil || got.SubjectID != p {
		t.Fatalf("main analysis after the failed projection = %+v, want the stale row about %s", got, p)
	}

	res, err := e.f.handler.ResumeMerge(ctx, command.ResumeMergeInput{BranchID: branch.ID})
	if errors.Is(err, command.ErrMergeResumeNeedsResolution) {
		res, err = e.f.handler.ResumeMerge(ctx, command.ResumeMergeInput{
			BranchID: branch.ID, Resolutions: map[uuid.UUID]command.MergeResolution{p: command.ResolveBranch},
		})
	}
	if err != nil {
		t.Fatalf("ResumeMerge failed: %v (result %+v)", err, res)
	}
	if got := e.mainAnalysis(t, analysis); got == nil || got.SubjectID != q {
		t.Errorf("main analysis after the resume = %+v, want it about %s", got, q)
	}
	if got, err := e.f.readStore.GetPerson(ctx, domain.MainBranchID, p); err != nil || got != nil {
		t.Errorf("main person %s = %+v (err=%v), want the branch's delete landed", p, got, err)
	}
}

// The re-point lands cleanly, and main then re-points the analysis back to
// the person the branch deletes. That is main's own change the branch never
// saw, so the delete is pending, "branch" is refused, and "main" keeps both.
func TestResumeMerge_SubjectDeleteAfterMainRepointsBackIsPending(t *testing.T) {
	e := newEvidenceResume(t)
	ctx := context.Background()
	branch, p, _, analysis := repointThenDeleteOnBranch(t, e)

	e.interrupt(t, branch, 2)
	landed := e.mainAnalysis(t, analysis)
	if landed == nil {
		t.Fatal("main lacks the analysis after the interruption")
	}
	if _, err := e.f.handler.UpdateEvidenceAnalysis(ctx, command.UpdateEvidenceAnalysisInput{
		ID: analysis, SubjectID: &p, Version: landed.Version,
	}); err != nil {
		t.Fatalf("main UpdateEvidenceAnalysis failed: %v", err)
	}

	result, err := e.f.handler.ResumeMerge(ctx, command.ResumeMergeInput{BranchID: branch.ID})
	if !errors.Is(err, command.ErrMergeResumeNeedsResolution) {
		t.Fatalf("ResumeMerge error = %v, want ErrMergeResumeNeedsResolution", err)
	}
	if !slices.Equal(result.PendingStreamIDs, []uuid.UUID{p}) {
		t.Fatalf("PendingStreamIDs = %v, want [%s]", result.PendingStreamIDs, p)
	}
	if _, err := e.f.handler.ResumeMerge(ctx, command.ResumeMergeInput{
		BranchID: branch.ID, Resolutions: map[uuid.UUID]command.MergeResolution{p: command.ResolveBranch},
	}); !errors.Is(err, command.ErrMergeDanglingReference) {
		t.Fatalf("ResumeMerge(branch) error = %v, want ErrMergeDanglingReference", err)
	}
	e.resume(t, branch.ID, map[uuid.UUID]command.MergeResolution{p: command.ResolveMain})
	if got := e.mainAnalysis(t, analysis); got == nil || got.SubjectID != p {
		t.Errorf("main analysis = %+v, want kept about %s", got, p)
	}
}
