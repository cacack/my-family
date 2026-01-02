package domain

import (
	"strings"
	"testing"

	"github.com/google/uuid"
)

func TestNewLifeEvent(t *testing.T) {
	personID := uuid.New()
	event := NewLifeEvent(personID, FactPersonBaptism)

	if event.ID == uuid.Nil {
		t.Error("ID should be generated")
	}
	if event.PersonID == nil || *event.PersonID != personID {
		t.Errorf("PersonID = %v, want %v", event.PersonID, personID)
	}
	if event.FamilyID != nil {
		t.Error("FamilyID should be nil for person events")
	}
	if event.FactType != FactPersonBaptism {
		t.Errorf("FactType = %v, want %v", event.FactType, FactPersonBaptism)
	}
	if event.Version != 1 {
		t.Errorf("Version = %v, want 1", event.Version)
	}
}

func TestNewFamilyLifeEvent(t *testing.T) {
	familyID := uuid.New()
	event := NewFamilyLifeEvent(familyID, FactFamilyEngagement)

	if event.ID == uuid.Nil {
		t.Error("ID should be generated")
	}
	if event.PersonID != nil {
		t.Error("PersonID should be nil for family events")
	}
	if event.FamilyID == nil || *event.FamilyID != familyID {
		t.Errorf("FamilyID = %v, want %v", event.FamilyID, familyID)
	}
	if event.FactType != FactFamilyEngagement {
		t.Errorf("FactType = %v, want %v", event.FactType, FactFamilyEngagement)
	}
}

func TestLifeEvent_Validate_Valid(t *testing.T) {
	tests := []struct {
		name  string
		event *LifeEvent
	}{
		{
			name:  "person event with minimum fields",
			event: NewLifeEvent(uuid.New(), FactPersonBaptism),
		},
		{
			name:  "family event with minimum fields",
			event: NewFamilyLifeEvent(uuid.New(), FactFamilyEngagement),
		},
		{
			name: "person event with all fields",
			event: func() *LifeEvent {
				e := NewLifeEvent(uuid.New(), FactPersonBurial)
				e.SetDate("1 JAN 1850")
				e.Place = "Springfield, IL"
				e.Description = "Buried at Oak Ridge Cemetery"
				e.Cause = "Natural causes"
				e.Age = "75"
				return e
			}(),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if err := tt.event.Validate(); err != nil {
				t.Errorf("Validate() error = %v, want nil", err)
			}
		})
	}
}

func TestLifeEvent_Validate_Invalid(t *testing.T) {
	tests := []struct {
		name          string
		event         *LifeEvent
		wantErrFields []string
	}{
		{
			name: "no owner set",
			event: &LifeEvent{
				ID:       uuid.New(),
				FactType: FactPersonBaptism,
				Version:  1,
			},
			wantErrFields: []string{"owner"},
		},
		{
			name: "both owners set",
			event: func() *LifeEvent {
				personID := uuid.New()
				familyID := uuid.New()
				return &LifeEvent{
					ID:       uuid.New(),
					PersonID: &personID,
					FamilyID: &familyID,
					FactType: FactPersonBaptism,
					Version:  1,
				}
			}(),
			wantErrFields: []string{"owner"},
		},
		{
			name: "empty fact type",
			event: func() *LifeEvent {
				e := NewLifeEvent(uuid.New(), "")
				return e
			}(),
			wantErrFields: []string{"fact_type"},
		},
		{
			name: "invalid fact type",
			event: func() *LifeEvent {
				e := NewLifeEvent(uuid.New(), "invalid_type")
				return e
			}(),
			wantErrFields: []string{"fact_type"},
		},
		{
			name: "invalid date",
			event: func() *LifeEvent {
				e := NewLifeEvent(uuid.New(), FactPersonBaptism)
				month := 13 // invalid month
				e.Date = &GenDate{Month: &month}
				return e
			}(),
			wantErrFields: []string{"date"},
		},
		{
			name: "place too long",
			event: func() *LifeEvent {
				e := NewLifeEvent(uuid.New(), FactPersonBaptism)
				e.Place = strings.Repeat("a", 501)
				return e
			}(),
			wantErrFields: []string{"place"},
		},
		{
			name: "description too long",
			event: func() *LifeEvent {
				e := NewLifeEvent(uuid.New(), FactPersonBaptism)
				e.Description = strings.Repeat("a", 2001)
				return e
			}(),
			wantErrFields: []string{"description"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.event.Validate()
			if err == nil {
				t.Error("Validate() error = nil, want error")
				return
			}
			for _, field := range tt.wantErrFields {
				if !strings.Contains(err.Error(), field) {
					t.Errorf("error should contain field %q, got: %v", field, err)
				}
			}
		})
	}
}

