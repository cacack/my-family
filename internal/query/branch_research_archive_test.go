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
