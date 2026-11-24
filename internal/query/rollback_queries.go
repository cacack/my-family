// Package query provides CQRS query services for the genealogy application.
package query

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/google/uuid"

	"github.com/cacack/my-family/internal/domain"
	"github.com/cacack/my-family/internal/repository"
)

// ErrInvalidVersion is returned when the target version is invalid.
var ErrInvalidVersion = errors.New("invalid version: must be positive and not exceed current version")

// ErrEntityDeleted is returned when trying to rollback a deleted entity.
var ErrEntityDeleted = errors.New("entity has been deleted")

// ErrNoEvents is returned when no events exist for an entity.
var ErrNoEvents = errors.New("no events found for entity")

// RollbackService provides query operations for rollback functionality.
type RollbackService struct {
	eventStore repository.EventStore
	readStore  repository.ReadModelStore
	redactor   changeRedactor
}

// NewRollbackService creates a new rollback query service. Its restore points
// are redacted for the principal on the context only when readStore is the
// privacy decorator itself (see NewHistoryService).
func NewRollbackService(eventStore repository.EventStore, readStore repository.ReadModelStore) *RollbackService {
	return &RollbackService{
		eventStore: eventStore,
		readStore:  readStore,
		redactor:   redactorFor(readStore),
	}
}

// RestorePoint represents a point in time to which an entity can be restored.
type RestorePoint struct {
	Version      int64     `json:"version"`
	Timestamp    time.Time `json:"timestamp"`
	Action       string    `json:"action"` // "created", "updated", "deleted", "linked", "unlinked"
	Summary      string    `json:"summary"`
	IsCurrent    bool      `json:"is_current"`
	IsRestorable bool      `json:"is_restorable"` // false for deleted or current version
}

// RestorePointsResult contains paginated restore points.
type RestorePointsResult struct {
	RestorePoints []RestorePoint `json:"restore_points"`
	TotalCount    int            `json:"total_count"`
	HasMore       bool           `json:"has_more"`
	Limit         int            `json:"limit"`
	Offset        int            `json:"offset"`
}

// EntityState represents the reconstructed state of an entity at a specific version.
type EntityState struct {
	EntityType string         `json:"entity_type"`
	EntityID   uuid.UUID      `json:"entity_id"`
	Version    int64          `json:"version"`
	IsDeleted  bool           `json:"is_deleted"`
	State      map[string]any `json:"state"`
}

// RollbackChanges represents the changes needed to rollback an entity.
type RollbackChanges struct {
	EntityType     string         `json:"entity_type"`
	EntityID       uuid.UUID      `json:"entity_id"`
	CurrentVersion int64          `json:"current_version"`
	TargetVersion  int64          `json:"target_version"`
	Changes        map[string]any `json:"changes"`       // field -> value to restore
	IsRecreation   bool           `json:"is_recreation"` // true if rolling back from deleted state
}

// GetRestorePoints returns a paginated list of restore points for an entity.
func (s *RollbackService) GetRestorePoints(ctx context.Context, entityType string, entityID uuid.UUID, limit, offset int) (*RestorePointsResult, error) {
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

	// Read all events for this entity
	events, err := s.eventStore.ReadStream(ctx, entityID)
	if err != nil {
		if errors.Is(err, repository.ErrStreamNotFound) {
			return nil, ErrNoEvents
		}
		return nil, fmt.Errorf("reading event stream: %w", err)
	}

	if len(events) == 0 {
		return nil, ErrNoEvents
	}

	// Sort events by version (ascending)
	sort.Slice(events, func(i, j int) bool {
		return events[i].Version < events[j].Version
	})

	totalCount := len(events)
	currentVersion := events[len(events)-1].Version
	isDeleted := s.isDeletedEvent(events[len(events)-1].EventType)

	// Build restore points (newest first for display)
	allPoints := make([]RestorePoint, 0, len(events))
	for i := len(events) - 1; i >= 0; i-- {
		evt := events[i]
		action := s.eventTypeToAction(evt.EventType)
		summary := s.buildChangeSummary(evt)

		point := RestorePoint{
			Version:      evt.Version,
			Timestamp:    evt.Timestamp,
			Action:       action,
			Summary:      summary,
			IsCurrent:    evt.Version == currentVersion,
			IsRestorable: evt.Version != currentVersion && !isDeleted,
		}
		allPoints = append(allPoints, point)
	}

	// Apply pagination
	start := offset
	if start >= len(allPoints) {
		return s.redactor.restorePoints(ctx, strings.ToLower(entityType), entityID, &RestorePointsResult{
			RestorePoints: []RestorePoint{},
			TotalCount:    totalCount,
			HasMore:       false,
			Limit:         limit,
			Offset:        offset,
		})
	}

	end := start + limit
	if end > len(allPoints) {
		end = len(allPoints)
	}

	// The entity types are capitalized here ("Person"), lower case in the
	// history's vocabulary the redactor speaks.
	return s.redactor.restorePoints(ctx, strings.ToLower(entityType), entityID, &RestorePointsResult{
		RestorePoints: allPoints[start:end],
		TotalCount:    totalCount,
		HasMore:       end < len(allPoints),
		Limit:         limit,
		Offset:        offset,
	})
}

