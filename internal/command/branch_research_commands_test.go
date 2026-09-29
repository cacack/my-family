package command_test

import (
	"context"
	"errors"
	"testing"

	"github.com/google/uuid"

	"github.com/cacack/my-family/internal/command"
	"github.com/cacack/my-family/internal/domain"
	"github.com/cacack/my-family/internal/query"
	"github.com/cacack/my-family/internal/repository"
	"github.com/cacack/my-family/internal/repository/memory"
)

// researchSeed is a person, a family and a proof summary on main.
type researchSeed struct {
	person, family, proof uuid.UUID
}

func seedResearch(t *testing.T, h *command.Handler) researchSeed {
	t.Helper()
	ctx := context.Background()
	person, err := h.CreatePerson(ctx, command.CreatePersonInput{GivenName: "Ada", Surname: "Placeholder"})
	if err != nil {
		t.Fatalf("CreatePerson: %v", err)
	}
	family, err := h.CreateFamily(ctx, command.CreateFamilyInput{Partner1ID: &person.ID})
	if err != nil {
		t.Fatalf("CreateFamily: %v", err)
	}
	proof, err := h.CreateProofSummary(ctx, command.CreateProofSummaryInput{
		FactType: string(domain.FactPersonBirth), SubjectID: person.ID,
		Conclusion: "Born 1815", Argument: "The census and the register agree",
	})
	if err != nil {
		t.Fatalf("CreateProofSummary: %v", err)
	}
	return researchSeed{person: person.ID, family: family.ID, proof: proof.ID}
}

func personSubject(id uuid.UUID) domain.BranchSubject {
	return domain.BranchSubject{Type: domain.BranchSubjectPerson, ID: id}
}

func familySubject(id uuid.UUID) domain.BranchSubject {
	return domain.BranchSubject{Type: domain.BranchSubjectFamily, ID: id}
}

func ptr[T any](v T) *T { return &v }

func TestCreateBranchWithResearch(t *testing.T) {
	ctx := context.Background()
	f := newBranchFixture()
	seed := seedResearch(t, f.handler)

	branch, err := f.handler.CreateBranchWithResearch(ctx, command.CreateBranchInput{
		Name: "mary",
		Research: domain.BranchResearch{
			Hypothesis:      "Was Ada the daughter of the family?",
			Subjects:        []domain.BranchSubject{personSubject(seed.person), familySubject(seed.family)},
			ProofSummaryIDs: []uuid.UUID{seed.proof},
		},
	})
	if err != nil {
		t.Fatalf("CreateBranchWithResearch: %v", err)
	}
	if branch.Outcome != domain.BranchOutcomeOpen {
		t.Errorf("Outcome = %q, want open by default", branch.Outcome)
	}

	stored, err := f.branchStore.Get(ctx, branch.ID)
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if stored.Hypothesis != "Was Ada the daughter of the family?" || len(stored.Subjects) != 2 ||
		len(stored.ProofSummaryIDs) != 1 || stored.Outcome != domain.BranchOutcomeOpen {
		t.Errorf("registry row = %+v", stored)
	}

	tests := []struct {
		name     string
		research domain.BranchResearch
		want     error
	}{
		{"missing person", domain.BranchResearch{Subjects: []domain.BranchSubject{personSubject(uuid.New())}}, command.ErrBranchSubjectNotFound},
		{"missing family", domain.BranchResearch{Subjects: []domain.BranchSubject{familySubject(uuid.New())}}, command.ErrBranchSubjectNotFound},
		{"person id as family", domain.BranchResearch{Subjects: []domain.BranchSubject{familySubject(seed.person)}}, command.ErrBranchSubjectNotFound},
		{"missing proof", domain.BranchResearch{ProofSummaryIDs: []uuid.UUID{uuid.New()}}, command.ErrBranchProofSummaryNotFound},
		{"bad outcome", domain.BranchResearch{Outcome: "maybe"}, domain.ErrBranchInvalidOutcome},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := f.handler.CreateBranchWithResearch(ctx, command.CreateBranchInput{Name: "x", Research: tt.research})
			if !errors.Is(err, tt.want) {
				t.Errorf("err = %v, want %v", err, tt.want)
			}
		})
	}
}

