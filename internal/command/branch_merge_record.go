package command

import (
	"context"
	"fmt"

	"github.com/google/uuid"

	"github.com/cacack/my-family/internal/domain"
	"github.com/cacack/my-family/internal/query"
)

// buildMergeClaim builds the BranchMerged claim: the replay plan and the merge
// record (#832). It is built before the claim, while the branch's overlay still
// names every entity it touched, and before the staleness check, so that no
// read happens between that check and claimMerge.
func (h *Handler) buildMergeClaim(
	ctx context.Context,
	branch *domain.Branch,
	mergedAtPosition int64,
	input MergeBranchInput,
	plan *query.MergePlan,
	groups []streamGroup,
	rationales map[uuid.UUID]string,
) (domain.BranchMerged, error) {
	claim := domain.NewBranchMerged(branch.ID, branch.BasePosition, mergedAtPosition, input.Note,
		replayPlan(groups, plan.MainStreamVersions, input.Resolutions))
	if err := h.recordMergeDecisions(ctx, &claim, plan, groups, input.Resolutions, rationales); err != nil {
		return domain.BranchMerged{}, err
	}
	return claim, nil
}

// recordMergeDecisions writes the merge record (#832) onto the claim: every
// conflict decision with the conflict as it was reviewed, every entity left
// behind without a conflict, how many events the merge sets out to replay,
// and which entities it skips. Every rationale is kept in
// ResolutionRationales as before (#828), and also rides on the decision or
// exclusion it explains.
//
// Conflicts arrive named by PlanMerge. The exclusions are named here, in one
// batched lookup through the branch's overlay, because after the merge the
// overlay is purged and an entity that existed only on the branch and was left
// behind would have nothing left to name it but its log.
func (h *Handler) recordMergeDecisions(
	ctx context.Context,
	claim *domain.BranchMerged,
	plan *query.MergePlan,
	groups []streamGroup,
	resolutions map[uuid.UUID]MergeResolution,
	rationales map[uuid.UUID]string,
) error {
	conflicted := make(map[uuid.UUID]bool, len(plan.Conflicts))
	decisions := make([]domain.MergeDecision, 0, len(plan.Conflicts))
	for _, conflict := range plan.Conflicts {
		conflicted[conflict.StreamID] = true
		decisions = append(decisions, domain.MergeDecision{
			StreamID:   conflict.StreamID,
			EntityType: conflict.EntityType,
			EntityName: conflict.EntityName,
			Kind:       string(conflict.Kind),
			Fields:     append([]string(nil), conflict.Fields...),
			DeletedBy:  conflict.DeletedBy,
			Resolution: string(resolutions[conflict.StreamID]),
			Rationale:  rationales[conflict.StreamID],
		})
	}

	var (
		replayed  int
		skipped   []uuid.UUID
		excluded  []domain.MergeExclusion
		refs      []query.EntityRef
		refByID   = make(map[uuid.UUID]query.EntityRef)
		leftOutOf = func(group streamGroup) bool { return resolutions[group.streamID] == ResolveMain }
	)
	for _, group := range groups {
		if !leftOutOf(group) {
			replayed += len(group.events)
			continue
		}
		skipped = append(skipped, group.streamID)
		if conflicted[group.streamID] {
			continue
		}
		ref := query.EntityRef{EntityType: query.EntityTypeOfStream(group.streamType), ID: group.streamID}
		refs = append(refs, ref)
		refByID[group.streamID] = ref
		excluded = append(excluded, domain.MergeExclusion{
			StreamID:   group.streamID,
			EntityType: ref.EntityType,
			Rationale:  rationales[group.streamID],
		})
	}
	if len(refs) > 0 {
		names, err := h.branchService.NameEntities(ctx, domain.BranchID(claim.BranchID), refs)
		if err != nil {
			return fmt.Errorf("naming the entities left out of the merge: %w", err)
		}
		for i := range excluded {
			excluded[i].EntityName = names[refByID[excluded[i].StreamID]]
		}
	}

	claim.ResolutionRationales = rationales
	claim.Resolutions = decisions
	claim.Exclusions = excluded
	claim.ReplayedEventCount = &replayed
	claim.SkippedStreamIDs = skipped
	return nil
}

// mergeProvenance is the provenance every event a merge replays is stamped
// with (#832). It is built from the branch and the claim alone, so the merge
// and every resume of it stamp identically.
func mergeProvenance(branch *domain.Branch, claim domain.BranchMerged) *domain.MergeProvenance {
	return &domain.MergeProvenance{
		BranchID:         branch.ID,
		BranchName:       branch.Name,
		ClaimID:          claim.ID,
		MergedAtPosition: claim.MergedAtPosition,
		MergedAt:         claim.Timestamp,
		Note:             claim.Note,
	}
}