// GetStateAtVersion reconstructs the entity state at a specific version.
func (s *RollbackService) GetStateAtVersion(ctx context.Context, entityType string, entityID uuid.UUID, version int64) (*EntityState, error) {
	if version < 1 {
		return nil, ErrInvalidVersion
	}

	// Read all events for this entity
	events, err := s.eventStore.ReadStream(ctx, entityID)
	if err != nil {
		if errors.Is(err, repository.ErrStreamNotFound) {
			return nil, ErrNoEvents
		}
		return nil, fmt.Errorf("reading event stream: %w", err)
	}

	if len(events) == 0 {
		return nil, ErrNoEvents
	}

	// Sort events by version (ascending)
	sort.Slice(events, func(i, j int) bool {
		return events[i].Version < events[j].Version
	})

	// Validate version
	maxVersion := events[len(events)-1].Version
	if version > maxVersion {
		return nil, ErrInvalidVersion
	}

	// Replay events up to target version
	state := make(map[string]any)
	var isDeleted bool

	for _, evt := range events {
		if evt.Version > version {
			break
		}

		isDeleted, err = s.applyEventToState(evt, state)
		if err != nil {
			return nil, fmt.Errorf("applying event: %w", err)
		}
	}

	return &EntityState{
		EntityType: entityType,
		EntityID:   entityID,
		Version:    version,
		IsDeleted:  isDeleted,
		State:      state,
	}, nil
}

// ComputeRollbackChanges computes the changes needed to rollback from current to target version.
func (s *RollbackService) ComputeRollbackChanges(ctx context.Context, entityType string, entityID uuid.UUID, targetVersion int64) (*RollbackChanges, error) {
	if targetVersion < 1 {
		return nil, ErrInvalidVersion
	}

	// Read all events for this entity
	events, err := s.eventStore.ReadStream(ctx, entityID)
	if err != nil {
		if errors.Is(err, repository.ErrStreamNotFound) {
			return nil, ErrNoEvents
		}
		return nil, fmt.Errorf("reading event stream: %w", err)
	}

	if len(events) == 0 {
		return nil, ErrNoEvents
	}

	// Sort events by version (ascending)
	sort.Slice(events, func(i, j int) bool {
		return events[i].Version < events[j].Version
	})

	// Validate target version
	currentVersion := events[len(events)-1].Version
	if targetVersion > currentVersion {
		return nil, ErrInvalidVersion
	}
	if targetVersion == currentVersion {
		// Nothing to rollback
		return &RollbackChanges{
			EntityType:     entityType,
			EntityID:       entityID,
			CurrentVersion: currentVersion,
			TargetVersion:  targetVersion,
			Changes:        make(map[string]any),
		}, nil
	}

	// Check if current state is deleted
	isCurrentlyDeleted := s.isDeletedEvent(events[len(events)-1].EventType)

	// Get target state
	targetState, err := s.GetStateAtVersion(ctx, entityType, entityID, targetVersion)
	if err != nil {
		return nil, fmt.Errorf("getting target state: %w", err)
	}

	// Get current state
	currentState, err := s.GetStateAtVersion(ctx, entityType, entityID, currentVersion)
	if err != nil {
		return nil, fmt.Errorf("getting current state: %w", err)
	}

	// Compute diff: what values from target state differ from current state
	changes := s.computeStateDiff(currentState.State, targetState.State)
	if err := keepBackfilledCoordinates(events, targetVersion, changes); err != nil {
		return nil, err
	}
	if mergedSince(events, targetVersion) {
		keepPrivacyMarks(changes, currentState.State, targetState.State)
	}

	return &RollbackChanges{
		EntityType:     entityType,
		EntityID:       entityID,
		CurrentVersion: currentVersion,
		TargetVersion:  targetVersion,
		Changes:        changes,
		IsRecreation:   isCurrentlyDeleted && !targetState.IsDeleted,
	}, nil
}