func TestUpdateBranch_ActiveBranch(t *testing.T) {
	ctx := context.Background()
	f := newBranchFixture()
	seed := seedResearch(t, f.handler)

	branch, err := f.handler.CreateBranch(ctx, "mary", "original")
	if err != nil {
		t.Fatalf("CreateBranch: %v", err)
	}

	// A person created on the branch is a valid subject of that branch.
	onBranch, err := f.handler.WithBranch(branch).CreatePerson(ctx, command.CreatePersonInput{GivenName: "Branch", Surname: "Only"})
	if err != nil {
		t.Fatalf("CreatePerson on branch: %v", err)
	}

	updated, err := f.handler.UpdateBranch(ctx, command.UpdateBranchInput{
		BranchID:        branch.ID,
		Hypothesis:      ptr("Was Ada the daughter of the family?"),
		Subjects:        &[]domain.BranchSubject{personSubject(seed.person), personSubject(onBranch.ID)},
		ProofSummaryIDs: &[]uuid.UUID{seed.proof},
	})
	if err != nil {
		t.Fatalf("UpdateBranch: %v", err)
	}
	if updated.Hypothesis != "Was Ada the daughter of the family?" || len(updated.Subjects) != 2 ||
		updated.Description != "original" || updated.Outcome != domain.BranchOutcomeOpen {
		t.Errorf("updated = %+v", updated)
	}

	// Partial: only the outcome changes; everything else is kept.
	updated, err = f.handler.UpdateBranch(ctx, command.UpdateBranchInput{
		BranchID: branch.ID,
		Outcome:  ptr(domain.BranchOutcomeProved),
	})
	if err != nil {
		t.Fatalf("UpdateBranch outcome: %v", err)
	}
	if updated.Outcome != domain.BranchOutcomeProved || updated.Hypothesis == "" || len(updated.Subjects) != 2 || len(updated.ProofSummaryIDs) != 1 {
		t.Errorf("after outcome edit = %+v", updated)
	}

	// The two edits are two BranchUpdated events on the branch's own stream.
	version, err := f.eventStore.GetStreamVersion(ctx, branch.ID, domain.BranchID(branch.ID))
	if err != nil {
		t.Fatalf("GetStreamVersion: %v", err)
	}
	if version != 3 {
		t.Errorf("branch stream version = %d, want 3 (created + 2 updates)", version)
	}
	events, err := f.eventStore.ReadStream(ctx, branch.ID)
	if err != nil {
		t.Fatalf("ReadStream: %v", err)
	}
	last, err := events[len(events)-1].DecodeEvent()
	if err != nil {
		t.Fatalf("DecodeEvent: %v", err)
	}
	if e, ok := last.(domain.BranchUpdated); !ok || len(e.ChangedFields) != 1 || e.ChangedFields[0] != "outcome" {
		t.Errorf("last event = %#v, want BranchUpdated changing outcome", last)
	}

	// A no-op edit appends nothing.
	if _, err := f.handler.UpdateBranch(ctx, command.UpdateBranchInput{BranchID: branch.ID, Outcome: ptr(domain.BranchOutcomeProved)}); err != nil {
		t.Fatalf("no-op UpdateBranch: %v", err)
	}
	if v, _ := f.eventStore.GetStreamVersion(ctx, branch.ID, domain.BranchID(branch.ID)); v != 3 {
		t.Errorf("no-op edit moved the stream to version %d", v)
	}

	// Clearing with empty values.
	updated, err = f.handler.UpdateBranch(ctx, command.UpdateBranchInput{
		BranchID: branch.ID, Description: ptr(""), Hypothesis: ptr(""),
		Subjects: &[]domain.BranchSubject{}, ProofSummaryIDs: &[]uuid.UUID{},
	})
	if err != nil {
		t.Fatalf("clearing UpdateBranch: %v", err)
	}
	if updated.Description != "" || updated.Hypothesis != "" || updated.Subjects != nil || updated.ProofSummaryIDs != nil {
		t.Errorf("after clearing = %+v", updated)
	}
}

