package memory_test

import (
	"context"
	"errors"
	"reflect"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/cacack/my-family/internal/domain"
	"github.com/cacack/my-family/internal/repository"
	"github.com/cacack/my-family/internal/repository/memory"
)

func TestBranchStore_ResearchRecord(t *testing.T) {
	runBranchResearchContract(t, memory.NewBranchStore())
}

// runBranchResearchContract pins the #835 research record on a BranchStore:
// Create/Upsert/Get/List round-trip it, UpdateDetails overwrites it (and the
// description) without touching status or the merge record, clearing works,
// and an empty outcome is stored and read as open. The body is an identical
// copy in the memory, sqlite and postgres test packages so the three backends
// prove the same behavior (DB-001).
func runBranchResearchContract(t *testing.T, store repository.BranchStore) {
	t.Helper()
	ctx := context.Background()

	personID, familyID, proofA, proofB := uuid.New(), uuid.New(), uuid.New(), uuid.New()
	branch := &domain.Branch{
		ID:           uuid.New(),
		Name:         "Research",
		BasePosition: 3,
		Status:       domain.BranchStatusActive,
		CreatedAt:    time.Now().UTC().Truncate(time.Millisecond),
		Hypothesis:   "Was Person A the child of Family B?",
		Subjects: []domain.BranchSubject{
			{Type: domain.BranchSubjectPerson, ID: personID},
			{Type: domain.BranchSubjectFamily, ID: familyID},
		},
		Outcome:         domain.BranchOutcomeInconclusive,
		ProofSummaryIDs: []uuid.UUID{proofA, proofB},
	}
	if err := store.Create(ctx, branch); err != nil {
		t.Fatalf("Create: %v", err)
	}

	assertResearch := func(label string, got *domain.Branch, want domain.BranchResearch) {
		t.Helper()
		if got.Hypothesis != want.Hypothesis {
			t.Errorf("%s: Hypothesis = %q, want %q", label, got.Hypothesis, want.Hypothesis)
		}
		if got.Outcome != want.Outcome {
			t.Errorf("%s: Outcome = %q, want %q", label, got.Outcome, want.Outcome)
		}
		if !reflect.DeepEqual(got.Subjects, want.Subjects) {
			t.Errorf("%s: Subjects = %v, want %v", label, got.Subjects, want.Subjects)
		}
		if !reflect.DeepEqual(got.ProofSummaryIDs, want.ProofSummaryIDs) {
			t.Errorf("%s: ProofSummaryIDs = %v, want %v", label, got.ProofSummaryIDs, want.ProofSummaryIDs)
		}
	}

	got, err := store.Get(ctx, branch.ID)
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	assertResearch("Get after Create", got, branch.Research())

	list, err := store.List(ctx)
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(list) != 1 {
		t.Fatalf("List returned %d branches, want 1", len(list))
	}
	assertResearch("List after Create", list[0], branch.Research())

	// The store hands out copies: mutating one must not reach the store.
	got.Subjects[0].ID = uuid.New()
	got.ProofSummaryIDs[0] = uuid.New()
	if again, err := store.Get(ctx, branch.ID); err != nil {
		t.Fatalf("Get: %v", err)
	} else {
		assertResearch("Get after mutating a copy", again, branch.Research())
	}

	// UpdateDetails overwrites description and research, and leaves the merge
	// record and status alone.
	mergedAt := time.Now().UTC().Truncate(time.Millisecond)
	if err := store.MarkMerged(ctx, branch.ID, mergedAt, "merged"); err != nil {
		t.Fatalf("MarkMerged: %v", err)
	}
	updated := domain.BranchResearch{
		Hypothesis:      "Person A was the child of Family B",
		Subjects:        []domain.BranchSubject{{Type: domain.BranchSubjectFamily, ID: familyID}},
		Outcome:         domain.BranchOutcomeProved,
		ProofSummaryIDs: []uuid.UUID{proofB},
	}
	if err := store.UpdateDetails(ctx, branch.ID, "described", updated); err != nil {
		t.Fatalf("UpdateDetails: %v", err)
	}
	got, err = store.Get(ctx, branch.ID)
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	assertResearch("Get after UpdateDetails", got, updated)
	if got.Description != "described" {
		t.Errorf("Description = %q, want described", got.Description)
	}
	if got.Status != domain.BranchStatusMerged || got.MergeNote != "merged" || got.MergedAt == nil || !got.MergedAt.Equal(mergedAt) {
		t.Errorf("UpdateDetails disturbed the merge record: %+v", got)
	}

	// Clearing: empty strings and lists, and an empty outcome, read back as
	// absent and open.
	if err := store.UpdateDetails(ctx, branch.ID, "", domain.BranchResearch{}); err != nil {
		t.Fatalf("UpdateDetails clear: %v", err)
	}
	got, err = store.Get(ctx, branch.ID)
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	assertResearch("Get after clearing", got, domain.BranchResearch{Outcome: domain.BranchOutcomeOpen})
	if got.Description != "" {
		t.Errorf("Description = %q, want empty", got.Description)
	}

	if err := store.UpdateDetails(ctx, uuid.New(), "", domain.BranchResearch{}); !errors.Is(err, repository.ErrBranchNotFound) {
		t.Errorf("UpdateDetails unknown branch = %v, want ErrBranchNotFound", err)
	}

	// Upsert round-trips the record too, and an unset outcome is stored as open.
	upserted := &domain.Branch{
		ID:           uuid.New(),
		Name:         "Upserted",
		Status:       domain.BranchStatusActive,
		CreatedAt:    time.Now().UTC().Truncate(time.Millisecond),
		Hypothesis:   "Upserted question",
		Subjects:     []domain.BranchSubject{{Type: domain.BranchSubjectPerson, ID: personID}},
		BasePosition: 4,
	}
	if err := store.Upsert(ctx, upserted); err != nil {
		t.Fatalf("Upsert: %v", err)
	}
	upserted.Outcome = domain.BranchOutcomeDisproved
	upserted.ProofSummaryIDs = []uuid.UUID{proofA}
	if err := store.Upsert(ctx, upserted); err != nil {
		t.Fatalf("Upsert again: %v", err)
	}
	got, err = store.Get(ctx, upserted.ID)
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	assertResearch("Get after Upsert", got, upserted.Research())

	unset := &domain.Branch{ID: uuid.New(), Name: "Unset", Status: domain.BranchStatusActive, CreatedAt: time.Now().UTC()}
	if err := store.Create(ctx, unset); err != nil {
		t.Fatalf("Create unset: %v", err)
	}
	if got, err = store.Get(ctx, unset.ID); err != nil || got.Outcome != domain.BranchOutcomeOpen ||
		got.Subjects != nil || got.ProofSummaryIDs != nil || got.Hypothesis != "" {
		t.Errorf("unset branch = %+v, %v; want open with no research", got, err)
	}
}