func TestLifeEvent_IsPersonEvent(t *testing.T) {
	personEvent := NewLifeEvent(uuid.New(), FactPersonBaptism)
	if !personEvent.IsPersonEvent() {
		t.Error("IsPersonEvent() = false, want true")
	}
	if personEvent.IsFamilyEvent() {
		t.Error("IsFamilyEvent() = true, want false")
	}

	familyEvent := NewFamilyLifeEvent(uuid.New(), FactFamilyEngagement)
	if familyEvent.IsPersonEvent() {
		t.Error("IsPersonEvent() = true, want false")
	}
	if !familyEvent.IsFamilyEvent() {
		t.Error("IsFamilyEvent() = false, want true")
	}
}

func TestLifeEvent_OwnerID(t *testing.T) {
	personID := uuid.New()
	personEvent := NewLifeEvent(personID, FactPersonBaptism)
	if personEvent.OwnerID() != personID {
		t.Errorf("OwnerID() = %v, want %v", personEvent.OwnerID(), personID)
	}

	familyID := uuid.New()
	familyEvent := NewFamilyLifeEvent(familyID, FactFamilyEngagement)
	if familyEvent.OwnerID() != familyID {
		t.Errorf("OwnerID() = %v, want %v", familyEvent.OwnerID(), familyID)
	}

	// Empty event
	emptyEvent := &LifeEvent{}
	if emptyEvent.OwnerID() != uuid.Nil {
		t.Errorf("OwnerID() = %v, want nil UUID", emptyEvent.OwnerID())
	}
}

func TestLifeEvent_SetDate(t *testing.T) {
	event := NewLifeEvent(uuid.New(), FactPersonBaptism)

	// Set a date
	event.SetDate("1 JAN 1850")
	if event.Date == nil {
		t.Error("Date should not be nil after SetDate")
	}
	if event.Date.Year == nil || *event.Date.Year != 1850 {
		t.Error("Date year not parsed correctly")
	}

	// Clear the date
	event.SetDate("")
	if event.Date != nil {
		t.Error("Date should be nil after SetDate with empty string")
	}
}

func TestNewAttribute(t *testing.T) {
	personID := uuid.New()
	attr := NewAttribute(personID, FactPersonOccupation, "Blacksmith")

	if attr.ID == uuid.Nil {
		t.Error("ID should be generated")
	}
	if attr.PersonID != personID {
		t.Errorf("PersonID = %v, want %v", attr.PersonID, personID)
	}
	if attr.FactType != FactPersonOccupation {
		t.Errorf("FactType = %v, want %v", attr.FactType, FactPersonOccupation)
	}
	if attr.Value != "Blacksmith" {
		t.Errorf("Value = %v, want Blacksmith", attr.Value)
	}
	if attr.Version != 1 {
		t.Errorf("Version = %v, want 1", attr.Version)
	}
}

func TestAttribute_Validate_Valid(t *testing.T) {
	tests := []struct {
		name string
		attr *Attribute
	}{
		{
			name: "minimum fields",
			attr: NewAttribute(uuid.New(), FactPersonOccupation, "Blacksmith"),
		},
		{
			name: "all fields",
			attr: func() *Attribute {
				a := NewAttribute(uuid.New(), FactPersonResidence, "123 Main St")
				a.SetDate("FROM 1850 TO 1860")
				a.Place = "Springfield, IL"
				return a
			}(),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if err := tt.attr.Validate(); err != nil {
				t.Errorf("Validate() error = %v, want nil", err)
			}
		})
	}
}

