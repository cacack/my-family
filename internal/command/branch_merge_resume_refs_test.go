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

// danglingSeed is a branch that edits person A and links main's person P into
// main's family F (partner A). Its merge is interrupted after A's stream
// lands, so F — whose link names P — is still to replay.
type danglingSeed struct {
	f      *branchFixture
	faulty *faultyReplayStore
	branch *domain.Branch
	a      uuid.UUID
	p      uuid.UUID
	family uuid.UUID
}

func seedDanglingResume(t *testing.T) danglingSeed {
	t.Helper()
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
	p, err := f.handler.CreatePerson(ctx, command.CreatePersonInput{GivenName: "Byron", Surname: "King"})
	if err != nil {
		t.Fatalf("CreatePerson P failed: %v", err)
	}
	family, err := f.handler.CreateFamily(ctx, command.CreateFamilyInput{Partner1ID: &a.ID})
	if err != nil {
		t.Fatalf("CreateFamily failed: %v", err)
	}
	branch, err := f.handler.CreateBranch(ctx, "dangling", "")
	if err != nil {
		t.Fatalf("CreateBranch failed: %v", err)
	}
	onBranch := f.handler.WithBranch(branch)
	surname := "Byron"
	if _, err := onBranch.UpdatePerson(ctx, command.UpdatePersonInput{ID: a.ID, Surname: &surname, Version: a.Version}); err != nil {
		t.Fatalf("branch UpdatePerson failed: %v", err)
	}
	if _, err := onBranch.LinkChild(ctx, command.LinkChildInput{FamilyID: family.ID, ChildID: p.ID}); err != nil {
		t.Fatalf("branch LinkChild failed: %v", err)
	}

	faulty.armed, faulty.failAt = true, 2
	_, err = f.handler.MergeBranch(ctx, command.MergeBranchInput{BranchID: branch.ID})
	faulty.armed = false
	if !errors.Is(err, command.ErrMergePartiallyApplied) {
		t.Fatalf("MergeBranch error = %v, want ErrMergePartiallyApplied", err)
	}
	if got := mainSurnameOf(t, f, a.ID); got != "Byron" {
		t.Fatalf("A's surname on main = %q, want A's stream to have landed", got)
	}
	return danglingSeed{f: f, faulty: faulty, branch: branch, a: a.ID, p: p.ID, family: family.ID}
}

// TestResumeMerge_DanglingAutoPlannedStreamCanBeRolledForward is the
// regression test for a resume that could never finish: after the
// interruption main removed P (a delete, or a merge into another person),
// which it may because P is not linked on main. F is not stale — main never
// wrote to it — so the recorded plan would replay it automatically, and the
// dangling-reference check refused that replay on every attempt while
// refusing any resolution for F as "already decided". F must instead be
// reported pending, so the caller can roll the merge forward without it.
func TestResumeMerge_DanglingAutoPlannedStreamCanBeRolledForward(t *testing.T) {
	ctx := context.Background()
	removals := map[string]func(t *testing.T, s danglingSeed){
		"deleted on main": func(t *testing.T, s danglingSeed) {
			row, err := s.f.readStore.GetPerson(ctx, domain.MainBranchID, s.p)
			if err != nil || row == nil {
				t.Fatalf("main GetPerson P = %v (err %v)", row, err)
			}
			if err := s.f.handler.DeletePerson(ctx, command.DeletePersonInput{ID: s.p, Version: row.Version}); err != nil {
				t.Fatalf("main DeletePerson failed: %v", err)
			}
		},
		"merged away on main": func(t *testing.T, s danglingSeed) {
			survivor, err := s.f.handler.CreatePerson(ctx, command.CreatePersonInput{GivenName: "Byron", Surname: "Noel"})
			if err != nil {
				t.Fatalf("CreatePerson failed: %v", err)
			}
			row, err := s.f.readStore.GetPerson(ctx, domain.MainBranchID, s.p)
			if err != nil || row == nil {
				t.Fatalf("main GetPerson P = %v (err %v)", row, err)
			}
			if _, err := s.f.handler.MergePersons(ctx, command.MergePersonsInput{
				SurvivorID: survivor.ID, MergedID: s.p,
				SurvivorVersion: survivor.Version, MergedVersion: row.Version,
			}); err != nil {
				t.Fatalf("MergePersons failed: %v", err)
			}
		},
	}

	for name, remove := range removals {
		t.Run(name, func(t *testing.T) {
			s := seedDanglingResume(t)
			remove(t, s)

			result, err := s.f.handler.ResumeMerge(ctx, command.ResumeMergeInput{BranchID: s.branch.ID})
			if !errors.Is(err, command.ErrMergeResumeNeedsResolution) {
				t.Fatalf("bare ResumeMerge error = %v, want ErrMergeResumeNeedsResolution", err)
			}
			if !slices.Equal(result.PendingStreamIDs, []uuid.UUID{s.family}) {
				t.Errorf("PendingStreamIDs = %v, want the family [%s]", result.PendingStreamIDs, s.family)
			}

			// Replaying F anyway is still a dangling reference, and records nothing.
			_, err = s.f.handler.ResumeMerge(ctx, command.ResumeMergeInput{
				BranchID:    s.branch.ID,
				Resolutions: map[uuid.UUID]command.MergeResolution{s.family: command.ResolveBranch},
			})
			if !errors.Is(err, command.ErrMergeDanglingReference) {
				t.Fatalf("branch-resolved ResumeMerge error = %v, want ErrMergeDanglingReference", err)
			}
			if records := branchResumeRecords(t, s.f, s.branch); len(records) != 0 {
				t.Errorf("a refused resume recorded %d decision(s), want none", len(records))
			}

			// Rolling forward without F finishes the merge.
			result, err = s.f.handler.ResumeMerge(ctx, command.ResumeMergeInput{
				BranchID:    s.branch.ID,
				Resolutions: map[uuid.UUID]command.MergeResolution{s.family: command.ResolveMain},
			})
			if err != nil {
				t.Fatalf("main-resolved ResumeMerge failed: %v", err)
			}
			if !slices.Equal(result.SkippedStreamIDs, []uuid.UUID{s.family}) {
				t.Errorf("SkippedStreamIDs = %v, want [%s]", result.SkippedStreamIDs, s.family)
			}
			if !slices.Equal(result.AlreadyReplayedStreamIDs, []uuid.UUID{s.a}) {
				t.Errorf("AlreadyReplayedStreamIDs = %v, want [%s]", result.AlreadyReplayedStreamIDs, s.a)
			}
			if result.ReplayedEventCount != 0 {
				t.Errorf("ReplayedEventCount = %d, want 0", result.ReplayedEventCount)
			}
			children, err := s.f.readStore.GetFamilyChildren(ctx, domain.MainBranchID, s.family)
			if err != nil {
				t.Fatalf("GetFamilyChildren failed: %v", err)
			}
			if len(children) != 0 {
				t.Errorf("main family children = %v, want no phantom child", children)
			}
			records := branchResumeRecords(t, s.f, s.branch)
			if len(records) != 1 || records[0].Resolutions[s.family] != string(command.ResolveMain) {
				t.Fatalf("resume records = %+v, want one recording %s as main", records, s.family)
			}
			if _, planned := records[0].ReplayStreamVersions[s.family]; planned {
				t.Errorf("recorded plan still replays %s", s.family)
			}

			// Done: a bare resume is a no-op, and F can no longer be re-decided.
			again, err := s.f.handler.ResumeMerge(ctx, command.ResumeMergeInput{BranchID: s.branch.ID})
			if err != nil {
				t.Fatalf("bare ResumeMerge after roll-forward failed: %v", err)
			}
			if again.ReplayedEventCount != 0 || !slices.Equal(again.SkippedStreamIDs, []uuid.UUID{s.family}) {
				t.Errorf("second resume replayed %d, skipped %v; want a no-op skipping %s",
					again.ReplayedEventCount, again.SkippedStreamIDs, s.family)
			}
			_, err = s.f.handler.ResumeMerge(ctx, command.ResumeMergeInput{
				BranchID:    s.branch.ID,
				Resolutions: map[uuid.UUID]command.MergeResolution{s.family: command.ResolveBranch},
			})
			if !errors.Is(err, command.ErrUnknownResolution) {
				t.Errorf("re-deciding %s error = %v, want ErrUnknownResolution", s.family, err)
			}
		})
	}
}

