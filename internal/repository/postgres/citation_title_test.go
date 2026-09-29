package postgres_test

import (
	"context"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/cacack/my-family/internal/domain"
	"github.com/cacack/my-family/internal/repository"
	pgstore "github.com/cacack/my-family/internal/repository/postgres"
)

// A citation projected before its source is on main has no source title.
// SaveCitation stores that empty title as NULL, and reading the row back
// must give the empty title, not a scan error (the SQLite and memory stores
// already read it back as "").
func TestCitation_EmptySourceTitleReadsBack(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test")
	}
	db, cleanup := setupPostgres(t)
	defer cleanup()
	store, err := pgstore.NewReadModelStore(db)
	if err != nil {
		t.Fatalf("create read model store: %v", err)
	}
	ctx := context.Background()
	id := uuid.New()
	sourceID := uuid.New()
	if err := store.SaveCitation(ctx, domain.MainBranchID, &repository.CitationReadModel{
		ID: id, SourceID: sourceID, FactType: domain.FactPersonBirth, FactOwnerID: uuid.New(),
		Version: 1, CreatedAt: time.Now().UTC(),
	}); err != nil {
		t.Fatalf("SaveCitation failed: %v", err)
	}
	got, err := store.GetCitation(ctx, domain.MainBranchID, id)
	if err != nil {
		t.Fatalf("GetCitation failed: %v", err)
	}
	if got == nil || got.SourceTitle != "" || got.SourceID != sourceID {
		t.Errorf("GetCitation = %+v, want the citation with an empty source title", got)
	}
	listed, err := store.GetCitationsForSource(ctx, domain.MainBranchID, sourceID)
	if err != nil || len(listed) != 1 {
		t.Errorf("GetCitationsForSource = %+v, %v; want the one citation", listed, err)
	}
}
