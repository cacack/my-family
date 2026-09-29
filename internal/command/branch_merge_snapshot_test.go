package command_test

import (
	"context"
	"encoding/json"
	"errors"
	"testing"

	"github.com/google/uuid"

	"github.com/cacack/my-family/internal/command"
	"github.com/cacack/my-family/internal/domain"
	"github.com/cacack/my-family/internal/repository"
)

// mergeClaim decodes the BranchMerged claim on a branch's own stream.
func mergeClaim(t *testing.T, f *branchFixture, branch *domain.Branch) domain.BranchMerged {
	t.Helper()
	for _, evt := range branchEventsFor(t, f, branch.ID, domain.BranchID(branch.ID)) {
		if evt.EventType != "BranchMerged" {
			continue
		}
		var claim domain.BranchMerged
		if err := json.Unmarshal(evt.Data, &claim); err != nil {
			t.Fatalf("decoding the claim: %v", err)
		}
		return claim
	}
	t.Fatal("the branch has no BranchMerged claim")
	return domain.BranchMerged{}
}

// TestMergeBranch_SnapshotBefore is #833's command contract: asked to, a merge
// marks the mainline with "Before merging <branch>" at the head it stood at
// before the merge, records the snapshot on its claim, and nothing but the
// snapshot's own marker lies on the mainline between that position and the
// merge's first replayed event.
func TestMergeBranch_SnapshotBefore(t *testing.T) {
	s := seedMerge(t, "Byron")
	ctx := context.Background()
	head := logHead(t, s.f)

	result, err := s.f.handler.MergeBranch(ctx, command.MergeBranchInput{BranchID: s.branch.ID, SnapshotBefore: true})
	if err != nil {
		t.Fatalf("MergeBranch failed: %v", err)
	}
	snapshot := result.PreMergeSnapshot
	if snapshot == nil {
		t.Fatal("the merge took no snapshot")
	}
	if snapshot.Name != "Before merging surname-hypothesis" {
		t.Errorf("snapshot name = %q", snapshot.Name)
	}
	if !snapshot.BranchID.IsMain() {
		t.Errorf("snapshot branch = %v, want the mainline", snapshot.BranchID)
	}
	if snapshot.Position != head {
		t.Errorf("snapshot position = %d, want the pre-merge head %d", snapshot.Position, head)
	}

	claim := mergeClaim(t, s.f, s.branch)
	if claim.PreMergeSnapshotID == nil || *claim.PreMergeSnapshotID != snapshot.ID {
		t.Errorf("claim pre_merge_snapshot_id = %v, want %s", claim.PreMergeSnapshotID, snapshot.ID)
	}

	after, err := s.f.eventStore.ReadBranch(ctx, domain.MainBranchID, snapshot.Position, 100)
	if err != nil {
		t.Fatalf("ReadBranch failed: %v", err)
	}
	if len(after) != 2 || after[0].EventType != "SnapshotCreated" || after[1].EventType != "PersonUpdated" {
		types := make([]string, len(after))
		for i := range after {
			types[i] = after[i].EventType
		}
		t.Fatalf("mainline after the snapshot = %v, want the snapshot marker then the replayed edit", types)
	}
}

// TestMergeBranch_NoSnapshotUnlessAsked keeps the flag opt-in at the command:
// a merge that did not ask records no snapshot and takes none.
func TestMergeBranch_NoSnapshotUnlessAsked(t *testing.T) {
	s, positions := seedStaleMerge(t, "Byron")
	ctx := context.Background()

	result, err := s.f.handler.MergeBranch(ctx, command.MergeBranchInput{BranchID: s.branch.ID})
	if err != nil {
		t.Fatalf("MergeBranch failed: %v", err)
	}
	if result.PreMergeSnapshot != nil {
		t.Errorf("PreMergeSnapshot = %+v, want none", result.PreMergeSnapshot)
	}
	if claim := mergeClaim(t, s.f, s.branch); claim.PreMergeSnapshotID != nil {
		t.Errorf("claim pre_merge_snapshot_id = %v, want none", claim.PreMergeSnapshotID)
	}
	listed, err := positions.List(ctx, domain.MainBranchID)
	if err != nil {
		t.Fatalf("List failed: %v", err)
	}
	if len(listed) != 0 {
		t.Errorf("snapshots = %d, want none", len(listed))
	}
}

