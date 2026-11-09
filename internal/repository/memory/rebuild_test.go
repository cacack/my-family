package memory_test

import (
	"context"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/cacack/my-family/internal/domain"
	"github.com/cacack/my-family/internal/repository"
	"github.com/cacack/my-family/internal/repository/memory"
)

// TestResetReadModel_EmptiesRowsExternalIDsAndMarkers covers the reset a
// rebuild starts from (#845), including the person, family and repository
// EXIDs that Reset used to leave behind.
func TestResetReadModel_EmptiesRowsExternalIDsAndMarkers(t *testing.T) {
	ctx := context.Background()
	store := memory.NewReadModelStore()
	personID, familyID, repoID := uuid.New(), uuid.New(), uuid.New()
	if err := store.SavePerson(ctx, domain.MainBranchID, &repository.PersonReadModel{ID: personID, GivenName: "Ada", Version: 1}); err != nil {
		t.Fatal(err)
	}
	if err := store.ReplacePersonExternalIDs(ctx, domain.MainBranchID, personID, []repository.PersonExternalIDReadModel{{PersonID: personID, Value: "P"}}); err != nil {
		t.Fatal(err)
	}
	if err := store.ReplaceFamilyExternalIDs(ctx, domain.MainBranchID, familyID, []repository.FamilyExternalIDReadModel{{FamilyID: familyID, Value: "F"}}); err != nil {
		t.Fatal(err)
	}
	if err := store.ReplaceRepositoryExternalIDs(ctx, repoID, []repository.RepositoryExternalIDReadModel{{RepositoryID: repoID, Value: "R"}}); err != nil {
		t.Fatal(err)
	}
	for v := int64(1); v <= 2; v++ {
		if err := store.RecordProjectionFailure(ctx, repository.ProjectionFailure{StreamID: personID, Version: v, FailedAt: time.Now()}); err != nil {
			t.Fatal(err)
		}
	}
	if got, _ := store.ListProjectionFailures(ctx, 1); len(got) != 1 || got[0].Version != 1 {
		t.Errorf("ListProjectionFailures(1) = %+v, want the oldest", got)
	}
	if got, _ := store.ListProjectionFailures(ctx, -1); len(got) != 0 {
		t.Errorf("ListProjectionFailures(-1) = %+v, want none", got)
	}

	if err := store.ResetReadModel(ctx); err != nil {
		t.Fatalf("reset: %v", err)
	}
	if p, _ := store.GetPerson(ctx, domain.MainBranchID, personID); p != nil {
		t.Errorf("person survived reset")
	}
	if ids, _ := store.GetPersonExternalIDs(ctx, domain.MainBranchID, personID); len(ids) != 0 {
		t.Errorf("person exids survived reset: %+v", ids)
	}
	if ids, _ := store.GetFamilyExternalIDs(ctx, domain.MainBranchID, familyID); len(ids) != 0 {
		t.Errorf("family exids survived reset: %+v", ids)
	}
	if ids, _ := store.GetRepositoryExternalIDs(ctx, repoID); len(ids) != 0 {
		t.Errorf("repository exids survived reset: %+v", ids)
	}
	if got, _ := store.ListProjectionFailures(ctx, 10); len(got) != 0 {
		t.Errorf("markers survived reset: %+v", got)
	}
}