// keepBackfilledCoordinates stops a rollback to before an entity's backfill
// from clearing the coordinates it logged. They came from an import before
// #1225 and were already in the record at the target, only not in its log.
func keepBackfilledCoordinates(events []repository.StoredEvent, targetVersion int64, changes map[string]any) error {
	for _, evt := range events {
		if evt.EventType != "ImportedValuesBackfilled" {
			continue
		}
		if evt.Version <= targetVersion {
			return nil
		}
		decoded, err := evt.DecodeEvent()
		if err != nil {
			return fmt.Errorf("decoding backfill: %w", err)
		}
		backfilled := map[string]any{}
		if e, ok := decoded.(domain.ImportedValuesBackfilled); ok && e.EntityType == domain.BackfillPerson {
			setCoordinateState(backfilled, "birth_place", e.BirthPlaceLat, e.BirthPlaceLong)
			setCoordinateState(backfilled, "death_place", e.DeathPlaceLat, e.DeathPlaceLong)
		}
		for key := range backfilled {
			if v, ok := changes[key]; ok && v == nil {
				delete(changes, key)
			}
		}
		return nil
	}
	return nil
}

// mergedSince reports whether a person merge comes after the target version.
func mergedSince(events []repository.StoredEvent, targetVersion int64) bool {
	for _, evt := range events {
		if evt.Version > targetVersion && evt.EventType == "PersonMerged" {
			return true
		}
	}
	return false
}

// keepPrivacyMarks stops a rollback across a person merge from lowering the
// privacy marks. The rollback does not undo the merge's transfers, so the
// survivor still holds what the merged person's record protected; it keeps
// the more protective of the current and target marks, as the merge did
// (ADR-008).
func keepPrivacyMarks(changes, current, target map[string]any) {
	currentRestriction := domain.Restriction(stateString(current, "restriction"))
	targetRestriction := domain.Restriction(stateString(target, "restriction"))
	if currentRestriction.IsPrivate() && !targetRestriction.IsPrivate() {
		if currentRestriction.WithPrivate(false) == targetRestriction {
			delete(changes, "restriction")
		} else {
			changes["restriction"] = string(targetRestriction.WithPrivate(true))
		}
	}

	currentOverride := domain.LivingOverride(stateString(current, "living_override"))
	targetOverride := domain.LivingOverride(stateString(target, "living_override"))
	if targetOverride.Protection() < currentOverride.Protection() {
		delete(changes, "living_override")
	}
}

// stateString returns a state field as a string, or "" when it is unset.
func stateString(state map[string]any, field string) string {
	s, _ := state[field].(string)
	return s
}

// setRestrictionState records a created record's restriction in the rollback
// state, so rolling back to it restores a private flag set at import.
func setRestrictionState(state map[string]any, r domain.Restriction) {
	if r != "" {
		state["restriction"] = string(r)
	}
}

