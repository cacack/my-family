package command_test

import (
	"context"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/cacack/my-family/internal/command"
	"github.com/cacack/my-family/internal/repository"
	"github.com/cacack/my-family/internal/repository/memory"
)

// --- EvidenceAnalysis Tests ---

func TestCreateEvidenceAnalysis(t *testing.T) {
	eventStore := memory.NewEventStore()
	readStore := memory.NewReadModelStore()
	handler := command.NewHandler(eventStore, readStore)
	ctx := context.Background()

	tests := []struct {
		name    string
		input   command.CreateEvidenceAnalysisInput
		wantErr bool
	}{
		{
			name: "valid analysis",
			input: command.CreateEvidenceAnalysisInput{
				FactType:       "person_birth",
				SubjectID:      uuid.New(),
				Conclusion:     "Born in 1850 in Springfield",
				ResearchStatus: "probable",
				Notes:          "Based on census records",
			},
			wantErr: false,
		},
		{
			name: "valid analysis with citation IDs",
			input: command.CreateEvidenceAnalysisInput{
				FactType:    "person_death",
				SubjectID:   uuid.New(),
				CitationIDs: []uuid.UUID{uuid.New(), uuid.New()},
				Conclusion:  "Died in 1920",
			},
			wantErr: false,
		},
		{
			name: "missing conclusion",
			input: command.CreateEvidenceAnalysisInput{
				FactType:  "person_birth",
				SubjectID: uuid.New(),
			},
			wantErr: true,
		},
		{
			name: "missing subject_id",
			input: command.CreateEvidenceAnalysisInput{
				FactType:   "person_birth",
				SubjectID:  uuid.Nil,
				Conclusion: "Some conclusion",
			},
			wantErr: true,
		},
		{
			name: "invalid fact type",
			input: command.CreateEvidenceAnalysisInput{
				FactType:   "invalid_type",
				SubjectID:  uuid.New(),
				Conclusion: "Some conclusion",
			},
			wantErr: true,
		},
		{
			name: "invalid research status",
			input: command.CreateEvidenceAnalysisInput{
				FactType:       "person_birth",
				SubjectID:      uuid.New(),
				Conclusion:     "Some conclusion",
				ResearchStatus: "bad_status",
			},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := handler.CreateEvidenceAnalysis(ctx, tt.input)
			if (err != nil) != tt.wantErr {
				t.Errorf("CreateEvidenceAnalysis() error = %v, wantErr %v", err, tt.wantErr)
				return
			}

			if !tt.wantErr {
				if result.ID == uuid.Nil {
					t.Error("Expected non-nil ID")
				}
				if result.Version != 1 {
					t.Errorf("Version = %d, want 1", result.Version)
				}

				// Verify in read model
				analysis, _ := readStore.GetEvidenceAnalysis(ctx, result.ID)
				if analysis == nil {
					t.Fatal("Analysis not found in read model")
				}
				if analysis.Conclusion != tt.input.Conclusion {
					t.Errorf("Conclusion = %s, want %s", analysis.Conclusion, tt.input.Conclusion)
				}
			}
		})
	}
}

func TestCreateEvidenceAnalysis_ConflictAutoDetection(t *testing.T) {
	eventStore := memory.NewEventStore()
	readStore := memory.NewReadModelStore()
	handler := command.NewHandler(eventStore, readStore)
	ctx := context.Background()

	subjectID := uuid.New()

	// Create first analysis
	result1, err := handler.CreateEvidenceAnalysis(ctx, command.CreateEvidenceAnalysisInput{
		FactType:   "person_birth",
		SubjectID:  subjectID,
		Conclusion: "Born in 1850",
	})
	if err != nil {
		t.Fatalf("First CreateEvidenceAnalysis failed: %v", err)
	}
	if result1.ConflictID != nil {
		t.Error("First analysis should not have a conflict")
	}

	// Create second analysis with same conclusion - no conflict
	result2, err := handler.CreateEvidenceAnalysis(ctx, command.CreateEvidenceAnalysisInput{
		FactType:   "person_birth",
		SubjectID:  subjectID,
		Conclusion: "Born in 1850",
	})
	if err != nil {
		t.Fatalf("Second CreateEvidenceAnalysis failed: %v", err)
	}
	if result2.ConflictID != nil {
		t.Error("Same conclusion should not create a conflict")
	}

	// Create third analysis with different conclusion - should detect conflict
	result3, err := handler.CreateEvidenceAnalysis(ctx, command.CreateEvidenceAnalysisInput{
		FactType:   "person_birth",
		SubjectID:  subjectID,
		Conclusion: "Born in 1852",
	})
	if err != nil {
		t.Fatalf("Third CreateEvidenceAnalysis failed: %v", err)
	}
	if result3.ConflictID == nil {
		t.Error("Different conclusion should create a conflict")
	}

	// Verify conflict exists in read model
	if result3.ConflictID != nil {
		conflict, _ := readStore.GetEvidenceConflict(ctx, *result3.ConflictID)
		if conflict == nil {
			t.Fatal("Conflict not found in read model")
		}
		if conflict.SubjectID != subjectID {
			t.Errorf("Conflict SubjectID = %v, want %v", conflict.SubjectID, subjectID)
		}
	}
}

