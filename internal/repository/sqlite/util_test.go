package sqlite_test

import (
	"database/sql"
	"os"
	"path/filepath"
	"testing"
	"time"

	_ "github.com/mattn/go-sqlite3"

	"github.com/cacack/my-family/internal/repository/sqlite"
)

func TestOpenDB_Success(t *testing.T) {
	tmpFile, err := os.CreateTemp("", "myfamily-opendb-test-*.db")
	if err != nil {
		t.Fatalf("create temp file: %v", err)
	}
	tmpFile.Close()
	defer os.Remove(tmpFile.Name())

	db, err := sqlite.OpenDB(tmpFile.Name())
	if err != nil {
		t.Fatalf("OpenDB failed: %v", err)
	}
	defer db.Close()

	// Verify connection is alive
	if err := db.Ping(); err != nil {
		t.Errorf("ping failed: %v", err)
	}

	// Verify connection pool settings
	stats := db.Stats()
	if stats.MaxOpenConnections != 1 {
		t.Errorf("expected MaxOpenConnections 1, got %d", stats.MaxOpenConnections)
	}
}

func TestOpenDB_InvalidPath(t *testing.T) {
	// Test with invalid path - directory that doesn't exist
	invalidPath := filepath.Join("/nonexistent", "directory", "test.db")
	db, err := sqlite.OpenDB(invalidPath)
	if err == nil {
		db.Close()
		t.Error("expected error for invalid path, got nil")
	}
}

