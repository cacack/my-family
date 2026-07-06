package postgres_test

import (
	"context"
	"testing"

	"github.com/google/uuid"

	"github.com/cacack/my-family/internal/repository"
)

func TestReadModelStore_PersonExternalIDs(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test")
	}

	store, cleanup := setupReadModelStore(t)
	defer cleanup()

	ctx := context.Background()
	personID := uuid.New()

	if err := store.SavePerson(ctx, &repository.PersonReadModel{ID: personID, GivenName: "Ada", Surname: "Lovelace", Version: 1}); err != nil {
		t.Fatalf("SavePerson: %v", err)
	}

	got, err := store.GetPersonExternalIDs(ctx, personID)
	if err != nil {
		t.Fatalf("GetPersonExternalIDs: %v", err)
	}
	if len(got) != 0 {
		t.Fatalf("expected no external ids, got %d", len(got))
	}

	ids := []repository.PersonExternalIDReadModel{
		{Value: "KWCJ-QN7", Type: "http://www.familysearch.org/ark"},
		{Value: "12345", Type: "https://www.findagrave.com/"},
	}
	if err := store.ReplacePersonExternalIDs(ctx, personID, ids); err != nil {
		t.Fatalf("ReplacePersonExternalIDs: %v", err)
	}

	got, err = store.GetPersonExternalIDs(ctx, personID)
	if err != nil {
		t.Fatalf("GetPersonExternalIDs: %v", err)
	}
	if len(got) != 2 {
		t.Fatalf("expected 2 external ids, got %d", len(got))
	}
	if got[0].Value != "KWCJ-QN7" || got[0].Sequence != 0 {
		t.Errorf("unexpected first id: %+v", got[0])
	}
	if got[1].Value != "12345" || got[1].Sequence != 1 {
		t.Errorf("unexpected second id: %+v", got[1])
	}

	// Overwrite cleanly.
	if err := store.ReplacePersonExternalIDs(ctx, personID, ids[:1]); err != nil {
		t.Fatalf("ReplacePersonExternalIDs (shrink): %v", err)
	}
	got, _ = store.GetPersonExternalIDs(ctx, personID)
	if len(got) != 1 {
		t.Fatalf("expected 1 external id after shrink, got %d", len(got))
	}

	// Cascade on person delete.
	if err := store.DeletePerson(ctx, personID); err != nil {
		t.Fatalf("DeletePerson: %v", err)
	}
	got, _ = store.GetPersonExternalIDs(ctx, personID)
	if len(got) != 0 {
		t.Fatalf("expected external ids cascade-deleted, got %d", len(got))
	}
}
