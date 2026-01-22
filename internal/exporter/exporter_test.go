package exporter_test

import (
	"bytes"
	"context"
	"encoding/csv"
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/cacack/my-family/internal/domain"
	"github.com/cacack/my-family/internal/exporter"
	"github.com/cacack/my-family/internal/repository"
	"github.com/cacack/my-family/internal/repository/memory"
)

// Test helpers

func setupTestStore(t *testing.T) *memory.ReadModelStore {
	t.Helper()
	return memory.NewReadModelStore()
}

func createTestPerson(t *testing.T, store *memory.ReadModelStore, givenName, surname string, gender domain.Gender) repository.PersonReadModel {
	t.Helper()
	now := time.Now()
	person := repository.PersonReadModel{
		ID:           uuid.New(),
		GivenName:    givenName,
		Surname:      surname,
		FullName:     givenName + " " + surname,
		Gender:       gender,
		BirthDateRaw: "15 JAN 1850",
		BirthPlace:   "Springfield, IL",
		DeathDateRaw: "20 MAR 1920",
		DeathPlace:   "Chicago, IL",
		Notes:        "Test person",
		Version:      1,
		UpdatedAt:    now,
	}
	err := store.SavePerson(context.Background(), &person)
	require.NoError(t, err)
	return person
}

func createTestFamily(t *testing.T, store *memory.ReadModelStore, partner1, partner2 *repository.PersonReadModel) repository.FamilyReadModel {
	t.Helper()
	now := time.Now()
	family := repository.FamilyReadModel{
		ID:               uuid.New(),
		RelationshipType: domain.RelationMarriage,
		MarriageDateRaw:  "10 JUN 1875",
		MarriagePlace:    "Springfield, IL",
		ChildCount:       0,
		Version:          1,
		UpdatedAt:        now,
	}
	if partner1 != nil {
		family.Partner1ID = &partner1.ID
		family.Partner1Name = partner1.FullName
	}
	if partner2 != nil {
		family.Partner2ID = &partner2.ID
		family.Partner2Name = partner2.FullName
	}
	err := store.SaveFamily(context.Background(), &family)
	require.NoError(t, err)
	return family
}

// JSON Exporter Tests

func TestJSONExporter_ExportTree_Empty(t *testing.T) {
	store := setupTestStore(t)
	exp := exporter.NewDataExporter(store)

	var buf bytes.Buffer
	result, err := exp.Export(context.Background(), &buf, exporter.ExportOptions{
		Format:     exporter.FormatJSON,
		EntityType: exporter.EntityTypeAll,
	})

	require.NoError(t, err)
	assert.Equal(t, 0, result.PersonsExported)
	assert.Equal(t, 0, result.FamiliesExported)
	assert.Greater(t, result.BytesWritten, int64(0))

	// Verify valid JSON structure
	var data exporter.TreeExport
	err = json.Unmarshal(buf.Bytes(), &data)
	require.NoError(t, err)
	assert.Empty(t, data.Persons)
	assert.Empty(t, data.Families)
}

func TestJSONExporter_ExportTree_WithData(t *testing.T) {
	store := setupTestStore(t)

	// Create test data
	person1 := createTestPerson(t, store, "John", "Doe", domain.GenderMale)
	person2 := createTestPerson(t, store, "Jane", "Smith", domain.GenderFemale)
	family := createTestFamily(t, store, &person1, &person2)

	exp := exporter.NewDataExporter(store)

	var buf bytes.Buffer
	result, err := exp.Export(context.Background(), &buf, exporter.ExportOptions{
		Format:     exporter.FormatJSON,
		EntityType: exporter.EntityTypeAll,
	})

	require.NoError(t, err)
	assert.Equal(t, 2, result.PersonsExported)
	assert.Equal(t, 1, result.FamiliesExported)
	assert.Greater(t, result.BytesWritten, int64(0))

	// Verify valid JSON with all data
	var data exporter.TreeExport
	err = json.Unmarshal(buf.Bytes(), &data)
	require.NoError(t, err)
	assert.Len(t, data.Persons, 2)
	assert.Len(t, data.Families, 1)

	// Check person data is included
	personNames := make([]string, len(data.Persons))
	for i, p := range data.Persons {
		personNames[i] = p.FullName
	}
	assert.Contains(t, personNames, "John Doe")
	assert.Contains(t, personNames, "Jane Smith")

	// Check family data is included
	assert.Equal(t, family.ID.String(), data.Families[0].ID.String())
}

func TestJSONExporter_ExportPersons_Empty(t *testing.T) {
	store := setupTestStore(t)
	exp := exporter.NewDataExporter(store)

	var buf bytes.Buffer
	result, err := exp.Export(context.Background(), &buf, exporter.ExportOptions{
		Format:     exporter.FormatJSON,
		EntityType: exporter.EntityTypePersons,
	})

	require.NoError(t, err)
	assert.Equal(t, 0, result.PersonsExported)

	// Verify valid JSON array
	var data []repository.PersonReadModel
	err = json.Unmarshal(buf.Bytes(), &data)
	require.NoError(t, err)
	assert.Empty(t, data)
}

func TestJSONExporter_ExportPersons_WithData(t *testing.T) {
	store := setupTestStore(t)

	// Create test persons
	person1 := createTestPerson(t, store, "John", "Doe", domain.GenderMale)
	person2 := createTestPerson(t, store, "Jane", "Smith", domain.GenderFemale)
	_ = person1
	_ = person2

	exp := exporter.NewDataExporter(store)

	var buf bytes.Buffer
	result, err := exp.Export(context.Background(), &buf, exporter.ExportOptions{
		Format:     exporter.FormatJSON,
		EntityType: exporter.EntityTypePersons,
	})

	require.NoError(t, err)
	assert.Equal(t, 2, result.PersonsExported)
	assert.Equal(t, 0, result.FamiliesExported)

	// Verify valid JSON array
	var data []repository.PersonReadModel
	err = json.Unmarshal(buf.Bytes(), &data)
	require.NoError(t, err)
	assert.Len(t, data, 2)
}

func TestJSONExporter_ExportFamilies_Empty(t *testing.T) {
	store := setupTestStore(t)
	exp := exporter.NewDataExporter(store)

	var buf bytes.Buffer
	result, err := exp.Export(context.Background(), &buf, exporter.ExportOptions{
		Format:     exporter.FormatJSON,
		EntityType: exporter.EntityTypeFamilies,
	})

	require.NoError(t, err)
	assert.Equal(t, 0, result.FamiliesExported)

	// Verify valid JSON array
	var data []repository.FamilyReadModel
	err = json.Unmarshal(buf.Bytes(), &data)
	require.NoError(t, err)
	assert.Empty(t, data)
}

