package command_test

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/google/uuid"

	"github.com/cacack/my-family/internal/command"
	"github.com/cacack/my-family/internal/domain"
	"github.com/cacack/my-family/internal/repository"
)

// dismissalOn reads the dismissal of the pair (a, b) on branchID.
func dismissalOn(t *testing.T, rs repository.ReadModelStore, branchID domain.BranchID, a, b uuid.UUID) *repository.DuplicateDismissalReadModel {
	t.Helper()
	pair, err := domain.NewDuplicatePair(a, b)
	if err != nil {
		t.Fatalf("NewDuplicatePair: %v", err)
	}
	d, err := rs.GetDuplicateDismissal(context.Background(), branchID, pair.ID())
	if err != nil {
		t.Fatalf("GetDuplicateDismissal: %v", err)
	}
	return d
}

func newDismissalPerson(t *testing.T, h *command.Handler, given string) uuid.UUID {
	t.Helper()
	res, err := h.CreatePerson(context.Background(), command.CreatePersonInput{GivenName: given, Surname: "Doe"})
	if err != nil {
		t.Fatalf("CreatePerson(%s): %v", given, err)
	}
	return res.ID
}

// TestDismissDuplicate_PersistsAndRestores is the #1036 regression: dismissing
// a pair records it (in either order of the two persons), and restoring it
// removes the record again, each through an event on the pair's own stream.
func TestDismissDuplicate_PersistsAndRestores(t *testing.T) {
	f := newBranchFixture()
	ctx := context.Background()
	father := newDismissalPerson(t, f.handler, "John")
	son := newDismissalPerson(t, f.handler, "John")

	if err := f.handler.DismissDuplicate(ctx, command.DismissDuplicateInput{Person1ID: son, Person2ID: father, Reason: "father and son"}); err != nil {
		t.Fatalf("DismissDuplicate: %v", err)
	}
	d := dismissalOn(t, f.readStore, domain.MainBranchID, father, son)
	if d == nil {
		t.Fatal("dismissal not recorded")
	}
	if d.Reason != "father and son" {
		t.Errorf("Reason = %q, want %q", d.Reason, "father and son")
	}

	// Dismissing an already dismissed pair is a no-op, not a second event.
	pair, _ := domain.NewDuplicatePair(father, son)
	if err := f.handler.DismissDuplicate(ctx, command.DismissDuplicateInput{Person1ID: father, Person2ID: son}); err != nil {
		t.Fatalf("second DismissDuplicate: %v", err)
	}
	events, err := f.eventStore.ReadStream(ctx, pair.ID())
	if err != nil {
		t.Fatalf("ReadStream: %v", err)
	}
	if len(events) != 1 || events[0].EventType != "DuplicateDismissed" {
		t.Fatalf("pair stream = %d events, want one DuplicateDismissed", len(events))
	}

	if err := f.handler.RestoreDuplicate(ctx, father, son); err != nil {
		t.Fatalf("RestoreDuplicate: %v", err)
	}
	if d := dismissalOn(t, f.readStore, domain.MainBranchID, father, son); d != nil {
		t.Errorf("dismissal after restore = %+v, want none", d)
	}
	if err := f.handler.RestoreDuplicate(ctx, father, son); !errors.Is(err, command.ErrDuplicateNotDismissed) {
		t.Errorf("second RestoreDuplicate error = %v, want ErrDuplicateNotDismissed", err)
	}

	// The pair can be dismissed again after a restore.
	if err := f.handler.DismissDuplicate(ctx, command.DismissDuplicateInput{Person1ID: father, Person2ID: son}); err != nil {
		t.Fatalf("re-dismiss: %v", err)
	}
	if d := dismissalOn(t, f.readStore, domain.MainBranchID, father, son); d == nil {
		t.Error("re-dismissed pair not recorded")
	}
}

func TestDismissDuplicate_Validation(t *testing.T) {
	f := newBranchFixture()
	ctx := context.Background()
	a := newDismissalPerson(t, f.handler, "Ann")

	if err := f.handler.DismissDuplicate(ctx, command.DismissDuplicateInput{Person1ID: a, Person2ID: a}); !errors.Is(err, command.ErrInvalidInput) {
		t.Errorf("same person: error = %v, want ErrInvalidInput", err)
	}
	if err := f.handler.DismissDuplicate(ctx, command.DismissDuplicateInput{Person1ID: a, Person2ID: uuid.New()}); !errors.Is(err, command.ErrPersonNotFound) {
		t.Errorf("missing person: error = %v, want ErrPersonNotFound", err)
	}
	b := newDismissalPerson(t, f.handler, "Bea")
	long := strings.Repeat("x", domain.MaxDismissalReasonLength+1)
	if err := f.handler.DismissDuplicate(ctx, command.DismissDuplicateInput{Person1ID: a, Person2ID: b, Reason: long}); !errors.Is(err, command.ErrInvalidInput) {
		t.Errorf("long reason: error = %v, want ErrInvalidInput", err)
	}
	if err := f.handler.RestoreDuplicate(ctx, a, a); !errors.Is(err, command.ErrInvalidInput) {
		t.Errorf("restore same person: error = %v, want ErrInvalidInput", err)
	}
}