func TestCreateEvidenceAnalysis_NoConflictDifferentSubject(t *testing.T) {
	eventStore := memory.NewEventStore()
	readStore := memory.NewReadModelStore()
	handler := command.NewHandler(eventStore, readStore)
	ctx := context.Background()

	// Create analyses for different subjects with different conclusions
	_, err := handler.CreateEvidenceAnalysis(ctx, command.CreateEvidenceAnalysisInput{
		FactType:   "person_birth",
		SubjectID:  uuid.New(),
		Conclusion: "Born in 1850",
	})
	if err != nil {
		t.Fatalf("First CreateEvidenceAnalysis failed: %v", err)
	}

	result2, err := handler.CreateEvidenceAnalysis(ctx, command.CreateEvidenceAnalysisInput{
		FactType:   "person_birth",
		SubjectID:  uuid.New(), // different subject
		Conclusion: "Born in 1852",
	})
	if err != nil {
		t.Fatalf("Second CreateEvidenceAnalysis failed: %v", err)
	}
	if result2.ConflictID != nil {
		t.Error("Different subjects should not create a conflict")
	}
}

func TestUpdateEvidenceAnalysis(t *testing.T) {
	eventStore := memory.NewEventStore()
	readStore := memory.NewReadModelStore()
	handler := command.NewHandler(eventStore, readStore)
	ctx := context.Background()

	createResult, err := handler.CreateEvidenceAnalysis(ctx, command.CreateEvidenceAnalysisInput{
		FactType:   "person_birth",
		SubjectID:  uuid.New(),
		Conclusion: "Born in 1850",
	})
	if err != nil {
		t.Fatalf("CreateEvidenceAnalysis failed: %v", err)
	}

	tests := []struct {
		name    string
		input   command.UpdateEvidenceAnalysisInput
		wantErr bool
	}{
		{
			name: "update conclusion",
			input: command.UpdateEvidenceAnalysisInput{
				ID:         createResult.ID,
				Conclusion: strPtr("Born in 1851"),
				Version:    createResult.Version,
			},
			wantErr: false,
		},
		{
			name: "update notes",
			input: command.UpdateEvidenceAnalysisInput{
				ID:      createResult.ID,
				Notes:   strPtr("Updated notes"),
				Version: 2,
			},
			wantErr: false,
		},
		{
			name: "wrong version",
			input: command.UpdateEvidenceAnalysisInput{
				ID:         createResult.ID,
				Conclusion: strPtr("Should fail"),
				Version:    999,
			},
			wantErr: true,
		},
		{
			name: "invalid research status",
			input: command.UpdateEvidenceAnalysisInput{
				ID:             createResult.ID,
				ResearchStatus: strPtr("bad_status"),
				Version:        3,
			},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := handler.UpdateEvidenceAnalysis(ctx, tt.input)
			if (err != nil) != tt.wantErr {
				t.Errorf("UpdateEvidenceAnalysis() error = %v, wantErr %v", err, tt.wantErr)
				return
			}

			if !tt.wantErr && result.Version <= tt.input.Version {
				t.Errorf("Version not incremented: got %d, want > %d", result.Version, tt.input.Version)
			}
		})
	}
}

func TestUpdateEvidenceAnalysis_NoChanges(t *testing.T) {
	eventStore := memory.NewEventStore()
	readStore := memory.NewReadModelStore()
	handler := command.NewHandler(eventStore, readStore)
	ctx := context.Background()

	createResult, _ := handler.CreateEvidenceAnalysis(ctx, command.CreateEvidenceAnalysisInput{
		FactType:   "person_birth",
		SubjectID:  uuid.New(),
		Conclusion: "Test",
	})

	result, err := handler.UpdateEvidenceAnalysis(ctx, command.UpdateEvidenceAnalysisInput{
		ID:      createResult.ID,
		Version: createResult.Version,
	})
	if err != nil {
		t.Fatalf("UpdateEvidenceAnalysis failed: %v", err)
	}
	if result.Version != createResult.Version {
		t.Errorf("Version changed without updates: got %d, want %d", result.Version, createResult.Version)
	}
}

