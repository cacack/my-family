package command_test

import (
	"context"
	"errors"
	"reflect"
	"strings"
	"testing"

	"github.com/google/uuid"

	"github.com/cacack/my-family/internal/command"
	"github.com/cacack/my-family/internal/domain"
	"github.com/cacack/my-family/internal/query"
	"github.com/cacack/my-family/internal/repository"
	"github.com/cacack/my-family/internal/repository/memory"
)

// branchFixture wires the in-memory backends the branch commands need.
type branchFixture struct {
	eventStore  *memory.EventStore
	readStore   *memory.ReadModelStore
	branchStore *memory.BranchStore
	handler     *command.Handler
}

func newBranchFixture() *branchFixture {
	eventStore := memory.NewEventStore()
	readStore := memory.NewReadModelStore()
	branchStore := memory.NewBranchStore()
	snapshotStore := memory.NewSnapshotStore(eventStore)

	return &branchFixture{
		eventStore:  eventStore,
		readStore:   readStore,
		branchStore: branchStore,
		handler:     command.NewHandlerWithBranches(eventStore, readStore, branchStore, snapshotStore),
	}
}

// failingPositions is a MaxPositionReader that always errors.
type failingPositions struct{ err error }

func (f failingPositions) GetMaxPosition(context.Context) (int64, error) { return 0, f.err }

func TestCreateBranch(t *testing.T) {
	f := newBranchFixture()
	ctx := context.Background()

	// One mainline event so the base position is non-zero and meaningful.
	person, err := f.handler.CreatePerson(ctx, command.CreatePersonInput{GivenName: "Ada", Surname: "Lovelace"})
	if err != nil {
		t.Fatalf("CreatePerson failed: %v", err)
	}

	branch, err := f.handler.CreateBranch(ctx, "maternal-line", "testing the Smith hypothesis")
	if err != nil {
		t.Fatalf("CreateBranch failed: %v", err)
	}

	if branch.ID == uuid.Nil {
		t.Error("branch ID is nil")
	}
	if branch.Name != "maternal-line" {
		t.Errorf("Name = %q, want %q", branch.Name, "maternal-line")
	}
	if branch.Description != "testing the Smith hypothesis" {
		t.Errorf("Description = %q, want the supplied description", branch.Description)
	}
	if branch.Status != domain.BranchStatusActive {
		t.Errorf("Status = %q, want %q", branch.Status, domain.BranchStatusActive)
	}
	if branch.BasePosition != 1 {
		t.Errorf("BasePosition = %d, want 1 (the head after one mainline event)", branch.BasePosition)
	}

	// The BranchCreated event is tagged with the new branch's own scope.
	events, err := f.eventStore.ReadBranch(ctx, domain.BranchID(branch.ID), 0, 100)
	if err != nil {
		t.Fatalf("ReadBranch failed: %v", err)
	}
	if len(events) != 1 {
		t.Fatalf("branch scope holds %d events, want 1", len(events))
	}
	if events[0].EventType != "BranchCreated" {
		t.Errorf("EventType = %q, want BranchCreated", events[0].EventType)
	}
	if events[0].StreamID != branch.ID {
		t.Errorf("StreamID = %s, want the branch ID %s", events[0].StreamID, branch.ID)
	}
	if events[0].Version != 1 {
		t.Errorf("Version = %d, want 1", events[0].Version)
	}

	// The registry row exists — written by the projection of that event, which is
	// why the event above must exist for the row to be reconstructible.
	stored, err := f.branchStore.Get(ctx, branch.ID)
	if err != nil {
		t.Fatalf("branchStore.Get failed: %v", err)
	}
	if stored.Name != branch.Name || stored.BasePosition != branch.BasePosition {
		t.Errorf("registry row = %+v, want name/base position matching %+v", stored, branch)
	}
	if stored.Status != domain.BranchStatusActive {
		t.Errorf("registry status = %q, want %q", stored.Status, domain.BranchStatusActive)
	}

	// The mainline is untouched by branch creation.
	if p, err := f.readStore.GetPerson(ctx, domain.MainBranchID, person.ID); err != nil || p == nil {
		t.Fatalf("main person missing after CreateBranch: person=%v err=%v", p, err)
	}
}

func TestCreateBranch_ValidationErrors(t *testing.T) {
	tests := []struct {
		name        string
		branchName  string
		description string
		wantErr     error
	}{
		{"empty name", "", "", domain.ErrBranchNameRequired},
		{"name too long", strings.Repeat("a", 101), "", domain.ErrBranchNameTooLong},
		{"description too long", "ok", strings.Repeat("a", 501), domain.ErrBranchDescTooLong},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			f := newBranchFixture()

			_, err := f.handler.CreateBranch(context.Background(), tt.branchName, tt.description)
			if !errors.Is(err, tt.wantErr) {
				t.Fatalf("CreateBranch error = %v, want %v", err, tt.wantErr)
			}
			branches, err := f.branchStore.List(context.Background())
			if err != nil {
				t.Fatalf("List failed: %v", err)
			}
			if len(branches) != 0 {
				t.Errorf("registry holds %d branches after a rejected create, want 0", len(branches))
			}
		})
	}
}

func TestCreateBranch_MissingCollaborators(t *testing.T) {
	eventStore := memory.NewEventStore()
	readStore := memory.NewReadModelStore()
	ctx := context.Background()

	// No branch store at all.
	noStore := command.NewHandler(eventStore, readStore)
	if _, err := noStore.CreateBranch(ctx, "x", ""); !errors.Is(err, command.ErrBranchStoreRequired) {
		t.Errorf("CreateBranch without a branch store = %v, want ErrBranchStoreRequired", err)
	}

	// Branch store but no position source.
	noPositions := command.NewHandlerWithBranchStore(eventStore, readStore, memory.NewBranchStore())
	if _, err := noPositions.CreateBranch(ctx, "x", ""); !errors.Is(err, command.ErrPositionSourceRequired) {
		t.Errorf("CreateBranch without a position source = %v, want ErrPositionSourceRequired", err)
	}
}

func TestCreateBranch_PositionSourceError(t *testing.T) {
	sentinel := errors.New("boom")
	handler := command.NewHandlerWithBranches(
		memory.NewEventStore(),
		memory.NewReadModelStore(),
		memory.NewBranchStore(),
		failingPositions{err: sentinel},
	)

	_, err := handler.CreateBranch(context.Background(), "x", "")
	if !errors.Is(err, sentinel) {
		t.Fatalf("CreateBranch error = %v, want it to wrap %v", err, sentinel)
	}
}

