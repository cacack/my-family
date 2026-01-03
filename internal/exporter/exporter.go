// Package exporter provides data export capabilities for genealogy data.
// Supports JSON and CSV formats with streaming output to io.Writer.
package exporter

import (
	"context"
	"fmt"
	"io"

	"github.com/cacack/my-family/internal/repository"
)

// Format specifies the export output format.
type Format string

const (
	FormatJSON Format = "json"
	FormatCSV  Format = "csv"
)

// EntityType specifies the type of entity to export.
type EntityType string

const (
	EntityTypePersons  EntityType = "persons"
	EntityTypeFamilies EntityType = "families"
	EntityTypeAll      EntityType = "all" // JSON only: exports complete tree
)

// ExportOptions configures an export operation.
type ExportOptions struct {
	// Format specifies the output format (json or csv).
	Format Format

	// EntityType specifies what to export.
	// For CSV: must be either "persons" or "families".
	// For JSON: can also be "all" for complete tree export.
	EntityType EntityType

	// Fields specifies which fields to include (CSV only).
	// If empty, default fields are used.
	Fields []string
}

// ExportResult contains statistics from an export operation.
type ExportResult struct {
	BytesWritten     int64
	PersonsExported  int
	FamiliesExported int
}

// Exporter provides data export functionality.
type Exporter interface {
	// Export writes data to the given writer according to the options.
	Export(ctx context.Context, w io.Writer, opts ExportOptions) (*ExportResult, error)
}

// countingWriter wraps an io.Writer and counts bytes written.
type countingWriter struct {
	w     io.Writer
	count int64
}

func (cw *countingWriter) Write(p []byte) (n int, err error) {
	n, err = cw.w.Write(p)
	cw.count += int64(n)
	return n, err
}

// DataExporter implements the Exporter interface with JSON and CSV support.
type DataExporter struct {
	readStore repository.ReadModelStore
}

// NewDataExporter creates a new data exporter.
func NewDataExporter(readStore repository.ReadModelStore) *DataExporter {
	return &DataExporter{readStore: readStore}
}

// Export writes data according to the specified options.
func (e *DataExporter) Export(ctx context.Context, w io.Writer, opts ExportOptions) (*ExportResult, error) {
	switch opts.Format {
	case FormatJSON:
		return e.exportJSON(ctx, w, opts)
	case FormatCSV:
		return e.exportCSV(ctx, w, opts)
	default:
		return nil, fmt.Errorf("unsupported export format: %s", opts.Format)
	}
}
