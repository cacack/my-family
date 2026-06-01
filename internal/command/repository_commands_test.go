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

func TestCreateRepository(t *testing.T) {
	eventStore := memory.NewEventStore()
	readStore := memory.NewReadModelStore()
	handler := command.NewHandler(eventStore, readStore)
	ctx := context.Background()

	tests := []struct {
		name    string
		input   command.CreateRepositoryInput
		wantErr bool
	}{
		{
			name: "create repository with name only",
			input: command.CreateRepositoryInput{
				Name: "National Archives",
			},
			wantErr: false,
		},
		{
			name: "create repository with all fields",
			input: command.CreateRepositoryInput{
				Name: "State Historical Society",
				Address: &domain.Address{
					Line1:      "123 Archive Way",
					City:       "Springfield",
					State:      "IL",
					PostalCode: "62701",
					Country:    "USA",
					Phone:      "555-1234",
					Email:      "info@archives.example.com",
					Website:    "https://archives.example.com",
				},
				Notes:      "Open weekdays 9-5",
				GedcomXref: "@REPO1@",
			},
			wantErr: false,
		},
		{
			name: "fail with empty name",
			input: command.CreateRepositoryInput{
				Name: "",
			},
			wantErr: true,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			result, err := handler.CreateRepository(ctx, tc.input)
			if tc.wantErr {
				require.Error(t, err)
				return
			}

			require.NoError(t, err)
			require.NotNil(t, result)
			assert.NotEqual(t, uuid.Nil, result.ID)
			assert.Equal(t, int64(1), result.Version)

			// Verify repository was saved in read model
			saved, err := readStore.GetRepository(ctx, result.ID)
			require.NoError(t, err)
			require.NotNil(t, saved)
			assert.Equal(t, tc.input.Name, saved.Name)
		})
	}
}

func TestCreateRepository_WithAddress(t *testing.T) {
	eventStore := memory.NewEventStore()
	readStore := memory.NewReadModelStore()
	handler := command.NewHandler(eventStore, readStore)
	ctx := context.Background()

	input := command.CreateRepositoryInput{
		Name: "County Library",
		Address: &domain.Address{
			Line1: "456 Oak Ave",
			City:  "Boston",
		},
	}

	result, err := handler.CreateRepository(ctx, input)
	require.NoError(t, err)
	require.NotNil(t, result)

	saved, err := readStore.GetRepository(ctx, result.ID)
	require.NoError(t, err)
	require.NotNil(t, saved)
	require.NotNil(t, saved.Address)
	assert.Equal(t, "456 Oak Ave", saved.Address.Line1)
	assert.Equal(t, "Boston", saved.Address.City)
}

func TestUpdateRepository(t *testing.T) {
	eventStore := memory.NewEventStore()
	readStore := memory.NewReadModelStore()
	handler := command.NewHandler(eventStore, readStore)
	ctx := context.Background()

	// Create initial repository
	createResult, err := handler.CreateRepository(ctx, command.CreateRepositoryInput{
		Name: "Old Name",
	})
	require.NoError(t, err)

	// Update the repository
	newName := "New Name"
	updateResult, err := handler.UpdateRepository(ctx, command.UpdateRepositoryInput{
		ID:      createResult.ID,
		Name:    &newName,
		Version: createResult.Version,
	})
	require.NoError(t, err)
	assert.Equal(t, int64(2), updateResult.Version)

	// Verify the update
	saved, err := readStore.GetRepository(ctx, createResult.ID)
	require.NoError(t, err)
	assert.Equal(t, "New Name", saved.Name)
}

func TestUpdateRepository_AllFields(t *testing.T) {
	eventStore := memory.NewEventStore()
	readStore := memory.NewReadModelStore()
	handler := command.NewHandler(eventStore, readStore)
	ctx := context.Background()

	// Create initial repository
	createResult, err := handler.CreateRepository(ctx, command.CreateRepositoryInput{
		Name: "Original Repo",
	})
	require.NoError(t, err)

	// Update with all fields
	newName := "Updated Repo"
	newNotes := "Updated notes"
	newXref := "@REPO99@"
	updateResult, err := handler.UpdateRepository(ctx, command.UpdateRepositoryInput{
		ID:   createResult.ID,
		Name: &newName,
		Address: &domain.Address{
			Line1: "789 Pine St",
			City:  "Chicago",
		},
		Notes:      &newNotes,
		GedcomXref: &newXref,
		Version:    createResult.Version,
	})
	require.NoError(t, err)
	assert.Equal(t, int64(2), updateResult.Version)

	// Verify the update
	saved, err := readStore.GetRepository(ctx, createResult.ID)
	require.NoError(t, err)
	assert.Equal(t, "Updated Repo", saved.Name)
	assert.Equal(t, "Updated notes", saved.Notes)
	assert.Equal(t, "@REPO99@", saved.GedcomXref)
	require.NotNil(t, saved.Address)
	assert.Equal(t, "789 Pine St", saved.Address.Line1)
}

