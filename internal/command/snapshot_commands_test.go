package command_test

import (
	"context"
	"errors"
	"testing"

	"github.com/google/uuid"

	"github.com/cacack/my-family/internal/command"
	"github.com/cacack/my-family/internal/domain"
	"github.com/cacack/my-family/internal/repository"
	"github.com/cacack/my-family/internal/repository/memory"
)

// snapshotFixture wires the in-memory backends the snapshot commands need.
type snapshotFixture struct {
	eventStore    *memory.EventStore
	readStore     *memory.ReadModelStore
	snapshotStore *memory.SnapshotStore
	handler       *command.Handler
}

func newSnapshotFixture() *snapshotFixture {
	eventStore := memory.NewEventStore()
	readStore := memory.NewReadModelStore()
	snapshotStore := memory.NewSnapshotStore(eventStore)

	return &snapshotFixture{
		eventStore:    eventStore,
		readStore:     readStore,
		snapshotStore: snapshotStore,
		handler: command.NewHandlerWithBranches(
			eventStore, readStore, memory.NewBranchStore(), snapshotStore),
	}
}

// seedPerson appends one mainline event so the log head is non-zero.
func (f *snapshotFixture) seedPerson(t *testing.T, given string) {
	t.Helper()
	if _, err := f.handler.CreatePerson(context.Background(),
		command.CreatePersonInput{GivenName: given, Surname: "Lovelace"}); err != nil {
		t.Fatalf("CreatePerson failed: %v", err)
	}
}

// snapshotEvents returns every snapshot lifecycle event on the mainline log.
func (f *snapshotFixture) snapshotEvents(t *testing.T) []repository.StoredEvent {
	t.Helper()
	all, err := f.eventStore.ReadAll(context.Background(), 0, 1000)
	if err != nil {
		t.Fatalf("ReadAll failed: %v", err)
	}
	var out []repository.StoredEvent
	for _, e := range all {
		if e.EventType == "SnapshotCreated" || e.EventType == "SnapshotDeleted" {
			out = append(out, e)
		}
	}
	return out
}

// TestCreateSnapshot is the core of issue #624: creating a snapshot appends an
// event and the REGISTRY ROW COMES FROM THE PROJECTION, not a direct store write.
func TestCreateSnapshot(t *testing.T) {
	f := newSnapshotFixture()
	ctx := context.Background()

	f.seedPerson(t, "Ada")

	snapshot, err := f.handler.CreateSnapshot(ctx, "Pre-DNA results", "before the test came back")
	if err != nil {
		t.Fatalf("CreateSnapshot failed: %v", err)
	}

	if snapshot.Name != "Pre-DNA results" {
		t.Errorf("Name = %q, want %q", snapshot.Name, "Pre-DNA results")
	}
	if snapshot.Description != "before the test came back" {
		t.Errorf("Description = %q, want the seeded description", snapshot.Description)
	}
	if snapshot.ID == uuid.Nil {
		t.Error("snapshot ID is nil")
	}

	// The event is on the log, on the snapshot's own stream.
	events := f.snapshotEvents(t)
	if len(events) != 1 {
		t.Fatalf("snapshot events = %d, want 1", len(events))
	}
	if events[0].EventType != "SnapshotCreated" {
		t.Errorf("event type = %q, want SnapshotCreated", events[0].EventType)
	}
	if events[0].StreamID != snapshot.ID {
		t.Errorf("stream id = %s, want the snapshot's id %s", events[0].StreamID, snapshot.ID)
	}
	if events[0].StreamType != "snapshot" {
		t.Errorf("stream type = %q, want snapshot", events[0].StreamType)
	}

	// The registry row was written by the PROJECTION, so it carries the event's
	// values — check against the event, not against the returned struct (which is
	// itself a read-back of this row and would compare with itself).
	decoded, err := events[0].DecodeEvent()
	if err != nil {
		t.Fatalf("DecodeEvent failed: %v", err)
	}
	created, ok := decoded.(domain.SnapshotCreated)
	if !ok {
		t.Fatalf("decoded event is %T, want domain.SnapshotCreated", decoded)
	}

	stored, err := f.snapshotStore.Get(ctx, snapshot.ID)
	if err != nil {
		t.Fatalf("snapshotStore.Get failed: %v", err)
	}
	if stored.Name != created.Name || stored.Description != created.Description {
		t.Errorf("registry row = %+v, want the event's name/description %+v", stored, created)
	}
	if stored.Position != created.Position {
		t.Errorf("registry position = %d, want the event's %d", stored.Position, created.Position)
	}
	if !stored.CreatedAt.Equal(created.OccurredAt()) {
		t.Errorf("registry CreatedAt = %s, want the event timestamp %s", stored.CreatedAt, created.OccurredAt())
	}
}

