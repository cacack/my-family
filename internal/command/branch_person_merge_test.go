package command_test

import (
	"context"
	"errors"
	"slices"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/cacack/my-family/internal/command"
	"github.com/cacack/my-family/internal/domain"
	"github.com/cacack/my-family/internal/repository"
)

// Person merge on a research branch (#834), in memory. The cross-backend
// versions of the main flows are internal/integration/branch_person_merge_test.go
// and the TestBranchScenario_PersonMerge scenarios in each repository backend;
// these cover the command-side rules: the branch-scoped reads, the replay
// order, the merge-chain reference checks, the staleness pin on the merged
// person, and resuming an interrupted person merge.

// mergePair creates two persons on main through h and returns their ids.
func mergePair(t *testing.T, h *command.Handler) (survivor, merged uuid.UUID) {
	t.Helper()
	ctx := context.Background()
	s, err := h.CreatePerson(ctx, command.CreatePersonInput{GivenName: "Morgan", Surname: "Duplicate", Gender: "male"})
	if err != nil {
		t.Fatalf("CreatePerson(survivor) failed: %v", err)
	}
	m, err := h.CreatePerson(ctx, command.CreatePersonInput{GivenName: "Morgan", Surname: "Duplicat", Gender: "male", BirthPlace: "Riverton"})
	if err != nil {
		t.Fatalf("CreatePerson(merged) failed: %v", err)
	}
	return s.ID, m.ID
}

// mergeOn merges merged into survivor through h, reading both versions through
// the handler's own scope.
func mergeOn(t *testing.T, f *branchFixture, h *command.Handler, scope domain.BranchID, survivor, merged uuid.UUID) *command.MergePersonsResult {
	t.Helper()
	ctx := context.Background()
	s, err := f.readStore.GetPerson(ctx, scope, survivor)
	if err != nil || s == nil {
		t.Fatalf("GetPerson(survivor) = %v, %v", s, err)
	}
	m, err := f.readStore.GetPerson(ctx, scope, merged)
	if err != nil || m == nil {
		t.Fatalf("GetPerson(merged) = %v, %v", m, err)
	}
	res, err := h.MergePersons(ctx, command.MergePersonsInput{
		SurvivorID: survivor, MergedID: merged,
		SurvivorVersion: s.Version, MergedVersion: m.Version,
	})
	if err != nil {
		t.Fatalf("MergePersons failed: %v", err)
	}
	return res
}

// personOn reads a person on a scope, failing on a store error.
func personOn(t *testing.T, f *branchFixture, scope domain.BranchID, id uuid.UUID) *repository.PersonReadModel {
	t.Helper()
	p, err := f.readStore.GetPerson(context.Background(), scope, id)
	if err != nil {
		t.Fatalf("GetPerson(%s) failed: %v", id, err)
	}
	return p
}

func TestMergePersons_OnBranchLeavesMainUntouched(t *testing.T) {
	f := newBranchFixture()
	ctx := context.Background()
	survivor, merged := mergePair(t, f.handler)
	if _, err := f.handler.AddName(ctx, command.AddNameInput{PersonID: merged, GivenName: "Morgen", Surname: "Duplicat", NameType: "aka"}); err != nil {
		t.Fatalf("AddName failed: %v", err)
	}
	branch, err := f.handler.CreateBranch(ctx, "same-person", "")
	if err != nil {
		t.Fatalf("CreateBranch failed: %v", err)
	}
	onBranch := domain.BranchID(branch.ID)

	res := mergeOn(t, f, f.handler.WithBranch(branch), onBranch, survivor, merged)
	if res.Summary.NamesTransferred != 1 {
		t.Errorf("NamesTransferred = %d, want the merged person's alternate name (read through the branch)", res.Summary.NamesTransferred)
	}

	if p := personOn(t, f, onBranch, merged); p != nil {
		t.Errorf("branch still has the merged person: %+v", p)
	}
	if p := personOn(t, f, onBranch, survivor); p == nil || p.BirthPlace != "Riverton" {
		t.Errorf("branch survivor = %+v, want the merged person's birth place", p)
	}
	if p := personOn(t, f, domain.MainBranchID, merged); p == nil {
		t.Error("main lost the merged person before the branch merged")
	}
	if p := personOn(t, f, domain.MainBranchID, survivor); p == nil || p.BirthPlace != "" {
		t.Errorf("main survivor = %+v, want it untouched", p)
	}

	// The merge was recorded on the branch, not on main.
	branchEvents, err := f.eventStore.ReadBranch(ctx, onBranch, 0, 100)
	if err != nil {
		t.Fatalf("ReadBranch failed: %v", err)
	}
	if !slices.ContainsFunc(branchEvents, func(e repository.StoredEvent) bool { return e.EventType == "PersonMerged" }) {
		t.Error("the branch holds no PersonMerged")
	}

	if _, err := f.handler.MergeBranch(ctx, command.MergeBranchInput{BranchID: branch.ID}); err != nil {
		t.Fatalf("MergeBranch failed: %v", err)
	}
	if p := personOn(t, f, domain.MainBranchID, merged); p != nil {
		t.Errorf("main still has the merged person after the branch merged: %+v", p)
	}
	if p := personOn(t, f, domain.MainBranchID, survivor); p == nil || p.BirthPlace != "Riverton" {
		t.Errorf("main survivor = %+v, want the merge carried onto main", p)
	}
	names, err := f.readStore.GetPersonNames(ctx, domain.MainBranchID, survivor)
	if err != nil || len(names) != 1 || names[0].GivenName != "Morgen" {
		t.Errorf("main survivor names = %+v (err=%v), want the merged person's alternate", names, err)
	}
}

