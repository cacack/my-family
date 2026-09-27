package command_test

// Resuming a merge that carries GPS artifacts (#760 on top of #685): the
// merge's GPS rules with resume's pending/decidable semantics, landed
// detection, and the read-model repair of evidence analysis, evidence
// conflict, research log and proof summary streams.

import (
	"context"
	"errors"
	"slices"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/cacack/my-family/internal/command"
	"github.com/cacack/my-family/internal/domain"
	"github.com/cacack/my-family/internal/repository"
)

func (s *faultyReadStore) SaveResearchLog(ctx context.Context, branchID domain.BranchID, log *repository.ResearchLogReadModel) error {
	if s.armed && branchID.IsMain() && log.ID == s.failGPS {
		return errInjectedProjectionFailure
	}
	return s.ReadModelStore.SaveResearchLog(ctx, branchID, log)
}

func (s *faultyReadStore) SaveEvidenceAnalysis(ctx context.Context, branchID domain.BranchID, analysis *repository.EvidenceAnalysisReadModel) error {
	if s.armed && branchID.IsMain() && analysis.ID == s.failGPS {
		return errInjectedProjectionFailure
	}
	return s.ReadModelStore.SaveEvidenceAnalysis(ctx, branchID, analysis)
}

// researchLog records a negative search about subject on h's scope.
func (e evidenceResume) researchLog(t *testing.T, h *command.Handler, subject uuid.UUID, repo string) uuid.UUID {
	t.Helper()
	res, err := h.CreateResearchLog(context.Background(), command.CreateResearchLogInput{
		SubjectID: subject, SubjectType: "person", Repository: repo,
		SearchDescription: "Baptisms", Outcome: "not_found", SearchDate: time.Date(2024, 3, 1, 0, 0, 0, 0, time.UTC),
	})
	if err != nil {
		t.Fatalf("CreateResearchLog(%q) failed: %v", repo, err)
	}
	return res.ID
}

// analysis records an evidence analysis of subject's birth on h's scope.
func (e evidenceResume) analysis(t *testing.T, h *command.Handler, subject uuid.UUID, conclusion string) uuid.UUID {
	t.Helper()
	res, err := h.CreateEvidenceAnalysis(context.Background(), command.CreateEvidenceAnalysisInput{
		FactType: string(domain.FactPersonBirth), SubjectID: subject, Conclusion: conclusion,
	})
	if err != nil {
		t.Fatalf("CreateEvidenceAnalysis(%q) failed: %v", conclusion, err)
	}
	return res.ID
}

func (e evidenceResume) mainLog(t *testing.T, id uuid.UUID) *repository.ResearchLogReadModel {
	t.Helper()
	l, err := e.f.readStore.GetResearchLog(context.Background(), domain.MainBranchID, id)
	if err != nil {
		t.Fatalf("GetResearchLog(%s) failed: %v", id, err)
	}
	return l
}

func (e evidenceResume) mainAnalysis(t *testing.T, id uuid.UUID) *repository.EvidenceAnalysisReadModel {
	t.Helper()
	a, err := e.f.readStore.GetEvidenceAnalysis(context.Background(), domain.MainBranchID, id)
	if err != nil {
		t.Fatalf("GetEvidenceAnalysis(%s) failed: %v", id, err)
	}
	return a
}

// failGPSProjection runs a merge whose main projection of one GPS artifact
// fails after its append.
func (e evidenceResume) failGPSProjection(t *testing.T, branch *domain.Branch, id uuid.UUID) {
	t.Helper()
	e.reads.armed, e.reads.failGPS = true, id
	_, err := e.f.handler.MergeBranch(context.Background(), command.MergeBranchInput{BranchID: branch.ID})
	e.reads.armed, e.reads.failGPS = false, uuid.Nil
	if !errors.Is(err, command.ErrMergePartiallyApplied) {
		t.Fatalf("MergeBranch error = %v, want ErrMergePartiallyApplied", err)
	}
}

