package command_test

import (
	"bytes"
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/google/uuid"

	"github.com/cacack/my-family/internal/command"
	"github.com/cacack/my-family/internal/domain"
	"github.com/cacack/my-family/internal/repository"
	"github.com/cacack/my-family/internal/repository/memory"
)

// mergeSeed is the standard merge fixture: one person on main, a branch forked
// after that person exists, and the branch's surname edit already applied.
type mergeSeed struct {
	f      *branchFixture
	branch *domain.Branch
	person uuid.UUID
}

// seedMerge creates the person, forks the branch and applies branchSurname on
// the branch. Main is left untouched.
func seedMerge(t *testing.T, branchSurname string) mergeSeed {
	t.Helper()
	return seedMergeInto(t, newBranchFixture(), branchSurname)
}

// seedMergeInto is seedMerge over a caller-supplied fixture, so a race harness
// can seed the standard scenario into a fixture whose collaborators are wrapped.
// seedMerge is this with the plain fixture; the sequence itself lives here once.
func seedMergeInto(t *testing.T, f *branchFixture, branchSurname string) mergeSeed {
	t.Helper()
	ctx := context.Background()

	person, err := f.handler.CreatePerson(ctx, command.CreatePersonInput{GivenName: "Ada", Surname: "Lovelace"})
	if err != nil {
		t.Fatalf("CreatePerson failed: %v", err)
	}
	branch, err := f.handler.CreateBranch(ctx, "surname-hypothesis", "")
	if err != nil {
		t.Fatalf("CreateBranch failed: %v", err)
	}
	if _, err := f.handler.WithBranch(branch).UpdatePerson(ctx, command.UpdatePersonInput{
		ID:      person.ID,
		Surname: &branchSurname,
		Version: person.Version,
	}); err != nil {
		t.Fatalf("branch UpdatePerson failed: %v", err)
	}

	return mergeSeed{f: f, branch: branch, person: person.ID}
}

// branchEventsFor returns one branch's events for a stream, in position order.
func branchEventsFor(t *testing.T, f *branchFixture, streamID uuid.UUID, branchID domain.BranchID) []repository.StoredEvent {
	t.Helper()
	all, err := f.eventStore.ReadStream(context.Background(), streamID)
	if err != nil {
		t.Fatalf("ReadStream failed: %v", err)
	}
	var filtered []repository.StoredEvent
	for _, evt := range all {
		if evt.BranchID == branchID {
			filtered = append(filtered, evt)
		}
	}
	return filtered
}

// logHead is the shared event log's current head position — what
// GetMaxPosition reports and what a merge records as MergedAtPosition.
func logHead(t *testing.T, f *branchFixture) int64 {
	t.Helper()
	events, err := f.eventStore.ReadAll(context.Background(), 0, 100000)
	if err != nil {
		t.Fatalf("ReadAll failed: %v", err)
	}
	if len(events) == 0 {
		return 0
	}
	return events[len(events)-1].Position
}

func TestMergeBranch(t *testing.T) {
	s := seedMerge(t, "Byron")
	ctx := context.Background()

	headBefore := logHead(t, s.f)

	res, err := s.f.handler.MergeBranch(ctx, command.MergeBranchInput{
		BranchID: s.branch.ID,
		Note:     "confirmed by the 1841 census",
	})
	if err != nil {
		t.Fatalf("MergeBranch failed: %v", err)
	}

	if res.ReplayedEventCount != 1 {
		t.Errorf("ReplayedEventCount = %d, want 1", res.ReplayedEventCount)
	}
	if len(res.SkippedStreamIDs) != 0 {
		t.Errorf("SkippedStreamIDs = %v, want none", res.SkippedStreamIDs)
	}
	if len(res.Conflicts) != 0 {
		t.Errorf("Conflicts = %+v, want none on a successful merge", res.Conflicts)
	}
	if res.MergedAtPosition != headBefore {
		t.Errorf("MergedAtPosition = %d, want the log head at merge time %d", res.MergedAtPosition, headBefore)
	}

	// The branch is terminal, with the merge record persisted.
	if res.Branch.Status != domain.BranchStatusMerged {
		t.Errorf("Status = %q, want %q", res.Branch.Status, domain.BranchStatusMerged)
	}
	if res.Branch.MergeNote != "confirmed by the 1841 census" {
		t.Errorf("MergeNote = %q, want the supplied note", res.Branch.MergeNote)
	}
	if res.Branch.MergedAt == nil {
		t.Error("MergedAt is nil after a merge")
	}
	stored, err := s.f.branchStore.Get(ctx, s.branch.ID)
	if err != nil {
		t.Fatalf("branchStore.Get failed: %v", err)
	}
	if stored.Status != domain.BranchStatusMerged || stored.MergeNote != res.Branch.MergeNote {
		t.Errorf("registry row = %+v, want it to match the returned branch", stored)
	}

	// The branch's change is now main's.
	onMain, err := s.f.readStore.GetPerson(ctx, domain.MainBranchID, s.person)
	if err != nil {
		t.Fatalf("GetPerson on main failed: %v", err)
	}
	if onMain.Surname != "Byron" {
		t.Errorf("main surname after merge = %q, want %q", onMain.Surname, "Byron")
	}

	// The BranchMerged marker lives on the branch's own stream and carries the note.
	markers := branchEventsFor(t, s.f, s.branch.ID, domain.BranchID(s.branch.ID))
	last := markers[len(markers)-1]
	if last.EventType != "BranchMerged" {
		t.Fatalf("last branch-stream event = %q, want BranchMerged", last.EventType)
	}
	decoded, err := last.DecodeEvent()
	if err != nil {
		t.Fatalf("DecodeEvent failed: %v", err)
	}
	merged, ok := decoded.(domain.BranchMerged)
	if !ok {
		t.Fatalf("decoded %T, want domain.BranchMerged", decoded)
	}
	if merged.Note != "confirmed by the 1841 census" {
		t.Errorf("BranchMerged.Note = %q, want the supplied note", merged.Note)
	}
	if merged.MergedAtPosition != res.MergedAtPosition || merged.BasePosition != s.branch.BasePosition {
		t.Errorf("BranchMerged positions = base %d / mergedAt %d, want %d / %d",
			merged.BasePosition, merged.MergedAtPosition, s.branch.BasePosition, res.MergedAtPosition)
	}
}

// TestMergeBranch_PreservesProvenance is ADR-005's first merge property: the
// replayed main event must carry the ORIGINAL branch event's timestamp and
// payload, so main's audit trail says when the research was done rather than
// when it was promoted.
func TestMergeBranch_PreservesProvenance(t *testing.T) {
	s := seedMerge(t, "Byron")
	ctx := context.Background()

	original := branchEventsFor(t, s.f, s.person, domain.BranchID(s.branch.ID))
	if len(original) != 1 {
		t.Fatalf("branch holds %d person events, want 1", len(original))
	}

	if _, err := s.f.handler.MergeBranch(ctx, command.MergeBranchInput{BranchID: s.branch.ID}); err != nil {
		t.Fatalf("MergeBranch failed: %v", err)
	}

	mainEvents := branchEventsFor(t, s.f, s.person, domain.MainBranchID)
	if len(mainEvents) != 2 { // PersonCreated, then the replayed PersonUpdated
		t.Fatalf("main holds %d person events, want 2", len(mainEvents))
	}
	replayed := mainEvents[1]

	if !replayed.Timestamp.Equal(original[0].Timestamp) {
		t.Errorf("replayed timestamp = %s, want the branch original %s", replayed.Timestamp, original[0].Timestamp)
	}
	if !bytes.Equal(replayed.Data, original[0].Data) {
		t.Errorf("replayed payload = %s, want byte-identical to the branch original %s", replayed.Data, original[0].Data)
	}
	if replayed.EventType != original[0].EventType {
		t.Errorf("replayed event type = %q, want %q", replayed.EventType, original[0].EventType)
	}
	if replayed.Position <= original[0].Position {
		t.Errorf("replayed position = %d, want a NEW position after the branch original %d",
			replayed.Position, original[0].Position)
	}
}

