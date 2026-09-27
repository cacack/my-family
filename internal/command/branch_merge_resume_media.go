package command

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"

	"github.com/google/uuid"

	"github.com/cacack/my-family/internal/domain"
	"github.com/cacack/my-family/internal/repository"
)

// Media on resume (#759 on top of #685).
//
// A media stream is resumed like any other branch-aware stream: its events are
// found on main by payload id, its main row is read with GetMedia (metadata
// only — never the bytes), and a row behind main's log is re-projected from
// main's own events. That repair cannot copy or lose file bytes: every event
// it re-projects is projected onto MAIN, the only event that carries bytes is
// MediaCreated (whose bytes are main's own copy, in main's log), and
// MediaUpdated/MediaDeleted re-read and re-save metadata only — a save with
// nil bytes keeps the bytes already stored, and a main delete of an item a
// branch still shows keeps main's row as a byte-holding tombstone. No branch
// row is ever written by a resume.
//
// The one media-specific question is a MISSING main row. Like a citation's,
// a media row can be removed by a write to ANOTHER stream: deleting its owner
// (person, family or source) cascades onto the owner's media without writing
// to the media stream (missingMediaCascadedAway). Re-projecting such an item
// would resurrect an orphan, so it counts as removed for a reason main's log
// explains.
//
// A person owner main merged into another is the case the repair cannot settle
// on its own: PersonMerged moves the merged person's media to the survivor
// when it is projected, so a media row still missing afterwards means its own
// MediaCreated projection failed BEFORE that merge. Re-projecting it from the
// media stream would attach it to the merged-away person — an orphan — and the
// transfer the person merge would have made is not in the media stream to
// replay. Unless the survivor (followed through any later merges) has itself
// been deleted, which removes the item either way, the resume refuses such a
// landed stream with ErrMergeResumeRepairUnsound before writing anything.

// ErrMergeResumeRepairUnsound is returned when an already-replayed media
// stream's main row is missing and its owner was merged into a person main
// still has, so re-projecting the stream would attach the item to the
// merged-away person (see the note above). The same holds for a GPS artifact
// whose subject was merged away (#760; see branch_merge_resume_gps.go). Nothing has been written by the
// refusing call. The refusal is permanent for this state: main's read model
// needs a rebuild from the log (#680) to repair the item, and resuming again
// will refuse the same way (the API reports it as 409
// merge_resume_repair_unsound).
var ErrMergeResumeRepairUnsound = errors.New("an already-replayed media item or GPS artifact cannot be repaired from its own stream")

// mediaOwner names the entity a media item is attached to.
type mediaOwner struct {
	entityType string
	id         uuid.UUID
}

// lastMediaOwner returns the owner the last MediaCreated of a media stream's
// main history attached the item to. found is false when the history creates
// nothing.
func lastMediaOwner(events []repository.StoredEvent) (owner mediaOwner, found bool, err error) {
	for i := range events {
		if events[i].EventType != "MediaCreated" {
			continue
		}
		owner.entityType, owner.id, err = mediaOwnerOf(events[i])
		if err != nil {
			return mediaOwner{}, false, err
		}
		found = true
	}
	return owner, found, nil
}

// missingMediaCascadedAway reports which missing media rows were removed by
// their owner's delete cascade: the owner the stream's main log attached the
// item to — followed, for a person, through any person merges main recorded
// since — ends in a delete on main. Only media streams whose row is missing and
// whose stream does not itself end in a delete are candidates; ownerOf holds
// them with their owners (missingMediaOwners), and survivorOf the person merges
// main recorded since the earliest of them (survivorsAfter).
//
// refuse names the candidates that are already on main by payload id: for
// those, an owner merged into a person main still has is refused with
// ErrMergeResumeRepairUnsound instead of being left to a re-projection that would
// orphan the item. A candidate not in refuse is simply reported as not
// removed; a resume's replay of its (metadata-only) events onto a missing row
// is a projection no-op.
//
// The work is set-based: the caller's one paged scan of main for person
// merges (shared with the merged-away person check, and made only when a
// candidate's owner is a person), then one paged scan of the final owners'
// streams — never a scan per media item.
func (h *Handler) missingMediaCascadedAway(
	ctx context.Context,
	missing []streamGroup,
	ownerOf map[uuid.UUID]mediaOwner,
	survivorOf map[uuid.UUID]uuid.UUID,
	refuse map[uuid.UUID]bool,
) (map[uuid.UUID]bool, error) {
	if len(ownerOf) == 0 {
		return nil, nil
	}
	finalOf := make(map[uuid.UUID]uuid.UUID, len(ownerOf))
	var finals []uuid.UUID
	for mediaID, owner := range ownerOf {
		final := owner.id
		if owner.entityType == "person" {
			final = finalSurvivor(final, survivorOf)
		}
		finalOf[mediaID] = final
		finals = appendUnique(finals, final)
	}
	ownerEvents, err := h.readMainStreams(ctx, finals)
	if err != nil {
		return nil, err
	}

	cascaded := make(map[uuid.UUID]bool)
	for _, group := range missing {
		owner, ok := ownerOf[group.streamID]
		if !ok {
			continue
		}
		final := finalOf[group.streamID]
		switch {
		case endsInDelete(ownerEvents[final]):
			cascaded[group.streamID] = true
		case final != owner.id && refuse[group.streamID]:
			return nil, fmt.Errorf(
				"%w: media %s is on main in the log but missing from main's read model, and its owner person %s "+
					"was merged into person %s since; re-projecting it would attach it to the merged-away person. "+
					"Nothing has been written; rebuild main's read model from the log to repair it",
				ErrMergeResumeRepairUnsound, group.streamID, owner.id, final)
		}
	}
	return cascaded, nil
}

