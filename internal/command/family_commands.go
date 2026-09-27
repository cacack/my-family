package command

import (
	"context"
	"errors"
	"fmt"

	"github.com/google/uuid"

	"github.com/cacack/my-family/internal/domain"
	"github.com/cacack/my-family/internal/repository"
)

// familyStreamType is the event-store stream type for family aggregates. It
// stays lowercase to match the events already in the log (RollbackFamily writes
// "Family" through the shared rollback path — a pre-existing inconsistency;
// nothing reads stream_type, history derives entity type from the event type).
const familyStreamType = "family"

// Family-related errors.
var (
	ErrFamilyNotFound     = errors.New("family not found")
	ErrChildAlreadyLinked = errors.New("child already linked to a family")
	ErrChildNotInFamily   = errors.New("child not in this family")
	ErrCircularAncestry   = errors.New("circular ancestry detected")
	ErrInvalidFamilyInput = errors.New("invalid family input")
	ErrFamilyHasChildren  = errors.New("family has children and cannot be deleted")
)

// CreateFamilyInput contains the data for creating a family.
type CreateFamilyInput struct {
	Partner1ID       *uuid.UUID
	Partner2ID       *uuid.UUID
	RelationshipType string
	MarriageDate     string
	MarriagePlace    string
}

// CreateFamilyResult contains the result of creating a family.
type CreateFamilyResult struct {
	ID      uuid.UUID
	Version int64
}

// CreateFamily creates a new family unit.
func (h *Handler) CreateFamily(ctx context.Context, input CreateFamilyInput) (*CreateFamilyResult, error) {
	// Validate at least one partner
	if input.Partner1ID == nil && input.Partner2ID == nil {
		return nil, errors.New("invalid family input: at least one partner is required")
	}

	// Validate partners exist if specified
	if input.Partner1ID != nil {
		p, err := h.readStore.GetPerson(ctx, h.branchID, *input.Partner1ID)
		if err != nil {
			return nil, fmt.Errorf("getting partner1: %w", err)
		}
		if p == nil {
			return nil, errors.New("invalid family input: partner1 not found")
		}
	}
	if input.Partner2ID != nil {
		p, err := h.readStore.GetPerson(ctx, h.branchID, *input.Partner2ID)
		if err != nil {
			return nil, fmt.Errorf("getting partner2: %w", err)
		}
		if p == nil {
			return nil, errors.New("invalid family input: partner2 not found")
		}
	}

	// Parse relationship type
	relType := domain.RelationUnknown
	if input.RelationshipType != "" {
		relType = domain.RelationType(input.RelationshipType)
	}

	// Create family entity
	family := domain.NewFamily()
	family.Partner1ID = input.Partner1ID
	family.Partner2ID = input.Partner2ID
	family.RelationshipType = relType

	if input.MarriageDate != "" {
		md := domain.ParseGenDate(input.MarriageDate)
		family.MarriageDate = &md
	}
	if input.MarriagePlace != "" {
		family.MarriagePlace = input.MarriagePlace
	}

	// Validate
	if err := family.Validate(); err != nil {
		return nil, errors.New("invalid family input: " + err.Error())
	}

	// Create event using the helper function
	event := domain.NewFamilyCreated(family)

	// Execute command (append + project) on the handler's branch scope.
	// expectedVersion 0 matches a fresh stream, as it always has for creates here.
	// -1, not 0: the stream does not exist yet. Both SQL backends only insert the
	// `streams` parent row for a first append when expectedVersion is -1, so
	// passing 0 here wrote an event referencing a missing parent and failed the
	// foreign key on every real backend (memory has no FK, which is why the
	// command tests never caught it).
	version, err := h.execute(ctx, family.ID.String(), familyStreamType, []domain.Event{event}, -1)
	if err != nil {
		return nil, fmt.Errorf("appending family created event: %w", err)
	}

	return &CreateFamilyResult{
		ID:      family.ID,
		Version: version,
	}, nil
}