func TestMergePersons_OnBranchChecksBranchVersions(t *testing.T) {
	f := newBranchFixture()
	ctx := context.Background()
	survivor, merged := mergePair(t, f.handler)
	branch, err := f.handler.CreateBranch(ctx, "versions", "")
	if err != nil {
		t.Fatalf("CreateBranch failed: %v", err)
	}
	scoped := f.handler.WithBranch(branch)
	surname := "Branchside"
	if _, err := scoped.UpdatePerson(ctx, command.UpdatePersonInput{ID: merged, Surname: &surname, Version: 1}); err != nil {
		t.Fatalf("branch UpdatePerson failed: %v", err)
	}

	// Main's version of the merged person is stale on the branch.
	_, err = scoped.MergePersons(ctx, command.MergePersonsInput{
		SurvivorID: survivor, MergedID: merged, SurvivorVersion: 1, MergedVersion: 1,
	})
	if !errors.Is(err, repository.ErrConcurrencyConflict) {
		t.Fatalf("MergePersons with main's version = %v, want ErrConcurrencyConflict", err)
	}
	mergeOn(t, f, scoped, domain.BranchID(branch.ID), survivor, merged)
}

// TestMergeBranch_PersonMergeReplaysAfterItsReferences: the branch touches the
// survivor first, then creates the duplicate and a family naming them, then
// merges. First-touch order would replay the merge before the duplicate and the
// family exist on main; the replay must reorder it after both, so main ends up
// with the family on the survivor and no duplicate.
func TestMergeBranch_PersonMergeReplaysAfterItsReferences(t *testing.T) {
	f := newBranchFixture()
	ctx := context.Background()
	survivor, _ := mergePair(t, f.handler)
	partner, err := f.handler.CreatePerson(ctx, command.CreatePersonInput{GivenName: "Jordan", Surname: "Partner", Gender: "female"})
	if err != nil {
		t.Fatalf("CreatePerson failed: %v", err)
	}
	branch, err := f.handler.CreateBranch(ctx, "reorder", "")
	if err != nil {
		t.Fatalf("CreateBranch failed: %v", err)
	}
	scoped := f.handler.WithBranch(branch)
	onBranch := domain.BranchID(branch.ID)

	notes := "touched first"
	if _, err := scoped.UpdatePerson(ctx, command.UpdatePersonInput{ID: survivor, Notes: &notes, Version: 1}); err != nil {
		t.Fatalf("branch UpdatePerson failed: %v", err)
	}
	dup, err := scoped.CreatePerson(ctx, command.CreatePersonInput{GivenName: "Morgan", Surname: "Dupe", Gender: "male"})
	if err != nil {
		t.Fatalf("branch CreatePerson failed: %v", err)
	}
	family, err := scoped.CreateFamily(ctx, command.CreateFamilyInput{Partner1ID: &dup.ID, Partner2ID: &partner.ID})
	if err != nil {
		t.Fatalf("branch CreateFamily failed: %v", err)
	}
	mergeOn(t, f, scoped, onBranch, survivor, dup.ID)

	if _, err := f.handler.MergeBranch(ctx, command.MergeBranchInput{BranchID: branch.ID}); err != nil {
		t.Fatalf("MergeBranch failed: %v", err)
	}
	if p := personOn(t, f, domain.MainBranchID, dup.ID); p != nil {
		t.Errorf("main has the duplicate after the merge: %+v", p)
	}
	fam, err := f.readStore.GetFamily(ctx, domain.MainBranchID, family.ID)
	if err != nil || fam == nil || fam.Partner1ID == nil || *fam.Partner1ID != survivor {
		t.Errorf("main family = %+v (err=%v), want partner1 the survivor", fam, err)
	}
	if fam != nil && fam.Partner1Surname != "Duplicate" {
		t.Errorf("main family partner1 surname = %q, want the survivor's", fam.Partner1Surname)
	}
}