func TestOpenDB_DirectoryAsPath(t *testing.T) {
	// Create a temporary directory
	tmpDir, err := os.MkdirTemp("", "myfamily-opendb-dir-test-*")
	if err != nil {
		t.Fatalf("create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	// Try to open the directory as a database file
	db, err := sqlite.OpenDB(tmpDir)
	if err == nil {
		db.Close()
		t.Error("expected error when opening directory as database, got nil")
	}
}

func TestParseTimestamp_ValidFormats(t *testing.T) {
	// Test various timestamp formats by parsing them with time.Parse
	// This validates the formats that parseTimestamp would handle

	tests := []struct {
		name      string
		timestamp string
		formats   []string
		wantErr   bool
	}{
		{
			name:      "RFC3339Nano with offset",
			timestamp: "2023-12-25T10:30:45.123456789-07:00",
			formats:   []string{time.RFC3339Nano, "2006-01-02T15:04:05.999999999Z07:00"},
			wantErr:   false,
		},
		{
			name:      "RFC3339 with offset",
			timestamp: "2023-12-25T10:30:45-07:00",
			formats:   []string{time.RFC3339, "2006-01-02T15:04:05Z07:00"},
			wantErr:   false,
		},
		{
			name:      "RFC3339Nano UTC",
			timestamp: "2023-12-25T10:30:45.123456789Z",
			formats:   []string{time.RFC3339Nano, "2006-01-02T15:04:05.999999999Z"},
			wantErr:   false,
		},
		{
			name:      "RFC3339 UTC",
			timestamp: "2023-12-25T10:30:45Z",
			formats:   []string{time.RFC3339, "2006-01-02T15:04:05Z"},
			wantErr:   false,
		},
		{
			name:      "Custom format with nanoseconds and offset",
			timestamp: "2023-12-25T10:30:45.999999999-05:00",
			formats:   []string{"2006-01-02T15:04:05.999999999Z07:00"},
			wantErr:   false,
		},
		{
			name:      "Custom format without nanoseconds with offset",
			timestamp: "2023-12-25T10:30:45-05:00",
			formats:   []string{"2006-01-02T15:04:05Z07:00"},
			wantErr:   false,
		},
		{
			name:      "Empty string",
			timestamp: "",
			formats:   []string{time.RFC3339},
			wantErr:   true,
		},
		{
			name:      "Invalid format",
			timestamp: "not-a-timestamp",
			formats:   []string{time.RFC3339},
			wantErr:   true,
		},
		{
			name:      "Invalid date components",
			timestamp: "2023-13-45T99:99:99Z",
			formats:   []string{time.RFC3339},
			wantErr:   true,
		},
		{
			name:      "Completely invalid",
			timestamp: "this is not a date at all",
			formats:   []string{time.RFC3339, time.RFC3339Nano, "2006-01-02T15:04:05.999999999Z07:00"},
			wantErr:   true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			parsed := false
			var lastErr error
			for _, format := range tt.formats {
				_, err := time.Parse(format, tt.timestamp)
				if err == nil {
					parsed = true
					break
				}
				lastErr = err
			}

			if tt.wantErr && parsed {
				t.Errorf("expected error for invalid timestamp, but parsing succeeded")
			} else if !tt.wantErr && !parsed {
				t.Errorf("expected valid timestamp, got error: %v", lastErr)
			}
		})
	}
}

func TestNullableUUID_Nil(t *testing.T) {
	// Test nullableUUID with nil pointer through database operations
	tmpFile, err := os.CreateTemp("", "myfamily-uuid-test-*.db")
	if err != nil {
		t.Fatalf("create temp file: %v", err)
	}
	tmpFile.Close()
	defer os.Remove(tmpFile.Name())

	db, err := sqlite.OpenDB(tmpFile.Name())
	if err != nil {
		t.Fatalf("open database: %v", err)
	}
	defer db.Close()

	_, err = sqlite.NewReadModelStore(db)
	if err != nil {
		t.Fatalf("create read model store: %v", err)
	}

	// Insert a test person record - full_name is generated so don't include it
	_, err = db.Exec(`INSERT INTO persons (id, given_name, surname, version, updated_at)
		VALUES (?, ?, ?, ?, ?)`,
		"test-id-1", "Test", "Person", 1, time.Now().Format(time.RFC3339))
	if err != nil {
		t.Fatalf("insert test person: %v", err)
	}

	// Query the record back to verify it was inserted
	var givenName, surname string
	err = db.QueryRow(`SELECT given_name, surname FROM persons WHERE id = ?`, "test-id-1").Scan(&givenName, &surname)
	if err != nil {
		t.Fatalf("query test person: %v", err)
	}

	if givenName != "Test" || surname != "Person" {
		t.Errorf("expected Test Person, got %s %s", givenName, surname)
	}

	// Insert a family with NULL partner IDs to test nullableUUID with nil pointers
	familyID := "test-family-1"
	_, err = db.Exec(`INSERT INTO families (id, partner1_id, partner2_id, version, updated_at)
		VALUES (?, NULL, NULL, ?, ?)`,
		familyID, 1, time.Now().Format(time.RFC3339))
	if err != nil {
		t.Fatalf("insert family with NULL partners: %v", err)
	}

	// Query back to verify NULL values
	var p1ID, p2ID sql.NullString
	err = db.QueryRow(`SELECT partner1_id, partner2_id FROM families WHERE id = ?`, familyID).Scan(&p1ID, &p2ID)
	if err != nil {
		t.Fatalf("query family: %v", err)
	}

	if p1ID.Valid {
		t.Error("expected partner1_id to be NULL")
	}
	if p2ID.Valid {
		t.Error("expected partner2_id to be NULL")
	}

	// Clean up
	_, err = db.Exec(`DELETE FROM families WHERE id = ?`, familyID)
	if err != nil {
		t.Fatalf("delete family: %v", err)
	}
	_, err = db.Exec(`DELETE FROM persons WHERE id = ?`, "test-id-1")
	if err != nil {
		t.Fatalf("delete test person: %v", err)
	}
}

func TestNullableUUID_NonNil(t *testing.T) {
	// Test nullableUUID with non-nil UUID pointer through database operations
	tmpFile, err := os.CreateTemp("", "myfamily-uuid-notnull-test-*.db")
	if err != nil {
		t.Fatalf("create temp file: %v", err)
	}
	tmpFile.Close()
	defer os.Remove(tmpFile.Name())

	db, err := sqlite.OpenDB(tmpFile.Name())
	if err != nil {
		t.Fatalf("open database: %v", err)
	}
	defer db.Close()

	_, err = sqlite.NewReadModelStore(db)
	if err != nil {
		t.Fatalf("create read model store: %v", err)
	}

	// Create persons that will be referenced
	partner1ID := "00000000-0000-0000-0000-000000000001"
	partner2ID := "00000000-0000-0000-0000-000000000002"

	_, err = db.Exec(`INSERT INTO persons (id, given_name, surname, version, updated_at)
		VALUES (?, ?, ?, ?, ?)`,
		partner1ID, "Partner", "One", 1, time.Now().Format(time.RFC3339))
	if err != nil {
		t.Fatalf("insert partner 1: %v", err)
	}

	_, err = db.Exec(`INSERT INTO persons (id, given_name, surname, version, updated_at)
		VALUES (?, ?, ?, ?, ?)`,
		partner2ID, "Partner", "Two", 1, time.Now().Format(time.RFC3339))
	if err != nil {
		t.Fatalf("insert partner 2: %v", err)
	}

	// Insert a family record with both partner IDs (non-nil UUIDs)
	familyID := "00000000-0000-0000-0000-000000000003"

	_, err = db.Exec(`INSERT INTO families (id, partner1_id, partner2_id, version, updated_at)
		VALUES (?, ?, ?, ?, ?)`,
		familyID, partner1ID, partner2ID, 1, time.Now().Format(time.RFC3339))
	if err != nil {
		t.Fatalf("insert family with non-nil UUIDs: %v", err)
	}

	// Query back to verify UUIDs were stored correctly
	var p1ID, p2ID sql.NullString
	err = db.QueryRow(`SELECT partner1_id, partner2_id FROM families WHERE id = ?`, familyID).Scan(&p1ID, &p2ID)
	if err != nil {
		t.Fatalf("query family: %v", err)
	}

	if !p1ID.Valid {
		t.Error("expected partner1_id to be valid")
	}
	if !p2ID.Valid {
		t.Error("expected partner2_id to be valid")
	}
	if p1ID.String != partner1ID {
		t.Errorf("expected partner1_id %s, got %s", partner1ID, p1ID.String)
	}
	if p2ID.String != partner2ID {
		t.Errorf("expected partner2_id %s, got %s", partner2ID, p2ID.String)
	}

	// Clean up
	_, err = db.Exec(`DELETE FROM families WHERE id = ?`, familyID)
	if err != nil {
		t.Fatalf("delete family: %v", err)
	}
	_, err = db.Exec(`DELETE FROM persons WHERE id IN (?, ?)`, partner1ID, partner2ID)
	if err != nil {
		t.Fatalf("delete persons: %v", err)
	}
}

func TestNullableUUID_SinglePartner(t *testing.T) {
	// Test nullableUUID with one nil and one non-nil UUID
	tmpFile, err := os.CreateTemp("", "myfamily-uuid-single-test-*.db")
	if err != nil {
		t.Fatalf("create temp file: %v", err)
	}
	tmpFile.Close()
	defer os.Remove(tmpFile.Name())

	db, err := sqlite.OpenDB(tmpFile.Name())
	if err != nil {
		t.Fatalf("open database: %v", err)
	}
	defer db.Close()

	_, err = sqlite.NewReadModelStore(db)
	if err != nil {
		t.Fatalf("create read model store: %v", err)
	}

	// Create a person that will be referenced
	partner1ID := "00000000-0000-0000-0000-000000000001"

	_, err = db.Exec(`INSERT INTO persons (id, given_name, surname, version, updated_at)
		VALUES (?, ?, ?, ?, ?)`,
		partner1ID, "Single", "Partner", 1, time.Now().Format(time.RFC3339))
	if err != nil {
		t.Fatalf("insert partner: %v", err)
	}

	// Insert a family with only partner1_id (partner2_id is NULL)
	familyID := "00000000-0000-0000-0000-000000000004"

	_, err = db.Exec(`INSERT INTO families (id, partner1_id, partner2_id, version, updated_at)
		VALUES (?, ?, NULL, ?, ?)`,
		familyID, partner1ID, 1, time.Now().Format(time.RFC3339))
	if err != nil {
		t.Fatalf("insert family with single partner: %v", err)
	}

	// Query back to verify
	var p1ID, p2ID sql.NullString
	err = db.QueryRow(`SELECT partner1_id, partner2_id FROM families WHERE id = ?`, familyID).Scan(&p1ID, &p2ID)
	if err != nil {
		t.Fatalf("query family: %v", err)
	}

	if !p1ID.Valid {
		t.Error("expected partner1_id to be valid")
	}
	if p2ID.Valid {
		t.Error("expected partner2_id to be NULL")
	}
	if p1ID.String != partner1ID {
		t.Errorf("expected partner1_id %s, got %s", partner1ID, p1ID.String)
	}

	// Clean up
	_, err = db.Exec(`DELETE FROM families WHERE id = ?`, familyID)
	if err != nil {
		t.Fatalf("delete family: %v", err)
	}
	_, err = db.Exec(`DELETE FROM persons WHERE id = ?`, partner1ID)
	if err != nil {
		t.Fatalf("delete person: %v", err)
	}
}