// UpdateFamilyInput contains the data for updating a family.
type UpdateFamilyInput struct {
	ID         uuid.UUID
	Partner1ID *uuid.UUID
	Partner2ID *uuid.UUID
	// ClearPartner1 and ClearPartner2 remove a partner from the family. Each is
	// exclusive with setting the same partner in the same update.
	ClearPartner1    bool
	ClearPartner2    bool
	RelationshipType *string
	MarriageDate     *string
	MarriagePlace    *string
	Version          int64
}

// UpdateFamilyResult contains the result of updating a family.
type UpdateFamilyResult struct {
	Version int64
}

// UpdateFamily updates an existing family.
func (h *Handler) UpdateFamily(ctx context.Context, input UpdateFamilyInput) (*UpdateFamilyResult, error) {
	// Check family exists
	family, err := h.readStore.GetFamily(ctx, h.branchID, input.ID)
	if err != nil {
		return nil, fmt.Errorf("getting family: %w", err)
	}
	if family == nil {
		return nil, ErrFamilyNotFound
	}

	if err := h.validateFamilyUpdate(ctx, family, input); err != nil {
		return nil, err
	}

	// Build changes map. Values are written in their JSON shape — the form
	// every reader of the stored event decodes (issue #848): IDs and the
	// relationship type as strings, the marriage date as its raw text (nil
	// clears it), matching how PersonUpdated carries birth_date.
	// A cleared partner is written as nil, which the projection, the merge's
	// reference check and the rollback state all read as "no partner".
	changes := make(map[string]any)
	if input.Partner1ID != nil {
		changes["partner1_id"] = input.Partner1ID.String()
	}
	if input.Partner2ID != nil {
		changes["partner2_id"] = input.Partner2ID.String()
	}
	if input.ClearPartner1 && family.Partner1ID != nil {
		changes["partner1_id"] = nil
	}
	if input.ClearPartner2 && family.Partner2ID != nil {
		changes["partner2_id"] = nil
	}
	if input.RelationshipType != nil {
		changes["relationship_type"] = *input.RelationshipType
	}
	if input.MarriageDate != nil {
		if *input.MarriageDate == "" {
			changes["marriage_date"] = nil
		} else {
			changes["marriage_date"] = *input.MarriageDate
		}
	}
	if input.MarriagePlace != nil {
		changes["marriage_place"] = *input.MarriagePlace
	}

	if len(changes) == 0 {
		return &UpdateFamilyResult{Version: family.Version}, nil
	}

	// Create event
	event := domain.NewFamilyUpdated(input.ID, changes)

	// Execute command (append + project) with optimistic locking.
	version, err := h.execute(ctx, input.ID.String(), familyStreamType, []domain.Event{event}, input.Version)
	if err != nil {
		if errors.Is(err, repository.ErrConcurrencyConflict) {
			return nil, repository.ErrConcurrencyConflict
		}
		return nil, fmt.Errorf("appending family updated event: %w", err)
	}

	return &UpdateFamilyResult{
		Version: version,
	}, nil
}

// validateFamilyUpdate checks an update against the family it changes: a
// partner cannot be both set and cleared, a new partner must exist on the handler's scope, the two partners must differ once
// the update is applied, a new partner must not be one of the family's
// children or their descendant (the same circular-ancestry rule LinkChild
// enforces from the other side), and the relationship type must be known.
func (h *Handler) validateFamilyUpdate(ctx context.Context, family *repository.FamilyReadModel, input UpdateFamilyInput) error {
	if input.RelationshipType != nil && !domain.RelationType(*input.RelationshipType).IsValid() {
		return fmt.Errorf("%w: invalid relationship_type %q", ErrInvalidFamilyInput, *input.RelationshipType)
	}

	if input.ClearPartner1 && input.Partner1ID != nil {
		return fmt.Errorf("%w: partner1_id cannot be set and cleared in one update", ErrInvalidFamilyInput)
	}
	if input.ClearPartner2 && input.Partner2ID != nil {
		return fmt.Errorf("%w: partner2_id cannot be set and cleared in one update", ErrInvalidFamilyInput)
	}

	partner1, partner2 := family.Partner1ID, family.Partner2ID
	if input.Partner1ID != nil {
		partner1 = input.Partner1ID
	}
	if input.Partner2ID != nil {
		partner2 = input.Partner2ID
	}
	if input.ClearPartner1 {
		partner1 = nil
	}
	if input.ClearPartner2 {
		partner2 = nil
	}
	if partner1 != nil && partner2 != nil && *partner1 == *partner2 {
		return fmt.Errorf("%w: partner1 and partner2 must be different people", ErrInvalidFamilyInput)
	}

	for _, change := range []struct {
		field string
		id    *uuid.UUID
	}{{"partner1_id", input.Partner1ID}, {"partner2_id", input.Partner2ID}} {
		if change.id == nil {
			continue
		}
		person, err := h.readStore.GetPerson(ctx, h.branchID, *change.id)
		if err != nil {
			return fmt.Errorf("getting %s: %w", change.field, err)
		}
		if person == nil {
			return fmt.Errorf("%w: %s not found", ErrInvalidFamilyInput, change.field)
		}
		if err := h.checkPartnerNotDescendant(ctx, family.ID, *change.id); err != nil {
			return err
		}
	}
	return nil
}

