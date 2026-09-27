package command_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/cacack/my-family/internal/command"
	"github.com/cacack/my-family/internal/domain"
)

// Merge guards for GPS research that main's subject-delete cascade removes
// with no event on the artifact's own stream (#760). Per-stream conflict
// detection cannot see those removals, so the merge checks them itself.

// refork replaces the fixture's branch with one forked now, so it sees main's
// current research.
func (e *evidenceFixture) refork(t *testing.T, name string) *command.Handler {
	t.Helper()
	branch, err := e.f.handler.CreateBranch(context.Background(), name, "")
	if err != nil {
		t.Fatalf("CreateBranch failed: %v", err)
	}
	e.branch = branch
	return e.f.handler.WithBranch(branch)
}

func (e evidenceFixture) researchLog(t *testing.T, h *command.Handler, subject uuid.UUID) *command.CreateResearchLogResult {
	t.Helper()
	res, err := h.CreateResearchLog(context.Background(), command.CreateResearchLogInput{
		SubjectID: subject, SubjectType: "person", Repository: "Archive", SearchDescription: "Baptisms",
		Outcome: string(domain.ResearchOutcomeNotFound), SearchDate: time.Date(2024, 3, 1, 0, 0, 0, 0, time.UTC),
	})
	if err != nil {
		t.Fatalf("CreateResearchLog failed: %v", err)
	}
	return res
}

// The branch edits main's analysis; main then deletes the analysis's subject,
// which cascades the analysis away with no event on its stream. Replaying the
// edit would be a silent no-op, so the merge is refused.
func TestMergeBranch_GPSEditOfMainCascadedArtifactIsRefused(t *testing.T) {
	e := newEvidenceFixture(t)
	ctx := context.Background()

	analysis := e.analyse(t, e.f.handler, e.person, "Born 1815")
	scoped := e.refork(t, "edit-analysis")
	conclusion := "Born 1816"
	if _, err := scoped.UpdateEvidenceAnalysis(ctx, command.UpdateEvidenceAnalysisInput{
		ID: analysis.ID, Conclusion: &conclusion, Version: analysis.Version,
	}); err != nil {
		t.Fatalf("UpdateEvidenceAnalysis on branch failed: %v", err)
	}
	e.deletePerson(t, e.f.handler, domain.MainBranchID, e.person)

	_, err := e.f.handler.MergeBranch(ctx, command.MergeBranchInput{BranchID: e.branch.ID})
	e.assertRefusedBeforeClaim(t, err)
}

// The same edit of an analysis main still has merges and lands on main.
func TestMergeBranch_GPSEditOfLiveArtifactMerges(t *testing.T) {
	e := newEvidenceFixture(t)
	ctx := context.Background()

	analysis := e.analyse(t, e.f.handler, e.person, "Born 1815")
	scoped := e.refork(t, "edit-analysis")
	conclusion := "Born 1816"
	if _, err := scoped.UpdateEvidenceAnalysis(ctx, command.UpdateEvidenceAnalysisInput{
		ID: analysis.ID, Conclusion: &conclusion, Version: analysis.Version,
	}); err != nil {
		t.Fatalf("UpdateEvidenceAnalysis on branch failed: %v", err)
	}

	if _, err := e.f.handler.MergeBranch(ctx, command.MergeBranchInput{BranchID: e.branch.ID}); err != nil {
		t.Fatalf("MergeBranch failed: %v", err)
	}
	got, err := e.f.readStore.GetEvidenceAnalysis(ctx, domain.MainBranchID, analysis.ID)
	if err != nil || got == nil || got.Conclusion != conclusion {
		t.Errorf("main GetEvidenceAnalysis = %+v (err=%v), want the branch's conclusion", got, err)
	}
}

