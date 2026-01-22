package query_test

import (
	"context"
	"testing"

	"github.com/google/uuid"

	"github.com/cacack/my-family/internal/query"
	"github.com/cacack/my-family/internal/repository"
	"github.com/cacack/my-family/internal/repository/memory"
)

func TestExportService_GetEstimate_Empty(t *testing.T) {
	readStore := memory.NewReadModelStore()
	svc := query.NewExportService(readStore)
	ctx := context.Background()

	estimate, err := svc.GetEstimate(ctx)
	if err != nil {
		t.Fatalf("GetEstimate failed: %v", err)
	}

	// Empty database should return zeros
	if estimate.PersonCount != 0 {
		t.Errorf("PersonCount = %d, want 0", estimate.PersonCount)
	}
	if estimate.FamilyCount != 0 {
		t.Errorf("FamilyCount = %d, want 0", estimate.FamilyCount)
	}
	if estimate.SourceCount != 0 {
		t.Errorf("SourceCount = %d, want 0", estimate.SourceCount)
	}
	if estimate.NoteCount != 0 {
		t.Errorf("NoteCount = %d, want 0", estimate.NoteCount)
	}
	if estimate.TotalRecords != 0 {
		t.Errorf("TotalRecords = %d, want 0", estimate.TotalRecords)
	}
	// EstimatedBytes includes header overhead (100 bytes)
	if estimate.EstimatedBytes != 100 {
		t.Errorf("EstimatedBytes = %d, want 100 (header overhead)", estimate.EstimatedBytes)
	}
	if estimate.IsLargeExport {
		t.Error("IsLargeExport = true, want false for empty database")
	}
}

func TestExportService_GetEstimate_WithData(t *testing.T) {
	readStore := memory.NewReadModelStore()
	svc := query.NewExportService(readStore)
	ctx := context.Background()

	// Add test data
	for i := 0; i < 10; i++ {
		person := &repository.PersonReadModel{
			ID:        uuid.New(),
			GivenName: "Test",
			Surname:   "Person",
		}
		if err := readStore.SavePerson(ctx, person); err != nil {
			t.Fatalf("SavePerson failed: %v", err)
		}
	}

	for i := 0; i < 5; i++ {
		family := &repository.FamilyReadModel{
			ID: uuid.New(),
		}
		if err := readStore.SaveFamily(ctx, family); err != nil {
			t.Fatalf("SaveFamily failed: %v", err)
		}
	}

	for i := 0; i < 3; i++ {
		source := &repository.SourceReadModel{
			ID:    uuid.New(),
			Title: "Test Source",
		}
		if err := readStore.SaveSource(ctx, source); err != nil {
			t.Fatalf("SaveSource failed: %v", err)
		}
	}

	for i := 0; i < 2; i++ {
		note := &repository.NoteReadModel{
			ID:   uuid.New(),
			Text: "Test note",
		}
		if err := readStore.SaveNote(ctx, note); err != nil {
			t.Fatalf("SaveNote failed: %v", err)
		}
	}

	estimate, err := svc.GetEstimate(ctx)
	if err != nil {
		t.Fatalf("GetEstimate failed: %v", err)
	}

	// Verify counts
	if estimate.PersonCount != 10 {
		t.Errorf("PersonCount = %d, want 10", estimate.PersonCount)
	}
	if estimate.FamilyCount != 5 {
		t.Errorf("FamilyCount = %d, want 5", estimate.FamilyCount)
	}
	if estimate.SourceCount != 3 {
		t.Errorf("SourceCount = %d, want 3", estimate.SourceCount)
	}
	if estimate.NoteCount != 2 {
		t.Errorf("NoteCount = %d, want 2", estimate.NoteCount)
	}

	// Total records = 10 + 5 + 3 + 2 = 20
	if estimate.TotalRecords != 20 {
		t.Errorf("TotalRecords = %d, want 20", estimate.TotalRecords)
	}

	// Citation count is estimated as 2x persons = 20
	if estimate.CitationCount != 20 {
		t.Errorf("CitationCount = %d, want 20", estimate.CitationCount)
	}

	// Event count is estimated as 3x persons = 30
	if estimate.EventCount != 30 {
		t.Errorf("EventCount = %d, want 30", estimate.EventCount)
	}

	// Estimated bytes should be > 0
	if estimate.EstimatedBytes <= 0 {
		t.Errorf("EstimatedBytes = %d, want > 0", estimate.EstimatedBytes)
	}

	// With 20 records (< 1000), should not be large export
	if estimate.IsLargeExport {
		t.Error("IsLargeExport = true, want false for small database")
	}
}

func TestExportService_GetEstimate_LargeExport_ByRecordCount(t *testing.T) {
	readStore := memory.NewReadModelStore()
	svc := query.NewExportService(readStore)
	ctx := context.Background()

	// Add enough records to exceed the large export threshold (1000 records)
	for i := 0; i < 1000; i++ {
		person := &repository.PersonReadModel{
			ID:        uuid.New(),
			GivenName: "Test",
			Surname:   "Person",
		}
		if err := readStore.SavePerson(ctx, person); err != nil {
			t.Fatalf("SavePerson failed: %v", err)
		}
	}

	estimate, err := svc.GetEstimate(ctx)
	if err != nil {
		t.Fatalf("GetEstimate failed: %v", err)
	}

	if !estimate.IsLargeExport {
		t.Error("IsLargeExport = false, want true for >= 1000 records")
	}
}

func TestExportService_GetEstimate_ByteCalculation(t *testing.T) {
	readStore := memory.NewReadModelStore()
	svc := query.NewExportService(readStore)
	ctx := context.Background()

	// Add 1 of each type
	person := &repository.PersonReadModel{ID: uuid.New(), GivenName: "Test", Surname: "Person"}
	family := &repository.FamilyReadModel{ID: uuid.New()}
	source := &repository.SourceReadModel{ID: uuid.New(), Title: "Source"}
	note := &repository.NoteReadModel{ID: uuid.New(), Text: "Note"}

	_ = readStore.SavePerson(ctx, person)
	_ = readStore.SaveFamily(ctx, family)
	_ = readStore.SaveSource(ctx, source)
	_ = readStore.SaveNote(ctx, note)

	estimate, err := svc.GetEstimate(ctx)
	if err != nil {
		t.Fatalf("GetEstimate failed: %v", err)
	}

	// Expected bytes calculation:
	// 1 person * 500 = 500
	// 1 family * 300 = 300
	// 1 source * 400 = 400
	// 2 citations (1 person * 2) * 200 = 400
	// 3 events (1 person * 3) * 150 = 450
	// 1 note * 250 = 250
	// + 100 overhead = 2400
	expectedBytes := int64(500 + 300 + 400 + 400 + 450 + 250 + 100)
	if estimate.EstimatedBytes != expectedBytes {
		t.Errorf("EstimatedBytes = %d, want %d", estimate.EstimatedBytes, expectedBytes)
	}
}
