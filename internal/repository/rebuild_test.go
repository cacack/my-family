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

// setLegacyBrickWall writes a brick wall straight to a person's main row, as
// every brick wall was written before #802, with no event behind it.
func setLegacyBrickWall(t *testing.T, rs repository.ReadModelStore, id uuid.UUID, note string, since time.Time, resolvedAt *time.Time) {
	t.Helper()
	ctx := context.Background()
	p, err := rs.GetPerson(ctx, domain.MainBranchID, id)
	if err != nil || p == nil {
		t.Fatalf("get person %s: %v", id, err)
	}
	p.BrickWallNote, p.BrickWallSince, p.BrickWallResolvedAt = note, &since, resolvedAt
	if err := rs.SavePerson(ctx, domain.MainBranchID, p); err != nil {
		t.Fatalf("save person: %v", err)
	}
}

// loggedEventTypes counts the logged events by type.
func loggedEventTypes(t *testing.T, events repository.EventStore) map[string]int {
	t.Helper()
	logged, err := events.ReadAll(context.Background(), 0, 1000)
	if err != nil {
		t.Fatalf("read log: %v", err)
	}
	types := map[string]int{}
	for i := range logged {
		types[logged[i].EventType]++
	}
	return types
}

// TestRebuildReadModel_KeepsBrickWalls covers brick walls set through the
// command: they are event-sourced (#802), so the replay restores them and
// nothing needs backfilling.
func TestRebuildReadModel_KeepsBrickWalls(t *testing.T) {
	ctx := context.Background()
	s := newRebuildStores()
	h := command.NewHandlerWithBranches(s.events, s.read, s.branches, s.snapshots)

	stuck, err := h.CreatePerson(ctx, command.CreatePersonInput{GivenName: "Ada", Surname: "Example"})
	if err != nil {
		t.Fatalf("create person: %v", err)
	}
	broken, err := h.CreatePerson(ctx, command.CreatePersonInput{GivenName: "Bea", Surname: "Example"})
	if err != nil {
		t.Fatalf("create person: %v", err)
	}
	for _, id := range []uuid.UUID{stuck.ID, broken.ID} {
		if err := h.SetBrickWall(ctx, id, "stuck"); err != nil {
			t.Fatalf("set brick wall: %v", err)
		}
	}
	if err := h.ResolveBrickWall(ctx, broken.ID); err != nil {
		t.Fatalf("resolve brick wall: %v", err)
	}
	before, err := s.read.GetBrickWalls(ctx, true)
	if err != nil || len(before) != 2 {
		t.Fatalf("brick walls before rebuild = %+v, %v; want 2", before, err)
	}

	result, err := s.rebuild(t, s.read)
	if err != nil {
		t.Fatalf("rebuild: %v", err)
	}
	if logged, _ := s.events.ReadAll(ctx, 0, 100); result.Events != len(logged) {
		t.Errorf("events replayed = %d, want all %d logged", result.Events, len(logged))
	}
	if result.BackfilledBrickWalls != 0 {
		t.Errorf("BackfilledBrickWalls = %d, want 0 for logged brick walls", result.BackfilledBrickWalls)
	}
	after, err := s.read.GetBrickWalls(ctx, true)
	if err != nil {
		t.Fatalf("brick walls after rebuild: %v", err)
	}
	if len(after) != len(before) {
		t.Fatalf("brick walls after rebuild = %+v, want %+v", after, before)
	}
	for i := range before {
		b, a := before[i], after[i]
		if a.PersonID != b.PersonID || a.Note != b.Note || !a.Since.Equal(b.Since) ||
			(a.ResolvedAt == nil) != (b.ResolvedAt == nil) || (a.ResolvedAt != nil && !a.ResolvedAt.Equal(*b.ResolvedAt)) {
			t.Errorf("brick wall %d after rebuild = %+v, want %+v", i, a, b)
		}
	}
	if result.DroppedImported != 0 {
		t.Errorf("DroppedImported = %d, want 0", result.DroppedImported)
	}
}