// TestResumeMerge_DanglingCheckFollowsThisCallsMainResolution: the branch
// edits P as well as linking them, and main then deletes P, so P's own stream
// is stale and pending. Replaying P's edits would not bring P back, so F —
// whose link names P — is pending too, and "branch" is refused for P: it
// would append P's edits after main's delete and restore nothing. Nothing is
// recorded until both are decided.
func TestResumeMerge_DanglingCheckFollowsThisCallsMainResolution(t *testing.T) {
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
	p, err := f.handler.CreatePerson(ctx, command.CreatePersonInput{GivenName: "Byron", Surname: "King"})
	if err != nil {
		t.Fatalf("CreatePerson P failed: %v", err)
	}
	family, err := f.handler.CreateFamily(ctx, command.CreateFamilyInput{Partner1ID: &a.ID})
	if err != nil {
		t.Fatalf("CreateFamily failed: %v", err)
	}
	branch, err := f.handler.CreateBranch(ctx, "dangling-pending", "")
	if err != nil {
		t.Fatalf("CreateBranch failed: %v", err)
	}
	onBranch := f.handler.WithBranch(branch)
	surname := "Byron"
	if _, err := onBranch.UpdatePerson(ctx, command.UpdatePersonInput{ID: a.ID, Surname: &surname, Version: a.Version}); err != nil {
		t.Fatalf("branch UpdatePerson A failed: %v", err)
	}
	given := "George"
	if _, err := onBranch.UpdatePerson(ctx, command.UpdatePersonInput{ID: p.ID, GivenName: &given, Version: p.Version}); err != nil {
		t.Fatalf("branch UpdatePerson P failed: %v", err)
	}
	if _, err := onBranch.LinkChild(ctx, command.LinkChildInput{FamilyID: family.ID, ChildID: p.ID}); err != nil {
		t.Fatalf("branch LinkChild failed: %v", err)
	}

	faulty.armed, faulty.failAt = true, 2
	_, err = f.handler.MergeBranch(ctx, command.MergeBranchInput{BranchID: branch.ID})
	faulty.armed = false
	if !errors.Is(err, command.ErrMergePartiallyApplied) {
		t.Fatalf("MergeBranch error = %v, want ErrMergePartiallyApplied", err)
	}
	if err := f.handler.DeletePerson(ctx, command.DeletePersonInput{ID: p.ID, Version: p.Version}); err != nil {
		t.Fatalf("main DeletePerson failed: %v", err)
	}

	result, err := f.handler.ResumeMerge(ctx, command.ResumeMergeInput{BranchID: branch.ID})
	if !errors.Is(err, command.ErrMergeResumeNeedsResolution) {
		t.Fatalf("bare ResumeMerge error = %v, want ErrMergeResumeNeedsResolution", err)
	}
	if !slices.Equal(result.PendingStreamIDs, []uuid.UUID{p.ID, family.ID}) {
		t.Errorf("PendingStreamIDs = %v, want the deleted person and the family linking them [%s %s]",
			result.PendingStreamIDs, p.ID, family.ID)
	}

	_, err = f.handler.ResumeMerge(ctx, command.ResumeMergeInput{
		BranchID:    branch.ID,
		Resolutions: map[uuid.UUID]command.MergeResolution{p.ID: command.ResolveBranch},
	})
	if !errors.Is(err, command.ErrUnknownResolution) {
		t.Fatalf("branch resolution over main's delete: error = %v, want ErrUnknownResolution", err)
	}

	result, err = f.handler.ResumeMerge(ctx, command.ResumeMergeInput{
		BranchID:    branch.ID,
		Resolutions: map[uuid.UUID]command.MergeResolution{p.ID: command.ResolveMain},
	})
	if !errors.Is(err, command.ErrMergeResumeNeedsResolution) {
		t.Fatalf("P-only ResumeMerge error = %v, want ErrMergeResumeNeedsResolution", err)
	}
	if !slices.Equal(result.PendingStreamIDs, []uuid.UUID{family.ID}) {
		t.Errorf("PendingStreamIDs = %v, want the family [%s]", result.PendingStreamIDs, family.ID)
	}
	if records := branchResumeRecords(t, f, branch); len(records) != 0 {
		t.Errorf("a refused resume recorded %d decision(s), want none", len(records))
	}

	if _, err := f.handler.ResumeMerge(ctx, command.ResumeMergeInput{
		BranchID: branch.ID,
		Resolutions: map[uuid.UUID]command.MergeResolution{
			p.ID: command.ResolveMain, family.ID: command.ResolveMain,
		},
	}); err != nil {
		t.Fatalf("fully resolved ResumeMerge failed: %v", err)
	}
	children, err := f.readStore.GetFamilyChildren(ctx, domain.MainBranchID, family.ID)
	if err != nil {
		t.Fatalf("GetFamilyChildren failed: %v", err)
	}
	if len(children) != 0 {
		t.Errorf("main family children = %v, want no phantom child", children)
	}
	assertNoMainEventsAfter(t, f, p.ID, "PersonDeleted")
}