func TestUpdateEvidenceAnalysis_NotFound(t *testing.T) {
	eventStore := memory.NewEventStore()
	readStore := memory.NewReadModelStore()
	handler := command.NewHandler(eventStore, readStore)
	ctx := context.Background()

	_, err := handler.UpdateEvidenceAnalysis(ctx, command.UpdateEvidenceAnalysisInput{
		ID:         uuid.New(),
		Conclusion: strPtr("Should fail"),
		Version:    1,
	})
	if err != command.ErrEvidenceAnalysisNotFound {
		t.Errorf("Expected ErrEvidenceAnalysisNotFound, got %v", err)
	}
}

func TestDeleteEvidenceAnalysis(t *testing.T) {
	eventStore := memory.NewEventStore()
	readStore := memory.NewReadModelStore()
	handler := command.NewHandler(eventStore, readStore)
	ctx := context.Background()

	createResult, _ := handler.CreateEvidenceAnalysis(ctx, command.CreateEvidenceAnalysisInput{
		FactType:   "person_birth",
		SubjectID:  uuid.New(),
		Conclusion: "Test",
	})

	err := handler.DeleteEvidenceAnalysis(ctx, createResult.ID, createResult.Version, "Test deletion")
	if err != nil {
		t.Errorf("DeleteEvidenceAnalysis failed: %v", err)
	}

	analysis, _ := readStore.GetEvidenceAnalysis(ctx, createResult.ID)
	if analysis != nil {
		t.Error("Analysis should be deleted from read model")
	}
}

func TestDeleteEvidenceAnalysis_NotFound(t *testing.T) {
	eventStore := memory.NewEventStore()
	readStore := memory.NewReadModelStore()
	handler := command.NewHandler(eventStore, readStore)
	ctx := context.Background()

	err := handler.DeleteEvidenceAnalysis(ctx, uuid.New(), 1, "Should fail")
	if err != command.ErrEvidenceAnalysisNotFound {
		t.Errorf("Expected ErrEvidenceAnalysisNotFound, got %v", err)
	}
}

func TestDeleteEvidenceAnalysis_WrongVersion(t *testing.T) {
	eventStore := memory.NewEventStore()
	readStore := memory.NewReadModelStore()
	handler := command.NewHandler(eventStore, readStore)
	ctx := context.Background()

	createResult, _ := handler.CreateEvidenceAnalysis(ctx, command.CreateEvidenceAnalysisInput{
		FactType:   "person_birth",
		SubjectID:  uuid.New(),
		Conclusion: "Test",
	})

	err := handler.DeleteEvidenceAnalysis(ctx, createResult.ID, 999, "Should fail")
	if err != repository.ErrConcurrencyConflict {
		t.Errorf("Expected ErrConcurrencyConflict, got %v", err)
	}
}

// --- EvidenceConflict Tests ---

func TestResolveEvidenceConflict(t *testing.T) {
	eventStore := memory.NewEventStore()
	readStore := memory.NewReadModelStore()
	handler := command.NewHandler(eventStore, readStore)
	ctx := context.Background()

	subjectID := uuid.New()

	// Create two analyses with conflicting conclusions to trigger auto-detection
	_, _ = handler.CreateEvidenceAnalysis(ctx, command.CreateEvidenceAnalysisInput{
		FactType:   "person_birth",
		SubjectID:  subjectID,
		Conclusion: "Born in 1850",
	})

	result2, _ := handler.CreateEvidenceAnalysis(ctx, command.CreateEvidenceAnalysisInput{
		FactType:   "person_birth",
		SubjectID:  subjectID,
		Conclusion: "Born in 1852",
	})

	if result2.ConflictID == nil {
		t.Fatal("Expected a conflict to be auto-detected")
	}

	// Resolve the conflict
	conflict, _ := readStore.GetEvidenceConflict(ctx, *result2.ConflictID)
	resolveResult, err := handler.ResolveEvidenceConflict(ctx, *result2.ConflictID, "1850 is correct based on birth certificate", conflict.Version)
	if err != nil {
		t.Fatalf("ResolveEvidenceConflict failed: %v", err)
	}

	if resolveResult.Version <= conflict.Version {
		t.Errorf("Version not incremented")
	}

	// Verify resolved in read model
	resolved, _ := readStore.GetEvidenceConflict(ctx, *result2.ConflictID)
	if resolved == nil {
		t.Fatal("Conflict not found after resolution")
	}
	if resolved.Resolution != "1850 is correct based on birth certificate" {
		t.Errorf("Resolution = %s, want '1850 is correct based on birth certificate'", resolved.Resolution)
	}
	if string(resolved.Status) != "resolved" {
		t.Errorf("Status = %s, want resolved", resolved.Status)
	}
}

