package command

import (
	"context"
	"errors"
	"fmt"

	"github.com/google/uuid"

	"github.com/cacack/my-family/internal/domain"
	"github.com/cacack/my-family/internal/repository"
)

// Merge-related errors.
var (
	ErrSamePersonMerge     = errors.New("cannot merge a person with themselves")
	ErrCircularMerge       = errors.New("cannot merge: persons have an ancestor-descendant relationship")
	ErrChildFamilyConflict = errors.New("cannot merge: both persons are children in different families")
)

// MergePersonsInput contains the data for merging two persons.
type MergePersonsInput struct {
	SurvivorID      uuid.UUID
	MergedID        uuid.UUID
	SurvivorVersion int64
	MergedVersion   int64
	FieldResolution map[string]string // field -> "survivor" or "merged"
}

// MergeSummary contains statistics about the merge operation.
type MergeSummary struct {
	MergedPersonName     string
	FieldsUpdated        []string
	FamiliesUpdated      int
	CitationsTransferred int
	NamesTransferred     int
	EventsTransferred    int
	MediaTransferred     int
}

// MergePersonsResult contains the result of merging two persons.
type MergePersonsResult struct {
	SurvivorID uuid.UUID
	Version    int64
	Summary    MergeSummary
}

// MergePersons merges two person records, consolidating data from the merged person
// into the survivor. The merged person is deleted after the merge.
func (h *Handler) MergePersons(ctx context.Context, input MergePersonsInput) (*MergePersonsResult, error) {
	// 1. Validate not same person
	if input.SurvivorID == input.MergedID {
		return nil, ErrSamePersonMerge
	}

	// 2. Fetch and validate both persons exist with version check
	survivor, merged, err := h.validateMergePersons(ctx, input)
	if err != nil {
		return nil, err
	}

	// 3. Check for circular ancestry and child-family conflicts
	if err := h.validateMergeRelationships(ctx, input.SurvivorID, input.MergedID); err != nil {
		return nil, err
	}

	// 4. Build merge event and execute
	event, fieldsUpdated, err := h.buildMergeEvent(ctx, input, survivor, merged)
	if err != nil {
		return nil, err
	}

	// 5. Execute command (append + project)
	version, err := h.execute(ctx, input.SurvivorID.String(), "Person", []domain.Event{event}, input.SurvivorVersion)
	if err != nil {
		return nil, err
	}

	// 6. Build summary
	summary := MergeSummary{
		MergedPersonName:     merged.FullName,
		FieldsUpdated:        fieldsUpdated,
		FamiliesUpdated:      len(event.AffectedFamilyIDs),
		CitationsTransferred: len(event.AffectedCitationIDs),
		NamesTransferred:     len(event.TransferredNameIDs),
		EventsTransferred:    len(event.TransferredEventIDs),
		MediaTransferred:     len(event.TransferredMediaIDs),
	}

	return &MergePersonsResult{
		SurvivorID: input.SurvivorID,
		Version:    version,
		Summary:    summary,
	}, nil
}

// validateMergePersons fetches and validates both persons exist with correct versions.
func (h *Handler) validateMergePersons(ctx context.Context, input MergePersonsInput) (*repository.PersonReadModel, *repository.PersonReadModel, error) {
	survivor, err := h.readStore.GetPerson(ctx, input.SurvivorID)
	if err != nil {
		return nil, nil, err
	}
	if survivor == nil {
		return nil, nil, fmt.Errorf("%w: survivor not found", ErrPersonNotFound)
	}

	merged, err := h.readStore.GetPerson(ctx, input.MergedID)
	if err != nil {
		return nil, nil, err
	}
	if merged == nil {
		return nil, nil, fmt.Errorf("%w: merged person not found", ErrPersonNotFound)
	}

	// Check versions for optimistic locking
	if survivor.Version != input.SurvivorVersion {
		return nil, nil, repository.ErrConcurrencyConflict
	}
	if merged.Version != input.MergedVersion {
		return nil, nil, repository.ErrConcurrencyConflict
	}

	return survivor, merged, nil
}