// TestMergeBranch_PersonMergeChain: the branch merges A into B and then B into
// C, having touched C first. The merge into C must replay after the merge into
// B, or B would absorb A after B itself was gone.
func TestMergeBranch_PersonMergeChain(t *testing.T) {
	f := newBranchFixture()
	ctx := context.Background()
	var ids [3]uuid.UUID
	for i, given := range []string{"Avery", "Blair", "Casey"} {
		p, err := f.handler.CreatePerson(ctx, command.CreatePersonInput{GivenName: given, Surname: "Chain"})
		if err != nil {
			t.Fatalf("CreatePerson failed: %v", err)
		}
		ids[i] = p.ID
		if _, err := f.handler.AddName(ctx, command.AddNameInput{PersonID: p.ID, GivenName: given + "a", Surname: "Chain", NameType: "aka"}); err != nil {
			t.Fatalf("AddName failed: %v", err)
		}
	}
	a, b, c := ids[0], ids[1], ids[2]
	branch, err := f.handler.CreateBranch(ctx, "chain", "")
	if err != nil {
		t.Fatalf("CreateBranch failed: %v", err)
	}
	scoped := f.handler.WithBranch(branch)
	onBranch := domain.BranchID(branch.ID)
	notes := "touched first"
	if _, err := scoped.UpdatePerson(ctx, command.UpdatePersonInput{ID: c, Notes: &notes, Version: 2}); err != nil {
		t.Fatalf("branch UpdatePerson failed: %v", err)
	}
	mergeOn(t, f, scoped, onBranch, b, a)
	mergeOn(t, f, scoped, onBranch, c, b)

	if _, err := f.handler.MergeBranch(ctx, command.MergeBranchInput{BranchID: branch.ID}); err != nil {
		t.Fatalf("MergeBranch failed: %v", err)
	}
	for _, gone := range []uuid.UUID{a, b} {
		if p := personOn(t, f, domain.MainBranchID, gone); p != nil {
			t.Errorf("main still has %s after the chain merged: %+v", p.GivenName, p)
		}
	}
	names, err := f.readStore.GetPersonNames(ctx, domain.MainBranchID, c)
	if err != nil || len(names) != 3 {
		t.Errorf("main names of the final survivor = %d (err=%v), want 3 (its own alternate and those of both merged persons)", len(names), err)
	}
}

