package domain_test

import (
	"strings"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/cacack/my-family/internal/domain"
)

func TestSubmitterValidationError(t *testing.T) {
	err := domain.SubmitterValidationError{Field: "name", Message: "cannot be empty"}
	assert.Equal(t, "name: cannot be empty", err.Error())
}

func TestNewSubmitter(t *testing.T) {
	name := "John Doe"
	s := domain.NewSubmitter(name)

	require.NotNil(t, s)
	assert.NotEqual(t, uuid.Nil, s.ID)
	assert.Equal(t, name, s.Name)
	assert.Equal(t, int64(1), s.Version)
	assert.Nil(t, s.Address)
	assert.Nil(t, s.Phone)
	assert.Nil(t, s.Email)
	assert.Empty(t, s.Language)
	assert.Nil(t, s.MediaID)
	assert.Empty(t, s.GedcomXref)
}

func TestNewSubmitterWithID(t *testing.T) {
	id := uuid.New()
	name := "Jane Smith"
	s := domain.NewSubmitterWithID(id, name)

	require.NotNil(t, s)
	assert.Equal(t, id, s.ID)
	assert.Equal(t, name, s.Name)
	assert.Equal(t, int64(1), s.Version)
}

func TestSubmitter_Validate(t *testing.T) {
	tests := []struct {
		name      string
		submitter *domain.Submitter
		wantErr   bool
		errField  string
	}{
		{
			name:      "valid submitter",
			submitter: domain.NewSubmitter("John Doe"),
			wantErr:   false,
		},
		{
			name:      "empty name",
			submitter: &domain.Submitter{ID: uuid.New(), Name: ""},
			wantErr:   true,
			errField:  "name",
		},
		{
			name:      "name too long",
			submitter: &domain.Submitter{ID: uuid.New(), Name: strings.Repeat("a", 201)},
			wantErr:   true,
			errField:  "name",
		},
		{
			name:      "name at max length",
			submitter: &domain.Submitter{ID: uuid.New(), Name: strings.Repeat("a", 200)},
			wantErr:   false,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			err := tc.submitter.Validate()
			if tc.wantErr {
				require.Error(t, err)
				var validErr *domain.SubmitterValidationError
				require.ErrorAs(t, err, &validErr)
				assert.Equal(t, tc.errField, validErr.Field)
			} else {
				require.NoError(t, err)
			}
		})
	}
}

func TestSubmitter_SetName(t *testing.T) {
	s := domain.NewSubmitter("Original Name")
	s.SetName("New Name")
	assert.Equal(t, "New Name", s.Name)
}

func TestSubmitter_SetAddress(t *testing.T) {
	s := domain.NewSubmitter("John Doe")
	addr := &domain.Address{
		Line1: "123 Main St",
		City:  "Springfield",
		State: "IL",
	}
	s.SetAddress(addr)

	require.NotNil(t, s.Address)
	assert.Equal(t, "123 Main St", s.Address.Line1)
	assert.Equal(t, "Springfield", s.Address.City)
	assert.Equal(t, "IL", s.Address.State)
}

func TestSubmitter_AddPhone(t *testing.T) {
	s := domain.NewSubmitter("John Doe")

	// Add a phone number
	s.AddPhone("555-1234")
	require.Len(t, s.Phone, 1)
	assert.Equal(t, "555-1234", s.Phone[0])

	// Add another phone number
	s.AddPhone("555-5678")
	require.Len(t, s.Phone, 2)
	assert.Equal(t, "555-5678", s.Phone[1])

	// Empty phone should not be added
	s.AddPhone("")
	require.Len(t, s.Phone, 2)
}

func TestSubmitter_AddEmail(t *testing.T) {
	s := domain.NewSubmitter("John Doe")

	// Add an email
	s.AddEmail("john@example.com")
	require.Len(t, s.Email, 1)
	assert.Equal(t, "john@example.com", s.Email[0])

	// Add another email
	s.AddEmail("john.doe@work.com")
	require.Len(t, s.Email, 2)
	assert.Equal(t, "john.doe@work.com", s.Email[1])

	// Empty email should not be added
	s.AddEmail("")
	require.Len(t, s.Email, 2)
}

func TestSubmitter_SetLanguage(t *testing.T) {
	s := domain.NewSubmitter("John Doe")
	s.SetLanguage("English")
	assert.Equal(t, "English", s.Language)
}

func TestSubmitter_SetMediaID(t *testing.T) {
	s := domain.NewSubmitter("John Doe")
	mediaID := uuid.New()
	s.SetMediaID(&mediaID)

	require.NotNil(t, s.MediaID)
	assert.Equal(t, mediaID, *s.MediaID)

	// Set to nil
	s.SetMediaID(nil)
	assert.Nil(t, s.MediaID)
}

func TestSubmitter_SetGedcomXref(t *testing.T) {
	s := domain.NewSubmitter("John Doe")
	s.SetGedcomXref("@SUBM1@")
	assert.Equal(t, "@SUBM1@", s.GedcomXref)
}
