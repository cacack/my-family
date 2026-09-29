package command_test

import (
	"context"
	"errors"
	"slices"
	"testing"

	"github.com/google/uuid"

	"github.com/cacack/my-family/internal/command"
	"github.com/cacack/my-family/internal/domain"
)

// completenessOf reads a branch's merge state through the registry, as the
// API does.
func completenessOf(t *testing.T, h *command.Handler, f *branchFixture, branchID uuid.UUID) *command.MergeCompleteness {
	t.Helper()
	ctx := context.Background()
	branch, err := f.branchStore.Get(ctx, branchID)
	if err != nil {
		t.Fatalf("branchStore.Get failed: %v", err)
	}
	completeness, err := h.MergeCompleteness(ctx, branch)
	if err != nil {
		t.Fatalf("MergeCompleteness failed: %v", err)
	}
	return completeness
}

// assertNeedsRepair checks a merge reads incomplete with exactly the given
// streams pending as needs_repair, in that order.
func assertNeedsRepair(t *testing.T, got *command.MergeCompleteness, want ...uuid.UUID) {
	t.Helper()
	if got == nil || got.State != command.MergeStateIncomplete {
		t.Fatalf("completeness = %+v, want incomplete", got)
	}
	ids := make([]uuid.UUID, 0, len(got.Pending))
	for _, p := range got.Pending {
		ids = append(ids, p.StreamID)
		if p.Reason != command.PendingNeedsRepair || p.Reason.NeedsResolution() || len(p.Reason.SupportedResolutions()) != 0 {
			t.Errorf("pending %+v: want needs_repair, needing no decision", p)
		}
	}
	if !slices.Equal(ids, want) {
		t.Errorf("pending streams = %v, want %v", ids, want)
	}
}

// TestMergeCompleteness_LastStreamProjectionFailed: the merge's LAST stream
// reached main's log but its projection failed, so MergeBranch answered
// ErrMergePartiallyApplied with everything in the log. The merge must read as
// incomplete — a resume re-projects the stream — and, once resumed, complete.
// The read itself writes nothing.
func TestMergeCompleteness_LastStreamProjectionFailed(t *testing.T) {
	s := seedResume(t)
	ctx := context.Background()

	s.reads.armed, s.reads.failPerson = true, s.second
	_, err := s.handler.MergeBranch(ctx, command.MergeBranchInput{BranchID: s.branch.ID})
	s.reads.armed, s.reads.failPerson = false, uuid.Nil
	if !errors.Is(err, command.ErrMergePartiallyApplied) {
		t.Fatalf("MergeBranch error = %v, want ErrMergePartiallyApplied", err)
	}
	if got := mainSurnameOf(t, s.f, s.second); got != "Hopper" {
		t.Fatalf("second surname on main = %q, want the failed projection to have left it behind", got)
	}

	head := logHead(t, s.f)
	got := completenessOf(t, s.handler, s.f, s.branch.ID)
	assertNeedsRepair(t, got, s.second)
	if got.Pending[0].EntityName == "" || got.Pending[0].EntityType != "person" {
		t.Errorf("pending = %+v, want the person named", got.Pending[0])
	}
	// Read-only: asking again moved nothing and repaired nothing.
	assertNeedsRepair(t, completenessOf(t, s.handler, s.f, s.branch.ID), s.second)
	if logHead(t, s.f) != head || mainSurnameOf(t, s.f, s.second) != "Hopper" {
		t.Error("the merge-state read wrote to the log or repaired main's read model")
	}

	result, err := s.handler.ResumeMerge(ctx, command.ResumeMergeInput{BranchID: s.branch.ID})
	if err != nil {
		t.Fatalf("ResumeMerge failed: %v", err)
	}
	if !slices.Equal(result.ReprojectedStreamIDs, []uuid.UUID{s.second}) {
		t.Errorf("ReprojectedStreamIDs = %v, want [%s] (what the state reported)", result.ReprojectedStreamIDs, s.second)
	}
	if got := completenessOf(t, s.handler, s.f, s.branch.ID); got.State != command.MergeStateComplete || len(got.Pending) != 0 {
		t.Errorf("after resume completeness = %+v, want complete", got)
	}
}