func TestResolveEvidenceConflict_NotFound(t *testing.T) {
	eventStore := memory.NewEventStore()
	readStore := memory.NewReadModelStore()
	handler := command.NewHandler(eventStore, readStore)
	ctx := context.Background()

	_, err := handler.ResolveEvidenceConflict(ctx, uuid.New(), "resolution", 1)
	if err != command.ErrEvidenceConflictNotFound {
		t.Errorf("Expected ErrEvidenceConflictNotFound, got %v", err)
	}
}

func TestResolveEvidenceConflict_EmptyResolution(t *testing.T) {
	eventStore := memory.NewEventStore()
	readStore := memory.NewReadModelStore()
	handler := command.NewHandler(eventStore, readStore)
	ctx := context.Background()

	subjectID := uuid.New()

	_, _ = handler.CreateEvidenceAnalysis(ctx, command.CreateEvidenceAnalysisInput{
		FactType:   "person_birth",
		SubjectID:  subjectID,
		Conclusion: "Born in 1850",
	})
	result2, _ := handler.CreateEvidenceAnalysis(ctx, command.CreateEvidenceAnalysisInput{
		FactType:   "person_birth",
		SubjectID:  subjectID,
		Conclusion: "Born in 1852",
	})

	if result2.ConflictID == nil {
		t.Fatal("Expected conflict")
	}

	conflict, _ := readStore.GetEvidenceConflict(ctx, *result2.ConflictID)
	_, err := handler.ResolveEvidenceConflict(ctx, *result2.ConflictID, "", conflict.Version)
	if err == nil {
		t.Error("Expected error for empty resolution")
	}
}

func TestResolveEvidenceConflict_WrongVersion(t *testing.T) {
	eventStore := memory.NewEventStore()
	readStore := memory.NewReadModelStore()
	handler := command.NewHandler(eventStore, readStore)
	ctx := context.Background()

	subjectID := uuid.New()

	_, _ = handler.CreateEvidenceAnalysis(ctx, command.CreateEvidenceAnalysisInput{
		FactType:   "person_birth",
		SubjectID:  subjectID,
		Conclusion: "Born in 1850",
	})
	result2, _ := handler.CreateEvidenceAnalysis(ctx, command.CreateEvidenceAnalysisInput{
		FactType:   "person_birth",
		SubjectID:  subjectID,
		Conclusion: "Born in 1852",
	})

	if result2.ConflictID == nil {
		t.Fatal("Expected conflict")
	}

	_, err := handler.ResolveEvidenceConflict(ctx, *result2.ConflictID, "resolution", 999)
	if err != repository.ErrConcurrencyConflict {
		t.Errorf("Expected ErrConcurrencyConflict, got %v", err)
	}
}

// --- ResearchLog Tests ---

func TestCreateResearchLog(t *testing.T) {
	eventStore := memory.NewEventStore()
	readStore := memory.NewReadModelStore()
	handler := command.NewHandler(eventStore, readStore)
	ctx := context.Background()

	tests := []struct {
		name    string
		input   command.CreateResearchLogInput
		wantErr bool
	}{
		{
			name: "valid research log",
			input: command.CreateResearchLogInput{
				SubjectID:         uuid.New(),
				SubjectType:       "person",
				Repository:        "National Archives",
				SearchDescription: "Searched census records 1850-1860",
				Outcome:           "found",
				SearchDate:        time.Now(),
				Notes:             "Found matching record",
			},
			wantErr: false,
		},
		{
			name: "valid negative result",
			input: command.CreateResearchLogInput{
				SubjectID:         uuid.New(),
				SubjectType:       "family",
				Repository:        "County Clerk Office",
				SearchDescription: "Searched marriage records",
				Outcome:           "not_found",
				SearchDate:        time.Now(),
			},
			wantErr: false,
		},
		{
			name: "invalid subject type",
			input: command.CreateResearchLogInput{
				SubjectID:         uuid.New(),
				SubjectType:       "invalid",
				Repository:        "Test Repo",
				SearchDescription: "Test search",
				Outcome:           "found",
				SearchDate:        time.Now(),
			},
			wantErr: true,
		},
		{
			name: "missing repository",
			input: command.CreateResearchLogInput{
				SubjectID:         uuid.New(),
				SubjectType:       "person",
				Repository:        "",
				SearchDescription: "Test search",
				Outcome:           "found",
				SearchDate:        time.Now(),
			},
			wantErr: true,
		},
		{
			name: "invalid outcome",
			input: command.CreateResearchLogInput{
				SubjectID:         uuid.New(),
				SubjectType:       "person",
				Repository:        "Test Repo",
				SearchDescription: "Test search",
				Outcome:           "bad_outcome",
				SearchDate:        time.Now(),
			},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := handler.CreateResearchLog(ctx, tt.input)
			if (err != nil) != tt.wantErr {
				t.Errorf("CreateResearchLog() error = %v, wantErr %v", err, tt.wantErr)
				return
			}

			if !tt.wantErr {
				if result.ID == uuid.Nil {
					t.Error("Expected non-nil ID")
				}
				if result.Version != 1 {
					t.Errorf("Version = %d, want 1", result.Version)
				}

				log, _ := readStore.GetResearchLog(ctx, result.ID)
				if log == nil {
					t.Fatal("Research log not found in read model")
				}
				if log.Repository != tt.input.Repository {
					t.Errorf("Repository = %s, want %s", log.Repository, tt.input.Repository)
				}
			}
		})
	}
}

