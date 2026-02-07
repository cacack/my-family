package query

import (
	"context"
	"sort"
	"strings"

	"github.com/google/uuid"

	"github.com/cacack/gedcom-go/gedcom"
	"github.com/cacack/gedcom-go/validator"
	"github.com/cacack/my-family/internal/domain"
	"github.com/cacack/my-family/internal/repository"
)

// ValidationService provides data validation using gedcom-go's validator.
// It bridges the read model data to the validator by reconstructing
// minimal gedcom structures for validation.
type ValidationService struct {
	readStore repository.ReadModelStore
}

// NewValidationService creates a new ValidationService.
func NewValidationService(readStore repository.ReadModelStore) *ValidationService {
	return &ValidationService{readStore: readStore}
}

// ValidationReport contains aggregate validation metrics and top issues.
type ValidationReport struct {
	TotalIndividuals  int                     `json:"total_individuals"`
	TotalFamilies     int                     `json:"total_families"`
	TotalSources      int                     `json:"total_sources"`
	BirthDateCoverage float64                 `json:"birth_date_coverage"`
	DeathDateCoverage float64                 `json:"death_date_coverage"`
	SourceCoverage    float64                 `json:"source_coverage"`
	ErrorCount        int                     `json:"error_count"`
	WarningCount      int                     `json:"warning_count"`
	InfoCount         int                     `json:"info_count"`
	TopIssues         []ValidationReportIssue `json:"top_issues"`
}

// ValidationReportIssue represents an issue code with its count.
type ValidationReportIssue struct {
	Code  string `json:"code"`
	Count int    `json:"count"`
}

// DuplicateResult represents a potential duplicate pair of persons.
type DuplicateResult struct {
	Person1ID    uuid.UUID `json:"person1_id"`
	Person1Name  string    `json:"person1_name"`
	Person2ID    uuid.UUID `json:"person2_id"`
	Person2Name  string    `json:"person2_name"`
	Confidence   float64   `json:"confidence"`
	MatchReasons []string  `json:"match_reasons"`
}

// ValidationIssueResult represents a single validation issue.
type ValidationIssueResult struct {
	Severity        string     `json:"severity"`
	Code            string     `json:"code"`
	Message         string     `json:"message"`
	RecordID        *uuid.UUID `json:"record_id,omitempty"`
	RelatedRecordID *uuid.UUID `json:"related_record_id,omitempty"`
}

// GetQualityReport returns a comprehensive validation quality report.
func (s *ValidationService) GetQualityReport(ctx context.Context) (*ValidationReport, error) {
	// Build gedcom document from read model
	doc, _, err := s.buildGedcomDocument(ctx)
	if err != nil {
		return nil, err
	}

	// Create validator with strict mode to get all severity levels
	v := validator.NewWithConfig(&validator.ValidatorConfig{
		Strictness: validator.StrictnessStrict,
	})

	// Generate quality report
	qr := v.QualityReport(doc)

	// Count issues by code for top issues
	issueCounts := make(map[string]int)
	allIssues := append(append(append([]validator.Issue{}, qr.Errors...), qr.Warnings...), qr.Info...)
	for _, issue := range allIssues {
		issueCounts[issue.Code]++
	}

	// Sort issues by count and take top 10
	topIssues := make([]ValidationReportIssue, 0, len(issueCounts))
	for code, count := range issueCounts {
		topIssues = append(topIssues, ValidationReportIssue{Code: code, Count: count})
	}
	sort.Slice(topIssues, func(i, j int) bool {
		return topIssues[i].Count > topIssues[j].Count
	})
	if len(topIssues) > 10 {
		topIssues = topIssues[:10]
	}

	return &ValidationReport{
		TotalIndividuals:  qr.TotalIndividuals,
		TotalFamilies:     qr.TotalFamilies,
		TotalSources:      qr.TotalSources,
		BirthDateCoverage: qr.BirthDateCoverage,
		DeathDateCoverage: qr.DeathDateCoverage,
		SourceCoverage:    qr.SourceCoverage,
		ErrorCount:        qr.ErrorCount,
		WarningCount:      qr.WarningCount,
		InfoCount:         qr.InfoCount,
		TopIssues:         topIssues,
	}, nil
}