func TestAttribute_Validate_Invalid(t *testing.T) {
	tests := []struct {
		name          string
		attr          *Attribute
		wantErrFields []string
	}{
		{
			name: "empty person_id",
			attr: &Attribute{
				ID:       uuid.New(),
				FactType: FactPersonOccupation,
				Value:    "Blacksmith",
				Version:  1,
			},
			wantErrFields: []string{"person_id"},
		},
		{
			name: "empty fact_type",
			attr: &Attribute{
				ID:       uuid.New(),
				PersonID: uuid.New(),
				Value:    "Blacksmith",
				Version:  1,
			},
			wantErrFields: []string{"fact_type"},
		},
		{
			name: "invalid fact_type",
			attr: &Attribute{
				ID:       uuid.New(),
				PersonID: uuid.New(),
				FactType: "invalid_type",
				Value:    "Blacksmith",
				Version:  1,
			},
			wantErrFields: []string{"fact_type"},
		},
		{
			name: "empty value",
			attr: &Attribute{
				ID:       uuid.New(),
				PersonID: uuid.New(),
				FactType: FactPersonOccupation,
				Value:    "",
				Version:  1,
			},
			wantErrFields: []string{"value"},
		},
		{
			name: "value too long",
			attr: &Attribute{
				ID:       uuid.New(),
				PersonID: uuid.New(),
				FactType: FactPersonOccupation,
				Value:    strings.Repeat("a", 501),
				Version:  1,
			},
			wantErrFields: []string{"value"},
		},
		{
			name: "invalid date",
			attr: func() *Attribute {
				a := NewAttribute(uuid.New(), FactPersonOccupation, "Blacksmith")
				month := 13 // invalid month
				a.Date = &GenDate{Month: &month}
				return a
			}(),
			wantErrFields: []string{"date"},
		},
		{
			name: "place too long",
			attr: func() *Attribute {
				a := NewAttribute(uuid.New(), FactPersonOccupation, "Blacksmith")
				a.Place = strings.Repeat("a", 501)
				return a
			}(),
			wantErrFields: []string{"place"},
		},
		{
			name: "multiple errors",
			attr: &Attribute{
				ID:      uuid.New(),
				Version: 1,
			},
			wantErrFields: []string{"person_id", "fact_type", "value"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.attr.Validate()
			if err == nil {
				t.Error("Validate() error = nil, want error")
				return
			}
			for _, field := range tt.wantErrFields {
				if !strings.Contains(err.Error(), field) {
					t.Errorf("error should contain field %q, got: %v", field, err)
				}
			}
		})
	}
}

func TestAttribute_SetDate(t *testing.T) {
	attr := NewAttribute(uuid.New(), FactPersonOccupation, "Blacksmith")

	// Set a date
	attr.SetDate("FROM 1850 TO 1860")
	if attr.Date == nil {
		t.Error("Date should not be nil after SetDate")
	}
	if attr.Date.Qualifier != DateFrom {
		t.Errorf("Date qualifier = %v, want %v", attr.Date.Qualifier, DateFrom)
	}

	// Clear the date
	attr.SetDate("")
	if attr.Date != nil {
		t.Error("Date should be nil after SetDate with empty string")
	}
}

func TestLifeEventValidationError_Error(t *testing.T) {
	err := LifeEventValidationError{Field: "test_field", Message: "test message"}
	expected := "test_field: test message"
	if err.Error() != expected {
		t.Errorf("Error() = %v, want %v", err.Error(), expected)
	}
}

func TestAttributeValidationError_Error(t *testing.T) {
	err := AttributeValidationError{Field: "test_field", Message: "test message"}
	expected := "test_field: test message"
	if err.Error() != expected {
		t.Errorf("Error() = %v, want %v", err.Error(), expected)
	}
}