// seedEditedAndLinked is a branch that edits main's person P and links P into
// main's family F (partner A), merged with the replay failing at its first
// append, so none of the branch's streams reached main.
func seedEditedAndLinked(t *testing.T) (f *branchFixture, branch *domain.Branch, p, family uuid.UUID) {
	t.Helper()
	var faulty *faultyReplayStore
	f = newBranchFixtureWith(branchFixtureDeps{
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
	person, err := f.handler.CreatePerson(ctx, command.CreatePersonInput{GivenName: "Byron", Surname: "King"})
	if err != nil {
		t.Fatalf("CreatePerson P failed: %v", err)
	}
	fam, err := f.handler.CreateFamily(ctx, command.CreateFamilyInput{Partner1ID: &a.ID})
	if err != nil {
		t.Fatalf("CreateFamily failed: %v", err)
	}
	branch, err = f.handler.CreateBranch(ctx, "edited-and-linked", "")
	if err != nil {
		t.Fatalf("CreateBranch failed: %v", err)
	}
	onBranch := f.handler.WithBranch(branch)
	surname := "Noel"
	if _, err := onBranch.UpdatePerson(ctx, command.UpdatePersonInput{ID: person.ID, Surname: &surname, Version: person.Version}); err != nil {
		t.Fatalf("branch UpdatePerson P failed: %v", err)
	}
	if _, err := onBranch.LinkChild(ctx, command.LinkChildInput{FamilyID: fam.ID, ChildID: person.ID}); err != nil {
		t.Fatalf("branch LinkChild failed: %v", err)
	}

	faulty.armed, faulty.failAt = true, 1
	_, err = f.handler.MergeBranch(ctx, command.MergeBranchInput{BranchID: branch.ID})
	faulty.armed = false
	if !errors.Is(err, command.ErrMergePartiallyApplied) {
		t.Fatalf("MergeBranch error = %v, want ErrMergePartiallyApplied", err)
	}
	return f, branch, person.ID, fam.ID
}

// mergeAwayOnMain merges person p into a new person on main. PersonMerged is
// written to the survivor's stream, so p's own stream keeps its version.
func mergeAwayOnMain(t *testing.T, f *branchFixture, p uuid.UUID) {
	t.Helper()
	ctx := context.Background()
	survivor, err := f.handler.CreatePerson(ctx, command.CreatePersonInput{GivenName: "Byron", Surname: "Survivor"})
	if err != nil {
		t.Fatalf("CreatePerson survivor failed: %v", err)
	}
	row, err := f.readStore.GetPerson(ctx, domain.MainBranchID, p)
	if err != nil || row == nil {
		t.Fatalf("main GetPerson P = %v (err %v)", row, err)
	}
	if _, err := f.handler.MergePersons(ctx, command.MergePersonsInput{
		SurvivorID: survivor.ID, MergedID: p,
		SurvivorVersion: survivor.Version, MergedVersion: row.Version,
	}); err != nil {
		t.Fatalf("MergePersons failed: %v", err)
	}
}

// TestResumeMerge_PersonMergedAwayWhileEditedAndLinked is the regression test
// for a phantom child a resume reported as success. The branch edits P and
// links P into F; main merges P away after the interruption. P's stream stays
// at its pinned version (PersonMerged lands on the survivor's), so the plan
// would replay P's edit and F's link automatically — and the edit restores
// nothing, leaving F linking a person main does not have. Both must be
// pending, and "branch" is refused for P.
func TestResumeMerge_PersonMergedAwayWhileEditedAndLinked(t *testing.T) {
	ctx := context.Background()
	f, branch, p, family := seedEditedAndLinked(t)
	pinned, err := f.eventStore.GetStreamVersion(ctx, p, domain.MainBranchID)
	if err != nil {
		t.Fatalf("GetStreamVersion failed: %v", err)
	}
	mergeAwayOnMain(t, f, p)
	if after, err := f.eventStore.GetStreamVersion(ctx, p, domain.MainBranchID); err != nil || after != pinned {
		t.Fatalf("P's main stream version = %d (err %v), want it unchanged at %d by the merge-away", after, err, pinned)
	}

	result, err := f.handler.ResumeMerge(ctx, command.ResumeMergeInput{BranchID: branch.ID})
	if !errors.Is(err, command.ErrMergeResumeNeedsResolution) {
		t.Fatalf("bare ResumeMerge error = %v, want ErrMergeResumeNeedsResolution", err)
	}
	if !slices.Equal(result.PendingStreamIDs, []uuid.UUID{p, family}) {
		t.Errorf("PendingStreamIDs = %v, want [%s %s]", result.PendingStreamIDs, p, family)
	}

	for name, resolutions := range map[string]map[uuid.UUID]command.MergeResolution{
		"P to branch": {p: command.ResolveBranch, family: command.ResolveMain},
		"both branch": {p: command.ResolveBranch, family: command.ResolveBranch},
	} {
		_, err = f.handler.ResumeMerge(ctx, command.ResumeMergeInput{BranchID: branch.ID, Resolutions: resolutions})
		if !errors.Is(err, command.ErrUnknownResolution) {
			t.Errorf("%s: error = %v, want ErrUnknownResolution", name, err)
		}
	}
	_, err = f.handler.ResumeMerge(ctx, command.ResumeMergeInput{
		BranchID:    branch.ID,
		Resolutions: map[uuid.UUID]command.MergeResolution{p: command.ResolveMain, family: command.ResolveBranch},
	})
	if !errors.Is(err, command.ErrMergeDanglingReference) {
		t.Errorf("F to branch: error = %v, want ErrMergeDanglingReference", err)
	}
	if records := branchResumeRecords(t, f, branch); len(records) != 0 {
		t.Fatalf("refused resumes recorded %d decision(s), want none", len(records))
	}

	result, err = f.handler.ResumeMerge(ctx, command.ResumeMergeInput{
		BranchID:    branch.ID,
		Resolutions: map[uuid.UUID]command.MergeResolution{p: command.ResolveMain, family: command.ResolveMain},
	})
	if err != nil {
		t.Fatalf("main-resolved ResumeMerge failed: %v", err)
	}
	if result.ReplayedEventCount != 0 {
		t.Errorf("ReplayedEventCount = %d, want 0", result.ReplayedEventCount)
	}
	assertNoPhantomChild(t, f, family)
	if after, err := f.eventStore.GetStreamVersion(ctx, p, domain.MainBranchID); err != nil || after != pinned {
		t.Errorf("P's main stream version = %d (err %v), want no edit appended after the merge-away", after, err)
	}
}

// TestResumeMerge_LandedCreatedPersonRemovedOnMain: the branch creates P and
// links P into main's family F. P's stream reaches main before the
// interruption; main then deletes P. P's replay is on main, but it no longer
// vouches for P, so F's link — which the plan would replay automatically —
// must be pending rather than written.
func TestResumeMerge_LandedCreatedPersonRemovedOnMain(t *testing.T) {
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
		t.Fatalf("CreatePerson failed: %v", err)
	}
	family, err := f.handler.CreateFamily(ctx, command.CreateFamilyInput{Partner1ID: &a.ID})
	if err != nil {
		t.Fatalf("CreateFamily failed: %v", err)
	}
	branch, err := f.handler.CreateBranch(ctx, "created-then-deleted", "")
	if err != nil {
		t.Fatalf("CreateBranch failed: %v", err)
	}
	onBranch := f.handler.WithBranch(branch)
	child, err := onBranch.CreatePerson(ctx, command.CreatePersonInput{GivenName: "Byron", Surname: "King"})
	if err != nil {
		t.Fatalf("branch CreatePerson failed: %v", err)
	}
	if _, err := onBranch.LinkChild(ctx, command.LinkChildInput{FamilyID: family.ID, ChildID: child.ID}); err != nil {
		t.Fatalf("branch LinkChild failed: %v", err)
	}

	faulty.armed, faulty.failAt = true, 2
	_, err = f.handler.MergeBranch(ctx, command.MergeBranchInput{BranchID: branch.ID})
	faulty.armed = false
	if !errors.Is(err, command.ErrMergePartiallyApplied) {
		t.Fatalf("MergeBranch error = %v, want ErrMergePartiallyApplied", err)
	}
	row, err := f.readStore.GetPerson(ctx, domain.MainBranchID, child.ID)
	if err != nil || row == nil {
		t.Fatalf("main GetPerson = %v (err %v), want the child's stream landed", row, err)
	}
	if err := f.handler.DeletePerson(ctx, command.DeletePersonInput{ID: child.ID, Version: row.Version}); err != nil {
		t.Fatalf("main DeletePerson failed: %v", err)
	}

	result, err := f.handler.ResumeMerge(ctx, command.ResumeMergeInput{BranchID: branch.ID})
	if !errors.Is(err, command.ErrMergeResumeNeedsResolution) {
		t.Fatalf("bare ResumeMerge error = %v, want ErrMergeResumeNeedsResolution", err)
	}
	if !slices.Equal(result.PendingStreamIDs, []uuid.UUID{family.ID}) {
		t.Errorf("PendingStreamIDs = %v, want [%s]", result.PendingStreamIDs, family.ID)
	}
	if _, err := f.handler.ResumeMerge(ctx, command.ResumeMergeInput{
		BranchID:    branch.ID,
		Resolutions: map[uuid.UUID]command.MergeResolution{family.ID: command.ResolveMain},
	}); err != nil {
		t.Fatalf("main-resolved ResumeMerge failed: %v", err)
	}
	assertNoPhantomChild(t, f, family.ID)
}

// TestMergeBranch_RefusesALinkToAPersonMergedAwayOnMain: main merged P away,
// which does not write to P's stream, so the branch's edit to P is no
// conflict — but replaying it restores nothing, so F's link to P would dangle.
// The merge refuses before claiming; resolving F (and P) to main merges.
func TestMergeBranch_RefusesALinkToAPersonMergedAwayOnMain(t *testing.T) {
	f := newBranchFixture()
	ctx := context.Background()

	a, err := f.handler.CreatePerson(ctx, command.CreatePersonInput{GivenName: "Ada", Surname: "Lovelace"})
	if err != nil {
		t.Fatalf("CreatePerson A failed: %v", err)
	}
	p, err := f.handler.CreatePerson(ctx, command.CreatePersonInput{GivenName: "Byron", Surname: "King"})
	if err != nil {
		t.Fatalf("CreatePerson P failed: %v", err)
	}
	family, err := f.handler.CreateFamily(ctx, command.CreateFamilyInput{Partner1ID: &a.ID})
	if err != nil {
		t.Fatalf("CreateFamily failed: %v", err)
	}
	branch, err := f.handler.CreateBranch(ctx, "merged-away", "")
	if err != nil {
		t.Fatalf("CreateBranch failed: %v", err)
	}
	onBranch := f.handler.WithBranch(branch)
	surname := "Noel"
	if _, err := onBranch.UpdatePerson(ctx, command.UpdatePersonInput{ID: p.ID, Surname: &surname, Version: p.Version}); err != nil {
		t.Fatalf("branch UpdatePerson failed: %v", err)
	}
	if _, err := onBranch.LinkChild(ctx, command.LinkChildInput{FamilyID: family.ID, ChildID: p.ID}); err != nil {
		t.Fatalf("branch LinkChild failed: %v", err)
	}
	mergeAwayOnMain(t, f, p.ID)

	_, err = f.handler.MergeBranch(ctx, command.MergeBranchInput{BranchID: branch.ID})
	if !errors.Is(err, command.ErrMergeDanglingReference) {
		t.Fatalf("MergeBranch error = %v, want ErrMergeDanglingReference", err)
	}
	if got, err := f.branchStore.Get(ctx, branch.ID); err != nil || got.Status != domain.BranchStatusActive {
		t.Fatalf("branch = %v (err %v), want it still active (refused before the claim)", got, err)
	}

	if _, err := f.handler.MergeBranch(ctx, command.MergeBranchInput{
		BranchID:    branch.ID,
		Resolutions: map[uuid.UUID]command.MergeResolution{p.ID: command.ResolveMain, family.ID: command.ResolveMain},
	}); err != nil {
		t.Fatalf("main-resolved MergeBranch failed: %v", err)
	}
	assertNoPhantomChild(t, f, family.ID)
}

// assertNoPhantomChild checks every child main's family has is a person main
// has, and that the family's child count agrees.
func assertNoPhantomChild(t *testing.T, f *branchFixture, familyID uuid.UUID) {
	t.Helper()
	ctx := context.Background()
	children, err := f.readStore.GetFamilyChildren(ctx, domain.MainBranchID, familyID)
	if err != nil {
		t.Fatalf("GetFamilyChildren failed: %v", err)
	}
	for _, child := range children {
		person, err := f.readStore.GetPerson(ctx, domain.MainBranchID, child.PersonID)
		if err != nil {
			t.Fatalf("GetPerson failed: %v", err)
		}
		if person == nil {
			t.Errorf("main family %s links child %s, whom main does not have", familyID, child.PersonID)
		}
	}
	row, err := f.readStore.GetFamily(ctx, domain.MainBranchID, familyID)
	if err != nil || row == nil {
		t.Fatalf("main GetFamily = %v (err %v)", row, err)
	}
	if row.ChildCount != len(children) {
		t.Errorf("family child count = %d, want %d", row.ChildCount, len(children))
	}
}

// assertNoMainEventsAfter checks nothing was appended to a stream on main
// after its last event of the given type.
func assertNoMainEventsAfter(t *testing.T, f *branchFixture, streamID uuid.UUID, eventType string) {
	t.Helper()
	stored, err := f.eventStore.ReadStream(context.Background(), streamID)
	if err != nil {
		t.Fatalf("ReadStream failed: %v", err)
	}
	var mainEvents []repository.StoredEvent
	for i := range stored {
		if stored[i].BranchID == domain.MainBranchID {
			mainEvents = append(mainEvents, stored[i])
		}
	}
	if len(mainEvents) == 0 || mainEvents[len(mainEvents)-1].EventType != eventType {
		t.Errorf("main's events on %s = %d, want the last to be %s", streamID, len(mainEvents), eventType)
	}
}

// TestResumeMerge_LegacyClaimRefusesExcludingALandedLinksChild is the
// regression test for a phantom child left by a legacy (pre-#685) claim. The
// branch creates family F and person P and links P into F; F reached main
// before the interruption, P did not. Resolving P to main would leave F's
// link on main pointing at nobody, so it is refused; "branch" completes it.
func TestResumeMerge_LegacyClaimRefusesExcludingALandedLinksChild(t *testing.T) {
	f := newBranchFixture()
	ctx := context.Background()

	parent, err := f.handler.CreatePerson(ctx, command.CreatePersonInput{GivenName: "Ada", Surname: "Lovelace"})
	if err != nil {
		t.Fatalf("CreatePerson failed: %v", err)
	}
	branch, err := f.handler.CreateBranch(ctx, "legacy-child", "")
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
		t.Fatalf("branch CreatePerson failed: %v", err)
	}
	if _, err := onBranch.LinkChild(ctx, command.LinkChildInput{FamilyID: family.ID, ChildID: child.ID}); err != nil {
		t.Fatalf("branch LinkChild failed: %v", err)
	}

	// A pre-#685 claim (no recorded plan), then F's stream replayed by hand the
	// way that merge would have, before it was interrupted.
	claim := domain.BranchMerged{
		BaseEvent:        domain.NewBaseEvent(),
		BranchID:         branch.ID,
		BasePosition:     branch.BasePosition,
		MergedAtPosition: logHead(t, f),
	}
	scope := repository.AppendScope{BranchID: domain.BranchID(branch.ID), BasePosition: branch.BasePosition}
	version, err := f.eventStore.GetStreamVersion(ctx, branch.ID, scope.BranchID)
	if err != nil {
		t.Fatalf("GetStreamVersion failed: %v", err)
	}
	if err := f.eventStore.Append(ctx, branch.ID, "branch", []domain.Event{claim}, version, scope); err != nil {
		t.Fatalf("appending legacy claim failed: %v", err)
	}
	var familyEvents []domain.Event
	for _, stored := range branchEventsFor(t, f, family.ID, domain.BranchID(branch.ID)) {
		decoded, err := stored.DecodeEvent()
		if err != nil {
			t.Fatalf("DecodeEvent failed: %v", err)
		}
		familyEvents = append(familyEvents, decoded)
	}
	if err := f.eventStore.Append(ctx, family.ID, "family", familyEvents, 0, repository.MainScope); err != nil {
		t.Fatalf("replaying the family by hand failed: %v", err)
	}

	result, err := f.handler.ResumeMerge(ctx, command.ResumeMergeInput{BranchID: branch.ID})
	if !errors.Is(err, command.ErrMergeResumeNeedsResolution) {
		t.Fatalf("bare ResumeMerge error = %v, want ErrMergeResumeNeedsResolution", err)
	}
	if !slices.Equal(result.PendingStreamIDs, []uuid.UUID{child.ID}) {
		t.Fatalf("PendingStreamIDs = %v, want [%s]", result.PendingStreamIDs, child.ID)
	}

	_, err = f.handler.ResumeMerge(ctx, command.ResumeMergeInput{
		BranchID:    branch.ID,
		Resolutions: map[uuid.UUID]command.MergeResolution{child.ID: command.ResolveMain},
	})
	if !errors.Is(err, command.ErrMergeDanglingReference) {
		t.Fatalf("main-resolved ResumeMerge error = %v, want ErrMergeDanglingReference", err)
	}
	if records := branchResumeRecords(t, f, branch); len(records) != 0 {
		t.Errorf("the refused resume recorded %d decision(s), want none", len(records))
	}

	if _, err := f.handler.ResumeMerge(ctx, command.ResumeMergeInput{
		BranchID:    branch.ID,
		Resolutions: map[uuid.UUID]command.MergeResolution{child.ID: command.ResolveBranch},
	}); err != nil {
		t.Fatalf("branch-resolved ResumeMerge failed: %v", err)
	}
	if person, err := f.readStore.GetPerson(ctx, domain.MainBranchID, child.ID); err != nil || person == nil {
		t.Fatalf("child on main = %v (err %v), want it replayed", person, err)
	}
	children, err := f.readStore.GetFamilyChildren(ctx, domain.MainBranchID, family.ID)
	if err != nil {
		t.Fatalf("GetFamilyChildren failed: %v", err)
	}
	if len(children) != 1 || children[0].PersonID != child.ID {
		t.Errorf("main family children = %v, want the link to the replayed child", children)
	}
}

