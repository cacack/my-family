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

// exportCSV exports data in CSV format.
func (e *DataExporter) exportCSV(ctx context.Context, w io.Writer, opts ExportOptions) (*ExportResult, error) {
	cw := &countingWriter{w: w}
	result := &ExportResult{}

	switch opts.EntityType {
	case EntityTypePersons:
		return e.exportCSVPersons(ctx, cw, result, opts.Fields)
	case EntityTypeFamilies:
		return e.exportCSVFamilies(ctx, cw, result, opts.Fields)
	case EntityTypeAll:
		return nil, fmt.Errorf("entity type 'all' is not supported for CSV export; use 'persons' or 'families'")
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