// checkPartnerNotDescendant refuses a partner who is one of the family's
// children or descends from one: they would become their own ancestor.
func (h *Handler) checkPartnerNotDescendant(ctx context.Context, familyID, partnerID uuid.UUID) error {
	children, err := h.readStore.GetChildrenOfFamily(ctx, h.branchID, familyID)
	if err != nil {
		return fmt.Errorf("getting children of family: %w", err)
	}
	for _, child := range children {
		isAncestor, err := h.isAncestor(ctx, child.ID, partnerID)
		if err != nil {
			return fmt.Errorf("checking circular ancestry: %w", err)
		}
		if isAncestor {
			return ErrCircularAncestry
		}
	}
	return nil
}

// DeleteFamilyInput contains the data for deleting a family.
type DeleteFamilyInput struct {
	ID      uuid.UUID
	Version int64
}

// DeleteFamily deletes a family if it has no children.
func (h *Handler) DeleteFamily(ctx context.Context, input DeleteFamilyInput) error {
	// Check family exists
	family, err := h.readStore.GetFamily(ctx, h.branchID, input.ID)
	if err != nil {
		return fmt.Errorf("getting family: %w", err)
	}
	if family == nil {
		return ErrFamilyNotFound
	}

	// Check for children
	children, err := h.readStore.GetChildrenOfFamily(ctx, h.branchID, input.ID)
	if err != nil {
		return fmt.Errorf("getting children of family: %w", err)
	}
	if len(children) > 0 {
		return ErrFamilyHasChildren
	}

	// Create event
	event := domain.NewFamilyDeleted(input.ID, "")

	// Execute command (append + project) with optimistic locking.
	if _, err := h.execute(ctx, input.ID.String(), familyStreamType, []domain.Event{event}, input.Version); err != nil {
		if errors.Is(err, repository.ErrConcurrencyConflict) {
			return repository.ErrConcurrencyConflict
		}
		return fmt.Errorf("appending family deleted event: %w", err)
	}

	return nil
}

// LinkChildInput contains the data for linking a child to a family.
type LinkChildInput struct {
	FamilyID     uuid.UUID
	ChildID      uuid.UUID
	RelationType string // "biological", "adopted", "foster", "step"
}

// LinkChildResult contains the result of linking a child.
type LinkChildResult struct {
	FamilyVersion int64
}