// TestMergeCompleteness_MediaProjectionFailed: the same for a media edit's
// projection (the case a resume reports in ReprojectedStreamIDs).
func TestMergeCompleteness_MediaProjectionFailed(t *testing.T) {
	e := newEvidenceResume(t)
	photo := e.upload(t, e.f.handler, e.person, "Portrait")
	branch, scoped := e.branch(t, "retitle")
	e.retitleMedia(t, scoped, domain.BranchID(branch.ID), photo, "Portrait, 1901")

	e.failProjection(t, branch, photo)
	assertNeedsRepair(t, completenessOf(t, e.f.handler, e.f, branch.ID), photo)

	res := e.resume(t, branch.ID, nil)
	if !slices.Equal(res.ReprojectedStreamIDs, []uuid.UUID{photo}) {
		t.Errorf("ReprojectedStreamIDs = %v, want [%s]", res.ReprojectedStreamIDs, photo)
	}
	if got := completenessOf(t, e.f.handler, e.f, branch.ID); got.State != command.MergeStateComplete {
		t.Errorf("after resume completeness = %+v, want complete", got)
	}
}

// TestMergeCompleteness_CitationCountBehind: a citation saved on main whose
// source's count bump failed is level by version, so only the recount a
// resume makes finds it. The state reports the citation as needing repair.
func TestMergeCompleteness_CitationCountBehind(t *testing.T) {
	e := newEvidenceResume(t)
	ctx := context.Background()
	cited := e.source(t, e.f.handler, "1880 Census")
	branch, scoped := e.branch(t, "count")
	cit := e.cite(t, scoped, cited.ID)

	e.reads.armed, e.reads.failSourceCount = true, cited.ID
	_, err := e.f.handler.MergeBranch(ctx, command.MergeBranchInput{BranchID: branch.ID})
	e.reads.armed, e.reads.failSourceCount = false, uuid.Nil
	if !errors.Is(err, command.ErrMergePartiallyApplied) {
		t.Fatalf("MergeBranch error = %v, want ErrMergePartiallyApplied", err)
	}

	assertNeedsRepair(t, completenessOf(t, e.f.handler, e.f, branch.ID), cit.ID)
	if got := e.mainSource(t, cited.ID); got.CitationCount != 0 {
		t.Errorf("citation_count = %d after the read, want 0 (the read recounts nothing)", got.CitationCount)
	}

	res := e.resume(t, branch.ID, nil)
	if !slices.Equal(res.ReprojectedStreamIDs, []uuid.UUID{cit.ID}) {
		t.Errorf("ReprojectedStreamIDs = %v, want [%s]", res.ReprojectedStreamIDs, cit.ID)
	}
	if got := completenessOf(t, e.f.handler, e.f, branch.ID); got.State != command.MergeStateComplete {
		t.Errorf("after resume completeness = %+v, want complete", got)
	}
}

// TestMergeCompleteness_ReplayLeftAndRepairNeeded: when streams are still to
// replay AND a landed stream's projection failed, both are pending, in replay
// order, each with its own reason.
func TestMergeCompleteness_ReplayLeftAndRepairNeeded(t *testing.T) {
	s := seedResume(t)
	ctx := context.Background()

	// The first stream lands in the log but fails to project; the second
	// never lands.
	s.reads.armed, s.reads.failPerson = true, s.first
	_, err := s.handler.MergeBranch(ctx, command.MergeBranchInput{BranchID: s.branch.ID})
	s.reads.armed, s.reads.failPerson = false, uuid.Nil
	if !errors.Is(err, command.ErrMergePartiallyApplied) {
		t.Fatalf("MergeBranch error = %v, want ErrMergePartiallyApplied", err)
	}

	got := completenessOf(t, s.handler, s.f, s.branch.ID)
	if got.State != command.MergeStateIncomplete || len(got.Pending) != 2 {
		t.Fatalf("completeness = %+v, want both persons pending", got)
	}
	if got.Pending[0].StreamID != s.first || got.Pending[0].Reason != command.PendingNeedsRepair {
		t.Errorf("pending[0] = %+v, want the first person needing repair", got.Pending[0])
	}
	if got.Pending[1].StreamID != s.second || got.Pending[1].Reason != command.PendingReady {
		t.Errorf("pending[1] = %+v, want the second person ready to replay", got.Pending[1])
	}

	if _, err := s.handler.ResumeMerge(ctx, command.ResumeMergeInput{BranchID: s.branch.ID}); err != nil {
		t.Fatalf("ResumeMerge failed: %v", err)
	}
	if got := completenessOf(t, s.handler, s.f, s.branch.ID); got.State != command.MergeStateComplete {
		t.Errorf("after resume completeness = %+v, want complete", got)
	}
}

