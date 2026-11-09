package repository_test

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/cacack/my-family/internal/command"
	"github.com/cacack/my-family/internal/domain"
	"github.com/cacack/my-family/internal/repository"
	"github.com/cacack/my-family/internal/repository/memory"
)

var errRebuildTest = errors.New("injected")

type rebuildStores struct {
	events    *memory.EventStore
	read      *memory.ReadModelStore
	branches  *memory.BranchStore
	snapshots *memory.SnapshotStore
}

func newRebuildStores() rebuildStores {
	events := memory.NewEventStore()
	return rebuildStores{events: events, read: memory.NewReadModelStore(), branches: memory.NewBranchStore(), snapshots: memory.NewSnapshotStore(events)}
}

func (s rebuildStores) rebuild(t *testing.T, rs repository.ReadModelStore) (repository.RebuildResult, error) {
	t.Helper()
	return repository.RebuildReadModel(context.Background(), s.events, rs, s.branches, s.snapshots)
}

// TestRebuildReadModel_KeepsBrickWalls covers the one value written straight
// to the read model, a brick wall: a replay cannot derive it, so the rebuild
// must carry it over (#845).
func TestRebuildReadModel_KeepsBrickWalls(t *testing.T) {
	ctx := context.Background()
	s := newRebuildStores()
	h := command.NewHandlerWithBranches(s.events, s.read, s.branches, s.snapshots)

	person, err := h.CreatePerson(ctx, command.CreatePersonInput{GivenName: "Ada", Surname: "Example"})
	if err != nil {
		t.Fatalf("create person: %v", err)
	}
	if err := s.read.SetBrickWall(ctx, person.ID, "stuck"); err != nil {
		t.Fatalf("set brick wall: %v", err)
	}

	result, err := s.rebuild(t, s.read)
	if err != nil {
		t.Fatalf("rebuild: %v", err)
	}
	if logged, _ := s.events.ReadAll(ctx, 0, 100); result.Events != len(logged) {
		t.Errorf("events replayed = %d, want all %d logged", result.Events, len(logged))
	}
	p, _ := s.read.GetPerson(ctx, domain.MainBranchID, person.ID)
	if p == nil || p.BrickWallNote != "stuck" || p.BrickWallSince == nil {
		t.Errorf("person after rebuild = %+v, want the brick wall kept", p)
	}
}

// TestRebuildReadModel_DoesNotCarryOverUnloggedImportFields proves the rebuild
// no longer copies coordinates or EXIDs across its reset (#1225): they come
// from the log, so a value only the read model holds is not kept.
func TestRebuildReadModel_DoesNotCarryOverUnloggedImportFields(t *testing.T) {
	ctx := context.Background()
	s := newRebuildStores()
	h := command.NewHandlerWithBranches(s.events, s.read, s.branches, s.snapshots)
	person, err := h.CreatePerson(ctx, command.CreatePersonInput{GivenName: "Ada", Surname: "Example"})
	if err != nil {
		t.Fatalf("create person: %v", err)
	}
	lat := "N1.5"
	p, _ := s.read.GetPerson(ctx, domain.MainBranchID, person.ID)
	p.BirthPlaceLat = &lat
	if err := s.read.SavePerson(ctx, domain.MainBranchID, p); err != nil {
		t.Fatalf("save coordinates: %v", err)
	}
	if err := s.read.ReplacePersonExternalIDs(ctx, domain.MainBranchID, person.ID, []repository.PersonExternalIDReadModel{{PersonID: person.ID, Value: "P-1"}}); err != nil {
		t.Fatalf("save exid: %v", err)
	}

	if _, err := s.rebuild(t, s.read); err != nil {
		t.Fatalf("rebuild: %v", err)
	}
	if p, _ := s.read.GetPerson(ctx, domain.MainBranchID, person.ID); p == nil || p.BirthPlaceLat != nil {
		t.Errorf("person after rebuild = %+v, want it back without the unlogged coordinate", p)
	}
	if ids, _ := s.read.GetPersonExternalIDs(ctx, domain.MainBranchID, person.ID); len(ids) != 0 {
		t.Errorf("person exids after rebuild = %+v, want none", ids)
	}
}