// TestCreateSnapshot_ReturnsTheProjectedRecord pins why CreateSnapshot reads the
// registry back instead of returning the struct it built: the projection derives
// CreatedAt from the event, so returning the locally built value would make the
// create response disagree with every later read of the same snapshot.
func TestCreateSnapshot_ReturnsTheProjectedRecord(t *testing.T) {
	f := newSnapshotFixture()
	ctx := context.Background()

	created, err := f.handler.CreateSnapshot(ctx, "milestone", "note")
	if err != nil {
		t.Fatalf("CreateSnapshot failed: %v", err)
	}

	fetched, err := f.snapshotStore.Get(ctx, created.ID)
	if err != nil {
		t.Fatalf("snapshotStore.Get failed: %v", err)
	}

	if !created.CreatedAt.Equal(fetched.CreatedAt) {
		t.Errorf("create response CreatedAt = %s, but a later read gives %s — they must agree",
			created.CreatedAt, fetched.CreatedAt)
	}
	if created.Name != fetched.Name || created.Description != fetched.Description || created.Position != fetched.Position {
		t.Errorf("create response = %+v, want it to equal the stored record %+v", created, fetched)
	}
}

// TestDeleteSnapshot_LegacyRowWithoutEvents covers the snapshots that predate
// #624: they were written straight to the registry, so their stream holds no
// SnapshotCreated and the append has to claim a NEW stream rather than version 0.
func TestDeleteSnapshot_LegacyRowWithoutEvents(t *testing.T) {
	f := newSnapshotFixture()
	ctx := context.Background()

	// Seed the registry the way the pre-#624 code did: a direct store write, no event.
	legacy, err := domain.NewSnapshot("written before #624", "no event on its stream", 0)
	if err != nil {
		t.Fatalf("NewSnapshot failed: %v", err)
	}
	if err := f.snapshotStore.Create(ctx, legacy); err != nil {
		t.Fatalf("seeding the legacy row failed: %v", err)
	}
	if events := f.snapshotEvents(t); len(events) != 0 {
		t.Fatalf("seeded %d snapshot events, want 0 — the fixture must mimic a direct write", len(events))
	}

	if err := f.handler.DeleteSnapshot(ctx, legacy.ID); err != nil {
		t.Fatalf("DeleteSnapshot on a legacy row failed: %v", err)
	}

	if _, err := f.snapshotStore.Get(ctx, legacy.ID); !errors.Is(err, repository.ErrSnapshotNotFound) {
		t.Errorf("Get after delete = %v, want ErrSnapshotNotFound", err)
	}

	events := f.snapshotEvents(t)
	if len(events) != 1 || events[0].EventType != "SnapshotDeleted" {
		t.Fatalf("snapshot events = %+v, want exactly one SnapshotDeleted", events)
	}
	if events[0].Version != 1 {
		t.Errorf("tombstone version = %d, want 1 (first event on a stream that had none)", events[0].Version)
	}
}

