package query_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/cacack/my-family/internal/command"
	"github.com/cacack/my-family/internal/domain"
	"github.com/cacack/my-family/internal/query"
	"github.com/cacack/my-family/internal/repository"
	"github.com/cacack/my-family/internal/repository/memory"
)

type archiveFixture struct {
	events   *memory.EventStore
	reads    *memory.ReadModelStore
	branches *memory.BranchStore
	handler  *command.Handler
	service  *query.BranchService
}

func newArchiveFixture() *archiveFixture {
	events := memory.NewEventStore()
	reads := memory.NewReadModelStore()
	branches := memory.NewBranchStore()
	return &archiveFixture{
		events: events, reads: reads, branches: branches,
		handler: command.NewHandlerWithBranches(events, reads, branches, memory.NewSnapshotStore(events)),
		service: query.NewBranchService(branches, events, query.NewHistoryService(events, reads)),
	}
}

// mustOK unwraps a (value, error) pair; a fixture step that fails panics,
// which fails the test with the error.
func mustOK[T any](v T, err error) T {
	if err != nil {
		panic(err)
	}
	return v
}

// TestBranchResearchArchive_RebuildsFromEvents closes a branch that created,
// edited and deleted GPS artifacts and reads them back from the events.
func TestBranchResearchArchive_RebuildsFromEvents(t *testing.T) {
	ctx := context.Background()
	f := newArchiveFixture()
	h := f.handler

	person := mustOK(h.CreatePerson(ctx, command.CreatePersonInput{GivenName: "Ada", Surname: "Placeholder"}))
	family := mustOK(h.CreateFamily(ctx, command.CreateFamilyInput{Partner1ID: &person.ID}))
	mainAnalysis := mustOK(h.CreateEvidenceAnalysis(ctx, command.CreateEvidenceAnalysisInput{
		FactType: string(domain.FactPersonBirth), SubjectID: person.ID, Conclusion: "Born 1815",
	}))
	mainProof := mustOK(h.CreateProofSummary(ctx, command.CreateProofSummaryInput{
		FactType: string(domain.FactPersonBirth), SubjectID: person.ID, Conclusion: "Born 1815", Argument: "Agreed",
	}))

	notes := "Main note before the fork"
	beforeFork := mustOK(h.UpdateEvidenceAnalysis(ctx, command.UpdateEvidenceAnalysisInput{ID: mainAnalysis.ID, Notes: &notes, Version: mainAnalysis.Version}))

	branch := mustOK(h.CreateBranch(ctx, "theory", ""))
	onBranch := h.WithBranch(branch)

	// The branch edits main's analysis; a mainline edit after that is not in
	// the branch's copy.
	conclusion := "Born 1815, per the branch"
	mustOK(onBranch.UpdateEvidenceAnalysis(ctx, command.UpdateEvidenceAnalysisInput{ID: mainAnalysis.ID, Conclusion: &conclusion, Version: beforeFork.Version}))
	late := "Main note after the branch edit"
	mustOK(h.UpdateEvidenceAnalysis(ctx, command.UpdateEvidenceAnalysisInput{ID: mainAnalysis.ID, Notes: &late, Version: beforeFork.Version}))

	// The branch deletes main's proof summary and records its own about a
	// branch-only family.
	if err := onBranch.DeleteProofSummary(ctx, mainProof.ID, mainProof.Version, "wrong"); err != nil {
		t.Fatalf("DeleteProofSummary: %v", err)
	}
	branchFamily := mustOK(onBranch.CreateFamily(ctx, command.CreateFamilyInput{Partner1ID: &person.ID}))
	branchProof := mustOK(onBranch.CreateProofSummary(ctx, command.CreateProofSummaryInput{
		FactType: string(domain.FactPersonBirth), SubjectID: branchFamily.ID, Conclusion: "No link", Argument: "None found",
	}))
	familyLog := mustOK(onBranch.CreateResearchLog(ctx, command.CreateResearchLogInput{
		SubjectID: family.ID, SubjectType: "family", Repository: "Parish", SearchDescription: "Marriages",
		Outcome: string(domain.ResearchOutcomeNotFound), SearchDate: time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC),
	}))

	if err := h.CloseBranch(ctx, branch.ID, domain.BranchOutcomeDisproved, "reason"); err != nil {
		t.Fatalf("CloseBranch: %v", err)
	}

	archive, err := f.service.BranchResearchArchive(ctx, branch.ID)
	if err != nil {
		t.Fatalf("BranchResearchArchive: %v", err)
	}
	if archive.Branch.Outcome != domain.BranchOutcomeDisproved || archive.Truncated || archive.DeletedCount != 1 {
		t.Errorf("archive header = outcome %q truncated %v deleted %d", archive.Branch.Outcome, archive.Truncated, archive.DeletedCount)
	}

	if len(archive.EvidenceAnalyses) != 1 {
		t.Fatalf("analyses = %d, want 1", len(archive.EvidenceAnalyses))
	}
	analysis := archive.EvidenceAnalyses[0]
	if analysis.CreatedOnBranch || analysis.Conclusion != conclusion || analysis.Notes == nil || *analysis.Notes != notes {
		t.Errorf("archived analysis = %+v (notes %v), want the branch conclusion over the pre-fork main note", analysis, analysis.Notes)
	}
	if analysis.SubjectName != "Ada Placeholder" {
		t.Errorf("analysis subject name = %q", analysis.SubjectName)
	}

	if len(archive.ProofSummaries) != 1 || archive.ProofSummaries[0].ID != branchProof.ID || !archive.ProofSummaries[0].CreatedOnBranch {
		t.Fatalf("proof summaries = %+v, want only the branch's own", archive.ProofSummaries)
	}
	if archive.ProofSummaries[0].SubjectName != "Family created on this branch" {
		t.Errorf("branch-only family subject name = %q", archive.ProofSummaries[0].SubjectName)
	}

	if len(archive.ResearchLogs) != 1 || archive.ResearchLogs[0].ID != familyLog.ID {
		t.Fatalf("research logs = %+v", archive.ResearchLogs)
	}
	if archive.ResearchLogs[0].SubjectName == "" || archive.ResearchLogs[0].Outcome != "not_found" {
		t.Errorf("family research log = %+v", archive.ResearchLogs[0])
	}

	// Without a history service subjects are simply unnamed from main.
	bare := query.NewBranchService(f.branches, f.events, nil)
	unnamed, err := bare.BranchResearchArchive(ctx, branch.ID)
	if err != nil {
		t.Fatalf("BranchResearchArchive without history: %v", err)
	}
	if unnamed.EvidenceAnalyses[0].SubjectName != "" || unnamed.ProofSummaries[0].SubjectName == "" {
		t.Errorf("names without history = %q / %q", unnamed.EvidenceAnalyses[0].SubjectName, unnamed.ProofSummaries[0].SubjectName)
	}

	if _, err := f.service.BranchResearchArchive(ctx, uuid.New()); !errors.Is(err, repository.ErrBranchNotFound) {
		t.Errorf("unknown branch = %v, want ErrBranchNotFound", err)
	}
}

