package command_test

import (
	"context"
	"encoding/json"
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
//
// raceResumeRecord makes the next append of a BranchMergeResumed decision
// record lose its optimistic-concurrency check, as if a rival resume had
// recorded first.
type faultyReplayStore struct {
	repository.EventStore
	armed            bool
	failAt           int
	mainAppends      int
	raceResumeRecord bool

	// afterMainScan, when set, runs once right after a set-based read of
	// main's streams returns — the point where a rival resume can act behind
	// this one's back. skipMainScans lets that many reads pass first.
	afterMainScan func()
	skipMainScans int
}

func (s *faultyReplayStore) ReadStreamsForBranch(ctx context.Context, streamIDs []uuid.UUID, branchID domain.BranchID, fromPosition int64, limit int) ([]repository.StoredEvent, error) {
	events, err := s.EventStore.ReadStreamsForBranch(ctx, streamIDs, branchID, fromPosition, limit)
	if hook := s.afterMainScan; hook != nil && branchID.IsMain() {
		if s.skipMainScans > 0 {
			s.skipMainScans--
			return events, err
		}
		s.afterMainScan = nil
		hook()
	}
	return events, err
}

func (s *faultyReplayStore) Append(ctx context.Context, streamID uuid.UUID, streamType string, events []domain.Event, expectedVersion int64, scope repository.AppendScope) error {
	if s.raceResumeRecord && len(events) == 1 && events[0].EventType() == "BranchMergeResumed" {
		s.raceResumeRecord = false
		return repository.ErrConcurrencyConflict
	}
	if s.armed && scope.BranchID.IsMain() {
		s.mainAppends++
		if s.mainAppends == s.failAt {
			return errInjectedReplayFailure
		}
	}
	return s.EventStore.Append(ctx, streamID, streamType, events, expectedVersion, scope)
}

// errInjectedProjectionFailure is the read-model failure faultyReadStore
// injects.
var errInjectedProjectionFailure = errors.New("injected read-model failure during projection")

// faultyReadStore fails mainline SavePerson for one person, SaveFamily for
// one family, or SaveAssociation for one association, while armed, so a
// replay's Append lands in the log but its projection does not.
//
// beforeMainSavePerson, when set, runs once just before the next mainline
// SavePerson reaches the store — between a projection reading a person row
// and writing it back, where a racing write can slip in.
type faultyReadStore struct {
	repository.ReadModelStore
	armed           bool
	failPerson      uuid.UUID
	failFamily      uuid.UUID
	failAssociation uuid.UUID

	// Evidence (#758): failCitation fails the citation's own save,
	// failSourceCount only a save of the source that changes its citation
	// count (the step a citation projection takes after saving the citation),
	// and failNote the note's save.
	failCitation    uuid.UUID
	failSourceCount uuid.UUID
	failNote        uuid.UUID

	beforeMainSavePerson func()

	// beforeMainSaveSource, when set, runs once just before the next mainline
	// SaveSource reaches the store.
	beforeMainSaveSource func()
}

func (s *faultyReadStore) SaveCitation(ctx context.Context, branchID domain.BranchID, citation *repository.CitationReadModel) error {
	if s.armed && branchID.IsMain() && citation.ID == s.failCitation {
		return errInjectedProjectionFailure
	}
	return s.ReadModelStore.SaveCitation(ctx, branchID, citation)
}

func (s *faultyReadStore) SaveSource(ctx context.Context, branchID domain.BranchID, source *repository.SourceReadModel) error {
	if hook := s.beforeMainSaveSource; hook != nil && branchID.IsMain() {
		s.beforeMainSaveSource = nil
		hook()
	}
	if s.armed && branchID.IsMain() && source.ID == s.failSourceCount {
		current, err := s.GetSource(ctx, branchID, source.ID)
		if err != nil {
			return err
		}
		if current != nil && current.CitationCount != source.CitationCount {
			return errInjectedProjectionFailure
		}
	}
	return s.ReadModelStore.SaveSource(ctx, branchID, source)
}

func (s *faultyReadStore) SaveNote(ctx context.Context, branchID domain.BranchID, note *repository.NoteReadModel) error {
	if s.armed && branchID.IsMain() && note.ID == s.failNote {
		return errInjectedProjectionFailure
	}
	return s.ReadModelStore.SaveNote(ctx, branchID, note)
}

func (s *faultyReadStore) SaveAssociation(ctx context.Context, branchID domain.BranchID, association *repository.AssociationReadModel) error {
	if s.armed && branchID.IsMain() && association.ID == s.failAssociation {
		return errInjectedProjectionFailure
	}
	return s.ReadModelStore.SaveAssociation(ctx, branchID, association)
}

func (s *faultyReadStore) SaveFamily(ctx context.Context, branchID domain.BranchID, family *repository.FamilyReadModel) error {
	if s.armed && branchID.IsMain() && family.ID == s.failFamily {
		return errInjectedProjectionFailure
	}
	return s.ReadModelStore.SaveFamily(ctx, branchID, family)
}

func (s *faultyReadStore) SavePerson(ctx context.Context, branchID domain.BranchID, person *repository.PersonReadModel) error {
	if hook := s.beforeMainSavePerson; hook != nil && branchID.IsMain() {
		s.beforeMainSavePerson = nil
		hook()
	}
	if s.armed && branchID.IsMain() && person.ID == s.failPerson {
		return errInjectedProjectionFailure
	}
	return s.ReadModelStore.SavePerson(ctx, branchID, person)
}

// resumeSeed is a branch that edits two persons, so a replay can fail after one
// of them has reached main.
type resumeSeed struct {
	f       *branchFixture
	faulty  *faultyReplayStore
	reads   *faultyReadStore
	branch  *domain.Branch
	first   uuid.UUID
	second  uuid.UUID
	handler *command.Handler
}

func seedResume(t *testing.T) resumeSeed {
	t.Helper()
	var faulty *faultyReplayStore
	var reads *faultyReadStore
	f := newBranchFixtureWith(branchFixtureDeps{
		wrapEvents: func(inner repository.EventStore) repository.EventStore {
			faulty = &faultyReplayStore{EventStore: inner}
			return faulty
		},
		wrapReads: func(inner repository.ReadModelStore) repository.ReadModelStore {
			reads = &faultyReadStore{ReadModelStore: inner}
			return reads
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
	return resumeSeed{f: f, faulty: faulty, reads: reads, branch: branch, first: first.ID, second: second.ID, handler: f.handler}
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

// makeSecondStreamStale interrupts the merge on the second stream and then
// lands a mainline write on it, so a resume must ask for a resolution.
func (s resumeSeed) makeSecondStreamStale(t *testing.T) {
	t.Helper()
	ctx := context.Background()
	s.interruptSecondStream(t, command.MergeBranchInput{BranchID: s.branch.ID})
	version, err := s.f.eventStore.GetStreamVersion(ctx, s.second, domain.MainBranchID)
	if err != nil {
		t.Fatalf("GetStreamVersion failed: %v", err)
	}
	if _, err := s.handler.UpdatePerson(ctx, command.UpdatePersonInput{
		ID: s.second, GivenName: strPtr("Augusta"), Version: version,
	}); err != nil {
		t.Fatalf("mainline UpdatePerson failed: %v", err)
	}
}

// resumeRecords returns the BranchMergeResumed decision records on the
// branch's own stream, decoded.
func resumeRecords(t *testing.T, s resumeSeed) []domain.BranchMergeResumed {
	t.Helper()
	var records []domain.BranchMergeResumed
	for _, stored := range branchEventsFor(t, s.f, s.branch.ID, domain.BranchID(s.branch.ID)) {
		if stored.EventType != "BranchMergeResumed" {
			continue
		}
		decoded, err := stored.DecodeEvent()
		if err != nil {
			t.Fatalf("DecodeEvent failed: %v", err)
		}
		records = append(records, decoded.(domain.BranchMergeResumed))
	}
	return records
}

// TestResumeMerge_MainResolutionIsFinal is the regression test for a resume
// whose "main" decision was not durable: the next resume found the stream
// unreplayed and stale again, asked for a fresh decision, and accepted
// "branch" — replaying the rejected edit over main long after the merge
// reported complete.
func TestResumeMerge_MainResolutionIsFinal(t *testing.T) {
	s := seedResume(t)
	ctx := context.Background()
	s.makeSecondStreamStale(t)

	if _, err := s.handler.ResumeMerge(ctx, command.ResumeMergeInput{
		BranchID:    s.branch.ID,
		Resolutions: map[uuid.UUID]command.MergeResolution{s.second: command.ResolveMain},
	}); err != nil {
		t.Fatalf("resolved ResumeMerge failed: %v", err)
	}

	// The decision is in the log, with the stream dropped from the plan.
	records := resumeRecords(t, s)
	if len(records) != 1 {
		t.Fatalf("got %d BranchMergeResumed records, want 1", len(records))
	}
	if records[0].Resolutions[s.second] != string(command.ResolveMain) {
		t.Errorf("recorded resolutions = %v, want second -> main", records[0].Resolutions)
	}
	if _, planned := records[0].ReplayStreamVersions[s.second]; planned {
		t.Errorf("recorded plan %v still replays the stream resolved to main", records[0].ReplayStreamVersions)
	}

	// Idempotent: a bare resume is a no-op, not a fresh request for a decision.
	head := logHead(t, s.f)
	again, err := s.handler.ResumeMerge(ctx, command.ResumeMergeInput{BranchID: s.branch.ID})
	if err != nil {
		t.Fatalf("bare ResumeMerge after the resolved one failed: %v", err)
	}
	if again.ReplayedEventCount != 0 {
		t.Errorf("ReplayedEventCount = %d, want 0", again.ReplayedEventCount)
	}
	if !slices.Contains(again.SkippedStreamIDs, s.second) {
		t.Errorf("SkippedStreamIDs = %v, want the second stream skipped", again.SkippedStreamIDs)
	}
	if got := logHead(t, s.f); got != head {
		t.Errorf("log head moved %d -> %d on a no-op resume", head, got)
	}

	// ...and the decision cannot be reversed.
	_, err = s.handler.ResumeMerge(ctx, command.ResumeMergeInput{
		BranchID:    s.branch.ID,
		Resolutions: map[uuid.UUID]command.MergeResolution{s.second: command.ResolveBranch},
	})
	if !errors.Is(err, command.ErrUnknownResolution) {
		t.Fatalf("re-deciding a resolved stream: error = %v, want ErrUnknownResolution", err)
	}
	if got := mainSurnameOf(t, s.f, s.second); got != "Hopper" {
		t.Errorf("second stream surname = %q, want main's kept", got)
	}
	if got := logHead(t, s.f); got != head {
		t.Errorf("refused re-decision wrote to the log (%d -> %d)", head, got)
	}
}

// TestResumeMerge_BranchResolutionSurvivesInterruption: a "branch" decision is
// recorded before the replay it causes, so if that replay is interrupted the
// next resume carries the decision out without asking again.
func TestResumeMerge_BranchResolutionSurvivesInterruption(t *testing.T) {
	s := seedResume(t)
	ctx := context.Background()
	s.makeSecondStreamStale(t)

	s.faulty.armed, s.faulty.failAt, s.faulty.mainAppends = true, 1, 0
	_, err := s.handler.ResumeMerge(ctx, command.ResumeMergeInput{
		BranchID:    s.branch.ID,
		Resolutions: map[uuid.UUID]command.MergeResolution{s.second: command.ResolveBranch},
	})
	s.faulty.armed = false
	if !errors.Is(err, command.ErrMergePartiallyApplied) {
		t.Fatalf("interrupted resume: error = %v, want ErrMergePartiallyApplied", err)
	}
	if got := len(resumeRecords(t, s)); got != 1 {
		t.Fatalf("got %d BranchMergeResumed records, want the decision recorded before the replay", got)
	}

	result, err := s.handler.ResumeMerge(ctx, command.ResumeMergeInput{BranchID: s.branch.ID})
	if err != nil {
		t.Fatalf("bare ResumeMerge after the interrupted one failed: %v", err)
	}
	if result.ReplayedEventCount != 1 {
		t.Errorf("ReplayedEventCount = %d, want 1", result.ReplayedEventCount)
	}
	if got := mainSurnameOf(t, s.f, s.second); got != "Byron" {
		t.Errorf("second stream surname = %q, want the recorded branch decision carried out", got)
	}
	if got := len(resumeRecords(t, s)); got != 1 {
		t.Errorf("got %d BranchMergeResumed records, want no new record from a resume that decided nothing", got)
	}
}

// TestResumeMerge_BranchResolutionLapsesWhenMainMovesAgain pins the limit of
// a "branch" decision's finality: it re-pins the stream at the version main
// had when it was made, so if main writes to that entity again before the
// replay lands, the decision no longer vouches for the replay (that write was
// never reviewed). The entity is pending again and may be re-decided — here
// to "main", which keeps main's newer write.
func TestResumeMerge_BranchResolutionLapsesWhenMainMovesAgain(t *testing.T) {
	s := seedResume(t)
	ctx := context.Background()
	s.makeSecondStreamStale(t)

	s.faulty.armed, s.faulty.failAt, s.faulty.mainAppends = true, 1, 0
	_, err := s.handler.ResumeMerge(ctx, command.ResumeMergeInput{
		BranchID:    s.branch.ID,
		Resolutions: map[uuid.UUID]command.MergeResolution{s.second: command.ResolveBranch},
	})
	s.faulty.armed = false
	if !errors.Is(err, command.ErrMergePartiallyApplied) {
		t.Fatalf("interrupted resume: error = %v, want ErrMergePartiallyApplied", err)
	}

	// main writes to the entity again before the decision is carried out.
	version, err := s.f.eventStore.GetStreamVersion(ctx, s.second, domain.MainBranchID)
	if err != nil {
		t.Fatalf("GetStreamVersion failed: %v", err)
	}
	if _, err := s.handler.UpdatePerson(ctx, command.UpdatePersonInput{
		ID: s.second, Surname: strPtr("Murray"), Version: version,
	}); err != nil {
		t.Fatalf("mainline UpdatePerson failed: %v", err)
	}

	result, err := s.handler.ResumeMerge(ctx, command.ResumeMergeInput{BranchID: s.branch.ID})
	if !errors.Is(err, command.ErrMergeResumeNeedsResolution) {
		t.Fatalf("bare ResumeMerge error = %v, want ErrMergeResumeNeedsResolution", err)
	}
	if !slices.Equal(result.PendingStreamIDs, []uuid.UUID{s.second}) {
		t.Errorf("PendingStreamIDs = %v, want [%s] pending again", result.PendingStreamIDs, s.second)
	}

	if _, err := s.handler.ResumeMerge(ctx, command.ResumeMergeInput{
		BranchID:    s.branch.ID,
		Resolutions: map[uuid.UUID]command.MergeResolution{s.second: command.ResolveMain},
	}); err != nil {
		t.Fatalf("re-deciding the lapsed stream to main failed: %v", err)
	}
	if got := mainSurnameOf(t, s.f, s.second); got != "Murray" {
		t.Errorf("second stream surname = %q, want main's newer write kept", got)
	}
	if got := len(resumeRecords(t, s)); got != 2 {
		t.Errorf("got %d BranchMergeResumed records, want both decisions recorded", got)
	}
}

// TestResumeMerge_LegacyClaimDecisionIsFinal: for a claim that recorded no
// plan, the first resume's decisions become the plan.
func TestResumeMerge_LegacyClaimDecisionIsFinal(t *testing.T) {
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

	if _, err := s.f.handler.ResumeMerge(ctx, command.ResumeMergeInput{
		BranchID:    s.branch.ID,
		Resolutions: map[uuid.UUID]command.MergeResolution{s.person: command.ResolveMain},
	}); err != nil {
		t.Fatalf("resolved ResumeMerge failed: %v", err)
	}
	if _, err := s.f.handler.ResumeMerge(ctx, command.ResumeMergeInput{BranchID: s.branch.ID}); err != nil {
		t.Fatalf("bare ResumeMerge after the resolved one failed: %v", err)
	}
	_, err = s.f.handler.ResumeMerge(ctx, command.ResumeMergeInput{
		BranchID:    s.branch.ID,
		Resolutions: map[uuid.UUID]command.MergeResolution{s.person: command.ResolveBranch},
	})
	if !errors.Is(err, command.ErrUnknownResolution) {
		t.Fatalf("re-deciding: error = %v, want ErrUnknownResolution", err)
	}
	if got := mainSurnameOf(t, s.f, s.person); got != "Lovelace" {
		t.Errorf("surname = %q, want main's kept", got)
	}
}

// TestResumeMerge_ConcurrentDecisionRecordLoses: when a rival resume records
// its decisions first, this one replays nothing and says so.
func TestResumeMerge_ConcurrentDecisionRecordLoses(t *testing.T) {
	s := seedResume(t)
	ctx := context.Background()
	s.makeSecondStreamStale(t)

	head := logHead(t, s.f)
	s.faulty.raceResumeRecord = true
	_, err := s.handler.ResumeMerge(ctx, command.ResumeMergeInput{
		BranchID:    s.branch.ID,
		Resolutions: map[uuid.UUID]command.MergeResolution{s.second: command.ResolveBranch},
	})
	if !errors.Is(err, command.ErrMergeResumeConcurrent) {
		t.Fatalf("error = %v, want ErrMergeResumeConcurrent", err)
	}
	if got := logHead(t, s.f); got != head {
		t.Errorf("losing resume wrote to the log (%d -> %d)", head, got)
	}
	if got := mainSurnameOf(t, s.f, s.second); got != "Hopper" {
		t.Errorf("second stream surname = %q, want nothing replayed", got)
	}
}

// TestResumeMerge_RepairsAFailedProjection: the interrupted attempt's Append
// landed but its projection failed, so main's LOG has the stream and its read
// model does not. Resume must not append the events again, and must bring the
// read model level with the log.
func TestResumeMerge_RepairsAFailedProjection(t *testing.T) {
	s := seedResume(t)
	ctx := context.Background()

	s.reads.armed, s.reads.failPerson = true, s.second
	_, err := s.handler.MergeBranch(ctx, command.MergeBranchInput{BranchID: s.branch.ID})
	s.reads.armed = false
	if !errors.Is(err, command.ErrMergePartiallyApplied) || !errors.Is(err, errInjectedProjectionFailure) {
		t.Fatalf("MergeBranch error = %v, want the projection failure wrapped in ErrMergePartiallyApplied", err)
	}
	if got := mainSurnameOf(t, s.f, s.second); got != "Hopper" {
		t.Fatalf("second stream surname = %q, want the failed projection to have left the read model behind", got)
	}

	head := logHead(t, s.f)
	result, err := s.handler.ResumeMerge(ctx, command.ResumeMergeInput{BranchID: s.branch.ID})
	if err != nil {
		t.Fatalf("ResumeMerge failed: %v", err)
	}
	if result.ReplayedEventCount != 0 {
		t.Errorf("ReplayedEventCount = %d, want 0 (the events are already in the log)", result.ReplayedEventCount)
	}
	if !slices.Equal(result.ReprojectedStreamIDs, []uuid.UUID{s.second}) {
		t.Errorf("ReprojectedStreamIDs = %v, want [%s]", result.ReprojectedStreamIDs, s.second)
	}
	if got := logHead(t, s.f); got != head {
		t.Errorf("repairing the read model wrote to the log (%d -> %d)", head, got)
	}
	if got := mainSurnameOf(t, s.f, s.second); got != "Byron" {
		t.Errorf("second stream surname = %q, want the read model repaired", got)
	}
	person, err := s.f.readStore.GetPerson(ctx, domain.MainBranchID, s.second)
	if err != nil {
		t.Fatalf("GetPerson failed: %v", err)
	}
	logVersion, err := s.f.eventStore.GetStreamVersion(ctx, s.second, domain.MainBranchID)
	if err != nil {
		t.Fatalf("GetStreamVersion failed: %v", err)
	}
	if person.Version != logVersion {
		t.Errorf("read-model version = %d, want the log's %d", person.Version, logVersion)
	}

	again, err := s.handler.ResumeMerge(ctx, command.ResumeMergeInput{BranchID: s.branch.ID})
	if err != nil {
		t.Fatalf("second ResumeMerge failed: %v", err)
	}
	if len(again.ReprojectedStreamIDs) != 0 || again.ReplayedEventCount != 0 {
		t.Errorf("second resume reprojected %v and replayed %d, want a no-op", again.ReprojectedStreamIDs, again.ReplayedEventCount)
	}
}

// TestResumeMerge_RepairsAMissingRowAndLinksToIt: a person the branch created
// reached main's log but not its read model, and the family linking them as a
// child was never replayed. Resume re-projects the person and then replays the
// family — the link must not be refused as dangling, because the person IS on
// main in the log.
func TestResumeMerge_RepairsAMissingRowAndLinksToIt(t *testing.T) {
	var reads *faultyReadStore
	f := newBranchFixtureWith(branchFixtureDeps{
		wrapReads: func(inner repository.ReadModelStore) repository.ReadModelStore {
			reads = &faultyReadStore{ReadModelStore: inner}
			return reads
		},
	})
	ctx := context.Background()

	parent, err := f.handler.CreatePerson(ctx, command.CreatePersonInput{GivenName: "Ada", Surname: "Lovelace"})
	if err != nil {
		t.Fatalf("CreatePerson failed: %v", err)
	}
	branch, err := f.handler.CreateBranch(ctx, "new-child", "")
	if err != nil {
		t.Fatalf("CreateBranch failed: %v", err)
	}
	onBranch := f.handler.WithBranch(branch)
	child, err := onBranch.CreatePerson(ctx, command.CreatePersonInput{GivenName: "Byron", Surname: "King"})
	if err != nil {
		t.Fatalf("branch CreatePerson failed: %v", err)
	}
	family, err := onBranch.CreateFamily(ctx, command.CreateFamilyInput{Partner1ID: &parent.ID})
	if err != nil {
		t.Fatalf("branch CreateFamily failed: %v", err)
	}
	if _, err := onBranch.LinkChild(ctx, command.LinkChildInput{FamilyID: family.ID, ChildID: child.ID}); err != nil {
		t.Fatalf("branch LinkChild failed: %v", err)
	}

	reads.armed, reads.failPerson = true, child.ID
	_, err = f.handler.MergeBranch(ctx, command.MergeBranchInput{BranchID: branch.ID})
	reads.armed = false
	if !errors.Is(err, command.ErrMergePartiallyApplied) {
		t.Fatalf("MergeBranch error = %v, want ErrMergePartiallyApplied", err)
	}
	if person, err := f.readStore.GetPerson(ctx, domain.MainBranchID, child.ID); err != nil || person != nil {
		t.Fatalf("child on main read model = %v (err %v), want missing after the failed projection", person, err)
	}

	result, err := f.handler.ResumeMerge(ctx, command.ResumeMergeInput{BranchID: branch.ID})
	if err != nil {
		t.Fatalf("ResumeMerge failed: %v", err)
	}
	if !slices.Equal(result.ReprojectedStreamIDs, []uuid.UUID{child.ID}) {
		t.Errorf("ReprojectedStreamIDs = %v, want [%s]", result.ReprojectedStreamIDs, child.ID)
	}
	if person, err := f.readStore.GetPerson(ctx, domain.MainBranchID, child.ID); err != nil || person == nil {
		t.Fatalf("child on main read model = %v (err %v), want it re-projected", person, err)
	}
	children, err := f.readStore.GetFamilyChildren(ctx, domain.MainBranchID, family.ID)
	if err != nil {
		t.Fatalf("GetFamilyChildren failed: %v", err)
	}
	if len(children) != 1 || children[0].PersonID != child.ID {
		t.Errorf("main family children = %v, want the replayed link to the child", children)
	}
}

// TestResumeMerge_GoneEntitiesAreNotResurrected: a replayed person missing
// from main's read model is left alone when the log explains why — its stream
// ends in a delete, or main merged it into another person.
func TestResumeMerge_GoneEntitiesAreNotResurrected(t *testing.T) {
	ctx := context.Background()

	t.Run("deleted on the branch", func(t *testing.T) {
		s := seedMerge(t, "Byron")
		person, err := s.f.readStore.GetPerson(ctx, domain.BranchID(s.branch.ID), s.person)
		if err != nil || person == nil {
			t.Fatalf("branch GetPerson failed: %v", err)
		}
		if err := s.f.handler.WithBranch(s.branch).DeletePerson(ctx, command.DeletePersonInput{
			ID: s.person, Version: person.Version,
		}); err != nil {
			t.Fatalf("branch DeletePerson failed: %v", err)
		}
		if _, err := s.f.handler.MergeBranch(ctx, command.MergeBranchInput{BranchID: s.branch.ID}); err != nil {
			t.Fatalf("MergeBranch failed: %v", err)
		}

		result, err := s.f.handler.ResumeMerge(ctx, command.ResumeMergeInput{BranchID: s.branch.ID})
		if err != nil {
			t.Fatalf("ResumeMerge failed: %v", err)
		}
		if len(result.ReprojectedStreamIDs) != 0 {
			t.Errorf("ReprojectedStreamIDs = %v, want none", result.ReprojectedStreamIDs)
		}
		if got, err := s.f.readStore.GetPerson(ctx, domain.MainBranchID, s.person); err != nil || got != nil {
			t.Errorf("deleted person on main = %v (err %v), want it to stay deleted", got, err)
		}
	})

	t.Run("merged away on main", func(t *testing.T) {
		s := seedMerge(t, "Byron")
		if _, err := s.f.handler.MergeBranch(ctx, command.MergeBranchInput{BranchID: s.branch.ID}); err != nil {
			t.Fatalf("MergeBranch failed: %v", err)
		}
		survivor, err := s.f.handler.CreatePerson(ctx, command.CreatePersonInput{GivenName: "Augusta", Surname: "Byron"})
		if err != nil {
			t.Fatalf("CreatePerson failed: %v", err)
		}
		merged, err := s.f.readStore.GetPerson(ctx, domain.MainBranchID, s.person)
		if err != nil || merged == nil {
			t.Fatalf("main GetPerson failed: %v", err)
		}
		if _, err := s.f.handler.MergePersons(ctx, command.MergePersonsInput{
			SurvivorID: survivor.ID, MergedID: s.person,
			SurvivorVersion: survivor.Version, MergedVersion: merged.Version,
		}); err != nil {
			t.Fatalf("MergePersons failed: %v", err)
		}

		result, err := s.f.handler.ResumeMerge(ctx, command.ResumeMergeInput{BranchID: s.branch.ID})
		if err != nil {
			t.Fatalf("ResumeMerge failed: %v", err)
		}
		if len(result.ReprojectedStreamIDs) != 0 {
			t.Errorf("ReprojectedStreamIDs = %v, want none", result.ReprojectedStreamIDs)
		}
		if got, err := s.f.readStore.GetPerson(ctx, domain.MainBranchID, s.person); err != nil || got != nil {
			t.Errorf("merged-away person on main = %v (err %v), want it to stay gone", got, err)
		}
	})
}

// mainCopiesOf counts main's events on a stream that carry the given payload
// id — how many times one branch event reached main.
func mainCopiesOf(t *testing.T, s resumeSeed, streamID, eventID uuid.UUID) int {
	t.Helper()
	events, err := s.f.eventStore.ReadStreamsForBranch(context.Background(), []uuid.UUID{streamID}, domain.MainBranchID, 0, 1000)
	if err != nil {
		t.Fatalf("ReadStreamsForBranch failed: %v", err)
	}
	copies := 0
	for i := range events {
		if storedPayloadID(t, events[i]) == eventID {
			copies++
		}
	}
	return copies
}

// storedPayloadID is a stored event's domain event id, which a replay carries
// onto main unchanged.
func storedPayloadID(t *testing.T, evt repository.StoredEvent) uuid.UUID {
	t.Helper()
	var payload struct {
		ID uuid.UUID `json:"id"`
	}
	if err := json.Unmarshal(evt.Data, &payload); err != nil {
		t.Fatalf("decoding event id: %v", err)
	}
	return payload.ID
}

// TestResumeMerge_ConcurrentLandingIsNotReplayedTwice is the regression test
// for a race between the scan for landed streams and the read of main's
// versions: a rival resume that landed a stream between the two made it look
// unlanded yet already at main's new version, so a "branch" resolution for it
// was accepted and the branch's events were appended to main a second time.
func TestResumeMerge_ConcurrentLandingIsNotReplayedTwice(t *testing.T) {
	s := seedResume(t)
	ctx := context.Background()
	s.makeSecondStreamStale(t)

	// A first resume decides the stale stream for the branch, then fails to
	// replay it: the decision is recorded, the stream is not on main.
	s.faulty.armed, s.faulty.failAt, s.faulty.mainAppends = true, 1, 0
	_, err := s.handler.ResumeMerge(ctx, command.ResumeMergeInput{
		BranchID:    s.branch.ID,
		Resolutions: map[uuid.UUID]command.MergeResolution{s.second: command.ResolveBranch},
	})
	s.faulty.armed = false
	if !errors.Is(err, command.ErrMergePartiallyApplied) {
		t.Fatalf("interrupted resume: error = %v, want ErrMergePartiallyApplied", err)
	}

	var branchEventID uuid.UUID
	for _, stored := range branchEventsFor(t, s.f, s.second, domain.BranchID(s.branch.ID)) {
		branchEventID = storedPayloadID(t, stored)
	}
	if branchEventID == uuid.Nil {
		t.Fatal("branch has no event on the second stream")
	}
	before, err := s.f.eventStore.GetStreamVersion(ctx, s.second, domain.MainBranchID)
	if err != nil {
		t.Fatalf("GetStreamVersion failed: %v", err)
	}

	// Two users answer the same earlier refusal. This request's main scan is
	// overtaken by a rival resume that lands the stream.
	rivalRan := false
	s.faulty.afterMainScan = func() {
		rival, err := s.handler.ResumeMerge(ctx, command.ResumeMergeInput{BranchID: s.branch.ID})
		if err != nil {
			t.Errorf("rival ResumeMerge failed: %v", err)
			return
		}
		rivalRan = rival.ReplayedEventCount == 1
	}
	result, err := s.handler.ResumeMerge(ctx, command.ResumeMergeInput{
		BranchID:    s.branch.ID,
		Resolutions: map[uuid.UUID]command.MergeResolution{s.second: command.ResolveBranch},
	})
	if !rivalRan {
		t.Fatal("the rival resume did not land the stream mid-request; the race was not exercised")
	}
	if err == nil {
		t.Errorf("overtaken resume succeeded (replayed %d), want it refused", result.ReplayedEventCount)
	}

	if got := mainCopiesOf(t, s, s.second, branchEventID); got != 1 {
		t.Errorf("branch event reached main %d times, want exactly once", got)
	}
	after, err := s.f.eventStore.GetStreamVersion(ctx, s.second, domain.MainBranchID)
	if err != nil {
		t.Fatalf("GetStreamVersion failed: %v", err)
	}
	if after != before+1 {
		t.Errorf("main version of the stream went %d -> %d, want one replayed event", before, after)
	}

	// And the merge is complete: a further resume is a no-op.
	again, err := s.handler.ResumeMerge(ctx, command.ResumeMergeInput{BranchID: s.branch.ID})
	if err != nil {
		t.Fatalf("bare ResumeMerge after the race failed: %v", err)
	}
	if again.ReplayedEventCount != 0 || !slices.Contains(again.AlreadyReplayedStreamIDs, s.second) {
		t.Errorf("follow-up resume replayed %d, already-replayed %v; want a no-op with the stream landed",
			again.ReplayedEventCount, again.AlreadyReplayedStreamIDs)
	}
}

// TestResumeMerge_ConcurrentRepairsDoNotDoubleCount: a replayed
// ChildLinkedToFamily reached main's log but its projection failed before the
// family row was saved. Two resumes then repair the family at once, both having
// read the row at its old version. The repair must converge on one child, not
// count the link once per resume.
func TestResumeMerge_ConcurrentRepairsDoNotDoubleCount(t *testing.T) {
	var events *faultyReplayStore
	var reads *faultyReadStore
	f := newBranchFixtureWith(branchFixtureDeps{
		wrapEvents: func(inner repository.EventStore) repository.EventStore {
			events = &faultyReplayStore{EventStore: inner}
			return events
		},
		wrapReads: func(inner repository.ReadModelStore) repository.ReadModelStore {
			reads = &faultyReadStore{ReadModelStore: inner}
			return reads
		},
	})
	ctx := context.Background()

	parent, err := f.handler.CreatePerson(ctx, command.CreatePersonInput{GivenName: "Ada", Surname: "Lovelace"})
	if err != nil {
		t.Fatalf("CreatePerson failed: %v", err)
	}
	child, err := f.handler.CreatePerson(ctx, command.CreatePersonInput{GivenName: "Byron", Surname: "King"})
	if err != nil {
		t.Fatalf("CreatePerson failed: %v", err)
	}
	family, err := f.handler.CreateFamily(ctx, command.CreateFamilyInput{Partner1ID: &parent.ID})
	if err != nil {
		t.Fatalf("CreateFamily failed: %v", err)
	}
	branch, err := f.handler.CreateBranch(ctx, "link-child", "")
	if err != nil {
		t.Fatalf("CreateBranch failed: %v", err)
	}
	if _, err := f.handler.WithBranch(branch).LinkChild(ctx, command.LinkChildInput{FamilyID: family.ID, ChildID: child.ID}); err != nil {
		t.Fatalf("branch LinkChild failed: %v", err)
	}

	reads.armed, reads.failFamily = true, family.ID
	_, err = f.handler.MergeBranch(ctx, command.MergeBranchInput{BranchID: branch.ID})
	reads.armed = false
	if !errors.Is(err, command.ErrMergePartiallyApplied) || !errors.Is(err, errInjectedProjectionFailure) {
		t.Fatalf("MergeBranch error = %v, want the projection failure wrapped in ErrMergePartiallyApplied", err)
	}

	// The first resume's second main scan is the re-projection's read of the
	// family's events, taken after it read the row's version. A rival resume
	// repairs the family in full right there.
	events.skipMainScans = 1
	events.afterMainScan = func() {
		if _, err := f.handler.ResumeMerge(ctx, command.ResumeMergeInput{BranchID: branch.ID}); err != nil {
			t.Errorf("rival ResumeMerge failed: %v", err)
		}
	}
	result, err := f.handler.ResumeMerge(ctx, command.ResumeMergeInput{BranchID: branch.ID})
	if err != nil {
		t.Fatalf("ResumeMerge failed: %v", err)
	}
	if events.afterMainScan != nil {
		t.Fatal("the rival resume never ran; the race was not exercised")
	}
	if !slices.Equal(result.ReprojectedStreamIDs, []uuid.UUID{family.ID}) {
		t.Errorf("ReprojectedStreamIDs = %v, want [%s]", result.ReprojectedStreamIDs, family.ID)
	}

	row, err := f.readStore.GetFamily(ctx, domain.MainBranchID, family.ID)
	if err != nil || row == nil {
		t.Fatalf("main GetFamily = %v (err %v)", row, err)
	}
	if row.ChildCount != 1 {
		t.Errorf("ChildCount = %d, want 1 after two concurrent repairs", row.ChildCount)
	}
	logVersion, err := f.eventStore.GetStreamVersion(ctx, family.ID, domain.MainBranchID)
	if err != nil {
		t.Fatalf("GetStreamVersion failed: %v", err)
	}
	if row.Version != logVersion {
		t.Errorf("read-model version = %d, want the log's %d", row.Version, logVersion)
	}
}
