package command

import (
	"context"
	"fmt"

	"github.com/google/uuid"

	"github.com/cacack/my-family/internal/domain"
	"github.com/cacack/my-family/internal/repository"
)

// missingCitationsCascadedAway reports which missing citation rows were
// removed by their source's delete cascade: the source the citation's main log
// last has it citing ends in a delete on main. The store's source→citation
// cascade (#758) removes the citation row without writing to the citation's
// stream, so this is a removal the log explains, like an association losing a
// person — and re-projecting the citation would resurrect an orphan.
//
// Only citations whose row is missing and whose stream does not itself end in
// a delete are candidates, and the main histories of all their final sources
// are read in ONE set-based paged scan, not a scan per citation.
func (h *Handler) missingCitationsCascadedAway(
	ctx context.Context,
	missing []streamGroup,
	states map[uuid.UUID]readModelState,
	mainEvents map[uuid.UUID][]repository.StoredEvent,
) (map[uuid.UUID]bool, error) {
	sourceOf := make(map[uuid.UUID]uuid.UUID)
	var sourceIDs []uuid.UUID
	for _, group := range missing {
		events := mainEvents[group.streamID]
		if states[group.streamID].present || len(events) == 0 || !isCitationStream(group.streamType) || endsInDelete(events) {
			continue
		}
		outcome, err := citationOutcomeOf(streamGroup{streamID: group.streamID, events: events})
		if err != nil {
			return nil, err
		}
		if !outcome.repointed {
			continue
		}
		sourceOf[group.streamID] = outcome.sourceID
		sourceIDs = appendUnique(sourceIDs, outcome.sourceID)
	}
	if len(sourceIDs) == 0 {
		return nil, nil
	}
	sourceEvents, err := h.readMainStreams(ctx, sourceIDs)
	if err != nil {
		return nil, err
	}
	cascaded := make(map[uuid.UUID]bool)
	for citationID, sourceID := range sourceOf {
		if endsInDelete(sourceEvents[sourceID]) {
			cascaded[citationID] = true
		}
	}
	return cascaded, nil
}

// reconcileCitationCounts recounts, on main, the citation_count of every
// source a citation stream of the replay set that is on main (onMain) has ever
// cited, and of every such source stream, and corrects any that disagree with
// the number of main citations of that source. ResumeMerge runs it once the
// replay is done. It returns the streams whose sources it corrected (the
// citation streams naming the source, and the source stream itself when it is
// one), which ResumeMerge reports as re-projected.
//
// Why a recount rather than the version rule reprojectLandedStreams applies:
// a citation projection steps its source's count in a SEPARATE save from the
// citation row. CitationCreated saves the citation (version included) and only
// then bumps the count, so a failure between the two leaves a citation row
// that is level with the log over a count that is one short — invisible to a
// version check. CitationUpdated (a re-point) and CitationDeleted step the
// counts before their own save, so re-projecting one that failed at its save
// steps them a second time. Neither is idempotent, so neither is repaired by
// replaying events. The count is a pure derivative of main's citation rows,
// though, so setting it absolutely is correct whatever happened before — the
// same reason the family's child count is set absolutely.
//
// Running after the replay, and setting the count absolutely, also makes it
// right when the log holds a citation that landed before its source (the
// evidence order prevents that, but a claim written before #685 cannot prove
// it): the citation's projection found no source to bump, and the recount
// supplies it.
//
// Every citation stream on main has its sources checked, not only the streams
// found behind, because the CitationCreated shape above leaves nothing behind
// to find. The cost is one set-based scan of those citation streams, then two
// read-model reads per distinct source; nothing is written for a source whose
// count already agrees, so a resume of a completed merge writes nothing.
func (h *Handler) reconcileCitationCounts(ctx context.Context, groups []streamGroup, onMain map[uuid.UUID]bool) ([]uuid.UUID, error) {
	sources, namedBy, err := h.citedSourcesOnMain(ctx, groups, onMain)
	if err != nil {
		return nil, err
	}
	var corrected []uuid.UUID
	for _, sourceID := range sources {
		changed, err := h.reconcileCitationCount(ctx, sourceID)
		if err != nil {
			return nil, err
		}
		if !changed {
			continue
		}
		corrected = appendUnique(corrected, namedBy[sourceID]...)
	}
	return corrected, nil
}

// citationCountsOff is the read half of reconcileCitationCounts: the streams
// whose sources' citation counts a resume would correct. It WRITES NOTHING,
// so the merge-completeness read (MergeCompleteness, #830) shares it. Unlike
// the recount, which reads and writes one source at a time, it reads every
// cited source and every count in two batched statements whatever the number
// of sources, because it runs on every uncached read of a merged branch.
func (h *Handler) citationCountsOff(ctx context.Context, groups []streamGroup, onMain map[uuid.UUID]bool) ([]uuid.UUID, error) {
	sources, namedBy, err := h.citedSourcesOnMain(ctx, groups, onMain)
	if err != nil || len(sources) == 0 {
		return nil, err
	}
	rows, err := h.readStore.GetSourcesByIDs(ctx, domain.MainBranchID, sources)
	if err != nil {
		return nil, fmt.Errorf("reading main sources: %w", err)
	}
	recorded := make(map[uuid.UUID]int, len(rows))
	for i := range rows {
		recorded[rows[i].ID] = rows[i].CitationCount
	}
	counts, err := h.readStore.CountCitationsBySource(ctx, domain.MainBranchID, sources)
	if err != nil {
		return nil, fmt.Errorf("counting main citations of the merge's sources: %w", err)
	}
	var off []uuid.UUID
	for _, sourceID := range sources {
		count, present := recorded[sourceID]
		if !present {
			continue // main has no such source: nothing to count
		}
		if count != counts[sourceID] {
			off = appendUnique(off, namedBy[sourceID]...)
		}
	}
	return off, nil
}

