package domain

import (
	"errors"
	"fmt"

	"github.com/google/uuid"
)

// Repository represents a physical or digital location where source documents are stored.
// This maps to GEDCOM REPO records and supports GPS-compliant source documentation.
type Repository struct {
	ID         uuid.UUID `json:"id"`
	Name       string    `json:"name"`
	Address    string    `json:"address,omitempty"`
	City       string    `json:"city,omitempty"`
	State      string    `json:"state,omitempty"`
	PostalCode string    `json:"postal_code,omitempty"`
	Country    string    `json:"country,omitempty"`
	Phone      string    `json:"phone,omitempty"`
	Email      string    `json:"email,omitempty"`
	Website    string    `json:"website,omitempty"`
	Notes      string    `json:"notes,omitempty"`
	GedcomXref string    `json:"gedcom_xref,omitempty"` // Original GEDCOM @XREF@ for round-trip
	Version    int64     `json:"version"`               // Optimistic locking version
}

// RepositoryValidationError represents a validation error for a Repository.
type RepositoryValidationError struct {
	Field   string
	Message string
}

func (e RepositoryValidationError) Error() string {
	return fmt.Sprintf("%s: %s", e.Field, e.Message)
}

// NewRepository creates a new Repository with the given name.
func NewRepository(name string) *Repository {
	return &Repository{
		ID:      uuid.New(),
		Name:    name,
		Version: 1,
	}
}

// Validate checks if the repository has valid data.
func (r *Repository) Validate() error {
	var errs []error

	// Required fields
	if r.Name == "" {
		errs = append(errs, RepositoryValidationError{Field: "name", Message: "cannot be empty"})
	}
	if len(r.Name) > 200 {
		errs = append(errs, RepositoryValidationError{Field: "name", Message: "cannot exceed 200 characters"})
	}

	if len(errs) > 0 {
		return errors.Join(errs...)
	}
	return nil
}

// FullAddress returns a formatted address string.
func (r *Repository) FullAddress() string {
	parts := []string{}
	if r.Address != "" {
		parts = append(parts, r.Address)
	}
	if r.City != "" {
		parts = append(parts, r.City)
	}
	if r.State != "" {
		parts = append(parts, r.State)
	}
	if r.PostalCode != "" {
		parts = append(parts, r.PostalCode)
	}
	if r.Country != "" {
		parts = append(parts, r.Country)
	}

	result := ""
	for i, part := range parts {
		if i > 0 {
			result += ", "
		}
		result += part
	}
	return result
}
