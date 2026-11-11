package postgres_test

import (
	"context"
	"slices"
	"testing"

	"github.com/google/uuid"

	"github.com/cacack/my-family/internal/domain"
	"github.com/cacack/my-family/internal/repository"
)

// A citation linked to one life event is listed with it and not with another
// of the same fact type; one with no link is listed with both (#1294).
func TestReadModelStore_GetCitationsForFactInstance(t *testing.T) {
	store, cleanup := setupReadModelStore(t)
	defer cleanup()
	ctx := context.Background()

	owner, first, second := uuid.New(), uuid.New(), uuid.New()
	save := func(factID *uuid.UUID) uuid.UUID {
		t.Helper()
		c := &repository.CitationReadModel{
			ID: uuid.New(), SourceID: uuid.New(), SourceTitle: "Census",
			FactType: domain.FactPersonCensus, FactOwnerID: owner, FactID: factID, Version: 1,
		}
		if err := store.SaveCitation(ctx, domain.MainBranchID, c); err != nil {
			t.Fatalf("save citation: %v", err)
		}
		return c.ID
	}
	ofFirst, ofSecond, ofAny := save(&first), save(&second), save(nil)

	if got, err := store.GetCitation(ctx, domain.MainBranchID, ofFirst); err != nil || got == nil || got.FactID == nil || *got.FactID != first {
		t.Fatalf("GetCitation = %+v, %v; want fact_id %s", got, err, first)
	}
	if got, err := store.GetCitation(ctx, domain.MainBranchID, ofAny); err != nil || got == nil || got.FactID != nil {
		t.Fatalf("GetCitation(unlinked) = %+v, %v; want no fact_id", got, err)
	}
	for factID, want := range map[uuid.UUID][]uuid.UUID{first: {ofFirst, ofAny}, second: {ofSecond, ofAny}} {
		got, err := store.GetCitationsForFactInstance(ctx, domain.MainBranchID, domain.FactPersonCensus, owner, factID)
		if err != nil {
			t.Fatalf("GetCitationsForFactInstance: %v", err)
		}
		var ids []uuid.UUID
		for _, c := range got {
			ids = append(ids, c.ID)
		}
		slices.SortFunc(ids, func(a, b uuid.UUID) int { return slices.Compare(a[:], b[:]) })
		slices.SortFunc(want, func(a, b uuid.UUID) int { return slices.Compare(a[:], b[:]) })
		if !slices.Equal(ids, want) {
			t.Errorf("citations of %s = %v, want %v", factID, ids, want)
		}
	}
}