func TestUpdateBranch_ReferenceChecks(t *testing.T) {
	ctx := context.Background()
	f := newBranchFixture()
	seed := seedResearch(t, f.handler)
	solo, err := f.handler.CreatePerson(ctx, command.CreatePersonInput{GivenName: "Solo", Surname: "Placeholder"})
	if err != nil {
		t.Fatalf("CreatePerson: %v", err)
	}

	branch, err := f.handler.CreateBranch(ctx, "a", "")
	if err != nil {
		t.Fatalf("CreateBranch: %v", err)
	}
	other, err := f.handler.CreateBranch(ctx, "b", "")
	if err != nil {
		t.Fatalf("CreateBranch: %v", err)
	}
	// A person that exists only on another branch is not visible here.
	elsewhere, err := f.handler.WithBranch(other).CreatePerson(ctx, command.CreatePersonInput{GivenName: "Else", Surname: "Where"})
	if err != nil {
		t.Fatalf("CreatePerson on other branch: %v", err)
	}
	if _, err := f.handler.UpdateBranch(ctx, command.UpdateBranchInput{
		BranchID: branch.ID, Subjects: &[]domain.BranchSubject{personSubject(elsewhere.ID)},
	}); !errors.Is(err, command.ErrBranchSubjectNotFound) {
		t.Errorf("other-branch subject = %v, want ErrBranchSubjectNotFound", err)
	}
	if _, err := f.handler.UpdateBranch(ctx, command.UpdateBranchInput{
		BranchID: branch.ID, ProofSummaryIDs: &[]uuid.UUID{uuid.New()},
	}); !errors.Is(err, command.ErrBranchProofSummaryNotFound) {
		t.Errorf("missing proof = %v, want ErrBranchProofSummaryNotFound", err)
	}

	// Link a person, then delete it ON THE BRANCH: the tombstone hides it, so
	// re-adding it fails, but keeping it while adding another subject does not.
	if _, err := f.handler.UpdateBranch(ctx, command.UpdateBranchInput{
		BranchID: branch.ID, Subjects: &[]domain.BranchSubject{personSubject(solo.ID)},
	}); err != nil {
		t.Fatalf("link person: %v", err)
	}
	if err := f.handler.WithBranch(branch).DeletePerson(ctx, command.DeletePersonInput{ID: solo.ID, Version: solo.Version}); err != nil {
		t.Fatalf("DeletePerson on branch: %v", err)
	}
	if _, err := f.handler.UpdateBranch(ctx, command.UpdateBranchInput{
		BranchID: branch.ID, Subjects: &[]domain.BranchSubject{personSubject(solo.ID), familySubject(seed.family)},
	}); err != nil {
		t.Errorf("keeping a stale subject while adding a live one = %v, want nil", err)
	}
	fresh, err := f.handler.CreateBranch(ctx, "c", "")
	if err != nil {
		t.Fatalf("CreateBranch: %v", err)
	}
	if err := f.handler.WithBranch(fresh).DeletePerson(ctx, command.DeletePersonInput{ID: solo.ID, Version: solo.Version}); err != nil {
		t.Fatalf("DeletePerson on fresh branch: %v", err)
	}
	if _, err := f.handler.UpdateBranch(ctx, command.UpdateBranchInput{
		BranchID: fresh.ID, Subjects: &[]domain.BranchSubject{personSubject(solo.ID)},
	}); !errors.Is(err, command.ErrBranchSubjectNotFound) {
		t.Errorf("subject deleted on the branch = %v, want ErrBranchSubjectNotFound", err)
	}
}