// TestBranchIsolation is the core promise of #670: an edit made on a branch is
// visible on the branch and invisible on main.
func TestBranchIsolation(t *testing.T) {
	f := newBranchFixture()
	ctx := context.Background()

	person, err := f.handler.CreatePerson(ctx, command.CreatePersonInput{GivenName: "Ada", Surname: "Lovelace"})
	if err != nil {
		t.Fatalf("CreatePerson failed: %v", err)
	}

	branch, err := f.handler.CreateBranch(ctx, "surname-hypothesis", "")
	if err != nil {
		t.Fatalf("CreateBranch failed: %v", err)
	}

	byron := "Byron"
	if _, err := f.handler.WithBranch(branch).UpdatePerson(ctx, command.UpdatePersonInput{
		ID:      person.ID,
		Surname: &byron,
		Version: person.Version,
	}); err != nil {
		t.Fatalf("branch-scoped UpdatePerson failed: %v", err)
	}

	onBranch, err := f.readStore.GetPerson(ctx, domain.BranchID(branch.ID), person.ID)
	if err != nil {
		t.Fatalf("GetPerson on branch failed: %v", err)
	}
	if onBranch.Surname != "Byron" {
		t.Errorf("branch surname = %q, want %q", onBranch.Surname, "Byron")
	}

	onMain, err := f.readStore.GetPerson(ctx, domain.MainBranchID, person.ID)
	if err != nil {
		t.Fatalf("GetPerson on main failed: %v", err)
	}
	if onMain.Surname != "Lovelace" {
		t.Errorf("main surname = %q, want it untouched (%q)", onMain.Surname, "Lovelace")
	}

	// The edit's event carries the branch scope, not main's.
	branchEvents, err := f.eventStore.ReadBranch(ctx, domain.BranchID(branch.ID), 0, 100)
	if err != nil {
		t.Fatalf("ReadBranch failed: %v", err)
	}
	var updates int
	for _, e := range branchEvents {
		if e.EventType == "PersonUpdated" {
			updates++
		}
	}
	if updates != 1 {
		t.Errorf("branch holds %d PersonUpdated events, want 1", updates)
	}

	mainEvents, err := f.eventStore.ReadBranch(ctx, domain.MainBranchID, 0, 100)
	if err != nil {
		t.Fatalf("ReadBranch(main) failed: %v", err)
	}
	for _, e := range mainEvents {
		if e.EventType == "PersonUpdated" {
			t.Fatal("branch edit leaked a PersonUpdated event onto main")
		}
	}
}

// TestBranchIsolation_Family mirrors TestBranchIsolation for the family
// aggregate, which reaches execute only since the family commands were routed
// through it.
func TestBranchIsolation_Family(t *testing.T) {
	f := newBranchFixture()
	ctx := context.Background()

	p1, err := f.handler.CreatePerson(ctx, command.CreatePersonInput{GivenName: "Ada", Surname: "Lovelace"})
	if err != nil {
		t.Fatalf("CreatePerson failed: %v", err)
	}
	p2, err := f.handler.CreatePerson(ctx, command.CreatePersonInput{GivenName: "William", Surname: "King"})
	if err != nil {
		t.Fatalf("CreatePerson failed: %v", err)
	}
	family, err := f.handler.CreateFamily(ctx, command.CreateFamilyInput{
		Partner1ID:    &p1.ID,
		Partner2ID:    &p2.ID,
		MarriagePlace: "London",
	})
	if err != nil {
		t.Fatalf("CreateFamily failed: %v", err)
	}

	branch, err := f.handler.CreateBranch(ctx, "marriage-place-hypothesis", "")
	if err != nil {
		t.Fatalf("CreateBranch failed: %v", err)
	}

	// Edit the family on the branch.
	bristol := "Bristol"
	updated, err := f.handler.WithBranch(branch).UpdateFamily(ctx, command.UpdateFamilyInput{
		ID:            family.ID,
		MarriagePlace: &bristol,
		Version:       family.Version,
	})
	if err != nil {
		t.Fatalf("branch-scoped UpdateFamily failed: %v", err)
	}
	if updated.Version != family.Version+1 {
		t.Errorf("branch update version = %d, want %d", updated.Version, family.Version+1)
	}

	onBranch, err := f.readStore.GetFamily(ctx, domain.BranchID(branch.ID), family.ID)
	if err != nil {
		t.Fatalf("GetFamily on branch failed: %v", err)
	}
	if onBranch.MarriagePlace != "Bristol" {
		t.Errorf("branch marriage place = %q, want %q", onBranch.MarriagePlace, "Bristol")
	}

	onMain, err := f.readStore.GetFamily(ctx, domain.MainBranchID, family.ID)
	if err != nil {
		t.Fatalf("GetFamily on main failed: %v", err)
	}
	if onMain.MarriagePlace != "London" {
		t.Errorf("main marriage place = %q, want it untouched (%q)", onMain.MarriagePlace, "London")
	}

	// A family created wholly on the branch is invisible on main.
	branchOnly, err := f.handler.WithBranch(branch).CreateFamily(ctx, command.CreateFamilyInput{
		Partner1ID:    &p1.ID,
		MarriagePlace: "Nowhere",
	})
	if err != nil {
		t.Fatalf("branch-scoped CreateFamily failed: %v", err)
	}
	if bf, err := f.readStore.GetFamily(ctx, domain.BranchID(branch.ID), branchOnly.ID); err != nil || bf == nil {
		t.Fatalf("branch-created family missing from the branch: family=%v err=%v", bf, err)
	}
	mf, err := f.readStore.GetFamily(ctx, domain.MainBranchID, branchOnly.ID)
	if err != nil {
		t.Fatalf("GetFamily on main failed: %v", err)
	}
	if mf != nil {
		t.Error("branch-created family leaked onto main")
	}

	// No family event carries the main scope beyond the original create.
	mainEvents, err := f.eventStore.ReadBranch(ctx, domain.MainBranchID, 0, 100)
	if err != nil {
		t.Fatalf("ReadBranch(main) failed: %v", err)
	}
	var mainFamilyEvents int
	for _, e := range mainEvents {
		if e.EventType == "FamilyCreated" || e.EventType == "FamilyUpdated" {
			mainFamilyEvents++
		}
	}
	if mainFamilyEvents != 1 {
		t.Errorf("main holds %d family events, want 1 (the original create)", mainFamilyEvents)
	}
}

