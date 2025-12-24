package domain

import (
	"testing"

	"github.com/google/uuid"
)

func TestNewSource(t *testing.T) {
	s := NewSource("1850 US Census", SourceCensus)

	if s.ID == uuid.Nil {
		t.Error("Expected non-nil UUID")
	}
	if s.Title != "1850 US Census" {
		t.Errorf("Title = %v, want 1850 US Census", s.Title)
	}
	if s.SourceType != SourceCensus {
		t.Errorf("SourceType = %v, want census", s.SourceType)
	}
	if s.Version != 1 {
		t.Errorf("Version = %v, want 1", s.Version)
	}
}

func TestSource_Validate(t *testing.T) {
	tests := []struct {
		name    string
		source  *Source
		wantErr bool
	}{
		{
			name:    "valid source",
			source:  NewSource("1850 US Census", SourceCensus),
			wantErr: false,
		},
		{
			name:    "empty title",
			source:  &Source{ID: uuid.New(), Title: "", SourceType: SourceBook},
			wantErr: true,
		},
		{
			name:    "invalid source type",
			source:  &Source{ID: uuid.New(), Title: "Test", SourceType: "invalid"},
			wantErr: true,
		},
		{
			name: "valid with all fields",
			source: func() *Source {
				s := NewSource("Book Title", SourceBook)
				s.Author = "John Smith"
				s.Publisher = "Acme Publishing"
				pd := ParseGenDate("1950")
				s.PublishDate = &pd
				s.URL = "https://example.com"
				s.RepositoryName = "National Archives"
				s.CollectionName = "Census Records"
				s.CallNumber = "RG-123"
				s.Notes = "Important source"
				return s
			}(),
			wantErr: false,
		},
		{
			name: "invalid publish date",
			source: func() *Source {
				s := NewSource("Test", SourceBook)
				pd := GenDate{Year: intPtr(1850), Month: intPtr(13)}
				s.PublishDate = &pd
				return s
			}(),
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.source.Validate()
			if (err != nil) != tt.wantErr {
				t.Errorf("Validate() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestNewCitation(t *testing.T) {
	sourceID := uuid.New()
	factOwnerID := uuid.New()
	c := NewCitation(sourceID, FactPersonBirth, factOwnerID)

	if c.ID == uuid.Nil {
		t.Error("Expected non-nil UUID")
	}
	if c.SourceID != sourceID {
		t.Errorf("SourceID = %v, want %v", c.SourceID, sourceID)
	}
	if c.FactType != FactPersonBirth {
		t.Errorf("FactType = %v, want person_birth", c.FactType)
	}
	if c.FactOwnerID != factOwnerID {
		t.Errorf("FactOwnerID = %v, want %v", c.FactOwnerID, factOwnerID)
	}
	if c.Version != 1 {
		t.Errorf("Version = %v, want 1", c.Version)
	}
}

func TestCitation_Validate(t *testing.T) {
	sourceID := uuid.New()
	factOwnerID := uuid.New()

	tests := []struct {
		name     string
		citation *Citation
		wantErr  bool
	}{
		{
			name:     "valid citation",
			citation: NewCitation(sourceID, FactPersonBirth, factOwnerID),
			wantErr:  false,
		},
		{
			name: "empty source_id",
			citation: &Citation{
				ID:          uuid.New(),
				SourceID:    uuid.Nil,
				FactType:    FactPersonBirth,
				FactOwnerID: factOwnerID,
			},
			wantErr: true,
		},
		{
			name: "empty fact_owner_id",
			citation: &Citation{
				ID:          uuid.New(),
				SourceID:    sourceID,
				FactType:    FactPersonBirth,
				FactOwnerID: uuid.Nil,
			},
			wantErr: true,
		},
		{
			name: "invalid fact_type",
			citation: &Citation{
				ID:          uuid.New(),
				SourceID:    sourceID,
				FactType:    "invalid",
				FactOwnerID: factOwnerID,
			},
			wantErr: true,
		},
		{
			name: "invalid source_quality",
			citation: &Citation{
				ID:            uuid.New(),
				SourceID:      sourceID,
				FactType:      FactPersonBirth,
				FactOwnerID:   factOwnerID,
				SourceQuality: "invalid",
			},
			wantErr: true,
		},
		{
			name: "invalid informant_type",
			citation: &Citation{
				ID:            uuid.New(),
				SourceID:      sourceID,
				FactType:      FactPersonBirth,
				FactOwnerID:   factOwnerID,
				InformantType: "invalid",
			},
			wantErr: true,
		},
		{
			name: "invalid evidence_type",
			citation: &Citation{
				ID:           uuid.New(),
				SourceID:     sourceID,
				FactType:     FactPersonBirth,
				FactOwnerID:  factOwnerID,
				EvidenceType: "invalid",
			},
			wantErr: true,
		},
		{
			name: "valid with all GPS fields",
			citation: &Citation{
				ID:            uuid.New(),
				SourceID:      sourceID,
				FactType:      FactPersonBirth,
				FactOwnerID:   factOwnerID,
				Page:          "123",
				Volume:        "1",
				SourceQuality: SourceOriginal,
				InformantType: InformantPrimary,
				EvidenceType:  EvidenceDirect,
				QuotedText:    "Born Jan 1, 1850",
				Analysis:      "This is primary evidence",
			},
			wantErr: false,
		},
		{
			name: "valid with empty optional enums",
			citation: &Citation{
				ID:            uuid.New(),
				SourceID:      sourceID,
				FactType:      FactPersonDeath,
				FactOwnerID:   factOwnerID,
				SourceQuality: "",
				InformantType: "",
				EvidenceType:  "",
			},
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.citation.Validate()
			if (err != nil) != tt.wantErr {
				t.Errorf("Validate() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}
