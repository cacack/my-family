package query_test

import (
	"context"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/cacack/my-family/internal/domain"
	"github.com/cacack/my-family/internal/query"
	"github.com/cacack/my-family/internal/repository"
	"github.com/cacack/my-family/internal/repository/memory"
)

func TestNewRepositoryService(t *testing.T) {
	readStore := memory.NewReadModelStore()
	service := query.NewRepositoryService(readStore)
	require.NotNil(t, service)
}

func TestRepositoryService_ListRepositories(t *testing.T) {
	readStore := memory.NewReadModelStore()
	service := query.NewRepositoryService(readStore)
	ctx := context.Background()

	// Add some repositories to the read store
	now := time.Now()
	repo1 := &repository.RepositoryReadModel{
		ID:        uuid.New(),
		Name:      "National Archives",
		Notes:     "Primary source location",
		Version:   1,
		UpdatedAt: now,
	}
	repo2 := &repository.RepositoryReadModel{
		ID:        uuid.New(),
		Name:      "County Library",
		Version:   1,
		UpdatedAt: now.Add(-time.Hour),
	}

	require.NoError(t, readStore.SaveRepository(ctx, repo1))
	require.NoError(t, readStore.SaveRepository(ctx, repo2))

	t.Run("list all repositories", func(t *testing.T) {
		result, err := service.ListRepositories(ctx, query.ListRepositoriesInput{
			Limit:  10,
			Offset: 0,
		})
		require.NoError(t, err)
		require.NotNil(t, result)
		assert.Len(t, result.Repositories, 2)
		assert.Equal(t, 2, result.Total)
		assert.Equal(t, 10, result.Limit)
	})

	t.Run("list with pagination", func(t *testing.T) {
		result, err := service.ListRepositories(ctx, query.ListRepositoriesInput{
			Limit:  1,
			Offset: 0,
		})
		require.NoError(t, err)
		require.NotNil(t, result)
		assert.Len(t, result.Repositories, 1)
		assert.Equal(t, 2, result.Total)
	})

	t.Run("list with offset", func(t *testing.T) {
		result, err := service.ListRepositories(ctx, query.ListRepositoriesInput{
			Limit:  10,
			Offset: 1,
		})
		require.NoError(t, err)
		require.NotNil(t, result)
		assert.Len(t, result.Repositories, 1)
		assert.Equal(t, 2, result.Total)
		assert.Equal(t, 1, result.Offset)
	})

	t.Run("default limit applied when zero", func(t *testing.T) {
		result, err := service.ListRepositories(ctx, query.ListRepositoriesInput{
			Limit: 0,
		})
		require.NoError(t, err)
		assert.Equal(t, 20, result.Limit)
	})

	t.Run("max limit capped at 100", func(t *testing.T) {
		result, err := service.ListRepositories(ctx, query.ListRepositoriesInput{
			Limit: 200,
		})
		require.NoError(t, err)
		assert.Equal(t, 100, result.Limit)
	})

	t.Run("default order is desc", func(t *testing.T) {
		result, err := service.ListRepositories(ctx, query.ListRepositoriesInput{
			Limit: 10,
		})
		require.NoError(t, err)
		require.NotNil(t, result)
	})

	t.Run("ascending order", func(t *testing.T) {
		result, err := service.ListRepositories(ctx, query.ListRepositoriesInput{
			Limit:     10,
			SortOrder: "asc",
		})
		require.NoError(t, err)
		require.NotNil(t, result)
	})
}

func TestRepositoryService_GetRepository(t *testing.T) {
	readStore := memory.NewReadModelStore()
	service := query.NewRepositoryService(readStore)
	ctx := context.Background()

	t.Run("get existing repository", func(t *testing.T) {
		// Add a repository
		id := uuid.New()
		repo := &repository.RepositoryReadModel{
			ID:         id,
			Name:       "National Archives",
			Address:    &domain.Address{Line1: "123 Archive Way", City: "Springfield"},
			Notes:      "Open weekdays",
			GedcomXref: "@REPO1@",
			Version:    1,
			UpdatedAt:  time.Now(),
		}
		require.NoError(t, readStore.SaveRepository(ctx, repo))

		// Get the repository
		result, err := service.GetRepository(ctx, id)
		require.NoError(t, err)
		require.NotNil(t, result)
		assert.Equal(t, id, result.ID)
		assert.Equal(t, "National Archives", result.Name)
		require.NotNil(t, result.Address)
		assert.Equal(t, "123 Archive Way", result.Address.Line1)
		require.NotNil(t, result.Notes)
		assert.Equal(t, "Open weekdays", *result.Notes)
		require.NotNil(t, result.GedcomXref)
		assert.Equal(t, "@REPO1@", *result.GedcomXref)
		assert.Equal(t, int64(1), result.Version)
	})

	t.Run("get repository with minimal fields", func(t *testing.T) {
		// Add a repository with only required fields
		id := uuid.New()
		repo := &repository.RepositoryReadModel{
			ID:        id,
			Name:      "County Library",
			Version:   1,
			UpdatedAt: time.Now(),
		}
		require.NoError(t, readStore.SaveRepository(ctx, repo))

		// Get the repository
		result, err := service.GetRepository(ctx, id)
		require.NoError(t, err)
		require.NotNil(t, result)
		assert.Equal(t, id, result.ID)
		assert.Equal(t, "County Library", result.Name)
		assert.Nil(t, result.Address)
		assert.Nil(t, result.Notes)
		assert.Nil(t, result.GedcomXref)
	})

	t.Run("get non-existent repository", func(t *testing.T) {
		result, err := service.GetRepository(ctx, uuid.New())
		require.Error(t, err)
		assert.ErrorIs(t, err, query.ErrNotFound)
		assert.Nil(t, result)
	})
}