// TestCreateSnapshot_PositionExcludesItsOwnEvent pins the chicken-and-egg
// resolution the issue asked for: the snapshot marks the log as it stood BEFORE
// its own creation event, so the marker never perturbs what it marks.
func TestCreateSnapshot_PositionExcludesItsOwnEvent(t *testing.T) {
	f := newSnapshotFixture()
	ctx := context.Background()

	f.seedPerson(t, "Ada")

	headBefore, err := f.snapshotStore.GetMaxPosition(ctx)
	if err != nil {
		t.Fatalf("GetMaxPosition failed: %v", err)
	}

	snapshot, err := f.handler.CreateSnapshot(ctx, "milestone", "")
	if err != nil {
		t.Fatalf("CreateSnapshot failed: %v", err)
	}

	if snapshot.Position != headBefore {
		t.Errorf("snapshot position = %d, want the pre-append head %d", snapshot.Position, headBefore)
	}

	headAfter, err := f.snapshotStore.GetMaxPosition(ctx)
	if err != nil {
		t.Fatalf("GetMaxPosition failed: %v", err)
	}
	if headAfter <= snapshot.Position {
		t.Errorf("head after = %d, want it beyond the snapshot position %d (its own event moved it)",
			headAfter, snapshot.Position)
	}

	// The snapshot's own event must sit outside the range it marks.
	events := f.snapshotEvents(t)
	if len(events) != 1 {
		t.Fatalf("snapshot events = %d, want 1", len(events))
	}
	if events[0].Position <= snapshot.Position {
		t.Errorf("SnapshotCreated at position %d, want it after the marked position %d",
			events[0].Position, snapshot.Position)
	}
}

// TestSnapshotRegistryRebuildsFromEvents is the payoff of the decision: the
// registry is derived data. Replaying the log into a fresh store reconstructs
// every snapshot, which a directly-written store could not do.
func TestSnapshotRegistryRebuildsFromEvents(t *testing.T) {
	f := newSnapshotFixture()
	ctx := context.Background()

	f.seedPerson(t, "Ada")
	first, err := f.handler.CreateSnapshot(ctx, "Pre-DNA results", "before")
	if err != nil {
		t.Fatalf("CreateSnapshot failed: %v", err)
	}
	f.seedPerson(t, "Grace")
	second, err := f.handler.CreateSnapshot(ctx, "After courthouse trip", "after")
	if err != nil {
		t.Fatalf("CreateSnapshot failed: %v", err)
	}
	// A third, deleted before the rebuild: its tombstone must replay too, or a
	// rebuild would resurrect snapshots the user removed.
	discarded, err := f.handler.CreateSnapshot(ctx, "mistake", "")
	if err != nil {
		t.Fatalf("CreateSnapshot failed: %v", err)
	}
	if err := f.handler.DeleteSnapshot(ctx, discarded.ID); err != nil {
		t.Fatalf("DeleteSnapshot failed: %v", err)
	}

	// Rebuild into an empty registry by replaying the log.
	rebuilt := memory.NewSnapshotStore(f.eventStore)
	projector := repository.NewProjectorWithSnapshots(memory.NewReadModelStore(), nil, rebuilt)

	all, err := f.eventStore.ReadAll(ctx, 0, 1000)
	if err != nil {
		t.Fatalf("ReadAll failed: %v", err)
	}
	for _, stored := range all {
		event, err := stored.DecodeEvent()
		if err != nil {
			t.Fatalf("DecodeEvent(%s) failed: %v", stored.EventType, err)
		}
		if err := projector.Project(ctx, event, stored.Version, domain.MainBranchID); err != nil {
			t.Fatalf("Project(%s) failed: %v", stored.EventType, err)
		}
	}

	for _, want := range []*domain.Snapshot{first, second} {
		got, err := rebuilt.Get(ctx, want.ID)
		if err != nil {
			t.Fatalf("rebuilt registry is missing snapshot %q: %v", want.Name, err)
		}
		if got.Name != want.Name || got.Description != want.Description || got.Position != want.Position {
			t.Errorf("rebuilt snapshot = %+v, want %+v", got, want)
		}
		if !got.CreatedAt.Equal(want.CreatedAt) {
			t.Errorf("rebuilt CreatedAt = %s, want %s", got.CreatedAt, want.CreatedAt)
		}
	}

	if _, err := rebuilt.Get(ctx, discarded.ID); !errors.Is(err, repository.ErrSnapshotNotFound) {
		t.Errorf("rebuilt registry Get(deleted snapshot) = %v, want ErrSnapshotNotFound", err)
	}

	remaining, err := rebuilt.List(ctx)
	if err != nil {
		t.Fatalf("List failed: %v", err)
	}
	if len(remaining) != 2 {
		t.Errorf("rebuilt registry holds %d snapshots, want 2", len(remaining))
	}
}

