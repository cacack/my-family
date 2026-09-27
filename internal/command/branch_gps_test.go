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

// GPS artifacts on a branch (#760): evidence analyses, evidence conflicts,
// research logs and proof summaries are written, read and merged through the
// branch like any other branch-scoped entity.

func (e evidenceFixture) analyse(t *testing.T, h *command.Handler, subject uuid.UUID, conclusion string) *command.CreateEvidenceAnalysisResult {
	t.Helper()
	res, err := h.CreateEvidenceAnalysis(context.Background(), command.CreateEvidenceAnalysisInput{
		FactType: string(domain.FactPersonBirth), SubjectID: subject, Conclusion: conclusion,
	})
	if err != nil {
		t.Fatalf("CreateEvidenceAnalysis(%q) failed: %v", conclusion, err)
	}
	return res
}

func (e evidenceFixture) openConflicts(t *testing.T, branchID domain.BranchID) []uuid.UUID {
	t.Helper()
	conflicts, err := e.f.readStore.ListUnresolvedConflicts(context.Background(), branchID)
	if err != nil {
		t.Fatalf("ListUnresolvedConflicts failed: %v", err)
	}
	ids := make([]uuid.UUID, 0, len(conflicts))
	for _, c := range conflicts {
		ids = append(ids, c.ID)
	}
	return ids
}

// Evidence conflict detection runs on the handler's branch: a rival analysis on
// the branch compares with the analyses the branch sees and records its
// evidence conflict there, and a branch resolution of main's conflict leaves
// main's conflict open.
func TestBranchGPS_EvidenceConflictsStayOnBranch(t *testing.T) {
	e := newEvidenceFixture(t)
	ctx := context.Background()
	main := domain.MainBranchID

	// Main: two disagreeing analyses and their open conflict, before the fork.
	e.analyse(t, e.f.handler, e.person, "Born 1815")
	mainRival := e.analyse(t, e.f.handler, e.person, "Born 1816")
	if mainRival.ConflictID == nil {
		t.Fatal("main rival recorded no evidence conflict")
	}
	mainConflict := *mainRival.ConflictID
	// Re-fork so the branch sees them.
	branch, err := e.f.handler.CreateBranch(ctx, "birth-year", "")
	if err != nil {
		t.Fatalf("CreateBranch failed: %v", err)
	}
	scoped := e.f.handler.WithBranch(branch)
	branchID := domain.BranchID(branch.ID)

	// The branch reuses main's open conflict rather than recording a second one.
	branchRival := e.analyse(t, scoped, e.person, "Born 1817")
	if branchRival.ConflictID == nil || *branchRival.ConflictID != mainConflict {
		t.Fatalf("branch rival conflict = %v, want main's open conflict %s", branchRival.ConflictID, mainConflict)
	}
	if got, err := e.f.readStore.GetEvidenceAnalysis(ctx, main, branchRival.ID); err != nil || got != nil {
		t.Errorf("main GetEvidenceAnalysis(branch rival) = %+v (err=%v), want absent", got, err)
	}

	// Resolve main's conflict on the branch only.
	conflict, err := e.f.readStore.GetEvidenceConflict(ctx, branchID, mainConflict)
	if err != nil || conflict == nil {
		t.Fatalf("branch GetEvidenceConflict = %+v (err=%v)", conflict, err)
	}
	if _, err := scoped.ResolveEvidenceConflict(ctx, mainConflict, "The register wins", conflict.Version); err != nil {
		t.Fatalf("ResolveEvidenceConflict on branch failed: %v", err)
	}
	if got := e.openConflicts(t, main); len(got) != 1 || got[0] != mainConflict {
		t.Errorf("main open conflicts = %v, want [%s]", got, mainConflict)
	}
	if got := e.openConflicts(t, branchID); len(got) != 0 {
		t.Errorf("branch open conflicts = %v, want none (the branch resolved it)", got)
	}

	// With main's conflict resolved on the branch, a further disagreement on the
	// branch records a NEW conflict, on the branch only.
	another := e.analyse(t, scoped, e.person, "Born 1818")
	if another.ConflictID == nil || *another.ConflictID == mainConflict {
		t.Fatalf("second branch rival conflict = %v, want a new conflict", another.ConflictID)
	}
	if got, err := e.f.readStore.GetEvidenceConflict(ctx, main, *another.ConflictID); err != nil || got != nil {
		t.Errorf("main GetEvidenceConflict(branch conflict) = %+v (err=%v), want absent", got, err)
	}
	if got := e.openConflicts(t, branchID); len(got) != 1 || got[0] != *another.ConflictID {
		t.Errorf("branch open conflicts = %v, want only the branch's new conflict", got)
	}
}

