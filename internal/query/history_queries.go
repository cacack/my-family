// Package query provides CQRS query services for the genealogy application.
package query

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"sort"
	"time"

	"github.com/google/uuid"

	"github.com/cacack/my-family/internal/domain"
	"github.com/cacack/my-family/internal/repository"
)

// maxComparisonEvents caps how many raw events any one side of a diff reads —
// snapshot-vs-snapshot (CompareSnapshots) and branch-vs-main (CompareBranch)
// alike. It lives here, next to the transform both diffs feed, so the two
// cannot be tuned independently and silently disagree.
const maxComparisonEvents = 1000

// HistoryService provides query operations for change history and audit trails.
type HistoryService struct {
	eventStore repository.EventStore
	readStore  repository.ReadModelStore
}

// NewHistoryService creates a new history query service.
func NewHistoryService(eventStore repository.EventStore, readStore repository.ReadModelStore) *HistoryService {
	return &HistoryService{
		eventStore: eventStore,
		readStore:  readStore,
	}
}

// ChangeEntry represents a user-friendly change record in the system's history.
type ChangeEntry struct {
	ID         uuid.UUID              `json:"id"`
	Timestamp  time.Time              `json:"timestamp"`
	EntityType string                 `json:"entity_type"` // see historyEventCatalog for the vocabulary
	EntityID   uuid.UUID              `json:"entity_id"`
	EntityName string                 `json:"entity_name"` // e.g., "John Smith", "Birth, 1 JAN 1850"
	Action     string                 `json:"action"`      // "created", "updated", "deleted", "merged"
	Changes    map[string]FieldChange `json:"changes,omitempty"`
	UserID     *string                `json:"user_id,omitempty"`
	// ParentEntityType and ParentEntityID name the entity whose page presents
	// a sub-record that has no page of its own: a life event's or LDS
	// ordinance's person or family, an attribute's or association's person, a
	// citation's source, a media item's owner. Empty otherwise.
	ParentEntityType string     `json:"parent_entity_type,omitempty"`
	ParentEntityID   *uuid.UUID `json:"parent_entity_id,omitempty"`
	// Origin is set only by branch-scoped entity history (GetEntityHistoryOn
	// with a non-main branch): ChangeOriginBranch for the branch's own events,
	// ChangeOriginMain for the mainline events its view inherits. Empty
	// everywhere else.
	Origin string `json:"origin,omitempty"`
}

// Origins of a branch-scoped history entry (ChangeEntry.Origin).
const (
	ChangeOriginMain   = "main"
	ChangeOriginBranch = "branch"
)

// FieldChange represents before/after values for a field update.
type FieldChange struct {
	OldValue any `json:"old_value,omitempty"`
	NewValue any `json:"new_value,omitempty"`
}

// ChangeHistoryResult contains paginated change history results.
type ChangeHistoryResult struct {
	Entries    []ChangeEntry `json:"entries"`
	TotalCount int           `json:"total_count"`
	HasMore    bool          `json:"has_more"`
	Limit      int           `json:"limit"`
	Offset     int           `json:"offset"`
}

// GetGlobalHistoryInput contains options for global history queries.
type GetGlobalHistoryInput struct {
	FromTime   time.Time
	ToTime     time.Time
	EventTypes []string
	Limit      int
	Offset     int
}

// GetEntityHistory retrieves the change history for a specific entity.
func (s *HistoryService) GetEntityHistory(ctx context.Context, entityType string, entityID uuid.UUID, limit, offset int) (*ChangeHistoryResult, error) {
	// Validate inputs
	if limit <= 0 {
		limit = 20
	}
	if limit > 100 {
		limit = 100
	}
	if offset < 0 {
		offset = 0
	}

	// Read events for this entity's stream. Scoped to main: the mainline audit
	// trail must not interleave any branch's in-progress edits (ADR-005). A
	// branch's view of the stream is GetEntityHistoryOn.
	page, err := s.eventStore.ReadByStream(ctx, entityID, domain.MainBranchID, limit, offset)
	if err != nil {
		return nil, fmt.Errorf("reading stream: %w", err)
	}

	// Transform events to change entries
	entries, err := s.transformStoredEvents(ctx, page.Events)
	if err != nil {
		return nil, fmt.Errorf("transforming events: %w", err)
	}

	return &ChangeHistoryResult{
		Entries:    entries,
		TotalCount: page.TotalCount,
		HasMore:    page.HasMore,
		Limit:      limit,
		Offset:     offset,
	}, nil
}