// Main deletes the analysis the branch edited through its own event: that is an
// edit-vs-delete merge conflict on the analysis stream, reported as such rather
// than pre-empted by the dangling-reference guard.
func TestMergeBranch_GPSEditOfMainDeletedArtifactIsAConflict(t *testing.T) {
	e := newEvidenceFixture(t)
	ctx := context.Background()

	analysis := e.analyse(t, e.f.handler, e.person, "Born 1815")
	scoped := e.refork(t, "edit-analysis")
	conclusion := "Born 1816"
	if _, err := scoped.UpdateEvidenceAnalysis(ctx, command.UpdateEvidenceAnalysisInput{
		ID: analysis.ID, Conclusion: &conclusion, Version: analysis.Version,
	}); err != nil {
		t.Fatalf("UpdateEvidenceAnalysis on branch failed: %v", err)
	}
	if err := e.f.handler.DeleteEvidenceAnalysis(ctx, analysis.ID, analysis.Version, "wrong"); err != nil {
		t.Fatalf("DeleteEvidenceAnalysis on main failed: %v", err)
	}

	res, err := e.f.handler.MergeBranch(ctx, command.MergeBranchInput{BranchID: e.branch.ID})
	if !errors.Is(err, command.ErrMergeConflicts) {
		t.Fatalf("MergeBranch error = %v, want ErrMergeConflicts", err)
	}
	if res == nil || len(res.Conflicts) != 1 || res.Conflicts[0].StreamID != analysis.ID {
		t.Fatalf("merge conflicts = %+v, want one on the analysis stream", res)
	}
	// Resolving it to main skips the edit, and the merge goes through.
	if _, err := e.f.handler.MergeBranch(ctx, command.MergeBranchInput{
		BranchID:    e.branch.ID,
		Resolutions: map[uuid.UUID]command.MergeResolution{analysis.ID: command.ResolveMain},
	}); err != nil {
		t.Fatalf("MergeBranch with a main resolution failed: %v", err)
	}
}

// Main records a negative search about a person after the fork; the branch,
// which never saw it, deletes the person. Replaying the delete would cascade
// main's research log away with no record, so the merge is refused.
func TestMergeBranch_SubjectDeleteOrphaningMainPostForkResearchIsRefused(t *testing.T) {
	e := newEvidenceFixture(t)
	ctx := context.Background()

	e.deletePerson(t, e.f.handler.WithBranch(e.branch), domain.BranchID(e.branch.ID), e.person)
	log := e.researchLog(t, e.f.handler, e.person)

	_, err := e.f.handler.MergeBranch(ctx, command.MergeBranchInput{BranchID: e.branch.ID})
	e.assertRefusedBeforeClaim(t, err)
	if got, err := e.f.readStore.GetResearchLog(ctx, domain.MainBranchID, log.ID); err != nil || got == nil {
		t.Errorf("main GetResearchLog after the refused merge = %+v (err=%v), want it kept", got, err)
	}
}

// Main changes, after the fork, an analysis the branch saw; the branch deletes
// the analysis's subject. The cascade would drop main's newer version.
func TestMergeBranch_SubjectDeleteOrphaningMainPostForkEditIsRefused(t *testing.T) {
	e := newEvidenceFixture(t)
	ctx := context.Background()

	analysis := e.analyse(t, e.f.handler, e.person, "Born 1815")
	scoped := e.refork(t, "delete-subject")
	e.deletePerson(t, scoped, domain.BranchID(e.branch.ID), e.person)
	conclusion := "Born 1816"
	if _, err := e.f.handler.UpdateEvidenceAnalysis(ctx, command.UpdateEvidenceAnalysisInput{
		ID: analysis.ID, Conclusion: &conclusion, Version: analysis.Version,
	}); err != nil {
		t.Fatalf("UpdateEvidenceAnalysis on main failed: %v", err)
	}

	_, err := e.f.handler.MergeBranch(ctx, command.MergeBranchInput{BranchID: e.branch.ID})
	e.assertRefusedBeforeClaim(t, err)
}