// TestMergeBranch_MergedPersonStaleRefuses: main writes to the merged person
// between the conflict verdict and the claim. The merged person's stream is not
// replayed, but the verdict covered it, so the merge is refused as stale with
// nothing written.
func TestMergeBranch_MergedPersonStaleRefuses(t *testing.T) {
	f, positions := newStaleMergeFixture()
	ctx := context.Background()
	survivor, merged := mergePair(t, f.handler)
	branch, err := f.handler.CreateBranch(ctx, "stale-merged", "")
	if err != nil {
		t.Fatalf("CreateBranch failed: %v", err)
	}
	mergeOn(t, f, f.handler.WithBranch(branch), domain.BranchID(branch.ID), survivor, merged)

	positions.rival = func() {
		surname := "Rival"
		if _, err := f.handler.UpdatePerson(ctx, command.UpdatePersonInput{ID: merged, Surname: &surname, Version: 1}); err != nil {
			t.Fatalf("rival UpdatePerson failed: %v", err)
		}
	}
	_, err = f.handler.MergeBranch(ctx, command.MergeBranchInput{BranchID: branch.ID})
	if !errors.Is(err, command.ErrMergePlanStale) {
		t.Fatalf("MergeBranch = %v, want ErrMergePlanStale", err)
	}
	if b, _ := f.branchStore.Get(ctx, branch.ID); b.Status != domain.BranchStatusActive {
		t.Errorf("branch status = %s, want active - a stale refusal writes nothing", b.Status)
	}
	if p := personOn(t, f, domain.MainBranchID, merged); p == nil || p.Surname != "Rival" {
		t.Errorf("main merged person = %+v, want the rival's edit kept", p)
	}

	// A fresh attempt sees the rival's edit as a conflict on the survivor.
	res, err := f.handler.MergeBranch(ctx, command.MergeBranchInput{BranchID: branch.ID})
	if !errors.Is(err, command.ErrMergeConflicts) {
		t.Fatalf("second MergeBranch = %v, want ErrMergeConflicts", err)
	}
	if len(res.Conflicts) != 1 || res.Conflicts[0].StreamID != survivor {
		t.Errorf("conflicts = %+v, want one on the survivor", res.Conflicts)
	}
}

// TestMergeBranch_MergeChainVouchesForMediaAndResearch: the branch attaches a
// photo and a research log to the duplicate and then merges them away; main
// then deletes the duplicate. Both items name a person main no longer has, but
// the replayed merge re-links them to the survivor, as it did on the branch.
func TestMergeBranch_MergeChainVouchesForMediaAndResearch(t *testing.T) {
	f := newBranchFixture()
	ctx := context.Background()
	survivor, merged := mergePair(t, f.handler)
	branch, err := f.handler.CreateBranch(ctx, "attach-then-merge", "")
	if err != nil {
		t.Fatalf("CreateBranch failed: %v", err)
	}
	scoped := f.handler.WithBranch(branch)
	onBranch := domain.BranchID(branch.ID)
	photo, err := scoped.UploadMedia(ctx, command.UploadMediaInput{
		EntityType: "person", EntityID: merged, Title: "Portrait",
		MediaType: "photo", Filename: "portrait.jpg", FileData: createTestJPEG(),
	})
	if err != nil {
		t.Fatalf("branch UploadMedia failed: %v", err)
	}
	log, err := scoped.CreateResearchLog(ctx, command.CreateResearchLogInput{
		SubjectID: merged, SubjectType: "person", Repository: "County Archive",
		SearchDescription: "Baptisms", Outcome: string(domain.ResearchOutcomeNotFound),
		SearchDate: time.Date(2024, 3, 1, 0, 0, 0, 0, time.UTC),
	})
	if err != nil {
		t.Fatalf("branch CreateResearchLog failed: %v", err)
	}
	mergeOn(t, f, scoped, onBranch, survivor, merged)

	if err := f.handler.DeletePerson(ctx, command.DeletePersonInput{ID: merged, Version: 1}); err != nil {
		t.Fatalf("main DeletePerson failed: %v", err)
	}

	if _, err := f.handler.MergeBranch(ctx, command.MergeBranchInput{BranchID: branch.ID}); err != nil {
		t.Fatalf("MergeBranch failed: %v", err)
	}
	media, err := f.readStore.GetMedia(ctx, domain.MainBranchID, photo.ID)
	if err != nil || media == nil || media.EntityID != survivor {
		t.Errorf("main photo = %+v (err=%v), want it on the survivor", media, err)
	}
	research, err := f.readStore.GetResearchLog(ctx, domain.MainBranchID, log.ID)
	if err != nil || research == nil || research.SubjectID != survivor {
		t.Errorf("main research log = %+v (err=%v), want it about the survivor", research, err)
	}
}