// applyEventToState applies a single event to the state map and returns whether the entity is deleted.
func (s *RollbackService) applyEventToState(evt repository.StoredEvent, state map[string]any) (bool, error) {
	domainEvent, err := evt.DecodeEvent()
	if err != nil {
		return false, err
	}

	switch e := domainEvent.(type) {
	// Person events
	case domain.PersonCreated:
		state["id"] = e.PersonID.String()
		state["given_name"] = e.GivenName
		state["surname"] = e.Surname
		if e.Gender != "" {
			state["gender"] = string(e.Gender)
		}
		if e.BirthDate != nil {
			state["birth_date"] = e.BirthDate.String()
		}
		if e.BirthPlace != "" {
			state["birth_place"] = e.BirthPlace
		}
		if e.DeathDate != nil {
			state["death_date"] = e.DeathDate.String()
		}
		if e.DeathPlace != "" {
			state["death_place"] = e.DeathPlace
		}
		setCoordinateState(state, "birth_place", e.BirthPlaceLat, e.BirthPlaceLong)
		setCoordinateState(state, "death_place", e.DeathPlaceLat, e.DeathPlaceLong)
		setOriginState(state, "birth_place", e.BirthPlaceOrigin)
		setOriginState(state, "death_place", e.DeathPlaceOrigin)
		if e.Notes != "" {
			state["notes"] = e.Notes
		}
		setRestrictionState(state, e.Restriction)
		if e.LivingOverride != "" {
			state["living_override"] = string(e.LivingOverride)
		}
		return false, nil

	case domain.PersonUpdated:
		for field, value := range e.Changes {
			if value == nil {
				delete(state, field)
			} else {
				state[field] = normalizeValue(value)
			}
		}
		return false, nil

	case domain.PersonDeleted:
		// Mark as deleted but preserve state for potential restore
		return true, nil

	// A merge is on the survivor's stream and sets the fields it resolved,
	// the privacy marks among them, as an update would.
	case domain.PersonMerged:
		for field, value := range e.ResolvedFields {
			state[field] = normalizeValue(value)
		}
		return false, nil

	// A primary name sets the person's given name and surname, as the
	// projection does (docs/CONVENTIONS.md "Person Name and Primary Name").
	case domain.NameAdded:
		if e.IsPrimary {
			state["given_name"] = e.GivenName
			state["surname"] = e.Surname
		}
		return false, nil

	case domain.NameUpdated:
		if e.IsPrimary {
			state["given_name"] = e.GivenName
			state["surname"] = e.Surname
		}
		return false, nil

	// Family events
	case domain.FamilyCreated:
		state["id"] = e.FamilyID.String()
		setRestrictionState(state, e.Restriction)
		if e.Partner1ID != nil {
			state["partner1_id"] = e.Partner1ID.String()
		}
		if e.Partner2ID != nil {
			state["partner2_id"] = e.Partner2ID.String()
		}
		if e.RelationshipType != "" {
			state["relationship_type"] = string(e.RelationshipType)
		}
		if e.MarriageDate != nil {
			state["marriage_date"] = e.MarriageDate.String()
		}
		if e.MarriagePlace != "" {
			state["marriage_place"] = e.MarriagePlace
		}
		setCoordinateState(state, "marriage_place", e.MarriagePlaceLat, e.MarriagePlaceLong)
		setOriginState(state, "marriage_place", e.MarriagePlaceOrigin)
		return false, nil

	case domain.FamilyUpdated:
		for field, value := range e.Changes {
			if value == nil {
				delete(state, field)
			} else {
				state[field] = normalizeValue(value)
			}
		}
		return false, nil

	case domain.FamilyDeleted:
		return true, nil

	case domain.ChildLinkedToFamily:
		// Track children as a list
		children, ok := state["children"].([]string)
		if !ok {
			children = []string{}
		}
		children = append(children, e.PersonID.String())
		state["children"] = children
		return false, nil

	case domain.ChildUnlinkedFromFamily:
		children, ok := state["children"].([]string)
		if ok {
			newChildren := make([]string, 0, len(children))
			for _, c := range children {
				if c != e.PersonID.String() {
					newChildren = append(newChildren, c)
				}
			}
			state["children"] = newChildren
		}
		return false, nil

	// Source events
	case domain.SourceCreated:
		state["id"] = e.SourceID.String()
		setRestrictionState(state, e.Restriction)
		state["title"] = e.Title
		if e.SourceType != "" {
			state["source_type"] = string(e.SourceType)
		}
		if e.Author != "" {
			state["author"] = e.Author
		}
		if e.Publisher != "" {
			state["publisher"] = e.Publisher
		}
		if e.PublishDate != nil {
			state["publish_date"] = e.PublishDate.String()
		}
		if e.URL != "" {
			state["url"] = e.URL
		}
		if e.RepositoryName != "" {
			state["repository_name"] = e.RepositoryName
		}
		if e.CollectionName != "" {
			state["collection_name"] = e.CollectionName
		}
		if e.CallNumber != "" {
			state["call_number"] = e.CallNumber
		}
		if e.Notes != "" {
			state["notes"] = e.Notes
		}
		return false, nil

	case domain.SourceUpdated:
		for field, value := range e.Changes {
			if value == nil {
				delete(state, field)
			} else {
				state[field] = normalizeValue(value)
			}
		}
		return false, nil

	case domain.SourceDeleted:
		return true, nil

	// Citation events
	case domain.CitationCreated:
		state["id"] = e.CitationID.String()
		setRestrictionState(state, e.Restriction)
		state["source_id"] = e.SourceID.String()
		state["fact_type"] = string(e.FactType)
		state["fact_owner_id"] = e.FactOwnerID.String()
		if e.FactID != nil {
			state["fact_id"] = e.FactID.String()
		}
		if e.Page != "" {
			state["page"] = e.Page
		}
		if e.Volume != "" {
			state["volume"] = e.Volume
		}
		if e.SourceQuality != "" {
			state["source_quality"] = string(e.SourceQuality)
		}
		if e.InformantType != "" {
			state["informant_type"] = string(e.InformantType)
		}
		if e.EvidenceType != "" {
			state["evidence_type"] = string(e.EvidenceType)
		}
		if e.QuotedText != "" {
			state["quoted_text"] = e.QuotedText
		}
		if e.Analysis != "" {
			state["analysis"] = e.Analysis
		}
		if e.TemplateID != "" {
			state["template_id"] = e.TemplateID
		}
		return false, nil

	case domain.CitationUpdated:
		for field, value := range e.Changes {
			if value == nil {
				delete(state, field)
			} else {
				state[field] = normalizeValue(value)
			}
		}
		return false, nil

	case domain.CitationDeleted:
		return true, nil

	// Media events
	case domain.MediaCreated:
		state["id"] = e.MediaID.String()
		state["entity_type"] = e.EntityType
		state["entity_id"] = e.EntityID.String()
		state["title"] = e.Title
		if e.Description != "" {
			state["description"] = e.Description
		}
		if e.MimeType != "" {
			state["mime_type"] = e.MimeType
		}
		if e.MediaType != "" {
			state["media_type"] = string(e.MediaType)
		}
		if e.Filename != "" {
			state["filename"] = e.Filename
		}
		if e.FileSize > 0 {
			state["file_size"] = e.FileSize
		}
		if e.GedcomXref != "" {
			state["gedcom_xref"] = e.GedcomXref
		}
		setCropState(state, "crop_left", e.CropLeft)
		setCropState(state, "crop_top", e.CropTop)
		setCropState(state, "crop_width", e.CropWidth)
		setCropState(state, "crop_height", e.CropHeight)
		setRestrictionState(state, e.Restriction)
		return false, nil

	case domain.MediaUpdated:
		for field, value := range e.Changes {
			if value == nil {
				delete(state, field)
			} else {
				state[field] = normalizeValue(value)
			}
		}
		return false, nil

	// A backfill logs a person's imported coordinates, which an update may
	// since have changed (#1604).
	case domain.ImportedValuesBackfilled:
		if e.EntityType == domain.BackfillPerson {
			setCoordinateState(state, "birth_place", e.BirthPlaceLat, e.BirthPlaceLong)
			setCoordinateState(state, "death_place", e.DeathPlaceLat, e.DeathPlaceLong)
		}
		return false, nil

	case domain.MediaContentAttached:
		// The attached file is not rolled back: no command removes content.
		return false, nil

	case domain.MediaDeleted:
		return true, nil

	default:
		// Unknown event type, skip
		return false, nil
	}
}