// TestRebuildReadModel_BackfillsLegacyBrickWalls covers a database from before
// #802, whose brick walls only the read model held. The rebuild logs them
// before its reset, so it keeps them with their original times, and a second
// rebuild has nothing left to log.
func TestRebuildReadModel_BackfillsLegacyBrickWalls(t *testing.T) {
	ctx := context.Background()
	s := newRebuildStores()
	h := command.NewHandlerWithBranches(s.events, s.read, s.branches, s.snapshots)
	active, err := h.CreatePerson(ctx, command.CreatePersonInput{GivenName: "Ada", Surname: "Example"})
	if err != nil {
		t.Fatalf("create person: %v", err)
	}
	resolved, err := h.CreatePerson(ctx, command.CreatePersonInput{GivenName: "Bea", Surname: "Example"})
	if err != nil {
		t.Fatalf("create person: %v", err)
	}
	since := time.Date(2020, 3, 1, 12, 0, 0, 0, time.UTC)
	resolvedAt := since.Add(48 * time.Hour)
	setLegacyBrickWall(t, s.read, active.ID, "no baptism found", since, nil)
	setLegacyBrickWall(t, s.read, resolved.ID, "census gap", since, &resolvedAt)

	result, err := s.rebuild(t, s.read)
	if err != nil {
		t.Fatalf("rebuild: %v", err)
	}
	if result.BackfilledBrickWalls != 2 {
		t.Errorf("BackfilledBrickWalls = %d, want 2", result.BackfilledBrickWalls)
	}
	if types := loggedEventTypes(t, s.events); types["BrickWallBackfilled"] != 2 || types["BrickWallSet"] != 0 || types["BrickWallResolved"] != 0 {
		t.Errorf("logged event types = %v, want two BrickWallBackfilled and no live brick-wall events", types)
	}
	a, _ := s.read.GetPerson(ctx, domain.MainBranchID, active.ID)
	if a == nil || a.BrickWallNote != "no baptism found" || a.BrickWallSince == nil || !a.BrickWallSince.Equal(since) || a.BrickWallResolvedAt != nil {
		t.Errorf("active brick wall after rebuild = %+v, want it kept with its original time", a)
	}
	r, _ := s.read.GetPerson(ctx, domain.MainBranchID, resolved.ID)
	if r == nil || r.BrickWallNote != "census gap" || r.BrickWallResolvedAt == nil || !r.BrickWallResolvedAt.Equal(resolvedAt) {
		t.Errorf("resolved brick wall after rebuild = %+v, want it kept resolved", r)
	}
	// The person's version follows its stream, so a later edit is not refused.
	if err := h.SetBrickWall(ctx, active.ID, "still stuck"); err != nil {
		t.Fatalf("set brick wall after rebuild: %v", err)
	}

	again, err := s.rebuild(t, s.read)
	if err != nil {
		t.Fatalf("second rebuild: %v", err)
	}
	if again.BackfilledBrickWalls != 0 {
		t.Errorf("second rebuild BackfilledBrickWalls = %d, want 0", again.BackfilledBrickWalls)
	}
	if a, _ := s.read.GetPerson(ctx, domain.MainBranchID, active.ID); a == nil || a.BrickWallNote != "still stuck" {
		t.Errorf("brick wall after second rebuild = %+v, want the later note", a)
	}
}