func TestUpdateBranch_Refusals(t *testing.T) {
	ctx := context.Background()
	f := newBranchFixture()

	if _, err := f.handler.UpdateBranch(ctx, command.UpdateBranchInput{BranchID: uuid.New(), Outcome: ptr(domain.BranchOutcomeProved)}); !errors.Is(err, repository.ErrBranchNotFound) {
		t.Errorf("unknown branch = %v, want ErrBranchNotFound", err)
	}

	branch, err := f.handler.CreateBranch(ctx, "a", "")
	if err != nil {
		t.Fatalf("CreateBranch: %v", err)
	}
	if _, err := f.handler.UpdateBranch(ctx, command.UpdateBranchInput{BranchID: branch.ID}); !errors.Is(err, command.ErrBranchUpdateEmpty) {
		t.Errorf("empty update = %v, want ErrBranchUpdateEmpty", err)
	}
	if _, err := f.handler.UpdateBranch(ctx, command.UpdateBranchInput{BranchID: branch.ID, Outcome: ptr(domain.BranchOutcome("maybe"))}); !errors.Is(err, domain.ErrBranchInvalidOutcome) {
		t.Errorf("bad outcome = %v, want ErrBranchInvalidOutcome", err)
	}
	// An explicit empty outcome is not "open": it is refused, and nothing is
	// appended (it would otherwise read as a change from "open").
	before, err := f.eventStore.GetStreamVersion(ctx, branch.ID, domain.BranchID(branch.ID))
	if err != nil {
		t.Fatalf("GetStreamVersion: %v", err)
	}
	if _, err := f.handler.UpdateBranch(ctx, command.UpdateBranchInput{BranchID: branch.ID, Outcome: ptr(domain.BranchOutcome(""))}); !errors.Is(err, domain.ErrBranchInvalidOutcome) {
		t.Errorf("empty outcome = %v, want ErrBranchInvalidOutcome", err)
	}
	if after, err := f.eventStore.GetStreamVersion(ctx, branch.ID, domain.BranchID(branch.ID)); err != nil || after != before {
		t.Errorf("stream version after empty outcome = %d (%v), want %d", after, err, before)
	}
	long := make([]byte, 501)
	for i := range long {
		long[i] = 'a'
	}
	if _, err := f.handler.UpdateBranch(ctx, command.UpdateBranchInput{BranchID: branch.ID, Description: ptr(string(long))}); !errors.Is(err, domain.ErrBranchDescTooLong) {
		t.Errorf("long description = %v, want ErrBranchDescTooLong", err)
	}

	if err := f.handler.DeleteBranch(ctx, branch.ID); err != nil {
		t.Fatalf("DeleteBranch: %v", err)
	}
	if _, err := f.handler.UpdateBranch(ctx, command.UpdateBranchInput{BranchID: branch.ID, Outcome: ptr(domain.BranchOutcomeDisproved)}); !errors.Is(err, command.ErrBranchNotActive) {
		t.Errorf("archived branch = %v, want ErrBranchNotActive", err)
	}

	noRegistry := command.NewHandler(memory.NewEventStore(), memory.NewReadModelStore())
	if _, err := noRegistry.UpdateBranch(ctx, command.UpdateBranchInput{BranchID: branch.ID, Outcome: ptr(domain.BranchOutcomeProved)}); !errors.Is(err, command.ErrBranchStoreRequired) {
		t.Errorf("no registry = %v, want ErrBranchStoreRequired", err)
	}
}

