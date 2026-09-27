package command_test

import (
	"context"
	"errors"
	"slices"
	"strings"
	"testing"

	"github.com/google/uuid"

	"github.com/cacack/my-family/internal/command"
	"github.com/cacack/my-family/internal/domain"
	"github.com/cacack/my-family/internal/repository"
)

// errInjectedReplayFailure is the storage failure faultyReplayStore injects.
var errInjectedReplayFailure = errors.New("injected storage failure during replay")

// faultyReplayStore fails the Nth mainline Append once it is armed, and passes
// everything else through. Mainline appends made while seeding happen before it
// is armed, so the count covers only the merge's replay (the claim is on the
// branch's own scope and is never counted).
type faultyReplayStore struct {
	repository.EventStore
	armed       bool
	failAt      int
	mainAppends int
}

func (s *faultyReplayStore) Append(ctx context.Context, streamID uuid.UUID, streamType string, events []domain.Event, expectedVersion int64, scope repository.AppendScope) error {
	if s.armed && scope.BranchID.IsMain() {
		s.mainAppends++
		if s.mainAppends == s.failAt {
			return errInjectedReplayFailure
		}
	}
	return s.EventStore.Append(ctx, streamID, streamType, events, expectedVersion, scope)
}

// resumeSeed is a branch that edits two persons, so a replay can fail after one
// of them has reached main.
type resumeSeed struct {
	f       *branchFixture
	faulty  *faultyReplayStore
	branch  *domain.Branch
	first   uuid.UUID
	second  uuid.UUID
	handler *command.Handler
}

func seedResume(t *testing.T) resumeSeed {
	t.Helper()
	var faulty *faultyReplayStore
	f := newBranchFixtureWith(branchFixtureDeps{
		wrapEvents: func(inner repository.EventStore) repository.EventStore {
			faulty = &faultyReplayStore{EventStore: inner}
			return faulty
		},
	})
	ctx := context.Background()

	first, err := f.handler.CreatePerson(ctx, command.CreatePersonInput{GivenName: "Ada", Surname: "Lovelace"})
	if err != nil {
		t.Fatalf("CreatePerson failed: %v", err)
	}
	second, err := f.handler.CreatePerson(ctx, command.CreatePersonInput{GivenName: "Grace", Surname: "Hopper"})
	if err != nil {
		t.Fatalf("CreatePerson failed: %v", err)
	}
	branch, err := f.handler.CreateBranch(ctx, "two-streams", "")
	if err != nil {
		t.Fatalf("CreateBranch failed: %v", err)
	}
	for _, person := range []*command.CreatePersonResult{first, second} {
		surname := "Byron"
		if _, err := f.handler.WithBranch(branch).UpdatePerson(ctx, command.UpdatePersonInput{
			ID:      person.ID,
			Surname: &surname,
			Version: person.Version,
		}); err != nil {
			t.Fatalf("branch UpdatePerson failed: %v", err)
		}
	}
	return resumeSeed{f: f, faulty: faulty, branch: branch, first: first.ID, second: second.ID, handler: f.handler}
}

// interruptSecondStream runs a merge whose replay fails on its second stream,
// leaving the branch merged and main half-updated.
func (s resumeSeed) interruptSecondStream(t *testing.T, input command.MergeBranchInput) {
	t.Helper()
	s.faulty.armed, s.faulty.failAt = true, 2
	_, err := s.handler.MergeBranch(context.Background(), input)
	s.faulty.armed = false
	if !errors.Is(err, command.ErrMergePartiallyApplied) || !errors.Is(err, errInjectedReplayFailure) {
		t.Fatalf("MergeBranch error = %v, want the injected failure wrapped in ErrMergePartiallyApplied", err)
	}
}

// mainSurnameOf reads a person's surname on main.
func mainSurnameOf(t *testing.T, f *branchFixture, personID uuid.UUID) string {
	t.Helper()
	person, err := f.readStore.GetPerson(context.Background(), domain.MainBranchID, personID)
	if err != nil {
		t.Fatalf("GetPerson on main failed: %v", err)
	}
	if person == nil {
		t.Fatalf("person %s missing on main", personID)
	}
	return person.Surname
}

