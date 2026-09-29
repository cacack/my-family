package domain_test

import (
	"encoding/json"
	"errors"
	"strings"
	"testing"

	"github.com/google/uuid"

	"github.com/cacack/my-family/internal/domain"
)

func TestBranchOutcome_IsValid(t *testing.T) {
	for _, outcome := range domain.BranchOutcomes() {
		if !outcome.IsValid() {
			t.Errorf("%q.IsValid() = false, want true", outcome)
		}
	}
	for _, outcome := range []domain.BranchOutcome{"", "maybe", "Proved"} {
		if outcome.IsValid() {
			t.Errorf("%q.IsValid() = true, want false", outcome)
		}
	}
	if got := domain.BranchOutcome("").OrDefault(); got != domain.BranchOutcomeOpen {
		t.Errorf(`"".OrDefault() = %q, want open`, got)
	}
	if got := domain.BranchOutcomeProved.OrDefault(); got != domain.BranchOutcomeProved {
		t.Errorf("proved.OrDefault() = %q, want proved", got)
	}
}

func TestBranchResearch_Validate(t *testing.T) {
	person := domain.BranchSubject{Type: domain.BranchSubjectPerson, ID: uuid.New()}
	proof := uuid.New()
	tooManySubjects := make([]domain.BranchSubject, domain.MaxBranchSubjects+1)
	for i := range tooManySubjects {
		tooManySubjects[i] = domain.BranchSubject{Type: domain.BranchSubjectFamily, ID: uuid.New()}
	}
	tooManyProofs := make([]uuid.UUID, domain.MaxBranchProofSummaries+1)
	for i := range tooManyProofs {
		tooManyProofs[i] = uuid.New()
	}

	tests := []struct {
		name     string
		research domain.BranchResearch
		want     error
	}{
		{"minimal", domain.BranchResearch{Outcome: domain.BranchOutcomeOpen}, nil},
		{"full", domain.BranchResearch{
			Hypothesis: "Q", Subjects: []domain.BranchSubject{person}, Outcome: domain.BranchOutcomeProved,
			ProofSummaryIDs: []uuid.UUID{proof},
		}, nil},
		{"hypothesis at limit (multibyte)", domain.BranchResearch{
			Hypothesis: strings.Repeat("é", domain.MaxBranchHypothesisLength), Outcome: domain.BranchOutcomeOpen,
		}, nil},
		{"hypothesis too long", domain.BranchResearch{
			Hypothesis: strings.Repeat("a", domain.MaxBranchHypothesisLength+1), Outcome: domain.BranchOutcomeOpen,
		}, domain.ErrBranchHypothesisTooLong},
		{"empty outcome", domain.BranchResearch{}, domain.ErrBranchInvalidOutcome},
		{"unknown outcome", domain.BranchResearch{Outcome: "maybe"}, domain.ErrBranchInvalidOutcome},
		{"too many subjects", domain.BranchResearch{Outcome: domain.BranchOutcomeOpen, Subjects: tooManySubjects}, domain.ErrBranchTooManySubjects},
		{"subject bad type", domain.BranchResearch{Outcome: domain.BranchOutcomeOpen,
			Subjects: []domain.BranchSubject{{Type: "source", ID: uuid.New()}}}, domain.ErrBranchInvalidSubject},
		{"subject nil id", domain.BranchResearch{Outcome: domain.BranchOutcomeOpen,
			Subjects: []domain.BranchSubject{{Type: domain.BranchSubjectPerson}}}, domain.ErrBranchInvalidSubject},
		{"duplicate subject", domain.BranchResearch{Outcome: domain.BranchOutcomeOpen,
			Subjects: []domain.BranchSubject{person, person}}, domain.ErrBranchDuplicateSubject},
		{"too many proofs", domain.BranchResearch{Outcome: domain.BranchOutcomeOpen, ProofSummaryIDs: tooManyProofs}, domain.ErrBranchTooManyProofSummaries},
		{"nil proof id", domain.BranchResearch{Outcome: domain.BranchOutcomeOpen, ProofSummaryIDs: []uuid.UUID{uuid.Nil}}, domain.ErrBranchInvalidProofSummaryID},
		{"duplicate proof", domain.BranchResearch{Outcome: domain.BranchOutcomeOpen, ProofSummaryIDs: []uuid.UUID{proof, proof}}, domain.ErrBranchDuplicateProofSummary},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if err := tt.research.Validate(); !errors.Is(err, tt.want) {
				t.Errorf("Validate() = %v, want %v", err, tt.want)
			}
		})
	}
}

