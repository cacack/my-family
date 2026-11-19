package command_test

import (
	"bytes"
	"context"
	"os"
	"testing"

	"github.com/google/uuid"

	"github.com/cacack/my-family/internal/command"
	"github.com/cacack/my-family/internal/domain"
	"github.com/cacack/my-family/internal/query"
	"github.com/cacack/my-family/internal/repository"
)

// storedUserID is the user ID a stored event's envelope metadata records.
func storedUserID(t *testing.T, evt *repository.StoredEvent) string {
	t.Helper()
	meta, err := evt.DecodeMetadata()
	if err != nil {
		t.Fatalf("%s metadata does not decode: %v", evt.EventType, err)
	}
	return meta.UserID
}

func allEvents(t *testing.T, f *branchFixture) []repository.StoredEvent {
	t.Helper()
	all, err := f.eventStore.ReadAll(context.Background(), 0, 1_000_000)
	if err != nil {
		t.Fatalf("ReadAll failed: %v", err)
	}
	return all
}

// TestAttribution_EveryCommandRecordsTheActor is invariant ES-009's coverage
// scenario: with an actor on the context, a broad run of commands — entity
// CRUD, a GEDCOM import, the branch lifecycle with a merge, snapshots and a
// rollback — leaves no event in the log without that actor. A command that
// appends around the handler's attributing store leaves an authorless event
// here and fails the test; add new event-writing commands to this run.
func TestAttribution_EveryCommandRecordsTheActor(t *testing.T) {
	const actor = "user-1407"
	f := newBranchFixture()
	h := f.handler
	ctx := domain.ContextWithActor(context.Background(), actor)

	must := func(step string, err error) {
		t.Helper()
		if err != nil {
			t.Fatalf("%s failed: %v", step, err)
		}
	}

	// Persons, with a rollback.
	person, err := h.CreatePerson(ctx, command.CreatePersonInput{GivenName: "Ada", Surname: "Lovelace"})
	must("CreatePerson", err)
	_, err = h.UpdatePerson(ctx, command.UpdatePersonInput{ID: person.ID, Surname: command.Set("Byron"), Version: person.Version})
	must("UpdatePerson", err)
	rolled, err := h.RollbackPerson(ctx, person.ID, person.Version)
	must("RollbackPerson", err)
	must("DeletePerson", h.DeletePerson(ctx, command.DeletePersonInput{ID: person.ID, Version: rolled.NewVersion}))

	// Families.
	partner, err := h.CreatePerson(ctx, command.CreatePersonInput{GivenName: "Charles", Surname: "Babbage"})
	must("CreatePerson", err)
	family, err := h.CreateFamily(ctx, command.CreateFamilyInput{Partner1ID: &partner.ID})
	must("CreateFamily", err)
	famUpdated, err := h.UpdateFamily(ctx, command.UpdateFamilyInput{ID: family.ID, MarriagePlace: command.Set("London"), Version: family.Version})
	must("UpdateFamily", err)
	must("DeleteFamily", h.DeleteFamily(ctx, command.DeleteFamilyInput{ID: family.ID, Version: famUpdated.Version}))

	// Sources.
	source, err := h.CreateSource(ctx, command.CreateSourceInput{SourceType: "book", Title: "Memoirs"})
	must("CreateSource", err)
	title := "Memoirs, 2nd ed."
	srcUpdated, err := h.UpdateSource(ctx, command.UpdateSourceInput{ID: source.ID, Title: &title, Version: source.Version})
	must("UpdateSource", err)
	must("DeleteSource", h.DeleteSource(ctx, source.ID, srcUpdated.Version, "duplicate"))

	// GEDCOM import.
	data, err := os.ReadFile("../../testdata/gedcom-5.5/comprehensive.ged")
	must("reading GEDCOM testdata", err)
	_, err = h.ImportGedcom(ctx, command.ImportGedcomInput{Filename: "comprehensive.ged", FileSize: int64(len(data)), Reader: bytes.NewReader(data)})
	must("ImportGedcom", err)

	// Snapshots.
	snapshot, err := h.CreateSnapshot(ctx, "before research", "")
	must("CreateSnapshot", err)
	must("DeleteSnapshot", h.DeleteSnapshot(ctx, snapshot.ID))

	// Branch lifecycle: edit on a branch, describe it, merge it; close another.
	branch, err := h.CreateBranch(ctx, "theory", "")
	must("CreateBranch", err)
	_, err = h.WithBranch(branch).CreatePerson(ctx, command.CreatePersonInput{GivenName: "Allegra", Surname: "Byron"})
	must("branch CreatePerson", err)
	_, err = h.UpdateBranch(ctx, command.UpdateBranchInput{BranchID: branch.ID, Hypothesis: command.Set("Allegra belongs here")})
	must("UpdateBranch", err)
	_, err = h.MergeBranch(ctx, command.MergeBranchInput{BranchID: branch.ID, Note: "proved"})
	must("MergeBranch", err)
	abandoned, err := h.CreateBranch(ctx, "dead end", "")
	must("CreateBranch", err)
	must("CloseBranch", h.CloseBranch(ctx, abandoned.ID, domain.BranchOutcomeAbandoned, "no evidence"))

	events := allEvents(t, f)
	if len(events) < 20 {
		t.Fatalf("the scenario wrote only %d events; it no longer exercises the commands it names", len(events))
	}
	for i := range events {
		if got := storedUserID(t, &events[i]); got != actor {
			t.Errorf("%s (position %d) UserID = %q, want %q", events[i].EventType, events[i].Position, got, actor)
		}
	}
}

