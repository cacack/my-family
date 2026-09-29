package command

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"

	"github.com/cacack/my-family/internal/domain"
	"github.com/cacack/my-family/internal/repository"
)

// GPS artifacts on resume (#760 on top of #685).
//
// An evidence analysis, evidence conflict, research log or proof summary
// stream is resumed like any other branch-aware stream. Its events are found
// on main by payload id; its main row is read with the artifact's Get (every
// GPS projection writes the row, version included, in one save, or deletes
// it), and a row behind main's log is re-projected from main's own events.
//
// Before a resume replays anything it applies the merge's three GPS rules
// (checkGPSSubjectSurvives, checkSubjectDeleteOrphansNoGPS, both through
// checkEvidence) with the resume's pending/decidable semantics: an
// auto-planned stream that breaks one is reported pending, and a "branch"
// decision for it is refused by the final check, so "main" rolls the merge
// forward without it.
//
// As with media (branch_merge_resume_media.go), the GPS-specific question is a
// MISSING main row. A GPS row can be removed by a write to ANOTHER stream: a
// PersonDeleted or FamilyDeleted of its subject cascades onto it without
// writing to the artifact's stream (missingGPSCascadedAway). Re-projecting it
// would resurrect research about nothing, so it counts as removed for a reason
// main's log explains. And PersonMerged re-points a merged person's artifacts
// to the survivor when it is projected, so a row still missing after such a
// merge means its own create projection failed BEFORE the merge, and
// re-projecting the stream alone would attach it to the merged-away person.
// As for media, the transfer the merge would have made is fully determined by
// main's log (the merges recorded since, followed to the final survivor) and
// is nothing more than re-pointing the row's subject — the one field
// PersonMerged changes on a GPS row, version and updated-at untouched. So the
// repair re-projects the stream and then re-links the row (relinkMergedGPS).
// If the survivor has itself been deleted since, the artifact is gone either
// way (the cascade), and nothing is re-projected.

// GPS artifact stream types, as the command layer writes them.
var gpsStreamTypes = []string{"EvidenceAnalysis", "EvidenceConflict", "ResearchLog", "ProofSummary"}

// isGPSStream reports whether a stream type is a GPS artifact's (#760).
func isGPSStream(streamType string) bool {
	for _, t := range gpsStreamTypes {
		if strings.EqualFold(streamType, t) {
			return true
		}
	}
	return false
}

// gpsRow is what a resume reads of a GPS artifact's main row.
type gpsRow struct {
	version   int64
	subjectID uuid.UUID
}

// mainGPSRow reads a GPS artifact's main row; found is false when main has
// none.
func (h *Handler) mainGPSRow(ctx context.Context, group streamGroup) (row gpsRow, found bool, err error) {
	switch {
	case strings.EqualFold(group.streamType, "EvidenceAnalysis"):
		var a *repository.EvidenceAnalysisReadModel
		if a, err = h.readStore.GetEvidenceAnalysis(ctx, domain.MainBranchID, group.streamID); a != nil {
			row, found = gpsRow{version: a.Version, subjectID: a.SubjectID}, true
		}
	case strings.EqualFold(group.streamType, "EvidenceConflict"):
		var c *repository.EvidenceConflictReadModel
		if c, err = h.readStore.GetEvidenceConflict(ctx, domain.MainBranchID, group.streamID); c != nil {
			row, found = gpsRow{version: c.Version, subjectID: c.SubjectID}, true
		}
	case strings.EqualFold(group.streamType, "ResearchLog"):
		var l *repository.ResearchLogReadModel
		if l, err = h.readStore.GetResearchLog(ctx, domain.MainBranchID, group.streamID); l != nil {
			row, found = gpsRow{version: l.Version, subjectID: l.SubjectID}, true
		}
	default: // isGPSStream admits only the four types
		var p *repository.ProofSummaryReadModel
		if p, err = h.readStore.GetProofSummary(ctx, domain.MainBranchID, group.streamID); p != nil {
			row, found = gpsRow{version: p.Version, subjectID: p.SubjectID}, true
		}
	}
	if err != nil {
		return gpsRow{}, false, fmt.Errorf("reading main %s %s: %w", group.streamType, group.streamID, err)
	}
	return row, found, nil
}

// mainGPSState is mainReadModelState for the GPS artifact streams.
func (h *Handler) mainGPSState(ctx context.Context, group streamGroup) (readModelState, error) {
	row, found, err := h.mainGPSRow(ctx, group)
	if err != nil || !found {
		return readModelState{}, err
	}
	return readModelState{present: true, version: row.version}, nil
}

