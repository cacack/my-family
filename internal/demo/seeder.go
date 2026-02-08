// Package demo provides sample data seeding for demo mode.
package demo

import (
	"context"
	"fmt"

	"github.com/google/uuid"

	"github.com/cacack/my-family/internal/command"
)

// personRef holds the result of creating a person for linking into families.
type personRef struct {
	ID      uuid.UUID
	Version int64
}

// demoPersons holds all persons created during seeding.
type demoPersons struct {
	williamThompson   personRef
	annaMueller       personRef
	johnThompson      personRef
	maryJohnson       personRef
	elizabethThompson personRef
	robertThompson    personRef
	susanWilliams     personRef
	michaelThompson   personRef
	jenniferBrown     personRef
	emilyThompson     personRef
	davidThompson     personRef
	jamesThompson     personRef
	sarahThompson     personRef
}

// SeedDemoData populates the application with a multi-generational sample family tree.
// It creates 13 persons across 4 generations, 4 families, 1 source, and 2 citations.
func SeedDemoData(ctx context.Context, cmdHandler *command.Handler) error {
	people, err := seedPersons(ctx, cmdHandler)
	if err != nil {
		return err
	}

	if err := seedFamilies(ctx, cmdHandler, people); err != nil {
		return err
	}

	return seedSources(ctx, cmdHandler, people)
}

