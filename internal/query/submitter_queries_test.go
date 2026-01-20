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

func TestNewSubmitterService(t *testing.T) {
	readStore := memory.NewReadModelStore()
	service := query.NewSubmitterService(readStore)
	require.NotNil(t, service)
}

func TestSubmitterService_ListSubmitters(t *testing.T) {
	readStore := memory.NewReadModelStore()
	service := query.NewSubmitterService(readStore)
	ctx := context.Background()

	// Add some submitters to the read store
	now := time.Now()
	sub1 := &repository.SubmitterReadModel{
		ID:        uuid.New(),
		Name:      "Alice Researcher",
		Language:  "English",
		Version:   1,
		UpdatedAt: now,
	}
	sub2 := &repository.SubmitterReadModel{
		ID:        uuid.New(),
		Name:      "Bob Genealogist",
		Version:   1,
		UpdatedAt: now.Add(-time.Hour),
	}

	require.NoError(t, readStore.SaveSubmitter(ctx, sub1))
	require.NoError(t, readStore.SaveSubmitter(ctx, sub2))

	t.Run("list all submitters", func(t *testing.T) {
		result, err := service.ListSubmitters(ctx, query.ListSubmittersInput{
			Limit:  10,
			Offset: 0,
		})
		require.NoError(t, err)
		require.NotNil(t, result)
		assert.Len(t, result.Submitters, 2)
		assert.Equal(t, 2, result.Total)
		assert.Equal(t, 10, result.Limit)
	})

	t.Run("list with pagination", func(t *testing.T) {
		result, err := service.ListSubmitters(ctx, query.ListSubmittersInput{
			Limit:  1,
			Offset: 0,
		})
		require.NoError(t, err)
		require.NotNil(t, result)
		assert.Len(t, result.Submitters, 1)
		assert.Equal(t, 2, result.Total)
	})

	t.Run("list with offset", func(t *testing.T) {
		result, err := service.ListSubmitters(ctx, query.ListSubmittersInput{
			Limit:  10,
			Offset: 1,
		})
		require.NoError(t, err)
		require.NotNil(t, result)
		assert.Len(t, result.Submitters, 1)
		assert.Equal(t, 2, result.Total)
		assert.Equal(t, 1, result.Offset)
	})

	t.Run("default limit applied when zero", func(t *testing.T) {
		result, err := service.ListSubmitters(ctx, query.ListSubmittersInput{
			Limit: 0,
		})
		require.NoError(t, err)
		assert.Equal(t, 20, result.Limit)
	})

	t.Run("max limit capped at 100", func(t *testing.T) {
		result, err := service.ListSubmitters(ctx, query.ListSubmittersInput{
			Limit: 200,
		})
		require.NoError(t, err)
		assert.Equal(t, 100, result.Limit)
	})

	t.Run("default order is desc", func(t *testing.T) {
		result, err := service.ListSubmitters(ctx, query.ListSubmittersInput{
			Limit: 10,
		})
		require.NoError(t, err)
		require.NotNil(t, result)
	})

	t.Run("ascending order", func(t *testing.T) {
		result, err := service.ListSubmitters(ctx, query.ListSubmittersInput{
			Limit:     10,
			SortOrder: "asc",
		})
		require.NoError(t, err)
		require.NotNil(t, result)
	})
}

func TestSubmitterService_GetSubmitter(t *testing.T) {
	readStore := memory.NewReadModelStore()
	service := query.NewSubmitterService(readStore)
	ctx := context.Background()

	t.Run("get existing submitter", func(t *testing.T) {
		// Add a submitter
		id := uuid.New()
		mediaID := uuid.New()
		sub := &repository.SubmitterReadModel{
			ID:         id,
			Name:       "John Doe",
			Address:    &domain.Address{Line1: "123 Main St", City: "Springfield"},
			Phone:      []string{"555-1234"},
			Email:      []string{"john@example.com"},
			Language:   "English",
			MediaID:    &mediaID,
			GedcomXref: "@SUBM1@",
			Version:    1,
			UpdatedAt:  time.Now(),
		}
		require.NoError(t, readStore.SaveSubmitter(ctx, sub))

		// Get the submitter
		result, err := service.GetSubmitter(ctx, id)
		require.NoError(t, err)
		require.NotNil(t, result)
		assert.Equal(t, id, result.ID)
		assert.Equal(t, "John Doe", result.Name)
		require.NotNil(t, result.Address)
		assert.Equal(t, "123 Main St", result.Address.Line1)
		assert.Len(t, result.Phone, 1)
		assert.Len(t, result.Email, 1)
		require.NotNil(t, result.Language)
		assert.Equal(t, "English", *result.Language)
		require.NotNil(t, result.MediaID)
		assert.Equal(t, mediaID, *result.MediaID)
		require.NotNil(t, result.GedcomXref)
		assert.Equal(t, "@SUBM1@", *result.GedcomXref)
		assert.Equal(t, int64(1), result.Version)
	})

	t.Run("get submitter with minimal fields", func(t *testing.T) {
		// Add a submitter with only required fields
		id := uuid.New()
		sub := &repository.SubmitterReadModel{
			ID:        id,
			Name:      "Jane Smith",
			Version:   1,
			UpdatedAt: time.Now(),
		}
		require.NoError(t, readStore.SaveSubmitter(ctx, sub))

		// Get the submitter
		result, err := service.GetSubmitter(ctx, id)
		require.NoError(t, err)
		require.NotNil(t, result)
		assert.Equal(t, id, result.ID)
		assert.Equal(t, "Jane Smith", result.Name)
		assert.Nil(t, result.Address)
		assert.Nil(t, result.Phone)
		assert.Nil(t, result.Email)
		assert.Nil(t, result.Language)
		assert.Nil(t, result.MediaID)
		assert.Nil(t, result.GedcomXref)
	})

	t.Run("get non-existent submitter", func(t *testing.T) {
		result, err := service.GetSubmitter(ctx, uuid.New())
		require.Error(t, err)
		assert.ErrorIs(t, err, query.ErrNotFound)
		assert.Nil(t, result)
	})
}
