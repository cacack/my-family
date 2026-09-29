package query

import (
	"context"
	"encoding/json"
	"fmt"
	"sort"
	"time"

	"github.com/google/uuid"

	"github.com/cacack/my-family/internal/domain"
	"github.com/cacack/my-family/internal/repository"
)

// Where a merge-record decision was taken (MergeRecordDecision.DecidedAt).
const (
	MergeDecidedAtMerge  = "merge"
	MergeDecidedAtResume = "resume"
)

// MergeRecord is what a merged branch's own stream says about its merge
// (#832): when, why, what was decided and what was left behind. It is read
// from the BranchMerged claim and any BranchMergeResumed records, never
// recomputed from the diff, so it shows the decisions as they were taken.
type MergeRecord struct {
	ClaimID          uuid.UUID `json:"claim_id"`
	MergedAt         time.Time `json:"merged_at"`
	MergedAtPosition int64     `json:"merged_at_position"`
	Note             string    `json:"note,omitempty"`

	// Recorded is false for a claim written before #832, which recorded only
	// its note and replay plan. Its decisions are then derived from that plan:
	// every entity the branch changed that the plan left out was kept as the
	// mainline had it, but whether that was a conflict or an exclusion, and
	// the conflict's kind, were not recorded.
	Recorded bool `json:"recorded"`

	// ReplayedEventCount is how many branch events the merge set out to
	// replay; nil when the claim did not record it.
	ReplayedEventCount *int `json:"replayed_event_count,omitempty"`

	// SkippedStreamIDs are the entities whose branch changes were not
	// replayed, as the plan now stands (a resume can add to them). Never nil.
	SkippedStreamIDs []uuid.UUID `json:"skipped_stream_ids"`

	// Decisions are the conflict decisions — and any decision a resume took —
	// in the order they were recorded. Exclusions are the entities left
	// behind without a conflict. Never nil.
	Decisions  []MergeRecordDecision  `json:"decisions"`
	Exclusions []MergeRecordExclusion `json:"exclusions"`

	// ResumeCount is how many resumes recorded decisions for this merge.
	ResumeCount int `json:"resume_count"`
}

// MergeRecordDecision is one decision on a merge record.
type MergeRecordDecision struct {
	StreamID   uuid.UUID `json:"stream_id"`
	EntityType string    `json:"entity_type"`
	EntityName string    `json:"entity_name"`
	// Kind is the conflict class, empty when it was not recorded (a resume's
	// decision about a stream that was not a conflict at the merge, or a
	// pre-#832 claim).
	Kind       string   `json:"kind,omitempty"`
	Fields     []string `json:"fields,omitempty"`
	DeletedBy  string   `json:"deleted_by,omitempty"`
	Resolution string   `json:"resolution"`
	Rationale  string   `json:"rationale,omitempty"`
	DecidedAt  string   `json:"decided_at"`
}

// MergeRecordExclusion is one entity a merge left behind.
type MergeRecordExclusion struct {
	StreamID   uuid.UUID `json:"stream_id"`
	EntityType string    `json:"entity_type"`
	EntityName string    `json:"entity_name"`
	Rationale  string    `json:"rationale,omitempty"`
}

// mergeMarkers is a branch's merge claim and resume records, in the order
// they were appended.
type mergeMarkers struct {
	claim   domain.BranchMerged
	resumes []domain.BranchMergeResumed
}

// readMergeMarkers reads a branch's own stream for its merge claim and resume
// records. ok is false when the branch was never claimed. The stream holds
// only the branch's lifecycle events, so the read is small.
func (s *BranchService) readMergeMarkers(ctx context.Context, branchID uuid.UUID) (mergeMarkers, bool, error) {
	stored, err := s.eventStore.ReadStream(ctx, branchID)
	if err != nil {
		return mergeMarkers{}, false, fmt.Errorf("read branch stream: %w", err)
	}
	own := make([]repository.StoredEvent, 0, len(stored))
	for i := range stored {
		if stored[i].BranchID == domain.BranchID(branchID) {
			own = append(own, stored[i])
		}
	}
	sort.Slice(own, func(i, j int) bool { return own[i].Version < own[j].Version })

	var (
		markers mergeMarkers
		claimed bool
	)
	for i := range own {
		switch own[i].EventType {
		case "BranchMerged":
			if claimed {
				continue // the claim is a compare-and-set: the first one is it
			}
			if err := json.Unmarshal(own[i].Data, &markers.claim); err != nil {
				return mergeMarkers{}, false, fmt.Errorf("decode merge claim: %w", err)
			}
			claimed = true
		case "BranchMergeResumed":
			var resumed domain.BranchMergeResumed
			if err := json.Unmarshal(own[i].Data, &resumed); err != nil {
				return mergeMarkers{}, false, fmt.Errorf("decode merge resume record: %w", err)
			}
			markers.resumes = append(markers.resumes, resumed)
		}
	}
	return markers, claimed, nil
}