// FindDuplicates returns potential duplicate persons with pagination.
func (s *ValidationService) FindDuplicates(ctx context.Context, limit, offset int) ([]DuplicateResult, int, error) {
	// Build gedcom document from read model
	doc, xrefMap, err := s.buildGedcomDocument(ctx)
	if err != nil {
		return nil, 0, err
	}

	// Create validator and find duplicates
	v := validator.New()
	pairs := v.FindPotentialDuplicates(doc)

	// Total count before pagination
	total := len(pairs)

	// Apply pagination
	if offset >= len(pairs) {
		return []DuplicateResult{}, total, nil
	}
	end := offset + limit
	if end > len(pairs) {
		end = len(pairs)
	}
	pairs = pairs[offset:end]

	// Convert to DuplicateResult
	results := make([]DuplicateResult, 0, len(pairs))
	for _, pair := range pairs {
		result := DuplicateResult{
			Confidence:   pair.Confidence,
			MatchReasons: pair.MatchReasons,
		}

		// Map XRef back to UUID
		if id, ok := xrefMap[pair.Individual1.XRef]; ok {
			result.Person1ID = id
			result.Person1Name = getDisplayNameFromIndividual(pair.Individual1)
		}
		if id, ok := xrefMap[pair.Individual2.XRef]; ok {
			result.Person2ID = id
			result.Person2Name = getDisplayNameFromIndividual(pair.Individual2)
		}

		results = append(results, result)
	}

	return results, total, nil
}

// GetValidationIssues returns validation issues, optionally filtered by severity.
func (s *ValidationService) GetValidationIssues(ctx context.Context, severityFilter string) ([]ValidationIssueResult, error) {
	// Build gedcom document from read model
	doc, xrefMap, err := s.buildGedcomDocument(ctx)
	if err != nil {
		return nil, err
	}

	// Create validator with strict mode to get all severity levels
	v := validator.NewWithConfig(&validator.ValidatorConfig{
		Strictness: validator.StrictnessStrict,
	})

	// Get all validation issues
	issues := v.ValidateAll(doc)

	// Filter by severity if specified
	if severityFilter != "" {
		var filtered []validator.Issue
		targetSeverity := severityStringToConst(severityFilter)
		for _, issue := range issues {
			if issue.Severity == targetSeverity {
				filtered = append(filtered, issue)
			}
		}
		issues = filtered
	}

	// Convert to ValidationIssueResult
	results := make([]ValidationIssueResult, 0, len(issues))
	for _, issue := range issues {
		result := ValidationIssueResult{
			Severity: severityConstToString(issue.Severity),
			Code:     issue.Code,
			Message:  issue.Message,
		}

		// Map XRef back to UUID
		if issue.RecordXRef != "" {
			if id, ok := xrefMap[issue.RecordXRef]; ok {
				result.RecordID = &id
			}
		}
		if issue.RelatedXRef != "" {
			if id, ok := xrefMap[issue.RelatedXRef]; ok {
				result.RelatedRecordID = &id
			}
		}

		results = append(results, result)
	}

	return results, nil
}