// TestDeleteSnapshot_RemovesMarkerNotHistory covers ES-002: dropping a marker
// leaves every event it pointed at on the log.
func TestDeleteSnapshot_RemovesMarkerNotHistory(t *testing.T) {
	f := newSnapshotFixture()
	ctx := context.Background()

	f.seedPerson(t, "Ada")
	snapshot, err := f.handler.CreateSnapshot(ctx, "milestone", "")
	if err != nil {
		t.Fatalf("CreateSnapshot failed: %v", err)
	}

	before, err := f.eventStore.ReadAll(ctx, 0, 1000)
	if err != nil {
		t.Fatalf("ReadAll failed: %v", err)
	}

	if err := f.handler.DeleteSnapshot(ctx, snapshot.ID); err != nil {
		t.Fatalf("DeleteSnapshot failed: %v", err)
	}

	// Registry row gone.
	if _, err := f.snapshotStore.Get(ctx, snapshot.ID); !errors.Is(err, repository.ErrSnapshotNotFound) {
		t.Errorf("Get after delete = %v, want ErrSnapshotNotFound", err)
	}

	// Log only grew — nothing was rewritten or removed.
	after, err := f.eventStore.ReadAll(ctx, 0, 1000)
	if err != nil {
		t.Fatalf("ReadAll failed: %v", err)
	}
	if len(after) != len(before)+1 {
		t.Fatalf("log length = %d, want %d (one appended tombstone)", len(after), len(before)+1)
	}
	for i, e := range before {
		if after[i].ID != e.ID {
			t.Fatalf("event %d changed identity: %s became %s", i, e.ID, after[i].ID)
		}
	}
	if after[len(after)-1].EventType != "SnapshotDeleted" {
		t.Errorf("last event = %q, want SnapshotDeleted", after[len(after)-1].EventType)
	}
}

// TestDeleteSnapshot_UnknownAppendsNothing keeps a 404 from writing a tombstone
// for a snapshot that never existed.
func TestDeleteSnapshot_UnknownAppendsNothing(t *testing.T) {
	f := newSnapshotFixture()
	ctx := context.Background()

	err := f.handler.DeleteSnapshot(ctx, uuid.New())
	if !errors.Is(err, repository.ErrSnapshotNotFound) {
		t.Fatalf("DeleteSnapshot error = %v, want ErrSnapshotNotFound", err)
	}

	if events := f.snapshotEvents(t); len(events) != 0 {
		t.Errorf("snapshot events = %d, want 0 after a refused delete", len(events))
	}
}

// TestDeleteSnapshot_ConvergesOnAnExistingTombstone covers the state a delete
// leaves behind when its append succeeded but its projection did not — the same
// state a rival concurrent delete produces. Deleting again must converge the
// registry rather than fail, and must not append a second tombstone (ES-002).
func TestDeleteSnapshot_ConvergesOnAnExistingTombstone(t *testing.T) {
	f := newSnapshotFixture()
	ctx := context.Background()

	snapshot, err := f.handler.CreateSnapshot(ctx, "milestone", "")
	if err != nil {
		t.Fatalf("CreateSnapshot failed: %v", err)
	}

	// Tombstone on the log, registry row still present: append it behind the
	// handler's back, exactly as a failed projection would leave things.
	tombstone := domain.NewSnapshotDeleted(snapshot.ID)
	if err := f.eventStore.Append(ctx, snapshot.ID, "snapshot",
		[]domain.Event{tombstone}, 1, repository.MainScope); err != nil {
		t.Fatalf("seeding the tombstone failed: %v", err)
	}
	if _, err := f.snapshotStore.Get(ctx, snapshot.ID); err != nil {
		t.Fatalf("the registry row should still be present, got %v", err)
	}

	if err := f.handler.DeleteSnapshot(ctx, snapshot.ID); err != nil {
		t.Fatalf("DeleteSnapshot over an existing tombstone failed: %v", err)
	}

	if _, err := f.snapshotStore.Get(ctx, snapshot.ID); !errors.Is(err, repository.ErrSnapshotNotFound) {
		t.Errorf("Get after converge = %v, want ErrSnapshotNotFound", err)
	}

	// Exactly one tombstone: converging re-projects, it does not re-append.
	var tombstones int
	for _, e := range f.snapshotEvents(t) {
		if e.EventType == "SnapshotDeleted" {
			tombstones++
		}
	}
	if tombstones != 1 {
		t.Errorf("SnapshotDeleted events = %d, want 1", tombstones)
	}
}