// Research-log, proof-summary and analysis edits and deletes on a branch never
// reach main, and a research log's search date change is applied by the live
// projection (it used to be dropped until a replay).
func TestBranchGPS_EditsAndDeletesStayOnBranch(t *testing.T) {
	e := newEvidenceFixture(t)
	ctx := context.Background()
	main := domain.MainBranchID

	log, err := e.f.handler.CreateResearchLog(ctx, command.CreateResearchLogInput{
		SubjectID: e.person, SubjectType: "person", Repository: "County Archive",
		SearchDescription: "Baptisms", Outcome: string(domain.ResearchOutcomeNotFound),
		SearchDate: time.Date(2024, 3, 1, 0, 0, 0, 0, time.UTC),
	})
	if err != nil {
		t.Fatalf("CreateResearchLog failed: %v", err)
	}
	proof, err := e.f.handler.CreateProofSummary(ctx, command.CreateProofSummaryInput{
		FactType: string(domain.FactPersonBirth), SubjectID: e.person, Conclusion: "Born 1815", Argument: "Census",
	})
	if err != nil {
		t.Fatalf("CreateProofSummary failed: %v", err)
	}
	analysis := e.analyse(t, e.f.handler, e.person, "Born 1815")

	branch, err := e.f.handler.CreateBranch(ctx, "edits", "")
	if err != nil {
		t.Fatalf("CreateBranch failed: %v", err)
	}
	scoped := e.f.handler.WithBranch(branch)
	branchID := domain.BranchID(branch.ID)

	moved := time.Date(2024, 5, 2, 0, 0, 0, 0, time.UTC)
	if _, err := scoped.UpdateResearchLog(ctx, command.UpdateResearchLogInput{ID: log.ID, SearchDate: &moved, Version: log.Version}); err != nil {
		t.Fatalf("UpdateResearchLog on branch failed: %v", err)
	}
	if got, err := e.f.readStore.GetResearchLog(ctx, branchID, log.ID); err != nil || got == nil || !got.SearchDate.Equal(moved) {
		t.Errorf("branch research log = %+v (err=%v), want search date %s", got, err, moved)
	}
	if got, err := e.f.readStore.GetResearchLog(ctx, main, log.ID); err != nil || got == nil || !got.SearchDate.Equal(time.Date(2024, 3, 1, 0, 0, 0, 0, time.UTC)) {
		t.Errorf("main research log = %+v (err=%v), want the original date", got, err)
	}

	if err := scoped.DeleteProofSummary(ctx, proof.ID, proof.Version, "theory"); err != nil {
		t.Fatalf("DeleteProofSummary on branch failed: %v", err)
	}
	if got, err := e.f.readStore.GetProofSummary(ctx, branchID, proof.ID); err != nil || got != nil {
		t.Errorf("branch proof after delete = %+v (err=%v), want tombstoned", got, err)
	}
	if got, err := e.f.readStore.GetProofSummary(ctx, main, proof.ID); err != nil || got == nil {
		t.Errorf("main proof after branch delete = %+v (err=%v), want kept", got, err)
	}

	if err := scoped.DeleteEvidenceAnalysis(ctx, analysis.ID, analysis.Version, "theory"); err != nil {
		t.Fatalf("DeleteEvidenceAnalysis on branch failed: %v", err)
	}
	if err := scoped.DeleteEvidenceAnalysis(ctx, analysis.ID, analysis.Version, "theory"); !errors.Is(err, command.ErrEvidenceAnalysisNotFound) {
		t.Errorf("second branch delete = %v, want ErrEvidenceAnalysisNotFound", err)
	}
	if got, err := e.f.readStore.GetAnalysesBySubject(ctx, main, e.person); err != nil || len(got) != 1 {
		t.Errorf("main analyses of the person = %d (err=%v), want 1", len(got), err)
	}
}

