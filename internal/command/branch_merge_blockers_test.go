package command_test

import (
	"context"
	"errors"
	"reflect"
	"strings"
	"sync"
	"testing"

	"github.com/google/uuid"

	"github.com/cacack/my-family/internal/command"
	"github.com/cacack/my-family/internal/domain"
	"github.com/cacack/my-family/internal/repository"
)

// blockerSummary is what a test pins about one blocker.
type blockerSummary struct {
	stream, referenced  uuid.UUID
	kind                command.MergeBlockerKind
	entityName, refName string
	fix                 command.MergeBlockerFix
}

func summarize(blockers []command.MergeBlocker) []blockerSummary {
	out := make([]blockerSummary, 0, len(blockers))
	for _, b := range blockers {
		out = append(out, blockerSummary{b.StreamID, b.ReferencedID, b.Kind, b.EntityName, b.ReferencedName, b.SuggestedResolution})
	}
	return out
}

// The branch deletes a source and a person; main then cites the source twice,
// photographs the person twice and records two research logs about them. Each
// of the six rows the deletes would cascade away is its own blocker, named,
// and the precheck reports exactly what the merge refuses with (#831).
func TestMergeBranch_ReportsEveryCascadeBlocker(t *testing.T) {
	e := newEvidenceFixture(t)
	ctx := context.Background()
	scoped := e.f.handler.WithBranch(e.branch)
	branchID := domain.BranchID(e.branch.ID)

	e.deleteSource(t, scoped, branchID, e.source)
	e.deletePerson(t, scoped, branchID, e.person)

	cit1 := e.cite(t, e.f.handler, e.source)
	cit2 := e.cite(t, e.f.handler, e.source)
	photo1 := e.upload(t, e.f.handler, "person", e.person)
	photo2 := e.upload(t, e.f.handler, "person", e.person)
	log1 := e.researchLog(t, e.f.handler, e.person)
	log2 := e.researchLog(t, e.f.handler, e.person)

	_, err := e.f.handler.MergeBranch(ctx, command.MergeBranchInput{BranchID: e.branch.ID})
	e.assertRefusedBeforeClaim(t, err)
	var blocked *command.MergeBlockedError
	if !errors.As(err, &blocked) {
		t.Fatalf("MergeBranch error = %T %v, want *MergeBlockedError", err, err)
	}
	if !strings.Contains(err.Error(), "5 more blocker") {
		t.Errorf("error = %q, want it to count the other five blockers", err)
	}

	leave := command.FixLeaveOut
	want := map[blockerSummary]bool{
		{e.source, cit1.ID, command.BlockerSourceDeleteOrphansCitation, "1880 Census", "1880 Census (Birth)", leave}: true,
		{e.source, cit2.ID, command.BlockerSourceDeleteOrphansCitation, "1880 Census", "1880 Census (Birth)", leave}: true,
		{e.person, photo1, command.BlockerOwnerDeleteOrphansMedia, "Ada Lovelace", "Scan", leave}:                    true,
		{e.person, photo2, command.BlockerOwnerDeleteOrphansMedia, "Ada Lovelace", "Scan", leave}:                    true,
		{e.person, log1.ID, command.BlockerSubjectDeleteOrphansGPS, "Ada Lovelace", "Baptisms (Archive)", leave}:     true,
		{e.person, log2.ID, command.BlockerSubjectDeleteOrphansGPS, "Ada Lovelace", "Baptisms (Archive)", leave}:     true,
	}
	got := summarize(blocked.Blockers)
	if len(got) != len(want) {
		t.Fatalf("blockers = %+v, want the %d in %+v", got, len(want), want)
	}
	for _, b := range got {
		if !want[b] {
			t.Errorf("unexpected blocker %+v; want one of %+v", b, want)
		}
	}
	for _, b := range blocked.Blockers {
		if b.Message == "" || b.EntityType == "" || b.ReferencedType == "" {
			t.Errorf("blocker %+v is missing its message or types", b)
		}
	}

	pre, err := e.f.handler.PrecheckMerge(ctx, command.PrecheckMergeInput{BranchID: e.branch.ID})
	if err != nil {
		t.Fatalf("PrecheckMerge failed: %v", err)
	}
	if !reflect.DeepEqual(pre, blocked.Blockers) {
		t.Errorf("precheck blockers =\n  %+v\nwant the merge's\n  %+v", pre, blocked.Blockers)
	}

	// Leaving both deletes out clears every blocker.
	cleared := map[uuid.UUID]command.MergeResolution{e.source: command.ResolveMain, e.person: command.ResolveMain}
	pre, err = e.f.handler.PrecheckMerge(ctx, command.PrecheckMergeInput{BranchID: e.branch.ID, Resolutions: cleared})
	if err != nil || len(pre) != 0 {
		t.Errorf("PrecheckMerge with both left out = %+v, %v; want none", pre, err)
	}
}