// setCoordinateState records a place's coordinate pair under the change keys
// an update writes (prefix+"_lat", prefix+"_long"), so a rollback restores or
// clears coordinates as it does any other field (#1604).
func setCoordinateState(state map[string]any, prefix string, lat, long *string) {
	for key, v := range map[string]*string{prefix + "_lat": lat, prefix + "_long": long} {
		if v != nil {
			state[key] = *v
		} else {
			delete(state, key)
		}
	}
}

// setOriginState records the origin a record was created with under the
// change key an update writes (prefix+"_origin", #1563). None is imported,
// which an update records as a clear, so it removes the key. A backfill does
// not set it: its pair is imported unless an update has since said otherwise.
func setOriginState(state map[string]any, prefix string, origin *domain.CoordinateOrigin) {
	if origin == nil {
		delete(state, prefix+"_origin")
		return
	}
	state[prefix+"_origin"] = origin.ChangeValue()
}

// setCropState records a crop field an imported item was created with (#1584).
func setCropState(state map[string]any, field string, crop *int) {
	if crop != nil {
		state[field] = *crop
	}
}

// normalizeValue converts values to string representation for consistent comparison.
func normalizeValue(v any) any {
	switch val := v.(type) {
	case uuid.UUID:
		return val.String()
	case *uuid.UUID:
		if val == nil {
			return nil
		}
		return val.String()
	case domain.GenDate:
		return val.String()
	case *domain.GenDate:
		if val == nil {
			return nil
		}
		return val.String()
	default:
		return v
	}
}