func TestNewBranchWithResearch(t *testing.T) {
	subject := domain.BranchSubject{Type: domain.BranchSubjectFamily, ID: uuid.New()}
	b, err := domain.NewBranchWithResearch("line", "desc", 5, domain.BranchResearch{
		Hypothesis: "Q", Subjects: []domain.BranchSubject{subject},
	})
	if err != nil {
		t.Fatalf("NewBranchWithResearch: %v", err)
	}
	if b.Outcome != domain.BranchOutcomeOpen || b.Hypothesis != "Q" || len(b.Subjects) != 1 || b.Subjects[0] != subject {
		t.Errorf("branch = %+v", b)
	}

	if _, err := domain.NewBranchWithResearch("", "", 0, domain.BranchResearch{}); !errors.Is(err, domain.ErrBranchNameRequired) {
		t.Errorf("empty name = %v, want ErrBranchNameRequired", err)
	}
	if _, err := domain.NewBranchWithResearch("line", "", 0, domain.BranchResearch{Outcome: "maybe"}); !errors.Is(err, domain.ErrBranchInvalidOutcome) {
		t.Errorf("bad outcome = %v, want ErrBranchInvalidOutcome", err)
	}

	// A branch with no outcome at all (a pre-#835 row) still validates.
	legacy := &domain.Branch{Name: "old", Status: domain.BranchStatusActive}
	if err := legacy.Validate(); err != nil {
		t.Errorf("legacy branch Validate() = %v, want nil", err)
	}

	// Research() copies: mutating the result does not reach the branch.
	research := b.Research()
	research.Subjects[0].ID = uuid.New()
	if b.Subjects[0] != subject {
		t.Error("Research() shares its subjects slice with the branch")
	}
}

func TestBranchUpdated_RoundTrip(t *testing.T) {
	b := &domain.Branch{
		ID:              uuid.New(),
		Description:     "d",
		Hypothesis:      "Q",
		Subjects:        []domain.BranchSubject{{Type: domain.BranchSubjectPerson, ID: uuid.New()}},
		Outcome:         domain.BranchOutcomeDisproved,
		ProofSummaryIDs: []uuid.UUID{uuid.New()},
	}
	event := domain.NewBranchUpdated(b, []string{"outcome", "hypothesis"})
	if event.EventType() != "BranchUpdated" || event.AggregateID() != b.ID {
		t.Errorf("EventType/AggregateID = %s/%s", event.EventType(), event.AggregateID())
	}

	data, err := json.Marshal(event)
	if err != nil {
		t.Fatalf("Marshal: %v", err)
	}
	var decoded domain.BranchUpdated
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatalf("Unmarshal: %v", err)
	}
	research := decoded.Research()
	if decoded.Description != "d" || research.Hypothesis != "Q" || research.Outcome != domain.BranchOutcomeDisproved ||
		len(research.Subjects) != 1 || research.Subjects[0] != b.Subjects[0] ||
		len(research.ProofSummaryIDs) != 1 || research.ProofSummaryIDs[0] != b.ProofSummaryIDs[0] ||
		len(decoded.ChangedFields) != 2 {
		t.Errorf("decoded = %+v", decoded)
	}

	// Empty lists are stored as [] rather than null, and an empty outcome as open.
	empty, err := json.Marshal(domain.NewBranchUpdated(&domain.Branch{ID: uuid.New()}, nil))
	if err != nil {
		t.Fatalf("Marshal: %v", err)
	}
	for _, want := range []string{`"subjects":[]`, `"proof_summary_ids":[]`, `"changed_fields":[]`, `"outcome":"open"`} {
		if !strings.Contains(string(empty), want) {
			t.Errorf("empty event %s lacks %s", empty, want)
		}
	}
}

