package command_test

import (
	"context"
	"errors"
	"testing"

	"github.com/google/uuid"

	"github.com/cacack/my-family/internal/command"
	"github.com/cacack/my-family/internal/domain"
)

// Cross-stream changes main makes to the two persons of a branch person merge
// (#834): the replay order when the branch created the survivor, main linking
// the merged person as a child after the fork, and main merging the merged
// person into someone else.

// familyOf creates a family with the given partners through h.
func familyOf(t *testing.T, h *command.Handler, partner1, partner2 *uuid.UUID) uuid.UUID {
	t.Helper()
	fam, err := h.CreateFamily(context.Background(), command.CreateFamilyInput{Partner1ID: partner1, Partner2ID: partner2})
	if err != nil {
		t.Fatalf("CreateFamily failed: %v", err)
	}
	return fam.ID
}

// parentsFamily creates a parent on main through h and a family with them as
// its one partner.
func parentsFamily(t *testing.T, h *command.Handler) uuid.UUID {
	t.Helper()
	parent, err := h.CreatePerson(context.Background(), command.CreatePersonInput{GivenName: "Pat", Surname: "Parent", Gender: "female"})
	if err != nil {
		t.Fatalf("CreatePerson(parent) failed: %v", err)
	}
	return familyOf(t, h, &parent.ID, nil)
}

// linkChild links child into family through h.
func linkChild(t *testing.T, h *command.Handler, family, child uuid.UUID) {
	t.Helper()
	if _, err := h.LinkChild(context.Background(), command.LinkChildInput{FamilyID: family, ChildID: child}); err != nil {
		t.Fatalf("LinkChild failed: %v", err)
	}
}

// TestMergeBranch_BranchCreatedSurvivorLandsBeforeItsReferences: the branch
// creates the survivor, a family naming them, and a child link under that
// family, touches the merged person, then merges. Moving the survivor's stream
// (creation and merge together) after the merged person's would land the
// family and the child link before the survivor exists, leaving main with a
// blank partner and a fatherless pedigree edge.
func TestMergeBranch_BranchCreatedSurvivorLandsBeforeItsReferences(t *testing.T) {
	f := newBranchFixture()
	ctx := context.Background()
	_, merged := mergePair(t, f.handler)
	child, err := f.handler.CreatePerson(ctx, command.CreatePersonInput{GivenName: "Kit", Surname: "Child"})
	if err != nil {
		t.Fatalf("CreatePerson failed: %v", err)
	}
	branch, err := f.handler.CreateBranch(ctx, "created-survivor", "")
	if err != nil {
		t.Fatalf("CreateBranch failed: %v", err)
	}
	scoped := f.handler.WithBranch(branch)
	onBranch := domain.BranchID(branch.ID)

	survivor, err := scoped.CreatePerson(ctx, command.CreatePersonInput{GivenName: "Sam", Surname: "Survivor", Gender: "male"})
	if err != nil {
		t.Fatalf("branch CreatePerson failed: %v", err)
	}
	family := familyOf(t, scoped, &survivor.ID, nil)
	linkChild(t, scoped, family, child.ID)
	notes := "touched on the branch"
	if _, err := scoped.UpdatePerson(ctx, command.UpdatePersonInput{ID: merged, Notes: &notes, Version: 1}); err != nil {
		t.Fatalf("branch UpdatePerson failed: %v", err)
	}
	mergeOn(t, f, scoped, onBranch, survivor.ID, merged)

	if _, err := f.handler.MergeBranch(ctx, command.MergeBranchInput{BranchID: branch.ID}); err != nil {
		t.Fatalf("MergeBranch failed: %v", err)
	}
	for _, scope := range []domain.BranchID{onBranch, domain.MainBranchID} {
		fam, err := f.readStore.GetFamily(ctx, scope, family)
		if err != nil || fam == nil || fam.Partner1GivenName != "Sam" || fam.Partner1Surname != "Survivor" {
			t.Errorf("%s family = %+v (err=%v), want partner1 Sam Survivor", scope, fam, err)
		}
		edge, err := f.readStore.GetPedigreeEdge(ctx, scope, child.ID)
		if err != nil || edge == nil || edge.FatherID == nil || *edge.FatherID != survivor.ID || edge.FatherName == "" {
			t.Errorf("%s child pedigree edge = %+v (err=%v), want the survivor as named father", scope, edge, err)
		}
	}
	if p := personOn(t, f, domain.MainBranchID, merged); p != nil {
		t.Errorf("main still has the merged person: %+v", p)
	}
}