// missingMediaOwners returns the owner each candidate of missingMediaCascadedAway
// was attached to by its main log, and the position a scan for person merges
// must start from (-1 when no candidate's owner is a person).
func missingMediaOwners(
	missing []streamGroup,
	states map[uuid.UUID]readModelState,
	mainEvents map[uuid.UUID][]repository.StoredEvent,
) (map[uuid.UUID]mediaOwner, int64, error) {
	ownerOf := make(map[uuid.UUID]mediaOwner)
	scanFrom := int64(-1)
	for _, group := range missing {
		events := mainEvents[group.streamID]
		if states[group.streamID].present || len(events) == 0 || !isMediaStream(group.streamType) || endsInDelete(events) {
			continue
		}
		owner, found, err := lastMediaOwner(events)
		if err != nil {
			return nil, 0, err
		}
		if !found {
			continue
		}
		ownerOf[group.streamID] = owner
		if first := events[0].Position; owner.entityType == "person" && (scanFrom < 0 || first < scanFrom) {
			scanFrom = first
		}
	}
	return ownerOf, scanFrom, nil
}

// finalSurvivor follows a person through main's recorded merges to the person
// that finally holds their data. It stops after as many steps as there are
// merges, so a malformed log with a cycle cannot loop it.
func finalSurvivor(personID uuid.UUID, survivorOf map[uuid.UUID]uuid.UUID) uuid.UUID {
	for steps := 0; steps < len(survivorOf); steps++ {
		next, merged := survivorOf[personID]
		if !merged {
			break
		}
		personID = next
	}
	return personID
}

// personMerge is one PersonMerged main recorded: MergedID folded into
// SurvivorID at a log position.
type personMerge struct {
	mergedID, survivorID uuid.UUID
	position             int64
}

// personMergesOnMain lists every person merge main recorded after
// fromPosition, in log order, paging through main's own events from that
// point. It is the one scan of main for person merges a resume's read-model
// repair makes; survivorsAfter narrows it for each consumer.
func (h *Handler) personMergesOnMain(ctx context.Context, fromPosition int64) ([]personMerge, error) {
	var merges []personMerge
	from := fromPosition
	for {
		page, err := h.eventStore.ReadBranch(ctx, domain.MainBranchID, from, resumeScanPage)
		if err != nil {
			return nil, fmt.Errorf("reading main events for person merges: %w", err)
		}
		for i := range page {
			if page[i].EventType != "PersonMerged" {
				continue
			}
			var payload struct {
				SurvivorID uuid.UUID `json:"survivor_id"`
				MergedID   uuid.UUID `json:"merged_id"`
			}
			if err := json.Unmarshal(page[i].Data, &payload); err != nil {
				return nil, fmt.Errorf("decoding PersonMerged at position %d: %w", page[i].Position, err)
			}
			merges = append(merges, personMerge{mergedID: payload.MergedID, survivorID: payload.SurvivorID, position: page[i].Position})
		}
		if len(page) < resumeScanPage {
			return merges, nil
		}
		from = page[len(page)-1].Position
	}
}

// survivorsAfter maps every person merged into another after fromPosition to
// the survivor (MergedID → SurvivorID). A negative fromPosition means no
// consumer asked, and yields an empty map.
func survivorsAfter(merges []personMerge, fromPosition int64) map[uuid.UUID]uuid.UUID {
	survivorOf := make(map[uuid.UUID]uuid.UUID)
	if fromPosition < 0 {
		return survivorOf
	}
	for _, m := range merges {
		if m.position > fromPosition {
			survivorOf[m.mergedID] = m.survivorID
		}
	}
	return survivorOf
}

// mainMediaState is mainReadModelState for media streams (#759). It reads the
// main row's metadata only (GetMedia never reads the bytes); the version it
// reports is the one every media projection writes in its single save.
func (h *Handler) mainMediaState(ctx context.Context, group streamGroup) (readModelState, error) {
	media, err := h.readStore.GetMedia(ctx, domain.MainBranchID, group.streamID)
	if err != nil {
		return readModelState{}, fmt.Errorf("reading main media %s: %w", group.streamID, err)
	}
	if media == nil {
		return readModelState{}, nil
	}
	return readModelState{present: true, version: media.Version}, nil
}

// checkLandedMediaOwners is the media half of validateResumeEvidence's
// landed-stream check: a media upload already on main may not lose its owner
// to this call's own "main" resolution of the stream that creates that owner.
// Decisions recorded earlier were checked when they were made, and an owner
// main itself removed later is main's change (its delete cascaded the item).
func (h *Handler) checkLandedMediaOwners(
	ctx context.Context,
	groups []streamGroup,
	byID map[uuid.UUID]streamGroup,
	view resumeView,
	resolutions map[uuid.UUID]MergeResolution,
) error {
	for _, group := range groups {
		if !view.landed[group.streamID] {
			continue
		}
		entityType, entityID, ok, err := mediaUploadOf(group)
		if err != nil {
			return err
		}
		if !ok || resolutions[entityID] != ResolveMain || !createsMediaOwner(byID[entityID], entityType) {
			continue
		}
		exists, err := h.mediaOwnerOnMain(ctx, entityType, entityID)
		if err != nil {
			return err
		}
		if !exists {
			return fmt.Errorf(
				"%w: media %s is already on main and is attached to %s %s, which main does not have; "+
					"resolving that %s to main would leave the media orphaned — resolve it to branch instead",
				ErrMergeDanglingReference, group.streamID, entityType, entityID, entityType)
		}
	}
	return nil
}