// TestMergeBranch_AppendOnly is invariant BR-004: a merge adds new main events
// and rewrites nothing. The branch's own events must survive the merge byte for
// byte, at their original positions and versions.
func TestMergeBranch_AppendOnly(t *testing.T) {
	s := seedMerge(t, "Byron")
	ctx := context.Background()

	before := branchEventsFor(t, s.f, s.person, domain.BranchID(s.branch.ID))
	mainBefore := branchEventsFor(t, s.f, s.person, domain.MainBranchID)

	if _, err := s.f.handler.MergeBranch(ctx, command.MergeBranchInput{BranchID: s.branch.ID}); err != nil {
		t.Fatalf("MergeBranch failed: %v", err)
	}

	after := branchEventsFor(t, s.f, s.person, domain.BranchID(s.branch.ID))
	if len(after) != len(before) {
		t.Fatalf("branch holds %d events after the merge, want the original %d", len(after), len(before))
	}
	for i := range before {
		if after[i].ID != before[i].ID || after[i].Position != before[i].Position ||
			after[i].Version != before[i].Version || !bytes.Equal(after[i].Data, before[i].Data) {
			t.Errorf("branch event %d was rewritten by the merge:\nbefore %+v\nafter  %+v", i, before[i], after[i])
		}
	}

	// Main gained events; the ones it already had are untouched.
	mainAfter := branchEventsFor(t, s.f, s.person, domain.MainBranchID)
	if len(mainAfter) != len(mainBefore)+1 {
		t.Fatalf("main holds %d person events, want %d", len(mainAfter), len(mainBefore)+1)
	}
	for i := range mainBefore {
		if mainAfter[i].ID != mainBefore[i].ID || mainAfter[i].Position != mainBefore[i].Position {
			t.Errorf("main event %d was rewritten by the merge:\nbefore %+v\nafter  %+v", i, mainBefore[i], mainAfter[i])
		}
	}
	if mainAfter[len(mainAfter)-1].Version != mainBefore[len(mainBefore)-1].Version+1 {
		t.Errorf("replayed version = %d, want main's next version %d",
			mainAfter[len(mainAfter)-1].Version, mainBefore[len(mainBefore)-1].Version+1)
	}
}

// divergeOnMain applies a competing surname edit on main, so the branch's edit
// to the same person becomes an edit/edit conflict.
func divergeOnMain(t *testing.T, s mergeSeed, surname string) {
	t.Helper()
	person, err := s.f.readStore.GetPerson(context.Background(), domain.MainBranchID, s.person)
	if err != nil {
		t.Fatalf("GetPerson on main failed: %v", err)
	}
	if _, err := s.f.handler.UpdatePerson(context.Background(), command.UpdatePersonInput{
		ID:      s.person,
		Surname: &surname,
		Version: person.Version,
	}); err != nil {
		t.Fatalf("main UpdatePerson failed: %v", err)
	}
}

func TestMergeBranch_UnresolvedConflictRefuses(t *testing.T) {
	s := seedMerge(t, "Byron")
	divergeOnMain(t, s, "King")
	ctx := context.Background()

	mainBefore := branchEventsFor(t, s.f, s.person, domain.MainBranchID)

	res, err := s.f.handler.MergeBranch(ctx, command.MergeBranchInput{BranchID: s.branch.ID})
	if !errors.Is(err, command.ErrMergeConflicts) {
		t.Fatalf("MergeBranch error = %v, want ErrMergeConflicts", err)
	}
	if res == nil {
		t.Fatal("MergeBranch returned a nil result alongside ErrMergeConflicts, want the conflicts")
	}
	if len(res.Conflicts) != 1 {
		t.Fatalf("Conflicts = %+v, want exactly one", res.Conflicts)
	}
	if res.Conflicts[0].StreamID != s.person {
		t.Errorf("conflict stream = %s, want the person %s", res.Conflicts[0].StreamID, s.person)
	}

	// Zero writes to main, and the branch is still mergeable.
	mainAfter := branchEventsFor(t, s.f, s.person, domain.MainBranchID)
	if len(mainAfter) != len(mainBefore) {
		t.Errorf("main gained %d events from a refused merge, want 0", len(mainAfter)-len(mainBefore))
	}
	onMain, err := s.f.readStore.GetPerson(ctx, domain.MainBranchID, s.person)
	if err != nil {
		t.Fatalf("GetPerson on main failed: %v", err)
	}
	if onMain.Surname != "King" {
		t.Errorf("main surname = %q, want main's own edit %q", onMain.Surname, "King")
	}
	branch, err := s.f.branchStore.Get(ctx, s.branch.ID)
	if err != nil {
		t.Fatalf("branchStore.Get failed: %v", err)
	}
	if branch.Status != domain.BranchStatusActive {
		t.Errorf("branch status after a refused merge = %q, want it still active", branch.Status)
	}
}

func TestMergeBranch_ResolutionBranchWins(t *testing.T) {
	s := seedMerge(t, "Byron")
	divergeOnMain(t, s, "King")
	ctx := context.Background()

	res, err := s.f.handler.MergeBranch(ctx, command.MergeBranchInput{
		BranchID:    s.branch.ID,
		Resolutions: map[uuid.UUID]command.MergeResolution{s.person: command.ResolveBranch},
	})
	if err != nil {
		t.Fatalf("MergeBranch failed: %v", err)
	}
	if res.ReplayedEventCount != 1 {
		t.Errorf("ReplayedEventCount = %d, want 1", res.ReplayedEventCount)
	}
	if len(res.SkippedStreamIDs) != 0 {
		t.Errorf("SkippedStreamIDs = %v, want none", res.SkippedStreamIDs)
	}

	onMain, err := s.f.readStore.GetPerson(ctx, domain.MainBranchID, s.person)
	if err != nil {
		t.Fatalf("GetPerson on main failed: %v", err)
	}
	if onMain.Surname != "Byron" {
		t.Errorf("main surname = %q, want the branch's %q", onMain.Surname, "Byron")
	}
}

func TestMergeBranch_ResolutionMainWinsSkipsStream(t *testing.T) {
	s := seedMerge(t, "Byron")
	divergeOnMain(t, s, "King")
	ctx := context.Background()

	mainBefore := branchEventsFor(t, s.f, s.person, domain.MainBranchID)

	res, err := s.f.handler.MergeBranch(ctx, command.MergeBranchInput{
		BranchID:    s.branch.ID,
		Resolutions: map[uuid.UUID]command.MergeResolution{s.person: command.ResolveMain},
	})
	if err != nil {
		t.Fatalf("MergeBranch failed: %v", err)
	}
	if res.ReplayedEventCount != 0 {
		t.Errorf("ReplayedEventCount = %d, want 0 — every stream was resolved to main", res.ReplayedEventCount)
	}
	if len(res.SkippedStreamIDs) != 1 || res.SkippedStreamIDs[0] != s.person {
		t.Errorf("SkippedStreamIDs = %v, want [%s]", res.SkippedStreamIDs, s.person)
	}

	// Main kept its own value and gained no events, but the branch still merged.
	mainAfter := branchEventsFor(t, s.f, s.person, domain.MainBranchID)
	if len(mainAfter) != len(mainBefore) {
		t.Errorf("main gained %d events for a skipped stream, want 0", len(mainAfter)-len(mainBefore))
	}
	onMain, err := s.f.readStore.GetPerson(ctx, domain.MainBranchID, s.person)
	if err != nil {
		t.Fatalf("GetPerson on main failed: %v", err)
	}
	if onMain.Surname != "King" {
		t.Errorf("main surname = %q, want main's own %q", onMain.Surname, "King")
	}
	if res.Branch.Status != domain.BranchStatusMerged {
		t.Errorf("branch status = %q, want %q", res.Branch.Status, domain.BranchStatusMerged)
	}
}