// TestResumeMerge_FinishesAnInterruptedReplay is #685's acceptance test: a
// merge interrupted mid-replay (one stream on main, one not) is resumed to
// completion without duplicating the stream that already landed, and resuming
// again afterwards is a no-op.
func TestResumeMerge_FinishesAnInterruptedReplay(t *testing.T) {
	s := seedResume(t)
	ctx := context.Background()
	s.interruptSecondStream(t, command.MergeBranchInput{BranchID: s.branch.ID})

	// The interrupted state: first stream on main, second not.
	if got := mainSurnameOf(t, s.f, s.first); got != "Byron" {
		t.Fatalf("first stream surname = %q, want it replayed before the failure", got)
	}
	if got := mainSurnameOf(t, s.f, s.second); got != "Hopper" {
		t.Fatalf("second stream surname = %q, want the failed replay to have left it alone", got)
	}
	firstBefore := len(branchEventsFor(t, s.f, s.first, domain.MainBranchID))

	result, err := s.handler.ResumeMerge(ctx, command.ResumeMergeInput{BranchID: s.branch.ID})
	if err != nil {
		t.Fatalf("ResumeMerge failed: %v", err)
	}
	if result.ReplayedEventCount != 1 {
		t.Errorf("ReplayedEventCount = %d, want 1 (only the second stream)", result.ReplayedEventCount)
	}
	if !slices.Equal(result.AlreadyReplayedStreamIDs, []uuid.UUID{s.first}) {
		t.Errorf("AlreadyReplayedStreamIDs = %v, want [%s]", result.AlreadyReplayedStreamIDs, s.first)
	}
	if result.Branch.Status != domain.BranchStatusMerged {
		t.Errorf("branch status = %s, want merged", result.Branch.Status)
	}
	if got := mainSurnameOf(t, s.f, s.second); got != "Byron" {
		t.Errorf("second stream surname = %q, want the resume to have replayed it", got)
	}
	// No duplicate: the stream that had already landed gained nothing.
	if got := len(branchEventsFor(t, s.f, s.first, domain.MainBranchID)); got != firstBefore {
		t.Errorf("first stream has %d main events after resume, want %d (no duplicate replay)", got, firstBefore)
	}

	// Idempotent: a second resume writes nothing.
	head := logHead(t, s.f)
	again, err := s.handler.ResumeMerge(ctx, command.ResumeMergeInput{BranchID: s.branch.ID})
	if err != nil {
		t.Fatalf("second ResumeMerge failed: %v", err)
	}
	if again.ReplayedEventCount != 0 {
		t.Errorf("second resume ReplayedEventCount = %d, want 0", again.ReplayedEventCount)
	}
	if len(again.AlreadyReplayedStreamIDs) != 2 {
		t.Errorf("second resume AlreadyReplayedStreamIDs = %v, want both streams", again.AlreadyReplayedStreamIDs)
	}
	if got := logHead(t, s.f); got != head {
		t.Errorf("log head moved %d -> %d on a no-op resume", head, got)
	}
}

// TestResumeMerge_AfterSuccessfulMergeIsNoOp covers resuming a merge that never
// failed at all.
func TestResumeMerge_AfterSuccessfulMergeIsNoOp(t *testing.T) {
	s := seedMerge(t, "Byron")
	ctx := context.Background()
	merged, err := s.f.handler.MergeBranch(ctx, command.MergeBranchInput{BranchID: s.branch.ID})
	if err != nil {
		t.Fatalf("MergeBranch failed: %v", err)
	}

	head := logHead(t, s.f)
	result, err := s.f.handler.ResumeMerge(ctx, command.ResumeMergeInput{BranchID: s.branch.ID})
	if err != nil {
		t.Fatalf("ResumeMerge failed: %v", err)
	}
	if result.ReplayedEventCount != 0 {
		t.Errorf("ReplayedEventCount = %d, want 0", result.ReplayedEventCount)
	}
	if result.MergedAtPosition != merged.MergedAtPosition {
		t.Errorf("MergedAtPosition = %d, want the claim's %d", result.MergedAtPosition, merged.MergedAtPosition)
	}
	if got := logHead(t, s.f); got != head {
		t.Errorf("log head moved %d -> %d on a no-op resume", head, got)
	}
}

