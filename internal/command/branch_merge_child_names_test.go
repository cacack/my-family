package command_test

import (
	"context"
	"errors"
	"testing"

	"github.com/google/uuid"

	"github.com/cacack/my-family/internal/command"
	"github.com/cacack/my-family/internal/domain"
	"github.com/cacack/my-family/internal/repository"
)

// childAfterFamilyBranch builds #817's branch shape: the branch creates family
// F, then person P, then links P into F. F's stream starts first, so a merge
// replays F (child link included) before P's PersonCreated.
func childAfterFamilyBranch(t *testing.T, f *branchFixture) (branch *domain.Branch, familyID, childID uuid.UUID) {
	t.Helper()
	ctx := context.Background()
	parent, err := f.handler.CreatePerson(ctx, command.CreatePersonInput{GivenName: "Ada", Surname: "Lovelace"})
	if err != nil {
		t.Fatalf("CreatePerson parent failed: %v", err)
	}
	branch, err = f.handler.CreateBranch(ctx, "child-after-family", "")
	if err != nil {
		t.Fatalf("CreateBranch failed: %v", err)
	}
	onBranch := f.handler.WithBranch(branch)
	family, err := onBranch.CreateFamily(ctx, command.CreateFamilyInput{Partner1ID: &parent.ID})
	if err != nil {
		t.Fatalf("branch CreateFamily failed: %v", err)
	}
	child, err := onBranch.CreatePerson(ctx, command.CreatePersonInput{GivenName: "Byron", Surname: "King"})
	if err != nil {
		t.Fatalf("branch CreatePerson child failed: %v", err)
	}
	if _, err := onBranch.LinkChild(ctx, command.LinkChildInput{FamilyID: family.ID, ChildID: child.ID}); err != nil {
		t.Fatalf("branch LinkChild failed: %v", err)
	}
	return branch, family.ID, child.ID
}

// assertMainChildNamed checks main's child row for childID in familyID carries
// the child's real names.
func assertMainChildNamed(t *testing.T, f *branchFixture, familyID, childID uuid.UUID) {
	t.Helper()
	children, err := f.readStore.GetFamilyChildren(context.Background(), domain.MainBranchID, familyID)
	if err != nil {
		t.Fatalf("main GetFamilyChildren failed: %v", err)
	}
	if len(children) != 1 || children[0].PersonID != childID {
		t.Fatalf("main children = %+v, want the one child %s", children, childID)
	}
	if got := children[0]; got.PersonGivenName != "Byron" || got.PersonSurname != "King" {
		t.Errorf("main child row names = %q %q, want Byron King", got.PersonGivenName, got.PersonSurname)
	}
}

// TestMergeBranch_ChildCreatedAfterFamilyKeepsItsNames (#817): replaying the
// family's child link before the child's PersonCreated must not leave main's
// child row nameless.
func TestMergeBranch_ChildCreatedAfterFamilyKeepsItsNames(t *testing.T) {
	f := newBranchFixture()
	branch, familyID, childID := childAfterFamilyBranch(t, f)

	if _, err := f.handler.MergeBranch(context.Background(), command.MergeBranchInput{BranchID: branch.ID}); err != nil {
		t.Fatalf("MergeBranch failed: %v", err)
	}
	assertMainChildNamed(t, f, familyID, childID)
}

// TestResumeMerge_ChildCreatedAfterFamilyKeepsItsNames (#817): the same shape
// interrupted after the family lands and before the child does; the resume
// replays the child and main's child row ends up named.
func TestResumeMerge_ChildCreatedAfterFamilyKeepsItsNames(t *testing.T) {
	var faulty *faultyReplayStore
	f := newBranchFixtureWith(branchFixtureDeps{
		wrapEvents: func(inner repository.EventStore) repository.EventStore {
			faulty = &faultyReplayStore{EventStore: inner}
			return faulty
		},
	})
	ctx := context.Background()
	branch, familyID, childID := childAfterFamilyBranch(t, f)

	// The family's stream lands first; the child's is the merge's second
	// mainline append.
	faulty.armed, faulty.failAt = true, 2
	_, err := f.handler.MergeBranch(ctx, command.MergeBranchInput{BranchID: branch.ID})
	faulty.armed = false
	if !errors.Is(err, command.ErrMergePartiallyApplied) {
		t.Fatalf("MergeBranch error = %v, want ErrMergePartiallyApplied", err)
	}
	children, err := f.readStore.GetFamilyChildren(ctx, domain.MainBranchID, familyID)
	if err != nil || len(children) != 1 {
		t.Fatalf("main children after interruption = %+v (err %v), want the family's link landed", children, err)
	}

	if _, err := f.handler.ResumeMerge(ctx, command.ResumeMergeInput{BranchID: branch.ID}); err != nil {
		t.Fatalf("ResumeMerge failed: %v", err)
	}
	assertMainChildNamed(t, f, familyID, childID)
}
