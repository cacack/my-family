package command

import (
	"context"
	"errors"
	"fmt"
	"unicode/utf8"

	"github.com/cacack/my-family/internal/domain"
	"github.com/cacack/my-family/internal/query"
)

// The pre-merge snapshot's name is "Before merging <branch>" (#833).
const (
	preMergeSnapshotPrefix = "Before merging "
	// preMergeSnapshotNameMax mirrors domain.Snapshot.Validate's name limit,
	// counted in bytes as the domain counts it.
	preMergeSnapshotNameMax = 100
	ellipsis                = "…"
)

// preMergeSnapshotName names the snapshot a merge takes before it claims the
// branch. A branch name may be as long as a snapshot name may, so the prefix
// can push it over the limit; the branch name is then shortened on a rune
// boundary and marked with an ellipsis. The description carries it whole.
func preMergeSnapshotName(branchName string) string {
	name := preMergeSnapshotPrefix + branchName
	if len(name) <= preMergeSnapshotNameMax {
		return name
	}
	budget := preMergeSnapshotNameMax - len(preMergeSnapshotPrefix) - len(ellipsis)
	cut := 0
	for i, r := range branchName {
		if i+utf8.RuneLen(r) > budget {
			break
		}
		cut = i + utf8.RuneLen(r)
	}
	return preMergeSnapshotPrefix + branchName[:cut] + ellipsis
}

// snapshotAndClaim takes the pre-merge snapshot when the input asks for one,
// records it on the claim, and then runs the staleness check and the claim
// (claimFreshMerge). A merge refused by either leaves no snapshot behind (see
// discardPreMergeSnapshot). It returns the snapshot, nil when none was asked
// for.
func (h *Handler) snapshotAndClaim(
	ctx context.Context,
	branch *domain.Branch,
	claim *domain.BranchMerged,
	plan *query.MergePlan,
	groups []streamGroup,
	input MergeBranchInput,
) (*domain.Snapshot, error) {
	preMerge, err := h.takePreMergeSnapshot(ctx, branch, input.SnapshotBefore)
	if err != nil {
		return nil, err
	}
	if preMerge != nil {
		snapshotID := preMerge.ID
		claim.PreMergeSnapshotID = &snapshotID
	}
	if err := h.claimFreshMerge(ctx, branch, *claim, plan, groups, input.Resolutions); err != nil {
		return nil, h.discardPreMergeSnapshot(ctx, preMerge, err)
	}
	return preMerge, nil
}

// takePreMergeSnapshot marks the mainline as it stands before a merge replays
// anything onto it (#833), through the event-sourced snapshot command, so the
// merge's effect can be compared from it afterwards. It returns nil when the
// caller did not ask for one.
//
// WHERE IT RUNS: after the merge plan, the claim and the merge record are
// built — every read the merge makes before its staleness check — and before
// that check and the claim. Taking it any later would put a write between the
// staleness check and the claim, which claimFreshMerge forbids; taking it any
// earlier would only widen the window in which an unrelated mainline write can
// land between the snapshot and the first replayed event.
//
// RACES: without a transaction across the snapshot, the claim and the replay,
// the snapshot is "exactly before the merge" only for writes the merge itself
// guards. A concurrent mainline write to a stream the merge replays is caught
// by the staleness check (the merge is refused and the snapshot discarded) or
// by the replay's per-stream assertion (the partially-applied state, where the
// snapshot is kept: it still marks the mainline before the merge). A
// concurrent write to an unrelated entity can land between the snapshot and
// the replay; it then appears in the snapshot's comparison, without the merge
// provenance every replayed change carries, which is how the comparison tells
// the two apart. The snapshot's own marker and the branch's claim sit in that
// range too, but neither is a mainline change: snapshot markers are left out
// of every change log and the claim is on the branch's envelope.
func (h *Handler) takePreMergeSnapshot(ctx context.Context, branch *domain.Branch, take bool) (*domain.Snapshot, error) {
	if !take {
		return nil, nil
	}
	snapshot, err := h.createSnapshotOn(ctx, domain.MainBranchID, preMergeSnapshotName(branch.Name),
		fmt.Sprintf("Taken automatically before merging the research branch %q into the mainline.", branch.Name))
	if err != nil {
		return nil, fmt.Errorf("taking the pre-merge snapshot: %w", err)
	}
	return snapshot, nil
}

// discardPreMergeSnapshot removes the pre-merge snapshot of a merge that was
// refused before it claimed the branch, so a refused merge leaves no
// "Before merging" snapshot behind for a merge that never happened (and a
// retry does not stack a second one beside it). The log keeps both events
// (ES-002); only the registry row goes. cause is the refusal, returned as-is
// when the discard succeeds, so callers still classify it with errors.Is.
func (h *Handler) discardPreMergeSnapshot(ctx context.Context, snapshot *domain.Snapshot, cause error) error {
	if snapshot == nil {
		return cause
	}
	if err := h.deleteSnapshotOn(ctx, domain.MainBranchID, snapshot.ID); err != nil {
		return errors.Join(cause, fmt.Errorf("removing the pre-merge snapshot %s of the refused merge: %w", snapshot.ID, err))
	}
	return cause
}
