package domain

import (
	"strings"
	"testing"

	"github.com/google/uuid"
)

func TestLDSOrdinanceType_IsValid(t *testing.T) {
	tests := []struct {
		name      string
		ordType   LDSOrdinanceType
		wantValid bool
	}{
		{"BAPL is valid", LDSBaptism, true},
		{"CONL is valid", LDSConfirmation, true},
		{"ENDL is valid", LDSEndowment, true},
		{"SLGC is valid", LDSSealingChild, true},
		{"SLGS is valid", LDSSealingSpouse, true},
		{"empty is invalid", LDSOrdinanceType(""), false},
		{"unknown is invalid", LDSOrdinanceType("UNKNOWN"), false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.ordType.IsValid(); got != tt.wantValid {
				t.Errorf("IsValid() = %v, want %v", got, tt.wantValid)
			}
		})
	}
}

func TestLDSOrdinanceType_IsIndividual(t *testing.T) {
	tests := []struct {
		name           string
		ordType        LDSOrdinanceType
		wantIndividual bool
	}{
		{"BAPL is individual", LDSBaptism, true},
		{"CONL is individual", LDSConfirmation, true},
		{"ENDL is individual", LDSEndowment, true},
		{"SLGC is individual", LDSSealingChild, true},
		{"SLGS is not individual", LDSSealingSpouse, false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.ordType.IsIndividual(); got != tt.wantIndividual {
				t.Errorf("IsIndividual() = %v, want %v", got, tt.wantIndividual)
			}
		})
	}
}

func TestLDSOrdinanceType_Label(t *testing.T) {
	tests := []struct {
		name      string
		ordType   LDSOrdinanceType
		wantLabel string
	}{
		{"BAPL label", LDSBaptism, "Baptism (LDS)"},
		{"CONL label", LDSConfirmation, "Confirmation (LDS)"},
		{"ENDL label", LDSEndowment, "Endowment"},
		{"SLGC label", LDSSealingChild, "Sealing to Parents"},
		{"SLGS label", LDSSealingSpouse, "Sealing to Spouse"},
		{"unknown type returns raw value", LDSOrdinanceType("UNKNOWN"), "UNKNOWN"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.ordType.Label(); got != tt.wantLabel {
				t.Errorf("Label() = %v, want %v", got, tt.wantLabel)
			}
		})
	}
}

func TestNewLDSOrdinance(t *testing.T) {
	o := NewLDSOrdinance(LDSBaptism)

	if o.ID == uuid.Nil {
		t.Error("Expected non-nil UUID")
	}
	if o.Type != LDSBaptism {
		t.Errorf("Type = %v, want %v", o.Type, LDSBaptism)
	}
	if o.Version != 1 {
		t.Errorf("Version = %v, want 1", o.Version)
	}
	if o.PersonID != nil {
		t.Error("PersonID should be nil")
	}
	if o.FamilyID != nil {
		t.Error("FamilyID should be nil")
	}
}

func TestNewLDSOrdinanceWithID(t *testing.T) {
	id := uuid.New()
	o := NewLDSOrdinanceWithID(id, LDSEndowment)

	if o.ID != id {
		t.Errorf("ID = %v, want %v", o.ID, id)
	}
	if o.Type != LDSEndowment {
		t.Errorf("Type = %v, want %v", o.Type, LDSEndowment)
	}
	if o.Version != 1 {
		t.Errorf("Version = %v, want 1", o.Version)
	}
}