// TestResumeMerge_GPSArtifactsFinish: the branch edits main's evidence
// analysis and records a research log and a proof summary. The merge stops
// after the analysis edit lands; the resume replays the rest, and a second
// resume does nothing.
func TestResumeMerge_GPSArtifactsFinish(t *testing.T) {
	e := newEvidenceResume(t)
	ctx := context.Background()
	analysis := e.analysis(t, e.f.handler, e.person, "Born 1815")
	branch, scoped := e.branch(t, "gps-line")
	conclusion := "Born 1815, London"
	if _, err := scoped.UpdateEvidenceAnalysis(ctx, command.UpdateEvidenceAnalysisInput{ID: analysis, Conclusion: &conclusion, Version: 1}); err != nil {
		t.Fatalf("branch UpdateEvidenceAnalysis failed: %v", err)
	}
	logID := e.researchLog(t, scoped, e.person, "Parish chest")
	proof, err := scoped.CreateProofSummary(ctx, command.CreateProofSummaryInput{
		FactType: string(domain.FactPersonBirth), SubjectID: e.person, Conclusion: "Born 1815", Argument: "The register agrees",
	})
	if err != nil {
		t.Fatalf("branch CreateProofSummary failed: %v", err)
	}

	e.interrupt(t, branch, 2)
	if got := e.mainAnalysis(t, analysis); got == nil || got.Conclusion != conclusion {
		t.Fatalf("main analysis after the interruption = %+v, want the landed edit", got)
	}
	if e.mainLog(t, logID) != nil {
		t.Fatalf("main has the research log before the resume")
	}

	res := e.resume(t, branch.ID, nil)
	if !slices.Equal(res.AlreadyReplayedStreamIDs, []uuid.UUID{analysis}) {
		t.Errorf("AlreadyReplayedStreamIDs = %v, want [%s]", res.AlreadyReplayedStreamIDs, analysis)
	}
	if res.ReplayedEventCount != 2 {
		t.Errorf("ReplayedEventCount = %d, want 2 (the log and the proof)", res.ReplayedEventCount)
	}
	if e.mainLog(t, logID) == nil {
		t.Error("main lacks the branch's research log")
	}
	if got, err := e.f.readStore.GetProofSummary(ctx, domain.MainBranchID, proof.ID); err != nil || got == nil {
		t.Errorf("main proof summary = %v (err=%v), want it replayed", got, err)
	}
	assertResumeNoop(t, e.resume(t, branch.ID, nil))
}

// TestResumeMerge_MainDeletesGPSSubjectAfterInterruption: the branch renames
// a person and logs research about main's person O; the replay stops after
// the rename, and main then deletes O. Replaying the log would leave research
// about nothing, so it is pending, "branch" is refused, and "main" rolls the
// merge forward without it.
func TestResumeMerge_MainDeletesGPSSubjectAfterInterruption(t *testing.T) {
	e := newEvidenceResume(t)
	ctx := context.Background()
	subject := e.mainPerson(t, "Owen")
	branch, scoped := e.branch(t, "subject-lost")
	e.renameOnBranch(t, scoped, domain.BranchID(branch.ID), e.person)
	logID := e.researchLog(t, scoped, subject, "County archive")

	e.interrupt(t, branch, 2)
	e.deleteMainPerson(t, subject)

	result, err := e.f.handler.ResumeMerge(ctx, command.ResumeMergeInput{BranchID: branch.ID})
	if !errors.Is(err, command.ErrMergeResumeNeedsResolution) {
		t.Fatalf("ResumeMerge error = %v, want ErrMergeResumeNeedsResolution", err)
	}
	if !slices.Equal(result.PendingStreamIDs, []uuid.UUID{logID}) {
		t.Fatalf("PendingStreamIDs = %v, want [%s]", result.PendingStreamIDs, logID)
	}
	if _, err := e.f.handler.ResumeMerge(ctx, command.ResumeMergeInput{
		BranchID: branch.ID, Resolutions: map[uuid.UUID]command.MergeResolution{logID: command.ResolveBranch},
	}); !errors.Is(err, command.ErrMergeDanglingReference) {
		t.Fatalf("ResumeMerge(branch) error = %v, want ErrMergeDanglingReference", err)
	}
	res := e.resume(t, branch.ID, map[uuid.UUID]command.MergeResolution{logID: command.ResolveMain})
	if !slices.Equal(res.SkippedStreamIDs, []uuid.UUID{logID}) {
		t.Errorf("SkippedStreamIDs = %v, want [%s]", res.SkippedStreamIDs, logID)
	}
	if e.mainLog(t, logID) != nil {
		t.Error("main gained research about a person it deleted")
	}
	assertResumeNoop(t, e.resume(t, branch.ID, nil))
}

