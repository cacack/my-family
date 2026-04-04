package query_test

import (
	"context"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/cacack/my-family/internal/command"
	"github.com/cacack/my-family/internal/query"
	"github.com/cacack/my-family/internal/repository/memory"
)

// --- EvidenceAnalysis Query Tests ---

func TestListEvidenceAnalyses(t *testing.T) {
	eventStore := memory.NewEventStore()
	readStore := memory.NewReadModelStore()
	cmdHandler := command.NewHandler(eventStore, readStore)
	queryService := query.NewEvidenceQueryService(readStore)
	ctx := context.Background()

	// Create test analyses
	for i := 0; i < 3; i++ {
		_, err := cmdHandler.CreateEvidenceAnalysis(ctx, command.CreateEvidenceAnalysisInput{
			FactType:   "person_birth",
			SubjectID:  uuid.New(),
			Conclusion: "Test conclusion",
		})
		if err != nil {
			t.Fatalf("Failed to create analysis: %v", err)
		}
	}

	tests := []struct {
		name      string
		input     query.ListInput
		wantCount int
		wantTotal int
	}{
		{
			name:      "list all",
			input:     query.ListInput{Limit: 10},
			wantCount: 3,
			wantTotal: 3,
		},
		{
			name:      "with pagination",
			input:     query.ListInput{Limit: 2, Offset: 0},
			wantCount: 2,
			wantTotal: 3,
		},
		{
			name:      "second page",
			input:     query.ListInput{Limit: 2, Offset: 2},
			wantCount: 1,
			wantTotal: 3,
		},
		{
			name:      "default limit",
			input:     query.ListInput{Limit: 0},
			wantCount: 3,
			wantTotal: 3,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := queryService.ListEvidenceAnalyses(ctx, tt.input)
			if err != nil {
				t.Fatalf("ListEvidenceAnalyses failed: %v", err)
			}

			if len(result.Analyses) != tt.wantCount {
				t.Errorf("Got %d analyses, want %d", len(result.Analyses), tt.wantCount)
			}
			if result.Total != tt.wantTotal {
				t.Errorf("Total = %d, want %d", result.Total, tt.wantTotal)
			}
		})
	}
}

func TestGetEvidenceAnalysis(t *testing.T) {
	eventStore := memory.NewEventStore()
	readStore := memory.NewReadModelStore()
	cmdHandler := command.NewHandler(eventStore, readStore)
	queryService := query.NewEvidenceQueryService(readStore)
	ctx := context.Background()

	createResult, err := cmdHandler.CreateEvidenceAnalysis(ctx, command.CreateEvidenceAnalysisInput{
		FactType:       "person_birth",
		SubjectID:      uuid.New(),
		Conclusion:     "Born in 1850",
		ResearchStatus: "probable",
		Notes:          "Census evidence",
	})
	if err != nil {
		t.Fatalf("CreateEvidenceAnalysis failed: %v", err)
	}

	result, err := queryService.GetEvidenceAnalysis(ctx, createResult.ID)
	if err != nil {
		t.Fatalf("GetEvidenceAnalysis failed: %v", err)
	}

	if result.ID != createResult.ID {
		t.Errorf("ID = %v, want %v", result.ID, createResult.ID)
	}
	if result.Conclusion != "Born in 1850" {
		t.Errorf("Conclusion = %s, want 'Born in 1850'", result.Conclusion)
	}
	if result.FactType != "person_birth" {
		t.Errorf("FactType = %s, want person_birth", result.FactType)
	}
	if result.ResearchStatus == nil || *result.ResearchStatus != "probable" {
		t.Error("ResearchStatus not set correctly")
	}
	if result.Notes == nil || *result.Notes != "Census evidence" {
		t.Error("Notes not set correctly")
	}
}

func TestGetEvidenceAnalysis_NotFound(t *testing.T) {
	readStore := memory.NewReadModelStore()
	queryService := query.NewEvidenceQueryService(readStore)
	ctx := context.Background()

	_, err := queryService.GetEvidenceAnalysis(ctx, uuid.New())
	if err != query.ErrNotFound {
		t.Errorf("Expected ErrNotFound, got %v", err)
	}
}