// TestExecute_RejectsNonBranchAwareEvent guards the silent-write-to-main hazard:
// a Source is not part of the branch-aware slice, so a branch-scoped source
// command must fail rather than land on main.
func TestExecute_RejectsNonBranchAwareEvent(t *testing.T) {
	f := newBranchFixture()
	ctx := context.Background()

	branch, err := f.handler.CreateBranch(ctx, "sources", "")
	if err != nil {
		t.Fatalf("CreateBranch failed: %v", err)
	}

	_, err = f.handler.WithBranch(branch).CreateSource(ctx, command.CreateSourceInput{
		Title:      "1880 Census",
		SourceType: "census",
	})
	if !errors.Is(err, command.ErrEventTypeNotBranchAware) {
		t.Fatalf("branch-scoped CreateSource error = %v, want ErrEventTypeNotBranchAware", err)
	}
	if !strings.Contains(err.Error(), "SourceCreated") {
		t.Errorf("error %v should name the offending event type SourceCreated", err)
	}

	// Nothing landed anywhere: no source on main, no event in the log.
	sources, _, err := f.readStore.ListSources(ctx, repository.ListOptions{Limit: 10})
	if err != nil {
		t.Fatalf("ListSources failed: %v", err)
	}
	if len(sources) != 0 {
		t.Errorf("main holds %d sources after a rejected branch write, want 0", len(sources))
	}
	all, err := f.eventStore.ReadAll(ctx, 0, 100)
	if err != nil {
		t.Fatalf("ReadAll failed: %v", err)
	}
	for _, e := range all {
		if e.EventType == "SourceCreated" {
			t.Fatal("rejected branch write still appended a SourceCreated event")
		}
	}

	// The same command on the unscoped handler still works.
	if _, err := f.handler.CreateSource(ctx, command.CreateSourceInput{
		Title:      "1880 Census",
		SourceType: "census",
	}); err != nil {
		t.Fatalf("mainline CreateSource failed: %v", err)
	}
}

func TestDeleteBranch(t *testing.T) {
	f := newBranchFixture()
	ctx := context.Background()

	person, err := f.handler.CreatePerson(ctx, command.CreatePersonInput{GivenName: "Ada", Surname: "Lovelace"})
	if err != nil {
		t.Fatalf("CreatePerson failed: %v", err)
	}
	branch, err := f.handler.CreateBranch(ctx, "discard-me", "")
	if err != nil {
		t.Fatalf("CreateBranch failed: %v", err)
	}

	byron := "Byron"
	if _, err := f.handler.WithBranch(branch).UpdatePerson(ctx, command.UpdatePersonInput{
		ID:      person.ID,
		Surname: &byron,
		Version: person.Version,
	}); err != nil {
		t.Fatalf("branch-scoped UpdatePerson failed: %v", err)
	}

	if err := f.handler.DeleteBranch(ctx, branch.ID); err != nil {
		t.Fatalf("DeleteBranch failed: %v", err)
	}

	stored, err := f.branchStore.Get(ctx, branch.ID)
	if err != nil {
		t.Fatalf("branchStore.Get failed: %v", err)
	}
	if stored.Status != domain.BranchStatusArchived {
		t.Errorf("Status = %q, want %q", stored.Status, domain.BranchStatusArchived)
	}

	// The overlay row is purged, so a branch read falls back to main again.
	onBranch, err := f.readStore.GetPerson(ctx, domain.BranchID(branch.ID), person.ID)
	if err != nil {
		t.Fatalf("GetPerson on branch failed: %v", err)
	}
	if onBranch.Surname != "Lovelace" {
		t.Errorf("branch surname after delete = %q, want the main row %q", onBranch.Surname, "Lovelace")
	}

	// History is append-only: the branch's events remain in the log.
	branchEvents, err := f.eventStore.ReadBranch(ctx, domain.BranchID(branch.ID), 0, 100)
	if err != nil {
		t.Fatalf("ReadBranch failed: %v", err)
	}
	if len(branchEvents) != 3 { // BranchCreated, PersonUpdated, BranchDeleted
		t.Errorf("branch holds %d events after delete, want 3", len(branchEvents))
	}
	last := branchEvents[len(branchEvents)-1]
	if last.EventType != "BranchDeleted" {
		t.Errorf("last branch event = %q, want BranchDeleted", last.EventType)
	}
	// Versions are per (stream, branch): the branch's own stream holds
	// BranchCreated at 1, so BranchDeleted lands at 2 regardless of the
	// PersonUpdated event on the person's stream.
	if last.Version != 2 {
		t.Errorf("BranchDeleted version = %d, want 2", last.Version)
	}
}

func TestDeleteBranch_NotActive(t *testing.T) {
	f := newBranchFixture()
	ctx := context.Background()

	branch, err := f.handler.CreateBranch(ctx, "already-gone", "")
	if err != nil {
		t.Fatalf("CreateBranch failed: %v", err)
	}
	if err := f.handler.DeleteBranch(ctx, branch.ID); err != nil {
		t.Fatalf("first DeleteBranch failed: %v", err)
	}

	err = f.handler.DeleteBranch(ctx, branch.ID)
	if !errors.Is(err, command.ErrBranchNotActive) {
		t.Fatalf("second DeleteBranch error = %v, want ErrBranchNotActive", err)
	}

	// A merged branch is equally terminal.
	merged, err := f.handler.CreateBranch(ctx, "merged", "")
	if err != nil {
		t.Fatalf("CreateBranch failed: %v", err)
	}
	if err := f.branchStore.UpdateStatus(ctx, merged.ID, domain.BranchStatusMerged); err != nil {
		t.Fatalf("UpdateStatus failed: %v", err)
	}
	if err := f.handler.DeleteBranch(ctx, merged.ID); !errors.Is(err, command.ErrBranchNotActive) {
		t.Fatalf("DeleteBranch on a merged branch = %v, want ErrBranchNotActive", err)
	}
}

func TestDeleteBranch_NotFound(t *testing.T) {
	f := newBranchFixture()

	err := f.handler.DeleteBranch(context.Background(), uuid.New())
	if !errors.Is(err, repository.ErrBranchNotFound) {
		t.Fatalf("DeleteBranch error = %v, want repository.ErrBranchNotFound", err)
	}
}

func TestDeleteBranch_NoBranchStore(t *testing.T) {
	handler := command.NewHandler(memory.NewEventStore(), memory.NewReadModelStore())

	err := handler.DeleteBranch(context.Background(), uuid.New())
	if !errors.Is(err, command.ErrBranchStoreRequired) {
		t.Fatalf("DeleteBranch without a branch store = %v, want ErrBranchStoreRequired", err)
	}
}

func TestWithBranch_NilIsMainline(t *testing.T) {
	f := newBranchFixture()
	ctx := context.Background()

	res, err := f.handler.WithBranch(nil).CreatePerson(ctx, command.CreatePersonInput{GivenName: "Ada"})
	if err != nil {
		t.Fatalf("CreatePerson on WithBranch(nil) failed: %v", err)
	}

	p, err := f.readStore.GetPerson(ctx, domain.MainBranchID, res.ID)
	if err != nil {
		t.Fatalf("GetPerson failed: %v", err)
	}
	if p == nil {
		t.Fatal("WithBranch(nil) did not write to main")
	}
}