// TestResumeMerge_MainCascadesAnEditedArtifactAfterInterruption: the branch
// edits main's analysis of O; the replay stops before the edit lands, and main
// then deletes O, whose cascade removes the analysis with no event on its
// stream. The edit's stream is removed on main, so only "main" resolves it.
func TestResumeMerge_MainCascadesAnEditedArtifactAfterInterruption(t *testing.T) {
	e := newEvidenceResume(t)
	ctx := context.Background()
	subject := e.mainPerson(t, "Owen")
	analysis := e.analysis(t, e.f.handler, subject, "Born 1850")
	branch, scoped := e.branch(t, "edit-lost")
	e.renameOnBranch(t, scoped, domain.BranchID(branch.ID), e.person)
	conclusion := "Born 1851"
	if _, err := scoped.UpdateEvidenceAnalysis(ctx, command.UpdateEvidenceAnalysisInput{ID: analysis, Conclusion: &conclusion, Version: 1}); err != nil {
		t.Fatalf("branch UpdateEvidenceAnalysis failed: %v", err)
	}

	e.interrupt(t, branch, 2)
	e.deleteMainPerson(t, subject)

	result, err := e.f.handler.ResumeMerge(ctx, command.ResumeMergeInput{BranchID: branch.ID})
	if !errors.Is(err, command.ErrMergeResumeNeedsResolution) || !slices.Equal(result.PendingStreamIDs, []uuid.UUID{analysis}) {
		t.Fatalf("ResumeMerge = %v pending %v, want ErrMergeResumeNeedsResolution for [%s]", err, result.PendingStreamIDs, analysis)
	}
	if _, err := e.f.handler.ResumeMerge(ctx, command.ResumeMergeInput{
		BranchID: branch.ID, Resolutions: map[uuid.UUID]command.MergeResolution{analysis: command.ResolveBranch},
	}); !errors.Is(err, command.ErrUnknownResolution) {
		t.Fatalf("ResumeMerge(branch) error = %v, want ErrUnknownResolution (removed on main)", err)
	}
	e.resume(t, branch.ID, map[uuid.UUID]command.MergeResolution{analysis: command.ResolveMain})
	if e.mainAnalysis(t, analysis) != nil {
		t.Error("main resurrected an analysis its subject's delete cascaded")
	}
}

// TestResumeMerge_SubjectDeleteOntoLaterMainResearchIsPending: the branch
// deletes a person nothing is researched about; the replay stops before the
// delete lands, and main then logs research about that person. Replaying the
// delete would cascade the new log off main with no record, so the delete is
// pending, and "main" keeps both.
func TestResumeMerge_SubjectDeleteOntoLaterMainResearchIsPending(t *testing.T) {
	e := newEvidenceResume(t)
	ctx := context.Background()
	doomed := e.mainPerson(t, "Doomed")
	branch, scoped := e.branch(t, "tidy-persons")
	e.renameOnBranch(t, scoped, domain.BranchID(branch.ID), e.person)
	p, err := e.f.readStore.GetPerson(ctx, domain.BranchID(branch.ID), doomed)
	if err != nil || p == nil {
		t.Fatalf("GetPerson = %v, %v", p, err)
	}
	if err := scoped.DeletePerson(ctx, command.DeletePersonInput{ID: doomed, Version: p.Version, Reason: "duplicate"}); err != nil {
		t.Fatalf("branch DeletePerson failed: %v", err)
	}

	e.interrupt(t, branch, 2)
	logID := e.researchLog(t, e.f.handler, doomed, "Later archive")

	result, err := e.f.handler.ResumeMerge(ctx, command.ResumeMergeInput{BranchID: branch.ID})
	if !errors.Is(err, command.ErrMergeResumeNeedsResolution) {
		t.Fatalf("ResumeMerge error = %v, want ErrMergeResumeNeedsResolution", err)
	}
	if !slices.Equal(result.PendingStreamIDs, []uuid.UUID{doomed}) {
		t.Fatalf("PendingStreamIDs = %v, want [%s]", result.PendingStreamIDs, doomed)
	}
	if _, err := e.f.handler.ResumeMerge(ctx, command.ResumeMergeInput{
		BranchID: branch.ID, Resolutions: map[uuid.UUID]command.MergeResolution{doomed: command.ResolveBranch},
	}); !errors.Is(err, command.ErrMergeDanglingReference) {
		t.Fatalf("ResumeMerge(branch) error = %v, want ErrMergeDanglingReference", err)
	}
	e.resume(t, branch.ID, map[uuid.UUID]command.MergeResolution{doomed: command.ResolveMain})
	if e.mainLog(t, logID) == nil {
		t.Error("main lost its research log to a delete the resume rolled forward without")
	}
	if got, err := e.f.readStore.GetPerson(ctx, domain.MainBranchID, doomed); err != nil || got == nil {
		t.Errorf("main person = %v (err=%v), want kept", got, err)
	}
}

