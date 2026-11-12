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
	ErrPersonNotFound    = errors.New("person not found")
	ErrVersionMismatch   = errors.New("version mismatch - person was modified")
	ErrPersonHasFamilies = errors.New("person is linked to families and cannot be deleted")
	ErrInvalidInput      = errors.New("invalid input")
)

// InvalidDateError is a newly entered date that was rejected, naming the
// field. It matches ErrInvalidInput and the underlying *domain.DateInputError
// with errors.Is and errors.As.
type InvalidDateError struct {
	Field string
	Err   error
}

func (e *InvalidDateError) Error() string {
	return fmt.Sprintf("%v: %s: %v", ErrInvalidInput, e.Field, e.Err)
}

func (e *InvalidDateError) Unwrap() []error { return []error{ErrInvalidInput, e.Err} }

// normalizeDateInput returns a newly entered date in the GEDCOM form it is
// stored in (see domain.NormalizeDateInput), or an *InvalidDateError. Only new
// input is checked, so a stored or imported date the parser can't read never
// blocks an unrelated edit.
func normalizeDateInput(field, value string) (string, error) {
	normalized, err := domain.NormalizeDateInput(value)
	if err != nil {
		return "", &InvalidDateError{Field: field, Err: err}
	}
	return normalized, nil
}

// recordChange is the per-field step of an Update* command for a text field:
// when input is not omitted and its new value (empty for a clear) differs
// from *field (which starts as the current value), it sets *field and records
// the new value under key in changes. A field equal to its current value is
// not a change (the edit form sends every field). A required field passes
// SetPtr of its pointer.
func recordChange[T ~string](changes map[string]any, key string, input Patch[string], field *T) {
	if v, ok := input.Change(); ok && T(v) != *field {
		*field = T(v)
		changes[key] = v
	}
}

// changedDate normalizes a date field's new input (see normalizeDateInput) and
// reports whether it differs from the current raw date. An input that is
// omitted or equal to the current text is not a change; a clear is the empty
// date.
func changedDate(field string, input Patch[string], current string) (string, bool, error) {
	v, ok := input.Change()
	if !ok || v == current {
		return "", false, nil
	}
	normalized, err := normalizeDateInput(field, v)
	if err != nil {
		return "", false, err
	}
	return normalized, normalized != current, nil
}

// CreatePersonInput contains the data for creating a new person.
type CreatePersonInput struct {
	GivenName      string
	Surname        string
	Gender         string
	BirthDate      string
	BirthPlace     string
	DeathDate      string
	DeathPlace     string
	Notes          string
	ResearchStatus string
}

// CreatePersonResult contains the result of creating a person.
type CreatePersonResult struct {
	ID      uuid.UUID
	Version int64
}

// CreatePerson creates a new person record.
func (h *Handler) CreatePerson(ctx context.Context, input CreatePersonInput) (*CreatePersonResult, error) {
	// Validate required fields
	if input.GivenName == "" {
		return nil, fmt.Errorf("%w: given_name is required", ErrInvalidInput)
	}
	birthDate, err := normalizeDateInput("birth_date", input.BirthDate)
	if err != nil {
		return nil, err
	}
	deathDate, err := normalizeDateInput("death_date", input.DeathDate)
	if err != nil {
		return nil, err
	}

	// Create person entity
	person := domain.NewPerson(input.GivenName, input.Surname)

	if input.Gender != "" {
		person.Gender = domain.Gender(input.Gender)
	}
	if birthDate != "" {
		person.SetBirthDate(birthDate)
	}
	if input.BirthPlace != "" {
		person.BirthPlace = input.BirthPlace
	}
	if deathDate != "" {
		person.SetDeathDate(deathDate)
	}
	if input.DeathPlace != "" {
		person.DeathPlace = input.DeathPlace
	}
	if input.Notes != "" {
		person.Notes = input.Notes
	}
	if input.ResearchStatus != "" {
		person.ResearchStatus = domain.ParseResearchStatus(input.ResearchStatus)
	}

	// Validate person
	if err := person.Validate(); err != nil {
		return nil, fmt.Errorf("%w: %v", ErrInvalidInput, err)
	}

	// A person's name is its primary name (docs/CONVENTIONS.md "Person Name and
	// Primary Name"), recorded with the person so its Names list is never
	// empty (#1131).
	primary := domain.NewPersonName(person.ID, person.GivenName, person.Surname)
	primary.IsPrimary = true
	events := []domain.Event{domain.NewPersonCreated(person), domain.NewNameAdded(primary)}

	// Execute command (append + project)
	version, err := h.execute(ctx, person.ID.String(), "Person", events, -1)
	if err != nil {
		return nil, fmt.Errorf("executing create person command: %w", err)
	}

	return &CreatePersonResult{
		ID:      person.ID,
		Version: version,
	}, nil
}

