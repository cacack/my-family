package command

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

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
// A person owner main merged into another needs one more step. PersonMerged
// moves the merged person's media to the survivor when it is projected, so a
// media row still missing afterwards means its own MediaCreated projection
// failed BEFORE that merge, and re-projecting the stream alone would attach
// the item to the merged-away person. The transfer the person merge would have
// made is not in the media stream, but it is fully determined by main's log:
// the merges main recorded since the upload (followed through any later
// merges) name the survivor, and moving a media item is nothing more than
// re-linking its row to that survivor — the same save PersonMerged makes, with
// the item's version untouched. So the repair re-projects the stream and then
// re-links the row (relinkMergedMedia). If the survivor has itself been
// deleted since, the item is gone either way (the cascade), and nothing is
// re-projected.

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

// mediaRelink is the owner transfer a missing media row needs after its
// re-projection: main merged the person the upload attached it to into
// target (followed through every later merge), and the last of those merges
// was recorded at mergedAt. scanFrom is where the scan for those merges
// started, so relinkMergedMedia can repeat it to catch a merge that raced it.
type mediaRelink struct {
	target   uuid.UUID
	mergedAt time.Time
	scanFrom int64
}

// missingMediaCascadedAway reports which missing media rows were removed by
// their owner's delete cascade: the owner the stream's main log attached the
// item to — followed, for a person, through any person merges main recorded
// since — ends in a delete on main. Only media streams whose row is missing and
// whose stream does not itself end in a delete are candidates; ownerOf holds
// them with their owners (missingMediaOwners), and mergeOf the person merges
// main recorded since the earliest of them (survivorsAfter), scanned from
// scanFrom.
//
// It also reports, for each candidate NOT cascaded away whose person owner
// main merged into a person it still has, the transfer its re-projection must
// be followed by (see the note at the top of this file): re-projecting the
// stream alone would attach the item to the merged-away person.
//
// The work is set-based: the caller's one paged scan of main for person
// merges (shared with the merged-away person check, and made only when a
// candidate's owner is a person), then one paged scan of the final owners'
// streams — never a scan per media item.
func (h *Handler) missingMediaCascadedAway(
	ctx context.Context,
	missing []streamGroup,
	ownerOf map[uuid.UUID]mediaOwner,
	mergeOf map[uuid.UUID]personMerge,
	scanFrom int64,
) (map[uuid.UUID]bool, map[uuid.UUID]mediaRelink, error) {
	if len(ownerOf) == 0 {
		return nil, nil, nil
	}
	finalOf := make(map[uuid.UUID]uuid.UUID, len(ownerOf))
	mergedAtOf := make(map[uuid.UUID]time.Time, len(ownerOf))
	var finals []uuid.UUID
	for mediaID, owner := range ownerOf {
		final := owner.id
		if owner.entityType == "person" {
			final, mergedAtOf[mediaID] = finalSurvivor(final, mergeOf)
		}
		finalOf[mediaID] = final
		finals = appendUnique(finals, final)
	}
	ownerEvents, err := h.readMainStreams(ctx, finals)
	if err != nil {
		return nil, nil, err
	}

	cascaded := make(map[uuid.UUID]bool)
	relink := make(map[uuid.UUID]mediaRelink)
	for _, group := range missing {
		owner, ok := ownerOf[group.streamID]
		if !ok {
			continue
		}
		final := finalOf[group.streamID]
		switch {
		case endsInDelete(ownerEvents[final]):
			cascaded[group.streamID] = true
		case final != owner.id:
			relink[group.streamID] = mediaRelink{target: final, mergedAt: mergedAtOf[group.streamID], scanFrom: scanFrom}
		}
	}
	return cascaded, relink, nil
}

