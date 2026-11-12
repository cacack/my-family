package command_test

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/google/uuid"

	"github.com/cacack/my-family/internal/command"
	"github.com/cacack/my-family/internal/domain"
	"github.com/cacack/my-family/internal/repository"
	"github.com/cacack/my-family/internal/repository/memory"
)

// TestCreateLDSOrdinance tests creating a new LDS ordinance.
func TestCreateLDSOrdinance(t *testing.T) {
	eventStore := memory.NewEventStore()
	readStore := memory.NewReadModelStore()
	handler := command.NewHandler(eventStore, readStore)
	ctx := context.Background()

	// First create a person for individual ordinances
	personResult, err := handler.CreatePerson(ctx, command.CreatePersonInput{
		GivenName: "John",
		Surname:   "Doe",
		Gender:    "male",
	})
	if err != nil {
		t.Fatalf("Failed to create person: %v", err)
	}

	// Create a second person and family for spouse sealing
	person2Result, err := handler.CreatePerson(ctx, command.CreatePersonInput{
		GivenName: "Jane",
		Surname:   "Doe",
		Gender:    "female",
	})
	if err != nil {
		t.Fatalf("Failed to create person: %v", err)
	}

	familyResult, err := handler.CreateFamily(ctx, command.CreateFamilyInput{
		Partner1ID: &personResult.ID,
		Partner2ID: &person2Result.ID,
	})
	if err != nil {
		t.Fatalf("Failed to create family: %v", err)
	}

	tests := []struct {
		name    string
		input   command.CreateLDSOrdinanceInput
		wantErr bool
	}{
		{
			name: "valid baptism ordinance",
			input: command.CreateLDSOrdinanceInput{
				Type:     domain.LDSBaptism,
				PersonID: &personResult.ID,
				Date:     "15 JAN 1900",
				Temple:   "SLAKE",
				Status:   "COMPLETED",
			},
			wantErr: false,
		},
		{
			name: "valid confirmation ordinance",
			input: command.CreateLDSOrdinanceInput{
				Type:     domain.LDSConfirmation,
				PersonID: &personResult.ID,
			},
			wantErr: false,
		},
		{
			name: "valid endowment ordinance",
			input: command.CreateLDSOrdinanceInput{
				Type:     domain.LDSEndowment,
				PersonID: &personResult.ID,
				Place:    "Salt Lake Temple",
			},
			wantErr: false,
		},
		{
			name: "valid sealing to parents",
			input: command.CreateLDSOrdinanceInput{
				Type:     domain.LDSSealingChild,
				PersonID: &personResult.ID,
			},
			wantErr: false,
		},
		{
			name: "valid spouse sealing",
			input: command.CreateLDSOrdinanceInput{
				Type:     domain.LDSSealingSpouse,
				FamilyID: &familyResult.ID,
			},
			wantErr: false,
		},
		{
			name: "invalid - individual ordinance without person ID",
			input: command.CreateLDSOrdinanceInput{
				Type: domain.LDSBaptism,
			},
			wantErr: true,
		},
		{
			name: "invalid - spouse sealing without family ID",
			input: command.CreateLDSOrdinanceInput{
				Type: domain.LDSSealingSpouse,
			},
			wantErr: true,
		},
		{
			name: "invalid - unknown ordinance type",
			input: command.CreateLDSOrdinanceInput{
				Type:     domain.LDSOrdinanceType("INVALID"),
				PersonID: &personResult.ID,
			},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := handler.CreateLDSOrdinance(ctx, tt.input)
			if (err != nil) != tt.wantErr {
				t.Errorf("CreateLDSOrdinance() error = %v, wantErr %v", err, tt.wantErr)
				return
			}

			if !tt.wantErr {
				if result.ID == uuid.Nil {
					t.Error("Expected non-nil ID")
				}
				if result.Version != 1 {
					t.Errorf("Version = %d, want 1", result.Version)
				}

				// Verify ordinance in read model
				ord, _ := readStore.GetLDSOrdinance(ctx, result.ID)
				if ord == nil {
					t.Fatal("LDS ordinance not found in read model")
				}
				if ord.Type != tt.input.Type {
					t.Errorf("Type = %s, want %s", ord.Type, tt.input.Type)
				}
			}
		})
	}
}