func TestUpdateResearchLog(t *testing.T) {
	eventStore := memory.NewEventStore()
	readStore := memory.NewReadModelStore()
	handler := command.NewHandler(eventStore, readStore)
	ctx := context.Background()

	createResult, err := handler.CreateResearchLog(ctx, command.CreateResearchLogInput{
		SubjectID:         uuid.New(),
		SubjectType:       "person",
		Repository:        "National Archives",
		SearchDescription: "Searched census",
		Outcome:           "found",
		SearchDate:        time.Now(),
	})
	if err != nil {
		t.Fatalf("CreateResearchLog failed: %v", err)
	}

	tests := []struct {
		name    string
		input   command.UpdateResearchLogInput
		wantErr bool
	}{
		{
			name: "update notes",
			input: command.UpdateResearchLogInput{
				ID:      createResult.ID,
				Notes:   strPtr("Updated notes"),
				Version: createResult.Version,
			},
			wantErr: false,
		},
		{
			name: "update outcome",
			input: command.UpdateResearchLogInput{
				ID:      createResult.ID,
				Outcome: strPtr("inconclusive"),
				Version: 2,
			},
			wantErr: false,
		},
		{
			name: "wrong version",
			input: command.UpdateResearchLogInput{
				ID:      createResult.ID,
				Notes:   strPtr("Should fail"),
				Version: 999,
			},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := handler.UpdateResearchLog(ctx, tt.input)
			if (err != nil) != tt.wantErr {
				t.Errorf("UpdateResearchLog() error = %v, wantErr %v", err, tt.wantErr)
				return
			}

			if !tt.wantErr && result.Version <= tt.input.Version {
				t.Errorf("Version not incremented: got %d, want > %d", result.Version, tt.input.Version)
			}
		})
	}
}

func TestUpdateResearchLog_NoChanges(t *testing.T) {
	eventStore := memory.NewEventStore()
	readStore := memory.NewReadModelStore()
	handler := command.NewHandler(eventStore, readStore)
	ctx := context.Background()

	createResult, _ := handler.CreateResearchLog(ctx, command.CreateResearchLogInput{
		SubjectID:         uuid.New(),
		SubjectType:       "person",
		Repository:        "Test Repo",
		SearchDescription: "Test search",
		Outcome:           "found",
		SearchDate:        time.Now(),
	})

	result, err := handler.UpdateResearchLog(ctx, command.UpdateResearchLogInput{
		ID:      createResult.ID,
		Version: createResult.Version,
	})
	if err != nil {
		t.Fatalf("UpdateResearchLog failed: %v", err)
	}
	if result.Version != createResult.Version {
		t.Errorf("Version changed without updates: got %d, want %d", result.Version, createResult.Version)
	}
}

func TestUpdateResearchLog_NotFound(t *testing.T) {
	eventStore := memory.NewEventStore()
	readStore := memory.NewReadModelStore()
	handler := command.NewHandler(eventStore, readStore)
	ctx := context.Background()

	_, err := handler.UpdateResearchLog(ctx, command.UpdateResearchLogInput{
		ID:      uuid.New(),
		Notes:   strPtr("Should fail"),
		Version: 1,
	})
	if err != command.ErrResearchLogNotFound {
		t.Errorf("Expected ErrResearchLogNotFound, got %v", err)
	}
}

func TestDeleteResearchLog(t *testing.T) {
	eventStore := memory.NewEventStore()
	readStore := memory.NewReadModelStore()
	handler := command.NewHandler(eventStore, readStore)
	ctx := context.Background()

	createResult, _ := handler.CreateResearchLog(ctx, command.CreateResearchLogInput{
		SubjectID:         uuid.New(),
		SubjectType:       "person",
		Repository:        "Test Repo",
		SearchDescription: "Test search",
		Outcome:           "found",
		SearchDate:        time.Now(),
	})

	err := handler.DeleteResearchLog(ctx, createResult.ID, createResult.Version, "Test deletion")
	if err != nil {
		t.Errorf("DeleteResearchLog failed: %v", err)
	}

	log, _ := readStore.GetResearchLog(ctx, createResult.ID)
	if log != nil {
		t.Error("Research log should be deleted from read model")
	}
}