func seedPersons(ctx context.Context, h *command.Handler) (*demoPersons, error) {
	p := &demoPersons{}
	var err error

	// Generation 1: Great-grandparents
	p.williamThompson, err = createPerson(ctx, h, command.CreatePersonInput{
		GivenName: "William", Surname: "Thompson", Gender: "male",
		BirthDate: "ABT 1890", BirthPlace: "Hamburg, Germany",
		DeathDate: "1965", DeathPlace: "Boston, Massachusetts",
		Notes: "[DEMO DATA] Immigrated to the United States around 1910.", ResearchStatus: "probable",
	})
	if err != nil {
		return nil, fmt.Errorf("create William Thompson: %w", err)
	}

	p.annaMueller, err = createPerson(ctx, h, command.CreatePersonInput{
		GivenName: "Anna", Surname: "Mueller", Gender: "female",
		BirthDate: "ABT 1895", BirthPlace: "Hamburg, Germany",
		DeathDate: "1972", DeathPlace: "Boston, Massachusetts",
		Notes: "[DEMO DATA] Maiden name Mueller, married William Thompson.", ResearchStatus: "probable",
	})
	if err != nil {
		return nil, fmt.Errorf("create Anna Mueller: %w", err)
	}

	// Generation 2: Grandparents
	p.johnThompson, err = createPerson(ctx, h, command.CreatePersonInput{
		GivenName: "John", Surname: "Thompson", Gender: "male",
		BirthDate: "15 MAR 1920", BirthPlace: "Boston, Massachusetts",
		DeathDate: "10 JAN 2005", DeathPlace: "Chicago, Illinois",
		Notes: "[DEMO DATA] First-generation American. Served in WWII.", ResearchStatus: "certain",
	})
	if err != nil {
		return nil, fmt.Errorf("create John Thompson: %w", err)
	}

	p.maryJohnson, err = createPerson(ctx, h, command.CreatePersonInput{
		GivenName: "Mary", Surname: "Johnson", Gender: "female",
		BirthDate: "ABT 1922", BirthPlace: "New York, New York",
		DeathDate: "2010", DeathPlace: "Chicago, Illinois",
		Notes: "[DEMO DATA] Met John Thompson during the war years.", ResearchStatus: "probable",
	})
	if err != nil {
		return nil, fmt.Errorf("create Mary Johnson: %w", err)
	}

	p.elizabethThompson, err = createPerson(ctx, h, command.CreatePersonInput{
		GivenName: "Elizabeth", Surname: "Thompson", Gender: "female",
		BirthDate: "1948", BirthPlace: "Chicago, Illinois",
		Notes: "[DEMO DATA] John and Mary's daughter.", ResearchStatus: "certain",
	})
	if err != nil {
		return nil, fmt.Errorf("create Elizabeth Thompson: %w", err)
	}

	// Generation 3: Parents
	p.robertThompson, err = createPerson(ctx, h, command.CreatePersonInput{
		GivenName: "Robert", Surname: "Thompson", Gender: "male",
		BirthDate: "22 JUN 1945", BirthPlace: "Chicago, Illinois",
		Notes: "[DEMO DATA] Eldest son of John and Mary Thompson.", ResearchStatus: "certain",
	})
	if err != nil {
		return nil, fmt.Errorf("create Robert Thompson: %w", err)
	}

	p.susanWilliams, err = createPerson(ctx, h, command.CreatePersonInput{
		GivenName: "Susan", Surname: "Williams", Gender: "female",
		BirthDate: "1948", BirthPlace: "Los Angeles, California",
		Notes: "[DEMO DATA] Married Robert Thompson in 1968.", ResearchStatus: "certain",
	})
	if err != nil {
		return nil, fmt.Errorf("create Susan Williams: %w", err)
	}

	// Generation 4: Current generation
	p.michaelThompson, err = createPerson(ctx, h, command.CreatePersonInput{
		GivenName: "Michael", Surname: "Thompson", Gender: "male",
		BirthDate: "14 FEB 1970", BirthPlace: "Denver, Colorado",
		Notes: "[DEMO DATA] Son of Robert and Susan Thompson.", ResearchStatus: "certain",
	})
	if err != nil {
		return nil, fmt.Errorf("create Michael Thompson: %w", err)
	}

	p.jenniferBrown, err = createPerson(ctx, h, command.CreatePersonInput{
		GivenName: "Jennifer", Surname: "Brown", Gender: "female",
		BirthDate: "1972", BirthPlace: "Seattle, Washington",
		Notes: "[DEMO DATA] Married Michael Thompson in 1995.", ResearchStatus: "certain",
	})
	if err != nil {
		return nil, fmt.Errorf("create Jennifer Brown: %w", err)
	}

	p.emilyThompson, err = createPerson(ctx, h, command.CreatePersonInput{
		GivenName: "Emily", Surname: "Thompson", Gender: "female",
		BirthDate: "3 SEP 1998", BirthPlace: "Denver, Colorado",
		Notes: "[DEMO DATA] Eldest child of Michael and Jennifer.", ResearchStatus: "certain",
	})
	if err != nil {
		return nil, fmt.Errorf("create Emily Thompson: %w", err)
	}

	p.davidThompson, err = createPerson(ctx, h, command.CreatePersonInput{
		GivenName: "David", Surname: "Thompson", Gender: "male",
		BirthDate: "2001", BirthPlace: "Denver, Colorado",
		Notes: "[DEMO DATA] Youngest child of Michael and Jennifer.", ResearchStatus: "certain",
	})
	if err != nil {
		return nil, fmt.Errorf("create David Thompson: %w", err)
	}

	p.jamesThompson, err = createPerson(ctx, h, command.CreatePersonInput{
		GivenName: "James", Surname: "Thompson", Gender: "male",
		BirthDate: "1973", BirthPlace: "Unknown",
		Notes: "[DEMO DATA] Adopted by Robert and Susan Thompson.", ResearchStatus: "possible",
	})
	if err != nil {
		return nil, fmt.Errorf("create James Thompson: %w", err)
	}

	p.sarahThompson, err = createPerson(ctx, h, command.CreatePersonInput{
		GivenName: "Sarah", Surname: "Thompson", Gender: "female",
		BirthDate: "1950", BirthPlace: "Chicago, Illinois",
		Notes: "[DEMO DATA] John and Mary's youngest daughter.", ResearchStatus: "certain",
	})
	if err != nil {
		return nil, fmt.Errorf("create Sarah Thompson: %w", err)
	}

	return p, nil
}

