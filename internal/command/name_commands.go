package command

import (
	"context"
	"errors"
	"fmt"

	"github.com/google/uuid"

	"github.com/cacack/my-family/internal/domain"
	"github.com/cacack/my-family/internal/repository"
)

// Name command errors.
var (
	ErrNameNotFound        = errors.New("name not found")
	ErrCannotDeleteLast    = errors.New("cannot delete the last remaining name")
	ErrCannotDeletePrimary = errors.New("cannot delete the primary name; transfer primary status first")
)

// AddNameInput contains the data for adding a name to a person.
type AddNameInput struct {
	PersonID      uuid.UUID
	GivenName     string
	Surname       string
	NamePrefix    string
	NameSuffix    string
	SurnamePrefix string
	Nickname      string
	NameType      string
	IsPrimary     bool
}

// AddNameResult contains the result of adding a name.
type AddNameResult struct {
	ID        uuid.UUID
	PersonID  uuid.UUID
	IsPrimary bool
}

// AddName adds a new name variant to a person.
func (h *Handler) AddName(ctx context.Context, input AddNameInput) (*AddNameResult, error) {
	// Validate required fields
	if input.GivenName == "" {
		return nil, fmt.Errorf("%w: given_name is required", ErrInvalidInput)
	}

	// Verify person exists
	person, err := h.readStore.GetPerson(ctx, input.PersonID)
	if err != nil {
		return nil, err
	}
	if person == nil {
		return nil, ErrPersonNotFound
	}

	// Get existing names to check primary status
	existingNames, err := h.readStore.GetPersonNames(ctx, input.PersonID)
	if err != nil {
		return nil, err
	}

	// Create the person name entity
	pn := domain.NewPersonName(input.PersonID, input.GivenName, input.Surname)
	pn.NamePrefix = input.NamePrefix
	pn.NameSuffix = input.NameSuffix
	pn.SurnamePrefix = input.SurnamePrefix
	pn.Nickname = input.Nickname
	if input.NameType != "" {
		pn.NameType = domain.NameType(input.NameType)
	}
	pn.IsPrimary = input.IsPrimary

	// Validate the name
	if err := pn.Validate(); err != nil {
		return nil, fmt.Errorf("%w: %v", ErrInvalidInput, err)
	}

	// If this is the first name, make it primary
	if len(existingNames) == 0 {
		pn.IsPrimary = true
	}

	// If setting as primary, we need to demote the existing primary
	var events []domain.Event
	if pn.IsPrimary {
		for _, existing := range existingNames {
			if existing.IsPrimary {
				// Create updated name to demote
				demoted := &domain.PersonName{
					ID:            existing.ID,
					PersonID:      existing.PersonID,
					GivenName:     existing.GivenName,
					Surname:       existing.Surname,
					NamePrefix:    existing.NamePrefix,
					NameSuffix:    existing.NameSuffix,
					SurnamePrefix: existing.SurnamePrefix,
					Nickname:      existing.Nickname,
					NameType:      existing.NameType,
					IsPrimary:     false,
				}
				events = append(events, domain.NewNameUpdated(demoted))
				break
			}
		}
	}

	// Add the new name event
	events = append(events, domain.NewNameAdded(pn))

	// Execute command
	_, err = h.execute(ctx, input.PersonID.String(), "Person", events, person.Version)
	if err != nil {
		return nil, err
	}

	return &AddNameResult{
		ID:        pn.ID,
		PersonID:  input.PersonID,
		IsPrimary: pn.IsPrimary,
	}, nil
}

// UpdateNameInput contains the data for updating a name.
type UpdateNameInput struct {
	PersonID      uuid.UUID
	NameID        uuid.UUID
	GivenName     *string
	Surname       *string
	NamePrefix    *string
	NameSuffix    *string
	SurnamePrefix *string
	Nickname      *string
	NameType      *string
	IsPrimary     *bool
}

// UpdateNameResult contains the result of updating a name.
type UpdateNameResult struct {
	ID        uuid.UUID
	PersonID  uuid.UUID
	IsPrimary bool
}

// applyNameUpdates applies the non-nil fields from input to the PersonName.
func applyNameUpdates(pn *domain.PersonName, input UpdateNameInput) {
	if input.GivenName != nil {
		pn.GivenName = *input.GivenName
	}
	if input.Surname != nil {
		pn.Surname = *input.Surname
	}
	if input.NamePrefix != nil {
		pn.NamePrefix = *input.NamePrefix
	}
	if input.NameSuffix != nil {
		pn.NameSuffix = *input.NameSuffix
	}
	if input.SurnamePrefix != nil {
		pn.SurnamePrefix = *input.SurnamePrefix
	}
	if input.Nickname != nil {
		pn.Nickname = *input.Nickname
	}
	if input.NameType != nil {
		pn.NameType = domain.NameType(*input.NameType)
	}
	if input.IsPrimary != nil {
		pn.IsPrimary = *input.IsPrimary
	}
}