// TestMergeBranch_SnapshotDiscardedWhenRefused: a merge refused after its
// snapshot was taken (here, the mainline moved under a replayed stream) leaves
// no "Before merging" snapshot behind for a merge that never happened.
func TestMergeBranch_SnapshotDiscardedWhenRefused(t *testing.T) {
	s, positions := seedStaleMerge(t, "Byron")
	ctx := context.Background()
	positions.rival = rivalGivenNameEdit(t, s)

	_, err := s.f.handler.MergeBranch(ctx, command.MergeBranchInput{BranchID: s.branch.ID, SnapshotBefore: true})
	if !errors.Is(err, command.ErrMergePlanStale) {
		t.Fatalf("MergeBranch error = %v, want ErrMergePlanStale", err)
	}
	listed, err := positions.List(ctx, domain.MainBranchID)
	if err != nil {
		t.Fatalf("List failed: %v", err)
	}
	if len(listed) != 0 {
		t.Errorf("snapshots after a refused merge = %d, want the pre-merge snapshot discarded", len(listed))
	}
	// The log keeps both lifecycle events (ES-002).
	all, err := s.f.eventStore.ReadAll(ctx, 0, 1000)
	if err != nil {
		t.Fatalf("ReadAll failed: %v", err)
	}
	var created, deleted int
	for _, evt := range all {
		switch evt.EventType {
		case "SnapshotCreated":
			created++
		case "SnapshotDeleted":
			deleted++
		}
	}
	if created != 1 || deleted != 1 {
		t.Errorf("snapshot events: created %d, deleted %d; want one of each", created, deleted)
	}
}

// failingSnapshotWrites fails the registry writes the snapshot commands'
// projections make, on request.
type failingSnapshotWrites struct {
	*stalePositions
	upsertErr error
	deleteErr error
}

func (f *failingSnapshotWrites) Upsert(ctx context.Context, snapshot *domain.Snapshot) error {
	if f.upsertErr != nil {
		return f.upsertErr
	}
	return f.stalePositions.Upsert(ctx, snapshot)
}

func (f *failingSnapshotWrites) Delete(ctx context.Context, id uuid.UUID) error {
	if f.deleteErr != nil {
		return f.deleteErr
	}
	return f.stalePositions.Delete(ctx, id)
}

func newFailingSnapshotFixture(t *testing.T) (mergeSeed, *failingSnapshotWrites) {
	t.Helper()
	var wrapped *failingSnapshotWrites
	f := newBranchFixtureWith(branchFixtureDeps{
		wrapPositions: func(inner repository.SnapshotStore) repository.SnapshotStore {
			wrapped = &failingSnapshotWrites{stalePositions: &stalePositions{SnapshotStore: inner}}
			return wrapped
		},
	})
	return seedMergeInto(t, f, "Byron"), wrapped
}

// TestMergeBranch_SnapshotFailureRefusesTheMerge: a merge asked to take a
// snapshot does not go ahead without one — it is refused before the claim,
// with the branch still active and the mainline untouched.
func TestMergeBranch_SnapshotFailureRefusesTheMerge(t *testing.T) {
	s, store := newFailingSnapshotFixture(t)
	ctx := context.Background()
	store.upsertErr = errors.New("registry unavailable")

	_, err := s.f.handler.MergeBranch(ctx, command.MergeBranchInput{BranchID: s.branch.ID, SnapshotBefore: true})
	if !errors.Is(err, store.upsertErr) {
		t.Fatalf("MergeBranch error = %v, want the snapshot failure", err)
	}
	branch, err := s.f.branchStore.Get(ctx, s.branch.ID)
	if err != nil {
		t.Fatalf("branchStore.Get failed: %v", err)
	}
	if branch.Status != domain.BranchStatusActive {
		t.Errorf("branch status = %q, want it still active", branch.Status)
	}
	onMain, err := s.f.readStore.GetPerson(ctx, domain.MainBranchID, s.person)
	if err != nil {
		t.Fatalf("GetPerson failed: %v", err)
	}
	if onMain.Surname != "Lovelace" {
		t.Errorf("main surname = %q, want it untouched", onMain.Surname)
	}
}

// TestMergeBranch_SnapshotDiscardFailureKeepsTheRefusal: when the discard of a
// refused merge's snapshot fails too, both are reported and the refusal still
// classifies as itself.
func TestMergeBranch_SnapshotDiscardFailureKeepsTheRefusal(t *testing.T) {
	s, store := newFailingSnapshotFixture(t)
	ctx := context.Background()
	store.rival = rivalGivenNameEdit(t, s)
	store.deleteErr = errors.New("registry unavailable")

	_, err := s.f.handler.MergeBranch(ctx, command.MergeBranchInput{BranchID: s.branch.ID, SnapshotBefore: true})
	if !errors.Is(err, command.ErrMergePlanStale) {
		t.Errorf("MergeBranch error = %v, want it to still be ErrMergePlanStale", err)
	}
	if !errors.Is(err, store.deleteErr) {
		t.Errorf("MergeBranch error = %v, want it to report the failed discard", err)
	}
}

// rivalGivenNameEdit is a mainline edit to the seeded person, landing inside
// the merge's planning→claim window, which makes the merge's plan stale.
func rivalGivenNameEdit(t *testing.T, s mergeSeed) func() {
	t.Helper()
	ctx := context.Background()
	return func() {
		person, err := s.f.readStore.GetPerson(ctx, domain.MainBranchID, s.person)
		if err != nil {
			t.Errorf("rival GetPerson failed: %v", err)
			return
		}
		given := "Augusta"
		if _, err := s.f.handler.UpdatePerson(ctx, command.UpdatePersonInput{ID: s.person, GivenName: &given, Version: person.Version}); err != nil {
			t.Errorf("rival UpdatePerson failed: %v", err)
		}
	}
}
