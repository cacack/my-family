package query_test

import (
	"context"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/cacack/my-family/internal/domain"
	"github.com/cacack/my-family/internal/repository"
)

// dismissPair records a mainline dismissal of (a, b) straight in the store.
func dismissPair(t *testing.T, store repository.ReadModelStore, a, b uuid.UUID, reason string, at time.Time) domain.DuplicatePair {
	t.Helper()
	pair, err := domain.NewDuplicatePair(a, b)
	require.NoError(t, err)
	require.NoError(t, store.SaveDuplicateDismissal(context.Background(), domain.MainBranchID, &repository.DuplicateDismissalReadModel{
		ID: pair.ID(), Person1ID: pair.Person1ID, Person2ID: pair.Person2ID, Reason: reason,
		Version: 1, CreatedAt: at, UpdatedAt: at,
	}))
	return pair
}

// TestFindDuplicates_ExcludesDismissedPairs is the #1036 read side: a pair
// marked not a duplicate is no longer suggested, is listed as dismissed with
// both names, and comes back once its dismissal is removed.
func TestFindDuplicates_ExcludesDismissedPairs(t *testing.T) {
	service, store := setupValidationService()
	ctx := context.Background()
	father := addPerson(store, "John", "Smith", "1950")
	son := addPerson(store, "John", "Smith", "1951")

	results, total, err := service.FindDuplicates(ctx, 100, 0)
	require.NoError(t, err)
	require.Equal(t, 1, total)
	require.Len(t, results, 1)

	dismissedAt := time.Date(2026, 10, 12, 9, 0, 0, 0, time.UTC)
	pair := dismissPair(t, store, father, son, "father and son", dismissedAt)

	results, total, err = service.FindDuplicates(ctx, 100, 0)
	require.NoError(t, err)
	assert.Zero(t, total, "a dismissed pair is not suggested")
	assert.Empty(t, results)

	dismissed, err := service.ListDismissedDuplicatesOn(ctx, domain.MainBranchID, 100, 0)
	require.NoError(t, err)
	require.Equal(t, 1, dismissed.Total)
	require.Len(t, dismissed.Items, 1)
	got := dismissed.Items[0]
	assert.Equal(t, pair.Person1ID, got.Person1ID)
	assert.Equal(t, pair.Person2ID, got.Person2ID)
	assert.Equal(t, "John Smith", got.Person1Name)
	assert.Equal(t, "John Smith", got.Person2Name)
	assert.Equal(t, "father and son", got.Reason)
	assert.True(t, dismissedAt.Equal(got.DismissedAt))

	require.NoError(t, store.DeleteDuplicateDismissal(ctx, domain.MainBranchID, pair.ID()))
	_, total, err = service.FindDuplicates(ctx, 100, 0)
	require.NoError(t, err)
	assert.Equal(t, 1, total, "a restored pair is suggested again")
}

// A dismissal naming a person who no longer exists is not listed: it can no
// longer match any suggestion.
func TestListDismissedDuplicates_SkipsVanishedPersons(t *testing.T) {
	service, store := setupValidationService()
	ctx := context.Background()
	a := addPerson(store, "Ann", "Smith", "1900")
	b := addPerson(store, "Ann", "Smith", "1901")
	c := addPerson(store, "Cal", "Jones", "1800")
	dismissPair(t, store, a, b, "", time.Now().UTC())
	dismissPair(t, store, a, c, "", time.Now().UTC())
	require.NoError(t, store.DeletePerson(ctx, domain.MainBranchID, b))

	dismissed, err := service.ListDismissedDuplicatesOn(ctx, domain.MainBranchID, 100, 0)
	require.NoError(t, err)
	assert.Equal(t, 1, dismissed.Total)
	require.Len(t, dismissed.Items, 1)
	assert.Equal(t, "Cal Jones", map[bool]string{true: dismissed.Items[0].Person1Name, false: dismissed.Items[0].Person2Name}[dismissed.Items[0].Person1ID == c])

	page, err := service.ListDismissedDuplicatesOn(ctx, domain.MainBranchID, 1, 1)
	require.NoError(t, err)
	assert.Equal(t, 1, page.Total)
	assert.Empty(t, page.Items, "offset past the end")
}

// A pair the branch marks not a duplicate is not reported as introduced by
// it, and a mainline pair it dismisses counts as resolved there (#1036).
func TestBranchHealth_HonorsDismissals(t *testing.T) {
	f := newHealthFixture()
	zed1, zed2, kit1, kit2 := uuid.New(), uuid.New(), uuid.New(), uuid.New()
	f.savePerson(t, domain.MainBranchID, zed1, "Zed", "1 JAN 1900", "")
	f.savePerson(t, domain.MainBranchID, zed2, "Zed", "1 JAN 1900", "")
	f.savePerson(t, f.branch, kit1, "Kit", "1 JAN 1880", "")
	f.savePerson(t, f.branch, kit2, "Kit", "1 JAN 1880", "")

	before, err := f.health.BranchHealth(f.ctx, f.branch)
	require.NoError(t, err)
	require.Len(t, before.Duplicates, 1, "the Kits are introduced by the branch")

	now := time.Now().UTC()
	for _, p := range [][2]uuid.UUID{{kit1, kit2}, {zed1, zed2}} {
		pair, err := domain.NewDuplicatePair(p[0], p[1])
		require.NoError(t, err)
		require.NoError(t, f.store.SaveDuplicateDismissal(f.ctx, f.branch, &repository.DuplicateDismissalReadModel{
			ID: pair.ID(), Person1ID: pair.Person1ID, Person2ID: pair.Person2ID, Version: 1, CreatedAt: now, UpdatedAt: now,
		}))
	}

	after, err := f.health.BranchHealth(f.ctx, f.branch)
	require.NoError(t, err)
	assert.Empty(t, after.Duplicates)
	assert.Equal(t, before.ResolvedCount+1, after.ResolvedCount, "dismissing main's Zed pair on the branch resolves it there")
}
