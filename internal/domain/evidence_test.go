package domain

import (
	"testing"
	"time"

	"github.com/google/uuid"
)

// --- Enum Tests ---

func TestConflictStatus_IsValid(t *testing.T) {
	tests := []struct {
		name   string
		status ConflictStatus
		want   bool
	}{
		{name: "open is valid", status: ConflictStatusOpen, want: true},
		{name: "resolved is valid", status: ConflictStatusResolved, want: true},
		{name: "accepted is valid", status: ConflictStatusAccepted, want: true},
		{name: "empty is invalid", status: "", want: false},
		{name: "invalid value", status: "invalid", want: false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.status.IsValid(); got != tt.want {
				t.Errorf("ConflictStatus(%q).IsValid() = %v, want %v", tt.status, got, tt.want)
			}
		})
	}
}

func TestAllConflictStatuses(t *testing.T) {
	statuses := AllConflictStatuses()
	if len(statuses) != 3 {
		t.Errorf("AllConflictStatuses() returned %d items, want 3", len(statuses))
	}
	for _, s := range statuses {
		if !s.IsValid() {
			t.Errorf("AllConflictStatuses() contains invalid status: %q", s)
		}
	}
}

func TestResearchOutcome_IsValid(t *testing.T) {
	tests := []struct {
		name    string
		outcome ResearchOutcome
		want    bool
	}{
		{name: "found is valid", outcome: ResearchOutcomeFound, want: true},
		{name: "not_found is valid", outcome: ResearchOutcomeNotFound, want: true},
		{name: "inconclusive is valid", outcome: ResearchOutcomeInconclusive, want: true},
		{name: "empty is invalid", outcome: "", want: false},
		{name: "invalid value", outcome: "invalid", want: false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.outcome.IsValid(); got != tt.want {
				t.Errorf("ResearchOutcome(%q).IsValid() = %v, want %v", tt.outcome, got, tt.want)
			}
		})
	}
}

func TestAllResearchOutcomes(t *testing.T) {
	outcomes := AllResearchOutcomes()
	if len(outcomes) != 3 {
		t.Errorf("AllResearchOutcomes() returned %d items, want 3", len(outcomes))
	}
	for _, o := range outcomes {
		if !o.IsValid() {
			t.Errorf("AllResearchOutcomes() contains invalid outcome: %q", o)
		}
	}
}

// --- EvidenceAnalysis Tests ---

func TestNewEvidenceAnalysis(t *testing.T) {
	subjectID := uuid.New()
	ea := NewEvidenceAnalysis(FactPersonBirth, subjectID, "Born in 1850")

	if ea.ID == uuid.Nil {
		t.Error("Expected non-nil UUID")
	}
	if ea.FactType != FactPersonBirth {
		t.Errorf("FactType = %v, want person_birth", ea.FactType)
	}
	if ea.SubjectID != subjectID {
		t.Errorf("SubjectID = %v, want %v", ea.SubjectID, subjectID)
	}
	if ea.Conclusion != "Born in 1850" {
		t.Errorf("Conclusion = %v, want 'Born in 1850'", ea.Conclusion)
	}
	if ea.Version != 1 {
		t.Errorf("Version = %v, want 1", ea.Version)
	}
}