// A person the branch creates and a family naming them: excluding the person
// blocks the family, and the suggested fix is to include the person again.
func TestPrecheckMerge_SuggestsIncludingAnExcludedPerson(t *testing.T) {
	f := newBranchFixture()
	ctx := context.Background()
	branch, err := f.handler.CreateBranch(ctx, "newcomer", "")
	if err != nil {
		t.Fatalf("CreateBranch failed: %v", err)
	}
	scoped := f.handler.WithBranch(branch)
	p, err := scoped.CreatePerson(ctx, command.CreatePersonInput{GivenName: "Pat", Surname: "Newcomer"})
	if err != nil {
		t.Fatalf("CreatePerson failed: %v", err)
	}
	fam, err := scoped.CreateFamily(ctx, command.CreateFamilyInput{Partner1ID: &p.ID})
	if err != nil {
		t.Fatalf("CreateFamily failed: %v", err)
	}

	got, err := f.handler.PrecheckMerge(ctx, command.PrecheckMergeInput{
		BranchID: branch.ID, Resolutions: map[uuid.UUID]command.MergeResolution{p.ID: command.ResolveMain},
	})
	if err != nil {
		t.Fatalf("PrecheckMerge failed: %v", err)
	}
	want := []blockerSummary{{fam.ID, p.ID, command.BlockerMissingPerson, "Pat Newcomer", "Pat Newcomer", command.FixIncludeReferenced}}
	if !reflect.DeepEqual(summarize(got), want) {
		t.Errorf("blockers = %+v, want %+v", summarize(got), want)
	}
	if got[0].EntityType != "family" || got[0].ReferencedType != "person" {
		t.Errorf("types = %s -> %s, want family -> person", got[0].EntityType, got[0].ReferencedType)
	}

	// Nothing excluded, nothing blocked — and the precheck wrote nothing.
	if got, err := f.handler.PrecheckMerge(ctx, command.PrecheckMergeInput{BranchID: branch.ID}); err != nil || len(got) != 0 {
		t.Errorf("PrecheckMerge with no resolutions = %+v, %v; want none", got, err)
	}
	after, err := f.branchStore.Get(ctx, branch.ID)
	if err != nil || after.Status != domain.BranchStatusActive {
		t.Errorf("branch after precheck = %+v, %v; want still active", after, err)
	}
}

// mainPersonCounter counts, per person, the GetPerson lookups made against
// main while counting is on.
type mainPersonCounter struct {
	repository.ReadModelStore
	mu       sync.Mutex
	counting bool
	asked    map[uuid.UUID]int
}

func (c *mainPersonCounter) GetPerson(ctx context.Context, branchID domain.BranchID, id uuid.UUID) (*repository.PersonReadModel, error) {
	c.mu.Lock()
	if c.counting && branchID == domain.MainBranchID {
		c.asked[id]++
	}
	c.mu.Unlock()
	return c.ReadModelStore.GetPerson(ctx, branchID, id)
}