// branchResumeRecords returns the BranchMergeResumed decision records on a
// branch's own stream.
func branchResumeRecords(t *testing.T, f *branchFixture, branch *domain.Branch) []domain.BranchMergeResumed {
	t.Helper()
	return resumeRecords(t, resumeSeed{f: f, branch: branch})
}

// TestResumeMerge_RepairNeverLowersARow: two resumes find the same person row
// behind the log, and a user edit lands after the rival resume repairs it but
// before this resume re-runs the events it read. Re-running them must not
// roll the row back to an older version (and older fields).
func TestResumeMerge_RepairNeverLowersARow(t *testing.T) {
	s := seedResume(t)
	ctx := context.Background()

	s.reads.armed, s.reads.failPerson = true, s.second
	_, err := s.handler.MergeBranch(ctx, command.MergeBranchInput{BranchID: s.branch.ID})
	s.reads.armed = false
	if !errors.Is(err, command.ErrMergePartiallyApplied) {
		t.Fatalf("MergeBranch error = %v, want ErrMergePartiallyApplied", err)
	}

	// The first main scan finds the landed streams; the second is the repair's
	// read of the second person's events, taken after the row was found behind.
	s.faulty.skipMainScans = 1
	s.faulty.afterMainScan = func() {
		if _, err := s.handler.ResumeMerge(ctx, command.ResumeMergeInput{BranchID: s.branch.ID}); err != nil {
			t.Errorf("rival ResumeMerge failed: %v", err)
		}
		mainUpdate(t, s.f, s.second, "Augusta")
	}
	result, err := s.handler.ResumeMerge(ctx, command.ResumeMergeInput{BranchID: s.branch.ID})
	if err != nil {
		t.Fatalf("ResumeMerge failed: %v", err)
	}
	if s.faulty.afterMainScan != nil {
		t.Fatal("the rival resume never ran; the race was not exercised")
	}
	if !slices.Equal(result.ReprojectedStreamIDs, []uuid.UUID{s.second}) {
		t.Errorf("ReprojectedStreamIDs = %v, want [%s]", result.ReprojectedStreamIDs, s.second)
	}
	assertPersonLevelWithLog(t, s.f, s.second, "Augusta", "Byron")
}