func TestDeleteResearchLog_NotFound(t *testing.T) {
	eventStore := memory.NewEventStore()
	readStore := memory.NewReadModelStore()
	handler := command.NewHandler(eventStore, readStore)
	ctx := context.Background()

	err := handler.DeleteResearchLog(ctx, uuid.New(), 1, "Should fail")
	if err != command.ErrResearchLogNotFound {
		t.Errorf("Expected ErrResearchLogNotFound, got %v", err)
	}
}

func TestDeleteResearchLog_WrongVersion(t *testing.T) {
	eventStore := memory.NewEventStore()
	readStore := memory.NewReadModelStore()
	handler := command.NewHandler(eventStore, readStore)
	ctx := context.Background()

	createResult, _ := handler.CreateResearchLog(ctx, command.CreateResearchLogInput{
		SubjectID:         uuid.New(),
		SubjectType:       "person",
		Repository:        "Test Repo",
		SearchDescription: "Test search",
		Outcome:           "found",
		SearchDate:        time.Now(),
	})

	err := handler.DeleteResearchLog(ctx, createResult.ID, 999, "Should fail")
	if err != repository.ErrConcurrencyConflict {
		t.Errorf("Expected ErrConcurrencyConflict, got %v", err)
	}
}

// --- ProofSummary Tests ---

func TestCreateProofSummary(t *testing.T) {
	eventStore := memory.NewEventStore()
	readStore := memory.NewReadModelStore()
	handler := command.NewHandler(eventStore, readStore)
	ctx := context.Background()

	tests := []struct {
		name    string
		input   command.CreateProofSummaryInput
		wantErr bool
	}{
		{
			name: "valid proof summary",
			input: command.CreateProofSummaryInput{
				FactType:       "person_birth",
				SubjectID:      uuid.New(),
				Conclusion:     "Born in 1850 in Springfield",
				Argument:       "Three independent sources confirm this date and location",
				AnalysisIDs:    []uuid.UUID{uuid.New(), uuid.New()},
				ResearchStatus: "certain",
			},
			wantErr: false,
		},
		{
			name: "minimal proof summary",
			input: command.CreateProofSummaryInput{
				FactType:   "person_death",
				SubjectID:  uuid.New(),
				Conclusion: "Died in 1920",
				Argument:   "Death certificate confirms",
			},
			wantErr: false,
		},
		{
			name: "missing conclusion",
			input: command.CreateProofSummaryInput{
				FactType:  "person_birth",
				SubjectID: uuid.New(),
				Argument:  "Some argument",
			},
			wantErr: true,
		},
		{
			name: "missing argument",
			input: command.CreateProofSummaryInput{
				FactType:   "person_birth",
				SubjectID:  uuid.New(),
				Conclusion: "Some conclusion",
			},
			wantErr: true,
		},
		{
			name: "invalid fact type",
			input: command.CreateProofSummaryInput{
				FactType:   "invalid_type",
				SubjectID:  uuid.New(),
				Conclusion: "Some conclusion",
				Argument:   "Some argument",
			},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := handler.CreateProofSummary(ctx, tt.input)
			if (err != nil) != tt.wantErr {
				t.Errorf("CreateProofSummary() error = %v, wantErr %v", err, tt.wantErr)
				return
			}

			if !tt.wantErr {
				if result.ID == uuid.Nil {
					t.Error("Expected non-nil ID")
				}
				if result.Version != 1 {
					t.Errorf("Version = %d, want 1", result.Version)
				}

				summary, _ := readStore.GetProofSummary(ctx, result.ID)
				if summary == nil {
					t.Fatal("Proof summary not found in read model")
				}
				if summary.Conclusion != tt.input.Conclusion {
					t.Errorf("Conclusion = %s, want %s", summary.Conclusion, tt.input.Conclusion)
				}
			}
		})
	}
}

func TestUpdateProofSummary(t *testing.T) {
	eventStore := memory.NewEventStore()
	readStore := memory.NewReadModelStore()
	handler := command.NewHandler(eventStore, readStore)
	ctx := context.Background()

	createResult, err := handler.CreateProofSummary(ctx, command.CreateProofSummaryInput{
		FactType:   "person_birth",
		SubjectID:  uuid.New(),
		Conclusion: "Born in 1850",
		Argument:   "Based on census records",
	})
	if err != nil {
		t.Fatalf("CreateProofSummary failed: %v", err)
	}

	tests := []struct {
		name    string
		input   command.UpdateProofSummaryInput
		wantErr bool
	}{
		{
			name: "update argument",
			input: command.UpdateProofSummaryInput{
				ID:       createResult.ID,
				Argument: strPtr("Updated argument with more detail"),
				Version:  createResult.Version,
			},
			wantErr: false,
		},
		{
			name: "update research status",
			input: command.UpdateProofSummaryInput{
				ID:             createResult.ID,
				ResearchStatus: strPtr("certain"),
				Version:        2,
			},
			wantErr: false,
		},
		{
			name: "wrong version",
			input: command.UpdateProofSummaryInput{
				ID:       createResult.ID,
				Argument: strPtr("Should fail"),
				Version:  999,
			},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := handler.UpdateProofSummary(ctx, tt.input)
			if (err != nil) != tt.wantErr {
				t.Errorf("UpdateProofSummary() error = %v, wantErr %v", err, tt.wantErr)
				return
			}

			if !tt.wantErr && result.Version <= tt.input.Version {
				t.Errorf("Version not incremented: got %d, want > %d", result.Version, tt.input.Version)
			}
		})
	}
}

