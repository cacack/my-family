package command

import (
	"context"
	"errors"

	"github.com/google/uuid"

	"github.com/cacack/my-family/internal/domain"
	"github.com/cacack/my-family/internal/repository"
)

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
		p, err := h.readStore.GetPerson(ctx, *input.Partner1ID)
		if err != nil {
			return nil, err
		}
		if p == nil {
			return nil, errors.New("invalid family input: partner1 not found")
		}
	}
	if input.Partner2ID != nil {
		p, err := h.readStore.GetPerson(ctx, *input.Partner2ID)
		if err != nil {
			return nil, err
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

	// Append to event store
	err := h.eventStore.Append(ctx, family.ID, "family", []domain.Event{event}, 0)
	if err != nil {
		return nil, err
	}

	// Update read model
	if err := h.projector.Apply(ctx, event); err != nil {
		return nil, err
	}

	return &CreateFamilyResult{
		ID:      family.ID,
		Version: 1,
	}, nil
}

// UpdateFamilyInput contains the data for updating a family.
type UpdateFamilyInput struct {
	ID               uuid.UUID
	Partner1ID       *uuid.UUID
	Partner2ID       *uuid.UUID
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
	family, err := h.readStore.GetFamily(ctx, input.ID)
	if err != nil {
		return nil, err
	}
	if family == nil {
		return nil, ErrFamilyNotFound
	}

	// Build changes map
	changes := make(map[string]any)
	if input.Partner1ID != nil {
		changes["partner1_id"] = input.Partner1ID
	}
	if input.Partner2ID != nil {
		changes["partner2_id"] = input.Partner2ID
	}
	if input.RelationshipType != nil {
		changes["relationship_type"] = domain.RelationType(*input.RelationshipType)
	}
	if input.MarriageDate != nil {
		if *input.MarriageDate == "" {
			changes["marriage_date"] = nil
		} else {
			md := domain.ParseGenDate(*input.MarriageDate)
			changes["marriage_date"] = &md
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

	// Append to event store with optimistic locking
	err = h.eventStore.Append(ctx, input.ID, "family", []domain.Event{event}, input.Version)
	if err != nil {
		if errors.Is(err, repository.ErrConcurrencyConflict) {
			return nil, repository.ErrConcurrencyConflict
		}
		return nil, err
	}

	// Update read model
	if err := h.projector.Apply(ctx, event); err != nil {
		return nil, err
	}

	return &UpdateFamilyResult{
		Version: input.Version + 1,
	}, nil
}

// DeleteFamilyInput contains the data for deleting a family.
type DeleteFamilyInput struct {
	ID      uuid.UUID
	Version int64
}

// DeleteFamily deletes a family if it has no children.
func (h *Handler) DeleteFamily(ctx context.Context, input DeleteFamilyInput) error {
	// Check family exists
	family, err := h.readStore.GetFamily(ctx, input.ID)
	if err != nil {
		return err
	}
	if family == nil {
		return ErrFamilyNotFound
	}

	// Check for children
	children, err := h.readStore.GetChildrenOfFamily(ctx, input.ID)
	if err != nil {
		return err
	}
	if len(children) > 0 {
		return ErrFamilyHasChildren
	}

	// Create event
	event := domain.NewFamilyDeleted(input.ID, "")

	// Append to event store with optimistic locking
	err = h.eventStore.Append(ctx, input.ID, "family", []domain.Event{event}, input.Version)
	if err != nil {
		if errors.Is(err, repository.ErrConcurrencyConflict) {
			return repository.ErrConcurrencyConflict
		}
		return err
	}

	// Update read model
	return h.projector.Apply(ctx, event)
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
	family, err := h.readStore.GetFamily(ctx, input.FamilyID)
	if err != nil {
		return nil, err
	}
	if family == nil {
		return nil, ErrFamilyNotFound
	}

	// Verify child exists
	child, err := h.readStore.GetPerson(ctx, input.ChildID)
	if err != nil {
		return nil, err
	}
	if child == nil {
		return nil, ErrPersonNotFound
	}

	// Check if child is already linked to a family
	existingFamily, err := h.readStore.GetChildFamily(ctx, input.ChildID)
	if err != nil {
		return nil, err
	}
	if existingFamily != nil {
		return nil, ErrChildAlreadyLinked
	}

	// Circular ancestry check: child cannot be an ancestor of either partner
	if family.Partner1ID != nil {
		if isAncestor, err := h.isAncestor(ctx, input.ChildID, *family.Partner1ID); err != nil {
			return nil, err
		} else if isAncestor {
			return nil, ErrCircularAncestry
		}
	}
	if family.Partner2ID != nil {
		if isAncestor, err := h.isAncestor(ctx, input.ChildID, *family.Partner2ID); err != nil {
			return nil, err
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

	// Append to event store
	err = h.eventStore.Append(ctx, input.FamilyID, "family", []domain.Event{event}, family.Version)
	if err != nil {
		if errors.Is(err, repository.ErrConcurrencyConflict) {
			return nil, repository.ErrConcurrencyConflict
		}
		return nil, err
	}

	// Update read model
	if err := h.projector.Apply(ctx, event); err != nil {
		return nil, err
	}

	return &LinkChildResult{
		FamilyVersion: family.Version + 1,
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
	family, err := h.readStore.GetFamily(ctx, input.FamilyID)
	if err != nil {
		return err
	}
	if family == nil {
		return ErrFamilyNotFound
	}

	// Verify child is in this family
	childFamily, err := h.readStore.GetChildFamily(ctx, input.ChildID)
	if err != nil {
		return err
	}
	if childFamily == nil || childFamily.ID != input.FamilyID {
		return ErrChildNotInFamily
	}

	// Create event
	event := domain.NewChildUnlinkedFromFamily(input.FamilyID, input.ChildID)

	// Append to event store
	err = h.eventStore.Append(ctx, input.FamilyID, "family", []domain.Event{event}, family.Version)
	if err != nil {
		if errors.Is(err, repository.ErrConcurrencyConflict) {
			return repository.ErrConcurrencyConflict
		}
		return err
	}

	// Update read model
	return h.projector.Apply(ctx, event)
}

// isAncestor checks if potentialAncestor is an ancestor of personID.
// This is used for circular ancestry detection when linking children.
func (h *Handler) isAncestor(ctx context.Context, potentialAncestor, personID uuid.UUID) (bool, error) {
	if potentialAncestor == personID {
		return true, nil
	}

	// Get the person's parent family
	parentFamily, err := h.readStore.GetChildFamily(ctx, personID)
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