// TestResumeMerge_RepairConvergesAfterARacingWrite: a rival repair and a user
// edit commit between this repair's check of the row and its save, so the
// save rolls the row back.
// The repair must notice the row behind the log and finish it rather than
// report success over it.
func TestResumeMerge_RepairConvergesAfterARacingWrite(t *testing.T) {
	s := seedResume(t)
	ctx := context.Background()

	s.reads.armed, s.reads.failPerson = true, s.second
	_, err := s.handler.MergeBranch(ctx, command.MergeBranchInput{BranchID: s.branch.ID})
	s.reads.armed = false
	if !errors.Is(err, command.ErrMergePartiallyApplied) {
		t.Fatalf("MergeBranch error = %v, want ErrMergePartiallyApplied", err)
	}

	// A mainline write needs the row level with the log (the command checks
	// the row's version), so a rival resume repairs it first — all between
	// this resume's check of the row and its save.
	s.reads.beforeMainSavePerson = func() {
		if _, err := s.handler.ResumeMerge(ctx, command.ResumeMergeInput{BranchID: s.branch.ID}); err != nil {
			t.Errorf("rival ResumeMerge failed: %v", err)
		}
		mainUpdate(t, s.f, s.second, "Augusta")
	}
	result, err := s.handler.ResumeMerge(ctx, command.ResumeMergeInput{BranchID: s.branch.ID})
	if err != nil {
		t.Fatalf("ResumeMerge failed: %v", err)
	}
	if s.reads.beforeMainSavePerson != nil {
		t.Fatal("the racing write never ran; the race was not exercised")
	}
	if !slices.Equal(result.ReprojectedStreamIDs, []uuid.UUID{s.second}) {
		t.Errorf("ReprojectedStreamIDs = %v, want [%s]", result.ReprojectedStreamIDs, s.second)
	}
	assertPersonLevelWithLog(t, s.f, s.second, "Augusta", "Byron")
}