func TestBranchCreated_CarriesResearch(t *testing.T) {
	b, err := domain.NewBranchWithResearch("line", "", 1, domain.BranchResearch{
		Hypothesis: "Q", Outcome: domain.BranchOutcomeSuperseded, ProofSummaryIDs: []uuid.UUID{uuid.New()},
	})
	if err != nil {
		t.Fatalf("NewBranchWithResearch: %v", err)
	}
	research := domain.NewBranchCreated(b).Research()
	if research.Hypothesis != "Q" || research.Outcome != domain.BranchOutcomeSuperseded || len(research.ProofSummaryIDs) != 1 {
		t.Errorf("research = %+v", research)
	}

	// A pre-#835 event has no research fields and reads as open.
	var legacy domain.BranchCreated
	if err := json.Unmarshal([]byte(`{"branch_id":"`+uuid.NewString()+`","name":"old","base_position":1}`), &legacy); err != nil {
		t.Fatalf("Unmarshal: %v", err)
	}
	if got := legacy.Research(); got.Outcome != domain.BranchOutcomeOpen || got.Subjects != nil {
		t.Errorf("legacy research = %+v", got)
	}
}

func TestBranchOutcome_Close(t *testing.T) {
	if !domain.BranchOutcomeAbandoned.IsValid() {
		t.Error("abandoned is not a valid outcome")
	}
	closing := map[domain.BranchOutcome]bool{
		domain.BranchOutcomeDisproved: true, domain.BranchOutcomeInconclusive: true,
		domain.BranchOutcomeSuperseded: true, domain.BranchOutcomeAbandoned: true,
	}
	for _, outcome := range append(domain.BranchOutcomes(), "", "bogus") {
		if got := outcome.IsCloseOutcome(); got != closing[outcome] {
			t.Errorf("%q.IsCloseOutcome() = %v, want %v", outcome, got, closing[outcome])
		}
		err := domain.ValidateBranchClose(outcome, "")
		if closing[outcome] != (err == nil) {
			t.Errorf("ValidateBranchClose(%q) = %v", outcome, err)
		}
	}
	if err := domain.ValidateBranchClose(domain.BranchOutcomeDisproved, strings.Repeat("é", domain.MaxBranchCloseReasonLength)); err != nil {
		t.Errorf("reason at the limit = %v, want nil", err)
	}
	if err := domain.ValidateBranchClose(domain.BranchOutcomeDisproved, strings.Repeat("é", domain.MaxBranchCloseReasonLength+1)); !errors.Is(err, domain.ErrBranchCloseReasonTooLong) {
		t.Errorf("reason over the limit = %v, want ErrBranchCloseReasonTooLong", err)
	}

	b, err := domain.NewBranch("x", "", 0)
	if err != nil {
		t.Fatalf("NewBranch: %v", err)
	}
	b.CloseReason = strings.Repeat("a", domain.MaxBranchCloseReasonLength+1)
	if err := b.Validate(); !errors.Is(err, domain.ErrBranchCloseReasonTooLong) {
		t.Errorf("Branch.Validate with a long close reason = %v", err)
	}
}

func TestBranchDeleted_CloseRecordIsBackwardCompatible(t *testing.T) {
	id := uuid.New()
	closed := domain.NewBranchClosed(id, domain.BranchOutcomeDisproved, "because")
	data, err := json.Marshal(closed)
	if err != nil {
		t.Fatalf("Marshal: %v", err)
	}
	var decoded domain.BranchDeleted
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatalf("Unmarshal: %v", err)
	}
	if decoded.BranchID != id || decoded.Outcome != domain.BranchOutcomeDisproved || decoded.Reason != "because" {
		t.Errorf("round trip = %+v", decoded)
	}
	if closed.EventType() != "BranchDeleted" || closed.AggregateID() != id {
		t.Errorf("event identity = %s %s", closed.EventType(), closed.AggregateID())
	}

	// A pre-#836 payload has neither field; the legacy constructor writes none.
	legacy, err := json.Marshal(domain.NewBranchDeleted(id))
	if err != nil {
		t.Fatalf("Marshal legacy: %v", err)
	}
	if strings.Contains(string(legacy), "outcome") || strings.Contains(string(legacy), "reason") {
		t.Errorf("legacy payload carries close fields: %s", legacy)
	}
	var old domain.BranchDeleted
	if err := json.Unmarshal([]byte(`{"branch_id":"`+id.String()+`"}`), &old); err != nil || old.Outcome != "" || old.Reason != "" {
		t.Errorf("pre-#836 payload decoded = %+v, %v", old, err)
	}
}