// TestUpdateBranch_MergedBranch pins the merged-branch rule: the outcome can
// still be recorded (the verdict often lands with or after the merge), every
// other field is locked, and a BranchUpdated is neither a branch change in the
// comparison nor replayed onto main.
func TestUpdateBranch_MergedBranch(t *testing.T) {
	ctx := context.Background()
	f := newBranchFixture()
	seed := seedResearch(t, f.handler)

	branch, err := f.handler.CreateBranchWithResearch(ctx, command.CreateBranchInput{
		Name: "mary", Research: domain.BranchResearch{Hypothesis: "Q", Subjects: []domain.BranchSubject{personSubject(seed.person)}},
	})
	if err != nil {
		t.Fatalf("CreateBranch: %v", err)
	}
	// A research edit alone is not a change to the tree.
	if _, err := f.handler.UpdateBranch(ctx, command.UpdateBranchInput{BranchID: branch.ID, Hypothesis: ptr("Q2")}); err != nil {
		t.Fatalf("UpdateBranch: %v", err)
	}
	svc := query.NewBranchService(f.branchStore, f.eventStore, query.NewHistoryService(f.eventStore, f.readStore))
	comparison, err := svc.CompareBranch(ctx, branch.ID)
	if err != nil {
		t.Fatalf("CompareBranch: %v", err)
	}
	if comparison.BranchChangeCount != 0 {
		t.Errorf("BranchChangeCount = %d, want 0: a BranchUpdated is research metadata", comparison.BranchChangeCount)
	}

	// A real change so the merge has something to replay.
	if _, err := f.handler.WithBranch(branch).CreatePerson(ctx, command.CreatePersonInput{GivenName: "New", Surname: "Child"}); err != nil {
		t.Fatalf("CreatePerson on branch: %v", err)
	}
	result, err := f.handler.MergeBranch(ctx, command.MergeBranchInput{BranchID: branch.ID})
	if err != nil {
		t.Fatalf("MergeBranch: %v", err)
	}
	if result.ReplayedEventCount != 1 {
		t.Errorf("ReplayedEventCount = %d, want 1 (the person, not the BranchUpdated)", result.ReplayedEventCount)
	}

	updated, err := f.handler.UpdateBranch(ctx, command.UpdateBranchInput{BranchID: branch.ID, Outcome: ptr(domain.BranchOutcomeProved)})
	if err != nil {
		t.Fatalf("outcome on merged branch: %v", err)
	}
	if updated.Outcome != domain.BranchOutcomeProved || updated.Status != domain.BranchStatusMerged || updated.MergedAt == nil {
		t.Errorf("merged branch after outcome edit = %+v", updated)
	}
	if updated.Hypothesis != "Q2" || len(updated.Subjects) != 1 {
		t.Errorf("outcome edit disturbed the research record: %+v", updated)
	}

	for name, input := range map[string]command.UpdateBranchInput{
		"hypothesis":  {BranchID: branch.ID, Hypothesis: ptr("changed")},
		"description": {BranchID: branch.ID, Description: ptr("changed")},
		"subjects":    {BranchID: branch.ID, Subjects: &[]domain.BranchSubject{}},
		"proofs":      {BranchID: branch.ID, ProofSummaryIDs: &[]uuid.UUID{seed.proof}},
		"mixed":       {BranchID: branch.ID, Outcome: ptr(domain.BranchOutcomeDisproved), Hypothesis: ptr("x")},
	} {
		if _, err := f.handler.UpdateBranch(ctx, input); !errors.Is(err, command.ErrBranchFieldLocked) {
			t.Errorf("%s on merged branch = %v, want ErrBranchFieldLocked", name, err)
		}
	}
}