func TestJSONExporter_ExportFamilies_WithData(t *testing.T) {
	store := setupTestStore(t)

	// Create test data
	person1 := createTestPerson(t, store, "John", "Doe", domain.GenderMale)
	person2 := createTestPerson(t, store, "Jane", "Smith", domain.GenderFemale)
	_ = createTestFamily(t, store, &person1, &person2)
	_ = createTestFamily(t, store, nil, nil) // Empty family

	exp := exporter.NewDataExporter(store)

	var buf bytes.Buffer
	result, err := exp.Export(context.Background(), &buf, exporter.ExportOptions{
		Format:     exporter.FormatJSON,
		EntityType: exporter.EntityTypeFamilies,
	})

	require.NoError(t, err)
	assert.Equal(t, 0, result.PersonsExported)
	assert.Equal(t, 2, result.FamiliesExported)

	// Verify valid JSON array
	var data []repository.FamilyReadModel
	err = json.Unmarshal(buf.Bytes(), &data)
	require.NoError(t, err)
	assert.Len(t, data, 2)
}

func TestJSONExporter_InvalidEntityType(t *testing.T) {
	store := setupTestStore(t)
	exp := exporter.NewDataExporter(store)

	var buf bytes.Buffer
	_, err := exp.Export(context.Background(), &buf, exporter.ExportOptions{
		Format:     exporter.FormatJSON,
		EntityType: "invalid",
	})

	require.Error(t, err)
	assert.Contains(t, err.Error(), "unsupported entity type")
}

func TestCSVExporter_InvalidEntityType(t *testing.T) {
	store := setupTestStore(t)
	exp := exporter.NewDataExporter(store)

	var buf bytes.Buffer
	_, err := exp.Export(context.Background(), &buf, exporter.ExportOptions{
		Format:     exporter.FormatCSV,
		EntityType: "invalid",
	})

	require.Error(t, err)
	assert.Contains(t, err.Error(), "unsupported entity type")
}

// CSV Exporter Tests

func TestCSVExporter_ExportPersons_Empty(t *testing.T) {
	store := setupTestStore(t)
	exp := exporter.NewDataExporter(store)

	var buf bytes.Buffer
	result, err := exp.Export(context.Background(), &buf, exporter.ExportOptions{
		Format:     exporter.FormatCSV,
		EntityType: exporter.EntityTypePersons,
	})

	require.NoError(t, err)
	assert.Equal(t, 0, result.PersonsExported)
	assert.Greater(t, result.BytesWritten, int64(0)) // Headers still written

	// Verify valid CSV with headers only
	reader := csv.NewReader(strings.NewReader(buf.String()))
	records, err := reader.ReadAll()
	require.NoError(t, err)
	assert.Len(t, records, 1) // Headers only
	assert.Equal(t, exporter.DefaultPersonFields, records[0])
}

func TestCSVExporter_ExportPersons_WithData(t *testing.T) {
	store := setupTestStore(t)

	// Create test persons
	_ = createTestPerson(t, store, "John", "Doe", domain.GenderMale)
	_ = createTestPerson(t, store, "Jane", "Smith", domain.GenderFemale)

	exp := exporter.NewDataExporter(store)

	var buf bytes.Buffer
	result, err := exp.Export(context.Background(), &buf, exporter.ExportOptions{
		Format:     exporter.FormatCSV,
		EntityType: exporter.EntityTypePersons,
	})

	require.NoError(t, err)
	assert.Equal(t, 2, result.PersonsExported)

	// Verify valid CSV
	reader := csv.NewReader(strings.NewReader(buf.String()))
	records, err := reader.ReadAll()
	require.NoError(t, err)
	assert.Len(t, records, 3) // Header + 2 data rows

	// Check headers are default fields
	assert.Equal(t, exporter.DefaultPersonFields, records[0])

	// Check data contains expected values
	allData := buf.String()
	assert.Contains(t, allData, "John")
	assert.Contains(t, allData, "Doe")
	assert.Contains(t, allData, "Jane")
	assert.Contains(t, allData, "Smith")
}

func TestCSVExporter_ExportPersons_CustomFields(t *testing.T) {
	store := setupTestStore(t)

	// Create test person
	_ = createTestPerson(t, store, "John", "Doe", domain.GenderMale)

	exp := exporter.NewDataExporter(store)

	var buf bytes.Buffer
	customFields := []string{"id", "surname", "given_name"}
	result, err := exp.Export(context.Background(), &buf, exporter.ExportOptions{
		Format:     exporter.FormatCSV,
		EntityType: exporter.EntityTypePersons,
		Fields:     customFields,
	})

	require.NoError(t, err)
	assert.Equal(t, 1, result.PersonsExported)

	// Verify CSV has only custom fields
	reader := csv.NewReader(strings.NewReader(buf.String()))
	records, err := reader.ReadAll()
	require.NoError(t, err)
	assert.Len(t, records, 2) // Header + 1 data row

	// Check headers match custom fields
	assert.Equal(t, customFields, records[0])

	// Check row has only 3 columns
	assert.Len(t, records[1], 3)
}

func TestCSVExporter_ExportPersons_InvalidField(t *testing.T) {
	store := setupTestStore(t)
	exp := exporter.NewDataExporter(store)

	var buf bytes.Buffer
	_, err := exp.Export(context.Background(), &buf, exporter.ExportOptions{
		Format:     exporter.FormatCSV,
		EntityType: exporter.EntityTypePersons,
		Fields:     []string{"id", "invalid_field"},
	})

	require.Error(t, err)
	assert.Contains(t, err.Error(), "invalid fields")
	assert.Contains(t, err.Error(), "invalid_field")
}

func TestCSVExporter_ExportFamilies_Empty(t *testing.T) {
	store := setupTestStore(t)
	exp := exporter.NewDataExporter(store)

	var buf bytes.Buffer
	result, err := exp.Export(context.Background(), &buf, exporter.ExportOptions{
		Format:     exporter.FormatCSV,
		EntityType: exporter.EntityTypeFamilies,
	})

	require.NoError(t, err)
	assert.Equal(t, 0, result.FamiliesExported)

	// Verify valid CSV with headers only
	reader := csv.NewReader(strings.NewReader(buf.String()))
	records, err := reader.ReadAll()
	require.NoError(t, err)
	assert.Len(t, records, 1) // Headers only
	assert.Equal(t, exporter.DefaultFamilyFields, records[0])
}

func TestCSVExporter_ExportFamilies_WithData(t *testing.T) {
	store := setupTestStore(t)

	// Create test data
	person1 := createTestPerson(t, store, "John", "Doe", domain.GenderMale)
	person2 := createTestPerson(t, store, "Jane", "Smith", domain.GenderFemale)
	_ = createTestFamily(t, store, &person1, &person2)

	exp := exporter.NewDataExporter(store)

	var buf bytes.Buffer
	result, err := exp.Export(context.Background(), &buf, exporter.ExportOptions{
		Format:     exporter.FormatCSV,
		EntityType: exporter.EntityTypeFamilies,
	})

	require.NoError(t, err)
	assert.Equal(t, 1, result.FamiliesExported)

	// Verify valid CSV
	reader := csv.NewReader(strings.NewReader(buf.String()))
	records, err := reader.ReadAll()
	require.NoError(t, err)
	assert.Len(t, records, 2) // Header + 1 data row

	// Check data contains expected values
	allData := buf.String()
	assert.Contains(t, allData, "John Doe")
	assert.Contains(t, allData, "Jane Smith")
	assert.Contains(t, allData, "marriage")
}

