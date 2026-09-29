package repository_test

import (
	"testing"

	"github.com/google/uuid"

	"github.com/cacack/my-family/internal/domain"
	"github.com/cacack/my-family/internal/repository"
)

func TestBranchListsJSON_RoundTrip(t *testing.T) {
	research := domain.BranchResearch{
		Hypothesis:      "Q",
		Subjects:        []domain.BranchSubject{{Type: domain.BranchSubjectPerson, ID: uuid.New()}},
		Outcome:         domain.BranchOutcomeProved,
		ProofSummaryIDs: []uuid.UUID{uuid.New()},
	}
	lists, err := repository.EncodeBranchLists(research)
	if err != nil {
		t.Fatalf("EncodeBranchLists: %v", err)
	}
	got, err := repository.DecodeBranchResearch("Q", "proved", lists)
	if err != nil {
		t.Fatalf("DecodeBranchResearch: %v", err)
	}
	if got.Hypothesis != "Q" || got.Outcome != domain.BranchOutcomeProved ||
		len(got.Subjects) != 1 || got.Subjects[0] != research.Subjects[0] ||
		len(got.ProofSummaryIDs) != 1 || got.ProofSummaryIDs[0] != research.ProofSummaryIDs[0] {
		t.Errorf("round trip = %+v, want %+v", got, research)
	}

	empty, err := repository.EncodeBranchLists(domain.BranchResearch{})
	if err != nil || empty.Subjects != "" || empty.ProofSummaryIDs != "" {
		t.Errorf("empty lists = %+v, %v; want both empty", empty, err)
	}
	got, err = repository.DecodeBranchResearch("", "", repository.BranchListsJSON{Subjects: "[]", ProofSummaryIDs: "[]"})
	if err != nil || got.Outcome != domain.BranchOutcomeOpen || got.Subjects != nil || got.ProofSummaryIDs != nil {
		t.Errorf("decode of [] = %+v, %v; want open with nil lists", got, err)
	}
}

func TestDecodeBranchResearch_Corrupt(t *testing.T) {
	if _, err := repository.DecodeBranchResearch("", "", repository.BranchListsJSON{Subjects: "{"}); err == nil {
		t.Error("corrupt subjects decoded without error")
	}
	if _, err := repository.DecodeBranchResearch("", "", repository.BranchListsJSON{ProofSummaryIDs: "{"}); err == nil {
		t.Error("corrupt proof summary ids decoded without error")
	}
}