// TestUpdateLDSOrdinance tests updating an LDS ordinance.
func TestUpdateLDSOrdinance(t *testing.T) {
	eventStore := memory.NewEventStore()
	readStore := memory.NewReadModelStore()
	handler := command.NewHandler(eventStore, readStore)
	ctx := context.Background()

	// Create person and ordinance
	personResult, _ := handler.CreatePerson(ctx, command.CreatePersonInput{
		GivenName: "John",
		Surname:   "Doe",
		Gender:    "male",
	})

	createResult, err := handler.CreateLDSOrdinance(ctx, command.CreateLDSOrdinanceInput{
		Type:     domain.LDSBaptism,
		PersonID: &personResult.ID,
		Temple:   "SLAKE",
	})
	if err != nil {
		t.Fatalf("CreateLDSOrdinance failed: %v", err)
	}

	tests := []struct {
		name    string
		input   command.UpdateLDSOrdinanceInput
		wantErr bool
	}{
		{
			name: "update temple",
			input: command.UpdateLDSOrdinanceInput{
				ID:      createResult.ID,
				Temple:  strPtr("LOGAN"),
				Version: createResult.Version,
			},
			wantErr: false,
		},
		{
			name: "wrong version (optimistic locking)",
			input: command.UpdateLDSOrdinanceInput{
				ID:      createResult.ID,
				Temple:  strPtr("MANTI"),
				Version: 999,
			},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := handler.UpdateLDSOrdinance(ctx, tt.input)
			if (err != nil) != tt.wantErr {
				t.Errorf("UpdateLDSOrdinance() error = %v, wantErr %v", err, tt.wantErr)
				return
			}

			if !tt.wantErr && result.Version <= tt.input.Version {
				t.Errorf("Version not incremented: got %d, want > %d", result.Version, tt.input.Version)
			}
		})
	}
}

// TestUpdateLDSOrdinance_NoChanges tests that updating without changes returns current version.
func TestUpdateLDSOrdinance_NoChanges(t *testing.T) {
	eventStore := memory.NewEventStore()
	readStore := memory.NewReadModelStore()
	handler := command.NewHandler(eventStore, readStore)
	ctx := context.Background()

	// Create person and ordinance
	personResult, _ := handler.CreatePerson(ctx, command.CreatePersonInput{
		GivenName: "John",
		Surname:   "Doe",
		Gender:    "male",
	})

	createResult, _ := handler.CreateLDSOrdinance(ctx, command.CreateLDSOrdinanceInput{
		Type:     domain.LDSBaptism,
		PersonID: &personResult.ID,
	})

	// Update with no changes
	result, err := handler.UpdateLDSOrdinance(ctx, command.UpdateLDSOrdinanceInput{
		ID:      createResult.ID,
		Version: createResult.Version,
	})
	if err != nil {
		t.Fatalf("UpdateLDSOrdinance failed: %v", err)
	}

	if result.Version != createResult.Version {
		t.Errorf("Version changed without updates: got %d, want %d", result.Version, createResult.Version)
	}
}

// TestLDSOrdinance_InvalidStatus tests that create and update reject a status
// outside domain.AllLDSOrdinanceStatuses.
func TestLDSOrdinance_InvalidStatus(t *testing.T) {
	eventStore := memory.NewEventStore()
	readStore := memory.NewReadModelStore()
	handler := command.NewHandler(eventStore, readStore)
	ctx := context.Background()

	personResult, _ := handler.CreatePerson(ctx, command.CreatePersonInput{GivenName: "John", Surname: "Doe"})

	if _, err := handler.CreateLDSOrdinance(ctx, command.CreateLDSOrdinanceInput{
		Type:     domain.LDSBaptism,
		PersonID: &personResult.ID,
		Status:   "DONE",
	}); !errors.Is(err, command.ErrInvalidInput) {
		t.Errorf("CreateLDSOrdinance with an invalid status: err = %v, want ErrInvalidInput", err)
	}

	createResult, err := handler.CreateLDSOrdinance(ctx, command.CreateLDSOrdinanceInput{
		Type:     domain.LDSBaptism,
		PersonID: &personResult.ID,
		Status:   string(domain.LDSStatusSubmitted),
	})
	if err != nil {
		t.Fatalf("CreateLDSOrdinance failed: %v", err)
	}
	if _, err := handler.UpdateLDSOrdinance(ctx, command.UpdateLDSOrdinanceInput{
		ID:      createResult.ID,
		Status:  strPtr("DONE"),
		Version: createResult.Version,
	}); !errors.Is(err, command.ErrInvalidInput) {
		t.Errorf("UpdateLDSOrdinance with an invalid status: err = %v, want ErrInvalidInput", err)
	}
	if _, err := handler.UpdateLDSOrdinance(ctx, command.UpdateLDSOrdinanceInput{
		ID:      createResult.ID,
		Status:  strPtr(string(domain.LDSStatusCompleted)),
		Version: createResult.Version,
	}); err != nil {
		t.Errorf("UpdateLDSOrdinance with a valid status failed: %v", err)
	}
}