// UpdateName updates an existing name variant.
func (h *Handler) UpdateName(ctx context.Context, input UpdateNameInput) (*UpdateNameResult, error) {
	person, existingName, err := h.validateNameUpdate(ctx, input)
	if err != nil {
		return nil, err
	}

	// Build updated name from existing
	pn := personNameFromReadModel(existingName)
	applyNameUpdates(pn, input)

	if err := pn.Validate(); err != nil {
		return nil, fmt.Errorf("%w: %v", ErrInvalidInput, err)
	}

	events, err := h.buildNameUpdateEvents(ctx, pn, existingName, input)
	if err != nil {
		return nil, err
	}

	if _, err = h.execute(ctx, input.PersonID.String(), "Person", events, person.Version); err != nil {
		return nil, err
	}

	return &UpdateNameResult{
		ID:        pn.ID,
		PersonID:  input.PersonID,
		IsPrimary: pn.IsPrimary,
	}, nil
}

// validateNameUpdate verifies person exists and name belongs to them.
func (h *Handler) validateNameUpdate(ctx context.Context, input UpdateNameInput) (*repository.PersonReadModel, *repository.PersonNameReadModel, error) {
	person, err := h.readStore.GetPerson(ctx, input.PersonID)
	if err != nil {
		return nil, nil, err
	}
	if person == nil {
		return nil, nil, ErrPersonNotFound
	}

	existingName, err := h.readStore.GetPersonName(ctx, input.NameID)
	if err != nil {
		return nil, nil, err
	}
	if existingName == nil || existingName.PersonID != input.PersonID {
		return nil, nil, ErrNameNotFound
	}

	return person, existingName, nil
}

// personNameFromReadModel creates a PersonName from a read model.
func personNameFromReadModel(rm *repository.PersonNameReadModel) *domain.PersonName {
	return &domain.PersonName{
		ID:            rm.ID,
		PersonID:      rm.PersonID,
		GivenName:     rm.GivenName,
		Surname:       rm.Surname,
		NamePrefix:    rm.NamePrefix,
		NameSuffix:    rm.NameSuffix,
		SurnamePrefix: rm.SurnamePrefix,
		Nickname:      rm.Nickname,
		NameType:      rm.NameType,
		IsPrimary:     rm.IsPrimary,
	}
}

// buildNameUpdateEvents creates the events needed for a name update, including demoting existing primary if needed.
func (h *Handler) buildNameUpdateEvents(ctx context.Context, pn *domain.PersonName, existingName *repository.PersonNameReadModel, input UpdateNameInput) ([]domain.Event, error) {
	var events []domain.Event

	// If setting as primary and it wasn't before, demote existing primary
	if pn.IsPrimary && !existingName.IsPrimary {
		existingNames, err := h.readStore.GetPersonNames(ctx, input.PersonID)
		if err != nil {
			return nil, err
		}
		for _, existing := range existingNames {
			if existing.IsPrimary && existing.ID != input.NameID {
				demoted := personNameFromReadModel(&existing)
				demoted.IsPrimary = false
				events = append(events, domain.NewNameUpdated(demoted))
				break
			}
		}
	}

	events = append(events, domain.NewNameUpdated(pn))
	return events, nil
}

// DeleteNameInput contains the data for deleting a name.
type DeleteNameInput struct {
	PersonID uuid.UUID
	NameID   uuid.UUID
}

// DeleteName removes a name from a person.
func (h *Handler) DeleteName(ctx context.Context, input DeleteNameInput) error {
	// Verify person exists
	person, err := h.readStore.GetPerson(ctx, input.PersonID)
	if err != nil {
		return err
	}
	if person == nil {
		return ErrPersonNotFound
	}

	// Get all names for the person
	existingNames, err := h.readStore.GetPersonNames(ctx, input.PersonID)
	if err != nil {
		return err
	}

	// Find the name to delete
	var nameToDelete *domain.PersonName
	for _, n := range existingNames {
		if n.ID == input.NameID {
			nameToDelete = &domain.PersonName{
				ID:        n.ID,
				PersonID:  n.PersonID,
				IsPrimary: n.IsPrimary,
			}
			break
		}
	}

	if nameToDelete == nil {
		return ErrNameNotFound
	}

	// Cannot delete the last remaining name
	if len(existingNames) <= 1 {
		return ErrCannotDeleteLast
	}

	// Cannot delete primary name
	if nameToDelete.IsPrimary {
		return ErrCannotDeletePrimary
	}

	// Create delete event
	event := domain.NewNameRemoved(input.PersonID, input.NameID)

	// Execute command
	_, err = h.execute(ctx, input.PersonID.String(), "Person", []domain.Event{event}, person.Version)
	return err
}
