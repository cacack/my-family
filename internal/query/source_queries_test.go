package query_test

import (
	"context"
	"testing"

	"github.com/google/uuid"

	"github.com/cacack/my-family/internal/command"
	"github.com/cacack/my-family/internal/query"
	"github.com/cacack/my-family/internal/repository/memory"
)

// TestListSources tests listing sources with pagination.
func TestListSources(t *testing.T) {
	eventStore := memory.NewEventStore()
	readStore := memory.NewReadModelStore()
	cmdHandler := command.NewHandler(eventStore, readStore)
	queryService := query.NewSourceService(readStore)
	ctx := context.Background()

	// Create test sources
	sources := []string{"Alpha Book", "Beta Archive", "Gamma Census"}
	for _, title := range sources {
		_, err := cmdHandler.CreateSource(ctx, command.CreateSourceInput{
			SourceType: "book",
			Title:      title,
		})
		if err != nil {
			t.Fatalf("Failed to create source: %v", err)
		}
	}

	tests := []struct {
		name       string
		input      query.ListSourcesInput
		wantCount  int
		wantTotal  int
	}{
		{
			name: "list all",
			input: query.ListSourcesInput{
				Limit: 10,
			},
			wantCount: 3,
			wantTotal: 3,
		},
		{
			name: "with pagination",
			input: query.ListSourcesInput{
				Limit:  2,
				Offset: 0,
			},
			wantCount: 2,
			wantTotal: 3,
		},
		{
			name: "second page",
			input: query.ListSourcesInput{
				Limit:  2,
				Offset: 2,
			},
			wantCount: 1,
			wantTotal: 3,
		},
		{
			name: "default limit when not specified",
			input: query.ListSourcesInput{
				Limit: 0, // should default to 20
			},
			wantCount: 3,
			wantTotal: 3,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := queryService.ListSources(ctx, tt.input)
			if err != nil {
				t.Fatalf("ListSources failed: %v", err)
			}

			if len(result.Sources) != tt.wantCount {
				t.Errorf("Got %d sources, want %d", len(result.Sources), tt.wantCount)
			}

			if result.Total != tt.wantTotal {
				t.Errorf("Total = %d, want %d", result.Total, tt.wantTotal)
			}
		})
	}
}

// TestGetSource tests getting a source by ID.
func TestGetSource(t *testing.T) {
	eventStore := memory.NewEventStore()
	readStore := memory.NewReadModelStore()
	cmdHandler := command.NewHandler(eventStore, readStore)
	queryService := query.NewSourceService(readStore)
	ctx := context.Background()

	// Create source
	createResult, err := cmdHandler.CreateSource(ctx, command.CreateSourceInput{
		SourceType:  "book",
		Title:       "Test Source",
		Author:      "Test Author",
		Publisher:   "Test Publisher",
		PublishDate: "2020",
	})
	if err != nil {
		t.Fatalf("CreateSource failed: %v", err)
	}

	// Create person for citation
	personResult, _ := cmdHandler.CreatePerson(ctx, command.CreatePersonInput{
		GivenName: "John",
		Surname:   "Doe",
	})

	// Create citations
	_, _ = cmdHandler.CreateCitation(ctx, command.CreateCitationInput{
		SourceID:    createResult.ID,
		FactType:    "person_birth",
		FactOwnerID: personResult.ID,
	})
	_, _ = cmdHandler.CreateCitation(ctx, command.CreateCitationInput{
		SourceID:    createResult.ID,
		FactType:    "person_death",
		FactOwnerID: personResult.ID,
	})

	// Get source
	result, err := queryService.GetSource(ctx, createResult.ID)
	if err != nil {
		t.Fatalf("GetSource failed: %v", err)
	}

	if result.ID != createResult.ID {
		t.Errorf("ID = %v, want %v", result.ID, createResult.ID)
	}
	if result.Title != "Test Source" {
		t.Errorf("Title = %s, want Test Source", result.Title)
	}
	if result.Author == nil || *result.Author != "Test Author" {
		t.Error("Author not set correctly")
	}
	if len(result.Citations) != 2 {
		t.Errorf("Got %d citations, want 2", len(result.Citations))
	}
}

// TestGetSource_NotFound tests getting a non-existent source.
func TestGetSource_NotFound(t *testing.T) {
	readStore := memory.NewReadModelStore()
	queryService := query.NewSourceService(readStore)
	ctx := context.Background()

	_, err := queryService.GetSource(ctx, uuid.New())
	if err != query.ErrNotFound {
		t.Errorf("Expected ErrNotFound, got %v", err)
	}
}