// TestResumeMerge_RepairsGPSProjection: the branch's research log lands in
// main's log but its projection fails. The resume re-projects it from main's
// own events without appending, and a second resume finds nothing to do.
func TestResumeMerge_RepairsGPSProjection(t *testing.T) {
	e := newEvidenceResume(t)
	branch, scoped := e.branch(t, "log-line")
	logID := e.researchLog(t, scoped, e.person, "Parish chest")

	e.failGPSProjection(t, branch, logID)
	if e.mainLog(t, logID) != nil {
		t.Fatalf("main has the log despite the failed projection")
	}
	before := e.mainEventCount(t)

	res := e.resume(t, branch.ID, nil)
	if res.ReplayedEventCount != 0 {
		t.Errorf("ReplayedEventCount = %d, want 0 (already in the log)", res.ReplayedEventCount)
	}
	if !slices.Equal(res.ReprojectedStreamIDs, []uuid.UUID{logID}) {
		t.Errorf("ReprojectedStreamIDs = %v, want [%s]", res.ReprojectedStreamIDs, logID)
	}
	if got := e.mainEventCount(t); got != before {
		t.Errorf("resume appended %d event(s), want none", got-before)
	}
	if got := e.mainLog(t, logID); got == nil || got.Repository != "Parish chest" {
		t.Errorf("main log after repair = %+v, want it re-projected", got)
	}
	assertResumeNoop(t, e.resume(t, branch.ID, nil))
}

// TestResumeMerge_CascadedGPSIsNotResurrected: a landed analysis whose
// projection failed, and a landed log whose projection succeeded, are both
// about O; main then deletes O. Both rows are gone for a reason main's log
// explains (the subject's cascade), so the resume re-projects neither.
func TestResumeMerge_CascadedGPSIsNotResurrected(t *testing.T) {
	e := newEvidenceResume(t)
	subject := e.mainPerson(t, "Owen")
	branch, scoped := e.branch(t, "research-then-cascade")
	logID := e.researchLog(t, scoped, subject, "County archive")
	analysis := e.analysis(t, scoped, subject, "Born 1850")

	e.failGPSProjection(t, branch, analysis)
	if e.mainLog(t, logID) == nil {
		t.Fatalf("main lacks the landed log")
	}
	e.deleteMainPerson(t, subject)

	res := e.resume(t, branch.ID, nil)
	if res.ReplayedEventCount != 0 || len(res.ReprojectedStreamIDs) != 0 {
		t.Errorf("resume replayed %d, re-projected %v; want nothing", res.ReplayedEventCount, res.ReprojectedStreamIDs)
	}
	if e.mainLog(t, logID) != nil || e.mainAnalysis(t, analysis) != nil {
		t.Error("main resurrected research whose subject it deleted")
	}
}

