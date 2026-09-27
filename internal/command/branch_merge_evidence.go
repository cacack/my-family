package command

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/google/uuid"

	"github.com/cacack/my-family/internal/domain"
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
	for _, evt := range group.events {
		switch evt.EventType {
		case "CitationCreated":
			var payload struct {
				SourceID uuid.UUID `json:"source_id"`
			}
			if err := json.Unmarshal(evt.Data, &payload); err != nil {
				return out, fmt.Errorf("decoding citation create on stream %s: %w", group.streamID, err)
			}
			out = citationOutcome{repointed: true, sourceID: payload.SourceID}
		case "CitationUpdated":
			var payload struct {
				Changes map[string]any `json:"changes"`
			}
			if err := json.Unmarshal(evt.Data, &payload); err != nil {
				return out, fmt.Errorf("decoding citation update on stream %s: %w", group.streamID, err)
			}
			raw, ok := payload.Changes["source_id"].(string)
			if !ok {
				continue
			}
			id, err := uuid.Parse(raw)
			if err != nil {
				return out, fmt.Errorf("decoding citation update on stream %s: source_id %q: %w", group.streamID, raw, err)
			}
			out.repointed, out.sourceID = true, id
		case "CitationDeleted":
			out.deleted = true
		}
	}
	return out, nil
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
// Both are refused before the claim, like the dangling child link.
func (h *Handler) validateNoDanglingEvidence(ctx context.Context, groups []streamGroup, resolutions map[uuid.UUID]MergeResolution) error {
	replayed := make(map[uuid.UUID]streamGroup, len(groups))
	for _, group := range groups {
		if resolutions[group.streamID] != ResolveMain {
			replayed[group.streamID] = group
		}
	}

	for _, group := range groups {
		if _, ok := replayed[group.streamID]; !ok {
			continue
		}
		if err := h.checkCitationSourceSurvives(ctx, group, replayed); err != nil {
			return err
		}
		if err := h.checkSourceDeleteOrphansNothing(ctx, group, replayed); err != nil {
			return err
		}
	}
	return nil
}

// checkCitationSourceSurvives refuses a replayed citation stream whose final
// source will not exist on main once the replay is done. Only the FINAL source
// matters: a citation created on a source and later re-pointed lands on the
// second one, and one the branch deleted cites nothing.
func (h *Handler) checkCitationSourceSurvives(ctx context.Context, group streamGroup, replayed map[uuid.UUID]streamGroup) error {
	outcome, err := citationOutcomeOf(group)
	if err != nil {
		return err
	}
	if outcome.deleted || !outcome.repointed {
		return nil
	}
	if sourceGroup, ok := replayed[outcome.sourceID]; ok {
		if !groupDeletesSource(sourceGroup) {
			return nil
		}
	} else {
		source, err := h.readStore.GetSource(ctx, domain.MainBranchID, outcome.sourceID)
		if err != nil {
			return fmt.Errorf("checking source %s on main: %w", outcome.sourceID, err)
		}
		if source != nil {
			return nil
		}
	}
	return fmt.Errorf(
		"%w: the branch's citation %s cites source %s, but that source will not exist on main "+
			"(deleted there, or excluded by a \"main\" resolution)",
		ErrMergeDanglingReference, group.streamID, outcome.sourceID)
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