// TestResolveBranchLinks resolves subject names and proof summaries through
// the branch's overlay while it is active and through main once it is not.
func TestResolveBranchLinks(t *testing.T) {
	ctx := context.Background()
	f := newBranchFixture()
	seed := seedResearch(t, f.handler)
	svc := query.NewBranchService(f.branchStore, f.eventStore, query.NewHistoryService(f.eventStore, f.readStore))

	branch, err := f.handler.CreateBranch(ctx, "mary", "")
	if err != nil {
		t.Fatalf("CreateBranch: %v", err)
	}
	onBranch, err := f.handler.WithBranch(branch).CreatePerson(ctx, command.CreatePersonInput{GivenName: "Branch", Surname: "Only"})
	if err != nil {
		t.Fatalf("CreatePerson on branch: %v", err)
	}
	branchProof, err := f.handler.WithBranch(branch).CreateProofSummary(ctx, command.CreateProofSummaryInput{
		FactType: string(domain.FactPersonBirth), SubjectID: onBranch.ID, Conclusion: "Born 1820", Argument: "Register",
	})
	if err != nil {
		t.Fatalf("CreateProofSummary on branch: %v", err)
	}
	updated, err := f.handler.UpdateBranch(ctx, command.UpdateBranchInput{
		BranchID:        branch.ID,
		Subjects:        &[]domain.BranchSubject{personSubject(seed.person), familySubject(seed.family), personSubject(onBranch.ID)},
		ProofSummaryIDs: &[]uuid.UUID{seed.proof, branchProof.ID},
	})
	if err != nil {
		t.Fatalf("UpdateBranch: %v", err)
	}

	links, err := svc.ResolveBranchLinks(ctx, updated)
	if err != nil {
		t.Fatalf("ResolveBranchLinks: %v", err)
	}
	if links.SubjectNames[seed.person] != "Ada Placeholder" || links.SubjectNames[onBranch.ID] != "Branch Only" ||
		links.SubjectNames[seed.family] != "Ada Placeholder" {
		t.Errorf("SubjectNames = %v", links.SubjectNames)
	}
	if len(links.ProofSummaries) != 2 || links.ProofSummaries[0].ID != seed.proof || links.ProofSummaries[1].Conclusion != "Born 1820" {
		t.Errorf("ProofSummaries = %+v", links.ProofSummaries)
	}

	// Archived: the overlay is purged, so the branch-only records no longer
	// resolve and are simply absent.
	if err := f.handler.DeleteBranch(ctx, branch.ID); err != nil {
		t.Fatalf("DeleteBranch: %v", err)
	}
	archived, err := f.branchStore.Get(ctx, branch.ID)
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	links, err = svc.ResolveBranchLinks(ctx, archived)
	if err != nil {
		t.Fatalf("ResolveBranchLinks archived: %v", err)
	}
	if _, ok := links.SubjectNames[onBranch.ID]; ok || links.SubjectNames[seed.person] == "" {
		t.Errorf("archived SubjectNames = %v", links.SubjectNames)
	}
	if len(links.ProofSummaries) != 1 || links.ProofSummaries[0].ID != seed.proof {
		t.Errorf("archived ProofSummaries = %+v", links.ProofSummaries)
	}

	// No history service (no read model): nothing to resolve, no error.
	bare := query.NewBranchService(f.branchStore, f.eventStore, nil)
	if links, err := bare.ResolveBranchLinks(ctx, archived); err != nil || len(links.SubjectNames) != 0 {
		t.Errorf("bare ResolveBranchLinks = %+v, %v", links, err)
	}
}

// versionRacingEventStore runs rival once, right after the first
// GetStreamVersion read of stream — i.e. inside the window between a command
// pinning a branch stream's version and appending to it.
type versionRacingEventStore struct {
	repository.EventStore
	stream uuid.UUID
	rival  func()
	fired  bool
}

func (s *versionRacingEventStore) GetStreamVersion(ctx context.Context, streamID uuid.UUID, branchID domain.BranchID) (int64, error) {
	version, err := s.EventStore.GetStreamVersion(ctx, streamID, branchID)
	if err == nil && !s.fired && s.rival != nil && streamID == s.stream {
		s.fired = true
		s.rival()
	}
	return version, err
}

func newVersionRacingFixture() (*branchFixture, *versionRacingEventStore) {
	var racing *versionRacingEventStore
	f := newBranchFixtureWith(branchFixtureDeps{
		wrapEvents: func(inner repository.EventStore) repository.EventStore {
			racing = &versionRacingEventStore{EventStore: inner}
			return racing
		},
	})
	return f, racing
}

// subjectLookupRacingReads runs rival once, inside UpdateBranch's reference
// check — after the command read and validated the branch, before it appends.
type subjectLookupRacingReads struct {
	repository.ReadModelStore
	rival func()
	fired bool
}

func (s *subjectLookupRacingReads) GetPersonsByIDs(ctx context.Context, branchID domain.BranchID, ids []uuid.UUID) ([]repository.PersonReadModel, error) {
	if !s.fired && s.rival != nil {
		s.fired = true
		s.rival()
	}
	return s.ReadModelStore.GetPersonsByIDs(ctx, branchID, ids)
}

func newSubjectLookupRacingFixture() (*branchFixture, *subjectLookupRacingReads) {
	var racing *subjectLookupRacingReads
	f := newBranchFixtureWith(branchFixtureDeps{
		wrapReads: func(inner repository.ReadModelStore) repository.ReadModelStore {
			racing = &subjectLookupRacingReads{ReadModelStore: inner}
			return racing
		},
	})
	return f, racing
}