// TestBranchDuplicateDismissal: a dismissal made on a branch stays on it, a
// branch restore of a mainline dismissal leaves main's alone, and a merge
// carries the branch's dismissals to main (ADR-005).
func TestBranchDuplicateDismissal(t *testing.T) {
	f := newBranchFixture()
	ctx := context.Background()
	a := newDismissalPerson(t, f.handler, "Ann")
	b := newDismissalPerson(t, f.handler, "Ann")
	c := newDismissalPerson(t, f.handler, "Cal")
	d := newDismissalPerson(t, f.handler, "Cal")
	if err := f.handler.DismissDuplicate(ctx, command.DismissDuplicateInput{Person1ID: c, Person2ID: d}); err != nil {
		t.Fatalf("main DismissDuplicate: %v", err)
	}

	branch, err := f.handler.CreateBranch(ctx, "dismissals", "")
	if err != nil {
		t.Fatalf("CreateBranch: %v", err)
	}
	scoped := f.handler.WithBranch(branch)
	branchID := domain.BranchID(branch.ID)

	if dismissalOn(t, f.readStore, branchID, c, d) == nil {
		t.Fatal("branch does not see main's dismissal")
	}
	if err := scoped.DismissDuplicate(ctx, command.DismissDuplicateInput{Person1ID: a, Person2ID: b}); err != nil {
		t.Fatalf("branch DismissDuplicate: %v", err)
	}
	if err := scoped.RestoreDuplicate(ctx, c, d); err != nil {
		t.Fatalf("branch RestoreDuplicate: %v", err)
	}
	if dismissalOn(t, f.readStore, branchID, a, b) == nil {
		t.Error("branch dismissal not visible on the branch")
	}
	if dismissalOn(t, f.readStore, domain.MainBranchID, a, b) != nil {
		t.Error("branch dismissal leaked to main")
	}
	if dismissalOn(t, f.readStore, branchID, c, d) != nil {
		t.Error("branch restore did not hide main's dismissal on the branch")
	}
	if dismissalOn(t, f.readStore, domain.MainBranchID, c, d) == nil {
		t.Error("branch restore removed main's dismissal")
	}

	if _, err := f.handler.MergeBranch(ctx, command.MergeBranchInput{BranchID: branch.ID}); err != nil {
		t.Fatalf("MergeBranch: %v", err)
	}
	if dismissalOn(t, f.readStore, domain.MainBranchID, a, b) == nil {
		t.Error("merge did not carry the branch's dismissal to main")
	}
	if dismissalOn(t, f.readStore, domain.MainBranchID, c, d) != nil {
		t.Error("merge did not carry the branch's restore to main")
	}
}

// TestBranchDuplicateDismissal_ConflictsWithMain: main and a branch deciding
// the same pair differently since the fork is a merge conflict, not a silent
// override; resolving it for the branch carries the branch's decision.
func TestBranchDuplicateDismissal_ConflictsWithMain(t *testing.T) {
	f := newBranchFixture()
	ctx := context.Background()
	a := newDismissalPerson(t, f.handler, "Ann")
	b := newDismissalPerson(t, f.handler, "Ann")

	branch, err := f.handler.CreateBranch(ctx, "conflict", "")
	if err != nil {
		t.Fatalf("CreateBranch: %v", err)
	}
	// Main dismisses the pair after the fork; the branch sees it live and
	// restores it.
	if err := f.handler.DismissDuplicate(ctx, command.DismissDuplicateInput{Person1ID: a, Person2ID: b}); err != nil {
		t.Fatalf("main DismissDuplicate: %v", err)
	}
	if err := f.handler.WithBranch(branch).RestoreDuplicate(ctx, a, b); err != nil {
		t.Fatalf("branch RestoreDuplicate: %v", err)
	}

	if _, err := f.handler.MergeBranch(ctx, command.MergeBranchInput{BranchID: branch.ID}); !errors.Is(err, command.ErrMergeConflicts) {
		t.Fatalf("MergeBranch error = %v, want ErrMergeConflicts", err)
	}
	pair, _ := domain.NewDuplicatePair(a, b)
	_, err = f.handler.MergeBranch(ctx, command.MergeBranchInput{
		BranchID:    branch.ID,
		Resolutions: map[uuid.UUID]command.MergeResolution{pair.ID(): command.ResolveBranch},
	})
	if err != nil {
		t.Fatalf("MergeBranch with resolution: %v", err)
	}
	if dismissalOn(t, f.readStore, domain.MainBranchID, a, b) != nil {
		t.Error("the branch's restore did not win on main")
	}
}