// TestWithBranch_DoesNotMutateReceiver proves the scope is a copy: the original
// handler keeps writing to main after a scoped copy is taken.
func TestWithBranch_DoesNotMutateReceiver(t *testing.T) {
	f := newBranchFixture()
	ctx := context.Background()

	branch, err := f.handler.CreateBranch(ctx, "scratch", "")
	if err != nil {
		t.Fatalf("CreateBranch failed: %v", err)
	}
	scoped := f.handler.WithBranch(branch)
	if scoped == f.handler {
		t.Fatal("WithBranch returned the receiver, want a copy")
	}

	res, err := f.handler.CreatePerson(ctx, command.CreatePersonInput{GivenName: "Ada"})
	if err != nil {
		t.Fatalf("CreatePerson failed: %v", err)
	}
	mainEvents, err := f.eventStore.ReadBranch(ctx, domain.MainBranchID, 0, 100)
	if err != nil {
		t.Fatalf("ReadBranch(main) failed: %v", err)
	}
	var found bool
	for _, e := range mainEvents {
		if e.StreamID == res.ID {
			found = true
		}
	}
	if !found {
		t.Error("the original handler stopped writing to main after WithBranch")
	}
}

// TestBranchIsolation_LinkChild covers the child-link path, which reaches
// execute only since the family commands were routed through it.
func TestBranchIsolation_LinkChild(t *testing.T) {
	f := newBranchFixture()
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

	branch, err := f.handler.CreateBranch(ctx, "child-hypothesis", "")
	if err != nil {
		t.Fatalf("CreateBranch failed: %v", err)
	}

	if _, err := f.handler.WithBranch(branch).LinkChild(ctx, command.LinkChildInput{
		FamilyID: family.ID,
		ChildID:  child.ID,
	}); err != nil {
		t.Fatalf("branch-scoped LinkChild failed: %v", err)
	}

	onBranch, err := f.readStore.GetFamilyChildren(ctx, domain.BranchID(branch.ID), family.ID)
	if err != nil {
		t.Fatalf("GetFamilyChildren on branch failed: %v", err)
	}
	if len(onBranch) != 1 || onBranch[0].PersonID != child.ID {
		t.Errorf("branch children = %+v, want the linked child", onBranch)
	}

	onMain, err := f.readStore.GetFamilyChildren(ctx, domain.MainBranchID, family.ID)
	if err != nil {
		t.Fatalf("GetFamilyChildren on main failed: %v", err)
	}
	if len(onMain) != 0 {
		t.Errorf("main children = %+v, want none", onMain)
	}
}

// TestBranchIsolation_FamilyDeleteAndUnlink pins down the remaining two family
// mutations on a branch, so the HTTP layer knows which operations are safe to
// expose with a branch parameter.
func TestBranchIsolation_FamilyDeleteAndUnlink(t *testing.T) {
	f := newBranchFixture()
	ctx := context.Background()

	parent, err := f.handler.CreatePerson(ctx, command.CreatePersonInput{GivenName: "Ada", Surname: "Lovelace"})
	if err != nil {
		t.Fatalf("CreatePerson failed: %v", err)
	}
	child, err := f.handler.CreatePerson(ctx, command.CreatePersonInput{GivenName: "Byron", Surname: "King"})
	if err != nil {
		t.Fatalf("CreatePerson failed: %v", err)
	}

	withChild, err := f.handler.CreateFamily(ctx, command.CreateFamilyInput{Partner1ID: &parent.ID})
	if err != nil {
		t.Fatalf("CreateFamily failed: %v", err)
	}
	if _, err := f.handler.LinkChild(ctx, command.LinkChildInput{FamilyID: withChild.ID, ChildID: child.ID}); err != nil {
		t.Fatalf("LinkChild failed: %v", err)
	}
	childless, err := f.handler.CreateFamily(ctx, command.CreateFamilyInput{Partner1ID: &parent.ID})
	if err != nil {
		t.Fatalf("CreateFamily failed: %v", err)
	}

	branch, err := f.handler.CreateBranch(ctx, "prune", "")
	if err != nil {
		t.Fatalf("CreateBranch failed: %v", err)
	}
	scoped := f.handler.WithBranch(branch)

	// Unlink on the branch: gone from the branch, still linked on main.
	if err := scoped.UnlinkChild(ctx, command.UnlinkChildInput{FamilyID: withChild.ID, ChildID: child.ID}); err != nil {
		t.Fatalf("branch-scoped UnlinkChild failed: %v", err)
	}
	onBranch, err := f.readStore.GetFamilyChildren(ctx, domain.BranchID(branch.ID), withChild.ID)
	if err != nil {
		t.Fatalf("GetFamilyChildren on branch failed: %v", err)
	}
	if len(onBranch) != 0 {
		t.Errorf("branch children = %+v, want none after unlink", onBranch)
	}
	onMain, err := f.readStore.GetFamilyChildren(ctx, domain.MainBranchID, withChild.ID)
	if err != nil {
		t.Fatalf("GetFamilyChildren on main failed: %v", err)
	}
	if len(onMain) != 1 {
		t.Errorf("main children = %+v, want the link untouched", onMain)
	}

	// Delete on the branch: tombstoned on the branch, alive on main.
	if err := scoped.DeleteFamily(ctx, command.DeleteFamilyInput{ID: childless.ID, Version: childless.Version}); err != nil {
		t.Fatalf("branch-scoped DeleteFamily failed: %v", err)
	}
	bf, err := f.readStore.GetFamily(ctx, domain.BranchID(branch.ID), childless.ID)
	if err != nil {
		t.Fatalf("GetFamily on branch failed: %v", err)
	}
	if bf != nil {
		t.Error("family still visible on the branch after a branch-scoped delete (missing tombstone)")
	}
	mf, err := f.readStore.GetFamily(ctx, domain.MainBranchID, childless.ID)
	if err != nil {
		t.Fatalf("GetFamily on main failed: %v", err)
	}
	if mf == nil {
		t.Error("branch-scoped delete removed the family from main")
	}
}

// ============================================================================
// Branch-scoped command reads (the fix that makes a branch usable past its
// first write). Every one of these fails when the command layer resolves
// entities on main: the version check compares main's version against the
// branch's stream, and a branch-only entity has no main row at all.
// ============================================================================