func TestGetAnalysesForFact(t *testing.T) {
	eventStore := memory.NewEventStore()
	readStore := memory.NewReadModelStore()
	cmdHandler := command.NewHandler(eventStore, readStore)
	queryService := query.NewEvidenceQueryService(readStore)
	ctx := context.Background()

	subjectID := uuid.New()

	// Create analyses for same fact
	_, _ = cmdHandler.CreateEvidenceAnalysis(ctx, command.CreateEvidenceAnalysisInput{
		FactType:   "person_birth",
		SubjectID:  subjectID,
		Conclusion: "Born in 1850",
	})
	_, _ = cmdHandler.CreateEvidenceAnalysis(ctx, command.CreateEvidenceAnalysisInput{
		FactType:   "person_birth",
		SubjectID:  subjectID,
		Conclusion: "Born in 1850",
	})

	// Create analysis for different fact type
	_, _ = cmdHandler.CreateEvidenceAnalysis(ctx, command.CreateEvidenceAnalysisInput{
		FactType:   "person_death",
		SubjectID:  subjectID,
		Conclusion: "Died in 1920",
	})

	// Create analysis for different subject
	_, _ = cmdHandler.CreateEvidenceAnalysis(ctx, command.CreateEvidenceAnalysisInput{
		FactType:   "person_birth",
		SubjectID:  uuid.New(),
		Conclusion: "Born in 1860",
	})

	results, err := queryService.GetAnalysesForFact(ctx, "person_birth", subjectID)
	if err != nil {
		t.Fatalf("GetAnalysesForFact failed: %v", err)
	}

	if len(results) != 2 {
		t.Errorf("Got %d results, want 2", len(results))
	}

	for _, r := range results {
		if r.FactType != "person_birth" {
			t.Errorf("FactType = %s, want person_birth", r.FactType)
		}
		if r.SubjectID != subjectID {
			t.Errorf("SubjectID = %v, want %v", r.SubjectID, subjectID)
		}
	}
}

// --- EvidenceConflict Query Tests ---

func TestGetEvidenceConflict(t *testing.T) {
	eventStore := memory.NewEventStore()
	readStore := memory.NewReadModelStore()
	cmdHandler := command.NewHandler(eventStore, readStore)
	queryService := query.NewEvidenceQueryService(readStore)
	ctx := context.Background()

	subjectID := uuid.New()

	// Create conflicting analyses
	_, _ = cmdHandler.CreateEvidenceAnalysis(ctx, command.CreateEvidenceAnalysisInput{
		FactType:   "person_birth",
		SubjectID:  subjectID,
		Conclusion: "Born in 1850",
	})
	result2, _ := cmdHandler.CreateEvidenceAnalysis(ctx, command.CreateEvidenceAnalysisInput{
		FactType:   "person_birth",
		SubjectID:  subjectID,
		Conclusion: "Born in 1852",
	})

	if result2.ConflictID == nil {
		t.Fatal("Expected conflict to be auto-detected")
	}

	conflict, err := queryService.GetEvidenceConflict(ctx, *result2.ConflictID)
	if err != nil {
		t.Fatalf("GetEvidenceConflict failed: %v", err)
	}

	if conflict.ID != *result2.ConflictID {
		t.Errorf("ID mismatch")
	}
	if conflict.Status != "open" {
		t.Errorf("Status = %s, want open", conflict.Status)
	}
	if conflict.SubjectID != subjectID {
		t.Errorf("SubjectID mismatch")
	}
}

func TestGetEvidenceConflict_NotFound(t *testing.T) {
	readStore := memory.NewReadModelStore()
	queryService := query.NewEvidenceQueryService(readStore)
	ctx := context.Background()

	_, err := queryService.GetEvidenceConflict(ctx, uuid.New())
	if err != query.ErrNotFound {
		t.Errorf("Expected ErrNotFound, got %v", err)
	}
}