// TestResumeMerge_KeepsClaimTimeMainResolution: a stream resolved to main when
// the merge was claimed stays unreplayed on resume.
func TestResumeMerge_KeepsClaimTimeMainResolution(t *testing.T) {
	s := seedResume(t)
	ctx := context.Background()

	// Exclude the first stream, so the replay's FIRST main append is the
	// second stream's, and fail that one.
	s.faulty.armed, s.faulty.failAt = true, 1
	_, err := s.handler.MergeBranch(ctx, command.MergeBranchInput{
		BranchID:    s.branch.ID,
		Resolutions: map[uuid.UUID]command.MergeResolution{s.first: command.ResolveMain},
	})
	s.faulty.armed = false
	if !errors.Is(err, command.ErrMergePartiallyApplied) {
		t.Fatalf("MergeBranch error = %v, want ErrMergePartiallyApplied", err)
	}

	result, err := s.handler.ResumeMerge(ctx, command.ResumeMergeInput{BranchID: s.branch.ID})
	if err != nil {
		t.Fatalf("ResumeMerge failed: %v", err)
	}
	if !slices.Equal(result.SkippedStreamIDs, []uuid.UUID{s.first}) {
		t.Errorf("SkippedStreamIDs = %v, want [%s]", result.SkippedStreamIDs, s.first)
	}
	if got := mainSurnameOf(t, s.f, s.first); got != "Lovelace" {
		t.Errorf("first stream surname = %q, want main's version kept", got)
	}
	if got := mainSurnameOf(t, s.f, s.second); got != "Byron" {
		t.Errorf("second stream surname = %q, want it replayed", got)
	}

	// Re-deciding a stream the claim already decided is refused.
	_, err = s.handler.ResumeMerge(ctx, command.ResumeMergeInput{
		BranchID:    s.branch.ID,
		Resolutions: map[uuid.UUID]command.MergeResolution{s.first: command.ResolveBranch},
	})
	if !errors.Is(err, command.ErrUnknownResolution) {
		t.Errorf("ResumeMerge re-deciding a claimed stream: error = %v, want ErrUnknownResolution", err)
	}
}

// TestResumeMerge_StaleStreamNeedsResolution: main moved on a stream the claim
// pinned, so the claim's verdict no longer covers it. Resume must refuse with
// nothing written until the caller decides, then honor the decision.
func TestResumeMerge_StaleStreamNeedsResolution(t *testing.T) {
	for _, tc := range []struct {
		name        string
		resolution  command.MergeResolution
		wantSurname string
		wantSkipped bool
	}{
		{"branch wins", command.ResolveBranch, "Byron", false},
		{"main wins", command.ResolveMain, "Hopper", true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			s := seedResume(t)
			ctx := context.Background()
			s.interruptSecondStream(t, command.MergeBranchInput{BranchID: s.branch.ID})

			// A mainline write to the unreplayed stream, after the claim.
			version, err := s.f.eventStore.GetStreamVersion(ctx, s.second, domain.MainBranchID)
			if err != nil {
				t.Fatalf("GetStreamVersion failed: %v", err)
			}
			if _, err := s.handler.UpdatePerson(ctx, command.UpdatePersonInput{
				ID: s.second, GivenName: strPtr("Augusta"), Version: version,
			}); err != nil {
				t.Fatalf("mainline UpdatePerson failed: %v", err)
			}

			head := logHead(t, s.f)
			result, err := s.handler.ResumeMerge(ctx, command.ResumeMergeInput{BranchID: s.branch.ID})
			if !errors.Is(err, command.ErrMergeResumeNeedsResolution) {
				t.Fatalf("ResumeMerge error = %v, want ErrMergeResumeNeedsResolution", err)
			}
			if result == nil || !slices.Equal(result.PendingStreamIDs, []uuid.UUID{s.second}) {
				t.Fatalf("PendingStreamIDs = %v, want [%s]", result, s.second)
			}
			if got := logHead(t, s.f); got != head {
				t.Fatalf("refused resume wrote to the log (%d -> %d)", head, got)
			}

			result, err = s.handler.ResumeMerge(ctx, command.ResumeMergeInput{
				BranchID:    s.branch.ID,
				Resolutions: map[uuid.UUID]command.MergeResolution{s.second: tc.resolution},
			})
			if err != nil {
				t.Fatalf("resolved ResumeMerge failed: %v", err)
			}
			if got := mainSurnameOf(t, s.f, s.second); got != tc.wantSurname {
				t.Errorf("second stream surname = %q, want %q", got, tc.wantSurname)
			}
			if got := slices.Contains(result.SkippedStreamIDs, s.second); got != tc.wantSkipped {
				t.Errorf("SkippedStreamIDs = %v, want second skipped = %v", result.SkippedStreamIDs, tc.wantSkipped)
			}
			// The mainline write survives either way: the branch replay is an
			// update on top of it, never a rewrite.
			person, err := s.f.readStore.GetPerson(ctx, domain.MainBranchID, s.second)
			if err != nil || person == nil {
				t.Fatalf("GetPerson failed: %v", err)
			}
			if person.GivenName != "Augusta" {
				t.Errorf("given name = %q, want the mainline edit kept", person.GivenName)
			}
		})
	}
}

