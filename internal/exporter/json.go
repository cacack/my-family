package exporter

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"sort"

	"github.com/cacack/my-family/internal/repository"
)

// TreeExport represents a complete family tree export in JSON format.
type TreeExport struct {
	Persons  []repository.PersonReadModel `json:"persons"`
	Families []repository.FamilyReadModel `json:"families"`
}

// exportJSON exports data in JSON format.
func (e *DataExporter) exportJSON(ctx context.Context, w io.Writer, opts ExportOptions) (*ExportResult, error) {
	cw := &countingWriter{w: w}
	result := &ExportResult{}

	switch opts.EntityType {
	case EntityTypeAll:
		return e.exportJSONTree(ctx, cw, result)
	case EntityTypePersons:
		return e.exportJSONPersons(ctx, cw, result)
	case EntityTypeFamilies:
		return e.exportJSONFamilies(ctx, cw, result)
	default:
		return nil, fmt.Errorf("unsupported entity type for JSON export: %s", opts.EntityType)
	}
}

// exportJSONTree exports the complete tree (persons + families) as JSON.
func (e *DataExporter) exportJSONTree(ctx context.Context, cw *countingWriter, result *ExportResult) (*ExportResult, error) {
	// Get all persons
	persons, _, err := e.readStore.ListPersons(ctx, repository.ListOptions{
		Limit: 100000, // Large limit to get all
	})
	if err != nil {
		return nil, fmt.Errorf("failed to list persons: %w", err)
	}

	// Get all families
	families, _, err := e.readStore.ListFamilies(ctx, repository.ListOptions{
		Limit: 100000,
	})
	if err != nil {
		return nil, fmt.Errorf("failed to list families: %w", err)
	}

	// Sort by ID for deterministic output
	sort.Slice(persons, func(i, j int) bool {
		return persons[i].ID.String() < persons[j].ID.String()
	})
	sort.Slice(families, func(i, j int) bool {
		return families[i].ID.String() < families[j].ID.String()
	})

	// Build export structure
	tree := TreeExport{
		Persons:  persons,
		Families: families,
	}

	// Encode to JSON
	encoder := json.NewEncoder(cw)
	encoder.SetIndent("", "  ")
	if err := encoder.Encode(tree); err != nil {
		return result, fmt.Errorf("failed to encode JSON: %w", err)
	}

	result.PersonsExported = len(persons)
	result.FamiliesExported = len(families)
	result.BytesWritten = cw.count

	return result, nil
}

// exportJSONPersons exports only persons as a JSON array.
func (e *DataExporter) exportJSONPersons(ctx context.Context, cw *countingWriter, result *ExportResult) (*ExportResult, error) {
	persons, _, err := e.readStore.ListPersons(ctx, repository.ListOptions{
		Limit: 100000,
	})
	if err != nil {
		return nil, fmt.Errorf("failed to list persons: %w", err)
	}

	// Sort by ID for deterministic output
	sort.Slice(persons, func(i, j int) bool {
		return persons[i].ID.String() < persons[j].ID.String()
	})

	encoder := json.NewEncoder(cw)
	encoder.SetIndent("", "  ")
	if err := encoder.Encode(persons); err != nil {
		return result, fmt.Errorf("failed to encode JSON: %w", err)
	}

	result.PersonsExported = len(persons)
	result.BytesWritten = cw.count

	return result, nil
}

// exportJSONFamilies exports only families as a JSON array.
func (e *DataExporter) exportJSONFamilies(ctx context.Context, cw *countingWriter, result *ExportResult) (*ExportResult, error) {
	families, _, err := e.readStore.ListFamilies(ctx, repository.ListOptions{
		Limit: 100000,
	})
	if err != nil {
		return nil, fmt.Errorf("failed to list families: %w", err)
	}

	// Sort by ID for deterministic output
	sort.Slice(families, func(i, j int) bool {
		return families[i].ID.String() < families[j].ID.String()
	})

	encoder := json.NewEncoder(cw)
	encoder.SetIndent("", "  ")
	if err := encoder.Encode(families); err != nil {
		return result, fmt.Errorf("failed to encode JSON: %w", err)
	}

	result.FamiliesExported = len(families)
	result.BytesWritten = cw.count

	return result, nil
}