func TestListEvidenceConflicts(t *testing.T) {
	eventStore := memory.NewEventStore()
	readStore := memory.NewReadModelStore()
	cmdHandler := command.NewHandler(eventStore, readStore)
	queryService := query.NewEvidenceQueryService(readStore)
	ctx := context.Background()

	// Create two sets of conflicting analyses
	subjectID1 := uuid.New()
	_, _ = cmdHandler.CreateEvidenceAnalysis(ctx, command.CreateEvidenceAnalysisInput{
		FactType: "person_birth", SubjectID: subjectID1, Conclusion: "Born 1850",
	})
	_, _ = cmdHandler.CreateEvidenceAnalysis(ctx, command.CreateEvidenceAnalysisInput{
		FactType: "person_birth", SubjectID: subjectID1, Conclusion: "Born 1852",
	})

	subjectID2 := uuid.New()
	_, _ = cmdHandler.CreateEvidenceAnalysis(ctx, command.CreateEvidenceAnalysisInput{
		FactType: "person_death", SubjectID: subjectID2, Conclusion: "Died 1920",
	})
	_, _ = cmdHandler.CreateEvidenceAnalysis(ctx, command.CreateEvidenceAnalysisInput{
		FactType: "person_death", SubjectID: subjectID2, Conclusion: "Died 1921",
	})

	result, err := queryService.ListEvidenceConflicts(ctx, query.ListInput{Limit: 10})
	if err != nil {
		t.Fatalf("ListEvidenceConflicts failed: %v", err)
	}

	if result.Total != 2 {
		t.Errorf("Total = %d, want 2", result.Total)
	}
}

func TestGetConflictsForSubject(t *testing.T) {
	eventStore := memory.NewEventStore()
	readStore := memory.NewReadModelStore()
	cmdHandler := command.NewHandler(eventStore, readStore)
	queryService := query.NewEvidenceQueryService(readStore)
	ctx := context.Background()

	subjectID := uuid.New()

	_, _ = cmdHandler.CreateEvidenceAnalysis(ctx, command.CreateEvidenceAnalysisInput{
		FactType: "person_birth", SubjectID: subjectID, Conclusion: "Born 1850",
	})
	_, _ = cmdHandler.CreateEvidenceAnalysis(ctx, command.CreateEvidenceAnalysisInput{
		FactType: "person_birth", SubjectID: subjectID, Conclusion: "Born 1852",
	})

	// Create conflict for different subject
	otherSubject := uuid.New()
	_, _ = cmdHandler.CreateEvidenceAnalysis(ctx, command.CreateEvidenceAnalysisInput{
		FactType: "person_birth", SubjectID: otherSubject, Conclusion: "Born 1860",
	})
	_, _ = cmdHandler.CreateEvidenceAnalysis(ctx, command.CreateEvidenceAnalysisInput{
		FactType: "person_birth", SubjectID: otherSubject, Conclusion: "Born 1862",
	})

	conflicts, err := queryService.GetConflictsForSubject(ctx, subjectID)
	if err != nil {
		t.Fatalf("GetConflictsForSubject failed: %v", err)
	}

	if len(conflicts) != 1 {
		t.Errorf("Got %d conflicts, want 1", len(conflicts))
	}
	if len(conflicts) > 0 && conflicts[0].SubjectID != subjectID {
		t.Errorf("SubjectID mismatch")
	}
}