// TestMergeBranch_MergeIntoRemovedSurvivorIsDangling: main merges the branch's
// survivor away after the fork. Replaying the branch's merge would re-link the
// duplicate's data to a person main no longer has, so it is refused as a
// dangling reference, with nothing written.
func TestMergeBranch_MergeIntoRemovedSurvivorIsDangling(t *testing.T) {
	f := newBranchFixture()
	ctx := context.Background()
	survivor, merged := mergePair(t, f.handler)
	branch, err := f.handler.CreateBranch(ctx, "survivor-merged-away", "")
	if err != nil {
		t.Fatalf("CreateBranch failed: %v", err)
	}
	mergeOn(t, f, f.handler.WithBranch(branch), domain.BranchID(branch.ID), survivor, merged)

	// Main merges the survivor into a third person, which writes nothing to
	// the survivor's stream: no conflict, but the branch's merge would target
	// a person main no longer has.
	third, err := f.handler.CreatePerson(ctx, command.CreatePersonInput{GivenName: "Morgan", Surname: "Third"})
	if err != nil {
		t.Fatalf("CreatePerson failed: %v", err)
	}
	mergeOn(t, f, f.handler, domain.MainBranchID, third.ID, survivor)

	_, err = f.handler.MergeBranch(ctx, command.MergeBranchInput{BranchID: branch.ID})
	if !errors.Is(err, command.ErrMergeDanglingReference) {
		t.Fatalf("MergeBranch = %v, want ErrMergeDanglingReference", err)
	}
	if p := personOn(t, f, domain.MainBranchID, merged); p == nil {
		t.Error("the refused merge removed the merged person from main")
	}
}

// TestResumeMerge_RepairsAHalfProjectedPersonMerge: the replayed PersonMerged
// reached main's log but its projection failed partway (at the family
// re-link). The survivor's row is saved last, so it reads as behind the log
// and the resume re-runs the whole merge.
func TestResumeMerge_RepairsAHalfProjectedPersonMerge(t *testing.T) {
	var reads *faultyReadStore
	f := newBranchFixtureWith(branchFixtureDeps{
		wrapReads: func(inner repository.ReadModelStore) repository.ReadModelStore {
			reads = &faultyReadStore{ReadModelStore: inner}
			return reads
		},
	})
	ctx := context.Background()
	survivor, merged := mergePair(t, f.handler)
	partner, err := f.handler.CreatePerson(ctx, command.CreatePersonInput{GivenName: "Jordan", Surname: "Partner", Gender: "female"})
	if err != nil {
		t.Fatalf("CreatePerson failed: %v", err)
	}
	family, err := f.handler.CreateFamily(ctx, command.CreateFamilyInput{Partner1ID: &merged, Partner2ID: &partner.ID})
	if err != nil {
		t.Fatalf("CreateFamily failed: %v", err)
	}
	branch, err := f.handler.CreateBranch(ctx, "half-projected", "")
	if err != nil {
		t.Fatalf("CreateBranch failed: %v", err)
	}
	mergeOn(t, f, f.handler.WithBranch(branch), domain.BranchID(branch.ID), survivor, merged)

	reads.armed, reads.failFamily = true, family.ID
	_, err = f.handler.MergeBranch(ctx, command.MergeBranchInput{BranchID: branch.ID})
	reads.armed = false
	if !errors.Is(err, command.ErrMergePartiallyApplied) || !errors.Is(err, errInjectedProjectionFailure) {
		t.Fatalf("MergeBranch = %v, want the projection failure wrapped in ErrMergePartiallyApplied", err)
	}
	if p := personOn(t, f, domain.MainBranchID, survivor); p == nil || p.BirthPlace != "" {
		t.Fatalf("main survivor = %+v, want its row left behind the log", p)
	}

	res, err := f.handler.ResumeMerge(ctx, command.ResumeMergeInput{BranchID: branch.ID})
	if err != nil {
		t.Fatalf("ResumeMerge failed: %v", err)
	}
	if !slices.Equal(res.ReprojectedStreamIDs, []uuid.UUID{survivor}) {
		t.Errorf("ReprojectedStreamIDs = %v, want the survivor", res.ReprojectedStreamIDs)
	}
	if p := personOn(t, f, domain.MainBranchID, merged); p != nil {
		t.Errorf("main still has the merged person after the resume: %+v", p)
	}
	fam, err := f.readStore.GetFamily(ctx, domain.MainBranchID, family.ID)
	if err != nil || fam == nil || fam.Partner1ID == nil || *fam.Partner1ID != survivor {
		t.Errorf("main family = %+v (err=%v), want it re-linked to the survivor", fam, err)
	}
	if p := personOn(t, f, domain.MainBranchID, survivor); p == nil || p.BirthPlace != "Riverton" {
		t.Errorf("main survivor = %+v, want the merge's resolved birth place", p)
	}
}
