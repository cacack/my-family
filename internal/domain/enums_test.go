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
			want:         true,
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
		// Core person facts
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
		// Individual life events
		{
			name:     "person_burial is valid",
			factType: FactPersonBurial,
			want:     true,
		},
		{
			name:     "person_cremation is valid",
			factType: FactPersonCremation,
			want:     true,
		},
		{
			name:     "person_baptism is valid",
			factType: FactPersonBaptism,
			want:     true,
		},
		{
			name:     "person_christening is valid",
			factType: FactPersonChristening,
			want:     true,
		},
		{
			name:     "person_emigration is valid",
			factType: FactPersonEmigration,
			want:     true,
		},
		{
			name:     "person_immigration is valid",
			factType: FactPersonImmigration,
			want:     true,
		},
		{
			name:     "person_naturalization is valid",
			factType: FactPersonNaturalization,
			want:     true,
		},
		{
			name:     "person_census is valid",
			factType: FactPersonCensus,
			want:     true,
		},
		{
			name:     "person_generic_event is valid",
			factType: FactPersonGenericEvent,
			want:     true,
		},
		// Individual attributes
		{
			name:     "person_occupation is valid",
			factType: FactPersonOccupation,
			want:     true,
		},
		{
			name:     "person_residence is valid",
			factType: FactPersonResidence,
			want:     true,
		},
		{
			name:     "person_education is valid",
			factType: FactPersonEducation,
			want:     true,
		},
		{
			name:     "person_religion is valid",
			factType: FactPersonReligion,
			want:     true,
		},
		{
			name:     "person_title is valid",
			factType: FactPersonTitle,
			want:     true,
		},
		// Core family facts
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
		// Family events
		{
			name:     "family_marriage_bann is valid",
			factType: FactFamilyMarriageBann,
			want:     true,
		},
		{
			name:     "family_marriage_contract is valid",
			factType: FactFamilyMarriageContract,
			want:     true,
		},
		{
			name:     "family_marriage_license is valid",
			factType: FactFamilyMarriageLicense,
			want:     true,
		},
		{
			name:     "family_marriage_settlement is valid",
			factType: FactFamilyMarriageSettlement,
			want:     true,
		},
		{
			name:     "family_annulment is valid",
			factType: FactFamilyAnnulment,
			want:     true,
		},
		{
			name:     "family_engagement is valid",
			factType: FactFamilyEngagement,
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

func TestNameType_IsValid(t *testing.T) {
	tests := []struct {
		name     string
		nameType NameType
		want     bool
	}{
		{
			name:     "birth is valid",
			nameType: NameTypeBirth,
			want:     true,
		},
		{
			name:     "married is valid",
			nameType: NameTypeMarried,
			want:     true,
		},
		{
			name:     "aka is valid",
			nameType: NameTypeAKA,
			want:     true,
		},
		{
			name:     "empty string is valid",
			nameType: "",
			want:     true,
		},
		{
			name:     "invalid value",
			nameType: "invalid",
			want:     false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.nameType.IsValid(); got != tt.want {
				t.Errorf("NameType(%q).IsValid() = %v, want %v", tt.nameType, got, tt.want)
			}
		})
	}
}

func TestResearchStatus_IsValid(t *testing.T) {
	tests := []struct {
		name           string
		researchStatus ResearchStatus
		want           bool
	}{
		{
			name:           "certain is valid",
			researchStatus: ResearchStatusCertain,
			want:           true,
		},
		{
			name:           "probable is valid",
			researchStatus: ResearchStatusProbable,
			want:           true,
		},
		{
			name:           "possible is valid",
			researchStatus: ResearchStatusPossible,
			want:           true,
		},
		{
			name:           "unknown is valid",
			researchStatus: ResearchStatusUnknown,
			want:           true,
		},
		{
			name:           "empty string is valid",
			researchStatus: "",
			want:           true,
		},
		{
			name:           "invalid value",
			researchStatus: "invalid",
			want:           false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.researchStatus.IsValid(); got != tt.want {
				t.Errorf("ResearchStatus(%q).IsValid() = %v, want %v", tt.researchStatus, got, tt.want)
			}
		})
	}
}

func TestResearchStatus_String(t *testing.T) {
	tests := []struct {
		name           string
		researchStatus ResearchStatus
		want           string
	}{
		{
			name:           "certain string",
			researchStatus: ResearchStatusCertain,
			want:           "certain",
		},
		{
			name:           "probable string",
			researchStatus: ResearchStatusProbable,
			want:           "probable",
		},
		{
			name:           "possible string",
			researchStatus: ResearchStatusPossible,
			want:           "possible",
		},
		{
			name:           "unknown string",
			researchStatus: ResearchStatusUnknown,
			want:           "unknown",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.researchStatus.String(); got != tt.want {
				t.Errorf("ResearchStatus(%q).String() = %v, want %v", tt.researchStatus, got, tt.want)
			}
		})
	}
}

func TestParseResearchStatus(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  ResearchStatus
	}{
		{
			name:  "parse certain",
			input: "certain",
			want:  ResearchStatusCertain,
		},
		{
			name:  "parse probable",
			input: "probable",
			want:  ResearchStatusProbable,
		},
		{
			name:  "parse possible",
			input: "possible",
			want:  ResearchStatusPossible,
		},
		{
			name:  "parse unknown",
			input: "unknown",
			want:  ResearchStatusUnknown,
		},
		{
			name:  "parse empty string defaults to unknown",
			input: "",
			want:  ResearchStatusUnknown,
		},
		{
			name:  "parse invalid defaults to unknown",
			input: "invalid",
			want:  ResearchStatusUnknown,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := ParseResearchStatus(tt.input); got != tt.want {
				t.Errorf("ParseResearchStatus(%q) = %v, want %v", tt.input, got, tt.want)
			}
		})
	}
}
