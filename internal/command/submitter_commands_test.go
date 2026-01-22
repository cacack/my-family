package command_test

import (
	"context"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/cacack/my-family/internal/command"
	"github.com/cacack/my-family/internal/domain"
	"github.com/cacack/my-family/internal/repository"
	"github.com/cacack/my-family/internal/repository/memory"
)

func TestCreateSubmitter(t *testing.T) {
	eventStore := memory.NewEventStore()
	readStore := memory.NewReadModelStore()
	handler := command.NewHandler(eventStore, readStore)
	ctx := context.Background()

	tests := []struct {
		name    string
		input   command.CreateSubmitterInput
		wantErr bool
	}{
		{
			name: "create submitter with name only",
			input: command.CreateSubmitterInput{
				Name: "John Doe",
			},
			wantErr: false,
		},
		{
			name: "create submitter with all fields",
			input: command.CreateSubmitterInput{
				Name: "Jane Smith",
				Address: &domain.Address{
					Line1:      "123 Main St",
					City:       "Springfield",
					State:      "IL",
					PostalCode: "62701",
					Country:    "USA",
				},
				Phone:      []string{"555-1234", "555-5678"},
				Email:      []string{"jane@example.com"},
				Language:   "English",
				GedcomXref: "@SUBM1@",
			},
			wantErr: false,
		},
		{
			name: "fail with empty name",
			input: command.CreateSubmitterInput{
				Name: "",
			},
			wantErr: true,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			result, err := handler.CreateSubmitter(ctx, tc.input)
			if tc.wantErr {
				require.Error(t, err)
				return
			}

			require.NoError(t, err)
			require.NotNil(t, result)
			assert.NotEqual(t, uuid.Nil, result.ID)
			assert.Equal(t, int64(1), result.Version)

			// Verify submitter was saved in read model
			saved, err := readStore.GetSubmitter(ctx, result.ID)
			require.NoError(t, err)
			require.NotNil(t, saved)
			assert.Equal(t, tc.input.Name, saved.Name)
		})
	}
}

func TestCreateSubmitterWithMediaID(t *testing.T) {
	eventStore := memory.NewEventStore()
	readStore := memory.NewReadModelStore()
	handler := command.NewHandler(eventStore, readStore)
	ctx := context.Background()

	mediaID := uuid.New()
	input := command.CreateSubmitterInput{
		Name:    "John Doe",
		MediaID: &mediaID,
	}

	result, err := handler.CreateSubmitter(ctx, input)
	require.NoError(t, err)
	require.NotNil(t, result)

	saved, err := readStore.GetSubmitter(ctx, result.ID)
	require.NoError(t, err)
	require.NotNil(t, saved)
	require.NotNil(t, saved.MediaID)
	assert.Equal(t, mediaID, *saved.MediaID)
}

func TestUpdateSubmitter(t *testing.T) {
	eventStore := memory.NewEventStore()
	readStore := memory.NewReadModelStore()
	handler := command.NewHandler(eventStore, readStore)
	ctx := context.Background()

	// Create initial submitter
	createResult, err := handler.CreateSubmitter(ctx, command.CreateSubmitterInput{
		Name: "John Doe",
	})
	require.NoError(t, err)

	// Update the submitter
	newName := "John Smith"
	updateResult, err := handler.UpdateSubmitter(ctx, command.UpdateSubmitterInput{
		ID:      createResult.ID,
		Name:    &newName,
		Version: createResult.Version,
	})
	require.NoError(t, err)
	assert.Equal(t, int64(2), updateResult.Version)

	// Verify the update
	saved, err := readStore.GetSubmitter(ctx, createResult.ID)
	require.NoError(t, err)
	assert.Equal(t, "John Smith", saved.Name)
}

func TestUpdateSubmitter_AllFields(t *testing.T) {
	eventStore := memory.NewEventStore()
	readStore := memory.NewReadModelStore()
	handler := command.NewHandler(eventStore, readStore)
	ctx := context.Background()

	// Create initial submitter
	createResult, err := handler.CreateSubmitter(ctx, command.CreateSubmitterInput{
		Name: "John Doe",
	})
	require.NoError(t, err)

	// Update with all fields
	newName := "Jane Smith"
	newLanguage := "French"
	mediaID := uuid.New()
	updateResult, err := handler.UpdateSubmitter(ctx, command.UpdateSubmitterInput{
		ID:   createResult.ID,
		Name: &newName,
		Address: &domain.Address{
			Line1: "456 Oak Ave",
			City:  "Boston",
		},
		Phone:    []string{"555-9999"},
		Email:    []string{"jane@example.com"},
		Language: &newLanguage,
		MediaID:  &mediaID,
		Version:  createResult.Version,
	})
	require.NoError(t, err)
	assert.Equal(t, int64(2), updateResult.Version)

	// Verify the update
	saved, err := readStore.GetSubmitter(ctx, createResult.ID)
	require.NoError(t, err)
	assert.Equal(t, "Jane Smith", saved.Name)
	assert.Equal(t, "French", saved.Language)
}