// TestRebuildReadModel_BackfillsBrickWallsOfAnInterruptedLegacyRebuild covers
// a rebuild from before #802 that stopped after its reset: its brick walls are
// only in the rebuild state it saved. The rerun logs them from there.
func TestRebuildReadModel_BackfillsBrickWallsOfAnInterruptedLegacyRebuild(t *testing.T) {
	ctx := context.Background()
	s := newRebuildStores()
	h := command.NewHandlerWithBranches(s.events, s.read, s.branches, s.snapshots)
	person, err := h.CreatePerson(ctx, command.CreatePersonInput{GivenName: "Ada", Surname: "Example"})
	if err != nil {
		t.Fatalf("create person: %v", err)
	}
	if err := s.read.ResetReadModel(ctx); err != nil {
		t.Fatalf("reset: %v", err)
	}
	state := `{"brick_walls":{"` + person.ID.String() + `":{"note":"stuck","since":"2020-03-01T12:00:00Z"}}}`
	if err := s.read.SaveRebuildState(ctx, []byte(state)); err != nil {
		t.Fatalf("save state: %v", err)
	}

	result, err := s.rebuild(t, s.read)
	if err != nil {
		t.Fatalf("rebuild: %v", err)
	}
	if result.BackfilledBrickWalls != 1 {
		t.Errorf("BackfilledBrickWalls = %d, want 1", result.BackfilledBrickWalls)
	}
	if p, _ := s.read.GetPerson(ctx, domain.MainBranchID, person.ID); p == nil || p.BrickWallNote != "stuck" || p.BrickWallSince == nil {
		t.Errorf("person after rerun = %+v, want the saved brick wall", p)
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
	if _, err := h.UpdatePerson(ctx, command.UpdatePersonInput{ID: person.ID, Surname: command.Set(surname), Version: p.Version}); err != nil {
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

func (f *failingRead) GetSchemaVersion(ctx context.Context) (int, error) {
	if err := f.err("get version"); err != nil {
		return 0, err
	}
	return f.ReadModelStore.GetSchemaVersion(ctx)
}

func (f *failingRead) SetSchemaVersion(ctx context.Context, version int) error {
	if err := f.err("stamp"); err != nil {
		return err
	}
	return f.ReadModelStore.SetSchemaVersion(ctx, version)
}

func (f *failingRead) RecordProjectionFailure(ctx context.Context, failure repository.ProjectionFailure) error {
	if err := f.err("record"); err != nil {
		return err
	}
	return f.ReadModelStore.RecordProjectionFailure(ctx, failure)
}

func TestRebuildReadModel_ReportsStoreErrors(t *testing.T) {
	for _, op := range []string{
		"reset", "list persons", "list repositories", "get state", "save state", "clear state", "stamp",
	} {
		t.Run(op, func(t *testing.T) {
			ctx := context.Background()
			s := newRebuildStores()
			h := command.NewHandlerWithBranches(s.events, s.read, s.branches, s.snapshots)
			p, err := h.CreatePerson(ctx, command.CreatePersonInput{GivenName: "Ada", Surname: "Example"})
			if err != nil {
				t.Fatalf("create person: %v", err)
			}
			if err := h.SetBrickWall(ctx, p.ID, "stuck"); err != nil {
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
// rerun must still restore a legacy brick wall, which the reset has already
// removed from the read model: the interrupted rebuild logged it first.
func TestRebuildReadModel_InterruptedRebuildIsRecoverable(t *testing.T) {
	ctx := context.Background()
	s := newRebuildStores()
	h := command.NewHandlerWithBranches(s.events, s.read, s.branches, s.snapshots)
	person, err := h.CreatePerson(ctx, command.CreatePersonInput{GivenName: "Ada", Surname: "Example"})
	if err != nil {
		t.Fatalf("create person: %v", err)
	}
	setLegacyBrickWall(t, s.read, person.ID, "stuck", time.Now().UTC(), nil)

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

// TestProjectImportedValuesBackfilled_FillsBranchCopies covers the branch rows
// a backfill must also fill in (#1254): a branch's own copy of the person or
// life event that lacks the coordinates gets them, a branch without its own
// row still reads main's, and a branch copy that has them is left alone.
func TestProjectImportedValuesBackfilled_FillsBranchCopies(t *testing.T) {
	ctx := context.Background()
	rs, branches := memory.NewReadModelStore(), memory.NewBranchStore()
	copied, _ := domain.NewBranch("copied", "", 0)
	plain, _ := domain.NewBranch("plain", "", 0)
	for _, b := range []*domain.Branch{copied, plain} {
		mustRebuildSetup(t, branches.Create(ctx, b))
	}
	personID, eventID := uuid.New(), uuid.New()
	for _, b := range []domain.BranchID{domain.MainBranchID, domain.BranchID(copied.ID)} {
		mustRebuildSetup(t, rs.SavePerson(ctx, b, &repository.PersonReadModel{ID: personID, GivenName: "Ada", Version: 1}))
		mustRebuildSetup(t, rs.SaveEvent(ctx, b, &repository.EventReadModel{ID: eventID, OwnerType: "person", OwnerID: personID, FactType: domain.FactPersonResidence, Version: 1}))
	}
	lat, long := "N1.5", "W2.5"
	projector := repository.NewProjector(rs, branches)
	mustRebuildSetup(t, projector.Project(ctx, domain.ImportedValuesBackfilled{BaseEvent: domain.NewBaseEvent(),
		EntityType: domain.BackfillPerson, EntityID: personID, BirthPlaceLat: &lat}, 2, domain.MainBranchID))
	mustRebuildSetup(t, projector.Project(ctx, domain.ImportedValuesBackfilled{BaseEvent: domain.NewBaseEvent(),
		EntityType: domain.BackfillLifeEvent, EntityID: eventID, PlaceLat: &lat, PlaceLong: &long}, 2, domain.MainBranchID))

	for _, b := range []*domain.Branch{copied, plain} {
		p, _ := rs.GetPerson(ctx, domain.BranchID(b.ID), personID)
		if p == nil || p.BirthPlaceLat == nil || *p.BirthPlaceLat != lat {
			t.Errorf("person on branch %s = %+v, want the backfilled coordinate", b.Name, p)
		}
		e, _ := rs.GetEvent(ctx, domain.BranchID(b.ID), eventID)
		if e == nil || e.PlaceLat == nil || e.PlaceLong == nil {
			t.Errorf("life event on branch %s = %+v, want the backfilled coordinates", b.Name, e)
		}
	}
	if p, _ := rs.GetPerson(ctx, domain.BranchID(copied.ID), personID); p == nil || p.Version != 1 {
		t.Errorf("branch copy = %+v, want its own version kept", p)
	}

	// A branch copy with coordinates of its own is not overwritten.
	other := "S9.9"
	p, _ := rs.GetPerson(ctx, domain.BranchID(copied.ID), personID)
	p.BirthPlaceLat = &other
	mustRebuildSetup(t, rs.SavePerson(ctx, domain.BranchID(copied.ID), p))
	mustRebuildSetup(t, projector.Project(ctx, domain.ImportedValuesBackfilled{BaseEvent: domain.NewBaseEvent(),
		EntityType: domain.BackfillPerson, EntityID: personID, BirthPlaceLat: &lat}, 3, domain.MainBranchID))
	if p, _ := rs.GetPerson(ctx, domain.BranchID(copied.ID), personID); p == nil || p.BirthPlaceLat == nil || *p.BirthPlaceLat != other {
		t.Errorf("branch copy with its own coordinate = %+v, want it left alone", p)
	}
}

// failingBranches fails List.
type failingBranches struct{ *memory.BranchStore }

func (failingBranches) List(context.Context) ([]*domain.Branch, error) { return nil, errRebuildTest }

func TestProjectImportedValuesBackfilled_ReportsBranchListError(t *testing.T) {
	ctx := context.Background()
	rs := memory.NewReadModelStore()
	personID := uuid.New()
	mustRebuildSetup(t, rs.SavePerson(ctx, domain.MainBranchID, &repository.PersonReadModel{ID: personID, GivenName: "Ada", Version: 1}))
	lat := "N1.5"
	err := repository.NewProjector(rs, failingBranches{memory.NewBranchStore()}).Project(ctx, domain.ImportedValuesBackfilled{
		BaseEvent: domain.NewBaseEvent(), EntityType: domain.BackfillPerson, EntityID: personID, BirthPlaceLat: &lat}, 2, domain.MainBranchID)
	if !errors.Is(err, errRebuildTest) {
		t.Errorf("project = %v, want the branch list failure", err)
	}
}

// TestProjectImportedValuesBackfilled_ExidOnlyLeavesBranchesOnMain proves an
// EXID-only backfill gives no branch a row of its own: a branch keeps reading
// main's row, so a later edit on main still shows there (#1254).
func TestProjectImportedValuesBackfilled_ExidOnlyLeavesBranchesOnMain(t *testing.T) {
	ctx := context.Background()
	rs, branches := memory.NewReadModelStore(), memory.NewBranchStore()
	branch, _ := domain.NewBranch("research", "", 0)
	mustRebuildSetup(t, branches.Create(ctx, branch))
	personID := uuid.New()
	mustRebuildSetup(t, rs.SavePerson(ctx, domain.MainBranchID, &repository.PersonReadModel{ID: personID, GivenName: "Ada", Version: 1}))
	mustRebuildSetup(t, repository.NewProjector(rs, branches).Project(ctx, domain.ImportedValuesBackfilled{BaseEvent: domain.NewBaseEvent(),
		EntityType: domain.BackfillPerson, EntityID: personID, ExternalIDs: []domain.ExternalIdentifier{{Value: "FS-1"}}}, 2, domain.MainBranchID))

	p, _ := rs.GetPerson(ctx, domain.MainBranchID, personID)
	p.GivenName = "Augusta"
	mustRebuildSetup(t, rs.SavePerson(ctx, domain.MainBranchID, p))
	if got, _ := rs.GetPerson(ctx, domain.BranchID(branch.ID), personID); got == nil || got.GivenName != "Augusta" {
		t.Errorf("person on the branch = %+v, want main's later edit seen through the overlay", got)
	}
}

// TestProjectImportedValuesBackfilled_MainDeletedFillsBranchCopiesOnly covers
// a backfill on a stream main has deleted (#1255): it fills the branch copy
// and writes nothing to main.
func TestProjectImportedValuesBackfilled_MainDeletedFillsBranchCopiesOnly(t *testing.T) {
	ctx := context.Background()
	rs, branches := memory.NewReadModelStore(), memory.NewBranchStore()
	branch, _ := domain.NewBranch("research", "", 0)
	mustRebuildSetup(t, branches.Create(ctx, branch))
	personID := uuid.New()
	mustRebuildSetup(t, rs.SavePerson(ctx, domain.BranchID(branch.ID), &repository.PersonReadModel{ID: personID, GivenName: "Ada", Version: 1}))
	lat := "N1.5"
	mustRebuildSetup(t, repository.NewProjector(rs, branches).Project(ctx, domain.ImportedValuesBackfilled{BaseEvent: domain.NewBaseEvent(),
		EntityType: domain.BackfillPerson, EntityID: personID, BirthPlaceLat: &lat,
		ExternalIDs: []domain.ExternalIdentifier{{Value: "FS-1"}}}, 3, domain.MainBranchID))

	if p, _ := rs.GetPerson(ctx, domain.BranchID(branch.ID), personID); p == nil || p.BirthPlaceLat == nil || *p.BirthPlaceLat != lat {
		t.Errorf("branch copy = %+v, want the backfilled coordinate", p)
	}
	if p, _ := rs.GetPerson(ctx, domain.MainBranchID, personID); p != nil {
		t.Errorf("main = %+v, want no row", p)
	}
	if ids, _ := rs.GetPersonExternalIDs(ctx, domain.MainBranchID, personID); len(ids) != 0 {
		t.Errorf("main EXIDs = %+v, want none for a deleted person", ids)
	}
}

func TestRebuildReadModel_BackfillReportsBranchListError(t *testing.T) {
	ctx := context.Background()
	events := memory.NewEventStore()
	_, err := repository.RebuildReadModel(ctx, events, memory.NewReadModelStore(), failingBranches{memory.NewBranchStore()}, memory.NewSnapshotStore(events))
	if !errors.Is(err, errRebuildTest) {
		t.Errorf("rebuild = %v, want the branch list failure", err)
	}
}

// TestRebuildReadModel_BackfillsBranchCopiesMainDeleted covers the collection
// side of #1255: a branch's own copy of a person and a life event that main
// has deleted gets a backfill on main's stream, after the delete. A branch row
// with no main stream behind it gets none.
func TestRebuildReadModel_BackfillsBranchCopiesMainDeleted(t *testing.T) {
	ctx := context.Background()
	s := newRebuildStores()
	branch, _ := domain.NewBranch("research", "", 0)
	mustRebuildSetup(t, s.branches.Create(ctx, branch))
	onBranch := domain.BranchID(branch.ID)

	person := domain.NewPerson("Ada", "Byron")
	lifeEventID, strayID := uuid.New(), uuid.New()
	mustRebuildSetup(t, s.events.Append(ctx, person.ID, "person", []domain.Event{
		domain.NewPersonCreated(person), domain.NewPersonDeleted(person.ID, "duplicate")}, 0, repository.MainScope))
	mustRebuildSetup(t, s.events.Append(ctx, lifeEventID, "event", []domain.Event{
		domain.LifeEventCreated{BaseEvent: domain.NewBaseEvent(), EventID: lifeEventID, PersonID: &person.ID, FactType: domain.FactPersonResidence},
		domain.NewLifeEventDeleted(lifeEventID, "duplicate")}, 0, repository.MainScope))
	lat, long := "N1.5", "W2.5"
	for _, id := range []uuid.UUID{person.ID, strayID} {
		mustRebuildSetup(t, s.read.SavePerson(ctx, onBranch, &repository.PersonReadModel{ID: id, GivenName: "Ada", BirthPlaceLat: &lat, Version: 2}))
	}
	mustRebuildSetup(t, s.read.SaveEvent(ctx, onBranch, &repository.EventReadModel{ID: lifeEventID, OwnerType: "person", OwnerID: person.ID,
		FactType: domain.FactPersonResidence, PlaceLat: &lat, PlaceLong: &long, Version: 2}))

	result, err := s.rebuild(t, s.read)
	if err != nil {
		t.Fatalf("rebuild: %v", err)
	}
	if result.Backfilled != 2 {
		t.Errorf("Backfilled = %d, want 2 (the person and the life event, not the stray)", result.Backfilled)
	}
	for _, id := range []uuid.UUID{person.ID, lifeEventID} {
		stored, err := s.events.ReadStream(ctx, id)
		if err != nil || len(stored) != 3 || stored[2].EventType != "ImportedValuesBackfilled" || stored[2].Version != 3 {
			t.Errorf("stream %s = %+v, %v; want a backfill at version 3 after the delete", id, stored, err)
		}
	}
	again, err := s.rebuild(t, s.read)
	if err != nil || again.Backfilled != 0 {
		t.Errorf("second rebuild = %+v, %v; want nothing backfilled", again, err)
	}
}
