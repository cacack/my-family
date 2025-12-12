package domain

import (
	"testing"

	"github.com/google/uuid"
)

func TestNewFamily(t *testing.T) {
	f := NewFamily()

	if f.ID == uuid.Nil {
		t.Error("Expected non-nil UUID")
	}
	if f.Version != 1 {
		t.Errorf("Version = %v, want 1", f.Version)
	}
}

func TestNewFamilyWithPartners(t *testing.T) {
	p1 := uuid.New()
	p2 := uuid.New()
	f := NewFamilyWithPartners(&p1, &p2)

	if f.Partner1ID == nil || *f.Partner1ID != p1 {
		t.Error("Partner1ID not set correctly")
	}
	if f.Partner2ID == nil || *f.Partner2ID != p2 {
		t.Error("Partner2ID not set correctly")
	}
}

func TestFamily_Validate(t *testing.T) {
	p1 := uuid.New()
	p2 := uuid.New()

	tests := []struct {
		name    string
		family  *Family
		wantErr bool
	}{
		{
			name: "valid family with both partners",
			family: &Family{
				ID:         uuid.New(),
				Partner1ID: &p1,
				Partner2ID: &p2,
			},
			wantErr: false,
		},
		{
			name: "valid single parent family",
			family: &Family{
				ID:         uuid.New(),
				Partner1ID: &p1,
			},
			wantErr: false,
		},
		{
			name: "no partners",
			family: &Family{
				ID: uuid.New(),
			},
			wantErr: true,
		},
		{
			name: "same partner IDs",
			family: &Family{
				ID:         uuid.New(),
				Partner1ID: &p1,
				Partner2ID: &p1,
			},
			wantErr: true,
		},
		{
			name: "invalid relationship type",
			family: &Family{
				ID:               uuid.New(),
				Partner1ID:       &p1,
				RelationshipType: "invalid",
			},
			wantErr: true,
		},
		{
			name: "valid marriage relationship",
			family: &Family{
				ID:               uuid.New(),
				Partner1ID:       &p1,
				Partner2ID:       &p2,
				RelationshipType: RelationMarriage,
			},
			wantErr: false,
		},
		{
			name: "invalid marriage date",
			family: &Family{
				ID:           uuid.New(),
				Partner1ID:   &p1,
				MarriageDate: &GenDate{Year: intPtr(1850), Month: intPtr(13)},
			},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.family.Validate()
			if (err != nil) != tt.wantErr {
				t.Errorf("Validate() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestFamily_HasPartner(t *testing.T) {
	p1 := uuid.New()
	p2 := uuid.New()
	p3 := uuid.New()

	f := NewFamilyWithPartners(&p1, &p2)

	if !f.HasPartner(p1) {
		t.Error("Should have partner1")
	}
	if !f.HasPartner(p2) {
		t.Error("Should have partner2")
	}
	if f.HasPartner(p3) {
		t.Error("Should not have partner3")
	}
}

func TestFamily_SetMarriageDate(t *testing.T) {
	f := NewFamily()
	p1 := uuid.New()
	f.Partner1ID = &p1

	f.SetMarriageDate("1 JAN 1850")
	if f.MarriageDate == nil {
		t.Fatal("MarriageDate should not be nil")
	}
	if *f.MarriageDate.Year != 1850 {
		t.Errorf("MarriageDate.Year = %v, want 1850", *f.MarriageDate.Year)
	}

	f.SetMarriageDate("")
	if f.MarriageDate != nil {
		t.Error("MarriageDate should be nil after setting empty string")
	}
}

func TestNewFamilyChild(t *testing.T) {
	familyID := uuid.New()
	personID := uuid.New()

	fc := NewFamilyChild(familyID, personID, ChildBiological)

	if fc.FamilyID != familyID {
		t.Error("FamilyID not set correctly")
	}
	if fc.PersonID != personID {
		t.Error("PersonID not set correctly")
	}
	if fc.RelationshipType != ChildBiological {
		t.Error("RelationshipType should be biological")
	}
}

func TestNewFamilyChild_DefaultRelationship(t *testing.T) {
	familyID := uuid.New()
	personID := uuid.New()

	fc := NewFamilyChild(familyID, personID, "")

	if fc.RelationshipType != ChildBiological {
		t.Errorf("RelationshipType = %v, want biological", fc.RelationshipType)
	}
}

func TestFamilyChild_Validate(t *testing.T) {
	familyID := uuid.New()
	personID := uuid.New()

	tests := []struct {
		name    string
		fc      *FamilyChild
		wantErr bool
	}{
		{
			name:    "valid family child",
			fc:      NewFamilyChild(familyID, personID, ChildBiological),
			wantErr: false,
		},
		{
			name: "empty family ID",
			fc: &FamilyChild{
				FamilyID:         uuid.Nil,
				PersonID:         personID,
				RelationshipType: ChildBiological,
			},
			wantErr: true,
		},
		{
			name: "empty person ID",
			fc: &FamilyChild{
				FamilyID:         familyID,
				PersonID:         uuid.Nil,
				RelationshipType: ChildBiological,
			},
			wantErr: true,
		},
		{
			name: "invalid relationship type",
			fc: &FamilyChild{
				FamilyID:         familyID,
				PersonID:         personID,
				RelationshipType: "invalid",
			},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.fc.Validate()
			if (err != nil) != tt.wantErr {
				t.Errorf("Validate() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestValidateChildNotPartner(t *testing.T) {
	p1 := uuid.New()
	p2 := uuid.New()
	child := uuid.New()

	f := NewFamilyWithPartners(&p1, &p2)

	// Child that is not a partner - should be valid
	if err := ValidateChildNotPartner(f, child); err != nil {
		t.Errorf("Child should be valid: %v", err)
	}

	// Child that is partner1 - should be invalid
	if err := ValidateChildNotPartner(f, p1); err == nil {
		t.Error("Partner1 as child should be invalid")
	}

	// Child that is partner2 - should be invalid
	if err := ValidateChildNotPartner(f, p2); err == nil {
		t.Error("Partner2 as child should be invalid")
	}
}