// TestResumeMerge_GPSOfMergedAwaySubjectIsRelinked: the analysis lands in
// main's log but its projection fails, and main then merges its subject into
// another person — and that one into a third. The person merges would have
// re-pointed the analysis to the final survivor, which is not in the
// analysis's stream, so the resume re-projects the analysis from main's log
// and then re-points it, exactly as the merges would have: version as the log
// has it, nothing appended, and a second resume does nothing.
func TestResumeMerge_GPSOfMergedAwaySubjectIsRelinked(t *testing.T) {
	e := newEvidenceResume(t)
	ctx := context.Background()
	subject := e.mainPerson(t, "Owen")
	final := e.mainPerson(t, "Fay")
	branch, scoped := e.branch(t, "research-then-merge")
	analysis := e.analysis(t, scoped, subject, "Born 1850")

	e.failGPSProjection(t, branch, analysis)
	e.mergeMainPersons(t, e.person, subject)
	e.mergeMainPersons(t, final, e.person)
	before := e.mainEventCount(t)

	res := e.resume(t, branch.ID, nil)
	if res.ReplayedEventCount != 0 || !slices.Equal(res.ReprojectedStreamIDs, []uuid.UUID{analysis}) {
		t.Errorf("resume replayed %d, re-projected %v; want 0 and [%s]", res.ReplayedEventCount, res.ReprojectedStreamIDs, analysis)
	}
	if got := e.mainEventCount(t); got != before {
		t.Errorf("main event count = %d, want %d (a repair appends nothing)", got, before)
	}
	row := e.mainAnalysis(t, analysis)
	if row == nil {
		t.Fatalf("main has no row for the repaired analysis")
	}
	if row.SubjectID != final {
		t.Errorf("repaired analysis is about %s, want the final merge survivor %s", row.SubjectID, final)
	}
	head, err := e.f.eventStore.GetStreamVersion(ctx, analysis, domain.MainBranchID)
	if err != nil {
		t.Fatalf("GetStreamVersion failed: %v", err)
	}
	if row.Version != head {
		t.Errorf("repaired analysis version = %d, want main's stream version %d", row.Version, head)
	}
	listed, err := e.f.readStore.GetAnalysesBySubject(ctx, domain.MainBranchID, final)
	if err != nil || len(listed) != 1 || listed[0].ID != analysis {
		t.Errorf("survivor's analyses = %v, %v; want the repaired one", listed, err)
	}
	assertResumeNoop(t, e.resume(t, branch.ID, nil))
}

// TestResumeMerge_GPSOfMergedThenDeletedSubjectStaysGone: as above, but main
// deletes the survivor before the resume, so the analysis is gone either way
// (the survivor's delete cascade) and nothing is re-projected.
func TestResumeMerge_GPSOfMergedThenDeletedSubjectStaysGone(t *testing.T) {
	e := newEvidenceResume(t)
	subject := e.mainPerson(t, "Owen")
	branch, scoped := e.branch(t, "research-merge-delete")
	analysis := e.analysis(t, scoped, subject, "Born 1850")

	e.failGPSProjection(t, branch, analysis)
	e.mergeMainPersons(t, e.person, subject)
	e.deleteMainPerson(t, e.person)

	res := e.resume(t, branch.ID, nil)
	if len(res.ReprojectedStreamIDs) != 0 || e.mainAnalysis(t, analysis) != nil {
		t.Errorf("resume re-projected %v; want the cascaded analysis left gone", res.ReprojectedStreamIDs)
	}
}

// TestResumeMerge_MergedAwaySubjectResolvedToMainKeepsRelinkedGPS: the branch
// also edits the log's subject, and main merges that subject away during the
// interruption, so the subject's edit is pending and can only be resolved to
// main. That does not orphan the landed log: its repair re-points it to the
// survivor main still has.
func TestResumeMerge_MergedAwaySubjectResolvedToMainKeepsRelinkedGPS(t *testing.T) {
	e := newEvidenceResume(t)
	subject := e.mainPerson(t, "Owen")
	branch, scoped := e.branch(t, "research-and-edit-then-merge")
	logID := e.researchLog(t, scoped, subject, "Parish chest")
	e.renameOnBranch(t, scoped, domain.BranchID(branch.ID), subject)

	e.failGPSProjection(t, branch, logID)
	e.mergeMainPersons(t, e.person, subject)

	res := e.resume(t, branch.ID, map[uuid.UUID]command.MergeResolution{subject: command.ResolveMain})
	if !slices.Contains(res.ReprojectedStreamIDs, logID) {
		t.Errorf("resume re-projected %v; want the log %s", res.ReprojectedStreamIDs, logID)
	}
	if row := e.mainLog(t, logID); row == nil || row.SubjectID != e.person {
		t.Fatalf("repaired log = %+v, want it about the survivor %s", row, e.person)
	}
	assertResumeNoop(t, e.resume(t, branch.ID, nil))
}

