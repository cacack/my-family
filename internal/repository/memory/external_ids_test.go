package memory_test

import (
	"context"
	"testing"

	"github.com/google/uuid"

	"github.com/cacack/my-family/internal/repository"
	"github.com/cacack/my-family/internal/repository/memory"
)

func TestMemoryPersonExternalIDs(t *testing.T) {
	store := memory.NewReadModelStore()
	ctx := context.Background()
	personID := uuid.New()

	// Empty when none stored.
	got, err := store.GetPersonExternalIDs(ctx, personID)
	if err != nil {
		t.Fatalf("GetPersonExternalIDs: %v", err)
	}
	if len(got) != 0 {
		t.Fatalf("expected no external ids, got %d", len(got))
	}

	// Replace with two identifiers; sequence should be assigned by position.
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
	if got[0].Value != "KWCJ-QN7" || got[0].Sequence != 0 || got[0].PersonID != personID {
		t.Errorf("unexpected first id: %+v", got[0])
	}
	if got[1].Value != "12345" || got[1].Sequence != 1 {
		t.Errorf("unexpected second id: %+v", got[1])
	}

	// Replacing with a single identifier drops the removed one.
	if err := store.ReplacePersonExternalIDs(ctx, personID, ids[:1]); err != nil {
		t.Fatalf("ReplacePersonExternalIDs (shrink): %v", err)
	}
	got, _ = store.GetPersonExternalIDs(ctx, personID)
	if len(got) != 1 {
		t.Fatalf("expected 1 external id after shrink, got %d", len(got))
	}

	// Empty slice clears all.
	if err := store.ReplacePersonExternalIDs(ctx, personID, nil); err != nil {
		t.Fatalf("ReplacePersonExternalIDs (clear): %v", err)
	}
	got, _ = store.GetPersonExternalIDs(ctx, personID)
	if len(got) != 0 {
		t.Fatalf("expected 0 external ids after clear, got %d", len(got))
	}
}

func TestMemoryPersonExternalIDsCascadeDelete(t *testing.T) {
	store := memory.NewReadModelStore()
	ctx := context.Background()
	personID := uuid.New()

	if err := store.SavePerson(ctx, &repository.PersonReadModel{ID: personID, GivenName: "Ada", Surname: "Lovelace", Version: 1}); err != nil {
		t.Fatalf("SavePerson: %v", err)
	}
	if err := store.ReplacePersonExternalIDs(ctx, personID, []repository.PersonExternalIDReadModel{{Value: "X1"}}); err != nil {
		t.Fatalf("ReplacePersonExternalIDs: %v", err)
	}
	if err := store.DeletePerson(ctx, personID); err != nil {
		t.Fatalf("DeletePerson: %v", err)
	}
	got, _ := store.GetPersonExternalIDs(ctx, personID)
	if len(got) != 0 {
		t.Fatalf("expected external ids cascade-deleted, got %d", len(got))
	}
}