func TestMergeBranch_UnknownResolution(t *testing.T) {
	tests := []struct {
		name        string
		resolutions func(s mergeSeed) map[uuid.UUID]command.MergeResolution
		wantIn      string
	}{
		{
			name: "stream the branch never touched",
			resolutions: func(mergeSeed) map[uuid.UUID]command.MergeResolution {
				return map[uuid.UUID]command.MergeResolution{uuid.New(): command.ResolveBranch}
			},
			wantIn: "never changed stream",
		},
		{
			name: "unrecognised value",
			resolutions: func(s mergeSeed) map[uuid.UUID]command.MergeResolution {
				return map[uuid.UUID]command.MergeResolution{s.person: "theirs"}
			},
			wantIn: `"theirs"`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			s := seedMerge(t, "Byron")
			ctx := context.Background()

			_, err := s.f.handler.MergeBranch(ctx, command.MergeBranchInput{
				BranchID:    s.branch.ID,
				Resolutions: tt.resolutions(s),
			})
			if !errors.Is(err, command.ErrUnknownResolution) {
				t.Fatalf("MergeBranch error = %v, want ErrUnknownResolution", err)
			}
			if !strings.Contains(err.Error(), tt.wantIn) {
				t.Errorf("error %v should mention %s", err, tt.wantIn)
			}

			// Nothing was claimed or replayed.
			branch, err := s.f.branchStore.Get(ctx, s.branch.ID)
			if err != nil {
				t.Fatalf("branchStore.Get failed: %v", err)
			}
			if branch.Status != domain.BranchStatusActive {
				t.Errorf("branch status = %q, want it still active", branch.Status)
			}
		})
	}
}

func TestMergeBranch_NonActiveBranch(t *testing.T) {
	s := seedMerge(t, "Byron")
	ctx := context.Background()

	if err := s.f.handler.DeleteBranch(ctx, s.branch.ID); err != nil {
		t.Fatalf("DeleteBranch failed: %v", err)
	}

	_, err := s.f.handler.MergeBranch(ctx, command.MergeBranchInput{BranchID: s.branch.ID})
	if !errors.Is(err, command.ErrBranchNotActive) {
		t.Fatalf("MergeBranch on an archived branch = %v, want ErrBranchNotActive", err)
	}
}

func TestMergeBranch_NotFound(t *testing.T) {
	f := newBranchFixture()

	_, err := f.handler.MergeBranch(context.Background(), command.MergeBranchInput{BranchID: uuid.New()})
	if !errors.Is(err, repository.ErrBranchNotFound) {
		t.Fatalf("MergeBranch error = %v, want repository.ErrBranchNotFound", err)
	}
}

func TestMergeBranch_NoteTooLong(t *testing.T) {
	s := seedMerge(t, "Byron")

	_, err := s.f.handler.MergeBranch(context.Background(), command.MergeBranchInput{
		BranchID: s.branch.ID,
		Note:     strings.Repeat("a", 1001),
	})
	if !errors.Is(err, domain.ErrBranchMergeNoteTooLong) {
		t.Fatalf("MergeBranch error = %v, want domain.ErrBranchMergeNoteTooLong", err)
	}
}

// TestMergeBranch_SecondMergeIsRefused is the idempotency half of ADR-005's
// second merge property: once a branch is merged the status guard stops any
// repeat, and main gains no duplicate copies of the branch's work.
func TestMergeBranch_SecondMergeIsRefused(t *testing.T) {
	s := seedMerge(t, "Byron")
	ctx := context.Background()

	if _, err := s.f.handler.MergeBranch(ctx, command.MergeBranchInput{BranchID: s.branch.ID}); err != nil {
		t.Fatalf("first MergeBranch failed: %v", err)
	}
	afterFirst := branchEventsFor(t, s.f, s.person, domain.MainBranchID)

	_, err := s.f.handler.MergeBranch(ctx, command.MergeBranchInput{BranchID: s.branch.ID})
	if !errors.Is(err, command.ErrBranchNotActive) {
		t.Fatalf("second MergeBranch = %v, want ErrBranchNotActive", err)
	}

	afterSecond := branchEventsFor(t, s.f, s.person, domain.MainBranchID)
	if len(afterSecond) != len(afterFirst) {
		t.Errorf("main gained %d duplicate events from the second merge, want 0", len(afterSecond)-len(afterFirst))
	}
}

// racingEventStore lets a test land a rival write on the branch's own stream in
// the window between MergeBranch reading the stream version and appending its
// BranchMerged claim — the concurrency ADR-005 says the CAS must serialize.
type racingEventStore struct {
	repository.EventStore
	rival func()
	fired bool
}

func (s *racingEventStore) Append(ctx context.Context, streamID uuid.UUID, streamType string, events []domain.Event, expectedVersion int64, scope repository.AppendScope) error {
	if streamType == "branch" && !s.fired && s.rival != nil {
		s.fired = true
		s.rival()
	}
	return s.EventStore.Append(ctx, streamID, streamType, events, expectedVersion, scope)
}

// TestMergeBranch_ConcurrentClaimLoses proves the claim is a compare-and-set and
// not a read-then-act check: the losing request writes nothing to main.
func TestMergeBranch_ConcurrentClaimLoses(t *testing.T) {
	inner := memory.NewEventStore()
	racing := &racingEventStore{EventStore: inner}
	readStore := memory.NewReadModelStore()
	branchStore := memory.NewBranchStore()
	f := &branchFixture{
		eventStore:  inner,
		readStore:   readStore,
		branchStore: branchStore,
		handler:     command.NewHandlerWithBranches(racing, readStore, branchStore, memory.NewSnapshotStore(inner)),
	}
	ctx := context.Background()

	person, err := f.handler.CreatePerson(ctx, command.CreatePersonInput{GivenName: "Ada", Surname: "Lovelace"})
	if err != nil {
		t.Fatalf("CreatePerson failed: %v", err)
	}
	branch, err := f.handler.CreateBranch(ctx, "raced", "")
	if err != nil {
		t.Fatalf("CreateBranch failed: %v", err)
	}
	byron := "Byron"
	if _, err := f.handler.WithBranch(branch).UpdatePerson(ctx, command.UpdatePersonInput{
		ID:      person.ID,
		Surname: &byron,
		Version: person.Version,
	}); err != nil {
		t.Fatalf("branch UpdatePerson failed: %v", err)
	}

	// The rival merge claims the branch stream first, straight through the inner
	// store, so our claim's expected version is stale by the time it lands.
	racing.rival = func() {
		rivalEvent := domain.NewBranchMerged(branch.ID, branch.BasePosition, branch.BasePosition, "rival")
		if err := inner.Append(ctx, branch.ID, "branch", []domain.Event{rivalEvent}, 1, repository.AppendScope{
			BranchID:     domain.BranchID(branch.ID),
			BasePosition: branch.BasePosition,
		}); err != nil {
			t.Errorf("rival claim failed: %v", err)
		}
	}

	mainBefore := branchEventsFor(t, f, person.ID, domain.MainBranchID)

	_, err = f.handler.MergeBranch(ctx, command.MergeBranchInput{BranchID: branch.ID})
	if !errors.Is(err, command.ErrMergeAlreadyClaimed) {
		t.Fatalf("MergeBranch error = %v, want ErrMergeAlreadyClaimed", err)
	}

	mainAfter := branchEventsFor(t, f, person.ID, domain.MainBranchID)
	if len(mainAfter) != len(mainBefore) {
		t.Errorf("the losing merge wrote %d events to main, want 0", len(mainAfter)-len(mainBefore))
	}
	onMain, err := readStore.GetPerson(ctx, domain.MainBranchID, person.ID)
	if err != nil {
		t.Fatalf("GetPerson on main failed: %v", err)
	}
	if onMain.Surname != "Lovelace" {
		t.Errorf("main surname = %q, want it untouched by the losing merge", onMain.Surname)
	}
}