// validateMergeRelationships checks for circular ancestry and child-family conflicts.
func (h *Handler) validateMergeRelationships(ctx context.Context, survivorID, mergedID uuid.UUID) error {
	// Circular ancestry check: neither person can be an ancestor of the other
	if isAnc, err := h.isAncestor(ctx, mergedID, survivorID); err != nil {
		return err
	} else if isAnc {
		return ErrCircularMerge
	}
	if isAnc, err := h.isAncestor(ctx, survivorID, mergedID); err != nil {
		return err
	} else if isAnc {
		return ErrCircularMerge
	}

	// Child-family conflict check: if both are children in different families, block
	survivorChildFamily, err := h.readStore.GetChildFamily(ctx, survivorID)
	if err != nil {
		return err
	}
	mergedChildFamily, err := h.readStore.GetChildFamily(ctx, mergedID)
	if err != nil {
		return err
	}
	if survivorChildFamily != nil && mergedChildFamily != nil &&
		survivorChildFamily.ID != mergedChildFamily.ID {
		return ErrChildFamilyConflict
	}

	return nil
}

// buildMergeEvent constructs the PersonMerged event with all affected entities.
func (h *Handler) buildMergeEvent(ctx context.Context, input MergePersonsInput, survivor, merged *repository.PersonReadModel) (domain.PersonMerged, []string, error) {
	// Build merged person snapshot for audit trail
	mergedSnapshot := buildPersonSnapshot(merged)

	// Resolve fields based on field resolution strategy
	resolvedFields, fieldsUpdated := resolveFields(survivor, merged, input.FieldResolution)

	// Collect affected entities
	affectedFamilies, err := h.collectAffectedFamilies(ctx, input.MergedID)
	if err != nil {
		return domain.PersonMerged{}, nil, err
	}

	affectedCitations, err := h.collectAffectedCitations(ctx, input.MergedID)
	if err != nil {
		return domain.PersonMerged{}, nil, err
	}

	transferredNames, err := h.collectTransferredNames(ctx, input.MergedID)
	if err != nil {
		return domain.PersonMerged{}, nil, err
	}

	transferredEvents, err := h.collectTransferredEvents(ctx, input.MergedID)
	if err != nil {
		return domain.PersonMerged{}, nil, err
	}

	transferredMedia, err := h.collectTransferredMedia(ctx, input.MergedID)
	if err != nil {
		return domain.PersonMerged{}, nil, err
	}

	event := domain.NewPersonMerged(
		input.SurvivorID,
		input.MergedID,
		mergedSnapshot,
		resolvedFields,
		affectedFamilies,
		affectedCitations,
		transferredNames,
		transferredEvents,
		transferredMedia,
	)

	return event, fieldsUpdated, nil
}

// buildPersonSnapshot creates a map representation of a person for the event audit trail.
func buildPersonSnapshot(p *repository.PersonReadModel) map[string]any {
	snapshot := map[string]any{
		"id":         p.ID.String(),
		"given_name": p.GivenName,
		"surname":    p.Surname,
		"full_name":  p.FullName,
		"version":    p.Version,
	}

	if p.Gender != "" {
		snapshot["gender"] = string(p.Gender)
	}
	if p.BirthDateRaw != "" {
		snapshot["birth_date"] = p.BirthDateRaw
	}
	if p.BirthPlace != "" {
		snapshot["birth_place"] = p.BirthPlace
	}
	if p.DeathDateRaw != "" {
		snapshot["death_date"] = p.DeathDateRaw
	}
	if p.DeathPlace != "" {
		snapshot["death_place"] = p.DeathPlace
	}
	if p.Notes != "" {
		snapshot["notes"] = p.Notes
	}
	if p.ResearchStatus != "" {
		snapshot["research_status"] = string(p.ResearchStatus)
	}

	return snapshot
}