func TestLDSOrdinance_Validate(t *testing.T) {
	personID := uuid.New()
	familyID := uuid.New()

	tests := []struct {
		name       string
		ordinance  *LDSOrdinance
		wantErr    bool
		errContain string
	}{
		{
			name: "valid individual ordinance with person ID",
			ordinance: func() *LDSOrdinance {
				o := NewLDSOrdinance(LDSBaptism)
				o.SetPersonID(personID)
				return o
			}(),
			wantErr: false,
		},
		{
			name: "valid spouse sealing with family ID",
			ordinance: func() *LDSOrdinance {
				o := NewLDSOrdinance(LDSSealingSpouse)
				o.SetFamilyID(familyID)
				return o
			}(),
			wantErr: false,
		},
		{
			name: "invalid type",
			ordinance: func() *LDSOrdinance {
				o := NewLDSOrdinance(LDSOrdinanceType("INVALID"))
				return o
			}(),
			wantErr:    true,
			errContain: "type",
		},
		{
			name: "individual ordinance missing person ID",
			ordinance: func() *LDSOrdinance {
				return NewLDSOrdinance(LDSBaptism)
			}(),
			wantErr:    true,
			errContain: "person_id",
		},
		{
			name: "spouse sealing missing family ID",
			ordinance: func() *LDSOrdinance {
				return NewLDSOrdinance(LDSSealingSpouse)
			}(),
			wantErr:    true,
			errContain: "family_id",
		},
		{
			name: "individual ordinance should not have family ID",
			ordinance: func() *LDSOrdinance {
				o := NewLDSOrdinance(LDSBaptism)
				o.SetPersonID(personID)
				o.SetFamilyID(familyID)
				return o
			}(),
			wantErr:    true,
			errContain: "family_id",
		},
		{
			name: "spouse sealing should not have person ID",
			ordinance: func() *LDSOrdinance {
				o := NewLDSOrdinance(LDSSealingSpouse)
				o.SetPersonID(personID)
				o.SetFamilyID(familyID)
				return o
			}(),
			wantErr:    true,
			errContain: "person_id",
		},
		{
			name: "temple code too long",
			ordinance: func() *LDSOrdinance {
				o := NewLDSOrdinance(LDSBaptism)
				o.SetPersonID(personID)
				o.Temple = "TOOLONGTEMPLE"
				return o
			}(),
			wantErr:    true,
			errContain: "temple",
		},
		{
			name: "status too long",
			ordinance: func() *LDSOrdinance {
				o := NewLDSOrdinance(LDSBaptism)
				o.SetPersonID(personID)
				o.Status = strings.Repeat("X", 51)
				return o
			}(),
			wantErr:    true,
			errContain: "status",
		},
		{
			name: "place too long",
			ordinance: func() *LDSOrdinance {
				o := NewLDSOrdinance(LDSBaptism)
				o.SetPersonID(personID)
				o.Place = strings.Repeat("X", 256)
				return o
			}(),
			wantErr:    true,
			errContain: "place",
		},
		{
			name: "valid with all optional fields",
			ordinance: func() *LDSOrdinance {
				o := NewLDSOrdinance(LDSConfirmation)
				o.SetPersonID(personID)
				o.SetTemple("SLAKE")
				o.SetStatus("COMPLETED")
				o.SetPlace("Salt Lake Temple")
				o.SetDate("15 JAN 1900")
				return o
			}(),
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.ordinance.Validate()
			if (err != nil) != tt.wantErr {
				t.Errorf("Validate() error = %v, wantErr %v", err, tt.wantErr)
			}
			if tt.wantErr && tt.errContain != "" && err != nil {
				if !strings.Contains(err.Error(), tt.errContain) {
					t.Errorf("Error should contain %q, got %q", tt.errContain, err.Error())
				}
			}
		})
	}
}

func TestLDSOrdinance_SetDate(t *testing.T) {
	o := NewLDSOrdinance(LDSBaptism)

	// Set date
	o.SetDate("1 JAN 1900")
	if o.Date == nil {
		t.Error("Date should not be nil")
	}

	// Clear date
	o.SetDate("")
	if o.Date != nil {
		t.Error("Date should be nil after clearing")
	}
}

func TestLDSOrdinance_SetPersonID(t *testing.T) {
	o := NewLDSOrdinance(LDSBaptism)
	personID := uuid.New()

	o.SetPersonID(personID)
	if o.PersonID == nil || *o.PersonID != personID {
		t.Errorf("PersonID = %v, want %v", o.PersonID, personID)
	}
}

func TestLDSOrdinance_SetFamilyID(t *testing.T) {
	o := NewLDSOrdinance(LDSSealingSpouse)
	familyID := uuid.New()

	o.SetFamilyID(familyID)
	if o.FamilyID == nil || *o.FamilyID != familyID {
		t.Errorf("FamilyID = %v, want %v", o.FamilyID, familyID)
	}
}

func TestLDSOrdinance_SetTemple(t *testing.T) {
	o := NewLDSOrdinance(LDSBaptism)

	o.SetTemple("SLAKE")
	if o.Temple != "SLAKE" {
		t.Errorf("Temple = %v, want SLAKE", o.Temple)
	}
}

func TestLDSOrdinance_SetStatus(t *testing.T) {
	o := NewLDSOrdinance(LDSBaptism)

	o.SetStatus("COMPLETED")
	if o.Status != "COMPLETED" {
		t.Errorf("Status = %v, want COMPLETED", o.Status)
	}
}

func TestLDSOrdinance_SetPlace(t *testing.T) {
	o := NewLDSOrdinance(LDSBaptism)

	o.SetPlace("Salt Lake Temple")
	if o.Place != "Salt Lake Temple" {
		t.Errorf("Place = %v, want Salt Lake Temple", o.Place)
	}
}

func TestLDSOrdinanceValidationError_Error(t *testing.T) {
	err := LDSOrdinanceValidationError{Field: "temple", Message: "temple code cannot exceed 10 characters"}
	expected := "temple: temple code cannot exceed 10 characters"
	if err.Error() != expected {
		t.Errorf("Error() = %v, want %v", err.Error(), expected)
	}
}
