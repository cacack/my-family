package sqlite_test

import (
	"context"
	"errors"
	"reflect"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/cacack/my-family/internal/domain"
	"github.com/cacack/my-family/internal/repository"
	"github.com/cacack/my-family/internal/repository/sqlite"
)

func TestSQLiteBranchStore_ResearchRecord(t *testing.T) {
	db := setupBranchTestDB(t)
	defer db.Close()

	store, err := sqlite.NewBranchStore(db)
	if err != nil {
		t.Fatalf("NewBranchStore() error = %v", err)
	}
	runBranchResearchContract(t, store)
}

// TestSQLiteBranchStore_ResearchMigration covers the pre-#835 upgrade path: a
// branches table without the research columns gains them on open (twice, to
// prove the swallowed duplicate-column error is the only failure), and an
// existing row reads as an open branch with no research record.
func TestSQLiteBranchStore_ResearchMigration(t *testing.T) {
	db := setupBranchTestDB(t)
	defer db.Close()

	if _, err := db.Exec(`
		CREATE TABLE branches (
			id TEXT PRIMARY KEY,
			name TEXT NOT NULL,
			description TEXT,
			base_position INTEGER NOT NULL,
			status TEXT NOT NULL,
			created_at TEXT NOT NULL,
			merged_at TEXT,
			merge_note TEXT
		)`); err != nil {
		t.Fatalf("create pre-#835 branches table: %v", err)
	}
	existing := uuid.New()
	if _, err := db.Exec(`INSERT INTO branches (id, name, base_position, status, created_at)
		VALUES (?, 'Pre-existing', 7, 'active', '2026-01-01T00:00:00Z')`, existing.String()); err != nil {
		t.Fatalf("seed pre-#835 branch: %v", err)
	}

	var store *sqlite.BranchStore
	for i := 0; i < 2; i++ {
		var err error
		if store, err = sqlite.NewBranchStore(db); err != nil {
			t.Fatalf("NewBranchStore pass %d: %v", i, err)
		}
	}

	ctx := context.Background()
	got, err := store.Get(ctx, existing)
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if got.Outcome != domain.BranchOutcomeOpen || got.Hypothesis != "" || got.Subjects != nil || got.ProofSummaryIDs != nil {
		t.Errorf("migrated branch = %+v, want open with no research record", got)
	}
	research := domain.BranchResearch{Hypothesis: "Q", Outcome: domain.BranchOutcomeProved}
	if err := store.UpdateDetails(ctx, existing, "", research); err != nil {
		t.Fatalf("UpdateDetails after migration: %v", err)
	}
	if got, err = store.Get(ctx, existing); err != nil || got.Outcome != domain.BranchOutcomeProved || got.Hypothesis != "Q" {
		t.Errorf("after UpdateDetails = %+v, %v", got, err)
	}
}

// TestSQLiteBranchStore_CorruptResearchLists proves a stored list that is not
// valid JSON surfaces as an error rather than an empty list.
func TestSQLiteBranchStore_CorruptResearchLists(t *testing.T) {
	db := setupBranchTestDB(t)
	defer db.Close()

	store, err := sqlite.NewBranchStore(db)
	if err != nil {
		t.Fatalf("NewBranchStore() error = %v", err)
	}
	id := uuid.New()
	if _, err := db.Exec(`INSERT INTO branches (id, name, base_position, status, created_at, subjects)
		VALUES (?, 'Corrupt', 1, 'active', '2026-01-01T00:00:00Z', '{not json')`, id.String()); err != nil {
		t.Fatalf("seed: %v", err)
	}
	if _, err := store.Get(context.Background(), id); err == nil {
		t.Error("Get of a corrupt subjects column succeeded, want an error")
	}
	if _, err := store.List(context.Background()); err == nil {
		t.Error("List with a corrupt subjects column succeeded, want an error")
	}
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