// TestAttribution_MergeKeepsTheOriginalAuthor checks that a merge credits each
// replayed change to whoever made it on the branch — or to no one — and names
// the merger only on its own claim and the merge record.
func TestAttribution_MergeKeepsTheOriginalAuthor(t *testing.T) {
	f := newBranchFixture()
	bg := context.Background()
	asA := domain.ContextWithActor(bg, "user-a")
	asB := domain.ContextWithActor(bg, "user-b")

	branch, err := f.handler.CreateBranch(bg, "theory", "")
	if err != nil {
		t.Fatalf("CreateBranch failed: %v", err)
	}
	onBranch := f.handler.WithBranch(branch)
	authored, err := onBranch.CreatePerson(asA, command.CreatePersonInput{GivenName: "Ada", Surname: "Lovelace"})
	if err != nil {
		t.Fatalf("branch CreatePerson as A failed: %v", err)
	}
	authorless, err := onBranch.CreatePerson(bg, command.CreatePersonInput{GivenName: "Allegra", Surname: "Byron"})
	if err != nil {
		t.Fatalf("branch CreatePerson without an actor failed: %v", err)
	}

	if _, err := f.handler.MergeBranch(asB, command.MergeBranchInput{BranchID: branch.ID, Note: "proved"}); err != nil {
		t.Fatalf("MergeBranch as B failed: %v", err)
	}

	// Each person's creation is all replayed: main had none of its events.
	assertMainAuthor := func(streamID uuid.UUID, want string) {
		t.Helper()
		events := branchEventsFor(t, f, streamID, domain.MainBranchID)
		if len(events) == 0 {
			t.Fatalf("stream %s was not replayed onto main", streamID)
		}
		for i := range events {
			if got := storedUserID(t, &events[i]); got != want {
				t.Errorf("replayed %s UserID = %q, want %q", events[i].EventType, got, want)
			}
		}
	}
	assertMainAuthor(authored.ID, "user-a")
	// The merger must not be credited with a change nobody signed.
	assertMainAuthor(authorless.ID, "")

	claims := 0
	for _, evt := range branchEventsFor(t, f, branch.ID, domain.BranchID(branch.ID)) {
		if evt.EventType != "BranchMerged" {
			continue
		}
		claims++
		if got := storedUserID(t, &evt); got != "user-b" {
			t.Errorf("BranchMerged UserID = %q, want the merger user-b", got)
		}
	}
	if claims != 1 {
		t.Fatalf("found %d BranchMerged claims, want 1", claims)
	}

	service := query.NewBranchService(f.branchStore, f.eventStore, query.NewHistoryService(f.eventStore, f.readStore))
	comparison, err := service.CompareBranch(bg, branch.ID)
	if err != nil {
		t.Fatalf("CompareBranch failed: %v", err)
	}
	if comparison.MergeRecord == nil || comparison.MergeRecord.MergedBy != "user-b" {
		t.Errorf("merge record = %+v, want MergedBy user-b", comparison.MergeRecord)
	}
}

// staticUserNames is a query.UserNames over a fixed map.
type staticUserNames map[uuid.UUID]string

func (n staticUserNames) DisplayNames(_ context.Context, ids []uuid.UUID) (map[uuid.UUID]string, error) {
	out := make(map[uuid.UUID]string)
	for _, id := range ids {
		if name, ok := n[id]; ok {
			out[id] = name
		}
	}
	return out, nil
}

// History entries and the merge record show the display name of the user
// whose ID they carry (#1408).
func TestAttribution_NamesTheUser(t *testing.T) {
	f := newBranchFixture()
	bg := context.Background()
	merger := uuid.New()
	asMerger := domain.ContextWithActor(bg, merger.String())

	branch, err := f.handler.CreateBranch(bg, "theory", "")
	if err != nil {
		t.Fatalf("CreateBranch failed: %v", err)
	}
	if _, err := f.handler.WithBranch(branch).CreatePerson(asMerger, command.CreatePersonInput{GivenName: "Ada", Surname: "Lovelace"}); err != nil {
		t.Fatalf("CreatePerson failed: %v", err)
	}
	if _, err := f.handler.MergeBranch(asMerger, command.MergeBranchInput{BranchID: branch.ID, Note: "proved"}); err != nil {
		t.Fatalf("MergeBranch failed: %v", err)
	}

	history := query.NewHistoryService(f.eventStore, f.readStore)
	history.UseUserNames(staticUserNames{merger: "Grace Hopper"})
	comparison, err := query.NewBranchService(f.branchStore, f.eventStore, history).CompareBranch(bg, branch.ID)
	if err != nil {
		t.Fatalf("CompareBranch failed: %v", err)
	}
	if comparison.MergeRecord == nil || comparison.MergeRecord.MergedByName != "Grace Hopper" {
		t.Errorf("merge record = %+v, want MergedByName Grace Hopper", comparison.MergeRecord)
	}

	global, err := history.GetGlobalHistory(bg, query.GetGlobalHistoryInput{Limit: 50})
	if err != nil {
		t.Fatalf("GetGlobalHistory failed: %v", err)
	}
	named := 0
	for _, entry := range global.Entries {
		if entry.UserID != nil && *entry.UserID == merger.String() {
			if entry.UserName == nil || *entry.UserName != "Grace Hopper" {
				t.Errorf("entry %s %s UserName = %v, want Grace Hopper", entry.EntityType, entry.Action, entry.UserName)
			}
			named++
		}
	}
	if named == 0 {
		t.Error("no history entry carries the merger's user ID")
	}
}
