package command

import (
	"context"
	"errors"
	"fmt"

	"github.com/google/uuid"

	"github.com/cacack/my-family/internal/domain"
	"github.com/cacack/my-family/internal/repository"
)

// Common command errors.
var (
	ErrPersonNotFound      = errors.New("person not found")
	ErrVersionMismatch     = errors.New("version mismatch - person was modified")
	ErrPersonHasFamilies   = errors.New("person is linked to families and cannot be deleted")
	ErrInvalidInput        = errors.New("invalid input")
)

// CreatePersonInput contains the data for creating a new person.
type CreatePersonInput struct {
	GivenName  string
	Surname    string
	Gender     string
	BirthDate  string
	BirthPlace string
	DeathDate  string
	DeathPlace string
	Notes      string
}

// CreatePersonResult contains the result of creating a person.
type CreatePersonResult struct {
	ID      uuid.UUID
	Version int64
}

// CreatePerson creates a new person record.
func (h *Handler) CreatePerson(ctx context.Context, input CreatePersonInput) (*CreatePersonResult, error) {
	// Validate required fields
	if input.GivenName == "" || input.Surname == "" {
		return nil, fmt.Errorf("%w: given_name and surname are required", ErrInvalidInput)
	}

	// Create person entity
	person := domain.NewPerson(input.GivenName, input.Surname)

	if input.Gender != "" {
		person.Gender = domain.Gender(input.Gender)
	}
	if input.BirthDate != "" {
		person.SetBirthDate(input.BirthDate)
	}
	if input.BirthPlace != "" {
		person.BirthPlace = input.BirthPlace
	}
	if input.DeathDate != "" {
		person.SetDeathDate(input.DeathDate)
	}
	if input.DeathPlace != "" {
		person.DeathPlace = input.DeathPlace
	}
	if input.Notes != "" {
		person.Notes = input.Notes
	}

	// Validate person
	if err := person.Validate(); err != nil {
		return nil, fmt.Errorf("%w: %v", ErrInvalidInput, err)
	}

	// Create event
	event := domain.NewPersonCreated(person)

	// Execute command (append + project)
	version, err := h.execute(ctx, person.ID.String(), "Person", []domain.Event{event}, -1)
	if err != nil {
		return nil, err
	}

	return &CreatePersonResult{
		ID:      person.ID,
		Version: version,
	}, nil
}

// UpdatePersonInput contains the data for updating a person.
type UpdatePersonInput struct {
	ID         uuid.UUID
	GivenName  *string
	Surname    *string
	Gender     *string
	BirthDate  *string
	BirthPlace *string
	DeathDate  *string
	DeathPlace *string
	Notes      *string
	Version    int64 // Required for optimistic locking
}

// UpdatePersonResult contains the result of updating a person.
type UpdatePersonResult struct {
	Version int64
}

// UpdatePerson updates an existing person record.
func (h *Handler) UpdatePerson(ctx context.Context, input UpdatePersonInput) (*UpdatePersonResult, error) {
	// Get current person from read model
	current, err := h.readStore.GetPerson(ctx, input.ID)
	if err != nil {
		return nil, err
	}
	if current == nil {
		return nil, ErrPersonNotFound
	}

	// Check version for optimistic locking
	if current.Version != input.Version {
		return nil, repository.ErrConcurrencyConflict
	}

	// Build changes map
	changes := make(map[string]any)

	// Apply and validate changes
	testPerson := &domain.Person{
		ID:         current.ID,
		GivenName:  current.GivenName,
		Surname:    current.Surname,
		Gender:     current.Gender,
		BirthPlace: current.BirthPlace,
		DeathPlace: current.DeathPlace,
		Notes:      current.Notes,
	}

	if current.BirthDateRaw != "" {
		bd := domain.ParseGenDate(current.BirthDateRaw)
		testPerson.BirthDate = &bd
	}
	if current.DeathDateRaw != "" {
		dd := domain.ParseGenDate(current.DeathDateRaw)
		testPerson.DeathDate = &dd
	}

	if input.GivenName != nil {
		testPerson.GivenName = *input.GivenName
		changes["given_name"] = *input.GivenName
	}
	if input.Surname != nil {
		testPerson.Surname = *input.Surname
		changes["surname"] = *input.Surname
	}
	if input.Gender != nil {
		testPerson.Gender = domain.Gender(*input.Gender)
		changes["gender"] = *input.Gender
	}
	if input.BirthDate != nil {
		testPerson.SetBirthDate(*input.BirthDate)
		changes["birth_date"] = *input.BirthDate
	}
	if input.BirthPlace != nil {
		testPerson.BirthPlace = *input.BirthPlace
		changes["birth_place"] = *input.BirthPlace
	}
	if input.DeathDate != nil {
		testPerson.SetDeathDate(*input.DeathDate)
		changes["death_date"] = *input.DeathDate
	}
	if input.DeathPlace != nil {
		testPerson.DeathPlace = *input.DeathPlace
		changes["death_place"] = *input.DeathPlace
	}
	if input.Notes != nil {
		testPerson.Notes = *input.Notes
		changes["notes"] = *input.Notes
	}

	// No changes?
	if len(changes) == 0 {
		return &UpdatePersonResult{Version: current.Version}, nil
	}

	// Validate updated person
	if err := testPerson.Validate(); err != nil {
		return nil, fmt.Errorf("%w: %v", ErrInvalidInput, err)
	}

	// Create event
	event := domain.NewPersonUpdated(input.ID, changes)

	// Execute command
	version, err := h.execute(ctx, input.ID.String(), "Person", []domain.Event{event}, input.Version)
	if err != nil {
		return nil, err
	}

	return &UpdatePersonResult{Version: version}, nil
}

// DeletePersonInput contains the data for deleting a person.
type DeletePersonInput struct {
	ID      uuid.UUID
	Version int64
	Reason  string
}

// DeletePerson deletes a person record.
func (h *Handler) DeletePerson(ctx context.Context, input DeletePersonInput) error {
	// Get current person from read model
	current, err := h.readStore.GetPerson(ctx, input.ID)
	if err != nil {
		return err
	}
	if current == nil {
		return ErrPersonNotFound
	}

	// Check version for optimistic locking
	if current.Version != input.Version {
		return repository.ErrConcurrencyConflict
	}

	// Check if person is linked to any families as partner
	families, err := h.readStore.GetFamiliesForPerson(ctx, input.ID)
	if err != nil {
		return err
	}
	if len(families) > 0 {
		return ErrPersonHasFamilies
	}

	// Check if person is a child in any family
	childFamily, err := h.readStore.GetChildFamily(ctx, input.ID)
	if err != nil {
		return err
	}
	if childFamily != nil {
		return ErrPersonHasFamilies
	}

	// Create event
	event := domain.NewPersonDeleted(input.ID, input.Reason)

	// Execute command
	_, err = h.execute(ctx, input.ID.String(), "Person", []domain.Event{event}, input.Version)
	return err
}

// parseUUID parses a string to UUID.
func parseUUID(s string) (uuid.UUID, error) {
	return uuid.Parse(s)
}