// TestBranchSequentialPersonUpdates is the headline case: before command reads
// were branch-scoped, the second update on a branch was an unsatisfiable
// version check — main's version and the branch's stream version diverge after
// the first write.
func TestBranchSequentialPersonUpdates(t *testing.T) {
	f := newBranchFixture()
	ctx := context.Background()

	person, err := f.handler.CreatePerson(ctx, command.CreatePersonInput{GivenName: "Ada", Surname: "Lovelace"})
	if err != nil {
		t.Fatalf("CreatePerson failed: %v", err)
	}
	branch, err := f.handler.CreateBranch(ctx, "surnames", "")
	if err != nil {
		t.Fatalf("CreateBranch failed: %v", err)
	}
	scoped := f.handler.WithBranch(branch)

	version := person.Version
	for _, surname := range []string{"Byron", "King", "Noel"} {
		s := surname
		res, err := scoped.UpdatePerson(ctx, command.UpdatePersonInput{
			ID:      person.ID,
			Surname: &s,
			Version: version,
		})
		if err != nil {
			t.Fatalf("branch UpdatePerson to %q failed: %v", surname, err)
		}
		version = res.Version

		onBranch, err := f.readStore.GetPerson(ctx, domain.BranchID(branch.ID), person.ID)
		if err != nil {
			t.Fatalf("GetPerson on branch failed: %v", err)
		}
		if onBranch.Surname != surname {
			t.Errorf("branch surname = %q, want %q", onBranch.Surname, surname)
		}
		if onBranch.Version != version {
			t.Errorf("branch read-model version = %d, want %d", onBranch.Version, version)
		}
	}
	if version != person.Version+3 {
		t.Errorf("final version = %d, want %d", version, person.Version+3)
	}

	// Main is untouched by all three.
	onMain, err := f.readStore.GetPerson(ctx, domain.MainBranchID, person.ID)
	if err != nil {
		t.Fatalf("GetPerson on main failed: %v", err)
	}
	if onMain.Surname != "Lovelace" {
		t.Errorf("main surname = %q, want it untouched", onMain.Surname)
	}
	if onMain.Version != person.Version {
		t.Errorf("main version = %d, want %d", onMain.Version, person.Version)
	}
}

// TestBranchOnlyPersonIsEditable covers the 404 dead end: a person created on a
// branch has no mainline row, so a main-scoped read resolved to nil and every
// subsequent command reported ErrPersonNotFound.
func TestBranchOnlyPersonIsEditable(t *testing.T) {
	f := newBranchFixture()
	ctx := context.Background()

	branch, err := f.handler.CreateBranch(ctx, "new-person", "")
	if err != nil {
		t.Fatalf("CreateBranch failed: %v", err)
	}
	scoped := f.handler.WithBranch(branch)

	person, err := scoped.CreatePerson(ctx, command.CreatePersonInput{GivenName: "Ghost", Surname: "Ancestor"})
	if err != nil {
		t.Fatalf("branch CreatePerson failed: %v", err)
	}

	place := "Bristol"
	updated, err := scoped.UpdatePerson(ctx, command.UpdatePersonInput{
		ID:         person.ID,
		BirthPlace: &place,
		Version:    person.Version,
	})
	if err != nil {
		t.Fatalf("branch UpdatePerson on a branch-only person failed: %v", err)
	}

	onBranch, err := f.readStore.GetPerson(ctx, domain.BranchID(branch.ID), person.ID)
	if err != nil {
		t.Fatalf("GetPerson on branch failed: %v", err)
	}
	if onBranch.BirthPlace != "Bristol" {
		t.Errorf("branch birth place = %q, want Bristol", onBranch.BirthPlace)
	}

	// Still invisible on main, and deletable on the branch.
	if onMain, err := f.readStore.GetPerson(ctx, domain.MainBranchID, person.ID); err != nil || onMain != nil {
		t.Fatalf("branch-only person leaked onto main: person=%v err=%v", onMain, err)
	}
	if err := scoped.DeletePerson(ctx, command.DeletePersonInput{ID: person.ID, Version: updated.Version}); err != nil {
		t.Fatalf("branch DeletePerson on a branch-only person failed: %v", err)
	}
}

// TestBranchNameLifecycle exercises add/update/remove as a sequence on one
// branch — each step depends on the previous step's branch state.
func TestBranchNameLifecycle(t *testing.T) {
	f := newBranchFixture()
	ctx := context.Background()

	person, err := f.handler.CreatePerson(ctx, command.CreatePersonInput{GivenName: "Ada", Surname: "Lovelace"})
	if err != nil {
		t.Fatalf("CreatePerson failed: %v", err)
	}
	// A primary name on main, so the branch's alias is an addition to an
	// existing bucket rather than the first name.
	if _, err := f.handler.AddName(ctx, command.AddNameInput{
		PersonID:  person.ID,
		GivenName: "Ada",
		Surname:   "Lovelace",
		IsPrimary: true,
	}); err != nil {
		t.Fatalf("main AddName failed: %v", err)
	}

	branch, err := f.handler.CreateBranch(ctx, "aliases", "")
	if err != nil {
		t.Fatalf("CreateBranch failed: %v", err)
	}
	scoped := f.handler.WithBranch(branch)

	added, err := scoped.AddName(ctx, command.AddNameInput{
		PersonID:  person.ID,
		GivenName: "Augusta",
		Surname:   "Byron",
	})
	if err != nil {
		t.Fatalf("branch AddName failed: %v", err)
	}

	nickname := "Ada"
	if _, err := scoped.UpdateName(ctx, command.UpdateNameInput{
		PersonID: person.ID,
		NameID:   added.ID,
		Nickname: &nickname,
	}); err != nil {
		t.Fatalf("branch UpdateName failed: %v", err)
	}

	branchNames, err := f.readStore.GetPersonNames(ctx, domain.BranchID(branch.ID), person.ID)
	if err != nil {
		t.Fatalf("GetPersonNames on branch failed: %v", err)
	}
	if len(branchNames) != 2 {
		t.Fatalf("branch names = %d, want 2 (the primary plus the alias)", len(branchNames))
	}

	if err := scoped.DeleteName(ctx, command.DeleteNameInput{PersonID: person.ID, NameID: added.ID}); err != nil {
		t.Fatalf("branch DeleteName failed: %v", err)
	}
	branchNames, err = f.readStore.GetPersonNames(ctx, domain.BranchID(branch.ID), person.ID)
	if err != nil {
		t.Fatalf("GetPersonNames on branch failed: %v", err)
	}
	if len(branchNames) != 1 {
		t.Errorf("branch names after delete = %d, want 1", len(branchNames))
	}

	// Main still has only the name CreatePerson generated.
	mainNames, err := f.readStore.GetPersonNames(ctx, domain.MainBranchID, person.ID)
	if err != nil {
		t.Fatalf("GetPersonNames on main failed: %v", err)
	}
	if len(mainNames) != 1 {
		t.Errorf("main names = %d, want 1 - the branch alias leaked", len(mainNames))
	}
}

