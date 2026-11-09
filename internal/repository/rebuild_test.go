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
	if result.DroppedImported != 0 {
		t.Errorf("DroppedImported = %d, want 0", result.DroppedImported)
	}
}

// TestRebuildReadModel_BackfillsUnloggedImportFields covers a database an
// import made before #1225 left behind: coordinates and EXIDs written only to
// the read model. The rebuild logs them before its reset, so it keeps them,
// and a second rebuild has nothing left to log (#1251).
func TestRebuildReadModel_BackfillsUnloggedImportFields(t *testing.T) {
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
	source, err := h.CreateSource(ctx, command.CreateSourceInput{SourceType: "census", Title: "Census"})
	if err != nil {
		t.Fatalf("create source: %v", err)
	}
	repo, err := h.CreateRepository(ctx, command.CreateRepositoryInput{Name: "Archive"})
	if err != nil {
		t.Fatalf("create repository: %v", err)
	}
	residence := domain.NewLifeEvent(person.ID, domain.FactPersonResidence)
	if err := s.events.Append(ctx, residence.ID, "event", []domain.Event{domain.NewLifeEventCreatedFromModel(residence)}, -1, repository.MainScope); err != nil {
		t.Fatalf("append life event: %v", err)
	}
	if err := repository.NewProjector(s.read, nil).Project(ctx, domain.NewLifeEventCreatedFromModel(residence), 1, domain.MainBranchID); err != nil {
		t.Fatalf("project life event: %v", err)
	}

	// Write the values straight to the read model, as import did before #1225.
	lat, long := "N1.5", "W2.5"
	p, _ := s.read.GetPerson(ctx, domain.MainBranchID, person.ID)
	p.BirthPlaceLat, p.DeathPlaceLong = &lat, &long
	personVersion := p.Version
	mustRebuildSetup(t, s.read.SavePerson(ctx, domain.MainBranchID, p))
	mustRebuildSetup(t, s.read.ReplacePersonExternalIDs(ctx, domain.MainBranchID, person.ID, []repository.PersonExternalIDReadModel{{PersonID: person.ID, Value: "P-1", Type: "urn:p"}}))
	le, _ := s.read.GetEvent(ctx, domain.MainBranchID, residence.ID)
	le.PlaceLat, le.PlaceLong = &lat, &long
	mustRebuildSetup(t, s.read.SaveEvent(ctx, domain.MainBranchID, le))
	mustRebuildSetup(t, s.read.ReplaceFamilyExternalIDs(ctx, domain.MainBranchID, family.ID, []repository.FamilyExternalIDReadModel{{FamilyID: family.ID, Value: "F-1"}}))
	mustRebuildSetup(t, s.read.ReplaceSourceExternalIDs(ctx, domain.MainBranchID, source.ID, []repository.SourceExternalIDReadModel{{SourceID: source.ID, Value: "S-1"}}))
	mustRebuildSetup(t, s.read.ReplaceRepositoryExternalIDs(ctx, repo.ID, []repository.RepositoryExternalIDReadModel{{RepositoryID: repo.ID, Value: "R-1"}}))

	result, err := s.rebuild(t, s.read)
	if err != nil {
		t.Fatalf("rebuild: %v", err)
	}
	if result.Backfilled != 5 || result.DroppedImported != 0 {
		t.Errorf("Backfilled = %d, DroppedImported = %d; want 5 and 0", result.Backfilled, result.DroppedImported)
	}
	if p, _ := s.read.GetPerson(ctx, domain.MainBranchID, person.ID); p == nil || p.BirthPlaceLat == nil || *p.BirthPlaceLat != lat ||
		p.DeathPlaceLong == nil || *p.DeathPlaceLong != long || p.BirthPlaceLong != nil || p.Version != personVersion+1 {
		t.Errorf("person after rebuild = %+v, want its coordinates kept, one version on from %d", p, personVersion)
	}
	if ids, _ := s.read.GetPersonExternalIDs(ctx, domain.MainBranchID, person.ID); len(ids) != 1 || ids[0].Value != "P-1" || ids[0].Type != "urn:p" {
		t.Errorf("person exids after rebuild = %+v, want P-1", ids)
	}
	if le, _ := s.read.GetEvent(ctx, domain.MainBranchID, residence.ID); le == nil || le.PlaceLat == nil || le.PlaceLong == nil || le.Version != 2 {
		t.Errorf("life event after rebuild = %+v, want its coordinates kept at version 2", le)
	}
	if ids, _ := s.read.GetFamilyExternalIDs(ctx, domain.MainBranchID, family.ID); len(ids) != 1 || ids[0].Value != "F-1" {
		t.Errorf("family exids after rebuild = %+v, want F-1", ids)
	}
	if ids, _ := s.read.GetSourceExternalIDs(ctx, domain.MainBranchID, source.ID); len(ids) != 1 || ids[0].Value != "S-1" {
		t.Errorf("source exids after rebuild = %+v, want S-1", ids)
	}
	if ids, _ := s.read.GetRepositoryExternalIDs(ctx, repo.ID); len(ids) != 1 || ids[0].Value != "R-1" {
		t.Errorf("repository exids after rebuild = %+v, want R-1", ids)
	}

	again, err := s.rebuild(t, s.read)
	if err != nil || again.Backfilled != 0 || again.DroppedImported != 0 {
		t.Errorf("second rebuild = %+v, %v; want nothing left to backfill or drop", again, err)
	}
}