// TestMergeBranch_TooLargeRefuses covers the truncated plan: a branch bigger
// than the comparison cap has an incomplete conflict list, so merging it could
// silently promote half the branch past an undetected conflict.
func TestMergeBranch_TooLargeRefuses(t *testing.T) {
	f := newBranchFixture()
	ctx := context.Background()

	branch, err := f.handler.CreateBranch(ctx, "huge", "")
	if err != nil {
		t.Fatalf("CreateBranch failed: %v", err)
	}

	// Written straight to the store: 1000+ events through the command layer
	// would take seconds and prove nothing extra. The branch's own scope keeps
	// them off main.
	streamID := uuid.New()
	events := make([]domain.Event, 0, 1000)
	for i := 0; i < 1000; i++ {
		events = append(events, domain.NewPersonUpdated(streamID, map[string]any{"surname": "Byron"}))
	}
	if err := f.eventStore.Append(ctx, streamID, "Person", events, -1, repository.AppendScope{
		BranchID:     domain.BranchID(branch.ID),
		BasePosition: branch.BasePosition,
	}); err != nil {
		t.Fatalf("seeding an over-cap branch failed: %v", err)
	}

	mainBefore, err := f.eventStore.ReadBranch(ctx, domain.MainBranchID, 0, 10000)
	if err != nil {
		t.Fatalf("ReadBranch(main) failed: %v", err)
	}

	_, err = f.handler.MergeBranch(ctx, command.MergeBranchInput{BranchID: branch.ID})
	if !errors.Is(err, command.ErrBranchTooLargeToMerge) {
		t.Fatalf("MergeBranch error = %v, want ErrBranchTooLargeToMerge", err)
	}

	mainAfter, err := f.eventStore.ReadBranch(ctx, domain.MainBranchID, 0, 10000)
	if err != nil {
		t.Fatalf("ReadBranch(main) failed: %v", err)
	}
	if len(mainAfter) != len(mainBefore) {
		t.Errorf("a refused over-cap merge wrote %d events to main, want 0", len(mainAfter)-len(mainBefore))
	}
	stored, err := f.branchStore.Get(ctx, branch.ID)
	if err != nil {
		t.Fatalf("branchStore.Get failed: %v", err)
	}
	if stored.Status != domain.BranchStatusActive {
		t.Errorf("branch status = %q, want it still active", stored.Status)
	}
}

func TestMergeBranch_MissingCollaborators(t *testing.T) {
	eventStore := memory.NewEventStore()
	readStore := memory.NewReadModelStore()
	ctx := context.Background()

	noStore := command.NewHandler(eventStore, readStore)
	if _, err := noStore.MergeBranch(ctx, command.MergeBranchInput{BranchID: uuid.New()}); !errors.Is(err, command.ErrBranchStoreRequired) {
		t.Errorf("MergeBranch without a branch store = %v, want ErrBranchStoreRequired", err)
	}

	noPositions := command.NewHandlerWithBranchStore(eventStore, readStore, memory.NewBranchStore())
	if _, err := noPositions.MergeBranch(ctx, command.MergeBranchInput{BranchID: uuid.New()}); !errors.Is(err, command.ErrPositionSourceRequired) {
		t.Errorf("MergeBranch without a position source = %v, want ErrPositionSourceRequired", err)
	}
}

// TestMergeBranch_PositionSourceError covers the head read failing between the
// conflict check and the claim — nothing may be written.
func TestMergeBranch_PositionSourceError(t *testing.T) {
	sentinel := errors.New("boom")
	eventStore := memory.NewEventStore()
	readStore := memory.NewReadModelStore()
	branchStore := memory.NewBranchStore()
	handler := command.NewHandlerWithBranches(eventStore, readStore, branchStore, memory.NewSnapshotStore(eventStore))
	ctx := context.Background()

	branch, err := handler.CreateBranch(ctx, "positions", "")
	if err != nil {
		t.Fatalf("CreateBranch failed: %v", err)
	}

	failing := command.NewHandlerWithBranches(eventStore, readStore, branchStore, failingPositions{err: sentinel})
	if _, err := failing.MergeBranch(ctx, command.MergeBranchInput{BranchID: branch.ID}); !errors.Is(err, sentinel) {
		t.Fatalf("MergeBranch error = %v, want it to wrap %v", err, sentinel)
	}

	stored, err := branchStore.Get(ctx, branch.ID)
	if err != nil {
		t.Fatalf("branchStore.Get failed: %v", err)
	}
	if stored.Status != domain.BranchStatusActive {
		t.Errorf("branch status = %q, want it still active", stored.Status)
	}
}

// TestMergeBranch_MultipleStreams checks the grouping: several aggregates, each
// replayed in one append, with per-stream resolutions honored independently.
func TestMergeBranch_MultipleStreams(t *testing.T) {
	f := newBranchFixture()
	ctx := context.Background()

	keep, err := f.handler.CreatePerson(ctx, command.CreatePersonInput{GivenName: "Ada", Surname: "Lovelace"})
	if err != nil {
		t.Fatalf("CreatePerson failed: %v", err)
	}
	drop, err := f.handler.CreatePerson(ctx, command.CreatePersonInput{GivenName: "William", Surname: "King"})
	if err != nil {
		t.Fatalf("CreatePerson failed: %v", err)
	}

	branch, err := f.handler.CreateBranch(ctx, "two-hypotheses", "")
	if err != nil {
		t.Fatalf("CreateBranch failed: %v", err)
	}
	scoped := f.handler.WithBranch(branch)

	// Two edits to the same person on the branch, so its stream carries an
	// ordered pair that must replay as one append.
	version := keep.Version
	for _, surname := range []string{"Byron", "Noel"} {
		s := surname
		res, err := scoped.UpdatePerson(ctx, command.UpdatePersonInput{ID: keep.ID, Surname: &s, Version: version})
		if err != nil {
			t.Fatalf("branch UpdatePerson to %q failed: %v", surname, err)
		}
		version = res.Version
	}
	milbanke := "Milbanke"
	if _, err := scoped.UpdatePerson(ctx, command.UpdatePersonInput{ID: drop.ID, Surname: &milbanke, Version: drop.Version}); err != nil {
		t.Fatalf("branch UpdatePerson failed: %v", err)
	}

	// A branch-only person, which main has never seen.
	fresh, err := scoped.CreatePerson(ctx, command.CreatePersonInput{GivenName: "Grace", Surname: "Hopper"})
	if err != nil {
		t.Fatalf("branch CreatePerson failed: %v", err)
	}

	res, err := f.handler.MergeBranch(ctx, command.MergeBranchInput{
		BranchID:    branch.ID,
		Resolutions: map[uuid.UUID]command.MergeResolution{drop.ID: command.ResolveMain},
	})
	if err != nil {
		t.Fatalf("MergeBranch failed: %v", err)
	}
	if res.ReplayedEventCount != 3 { // two edits to keep, one create for fresh
		t.Errorf("ReplayedEventCount = %d, want 3", res.ReplayedEventCount)
	}
	if len(res.SkippedStreamIDs) != 1 || res.SkippedStreamIDs[0] != drop.ID {
		t.Errorf("SkippedStreamIDs = %v, want [%s]", res.SkippedStreamIDs, drop.ID)
	}

	onMain, err := f.readStore.GetPerson(ctx, domain.MainBranchID, keep.ID)
	if err != nil {
		t.Fatalf("GetPerson on main failed: %v", err)
	}
	if onMain.Surname != "Noel" {
		t.Errorf("main surname = %q, want the branch's last value %q", onMain.Surname, "Noel")
	}
	if onMain.Version != keep.Version+2 {
		t.Errorf("main version = %d, want %d — the replay must continue main's version line",
			onMain.Version, keep.Version+2)
	}

	skipped, err := f.readStore.GetPerson(ctx, domain.MainBranchID, drop.ID)
	if err != nil {
		t.Fatalf("GetPerson on main failed: %v", err)
	}
	if skipped.Surname != "King" {
		t.Errorf("skipped person's main surname = %q, want it untouched (%q)", skipped.Surname, "King")
	}

	promoted, err := f.readStore.GetPerson(ctx, domain.MainBranchID, fresh.ID)
	if err != nil {
		t.Fatalf("GetPerson on main failed: %v", err)
	}
	if promoted == nil {
		t.Fatal("the branch-only person did not reach main")
	}
	if promoted.GivenName != "Grace" {
		t.Errorf("promoted given name = %q, want %q", promoted.GivenName, "Grace")
	}
}

// deleteOnMain removes the seeded person from main, leaving the branch's edit
// to that same person in place. This is the delete-vs-edit shape where MAIN is
// the deleter.
func deleteOnMain(t *testing.T, s mergeSeed) {
	t.Helper()
	ctx := context.Background()
	person, err := s.f.readStore.GetPerson(ctx, domain.MainBranchID, s.person)
	if err != nil {
		t.Fatalf("GetPerson on main failed: %v", err)
	}
	if err := s.f.handler.DeletePerson(ctx, command.DeletePersonInput{
		ID:      s.person,
		Version: person.Version,
		Reason:  "merged into another record",
	}); err != nil {
		t.Fatalf("main DeletePerson failed: %v", err)
	}
}

