package repository

import (
	"context"
	"time"

	"github.com/google/uuid"

	"github.com/cacack/my-family/internal/domain"
)

// PersonReadModel represents a person in the read model.
type PersonReadModel struct {
	ID            uuid.UUID     `json:"id"`
	GivenName     string        `json:"given_name"`
	Surname       string        `json:"surname"`
	FullName      string        `json:"full_name"`
	Gender        domain.Gender `json:"gender,omitempty"`
	BirthDateRaw  string        `json:"birth_date_raw,omitempty"`
	BirthDateSort *time.Time    `json:"birth_date_sort,omitempty"`
	BirthPlace    string        `json:"birth_place,omitempty"`
	DeathDateRaw  string        `json:"death_date_raw,omitempty"`
	DeathDateSort *time.Time    `json:"death_date_sort,omitempty"`
	DeathPlace    string        `json:"death_place,omitempty"`
	Notes         string        `json:"notes,omitempty"`
	Version       int64         `json:"version"`
	UpdatedAt     time.Time     `json:"updated_at"`
}

// FamilyReadModel represents a family in the read model.
type FamilyReadModel struct {
	ID               uuid.UUID           `json:"id"`
	Partner1ID       *uuid.UUID          `json:"partner1_id,omitempty"`
	Partner1Name     string              `json:"partner1_name,omitempty"`
	Partner2ID       *uuid.UUID          `json:"partner2_id,omitempty"`
	Partner2Name     string              `json:"partner2_name,omitempty"`
	RelationshipType domain.RelationType `json:"relationship_type,omitempty"`
	MarriageDateRaw  string              `json:"marriage_date_raw,omitempty"`
	MarriageDateSort *time.Time          `json:"marriage_date_sort,omitempty"`
	MarriagePlace    string              `json:"marriage_place,omitempty"`
	ChildCount       int                 `json:"child_count"`
	Version          int64               `json:"version"`
	UpdatedAt        time.Time           `json:"updated_at"`
}

// FamilyChildReadModel represents a child in a family.
type FamilyChildReadModel struct {
	FamilyID         uuid.UUID                `json:"family_id"`
	PersonID         uuid.UUID                `json:"person_id"`
	PersonName       string                   `json:"person_name"`
	RelationshipType domain.ChildRelationType `json:"relationship_type"`
	Sequence         *int                     `json:"sequence,omitempty"`
}

// PedigreeEdge represents a parent-child relationship for pedigree traversal.
type PedigreeEdge struct {
	PersonID   uuid.UUID  `json:"person_id"`
	FatherID   *uuid.UUID `json:"father_id,omitempty"`
	MotherID   *uuid.UUID `json:"mother_id,omitempty"`
	FatherName string     `json:"father_name,omitempty"`
	MotherName string     `json:"mother_name,omitempty"`
}

// ReadModelStore provides access to denormalized read models.
type ReadModelStore interface {
	// Person operations
	GetPerson(ctx context.Context, id uuid.UUID) (*PersonReadModel, error)
	ListPersons(ctx context.Context, opts ListOptions) ([]PersonReadModel, int, error)
	SearchPersons(ctx context.Context, query string, fuzzy bool, limit int) ([]PersonReadModel, error)
	SavePerson(ctx context.Context, person *PersonReadModel) error
	DeletePerson(ctx context.Context, id uuid.UUID) error

	// Family operations
	GetFamily(ctx context.Context, id uuid.UUID) (*FamilyReadModel, error)
	ListFamilies(ctx context.Context, opts ListOptions) ([]FamilyReadModel, int, error)
	GetFamiliesForPerson(ctx context.Context, personID uuid.UUID) ([]FamilyReadModel, error)
	SaveFamily(ctx context.Context, family *FamilyReadModel) error
	DeleteFamily(ctx context.Context, id uuid.UUID) error

	// Family children operations
	GetFamilyChildren(ctx context.Context, familyID uuid.UUID) ([]FamilyChildReadModel, error)
	GetChildrenOfFamily(ctx context.Context, familyID uuid.UUID) ([]PersonReadModel, error)
	GetChildFamily(ctx context.Context, personID uuid.UUID) (*FamilyReadModel, error)
	SaveFamilyChild(ctx context.Context, child *FamilyChildReadModel) error
	DeleteFamilyChild(ctx context.Context, familyID, personID uuid.UUID) error

	// Pedigree operations
	GetPedigreeEdge(ctx context.Context, personID uuid.UUID) (*PedigreeEdge, error)
	SavePedigreeEdge(ctx context.Context, edge *PedigreeEdge) error
	DeletePedigreeEdge(ctx context.Context, personID uuid.UUID) error
}

// ListOptions contains options for list queries.
type ListOptions struct {
	Limit  int
	Offset int
	Sort   string
	Order  string // "asc" or "desc"
}

// DefaultListOptions returns sensible defaults for list queries.
func DefaultListOptions() ListOptions {
	return ListOptions{
		Limit:  20,
		Offset: 0,
		Sort:   "surname",
		Order:  "asc",
	}
}