func TestListUnresolvedConflicts(t *testing.T) {
	eventStore := memory.NewEventStore()
	readStore := memory.NewReadModelStore()
	cmdHandler := command.NewHandler(eventStore, readStore)
	queryService := query.NewEvidenceQueryService(readStore)
	ctx := context.Background()

	subjectID1 := uuid.New()
	_, _ = cmdHandler.CreateEvidenceAnalysis(ctx, command.CreateEvidenceAnalysisInput{
		FactType: "person_birth", SubjectID: subjectID1, Conclusion: "Born 1850",
	})
	result1, _ := cmdHandler.CreateEvidenceAnalysis(ctx, command.CreateEvidenceAnalysisInput{
		FactType: "person_birth", SubjectID: subjectID1, Conclusion: "Born 1852",
	})

	subjectID2 := uuid.New()
	_, _ = cmdHandler.CreateEvidenceAnalysis(ctx, command.CreateEvidenceAnalysisInput{
		FactType: "person_death", SubjectID: subjectID2, Conclusion: "Died 1920",
	})
	_, _ = cmdHandler.CreateEvidenceAnalysis(ctx, command.CreateEvidenceAnalysisInput{
		FactType: "person_death", SubjectID: subjectID2, Conclusion: "Died 1921",
	})

	// Resolve the first conflict
	if result1.ConflictID != nil {
		conflict, _ := readStore.GetEvidenceConflict(ctx, *result1.ConflictID)
		_, _ = cmdHandler.ResolveEvidenceConflict(ctx, *result1.ConflictID, "Resolved", conflict.Version)
	}

	unresolved, err := queryService.ListUnresolvedConflicts(ctx)
	if err != nil {
		t.Fatalf("ListUnresolvedConflicts failed: %v", err)
	}

	if len(unresolved) != 1 {
		t.Errorf("Got %d unresolved conflicts, want 1", len(unresolved))
	}
}

// --- ResearchLog Query Tests ---

func TestListResearchLogs(t *testing.T) {
	eventStore := memory.NewEventStore()
	readStore := memory.NewReadModelStore()
	cmdHandler := command.NewHandler(eventStore, readStore)
	queryService := query.NewEvidenceQueryService(readStore)
	ctx := context.Background()

	for i := 0; i < 3; i++ {
		_, err := cmdHandler.CreateResearchLog(ctx, command.CreateResearchLogInput{
			SubjectID:         uuid.New(),
			SubjectType:       "person",
			Repository:        "Test Repo",
			SearchDescription: "Search",
			Outcome:           "found",
			SearchDate:        time.Now(),
		})
		if err != nil {
			t.Fatalf("Failed to create research log: %v", err)
		}
	}

	result, err := queryService.ListResearchLogs(ctx, query.ListInput{Limit: 10})
	if err != nil {
		t.Fatalf("ListResearchLogs failed: %v", err)
	}

	if result.Total != 3 {
		t.Errorf("Total = %d, want 3", result.Total)
	}
	if len(result.Logs) != 3 {
		t.Errorf("Got %d logs, want 3", len(result.Logs))
	}
}

func TestGetResearchLog(t *testing.T) {
	eventStore := memory.NewEventStore()
	readStore := memory.NewReadModelStore()
	cmdHandler := command.NewHandler(eventStore, readStore)
	queryService := query.NewEvidenceQueryService(readStore)
	ctx := context.Background()

	createResult, err := cmdHandler.CreateResearchLog(ctx, command.CreateResearchLogInput{
		SubjectID:         uuid.New(),
		SubjectType:       "person",
		Repository:        "National Archives",
		SearchDescription: "Census search 1850",
		Outcome:           "found",
		Notes:             "Found matching record",
		SearchDate:        time.Date(2024, 1, 15, 0, 0, 0, 0, time.UTC),
	})
	if err != nil {
		t.Fatalf("CreateResearchLog failed: %v", err)
	}

	result, err := queryService.GetResearchLog(ctx, createResult.ID)
	if err != nil {
		t.Fatalf("GetResearchLog failed: %v", err)
	}

	if result.ID != createResult.ID {
		t.Errorf("ID mismatch")
	}
	if result.Repository != "National Archives" {
		t.Errorf("Repository = %s, want 'National Archives'", result.Repository)
	}
	if result.Outcome != "found" {
		t.Errorf("Outcome = %s, want found", result.Outcome)
	}
	if result.Notes == nil || *result.Notes != "Found matching record" {
		t.Error("Notes not set correctly")
	}
}

func TestGetResearchLog_NotFound(t *testing.T) {
	readStore := memory.NewReadModelStore()
	queryService := query.NewEvidenceQueryService(readStore)
	ctx := context.Background()

	_, err := queryService.GetResearchLog(ctx, uuid.New())
	if err != query.ErrNotFound {
		t.Errorf("Expected ErrNotFound, got %v", err)
	}
}