// An imported ordinance keeps STAT and TEMP as written: a GEDCOM 5.5.1
// status spelling and an over-long temple must not block an edit to an
// unrelated field, while a changed field is still validated.
func TestUpdateLDSOrdinance_ImportedValuesDoNotBlockUnrelatedEdit(t *testing.T) {
	const ged = `0 HEAD
1 GEDC
2 VERS 5.5.1
1 CHAR UTF-8
0 @I1@ INDI
1 NAME A /B/
1 BAPL
2 DATE 1 JAN 1900
2 TEMP SALTLAKECITY
2 PLAC Salt Lake City
2 STAT PRE-1970
0 TRLR
`
	ctx := context.Background()
	readStore := memory.NewReadModelStore()
	handler := command.NewHandler(memory.NewEventStore(), readStore)
	if _, err := handler.ImportGedcom(ctx, command.ImportGedcomInput{Filename: "t.ged", Reader: strings.NewReader(ged)}); err != nil {
		t.Fatalf("ImportGedcom failed: %v", err)
	}
	ords, _, err := readStore.ListLDSOrdinances(ctx, repository.ListOptions{Limit: 10})
	if err != nil || len(ords) != 1 {
		t.Fatalf("ListLDSOrdinances = %d ordinances, err %v; want 1", len(ords), err)
	}
	ord := ords[0]
	if ord.Status != "PRE-1970" || ord.Temple != "SALTLAKECITY" {
		t.Fatalf("imported status/temple = %q/%q, want PRE-1970/SALTLAKECITY", ord.Status, ord.Temple)
	}

	result, err := handler.UpdateLDSOrdinance(ctx, command.UpdateLDSOrdinanceInput{
		ID:      ord.ID,
		Place:   strPtr("Logan"),
		Version: ord.Version,
	})
	if err != nil {
		t.Fatalf("place-only update of an imported ordinance failed: %v", err)
	}
	if got, _ := readStore.GetLDSOrdinance(ctx, ord.ID); got.Place != "Logan" || got.Status != "PRE-1970" {
		t.Errorf("after update place/status = %q/%q, want Logan/PRE-1970", got.Place, got.Status)
	}

	if _, err := handler.UpdateLDSOrdinance(ctx, command.UpdateLDSOrdinanceInput{
		ID:      ord.ID,
		Temple:  strPtr("ANOTHERLONGTEMPLE"),
		Version: result.Version,
	}); !errors.Is(err, command.ErrInvalidInput) {
		t.Errorf("update to an over-long temple: err = %v, want ErrInvalidInput", err)
	}
}

// TestUpdateLDSOrdinance_NotFound tests updating a non-existent ordinance.
func TestUpdateLDSOrdinance_NotFound(t *testing.T) {
	eventStore := memory.NewEventStore()
	readStore := memory.NewReadModelStore()
	handler := command.NewHandler(eventStore, readStore)
	ctx := context.Background()

	// Try to update non-existent ordinance
	_, err := handler.UpdateLDSOrdinance(ctx, command.UpdateLDSOrdinanceInput{
		ID:      uuid.New(),
		Temple:  strPtr("SLAKE"),
		Version: 1,
	})
	if err != command.ErrLDSOrdinanceNotFound {
		t.Errorf("UpdateLDSOrdinance should fail with ErrLDSOrdinanceNotFound, got: %v", err)
	}
}