// mainUpdate sets a person's given name on main at the log's current version.
func mainUpdate(t *testing.T, f *branchFixture, personID uuid.UUID, givenName string) {
	t.Helper()
	ctx := context.Background()
	version, err := f.eventStore.GetStreamVersion(ctx, personID, domain.MainBranchID)
	if err != nil {
		t.Fatalf("GetStreamVersion failed: %v", err)
	}
	if _, err := f.handler.UpdatePerson(ctx, command.UpdatePersonInput{ID: personID, GivenName: &givenName, Version: version}); err != nil {
		t.Fatalf("mainline UpdatePerson failed: %v", err)
	}
}

// assertPersonLevelWithLog checks a person's main row is at the log's version
// with the given names.
func assertPersonLevelWithLog(t *testing.T, f *branchFixture, personID uuid.UUID, givenName, surname string) {
	t.Helper()
	ctx := context.Background()
	row, err := f.readStore.GetPerson(ctx, domain.MainBranchID, personID)
	if err != nil || row == nil {
		t.Fatalf("main GetPerson = %v (err %v)", row, err)
	}
	logVersion, err := f.eventStore.GetStreamVersion(ctx, personID, domain.MainBranchID)
	if err != nil {
		t.Fatalf("GetStreamVersion failed: %v", err)
	}
	if row.Version != logVersion {
		t.Errorf("read-model version = %d, want the log's %d", row.Version, logVersion)
	}
	if row.GivenName != givenName || row.Surname != surname {
		t.Errorf("main row = %s %s, want %s %s", row.GivenName, row.Surname, givenName, surname)
	}
}