// computeStateDiff computes differences between current and target state.
// Returns a map of field -> target value for fields that differ.
func (s *RollbackService) computeStateDiff(current, target map[string]any) map[string]any {
	changes := make(map[string]any)

	// Check fields in target that differ from current
	for field, targetValue := range target {
		currentValue, exists := current[field]
		if !exists || !valuesEqual(currentValue, targetValue) {
			changes[field] = targetValue
		}
	}

	// Check fields in current that don't exist in target (need to be removed)
	for field := range current {
		if _, exists := target[field]; !exists {
			changes[field] = nil
		}
	}

	return changes
}

// valuesEqual compares two values for equality.
func valuesEqual(a, b any) bool {
	// Handle nil cases
	if a == nil && b == nil {
		return true
	}
	if a == nil || b == nil {
		return false
	}

	// Convert to strings for comparison
	return fmt.Sprintf("%v", a) == fmt.Sprintf("%v", b)
}

// eventTypeToAction maps event types to user-friendly action names.
func (s *RollbackService) eventTypeToAction(eventType string) string {
	switch eventType {
	case "PersonCreated", "FamilyCreated", "SourceCreated", "CitationCreated":
		return "created"
	case "PersonUpdated", "FamilyUpdated", "SourceUpdated", "CitationUpdated":
		return "updated"
	case "PersonDeleted", "FamilyDeleted", "SourceDeleted", "CitationDeleted":
		return "deleted"
	case "ChildLinkedToFamily":
		return "linked"
	case "ChildUnlinkedFromFamily":
		return "unlinked"
	default:
		return "unknown"
	}
}

// isDeletedEvent checks if an event type represents deletion.
func (s *RollbackService) isDeletedEvent(eventType string) bool {
	return strings.HasSuffix(eventType, "Deleted")
}

// buildChangeSummary creates a human-readable summary of what changed in an event.
func (s *RollbackService) buildChangeSummary(evt repository.StoredEvent) string {
	domainEvent, err := evt.DecodeEvent()
	if err != nil {
		return "unknown change"
	}

	switch e := domainEvent.(type) {
	case domain.PersonCreated:
		return fmt.Sprintf("created %s %s", e.GivenName, e.Surname)
	case domain.PersonUpdated:
		return s.summarizeChanges("updated", e.Changes)
	case domain.PersonDeleted:
		if e.Reason != "" {
			return fmt.Sprintf("deleted: %s", e.Reason)
		}
		return "deleted"

	case domain.FamilyCreated:
		return "created family"
	case domain.FamilyUpdated:
		return s.summarizeChanges("updated", e.Changes)
	case domain.FamilyDeleted:
		return "deleted"
	case domain.ChildLinkedToFamily:
		return fmt.Sprintf("linked child %s", e.PersonID.String()[:8])
	case domain.ChildUnlinkedFromFamily:
		return fmt.Sprintf("unlinked child %s", e.PersonID.String()[:8])

	case domain.SourceCreated:
		return fmt.Sprintf("created source: %s", truncate(e.Title, 40))
	case domain.SourceUpdated:
		return s.summarizeChanges("updated", e.Changes)
	case domain.SourceDeleted:
		return "deleted"

	case domain.CitationCreated:
		return fmt.Sprintf("created citation for %s", e.FactType)
	case domain.CitationUpdated:
		return s.summarizeChanges("updated", e.Changes)
	case domain.CitationDeleted:
		return "deleted"

	default:
		return "unknown change"
	}
}

// summarizeChanges creates a summary of field changes.
func (s *RollbackService) summarizeChanges(action string, changes map[string]any) string {
	if len(changes) == 0 {
		return action
	}

	fields := make([]string, 0, len(changes))
	for field := range changes {
		fields = append(fields, field)
	}
	sort.Strings(fields)

	if len(fields) <= 3 {
		return fmt.Sprintf("%s %s", action, strings.Join(fields, ", "))
	}
	return fmt.Sprintf("%s %s and %d more", action, strings.Join(fields[:3], ", "), len(fields)-3)
}

// truncate shortens a string to the specified length.
func truncate(s string, maxLen int) string {
	if len(s) <= maxLen {
		return s
	}
	return s[:maxLen-3] + "..."
}
