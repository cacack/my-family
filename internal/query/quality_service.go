package query

import (
	"context"
	"sort"
	"strconv"
	"time"

	"github.com/google/uuid"

	"github.com/cacack/my-family/internal/domain"
	"github.com/cacack/my-family/internal/repository"
)

// QualityService provides data quality metrics and statistics.
type QualityService struct {
	readStore repository.ReadModelStore
}

// NewQualityService creates a new QualityService.
func NewQualityService(readStore repository.ReadModelStore) *QualityService {
	return &QualityService{readStore: readStore}
}

// QualityOverview contains aggregate quality metrics.
type QualityOverview struct {
	TotalPersons        int            `json:"total_persons"`
	AverageCompleteness float64        `json:"average_completeness"`
	RecordsWithIssues   int            `json:"records_with_issues"`
	TopIssues           []QualityIssue `json:"top_issues"`
}

// QualityIssue represents a data quality issue with count.
type QualityIssue struct {
	Issue string `json:"issue"`
	Count int    `json:"count"`
}

// PersonQuality contains quality metrics for a single person.
type PersonQuality struct {
	PersonID          uuid.UUID `json:"person_id"`
	CompletenessScore float64   `json:"completeness_score"`
	Issues            []string  `json:"issues"`
	Suggestions       []string  `json:"suggestions"`
}

// Statistics contains tree-wide statistics.
type Statistics struct {
	TotalPersons       int                `json:"total_persons"`
	TotalFamilies      int                `json:"total_families"`
	DateRange          DateRange          `json:"date_range"`
	TopSurnames        []SurnameCount     `json:"top_surnames"`
	GenderDistribution GenderDistribution `json:"gender_distribution"`
}

// DateRange represents the range of birth dates in the tree.
type DateRange struct {
	EarliestBirth *string `json:"earliest_birth,omitempty"`
	LatestBirth   *string `json:"latest_birth,omitempty"`
}

// SurnameCount represents a surname with its count.
type SurnameCount struct {
	Surname string `json:"surname"`
	Count   int    `json:"count"`
}

// GenderDistribution contains counts by gender.
type GenderDistribution struct {
	Male    int `json:"male"`
	Female  int `json:"female"`
	Unknown int `json:"unknown"`
}

// GetQualityOverview returns aggregate quality metrics for all persons.
func (s *QualityService) GetQualityOverview(ctx context.Context) (*QualityOverview, error) {
	// Get all persons
	opts := repository.ListOptions{
		Limit:  10000, // Large limit to get all persons
		Offset: 0,
	}
	persons, total, err := s.readStore.ListPersons(ctx, opts)
	if err != nil {
		return nil, err
	}

	if total == 0 {
		return &QualityOverview{
			TotalPersons:        0,
			AverageCompleteness: 0,
			RecordsWithIssues:   0,
			TopIssues:           []QualityIssue{},
		}, nil
	}

	// Calculate quality for each person
	var totalScore float64
	recordsWithIssues := 0
	issueCounts := make(map[string]int)

	for _, person := range persons {
		score, issues := s.computePersonScore(ctx, person)
		totalScore += score

		if len(issues) > 0 {
			recordsWithIssues++
			for _, issue := range issues {
				issueCounts[issue]++
			}
		}
	}

	// Calculate average
	avgCompleteness := totalScore / float64(total)

	// Sort issues by count and take top 10
	topIssues := make([]QualityIssue, 0, len(issueCounts))
	for issue, count := range issueCounts {
		topIssues = append(topIssues, QualityIssue{Issue: issue, Count: count})
	}
	sort.Slice(topIssues, func(i, j int) bool {
		return topIssues[i].Count > topIssues[j].Count
	})
	if len(topIssues) > 10 {
		topIssues = topIssues[:10]
	}

	return &QualityOverview{
		TotalPersons:        total,
		AverageCompleteness: avgCompleteness,
		RecordsWithIssues:   recordsWithIssues,
		TopIssues:           topIssues,
	}, nil
}

// GetPersonQuality returns quality metrics for a specific person.
func (s *QualityService) GetPersonQuality(ctx context.Context, id uuid.UUID) (*PersonQuality, error) {
	person, err := s.readStore.GetPerson(ctx, id)
	if err != nil {
		return nil, err
	}
	if person == nil {
		return nil, ErrNotFound
	}

	score, issues := s.computePersonScore(ctx, *person)

	// Check for orphaned status
	isOrphan, err := s.isOrphaned(ctx, id)
	if err != nil {
		return nil, err
	}
	if isOrphan {
		issues = append(issues, "No family connections")
	}

	// Generate suggestions based on issues
	suggestions := s.generateSuggestions(issues)

	return &PersonQuality{
		PersonID:          id,
		CompletenessScore: score,
		Issues:            issues,
		Suggestions:       suggestions,
	}, nil
}

