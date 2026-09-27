package command_test

import (
	"context"
	"errors"
	"slices"
	"testing"

	"github.com/google/uuid"

	"github.com/cacack/my-family/internal/command"
	"github.com/cacack/my-family/internal/domain"
	"github.com/cacack/my-family/internal/repository"
)

// deletePersonOnMain deletes a person on main at the version main has.
func deletePersonOnMain(t *testing.T, f *branchFixture, personID uuid.UUID) {
	t.Helper()
	ctx := context.Background()
	row, err := f.readStore.GetPerson(ctx, domain.MainBranchID, personID)
	if err != nil || row == nil {
		t.Fatalf("main GetPerson %s = %v (err %v)", personID, row, err)
	}
	if err := f.handler.DeletePerson(ctx, command.DeletePersonInput{ID: personID, Version: row.Version}); err != nil {
		t.Fatalf("main DeletePerson failed: %v", err)
	}
}

// TestMergeBranch_RefusesAFamilyPartnerMainDeleted: a family partner is a
// reference to a person just like a child link. main may delete the partner
// while the branch's family is unmerged (on main the person has no family),
// and replaying the family would leave main's family naming a partner main
// does not have.
func TestMergeBranch_RefusesAFamilyPartnerMainDeleted(t *testing.T) {
	ctx := context.Background()
	// Each shape returns the family the branch makes name P; mainFamily is a
	// family main already has, with A as its only partner.
	shapes := map[string]func(t *testing.T, onBranch *command.Handler, a, p uuid.UUID, mainFamily *command.CreateFamilyResult) uuid.UUID{
		"created with the partner": func(t *testing.T, onBranch *command.Handler, a, p uuid.UUID, _ *command.CreateFamilyResult) uuid.UUID {
			family, err := onBranch.CreateFamily(ctx, command.CreateFamilyInput{Partner1ID: &a, Partner2ID: &p})
			if err != nil {
				t.Fatalf("branch CreateFamily failed: %v", err)
			}
			return family.ID
		},
		"updated to the partner": func(t *testing.T, onBranch *command.Handler, _, p uuid.UUID, mainFamily *command.CreateFamilyResult) uuid.UUID {
			if _, err := onBranch.UpdateFamily(ctx, command.UpdateFamilyInput{
				ID: mainFamily.ID, Partner2ID: &p, Version: mainFamily.Version,
			}); err != nil {
				t.Fatalf("branch UpdateFamily failed: %v", err)
			}
			return mainFamily.ID
		},
	}
	for name, shape := range shapes {
		t.Run(name, func(t *testing.T) {
			f := newBranchFixture()
			a, err := f.handler.CreatePerson(ctx, command.CreatePersonInput{GivenName: "Ada", Surname: "Lovelace"})
			if err != nil {
				t.Fatalf("CreatePerson A failed: %v", err)
			}
			p, err := f.handler.CreatePerson(ctx, command.CreatePersonInput{GivenName: "William", Surname: "King"})
			if err != nil {
				t.Fatalf("CreatePerson P failed: %v", err)
			}
			mainFamily, err := f.handler.CreateFamily(ctx, command.CreateFamilyInput{Partner1ID: &a.ID})
			if err != nil {
				t.Fatalf("main CreateFamily failed: %v", err)
			}
			branch, err := f.handler.CreateBranch(ctx, "partner", "")
			if err != nil {
				t.Fatalf("CreateBranch failed: %v", err)
			}
			familyID := shape(t, f.handler.WithBranch(branch), a.ID, p.ID, mainFamily)
			deletePersonOnMain(t, f, p.ID)

			_, err = f.handler.MergeBranch(ctx, command.MergeBranchInput{BranchID: branch.ID})
			if !errors.Is(err, command.ErrMergeDanglingReference) {
				t.Fatalf("MergeBranch error = %v, want ErrMergeDanglingReference", err)
			}
			got, err := f.branchStore.Get(ctx, branch.ID)
			if err != nil {
				t.Fatalf("branch Get failed: %v", err)
			}
			if got.Status != domain.BranchStatusActive {
				t.Errorf("branch status = %s, want active (refused before the claim)", got.Status)
			}

			// Leaving the family as main has it merges cleanly.
			if _, err := f.handler.MergeBranch(ctx, command.MergeBranchInput{
				BranchID:    branch.ID,
				Resolutions: map[uuid.UUID]command.MergeResolution{familyID: command.ResolveMain},
			}); err != nil {
				t.Fatalf("main-resolved MergeBranch failed: %v", err)
			}
			assertNoDanglingPartner(t, f, familyID)
		})
	}
}

