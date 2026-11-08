package postgres_test

import (
	"context"
	"database/sql"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/cacack/my-family/internal/domain"
	"github.com/cacack/my-family/internal/repository"
	pgstore "github.com/cacack/my-family/internal/repository/postgres"
)

// longMultiline returns an n-character value made of 50-character lines, the
// shape gedcom-go v3.3.0 imports from a field carrying CONT/CONC lines (#875).
func longMultiline(n int) string {
	line := strings.Repeat("x", 49) + "\n"
	return strings.Repeat(line, n/len(line)+1)[:n]
}

// assertLongMultilineEvidenceRoundTrips saves a source and a citation whose
// multi-line fields exceed the old VARCHAR caps and reads them back intact.
func assertLongMultilineEvidenceRoundTrips(t *testing.T, store *pgstore.ReadModelStore) {
	t.Helper()
	ctx := context.Background()
	main := domain.MainBranchID
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

func columnDataType(t *testing.T, db *sql.DB, table, column string) string {
	t.Helper()
	var dataType string
	if err := db.QueryRow(`
		SELECT data_type FROM information_schema.columns
		WHERE table_schema = CURRENT_SCHEMA() AND table_name = $1 AND column_name = $2`,
		table, column).Scan(&dataType); err != nil {
		t.Fatalf("inspect %s.%s: %v", table, column, err)
	}
	return dataType
}

// TestMultilineEvidenceFields_LongValues pins that a fresh schema stores
// source and citation values longer than the pre-#875 VARCHAR caps.
func TestMultilineEvidenceFields_LongValues(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test")
	}
	db, cleanup := setupPostgres(t)
	defer cleanup()
	store, err := pgstore.NewReadModelStore(db)
	if err != nil {
		t.Fatalf("create read model store: %v", err)
	}
	assertLongMultilineEvidenceRoundTrips(t, store)
}

// TestReadModelStore_WidensMultilineEvidenceColumns covers the upgrade of a
// database whose source and citation columns are the pre-#875 VARCHARs: they
// become TEXT, their rows are kept, and long multi-line values then save.
func TestReadModelStore_WidensMultilineEvidenceColumns(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test")
	}
	db, cleanup := setupPostgres(t)
	defer cleanup()

	if _, err := db.Exec(preEvidenceBranchDDL); err != nil {
		t.Fatalf("create pre-#875 evidence tables: %v", err)
	}
	sourceID := uuid.New()
	if _, err := db.Exec(`INSERT INTO sources (id, source_type, title) VALUES ($1, 'book', 'Parish Register')`, sourceID); err != nil {
		t.Fatalf("seed source: %v", err)
	}

	store, err := pgstore.NewReadModelStore(db)
	if err != nil {
		t.Fatalf("create read model store: %v", err)
	}
	for _, c := range [][2]string{
		{"sources", "title"}, {"sources", "author"}, {"sources", "publisher"},
		{"citations", "source_title"}, {"citations", "page"},
	} {
		if got := columnDataType(t, db, c[0], c[1]); got != "text" {
			t.Errorf("%s.%s is %q after migration, want text", c[0], c[1], got)
		}
	}
	src, err := store.GetSource(context.Background(), domain.MainBranchID, sourceID)
	if err != nil || src == nil || src.Title != "Parish Register" {
		t.Errorf("GetSource after migration = %+v, %v; want the seeded source", src, err)
	}
	assertLongMultilineEvidenceRoundTrips(t, store)

	if _, err := pgstore.NewReadModelStore(db); err != nil {
		t.Fatalf("reopen after migration: %v", err)
	}
}
