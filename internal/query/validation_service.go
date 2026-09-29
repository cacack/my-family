package query

import (
	"context"
	"sort"
	"strings"

	"github.com/google/uuid"

	"github.com/cacack/gedcom-go/v2/gedcom"
	"github.com/cacack/gedcom-go/v2/validator"
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

// GetQualityReport returns a comprehensive validation quality report for the
// mainline.
func (s *ValidationService) GetQualityReport(ctx context.Context) (*ValidationReport, error) {
	return s.GetQualityReportOn(ctx, domain.MainBranchID)
}

// GetQualityReportOn returns the validation quality report of branchID's view
// of the tree (ADR-005): the mainline for domain.MainBranchID, else the
// branch's copy-on-write overlay.
func (s *ValidationService) GetQualityReportOn(ctx context.Context, branchID domain.BranchID) (*ValidationReport, error) {
	view, err := s.buildGedcomDocument(ctx, branchID)
	if err != nil {
		return nil, err
	}

	// Generate quality report with strict mode to get all severity levels
	qr := strictValidator().QualityReport(view.doc)

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

// strictValidator returns a validator reporting every severity level.
func strictValidator() *validator.Validator {
	return validator.NewWithOptions(&validator.ValidateOptions{
		Strictness: validator.StrictnessStrict,
	})
}

// FindDuplicates returns potential duplicate persons on the mainline with
// pagination.
func (s *ValidationService) FindDuplicates(ctx context.Context, limit, offset int) ([]DuplicateResult, int, error) {
	return s.FindDuplicatesOn(ctx, domain.MainBranchID, limit, offset)
}

// FindDuplicatesOn returns potential duplicate persons in branchID's view of
// the tree, with pagination.
func (s *ValidationService) FindDuplicatesOn(ctx context.Context, branchID domain.BranchID, limit, offset int) ([]DuplicateResult, int, error) {
	view, err := s.buildGedcomDocument(ctx, branchID)
	if err != nil {
		return nil, 0, err
	}
	pairs := view.duplicates()

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
	return pairs[offset:end], total, nil
}

// ValidationIssuesPage is the result of a paginated validation-issues query.
// Counts always reflect the full unfiltered issue set; Total reflects the
// post-severity-filter, pre-pagination count.
type ValidationIssuesPage struct {
	Issues       []ValidationIssueResult
	Total        int
	ErrorCount   int
	WarningCount int
	InfoCount    int
}

// GetValidationIssues returns the mainline's validation issues, optionally
// filtered by severity and paginated. limit <= 0 means "no limit"; offset < 0
// is treated as 0.
func (s *ValidationService) GetValidationIssues(ctx context.Context, severityFilter string, limit, offset int) (*ValidationIssuesPage, error) {
	return s.GetValidationIssuesOn(ctx, domain.MainBranchID, severityFilter, limit, offset)
}

// GetValidationIssuesOn is GetValidationIssues over branchID's view of the tree.
func (s *ValidationService) GetValidationIssuesOn(ctx context.Context, branchID domain.BranchID, severityFilter string, limit, offset int) (*ValidationIssuesPage, error) {
	view, err := s.buildGedcomDocument(ctx, branchID)
	if err != nil {
		return nil, err
	}

	// Get all validation issues (unfiltered) so we can compute global counts
	// even when a severity filter is applied.
	allIssues := strictValidator().ValidateAll(view.doc)

	page := &ValidationIssuesPage{
		Issues: []ValidationIssueResult{},
	}
	for _, issue := range allIssues {
		switch issue.Severity {
		case validator.SeverityError:
			page.ErrorCount++
		case validator.SeverityWarning:
			page.WarningCount++
		case validator.SeverityInfo:
			page.InfoCount++
		}
	}

	// Filter by severity if specified
	issues := allIssues
	if severityFilter != "" {
		filtered := make([]validator.Issue, 0, len(allIssues))
		targetSeverity := severityStringToConst(severityFilter)
		for _, issue := range allIssues {
			if issue.Severity == targetSeverity {
				filtered = append(filtered, issue)
			}
		}
		issues = filtered
	}

	page.Total = len(issues)

	// Apply pagination. Defensive bounds: clamp offset/limit to [0, len].
	if offset < 0 {
		offset = 0
	}
	if offset >= len(issues) {
		return page, nil
	}
	end := len(issues)
	if limit > 0 {
		end = offset + limit
		if end > len(issues) {
			end = len(issues)
		}
	}
	pageIssues := issues[offset:end]

	results := make([]ValidationIssueResult, 0, len(pageIssues))
	for _, issue := range pageIssues {
		results = append(results, view.issueResult(issue))
	}

	page.Issues = results
	return page, nil
}

// gedcomView is one scope's tree rebuilt as a gedcom.Document for the
// validator, with what it takes to map the validator's answers back: record
// ids by XRef, and each record's kind and display name.
type gedcomView struct {
	doc     *gedcom.Document
	xrefMap map[string]uuid.UUID
	labels  map[uuid.UUID]recordLabel
}

// recordLabel is a validated record's kind ("person", "family", "source") and
// display name.
type recordLabel struct {
	kind string
	name string
}

// issueResult maps a validator issue back onto record ids.
func (v *gedcomView) issueResult(issue validator.Issue) ValidationIssueResult {
	result := ValidationIssueResult{
		Severity: severityConstToString(issue.Severity),
		Code:     issue.Code,
		Message:  issue.Message,
	}
	if id, ok := v.xrefMap[issue.RecordXRef]; ok && issue.RecordXRef != "" {
		result.RecordID = &id
	}
	if id, ok := v.xrefMap[issue.RelatedXRef]; ok && issue.RelatedXRef != "" {
		result.RelatedRecordID = &id
	}
	return result
}

// duplicates returns every potential duplicate pair in the view, in the
// validator's order, mapped back onto person ids.
func (v *gedcomView) duplicates() []DuplicateResult {
	pairs := validator.New().FindPotentialDuplicates(v.doc)
	results := make([]DuplicateResult, 0, len(pairs))
	for _, pair := range pairs {
		result := DuplicateResult{
			Confidence:   pair.Confidence,
			MatchReasons: pair.MatchReasons,
		}

		// Map XRef back to UUID
		if id, ok := v.xrefMap[pair.Individual1.XRef]; ok {
			result.Person1ID = id
			result.Person1Name = getDisplayNameFromIndividual(pair.Individual1)
		}
		if id, ok := v.xrefMap[pair.Individual2.XRef]; ok {
			result.Person2ID = id
			result.Person2Name = getDisplayNameFromIndividual(pair.Individual2)
		}

		results = append(results, result)
	}
	return results
}

// buildGedcomDocument reconstructs a gedcom.Document from branchID's view of
// the read model (ADR-005). Every read is set-based: one paged list per record
// type and one read of every child link, so the cost does not grow with a
// query per family.
func (s *ValidationService) buildGedcomDocument(ctx context.Context, branchID domain.BranchID) (*gedcomView, error) {
	records, err := loadScopeRecords(ctx, s.readStore, branchID)
	if err != nil {
		return nil, err
	}
	return s.gedcomViewOf(records), nil
}

// scopeRecords is every person, family, source and child link one scope sees,
// read once so several checks over the same scope share the reads.
type scopeRecords struct {
	persons  []repository.PersonReadModel
	families []repository.FamilyReadModel
	sources  []repository.SourceReadModel
	links    []repository.FamilyChildReadModel
}

// loadScopeRecords reads branchID's persons, families, sources and child links:
// four set-based reads whatever the tree size.
func loadScopeRecords(ctx context.Context, readStore repository.ReadModelStore, branchID domain.BranchID) (*scopeRecords, error) {
	// Load all persons using pagination to avoid truncation
	persons, err := repository.ListAllOn(ctx, branchID, 1000, readStore.ListPersons)
	if err != nil {
		return nil, err
	}

	// Load all families
	families, err := repository.ListAllOn(ctx, branchID, 1000, readStore.ListFamilies)
	if err != nil {
		return nil, err
	}

	// Load all sources
	sources, err := repository.ListAllOn(ctx, branchID, 1000, readStore.ListSources)
	if err != nil {
		return nil, err
	}

	// Every child link the scope sees.
	links, err := readStore.ListAllFamilyChildren(ctx, branchID)
	if err != nil {
		return nil, err
	}
	return &scopeRecords{persons: persons, families: families, sources: sources, links: links}, nil
}

// gedcomViewOf builds the gedcom view of one scope's already-loaded records.
func (s *ValidationService) gedcomViewOf(records *scopeRecords) *gedcomView {
	persons, families, sources := records.persons, records.families, records.sources

	// Child links grouped by family, in birth order. The grouping copies the
	// links, so the shared slice keeps its order for other checks.
	childrenByFamily := make(map[uuid.UUID][]repository.FamilyChildReadModel)
	for _, link := range records.links {
		childrenByFamily[link.FamilyID] = append(childrenByFamily[link.FamilyID], link)
	}
	for _, children := range childrenByFamily {
		sortChildrenByBirthOrder(children)
	}

	view := &gedcomView{
		doc: &gedcom.Document{
			Records: make([]*gedcom.Record, 0, len(persons)+len(families)+len(sources)),
			XRefMap: make(map[string]*gedcom.Record),
		},
		xrefMap: make(map[string]uuid.UUID),
		labels:  make(map[uuid.UUID]recordLabel, len(persons)+len(families)+len(sources)),
	}
	add := func(id uuid.UUID, recordType gedcom.RecordType, entity any, label recordLabel) {
		xref := recordXRef(id)
		record := &gedcom.Record{XRef: xref, Type: recordType, Entity: entity}
		view.xrefMap[xref] = id
		view.labels[id] = label
		view.doc.Records = append(view.doc.Records, record)
		view.doc.XRefMap[xref] = record
	}

	// Add persons as individuals
	for _, person := range persons {
		add(person.ID, gedcom.RecordTypeIndividual, s.personToIndividual(person),
			recordLabel{kind: entityTypePerson, name: person.FullName})
	}

	// Add families
	for i := range families {
		family := &families[i]
		add(family.ID, gedcom.RecordTypeFamily, s.familyToGedcomFamily(*family, childrenByFamily[family.ID]),
			recordLabel{kind: entityTypeFamily, name: familyReadModelName(family)})
	}

	// Add sources
	for _, source := range sources {
		add(source.ID, gedcom.RecordTypeSource, s.sourceToGedcomSource(source),
			recordLabel{kind: entityTypeSource, name: source.Title})
	}

	return view
}

// sortChildrenByBirthOrder orders one family's children as the family page
// lists them: by sequence (unsequenced last), then name, then id, so every
// backend yields the same document.
func sortChildrenByBirthOrder(children []repository.FamilyChildReadModel) {
	sort.SliceStable(children, func(i, j int) bool {
		a, b := children[i], children[j]
		if (a.Sequence == nil) != (b.Sequence == nil) {
			return a.Sequence != nil
		}
		if a.Sequence != nil && *a.Sequence != *b.Sequence {
			return *a.Sequence < *b.Sequence
		}
		if a.PersonSurname != b.PersonSurname {
			return a.PersonSurname < b.PersonSurname
		}
		if a.PersonGivenName != b.PersonGivenName {
			return a.PersonGivenName < b.PersonGivenName
		}
		return a.PersonID.String() < b.PersonID.String()
	})
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
			Type: gedcom.EventBirth,
			Date: person.BirthDateRaw,
		}
		birthEvent.SetPlaceName(person.BirthPlace)
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
			Type: gedcom.EventDeath,
			Date: person.DeathDateRaw,
		}
		deathEvent.SetPlaceName(person.DeathPlace)
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
			Type: gedcom.EventMarriage,
			Date: family.MarriageDateRaw,
		}
		marriageEvent.SetPlaceName(family.MarriagePlace)
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

// Helper functions for XRef generation. Every record's XRef is its id, so
// the kinds share one form.
func recordXRef(id uuid.UUID) string {
	return "@" + id.String() + "@"
}

func personXRef(id uuid.UUID) string { return recordXRef(id) }

func familyXRef(id uuid.UUID) string { return recordXRef(id) }

func sourceXRef(id uuid.UUID) string { return recordXRef(id) }

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
		return fullName(name.Given, name.Surname)
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