func seedFamilies(ctx context.Context, h *command.Handler, p *demoPersons) error {
	// Family 1: William & Anna (Gen 1)
	family1, err := h.CreateFamily(ctx, command.CreateFamilyInput{
		Partner1ID: &p.williamThompson.ID, Partner2ID: &p.annaMueller.ID,
		RelationshipType: "marriage", MarriageDate: "ABT 1918", MarriagePlace: "Boston, Massachusetts",
	})
	if err != nil {
		return fmt.Errorf("create family William+Anna: %w", err)
	}
	if _, err := h.LinkChild(ctx, command.LinkChildInput{FamilyID: family1.ID, ChildID: p.johnThompson.ID}); err != nil {
		return fmt.Errorf("link John to family 1: %w", err)
	}

	// Family 2: John & Mary (Gen 2)
	family2, err := h.CreateFamily(ctx, command.CreateFamilyInput{
		Partner1ID: &p.johnThompson.ID, Partner2ID: &p.maryJohnson.ID,
		RelationshipType: "marriage", MarriageDate: "1942", MarriagePlace: "Chicago, Illinois",
	})
	if err != nil {
		return fmt.Errorf("create family John+Mary: %w", err)
	}
	for _, child := range []personRef{p.robertThompson, p.elizabethThompson, p.sarahThompson} {
		if _, err := h.LinkChild(ctx, command.LinkChildInput{FamilyID: family2.ID, ChildID: child.ID}); err != nil {
			return fmt.Errorf("link child to family 2: %w", err)
		}
	}

	// Family 3: Robert & Susan (Gen 3)
	family3, err := h.CreateFamily(ctx, command.CreateFamilyInput{
		Partner1ID: &p.robertThompson.ID, Partner2ID: &p.susanWilliams.ID,
		RelationshipType: "marriage", MarriageDate: "1968", MarriagePlace: "Denver, Colorado",
	})
	if err != nil {
		return fmt.Errorf("create family Robert+Susan: %w", err)
	}
	if _, err := h.LinkChild(ctx, command.LinkChildInput{FamilyID: family3.ID, ChildID: p.michaelThompson.ID}); err != nil {
		return fmt.Errorf("link Michael to family 3: %w", err)
	}
	if _, err := h.LinkChild(ctx, command.LinkChildInput{FamilyID: family3.ID, ChildID: p.jamesThompson.ID, RelationType: "adopted"}); err != nil {
		return fmt.Errorf("link James to family 3: %w", err)
	}

	// Family 4: Michael & Jennifer (Gen 4)
	family4, err := h.CreateFamily(ctx, command.CreateFamilyInput{
		Partner1ID: &p.michaelThompson.ID, Partner2ID: &p.jenniferBrown.ID,
		RelationshipType: "marriage", MarriageDate: "1995", MarriagePlace: "Seattle, Washington",
	})
	if err != nil {
		return fmt.Errorf("create family Michael+Jennifer: %w", err)
	}
	for _, child := range []personRef{p.emilyThompson, p.davidThompson} {
		if _, err := h.LinkChild(ctx, command.LinkChildInput{FamilyID: family4.ID, ChildID: child.ID}); err != nil {
			return fmt.Errorf("link child to family 4: %w", err)
		}
	}

	return nil
}

func seedSources(ctx context.Context, h *command.Handler, p *demoPersons) error {
	sourceResult, err := h.CreateSource(ctx, command.CreateSourceInput{
		SourceType: "vital_record", Title: "Hamburg Civil Records, 1874-1920",
		Author: "Hamburg State Archives", RepositoryName: "Hamburg Staatsarchiv",
		Notes: "[DEMO DATA] Sample source demonstrating GPS-compliant sourcing.",
	})
	if err != nil {
		return fmt.Errorf("create source: %w", err)
	}

	if _, err := h.CreateCitation(ctx, command.CreateCitationInput{
		SourceID: sourceResult.ID, FactType: "person_birth", FactOwnerID: p.williamThompson.ID,
		Page: "Book 12, Page 47", SourceQuality: "original", InformantType: "primary",
		EvidenceType: "direct", Analysis: "[DEMO DATA] Original birth record from Hamburg civil registry.",
	}); err != nil {
		return fmt.Errorf("create citation for William birth: %w", err)
	}

	if _, err := h.CreateCitation(ctx, command.CreateCitationInput{
		SourceID: sourceResult.ID, FactType: "person_birth", FactOwnerID: p.annaMueller.ID,
		Page: "Book 15, Page 112", SourceQuality: "original", InformantType: "primary",
		EvidenceType: "direct", Analysis: "[DEMO DATA] Original birth record from Hamburg civil registry.",
	}); err != nil {
		return fmt.Errorf("create citation for Anna birth: %w", err)
	}

	return nil
}

func createPerson(ctx context.Context, h *command.Handler, input command.CreatePersonInput) (personRef, error) {
	result, err := h.CreatePerson(ctx, input)
	if err != nil {
		return personRef{}, err
	}
	return personRef{ID: result.ID, Version: result.Version}, nil
}