// UpdatePersonInput contains the data for updating a person.
type UpdatePersonInput struct {
	ID             uuid.UUID
	GivenName      *string // required: cannot be cleared
	Surname        Patch[string]
	Gender         Patch[string]
	BirthDate      Patch[string]
	BirthPlace     Patch[string]
	DeathDate      Patch[string]
	DeathPlace     Patch[string]
	Notes          Patch[string]
	ResearchStatus Patch[string]
	Version        int64 // Required for optimistic locking
}

// UpdatePersonResult contains the result of updating a person.
type UpdatePersonResult struct {
	Version int64
}

// UpdatePerson updates an existing person record.
func (h *Handler) UpdatePerson(ctx context.Context, input UpdatePersonInput) (*UpdatePersonResult, error) {
	// Get current person from read model
	current, err := h.readStore.GetPerson(ctx, h.branchID, input.ID)
	if err != nil {
		return nil, fmt.Errorf("getting person: %w", err)
	}
	if current == nil {
		return nil, ErrPersonNotFound
	}

	// Check version for optimistic locking
	if current.Version != input.Version {
		return nil, repository.ErrConcurrencyConflict
	}

	// Build changes map. A field equal to its current value is not a change
	// (the edit form sends every field), so only differing fields are recorded.
	changes := make(map[string]any)

	// Apply and validate changes
	testPerson := &domain.Person{
		ID:             current.ID,
		GivenName:      current.GivenName,
		Surname:        current.Surname,
		Gender:         current.Gender,
		BirthPlace:     current.BirthPlace,
		DeathPlace:     current.DeathPlace,
		Notes:          current.Notes,
		ResearchStatus: current.ResearchStatus,
	}

	if current.BirthDateRaw != "" {
		bd := domain.ParseGenDate(current.BirthDateRaw)
		testPerson.BirthDate = &bd
	}
	if current.DeathDateRaw != "" {
		dd := domain.ParseGenDate(current.DeathDateRaw)
		testPerson.DeathDate = &dd
	}

	recordChange(changes, "given_name", SetPtr(input.GivenName), &testPerson.GivenName)
	recordChange(changes, "surname", input.Surname, &testPerson.Surname)
	recordChange(changes, "gender", input.Gender, &testPerson.Gender)
	birthDate, birthChanged, err := changedDate("birth_date", input.BirthDate, current.BirthDateRaw)
	if err != nil {
		return nil, err
	}
	if birthChanged {
		testPerson.SetBirthDate(birthDate)
		changes["birth_date"] = birthDate
	}
	recordChange(changes, "birth_place", input.BirthPlace, &testPerson.BirthPlace)
	deathDate, deathChanged, err := changedDate("death_date", input.DeathDate, current.DeathDateRaw)
	if err != nil {
		return nil, err
	}
	if deathChanged {
		testPerson.SetDeathDate(deathDate)
		changes["death_date"] = deathDate
	}
	recordChange(changes, "death_place", input.DeathPlace, &testPerson.DeathPlace)
	recordChange(changes, "notes", input.Notes, &testPerson.Notes)
	// A clear leaves the status unset (""), recorded as nil: an empty string
	// in a stored event has always meant "unknown" (ParseResearchStatus).
	if input.ResearchStatus.Cleared() {
		if current.ResearchStatus != "" {
			testPerson.ResearchStatus = ""
			changes["research_status"] = nil
		}
	} else if status, ok := input.ResearchStatus.Value(); ok && domain.ParseResearchStatus(status) != current.ResearchStatus {
		testPerson.ResearchStatus = domain.ParseResearchStatus(status)
		changes["research_status"] = status
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
		return nil, fmt.Errorf("executing update person command: %w", err)
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
	current, err := h.readStore.GetPerson(ctx, h.branchID, input.ID)
	if err != nil {
		return fmt.Errorf("getting person: %w", err)
	}
	if current == nil {
		return ErrPersonNotFound
	}

	// Check version for optimistic locking
	if current.Version != input.Version {
		return repository.ErrConcurrencyConflict
	}

	// Check if person is linked to any families as partner
	families, err := h.readStore.GetFamiliesForPerson(ctx, h.branchID, input.ID)
	if err != nil {
		return fmt.Errorf("checking families for person: %w", err)
	}
	if len(families) > 0 {
		return ErrPersonHasFamilies
	}

	// Check if person is a child in any family
	childFamily, err := h.readStore.GetChildFamily(ctx, h.branchID, input.ID)
	if err != nil {
		return fmt.Errorf("getting child family: %w", err)
	}
	if childFamily != nil {
		return ErrPersonHasFamilies
	}

	// Create event
	event := domain.NewPersonDeleted(input.ID, input.Reason)

	// Execute command
	_, err = h.execute(ctx, input.ID.String(), "Person", []domain.Event{event}, input.Version)
	if err != nil {
		return fmt.Errorf("executing delete person command: %w", err)
	}
	return nil
}

// parseUUID parses a string to UUID.
func parseUUID(s string) (uuid.UUID, error) {
	return uuid.Parse(s)
}