func TestUpdateRepository_NoChanges(t *testing.T) {
	eventStore := memory.NewEventStore()
	readStore := memory.NewReadModelStore()
	handler := command.NewHandler(eventStore, readStore)
	ctx := context.Background()

	// Create initial repository
	createResult, err := handler.CreateRepository(ctx, command.CreateRepositoryInput{
		Name: "Same Name",
	})
	require.NoError(t, err)

	// Update with same name (no changes)
	sameName := "Same Name"
	updateResult, err := handler.UpdateRepository(ctx, command.UpdateRepositoryInput{
		ID:      createResult.ID,
		Name:    &sameName,
		Version: createResult.Version,
	})
	require.NoError(t, err)
	// Version should remain the same since no actual changes
	assert.Equal(t, createResult.Version, updateResult.Version)
}

func TestUpdateRepository_NotFound(t *testing.T) {
	eventStore := memory.NewEventStore()
	readStore := memory.NewReadModelStore()
	handler := command.NewHandler(eventStore, readStore)
	ctx := context.Background()

	newName := "New Name"
	_, err := handler.UpdateRepository(ctx, command.UpdateRepositoryInput{
		ID:      uuid.New(),
		Name:    &newName,
		Version: 1,
	})
	require.Error(t, err)
	assert.ErrorIs(t, err, command.ErrRepositoryNotFound)
}

func TestUpdateRepository_ConcurrencyConflict(t *testing.T) {
	eventStore := memory.NewEventStore()
	readStore := memory.NewReadModelStore()
	handler := command.NewHandler(eventStore, readStore)
	ctx := context.Background()

	// Create initial repository
	createResult, err := handler.CreateRepository(ctx, command.CreateRepositoryInput{
		Name: "Test Repo",
	})
	require.NoError(t, err)

	// Update with wrong version
	newName := "Changed"
	_, err = handler.UpdateRepository(ctx, command.UpdateRepositoryInput{
		ID:      createResult.ID,
		Name:    &newName,
		Version: 99, // Wrong version
	})
	require.Error(t, err)
	assert.ErrorIs(t, err, repository.ErrConcurrencyConflict)
}

func TestDeleteRepository(t *testing.T) {
	eventStore := memory.NewEventStore()
	readStore := memory.NewReadModelStore()
	handler := command.NewHandler(eventStore, readStore)
	ctx := context.Background()

	// Create a repository
	createResult, err := handler.CreateRepository(ctx, command.CreateRepositoryInput{
		Name: "To Delete",
	})
	require.NoError(t, err)

	// Delete the repository
	err = handler.DeleteRepository(ctx, createResult.ID, createResult.Version, "Test deletion")
	require.NoError(t, err)

	// Verify it's deleted from read model
	deleted, err := readStore.GetRepository(ctx, createResult.ID)
	require.NoError(t, err)
	assert.Nil(t, deleted)
}

func TestDeleteRepository_NotFound(t *testing.T) {
	eventStore := memory.NewEventStore()
	readStore := memory.NewReadModelStore()
	handler := command.NewHandler(eventStore, readStore)
	ctx := context.Background()

	err := handler.DeleteRepository(ctx, uuid.New(), 1, "Test deletion")
	require.Error(t, err)
	assert.ErrorIs(t, err, command.ErrRepositoryNotFound)
}

func TestDeleteRepository_ConcurrencyConflict(t *testing.T) {
	eventStore := memory.NewEventStore()
	readStore := memory.NewReadModelStore()
	handler := command.NewHandler(eventStore, readStore)
	ctx := context.Background()

	// Create a repository
	createResult, err := handler.CreateRepository(ctx, command.CreateRepositoryInput{
		Name: "Test Repo",
	})
	require.NoError(t, err)

	// Delete with wrong version
	err = handler.DeleteRepository(ctx, createResult.ID, 99, "Test deletion")
	require.Error(t, err)
	assert.ErrorIs(t, err, repository.ErrConcurrencyConflict)
}