// TestUpdateBranch_ConcurrentEditIsNotLost: a rival edit landing after the
// victim read the branch must fail the victim's append, not be overwritten by
// the victim's full-state BranchUpdated.
func TestUpdateBranch_ConcurrentEditIsNotLost(t *testing.T) {
	ctx := context.Background()
	f, racing := newSubjectLookupRacingFixture()
	seed := seedResearch(t, f.handler)

	branch, err := f.handler.CreateBranchWithResearch(ctx, command.CreateBranchInput{
		Name: "mary", Research: domain.BranchResearch{Hypothesis: "Q"},
	})
	if err != nil {
		t.Fatalf("CreateBranch: %v", err)
	}

	racing.rival = func() {
		if _, err := f.handler.UpdateBranch(ctx, command.UpdateBranchInput{
			BranchID: branch.ID, Outcome: ptr(domain.BranchOutcomeProved),
		}); err != nil {
			t.Errorf("rival UpdateBranch: %v", err)
		}
	}

	_, err = f.handler.UpdateBranch(ctx, command.UpdateBranchInput{
		BranchID: branch.ID, Hypothesis: ptr("H"), Subjects: &[]domain.BranchSubject{personSubject(seed.person)},
	})
	if !errors.Is(err, repository.ErrConcurrencyConflict) {
		t.Fatalf("victim UpdateBranch = %v, want ErrConcurrencyConflict", err)
	}
	if !racing.fired {
		t.Fatal("the rival write never fired, so nothing was raced")
	}

	got, err := f.branchStore.Get(ctx, branch.ID)
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if got.Outcome != domain.BranchOutcomeProved || got.Hypothesis != "Q" || len(got.Subjects) != 0 {
		t.Errorf("branch after race = outcome %q hypothesis %q, want the rival's proved and the untouched Q", got.Outcome, got.Hypothesis)
	}

	// A retry against the fresh state goes through and keeps the rival's verdict.
	retried, err := f.handler.UpdateBranch(ctx, command.UpdateBranchInput{BranchID: branch.ID, Hypothesis: ptr("H")})
	if err != nil {
		t.Fatalf("retry UpdateBranch: %v", err)
	}
	if retried.Hypothesis != "H" || retried.Outcome != domain.BranchOutcomeProved {
		t.Errorf("retried branch = %+v", retried)
	}
}

// TestUpdateBranch_ConcurrentMergeKeepsTheLock: a merge landing after a
// hypothesis edit read the branch as active must not let that edit through —
// a merged branch accepts only an outcome change.
func TestUpdateBranch_ConcurrentMergeKeepsTheLock(t *testing.T) {
	ctx := context.Background()
	f, racing := newSubjectLookupRacingFixture()
	seed := seedResearch(t, f.handler)

	branch, err := f.handler.CreateBranchWithResearch(ctx, command.CreateBranchInput{
		Name: "mary", Research: domain.BranchResearch{Hypothesis: "Q"},
	})
	if err != nil {
		t.Fatalf("CreateBranch: %v", err)
	}
	if _, err := f.handler.WithBranch(branch).CreatePerson(ctx, command.CreatePersonInput{GivenName: "New", Surname: "Child"}); err != nil {
		t.Fatalf("CreatePerson on branch: %v", err)
	}

	racing.rival = func() {
		if _, err := f.handler.MergeBranch(ctx, command.MergeBranchInput{BranchID: branch.ID}); err != nil {
			t.Errorf("rival MergeBranch: %v", err)
		}
	}

	_, err = f.handler.UpdateBranch(ctx, command.UpdateBranchInput{
		BranchID: branch.ID, Hypothesis: ptr("changed after merge"), Subjects: &[]domain.BranchSubject{personSubject(seed.person)},
	})
	if !errors.Is(err, repository.ErrConcurrencyConflict) {
		t.Fatalf("victim UpdateBranch = %v, want ErrConcurrencyConflict", err)
	}
	if !racing.fired {
		t.Fatal("the rival merge never fired, so nothing was raced")
	}

	got, err := f.branchStore.Get(ctx, branch.ID)
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if got.Status != domain.BranchStatusMerged || got.Hypothesis != "Q" || len(got.Subjects) != 0 {
		t.Errorf("branch after race = status %q hypothesis %q, want merged with Q", got.Status, got.Hypothesis)
	}
	// And the retry now meets the lock.
	if _, err := f.handler.UpdateBranch(ctx, command.UpdateBranchInput{BranchID: branch.ID, Hypothesis: ptr("changed after merge")}); !errors.Is(err, command.ErrBranchFieldLocked) {
		t.Errorf("retry on merged branch = %v, want ErrBranchFieldLocked", err)
	}
}