// TestRebuildReadModel_DropsRowsTheLogDoesNotExplain proves the rebuild starts
// from an empty read model rather than layering the replay over what is there.
func TestRebuildReadModel_DropsRowsTheLogDoesNotExplain(t *testing.T) {
	ctx := context.Background()
	s := newRebuildStores()
	stray := uuid.New()
	if err := s.read.SaveNote(ctx, domain.MainBranchID, &repository.NoteReadModel{ID: stray, Text: "orphan", Version: 1}); err != nil {
		t.Fatalf("save note: %v", err)
	}
	if _, err := s.rebuild(t, s.read); err != nil {
		t.Fatalf("rebuild: %v", err)
	}
	if n, _ := s.read.GetNote(ctx, domain.MainBranchID, stray); n != nil {
		t.Errorf("stray note survived the rebuild: %+v", n)
	}
}

// failingRead fails the named operation.
type failingRead struct {
	*memory.ReadModelStore
	fail string
}

func (f *failingRead) err(op string) error {
	if f.fail == op {
		return errRebuildTest
	}
	return nil
}

func (f *failingRead) ResetReadModel(ctx context.Context) error {
	if err := f.err("reset"); err != nil {
		return err
	}
	return f.ReadModelStore.ResetReadModel(ctx)
}

func (f *failingRead) ListPersons(ctx context.Context, opts repository.ListOptions) ([]repository.PersonReadModel, int, error) {
	if err := f.err("list persons"); err != nil {
		return nil, 0, err
	}
	return f.ReadModelStore.ListPersons(ctx, opts)
}

// SavePerson fails only the write that restores a brick wall.
func (f *failingRead) SavePerson(ctx context.Context, b domain.BranchID, p *repository.PersonReadModel) error {
	if p.BrickWallNote != "" {
		if err := f.err("restore"); err != nil {
			return err
		}
	}
	return f.ReadModelStore.SavePerson(ctx, b, p)
}

func (f *failingRead) GetRebuildState(ctx context.Context) ([]byte, error) {
	if err := f.err("get state"); err != nil {
		return nil, err
	}
	return f.ReadModelStore.GetRebuildState(ctx)
}

func (f *failingRead) SaveRebuildState(ctx context.Context, state []byte) error {
	if err := f.err("save state"); err != nil {
		return err
	}
	return f.ReadModelStore.SaveRebuildState(ctx, state)
}

func (f *failingRead) ClearRebuildState(ctx context.Context) error {
	if err := f.err("clear state"); err != nil {
		return err
	}
	return f.ReadModelStore.ClearRebuildState(ctx)
}

func (f *failingRead) RecordProjectionFailure(ctx context.Context, failure repository.ProjectionFailure) error {
	if err := f.err("record"); err != nil {
		return err
	}
	return f.ReadModelStore.RecordProjectionFailure(ctx, failure)
}

func TestRebuildReadModel_ReportsStoreErrors(t *testing.T) {
	for _, op := range []string{
		"reset", "list persons", "restore", "get state", "save state", "clear state",
	} {
		t.Run(op, func(t *testing.T) {
			ctx := context.Background()
			s := newRebuildStores()
			h := command.NewHandlerWithBranches(s.events, s.read, s.branches, s.snapshots)
			p, err := h.CreatePerson(ctx, command.CreatePersonInput{GivenName: "Ada", Surname: "Example"})
			if err != nil {
				t.Fatalf("create person: %v", err)
			}
			if err := s.read.SetBrickWall(ctx, p.ID, "stuck"); err != nil {
				t.Fatalf("set brick wall: %v", err)
			}
			_, err = s.rebuild(t, &failingRead{ReadModelStore: s.read, fail: op})
			if !errors.Is(err, errRebuildTest) {
				t.Fatalf("rebuild error = %v, want the injected %q failure", err, op)
			}
		})
	}
}

// failingEvents fails ReadAll.
type failingEvents struct{ *memory.EventStore }

func (failingEvents) ReadAll(context.Context, int64, int) ([]repository.StoredEvent, error) {
	return nil, errRebuildTest
}

func TestRebuildReadModel_ReportsLogReadError(t *testing.T) {
	s := newRebuildStores()
	_, err := repository.RebuildReadModel(context.Background(), failingEvents{s.events}, s.read, s.branches, s.snapshots)
	if !errors.Is(err, errRebuildTest) {
		t.Fatalf("rebuild error = %v, want the log read failure", err)
	}
}

func TestRebuildReadModel_ReportsUndecodableEvent(t *testing.T) {
	ctx := context.Background()
	s := newRebuildStores()
	bad := undecodableEvent{id: uuid.New()}
	if err := s.events.Append(ctx, bad.id, "person", []domain.Event{bad}, -1, repository.MainScope); err != nil {
		t.Fatalf("append: %v", err)
	}
	note := &repository.NoteReadModel{ID: uuid.New(), Text: "kept", Version: 1}
	if err := s.read.SaveNote(ctx, domain.MainBranchID, note); err != nil {
		t.Fatalf("save note: %v", err)
	}
	if _, err := s.rebuild(t, s.read); err == nil {
		t.Fatal("rebuild of an undecodable event succeeded, want an error")
	}
	// The log is checked before the reset, so nothing was deleted.
	if n, _ := s.read.GetNote(ctx, domain.MainBranchID, note.ID); n == nil {
		t.Error("read model was reset although the log cannot be replayed")
	}
	if state, _ := s.read.GetRebuildState(ctx); state != nil {
		t.Errorf("rebuild state = %s, want none saved", state)
	}
}