func TestCSVExporter_ExportFamilies_CustomFields(t *testing.T) {
	store := setupTestStore(t)

	// Create test data
	person1 := createTestPerson(t, store, "John", "Doe", domain.GenderMale)
	person2 := createTestPerson(t, store, "Jane", "Smith", domain.GenderFemale)
	_ = createTestFamily(t, store, &person1, &person2)

	exp := exporter.NewDataExporter(store)

	var buf bytes.Buffer
	customFields := []string{"id", "partner1_name", "partner2_name"}
	result, err := exp.Export(context.Background(), &buf, exporter.ExportOptions{
		Format:     exporter.FormatCSV,
		EntityType: exporter.EntityTypeFamilies,
		Fields:     customFields,
	})

	require.NoError(t, err)
	assert.Equal(t, 1, result.FamiliesExported)

	// Verify CSV has only custom fields
	reader := csv.NewReader(strings.NewReader(buf.String()))
	records, err := reader.ReadAll()
	require.NoError(t, err)
	assert.Len(t, records, 2) // Header + 1 data row

	// Check headers match custom fields
	assert.Equal(t, customFields, records[0])

	// Check row has only 3 columns
	assert.Len(t, records[1], 3)
}

func TestCSVExporter_ExportFamilies_InvalidField(t *testing.T) {
	store := setupTestStore(t)
	exp := exporter.NewDataExporter(store)

	var buf bytes.Buffer
	_, err := exp.Export(context.Background(), &buf, exporter.ExportOptions{
		Format:     exporter.FormatCSV,
		EntityType: exporter.EntityTypeFamilies,
		Fields:     []string{"id", "nonexistent_field"},
	})

	require.Error(t, err)
	assert.Contains(t, err.Error(), "invalid fields")
	assert.Contains(t, err.Error(), "nonexistent_field")
}

func TestCSVExporter_EntityTypeAll_NotSupported(t *testing.T) {
	store := setupTestStore(t)
	exp := exporter.NewDataExporter(store)

	var buf bytes.Buffer
	_, err := exp.Export(context.Background(), &buf, exporter.ExportOptions{
		Format:     exporter.FormatCSV,
		EntityType: exporter.EntityTypeAll,
	})

	require.Error(t, err)
	assert.Contains(t, err.Error(), "not supported for CSV export")
}

// Format Tests

func TestExporter_UnsupportedFormat(t *testing.T) {
	store := setupTestStore(t)
	exp := exporter.NewDataExporter(store)

	var buf bytes.Buffer
	_, err := exp.Export(context.Background(), &buf, exporter.ExportOptions{
		Format:     "xml",
		EntityType: exporter.EntityTypeAll,
	})

	require.Error(t, err)
	assert.Contains(t, err.Error(), "unsupported export format")
}

// Field Value Tests

func TestCSVExporter_AllPersonFieldValues(t *testing.T) {
	store := setupTestStore(t)
	now := time.Now()

	// Create person with all fields populated
	person := repository.PersonReadModel{
		ID:           uuid.New(),
		GivenName:    "John",
		Surname:      "Doe",
		FullName:     "John Doe",
		Gender:       domain.GenderMale,
		BirthDateRaw: "15 JAN 1850",
		BirthPlace:   "Springfield, IL",
		DeathDateRaw: "20 MAR 1920",
		DeathPlace:   "Chicago, IL",
		Notes:        "Test notes",
		Version:      5,
		UpdatedAt:    now,
	}
	err := store.SavePerson(context.Background(), &person)
	require.NoError(t, err)

	exp := exporter.NewDataExporter(store)

	// Test all available person fields
	allFields := []string{
		"id", "given_name", "surname", "full_name", "gender",
		"birth_date", "birth_place", "death_date", "death_place",
		"notes", "version", "updated_at",
	}

	var buf bytes.Buffer
	result, err := exp.Export(context.Background(), &buf, exporter.ExportOptions{
		Format:     exporter.FormatCSV,
		EntityType: exporter.EntityTypePersons,
		Fields:     allFields,
	})

	require.NoError(t, err)
	assert.Equal(t, 1, result.PersonsExported)

	// Parse and verify
	reader := csv.NewReader(strings.NewReader(buf.String()))
	records, err := reader.ReadAll()
	require.NoError(t, err)
	assert.Len(t, records, 2)

	// Check all values are present
	row := records[1]
	assert.Equal(t, person.ID.String(), row[0]) // id
	assert.Equal(t, "John", row[1])             // given_name
	assert.Equal(t, "Doe", row[2])              // surname
	assert.Equal(t, "John Doe", row[3])         // full_name
	assert.Equal(t, "male", row[4])             // gender
	assert.Equal(t, "15 JAN 1850", row[5])      // birth_date
	assert.Equal(t, "Springfield, IL", row[6])  // birth_place
	assert.Equal(t, "20 MAR 1920", row[7])      // death_date
	assert.Equal(t, "Chicago, IL", row[8])      // death_place
	assert.Equal(t, "Test notes", row[9])       // notes
	assert.Equal(t, "5", row[10])               // version
	assert.NotEmpty(t, row[11])                 // updated_at
}

func TestCSVExporter_AllFamilyFieldValues(t *testing.T) {
	store := setupTestStore(t)
	now := time.Now()

	// Create persons
	person1 := repository.PersonReadModel{
		ID:        uuid.New(),
		GivenName: "John",
		Surname:   "Doe",
		FullName:  "John Doe",
		Version:   1,
		UpdatedAt: now,
	}
	person2 := repository.PersonReadModel{
		ID:        uuid.New(),
		GivenName: "Jane",
		Surname:   "Smith",
		FullName:  "Jane Smith",
		Version:   1,
		UpdatedAt: now,
	}
	err := store.SavePerson(context.Background(), &person1)
	require.NoError(t, err)
	err = store.SavePerson(context.Background(), &person2)
	require.NoError(t, err)

	// Create family with all fields populated
	family := repository.FamilyReadModel{
		ID:               uuid.New(),
		Partner1ID:       &person1.ID,
		Partner1Name:     person1.FullName,
		Partner2ID:       &person2.ID,
		Partner2Name:     person2.FullName,
		RelationshipType: domain.RelationMarriage,
		MarriageDateRaw:  "10 JUN 1875",
		MarriagePlace:    "Springfield, IL",
		ChildCount:       3,
		Version:          2,
		UpdatedAt:        now,
	}
	err = store.SaveFamily(context.Background(), &family)
	require.NoError(t, err)

	exp := exporter.NewDataExporter(store)

	// Test all available family fields
	allFields := []string{
		"id", "partner1_id", "partner1_name", "partner2_id", "partner2_name",
		"relationship_type", "marriage_date", "marriage_place", "child_count",
		"version", "updated_at",
	}

	var buf bytes.Buffer
	result, err := exp.Export(context.Background(), &buf, exporter.ExportOptions{
		Format:     exporter.FormatCSV,
		EntityType: exporter.EntityTypeFamilies,
		Fields:     allFields,
	})

	require.NoError(t, err)
	assert.Equal(t, 1, result.FamiliesExported)

	// Parse and verify
	reader := csv.NewReader(strings.NewReader(buf.String()))
	records, err := reader.ReadAll()
	require.NoError(t, err)
	assert.Len(t, records, 2)

	// Check all values are present
	row := records[1]
	assert.Equal(t, family.ID.String(), row[0])  // id
	assert.Equal(t, person1.ID.String(), row[1]) // partner1_id
	assert.Equal(t, "John Doe", row[2])          // partner1_name
	assert.Equal(t, person2.ID.String(), row[3]) // partner2_id
	assert.Equal(t, "Jane Smith", row[4])        // partner2_name
	assert.Equal(t, "marriage", row[5])          // relationship_type
	assert.Equal(t, "10 JUN 1875", row[6])       // marriage_date
	assert.Equal(t, "Springfield, IL", row[7])   // marriage_place
	assert.Equal(t, "3", row[8])                 // child_count
	assert.Equal(t, "2", row[9])                 // version
	assert.NotEmpty(t, row[10])                  // updated_at
}