// buildMergeRecord assembles a merged branch's merge record from its markers.
// branchEvents are the branch's own mutation events: the entities it changed,
// which a pre-#832 claim's record is derived from and which the final skipped
// list is read against. Entities the markers do not name are named in one
// batched lookup through the branch's view.
func (s *BranchService) buildMergeRecord(ctx context.Context, branch *domain.Branch, markers mergeMarkers, branchEvents []repository.StoredEvent) (*MergeRecord, error) {
	claim := markers.claim
	record := &MergeRecord{
		ClaimID:            claim.ID,
		MergedAt:           claim.Timestamp,
		MergedAtPosition:   claim.MergedAtPosition,
		Note:               claim.Note,
		Recorded:           claim.HasRecord(),
		ReplayedEventCount: claim.ReplayedEventCount,
		SkippedStreamIDs:   []uuid.UUID{},
		Decisions:          []MergeRecordDecision{},
		Exclusions:         []MergeRecordExclusion{},
		ResumeCount:        len(markers.resumes),
	}

	touched := branchStreamIDs(branchEvents)
	typeOf := make(map[uuid.UUID]string, len(touched))
	for i := range branchEvents {
		if _, seen := typeOf[branchEvents[i].StreamID]; !seen {
			typeOf[branchEvents[i].StreamID] = conflictEntityType(&branchEvents[i])
		}
	}

	if record.Recorded {
		for _, d := range claim.Resolutions {
			record.Decisions = append(record.Decisions, MergeRecordDecision{
				StreamID: d.StreamID, EntityType: d.EntityType, EntityName: d.EntityName,
				Kind: d.Kind, Fields: d.Fields, DeletedBy: d.DeletedBy,
				Resolution: d.Resolution, Rationale: d.Rationale, DecidedAt: MergeDecidedAtMerge,
			})
		}
		for _, e := range claim.Exclusions {
			record.Exclusions = append(record.Exclusions, MergeRecordExclusion{
				StreamID: e.StreamID, EntityType: e.EntityType, EntityName: e.EntityName, Rationale: e.Rationale,
			})
		}
	} else if claim.ReplayStreamVersions != nil {
		// A pre-#832 claim: what its plan left out was kept as main had it.
		for _, streamID := range touched {
			if _, replayed := claim.ReplayStreamVersions[streamID]; replayed {
				continue
			}
			record.Exclusions = append(record.Exclusions, MergeRecordExclusion{
				StreamID: streamID, EntityType: typeOf[streamID], Rationale: claim.ResolutionRationales[streamID],
			})
		}
	}

	// A resume's decisions supersede whatever the claim said about the same
	// entity; they are listed as decisions, taken at the resume.
	plan := claim.ReplayStreamVersions
	for _, resumed := range markers.resumes {
		plan = resumed.ReplayStreamVersions
		ids := make([]uuid.UUID, 0, len(resumed.Resolutions))
		for streamID := range resumed.Resolutions {
			ids = append(ids, streamID)
		}
		sort.Slice(ids, func(i, j int) bool { return ids[i].String() < ids[j].String() })
		for _, streamID := range ids {
			decision := MergeRecordDecision{
				StreamID: streamID, EntityType: typeOf[streamID], Resolution: resumed.Resolutions[streamID],
				Rationale: resumed.Rationales[streamID], DecidedAt: MergeDecidedAtResume,
			}
			record.Decisions, record.Exclusions = supersede(record.Decisions, record.Exclusions, &decision)
			record.Decisions = append(record.Decisions, decision)
		}
	}

	if plan != nil {
		for _, streamID := range touched {
			if _, replayed := plan[streamID]; !replayed {
				record.SkippedStreamIDs = append(record.SkippedStreamIDs, streamID)
			}
		}
	} else if claim.SkippedStreamIDs != nil {
		record.SkippedStreamIDs = append(record.SkippedStreamIDs, claim.SkippedStreamIDs...)
	}

	if err := s.nameMergeRecord(ctx, branch, record); err != nil {
		return nil, err
	}
	return record, nil
}