// TestMergeCompleteness_CompleteVerdictIsRememberedUntilTheLogMoves: a
// finished merge is remembered against the event log's head, so reading it
// again does not redo the work; any append anywhere invalidates that, and the
// next read recomputes it from the log and the read model.
func TestMergeCompleteness_CompleteVerdictIsRememberedUntilTheLogMoves(t *testing.T) {
	s := seedResume(t)
	ctx := context.Background()
	if _, err := s.handler.MergeBranch(ctx, command.MergeBranchInput{BranchID: s.branch.ID}); err != nil {
		t.Fatalf("MergeBranch failed: %v", err)
	}
	if got := completenessOf(t, s.handler, s.f, s.branch.ID); got.State != command.MergeStateComplete {
		t.Fatalf("completeness = %+v, want complete", got)
	}

	// Knock the first person's main row behind the log WITHOUT an append.
	person, err := s.f.readStore.GetPerson(ctx, domain.MainBranchID, s.first)
	if err != nil || person == nil {
		t.Fatalf("GetPerson = %v, %v", person, err)
	}
	person.Version = 1
	if err := s.f.readStore.SavePerson(ctx, domain.MainBranchID, person); err != nil {
		t.Fatalf("SavePerson failed: %v", err)
	}
	if got := completenessOf(t, s.handler, s.f, s.branch.ID); got.State != command.MergeStateComplete {
		t.Errorf("completeness = %+v, want the remembered complete verdict while the log has not moved", got)
	}

	// A write anywhere moves the head; the next read looks again.
	if _, err := s.handler.CreatePerson(ctx, command.CreatePersonInput{GivenName: "Mary", Surname: "Somerville"}); err != nil {
		t.Fatalf("CreatePerson failed: %v", err)
	}
	assertNeedsRepair(t, completenessOf(t, s.handler, s.f, s.branch.ID), s.first)

	// A handler without a head reader never remembers anything.
	uncached := command.NewHandlerWithBranchStore(s.f.eventStore, s.f.readStore, s.f.branchStore)
	assertNeedsRepair(t, completenessOf(t, uncached, s.f, s.branch.ID), s.first)
}

// TestMergeCompleteness_ScansMainsLogOnce: an uncached merge-state read scans
// main's log for the replay set's landed streams ONCE, whether streams are
// still left to replay (the resume's plan and the repair check share the scan)
// or not.
func TestMergeCompleteness_ScansMainsLogOnce(t *testing.T) {
	s := seedResume(t)
	ctx := context.Background()
	s.interruptSecondStream(t, command.MergeBranchInput{BranchID: s.branch.ID})

	// No head reader, so nothing is remembered and every read recomputes.
	uncached := command.NewHandlerWithBranchStore(s.faulty, s.reads, s.f.branchStore)
	scansOf := func() int {
		t.Helper()
		s.faulty.recordScans, s.faulty.mainScans = true, nil
		completenessOf(t, uncached, s.f, s.branch.ID)
		s.faulty.recordScans = false
		return len(s.faulty.mainScans)
	}

	if got := scansOf(); got != 1 {
		t.Errorf("incomplete merge: %d scans of main's log, want 1: %v", got, s.faulty.mainScans)
	}
	if _, err := s.handler.ResumeMerge(ctx, command.ResumeMergeInput{BranchID: s.branch.ID}); err != nil {
		t.Fatalf("ResumeMerge failed: %v", err)
	}
	if got := scansOf(); got != 1 {
		t.Errorf("complete merge: %d scans of main's log, want 1: %v", got, s.faulty.mainScans)
	}
}

// TestMergeCompleteness_CitationCountsReadInBatch: the citation-count check
// reads the merge's cited sources and their counts in a fixed number of
// statements, however many sources the merge cites — not two per source.
func TestMergeCompleteness_CitationCountsReadInBatch(t *testing.T) {
	e := newEvidenceResume(t)
	ctx := context.Background()
	branch, scoped := e.branch(t, "many sources")
	for _, title := range []string{"1850 Census", "1860 Census", "1870 Census", "1880 Census"} {
		e.cite(t, scoped, e.source(t, e.f.handler, title).ID)
	}
	if _, err := e.f.handler.MergeBranch(ctx, command.MergeBranchInput{BranchID: branch.ID}); err != nil {
		t.Fatalf("MergeBranch failed: %v", err)
	}

	uncached := command.NewHandlerWithBranchStore(e.faulty, e.reads, e.f.branchStore)
	e.reads.countSourceReads, e.reads.sourceReads = true, 0
	got := completenessOf(t, uncached, e.f, branch.ID)
	e.reads.countSourceReads = false
	if got.State != command.MergeStateComplete {
		t.Fatalf("completeness = %+v, want complete", got)
	}
	if e.reads.sourceReads != 2 {
		t.Errorf("source/citation-count reads = %d for 4 cited sources, want 2 (one batched read of each)", e.reads.sourceReads)
	}
}
