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
	Persons    []repository.PersonReadModel    `json:"persons"`
	Families   []repository.FamilyReadModel    `json:"families"`
	Sources    []repository.SourceReadModel    `json:"sources,omitempty"`
	Citations  []repository.CitationReadModel  `json:"citations,omitempty"`
	Events     []repository.EventReadModel     `json:"events,omitempty"`
	Attributes []repository.AttributeReadModel `json:"attributes,omitempty"`
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
	case EntityTypeSources:
		return e.exportJSONSources(ctx, cw, result)
	case EntityTypeCitations:
		return e.exportJSONCitations(ctx, cw, result)
	case EntityTypeEvents:
		return e.exportJSONEvents(ctx, cw, result)
	case EntityTypeAttributes:
		return e.exportJSONAttributes(ctx, cw, result)
	default:
		return nil, fmt.Errorf("unsupported entity type for JSON export: %s", opts.EntityType)
	}
}

// exportJSONTree exports the complete tree (all entities) as JSON.
func (e *DataExporter) exportJSONTree(ctx context.Context, cw *countingWriter, result *ExportResult) (*ExportResult, error) {
	// Get all persons
	persons, err := repository.ListAll(ctx, 1000, e.readStore.ListPersons)
	if err != nil {
		return nil, fmt.Errorf("failed to list persons: %w", err)
	}

	// Get all families
	families, err := repository.ListAll(ctx, 1000, e.readStore.ListFamilies)
	if err != nil {
		return nil, fmt.Errorf("failed to list families: %w", err)
	}

	// Get all sources
	sources, err := repository.ListAll(ctx, 1000, e.readStore.ListSources)
	if err != nil {
		return nil, fmt.Errorf("failed to list sources: %w", err)
	}

	// Get all citations
	citations, err := repository.ListAll(ctx, 1000, e.readStore.ListCitations)
	if err != nil {
		return nil, fmt.Errorf("failed to list citations: %w", err)
	}

	// Get all events
	events, err := repository.ListAll(ctx, 1000, e.readStore.ListEvents)
	if err != nil {
		return nil, fmt.Errorf("failed to list events: %w", err)
	}

	// Get all attributes
	attributes, err := repository.ListAll(ctx, 1000, e.readStore.ListAttributes)
	if err != nil {
		return nil, fmt.Errorf("failed to list attributes: %w", err)
	}

	// Sort by ID for deterministic output
	sort.Slice(persons, func(i, j int) bool {
		return persons[i].ID.String() < persons[j].ID.String()
	})
	sort.Slice(families, func(i, j int) bool {
		return families[i].ID.String() < families[j].ID.String()
	})
	sort.Slice(sources, func(i, j int) bool {
		return sources[i].ID.String() < sources[j].ID.String()
	})
	sort.Slice(citations, func(i, j int) bool {
		return citations[i].ID.String() < citations[j].ID.String()
	})
	sort.Slice(events, func(i, j int) bool {
		return events[i].ID.String() < events[j].ID.String()
	})
	sort.Slice(attributes, func(i, j int) bool {
		return attributes[i].ID.String() < attributes[j].ID.String()
	})

	// Build export structure
	tree := TreeExport{
		Persons:    persons,
		Families:   families,
		Sources:    sources,
		Citations:  citations,
		Events:     events,
		Attributes: attributes,
	}

	// Encode to JSON
	encoder := json.NewEncoder(cw)
	encoder.SetIndent("", "  ")
	if err := encoder.Encode(tree); err != nil {
		return result, fmt.Errorf("failed to encode JSON: %w", err)
	}

	result.PersonsExported = len(persons)
	result.FamiliesExported = len(families)
	result.SourcesExported = len(sources)
	result.CitationsExported = len(citations)
	result.EventsExported = len(events)
	result.AttributesExported = len(attributes)
	result.BytesWritten = cw.count

	return result, nil
}

// exportJSONPersons exports only persons as a JSON array.
func (e *DataExporter) exportJSONPersons(ctx context.Context, cw *countingWriter, result *ExportResult) (*ExportResult, error) {
	persons, err := repository.ListAll(ctx, 1000, e.readStore.ListPersons)
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
	families, err := repository.ListAll(ctx, 1000, e.readStore.ListFamilies)
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

// exportJSONSources exports only sources as a JSON array.
func (e *DataExporter) exportJSONSources(ctx context.Context, cw *countingWriter, result *ExportResult) (*ExportResult, error) {
	sources, err := repository.ListAll(ctx, 1000, e.readStore.ListSources)
	if err != nil {
		return nil, fmt.Errorf("failed to list sources: %w", err)
	}

	// Sort by ID for deterministic output
	sort.Slice(sources, func(i, j int) bool {
		return sources[i].ID.String() < sources[j].ID.String()
	})

	encoder := json.NewEncoder(cw)
	encoder.SetIndent("", "  ")
	if err := encoder.Encode(sources); err != nil {
		return result, fmt.Errorf("failed to encode JSON: %w", err)
	}

	result.SourcesExported = len(sources)
	result.BytesWritten = cw.count

	return result, nil
}

// exportJSONCitations exports only citations as a JSON array.
func (e *DataExporter) exportJSONCitations(ctx context.Context, cw *countingWriter, result *ExportResult) (*ExportResult, error) {
	citations, err := repository.ListAll(ctx, 1000, e.readStore.ListCitations)
	if err != nil {
		return nil, fmt.Errorf("failed to list citations: %w", err)
	}

	// Sort by ID for deterministic output
	sort.Slice(citations, func(i, j int) bool {
		return citations[i].ID.String() < citations[j].ID.String()
	})

	encoder := json.NewEncoder(cw)
	encoder.SetIndent("", "  ")
	if err := encoder.Encode(citations); err != nil {
		return result, fmt.Errorf("failed to encode JSON: %w", err)
	}

	result.CitationsExported = len(citations)
	result.BytesWritten = cw.count

	return result, nil
}

// exportJSONEvents exports only events as a JSON array.
func (e *DataExporter) exportJSONEvents(ctx context.Context, cw *countingWriter, result *ExportResult) (*ExportResult, error) {
	events, err := repository.ListAll(ctx, 1000, e.readStore.ListEvents)
	if err != nil {
		return nil, fmt.Errorf("failed to list events: %w", err)
	}

	// Sort by ID for deterministic output
	sort.Slice(events, func(i, j int) bool {
		return events[i].ID.String() < events[j].ID.String()
	})

	encoder := json.NewEncoder(cw)
	encoder.SetIndent("", "  ")
	if err := encoder.Encode(events); err != nil {
		return result, fmt.Errorf("failed to encode JSON: %w", err)
	}

	result.EventsExported = len(events)
	result.BytesWritten = cw.count

	return result, nil
}

// exportJSONAttributes exports only attributes as a JSON array.
func (e *DataExporter) exportJSONAttributes(ctx context.Context, cw *countingWriter, result *ExportResult) (*ExportResult, error) {
	attributes, err := repository.ListAll(ctx, 1000, e.readStore.ListAttributes)
	if err != nil {
		return nil, fmt.Errorf("failed to list attributes: %w", err)
	}

	// Sort by ID for deterministic output
	sort.Slice(attributes, func(i, j int) bool {
		return attributes[i].ID.String() < attributes[j].ID.String()
	})

	encoder := json.NewEncoder(cw)
	encoder.SetIndent("", "  ")
	if err := encoder.Encode(attributes); err != nil {
		return result, fmt.Errorf("failed to encode JSON: %w", err)
	}

	result.AttributesExported = len(attributes)
	result.BytesWritten = cw.count

	return result, nil
}