// TestUpdateLDSOrdinance_AllFields tests updating all optional fields.
func TestUpdateLDSOrdinance_AllFields(t *testing.T) {
	eventStore := memory.NewEventStore()
	readStore := memory.NewReadModelStore()
	handler := command.NewHandler(eventStore, readStore)
	ctx := context.Background()

	// Create person and ordinance
	personResult, _ := handler.CreatePerson(ctx, command.CreatePersonInput{
		GivenName: "John",
		Surname:   "Doe",
		Gender:    "male",
	})

	createResult, _ := handler.CreateLDSOrdinance(ctx, command.CreateLDSOrdinanceInput{
		Type:     domain.LDSBaptism,
		PersonID: &personResult.ID,
	})

	// Update all fields
	result, err := handler.UpdateLDSOrdinance(ctx, command.UpdateLDSOrdinanceInput{
		ID:      createResult.ID,
		Date:    strPtr("15 JAN 1900"),
		Place:   strPtr("Salt Lake Temple"),
		Temple:  strPtr("SLAKE"),
		Status:  strPtr("COMPLETED"),
		Version: createResult.Version,
	})
	if err != nil {
		t.Fatalf("UpdateLDSOrdinance failed: %v", err)
	}

	// Verify in read model
	ord, _ := readStore.GetLDSOrdinance(ctx, createResult.ID)
	if ord.Temple != "SLAKE" {
		t.Errorf("Temple = %s, want SLAKE", ord.Temple)
	}
	if ord.Status != "COMPLETED" {
		t.Errorf("Status = %s, want COMPLETED", ord.Status)
	}
	if ord.Place != "Salt Lake Temple" {
		t.Errorf("Place = %s, want Salt Lake Temple", ord.Place)
	}
	if result.Version <= createResult.Version {
		t.Errorf("Version not incremented: got %d, want > %d", result.Version, createResult.Version)
	}
}

// TestDeleteLDSOrdinance tests deleting an LDS ordinance.
func TestDeleteLDSOrdinance(t *testing.T) {
	eventStore := memory.NewEventStore()
	readStore := memory.NewReadModelStore()
	handler := command.NewHandler(eventStore, readStore)
	ctx := context.Background()

	// Create person and ordinance
	personResult, _ := handler.CreatePerson(ctx, command.CreatePersonInput{
		GivenName: "John",
		Surname:   "Doe",
		Gender:    "male",
	})

	createResult, _ := handler.CreateLDSOrdinance(ctx, command.CreateLDSOrdinanceInput{
		Type:     domain.LDSBaptism,
		PersonID: &personResult.ID,
	})

	// Delete ordinance
	err := handler.DeleteLDSOrdinance(ctx, createResult.ID, createResult.Version, "Test deletion")
	if err != nil {
		t.Errorf("DeleteLDSOrdinance failed: %v", err)
	}

	// Verify deleted from read model
	ord, _ := readStore.GetLDSOrdinance(ctx, createResult.ID)
	if ord != nil {
		t.Error("LDS ordinance should be deleted from read model")
	}
}

// TestDeleteLDSOrdinance_NotFound tests deleting a non-existent ordinance.
func TestDeleteLDSOrdinance_NotFound(t *testing.T) {
	eventStore := memory.NewEventStore()
	readStore := memory.NewReadModelStore()
	handler := command.NewHandler(eventStore, readStore)
	ctx := context.Background()

	// Try to delete non-existent ordinance
	err := handler.DeleteLDSOrdinance(ctx, uuid.New(), 1, "Should fail")
	if err != command.ErrLDSOrdinanceNotFound {
		t.Errorf("DeleteLDSOrdinance should fail with ErrLDSOrdinanceNotFound, got: %v", err)
	}
}

// TestDeleteLDSOrdinance_WrongVersion tests optimistic locking on delete.
func TestDeleteLDSOrdinance_WrongVersion(t *testing.T) {
	eventStore := memory.NewEventStore()
	readStore := memory.NewReadModelStore()
	handler := command.NewHandler(eventStore, readStore)
	ctx := context.Background()

	// Create person and ordinance
	personResult, _ := handler.CreatePerson(ctx, command.CreatePersonInput{
		GivenName: "John",
		Surname:   "Doe",
		Gender:    "male",
	})

	createResult, _ := handler.CreateLDSOrdinance(ctx, command.CreateLDSOrdinanceInput{
		Type:     domain.LDSBaptism,
		PersonID: &personResult.ID,
	})

	// Try to delete with wrong version
	err := handler.DeleteLDSOrdinance(ctx, createResult.ID, 999, "Should fail")
	if err != repository.ErrConcurrencyConflict {
		t.Errorf("DeleteLDSOrdinance should fail with ErrConcurrencyConflict, got: %v", err)
	}
}