// TestSearchSources tests searching for sources.
func TestSearchSources(t *testing.T) {
	eventStore := memory.NewEventStore()
	readStore := memory.NewReadModelStore()
	cmdHandler := command.NewHandler(eventStore, readStore)
	queryService := query.NewSourceService(readStore)
	ctx := context.Background()

	// Create test sources
	testSources := []struct {
		title  string
		author string
	}{
		{"Springfield City Directory", "John Smith"},
		{"County Census Records", "Jane Doe"},
		{"Springfield Historical Archive", "Bob Johnson"},
	}

	for _, ts := range testSources {
		_, err := cmdHandler.CreateSource(ctx, command.CreateSourceInput{
			SourceType: "book",
			Title:      ts.title,
			Author:     ts.author,
		})
		if err != nil {
			t.Fatalf("CreateSource failed: %v", err)
		}
	}

	tests := []struct {
		name      string
		query     string
		wantCount int
	}{
		{
			name:      "search by title",
			query:     "Springfield",
			wantCount: 2, // Should match both Springfield sources
		},
		{
			name:      "search by author",
			query:     "Smith",
			wantCount: 1,
		},
		{
			name:      "search no matches",
			query:     "NonExistent",
			wantCount: 0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			results, err := queryService.SearchSources(ctx, tt.query, 10)
			if err != nil {
				t.Fatalf("SearchSources failed: %v", err)
			}

			if len(results) != tt.wantCount {
				t.Errorf("Got %d results, want %d", len(results), tt.wantCount)
			}
		})
	}
}

// TestGetCitationsForPerson tests getting all citations for a person.
func TestGetCitationsForPerson(t *testing.T) {
	eventStore := memory.NewEventStore()
	readStore := memory.NewReadModelStore()
	cmdHandler := command.NewHandler(eventStore, readStore)
	queryService := query.NewSourceService(readStore)
	ctx := context.Background()

	// Create sources
	source1, _ := cmdHandler.CreateSource(ctx, command.CreateSourceInput{
		SourceType: "book",
		Title:      "Source 1",
	})
	source2, _ := cmdHandler.CreateSource(ctx, command.CreateSourceInput{
		SourceType: "book",
		Title:      "Source 2",
	})

	// Create person
	personResult, _ := cmdHandler.CreatePerson(ctx, command.CreatePersonInput{
		GivenName: "John",
		Surname:   "Doe",
	})

	// Create another person
	person2Result, _ := cmdHandler.CreatePerson(ctx, command.CreatePersonInput{
		GivenName: "Jane",
		Surname:   "Doe",
	})

	// Create citations for first person
	_, _ = cmdHandler.CreateCitation(ctx, command.CreateCitationInput{
		SourceID:    source1.ID,
		FactType:    "person_birth",
		FactOwnerID: personResult.ID,
	})
	_, _ = cmdHandler.CreateCitation(ctx, command.CreateCitationInput{
		SourceID:    source2.ID,
		FactType:    "person_death",
		FactOwnerID: personResult.ID,
	})

	// Create citation for second person
	_, _ = cmdHandler.CreateCitation(ctx, command.CreateCitationInput{
		SourceID:    source1.ID,
		FactType:    "person_birth",
		FactOwnerID: person2Result.ID,
	})

	// Get citations for first person
	citations, err := queryService.GetCitationsForPerson(ctx, personResult.ID)
	if err != nil {
		t.Fatalf("GetCitationsForPerson failed: %v", err)
	}

	if len(citations) != 2 {
		t.Errorf("Got %d citations, want 2", len(citations))
	}

	// Verify all citations belong to the right person
	for _, citation := range citations {
		if citation.FactOwnerID != personResult.ID {
			t.Errorf("Citation has wrong FactOwnerID: got %v, want %v", citation.FactOwnerID, personResult.ID)
		}
	}
}

// TestGetCitationsForFact tests getting citations for a specific fact.
func TestGetCitationsForFact(t *testing.T) {
	eventStore := memory.NewEventStore()
	readStore := memory.NewReadModelStore()
	cmdHandler := command.NewHandler(eventStore, readStore)
	queryService := query.NewSourceService(readStore)
	ctx := context.Background()

	// Create source
	sourceResult, _ := cmdHandler.CreateSource(ctx, command.CreateSourceInput{
		SourceType: "book",
		Title:      "Test Source",
	})

	// Create person
	personResult, _ := cmdHandler.CreatePerson(ctx, command.CreatePersonInput{
		GivenName: "John",
		Surname:   "Doe",
	})

	// Create citations for different facts
	_, _ = cmdHandler.CreateCitation(ctx, command.CreateCitationInput{
		SourceID:    sourceResult.ID,
		FactType:    "person_birth",
		FactOwnerID: personResult.ID,
		Page:        "10",
	})
	_, _ = cmdHandler.CreateCitation(ctx, command.CreateCitationInput{
		SourceID:    sourceResult.ID,
		FactType:    "person_birth",
		FactOwnerID: personResult.ID,
		Page:        "11",
	})
	_, _ = cmdHandler.CreateCitation(ctx, command.CreateCitationInput{
		SourceID:    sourceResult.ID,
		FactType:    "person_death",
		FactOwnerID: personResult.ID,
	})

	// Get citations for birth fact
	citations, err := queryService.GetCitationsForFact(ctx, "person_birth", personResult.ID)
	if err != nil {
		t.Fatalf("GetCitationsForFact failed: %v", err)
	}

	if len(citations) != 2 {
		t.Errorf("Got %d citations, want 2", len(citations))
	}

	// Verify all citations are for birth fact
	for _, citation := range citations {
		if citation.FactType != "person_birth" {
			t.Errorf("Citation has wrong FactType: got %s, want person_birth", citation.FactType)
		}
		if citation.FactOwnerID != personResult.ID {
			t.Errorf("Citation has wrong FactOwnerID: got %v, want %v", citation.FactOwnerID, personResult.ID)
		}
	}
}