func TestBranchResearchArchive_EmptyBranch(t *testing.T) {
	ctx := context.Background()
	f := newArchiveFixture()
	branch := mustOK(f.handler.CreateBranch(ctx, "empty", ""))
	archive, err := f.service.BranchResearchArchive(ctx, branch.ID)
	if err != nil {
		t.Fatalf("BranchResearchArchive: %v", err)
	}
	if len(archive.ResearchLogs)+len(archive.EvidenceAnalyses)+len(archive.ProofSummaries) != 0 || archive.DeletedCount != 0 {
		t.Errorf("empty branch archive = %+v", archive)
	}
}

// TestBranchResearchArchive_KeepsSubjectAcrossBranchPersonMerge pins the
// archive's deliberate divergence from the live overlay after a person merge
// on the branch (#834): the overlay re-points a GPS artifact's subject to the
// survivor without an event on the artifact's stream, but the archive keeps
// the subject the artifact was recorded about. A closed branch's person merge
// is part of the hypothesis it did not establish, so the archive — and a
// promotion from it — must not assert that identity on main.
func TestBranchResearchArchive_KeepsSubjectAcrossBranchPersonMerge(t *testing.T) {
	ctx := context.Background()
	f := newArchiveFixture()
	h := f.handler

	survivor := mustOK(h.CreatePerson(ctx, command.CreatePersonInput{GivenName: "Survivor", Surname: "Kept"}))
	merged := mustOK(h.CreatePerson(ctx, command.CreatePersonInput{GivenName: "MergedAway", Surname: "Duplicate"}))

	branch := mustOK(h.CreateBranch(ctx, "same-person", "Survivor and MergedAway are one person"))
	onBranch := h.WithBranch(branch)

	branchOnly := mustOK(onBranch.CreatePerson(ctx, command.CreatePersonInput{GivenName: "BranchOnly", Surname: "Duplicate"}))
	searchDate := time.Date(2024, 3, 1, 0, 0, 0, 0, time.UTC)
	mainSubjectLog := mustOK(onBranch.CreateResearchLog(ctx, command.CreateResearchLogInput{
		SubjectID: merged.ID, SubjectType: "person", Repository: "Parish", SearchDescription: "Baptisms",
		Outcome: string(domain.ResearchOutcomeNotFound), SearchDate: searchDate,
	}))
	branchSubjectLog := mustOK(onBranch.CreateResearchLog(ctx, command.CreateResearchLogInput{
		SubjectID: branchOnly.ID, SubjectType: "person", Repository: "Census", SearchDescription: "1850",
		Outcome: string(domain.ResearchOutcomeNotFound), SearchDate: searchDate,
	}))

	s := mustOK(onBranch.MergePersons(ctx, command.MergePersonsInput{
		SurvivorID: survivor.ID, MergedID: merged.ID, SurvivorVersion: survivor.Version, MergedVersion: merged.Version,
	}))
	mustOK(onBranch.MergePersons(ctx, command.MergePersonsInput{
		SurvivorID: survivor.ID, MergedID: branchOnly.ID, SurvivorVersion: s.Version, MergedVersion: branchOnly.Version,
	}))

	// The live overlay follows the merges.
	scope := domain.BranchID(branch.ID)
	for _, id := range []uuid.UUID{mainSubjectLog.ID, branchSubjectLog.ID} {
		live := mustOK(f.reads.GetResearchLog(ctx, scope, id))
		if live == nil || live.SubjectID != survivor.ID {
			t.Fatalf("overlay research log %s = %+v, want subject re-pointed to the survivor", id, live)
		}
	}

	if err := h.CloseBranch(ctx, branch.ID, domain.BranchOutcomeDisproved, "different parents"); err != nil {
		t.Fatalf("CloseBranch: %v", err)
	}

	archive, err := f.service.BranchResearchArchive(ctx, branch.ID)
	if err != nil {
		t.Fatalf("BranchResearchArchive: %v", err)
	}
	subjects := map[uuid.UUID]query.ArchivedResearchLog{}
	for _, entry := range archive.ResearchLogs {
		subjects[entry.ID] = entry
	}
	if got := subjects[mainSubjectLog.ID]; got.SubjectID != merged.ID || got.SubjectName != "MergedAway Duplicate" {
		t.Errorf("archived log about a mainline person = subject %s %q, want the recorded subject %s", got.SubjectID, got.SubjectName, merged.ID)
	}
	if got := subjects[branchSubjectLog.ID]; got.SubjectID != branchOnly.ID || got.SubjectName != "BranchOnly Duplicate" {
		t.Errorf("archived log about a branch-only person = subject %s %q, want the recorded subject %s", got.SubjectID, got.SubjectName, branchOnly.ID)
	}

	// Promotion keeps the recorded subject: the log about the mainline person
	// lands on them (main never merged them away), the one about the
	// branch-only person has no subject on main.
	result, err := h.PromoteBranchResearchLogs(ctx, branch.ID, nil)
	if err != nil {
		t.Fatalf("PromoteBranchResearchLogs: %v", err)
	}
	if len(result.Promoted) != 1 || result.Promoted[0] != mainSubjectLog.ID {
		t.Errorf("promoted = %v, want only %s", result.Promoted, mainSubjectLog.ID)
	}
	if len(result.Skipped) != 1 || result.Skipped[0].ID != branchSubjectLog.ID || result.Skipped[0].Reason != command.PromoteSkipSubjectNotOnMain {
		t.Errorf("skipped = %+v, want %s as %s", result.Skipped, branchSubjectLog.ID, command.PromoteSkipSubjectNotOnMain)
	}
	promoted := mustOK(f.reads.GetResearchLog(ctx, domain.MainBranchID, mainSubjectLog.ID))
	if promoted == nil || promoted.SubjectID != merged.ID {
		t.Errorf("promoted log on main = %+v, want subject %s", promoted, merged.ID)
	}
}