// setMainGPSSubject re-points a GPS artifact's main row at subjectID, as
// PersonMerged's projection does: the subject only, version and updated-at
// untouched. A row already gone is left alone.
func (h *Handler) setMainGPSSubject(ctx context.Context, group streamGroup, subjectID uuid.UUID) error {
	main := domain.MainBranchID
	var err error
	switch {
	case strings.EqualFold(group.streamType, "EvidenceAnalysis"):
		var a *repository.EvidenceAnalysisReadModel
		if a, err = h.readStore.GetEvidenceAnalysis(ctx, main, group.streamID); err == nil && a != nil {
			a.SubjectID = subjectID
			err = h.readStore.SaveEvidenceAnalysis(ctx, main, a)
		}
	case strings.EqualFold(group.streamType, "EvidenceConflict"):
		var c *repository.EvidenceConflictReadModel
		if c, err = h.readStore.GetEvidenceConflict(ctx, main, group.streamID); err == nil && c != nil {
			c.SubjectID = subjectID
			err = h.readStore.SaveEvidenceConflict(ctx, main, c)
		}
	case strings.EqualFold(group.streamType, "ResearchLog"):
		var l *repository.ResearchLogReadModel
		if l, err = h.readStore.GetResearchLog(ctx, main, group.streamID); err == nil && l != nil {
			l.SubjectID = subjectID
			err = h.readStore.SaveResearchLog(ctx, main, l)
		}
	default: // isGPSStream admits only the four types
		var p *repository.ProofSummaryReadModel
		if p, err = h.readStore.GetProofSummary(ctx, main, group.streamID); err == nil && p != nil {
			p.SubjectID = subjectID
			err = h.readStore.SaveProofSummary(ctx, main, p)
		}
	}
	if err != nil {
		return fmt.Errorf("re-linking main %s %s to merge survivor %s: %w", group.streamType, group.streamID, subjectID, err)
	}
	return nil
}

// missingGPSCascadedAway reports which missing GPS artifact rows were removed
// by their subject's delete cascade: the subject the stream's main log last
// set — followed through any person merges main recorded since — ends in a
// delete on main. subjectOf holds the candidates with their subjects
// (missingGPSSubjects), and mergeOf the person merges main recorded since
// the earliest of them (survivorsAfter), scanned from scanFrom. A subject may
// be a family, which no merge moves; following a family id through the person
// merges leaves it where it is.
//
// It also reports, for each candidate NOT cascaded away whose subject person
// main merged into a person it still has, the transfer its re-projection must
// be followed by (see the note above). Any other candidate is reported as not
// removed; the resume's own checks decide whether its replay is sound
// (checkGPSSubjectSurvives flags an edit of a missing artifact).
//
// The work is set-based: the caller's one paged scan of main for person
// merges (shared with the person and media checks), then one paged scan of
// the final subjects' streams — never a scan per artifact.
func (h *Handler) missingGPSCascadedAway(
	ctx context.Context,
	missing []streamGroup,
	subjectOf map[uuid.UUID]uuid.UUID,
	mergeOf map[uuid.UUID]personMerge,
	scanFrom int64,
) (map[uuid.UUID]bool, map[uuid.UUID]mergeRelink, error) {
	if len(subjectOf) == 0 {
		return nil, nil, nil
	}
	finalOf := make(map[uuid.UUID]uuid.UUID, len(subjectOf))
	mergedAtOf := make(map[uuid.UUID]time.Time, len(subjectOf))
	var finals []uuid.UUID
	for artifactID, subjectID := range subjectOf {
		final, mergedAt := finalSurvivor(subjectID, mergeOf)
		finalOf[artifactID], mergedAtOf[artifactID] = final, mergedAt
		finals = appendUnique(finals, final)
	}
	subjectEvents, err := h.readMainStreams(ctx, finals)
	if err != nil {
		return nil, nil, err
	}

	cascaded := make(map[uuid.UUID]bool)
	relink := make(map[uuid.UUID]mergeRelink)
	for _, group := range missing {
		subjectID, ok := subjectOf[group.streamID]
		if !ok {
			continue
		}
		final := finalOf[group.streamID]
		switch {
		case endsInDelete(subjectEvents[final]):
			cascaded[group.streamID] = true
		case final != subjectID:
			relink[group.streamID] = mergeRelink{
				from: subjectID, target: final, mergedAt: mergedAtOf[group.streamID], scanFrom: scanFrom,
			}
		}
	}
	return cascaded, relink, nil
}