// TestBranchDeletePersonAfterEdit deletes on a branch after a prior branch write
// (so the branch's version is ahead of main's) and checks the tombstone.
func TestBranchDeletePersonAfterEdit(t *testing.T) {
	f := newBranchFixture()
	ctx := context.Background()

	person, err := f.handler.CreatePerson(ctx, command.CreatePersonInput{GivenName: "Ada", Surname: "Lovelace"})
	if err != nil {
		t.Fatalf("CreatePerson failed: %v", err)
	}
	branch, err := f.handler.CreateBranch(ctx, "prune-person", "")
	if err != nil {
		t.Fatalf("CreateBranch failed: %v", err)
	}
	scoped := f.handler.WithBranch(branch)

	note := "duplicate of another record"
	edited, err := scoped.UpdatePerson(ctx, command.UpdatePersonInput{
		ID:      person.ID,
		Notes:   &note,
		Version: person.Version,
	})
	if err != nil {
		t.Fatalf("branch UpdatePerson failed: %v", err)
	}

	if err := scoped.DeletePerson(ctx, command.DeletePersonInput{
		ID:      person.ID,
		Version: edited.Version,
		Reason:  "hypothesis rejected",
	}); err != nil {
		t.Fatalf("branch DeletePerson after a branch edit failed: %v", err)
	}

	// Tombstoned on the branch: the main fallback must not resurrect them.
	onBranch, err := f.readStore.GetPerson(ctx, domain.BranchID(branch.ID), person.ID)
	if err != nil {
		t.Fatalf("GetPerson on branch failed: %v", err)
	}
	if onBranch != nil {
		t.Errorf("person still visible on the branch after delete: %+v", onBranch)
	}

	onMain, err := f.readStore.GetPerson(ctx, domain.MainBranchID, person.ID)
	if err != nil {
		t.Fatalf("GetPerson on main failed: %v", err)
	}
	if onMain == nil {
		t.Fatal("branch delete removed the person from main")
	}
	if onMain.Notes != "" {
		t.Errorf("main notes = %q, want empty - the branch edit leaked", onMain.Notes)
	}
}

// TestUnscopedHandler_ReadsResolveMain is the regression guard for the read
// change: with no branch scope every command read resolves on main exactly as
// before, including the not-found and version-conflict paths.
func TestUnscopedHandler_ReadsResolveMain(t *testing.T) {
	f := newBranchFixture()
	ctx := context.Background()

	person, err := f.handler.CreatePerson(ctx, command.CreatePersonInput{GivenName: "Ada", Surname: "Lovelace"})
	if err != nil {
		t.Fatalf("CreatePerson failed: %v", err)
	}

	// A person that exists only on a branch is invisible to the unscoped handler.
	branch, err := f.handler.CreateBranch(ctx, "hidden", "")
	if err != nil {
		t.Fatalf("CreateBranch failed: %v", err)
	}
	branchOnly, err := f.handler.WithBranch(branch).CreatePerson(ctx, command.CreatePersonInput{GivenName: "Ghost"})
	if err != nil {
		t.Fatalf("branch CreatePerson failed: %v", err)
	}
	surname := "Nobody"
	if _, err := f.handler.UpdatePerson(ctx, command.UpdatePersonInput{
		ID:      branchOnly.ID,
		Surname: &surname,
		Version: branchOnly.Version,
	}); !errors.Is(err, command.ErrPersonNotFound) {
		t.Errorf("unscoped UpdatePerson of a branch-only person = %v, want ErrPersonNotFound", err)
	}

	// A branch edit to a main person does not disturb the unscoped version check.
	byron := "Byron"
	if _, err := f.handler.WithBranch(branch).UpdatePerson(ctx, command.UpdatePersonInput{
		ID:      person.ID,
		Surname: &byron,
		Version: person.Version,
	}); err != nil {
		t.Fatalf("branch UpdatePerson failed: %v", err)
	}
	if _, err := f.handler.UpdatePerson(ctx, command.UpdatePersonInput{
		ID:      person.ID,
		Surname: &byron,
		Version: person.Version + 1,
	}); !errors.Is(err, repository.ErrConcurrencyConflict) {
		t.Errorf("unscoped UpdatePerson with a branch-derived version = %v, want ErrConcurrencyConflict", err)
	}
	res, err := f.handler.UpdatePerson(ctx, command.UpdatePersonInput{
		ID:      person.ID,
		Surname: &byron,
		Version: person.Version,
	})
	if err != nil {
		t.Fatalf("unscoped UpdatePerson with main's version failed: %v", err)
	}
	if res.Version != person.Version+1 {
		t.Errorf("main version = %d, want %d", res.Version, person.Version+1)
	}
}

// TestRollback_RefusedOnBranchScopedHandler covers the mixed-scope trap: the
// Rollback* commands read the current version and the deleted flag from main but
// append through execute on the handler's scope, so on a branch they would check
// a main-derived expected version against the branch's independent counter.
// They must refuse up front instead.
func TestRollback_RefusedOnBranchScopedHandler(t *testing.T) {
	f := newBranchFixture()
	ctx := context.Background()

	person, err := f.handler.CreatePerson(ctx, command.CreatePersonInput{GivenName: "Ada", Surname: "Lovelace"})
	if err != nil {
		t.Fatalf("CreatePerson failed: %v", err)
	}
	byron := "Byron"
	updated, err := f.handler.UpdatePerson(ctx, command.UpdatePersonInput{
		ID:      person.ID,
		Surname: &byron,
		Version: person.Version,
	})
	if err != nil {
		t.Fatalf("UpdatePerson failed: %v", err)
	}

	branch, err := f.handler.CreateBranch(ctx, "rollback-attempt", "")
	if err != nil {
		t.Fatalf("CreateBranch failed: %v", err)
	}
	scoped := f.handler.WithBranch(branch)

	// The guard runs before any store access, so an arbitrary ID is enough for
	// the entity types with no fixture.
	rollbacks := []struct {
		name string
		call func() error
	}{
		{"Person", func() error { _, err := scoped.RollbackPerson(ctx, person.ID, 1); return err }},
		{"Family", func() error { _, err := scoped.RollbackFamily(ctx, uuid.New(), 1); return err }},
		{"Source", func() error { _, err := scoped.RollbackSource(ctx, uuid.New(), 1); return err }},
		{"Citation", func() error { _, err := scoped.RollbackCitation(ctx, uuid.New(), 1); return err }},
		{"Media", func() error { _, err := scoped.RollbackMedia(ctx, uuid.New(), 1); return err }},
	}
	for _, rb := range rollbacks {
		if err := rb.call(); !errors.Is(err, command.ErrRollbackNotBranchScoped) {
			t.Errorf("Rollback%s on a branch-scoped handler = %v, want ErrRollbackNotBranchScoped", rb.name, err)
		}
	}

	// Nothing was appended: the branch holds only its BranchCreated event.
	branchEvents, err := f.eventStore.ReadBranch(ctx, domain.BranchID(branch.ID), 0, 100)
	if err != nil {
		t.Fatalf("ReadBranch failed: %v", err)
	}
	if len(branchEvents) != 1 {
		t.Errorf("branch holds %d events after refused rollbacks, want 1 (BranchCreated)", len(branchEvents))
	}

	// The unscoped handler still rolls back normally.
	res, err := f.handler.RollbackPerson(ctx, person.ID, 1)
	if err != nil {
		t.Fatalf("mainline RollbackPerson failed: %v", err)
	}
	if res.NewVersion != updated.Version+1 {
		t.Errorf("NewVersion = %d, want %d", res.NewVersion, updated.Version+1)
	}
	onMain, err := f.readStore.GetPerson(ctx, domain.MainBranchID, person.ID)
	if err != nil {
		t.Fatalf("GetPerson failed: %v", err)
	}
	if onMain.Surname != "Lovelace" {
		t.Errorf("main surname after rollback = %q, want %q", onMain.Surname, "Lovelace")
	}
}

