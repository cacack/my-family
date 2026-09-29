package command_test

import (
	"context"
	"errors"
	"testing"

	"github.com/google/uuid"

	"github.com/cacack/my-family/internal/command"
	"github.com/cacack/my-family/internal/domain"
)

// mergeCompletenessOf reads a branch's merge state as the API does: the
// registry's branch, then the read-only completeness.
func mergeCompletenessOf(t *testing.T, s resumeSeed) *command.MergeCompleteness {
	t.Helper()
	ctx := context.Background()
	branch, err := s.f.branchStore.Get(ctx, s.branch.ID)
	if err != nil {
		t.Fatalf("branchStore.Get failed: %v", err)
	}
	completeness, err := s.handler.MergeCompleteness(ctx, branch)
	if err != nil {
		t.Fatalf("MergeCompleteness failed: %v", err)
	}
	return completeness
}

// mainVersionOf is a person's version in main's read model.
func mainVersionOf(t *testing.T, s resumeSeed, id uuid.UUID) int64 {
	t.Helper()
	person, err := s.f.readStore.GetPerson(context.Background(), domain.MainBranchID, id)
	if err != nil || person == nil {
		t.Fatalf("GetPerson(%s) on main = %v, %v", id, person, err)
	}
	return person.Version
}

// TestMergeCompleteness_InterruptedMergeIsIncompleteUntilResumed is #830's
// acceptance test: an interrupted merge reads as incomplete, naming the entity
// still to replay, and the read writes nothing — not to the log, not to main's
// read model. Resuming it makes it complete.
func TestMergeCompleteness_InterruptedMergeIsIncompleteUntilResumed(t *testing.T) {
	s := seedResume(t)
	ctx := context.Background()

	if got := mergeCompletenessOf(t, s); got != nil {
		t.Fatalf("active branch completeness = %+v, want nil", got)
	}

	s.interruptSecondStream(t, command.MergeBranchInput{BranchID: s.branch.ID})

	head := logHead(t, s.f)
	versionBefore := mainVersionOf(t, s, s.second)
	completeness := mergeCompletenessOf(t, s)
	if completeness.State != command.MergeStateIncomplete {
		t.Fatalf("State = %s, want incomplete", completeness.State)
	}
	if len(completeness.Pending) != 1 {
		t.Fatalf("Pending = %+v, want exactly the second person", completeness.Pending)
	}
	pending := completeness.Pending[0]
	if pending.StreamID != s.second || pending.EntityType != "person" || pending.EntityName != "Grace Hopper" {
		t.Errorf("pending = %+v, want person %s named (Grace Hopper)", pending, s.second)
	}
	if pending.Reason != command.PendingReady || pending.Reason.NeedsResolution() {
		t.Errorf("pending reason = %s, want ready (the plan still vouches for it)", pending.Reason)
	}

	// Read-only: reading it again changed nothing anywhere.
	_ = mergeCompletenessOf(t, s)
	if got := logHead(t, s.f); got != head {
		t.Errorf("log head moved %d -> %d on a merge-state read", head, got)
	}
	if got := mainVersionOf(t, s, s.second); got != versionBefore {
		t.Errorf("main read model version moved %d -> %d on a merge-state read", versionBefore, got)
	}
	if got := mainSurnameOf(t, s.f, s.second); got != "Hopper" {
		t.Errorf("second surname on main = %q, want the read to have replayed nothing", got)
	}

	if _, err := s.handler.ResumeMerge(ctx, command.ResumeMergeInput{BranchID: s.branch.ID}); err != nil {
		t.Fatalf("ResumeMerge failed: %v", err)
	}
	completeness = mergeCompletenessOf(t, s)
	if completeness.State != command.MergeStateComplete || len(completeness.Pending) != 0 {
		t.Errorf("after resume completeness = %+v, want complete with nothing pending", completeness)
	}
}

// TestMergeCompleteness_CompleteMerge: a merge that never failed is complete,
// including when it left an entity behind.
func TestMergeCompleteness_CompleteMerge(t *testing.T) {
	s := seedResume(t)
	ctx := context.Background()
	if _, err := s.handler.MergeBranch(ctx, command.MergeBranchInput{
		BranchID:    s.branch.ID,
		Resolutions: map[uuid.UUID]command.MergeResolution{s.first: command.ResolveMain},
	}); err != nil {
		t.Fatalf("MergeBranch failed: %v", err)
	}
	completeness := mergeCompletenessOf(t, s)
	if completeness == nil || completeness.State != command.MergeStateComplete {
		t.Fatalf("completeness = %+v, want complete", completeness)
	}
	if completeness.Pending == nil || len(completeness.Pending) != 0 {
		t.Errorf("Pending = %#v, want an empty, non-nil list", completeness.Pending)
	}
}

