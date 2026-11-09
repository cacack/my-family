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

// TestRebuildReadModel_KeepsFieldsNoEventCarries covers each value written
// straight to the read model (GEDCOM coordinates and EXIDs, brick walls): a
// replay cannot derive them, so the rebuild must carry them over (#845).
func TestRebuildReadModel_KeepsFieldsNoEventCarries(t *testing.T) {
	ctx := context.Background()
	s := newRebuildStores()
	h := command.NewHandlerWithBranches(s.events, s.read, s.branches, s.snapshots)

	person, err := h.CreatePerson(ctx, command.CreatePersonInput{GivenName: "Ada", Surname: "Example"})
	if err != nil {
		t.Fatalf("create person: %v", err)
	}
	family, err := h.CreateFamily(ctx, command.CreateFamilyInput{Partner1ID: &person.ID})
	if err != nil {
		t.Fatalf("create family: %v", err)
	}
	source, err := h.CreateSource(ctx, command.CreateSourceInput{SourceType: "book", Title: "Register"})
	if err != nil {
		t.Fatalf("create source: %v", err)
	}
	repo, err := h.CreateRepository(ctx, command.CreateRepositoryInput{Name: "Archive"})
	if err != nil {
		t.Fatalf("create repository: %v", err)
	}
	life := domain.NewLifeEvent(person.ID, domain.FactPersonBirth)
	if err := s.events.Append(ctx, life.ID, "life_event", []domain.Event{domain.NewLifeEventCreatedFromModel(life)}, -1, repository.MainScope); err != nil {
		t.Fatalf("append life event: %v", err)
	}

	// Rebuild once so the life event is projected, then add the unlogged values.
	if _, err := s.rebuild(t, s.read); err != nil {
		t.Fatalf("first rebuild: %v", err)
	}
	lat, long := "N1.5", "W2.5"
	p, _ := s.read.GetPerson(ctx, domain.MainBranchID, person.ID)
	p.BirthPlaceLat, p.DeathPlaceLong = &lat, &long
	if err := s.read.SavePerson(ctx, domain.MainBranchID, p); err != nil {
		t.Fatalf("save person coordinates: %v", err)
	}
	if err := s.read.SetBrickWall(ctx, person.ID, "stuck"); err != nil {
		t.Fatalf("set brick wall: %v", err)
	}
	le, _ := s.read.GetEvent(ctx, domain.MainBranchID, life.ID)
	if le == nil {
		t.Fatal("life event not projected")
	}
	le.PlaceLat, le.PlaceLong = &lat, &long
	if err := s.read.SaveEvent(ctx, domain.MainBranchID, le); err != nil {
		t.Fatalf("save life event coordinates: %v", err)
	}
	must := func(err error) {
		t.Helper()
		if err != nil {
			t.Fatal(err)
		}
	}
	must(s.read.ReplacePersonExternalIDs(ctx, domain.MainBranchID, person.ID, []repository.PersonExternalIDReadModel{{PersonID: person.ID, Value: "P-1"}}))
	must(s.read.ReplaceFamilyExternalIDs(ctx, domain.MainBranchID, family.ID, []repository.FamilyExternalIDReadModel{{FamilyID: family.ID, Value: "F-1"}}))
	must(s.read.ReplaceSourceExternalIDs(ctx, domain.MainBranchID, source.ID, []repository.SourceExternalIDReadModel{{SourceID: source.ID, Value: "S-1"}}))
	must(s.read.ReplaceRepositoryExternalIDs(ctx, repo.ID, []repository.RepositoryExternalIDReadModel{{RepositoryID: repo.ID, Value: "R-1"}}))

	result, err := s.rebuild(t, s.read)
	if err != nil {
		t.Fatalf("rebuild: %v", err)
	}
	if logged, _ := s.events.ReadAll(ctx, 0, 100); result.Events != len(logged) {
		t.Errorf("events replayed = %d, want all %d logged", result.Events, len(logged))
	}

	p, _ = s.read.GetPerson(ctx, domain.MainBranchID, person.ID)
	if p == nil || p.BirthPlaceLat == nil || *p.BirthPlaceLat != lat || p.DeathPlaceLong == nil || *p.DeathPlaceLong != long ||
		p.BrickWallNote != "stuck" || p.BrickWallSince == nil {
		t.Errorf("person after rebuild = %+v, want coordinates and brick wall kept", p)
	}
	le, _ = s.read.GetEvent(ctx, domain.MainBranchID, life.ID)
	if le == nil || le.PlaceLat == nil || *le.PlaceLat != lat || le.PlaceLong == nil || *le.PlaceLong != long {
		t.Errorf("life event after rebuild = %+v, want coordinates kept", le)
	}
	if ids, _ := s.read.GetPersonExternalIDs(ctx, domain.MainBranchID, person.ID); len(ids) != 1 || ids[0].Value != "P-1" {
		t.Errorf("person exids = %+v", ids)
	}
	if ids, _ := s.read.GetFamilyExternalIDs(ctx, domain.MainBranchID, family.ID); len(ids) != 1 || ids[0].Value != "F-1" {
		t.Errorf("family exids = %+v", ids)
	}
	if ids, _ := s.read.GetSourceExternalIDs(ctx, domain.MainBranchID, source.ID); len(ids) != 1 || ids[0].Value != "S-1" {
		t.Errorf("source exids = %+v", ids)
	}
	if ids, _ := s.read.GetRepositoryExternalIDs(ctx, repo.ID); len(ids) != 1 || ids[0].Value != "R-1" {
		t.Errorf("repository exids = %+v", ids)
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

func (f *failingRead) ListEvents(ctx context.Context, opts repository.ListOptions) ([]repository.EventReadModel, int, error) {
	if err := f.err("list events"); err != nil {
		return nil, 0, err
	}
	return f.ReadModelStore.ListEvents(ctx, opts)
}

func (f *failingRead) ListFamilies(ctx context.Context, opts repository.ListOptions) ([]repository.FamilyReadModel, int, error) {
	if err := f.err("list families"); err != nil {
		return nil, 0, err
	}
	return f.ReadModelStore.ListFamilies(ctx, opts)
}

func (f *failingRead) ListSources(ctx context.Context, opts repository.ListOptions) ([]repository.SourceReadModel, int, error) {
	if err := f.err("list sources"); err != nil {
		return nil, 0, err
	}
	return f.ReadModelStore.ListSources(ctx, opts)
}

func (f *failingRead) ListRepositories(ctx context.Context, opts repository.ListOptions) ([]repository.RepositoryReadModel, int, error) {
	if err := f.err("list repositories"); err != nil {
		return nil, 0, err
	}
	return f.ReadModelStore.ListRepositories(ctx, opts)
}

func (f *failingRead) GetPersonExternalIDs(ctx context.Context, b domain.BranchID, id uuid.UUID) ([]repository.PersonExternalIDReadModel, error) {
	if err := f.err("person exids"); err != nil {
		return nil, err
	}
	return f.ReadModelStore.GetPersonExternalIDs(ctx, b, id)
}

func (f *failingRead) GetFamilyExternalIDs(ctx context.Context, b domain.BranchID, id uuid.UUID) ([]repository.FamilyExternalIDReadModel, error) {
	if err := f.err("family exids"); err != nil {
		return nil, err
	}
	return f.ReadModelStore.GetFamilyExternalIDs(ctx, b, id)
}

func (f *failingRead) GetSourceExternalIDs(ctx context.Context, b domain.BranchID, id uuid.UUID) ([]repository.SourceExternalIDReadModel, error) {
	if err := f.err("source exids"); err != nil {
		return nil, err
	}
	return f.ReadModelStore.GetSourceExternalIDs(ctx, b, id)
}

func (f *failingRead) GetRepositoryExternalIDs(ctx context.Context, id uuid.UUID) ([]repository.RepositoryExternalIDReadModel, error) {
	if err := f.err("repository exids"); err != nil {
		return nil, err
	}
	return f.ReadModelStore.GetRepositoryExternalIDs(ctx, id)
}

func (f *failingRead) ReplacePersonExternalIDs(ctx context.Context, b domain.BranchID, id uuid.UUID, ids []repository.PersonExternalIDReadModel) error {
	if err := f.err("restore"); err != nil {
		return err
	}
	return f.ReadModelStore.ReplacePersonExternalIDs(ctx, b, id, ids)
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
		"reset", "list persons", "list events", "list families", "list sources", "list repositories",
		"person exids", "family exids", "source exids", "repository exids", "restore",
		"get state", "save state", "clear state",
	} {
		t.Run(op, func(t *testing.T) {
			ctx := context.Background()
			s := newRebuildStores()
			h := command.NewHandlerWithBranches(s.events, s.read, s.branches, s.snapshots)
			p, err := h.CreatePerson(ctx, command.CreatePersonInput{GivenName: "Ada", Surname: "Example"})
			if err != nil {
				t.Fatalf("create person: %v", err)
			}
			if _, err := h.CreateFamily(ctx, command.CreateFamilyInput{Partner1ID: &p.ID}); err != nil {
				t.Fatalf("create family: %v", err)
			}
			if _, err := h.CreateSource(ctx, command.CreateSourceInput{SourceType: "book", Title: "Register"}); err != nil {
				t.Fatalf("create source: %v", err)
			}
			if _, err := h.CreateRepository(ctx, command.CreateRepositoryInput{Name: "Archive"}); err != nil {
				t.Fatalf("create repository: %v", err)
			}
			if err := s.read.ReplacePersonExternalIDs(ctx, domain.MainBranchID, p.ID, []repository.PersonExternalIDReadModel{{PersonID: p.ID, Value: "P-1"}}); err != nil {
				t.Fatalf("save exid: %v", err)
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
// rerun must still restore the values no event carries, which the reset has
// already removed from the read model.
func TestRebuildReadModel_InterruptedRebuildIsRecoverable(t *testing.T) {
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
	if err := s.read.SetBrickWall(ctx, person.ID, "stuck"); err != nil {
		t.Fatalf("set brick wall: %v", err)
	}
	if err := s.read.ReplacePersonExternalIDs(ctx, domain.MainBranchID, person.ID, []repository.PersonExternalIDReadModel{{PersonID: person.ID, Value: "P-1"}}); err != nil {
		t.Fatalf("save exid: %v", err)
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
	p, _ = s.read.GetPerson(ctx, domain.MainBranchID, person.ID)
	if p == nil || p.BirthPlaceLat == nil || *p.BirthPlaceLat != lat || p.BrickWallNote != "stuck" || p.BrickWallSince == nil {
		t.Errorf("person after rerun = %+v, want coordinates and brick wall restored", p)
	}
	if ids, _ := s.read.GetPersonExternalIDs(ctx, domain.MainBranchID, person.ID); len(ids) != 1 || ids[0].Value != "P-1" {
		t.Errorf("person exids after rerun = %+v", ids)
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