func TestEvidenceAnalysis_Validate(t *testing.T) {
	tests := []struct {
		name    string
		ea      *EvidenceAnalysis
		wantErr bool
	}{
		{
			name:    "valid analysis",
			ea:      NewEvidenceAnalysis(FactPersonBirth, uuid.New(), "Born in 1850"),
			wantErr: false,
		},
		{
			name:    "missing subject_id",
			ea:      &EvidenceAnalysis{ID: uuid.New(), FactType: FactPersonBirth, Conclusion: "test"},
			wantErr: true,
		},
		{
			name:    "missing conclusion",
			ea:      &EvidenceAnalysis{ID: uuid.New(), FactType: FactPersonBirth, SubjectID: uuid.New()},
			wantErr: true,
		},
		{
			name:    "invalid fact_type",
			ea:      &EvidenceAnalysis{ID: uuid.New(), FactType: "invalid", SubjectID: uuid.New(), Conclusion: "test"},
			wantErr: true,
		},
		{
			name: "invalid research_status",
			ea: &EvidenceAnalysis{
				ID: uuid.New(), FactType: FactPersonBirth, SubjectID: uuid.New(),
				Conclusion: "test", ResearchStatus: "invalid",
			},
			wantErr: true,
		},
		{
			name: "valid with optional fields",
			ea: func() *EvidenceAnalysis {
				ea := NewEvidenceAnalysis(FactPersonBirth, uuid.New(), "Born in 1850")
				ea.CitationIDs = []uuid.UUID{uuid.New(), uuid.New()}
				ea.ResearchStatus = ResearchStatusProbable
				ea.Notes = "Supporting evidence from census"
				return ea
			}(),
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.ea.Validate()
			if (err != nil) != tt.wantErr {
				t.Errorf("Validate() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

// --- EvidenceConflict Tests ---

func TestNewEvidenceConflict(t *testing.T) {
	subjectID := uuid.New()
	analysisIDs := []uuid.UUID{uuid.New(), uuid.New()}
	ec := NewEvidenceConflict(FactPersonBirth, subjectID, analysisIDs, "Conflicting birth dates")

	if ec.ID == uuid.Nil {
		t.Error("Expected non-nil UUID")
	}
	if ec.FactType != FactPersonBirth {
		t.Errorf("FactType = %v, want person_birth", ec.FactType)
	}
	if ec.SubjectID != subjectID {
		t.Errorf("SubjectID = %v, want %v", ec.SubjectID, subjectID)
	}
	if len(ec.AnalysisIDs) != 2 {
		t.Errorf("AnalysisIDs length = %d, want 2", len(ec.AnalysisIDs))
	}
	if ec.Description != "Conflicting birth dates" {
		t.Errorf("Description = %v, want 'Conflicting birth dates'", ec.Description)
	}
	if ec.Status != ConflictStatusOpen {
		t.Errorf("Status = %v, want open", ec.Status)
	}
	if ec.Version != 1 {
		t.Errorf("Version = %v, want 1", ec.Version)
	}
}

func TestEvidenceConflict_Validate(t *testing.T) {
	tests := []struct {
		name    string
		ec      *EvidenceConflict
		wantErr bool
	}{
		{
			name:    "valid conflict",
			ec:      NewEvidenceConflict(FactPersonBirth, uuid.New(), []uuid.UUID{uuid.New(), uuid.New()}, "Conflicting dates"),
			wantErr: false,
		},
		{
			name: "missing subject_id",
			ec: &EvidenceConflict{
				ID: uuid.New(), FactType: FactPersonBirth, Description: "test",
				AnalysisIDs: []uuid.UUID{uuid.New(), uuid.New()}, Status: ConflictStatusOpen,
			},
			wantErr: true,
		},
		{
			name: "missing description",
			ec: &EvidenceConflict{
				ID: uuid.New(), FactType: FactPersonBirth, SubjectID: uuid.New(),
				AnalysisIDs: []uuid.UUID{uuid.New(), uuid.New()}, Status: ConflictStatusOpen,
			},
			wantErr: true,
		},
		{
			name: "too few analysis_ids",
			ec: &EvidenceConflict{
				ID: uuid.New(), FactType: FactPersonBirth, SubjectID: uuid.New(),
				AnalysisIDs: []uuid.UUID{uuid.New()}, Description: "test", Status: ConflictStatusOpen,
			},
			wantErr: true,
		},
		{
			name: "invalid fact_type",
			ec: &EvidenceConflict{
				ID: uuid.New(), FactType: "invalid", SubjectID: uuid.New(),
				AnalysisIDs: []uuid.UUID{uuid.New(), uuid.New()}, Description: "test", Status: ConflictStatusOpen,
			},
			wantErr: true,
		},
		{
			name: "invalid status",
			ec: &EvidenceConflict{
				ID: uuid.New(), FactType: FactPersonBirth, SubjectID: uuid.New(),
				AnalysisIDs: []uuid.UUID{uuid.New(), uuid.New()}, Description: "test", Status: "invalid",
			},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.ec.Validate()
			if (err != nil) != tt.wantErr {
				t.Errorf("Validate() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

// --- ResearchLog Tests ---

func TestNewResearchLog(t *testing.T) {
	subjectID := uuid.New()
	searchDate := time.Date(2024, 1, 15, 0, 0, 0, 0, time.UTC)
	rl := NewResearchLog(subjectID, "person", "National Archives", "Census 1850 search", ResearchOutcomeFound, searchDate)

	if rl.ID == uuid.Nil {
		t.Error("Expected non-nil UUID")
	}
	if rl.SubjectID != subjectID {
		t.Errorf("SubjectID = %v, want %v", rl.SubjectID, subjectID)
	}
	if rl.SubjectType != "person" {
		t.Errorf("SubjectType = %v, want 'person'", rl.SubjectType)
	}
	if rl.Repository != "National Archives" {
		t.Errorf("Repository = %v, want 'National Archives'", rl.Repository)
	}
	if rl.SearchDescription != "Census 1850 search" {
		t.Errorf("SearchDescription = %v, want 'Census 1850 search'", rl.SearchDescription)
	}
	if rl.Outcome != ResearchOutcomeFound {
		t.Errorf("Outcome = %v, want found", rl.Outcome)
	}
	if !rl.SearchDate.Equal(searchDate) {
		t.Errorf("SearchDate = %v, want %v", rl.SearchDate, searchDate)
	}
	if rl.Version != 1 {
		t.Errorf("Version = %v, want 1", rl.Version)
	}
}

func TestResearchLog_Validate(t *testing.T) {
	searchDate := time.Date(2024, 1, 15, 0, 0, 0, 0, time.UTC)

	tests := []struct {
		name    string
		rl      *ResearchLog
		wantErr bool
	}{
		{
			name:    "valid research log",
			rl:      NewResearchLog(uuid.New(), "person", "National Archives", "Census search", ResearchOutcomeFound, searchDate),
			wantErr: false,
		},
		{
			name:    "valid with family subject",
			rl:      NewResearchLog(uuid.New(), "family", "FamilySearch", "Marriage records", ResearchOutcomeNotFound, searchDate),
			wantErr: false,
		},
		{
			name: "missing subject_id",
			rl: &ResearchLog{
				ID: uuid.New(), SubjectType: "person", Repository: "test",
				SearchDescription: "test", Outcome: ResearchOutcomeFound, SearchDate: searchDate,
			},
			wantErr: true,
		},
		{
			name: "invalid subject_type",
			rl: &ResearchLog{
				ID: uuid.New(), SubjectID: uuid.New(), SubjectType: "source",
				Repository: "test", SearchDescription: "test", Outcome: ResearchOutcomeFound, SearchDate: searchDate,
			},
			wantErr: true,
		},
		{
			name: "missing repository",
			rl: &ResearchLog{
				ID: uuid.New(), SubjectID: uuid.New(), SubjectType: "person",
				SearchDescription: "test", Outcome: ResearchOutcomeFound, SearchDate: searchDate,
			},
			wantErr: true,
		},
		{
			name: "missing search_description",
			rl: &ResearchLog{
				ID: uuid.New(), SubjectID: uuid.New(), SubjectType: "person",
				Repository: "test", Outcome: ResearchOutcomeFound, SearchDate: searchDate,
			},
			wantErr: true,
		},
		{
			name: "invalid outcome",
			rl: &ResearchLog{
				ID: uuid.New(), SubjectID: uuid.New(), SubjectType: "person",
				Repository: "test", SearchDescription: "test", Outcome: "invalid", SearchDate: searchDate,
			},
			wantErr: true,
		},
		{
			name: "missing search_date",
			rl: &ResearchLog{
				ID: uuid.New(), SubjectID: uuid.New(), SubjectType: "person",
				Repository: "test", SearchDescription: "test", Outcome: ResearchOutcomeFound,
			},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.rl.Validate()
			if (err != nil) != tt.wantErr {
				t.Errorf("Validate() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

// --- ProofSummary Tests ---

func TestNewProofSummary(t *testing.T) {
	subjectID := uuid.New()
	ps := NewProofSummary(FactPersonBirth, subjectID, "Born 1850 in Illinois", "Based on census and church records...")

	if ps.ID == uuid.Nil {
		t.Error("Expected non-nil UUID")
	}
	if ps.FactType != FactPersonBirth {
		t.Errorf("FactType = %v, want person_birth", ps.FactType)
	}
	if ps.SubjectID != subjectID {
		t.Errorf("SubjectID = %v, want %v", ps.SubjectID, subjectID)
	}
	if ps.Conclusion != "Born 1850 in Illinois" {
		t.Errorf("Conclusion = %v, want 'Born 1850 in Illinois'", ps.Conclusion)
	}
	if ps.Argument != "Based on census and church records..." {
		t.Errorf("Argument = %v, want 'Based on census and church records...'", ps.Argument)
	}
	if ps.Version != 1 {
		t.Errorf("Version = %v, want 1", ps.Version)
	}
}

func TestProofSummary_Validate(t *testing.T) {
	tests := []struct {
		name    string
		ps      *ProofSummary
		wantErr bool
	}{
		{
			name:    "valid proof summary",
			ps:      NewProofSummary(FactPersonBirth, uuid.New(), "Born 1850", "Evidence shows..."),
			wantErr: false,
		},
		{
			name: "missing subject_id",
			ps: &ProofSummary{
				ID: uuid.New(), FactType: FactPersonBirth, Conclusion: "test", Argument: "test",
			},
			wantErr: true,
		},
		{
			name: "missing conclusion",
			ps: &ProofSummary{
				ID: uuid.New(), FactType: FactPersonBirth, SubjectID: uuid.New(), Argument: "test",
			},
			wantErr: true,
		},
		{
			name: "missing argument",
			ps: &ProofSummary{
				ID: uuid.New(), FactType: FactPersonBirth, SubjectID: uuid.New(), Conclusion: "test",
			},
			wantErr: true,
		},
		{
			name: "invalid fact_type",
			ps: &ProofSummary{
				ID: uuid.New(), FactType: "invalid", SubjectID: uuid.New(),
				Conclusion: "test", Argument: "test",
			},
			wantErr: true,
		},
		{
			name: "invalid research_status",
			ps: &ProofSummary{
				ID: uuid.New(), FactType: FactPersonBirth, SubjectID: uuid.New(),
				Conclusion: "test", Argument: "test", ResearchStatus: "invalid",
			},
			wantErr: true,
		},
		{
			name: "valid with optional fields",
			ps: func() *ProofSummary {
				ps := NewProofSummary(FactPersonBirth, uuid.New(), "Born 1850", "Evidence shows...")
				ps.AnalysisIDs = []uuid.UUID{uuid.New()}
				ps.ResearchStatus = ResearchStatusCertain
				return ps
			}(),
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.ps.Validate()
			if (err != nil) != tt.wantErr {
				t.Errorf("Validate() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

// --- Event Factory Tests ---

func TestNewEvidenceAnalysisCreated(t *testing.T) {
	ea := NewEvidenceAnalysis(FactPersonBirth, uuid.New(), "Born in 1850")
	ea.CitationIDs = []uuid.UUID{uuid.New()}
	ea.ResearchStatus = ResearchStatusProbable
	ea.Notes = "Census evidence"

	event := NewEvidenceAnalysisCreated(ea)

	if event.EventType() != "EvidenceAnalysisCreated" {
		t.Errorf("EventType() = %v, want EvidenceAnalysisCreated", event.EventType())
	}
	if event.AggregateID() != ea.ID {
		t.Errorf("AggregateID() = %v, want %v", event.AggregateID(), ea.ID)
	}
	if event.AnalysisID != ea.ID {
		t.Errorf("AnalysisID = %v, want %v", event.AnalysisID, ea.ID)
	}
	if event.FactType != ea.FactType {
		t.Errorf("FactType = %v, want %v", event.FactType, ea.FactType)
	}
	if event.Conclusion != ea.Conclusion {
		t.Errorf("Conclusion = %v, want %v", event.Conclusion, ea.Conclusion)
	}
	if event.OccurredAt().IsZero() {
		t.Error("OccurredAt() should not be zero")
	}
}

func TestNewEvidenceAnalysisUpdated(t *testing.T) {
	analysisID := uuid.New()
	changes := map[string]any{"conclusion": "Updated conclusion"}

	event := NewEvidenceAnalysisUpdated(analysisID, changes)

	if event.EventType() != "EvidenceAnalysisUpdated" {
		t.Errorf("EventType() = %v, want EvidenceAnalysisUpdated", event.EventType())
	}
	if event.AggregateID() != analysisID {
		t.Errorf("AggregateID() = %v, want %v", event.AggregateID(), analysisID)
	}
}

func TestNewEvidenceAnalysisDeleted(t *testing.T) {
	analysisID := uuid.New()
	event := NewEvidenceAnalysisDeleted(analysisID, "no longer relevant")

	if event.EventType() != "EvidenceAnalysisDeleted" {
		t.Errorf("EventType() = %v, want EvidenceAnalysisDeleted", event.EventType())
	}
	if event.AggregateID() != analysisID {
		t.Errorf("AggregateID() = %v, want %v", event.AggregateID(), analysisID)
	}
	if event.Reason != "no longer relevant" {
		t.Errorf("Reason = %v, want 'no longer relevant'", event.Reason)
	}
}

func TestNewEvidenceConflictDetected(t *testing.T) {
	ec := NewEvidenceConflict(FactPersonBirth, uuid.New(), []uuid.UUID{uuid.New(), uuid.New()}, "Conflicting dates")

	event := NewEvidenceConflictDetected(ec)

	if event.EventType() != "EvidenceConflictDetected" {
		t.Errorf("EventType() = %v, want EvidenceConflictDetected", event.EventType())
	}
	if event.AggregateID() != ec.ID {
		t.Errorf("AggregateID() = %v, want %v", event.AggregateID(), ec.ID)
	}
	if event.Description != ec.Description {
		t.Errorf("Description = %v, want %v", event.Description, ec.Description)
	}
	if event.Status != ConflictStatusOpen {
		t.Errorf("Status = %v, want open", event.Status)
	}
}

func TestNewEvidenceConflictResolved(t *testing.T) {
	conflictID := uuid.New()
	event := NewEvidenceConflictResolved(conflictID, "Census record is authoritative", ConflictStatusResolved)

	if event.EventType() != "EvidenceConflictResolved" {
		t.Errorf("EventType() = %v, want EvidenceConflictResolved", event.EventType())
	}
	if event.AggregateID() != conflictID {
		t.Errorf("AggregateID() = %v, want %v", event.AggregateID(), conflictID)
	}
	if event.Resolution != "Census record is authoritative" {
		t.Errorf("Resolution = %v, want 'Census record is authoritative'", event.Resolution)
	}
	if event.Status != ConflictStatusResolved {
		t.Errorf("Status = %v, want resolved", event.Status)
	}
}

func TestNewResearchLogCreated(t *testing.T) {
	searchDate := time.Date(2024, 1, 15, 0, 0, 0, 0, time.UTC)
	rl := NewResearchLog(uuid.New(), "person", "National Archives", "Census search", ResearchOutcomeFound, searchDate)
	rl.Notes = "Found matching record"

	event := NewResearchLogCreated(rl)

	if event.EventType() != "ResearchLogCreated" {
		t.Errorf("EventType() = %v, want ResearchLogCreated", event.EventType())
	}
	if event.AggregateID() != rl.ID {
		t.Errorf("AggregateID() = %v, want %v", event.AggregateID(), rl.ID)
	}
	if event.Repository != rl.Repository {
		t.Errorf("Repository = %v, want %v", event.Repository, rl.Repository)
	}
	if event.Outcome != ResearchOutcomeFound {
		t.Errorf("Outcome = %v, want found", event.Outcome)
	}
}

func TestNewResearchLogUpdated(t *testing.T) {
	logID := uuid.New()
	changes := map[string]any{"notes": "Updated notes"}

	event := NewResearchLogUpdated(logID, changes)

	if event.EventType() != "ResearchLogUpdated" {
		t.Errorf("EventType() = %v, want ResearchLogUpdated", event.EventType())
	}
	if event.AggregateID() != logID {
		t.Errorf("AggregateID() = %v, want %v", event.AggregateID(), logID)
	}
}

func TestNewResearchLogDeleted(t *testing.T) {
	logID := uuid.New()
	event := NewResearchLogDeleted(logID, "duplicate entry")

	if event.EventType() != "ResearchLogDeleted" {
		t.Errorf("EventType() = %v, want ResearchLogDeleted", event.EventType())
	}
	if event.AggregateID() != logID {
		t.Errorf("AggregateID() = %v, want %v", event.AggregateID(), logID)
	}
	if event.Reason != "duplicate entry" {
		t.Errorf("Reason = %v, want 'duplicate entry'", event.Reason)
	}
}

func TestNewProofSummaryCreated(t *testing.T) {
	ps := NewProofSummary(FactPersonBirth, uuid.New(), "Born 1850", "Evidence shows...")
	ps.AnalysisIDs = []uuid.UUID{uuid.New()}
	ps.ResearchStatus = ResearchStatusCertain

	event := NewProofSummaryCreated(ps)

	if event.EventType() != "ProofSummaryCreated" {
		t.Errorf("EventType() = %v, want ProofSummaryCreated", event.EventType())
	}
	if event.AggregateID() != ps.ID {
		t.Errorf("AggregateID() = %v, want %v", event.AggregateID(), ps.ID)
	}
	if event.Conclusion != ps.Conclusion {
		t.Errorf("Conclusion = %v, want %v", event.Conclusion, ps.Conclusion)
	}
	if event.Argument != ps.Argument {
		t.Errorf("Argument = %v, want %v", event.Argument, ps.Argument)
	}
}

func TestNewProofSummaryUpdated(t *testing.T) {
	summaryID := uuid.New()
	changes := map[string]any{"argument": "Updated argument"}

	event := NewProofSummaryUpdated(summaryID, changes)

	if event.EventType() != "ProofSummaryUpdated" {
		t.Errorf("EventType() = %v, want ProofSummaryUpdated", event.EventType())
	}
	if event.AggregateID() != summaryID {
		t.Errorf("AggregateID() = %v, want %v", event.AggregateID(), summaryID)
	}
}

func TestNewProofSummaryDeleted(t *testing.T) {
	summaryID := uuid.New()
	event := NewProofSummaryDeleted(summaryID, "superseded")

	if event.EventType() != "ProofSummaryDeleted" {
		t.Errorf("EventType() = %v, want ProofSummaryDeleted", event.EventType())
	}
	if event.AggregateID() != summaryID {
		t.Errorf("AggregateID() = %v, want %v", event.AggregateID(), summaryID)
	}
	if event.Reason != "superseded" {
		t.Errorf("Reason = %v, want 'superseded'", event.Reason)
	}
}