// cancelAfterReset cancels the rebuild's context once the read model has
// been reset, as Ctrl-C or a crash mid-replay would stop it.
type cancelAfterReset struct {
	*memory.ReadModelStore
	cancel context.CancelFunc
}

func (c *cancelAfterReset) ResetReadModel(ctx context.Context) error {
	defer c.cancel()
	return c.ReadModelStore.ResetReadModel(ctx)
}

// TestRebuildReadModel_InterruptedRebuildIsRecoverable stops a rebuild right
// after its reset. The read model must stay flagged as out of sync, and a
// rerun must still restore the brick walls, which the reset has already
// removed from the read model.
func TestRebuildReadModel_InterruptedRebuildIsRecoverable(t *testing.T) {
	ctx := context.Background()
	s := newRebuildStores()
	h := command.NewHandlerWithBranches(s.events, s.read, s.branches, s.snapshots)
	person, err := h.CreatePerson(ctx, command.CreatePersonInput{GivenName: "Ada", Surname: "Example"})
	if err != nil {
		t.Fatalf("create person: %v", err)
	}
	if err := s.read.SetBrickWall(ctx, person.ID, "stuck"); err != nil {
		t.Fatalf("set brick wall: %v", err)
	}

	aborted, cancel := context.WithCancel(ctx)
	defer cancel()
	_, err = repository.RebuildReadModel(aborted, s.events, &cancelAfterReset{ReadModelStore: s.read, cancel: cancel}, s.branches, s.snapshots)
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("interrupted rebuild error = %v, want context.Canceled", err)
	}
	if row, _ := s.read.GetPerson(ctx, domain.MainBranchID, person.ID); row != nil {
		t.Fatalf("precondition: person replayed although the rebuild stopped at the first event")
	}
	if state, err := s.read.GetRebuildState(ctx); err != nil || state == nil {
		t.Fatalf("rebuild state after an interrupted rebuild = %v, %v; want it kept", state, err)
	}

	if _, err := s.rebuild(t, s.read); err != nil {
		t.Fatalf("rerun: %v", err)
	}
	if p, _ := s.read.GetPerson(ctx, domain.MainBranchID, person.ID); p == nil || p.BrickWallNote != "stuck" || p.BrickWallSince == nil {
		t.Errorf("person after rerun = %+v, want the brick wall restored", p)
	}
	if state, _ := s.read.GetRebuildState(ctx); state != nil {
		t.Errorf("rebuild state after a successful rerun = %s, want cleared", state)
	}
}

func TestRebuildReadModel_ReportsCorruptRebuildState(t *testing.T) {
	s := newRebuildStores()
	if err := s.read.SaveRebuildState(context.Background(), []byte("not json")); err != nil {
		t.Fatalf("save state: %v", err)
	}
	if _, err := s.rebuild(t, s.read); err == nil || !strings.Contains(err.Error(), "decode rebuild state") {
		t.Fatalf("rebuild error = %v, want a decode failure", err)
	}
}

// undecodableEvent is an event type DecodeEvent does not know.
type undecodableEvent struct{ id uuid.UUID }

func (undecodableEvent) EventType() string        { return "NoSuchEvent" }
func (e undecodableEvent) AggregateID() uuid.UUID { return e.id }
func (undecodableEvent) OccurredAt() time.Time    { return time.Time{} }

// TestReportProjectionFailure_RecordFailureIsOnlyLogged proves a marker that
// cannot be written does not panic or propagate: the event is durable anyway.
func TestReportProjectionFailure_RecordFailureIsOnlyLogged(t *testing.T) {
	s := newRebuildStores()
	rs := &failingRead{ReadModelStore: s.read, fail: "record"}
	repository.ReportProjectionFailure(context.Background(), rs, domain.NewPersonCreated(domain.NewPerson("Ada", "Example")), 1, domain.MainBranchID, errRebuildTest)
	if got, _ := s.read.ListProjectionFailures(context.Background(), 10); len(got) != 0 {
		t.Errorf("markers = %+v, want none", got)
	}
}
