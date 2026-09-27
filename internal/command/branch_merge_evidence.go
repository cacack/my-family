package command

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/google/uuid"

	"github.com/cacack/my-family/internal/domain"
	"github.com/cacack/my-family/internal/repository"
)

// citationOutcome is what the replay of one stream leaves a citation citing.
type citationOutcome struct {
	// deleted is true when the stream ends with CitationDeleted.
	deleted bool
	// repointed is true when the stream sets the citation's source (a create,
	// or an update carrying source_id); sourceID is then the last one set.
	repointed bool
	sourceID  uuid.UUID
}

// citationOutcomeOf folds a stream's citation events into the source the
// citation ends up citing. Events that are not citation events are ignored.
func citationOutcomeOf(group streamGroup) (citationOutcome, error) {
	var out citationOutcome
	for i := range group.events {
		evt := group.events[i]
		if evt.EventType == "CitationDeleted" {
			out.deleted = true
			continue
		}
		sourceID, sets, err := citedSource(evt)
		if err != nil {
			return out, err
		}
		if !sets {
			continue
		}
		if evt.EventType == "CitationCreated" {
			out = citationOutcome{}
		}
		out.repointed, out.sourceID = true, sourceID
	}
	return out, nil
}

// citedSource returns the source one citation event makes the citation cite:
// a CitationCreated's source, or the source_id a CitationUpdated sets. sets is
// false for any other event, and for an update that leaves the source alone.
func citedSource(evt repository.StoredEvent) (sourceID uuid.UUID, sets bool, err error) {
	switch evt.EventType {
	case "CitationCreated":
		var payload struct {
			SourceID uuid.UUID `json:"source_id"`
		}
		if err := json.Unmarshal(evt.Data, &payload); err != nil {
			return uuid.Nil, false, fmt.Errorf("decoding citation create on stream %s: %w", evt.StreamID, err)
		}
		return payload.SourceID, true, nil
	case "CitationUpdated":
		var payload struct {
			Changes map[string]any `json:"changes"`
		}
		if err := json.Unmarshal(evt.Data, &payload); err != nil {
			return uuid.Nil, false, fmt.Errorf("decoding citation update on stream %s: %w", evt.StreamID, err)
		}
		raw, ok := payload.Changes["source_id"].(string)
		if !ok {
			return uuid.Nil, false, nil
		}
		id, err := uuid.Parse(raw)
		if err != nil {
			return uuid.Nil, false, fmt.Errorf("decoding citation update on stream %s: source_id %q: %w", evt.StreamID, raw, err)
		}
		return id, true, nil
	}
	return uuid.Nil, false, nil
}

// orderEvidenceForReplay reorders the replay's stream groups so that citation
// replay never runs against a source in the wrong state (#758).
//
// The replay is one Append per stream, so a citation stream's events all land
// before or all land after a source stream's. The projection resolves a
// citation's source on main at the moment the citation event lands: to bump
// that source's citation_count and denormalize its title. The branch's own
// first-touch order gets both halves of that wrong:
//
//   - A citation re-pointed (or created) at a source the branch created after
//     first touching the citation would replay before that source exists on
//     main — the citation moves but keeps the old title, and the new source's
//     count is never bumped.
//
//   - A source the branch deletes after re-pointing main's citation away from
//     it, but touched before that citation, would replay first — and the
//     store's source→citation cascade would delete main's citation before its
//     re-point lands.
//
// So: every source stream that survives the replay goes first (the citations
// can then find it), every source stream that ends deleted goes last (by then
// every citation has left it, which checkSourceDeleteOrphansNothing and the
// branch's ErrSourceHasCitations guard ensure), and all other streams keep
// their relative first-touch order in between. Sources do not reference any
// other replayed aggregate, so moving them cannot break another ordering.
func orderEvidenceForReplay(groups []streamGroup) []streamGroup {
	ordered := make([]streamGroup, 0, len(groups))
	var middle, last []streamGroup
	for _, group := range groups {
		switch {
		case !isSourceStream(group.streamType):
			middle = append(middle, group)
		case groupDeletesSource(group):
			last = append(last, group)
		default:
			ordered = append(ordered, group)
		}
	}
	ordered = append(ordered, middle...)
	return append(ordered, last...)
}

// isSourceStream reports whether a stream type is a source's. Source streams
// are written as both "Source" (commands) and "source" (GEDCOM import).
func isSourceStream(streamType string) bool {
	return strings.EqualFold(streamType, "source")
}

// isCitationStream reports whether a stream type is a citation's ("Citation"
// from commands, "citation" from GEDCOM import).
func isCitationStream(streamType string) bool {
	return strings.EqualFold(streamType, "citation")
}

// isNoteStream reports whether a stream type is a note's ("Note" from
// commands, "note" from GEDCOM import).
func isNoteStream(streamType string) bool {
	return strings.EqualFold(streamType, "note")
}

// groupDeletesSource reports whether a stream's replay deletes its source.
func groupDeletesSource(group streamGroup) bool {
	for _, evt := range group.events {
		if evt.EventType == "SourceDeleted" {
			return true
		}
	}
	return false
}

// createsSource reports whether a replay group leaves its source in existence
// on main by itself: a source stream that creates the source and does not end
// by deleting it.
func createsSource(group streamGroup) bool {
	if !isSourceStream(group.streamType) || groupDeletesSource(group) {
		return false
	}
	for i := range group.events {
		if group.events[i].EventType == "SourceCreated" {
			return true
		}
	}
	return false
}