// TestMergeBranch_MainDeletedResolvedToBranchIsRefused covers the combination
// that previously returned a successful merge while doing the opposite of what
// the caller asked.
//
// Main deleted the person, the branch kept editing it, and the caller resolves
// the conflict to "branch" — i.e. "keep my version". Replaying the branch's
// PersonUpdated onto main cannot honor that: the *Updated projections skip an
// absent read-model row, so every replayed event no-ops and main's row stays
// deleted. Since there is no undelete event in the domain, the only honest
// answer is to refuse the resolution rather than report success.
func TestMergeBranch_MainDeletedResolvedToBranchIsRefused(t *testing.T) {
	s := seedMerge(t, "Byron")
	deleteOnMain(t, s)
	ctx := context.Background()

	mainBefore := branchEventsFor(t, s.f, s.person, domain.MainBranchID)

	_, err := s.f.handler.MergeBranch(ctx, command.MergeBranchInput{
		BranchID:    s.branch.ID,
		Resolutions: map[uuid.UUID]command.MergeResolution{s.person: command.ResolveBranch},
	})

	if !errors.Is(err, command.ErrUnsupportedResolution) {
		t.Fatalf("MergeBranch error = %v, want ErrUnsupportedResolution", err)
	}

	// Refused before the claim: the branch must still be mergeable the other way.
	branch, err := s.f.branchStore.Get(ctx, s.branch.ID)
	if err != nil {
		t.Fatalf("Get branch failed: %v", err)
	}
	if branch.Status != domain.BranchStatusActive {
		t.Errorf("branch status = %s, want active — the refusal must not claim the branch", branch.Status)
	}
	if after := branchEventsFor(t, s.f, s.person, domain.MainBranchID); len(after) != len(mainBefore) {
		t.Errorf("main gained %d events on a refused merge", len(after)-len(mainBefore))
	}
}

// The same conflict resolved to "main" is supported, and honestly reports that
// the branch's changes were dropped.
func TestMergeBranch_MainDeletedResolvedToMainSkipsStream(t *testing.T) {
	s := seedMerge(t, "Byron")
	deleteOnMain(t, s)
	ctx := context.Background()

	result, err := s.f.handler.MergeBranch(ctx, command.MergeBranchInput{
		BranchID:    s.branch.ID,
		Resolutions: map[uuid.UUID]command.MergeResolution{s.person: command.ResolveMain},
	})
	if err != nil {
		t.Fatalf("MergeBranch failed: %v", err)
	}

	if result.ReplayedEventCount != 0 {
		t.Errorf("ReplayedEventCount = %d, want 0", result.ReplayedEventCount)
	}
	if len(result.SkippedStreamIDs) != 1 || result.SkippedStreamIDs[0] != s.person {
		t.Errorf("SkippedStreamIDs = %v, want [%s]", result.SkippedStreamIDs, s.person)
	}
	if result.Branch.Status != domain.BranchStatusMerged {
		t.Errorf("branch status = %s, want merged", result.Branch.Status)
	}
	// Main's delete stands.
	person, err := s.f.readStore.GetPerson(ctx, domain.MainBranchID, s.person)
	if err != nil {
		t.Fatalf("GetPerson on main failed: %v", err)
	}
	if person != nil {
		t.Errorf("person is back on main after resolving to main; the delete should stand")
	}
}

// A main-deleted conflict advertises only the resolution that works, so a
// review UI can offer the real choice instead of one that silently no-ops.
func TestMergeBranch_MainDeletedConflictAdvertisesMainOnly(t *testing.T) {
	s := seedMerge(t, "Byron")
	deleteOnMain(t, s)

	// Merging with no resolutions is refused and hands back the conflict list.
	result, err := s.f.handler.MergeBranch(context.Background(), command.MergeBranchInput{
		BranchID: s.branch.ID,
	})
	if !errors.Is(err, command.ErrMergeConflicts) {
		t.Fatalf("MergeBranch error = %v, want ErrMergeConflicts", err)
	}
	if result == nil || len(result.Conflicts) != 1 {
		t.Fatalf("want exactly 1 conflict, got %+v", result)
	}
	if got := result.Conflicts[0].SupportedResolutions; len(got) != 1 || got[0] != "main" {
		t.Errorf("SupportedResolutions = %v, want [main]", got)
	}
}

// TestMergeBranch_DanglingChildLinkIsRefused covers the gap that per-entity
// resolutions leave open: the branch's events reference each other ACROSS
// entities, so excluding a person does not exclude the links to them.
//
// Main deletes person P. That forces P's conflict to a "main" resolution — the
// only one offered, since replaying edits onto a deleted entity cannot work. But
// the branch also linked P into family F, and that event lives on F's stream,
// which has no conflict of its own and would be replayed. Without a guard the
// merge returns 200 while main gains a family child pointing at a person it does
// not have, reported nowhere.
func TestMergeBranch_DanglingChildLinkIsRefused(t *testing.T) {
	f := newBranchFixture()
	ctx := context.Background()

	// Main: a person and a family the branch will link them into.
	person, err := f.handler.CreatePerson(ctx, command.CreatePersonInput{GivenName: "Ada", Surname: "Lovelace"})
	if err != nil {
		t.Fatalf("CreatePerson failed: %v", err)
	}
	partner, err := f.handler.CreatePerson(ctx, command.CreatePersonInput{GivenName: "William", Surname: "King"})
	if err != nil {
		t.Fatalf("CreatePerson (partner) failed: %v", err)
	}
	family, err := f.handler.CreateFamily(ctx, command.CreateFamilyInput{Partner1ID: &partner.ID})
	if err != nil {
		t.Fatalf("CreateFamily failed: %v", err)
	}

	branch, err := f.handler.CreateBranch(ctx, "parentage-theory", "")
	if err != nil {
		t.Fatalf("CreateBranch failed: %v", err)
	}

	// Branch: link the person into the family, and edit the person so their
	// own stream carries a change too.
	if _, err := f.handler.WithBranch(branch).LinkChild(ctx, command.LinkChildInput{
		FamilyID: family.ID, ChildID: person.ID, RelationType: "biological",
	}); err != nil {
		t.Fatalf("branch LinkChild failed: %v", err)
	}
	surname := "Byron"
	if _, err := f.handler.WithBranch(branch).UpdatePerson(ctx, command.UpdatePersonInput{
		ID: person.ID, Surname: &surname, Version: person.Version,
	}); err != nil {
		t.Fatalf("branch UpdatePerson failed: %v", err)
	}

	// Main deletes the person out from under the branch.
	current, err := f.readStore.GetPerson(ctx, domain.MainBranchID, person.ID)
	if err != nil {
		t.Fatalf("GetPerson on main failed: %v", err)
	}
	if err := f.handler.DeletePerson(ctx, command.DeletePersonInput{
		ID: person.ID, Version: current.Version, Reason: "duplicate",
	}); err != nil {
		t.Fatalf("main DeletePerson failed: %v", err)
	}

	// "main" is the only resolution the delete-vs-edit conflict offers, so the
	// caller has no choice here — which is exactly why the guard has to exist.
	_, err = f.handler.MergeBranch(ctx, command.MergeBranchInput{
		BranchID:    branch.ID,
		Resolutions: map[uuid.UUID]command.MergeResolution{person.ID: command.ResolveMain},
	})
	if !errors.Is(err, command.ErrMergeDanglingReference) {
		t.Fatalf("MergeBranch error = %v, want ErrMergeDanglingReference", err)
	}

	// Refused before the claim, and main gained no phantom child.
	after, err := f.branchStore.Get(ctx, branch.ID)
	if err != nil {
		t.Fatalf("Get branch failed: %v", err)
	}
	if after.Status != domain.BranchStatusActive {
		t.Errorf("branch status = %s, want active", after.Status)
	}
	children, err := f.readStore.GetFamilyChildren(ctx, domain.MainBranchID, family.ID)
	if err != nil {
		t.Fatalf("GetFamilyChildren failed: %v", err)
	}
	if len(children) != 0 {
		t.Errorf("main gained %d family children on a refused merge, want 0", len(children))
	}
}