func TestUpdateSubmitter_NoChanges(t *testing.T) {
	eventStore := memory.NewEventStore()
	readStore := memory.NewReadModelStore()
	handler := command.NewHandler(eventStore, readStore)
	ctx := context.Background()

	// Create initial submitter
	createResult, err := handler.CreateSubmitter(ctx, command.CreateSubmitterInput{
		Name: "John Doe",
	})
	require.NoError(t, err)

	// Update with same name (no changes)
	sameName := "John Doe"
	updateResult, err := handler.UpdateSubmitter(ctx, command.UpdateSubmitterInput{
		ID:      createResult.ID,
		Name:    &sameName,
		Version: createResult.Version,
	})
	require.NoError(t, err)
	// Version should remain the same since no actual changes
	assert.Equal(t, createResult.Version, updateResult.Version)
}

func TestUpdateSubmitter_NotFound(t *testing.T) {
	eventStore := memory.NewEventStore()
	readStore := memory.NewReadModelStore()
	handler := command.NewHandler(eventStore, readStore)
	ctx := context.Background()

	newName := "New Name"
	_, err := handler.UpdateSubmitter(ctx, command.UpdateSubmitterInput{
		ID:      uuid.New(),
		Name:    &newName,
		Version: 1,
	})
	require.Error(t, err)
	assert.ErrorIs(t, err, command.ErrSubmitterNotFound)
}

func TestUpdateSubmitter_ConcurrencyConflict(t *testing.T) {
	eventStore := memory.NewEventStore()
	readStore := memory.NewReadModelStore()
	handler := command.NewHandler(eventStore, readStore)
	ctx := context.Background()

	// Create initial submitter
	createResult, err := handler.CreateSubmitter(ctx, command.CreateSubmitterInput{
		Name: "John Doe",
	})
	require.NoError(t, err)

	// Update with wrong version
	newName := "Jane Smith"
	_, err = handler.UpdateSubmitter(ctx, command.UpdateSubmitterInput{
		ID:      createResult.ID,
		Name:    &newName,
		Version: 99, // Wrong version
	})
	require.Error(t, err)
	assert.ErrorIs(t, err, repository.ErrConcurrencyConflict)
}

func TestDeleteSubmitter(t *testing.T) {
	eventStore := memory.NewEventStore()
	readStore := memory.NewReadModelStore()
	handler := command.NewHandler(eventStore, readStore)
	ctx := context.Background()

	// Create a submitter
	createResult, err := handler.CreateSubmitter(ctx, command.CreateSubmitterInput{
		Name: "John Doe",
	})
	require.NoError(t, err)

	// Delete the submitter
	err = handler.DeleteSubmitter(ctx, createResult.ID, createResult.Version, "Test deletion")
	require.NoError(t, err)

	// Verify it's deleted from read model
	deleted, err := readStore.GetSubmitter(ctx, createResult.ID)
	require.NoError(t, err)
	assert.Nil(t, deleted)
}

func TestDeleteSubmitter_NotFound(t *testing.T) {
	eventStore := memory.NewEventStore()
	readStore := memory.NewReadModelStore()
	handler := command.NewHandler(eventStore, readStore)
	ctx := context.Background()

	err := handler.DeleteSubmitter(ctx, uuid.New(), 1, "Test deletion")
	require.Error(t, err)
	assert.ErrorIs(t, err, command.ErrSubmitterNotFound)
}

func TestDeleteSubmitter_ConcurrencyConflict(t *testing.T) {
	eventStore := memory.NewEventStore()
	readStore := memory.NewReadModelStore()
	handler := command.NewHandler(eventStore, readStore)
	ctx := context.Background()

	// Create a submitter
	createResult, err := handler.CreateSubmitter(ctx, command.CreateSubmitterInput{
		Name: "John Doe",
	})
	require.NoError(t, err)

	// Delete with wrong version
	err = handler.DeleteSubmitter(ctx, createResult.ID, 99, "Test deletion")
	require.Error(t, err)
	assert.ErrorIs(t, err, repository.ErrConcurrencyConflict)
}
