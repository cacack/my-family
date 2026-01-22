package exporter

import (
	"context"
	"encoding/csv"
	"fmt"
	"io"
	"sort"
	"strconv"

	"github.com/cacack/my-family/internal/repository"
)

// DefaultPersonFields are the default fields exported for persons.
var DefaultPersonFields = []string{
	"id",
	"given_name",
	"surname",
	"gender",
	"birth_date",
	"birth_place",
	"death_date",
	"death_place",
}

// DefaultFamilyFields are the default fields exported for families.
var DefaultFamilyFields = []string{
	"id",
	"partner1_name",
	"partner2_name",
	"relationship_type",
	"marriage_date",
	"marriage_place",
	"child_count",
}

// DefaultSourceFields are the default fields exported for sources.
var DefaultSourceFields = []string{
	"id",
	"source_type",
	"title",
	"author",
	"publisher",
	"publish_date",
	"url",
	"citation_count",
}

// DefaultCitationFields are the default fields exported for citations.
var DefaultCitationFields = []string{
	"id",
	"source_title",
	"fact_type",
	"fact_owner_id",
	"page",
	"source_quality",
	"evidence_type",
}

// DefaultEventFields are the default fields exported for events.
var DefaultEventFields = []string{
	"id",
	"owner_type",
	"owner_id",
	"fact_type",
	"date",
	"place",
	"description",
}

// DefaultAttributeFields are the default fields exported for attributes.
var DefaultAttributeFields = []string{
	"id",
	"person_id",
	"fact_type",
	"value",
	"date",
	"place",
}

// AvailablePersonFields lists all fields that can be exported for persons.
var AvailablePersonFields = map[string]bool{
	"id":          true,
	"given_name":  true,
	"surname":     true,
	"full_name":   true,
	"gender":      true,
	"birth_date":  true,
	"birth_place": true,
	"death_date":  true,
	"death_place": true,
	"notes":       true,
	"version":     true,
	"updated_at":  true,
}

// AvailableFamilyFields lists all fields that can be exported for families.
var AvailableFamilyFields = map[string]bool{
	"id":                true,
	"partner1_id":       true,
	"partner1_name":     true,
	"partner2_id":       true,
	"partner2_name":     true,
	"relationship_type": true,
	"marriage_date":     true,
	"marriage_place":    true,
	"child_count":       true,
	"version":           true,
	"updated_at":        true,
}

// AvailableSourceFields lists all fields that can be exported for sources.
var AvailableSourceFields = map[string]bool{
	"id":              true,
	"source_type":     true,
	"title":           true,
	"author":          true,
	"publisher":       true,
	"publish_date":    true,
	"url":             true,
	"repository_name": true,
	"collection_name": true,
	"call_number":     true,
	"notes":           true,
	"citation_count":  true,
	"version":         true,
	"updated_at":      true,
}

// AvailableCitationFields lists all fields that can be exported for citations.
var AvailableCitationFields = map[string]bool{
	"id":             true,
	"source_id":      true,
	"source_title":   true,
	"fact_type":      true,
	"fact_owner_id":  true,
	"page":           true,
	"volume":         true,
	"source_quality": true,
	"informant_type": true,
	"evidence_type":  true,
	"quoted_text":    true,
	"analysis":       true,
	"version":        true,
	"created_at":     true,
}

// AvailableEventFields lists all fields that can be exported for events.
var AvailableEventFields = map[string]bool{
	"id":              true,
	"owner_type":      true,
	"owner_id":        true,
	"fact_type":       true,
	"date":            true,
	"place":           true,
	"description":     true,
	"cause":           true,
	"age":             true,
	"research_status": true,
	"version":         true,
	"created_at":      true,
}

// AvailableAttributeFields lists all fields that can be exported for attributes.
var AvailableAttributeFields = map[string]bool{
	"id":         true,
	"person_id":  true,
	"fact_type":  true,
	"value":      true,
	"date":       true,
	"place":      true,
	"version":    true,
	"created_at": true,
}