func TestGetResearchLogsForSubject(t *testing.T) {
	eventStore := memory.NewEventStore()
	readStore := memory.NewReadModelStore()
	cmdHandler := command.NewHandler(eventStore, readStore)
	queryService := query.NewEvidenceQueryService(readStore)
	ctx := context.Background()

	subjectID := uuid.New()

	// Create logs for target subject
	_, _ = cmdHandler.CreateResearchLog(ctx, command.CreateResearchLogInput{
		SubjectID: subjectID, SubjectType: "person",
		Repository: "Archives", SearchDescription: "Search 1",
		Outcome: "found", SearchDate: time.Now(),
	})
	_, _ = cmdHandler.CreateResearchLog(ctx, command.CreateResearchLogInput{
		SubjectID: subjectID, SubjectType: "person",
		Repository: "Library", SearchDescription: "Search 2",
		Outcome: "not_found", SearchDate: time.Now(),
	})

	// Create log for different subject
	_, _ = cmdHandler.CreateResearchLog(ctx, command.CreateResearchLogInput{
		SubjectID: uuid.New(), SubjectType: "person",
		Repository: "Other", SearchDescription: "Search 3",
		Outcome: "found", SearchDate: time.Now(),
	})

	results, err := queryService.GetResearchLogsForSubject(ctx, subjectID)
	if err != nil {
		t.Fatalf("GetResearchLogsForSubject failed: %v", err)
	}

	if len(results) != 2 {
		t.Errorf("Got %d results, want 2", len(results))
	}
}

// --- ProofSummary Query Tests ---

func TestListProofSummaries(t *testing.T) {
	eventStore := memory.NewEventStore()
	readStore := memory.NewReadModelStore()
	cmdHandler := command.NewHandler(eventStore, readStore)
	queryService := query.NewEvidenceQueryService(readStore)
	ctx := context.Background()

	for i := 0; i < 3; i++ {
		_, err := cmdHandler.CreateProofSummary(ctx, command.CreateProofSummaryInput{
			FactType:   "person_birth",
			SubjectID:  uuid.New(),
			Conclusion: "Test conclusion",
			Argument:   "Test argument",
		})
		if err != nil {
			t.Fatalf("Failed to create proof summary: %v", err)
		}
	}

	result, err := queryService.ListProofSummaries(ctx, query.ListInput{Limit: 10})
	if err != nil {
		t.Fatalf("ListProofSummaries failed: %v", err)
	}

	if result.Total != 3 {
		t.Errorf("Total = %d, want 3", result.Total)
	}
	if len(result.Summaries) != 3 {
		t.Errorf("Got %d summaries, want 3", len(result.Summaries))
	}
}

func TestGetProofSummary(t *testing.T) {
	eventStore := memory.NewEventStore()
	readStore := memory.NewReadModelStore()
	cmdHandler := command.NewHandler(eventStore, readStore)
	queryService := query.NewEvidenceQueryService(readStore)
	ctx := context.Background()

	createResult, err := cmdHandler.CreateProofSummary(ctx, command.CreateProofSummaryInput{
		FactType:       "person_birth",
		SubjectID:      uuid.New(),
		Conclusion:     "Born in 1850",
		Argument:       "Three sources confirm",
		ResearchStatus: "certain",
	})
	if err != nil {
		t.Fatalf("CreateProofSummary failed: %v", err)
	}

	result, err := queryService.GetProofSummary(ctx, createResult.ID)
	if err != nil {
		t.Fatalf("GetProofSummary failed: %v", err)
	}

	if result.ID != createResult.ID {
		t.Errorf("ID mismatch")
	}
	if result.Conclusion != "Born in 1850" {
		t.Errorf("Conclusion = %s, want 'Born in 1850'", result.Conclusion)
	}
	if result.Argument != "Three sources confirm" {
		t.Errorf("Argument = %s, want 'Three sources confirm'", result.Argument)
	}
	if result.ResearchStatus == nil || *result.ResearchStatus != "certain" {
		t.Error("ResearchStatus not set correctly")
	}
}