// TestResumeMerge_LegacyClaimRefusesSkippingSubjectCreateDeleteUnderLandedGPS:
// the branch creates a person, logs research about them and deletes them (the
// delete cascades the log on the branch). A pre-#685 claim lands the log and
// leaves the person's create+delete stream to the caller. "main" would skip
// the delete and leave main's log about a person main never had, so it is
// refused; "branch" replays the delete, which cascades the log away.
func TestResumeMerge_LegacyClaimRefusesSkippingSubjectCreateDeleteUnderLandedGPS(t *testing.T) {
	e := newEvidenceResume(t)
	ctx := context.Background()
	branch, scoped := e.branch(t, "short-lived-lead")
	created, err := scoped.CreatePerson(ctx, command.CreatePersonInput{GivenName: "Brief", Surname: "Lead"})
	if err != nil {
		t.Fatalf("CreatePerson failed: %v", err)
	}
	logID := e.researchLog(t, scoped, created.ID, "Parish chest")
	e.deletePersonOnBranch(t, scoped, branch, created.ID)
	e.legacyClaimWithLanded(t, branch, logID)

	res, err := e.f.handler.ResumeMerge(ctx, command.ResumeMergeInput{BranchID: branch.ID})
	if !errors.Is(err, command.ErrMergeResumeNeedsResolution) {
		t.Fatalf("bare ResumeMerge error = %v, want ErrMergeResumeNeedsResolution", err)
	}
	if res == nil || !slices.Equal(res.PendingStreamIDs, []uuid.UUID{created.ID}) {
		t.Fatalf("PendingStreamIDs = %v, want [%s]", res, created.ID)
	}
	e.refuseMainResolution(t, branch, created.ID)

	done := e.resume(t, branch.ID, map[uuid.UUID]command.MergeResolution{created.ID: command.ResolveBranch})
	if done.ReplayedEventCount == 0 {
		t.Errorf("ReplayedEventCount = 0, want the person's stream replayed")
	}
	if p, err := e.f.readStore.GetPerson(ctx, domain.MainBranchID, created.ID); err != nil || p != nil {
		t.Errorf("main person = %v (err=%v), want absent after its delete replayed", p, err)
	}
	if e.mainLog(t, logID) != nil {
		t.Errorf("main still has the log after its subject's delete replayed")
	}
	assertResumeNoop(t, e.resume(t, branch.ID, nil))
}

// TestResumeMerge_LegacyClaimRefusesExcludingALandedLogsSubject: a pre-#685
// claim with a research log about a branch-created person replayed by hand.
// Excluding the person ("main") would orphan the landed log, so it is refused;
// "branch" lands the person.
func TestResumeMerge_LegacyClaimRefusesExcludingALandedLogsSubject(t *testing.T) {
	e := newEvidenceResume(t)
	f := e.f
	ctx := context.Background()
	branch, scoped := e.branch(t, "legacy-gps")
	created, err := scoped.CreatePerson(ctx, command.CreatePersonInput{GivenName: "New", Surname: "Lead"})
	if err != nil {
		t.Fatalf("CreatePerson failed: %v", err)
	}
	logID := e.researchLog(t, scoped, created.ID, "Parish chest")

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
	var logEvents []domain.Event
	for _, stored := range branchEventsFor(t, f, logID, domain.BranchID(branch.ID)) {
		decoded, err := stored.DecodeEvent()
		if err != nil {
			t.Fatalf("DecodeEvent failed: %v", err)
		}
		logEvents = append(logEvents, decoded)
	}
	if err := f.eventStore.Append(ctx, logID, "ResearchLog", logEvents, 0, repository.MainScope); err != nil {
		t.Fatalf("replaying the log by hand failed: %v", err)
	}

	result, err := f.handler.ResumeMerge(ctx, command.ResumeMergeInput{BranchID: branch.ID})
	if !errors.Is(err, command.ErrMergeResumeNeedsResolution) || !slices.Equal(result.PendingStreamIDs, []uuid.UUID{created.ID}) {
		t.Fatalf("bare ResumeMerge = %v pending %v, want ErrMergeResumeNeedsResolution for [%s]", err, result.PendingStreamIDs, created.ID)
	}
	if _, err := f.handler.ResumeMerge(ctx, command.ResumeMergeInput{
		BranchID: branch.ID, Resolutions: map[uuid.UUID]command.MergeResolution{created.ID: command.ResolveMain},
	}); !errors.Is(err, command.ErrMergeDanglingReference) {
		t.Fatalf("ResumeMerge(main) error = %v, want ErrMergeDanglingReference", err)
	}
	done := e.resume(t, branch.ID, map[uuid.UUID]command.MergeResolution{created.ID: command.ResolveBranch})
	if done.ReplayedEventCount != 1 {
		t.Errorf("ReplayedEventCount = %d, want 1 (the person)", done.ReplayedEventCount)
	}
	if got := e.mainLog(t, logID); got == nil || got.SubjectID != created.ID {
		t.Errorf("main log = %+v, want it about the landed person", got)
	}
}