// A link to a person who survives on main is not a dangling reference.
func TestMergeBranch_ChildLinkToLivePersonMerges(t *testing.T) {
	f := newBranchFixture()
	ctx := context.Background()

	person, err := f.handler.CreatePerson(ctx, command.CreatePersonInput{GivenName: "Ada", Surname: "Lovelace"})
	if err != nil {
		t.Fatalf("CreatePerson failed: %v", err)
	}
	partner, err := f.handler.CreatePerson(ctx, command.CreatePersonInput{GivenName: "William", Surname: "King"})
	if err != nil {
		t.Fatalf("CreatePerson (partner) failed: %v", err)
	}
	family, err := f.handler.CreateFamily(ctx, command.CreateFamilyInput{Partner1ID: &partner.ID})
	if err != nil {
		t.Fatalf("CreateFamily failed: %v", err)
	}
	branch, err := f.handler.CreateBranch(ctx, "parentage-theory", "")
	if err != nil {
		t.Fatalf("CreateBranch failed: %v", err)
	}
	if _, err := f.handler.WithBranch(branch).LinkChild(ctx, command.LinkChildInput{
		FamilyID: family.ID, ChildID: person.ID, RelationType: "biological",
	}); err != nil {
		t.Fatalf("branch LinkChild failed: %v", err)
	}

	if _, err := f.handler.MergeBranch(ctx, command.MergeBranchInput{BranchID: branch.ID}); err != nil {
		t.Fatalf("MergeBranch failed: %v", err)
	}

	children, err := f.readStore.GetFamilyChildren(ctx, domain.MainBranchID, family.ID)
	if err != nil {
		t.Fatalf("GetFamilyChildren failed: %v", err)
	}
	if len(children) != 1 || children[0].PersonID != person.ID {
		t.Errorf("main family children = %+v, want one entry for %s", children, person.ID)
	}
}

// TestMergeBranch_InterruptedClaimIsNotReplayedTwice covers the window between
// the claim's append and the projection that flips the registry status.
//
// The append is durable first, so a projection failure leaves a branch that is
// claimed in the log but still reads "active" in the registry. Trusting the
// registry alone, a retry would sail past the status guard, observe the
// already-incremented stream version, append a SECOND BranchMerged, and replay
// the whole branch onto main again — duplicating history permanently.
//
// The state is reproduced directly: append the claim behind the handler's back
// and leave the registry untouched, which is exactly what that failure yields.
func TestMergeBranch_InterruptedClaimIsNotReplayedTwice(t *testing.T) {
	s := seedMerge(t, "Byron")
	ctx := context.Background()

	// Reproduce the interrupted claim: the event lands, the projection doesn't.
	orphaned := domain.NewBranchMerged(s.branch.ID, s.branch.BasePosition, s.branch.BasePosition, "interrupted")
	if err := s.f.eventStore.Append(ctx, s.branch.ID, "branch", []domain.Event{orphaned}, 1,
		repository.AppendScope{BranchID: domain.BranchID(s.branch.ID), BasePosition: s.branch.BasePosition}); err != nil {
		t.Fatalf("seeding the interrupted claim failed: %v", err)
	}

	// The registry still says active — the trap the status guard alone falls into.
	before, err := s.f.branchStore.Get(ctx, s.branch.ID)
	if err != nil {
		t.Fatalf("Get branch failed: %v", err)
	}
	if before.Status != domain.BranchStatusActive {
		t.Fatalf("precondition: branch status = %s, want active", before.Status)
	}
	mainBefore := branchEventsFor(t, s.f, s.person, domain.MainBranchID)

	_, err = s.f.handler.MergeBranch(ctx, command.MergeBranchInput{BranchID: s.branch.ID})
	if !errors.Is(err, command.ErrMergeAlreadyClaimed) {
		t.Fatalf("MergeBranch error = %v, want ErrMergeAlreadyClaimed", err)
	}

	// No second replay onto main.
	if after := branchEventsFor(t, s.f, s.person, domain.MainBranchID); len(after) != len(mainBefore) {
		t.Errorf("main gained %d events from a re-claim, want 0", len(after)-len(mainBefore))
	}

	// Exactly one BranchMerged on the branch's own stream.
	branchEvents := branchEventsFor(t, s.f, s.branch.ID, domain.BranchID(s.branch.ID))
	merged := 0
	for _, evt := range branchEvents {
		if evt.EventType == "BranchMerged" {
			merged++
		}
	}
	if merged != 1 {
		t.Errorf("branch stream carries %d BranchMerged events, want exactly 1", merged)
	}

	// And the registry the failed attempt left behind was repaired.
	healed, err := s.f.branchStore.Get(ctx, s.branch.ID)
	if err != nil {
		t.Fatalf("Get branch failed: %v", err)
	}
	if healed.Status != domain.BranchStatusMerged {
		t.Errorf("branch status = %s, want merged (the refusal should repair the registry)", healed.Status)
	}
}

// --- Merge-plan staleness (#698) -------------------------------------------
//
// The window these tests inject into is the one between PlanMerge computing the
// conflict verdict and replayOntoMain writing against it. A mainline write
// landing there was never compared with the branch's events, so replaying over
// it would silently override it — exactly what ADR-005 §Conflict definition
// exists to prevent.

// stalePositions wraps the merge's position source so a test can land a rival
// mainline write inside that window. GetMaxPosition is called between PlanMerge
// returning and the staleness check, which makes it a precise hook for it.
//
// It is NOT the only collaborator call MergeBranch makes in that span, and a
// test extending this harness must not assume so: validateNoDanglingReferences
// runs first and reads readStore.GetPerson for every ChildLinkedToFamily event
// naming a person outside the replay set. The seedMerge fixture has no family
// events, so for THESE tests GetMaxPosition is the only such call; a
// family-link scenario would need to account for that read as well.
//
// rival is armed by the test AFTER setup: CreateBranch reads the same position
// source, and firing there would put the write before planning rather than
// after it, testing nothing.
// The embedded store carries every method but GetMaxPosition, which this type
// intercepts.
type stalePositions struct {
	repository.SnapshotStore
	rival func()
	fired bool
}

func (p *stalePositions) GetMaxPosition(ctx context.Context) (int64, error) {
	if p.rival != nil && !p.fired {
		p.fired = true
		p.rival()
	}
	return p.SnapshotStore.GetMaxPosition(ctx)
}

// newStaleMergeFixture is newBranchFixture with the position source wrapped, so
// the returned hook can inject into the planning→replay window.
func newStaleMergeFixture() (*branchFixture, *stalePositions) {
	var positions *stalePositions
	f := newBranchFixtureWith(branchFixtureDeps{
		wrapPositions: func(inner repository.SnapshotStore) repository.SnapshotStore {
			positions = &stalePositions{SnapshotStore: inner}
			return positions
		},
	})
	return f, positions
}

// seedStaleMerge is seedMerge over the injectable fixture: one person on main, a
// branch forked after it, and the branch's surname edit applied.
func seedStaleMerge(t *testing.T, branchSurname string) (mergeSeed, *stalePositions) {
	t.Helper()
	f, positions := newStaleMergeFixture()
	return seedMergeInto(t, f, branchSurname), positions
}

// planRacingEventStore fires a rival write when the merge plan reads MAIN's side
// of the diff — i.e. between PlanMerge's branch-side read and its main-side
// read, which is the window the version pin has to bracket.
//
// ReadStreamsForBranch is that main-side read (query.readMainTail) and nothing
// else on the merge path calls it, so the hook is unambiguous. It fires BEFORE
// delegating, so the rival's event is visible to the main tail the conflict
// classifier then compares against — which is what makes the pin's placement,
// and not merely its existence, the thing under test.
type planRacingEventStore struct {
	repository.EventStore
	rival func()
	fired bool
}

func (s *planRacingEventStore) ReadStreamsForBranch(ctx context.Context, streamIDs []uuid.UUID, branchID domain.BranchID, fromPosition int64, limit int) ([]repository.StoredEvent, error) {
	if s.rival != nil && !s.fired {
		s.fired = true
		s.rival()
	}
	return s.EventStore.ReadStreamsForBranch(ctx, streamIDs, branchID, fromPosition, limit)
}