// exportCSV exports data in CSV format.
func (e *DataExporter) exportCSV(ctx context.Context, w io.Writer, opts ExportOptions) (*ExportResult, error) {
	cw := &countingWriter{w: w}
	result := &ExportResult{}

	switch opts.EntityType {
	case EntityTypePersons:
		return e.exportCSVPersons(ctx, cw, result, opts.Fields)
	case EntityTypeFamilies:
		return e.exportCSVFamilies(ctx, cw, result, opts.Fields)
	case EntityTypeSources:
		return e.exportCSVSources(ctx, cw, result, opts.Fields)
	case EntityTypeCitations:
		return e.exportCSVCitations(ctx, cw, result, opts.Fields)
	case EntityTypeEvents:
		return e.exportCSVEvents(ctx, cw, result, opts.Fields)
	case EntityTypeAttributes:
		return e.exportCSVAttributes(ctx, cw, result, opts.Fields)
	case EntityTypeAll:
		return nil, fmt.Errorf("entity type 'all' is not supported for CSV export; use a specific entity type")
	default:
		return nil, fmt.Errorf("unsupported entity type for CSV export: %s", opts.EntityType)
	}
}

// validateFields checks that all requested fields are valid for the entity type.
func validateFields(fields []string, available map[string]bool) error {
	var invalid []string
	for _, f := range fields {
		if !available[f] {
			invalid = append(invalid, f)
		}
	}
	if len(invalid) > 0 {
		return fmt.Errorf("invalid fields: %v; available fields: %v", invalid, availableFieldNames(available))
	}
	return nil
}

// availableFieldNames returns the keys of a field map as a sorted slice.
func availableFieldNames(available map[string]bool) []string {
	names := make([]string, 0, len(available))
	for name := range available {
		names = append(names, name)
	}
	sort.Strings(names)
	return names
}

// exportCSVPersons exports persons to CSV format.
func (e *DataExporter) exportCSVPersons(ctx context.Context, cw *countingWriter, result *ExportResult, fields []string) (*ExportResult, error) {
	// Use default fields if none specified
	if len(fields) == 0 {
		fields = DefaultPersonFields
	}

	// Validate fields
	if err := validateFields(fields, AvailablePersonFields); err != nil {
		return nil, err
	}

	// Get all persons
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

	// Create CSV writer
	csvWriter := csv.NewWriter(cw)
	defer csvWriter.Flush()

	// Write header
	if err := csvWriter.Write(fields); err != nil {
		return result, fmt.Errorf("failed to write CSV header: %w", err)
	}

	// Write data rows
	for _, p := range persons {
		row := make([]string, len(fields))
		for i, field := range fields {
			row[i] = getPersonFieldValue(p, field)
		}
		if err := csvWriter.Write(row); err != nil {
			return result, fmt.Errorf("failed to write CSV row: %w", err)
		}
	}

	csvWriter.Flush()
	if err := csvWriter.Error(); err != nil {
		return result, fmt.Errorf("CSV write error: %w", err)
	}

	result.PersonsExported = len(persons)
	result.BytesWritten = cw.count

	return result, nil
}

// getPersonFieldValue returns the string value of a field from a PersonReadModel.
func getPersonFieldValue(p repository.PersonReadModel, field string) string {
	switch field {
	case "id":
		return p.ID.String()
	case "given_name":
		return p.GivenName
	case "surname":
		return p.Surname
	case "full_name":
		return p.FullName
	case "gender":
		return string(p.Gender)
	case "birth_date":
		return p.BirthDateRaw
	case "birth_place":
		return p.BirthPlace
	case "death_date":
		return p.DeathDateRaw
	case "death_place":
		return p.DeathPlace
	case "notes":
		return p.Notes
	case "version":
		return strconv.FormatInt(p.Version, 10)
	case "updated_at":
		return p.UpdatedAt.Format("2006-01-02T15:04:05Z07:00")
	default:
		return ""
	}
}