// TestMergeCompleteness_PendingStreamsNeedingDecisions: an entity main
// changed or removed after the interruption reads as needing a decision, with
// the reason and the decisions a resume accepts, and a resume's refusal names
// it the same way.
func TestMergeCompleteness_PendingStreamsNeedingDecisions(t *testing.T) {
	for _, tc := range []struct {
		name       string
		mainWrite  func(t *testing.T, s resumeSeed, version int64)
		wantReason command.MergePendingReason
		wantSides  []command.MergeResolution
		resolveTo  command.MergeResolution
	}{
		{
			name: "main changed it",
			mainWrite: func(t *testing.T, s resumeSeed, version int64) {
				if _, err := s.handler.UpdatePerson(context.Background(), command.UpdatePersonInput{
					ID: s.second, GivenName: strPtr("Augusta"), Version: version,
				}); err != nil {
					t.Fatalf("mainline UpdatePerson failed: %v", err)
				}
			},
			wantReason: command.PendingMainChanged,
			wantSides:  []command.MergeResolution{command.ResolveBranch, command.ResolveMain},
			resolveTo:  command.ResolveBranch,
		},
		{
			name: "main deleted it",
			mainWrite: func(t *testing.T, s resumeSeed, version int64) {
				if err := s.handler.DeletePerson(context.Background(), command.DeletePersonInput{
					ID: s.second, Version: version,
				}); err != nil {
					t.Fatalf("mainline DeletePerson failed: %v", err)
				}
			},
			wantReason: command.PendingMainRemoved,
			wantSides:  []command.MergeResolution{command.ResolveMain},
			resolveTo:  command.ResolveMain,
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			s := seedResume(t)
			ctx := context.Background()
			s.interruptSecondStream(t, command.MergeBranchInput{BranchID: s.branch.ID})
			version, err := s.f.eventStore.GetStreamVersion(ctx, s.second, domain.MainBranchID)
			if err != nil {
				t.Fatalf("GetStreamVersion failed: %v", err)
			}
			tc.mainWrite(t, s, version)

			head := logHead(t, s.f)
			completeness := mergeCompletenessOf(t, s)
			if completeness.State != command.MergeStateIncomplete || len(completeness.Pending) != 1 {
				t.Fatalf("completeness = %+v, want incomplete with one pending entity", completeness)
			}
			pending := completeness.Pending[0]
			if pending.StreamID != s.second || pending.Reason != tc.wantReason || !pending.Reason.NeedsResolution() {
				t.Errorf("pending = %+v, want %s needing a decision (%s)", pending, s.second, tc.wantReason)
			}
			if pending.EntityName == "" {
				t.Errorf("pending entity has no name, want the branch's name for it")
			}
			if got := pending.Reason.SupportedResolutions(); len(got) != len(tc.wantSides) || got[0] != tc.wantSides[0] {
				t.Errorf("SupportedResolutions = %v, want %v", got, tc.wantSides)
			}
			if got := logHead(t, s.f); got != head {
				t.Errorf("log head moved %d -> %d on a merge-state read", head, got)
			}

			// The resume's own refusal carries the same description.
			result, err := s.handler.ResumeMerge(ctx, command.ResumeMergeInput{BranchID: s.branch.ID})
			if !errors.Is(err, command.ErrMergeResumeNeedsResolution) {
				t.Fatalf("ResumeMerge error = %v, want ErrMergeResumeNeedsResolution", err)
			}
			if len(result.Pending) != 1 || result.Pending[0] != pending {
				t.Errorf("refusal Pending = %+v, want [%+v]", result.Pending, pending)
			}

			if _, err := s.handler.ResumeMerge(ctx, command.ResumeMergeInput{
				BranchID:    s.branch.ID,
				Resolutions: map[uuid.UUID]command.MergeResolution{s.second: tc.resolveTo},
			}); err != nil {
				t.Fatalf("resolved ResumeMerge failed: %v", err)
			}
			if got := mergeCompletenessOf(t, s); got.State != command.MergeStateComplete {
				t.Errorf("after the resolved resume completeness = %+v, want complete", got)
			}
		})
	}
}

// TestMergeCompleteness_ArchivedBranch: an archived branch has no merge state.
func TestMergeCompleteness_ArchivedBranch(t *testing.T) {
	s := seedResume(t)
	ctx := context.Background()
	if err := s.handler.DeleteBranch(ctx, s.branch.ID); err != nil {
		t.Fatalf("DeleteBranch failed: %v", err)
	}
	if got := mergeCompletenessOf(t, s); got != nil {
		t.Errorf("archived branch completeness = %+v, want nil", got)
	}
}

// TestMergeCompleteness_NoBranchStore: a handler without a registry cannot
// read a merge's state.
func TestMergeCompleteness_NoBranchStore(t *testing.T) {
	f := newBranchFixture()
	handler := command.NewHandler(f.eventStore, f.readStore)
	_, err := handler.MergeCompleteness(context.Background(), &domain.Branch{ID: uuid.New(), Status: domain.BranchStatusMerged})
	if !errors.Is(err, command.ErrBranchStoreRequired) {
		t.Errorf("error = %v, want ErrBranchStoreRequired", err)
	}
}

// TestPendingReasonSupportedResolutions pins the decisions each reason
// accepts.
func TestPendingReasonSupportedResolutions(t *testing.T) {
	for reason, want := range map[command.MergePendingReason]int{
		command.PendingReady:           0,
		command.PendingMainChanged:     2,
		command.PendingNoPlan:          2,
		command.PendingMainRemoved:     1,
		command.PendingBreaksReference: 1,
		command.PendingNeedsRepair:     0,
	} {
		if got := len(reason.SupportedResolutions()); got != want {
			t.Errorf("%s accepts %d resolution(s), want %d", reason, got, want)
		}
		wantDecision := reason != command.PendingReady && reason != command.PendingNeedsRepair
		if reason.NeedsResolution() != wantDecision {
			t.Errorf("%s NeedsResolution = %v", reason, reason.NeedsResolution())
		}
	}
}