// TestResumeMerge_LegacyClaimWithoutPlan: a claim written before #685 recorded
// no replay plan, so every stream not yet on main needs a caller decision.
func TestResumeMerge_LegacyClaimWithoutPlan(t *testing.T) {
	s := seedMerge(t, "Byron")
	ctx := context.Background()

	legacy := domain.BranchMerged{
		BaseEvent:        domain.NewBaseEvent(),
		BranchID:         s.branch.ID,
		BasePosition:     s.branch.BasePosition,
		MergedAtPosition: logHead(t, s.f),
	}
	scope := repository.AppendScope{BranchID: domain.BranchID(s.branch.ID), BasePosition: s.branch.BasePosition}
	version, err := s.f.eventStore.GetStreamVersion(ctx, s.branch.ID, scope.BranchID)
	if err != nil {
		t.Fatalf("GetStreamVersion failed: %v", err)
	}
	if err := s.f.eventStore.Append(ctx, s.branch.ID, "branch", []domain.Event{legacy}, version, scope); err != nil {
		t.Fatalf("appending legacy claim failed: %v", err)
	}

	// The registry never saw the claim; resume repairs it before anything else.
	result, err := s.f.handler.ResumeMerge(ctx, command.ResumeMergeInput{BranchID: s.branch.ID})
	if !errors.Is(err, command.ErrMergeResumeNeedsResolution) {
		t.Fatalf("ResumeMerge error = %v, want ErrMergeResumeNeedsResolution", err)
	}
	if !slices.Equal(result.PendingStreamIDs, []uuid.UUID{s.person}) {
		t.Errorf("PendingStreamIDs = %v, want [%s]", result.PendingStreamIDs, s.person)
	}
	branch, err := s.f.branchStore.Get(ctx, s.branch.ID)
	if err != nil {
		t.Fatalf("branch Get failed: %v", err)
	}
	if branch.Status != domain.BranchStatusMerged {
		t.Errorf("branch status = %s, want the registry repaired to merged", branch.Status)
	}

	result, err = s.f.handler.ResumeMerge(ctx, command.ResumeMergeInput{
		BranchID:    s.branch.ID,
		Resolutions: map[uuid.UUID]command.MergeResolution{s.person: command.ResolveBranch},
	})
	if err != nil {
		t.Fatalf("resolved ResumeMerge failed: %v", err)
	}
	if result.ReplayedEventCount != 1 {
		t.Errorf("ReplayedEventCount = %d, want 1", result.ReplayedEventCount)
	}
	if got := mainSurnameOf(t, s.f, s.person); got != "Byron" {
		t.Errorf("surname = %q, want Byron", got)
	}
}

