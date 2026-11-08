package sqlite_test

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/cacack/my-family/internal/domain"
	"github.com/cacack/my-family/internal/repository"
)

// TestMultilineEvidenceFields_LongValues mirrors the PostgreSQL test of the
// same name: source and citation values that gedcom-go v3.3.0 folds from
// CONT/CONC lines (#875) save and read back intact, however long.
func TestMultilineEvidenceFields_LongValues(t *testing.T) {
	store, cleanup := setupTestReadModelDB(t)
	defer cleanup()
	ctx := context.Background()
	main := domain.MainBranchID
	line := strings.Repeat("x", 49) + "\n"
	longMultiline := func(n int) string { return strings.Repeat(line, n/len(line)+1)[:n] }
	title, author, publisher, page := longMultiline(600), longMultiline(300), longMultiline(300), longMultiline(300)

	sourceID, citationID := uuid.New(), uuid.New()
	if err := store.SaveSource(ctx, main, &repository.SourceReadModel{
		ID: sourceID, SourceType: domain.SourceBook, Title: title, Author: author, Publisher: publisher,
		Version: 1, UpdatedAt: time.Now().UTC(),
	}); err != nil {
		t.Fatalf("SaveSource with long multi-line fields: %v", err)
	}
	if err := store.SaveCitation(ctx, main, &repository.CitationReadModel{
		ID: citationID, SourceID: sourceID, SourceTitle: title, FactType: domain.FactPersonBirth,
		FactOwnerID: uuid.New(), Page: page, Version: 1, CreatedAt: time.Now().UTC(),
	}); err != nil {
		t.Fatalf("SaveCitation with a long multi-line page: %v", err)
	}

	src, err := store.GetSource(ctx, main, sourceID)
	if err != nil || src == nil {
		t.Fatalf("GetSource = %v, %v", src, err)
	}
	if src.Title != title || src.Author != author || src.Publisher != publisher {
		t.Errorf("source fields did not round-trip: title %d/%d, author %d/%d, publisher %d/%d chars",
			len(src.Title), len(title), len(src.Author), len(author), len(src.Publisher), len(publisher))
	}
	cit, err := store.GetCitation(ctx, main, citationID)
	if err != nil || cit == nil {
		t.Fatalf("GetCitation = %v, %v", cit, err)
	}
	if cit.Page != page || cit.SourceTitle != title {
		t.Errorf("citation fields did not round-trip: page %d/%d, source_title %d/%d chars",
			len(cit.Page), len(page), len(cit.SourceTitle), len(title))
	}
}