// GetEntityHistoryOn retrieves an entity's change history as branchID sees it.
// On the mainline it is exactly GetEntityHistory.
//
// On a branch it follows the read model's copy-on-write overlay (ADR-005), so
// the history explains the state the branch actually shows:
//
//   - The branch's own events for the stream (Origin "branch").
//   - The mainline events the branch's view inherits (Origin "main"). The
//     overlay is live: until the branch first writes the entity, a branch read
//     falls back to main's current row, and the branch's first write seeds its
//     shadow row from that row. So every mainline event before the branch's
//     first event on the stream is inherited — including mainline edits made
//     after the fork but before that first write — and none after it, since
//     the branch's own row no longer reflects them (they surface in the branch
//     compare and at merge instead). With no branch events on the stream, the
//     whole mainline history is inherited.
//
// Entries are ordered by global position (oldest first) and paginated after
// the branch filter, so TotalCount and HasMore describe this branch's view.
// The stream is read once (ReadStream) and entity names are resolved in one
// batched read-model lookup through the branch's overlay — no per-event query.
func (s *HistoryService) GetEntityHistoryOn(ctx context.Context, branchID domain.BranchID, entityType string, entityID uuid.UUID, limit, offset int) (*ChangeHistoryResult, error) {
	if branchID.IsMain() {
		return s.GetEntityHistory(ctx, entityType, entityID, limit, offset)
	}
	if limit <= 0 {
		limit = 20
	}
	if limit > 100 {
		limit = 100
	}
	if offset < 0 {
		offset = 0
	}

	all, err := s.eventStore.ReadStream(ctx, entityID)
	if err != nil {
		return nil, fmt.Errorf("reading stream: %w", err)
	}
	visible := branchVisibleStreamEvents(all, branchID)

	total := len(visible)
	start := min(offset, total)
	end := min(start+limit, total)
	page := visible[start:end]

	entries, err := s.transformStoredEventsOn(ctx, branchID, page)
	if err != nil {
		return nil, fmt.Errorf("transforming events: %w", err)
	}
	origins := make(map[uuid.UUID]string, len(page))
	for i := range page {
		if page[i].BranchID == branchID {
			origins[page[i].ID] = ChangeOriginBranch
		} else {
			origins[page[i].ID] = ChangeOriginMain
		}
	}
	for i := range entries {
		entries[i].Origin = origins[entries[i].ID]
	}

	return &ChangeHistoryResult{
		Entries:    entries,
		TotalCount: total,
		HasMore:    end < total,
		Limit:      limit,
		Offset:     offset,
	}, nil
}

// branchVisibleStreamEvents filters one stream's events (from ReadStream, which
// interleaves every branch) down to those branchID's overlay view is built
// from — see GetEntityHistoryOn — ordered by global position. Other branches'
// events never appear.
func branchVisibleStreamEvents(events []repository.StoredEvent, branchID domain.BranchID) []repository.StoredEvent {
	sorted := make([]repository.StoredEvent, 0, len(events))
	for i := range events {
		if events[i].BranchID == branchID || events[i].BranchID.IsMain() {
			sorted = append(sorted, events[i])
		}
	}
	sort.SliceStable(sorted, func(i, j int) bool { return sorted[i].Position < sorted[j].Position })

	// The branch's first event on the stream is where its copy-on-write row
	// took over from main's.
	var firstBranchPos int64
	branchWrote := false
	for i := range sorted {
		if sorted[i].BranchID == branchID {
			firstBranchPos = sorted[i].Position
			branchWrote = true
			break
		}
	}

	visible := sorted[:0]
	for i := range sorted {
		evt := sorted[i]
		if evt.BranchID == branchID || !branchWrote || evt.Position < firstBranchPos {
			visible = append(visible, evt)
		}
	}
	return visible
}

// GetGlobalHistory retrieves system-wide change history with optional time and type filters.
func (s *HistoryService) GetGlobalHistory(ctx context.Context, input GetGlobalHistoryInput) (*ChangeHistoryResult, error) {
	// Validate inputs
	if input.Limit <= 0 {
		input.Limit = 20
	}
	if input.Limit > 100 {
		input.Limit = 100
	}
	if input.Offset < 0 {
		input.Offset = 0
	}

	// Every filter is applied in the store, before pagination, so the page
	// and its total describe the same set (#739): the event types no
	// change-log view shows are excluded there rather than skipped here, and
	// only the mainline's own events are read — a research branch's edits are
	// not part of the mainline's history until a merge replays them (ADR-005).
	page, err := s.eventStore.ReadGlobalHistory(ctx, repository.GlobalHistoryQuery{
		FromTime:          input.FromTime,
		ToTime:            input.ToTime,
		IncludeEventTypes: input.EventTypes,
		ExcludeEventTypes: HistoryExcludedEventTypes(),
		BranchID:          &domain.MainBranchID,
		Limit:             input.Limit,
		Offset:            input.Offset,
	})
	if err != nil {
		return nil, fmt.Errorf("reading global history: %w", err)
	}

	// Transform events to change entries
	entries, err := s.transformStoredEvents(ctx, page.Events)
	if err != nil {
		return nil, fmt.Errorf("transforming events: %w", err)
	}

	return &ChangeHistoryResult{
		Entries:    entries,
		TotalCount: page.TotalCount,
		HasMore:    page.HasMore,
		Limit:      input.Limit,
		Offset:     input.Offset,
	}, nil
}