func TestCSVExporter_FamilyWithNilPartners(t *testing.T) {
	store := setupTestStore(t)
	now := time.Now()

	// Create family with no partners
	family := repository.FamilyReadModel{
		ID:               uuid.New(),
		RelationshipType: domain.RelationUnknown,
		Version:          1,
		UpdatedAt:        now,
	}
	err := store.SaveFamily(context.Background(), &family)
	require.NoError(t, err)

	exp := exporter.NewDataExporter(store)

	var buf bytes.Buffer
	_, err = exp.Export(context.Background(), &buf, exporter.ExportOptions{
		Format:     exporter.FormatCSV,
		EntityType: exporter.EntityTypeFamilies,
		Fields:     []string{"id", "partner1_id", "partner2_id"},
	})

	require.NoError(t, err)

	// Parse and verify empty values for nil partners
	reader := csv.NewReader(strings.NewReader(buf.String()))
	records, err := reader.ReadAll()
	require.NoError(t, err)
	assert.Len(t, records, 2)

	row := records[1]
	assert.Equal(t, family.ID.String(), row[0]) // id
	assert.Empty(t, row[1])                     // partner1_id (nil)
	assert.Empty(t, row[2])                     // partner2_id (nil)
}

// Deterministic Output Tests

func TestJSONExporter_DeterministicOutput(t *testing.T) {
	store := setupTestStore(t)

	// Create multiple persons (order added shouldn't matter)
	_ = createTestPerson(t, store, "Zoe", "Adams", domain.GenderFemale)
	_ = createTestPerson(t, store, "Alice", "Williams", domain.GenderFemale)
	_ = createTestPerson(t, store, "Bob", "Johnson", domain.GenderMale)

	exp := exporter.NewDataExporter(store)

	// Export twice
	var buf1, buf2 bytes.Buffer
	_, err := exp.Export(context.Background(), &buf1, exporter.ExportOptions{
		Format:     exporter.FormatJSON,
		EntityType: exporter.EntityTypePersons,
	})
	require.NoError(t, err)

	_, err = exp.Export(context.Background(), &buf2, exporter.ExportOptions{
		Format:     exporter.FormatJSON,
		EntityType: exporter.EntityTypePersons,
	})
	require.NoError(t, err)

	// Output should be identical (sorted by ID)
	assert.Equal(t, buf1.String(), buf2.String())
}

func TestCSVExporter_DeterministicOutput(t *testing.T) {
	store := setupTestStore(t)

	// Create multiple persons
	_ = createTestPerson(t, store, "Zoe", "Adams", domain.GenderFemale)
	_ = createTestPerson(t, store, "Alice", "Williams", domain.GenderFemale)
	_ = createTestPerson(t, store, "Bob", "Johnson", domain.GenderMale)

	exp := exporter.NewDataExporter(store)

	// Export twice
	var buf1, buf2 bytes.Buffer
	_, err := exp.Export(context.Background(), &buf1, exporter.ExportOptions{
		Format:     exporter.FormatCSV,
		EntityType: exporter.EntityTypePersons,
	})
	require.NoError(t, err)

	_, err = exp.Export(context.Background(), &buf2, exporter.ExportOptions{
		Format:     exporter.FormatCSV,
		EntityType: exporter.EntityTypePersons,
	})
	require.NoError(t, err)

	// Output should be identical (sorted by ID)
	assert.Equal(t, buf1.String(), buf2.String())
}

// Bytes Written Tests

func TestExporter_BytesWrittenAccurate(t *testing.T) {
	store := setupTestStore(t)
	_ = createTestPerson(t, store, "John", "Doe", domain.GenderMale)

	exp := exporter.NewDataExporter(store)

	var buf bytes.Buffer
	result, err := exp.Export(context.Background(), &buf, exporter.ExportOptions{
		Format:     exporter.FormatJSON,
		EntityType: exporter.EntityTypePersons,
	})

	require.NoError(t, err)
	assert.Equal(t, int64(buf.Len()), result.BytesWritten)
}

// Sources Export Tests

func createTestSource(t *testing.T, store *memory.ReadModelStore, title, author string) repository.SourceReadModel {
	t.Helper()
	now := time.Now()
	source := repository.SourceReadModel{
		ID:             uuid.New(),
		SourceType:     domain.SourceBook,
		Title:          title,
		Author:         author,
		Publisher:      "Test Publisher",
		PublishDateRaw: "1995",
		URL:            "https://example.com/source",
		RepositoryName: "Library of Congress",
		CollectionName: "US Census",
		CallNumber:     "ABC-123",
		Notes:          "Test notes",
		CitationCount:  5,
		Version:        1,
		UpdatedAt:      now,
	}
	err := store.SaveSource(context.Background(), &source)
	require.NoError(t, err)
	return source
}

func createTestCitation(t *testing.T, store *memory.ReadModelStore, source *repository.SourceReadModel, factOwnerID uuid.UUID) repository.CitationReadModel {
	t.Helper()
	now := time.Now()
	citation := repository.CitationReadModel{
		ID:            uuid.New(),
		SourceID:      source.ID,
		SourceTitle:   source.Title,
		FactType:      domain.FactPersonBirth,
		FactOwnerID:   factOwnerID,
		Page:          "p. 42",
		Volume:        "Vol 1",
		SourceQuality: domain.SourceOriginal,
		InformantType: domain.InformantPrimary,
		EvidenceType:  domain.EvidenceDirect,
		QuotedText:    "Born on this date",
		Analysis:      "This appears reliable",
		Version:       1,
		CreatedAt:     now,
	}
	err := store.SaveCitation(context.Background(), &citation)
	require.NoError(t, err)
	return citation
}