// TestSnapshotCommands_RefuseOnBranch guards the gap ADR-005 leaves open: a
// branch snapshot needs a (branch_id, position) pointer the registry cannot yet
// hold, so recording one as mainline would be wrong.
func TestSnapshotCommands_RefuseOnBranch(t *testing.T) {
	f := newSnapshotFixture()
	ctx := context.Background()

	branch, err := domain.NewBranch("maternal-line", "", 0)
	if err != nil {
		t.Fatalf("NewBranch failed: %v", err)
	}
	scoped := f.handler.WithBranch(branch)

	if _, err := scoped.CreateSnapshot(ctx, "on a branch", ""); !errors.Is(err, command.ErrSnapshotNotBranchScoped) {
		t.Errorf("CreateSnapshot on a branch = %v, want ErrSnapshotNotBranchScoped", err)
	}
	if err := scoped.DeleteSnapshot(ctx, uuid.New()); !errors.Is(err, command.ErrSnapshotNotBranchScoped) {
		t.Errorf("DeleteSnapshot on a branch = %v, want ErrSnapshotNotBranchScoped", err)
	}

	if events := f.snapshotEvents(t); len(events) != 0 {
		t.Errorf("snapshot events = %d, want 0 after refused branch-scoped commands", len(events))
	}
}

// TestSnapshotCommands_RequireStore fails loudly rather than appending an event
// whose registry row would never appear.
func TestSnapshotCommands_RequireStore(t *testing.T) {
	handler := command.NewHandler(memory.NewEventStore(), memory.NewReadModelStore())
	ctx := context.Background()

	if _, err := handler.CreateSnapshot(ctx, "x", ""); !errors.Is(err, command.ErrSnapshotStoreRequired) {
		t.Errorf("CreateSnapshot error = %v, want ErrSnapshotStoreRequired", err)
	}
	if err := handler.DeleteSnapshot(ctx, uuid.New()); !errors.Is(err, command.ErrSnapshotStoreRequired) {
		t.Errorf("DeleteSnapshot error = %v, want ErrSnapshotStoreRequired", err)
	}
}

// TestCreateSnapshot_ValidationRefusesBeforeAppend keeps an invalid snapshot off
// the log entirely.
func TestCreateSnapshot_ValidationRefusesBeforeAppend(t *testing.T) {
	f := newSnapshotFixture()
	ctx := context.Background()

	if _, err := f.handler.CreateSnapshot(ctx, "", "no name"); !errors.Is(err, domain.ErrSnapshotNameRequired) {
		t.Fatalf("CreateSnapshot error = %v, want ErrSnapshotNameRequired", err)
	}

	if events := f.snapshotEvents(t); len(events) != 0 {
		t.Errorf("snapshot events = %d, want 0 after a validation failure", len(events))
	}
}

// TestCreateSnapshot_PositionSourceError surfaces a head-read failure instead of
// snapshotting a position it could not determine.
func TestCreateSnapshot_PositionSourceError(t *testing.T) {
	sentinel := errors.New("boom")
	handler := command.NewHandlerWithBranches(
		memory.NewEventStore(), memory.NewReadModelStore(), memory.NewBranchStore(),
		failingPositions{err: sentinel})

	if _, err := handler.CreateSnapshot(context.Background(), "x", ""); !errors.Is(err, sentinel) {
		t.Fatalf("CreateSnapshot error = %v, want it to wrap %v", err, sentinel)
	}
}