// transformStoredEvents converts raw StoredEvents to user-friendly ChangeEntries,
// described as the mainline sees them.
//
// Every event is classified by historyEventCatalog: a mapped event becomes one
// entry, an excluded one (a snapshot marker, an import record, a branch
// lifecycle event) none. Names, old values and parent links are resolved for
// the whole batch at once by describeEvents, so the query count does not grow
// with the number of events (#697).
func (s *HistoryService) transformStoredEvents(ctx context.Context, events []repository.StoredEvent) ([]ChangeEntry, error) {
	return s.transformStoredEventsOn(ctx, domain.MainBranchID, events)
}

// transformStoredEventsOn is transformStoredEvents as branchID sees the data:
// names resolve through the branch's overlay (resolveEntityNamesOn) and prior
// values come from the branch's view of each stream (branchVisibleStreamEvents),
// so an entity the branch created or renamed is labelled with its branch name
// and an update shows what it replaced on the branch.
func (s *HistoryService) transformStoredEventsOn(ctx context.Context, branchID domain.BranchID, events []repository.StoredEvent) ([]ChangeEntry, error) {
	shown := make([]repository.StoredEvent, 0, len(events))
	for i := range events {
		class, ok := classifyHistoryEvent(events[i].EventType)
		if !ok {
			// Unreachable for any type the store can decode:
			// TestHistoryCatalog_CoversEveryEventType fails first. Dropped rather
			// than reported as "unknown", which the ChangeEntry contract forbids.
			slog.Warn("history: event type missing from the history catalog", "event_type", events[i].EventType)
			continue
		}
		if class.Excluded() {
			continue
		}
		shown = append(shown, events[i])
	}
	if len(shown) == 0 {
		return []ChangeEntry{}, nil
	}

	desc, err := s.describeEvents(ctx, branchID, shown)
	if err != nil {
		return nil, err
	}

	entries := make([]ChangeEntry, 0, len(shown))
	for i := range shown {
		evt := &shown[i]
		class, _ := classifyHistoryEvent(evt.EventType)

		entry := ChangeEntry{
			ID:         evt.ID,
			Timestamp:  evt.Timestamp,
			EntityType: class.EntityType,
			EntityID:   evt.StreamID,
			Action:     class.Action,
			EntityName: desc.name(class.EntityType, evt.StreamID, evt),
		}
		if changes := desc.entryChanges(evt); len(changes) > 0 {
			entry.Changes = changes
		}
		if parentType, parentID, ok := desc.parent(class.EntityType, evt.StreamID); ok {
			entry.ParentEntityType = parentType
			entry.ParentEntityID = &parentID
		}

		// Extract user ID from metadata if present
		if len(evt.Metadata) > 0 {
			var metadata domain.EventMetadata
			if err := json.Unmarshal(evt.Metadata, &metadata); err == nil && metadata.UserID != "" {
				entry.UserID = &metadata.UserID
			}
		}

		entries = append(entries, entry)
	}

	return entries, nil
}

// entryChanges returns an entry's field-level changes: the stream-derived
// before/after values, or for a child link/unlink the child it names.
func (d *historyDescription) entryChanges(evt *repository.StoredEvent) map[string]FieldChange {
	switch evt.EventType {
	case "ChildLinkedToFamily", "ChildUnlinkedFromFamily":
		var link struct {
			PersonID uuid.UUID `json:"person_id"`
		}
		if err := json.Unmarshal(evt.Data, &link); err != nil {
			return nil
		}
		verb := "linked"
		if evt.EventType == "ChildUnlinkedFromFamily" {
			verb = "unlinked"
		}
		return map[string]FieldChange{
			"children": {NewValue: fmt.Sprintf("Child %s: %s", verb, d.names.personName(link.PersonID, nil))},
		}
	}
	return d.changes[evt.ID]
}