func TestUpdateProofSummary_NoChanges(t *testing.T) {
	eventStore := memory.NewEventStore()
	readStore := memory.NewReadModelStore()
	handler := command.NewHandler(eventStore, readStore)
	ctx := context.Background()

	createResult, _ := handler.CreateProofSummary(ctx, command.CreateProofSummaryInput{
		FactType:   "person_birth",
		SubjectID:  uuid.New(),
		Conclusion: "Test",
		Argument:   "Test argument",
	})

	result, err := handler.UpdateProofSummary(ctx, command.UpdateProofSummaryInput{
		ID:      createResult.ID,
		Version: createResult.Version,
	})
	if err != nil {
		t.Fatalf("UpdateProofSummary failed: %v", err)
	}
	if result.Version != createResult.Version {
		t.Errorf("Version changed without updates: got %d, want %d", result.Version, createResult.Version)
	}
}

func TestUpdateProofSummary_NotFound(t *testing.T) {
	eventStore := memory.NewEventStore()
	readStore := memory.NewReadModelStore()
	handler := command.NewHandler(eventStore, readStore)
	ctx := context.Background()

	_, err := handler.UpdateProofSummary(ctx, command.UpdateProofSummaryInput{
		ID:       uuid.New(),
		Argument: strPtr("Should fail"),
		Version:  1,
	})
	if err != command.ErrProofSummaryNotFound {
		t.Errorf("Expected ErrProofSummaryNotFound, got %v", err)
	}
}

func TestDeleteProofSummary(t *testing.T) {
	eventStore := memory.NewEventStore()
	readStore := memory.NewReadModelStore()
	handler := command.NewHandler(eventStore, readStore)
	ctx := context.Background()

	createResult, _ := handler.CreateProofSummary(ctx, command.CreateProofSummaryInput{
		FactType:   "person_birth",
		SubjectID:  uuid.New(),
		Conclusion: "Test",
		Argument:   "Test argument",
	})

	err := handler.DeleteProofSummary(ctx, createResult.ID, createResult.Version, "Test deletion")
	if err != nil {
		t.Errorf("DeleteProofSummary failed: %v", err)
	}

	summary, _ := readStore.GetProofSummary(ctx, createResult.ID)
	if summary != nil {
		t.Error("Proof summary should be deleted from read model")
	}
}

func TestDeleteProofSummary_NotFound(t *testing.T) {
	eventStore := memory.NewEventStore()
	readStore := memory.NewReadModelStore()
	handler := command.NewHandler(eventStore, readStore)
	ctx := context.Background()

	err := handler.DeleteProofSummary(ctx, uuid.New(), 1, "Should fail")
	if err != command.ErrProofSummaryNotFound {
		t.Errorf("Expected ErrProofSummaryNotFound, got %v", err)
	}
}

func TestDeleteProofSummary_WrongVersion(t *testing.T) {
	eventStore := memory.NewEventStore()
	readStore := memory.NewReadModelStore()
	handler := command.NewHandler(eventStore, readStore)
	ctx := context.Background()

	createResult, _ := handler.CreateProofSummary(ctx, command.CreateProofSummaryInput{
		FactType:   "person_birth",
		SubjectID:  uuid.New(),
		Conclusion: "Test",
		Argument:   "Test argument",
	})

	err := handler.DeleteProofSummary(ctx, createResult.ID, 999, "Should fail")
	if err != repository.ErrConcurrencyConflict {
		t.Errorf("Expected ErrConcurrencyConflict, got %v", err)
	}
}