// TestMergeBranch_StalePlanRefuses is the core of #698: main gains an edit to a
// stream the merge would replay, after the verdict was computed. The write is to
// a DIFFERENT field than the branch touched, so conflict detection would have
// cleared it — which is the point. The guard fails safe on movement rather than
// re-deriving a verdict, so it fires here too, and the remedy (re-plan, retry)
// costs one extra round trip instead of an overwritten mainline edit.
func TestMergeBranch_StalePlanRefuses(t *testing.T) {
	s, positions := seedStaleMerge(t, "Byron")
	ctx := context.Background()

	positions.rival = func() {
		person, err := s.f.readStore.GetPerson(ctx, domain.MainBranchID, s.person)
		if err != nil {
			t.Errorf("rival GetPerson failed: %v", err)
			return
		}
		given := "Augusta"
		if _, err := s.f.handler.UpdatePerson(ctx, command.UpdatePersonInput{
			ID:        s.person,
			GivenName: &given,
			Version:   person.Version,
		}); err != nil {
			t.Errorf("rival UpdatePerson failed: %v", err)
		}
	}

	_, err := s.f.handler.MergeBranch(ctx, command.MergeBranchInput{BranchID: s.branch.ID})
	if !errors.Is(err, command.ErrMergePlanStale) {
		t.Fatalf("MergeBranch error = %v, want ErrMergePlanStale", err)
	}
	// A stale plan is a refusal, not the partially-applied state — a client told
	// the wrong one either retries when it must not, or gives up when a retry
	// would have worked.
	if errors.Is(err, command.ErrMergePartiallyApplied) {
		t.Errorf("a pre-claim staleness refusal also matched ErrMergePartiallyApplied: %v", err)
	}

	// Nothing written: main still carries only the rival's own edit, and the
	// branch's surname never reached it.
	onMain, err := s.f.readStore.GetPerson(ctx, domain.MainBranchID, s.person)
	if err != nil {
		t.Fatalf("GetPerson on main failed: %v", err)
	}
	if onMain.Surname != "Lovelace" {
		t.Errorf("main surname = %q, want it untouched by the refused merge (%q)", onMain.Surname, "Lovelace")
	}
	if onMain.GivenName != "Augusta" {
		t.Errorf("main given name = %q, want the mainline write %q to have survived", onMain.GivenName, "Augusta")
	}

	// And the branch is still mergeable, so re-planning and retrying is open.
	branch, err := s.f.branchStore.Get(ctx, s.branch.ID)
	if err != nil {
		t.Fatalf("branchStore.Get failed: %v", err)
	}
	if branch.Status != domain.BranchStatusActive {
		t.Errorf("branch status = %q, want it still active after a refusal", branch.Status)
	}
}

// TestMergeBranch_StalePlanRefusesWriteInsideThePlanningWindow pins the ORDER of
// PlanMerge's reads, which is what makes the pin mean anything at all.
//
// The rival write lands between the branch-side read and the main-side read —
// inside planning, not after it. It touches a field the branch never touched, so
// the classifier clears it either way; the version pin is the only thing that can
// catch it, and only if the pin was taken BEFORE the main tail was read.
//
// Capture the versions after that read instead (the ordering this test was
// written against, and which it fails on) and the write is baked into the pin
// while never being compared against the branch's events: `current == planned`
// passes, the merge returns 200, and main has been replayed over an event the
// verdict never saw. That is #698 itself, in the one direction the guard cannot
// observe — the pin ending up NEWER than the verdict.
func TestMergeBranch_StalePlanRefusesWriteInsideThePlanningWindow(t *testing.T) {
	var racing *planRacingEventStore
	f := newBranchFixtureWith(branchFixtureDeps{
		wrapEvents: func(inner repository.EventStore) repository.EventStore {
			racing = &planRacingEventStore{EventStore: inner}
			return racing
		},
	})
	s := seedMergeInto(t, f, "Byron")
	ctx := context.Background()

	racing.rival = func() {
		person, err := f.readStore.GetPerson(ctx, domain.MainBranchID, s.person)
		if err != nil {
			t.Errorf("rival GetPerson failed: %v", err)
			return
		}
		given := "Augusta"
		if _, err := f.handler.UpdatePerson(ctx, command.UpdatePersonInput{
			ID:        s.person,
			GivenName: &given,
			Version:   person.Version,
		}); err != nil {
			t.Errorf("rival UpdatePerson failed: %v", err)
		}
	}

	_, err := f.handler.MergeBranch(ctx, command.MergeBranchInput{BranchID: s.branch.ID})
	if !errors.Is(err, command.ErrMergePlanStale) {
		t.Fatalf("MergeBranch error = %v, want ErrMergePlanStale", err)
	}
	if !racing.fired {
		t.Fatal("the rival write never fired, so nothing was raced")
	}

	onMain, err := f.readStore.GetPerson(ctx, domain.MainBranchID, s.person)
	if err != nil {
		t.Fatalf("GetPerson on main failed: %v", err)
	}
	if onMain.Surname != "Lovelace" {
		t.Errorf("main surname = %q, want it untouched by the refused merge (%q)", onMain.Surname, "Lovelace")
	}
	if onMain.GivenName != "Augusta" {
		t.Errorf("main given name = %q, want the mainline write %q to have survived", onMain.GivenName, "Augusta")
	}
}

// TestMergeBranch_StalePlanRefusesBranchCreatedStream closes the hole an
// Append-only guard would leave. A stream the branch created is appended to main
// with expectedVersion -1, and ALL THREE backends skip optimistic concurrency
// entirely when expectedVersion is negative — the `expectedVersion >= 0` gate in
// postgres/eventstore.go (the primary backend, ADR-002), sqlite/eventstore.go
// and memory/eventstore.go. So if main gains that stream in the window, Append
// would happily succeed on top of it. Only the explicit version comparison
// catches this, and this test runs against the memory store, so the postgres and
// sqlite gates are pinned by citation rather than by execution here.
func TestMergeBranch_StalePlanRefusesBranchCreatedStream(t *testing.T) {
	f, positions := newStaleMergeFixture()
	ctx := context.Background()

	branch, err := f.handler.CreateBranch(ctx, "new-person", "")
	if err != nil {
		t.Fatalf("CreateBranch failed: %v", err)
	}
	fresh, err := f.handler.WithBranch(branch).CreatePerson(ctx, command.CreatePersonInput{
		GivenName: "Grace",
		Surname:   "Hopper",
	})
	if err != nil {
		t.Fatalf("branch CreatePerson failed: %v", err)
	}

	// Main has never seen this stream, so the plan captured version 0. The rival
	// creates the SAME stream on main, taking it to 1.
	positions.rival = func() {
		rivalPerson := &domain.Person{ID: fresh.ID, GivenName: "Grace", Surname: "Murray"}
		created := domain.NewPersonCreated(rivalPerson)
		if err := f.eventStore.Append(ctx, fresh.ID, "person", []domain.Event{created}, -1, repository.MainScope); err != nil {
			t.Errorf("rival create on main failed: %v", err)
		}
	}

	if _, err := f.handler.MergeBranch(ctx, command.MergeBranchInput{BranchID: branch.ID}); !errors.Is(err, command.ErrMergePlanStale) {
		t.Fatalf("MergeBranch error = %v, want ErrMergePlanStale", err)
	}

	// Main's own create is the only event on the stream — the branch's was not
	// replayed on top of it.
	onMain := branchEventsFor(t, f, fresh.ID, domain.MainBranchID)
	if len(onMain) != 1 {
		t.Errorf("main has %d events for the contested stream, want only the rival's 1", len(onMain))
	}
	stored, err := f.branchStore.Get(ctx, branch.ID)
	if err != nil {
		t.Fatalf("branchStore.Get failed: %v", err)
	}
	if stored.Status != domain.BranchStatusActive {
		t.Errorf("branch status = %q, want it still active after a refusal", stored.Status)
	}
}