// exportCSVFamilies exports families to CSV format.
func (e *DataExporter) exportCSVFamilies(ctx context.Context, cw *countingWriter, result *ExportResult, fields []string) (*ExportResult, error) {
	// Use default fields if none specified
	if len(fields) == 0 {
		fields = DefaultFamilyFields
	}

	// Validate fields
	if err := validateFields(fields, AvailableFamilyFields); err != nil {
		return nil, err
	}

	// Get all families
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

	// Create CSV writer
	csvWriter := csv.NewWriter(cw)
	defer csvWriter.Flush()

	// Write header
	if err := csvWriter.Write(fields); err != nil {
		return result, fmt.Errorf("failed to write CSV header: %w", err)
	}

	// Write data rows
	for _, f := range families {
		row := make([]string, len(fields))
		for i, field := range fields {
			row[i] = getFamilyFieldValue(f, field)
		}
		if err := csvWriter.Write(row); err != nil {
			return result, fmt.Errorf("failed to write CSV row: %w", err)
		}
	}

	csvWriter.Flush()
	if err := csvWriter.Error(); err != nil {
		return result, fmt.Errorf("CSV write error: %w", err)
	}

	result.FamiliesExported = len(families)
	result.BytesWritten = cw.count

	return result, nil
}

// getFamilyFieldValue returns the string value of a field from a FamilyReadModel.
func getFamilyFieldValue(f repository.FamilyReadModel, field string) string {
	switch field {
	case "id":
		return f.ID.String()
	case "partner1_id":
		if f.Partner1ID != nil {
			return f.Partner1ID.String()
		}
		return ""
	case "partner1_name":
		return f.Partner1Name
	case "partner2_id":
		if f.Partner2ID != nil {
			return f.Partner2ID.String()
		}
		return ""
	case "partner2_name":
		return f.Partner2Name
	case "relationship_type":
		return string(f.RelationshipType)
	case "marriage_date":
		return f.MarriageDateRaw
	case "marriage_place":
		return f.MarriagePlace
	case "child_count":
		return strconv.Itoa(f.ChildCount)
	case "version":
		return strconv.FormatInt(f.Version, 10)
	case "updated_at":
		return f.UpdatedAt.Format("2006-01-02T15:04:05Z07:00")
	default:
		return ""
	}
}

// exportCSVSources exports sources to CSV format.
func (e *DataExporter) exportCSVSources(ctx context.Context, cw *countingWriter, result *ExportResult, fields []string) (*ExportResult, error) {
	if len(fields) == 0 {
		fields = DefaultSourceFields
	}

	if err := validateFields(fields, AvailableSourceFields); err != nil {
		return nil, err
	}

	sources, _, err := e.readStore.ListSources(ctx, repository.ListOptions{
		Limit: 100000,
	})
	if err != nil {
		return nil, fmt.Errorf("failed to list sources: %w", err)
	}

	sort.Slice(sources, func(i, j int) bool {
		return sources[i].ID.String() < sources[j].ID.String()
	})

	csvWriter := csv.NewWriter(cw)
	defer csvWriter.Flush()

	if err := csvWriter.Write(fields); err != nil {
		return result, fmt.Errorf("failed to write CSV header: %w", err)
	}

	for _, s := range sources {
		row := make([]string, len(fields))
		for i, field := range fields {
			row[i] = getSourceFieldValue(s, field)
		}
		if err := csvWriter.Write(row); err != nil {
			return result, fmt.Errorf("failed to write CSV row: %w", err)
		}
	}

	csvWriter.Flush()
	if err := csvWriter.Error(); err != nil {
		return result, fmt.Errorf("CSV write error: %w", err)
	}

	result.SourcesExported = len(sources)
	result.BytesWritten = cw.count

	return result, nil
}