// evidencePlan is what the evidence checks need to know about a replay.
type evidencePlan struct {
	// replayed holds every stream whose branch events are, or will be, on
	// main once the replay is done — for a merge, every stream not resolved
	// to main; for a resume, also the streams already on main.
	replayed map[uuid.UUID]streamGroup

	// removed names the streams whose entity main has removed since the
	// merge was claimed (resume only; see streamsRemovedOnMain). Replaying
	// such a source stream restores nothing, so it does not count as a
	// source main will have.
	removed map[uuid.UUID]bool
}

// validateNoDanglingEvidence is the source/citation half of
// validateNoDanglingReferences (#758). A citation lives on its own stream and
// names a source on another, so per-aggregate resolutions and per-aggregate
// conflict detection both miss two shapes:
//
//   - A replayed citation that ends up citing a source main will not have —
//     deleted on main after the fork, or excluded by a "main" resolution. The
//     projection saves such a citation anyway, with a blank source title, so
//     main would gain an orphaned citation.
//
//   - A replayed SourceDeleted while main still has citations of that source
//     that the replay does not delete or re-point — typically one main added
//     after the fork, which the branch never saw when its own delete guard
//     (ErrSourceHasCitations) ran. The store's source→citation cascade would
//     delete those citations on main with no CitationDeleted event and no
//     conflict shown.
//
// Both are refused before the claim, like the dangling child link. ResumeMerge
// applies the same two rules through checkEvidence (see
// danglingAutoPlannedStreams and validateResumeReferences).
func (h *Handler) validateNoDanglingEvidence(ctx context.Context, groups []streamGroup, resolutions map[uuid.UUID]MergeResolution) error {
	plan := evidencePlan{replayed: make(map[uuid.UUID]streamGroup, len(groups))}
	for _, group := range groups {
		if resolutions[group.streamID] != ResolveMain {
			plan.replayed[group.streamID] = group
		}
	}

	for _, group := range groups {
		if _, ok := plan.replayed[group.streamID]; !ok {
			continue
		}
		if err := h.checkEvidence(ctx, group, plan); err != nil {
			return err
		}
	}
	return nil
}

// checkEvidence applies both evidence rules to one stream the replay will
// append. A refusal wraps ErrMergeDanglingReference; any other error is a
// failure to check.
func (h *Handler) checkEvidence(ctx context.Context, group streamGroup, plan evidencePlan) error {
	if err := h.checkCitationSourceSurvives(ctx, group, plan); err != nil {
		return err
	}
	return h.checkSourceDeleteOrphansNothing(ctx, group, plan.replayed)
}

// checkCitationSourceSurvives refuses a replayed citation stream whose final
// source will not exist on main once the replay is done. Only the FINAL source
// matters: a citation created on a source and later re-pointed lands on the
// second one, and one the branch deleted cites nothing.
func (h *Handler) checkCitationSourceSurvives(ctx context.Context, group streamGroup, plan evidencePlan) error {
	outcome, err := citationOutcomeOf(group)
	if err != nil {
		return err
	}
	if outcome.deleted || !outcome.repointed {
		return nil
	}
	survives, err := h.sourceSurvivesReplay(ctx, outcome.sourceID, plan)
	if err != nil || survives {
		return err
	}
	return fmt.Errorf(
		"%w: the branch's citation %s cites source %s, but that source will not exist on main "+
			"(deleted there, or excluded by a \"main\" resolution)",
		ErrMergeDanglingReference, group.streamID, outcome.sourceID)
}

// sourceSurvivesReplay reports whether main will have a source once the
// replay is done. A replayed source stream decides it — unless it deletes the
// source, or main has removed the source since the claim; any other source is
// as main's read model has it.
func (h *Handler) sourceSurvivesReplay(ctx context.Context, sourceID uuid.UUID, plan evidencePlan) (bool, error) {
	if sourceGroup, ok := plan.replayed[sourceID]; ok {
		return !groupDeletesSource(sourceGroup) && !plan.removed[sourceID], nil
	}
	source, err := h.readStore.GetSource(ctx, domain.MainBranchID, sourceID)
	if err != nil {
		return false, fmt.Errorf("checking source %s on main: %w", sourceID, err)
	}
	return source != nil, nil
}

// checkSourceDeleteOrphansNothing refuses a replayed SourceDeleted while main
// has a citation of that source the replay does not itself delete or re-point
// elsewhere.
func (h *Handler) checkSourceDeleteOrphansNothing(ctx context.Context, group streamGroup, replayed map[uuid.UUID]streamGroup) error {
	if !groupDeletesSource(group) {
		return nil
	}
	citations, err := h.readStore.GetCitationsForSource(ctx, domain.MainBranchID, group.streamID)
	if err != nil {
		return fmt.Errorf("checking citations of source %s on main: %w", group.streamID, err)
	}
	for _, citation := range citations {
		if citationGroup, ok := replayed[citation.ID]; ok {
			outcome, err := citationOutcomeOf(citationGroup)
			if err != nil {
				return err
			}
			if outcome.deleted || (outcome.repointed && outcome.sourceID != group.streamID) {
				continue
			}
		}
		return fmt.Errorf(
			"%w: the branch deletes source %s, but main's citation %s still cites it; "+
				"merging would delete that citation from main with no record",
			ErrMergeDanglingReference, group.streamID, citation.ID)
	}
	return nil
}
