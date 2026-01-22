// Package query provides CQRS query services for the genealogy application.
package query

import (
	"context"

	"github.com/cacack/my-family/internal/repository"
)

// Average bytes per record type for export estimation.
// These are based on typical GEDCOM output sizes.
const (
	BytesPerPerson   = 500 // Average bytes per person record
	BytesPerFamily   = 300 // Average bytes per family record
	BytesPerSource   = 400 // Average bytes per source record
	BytesPerCitation = 200 // Average bytes per citation (inline in facts)
	BytesPerEvent    = 150 // Average bytes per event record
	BytesPerNote     = 250 // Average bytes per note record

	// Large export thresholds
	LargeExportRecordThreshold = 1000        // Records
	LargeExportByteThreshold   = 1024 * 1024 // 1MB
)

// ExportEstimate contains the estimated export size and record counts.
type ExportEstimate struct {
	PersonCount    int   `json:"person_count"`
	FamilyCount    int   `json:"family_count"`
	SourceCount    int   `json:"source_count"`
	CitationCount  int   `json:"citation_count"`
	EventCount     int   `json:"event_count"`
	NoteCount      int   `json:"note_count"`
	TotalRecords   int   `json:"total_records"`
	EstimatedBytes int64 `json:"estimated_bytes"`
	IsLargeExport  bool  `json:"is_large_export"`
}

// ExportService provides query operations for export estimation.
type ExportService struct {
	readStore repository.ReadModelStore
}

// NewExportService creates a new export query service.
func NewExportService(readStore repository.ReadModelStore) *ExportService {
	return &ExportService{readStore: readStore}
}

// GetEstimate returns an estimate of the export file size and record counts.
func (s *ExportService) GetEstimate(ctx context.Context) (*ExportEstimate, error) {
	// Get counts from read store using minimal queries (Limit: 0 returns count only)
	// We use a high limit since we need the count, but we don't need the full records
	_, personCount, err := s.readStore.ListPersons(ctx, repository.ListOptions{Limit: 1})
	if err != nil {
		return nil, err
	}

	_, familyCount, err := s.readStore.ListFamilies(ctx, repository.ListOptions{Limit: 1})
	if err != nil {
		return nil, err
	}

	_, sourceCount, err := s.readStore.ListSources(ctx, repository.ListOptions{Limit: 1})
	if err != nil {
		return nil, err
	}

	_, noteCount, err := s.readStore.ListNotes(ctx, repository.ListOptions{Limit: 1})
	if err != nil {
		return nil, err
	}

	// For citations and events, we estimate based on typical ratios
	// In GEDCOM exports, citations are embedded in facts, not separate records
	// Estimate ~2 citations per person on average
	citationCount := personCount * 2

	// Estimate ~3 events per person on average (birth, death, and one more)
	eventCount := personCount * 3

	// Calculate total records
	totalRecords := personCount + familyCount + sourceCount + noteCount

	// Calculate estimated bytes
	estimatedBytes := int64(personCount)*BytesPerPerson +
		int64(familyCount)*BytesPerFamily +
		int64(sourceCount)*BytesPerSource +
		int64(citationCount)*BytesPerCitation +
		int64(eventCount)*BytesPerEvent +
		int64(noteCount)*BytesPerNote

	// Add header/trailer overhead (~100 bytes)
	estimatedBytes += 100

	// Determine if this is a large export
	isLargeExport := totalRecords >= LargeExportRecordThreshold ||
		estimatedBytes >= LargeExportByteThreshold

	return &ExportEstimate{
		PersonCount:    personCount,
		FamilyCount:    familyCount,
		SourceCount:    sourceCount,
		CitationCount:  citationCount,
		EventCount:     eventCount,
		NoteCount:      noteCount,
		TotalRecords:   totalRecords,
		EstimatedBytes: estimatedBytes,
		IsLargeExport:  isLargeExport,
	}, nil
}