// getSourceFieldValue returns the string value of a field from a SourceReadModel.
func getSourceFieldValue(s repository.SourceReadModel, field string) string {
	switch field {
	case "id":
		return s.ID.String()
	case "source_type":
		return string(s.SourceType)
	case "title":
		return s.Title
	case "author":
		return s.Author
	case "publisher":
		return s.Publisher
	case "publish_date":
		return s.PublishDateRaw
	case "url":
		return s.URL
	case "repository_name":
		return s.RepositoryName
	case "collection_name":
		return s.CollectionName
	case "call_number":
		return s.CallNumber
	case "notes":
		return s.Notes
	case "citation_count":
		return strconv.Itoa(s.CitationCount)
	case "version":
		return strconv.FormatInt(s.Version, 10)
	case "updated_at":
		return s.UpdatedAt.Format("2006-01-02T15:04:05Z07:00")
	default:
		return ""
	}
}

// exportCSVCitations exports citations to CSV format.
func (e *DataExporter) exportCSVCitations(ctx context.Context, cw *countingWriter, result *ExportResult, fields []string) (*ExportResult, error) {
	if len(fields) == 0 {
		fields = DefaultCitationFields
	}

	if err := validateFields(fields, AvailableCitationFields); err != nil {
		return nil, err
	}

	citations, _, err := e.readStore.ListCitations(ctx, repository.ListOptions{
		Limit: 100000,
	})
	if err != nil {
		return nil, fmt.Errorf("failed to list citations: %w", err)
	}

	sort.Slice(citations, func(i, j int) bool {
		return citations[i].ID.String() < citations[j].ID.String()
	})

	csvWriter := csv.NewWriter(cw)
	defer csvWriter.Flush()

	if err := csvWriter.Write(fields); err != nil {
		return result, fmt.Errorf("failed to write CSV header: %w", err)
	}

	for _, c := range citations {
		row := make([]string, len(fields))
		for i, field := range fields {
			row[i] = getCitationFieldValue(c, field)
		}
		if err := csvWriter.Write(row); err != nil {
			return result, fmt.Errorf("failed to write CSV row: %w", err)
		}
	}

	csvWriter.Flush()
	if err := csvWriter.Error(); err != nil {
		return result, fmt.Errorf("CSV write error: %w", err)
	}

	result.CitationsExported = len(citations)
	result.BytesWritten = cw.count

	return result, nil
}

// getCitationFieldValue returns the string value of a field from a CitationReadModel.
func getCitationFieldValue(c repository.CitationReadModel, field string) string {
	switch field {
	case "id":
		return c.ID.String()
	case "source_id":
		return c.SourceID.String()
	case "source_title":
		return c.SourceTitle
	case "fact_type":
		return string(c.FactType)
	case "fact_owner_id":
		return c.FactOwnerID.String()
	case "page":
		return c.Page
	case "volume":
		return c.Volume
	case "source_quality":
		return string(c.SourceQuality)
	case "informant_type":
		return string(c.InformantType)
	case "evidence_type":
		return string(c.EvidenceType)
	case "quoted_text":
		return c.QuotedText
	case "analysis":
		return c.Analysis
	case "version":
		return strconv.FormatInt(c.Version, 10)
	case "created_at":
		return c.CreatedAt.Format("2006-01-02T15:04:05Z07:00")
	default:
		return ""
	}
}

