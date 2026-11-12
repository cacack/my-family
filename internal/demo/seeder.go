// Package demo provides sample data seeding for demo mode.
package demo

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"

	"github.com/cacack/my-family/internal/command"
	"github.com/cacack/my-family/internal/domain"
	"github.com/cacack/my-family/internal/repository"
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

// demoCitations holds the citations created during seeding.
type demoCitations struct {
	williamBirth uuid.UUID
	annaBirth    uuid.UUID
}

// SeedDemoData populates the application with a multi-generational sample family
// tree that gives every page in the global nav something to show (#495): 18 persons,
// 6 families, 1 source with 2 citations, 1 repository, geocoded and burial events,
// a brick wall, evidence records (analyses, a conflict, a research log, a proof
// summary), deliberate data-quality problems (a duplicate pair and validation issues
// of every severity), a snapshot, and a research branch.
//
// readStore must be the store cmdHandler projects into: it locates the
// GEDCOM-imported ancestors and records the brick wall, which is not event-sourced.
// cmdHandler must be wired with branch and snapshot stores
// (command.NewHandlerWithBranches), as the API server's handler is.
func SeedDemoData(ctx context.Context, cmdHandler *command.Handler, readStore repository.ReadModelStore) error {
	people, err := seedPersons(ctx, cmdHandler)
	if err != nil {
		return err
	}

	if err := seedFamilies(ctx, cmdHandler, people); err != nil {
		return err
	}

	citations, err := seedSources(ctx, cmdHandler, people)
	if err != nil {
		return err
	}

	heinrichID, err := seedMuellerAncestors(ctx, cmdHandler, readStore, people)
	if err != nil {
		return err
	}

	if err := seedEvidence(ctx, cmdHandler, people, citations, heinrichID); err != nil {
		return err
	}

	if err := seedDuplicate(ctx, cmdHandler); err != nil {
		return err
	}

	if err := seedRepository(ctx, cmdHandler); err != nil {
		return err
	}

	return seedVersionControl(ctx, cmdHandler, heinrichID)
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

func seedSources(ctx context.Context, h *command.Handler, p *demoPersons) (*demoCitations, error) {
	sourceResult, err := h.CreateSource(ctx, command.CreateSourceInput{
		SourceType: "vital_record", Title: "Hamburg Civil Records, 1874-1920",
		Author: "Hamburg State Archives", RepositoryName: "Hamburg Staatsarchiv",
		Notes: "[DEMO DATA] Sample source demonstrating GPS-compliant sourcing.",
	})
	if err != nil {
		return nil, fmt.Errorf("create source: %w", err)
	}

	williamBirth, err := h.CreateCitation(ctx, command.CreateCitationInput{
		SourceID: sourceResult.ID, FactType: "person_birth", FactOwnerID: p.williamThompson.ID,
		Page: "Book 12, Page 47", SourceQuality: "original", InformantType: "primary",
		EvidenceType: "direct", Analysis: "[DEMO DATA] Original birth record from Hamburg civil registry.",
	})
	if err != nil {
		return nil, fmt.Errorf("create citation for William birth: %w", err)
	}

	annaBirth, err := h.CreateCitation(ctx, command.CreateCitationInput{
		SourceID: sourceResult.ID, FactType: "person_birth", FactOwnerID: p.annaMueller.ID,
		Page: "Book 15, Page 112", SourceQuality: "original", InformantType: "primary",
		EvidenceType: "direct", Analysis: "[DEMO DATA] Original birth record from Hamburg civil registry.",
	})
	if err != nil {
		return nil, fmt.Errorf("create citation for Anna birth: %w", err)
	}

	return &demoCitations{williamBirth: williamBirth.ID, annaBirth: annaBirth.ID}, nil
}

// muellerAncestorsGedcom holds Anna Mueller's ancestors. They are imported as GEDCOM
// because only an import records place coordinates (for /map), burial events (for
// /browse/cemeteries), and the inconsistent dates real imported trees carry (for
// /quality); the create-person command takes the first two and rejects the last.
// Friedrich's death precedes his birth (a validation error) and Johann's lifespan
// exceeds 120 years (a warning).
const muellerAncestorsGedcom = `0 HEAD
1 GEDC
2 VERS 5.5.1
2 FORM LINEAGE-LINKED
1 CHAR UTF-8
0 @I1@ INDI
1 NAME Heinrich /Mueller/
1 SEX M
1 BIRT
2 DATE 12 APR 1862
2 PLAC Hamburg, Germany
3 MAP
4 LATI N53.5511
4 LONG E9.9937
1 DEAT
2 DATE 3 NOV 1931
2 PLAC Hamburg, Germany
3 MAP
4 LATI N53.5511
4 LONG E9.9937
1 BURI
2 DATE 1931
2 PLAC Ohlsdorf Cemetery, Hamburg, Germany
1 NOTE [DEMO DATA] Anna Mueller's father. His own parents are unknown: a brick wall.
0 @I2@ INDI
1 NAME Margarethe /Schmidt/
1 SEX F
1 BIRT
2 DATE 1866
2 PLAC Luebeck, Germany
3 MAP
4 LATI N53.8655
4 LONG E10.6866
1 DEAT
2 DATE 1940
2 PLAC Boston, Massachusetts
3 MAP
4 LATI N42.3601
4 LONG W71.0589
1 BURI
2 DATE 1940
2 PLAC Forest Hills Cemetery, Boston, Massachusetts
1 NOTE [DEMO DATA] Anna Mueller's mother. Followed her daughter to Boston as a widow.
0 @I3@ INDI
1 NAME Friedrich /Mueller/
1 SEX M
1 BIRT
2 DATE 1870
2 PLAC Hamburg, Germany
1 DEAT
2 DATE 1868
2 PLAC Hamburg, Germany
1 NOTE [DEMO DATA] Possibly Heinrich's brother. Death year precedes birth year: a transcription error copied from another tree.
0 @I4@ INDI
1 NAME Johann /Schmidt/
1 SEX M
1 BIRT
2 DATE 1790
2 PLAC Luebeck, Germany
1 DEAT
2 DATE 1915
2 PLAC Luebeck, Germany
1 NOTE [DEMO DATA] Margarethe's father. A 125-year lifespan suggests two men were merged into one record.
0 @F1@ FAM
1 HUSB @I1@
1 WIFE @I2@
1 MARR
2 DATE 1890
2 PLAC Hamburg, Germany
0 @F2@ FAM
1 HUSB @I4@
1 CHIL @I2@
0 TRLR
`

// seedMuellerAncestors imports Anna Mueller's parents, links Anna to them, and marks
// Heinrich Mueller as a brick wall. It returns Heinrich's ID.
func seedMuellerAncestors(ctx context.Context, h *command.Handler, readStore repository.ReadModelStore, p *demoPersons) (uuid.UUID, error) {
	result, err := h.ImportGedcom(ctx, command.ImportGedcomInput{
		Filename: "demo-mueller-ancestors.ged",
		Reader:   strings.NewReader(muellerAncestorsGedcom),
	})
	if err != nil {
		return uuid.Nil, fmt.Errorf("import Mueller ancestors: %w", err)
	}
	if len(result.Errors) > 0 {
		return uuid.Nil, fmt.Errorf("import Mueller ancestors: %s", strings.Join(result.Errors, "; "))
	}

	families, err := repository.ListAll(ctx, 100, readStore.ListFamilies)
	if err != nil {
		return uuid.Nil, fmt.Errorf("list families: %w", err)
	}
	for _, f := range families {
		if f.Partner1ID == nil || f.Partner1GivenName != "Heinrich" || f.Partner1Surname != "Mueller" {
			continue
		}
		if _, err := h.LinkChild(ctx, command.LinkChildInput{FamilyID: f.ID, ChildID: p.annaMueller.ID}); err != nil {
			return uuid.Nil, fmt.Errorf("link Anna to Mueller family: %w", err)
		}
		// Brick walls are recorded in the read model only (not event-sourced).
		if err := readStore.SetBrickWall(ctx, *f.Partner1ID, "[DEMO DATA] No baptism record found for Heinrich Mueller; his parents are unknown."); err != nil {
			return uuid.Nil, fmt.Errorf("mark Heinrich Mueller as brick wall: %w", err)
		}
		return *f.Partner1ID, nil
	}
	return uuid.Nil, fmt.Errorf("imported Mueller family not found")
}

// seedEvidence records GPS evidence work: agreeing and disagreeing analyses (the
// disagreement raises an evidence conflict), a proof summary, and a research log.
func seedEvidence(ctx context.Context, h *command.Handler, p *demoPersons, c *demoCitations, heinrichID uuid.UUID) error {
	if _, err := h.CreateEvidenceAnalysis(ctx, command.CreateEvidenceAnalysisInput{
		FactType: "person_birth", SubjectID: p.williamThompson.ID, CitationIDs: []uuid.UUID{c.williamBirth},
		Conclusion: "Born about 1890 in Hamburg, Germany", ResearchStatus: "probable",
		Notes: "[DEMO DATA] Supported by the Hamburg civil birth register.",
	}); err != nil {
		return fmt.Errorf("create analysis for William birth: %w", err)
	}

	// A second, disagreeing conclusion for the same fact auto-detects a conflict.
	if _, err := h.CreateEvidenceAnalysis(ctx, command.CreateEvidenceAnalysisInput{
		FactType: "person_birth", SubjectID: p.williamThompson.ID,
		Conclusion: "Born 1888 in Altona, Germany", ResearchStatus: "possible",
		Notes: "[DEMO DATA] Family recollection, unsourced; conflicts with the civil register.",
	}); err != nil {
		return fmt.Errorf("create conflicting analysis for William birth: %w", err)
	}

	annaAnalysis, err := h.CreateEvidenceAnalysis(ctx, command.CreateEvidenceAnalysisInput{
		FactType: "person_birth", SubjectID: p.annaMueller.ID, CitationIDs: []uuid.UUID{c.annaBirth},
		Conclusion: "Born about 1895 in Hamburg, Germany", ResearchStatus: "probable",
		Notes: "[DEMO DATA] Supported by the Hamburg civil birth register.",
	})
	if err != nil {
		return fmt.Errorf("create analysis for Anna birth: %w", err)
	}

	if _, err := h.CreateProofSummary(ctx, command.CreateProofSummaryInput{
		FactType: "person_birth", SubjectID: p.annaMueller.ID,
		Conclusion:  "Anna Mueller was born about 1895 in Hamburg, Germany",
		Argument:    "[DEMO DATA] The original civil birth register entry is direct primary evidence, and no source contradicts it.",
		AnalysisIDs: []uuid.UUID{annaAnalysis.ID}, ResearchStatus: "probable",
	}); err != nil {
		return fmt.Errorf("create proof summary for Anna birth: %w", err)
	}

	if _, err := h.CreateResearchLog(ctx, command.CreateResearchLogInput{
		SubjectID: heinrichID, SubjectType: "person", Repository: "Hamburg Staatsarchiv",
		SearchDescription: "Baptism registers 1855-1870 for Heinrich Mueller and his parents",
		Outcome:           "not_found", Notes: "[DEMO DATA] No matching baptism; the search continues.",
		SearchDate: time.Date(2024, time.March, 15, 0, 0, 0, 0, time.UTC),
	}); err != nil {
		return fmt.Errorf("create research log for Heinrich Mueller: %w", err)
	}

	return nil
}

// seedDuplicate adds a second "Mary Johnson" record, which /quality reports as a
// potential duplicate (an info-severity validation issue).
func seedDuplicate(ctx context.Context, h *command.Handler) error {
	if _, err := h.CreatePerson(ctx, command.CreatePersonInput{
		GivenName: "Mary", Surname: "Johnson", Gender: "female",
		BirthDate: "1923", BirthPlace: "New York, New York",
		Notes: "[DEMO DATA] Entered from a second record; likely a duplicate of Mary Johnson (b. ABT 1922).", ResearchStatus: "possible",
	}); err != nil {
		return fmt.Errorf("create duplicate Mary Johnson: %w", err)
	}
	return nil
}

// seedRepository adds the archive the demo source is held in, for /repositories.
func seedRepository(ctx context.Context, h *command.Handler) error {
	if _, err := h.CreateRepository(ctx, command.CreateRepositoryInput{
		Name:    "Hamburg Staatsarchiv",
		Address: &domain.Address{City: "Hamburg", Country: "Germany"},
		Notes:   "[DEMO DATA] State archive holding the Hamburg civil registers.",
	}); err != nil {
		return fmt.Errorf("create repository: %w", err)
	}
	return nil
}

// seedVersionControl marks the seeded tree with a snapshot (for /snapshots) and
// opens a research branch on Heinrich Mueller's unknown parents (for /branches).
func seedVersionControl(ctx context.Context, h *command.Handler, heinrichID uuid.UUID) error {
	if _, err := h.CreateSnapshot(ctx, "Demo tree loaded",
		"[DEMO DATA] The sample tree as seeded; compare against it after making changes."); err != nil {
		return fmt.Errorf("create snapshot: %w", err)
	}

	if _, err := h.CreateBranchWithResearch(ctx, command.CreateBranchInput{
		Name:        "heinrich-mueller-parents",
		Description: "[DEMO DATA] Exploring who Heinrich Mueller's parents were.",
		Research: domain.BranchResearch{
			Hypothesis: "Heinrich Mueller was baptised in Altona rather than Hamburg, so his parents appear in the Altona registers.",
			Subjects:   []domain.BranchSubject{{Type: domain.BranchSubjectPerson, ID: heinrichID}},
		},
	}); err != nil {
		return fmt.Errorf("create research branch: %w", err)
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