// resolveFields determines which values to use for each field based on the resolution strategy.
// Returns the resolved fields map and a list of field names that were updated from merged person.
func resolveFields(survivor, merged *repository.PersonReadModel, resolution map[string]string) (map[string]any, []string) {
	resolved := make(map[string]any)
	var fieldsUpdated []string

	// Default is "survivor" wins unless explicitly set to "merged"
	fields := []struct {
		name          string
		survivorValue string
		mergedValue   string
	}{
		{"given_name", survivor.GivenName, merged.GivenName},
		{"surname", survivor.Surname, merged.Surname},
		{"gender", string(survivor.Gender), string(merged.Gender)},
		{"birth_date", survivor.BirthDateRaw, merged.BirthDateRaw},
		{"birth_place", survivor.BirthPlace, merged.BirthPlace},
		{"death_date", survivor.DeathDateRaw, merged.DeathDateRaw},
		{"death_place", survivor.DeathPlace, merged.DeathPlace},
		{"notes", survivor.Notes, merged.Notes},
		{"research_status", string(survivor.ResearchStatus), string(merged.ResearchStatus)},
	}

	for _, field := range fields {
		source := "survivor"
		if resolution != nil {
			if s, ok := resolution[field.name]; ok {
				source = s
			}
		}

		// Use merged value if:
		// 1. Explicitly requested via field_resolution
		// 2. OR survivor value is empty and merged has a value
		useMerged := source == "merged" || (field.survivorValue == "" && field.mergedValue != "")

		if useMerged && field.mergedValue != "" && field.mergedValue != field.survivorValue {
			resolved[field.name] = field.mergedValue
			fieldsUpdated = append(fieldsUpdated, field.name)
		}
	}

	return resolved, fieldsUpdated
}

// collectAffectedFamilies returns IDs of families where merged person is a partner.
func (h *Handler) collectAffectedFamilies(ctx context.Context, mergedID uuid.UUID) ([]uuid.UUID, error) {
	families, err := h.readStore.GetFamiliesForPerson(ctx, mergedID)
	if err != nil {
		return nil, err
	}

	ids := make([]uuid.UUID, len(families))
	for i, f := range families {
		ids[i] = f.ID
	}
	return ids, nil
}

// collectAffectedCitations returns IDs of citations linked to merged person.
func (h *Handler) collectAffectedCitations(ctx context.Context, mergedID uuid.UUID) ([]uuid.UUID, error) {
	citations, err := h.readStore.GetCitationsForPerson(ctx, mergedID)
	if err != nil {
		return nil, err
	}

	ids := make([]uuid.UUID, len(citations))
	for i, c := range citations {
		ids[i] = c.ID
	}
	return ids, nil
}

// collectTransferredNames returns IDs of alternate names from merged person.
func (h *Handler) collectTransferredNames(ctx context.Context, mergedID uuid.UUID) ([]uuid.UUID, error) {
	names, err := h.readStore.GetPersonNames(ctx, mergedID)
	if err != nil {
		return nil, err
	}

	ids := make([]uuid.UUID, len(names))
	for i, n := range names {
		ids[i] = n.ID
	}
	return ids, nil
}

// collectTransferredEvents returns IDs of life events from merged person.
func (h *Handler) collectTransferredEvents(ctx context.Context, mergedID uuid.UUID) ([]uuid.UUID, error) {
	events, err := h.readStore.ListEventsForPerson(ctx, mergedID)
	if err != nil {
		return nil, err
	}

	ids := make([]uuid.UUID, len(events))
	for i, e := range events {
		ids[i] = e.ID
	}
	return ids, nil
}

// collectTransferredMedia returns IDs of media from merged person.
func (h *Handler) collectTransferredMedia(ctx context.Context, mergedID uuid.UUID) ([]uuid.UUID, error) {
	media, _, err := h.readStore.ListMediaForEntity(ctx, "person", mergedID, repository.ListOptions{Limit: 10000})
	if err != nil {
		return nil, err
	}

	ids := make([]uuid.UUID, len(media))
	for i, m := range media {
		ids[i] = m.ID
	}
	return ids, nil
}