func createTestEvent(t *testing.T, store *memory.ReadModelStore, ownerType string, ownerID uuid.UUID) repository.EventReadModel {
	t.Helper()
	now := time.Now()
	event := repository.EventReadModel{
		ID:             uuid.New(),
		OwnerType:      ownerType,
		OwnerID:        ownerID,
		FactType:       domain.FactPersonOccupation,
		DateRaw:        "1890",
		Place:          "Springfield, IL",
		Description:    "Worked as a farmer",
		Cause:          "",
		Age:            "25",
		ResearchStatus: domain.ResearchStatusCertain,
		Version:        1,
		CreatedAt:      now,
	}
	err := store.SaveEvent(context.Background(), &event)
	require.NoError(t, err)
	return event
}

func createTestAttribute(t *testing.T, store *memory.ReadModelStore, personID uuid.UUID) repository.AttributeReadModel {
	t.Helper()
	now := time.Now()
	attr := repository.AttributeReadModel{
		ID:        uuid.New(),
		PersonID:  personID,
		FactType:  domain.FactPersonOccupation,
		Value:     "Farmer",
		DateRaw:   "1890",
		Place:     "Springfield, IL",
		Version:   1,
		CreatedAt: now,
	}
	err := store.SaveAttribute(context.Background(), &attr)
	require.NoError(t, err)
	return attr
}

func TestJSONExporter_ExportSources_Empty(t *testing.T) {
	store := setupTestStore(t)
	exp := exporter.NewDataExporter(store)

	var buf bytes.Buffer
	result, err := exp.Export(context.Background(), &buf, exporter.ExportOptions{
		Format:     exporter.FormatJSON,
		EntityType: exporter.EntityTypeSources,
	})

	require.NoError(t, err)
	assert.Equal(t, 0, result.SourcesExported)

	var data []repository.SourceReadModel
	err = json.Unmarshal(buf.Bytes(), &data)
	require.NoError(t, err)
	assert.Empty(t, data)
}

func TestJSONExporter_ExportSources_WithData(t *testing.T) {
	store := setupTestStore(t)

	_ = createTestSource(t, store, "Family History Book", "John Author")
	_ = createTestSource(t, store, "Census Records", "Jane Compiler")

	exp := exporter.NewDataExporter(store)

	var buf bytes.Buffer
	result, err := exp.Export(context.Background(), &buf, exporter.ExportOptions{
		Format:     exporter.FormatJSON,
		EntityType: exporter.EntityTypeSources,
	})

	require.NoError(t, err)
	assert.Equal(t, 2, result.SourcesExported)

	var data []repository.SourceReadModel
	err = json.Unmarshal(buf.Bytes(), &data)
	require.NoError(t, err)
	assert.Len(t, data, 2)
}

func TestJSONExporter_ExportCitations_Empty(t *testing.T) {
	store := setupTestStore(t)
	exp := exporter.NewDataExporter(store)

	var buf bytes.Buffer
	result, err := exp.Export(context.Background(), &buf, exporter.ExportOptions{
		Format:     exporter.FormatJSON,
		EntityType: exporter.EntityTypeCitations,
	})

	require.NoError(t, err)
	assert.Equal(t, 0, result.CitationsExported)

	var data []repository.CitationReadModel
	err = json.Unmarshal(buf.Bytes(), &data)
	require.NoError(t, err)
	assert.Empty(t, data)
}

func TestJSONExporter_ExportCitations_WithData(t *testing.T) {
	store := setupTestStore(t)

	source := createTestSource(t, store, "Family History", "Author")
	person := createTestPerson(t, store, "John", "Doe", domain.GenderMale)
	_ = createTestCitation(t, store, &source, person.ID)

	exp := exporter.NewDataExporter(store)

	var buf bytes.Buffer
	result, err := exp.Export(context.Background(), &buf, exporter.ExportOptions{
		Format:     exporter.FormatJSON,
		EntityType: exporter.EntityTypeCitations,
	})

	require.NoError(t, err)
	assert.Equal(t, 1, result.CitationsExported)

	var data []repository.CitationReadModel
	err = json.Unmarshal(buf.Bytes(), &data)
	require.NoError(t, err)
	assert.Len(t, data, 1)
}

func TestJSONExporter_ExportEvents_Empty(t *testing.T) {
	store := setupTestStore(t)
	exp := exporter.NewDataExporter(store)

	var buf bytes.Buffer
	result, err := exp.Export(context.Background(), &buf, exporter.ExportOptions{
		Format:     exporter.FormatJSON,
		EntityType: exporter.EntityTypeEvents,
	})

	require.NoError(t, err)
	assert.Equal(t, 0, result.EventsExported)

	var data []repository.EventReadModel
	err = json.Unmarshal(buf.Bytes(), &data)
	require.NoError(t, err)
	assert.Empty(t, data)
}

func TestJSONExporter_ExportEvents_WithData(t *testing.T) {
	store := setupTestStore(t)

	person := createTestPerson(t, store, "John", "Doe", domain.GenderMale)
	_ = createTestEvent(t, store, "person", person.ID)

	exp := exporter.NewDataExporter(store)

	var buf bytes.Buffer
	result, err := exp.Export(context.Background(), &buf, exporter.ExportOptions{
		Format:     exporter.FormatJSON,
		EntityType: exporter.EntityTypeEvents,
	})

	require.NoError(t, err)
	assert.Equal(t, 1, result.EventsExported)

	var data []repository.EventReadModel
	err = json.Unmarshal(buf.Bytes(), &data)
	require.NoError(t, err)
	assert.Len(t, data, 1)
}

func TestJSONExporter_ExportAttributes_Empty(t *testing.T) {
	store := setupTestStore(t)
	exp := exporter.NewDataExporter(store)

	var buf bytes.Buffer
	result, err := exp.Export(context.Background(), &buf, exporter.ExportOptions{
		Format:     exporter.FormatJSON,
		EntityType: exporter.EntityTypeAttributes,
	})

	require.NoError(t, err)
	assert.Equal(t, 0, result.AttributesExported)

	var data []repository.AttributeReadModel
	err = json.Unmarshal(buf.Bytes(), &data)
	require.NoError(t, err)
	assert.Empty(t, data)
}

func TestJSONExporter_ExportAttributes_WithData(t *testing.T) {
	store := setupTestStore(t)

	person := createTestPerson(t, store, "John", "Doe", domain.GenderMale)
	_ = createTestAttribute(t, store, person.ID)

	exp := exporter.NewDataExporter(store)

	var buf bytes.Buffer
	result, err := exp.Export(context.Background(), &buf, exporter.ExportOptions{
		Format:     exporter.FormatJSON,
		EntityType: exporter.EntityTypeAttributes,
	})

	require.NoError(t, err)
	assert.Equal(t, 1, result.AttributesExported)

	var data []repository.AttributeReadModel
	err = json.Unmarshal(buf.Bytes(), &data)
	require.NoError(t, err)
	assert.Len(t, data, 1)
}

// CSV Sources Export Tests