// relinkMergedMedia makes, on main's re-projected row of a media item, the
// owner transfer the person merges main recorded since its upload would have
// made (see mediaRelink): the row is re-linked to the final survivor, exactly
// as PersonMerged re-links a merged person's media — entity id and updated-at
// only, the version untouched, and no bytes (GetMedia reads none, and a save
// with nil bytes keeps those stored).
//
// A merge of the survivor recorded while this ran would move the row only if
// its projection saw it re-linked, so the scan for merges is repeated after
// each save, and the row re-linked again to any new survivor, until a scan
// finds nothing new. A row already gone (removed while the repair ran) is
// left alone; reprojectStream reports that.
func (h *Handler) relinkMergedMedia(ctx context.Context, group streamGroup, relink mediaRelink) error {
	for attempt := 0; attempt < reprojectAttempts; attempt++ {
		row, err := h.readStore.GetMedia(ctx, domain.MainBranchID, group.streamID)
		if err != nil {
			return fmt.Errorf("reading main media %s to re-link it: %w", group.streamID, err)
		}
		if row == nil {
			return nil
		}
		if row.EntityType == "person" && row.EntityID != relink.target {
			row.EntityID = relink.target
			if relink.mergedAt.After(row.UpdatedAt) {
				row.UpdatedAt = relink.mergedAt
			}
			if err := h.readStore.SaveMedia(ctx, domain.MainBranchID, row); err != nil {
				return fmt.Errorf("re-linking main media %s to merge survivor %s: %w", group.streamID, relink.target, err)
			}
		}

		merges, err := h.personMergesOnMain(ctx, relink.scanFrom)
		if err != nil {
			return err
		}
		target, mergedAt := finalSurvivor(relink.target, survivorsAfter(merges, relink.scanFrom))
		if target == relink.target {
			return nil
		}
		relink.target = target
		if mergedAt.After(relink.mergedAt) {
			relink.mergedAt = mergedAt
		}
	}
	return fmt.Errorf("%w: media %s's owner kept being merged while the resume re-linked it; resume again to finish",
		errReprojectRaced, group.streamID)
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
// that finally holds their data, and reports when the last merge it followed
// was recorded (zero when it followed none). It stops after as many steps as
// there are merges, so a malformed log with a cycle cannot loop it.
func finalSurvivor(personID uuid.UUID, mergeOf map[uuid.UUID]personMerge) (uuid.UUID, time.Time) {
	var mergedAt time.Time
	for steps := 0; steps < len(mergeOf); steps++ {
		merge, merged := mergeOf[personID]
		if !merged {
			break
		}
		personID = merge.survivorID
		if merge.at.After(mergedAt) {
			mergedAt = merge.at
		}
	}
	return personID, mergedAt
}

// personMerge is one PersonMerged main recorded: MergedID folded into
// SurvivorID at a log position and time.
type personMerge struct {
	mergedID, survivorID uuid.UUID
	position             int64
	at                   time.Time
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
			merges = append(merges, personMerge{
				mergedID: payload.MergedID, survivorID: payload.SurvivorID,
				position: page[i].Position, at: page[i].Timestamp,
			})
		}
		if len(page) < resumeScanPage {
			return merges, nil
		}
		from = page[len(page)-1].Position
	}
}

// survivorsAfter maps every person merged into another after fromPosition to
// that merge (MergedID → the merge, naming SurvivorID). A negative
// fromPosition means no consumer asked, and yields an empty map.
func survivorsAfter(merges []personMerge, fromPosition int64) map[uuid.UUID]personMerge {
	mergeOf := make(map[uuid.UUID]personMerge)
	if fromPosition < 0 {
		return mergeOf
	}
	for _, m := range merges {
		if m.position > fromPosition {
			mergeOf[m.mergedID] = m
		}
	}
	return mergeOf
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
// landed-stream check: a media upload already on main may not be left attached
// to an owner main does not have because this call resolves that owner's
// stream to "main" (skipping it). That covers a stream that creates the owner
// and also one that creates AND deletes it: moveMediaBeforeOwnerDelete lands
// the upload ahead of such a stream and counts on its delete to cascade the
// item away, so skipping it leaves the item on main attached to an owner main
// never had.
//
// The owner checked is the one main's row now names (a person merge on main
// moves the item to the survivor); with no main row, it is the one the upload
// attached the item to — or, when main has merged that person away since, the
// final survivor (view.relinked) — since a resume's repair would re-project
// the item there — unless main's log explains the row's absence (view.removed: main
// deleted the item, or deleted an owner it had and cascaded it), which leaves
// nothing to orphan. Decisions recorded earlier were checked when they were
// made. A refused caller can resolve the owner's stream to branch, or delete
// the item on main first.
func (h *Handler) checkLandedMediaOwners(
	ctx context.Context,
	groups []streamGroup,
	view resumeView,
	resolutions map[uuid.UUID]MergeResolution,
) error {
	for _, group := range groups {
		if !view.landed[group.streamID] || view.removed[group.streamID] {
			continue
		}
		entityType, entityID, ok, err := mediaUploadOf(group)
		if err != nil {
			return err
		}
		if !ok {
			continue
		}
		row, err := h.readStore.GetMedia(ctx, domain.MainBranchID, group.streamID)
		if err != nil {
			return fmt.Errorf("reading main media %s: %w", group.streamID, err)
		}
		switch survivor, relinked := view.relinked[group.streamID]; {
		case row != nil:
			entityType, entityID = row.EntityType, row.EntityID
		case relinked:
			entityType, entityID = "person", survivor
		}
		if resolutions[entityID] != ResolveMain {
			continue
		}
		exists, err := h.mediaOwnerOnMain(ctx, entityType, entityID)
		if err != nil {
			return err
		}
		if !exists {
			return fmt.Errorf(
				"%w: media %s is already on main and is attached to %s %s, which main does not have; "+
					"resolving that %s to main would leave the media orphaned — resolve it to branch instead, "+
					"or delete the media on main first",
				ErrMergeDanglingReference, group.streamID, entityType, entityID, entityType)
		}
	}
	return nil
}