// Deleting the subject on a branch tombstones its GPS artifacts on the branch
// only; main keeps them.
func TestBranchGPS_SubjectDeleteCascadesOnBranchOnly(t *testing.T) {
	e := newEvidenceFixture(t)
	ctx := context.Background()
	e.analyse(t, e.f.handler, e.person, "Born 1815")
	if _, err := e.f.handler.CreateResearchLog(ctx, command.CreateResearchLogInput{
		SubjectID: e.person, SubjectType: "person", Repository: "Archive", SearchDescription: "Baptisms",
		Outcome: string(domain.ResearchOutcomeFound), SearchDate: time.Date(2024, 3, 1, 0, 0, 0, 0, time.UTC),
	}); err != nil {
		t.Fatalf("CreateResearchLog failed: %v", err)
	}
	branch, err := e.f.handler.CreateBranch(ctx, "no-such-person", "")
	if err != nil {
		t.Fatalf("CreateBranch failed: %v", err)
	}
	branchID := domain.BranchID(branch.ID)
	e.deletePerson(t, e.f.handler.WithBranch(branch), branchID, e.person)

	for _, scope := range []struct {
		name string
		id   domain.BranchID
		want int
	}{{"branch", branchID, 0}, {"main", domain.MainBranchID, 1}} {
		analyses, err := e.f.readStore.GetAnalysesBySubject(ctx, scope.id, e.person)
		if err != nil || len(analyses) != scope.want {
			t.Errorf("%s analyses of the deleted person = %d (err=%v), want %d", scope.name, len(analyses), err, scope.want)
		}
		logs, err := e.f.readStore.GetResearchLogsForSubject(ctx, scope.id, e.person)
		if err != nil || len(logs) != scope.want {
			t.Errorf("%s research logs of the deleted person = %d (err=%v), want %d", scope.name, len(logs), err, scope.want)
		}
	}
}

// Merging a branch replays its GPS artifacts onto main: a branch analysis, the
// branch's resolution of main's evidence conflict, and a branch research log.
func TestMergeBranch_GPSArtifactsReplayOntoMain(t *testing.T) {
	e := newEvidenceFixture(t)
	ctx := context.Background()
	main := domain.MainBranchID

	e.analyse(t, e.f.handler, e.person, "Born 1815")
	rival := e.analyse(t, e.f.handler, e.person, "Born 1816")
	branch, err := e.f.handler.CreateBranch(ctx, "resolve", "")
	if err != nil {
		t.Fatalf("CreateBranch failed: %v", err)
	}
	e.branch = branch
	scoped := e.f.handler.WithBranch(branch)
	branchID := domain.BranchID(branch.ID)

	conflict, err := e.f.readStore.GetEvidenceConflict(ctx, branchID, *rival.ConflictID)
	if err != nil || conflict == nil {
		t.Fatalf("branch GetEvidenceConflict = %+v (err=%v)", conflict, err)
	}
	if _, err := scoped.ResolveEvidenceConflict(ctx, conflict.ID, "The register wins", conflict.Version); err != nil {
		t.Fatalf("ResolveEvidenceConflict failed: %v", err)
	}
	proof, err := scoped.CreateProofSummary(ctx, command.CreateProofSummaryInput{
		FactType: string(domain.FactPersonBirth), SubjectID: e.person, Conclusion: "Born 1815", Argument: "The register",
	})
	if err != nil {
		t.Fatalf("CreateProofSummary on branch failed: %v", err)
	}

	if _, err := e.f.handler.MergeBranch(ctx, command.MergeBranchInput{BranchID: branch.ID}); err != nil {
		t.Fatalf("MergeBranch failed: %v", err)
	}
	if got := e.openConflicts(t, main); len(got) != 0 {
		t.Errorf("main open conflicts after the merge = %v, want none (the branch resolved it)", got)
	}
	if got, err := e.f.readStore.GetProofSummary(ctx, main, proof.ID); err != nil || got == nil || got.Argument != "The register" {
		t.Errorf("main proof after the merge = %+v (err=%v), want the branch's proof", got, err)
	}
}