// The fix suggestion re-runs the rules once per candidate "include" fix; the
// trial runs share the scan's main lookups, so each excluded person is looked
// up on main once per precheck, not once per candidate (#831).
func TestPrecheckMerge_TrialRunsShareMainLookups(t *testing.T) {
	counter := &mainPersonCounter{asked: map[uuid.UUID]int{}}
	f := newBranchFixtureWith(branchFixtureDeps{wrapReads: func(r repository.ReadModelStore) repository.ReadModelStore {
		counter.ReadModelStore = r
		return counter
	}})
	ctx := context.Background()
	branch, err := f.handler.CreateBranch(ctx, "many-newcomers", "")
	if err != nil {
		t.Fatalf("CreateBranch failed: %v", err)
	}
	scoped := f.handler.WithBranch(branch)
	const k = 4
	excluded := make(map[uuid.UUID]command.MergeResolution, k)
	for i := 0; i < k; i++ {
		p, err := scoped.CreatePerson(ctx, command.CreatePersonInput{GivenName: "Pat", Surname: "Newcomer"})
		if err != nil {
			t.Fatalf("CreatePerson failed: %v", err)
		}
		if _, err := scoped.CreateFamily(ctx, command.CreateFamilyInput{Partner1ID: &p.ID}); err != nil {
			t.Fatalf("CreateFamily failed: %v", err)
		}
		excluded[p.ID] = command.ResolveMain
	}

	counter.mu.Lock()
	counter.counting = true
	counter.mu.Unlock()
	got, err := f.handler.PrecheckMerge(ctx, command.PrecheckMergeInput{BranchID: branch.ID, Resolutions: excluded})
	if err != nil {
		t.Fatalf("PrecheckMerge failed: %v", err)
	}
	if len(got) != k {
		t.Fatalf("blockers = %d, want %d", len(got), k)
	}
	for _, b := range got {
		if b.SuggestedResolution != command.FixIncludeReferenced {
			t.Errorf("blocker on %s: fix = %s, want include_referenced", b.StreamID, b.SuggestedResolution)
		}
	}
	for id := range excluded {
		if n := counter.asked[id]; n != 1 {
			t.Errorf("person %s looked up on main %d times, want 1", id, n)
		}
	}
}

// The precheck refuses what the merge refuses before its reference check.
func TestPrecheckMerge_Refusals(t *testing.T) {
	f := newBranchFixture()
	ctx := context.Background()

	if _, err := f.handler.PrecheckMerge(ctx, command.PrecheckMergeInput{BranchID: uuid.New()}); !errors.Is(err, repository.ErrBranchNotFound) {
		t.Errorf("unknown branch: err = %v, want ErrBranchNotFound", err)
	}

	empty, err := f.handler.CreateBranch(ctx, "empty", "")
	if err != nil {
		t.Fatalf("CreateBranch failed: %v", err)
	}
	if _, err := f.handler.PrecheckMerge(ctx, command.PrecheckMergeInput{BranchID: empty.ID}); !errors.Is(err, command.ErrMergeEmpty) {
		t.Errorf("empty branch: err = %v, want ErrMergeEmpty", err)
	}

	branch, err := f.handler.CreateBranch(ctx, "one", "")
	if err != nil {
		t.Fatalf("CreateBranch failed: %v", err)
	}
	if _, err := f.handler.WithBranch(branch).CreatePerson(ctx, command.CreatePersonInput{GivenName: "Lee", Surname: "Lone"}); err != nil {
		t.Fatalf("CreatePerson failed: %v", err)
	}
	unknown := map[uuid.UUID]command.MergeResolution{uuid.New(): command.ResolveMain}
	if _, err := f.handler.PrecheckMerge(ctx, command.PrecheckMergeInput{BranchID: branch.ID, Resolutions: unknown}); !errors.Is(err, command.ErrUnknownResolution) {
		t.Errorf("unknown stream: err = %v, want ErrUnknownResolution", err)
	}

	if err := f.handler.DeleteBranch(ctx, empty.ID); err != nil {
		t.Fatalf("DeleteBranch failed: %v", err)
	}
	if _, err := f.handler.PrecheckMerge(ctx, command.PrecheckMergeInput{BranchID: empty.ID}); !errors.Is(err, command.ErrBranchNotActive) {
		t.Errorf("archived branch: err = %v, want ErrBranchNotActive", err)
	}

	bare := command.NewHandler(f.eventStore, f.readStore)
	if _, err := bare.PrecheckMerge(ctx, command.PrecheckMergeInput{BranchID: branch.ID}); !errors.Is(err, command.ErrBranchStoreRequired) {
		t.Errorf("no branch store: err = %v, want ErrBranchStoreRequired", err)
	}
}

func TestMergeBlockedError(t *testing.T) {
	var none command.MergeBlockedError
	if none.Error() != command.ErrMergeDanglingReference.Error() || !errors.Is(&none, command.ErrMergeDanglingReference) {
		t.Errorf("empty MergeBlockedError = %q", none.Error())
	}
	one := &command.MergeBlockedError{Blockers: []command.MergeBlocker{{Message: "the first"}}}
	if got := one.Error(); got != command.ErrMergeDanglingReference.Error()+": the first" {
		t.Errorf("one blocker: %q", got)
	}
}