func TestCSVExporter_ExportSources_Empty(t *testing.T) {
	store := setupTestStore(t)
	exp := exporter.NewDataExporter(store)

	var buf bytes.Buffer
	result, err := exp.Export(context.Background(), &buf, exporter.ExportOptions{
		Format:     exporter.FormatCSV,
		EntityType: exporter.EntityTypeSources,
	})

	require.NoError(t, err)
	assert.Equal(t, 0, result.SourcesExported)

	reader := csv.NewReader(strings.NewReader(buf.String()))
	records, err := reader.ReadAll()
	require.NoError(t, err)
	assert.Len(t, records, 1) // Headers only
	assert.Equal(t, exporter.DefaultSourceFields, records[0])
}

func TestCSVExporter_ExportSources_WithData(t *testing.T) {
	store := setupTestStore(t)

	_ = createTestSource(t, store, "Family History Book", "John Author")
	_ = createTestSource(t, store, "Census Records", "Jane Compiler")

	exp := exporter.NewDataExporter(store)

	var buf bytes.Buffer
	result, err := exp.Export(context.Background(), &buf, exporter.ExportOptions{
		Format:     exporter.FormatCSV,
		EntityType: exporter.EntityTypeSources,
	})

	require.NoError(t, err)
	assert.Equal(t, 2, result.SourcesExported)

	reader := csv.NewReader(strings.NewReader(buf.String()))
	records, err := reader.ReadAll()
	require.NoError(t, err)
	assert.Len(t, records, 3) // Header + 2 data rows

	allData := buf.String()
	assert.Contains(t, allData, "Family History Book")
	assert.Contains(t, allData, "Census Records")
}

func TestCSVExporter_ExportSources_CustomFields(t *testing.T) {
	store := setupTestStore(t)

	_ = createTestSource(t, store, "Test Source", "Test Author")

	exp := exporter.NewDataExporter(store)

	var buf bytes.Buffer
	customFields := []string{"id", "title", "author", "publisher"}
	result, err := exp.Export(context.Background(), &buf, exporter.ExportOptions{
		Format:     exporter.FormatCSV,
		EntityType: exporter.EntityTypeSources,
		Fields:     customFields,
	})

	require.NoError(t, err)
	assert.Equal(t, 1, result.SourcesExported)

	reader := csv.NewReader(strings.NewReader(buf.String()))
	records, err := reader.ReadAll()
	require.NoError(t, err)
	assert.Len(t, records, 2)
	assert.Equal(t, customFields, records[0])
	assert.Len(t, records[1], 4)
}

func TestCSVExporter_ExportSources_InvalidField(t *testing.T) {
	store := setupTestStore(t)
	exp := exporter.NewDataExporter(store)

	var buf bytes.Buffer
	_, err := exp.Export(context.Background(), &buf, exporter.ExportOptions{
		Format:     exporter.FormatCSV,
		EntityType: exporter.EntityTypeSources,
		Fields:     []string{"id", "invalid_field"},
	})

	require.Error(t, err)
	assert.Contains(t, err.Error(), "invalid fields")
}

func TestCSVExporter_ExportSources_AllFields(t *testing.T) {
	store := setupTestStore(t)

	source := createTestSource(t, store, "Complete Source", "Full Author")

	exp := exporter.NewDataExporter(store)

	allFields := []string{
		"id", "source_type", "title", "author", "publisher",
		"publish_date", "url", "repository_name", "collection_name",
		"call_number", "notes", "citation_count", "version", "updated_at",
	}

	var buf bytes.Buffer
	_, err := exp.Export(context.Background(), &buf, exporter.ExportOptions{
		Format:     exporter.FormatCSV,
		EntityType: exporter.EntityTypeSources,
		Fields:     allFields,
	})

	require.NoError(t, err)

	reader := csv.NewReader(strings.NewReader(buf.String()))
	records, err := reader.ReadAll()
	require.NoError(t, err)
	assert.Len(t, records, 2)

	row := records[1]
	assert.Equal(t, source.ID.String(), row[0])
	assert.Equal(t, "book", row[1])
	assert.Equal(t, "Complete Source", row[2])
	assert.Equal(t, "Full Author", row[3])
	assert.Equal(t, "Test Publisher", row[4])
	assert.Equal(t, "1995", row[5])
	assert.Equal(t, "https://example.com/source", row[6])
	assert.Equal(t, "Library of Congress", row[7])
	assert.Equal(t, "US Census", row[8])
	assert.Equal(t, "ABC-123", row[9])
	assert.Equal(t, "Test notes", row[10])
	assert.Equal(t, "5", row[11])
}

// CSV Citations Export Tests

func TestCSVExporter_ExportCitations_Empty(t *testing.T) {
	store := setupTestStore(t)
	exp := exporter.NewDataExporter(store)

	var buf bytes.Buffer
	result, err := exp.Export(context.Background(), &buf, exporter.ExportOptions{
		Format:     exporter.FormatCSV,
		EntityType: exporter.EntityTypeCitations,
	})

	require.NoError(t, err)
	assert.Equal(t, 0, result.CitationsExported)

	reader := csv.NewReader(strings.NewReader(buf.String()))
	records, err := reader.ReadAll()
	require.NoError(t, err)
	assert.Len(t, records, 1)
	assert.Equal(t, exporter.DefaultCitationFields, records[0])
}

func TestCSVExporter_ExportCitations_WithData(t *testing.T) {
	store := setupTestStore(t)

	source := createTestSource(t, store, "Family History", "Author")
	person := createTestPerson(t, store, "John", "Doe", domain.GenderMale)
	_ = createTestCitation(t, store, &source, person.ID)

	exp := exporter.NewDataExporter(store)

	var buf bytes.Buffer
	result, err := exp.Export(context.Background(), &buf, exporter.ExportOptions{
		Format:     exporter.FormatCSV,
		EntityType: exporter.EntityTypeCitations,
	})

	require.NoError(t, err)
	assert.Equal(t, 1, result.CitationsExported)

	reader := csv.NewReader(strings.NewReader(buf.String()))
	records, err := reader.ReadAll()
	require.NoError(t, err)
	assert.Len(t, records, 2)

	allData := buf.String()
	assert.Contains(t, allData, "Family History")
	assert.Contains(t, allData, "p. 42")
}

func TestCSVExporter_ExportCitations_CustomFields(t *testing.T) {
	store := setupTestStore(t)

	source := createTestSource(t, store, "Source", "Author")
	person := createTestPerson(t, store, "John", "Doe", domain.GenderMale)
	_ = createTestCitation(t, store, &source, person.ID)

	exp := exporter.NewDataExporter(store)

	var buf bytes.Buffer
	customFields := []string{"id", "source_title", "page", "volume"}
	result, err := exp.Export(context.Background(), &buf, exporter.ExportOptions{
		Format:     exporter.FormatCSV,
		EntityType: exporter.EntityTypeCitations,
		Fields:     customFields,
	})

	require.NoError(t, err)
	assert.Equal(t, 1, result.CitationsExported)

	reader := csv.NewReader(strings.NewReader(buf.String()))
	records, err := reader.ReadAll()
	require.NoError(t, err)
	assert.Equal(t, customFields, records[0])
	assert.Len(t, records[1], 4)
}

