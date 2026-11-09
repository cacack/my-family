package memory_test

import (
	"context"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/cacack/my-family/internal/domain"
	"github.com/cacack/my-family/internal/repository"
	"github.com/cacack/my-family/internal/repository/memory"
)

func TestBranchScenario_DuplicateDismissals(t *testing.T) {
	runDuplicateDismissalScenario(t, memory.NewReadModelStore(), memory.NewBranchStore())
}

// runDuplicateDismissalScenario is the backend-agnostic duplicate dismissal
// scenario (#1036). Each backend package carries an identical copy; keeping the
// assertions byte-identical is the DB-001 parity guarantee.
//
// Shape: main dismisses two pairs, restores one and dismisses it again. A
// branch sees main's dismissals, restores one of them (a tombstone over main's
// row) and dismisses a pair of its own; main is untouched throughout. Deleting
// the branch purges its rows, so the branch id resolves to main again.
func runDuplicateDismissalScenario(t *testing.T, readStore repository.ReadModelStore, branchStore repository.BranchStore) {
	t.Helper()
	ctx := context.Background()
	projector := repository.NewProjector(readStore, branchStore)
	main := domain.MainBranchID

	newPair := func() domain.DuplicatePair {
		t.Helper()
		pair, err := domain.NewDuplicatePair(uuid.New(), uuid.New())
		if err != nil {
			t.Fatalf("NewDuplicatePair: %v", err)
		}
		return pair
	}
	at := time.Date(2026, 10, 12, 9, 0, 0, 0, time.UTC)
	dismissed := func(pair domain.DuplicatePair, reason string) domain.DuplicateDismissed {
		e := domain.NewDuplicateDismissed(pair, reason)
		at = at.Add(time.Minute)
		e.Timestamp = at
		return e
	}
	project := func(label string, branchID domain.BranchID, version int64, ev domain.Event) {
		t.Helper()
		if err := projector.Project(ctx, ev, version, branchID); err != nil {
			t.Fatalf("%s: project %s: %v", label, ev.EventType(), err)
		}
	}
	listed := func(branchID domain.BranchID) []uuid.UUID {
		t.Helper()
		rows, total, err := readStore.ListDuplicateDismissals(ctx, repository.ListOptions{Limit: 100, Sort: "created_at", Order: "asc", BranchID: branchID})
		if err != nil {
			t.Fatalf("ListDuplicateDismissals: %v", err)
		}
		if total != len(rows) {
			t.Fatalf("ListDuplicateDismissals total = %d, rows = %d", total, len(rows))
		}
		ids := make([]uuid.UUID, len(rows))
		for i := range rows {
			ids[i] = rows[i].ID
		}
		return ids
	}
	assertListed := func(label string, branchID domain.BranchID, want ...domain.DuplicatePair) {
		t.Helper()
		got := listed(branchID)
		if len(got) != len(want) {
			t.Fatalf("%s: listed %d dismissals, want %d", label, len(got), len(want))
		}
		for i := range want {
			if got[i] != want[i].ID() {
				t.Errorf("%s: dismissal %d = %s, want %s", label, i, got[i], want[i].ID())
			}
		}
	}

	branch, err := domain.NewBranch("dismissal-scope", "duplicate dismissals must fork", 0)
	if err != nil {
		t.Fatalf("NewBranch: %v", err)
	}
	project("create branch", main, 1, domain.NewBranchCreated(branch))
	branchID := domain.BranchID(branch.ID)

	// Main: dismiss two pairs; restore the first and dismiss it again.
	kept, redone := newPair(), newPair()
	project("main dismiss kept", main, 1, dismissed(kept, "father and son"))
	project("main dismiss redone", main, 1, dismissed(redone, ""))
	project("main restore redone", main, 2, domain.NewDuplicateRestored(redone))
	assertListed("main after restore", main, kept)
	project("main re-dismiss redone", main, 3, dismissed(redone, "cousins"))
	assertListed("main after re-dismiss", main, kept, redone)

	got, err := readStore.GetDuplicateDismissal(ctx, main, kept.ID())
	if err != nil || got == nil {
		t.Fatalf("GetDuplicateDismissal(main, kept) = %v, %v", got, err)
	}
	if got.Person1ID != kept.Person1ID || got.Person2ID != kept.Person2ID || got.Reason != "father and son" || got.Version != 1 {
		t.Errorf("main kept dismissal = %+v", got)
	}
	if got, err := readStore.GetDuplicateDismissal(ctx, main, redone.ID()); err != nil || got == nil || got.Reason != "cousins" || got.Version != 3 {
		t.Errorf("main redone dismissal = %+v (err=%v), want reason cousins at version 3", got, err)
	}

	// Branch: sees main's dismissals, restores one, dismisses its own pair.
	assertListed("branch before edits", branchID, kept, redone)
	own := newPair()
	project("branch restore kept", branchID, 2, domain.NewDuplicateRestored(kept))
	project("branch dismiss own", branchID, 1, dismissed(own, ""))
	assertListed("branch after edits", branchID, redone, own)
	if got, err := readStore.GetDuplicateDismissal(ctx, branchID, kept.ID()); err != nil || got != nil {
		t.Errorf("branch kept dismissal = %+v (err=%v), want tombstoned", got, err)
	}
	assertListed("main after branch edits", main, kept, redone)
	if got, err := readStore.GetDuplicateDismissal(ctx, main, own.ID()); err != nil || got != nil {
		t.Errorf("main own dismissal = %+v (err=%v), want none", got, err)
	}

	// The branch re-dismisses the pair it restored: its own row wins again.
	project("branch re-dismiss kept", branchID, 3, dismissed(kept, "on the branch"))
	if got, err := readStore.GetDuplicateDismissal(ctx, branchID, kept.ID()); err != nil || got == nil || got.Reason != "on the branch" {
		t.Errorf("branch re-dismissed kept = %+v (err=%v)", got, err)
	}
	if got, err := readStore.GetDuplicateDismissal(ctx, main, kept.ID()); err != nil || got == nil || got.Reason != "father and son" {
		t.Errorf("main kept after branch re-dismiss = %+v (err=%v)", got, err)
	}

	// Deleting the branch drops its rows: it resolves to main again.
	project("delete branch", main, 2, domain.NewBranchDeleted(branch.ID))
	assertListed("purged branch", branchID, kept, redone)
}