// GetStatistics returns tree-wide statistics.
func (s *QualityService) GetStatistics(ctx context.Context) (*Statistics, error) {
	// Get all persons for statistics
	personOpts := repository.ListOptions{
		Limit:  10000,
		Offset: 0,
	}
	persons, totalPersons, err := s.readStore.ListPersons(ctx, personOpts)
	if err != nil {
		return nil, err
	}

	// Get all families
	familyOpts := repository.ListOptions{
		Limit:  10000,
		Offset: 0,
	}
	_, totalFamilies, err := s.readStore.ListFamilies(ctx, familyOpts)
	if err != nil {
		return nil, err
	}

	// Calculate statistics in a single pass
	var earliestYear, latestYear *int
	surnameCounts := make(map[string]int)
	genderDist := GenderDistribution{}

	for _, person := range persons {
		// Track birth year range
		if person.BirthDateRaw != "" {
			gd := domain.ParseGenDate(person.BirthDateRaw)
			if gd.Year != nil {
				if earliestYear == nil || *gd.Year < *earliestYear {
					earliestYear = gd.Year
				}
				if latestYear == nil || *gd.Year > *latestYear {
					latestYear = gd.Year
				}
			}
		}

		// Count surnames
		if person.Surname != "" {
			surnameCounts[person.Surname]++
		}

		// Count genders
		switch person.Gender {
		case domain.GenderMale:
			genderDist.Male++
		case domain.GenderFemale:
			genderDist.Female++
		default:
			genderDist.Unknown++
		}
	}

	// Build date range
	dateRange := DateRange{}
	if earliestYear != nil {
		s := intToString(*earliestYear)
		dateRange.EarliestBirth = &s
	}
	if latestYear != nil {
		s := intToString(*latestYear)
		dateRange.LatestBirth = &s
	}

	// Sort surnames by count and take top 10
	topSurnames := make([]SurnameCount, 0, len(surnameCounts))
	for surname, count := range surnameCounts {
		topSurnames = append(topSurnames, SurnameCount{Surname: surname, Count: count})
	}
	sort.Slice(topSurnames, func(i, j int) bool {
		return topSurnames[i].Count > topSurnames[j].Count
	})
	if len(topSurnames) > 10 {
		topSurnames = topSurnames[:10]
	}

	return &Statistics{
		TotalPersons:       totalPersons,
		TotalFamilies:      totalFamilies,
		DateRange:          dateRange,
		TopSurnames:        topSurnames,
		GenderDistribution: genderDist,
	}, nil
}

// computePersonScore calculates the quality score for a person.
// This algorithm is ported from the frontend (web/src/routes/analytics/+page.svelte).
//
// Scoring:
// - Birth date present: +20 points
// - Birth place present: +15 points
// - Death date present: +20 points (or +20 if living, birth > 100 years ago)
// - Death place present: +15 points (if deceased)
// - Base score is out of 70, normalized to 100
func (s *QualityService) computePersonScore(_ context.Context, person repository.PersonReadModel) (float64, []string) {
	var score float64
	var issues []string
	currentYear := time.Now().Year()

	// Parse birth date for year check
	var birthYear *int
	if person.BirthDateRaw != "" {
		gd := domain.ParseGenDate(person.BirthDateRaw)
		birthYear = gd.Year
	}

	// Has birth date: +20 points
	if person.BirthDateRaw != "" && birthYear != nil {
		score += 20
	} else {
		issues = append(issues, "Missing birth date")
	}

	// Has birth place: +15 points
	if person.BirthPlace != "" {
		score += 15
	} else {
		issues = append(issues, "Missing birth place")
	}

	// Determine if person is likely deceased (birth > 100 years ago)
	likelyDeceased := birthYear != nil && currentYear-*birthYear > 100

	// Has death date: +20 points (or +20 if living)
	if person.DeathDateRaw != "" {
		score += 20
	} else if !likelyDeceased {
		// Living person, no death expected
		score += 20
	} else {
		issues = append(issues, "Missing death date (likely deceased)")
	}

	// Has death place: +15 points (if applicable)
	if person.DeathPlace != "" {
		score += 15
	} else if person.DeathDateRaw != "" {
		// Only mark as issue if they have a death date but no place
		issues = append(issues, "Missing death place")
	} else if !likelyDeceased {
		// Living person, no death place expected
		score += 15
	}

	// Base score is out of 70, normalize to 100
	normalizedScore := (score / 70) * 100

	return normalizedScore, issues
}

// isOrphaned checks if a person has no family connections.
func (s *QualityService) isOrphaned(ctx context.Context, personID uuid.UUID) (bool, error) {
	// Check if person is a partner in any family
	families, err := s.readStore.GetFamiliesForPerson(ctx, personID)
	if err != nil {
		return false, err
	}
	if len(families) > 0 {
		return false, nil
	}

	// Check if person is a child in any family
	childFamily, err := s.readStore.GetChildFamily(ctx, personID)
	if err != nil {
		return false, err
	}
	if childFamily != nil {
		return false, nil
	}

	return true, nil
}

// generateSuggestions creates suggestions based on the identified issues.
func (s *QualityService) generateSuggestions(issues []string) []string {
	suggestions := make([]string, 0, len(issues))

	for _, issue := range issues {
		switch issue {
		case "Missing birth date":
			suggestions = append(suggestions, "Add birth date from vital records, census, or family sources")
		case "Missing birth place":
			suggestions = append(suggestions, "Research birth location in census records or vital records")
		case "Missing death date (likely deceased)":
			suggestions = append(suggestions, "Search death records, obituaries, or cemetery records")
		case "Missing death place":
			suggestions = append(suggestions, "Check death certificate or obituary for location")
		case "No family connections":
			suggestions = append(suggestions, "Link to existing family or create new family relationships")
		}
	}

	return suggestions
}

// intToString converts an int to a string.
func intToString(n int) string {
	return strconv.Itoa(n)
}