// buildGedcomDocument reconstructs a gedcom.Document from read model data.
// Returns the document and a map of XRef -> UUID for reverse lookup.
func (s *ValidationService) buildGedcomDocument(ctx context.Context) (*gedcom.Document, map[string]uuid.UUID, error) {
	// Load all persons using pagination to avoid truncation
	persons, err := repository.ListAll(ctx, 1000, s.readStore.ListPersons)
	if err != nil {
		return nil, nil, err
	}

	// Load all families
	families, err := repository.ListAll(ctx, 1000, s.readStore.ListFamilies)
	if err != nil {
		return nil, nil, err
	}

	// Load all sources
	sources, err := repository.ListAll(ctx, 1000, s.readStore.ListSources)
	if err != nil {
		return nil, nil, err
	}

	// Build XRef mapping
	xrefMap := make(map[string]uuid.UUID)

	// Create document
	doc := &gedcom.Document{
		Records: make([]*gedcom.Record, 0, len(persons)+len(families)+len(sources)),
		XRefMap: make(map[string]*gedcom.Record),
	}

	// Add persons as individuals
	for _, person := range persons {
		xref := personXRef(person.ID)
		xrefMap[xref] = person.ID

		individual := s.personToIndividual(person)
		record := &gedcom.Record{
			XRef:   xref,
			Type:   gedcom.RecordTypeIndividual,
			Entity: individual,
		}

		doc.Records = append(doc.Records, record)
		doc.XRefMap[xref] = record
	}

	// Add families
	for _, family := range families {
		xref := familyXRef(family.ID)
		xrefMap[xref] = family.ID

		// Load children for this family
		children, err := s.readStore.GetFamilyChildren(ctx, family.ID)
		if err != nil {
			return nil, nil, err
		}

		gedFamily := s.familyToGedcomFamily(family, children)
		record := &gedcom.Record{
			XRef:   xref,
			Type:   gedcom.RecordTypeFamily,
			Entity: gedFamily,
		}

		doc.Records = append(doc.Records, record)
		doc.XRefMap[xref] = record
	}

	// Add sources
	for _, source := range sources {
		xref := sourceXRef(source.ID)
		xrefMap[xref] = source.ID

		gedSource := s.sourceToGedcomSource(source)
		record := &gedcom.Record{
			XRef:   xref,
			Type:   gedcom.RecordTypeSource,
			Entity: gedSource,
		}

		doc.Records = append(doc.Records, record)
		doc.XRefMap[xref] = record
	}

	return doc, xrefMap, nil
}

// personToIndividual converts a PersonReadModel to a gedcom.Individual.
func (s *ValidationService) personToIndividual(person repository.PersonReadModel) *gedcom.Individual {
	individual := &gedcom.Individual{
		XRef:   personXRef(person.ID),
		Events: make([]*gedcom.Event, 0),
	}

	// Set name
	if person.GivenName != "" || person.Surname != "" {
		name := &gedcom.PersonalName{
			Given:   person.GivenName,
			Surname: person.Surname,
		}
		// Build full name in GEDCOM format
		if person.Surname != "" {
			name.Full = person.GivenName + " /" + person.Surname + "/"
		} else {
			name.Full = person.GivenName
		}
		individual.Names = []*gedcom.PersonalName{name}
	}

	// Set sex
	switch person.Gender {
	case domain.GenderMale:
		individual.Sex = "M"
	case domain.GenderFemale:
		individual.Sex = "F"
	default:
		individual.Sex = "U"
	}

	// Add birth event
	if person.BirthDateRaw != "" {
		birthEvent := &gedcom.Event{
			Type:  gedcom.EventBirth,
			Date:  person.BirthDateRaw,
			Place: person.BirthPlace,
		}
		// Parse the date
		gd := domain.ParseGenDate(person.BirthDateRaw)
		if gd.Year != nil {
			birthEvent.ParsedDate = &gedcom.Date{
				Original: person.BirthDateRaw,
				Year:     *gd.Year,
			}
			if gd.Month != nil {
				birthEvent.ParsedDate.Month = *gd.Month
			}
			if gd.Day != nil {
				birthEvent.ParsedDate.Day = *gd.Day
			}
		}
		individual.Events = append(individual.Events, birthEvent)
	}

	// Add death event
	if person.DeathDateRaw != "" {
		deathEvent := &gedcom.Event{
			Type:  gedcom.EventDeath,
			Date:  person.DeathDateRaw,
			Place: person.DeathPlace,
		}
		// Parse the date
		gd := domain.ParseGenDate(person.DeathDateRaw)
		if gd.Year != nil {
			deathEvent.ParsedDate = &gedcom.Date{
				Original: person.DeathDateRaw,
				Year:     *gd.Year,
			}
			if gd.Month != nil {
				deathEvent.ParsedDate.Month = *gd.Month
			}
			if gd.Day != nil {
				deathEvent.ParsedDate.Day = *gd.Day
			}
		}
		individual.Events = append(individual.Events, deathEvent)
	}

	return individual
}