// TestUpdateEvidenceAnalysis_AllFields exercises all update branches.
func TestUpdateEvidenceAnalysis_AllFields(t *testing.T) {
	eventStore := memory.NewEventStore()
	readStore := memory.NewReadModelStore()
	handler := command.NewHandler(eventStore, readStore)
	ctx := context.Background()

	subjectID := uuid.New()
	createResult, err := handler.CreateEvidenceAnalysis(ctx, command.CreateEvidenceAnalysisInput{
		FactType:   "person_birth",
		SubjectID:  subjectID,
		Conclusion: "Born in 1850",
	})
	if err != nil {
		t.Fatalf("CreateEvidenceAnalysis failed: %v", err)
	}

	newSubjectID := uuid.New()
	newCitationIDs := []uuid.UUID{uuid.New(), uuid.New()}
	result, err := handler.UpdateEvidenceAnalysis(ctx, command.UpdateEvidenceAnalysisInput{
		ID:             createResult.ID,
		FactType:       strPtr("person_death"),
		SubjectID:      &newSubjectID,
		CitationIDs:    newCitationIDs,
		Conclusion:     strPtr("Died in 1920"),
		ResearchStatus: strPtr("certain"),
		Notes:          strPtr("Updated notes"),
		Version:        createResult.Version,
	})
	if err != nil {
		t.Fatalf("UpdateEvidenceAnalysis failed: %v", err)
	}
	if result.Version <= createResult.Version {
		t.Errorf("Version not incremented")
	}

	// Verify in read model
	analysis, _ := readStore.GetEvidenceAnalysis(ctx, createResult.ID)
	if analysis.Conclusion != "Died in 1920" {
		t.Errorf("Conclusion = %s, want 'Died in 1920'", analysis.Conclusion)
	}
}

// TestUpdateResearchLog_AllFields exercises all research log update branches.
func TestUpdateResearchLog_AllFields(t *testing.T) {
	eventStore := memory.NewEventStore()
	readStore := memory.NewReadModelStore()
	handler := command.NewHandler(eventStore, readStore)
	ctx := context.Background()

	createResult, err := handler.CreateResearchLog(ctx, command.CreateResearchLogInput{
		SubjectID:         uuid.New(),
		SubjectType:       "person",
		Repository:        "National Archives",
		SearchDescription: "Census search",
		Outcome:           "found",
		SearchDate:        time.Now(),
	})
	if err != nil {
		t.Fatalf("CreateResearchLog failed: %v", err)
	}

	newSubjectID := uuid.New()
	newSearchDate := time.Date(2025, 6, 15, 0, 0, 0, 0, time.UTC)
	result, err := handler.UpdateResearchLog(ctx, command.UpdateResearchLogInput{
		ID:                createResult.ID,
		SubjectID:         &newSubjectID,
		SubjectType:       strPtr("family"),
		Repository:        strPtr("County Clerk"),
		SearchDescription: strPtr("Marriage records"),
		Outcome:           strPtr("not_found"),
		Notes:             strPtr("Searched all years"),
		SearchDate:        &newSearchDate,
		Version:           createResult.Version,
	})
	if err != nil {
		t.Fatalf("UpdateResearchLog failed: %v", err)
	}
	if result.Version <= createResult.Version {
		t.Errorf("Version not incremented")
	}

	log, _ := readStore.GetResearchLog(ctx, createResult.ID)
	if log.Repository != "County Clerk" {
		t.Errorf("Repository = %s, want 'County Clerk'", log.Repository)
	}
}

// TestUpdateProofSummary_AllFields exercises all proof summary update branches.
func TestUpdateProofSummary_AllFields(t *testing.T) {
	eventStore := memory.NewEventStore()
	readStore := memory.NewReadModelStore()
	handler := command.NewHandler(eventStore, readStore)
	ctx := context.Background()

	createResult, err := handler.CreateProofSummary(ctx, command.CreateProofSummaryInput{
		FactType:   "person_birth",
		SubjectID:  uuid.New(),
		Conclusion: "Born in 1850",
		Argument:   "Census records",
	})
	if err != nil {
		t.Fatalf("CreateProofSummary failed: %v", err)
	}

	newSubjectID := uuid.New()
	newAnalysisIDs := []uuid.UUID{uuid.New()}
	result, err := handler.UpdateProofSummary(ctx, command.UpdateProofSummaryInput{
		ID:             createResult.ID,
		FactType:       strPtr("person_death"),
		SubjectID:      &newSubjectID,
		Conclusion:     strPtr("Died in 1920"),
		Argument:       strPtr("Death certificate"),
		AnalysisIDs:    newAnalysisIDs,
		ResearchStatus: strPtr("certain"),
		Version:        createResult.Version,
	})
	if err != nil {
		t.Fatalf("UpdateProofSummary failed: %v", err)
	}
	if result.Version <= createResult.Version {
		t.Errorf("Version not incremented")
	}

	summary, _ := readStore.GetProofSummary(ctx, createResult.ID)
	if summary.Conclusion != "Died in 1920" {
		t.Errorf("Conclusion = %s, want 'Died in 1920'", summary.Conclusion)
	}
}