// assertNoDanglingPartner checks that each partner main's family names, if it
// exists at all on main, is a person main has.
func assertNoDanglingPartner(t *testing.T, f *branchFixture, familyID uuid.UUID) {
	t.Helper()
	ctx := context.Background()
	row, err := f.readStore.GetFamily(ctx, domain.MainBranchID, familyID)
	if err != nil {
		t.Fatalf("main GetFamily failed: %v", err)
	}
	if row == nil {
		return
	}
	for _, partner := range []*uuid.UUID{row.Partner1ID, row.Partner2ID} {
		if partner == nil {
			continue
		}
		person, err := f.readStore.GetPerson(ctx, domain.MainBranchID, *partner)
		if err != nil {
			t.Fatalf("main GetPerson failed: %v", err)
		}
		if person == nil {
			t.Errorf("main family %s names partner %s, whom main does not have", familyID, *partner)
		}
	}
}

// TestResumeMerge_FamilyPartnerDeletedAfterTheInterruption: the branch edits
// A and creates family F with main's person P as a partner. The merge is
// interrupted after A lands, and main then deletes P, which it may since P
// has no family on main. F is not stale, so the plan would replay it — naming
// a partner main no longer has. The resume lists F as pending instead, and a
// "main" resolution rolls the merge forward without it.
func TestResumeMerge_FamilyPartnerDeletedAfterTheInterruption(t *testing.T) {
	var faulty *faultyReplayStore
	f := newBranchFixtureWith(branchFixtureDeps{
		wrapEvents: func(inner repository.EventStore) repository.EventStore {
			faulty = &faultyReplayStore{EventStore: inner}
			return faulty
		},
	})
	ctx := context.Background()

	a, err := f.handler.CreatePerson(ctx, command.CreatePersonInput{GivenName: "Ada", Surname: "Lovelace"})
	if err != nil {
		t.Fatalf("CreatePerson A failed: %v", err)
	}
	p, err := f.handler.CreatePerson(ctx, command.CreatePersonInput{GivenName: "William", Surname: "King"})
	if err != nil {
		t.Fatalf("CreatePerson P failed: %v", err)
	}
	branch, err := f.handler.CreateBranch(ctx, "partner-resume", "")
	if err != nil {
		t.Fatalf("CreateBranch failed: %v", err)
	}
	onBranch := f.handler.WithBranch(branch)
	surname := "Byron"
	if _, err := onBranch.UpdatePerson(ctx, command.UpdatePersonInput{ID: a.ID, Surname: &surname, Version: a.Version}); err != nil {
		t.Fatalf("branch UpdatePerson failed: %v", err)
	}
	family, err := onBranch.CreateFamily(ctx, command.CreateFamilyInput{Partner1ID: &p.ID})
	if err != nil {
		t.Fatalf("branch CreateFamily failed: %v", err)
	}

	faulty.armed, faulty.failAt = true, 2
	_, err = f.handler.MergeBranch(ctx, command.MergeBranchInput{BranchID: branch.ID})
	faulty.armed = false
	if !errors.Is(err, command.ErrMergePartiallyApplied) {
		t.Fatalf("MergeBranch error = %v, want ErrMergePartiallyApplied", err)
	}
	deletePersonOnMain(t, f, p.ID)

	result, err := f.handler.ResumeMerge(ctx, command.ResumeMergeInput{BranchID: branch.ID})
	if !errors.Is(err, command.ErrMergeResumeNeedsResolution) {
		t.Fatalf("bare ResumeMerge error = %v, want ErrMergeResumeNeedsResolution", err)
	}
	if !slices.Equal(result.PendingStreamIDs, []uuid.UUID{family.ID}) {
		t.Errorf("PendingStreamIDs = %v, want the family [%s]", result.PendingStreamIDs, family.ID)
	}

	_, err = f.handler.ResumeMerge(ctx, command.ResumeMergeInput{
		BranchID:    branch.ID,
		Resolutions: map[uuid.UUID]command.MergeResolution{family.ID: command.ResolveBranch},
	})
	if !errors.Is(err, command.ErrMergeDanglingReference) {
		t.Fatalf("branch-resolved ResumeMerge error = %v, want ErrMergeDanglingReference", err)
	}

	if _, err := f.handler.ResumeMerge(ctx, command.ResumeMergeInput{
		BranchID:    branch.ID,
		Resolutions: map[uuid.UUID]command.MergeResolution{family.ID: command.ResolveMain},
	}); err != nil {
		t.Fatalf("main-resolved ResumeMerge failed: %v", err)
	}
	row, err := f.readStore.GetFamily(ctx, domain.MainBranchID, family.ID)
	if err != nil {
		t.Fatalf("main GetFamily failed: %v", err)
	}
	if row != nil {
		t.Errorf("main has family %+v, want it left out", row)
	}
}