// TestSourceDetail_WithCitations tests that source detail includes citations.
func TestSourceDetail_WithCitations(t *testing.T) {
	eventStore := memory.NewEventStore()
	readStore := memory.NewReadModelStore()
	cmdHandler := command.NewHandler(eventStore, readStore)
	queryService := query.NewSourceService(readStore)
	ctx := context.Background()

	// Create source
	sourceResult, _ := cmdHandler.CreateSource(ctx, command.CreateSourceInput{
		SourceType: "book",
		Title:      "Test Source",
	})

	// Create person
	personResult, _ := cmdHandler.CreatePerson(ctx, command.CreatePersonInput{
		GivenName: "John",
		Surname:   "Doe",
	})

	// Create citation with GPS fields
	_, _ = cmdHandler.CreateCitation(ctx, command.CreateCitationInput{
		SourceID:      sourceResult.ID,
		FactType:      "person_birth",
		FactOwnerID:   personResult.ID,
		Page:          "123",
		Volume:        "Vol 1",
		SourceQuality: "original",
		InformantType: "primary",
		EvidenceType:  "direct",
		QuotedText:    "Born January 1, 1850",
		Analysis:      "This is a reliable record",
	})

	// Get source detail
	detail, err := queryService.GetSource(ctx, sourceResult.ID)
	if err != nil {
		t.Fatalf("GetSource failed: %v", err)
	}

	if len(detail.Citations) != 1 {
		t.Fatalf("Got %d citations, want 1", len(detail.Citations))
	}

	citation := detail.Citations[0]
	if citation.Page == nil || *citation.Page != "123" {
		t.Error("Page not set correctly")
	}
	if citation.Volume == nil || *citation.Volume != "Vol 1" {
		t.Error("Volume not set correctly")
	}
	if citation.SourceQuality == nil || *citation.SourceQuality != "original" {
		t.Error("SourceQuality not set correctly")
	}
	if citation.InformantType == nil || *citation.InformantType != "primary" {
		t.Error("InformantType not set correctly")
	}
	if citation.EvidenceType == nil || *citation.EvidenceType != "direct" {
		t.Error("EvidenceType not set correctly")
	}
	if citation.QuotedText == nil || *citation.QuotedText != "Born January 1, 1850" {
		t.Error("QuotedText not set correctly")
	}
	if citation.Analysis == nil || *citation.Analysis != "This is a reliable record" {
		t.Error("Analysis not set correctly")
	}
}

// TestListSources_Sorting tests that sources are sorted correctly.
func TestListSources_Sorting(t *testing.T) {
	eventStore := memory.NewEventStore()
	readStore := memory.NewReadModelStore()
	cmdHandler := command.NewHandler(eventStore, readStore)
	queryService := query.NewSourceService(readStore)
	ctx := context.Background()

	// Create sources in non-alphabetical order
	sources := []string{"Zulu Book", "Alpha Book", "Mike Book"}
	for _, title := range sources {
		_, err := cmdHandler.CreateSource(ctx, command.CreateSourceInput{
			SourceType: "book",
			Title:      title,
		})
		if err != nil {
			t.Fatalf("CreateSource failed: %v", err)
		}
	}

	// List sources sorted by title ascending
	result, err := queryService.ListSources(ctx, query.ListSourcesInput{
		Limit:     10,
		SortBy:    "title",
		SortOrder: "asc",
	})
	if err != nil {
		t.Fatalf("ListSources failed: %v", err)
	}

	// First should be Alpha
	if result.Sources[0].Title != "Alpha Book" {
		t.Errorf("First source = %s, want Alpha Book", result.Sources[0].Title)
	}
	// Last should be Zulu
	if result.Sources[2].Title != "Zulu Book" {
		t.Errorf("Last source = %s, want Zulu Book", result.Sources[2].Title)
	}
}

// TestSearchSources_Limit tests search limit enforcement.
func TestSearchSources_Limit(t *testing.T) {
	eventStore := memory.NewEventStore()
	readStore := memory.NewReadModelStore()
	cmdHandler := command.NewHandler(eventStore, readStore)
	queryService := query.NewSourceService(readStore)
	ctx := context.Background()

	// Create many sources with "Test" in title
	for i := 0; i < 25; i++ {
		_, err := cmdHandler.CreateSource(ctx, command.CreateSourceInput{
			SourceType: "book",
			Title:      "Test Book " + string(rune('A'+i)),
		})
		if err != nil {
			t.Fatalf("CreateSource failed: %v", err)
		}
	}

	// Search with limit
	results, err := queryService.SearchSources(ctx, "Test", 10)
	if err != nil {
		t.Fatalf("SearchSources failed: %v", err)
	}

	if len(results) > 10 {
		t.Errorf("Got %d results, expected max 10", len(results))
	}
}