// Main and the branch resolve the same evidence conflict differently: the
// merge conflict scan must see it (EvidenceConflictResolved carries no Changes
// map, so the *Updated fold alone would read nothing from it).
func TestMergeBranch_DivergentEvidenceConflictResolutionsConflict(t *testing.T) {
	e := newEvidenceFixture(t)
	ctx := context.Background()

	e.analyse(t, e.f.handler, e.person, "Born 1815")
	rival := e.analyse(t, e.f.handler, e.person, "Born 1816")
	branch, err := e.f.handler.CreateBranch(ctx, "resolve-differently", "")
	if err != nil {
		t.Fatalf("CreateBranch failed: %v", err)
	}
	branchID := domain.BranchID(branch.ID)

	resolve := func(h *command.Handler, scope domain.BranchID, resolution string) {
		t.Helper()
		c, err := e.f.readStore.GetEvidenceConflict(ctx, scope, *rival.ConflictID)
		if err != nil || c == nil {
			t.Fatalf("GetEvidenceConflict = %+v (err=%v)", c, err)
		}
		if _, err := h.ResolveEvidenceConflict(ctx, c.ID, resolution, c.Version); err != nil {
			t.Fatalf("ResolveEvidenceConflict failed: %v", err)
		}
	}
	resolve(e.f.handler.WithBranch(branch), branchID, "The census wins")
	resolve(e.f.handler, domain.MainBranchID, "The register wins")

	res, err := e.f.handler.MergeBranch(ctx, command.MergeBranchInput{BranchID: branch.ID})
	if !errors.Is(err, command.ErrMergeConflicts) {
		t.Fatalf("MergeBranch error = %v, want ErrMergeConflicts", err)
	}
	if res == nil || len(res.Conflicts) != 1 || res.Conflicts[0].StreamID != *rival.ConflictID {
		t.Fatalf("merge conflicts = %+v, want one on the evidence conflict's stream", res)
	}
}

// A branch analyses a person main deletes after the fork: replaying it would
// leave main with research about nobody.
func TestMergeBranch_GPSArtifactOfMainDeletedSubjectIsRefused(t *testing.T) {
	e := newEvidenceFixture(t)
	ctx := context.Background()

	analysis := e.analyse(t, e.f.handler.WithBranch(e.branch), e.person, "Born 1815")
	e.deletePerson(t, e.f.handler, domain.MainBranchID, e.person)

	_, err := e.f.handler.MergeBranch(ctx, command.MergeBranchInput{BranchID: e.branch.ID})
	e.assertRefusedBeforeClaim(t, err)
	if got, err := e.f.readStore.GetEvidenceAnalysis(ctx, domain.MainBranchID, analysis.ID); err != nil || got != nil {
		t.Errorf("main GetEvidenceAnalysis after the refused merge = %+v (err=%v), want absent", got, err)
	}
}

// A branch re-points main's research log at a person main deletes after the
// fork: the final subject is what must survive.
func TestMergeBranch_GPSArtifactRepointedAtMainDeletedSubjectIsRefused(t *testing.T) {
	e := newEvidenceFixture(t)
	ctx := context.Background()

	other, err := e.f.handler.CreatePerson(ctx, command.CreatePersonInput{GivenName: "Other", Surname: "Person"})
	if err != nil {
		t.Fatalf("CreatePerson failed: %v", err)
	}
	log, err := e.f.handler.CreateResearchLog(ctx, command.CreateResearchLogInput{
		SubjectID: e.person, SubjectType: "person", Repository: "Archive", SearchDescription: "Baptisms",
		Outcome: string(domain.ResearchOutcomeFound), SearchDate: time.Date(2024, 3, 1, 0, 0, 0, 0, time.UTC),
	})
	if err != nil {
		t.Fatalf("CreateResearchLog failed: %v", err)
	}
	branch, err := e.f.handler.CreateBranch(ctx, "repoint", "")
	if err != nil {
		t.Fatalf("CreateBranch failed: %v", err)
	}
	e.branch = branch
	if _, err := e.f.handler.WithBranch(branch).UpdateResearchLog(ctx, command.UpdateResearchLogInput{
		ID: log.ID, SubjectID: &other.ID, Version: log.Version,
	}); err != nil {
		t.Fatalf("UpdateResearchLog on branch failed: %v", err)
	}
	e.deletePerson(t, e.f.handler, domain.MainBranchID, other.ID)

	_, err = e.f.handler.MergeBranch(ctx, command.MergeBranchInput{BranchID: branch.ID})
	e.assertRefusedBeforeClaim(t, err)
}