// associationSeed is a branch that associates two main persons, merged with
// the association's projection failing, so the association is on main's log
// but not its read model.
func associationSeed(t *testing.T) (*branchFixture, *domain.Branch, uuid.UUID, uuid.UUID) {
	t.Helper()
	var reads *faultyReadStore
	f := newBranchFixtureWith(branchFixtureDeps{
		wrapReads: func(inner repository.ReadModelStore) repository.ReadModelStore {
			reads = &faultyReadStore{ReadModelStore: inner}
			return reads
		},
	})
	ctx := context.Background()

	person, err := f.handler.CreatePerson(ctx, command.CreatePersonInput{GivenName: "Ada", Surname: "Lovelace"})
	if err != nil {
		t.Fatalf("CreatePerson failed: %v", err)
	}
	associate, err := f.handler.CreatePerson(ctx, command.CreatePersonInput{GivenName: "Charles", Surname: "Babbage"})
	if err != nil {
		t.Fatalf("CreatePerson failed: %v", err)
	}
	branch, err := f.handler.CreateBranch(ctx, "association", "")
	if err != nil {
		t.Fatalf("CreateBranch failed: %v", err)
	}
	association, err := f.handler.WithBranch(branch).CreateAssociation(ctx, command.CreateAssociationInput{
		PersonID: person.ID, AssociateID: associate.ID, Role: "friend",
	})
	if err != nil {
		t.Fatalf("branch CreateAssociation failed: %v", err)
	}

	reads.armed, reads.failAssociation = true, association.ID
	_, err = f.handler.MergeBranch(ctx, command.MergeBranchInput{BranchID: branch.ID})
	reads.armed = false
	if !errors.Is(err, command.ErrMergePartiallyApplied) || !errors.Is(err, errInjectedProjectionFailure) {
		t.Fatalf("MergeBranch error = %v, want the projection failure wrapped in ErrMergePartiallyApplied", err)
	}
	return f, branch, association.ID, associate.ID
}