// relinkMergedGPS makes, on main's re-projected row of a GPS artifact, the
// subject transfer the person merges main recorded since its creation would
// have made (see mergeRelink): the row's subject is re-pointed to the final
// survivor, exactly as PersonMerged re-points a merged person's artifacts.
//
// Only a row whose subject is still the merged-away person is re-pointed, so a
// racing main update that re-pointed the artifact elsewhere is kept. As for
// media, a merge of the survivor recorded while this ran would move the row
// only if its projection saw it re-pointed, so the scan for merges is repeated
// after each save, and the row followed to any new survivor, until a scan
// finds nothing new. A row already gone is left alone; reprojectStream reports
// that.
func (h *Handler) relinkMergedGPS(ctx context.Context, group streamGroup, relink mergeRelink) error {
	for attempt := 0; attempt < reprojectAttempts; attempt++ {
		row, found, err := h.mainGPSRow(ctx, group)
		if err != nil {
			return err
		}
		if !found {
			return nil
		}
		if row.subjectID == relink.from {
			if err := h.setMainGPSSubject(ctx, group, relink.target); err != nil {
				return err
			}
		}

		merges, err := h.personMergesOnMain(ctx, relink.scanFrom)
		if err != nil {
			return err
		}
		target, _ := finalSurvivor(relink.target, survivorsAfter(merges, relink.scanFrom))
		if target == relink.target {
			return nil
		}
		relink.from, relink.target = relink.target, target
	}
	return fmt.Errorf("%w: %s %s's subject kept being merged while the resume re-linked it; resume again to finish",
		errReprojectRaced, group.streamType, group.streamID)
}

// missingGPSSubjects returns the subject each missing GPS row's main log last
// set, for the rows missingGPSCascadedAway considers — GPS streams whose row
// is missing and whose stream does not itself end in a delete — and the
// position a scan for person merges must start from (-1 when there are none).
func missingGPSSubjects(
	missing []streamGroup,
	states map[uuid.UUID]readModelState,
	mainEvents map[uuid.UUID][]repository.StoredEvent,
) (map[uuid.UUID]uuid.UUID, int64, error) {
	subjectOf := make(map[uuid.UUID]uuid.UUID)
	scanFrom := int64(-1)
	for _, group := range missing {
		events := mainEvents[group.streamID]
		if states[group.streamID].present || len(events) == 0 || !isGPSStream(group.streamType) || endsInDelete(events) {
			continue
		}
		outcome, err := gpsOutcomeOf(streamGroup{streamID: group.streamID, streamType: group.streamType, events: events})
		if err != nil {
			return nil, 0, err
		}
		if !outcome.subjectSet {
			continue
		}
		subjectOf[group.streamID] = outcome.subjectID
		if first := events[0].Position; scanFrom < 0 || first < scanFrom {
			scanFrom = first
		}
	}
	return subjectOf, scanFrom, nil
}

// checkLandedGPSSubjects is the GPS half of validateResumeEvidence's
// landed-stream check, and follows checkLandedMediaOwners: a GPS artifact
// already on main may not be left about a subject main does not have because
// this call resolves that subject's stream to "main" (skipping it) — a stream
// that creates the subject, or one that creates and deletes it, whose delete
// would have cascaded the artifact away on main as it did on the branch.
//
// The subject checked is the one main's row now names (a person merge on main
// re-points the artifact to the survivor); with no main row, it is the one the
// stream last set — or, when main has merged that person away since, the final
// survivor (view.relinked), since a resume's repair would re-project the
// artifact there — unless main's log explains the row's absence
// (view.removed: main deleted the artifact, or deleted a subject it had and
// cascaded it), which leaves nothing to orphan. Decisions recorded earlier
// were checked when they were made. A refused caller can resolve the
// subject's stream to branch, or delete the artifact on main first.
func (h *Handler) checkLandedGPSSubjects(
	ctx context.Context,
	groups []streamGroup,
	view resumeView,
	resolutions map[uuid.UUID]MergeResolution,
	list *blockerList,
) error {
	for _, group := range groups {
		if !view.landed[group.streamID] || view.removed[group.streamID] || !isGPSStream(group.streamType) {
			continue
		}
		outcome, err := gpsOutcomeOf(group)
		if err != nil {
			return err
		}
		if !outcome.subjectSet || outcome.deleted {
			continue
		}
		subjectID := outcome.subjectID
		row, found, err := h.mainGPSRow(ctx, group)
		if err != nil {
			return err
		}
		switch survivor, relinked := view.relinked[group.streamID]; {
		case found:
			subjectID = row.subjectID
		case relinked:
			subjectID = survivor
		}
		if resolutions[subjectID] != ResolveMain {
			continue
		}
		exists, err := h.gpsSubjectOnMain(ctx, subjectID)
		if err != nil {
			return err
		}
		if !exists {
			// A "main" resolution names a stream of the replay set, whose type
			// says which the subject is.
			subjectType := "person"
			for _, g := range groups {
				if g.streamID == subjectID {
					subjectType = entityTypeOfStream(g.streamType)
					break
				}
			}
			b := streamBlocker(group, BlockerMissingGPSSubject, subjectID, subjectType,
				"%s %s is already on main and is about subject %s, which main does not have; "+
					"resolving that subject to main would leave the research orphaned — resolve it to branch instead, "+
					"or delete the %s on main first",
				group.streamType, group.streamID, subjectID, group.streamType)
			b.SuggestedResolution = FixIncludeReferenced
			list.add(b)
		}
	}
	return nil
}