// TestMergeBranch_StaleCheckIgnoresMainResolvedStream scopes the guard to what
// is actually written. A stream resolved to "main" is not replayed, so main
// moving under it cannot be overridden by this merge and must not block it.
func TestMergeBranch_StaleCheckIgnoresMainResolvedStream(t *testing.T) {
	s, positions := seedStaleMerge(t, "Byron")
	ctx := context.Background()

	positions.rival = func() {
		person, err := s.f.readStore.GetPerson(ctx, domain.MainBranchID, s.person)
		if err != nil {
			t.Errorf("rival GetPerson failed: %v", err)
			return
		}
		king := "King"
		if _, err := s.f.handler.UpdatePerson(ctx, command.UpdatePersonInput{
			ID:      s.person,
			Surname: &king,
			Version: person.Version,
		}); err != nil {
			t.Errorf("rival UpdatePerson failed: %v", err)
		}
	}

	res, err := s.f.handler.MergeBranch(ctx, command.MergeBranchInput{
		BranchID:    s.branch.ID,
		Resolutions: map[uuid.UUID]command.MergeResolution{s.person: command.ResolveMain},
	})
	if err != nil {
		t.Fatalf("MergeBranch failed for a stream resolved to main: %v", err)
	}
	if len(res.SkippedStreamIDs) != 1 || res.SkippedStreamIDs[0] != s.person {
		t.Errorf("SkippedStreamIDs = %v, want [%s]", res.SkippedStreamIDs, s.person)
	}

	onMain, err := s.f.readStore.GetPerson(ctx, domain.MainBranchID, s.person)
	if err != nil {
		t.Fatalf("GetPerson on main failed: %v", err)
	}
	if onMain.Surname != "King" {
		t.Errorf("main surname = %q, want the mainline write %q kept", onMain.Surname, "King")
	}
}

// TestMergeBranch_StaleCheckIgnoresUntouchedStream keeps the guard from making a
// merge hostage to unrelated mainline activity — the same scoping rule ADR-005
// applies to the whole branch comparison. It is also the happy path with noise:
// a clean merge still merges while main is busy elsewhere.
func TestMergeBranch_StaleCheckIgnoresUntouchedStream(t *testing.T) {
	s, positions := seedStaleMerge(t, "Byron")
	ctx := context.Background()

	positions.rival = func() {
		if _, err := s.f.handler.CreatePerson(ctx, command.CreatePersonInput{
			GivenName: "Charles",
			Surname:   "Babbage",
		}); err != nil {
			t.Errorf("rival CreatePerson failed: %v", err)
		}
	}

	res, err := s.f.handler.MergeBranch(ctx, command.MergeBranchInput{BranchID: s.branch.ID})
	if err != nil {
		t.Fatalf("MergeBranch failed on unrelated mainline activity: %v", err)
	}
	if res.ReplayedEventCount != 1 {
		t.Errorf("ReplayedEventCount = %d, want 1", res.ReplayedEventCount)
	}
	if res.Branch.Status != domain.BranchStatusMerged {
		t.Errorf("branch status = %q, want %q", res.Branch.Status, domain.BranchStatusMerged)
	}

	onMain, err := s.f.readStore.GetPerson(ctx, domain.MainBranchID, s.person)
	if err != nil {
		t.Fatalf("GetPerson on main failed: %v", err)
	}
	if onMain.Surname != "Byron" {
		t.Errorf("main surname = %q, want the branch's %q", onMain.Surname, "Byron")
	}
}

// TestMergeBranch_StalePlanDuringReplayIsPartiallyApplied covers the residual
// window the pre-claim check cannot close: the check is not a lock, so a write
// landing between it and the append is caught by replayStream's own assertion
// instead. Past the claim the branch is already terminal, so this correctly
// surfaces as the partially-applied state — a client must NOT retry it — while
// still carrying ErrMergePlanStale so the message says what went wrong.
//
// Injected at claimMerge's append, which is inside that window by construction.
//
// It is also the recoverability case: this branch has ONE stream, so the replay
// fails on the first thing it tries and main is never modified at all. The error
// has to say so — "claimed but main untouched" and "claimed and half-applied"
// need different responses from whoever reads the 500, and only the message can
// tell them apart (#685 tracks actually resuming either).
func TestMergeBranch_StalePlanDuringReplayIsPartiallyApplied(t *testing.T) {
	var racing *racingEventStore
	f := newBranchFixtureWith(branchFixtureDeps{
		wrapEvents: func(inner repository.EventStore) repository.EventStore {
			racing = &racingEventStore{EventStore: inner}
			return racing
		},
	})
	s := seedMergeInto(t, f, "Byron")
	ctx := context.Background()

	// Fires as the claim is appended: past the pre-claim check, before the
	// replay. Written through the UNWRAPPED store so it does not re-enter the hook.
	racing.rival = func() {
		changes := map[string]any{"given_name": "Augusta"}
		if err := f.eventStore.Append(ctx, s.person, "person",
			[]domain.Event{domain.NewPersonUpdated(s.person, changes)}, -1, repository.MainScope); err != nil {
			t.Errorf("rival mainline write failed: %v", err)
		}
	}

	_, err := f.handler.MergeBranch(ctx, command.MergeBranchInput{BranchID: s.branch.ID})
	if !errors.Is(err, command.ErrMergePlanStale) {
		t.Fatalf("MergeBranch error = %v, want it to carry ErrMergePlanStale", err)
	}
	// Past the claim, the honest report is the partially-applied state.
	if !errors.Is(err, command.ErrMergePartiallyApplied) {
		t.Errorf("MergeBranch error = %v, want it wrapped in ErrMergePartiallyApplied past the claim", err)
	}
	// Nothing reached main, and the message must say so unambiguously rather
	// than leaving an operator to infer it from "0 of 1 events".
	if !strings.Contains(err.Error(), "MAIN WAS NOT MODIFIED") {
		t.Errorf("MergeBranch error = %v, want it to state that main was not modified", err)
	}
	if strings.Contains(err.Error(), "MAIN IS PARTIALLY UPDATED") {
		t.Errorf("MergeBranch error = %v, reported a partial application when main was untouched", err)
	}

	// The branch's edit did not land on top of the mainline write.
	onMain, err := f.readStore.GetPerson(ctx, domain.MainBranchID, s.person)
	if err != nil {
		t.Fatalf("GetPerson on main failed: %v", err)
	}
	if onMain.Surname != "Lovelace" {
		t.Errorf("main surname = %q, want the branch's replay to have been refused", onMain.Surname)
	}
}

// TestMergeBranch_ReplayFailureAfterProgressReportsPartialApplication is the
// other half of the pair above: a branch with TWO streams, whose second stream
// goes stale. The first stream is already on main by then, so this really is the
// partially-applied state and the message must not claim main was untouched.
func TestMergeBranch_ReplayFailureAfterProgressReportsPartialApplication(t *testing.T) {
	var racing *racingEventStore
	f := newBranchFixtureWith(branchFixtureDeps{
		wrapEvents: func(inner repository.EventStore) repository.EventStore {
			racing = &racingEventStore{EventStore: inner}
			return racing
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

	// Stale only the SECOND stream, so the first replays successfully first.
	racing.rival = func() {
		changes := map[string]any{"given_name": "Augusta"}
		if err := f.eventStore.Append(ctx, second.ID, "person",
			[]domain.Event{domain.NewPersonUpdated(second.ID, changes)}, -1, repository.MainScope); err != nil {
			t.Errorf("rival mainline write failed: %v", err)
		}
	}

	_, err = f.handler.MergeBranch(ctx, command.MergeBranchInput{BranchID: branch.ID})
	if !errors.Is(err, command.ErrMergePartiallyApplied) {
		t.Fatalf("MergeBranch error = %v, want ErrMergePartiallyApplied", err)
	}
	if !strings.Contains(err.Error(), "MAIN IS PARTIALLY UPDATED") {
		t.Errorf("MergeBranch error = %v, want it to state that main is partially updated", err)
	}
	if strings.Contains(err.Error(), "MAIN WAS NOT MODIFIED") {
		t.Errorf("MergeBranch error = %v, claimed main was untouched after a stream had replayed", err)
	}

	// The first stream really did land, which is what makes this the partial case.
	landed, err := f.readStore.GetPerson(ctx, domain.MainBranchID, first.ID)
	if err != nil {
		t.Fatalf("GetPerson on main failed: %v", err)
	}
	if landed.Surname != "Byron" {
		t.Errorf("main surname for the first stream = %q, want the branch's %q", landed.Surname, "Byron")
	}
}