// TestResumeMerge_Refusals covers the resume's guard rails.
func TestResumeMerge_Refusals(t *testing.T) {
	ctx := context.Background()

	t.Run("never claimed", func(t *testing.T) {
		s := seedMerge(t, "Byron")
		_, err := s.f.handler.ResumeMerge(ctx, command.ResumeMergeInput{BranchID: s.branch.ID})
		if !errors.Is(err, command.ErrMergeNotClaimed) {
			t.Errorf("error = %v, want ErrMergeNotClaimed", err)
		}
	})

	t.Run("archived", func(t *testing.T) {
		s := seedMerge(t, "Byron")
		if err := s.f.handler.DeleteBranch(ctx, s.branch.ID); err != nil {
			t.Fatalf("DeleteBranch failed: %v", err)
		}
		_, err := s.f.handler.ResumeMerge(ctx, command.ResumeMergeInput{BranchID: s.branch.ID})
		if !errors.Is(err, command.ErrMergeNotClaimed) {
			t.Errorf("error = %v, want ErrMergeNotClaimed", err)
		}
	})

	t.Run("not found", func(t *testing.T) {
		f := newBranchFixture()
		_, err := f.handler.ResumeMerge(ctx, command.ResumeMergeInput{BranchID: uuid.New()})
		if !errors.Is(err, repository.ErrBranchNotFound) {
			t.Errorf("error = %v, want ErrBranchNotFound", err)
		}
	})

	t.Run("no branch store", func(t *testing.T) {
		handler := command.NewHandler(newBranchFixture().eventStore, newBranchFixture().readStore)
		_, err := handler.ResumeMerge(ctx, command.ResumeMergeInput{BranchID: uuid.New()})
		if !errors.Is(err, command.ErrBranchStoreRequired) {
			t.Errorf("error = %v, want ErrBranchStoreRequired", err)
		}
	})

	t.Run("resolution for an untouched stream", func(t *testing.T) {
		s := seedResume(t)
		s.interruptSecondStream(t, command.MergeBranchInput{BranchID: s.branch.ID})
		_, err := s.handler.ResumeMerge(ctx, command.ResumeMergeInput{
			BranchID:    s.branch.ID,
			Resolutions: map[uuid.UUID]command.MergeResolution{uuid.New(): command.ResolveMain},
		})
		if !errors.Is(err, command.ErrUnknownResolution) {
			t.Errorf("error = %v, want ErrUnknownResolution", err)
		}
	})

	t.Run("resume interrupted again", func(t *testing.T) {
		s := seedResume(t)
		s.interruptSecondStream(t, command.MergeBranchInput{BranchID: s.branch.ID})
		s.faulty.armed, s.faulty.failAt, s.faulty.mainAppends = true, 1, 0
		_, err := s.handler.ResumeMerge(ctx, command.ResumeMergeInput{BranchID: s.branch.ID})
		s.faulty.armed = false
		if !errors.Is(err, command.ErrMergePartiallyApplied) {
			t.Fatalf("error = %v, want ErrMergePartiallyApplied", err)
		}
		// ...and the remedy for a failed resume is another resume.
		result, err := s.handler.ResumeMerge(ctx, command.ResumeMergeInput{BranchID: s.branch.ID})
		if err != nil {
			t.Fatalf("second ResumeMerge failed: %v", err)
		}
		if result.ReplayedEventCount != 1 {
			t.Errorf("ReplayedEventCount = %d, want 1", result.ReplayedEventCount)
		}
	})
}

