package domain

import "testing"

func TestGender_IsValid(t *testing.T) {
	tests := []struct {
		name   string
		gender Gender
		want   bool
	}{
		{
			name:   "male is valid",
			gender: GenderMale,
			want:   true,
		},
		{
			name:   "female is valid",
			gender: GenderFemale,
			want:   true,
		},
		{
			name:   "unknown is valid",
			gender: GenderUnknown,
			want:   true,
		},
		{
			name:   "empty string is valid",
			gender: "",
			want:   true,
		},
		{
			name:   "invalid value",
			gender: "invalid",
			want:   false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.gender.IsValid(); got != tt.want {
				t.Errorf("Gender(%q).IsValid() = %v, want %v", tt.gender, got, tt.want)
			}
		})
	}
}

func TestRelationType_IsValid(t *testing.T) {
	tests := []struct {
		name         string
		relationType RelationType
		want         bool
	}{
		{
			name:         "marriage is valid",
			relationType: RelationMarriage,
			want:         true,
		},
		{
			name:         "partnership is valid",
			relationType: RelationPartnership,
			want:         true,
		},
		{
			name:         "unknown is valid",
			relationType: RelationUnknown,
			want:         true,
		},
		{
			name:         "empty string is valid",
			relationType: "",
			want:         true,
		},
		{
			name:         "invalid value",
			relationType: "divorce",
			want:         false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.relationType.IsValid(); got != tt.want {
				t.Errorf("RelationType(%q).IsValid() = %v, want %v", tt.relationType, got, tt.want)
			}
		})
	}
}

func TestChildRelationType_IsValid(t *testing.T) {
	tests := []struct {
		name              string
		childRelationType ChildRelationType
		want              bool
	}{
		{
			name:              "biological is valid",
			childRelationType: ChildBiological,
			want:              true,
		},
		{
			name:              "adopted is valid",
			childRelationType: ChildAdopted,
			want:              true,
		},
		{
			name:              "foster is valid",
			childRelationType: ChildFoster,
			want:              true,
		},
		{
			name:              "empty string is invalid",
			childRelationType: "",
			want:              false,
		},
		{
			name:              "invalid value",
			childRelationType: "stepchild",
			want:              false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.childRelationType.IsValid(); got != tt.want {
				t.Errorf("ChildRelationType(%q).IsValid() = %v, want %v", tt.childRelationType, got, tt.want)
			}
		})
	}
}

func TestSourceType_IsValid(t *testing.T) {
	tests := []struct {
		name       string
		sourceType SourceType
		want       bool
	}{
		{
			name:       "book is valid",
			sourceType: SourceBook,
			want:       true,
		},
		{
			name:       "archive is valid",
			sourceType: SourceArchive,
			want:       true,
		},
		{
			name:       "webpage is valid",
			sourceType: SourceWebpage,
			want:       true,
		},
		{
			name:       "census is valid",
			sourceType: SourceCensus,
			want:       true,
		},
		{
			name:       "vital_record is valid",
			sourceType: SourceVitalRecord,
			want:       true,
		},
		{
			name:       "church_record is valid",
			sourceType: SourceChurch,
			want:       true,
		},
		{
			name:       "newspaper is valid",
			sourceType: SourceNewspaper,
			want:       true,
		},
		{
			name:       "photograph is valid",
			sourceType: SourcePhotograph,
			want:       true,
		},
		{
			name:       "interview is valid",
			sourceType: SourceInterview,
			want:       true,
		},
		{
			name:       "correspondence is valid",
			sourceType: SourceCorrespond,
			want:       true,
		},
		{
			name:       "other is valid",
			sourceType: SourceOther,
			want:       true,
		},
		{
			name:       "empty string is valid",
			sourceType: "",
			want:       true,
		},
		{
			name:       "invalid value",
			sourceType: "invalid",
			want:       false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.sourceType.IsValid(); got != tt.want {
				t.Errorf("SourceType(%q).IsValid() = %v, want %v", tt.sourceType, got, tt.want)
			}
		})
	}
}

func TestSourceQuality_IsValid(t *testing.T) {
	tests := []struct {
		name          string
		sourceQuality SourceQuality
		want          bool
	}{
		{
			name:          "original is valid",
			sourceQuality: SourceOriginal,
			want:          true,
		},
		{
			name:          "derivative is valid",
			sourceQuality: SourceDerivative,
			want:          true,
		},
		{
			name:          "authored is valid",
			sourceQuality: SourceAuthored,
			want:          true,
		},
		{
			name:          "empty string is valid",
			sourceQuality: "",
			want:          true,
		},
		{
			name:          "invalid value",
			sourceQuality: "invalid",
			want:          false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.sourceQuality.IsValid(); got != tt.want {
				t.Errorf("SourceQuality(%q).IsValid() = %v, want %v", tt.sourceQuality, got, tt.want)
			}
		})
	}
}

func TestInformantType_IsValid(t *testing.T) {
	tests := []struct {
		name          string
		informantType InformantType
		want          bool
	}{
		{
			name:          "primary is valid",
			informantType: InformantPrimary,
			want:          true,
		},
		{
			name:          "secondary is valid",
			informantType: InformantSecondary,
			want:          true,
		},
		{
			name:          "indeterminate is valid",
			informantType: InformantIndeterminate,
			want:          true,
		},
		{
			name:          "empty string is valid",
			informantType: "",
			want:          true,
		},
		{
			name:          "invalid value",
			informantType: "invalid",
			want:          false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.informantType.IsValid(); got != tt.want {
				t.Errorf("InformantType(%q).IsValid() = %v, want %v", tt.informantType, got, tt.want)
			}
		})
	}
}

func TestEvidenceType_IsValid(t *testing.T) {
	tests := []struct {
		name         string
		evidenceType EvidenceType
		want         bool
	}{
		{
			name:         "direct is valid",
			evidenceType: EvidenceDirect,
			want:         true,
		},
		{
			name:         "indirect is valid",
			evidenceType: EvidenceIndirect,
			want:         true,
		},
		{
			name:         "negative is valid",
			evidenceType: EvidenceNegative,
			want:         true,
		},
		{
			name:         "empty string is valid",
			evidenceType: "",
			want:          true,
		},
		{
			name:         "invalid value",
			evidenceType: "invalid",
			want:         false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.evidenceType.IsValid(); got != tt.want {
				t.Errorf("EvidenceType(%q).IsValid() = %v, want %v", tt.evidenceType, got, tt.want)
			}
		})
	}
}

func TestFactType_IsValid(t *testing.T) {
	tests := []struct {
		name     string
		factType FactType
		want     bool
	}{
		{
			name:     "person_birth is valid",
			factType: FactPersonBirth,
			want:     true,
		},
		{
			name:     "person_death is valid",
			factType: FactPersonDeath,
			want:     true,
		},
		{
			name:     "person_name is valid",
			factType: FactPersonName,
			want:     true,
		},
		{
			name:     "person_gender is valid",
			factType: FactPersonGender,
			want:     true,
		},
		{
			name:     "family_marriage is valid",
			factType: FactFamilyMarriage,
			want:     true,
		},
		{
			name:     "family_divorce is valid",
			factType: FactFamilyDivorce,
			want:     true,
		},
		{
			name:     "empty string is valid",
			factType: "",
			want:     true,
		},
		{
			name:     "invalid value",
			factType: "invalid",
			want:     false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.factType.IsValid(); got != tt.want {
				t.Errorf("FactType(%q).IsValid() = %v, want %v", tt.factType, got, tt.want)
			}
		})
	}
}