// driftSeed is the mainline fixture the branch-aware projection probes run
// against: two partners, a family, a linked child and an extra name.
type driftSeed struct {
	f       *branchFixture
	branch  *domain.Branch
	person  uuid.UUID
	partner uuid.UUID
	child   uuid.UUID
	family  uuid.UUID
	name    uuid.UUID
}

func seedDriftFixture(t *testing.T) driftSeed {
	t.Helper()
	f := newBranchFixture()
	ctx := context.Background()

	newPerson := func(given, surname string) uuid.UUID {
		res, err := f.handler.CreatePerson(ctx, command.CreatePersonInput{GivenName: given, Surname: surname})
		if err != nil {
			t.Fatalf("CreatePerson(%s) failed: %v", given, err)
		}
		return res.ID
	}
	seed := driftSeed{f: f}
	seed.person = newPerson("Ada", "Lovelace")
	seed.partner = newPerson("William", "King")
	seed.child = newPerson("Byron", "King")

	family, err := f.handler.CreateFamily(ctx, command.CreateFamilyInput{
		Partner1ID:    &seed.person,
		Partner2ID:    &seed.partner,
		MarriagePlace: "London",
	})
	if err != nil {
		t.Fatalf("CreateFamily failed: %v", err)
	}
	seed.family = family.ID

	if _, err := f.handler.LinkChild(ctx, command.LinkChildInput{
		FamilyID:     seed.family,
		ChildID:      seed.child,
		RelationType: "biological",
	}); err != nil {
		t.Fatalf("LinkChild failed: %v", err)
	}

	name, err := f.handler.AddName(ctx, command.AddNameInput{
		PersonID:  seed.person,
		GivenName: "Augusta",
		Surname:   "Byron",
		NameType:  "birth",
	})
	if err != nil {
		t.Fatalf("AddName failed: %v", err)
	}
	seed.name = name.ID

	branch, err := f.handler.CreateBranch(ctx, "drift-probe", "")
	if err != nil {
		t.Fatalf("CreateBranch failed: %v", err)
	}
	seed.branch = branch

	return seed
}

// branchAwareProbes builds one representative event per entry in
// command.BranchAwareEventTypes. The key set is asserted to equal that allowlist,
// so an event type added to the allowlist without a probe here fails the test.
var branchAwareProbes = map[string]func(s driftSeed) domain.Event{
	"PersonCreated": func(driftSeed) domain.Event {
		return domain.NewPersonCreated(domain.NewPerson("Grace", "Hopper"))
	},
	"PersonUpdated": func(s driftSeed) domain.Event {
		return domain.NewPersonUpdated(s.person, map[string]any{"surname": "Byron"})
	},
	"PersonDeleted": func(s driftSeed) domain.Event {
		return domain.NewPersonDeleted(s.person, "branch hypothesis")
	},
	"FamilyCreated": func(s driftSeed) domain.Event {
		return domain.NewFamilyCreated(domain.NewFamilyWithPartners(&s.person, &s.partner))
	},
	"FamilyUpdated": func(s driftSeed) domain.Event {
		return domain.NewFamilyUpdated(s.family, map[string]any{"marriage_place": "Kent"})
	},
	"FamilyDeleted": func(s driftSeed) domain.Event {
		return domain.NewFamilyDeleted(s.family, "branch hypothesis")
	},
	"ChildLinkedToFamily": func(s driftSeed) domain.Event {
		return domain.NewChildLinkedToFamily(domain.NewFamilyChild(s.family, s.partner, domain.ChildBiological))
	},
	"ChildUnlinkedFromFamily": func(s driftSeed) domain.Event {
		return domain.NewChildUnlinkedFromFamily(s.family, s.child)
	},
	"NameAdded": func(s driftSeed) domain.Event {
		return domain.NewNameAdded(domain.NewPersonName(s.person, "Annabella", "Milbanke"))
	},
	"NameUpdated": func(s driftSeed) domain.Event {
		pn := domain.NewPersonName(s.person, "Annabella", "Milbanke")
		pn.ID = s.name
		return domain.NewNameUpdated(pn)
	},
	"NameRemoved": func(s driftSeed) domain.Event {
		return domain.NewNameRemoved(s.person, s.name)
	},
}

// mainRows is the mainline read-model state a branch-scoped projection must
// leave byte-identical.
type mainRows struct {
	Persons  []*repository.PersonReadModel
	Names    [][]repository.PersonNameReadModel
	Edges    []*repository.PedigreeEdge
	Family   *repository.FamilyReadModel
	Children []repository.FamilyChildReadModel
}

func readMainRows(t *testing.T, s driftSeed) mainRows {
	t.Helper()
	ctx := context.Background()
	rs := s.f.readStore

	var rows mainRows
	for _, id := range []uuid.UUID{s.person, s.partner, s.child} {
		person, err := rs.GetPerson(ctx, domain.MainBranchID, id)
		if err != nil {
			t.Fatalf("GetPerson(main, %s) failed: %v", id, err)
		}
		names, err := rs.GetPersonNames(ctx, domain.MainBranchID, id)
		if err != nil {
			t.Fatalf("GetPersonNames(main, %s) failed: %v", id, err)
		}
		edge, err := rs.GetPedigreeEdge(ctx, domain.MainBranchID, id)
		if err != nil {
			t.Fatalf("GetPedigreeEdge(main, %s) failed: %v", id, err)
		}
		rows.Persons = append(rows.Persons, person)
		rows.Names = append(rows.Names, names)
		rows.Edges = append(rows.Edges, edge)
	}

	family, err := rs.GetFamily(ctx, domain.MainBranchID, s.family)
	if err != nil {
		t.Fatalf("GetFamily(main) failed: %v", err)
	}
	children, err := rs.GetFamilyChildren(ctx, domain.MainBranchID, s.family)
	if err != nil {
		t.Fatalf("GetFamilyChildren(main) failed: %v", err)
	}
	rows.Family = family
	rows.Children = children

	return rows
}