func TestCSVExporter_ExportCitations_InvalidField(t *testing.T) {
	store := setupTestStore(t)
	exp := exporter.NewDataExporter(store)

	var buf bytes.Buffer
	_, err := exp.Export(context.Background(), &buf, exporter.ExportOptions{
		Format:     exporter.FormatCSV,
		EntityType: exporter.EntityTypeCitations,
		Fields:     []string{"id", "bad_field"},
	})

	require.Error(t, err)
	assert.Contains(t, err.Error(), "invalid fields")
}

func TestCSVExporter_ExportCitations_AllFields(t *testing.T) {
	store := setupTestStore(t)

	source := createTestSource(t, store, "Source Title", "Author")
	person := createTestPerson(t, store, "John", "Doe", domain.GenderMale)
	citation := createTestCitation(t, store, &source, person.ID)

	exp := exporter.NewDataExporter(store)

	allFields := []string{
		"id", "source_id", "source_title", "fact_type", "fact_owner_id",
		"page", "volume", "source_quality", "informant_type", "evidence_type",
		"quoted_text", "analysis", "version", "created_at",
	}

	var buf bytes.Buffer
	_, err := exp.Export(context.Background(), &buf, exporter.ExportOptions{
		Format:     exporter.FormatCSV,
		EntityType: exporter.EntityTypeCitations,
		Fields:     allFields,
	})

	require.NoError(t, err)

	reader := csv.NewReader(strings.NewReader(buf.String()))
	records, err := reader.ReadAll()
	require.NoError(t, err)
	assert.Len(t, records, 2)

	row := records[1]
	assert.Equal(t, citation.ID.String(), row[0])
	assert.Equal(t, source.ID.String(), row[1])
	assert.Equal(t, "Source Title", row[2])
	assert.Equal(t, "person_birth", row[3])
	assert.Equal(t, person.ID.String(), row[4])
	assert.Equal(t, "p. 42", row[5])
	assert.Equal(t, "Vol 1", row[6])
	assert.Equal(t, "original", row[7])
	assert.Equal(t, "primary", row[8])
	assert.Equal(t, "direct", row[9])
	assert.Equal(t, "Born on this date", row[10])
	assert.Equal(t, "This appears reliable", row[11])
}

// CSV Events Export Tests

func TestCSVExporter_ExportEvents_Empty(t *testing.T) {
	store := setupTestStore(t)
	exp := exporter.NewDataExporter(store)

	var buf bytes.Buffer
	result, err := exp.Export(context.Background(), &buf, exporter.ExportOptions{
		Format:     exporter.FormatCSV,
		EntityType: exporter.EntityTypeEvents,
	})

	require.NoError(t, err)
	assert.Equal(t, 0, result.EventsExported)

	reader := csv.NewReader(strings.NewReader(buf.String()))
	records, err := reader.ReadAll()
	require.NoError(t, err)
	assert.Len(t, records, 1)
	assert.Equal(t, exporter.DefaultEventFields, records[0])
}

func TestCSVExporter_ExportEvents_WithData(t *testing.T) {
	store := setupTestStore(t)

	person := createTestPerson(t, store, "John", "Doe", domain.GenderMale)
	_ = createTestEvent(t, store, "person", person.ID)

	exp := exporter.NewDataExporter(store)

	var buf bytes.Buffer
	result, err := exp.Export(context.Background(), &buf, exporter.ExportOptions{
		Format:     exporter.FormatCSV,
		EntityType: exporter.EntityTypeEvents,
	})

	require.NoError(t, err)
	assert.Equal(t, 1, result.EventsExported)

	reader := csv.NewReader(strings.NewReader(buf.String()))
	records, err := reader.ReadAll()
	require.NoError(t, err)
	assert.Len(t, records, 2)

	allData := buf.String()
	assert.Contains(t, allData, "person")
	assert.Contains(t, allData, "Springfield, IL")
}

func TestCSVExporter_ExportEvents_CustomFields(t *testing.T) {
	store := setupTestStore(t)

	person := createTestPerson(t, store, "John", "Doe", domain.GenderMale)
	_ = createTestEvent(t, store, "person", person.ID)

	exp := exporter.NewDataExporter(store)

	var buf bytes.Buffer
	customFields := []string{"id", "owner_type", "fact_type", "place"}
	result, err := exp.Export(context.Background(), &buf, exporter.ExportOptions{
		Format:     exporter.FormatCSV,
		EntityType: exporter.EntityTypeEvents,
		Fields:     customFields,
	})

	require.NoError(t, err)
	assert.Equal(t, 1, result.EventsExported)

	reader := csv.NewReader(strings.NewReader(buf.String()))
	records, err := reader.ReadAll()
	require.NoError(t, err)
	assert.Equal(t, customFields, records[0])
	assert.Len(t, records[1], 4)
}

func TestCSVExporter_ExportEvents_InvalidField(t *testing.T) {
	store := setupTestStore(t)
	exp := exporter.NewDataExporter(store)

	var buf bytes.Buffer
	_, err := exp.Export(context.Background(), &buf, exporter.ExportOptions{
		Format:     exporter.FormatCSV,
		EntityType: exporter.EntityTypeEvents,
		Fields:     []string{"id", "nonexistent"},
	})

	require.Error(t, err)
	assert.Contains(t, err.Error(), "invalid fields")
}

func TestCSVExporter_ExportEvents_AllFields(t *testing.T) {
	store := setupTestStore(t)

	person := createTestPerson(t, store, "John", "Doe", domain.GenderMale)
	event := createTestEvent(t, store, "person", person.ID)

	exp := exporter.NewDataExporter(store)

	allFields := []string{
		"id", "owner_type", "owner_id", "fact_type", "date", "place",
		"description", "cause", "age", "research_status", "version", "created_at",
	}

	var buf bytes.Buffer
	_, err := exp.Export(context.Background(), &buf, exporter.ExportOptions{
		Format:     exporter.FormatCSV,
		EntityType: exporter.EntityTypeEvents,
		Fields:     allFields,
	})

	require.NoError(t, err)

	reader := csv.NewReader(strings.NewReader(buf.String()))
	records, err := reader.ReadAll()
	require.NoError(t, err)
	assert.Len(t, records, 2)

	row := records[1]
	assert.Equal(t, event.ID.String(), row[0])
	assert.Equal(t, "person", row[1])
	assert.Equal(t, person.ID.String(), row[2])
	assert.Equal(t, "person_occupation", row[3])
	assert.Equal(t, "1890", row[4])
	assert.Equal(t, "Springfield, IL", row[5])
	assert.Equal(t, "Worked as a farmer", row[6])
	assert.Equal(t, "", row[7]) // cause
	assert.Equal(t, "25", row[8])
	assert.Equal(t, "certain", row[9])
}

// CSV Attributes Export Tests

func TestCSVExporter_ExportAttributes_Empty(t *testing.T) {
	store := setupTestStore(t)
	exp := exporter.NewDataExporter(store)

	var buf bytes.Buffer
	result, err := exp.Export(context.Background(), &buf, exporter.ExportOptions{
		Format:     exporter.FormatCSV,
		EntityType: exporter.EntityTypeAttributes,
	})

	require.NoError(t, err)
	assert.Equal(t, 0, result.AttributesExported)

	reader := csv.NewReader(strings.NewReader(buf.String()))
	records, err := reader.ReadAll()
	require.NoError(t, err)
	assert.Len(t, records, 1)
	assert.Equal(t, exporter.DefaultAttributeFields, records[0])
}