// TestResumeMerge_RepairsAnAssociation: associations are branch-writable
// (#757), so a resume must be able to check and repair their main rows too.
func TestResumeMerge_RepairsAnAssociation(t *testing.T) {
	ctx := context.Background()

	t.Run("repaired", func(t *testing.T) {
		f, branch, associationID, _ := associationSeed(t)
		result, err := f.handler.ResumeMerge(ctx, command.ResumeMergeInput{BranchID: branch.ID})
		if err != nil {
			t.Fatalf("ResumeMerge failed: %v", err)
		}
		if !slices.Equal(result.ReprojectedStreamIDs, []uuid.UUID{associationID}) {
			t.Errorf("ReprojectedStreamIDs = %v, want [%s]", result.ReprojectedStreamIDs, associationID)
		}
		row, err := f.readStore.GetAssociation(ctx, domain.MainBranchID, associationID)
		if err != nil || row == nil {
			t.Fatalf("main GetAssociation = %v (err %v), want it repaired", row, err)
		}
		if row.Version != 1 {
			t.Errorf("association version = %d, want 1", row.Version)
		}
	})

	t.Run("a person deleted on main is not resurrected into it", func(t *testing.T) {
		f, branch, associationID, associateID := associationSeed(t)
		row, err := f.readStore.GetPerson(ctx, domain.MainBranchID, associateID)
		if err != nil || row == nil {
			t.Fatalf("main GetPerson = %v (err %v)", row, err)
		}
		if err := f.handler.DeletePerson(ctx, command.DeletePersonInput{ID: associateID, Version: row.Version}); err != nil {
			t.Fatalf("main DeletePerson failed: %v", err)
		}
		result, err := f.handler.ResumeMerge(ctx, command.ResumeMergeInput{BranchID: branch.ID})
		if err != nil {
			t.Fatalf("ResumeMerge failed: %v", err)
		}
		if len(result.ReprojectedStreamIDs) != 0 {
			t.Errorf("ReprojectedStreamIDs = %v, want none", result.ReprojectedStreamIDs)
		}
		if got, err := f.readStore.GetAssociation(ctx, domain.MainBranchID, associationID); err != nil || got != nil {
			t.Errorf("association on main = %v (err %v), want it to stay gone with its person", got, err)
		}
	})
}

// TestMergeBranch_RefusesAnAssociationToADeletedPerson: an association the
// branch created names a person main deleted since. Replaying it would leave
// main a phantom association, so the merge refuses before claiming.
func TestMergeBranch_RefusesAnAssociationToADeletedPerson(t *testing.T) {
	f := newBranchFixture()
	ctx := context.Background()

	person, err := f.handler.CreatePerson(ctx, command.CreatePersonInput{GivenName: "Ada", Surname: "Lovelace"})
	if err != nil {
		t.Fatalf("CreatePerson failed: %v", err)
	}
	associate, err := f.handler.CreatePerson(ctx, command.CreatePersonInput{GivenName: "Charles", Surname: "Babbage"})
	if err != nil {
		t.Fatalf("CreatePerson failed: %v", err)
	}
	branch, err := f.handler.CreateBranch(ctx, "association", "")
	if err != nil {
		t.Fatalf("CreateBranch failed: %v", err)
	}
	if _, err := f.handler.WithBranch(branch).CreateAssociation(ctx, command.CreateAssociationInput{
		PersonID: person.ID, AssociateID: associate.ID, Role: "friend",
	}); err != nil {
		t.Fatalf("branch CreateAssociation failed: %v", err)
	}
	if err := f.handler.DeletePerson(ctx, command.DeletePersonInput{ID: associate.ID, Version: associate.Version}); err != nil {
		t.Fatalf("main DeletePerson failed: %v", err)
	}

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
}