// citedSourcesOnMain lists, in first-seen order, every source whose citation
// count reconcileCitationCounts checks (see there), with the replay-set
// streams that name each one.
func (h *Handler) citedSourcesOnMain(ctx context.Context, groups []streamGroup, onMain map[uuid.UUID]bool) ([]uuid.UUID, map[uuid.UUID][]uuid.UUID, error) {
	var (
		sources     []uuid.UUID
		namedBy     = make(map[uuid.UUID][]uuid.UUID)
		seen        = make(map[uuid.UUID]bool)
		citationIDs []uuid.UUID
	)
	addSource := func(sourceID, streamID uuid.UUID) {
		if !seen[sourceID] {
			seen[sourceID] = true
			sources = append(sources, sourceID)
		}
		namedBy[sourceID] = appendUnique(namedBy[sourceID], streamID)
	}
	for _, group := range groups {
		if !onMain[group.streamID] {
			continue
		}
		switch {
		case isSourceStream(group.streamType):
			addSource(group.streamID, group.streamID)
		case isCitationStream(group.streamType):
			citationIDs = append(citationIDs, group.streamID)
		}
	}
	if len(citationIDs) > 0 {
		// Main's whole history of each stream, not just the replayed events:
		// an update or delete also steps the source the citation cited
		// BEFORE the branch touched it.
		mainEvents, err := h.readMainStreams(ctx, citationIDs)
		if err != nil {
			return nil, nil, err
		}
		for _, citationID := range citationIDs {
			for _, evt := range mainEvents[citationID] {
				sourceID, sets, err := citedSource(evt)
				if err != nil {
					return nil, nil, err
				}
				if sets {
					addSource(sourceID, citationID)
				}
			}
		}
	}
	return sources, namedBy, nil
}

// reconcileCitationCount sets one main source's citation_count to the number of
// main citations of it, reporting whether it had to change anything. A source
// main does not have has nothing to count.
//
// Like reprojectStream it takes no lock, so it re-checks after writing: a
// racing citation projection may step the count between the recount and the
// save, and a racing SourceUpdated projected in that window would be rolled
// back by the save (fields and version). The first shows up as a count that
// disagrees again and is recounted; the second as a row behind main's log for
// the source, which is re-projected forward from the row's version — a
// SourceUpdated re-reads the row, so the count set here survives it. A source
// still unsettled after reprojectAttempts is left for the next resume.
func (h *Handler) reconcileCitationCount(ctx context.Context, sourceID uuid.UUID) (bool, error) {
	changed := false
	for attempt := 0; attempt < reprojectAttempts; attempt++ {
		source, err := h.readStore.GetSource(ctx, domain.MainBranchID, sourceID)
		if err != nil {
			return changed, fmt.Errorf("reading main source %s: %w", sourceID, err)
		}
		if source == nil {
			return changed, nil
		}
		citations, err := h.readStore.GetCitationsForSource(ctx, domain.MainBranchID, sourceID)
		if err != nil {
			return changed, fmt.Errorf("counting main citations of source %s: %w", sourceID, err)
		}
		if source.CitationCount == len(citations) {
			return changed, nil
		}
		source.CitationCount = len(citations)
		if err := h.readStore.SaveSource(ctx, domain.MainBranchID, source); err != nil {
			return changed, fmt.Errorf("correcting citation count of main source %s: %w", sourceID, err)
		}
		changed = true
		if err := h.levelSourceWithLog(ctx, sourceID); err != nil {
			return changed, err
		}
	}
	return changed, fmt.Errorf("%w: source %s's citation count kept moving while the resume recounted it; resume again to finish",
		errReprojectRaced, sourceID)
}

// levelSourceWithLog re-projects a main source row that is behind main's log
// for its stream — the trace a racing SourceUpdated leaves when the recount's
// save overwrote it.
func (h *Handler) levelSourceWithLog(ctx context.Context, sourceID uuid.UUID) error {
	group := streamGroup{streamID: sourceID, streamType: "Source"}
	head, err := h.eventStore.GetStreamVersion(ctx, sourceID, domain.MainBranchID)
	if err != nil {
		return fmt.Errorf("getting main stream version for %s: %w", sourceID, err)
	}
	state, err := h.mainReadModelState(ctx, group)
	if err != nil {
		return err
	}
	if !state.present || state.version >= head {
		return nil
	}
	events, err := h.readMainStreams(ctx, []uuid.UUID{sourceID})
	if err != nil {
		return err
	}
	return h.reprojectStream(ctx, group, events[sourceID], nil)
}

// appendUnique appends each id not already in ids, keeping order.
func appendUnique(ids []uuid.UUID, more ...uuid.UUID) []uuid.UUID {
	for _, id := range more {
		present := false
		for _, have := range ids {
			if have == id {
				present = true
				break
			}
		}
		if !present {
			ids = append(ids, id)
		}
	}
	return ids
}