// supersede drops the claim's entry for decision's stream, carrying over what
// the claim knew about it that the resume record does not (its name, and the
// conflict it was).
func supersede(decisions []MergeRecordDecision, exclusions []MergeRecordExclusion, decision *MergeRecordDecision) ([]MergeRecordDecision, []MergeRecordExclusion) {
	keptDecisions := decisions[:0]
	for _, d := range decisions {
		if d.StreamID != decision.StreamID {
			keptDecisions = append(keptDecisions, d)
			continue
		}
		decision.EntityName, decision.Kind, decision.Fields, decision.DeletedBy = d.EntityName, d.Kind, d.Fields, d.DeletedBy
		if decision.EntityType == "" {
			decision.EntityType = d.EntityType
		}
	}
	keptExclusions := exclusions[:0]
	for _, e := range exclusions {
		if e.StreamID != decision.StreamID {
			keptExclusions = append(keptExclusions, e)
			continue
		}
		decision.EntityName = e.EntityName
		if decision.EntityType == "" {
			decision.EntityType = e.EntityType
		}
	}
	return keptDecisions, keptExclusions
}

// nameMergeRecord names, in one batched lookup, every entry the markers did
// not name (a resume's decisions, a pre-#832 claim's exclusions).
func (s *BranchService) nameMergeRecord(ctx context.Context, branch *domain.Branch, record *MergeRecord) error {
	var refs []EntityRef
	for _, d := range record.Decisions {
		if d.EntityName == "" && d.EntityType != "" {
			refs = append(refs, EntityRef{EntityType: d.EntityType, ID: d.StreamID})
		}
	}
	for _, e := range record.Exclusions {
		if e.EntityName == "" && e.EntityType != "" {
			refs = append(refs, EntityRef{EntityType: e.EntityType, ID: e.StreamID})
		}
	}
	if len(refs) == 0 {
		return nil
	}
	names, err := s.NameEntities(ctx, domain.BranchID(branch.ID), refs)
	if err != nil {
		return fmt.Errorf("name merge record entities: %w", err)
	}
	for i := range record.Decisions {
		if d := &record.Decisions[i]; d.EntityName == "" {
			d.EntityName = names[EntityRef{EntityType: d.EntityType, ID: d.StreamID}]
		}
	}
	for i := range record.Exclusions {
		if e := &record.Exclusions[i]; e.EntityName == "" {
			e.EntityName = names[EntityRef{EntityType: e.EntityType, ID: e.StreamID}]
		}
	}
	return nil
}

// withoutReplayedCopies drops from main's side of a merged branch's diff the
// events that are this branch's own changes replayed by its merge, and
// returns how many it dropped. Without it, a merged branch's compare lists
// every change twice — once on the branch, once as if main had made it
// independently — and reports every replayed entity as changed on both sides.
//
// A copy is recognised by its merge provenance (#832), or, for a merge
// recorded before provenance was stamped, by its payload id: replay
// re-appends each branch event decoded, so the copy carries the original's
// id (the same test ResumeMerge uses to find what already landed).
func withoutReplayedCopies(branchID uuid.UUID, branchEvents, mainEvents []repository.StoredEvent) ([]repository.StoredEvent, int) {
	originals := make(map[uuid.UUID]bool, len(branchEvents))
	for i := range branchEvents {
		if id, ok := payloadID(&branchEvents[i]); ok {
			originals[id] = true
		}
	}
	kept := make([]repository.StoredEvent, 0, len(mainEvents))
	dropped := 0
	for i := range mainEvents {
		if isReplayedCopy(branchID, originals, &mainEvents[i]) {
			dropped++
			continue
		}
		kept = append(kept, mainEvents[i])
	}
	return kept, dropped
}

// isReplayedCopy reports whether a main event is branchID's merge replaying
// one of originals.
func isReplayedCopy(branchID uuid.UUID, originals map[uuid.UUID]bool, evt *repository.StoredEvent) bool {
	if len(evt.Metadata) > 0 {
		// Metadata that does not decode says nothing either way, so the
		// payload-id test below decides, as it does for an unstamped copy.
		var meta domain.EventMetadata
		if err := json.Unmarshal(evt.Metadata, &meta); err == nil && meta.MergedFromBranch != nil {
			return meta.MergedFromBranch.BranchID == branchID
		}
	}
	id, ok := payloadID(evt)
	return ok && originals[id]
}

// payloadID reads the event id from a stored payload (domain.BaseEvent.ID).
func payloadID(evt *repository.StoredEvent) (uuid.UUID, bool) {
	var payload struct {
		ID uuid.UUID `json:"id"`
	}
	if err := json.Unmarshal(evt.Data, &payload); err != nil || payload.ID == uuid.Nil {
		return uuid.Nil, false
	}
	return payload.ID, true
}