func TestCSVExporter_ExportAttributes_WithData(t *testing.T) {
	store := setupTestStore(t)

	person := createTestPerson(t, store, "John", "Doe", domain.GenderMale)
	_ = createTestAttribute(t, store, person.ID)

	exp := exporter.NewDataExporter(store)

	var buf bytes.Buffer
	result, err := exp.Export(context.Background(), &buf, exporter.ExportOptions{
		Format:     exporter.FormatCSV,
		EntityType: exporter.EntityTypeAttributes,
	})

	require.NoError(t, err)
	assert.Equal(t, 1, result.AttributesExported)

	reader := csv.NewReader(strings.NewReader(buf.String()))
	records, err := reader.ReadAll()
	require.NoError(t, err)
	assert.Len(t, records, 2)

	allData := buf.String()
	assert.Contains(t, allData, "Farmer")
	assert.Contains(t, allData, "Springfield, IL")
}

func TestCSVExporter_ExportAttributes_CustomFields(t *testing.T) {
	store := setupTestStore(t)

	person := createTestPerson(t, store, "John", "Doe", domain.GenderMale)
	_ = createTestAttribute(t, store, person.ID)

	exp := exporter.NewDataExporter(store)

	var buf bytes.Buffer
	customFields := []string{"id", "person_id", "fact_type", "value"}
	result, err := exp.Export(context.Background(), &buf, exporter.ExportOptions{
		Format:     exporter.FormatCSV,
		EntityType: exporter.EntityTypeAttributes,
		Fields:     customFields,
	})

	require.NoError(t, err)
	assert.Equal(t, 1, result.AttributesExported)

	reader := csv.NewReader(strings.NewReader(buf.String()))
	records, err := reader.ReadAll()
	require.NoError(t, err)
	assert.Equal(t, customFields, records[0])
	assert.Len(t, records[1], 4)
}

func TestCSVExporter_ExportAttributes_InvalidField(t *testing.T) {
	store := setupTestStore(t)
	exp := exporter.NewDataExporter(store)

	var buf bytes.Buffer
	_, err := exp.Export(context.Background(), &buf, exporter.ExportOptions{
		Format:     exporter.FormatCSV,
		EntityType: exporter.EntityTypeAttributes,
		Fields:     []string{"id", "wrong_field"},
	})

	require.Error(t, err)
	assert.Contains(t, err.Error(), "invalid fields")
}

func TestCSVExporter_ExportAttributes_AllFields(t *testing.T) {
	store := setupTestStore(t)

	person := createTestPerson(t, store, "John", "Doe", domain.GenderMale)
	attr := createTestAttribute(t, store, person.ID)

	exp := exporter.NewDataExporter(store)

	allFields := []string{
		"id", "person_id", "fact_type", "value", "date", "place", "version", "created_at",
	}

	var buf bytes.Buffer
	_, err := exp.Export(context.Background(), &buf, exporter.ExportOptions{
		Format:     exporter.FormatCSV,
		EntityType: exporter.EntityTypeAttributes,
		Fields:     allFields,
	})

	require.NoError(t, err)

	reader := csv.NewReader(strings.NewReader(buf.String()))
	records, err := reader.ReadAll()
	require.NoError(t, err)
	assert.Len(t, records, 2)

	row := records[1]
	assert.Equal(t, attr.ID.String(), row[0])
	assert.Equal(t, person.ID.String(), row[1])
	assert.Equal(t, "person_occupation", row[2])
	assert.Equal(t, "Farmer", row[3])
	assert.Equal(t, "1890", row[4])
	assert.Equal(t, "Springfield, IL", row[5])
}

// Tree Export with All Entity Types Tests

func TestJSONExporter_ExportTree_WithAllEntities(t *testing.T) {
	store := setupTestStore(t)

	// Create test data
	person := createTestPerson(t, store, "John", "Doe", domain.GenderMale)
	person2 := createTestPerson(t, store, "Jane", "Smith", domain.GenderFemale)
	_ = createTestFamily(t, store, &person, &person2)
	source := createTestSource(t, store, "Family History", "Author")
	_ = createTestCitation(t, store, &source, person.ID)
	_ = createTestEvent(t, store, "person", person.ID)
	_ = createTestAttribute(t, store, person.ID)

	exp := exporter.NewDataExporter(store)

	var buf bytes.Buffer
	result, err := exp.Export(context.Background(), &buf, exporter.ExportOptions{
		Format:     exporter.FormatJSON,
		EntityType: exporter.EntityTypeAll,
	})

	require.NoError(t, err)
	assert.Equal(t, 2, result.PersonsExported)
	assert.Equal(t, 1, result.FamiliesExported)
	assert.Equal(t, 1, result.SourcesExported)
	assert.Equal(t, 1, result.CitationsExported)
	assert.Equal(t, 1, result.EventsExported)
	assert.Equal(t, 1, result.AttributesExported)

	var data exporter.TreeExport
	err = json.Unmarshal(buf.Bytes(), &data)
	require.NoError(t, err)
	assert.Len(t, data.Persons, 2)
	assert.Len(t, data.Families, 1)
	assert.Len(t, data.Sources, 1)
	assert.Len(t, data.Citations, 1)
	assert.Len(t, data.Events, 1)
	assert.Len(t, data.Attributes, 1)
}

// Multiple Entities Tests

func TestExporter_MultiplePersonsAndFamilies(t *testing.T) {
	store := setupTestStore(t)

	// Create a family tree
	grandpa := createTestPerson(t, store, "Grandpa", "Smith", domain.GenderMale)
	grandma := createTestPerson(t, store, "Grandma", "Jones", domain.GenderFemale)
	dad := createTestPerson(t, store, "Dad", "Smith", domain.GenderMale)
	mom := createTestPerson(t, store, "Mom", "Brown", domain.GenderFemale)
	child := createTestPerson(t, store, "Child", "Smith", domain.GenderMale)

	grandparentsFamily := createTestFamily(t, store, &grandpa, &grandma)
	parentsFamily := createTestFamily(t, store, &dad, &mom)
	_, _, _ = grandparentsFamily, parentsFamily, child

	exp := exporter.NewDataExporter(store)

	var buf bytes.Buffer
	result, err := exp.Export(context.Background(), &buf, exporter.ExportOptions{
		Format:     exporter.FormatJSON,
		EntityType: exporter.EntityTypeAll,
	})

	require.NoError(t, err)
	assert.Equal(t, 5, result.PersonsExported)
	assert.Equal(t, 2, result.FamiliesExported)

	// Verify JSON structure
	var data exporter.TreeExport
	err = json.Unmarshal(buf.Bytes(), &data)
	require.NoError(t, err)
	assert.Len(t, data.Persons, 5)
	assert.Len(t, data.Families, 2)
}