// deletePersonOnBranch deletes a person on a branch at its current version.
func (e evidenceResume) deletePersonOnBranch(t *testing.T, h *command.Handler, branch *domain.Branch, id uuid.UUID) {
	t.Helper()
	ctx := context.Background()
	p, err := e.f.readStore.GetPerson(ctx, domain.BranchID(branch.ID), id)
	if err != nil || p == nil {
		t.Fatalf("GetPerson = %v, %v", p, err)
	}
	if err := h.DeletePerson(ctx, command.DeletePersonInput{ID: id, Version: p.Version, Reason: "duplicate"}); err != nil {
		t.Fatalf("branch DeletePerson failed: %v", err)
	}
}

// assertSubjectDeletePending resumes with no decisions and expects exactly the
// subject's delete to be pending; "branch" for it is then refused without
// writing, and "main" finishes the resume keeping the subject.
func (e evidenceResume) assertSubjectDeletePending(t *testing.T, branch *domain.Branch, subject uuid.UUID) {
	t.Helper()
	ctx := context.Background()
	before := e.mainEventCount(t)
	result, err := e.f.handler.ResumeMerge(ctx, command.ResumeMergeInput{BranchID: branch.ID})
	if !errors.Is(err, command.ErrMergeResumeNeedsResolution) {
		t.Fatalf("ResumeMerge error = %v, want ErrMergeResumeNeedsResolution", err)
	}
	if result == nil || !slices.Equal(result.PendingStreamIDs, []uuid.UUID{subject}) {
		t.Fatalf("PendingStreamIDs = %v, want [%s]", result, subject)
	}
	if _, err := e.f.handler.ResumeMerge(ctx, command.ResumeMergeInput{
		BranchID: branch.ID, Resolutions: map[uuid.UUID]command.MergeResolution{subject: command.ResolveBranch},
	}); !errors.Is(err, command.ErrMergeDanglingReference) {
		t.Fatalf("ResumeMerge(branch) error = %v, want ErrMergeDanglingReference", err)
	}
	if got := e.mainEventCount(t); got != before {
		t.Fatalf("refused resumes wrote %d event(s) to main", got-before)
	}
	e.resume(t, branch.ID, map[uuid.UUID]command.MergeResolution{subject: command.ResolveMain})
	if p, err := e.f.readStore.GetPerson(ctx, domain.MainBranchID, subject); err != nil || p == nil {
		t.Errorf("main person = %v (err=%v), want kept", p, err)
	}
	assertResumeNoop(t, e.resume(t, branch.ID, nil))
}