// LinkChild adds a child to a family with circular ancestry detection.
func (h *Handler) LinkChild(ctx context.Context, input LinkChildInput) (*LinkChildResult, error) {
	// Verify family exists
	family, err := h.readStore.GetFamily(ctx, h.branchID, input.FamilyID)
	if err != nil {
		return nil, fmt.Errorf("getting family: %w", err)
	}
	if family == nil {
		return nil, ErrFamilyNotFound
	}

	// Verify child exists
	child, err := h.readStore.GetPerson(ctx, h.branchID, input.ChildID)
	if err != nil {
		return nil, fmt.Errorf("getting child: %w", err)
	}
	if child == nil {
		return nil, ErrPersonNotFound
	}

	// Check if child is already linked to a family
	existingFamily, err := h.readStore.GetChildFamily(ctx, h.branchID, input.ChildID)
	if err != nil {
		return nil, fmt.Errorf("getting child family: %w", err)
	}
	if existingFamily != nil {
		return nil, ErrChildAlreadyLinked
	}

	// Circular ancestry check: child cannot be an ancestor of either partner
	if family.Partner1ID != nil {
		if isAncestor, err := h.isAncestor(ctx, input.ChildID, *family.Partner1ID); err != nil {
			return nil, fmt.Errorf("checking circular ancestry: %w", err)
		} else if isAncestor {
			return nil, ErrCircularAncestry
		}
	}
	if family.Partner2ID != nil {
		if isAncestor, err := h.isAncestor(ctx, input.ChildID, *family.Partner2ID); err != nil {
			return nil, fmt.Errorf("checking circular ancestry: %w", err)
		} else if isAncestor {
			return nil, ErrCircularAncestry
		}
	}

	// Parse relation type
	relType := domain.ChildBiological
	if input.RelationType != "" {
		relType = domain.ChildRelationType(input.RelationType)
	}

	// Create event
	fc := domain.NewFamilyChild(input.FamilyID, input.ChildID, relType)
	event := domain.NewChildLinkedToFamily(fc)

	// Execute command (append + project) with optimistic locking.
	version, err := h.execute(ctx, input.FamilyID.String(), familyStreamType, []domain.Event{event}, family.Version)
	if err != nil {
		if errors.Is(err, repository.ErrConcurrencyConflict) {
			return nil, repository.ErrConcurrencyConflict
		}
		return nil, fmt.Errorf("appending child linked event: %w", err)
	}

	return &LinkChildResult{
		FamilyVersion: version,
	}, nil
}

// UnlinkChildInput contains the data for unlinking a child from a family.
type UnlinkChildInput struct {
	FamilyID uuid.UUID
	ChildID  uuid.UUID
}

// UnlinkChild removes a child from a family.
func (h *Handler) UnlinkChild(ctx context.Context, input UnlinkChildInput) error {
	// Verify family exists
	family, err := h.readStore.GetFamily(ctx, h.branchID, input.FamilyID)
	if err != nil {
		return fmt.Errorf("getting family: %w", err)
	}
	if family == nil {
		return ErrFamilyNotFound
	}

	// Verify child is in this family
	childFamily, err := h.readStore.GetChildFamily(ctx, h.branchID, input.ChildID)
	if err != nil {
		return fmt.Errorf("getting child family: %w", err)
	}
	if childFamily == nil || childFamily.ID != input.FamilyID {
		return ErrChildNotInFamily
	}

	// Create event
	event := domain.NewChildUnlinkedFromFamily(input.FamilyID, input.ChildID)

	// Execute command (append + project) with optimistic locking.
	if _, err := h.execute(ctx, input.FamilyID.String(), familyStreamType, []domain.Event{event}, family.Version); err != nil {
		if errors.Is(err, repository.ErrConcurrencyConflict) {
			return repository.ErrConcurrencyConflict
		}
		return fmt.Errorf("appending child unlinked event: %w", err)
	}

	return nil
}

// isAncestor checks if potentialAncestor is an ancestor of personID.
// This is used for circular ancestry detection when linking children.
func (h *Handler) isAncestor(ctx context.Context, potentialAncestor, personID uuid.UUID) (bool, error) {
	if potentialAncestor == personID {
		return true, nil
	}

	// Get the person's parent family
	parentFamily, err := h.readStore.GetChildFamily(ctx, h.branchID, personID)
	if err != nil {
		return false, err
	}
	if parentFamily == nil {
		return false, nil // No parents, can't be an ancestor
	}

	// Check each parent recursively
	if parentFamily.Partner1ID != nil {
		if isAnc, err := h.isAncestor(ctx, potentialAncestor, *parentFamily.Partner1ID); err != nil {
			return false, err
		} else if isAnc {
			return true, nil
		}
	}
	if parentFamily.Partner2ID != nil {
		if isAnc, err := h.isAncestor(ctx, potentialAncestor, *parentFamily.Partner2ID); err != nil {
			return false, err
		} else if isAnc {
			return true, nil
		}
	}

	return false, nil
}