// familyToGedcomFamily converts a FamilyReadModel to a gedcom.Family.
func (s *ValidationService) familyToGedcomFamily(family repository.FamilyReadModel, children []repository.FamilyChildReadModel) *gedcom.Family {
	gedFamily := &gedcom.Family{
		XRef:     familyXRef(family.ID),
		Events:   make([]*gedcom.Event, 0),
		Children: make([]string, 0, len(children)),
	}

	// Set spouses
	if family.Partner1ID != nil {
		gedFamily.Husband = personXRef(*family.Partner1ID)
	}
	if family.Partner2ID != nil {
		gedFamily.Wife = personXRef(*family.Partner2ID)
	}

	// Set children
	for _, child := range children {
		gedFamily.Children = append(gedFamily.Children, personXRef(child.PersonID))
	}

	// Add marriage event
	if family.MarriageDateRaw != "" {
		marriageEvent := &gedcom.Event{
			Type:  gedcom.EventMarriage,
			Date:  family.MarriageDateRaw,
			Place: family.MarriagePlace,
		}
		// Parse the date
		gd := domain.ParseGenDate(family.MarriageDateRaw)
		if gd.Year != nil {
			marriageEvent.ParsedDate = &gedcom.Date{
				Original: family.MarriageDateRaw,
				Year:     *gd.Year,
			}
			if gd.Month != nil {
				marriageEvent.ParsedDate.Month = *gd.Month
			}
			if gd.Day != nil {
				marriageEvent.ParsedDate.Day = *gd.Day
			}
		}
		gedFamily.Events = append(gedFamily.Events, marriageEvent)
	}

	return gedFamily
}

// sourceToGedcomSource converts a SourceReadModel to a gedcom.Source.
func (s *ValidationService) sourceToGedcomSource(source repository.SourceReadModel) *gedcom.Source {
	return &gedcom.Source{
		XRef:        sourceXRef(source.ID),
		Title:       source.Title,
		Author:      source.Author,
		Publication: source.Publisher,
	}
}

// Helper functions for XRef generation
func personXRef(id uuid.UUID) string {
	return "@" + id.String() + "@"
}

func familyXRef(id uuid.UUID) string {
	return "@" + id.String() + "@"
}

func sourceXRef(id uuid.UUID) string {
	return "@" + id.String() + "@"
}

// getDisplayNameFromIndividual returns a display name for a gedcom.Individual.
func getDisplayNameFromIndividual(ind *gedcom.Individual) string {
	if ind == nil || len(ind.Names) == 0 {
		return ""
	}
	name := ind.Names[0]
	if name.Full != "" {
		// Remove slashes from GEDCOM format
		return strings.ReplaceAll(strings.ReplaceAll(name.Full, "/", ""), "  ", " ")
	}
	if name.Given != "" || name.Surname != "" {
		return strings.TrimSpace(name.Given + " " + name.Surname)
	}
	return ""
}

// severityConstToString converts validator.Severity to string.
func severityConstToString(s validator.Severity) string {
	switch s {
	case validator.SeverityError:
		return "error"
	case validator.SeverityWarning:
		return "warning"
	case validator.SeverityInfo:
		return "info"
	default:
		return "unknown"
	}
}

// severityStringToConst converts string to validator.Severity.
func severityStringToConst(s string) validator.Severity {
	switch strings.ToLower(s) {
	case "error":
		return validator.SeverityError
	case "warning":
		return validator.SeverityWarning
	case "info":
		return validator.SeverityInfo
	default:
		return validator.SeverityInfo
	}
}