// TestMergeBranch_ConcurrentEditIsAStalePlan: an edit landing between the
// merge's claim version read and its append makes the claim lose the race.
// Nothing was claimed, so the refusal must say "retry" (ErrMergePlanStale),
// not "already claimed — resume".
func TestMergeBranch_ConcurrentEditIsAStalePlan(t *testing.T) {
	ctx := context.Background()
	f, racing := newVersionRacingFixture()

	branch, err := f.handler.CreateBranchWithResearch(ctx, command.CreateBranchInput{
		Name: "mary", Research: domain.BranchResearch{Hypothesis: "Q"},
	})
	if err != nil {
		t.Fatalf("CreateBranch: %v", err)
	}
	if _, err := f.handler.WithBranch(branch).CreatePerson(ctx, command.CreatePersonInput{GivenName: "New", Surname: "Child"}); err != nil {
		t.Fatalf("CreatePerson on branch: %v", err)
	}

	racing.stream = branch.ID
	racing.rival = func() {
		if _, err := f.handler.UpdateBranch(ctx, command.UpdateBranchInput{BranchID: branch.ID, Hypothesis: ptr("Q2")}); err != nil {
			t.Errorf("rival UpdateBranch: %v", err)
		}
	}

	_, err = f.handler.MergeBranch(ctx, command.MergeBranchInput{BranchID: branch.ID})
	if !errors.Is(err, command.ErrMergePlanStale) || errors.Is(err, command.ErrMergeAlreadyClaimed) {
		t.Fatalf("MergeBranch = %v, want ErrMergePlanStale (and not ErrMergeAlreadyClaimed)", err)
	}
	if !racing.fired {
		t.Fatal("the rival edit never fired, so nothing was raced")
	}
	got, err := f.branchStore.Get(ctx, branch.ID)
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if got.Status != domain.BranchStatusActive || got.Hypothesis != "Q2" {
		t.Errorf("branch after refused merge = status %q hypothesis %q, want active with Q2", got.Status, got.Hypothesis)
	}
	if _, err := f.handler.MergeBranch(ctx, command.MergeBranchInput{BranchID: branch.ID}); err != nil {
		t.Errorf("retried MergeBranch: %v", err)
	}
}

// TestUpdateBranch_RefusesWithoutAStream: a registry row with no events on its
// stream would need the "new stream" sentinel, which turns the concurrency
// check off, so the edit is refused instead.
func TestUpdateBranch_RefusesWithoutAStream(t *testing.T) {
	ctx := context.Background()
	f := newBranchFixture()
	orphan, err := domain.NewBranch("orphan", "", 0)
	if err != nil {
		t.Fatalf("NewBranch: %v", err)
	}
	if err := f.branchStore.Create(ctx, orphan); err != nil {
		t.Fatalf("Create: %v", err)
	}
	if _, err := f.handler.UpdateBranch(ctx, command.UpdateBranchInput{BranchID: orphan.ID, Hypothesis: ptr("H")}); err == nil {
		t.Fatal("UpdateBranch on a branch with no stream succeeded, want a refusal")
	}
	got, err := f.branchStore.Get(ctx, orphan.ID)
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if got.Hypothesis != "" {
		t.Errorf("hypothesis = %q, want it untouched", got.Hypothesis)
	}
}