// readMainOnlyCounts totals every read-model table that is still main-only (not
// branch-keyed). A branch-scoped projection must not add a row to any of them —
// that is precisely what disqualifies an event type from the allowlist.
func readMainOnlyCounts(t *testing.T, s driftSeed) map[string]int {
	t.Helper()
	ctx := context.Background()
	rs := s.f.readStore
	opts := repository.ListOptions{Limit: 100}

	tables := []struct {
		name  string
		count func() (int, error)
	}{
		{"sources", func() (int, error) { _, n, err := rs.ListSources(ctx, opts); return n, err }},
		{"citations", func() (int, error) { _, n, err := rs.ListCitations(ctx, opts); return n, err }},
		{"life_events", func() (int, error) { _, n, err := rs.ListEvents(ctx, opts); return n, err }},
		{"attributes", func() (int, error) { _, n, err := rs.ListAttributes(ctx, opts); return n, err }},
		{"notes", func() (int, error) { _, n, err := rs.ListNotes(ctx, opts); return n, err }},
		{"submitters", func() (int, error) { _, n, err := rs.ListSubmitters(ctx, opts); return n, err }},
		{"repositories", func() (int, error) { _, n, err := rs.ListRepositories(ctx, opts); return n, err }},
		{"associations", func() (int, error) { _, n, err := rs.ListAssociations(ctx, opts); return n, err }},
		{"lds_ordinances", func() (int, error) { _, n, err := rs.ListLDSOrdinances(ctx, opts); return n, err }},
		{"evidence_analyses", func() (int, error) { _, n, err := rs.ListEvidenceAnalyses(ctx, opts); return n, err }},
		{"evidence_conflicts", func() (int, error) { _, n, err := rs.ListEvidenceConflicts(ctx, opts); return n, err }},
		{"research_logs", func() (int, error) { _, n, err := rs.ListResearchLogs(ctx, opts); return n, err }},
		{"proof_summaries", func() (int, error) { _, n, err := rs.ListProofSummaries(ctx, opts); return n, err }},
		{"media_for_person", func() (int, error) {
			_, n, err := rs.ListMediaForEntity(ctx, "person", s.person, opts)
			return n, err
		}},
	}

	counts := make(map[string]int, len(tables))
	for _, table := range tables {
		n, err := table.count()
		if err != nil {
			t.Fatalf("counting %s failed: %v", table.name, err)
		}
		counts[table.name] = n
	}
	return counts
}

// TestBranchAwareEventTypes_LeaveMainUntouched is the drift guard between
// command.BranchAwareEventTypes and the projection handlers in
// internal/repository/projection.go (the third hand-maintained sync point
// alongside ES-007 and PR-004). For every allowlisted event type it projects a
// representative event on a branch and asserts the mainline read model — both
// the branch-keyed rows and every still-main-only table — is unchanged.
//
// It catches the dangerous direction: an event type that is on the allowlist
// while its handler writes main, so a branch edit would silently land there. It
// does NOT catch the reverse (a projection handler that became branch-aware but
// was never added to the allowlist — that only over-restricts branches, and
// issue #676 is the tracked path for adding them), and it does not verify that
// the branch overlay itself is written correctly; the TestBranchIsolation* and
// TestBranch* lifecycle tests cover that half.
func TestBranchAwareEventTypes_LeaveMainUntouched(t *testing.T) {
	allowed := command.BranchAwareEventTypes()
	if len(allowed) != len(branchAwareProbes) {
		t.Fatalf("BranchAwareEventTypes has %d entries but %d probes are defined; keep branchAwareProbes in sync",
			len(allowed), len(branchAwareProbes))
	}
	for _, eventType := range allowed {
		if _, ok := branchAwareProbes[eventType]; !ok {
			t.Fatalf("no probe for allowlisted event type %q; add one so its main-safety is proven", eventType)
		}
	}

	ctx := context.Background()
	for _, eventType := range allowed {
		t.Run(eventType, func(t *testing.T) {
			s := seedDriftFixture(t)

			event := branchAwareProbes[eventType](s)
			if event.EventType() != eventType {
				t.Fatalf("probe for %q builds a %q event", eventType, event.EventType())
			}

			beforeRows := readMainRows(t, s)
			beforeCounts := readMainOnlyCounts(t, s)

			projector := repository.NewProjector(s.f.readStore, s.f.branchStore)
			if err := projector.Project(ctx, event, 2, domain.BranchID(s.branch.ID)); err != nil {
				t.Fatalf("projecting %s on the branch failed: %v", eventType, err)
			}

			if afterRows := readMainRows(t, s); !reflect.DeepEqual(beforeRows, afterRows) {
				t.Errorf("%s projected on a branch changed main's read model:\nbefore %+v\nafter  %+v",
					eventType, beforeRows, afterRows)
			}
			for table, before := range beforeCounts {
				if after := readMainOnlyCounts(t, s)[table]; after != before {
					t.Errorf("%s projected on a branch changed the main-only %s table: %d rows, want %d",
						eventType, table, after, before)
				}
			}
		})
	}
}

// conflictBlindEventTypes are the branch-writable event types that summarizeStreams
// deliberately extracts nothing from, with the reason each is safe.
//
// A *Created event opens a stream the branch alone owns, so there is no main-side
// counterpart to disagree with; two independent creates of the same identity are
// caught separately, by the GEDCOM-xref create-vs-create scan, not by field
// comparison. Everything else must be comparable.
var conflictBlindEventTypes = map[string]string{
	"PersonCreated": "opens a branch-only stream; identity collisions are the create_create scan's job",
	"FamilyCreated": "opens a branch-only stream; identity collisions are the create_create scan's job",
}

// TestBranchAwareEventTypes_AreConflictComparable is the drift guard between the
// two hand-maintained lists that together decide whether a branch edit gets
// reviewed: command.BranchAwareEventTypes (what a branch may write) and
// query.ConflictComparable (what the merge conflict scan can see).
//
// An event type on the first list but not the second is silent data loss, not a
// missing feature: the branch may make the edit, the classifier reports no
// conflict, and the merge promotes it over main's concurrent change with no
// review. That is exactly how NameAdded/NameUpdated/NameRemoved slipped through
// — NameUpdated ends in "Updated" but carries no Changes map, so the scan read
// nothing from it.
//
// Adding a branch-writable event type therefore forces a conscious choice here:
// teach the classifier to compare it, or record why it needs no comparison.
func TestBranchAwareEventTypes_AreConflictComparable(t *testing.T) {
	for _, eventType := range command.BranchAwareEventTypes() {
		t.Run(eventType, func(t *testing.T) {
			reason, blind := conflictBlindEventTypes[eventType]
			seenByScan := query.ConflictComparable(eventType)

			switch {
			case seenByScan && blind:
				t.Errorf("%s is comparable but is still listed as conflict-blind (%q); drop it from conflictBlindEventTypes",
					eventType, reason)
			case !seenByScan && !blind:
				t.Errorf("%s is branch-writable but the merge conflict scan extracts nothing from it, "+
					"so a branch edit would be promoted over a concurrent main change without review. "+
					"Teach summarizeStreams to fold it, or add it to conflictBlindEventTypes with a reason.",
					eventType)
			}
		})
	}
}
