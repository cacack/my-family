package exporter

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"sort"

	"github.com/cacack/my-family/internal/domain"
	"github.com/cacack/my-family/internal/repository"
)

// TreeExport represents a complete family tree export in JSON format.
type TreeExport struct {
	Persons    []repository.PersonReadModel   `json:"persons"`
	Families   []repository.FamilyReadModel   `json:"families"`
	Sources    []repository.SourceReadModel   `json:"sources,omitempty"`
	Citations  []repository.CitationReadModel `json:"citations,omitempty"`
	Events     []EventExport                  `json:"events,omitempty"`
	Attributes []AttributeExport              `json:"attributes,omitempty"`
}

// EventExport is a life event as exported, with the origin of its place
// coordinates (#1563).
type EventExport struct {
	repository.EventReadModel
	PlaceOrigin *domain.CoordinateOrigin `json:"place_origin,omitempty"`
}

// AttributeExport is an attribute as exported, with the origin of its place
// coordinates (#1563).
type AttributeExport struct {
	repository.AttributeReadModel
	PlaceOrigin *domain.CoordinateOrigin `json:"place_origin,omitempty"`
}

// withPersonOrigins sets each coordinate pair's origin as read
// (domain.EffectiveCoordinateOrigin), so the export writes a pair stored
// before #1563 as imported rather than with none. The other helpers here do
// the same for families, life events and attributes; the last two record no
// origin, as only an import sets their coordinates.
func withPersonOrigins(persons []repository.PersonReadModel) {
	for i := range persons {
		p := &persons[i]
		p.BirthPlaceOrigin = domain.EffectiveCoordinateOrigin(p.BirthPlaceLat, p.BirthPlaceLong, p.BirthPlaceOrigin)
		p.DeathPlaceOrigin = domain.EffectiveCoordinateOrigin(p.DeathPlaceLat, p.DeathPlaceLong, p.DeathPlaceOrigin)
	}
}

func withFamilyOrigins(families []repository.FamilyReadModel) {
	for i := range families {
		f := &families[i]
		f.MarriagePlaceOrigin = domain.EffectiveCoordinateOrigin(f.MarriagePlaceLat, f.MarriagePlaceLong, f.MarriagePlaceOrigin)
	}
}

func eventExports(events []repository.EventReadModel) []EventExport {
	out := make([]EventExport, len(events))
	for i, e := range events {
		out[i] = EventExport{EventReadModel: e, PlaceOrigin: domain.EffectiveCoordinateOrigin(e.PlaceLat, e.PlaceLong, nil)}
	}
	return out
}

func attributeExports(attributes []repository.AttributeReadModel) []AttributeExport {
	out := make([]AttributeExport, len(attributes))
	for i, a := range attributes {
		out[i] = AttributeExport{AttributeReadModel: a, PlaceOrigin: domain.EffectiveCoordinateOrigin(a.PlaceLat, a.PlaceLong, nil)}
	}
	return out
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
	withPersonOrigins(persons)
	withFamilyOrigins(families)
	tree := TreeExport{
		Persons:    persons,
		Families:   families,
		Sources:    sources,
		Citations:  citations,
		Events:     eventExports(events),
		Attributes: attributeExports(attributes),
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
	withPersonOrigins(persons)
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
	withFamilyOrigins(families)
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
	if err := encoder.Encode(eventExports(events)); err != nil {
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
	if err := encoder.Encode(attributeExports(attributes)); err != nil {
		return result, fmt.Errorf("failed to encode JSON: %w", err)
	}

	result.AttributesExported = len(attributes)
	result.BytesWritten = cw.count

	return result, nil
}