// exportCSVEvents exports events to CSV format.
func (e *DataExporter) exportCSVEvents(ctx context.Context, cw *countingWriter, result *ExportResult, fields []string) (*ExportResult, error) {
	if len(fields) == 0 {
		fields = DefaultEventFields
	}

	if err := validateFields(fields, AvailableEventFields); err != nil {
		return nil, err
	}

	events, _, err := e.readStore.ListEvents(ctx, repository.ListOptions{
		Limit: 100000,
	})
	if err != nil {
		return nil, fmt.Errorf("failed to list events: %w", err)
	}

	sort.Slice(events, func(i, j int) bool {
		return events[i].ID.String() < events[j].ID.String()
	})

	csvWriter := csv.NewWriter(cw)
	defer csvWriter.Flush()

	if err := csvWriter.Write(fields); err != nil {
		return result, fmt.Errorf("failed to write CSV header: %w", err)
	}

	for _, ev := range events {
		row := make([]string, len(fields))
		for i, field := range fields {
			row[i] = getEventFieldValue(ev, field)
		}
		if err := csvWriter.Write(row); err != nil {
			return result, fmt.Errorf("failed to write CSV row: %w", err)
		}
	}

	csvWriter.Flush()
	if err := csvWriter.Error(); err != nil {
		return result, fmt.Errorf("CSV write error: %w", err)
	}

	result.EventsExported = len(events)
	result.BytesWritten = cw.count

	return result, nil
}

// getEventFieldValue returns the string value of a field from an EventReadModel.
func getEventFieldValue(e repository.EventReadModel, field string) string {
	switch field {
	case "id":
		return e.ID.String()
	case "owner_type":
		return e.OwnerType
	case "owner_id":
		return e.OwnerID.String()
	case "fact_type":
		return string(e.FactType)
	case "date":
		return e.DateRaw
	case "place":
		return e.Place
	case "description":
		return e.Description
	case "cause":
		return e.Cause
	case "age":
		return e.Age
	case "research_status":
		return string(e.ResearchStatus)
	case "version":
		return strconv.FormatInt(e.Version, 10)
	case "created_at":
		return e.CreatedAt.Format("2006-01-02T15:04:05Z07:00")
	default:
		return ""
	}
}

// exportCSVAttributes exports attributes to CSV format.
func (e *DataExporter) exportCSVAttributes(ctx context.Context, cw *countingWriter, result *ExportResult, fields []string) (*ExportResult, error) {
	if len(fields) == 0 {
		fields = DefaultAttributeFields
	}

	if err := validateFields(fields, AvailableAttributeFields); err != nil {
		return nil, err
	}

	attributes, _, err := e.readStore.ListAttributes(ctx, repository.ListOptions{
		Limit: 100000,
	})
	if err != nil {
		return nil, fmt.Errorf("failed to list attributes: %w", err)
	}

	sort.Slice(attributes, func(i, j int) bool {
		return attributes[i].ID.String() < attributes[j].ID.String()
	})

	csvWriter := csv.NewWriter(cw)
	defer csvWriter.Flush()

	if err := csvWriter.Write(fields); err != nil {
		return result, fmt.Errorf("failed to write CSV header: %w", err)
	}

	for _, a := range attributes {
		row := make([]string, len(fields))
		for i, field := range fields {
			row[i] = getAttributeFieldValue(a, field)
		}
		if err := csvWriter.Write(row); err != nil {
			return result, fmt.Errorf("failed to write CSV row: %w", err)
		}
	}

	csvWriter.Flush()
	if err := csvWriter.Error(); err != nil {
		return result, fmt.Errorf("CSV write error: %w", err)
	}

	result.AttributesExported = len(attributes)
	result.BytesWritten = cw.count

	return result, nil
}

// getAttributeFieldValue returns the string value of a field from an AttributeReadModel.
func getAttributeFieldValue(a repository.AttributeReadModel, field string) string {
	switch field {
	case "id":
		return a.ID.String()
	case "person_id":
		return a.PersonID.String()
	case "fact_type":
		return string(a.FactType)
	case "value":
		return a.Value
	case "date":
		return a.DateRaw
	case "place":
		return a.Place
	case "version":
		return strconv.FormatInt(a.Version, 10)
	case "created_at":
		return a.CreatedAt.Format("2006-01-02T15:04:05Z07:00")
	default:
		return ""
	}
}