// TestRebuildReadModel_BackfillKeepsVersionsIfTheRebuildStops proves a
// rebuild that stops after its backfill but before its reset leaves each
// backfilled row at its stream's version, so the entity can still be edited.
func TestRebuildReadModel_BackfillKeepsVersionsIfTheRebuildStops(t *testing.T) {
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
	mustRebuildSetup(t, s.read.SavePerson(ctx, domain.MainBranchID, p))

	result, err := s.rebuild(t, &failingRead{ReadModelStore: s.read, fail: "save state"})
	if !errors.Is(err, errRebuildTest) || result.Backfilled != 1 {
		t.Fatalf("rebuild = %+v, %v; want the backfill done and the injected failure", result, err)
	}
	p, _ = s.read.GetPerson(ctx, domain.MainBranchID, person.ID)
	surname := "Edited"
	if _, err := h.UpdatePerson(ctx, command.UpdatePersonInput{ID: person.ID, Surname: &surname, Version: p.Version}); err != nil {
		t.Errorf("update at read-model version %d after the stopped rebuild: %v", p.Version, err)
	}
}

// TestRebuildReadModel_ReportsRowsWithoutAStream proves a row whose values no
// stream can carry is not backfilled; the replay drops it and the recount
// reports what it held (#1251).
func TestRebuildReadModel_ReportsRowsWithoutAStream(t *testing.T) {
	ctx := context.Background()
	s := newRebuildStores()
	lat := "N1.5"
	mustRebuildSetup(t, s.read.SavePerson(ctx, domain.MainBranchID, &repository.PersonReadModel{ID: uuid.New(), GivenName: "Stray", BirthPlaceLat: &lat, Version: 1}))
	result, err := s.rebuild(t, s.read)
	if err != nil {
		t.Fatalf("rebuild: %v", err)
	}
	if result.Backfilled != 0 || result.DroppedImported != 1 {
		t.Errorf("Backfilled = %d, DroppedImported = %d; want 0 and 1", result.Backfilled, result.DroppedImported)
	}
}

func TestProjectImportedValuesBackfilled_RejectsUnknownEntityType(t *testing.T) {
	event := domain.ImportedValuesBackfilled{BaseEvent: domain.NewBaseEvent(), EntityType: "note", EntityID: uuid.New()}
	if err := repository.NewProjector(memory.NewReadModelStore(), nil).Project(context.Background(), event, 2, domain.MainBranchID); err == nil {
		t.Error("projecting a backfill for an unknown entity type succeeded")
	}
}

func mustRebuildSetup(t *testing.T, err error) {
	t.Helper()
	if err != nil {
		t.Fatalf("setup: %v", err)
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
	fail     string
	didReset bool
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
	f.didReset = true
	return f.ReadModelStore.ResetReadModel(ctx)
}

// ListRepositories fails as "list repositories" always, or as "recount" once
// the read model has been reset.
func (f *failingRead) ListRepositories(ctx context.Context, opts repository.ListOptions) ([]repository.RepositoryReadModel, int, error) {
	if err := f.err("list repositories"); err != nil {
		return nil, 0, err
	}
	if f.didReset {
		if err := f.err("recount"); err != nil {
			return nil, 0, err
		}
	}
	return f.ReadModelStore.ListRepositories(ctx, opts)
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
		"reset", "list persons", "list repositories", "restore", "get state", "save state", "clear state",
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

// TestRebuildReadModel_RecountFailureIsOnlyLogged proves the check for
// dropped coordinates and EXIDs after the replay does not fail a rebuild that
// otherwise succeeded (#1251).
func TestRebuildReadModel_RecountFailureIsOnlyLogged(t *testing.T) {
	ctx := context.Background()
	s := newRebuildStores()
	h := command.NewHandlerWithBranches(s.events, s.read, s.branches, s.snapshots)
	if _, err := h.CreatePerson(ctx, command.CreatePersonInput{GivenName: "Ada", Surname: "Example"}); err != nil {
		t.Fatalf("create person: %v", err)
	}
	result, err := s.rebuild(t, &failingRead{ReadModelStore: s.read, fail: "recount"})
	if err != nil {
		t.Fatalf("rebuild error = %v, want the failed recount only logged", err)
	}
	if result.DroppedImported != 0 {
		t.Errorf("DroppedImported = %d, want 0 when the recount failed", result.DroppedImported)
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