// The branch analyses main's person and later deletes that person. The analysis
// stream replays first and the person's delete then cascades it on main, so
// nothing dangles and the merge goes through.
func TestMergeBranch_GPSArtifactThenSubjectDeleteOnBranchMerges(t *testing.T) {
	e := newEvidenceFixture(t)
	ctx := context.Background()
	scoped := e.f.handler.WithBranch(e.branch)

	analysis := e.analyse(t, scoped, e.person, "Born 1815")
	e.deletePerson(t, scoped, domain.BranchID(e.branch.ID), e.person)

	if _, err := e.f.handler.MergeBranch(ctx, command.MergeBranchInput{BranchID: e.branch.ID}); err != nil {
		t.Fatalf("MergeBranch failed: %v", err)
	}
	if got, err := e.f.readStore.GetEvidenceAnalysis(ctx, domain.MainBranchID, analysis.ID); err != nil || got != nil {
		t.Errorf("main GetEvidenceAnalysis = %+v (err=%v), want cascaded away with its subject", got, err)
	}
}

// A branch creates a person, analyses them, then deletes them: the person's
// stream replays first, so the analysis would land on a person main no longer
// has.
func TestMergeBranch_GPSArtifactOfSubjectDeletedEarlierInReplayIsRefused(t *testing.T) {
	e := newEvidenceFixture(t)
	ctx := context.Background()
	scoped := e.f.handler.WithBranch(e.branch)
	branchID := domain.BranchID(e.branch.ID)

	p, err := scoped.CreatePerson(ctx, command.CreatePersonInput{GivenName: "Brief", Surname: "Hypothesis"})
	if err != nil {
		t.Fatalf("CreatePerson failed: %v", err)
	}
	e.analyse(t, scoped, p.ID, "Born 1815")
	e.deletePerson(t, scoped, branchID, p.ID)

	_, err = e.f.handler.MergeBranch(ctx, command.MergeBranchInput{BranchID: e.branch.ID})
	e.assertRefusedBeforeClaim(t, err)
}

// A branch analysis of a family main still has merges.
func TestMergeBranch_GPSArtifactOfLiveFamilyMerges(t *testing.T) {
	e := newEvidenceFixture(t)
	ctx := context.Background()

	fam, err := e.f.handler.CreateFamily(ctx, command.CreateFamilyInput{Partner1ID: &e.person})
	if err != nil {
		t.Fatalf("CreateFamily failed: %v", err)
	}
	branch, err := e.f.handler.CreateBranch(ctx, "family-analysis", "")
	if err != nil {
		t.Fatalf("CreateBranch failed: %v", err)
	}
	res, err := e.f.handler.WithBranch(branch).CreateEvidenceAnalysis(ctx, command.CreateEvidenceAnalysisInput{
		FactType: string(domain.FactFamilyMarriage), SubjectID: fam.ID, Conclusion: "Married 1835",
	})
	if err != nil {
		t.Fatalf("CreateEvidenceAnalysis failed: %v", err)
	}
	if _, err := e.f.handler.MergeBranch(ctx, command.MergeBranchInput{BranchID: branch.ID}); err != nil {
		t.Fatalf("MergeBranch failed: %v", err)
	}
	if got, err := e.f.readStore.GetEvidenceAnalysis(ctx, domain.MainBranchID, res.ID); err != nil || got == nil {
		t.Errorf("main GetEvidenceAnalysis = %+v (err=%v), want the merged analysis", got, err)
	}
}