// requirePersonMergeBlocker asserts the precheck reports the refusal as one
// named person_merge_conflicts_main blocker on the survivor's stream whose fix
// leaves that stream out (#831 with #834).
func requirePersonMergeBlocker(t *testing.T, h *command.Handler, branchID, survivor, merged uuid.UUID) {
	t.Helper()
	blockers, err := h.PrecheckMerge(context.Background(), command.PrecheckMergeInput{BranchID: branchID})
	if err != nil {
		t.Fatalf("PrecheckMerge failed: %v", err)
	}
	if len(blockers) != 1 {
		t.Fatalf("blockers = %+v, want exactly the person merge", blockers)
	}
	b := blockers[0]
	if b.Kind != command.BlockerPersonMergeConflictsMain || b.StreamID != survivor || b.ReferencedID != merged ||
		b.EntityType != "person" || b.ReferencedType != "person" || b.SuggestedResolution != command.FixLeaveOut {
		t.Errorf("blocker = %+v, want person_merge_conflicts_main on the survivor, naming the merged person, fixed by leave_out", b)
	}
	if b.EntityName == "" || b.ReferencedName == "" {
		t.Errorf("blocker = %+v, want both persons named", b)
	}
}

// TestMergeBranch_MainChildLinkAfterForkRefuses: the survivor is a child of
// one family; after the fork main makes the merged person the child of
// another. The replayed merge would drop the merged person's parentage (a
// person has one child family), so it is refused with nothing written, and a
// "main" resolution of the survivor's stream skips it.
func TestMergeBranch_MainChildLinkAfterForkRefuses(t *testing.T) {
	f := newBranchFixture()
	ctx := context.Background()
	survivor, merged := mergePair(t, f.handler)
	linkChild(t, f.handler, parentsFamily(t, f.handler), survivor)
	branch, err := f.handler.CreateBranch(ctx, "parentage", "")
	if err != nil {
		t.Fatalf("CreateBranch failed: %v", err)
	}
	mergeOn(t, f, f.handler.WithBranch(branch), domain.BranchID(branch.ID), survivor, merged)

	later := parentsFamily(t, f.handler)
	linkChild(t, f.handler, later, merged)

	_, err = f.handler.MergeBranch(ctx, command.MergeBranchInput{BranchID: branch.ID})
	if !errors.Is(err, command.ErrMergeDanglingReference) || !errors.Is(err, command.ErrChildFamilyConflict) {
		t.Fatalf("MergeBranch = %v, want ErrMergeDanglingReference wrapping ErrChildFamilyConflict", err)
	}
	if b, _ := f.branchStore.Get(ctx, branch.ID); b.Status != domain.BranchStatusActive {
		t.Errorf("branch status = %s, want active - the refusal writes nothing", b.Status)
	}
	requirePersonMergeBlocker(t, f.handler, branch.ID, survivor, merged)

	if _, err := f.handler.MergeBranch(ctx, command.MergeBranchInput{
		BranchID:    branch.ID,
		Resolutions: map[uuid.UUID]command.MergeResolution{survivor: command.ResolveMain},
	}); err != nil {
		t.Fatalf("MergeBranch resolving the survivor to main failed: %v", err)
	}
	children, err := f.readStore.GetFamilyChildren(ctx, domain.MainBranchID, later)
	if err != nil || len(children) != 1 || children[0].PersonID != merged {
		t.Errorf("main's later family children = %+v (err=%v), want the merged person kept", children, err)
	}
}