// Research main had at the fork, unchanged since, is exactly what the branch's
// own delete cascaded: the merge goes through and cascades it on main too.
func TestMergeBranch_SubjectDeleteOfResearchSeenAtForkMerges(t *testing.T) {
	e := newEvidenceFixture(t)
	ctx := context.Background()

	analysis := e.analyse(t, e.f.handler, e.person, "Born 1815")
	log := e.researchLog(t, e.f.handler, e.person)
	scoped := e.refork(t, "delete-subject")
	e.deletePerson(t, scoped, domain.BranchID(e.branch.ID), e.person)

	if _, err := e.f.handler.MergeBranch(ctx, command.MergeBranchInput{BranchID: e.branch.ID}); err != nil {
		t.Fatalf("MergeBranch failed: %v", err)
	}
	if got, err := e.f.readStore.GetEvidenceAnalysis(ctx, domain.MainBranchID, analysis.ID); err != nil || got != nil {
		t.Errorf("main GetEvidenceAnalysis = %+v (err=%v), want cascaded away", got, err)
	}
	if got, err := e.f.readStore.GetResearchLog(ctx, domain.MainBranchID, log.ID); err != nil || got != nil {
		t.Errorf("main GetResearchLog = %+v (err=%v), want cascaded away", got, err)
	}
}

// repointThenDelete has the branch re-point main's research log at another
// person and delete the original subject. touchFirst edits the subject before
// the re-point, so the subject's stream replays first.
func repointThenDelete(t *testing.T, touchFirst bool) (evidenceFixture, uuid.UUID, uuid.UUID, error) {
	t.Helper()
	e := newEvidenceFixture(t)
	ctx := context.Background()

	other, err := e.f.handler.CreatePerson(ctx, command.CreatePersonInput{GivenName: "Other", Surname: "Person"})
	if err != nil {
		t.Fatalf("CreatePerson failed: %v", err)
	}
	log := e.researchLog(t, e.f.handler, e.person)
	scoped := e.refork(t, "repoint")
	branchID := domain.BranchID(e.branch.ID)
	if touchFirst {
		p, err := e.f.readStore.GetPerson(ctx, branchID, e.person)
		if err != nil || p == nil {
			t.Fatalf("GetPerson = %+v (err=%v)", p, err)
		}
		notes := "Touched first"
		if _, err := scoped.UpdatePerson(ctx, command.UpdatePersonInput{ID: e.person, Notes: &notes, Version: p.Version}); err != nil {
			t.Fatalf("UpdatePerson failed: %v", err)
		}
	}
	if _, err := scoped.UpdateResearchLog(ctx, command.UpdateResearchLogInput{
		ID: log.ID, SubjectID: &other.ID, Version: log.Version,
	}); err != nil {
		t.Fatalf("UpdateResearchLog on branch failed: %v", err)
	}
	e.deletePerson(t, scoped, branchID, e.person)

	_, err = e.f.handler.MergeBranch(ctx, command.MergeBranchInput{BranchID: e.branch.ID})
	return e, log.ID, other.ID, err
}

// The re-point replays before the subject's delete: the log survives on main
// under its new subject.
func TestMergeBranch_GPSRepointBeforeSubjectDeleteMerges(t *testing.T) {
	e, logID, otherID, err := repointThenDelete(t, false)
	if err != nil {
		t.Fatalf("MergeBranch failed: %v", err)
	}
	got, err := e.f.readStore.GetResearchLog(context.Background(), domain.MainBranchID, logID)
	if err != nil || got == nil || got.SubjectID != otherID {
		t.Errorf("main GetResearchLog = %+v (err=%v), want it re-pointed at the other person", got, err)
	}
}

// The subject's stream replays first: its delete would cascade the log off
// main before the re-point lands, so the merge is refused.
func TestMergeBranch_GPSRepointAfterSubjectDeleteIsRefused(t *testing.T) {
	e, logID, _, err := repointThenDelete(t, true)
	e.assertRefusedBeforeClaim(t, err)
	if got, err := e.f.readStore.GetResearchLog(context.Background(), domain.MainBranchID, logID); err != nil || got == nil {
		t.Errorf("main GetResearchLog after the refused merge = %+v (err=%v), want it kept", got, err)
	}
}