// TestResumeMerge_SubjectDeleteOntoMainEditOfLandedArtifactIsPending: the
// branch edits main's evidence analysis about a person and then deletes the
// person. The merge stops after the analysis edit lands, and main then edits
// the analysis again. The landed stream is never conflict-checked again, so
// replaying the delete would cascade main's post-landing edit away with no
// record: the delete is pending, and "main" keeps the analysis with main's
// edit.
func TestResumeMerge_SubjectDeleteOntoMainEditOfLandedArtifactIsPending(t *testing.T) {
	e := newEvidenceResume(t)
	ctx := context.Background()
	subject := e.mainPerson(t, "Owen")
	analysis := e.analysis(t, e.f.handler, subject, "Born 1850")
	branch, scoped := e.branch(t, "tidy-owen")
	branchConclusion := "Born 1851"
	if _, err := scoped.UpdateEvidenceAnalysis(ctx, command.UpdateEvidenceAnalysisInput{
		ID: analysis, Conclusion: &branchConclusion, Version: 1,
	}); err != nil {
		t.Fatalf("branch UpdateEvidenceAnalysis failed: %v", err)
	}
	e.deletePersonOnBranch(t, scoped, branch, subject)

	e.interrupt(t, branch, 2)
	landed := e.mainAnalysis(t, analysis)
	if landed == nil || landed.Conclusion != branchConclusion {
		t.Fatalf("main analysis after interruption = %+v, want the branch's edit landed", landed)
	}
	mainConclusion := "Born 1852, per the parish register"
	if _, err := e.f.handler.UpdateEvidenceAnalysis(ctx, command.UpdateEvidenceAnalysisInput{
		ID: analysis, Conclusion: &mainConclusion, Version: landed.Version,
	}); err != nil {
		t.Fatalf("main UpdateEvidenceAnalysis failed: %v", err)
	}

	e.assertSubjectDeletePending(t, branch, subject)
	if got := e.mainAnalysis(t, analysis); got == nil || got.Conclusion != mainConclusion {
		t.Errorf("main analysis = %+v, want main's post-landing edit kept", got)
	}
}

// TestResumeMerge_SubjectDeleteOntoMainEditOfLandedLogIsPending: the branch
// creates a research log about a main person and then deletes the person. The
// log lands before the interruption; main then adds notes to it. Replaying the
// delete would cascade main's notes away, so it is pending.
func TestResumeMerge_SubjectDeleteOntoMainEditOfLandedLogIsPending(t *testing.T) {
	e := newEvidenceResume(t)
	ctx := context.Background()
	doomed := e.mainPerson(t, "Doomed")
	branch, scoped := e.branch(t, "tidy-doomed")
	logID := e.researchLog(t, scoped, doomed, "County archive")
	e.deletePersonOnBranch(t, scoped, branch, doomed)

	e.interrupt(t, branch, 2)
	landed := e.mainLog(t, logID)
	if landed == nil {
		t.Fatal("main has no research log after interruption, want the branch's log landed")
	}
	notes := "Checked the index too"
	if _, err := e.f.handler.UpdateResearchLog(ctx, command.UpdateResearchLogInput{
		ID: logID, Notes: &notes, Version: landed.Version,
	}); err != nil {
		t.Fatalf("main UpdateResearchLog failed: %v", err)
	}

	e.assertSubjectDeletePending(t, branch, doomed)
	if got := e.mainLog(t, logID); got == nil || got.Notes != notes {
		t.Errorf("main research log = %+v, want main's notes kept", got)
	}
}

// TestResumeMerge_SubjectDeleteCascadesUnchangedLandedArtifact: as above, but
// main leaves the landed analysis alone. The branch saw everything the
// cascade removes, so the resume replays the delete and the analysis goes
// with the person, as on the branch.
func TestResumeMerge_SubjectDeleteCascadesUnchangedLandedArtifact(t *testing.T) {
	e := newEvidenceResume(t)
	ctx := context.Background()
	subject := e.mainPerson(t, "Owen")
	analysis := e.analysis(t, e.f.handler, subject, "Born 1850")
	branch, scoped := e.branch(t, "tidy-owen")
	conclusion := "Born 1851"
	if _, err := scoped.UpdateEvidenceAnalysis(ctx, command.UpdateEvidenceAnalysisInput{
		ID: analysis, Conclusion: &conclusion, Version: 1,
	}); err != nil {
		t.Fatalf("branch UpdateEvidenceAnalysis failed: %v", err)
	}
	e.deletePersonOnBranch(t, scoped, branch, subject)

	e.interrupt(t, branch, 2)
	if e.mainAnalysis(t, analysis) == nil {
		t.Fatal("main has no analysis after interruption, want the branch's edit landed")
	}
	done := e.resume(t, branch.ID, nil)
	if len(done.PendingStreamIDs) != 0 {
		t.Fatalf("PendingStreamIDs = %v, want none", done.PendingStreamIDs)
	}
	if got := e.mainAnalysis(t, analysis); got != nil {
		t.Errorf("main analysis = %+v, want cascaded with its subject as on the branch", got)
	}
	if p, err := e.f.readStore.GetPerson(ctx, domain.MainBranchID, subject); err != nil || p != nil {
		t.Errorf("main person = %v (err=%v), want deleted", p, err)
	}
}