// TestResumeMerge_PartiallyPresentStreamIsRefused: a stream with only some of
// its branch events on main cannot come from an interrupted merge (each stream
// is one atomic append), so resume refuses rather than guess.
func TestResumeMerge_PartiallyPresentStreamIsRefused(t *testing.T) {
	s := seedMerge(t, "Byron")
	ctx := context.Background()
	branch := s.f.handler.WithBranch(s.branch)
	person, err := s.f.readStore.GetPerson(ctx, domain.BranchID(s.branch.ID), s.person)
	if err != nil || person == nil {
		t.Fatalf("branch GetPerson failed: %v", err)
	}
	if _, err := branch.UpdatePerson(ctx, command.UpdatePersonInput{
		ID: s.person, GivenName: strPtr("Augusta"), Version: person.Version,
	}); err != nil {
		t.Fatalf("second branch edit failed: %v", err)
	}

	// Claim with a nothing-replayed plan, then copy ONE of the two branch
	// events onto main by hand.
	head := logHead(t, s.f)
	scope := repository.AppendScope{BranchID: domain.BranchID(s.branch.ID), BasePosition: s.branch.BasePosition}
	version, err := s.f.eventStore.GetStreamVersion(ctx, s.branch.ID, scope.BranchID)
	if err != nil {
		t.Fatalf("GetStreamVersion failed: %v", err)
	}
	claim := domain.NewBranchMerged(s.branch.ID, s.branch.BasePosition, head, "", map[uuid.UUID]int64{s.person: 1})
	if err := s.f.eventStore.Append(ctx, s.branch.ID, "branch", []domain.Event{claim}, version, scope); err != nil {
		t.Fatalf("claim append failed: %v", err)
	}
	onBranch := branchEventsFor(t, s.f, s.person, domain.BranchID(s.branch.ID))
	decoded, err := onBranch[0].DecodeEvent()
	if err != nil {
		t.Fatalf("DecodeEvent failed: %v", err)
	}
	if err := s.f.eventStore.Append(ctx, s.person, "person", []domain.Event{decoded}, 1, repository.MainScope); err != nil {
		t.Fatalf("main append failed: %v", err)
	}

	_, err = s.f.handler.ResumeMerge(ctx, command.ResumeMergeInput{BranchID: s.branch.ID})
	if err == nil || !strings.Contains(err.Error(), "1 of its 2 branch events on main") {
		t.Fatalf("ResumeMerge error = %v, want a refusal naming the half-present stream", err)
	}
}

// TestResumeMerge_RefusesBranchWritesAfterTheClaim: a claim whose projection
// failed leaves the registry reading "active", so the branch can still be
// written. Those writes were never part of the claim's plan, and resume must
// not quietly replay or drop them.
func TestResumeMerge_RefusesBranchWritesAfterTheClaim(t *testing.T) {
	s := seedMerge(t, "Byron")
	ctx := context.Background()

	scope := repository.AppendScope{BranchID: domain.BranchID(s.branch.ID), BasePosition: s.branch.BasePosition}
	version, err := s.f.eventStore.GetStreamVersion(ctx, s.branch.ID, scope.BranchID)
	if err != nil {
		t.Fatalf("GetStreamVersion failed: %v", err)
	}
	claim := domain.NewBranchMerged(s.branch.ID, s.branch.BasePosition, logHead(t, s.f), "", map[uuid.UUID]int64{s.person: 1})
	if err := s.f.eventStore.Append(ctx, s.branch.ID, "branch", []domain.Event{claim}, version, scope); err != nil {
		t.Fatalf("claim append failed: %v", err)
	}

	person, err := s.f.readStore.GetPerson(ctx, domain.BranchID(s.branch.ID), s.person)
	if err != nil || person == nil {
		t.Fatalf("branch GetPerson failed: %v", err)
	}
	if _, err := s.f.handler.WithBranch(s.branch).UpdatePerson(ctx, command.UpdatePersonInput{
		ID: s.person, GivenName: strPtr("Augusta"), Version: person.Version,
	}); err != nil {
		t.Fatalf("post-claim branch edit failed: %v", err)
	}

	head := logHead(t, s.f)
	_, err = s.f.handler.ResumeMerge(ctx, command.ResumeMergeInput{BranchID: s.branch.ID})
	if err == nil || !strings.Contains(err.Error(), "after its merge was claimed") {
		t.Fatalf("ResumeMerge error = %v, want a refusal naming the post-claim write", err)
	}
	if got := logHead(t, s.f); got != head {
		t.Errorf("refused resume wrote to the log (%d -> %d)", head, got)
	}
}
