package sqlite_test

import (
	"context"
	"testing"

	"github.com/google/uuid"

	"github.com/cacack/my-family/internal/domain"
	"github.com/cacack/my-family/internal/repository"
)

func TestSQLitePersonExternalIDs(t *testing.T) {
	store, cleanup := setupTestReadModelDB(t)
	defer cleanup()

	ctx := context.Background()
	personID := uuid.New()

	if err := store.SavePerson(ctx, domain.MainBranchID, &repository.PersonReadModel{ID: personID, GivenName: "Ada", Surname: "Lovelace", Version: 1}); err != nil {
		t.Fatalf("SavePerson: %v", err)
	}

	// Empty initially.
	got, err := store.GetPersonExternalIDs(ctx, domain.MainBranchID, personID)
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
	if err := store.ReplacePersonExternalIDs(ctx, domain.MainBranchID, personID, ids); err != nil {
		t.Fatalf("ReplacePersonExternalIDs: %v", err)
	}

	got, err = store.GetPersonExternalIDs(ctx, domain.MainBranchID, personID)
	if err != nil {
		t.Fatalf("GetPersonExternalIDs: %v", err)
	}
	if len(got) != 2 {
		t.Fatalf("expected 2 external ids, got %d", len(got))
	}
	if got[0].Value != "KWCJ-QN7" || got[0].Type != "http://www.familysearch.org/ark" || got[0].Sequence != 0 {
		t.Errorf("unexpected first id: %+v", got[0])
	}
	if got[1].Value != "12345" || got[1].Sequence != 1 {
		t.Errorf("unexpected second id: %+v", got[1])
	}

	// Replace is idempotent / overwrites cleanly.
	if err := store.ReplacePersonExternalIDs(ctx, domain.MainBranchID, personID, ids[:1]); err != nil {
		t.Fatalf("ReplacePersonExternalIDs (shrink): %v", err)
	}
	got, _ = store.GetPersonExternalIDs(ctx, domain.MainBranchID, personID)
	if len(got) != 1 {
		t.Fatalf("expected 1 external id after shrink, got %d", len(got))
	}

	// Deleting the person cascades to external ids.
	if err := store.DeletePerson(ctx, domain.MainBranchID, personID); err != nil {
		t.Fatalf("DeletePerson: %v", err)
	}
	got, _ = store.GetPersonExternalIDs(ctx, domain.MainBranchID, personID)
	if len(got) != 0 {
		t.Fatalf("expected external ids cascade-deleted, got %d", len(got))
	}
}

func TestSQLiteFamilyExternalIDs(t *testing.T) {
	store, cleanup := setupTestReadModelDB(t)
	defer cleanup()
	ctx := context.Background()
	familyID := uuid.New()
	if err := store.SaveFamily(ctx, domain.MainBranchID, &repository.FamilyReadModel{ID: familyID, RelationshipType: domain.RelationMarriage, Version: 1}); err != nil {
		t.Fatalf("SaveFamily: %v", err)
	}

	ids := []repository.FamilyExternalIDReadModel{
		{Value: "F-1", Type: "http://example.com/fam"},
		{Value: "F-2"},
	}
	if err := store.ReplaceFamilyExternalIDs(ctx, domain.MainBranchID, familyID, ids); err != nil {
		t.Fatalf("ReplaceFamilyExternalIDs: %v", err)
	}
	got, _ := store.GetFamilyExternalIDs(ctx, domain.MainBranchID, familyID)
	if len(got) != 2 || got[0].Value != "F-1" || got[0].Sequence != 0 || got[0].FamilyID != familyID || got[1].Value != "F-2" || got[1].Sequence != 1 {
		t.Fatalf("unexpected external ids: %+v", got)
	}

	if err := store.DeleteFamily(ctx, domain.MainBranchID, familyID); err != nil {
		t.Fatalf("DeleteFamily: %v", err)
	}
	got, _ = store.GetFamilyExternalIDs(ctx, domain.MainBranchID, familyID)
	if len(got) != 0 {
		t.Fatalf("expected external ids cascade-deleted, got %d", len(got))
	}
}

func TestSQLiteSourceExternalIDs(t *testing.T) {
	store, cleanup := setupTestReadModelDB(t)
	defer cleanup()
	ctx := context.Background()
	sourceID := uuid.New()
	if err := store.SaveSource(ctx, &repository.SourceReadModel{ID: sourceID, SourceType: domain.SourceBook, Title: "Test Source", Version: 1}); err != nil {
		t.Fatalf("SaveSource: %v", err)
	}

	ids := []repository.SourceExternalIDReadModel{
		{Value: "S-1", Type: "http://example.com/src"},
		{Value: "S-2"},
	}
	if err := store.ReplaceSourceExternalIDs(ctx, sourceID, ids); err != nil {
		t.Fatalf("ReplaceSourceExternalIDs: %v", err)
	}
	got, _ := store.GetSourceExternalIDs(ctx, sourceID)
	if len(got) != 2 || got[0].Value != "S-1" || got[0].Sequence != 0 || got[0].SourceID != sourceID || got[1].Value != "S-2" || got[1].Sequence != 1 {
		t.Fatalf("unexpected external ids: %+v", got)
	}

	if err := store.DeleteSource(ctx, sourceID); err != nil {
		t.Fatalf("DeleteSource: %v", err)
	}
	got, _ = store.GetSourceExternalIDs(ctx, sourceID)
	if len(got) != 0 {
		t.Fatalf("expected external ids cascade-deleted, got %d", len(got))
	}
}

func TestSQLiteRepositoryExternalIDs(t *testing.T) {
	store, cleanup := setupTestReadModelDB(t)
	defer cleanup()
	ctx := context.Background()
	repoID := uuid.New()
	if err := store.SaveRepository(ctx, &repository.RepositoryReadModel{ID: repoID, Name: "Test Repo", Version: 1}); err != nil {
		t.Fatalf("SaveRepository: %v", err)
	}

	ids := []repository.RepositoryExternalIDReadModel{
		{Value: "R-1", Type: "http://example.com/repo"},
		{Value: "R-2"},
	}
	if err := store.ReplaceRepositoryExternalIDs(ctx, repoID, ids); err != nil {
		t.Fatalf("ReplaceRepositoryExternalIDs: %v", err)
	}
	got, _ := store.GetRepositoryExternalIDs(ctx, repoID)
	if len(got) != 2 || got[0].Value != "R-1" || got[0].Sequence != 0 || got[0].RepositoryID != repoID || got[1].Value != "R-2" || got[1].Sequence != 1 {
		t.Fatalf("unexpected external ids: %+v", got)
	}

	if err := store.DeleteRepository(ctx, repoID); err != nil {
		t.Fatalf("DeleteRepository: %v", err)
	}
	got, _ = store.GetRepositoryExternalIDs(ctx, repoID)
	if len(got) != 0 {
		t.Fatalf("expected external ids cascade-deleted, got %d", len(got))
	}
}