func TestGetProofSummary_NotFound(t *testing.T) {
	readStore := memory.NewReadModelStore()
	queryService := query.NewEvidenceQueryService(readStore)
	ctx := context.Background()

	_, err := queryService.GetProofSummary(ctx, uuid.New())
	if err != query.ErrNotFound {
		t.Errorf("Expected ErrNotFound, got %v", err)
	}
}

func TestGetProofSummaryForFact(t *testing.T) {
	eventStore := memory.NewEventStore()
	readStore := memory.NewReadModelStore()
	cmdHandler := command.NewHandler(eventStore, readStore)
	queryService := query.NewEvidenceQueryService(readStore)
	ctx := context.Background()

	subjectID := uuid.New()

	_, err := cmdHandler.CreateProofSummary(ctx, command.CreateProofSummaryInput{
		FactType:   "person_birth",
		SubjectID:  subjectID,
		Conclusion: "Born in 1850",
		Argument:   "Evidence supports",
	})
	if err != nil {
		t.Fatalf("CreateProofSummary failed: %v", err)
	}

	// Also create a summary for different fact
	_, _ = cmdHandler.CreateProofSummary(ctx, command.CreateProofSummaryInput{
		FactType:   "person_death",
		SubjectID:  subjectID,
		Conclusion: "Died in 1920",
		Argument:   "Death cert confirms",
	})

	results, err := queryService.GetProofSummaryForFact(ctx, "person_birth", subjectID)
	if err != nil {
		t.Fatalf("GetProofSummaryForFact failed: %v", err)
	}

	if len(results) != 1 {
		t.Fatalf("Got %d results, want 1", len(results))
	}
	if results[0].FactType != "person_birth" {
		t.Errorf("FactType = %s, want person_birth", results[0].FactType)
	}
	if results[0].SubjectID != subjectID {
		t.Errorf("SubjectID mismatch")
	}
}

func TestGetProofSummaryForFact_NotFound(t *testing.T) {
	readStore := memory.NewReadModelStore()
	queryService := query.NewEvidenceQueryService(readStore)
	ctx := context.Background()

	results, err := queryService.GetProofSummaryForFact(ctx, "person_birth", uuid.New())
	if err != nil {
		t.Fatalf("GetProofSummaryForFact failed: %v", err)
	}
	if len(results) != 0 {
		t.Errorf("Expected empty results, got %d", len(results))
	}
}

func TestListResearchLogs_Pagination(t *testing.T) {
	eventStore := memory.NewEventStore()
	readStore := memory.NewReadModelStore()
	cmdHandler := command.NewHandler(eventStore, readStore)
	queryService := query.NewEvidenceQueryService(readStore)
	ctx := context.Background()

	for i := 0; i < 5; i++ {
		_, _ = cmdHandler.CreateResearchLog(ctx, command.CreateResearchLogInput{
			SubjectID:         uuid.New(),
			SubjectType:       "person",
			Repository:        "Test Repo",
			SearchDescription: "Search",
			Outcome:           "found",
			SearchDate:        time.Now(),
		})
	}

	result, err := queryService.ListResearchLogs(ctx, query.ListInput{Limit: 3, Offset: 0})
	if err != nil {
		t.Fatalf("ListResearchLogs failed: %v", err)
	}

	if len(result.Logs) != 3 {
		t.Errorf("Got %d logs, want 3", len(result.Logs))
	}
	if result.Total != 5 {
		t.Errorf("Total = %d, want 5", result.Total)
	}

	// Second page
	result2, err := queryService.ListResearchLogs(ctx, query.ListInput{Limit: 3, Offset: 3})
	if err != nil {
		t.Fatalf("ListResearchLogs page 2 failed: %v", err)
	}

	if len(result2.Logs) != 2 {
		t.Errorf("Got %d logs on page 2, want 2", len(result2.Logs))
	}
}