// TestMergeBranch_BranchUnlinkBeforeMergeIsNotRefused: at the fork the two
// persons are children of different families; the branch unlinks one before
// merging, as MergePersons requires. The child-family check follows the replay,
// so the merge is not refused on main's pre-replay state.
func TestMergeBranch_BranchUnlinkBeforeMergeIsNotRefused(t *testing.T) {
	f := newBranchFixture()
	ctx := context.Background()
	survivor, merged := mergePair(t, f.handler)
	linkChild(t, f.handler, parentsFamily(t, f.handler), survivor)
	other := parentsFamily(t, f.handler)
	linkChild(t, f.handler, other, merged)
	branch, err := f.handler.CreateBranch(ctx, "unlink-then-merge", "")
	if err != nil {
		t.Fatalf("CreateBranch failed: %v", err)
	}
	scoped := f.handler.WithBranch(branch)
	if err := scoped.UnlinkChild(ctx, command.UnlinkChildInput{FamilyID: other, ChildID: merged}); err != nil {
		t.Fatalf("branch UnlinkChild failed: %v", err)
	}
	mergeOn(t, f, scoped, domain.BranchID(branch.ID), survivor, merged)

	if _, err := f.handler.MergeBranch(ctx, command.MergeBranchInput{BranchID: branch.ID}); err != nil {
		t.Fatalf("MergeBranch failed: %v", err)
	}
	fam, err := f.readStore.GetFamily(ctx, domain.MainBranchID, other)
	if err != nil || fam == nil || fam.ChildCount != 0 {
		t.Errorf("main's unlinked family = %+v (err=%v), want no children", fam, err)
	}
}

// TestMergeBranch_MergedPersonMergedAwayOnMainRefuses: the branch merges M
// into S while main merges M into T. Main's merge writes only to T's stream,
// so no per-stream comparison sees it; the two merges disagree about who M is,
// so the branch merge is refused, and a "main" resolution keeps main's.
func TestMergeBranch_MergedPersonMergedAwayOnMainRefuses(t *testing.T) {
	f := newBranchFixture()
	ctx := context.Background()
	survivor, merged := mergePair(t, f.handler)
	branch, err := f.handler.CreateBranch(ctx, "two-hypotheses", "")
	if err != nil {
		t.Fatalf("CreateBranch failed: %v", err)
	}
	mergeOn(t, f, f.handler.WithBranch(branch), domain.BranchID(branch.ID), survivor, merged)

	third, err := f.handler.CreatePerson(ctx, command.CreatePersonInput{GivenName: "Morgan", Surname: "Third", Gender: "male"})
	if err != nil {
		t.Fatalf("CreatePerson failed: %v", err)
	}
	mergeOn(t, f, f.handler, domain.MainBranchID, third.ID, merged)

	_, err = f.handler.MergeBranch(ctx, command.MergeBranchInput{BranchID: branch.ID})
	if !errors.Is(err, command.ErrMergeDanglingReference) {
		t.Fatalf("MergeBranch = %v, want ErrMergeDanglingReference", err)
	}
	if p := personOn(t, f, domain.MainBranchID, survivor); p == nil || p.BirthPlace != "" {
		t.Errorf("main survivor = %+v, want untouched by the refused merge", p)
	}
	requirePersonMergeBlocker(t, f.handler, branch.ID, survivor, merged)

	if _, err := f.handler.MergeBranch(ctx, command.MergeBranchInput{
		BranchID:    branch.ID,
		Resolutions: map[uuid.UUID]command.MergeResolution{survivor: command.ResolveMain},
	}); err != nil {
		t.Fatalf("MergeBranch resolving the survivor to main failed: %v", err)
	}
	if p := personOn(t, f, domain.MainBranchID, third.ID); p == nil || p.BirthPlace != "Riverton" {
		t.Errorf("main third person = %+v, want main's merge kept", p)
	}
	if p := personOn(t, f, domain.MainBranchID, survivor); p == nil || p.BirthPlace != "" {
		t.Errorf("main survivor = %+v, want the branch's merge skipped", p)
	}
}
