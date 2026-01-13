package api

import (
	"net/http"

	"github.com/google/uuid"
	"github.com/labstack/echo/v4"

	"github.com/cacack/my-family/internal/query"
)

// Quality and statistics response types

// QualityOverviewResponse contains aggregate quality metrics.
type QualityOverviewResponse struct {
	TotalPersons        int          `json:"total_persons"`
	AverageCompleteness float64      `json:"average_completeness"`
	RecordsWithIssues   int          `json:"records_with_issues"`
	TopIssues           []IssueCount `json:"top_issues"`
}

// IssueCount represents a data quality issue with its count.
type IssueCount struct {
	Issue string `json:"issue"`
	Count int    `json:"count"`
}

// PersonQualityResponse contains quality metrics for a single person.
type PersonQualityResponse struct {
	PersonID          string   `json:"person_id"`
	CompletenessScore float64  `json:"completeness_score"`
	Issues            []string `json:"issues"`
	Suggestions       []string `json:"suggestions"`
}

// StatisticsResponse contains tree-wide statistics.
type StatisticsResponse struct {
	TotalPersons       int                    `json:"total_persons"`
	TotalFamilies      int                    `json:"total_families"`
	DateRange          *DateRangeInfo         `json:"date_range,omitempty"`
	TopSurnames        []SurnameCountResponse `json:"top_surnames"`
	GenderDistribution GenderDistributionInfo `json:"gender_distribution"`
}

// DateRangeInfo represents the range of birth dates in the tree.
type DateRangeInfo struct {
	EarliestBirth *string `json:"earliest_birth,omitempty"`
	LatestBirth   *string `json:"latest_birth,omitempty"`
}

// SurnameCountResponse represents a surname with its count.
type SurnameCountResponse struct {
	Surname string `json:"surname"`
	Count   int    `json:"count"`
}

// GenderDistributionInfo contains counts by gender.
type GenderDistributionInfo struct {
	Male    int `json:"male"`
	Female  int `json:"female"`
	Unknown int `json:"unknown"`
}

// getQualityOverview handles GET /quality/overview
func (s *Server) getQualityOverview(c echo.Context) error {
	result, err := s.qualityService.GetQualityOverview(c.Request().Context())
	if err != nil {
		return err
	}

	response := QualityOverviewResponse{
		TotalPersons:        result.TotalPersons,
		AverageCompleteness: result.AverageCompleteness,
		RecordsWithIssues:   result.RecordsWithIssues,
		TopIssues:           make([]IssueCount, len(result.TopIssues)),
	}

	for i, issue := range result.TopIssues {
		response.TopIssues[i] = IssueCount{
			Issue: issue.Issue,
			Count: issue.Count,
		}
	}

	return c.JSON(http.StatusOK, response)
}

// getPersonQuality handles GET /quality/persons/:id
func (s *Server) getPersonQuality(c echo.Context) error {
	id, err := uuid.Parse(c.Param("id"))
	if err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, "Invalid person ID")
	}

	result, err := s.qualityService.GetPersonQuality(c.Request().Context(), id)
	if err != nil {
		if err == query.ErrNotFound {
			return echo.NewHTTPError(http.StatusNotFound, "Person not found")
		}
		return err
	}

	response := PersonQualityResponse{
		PersonID:          result.PersonID.String(),
		CompletenessScore: result.CompletenessScore,
		Issues:            result.Issues,
		Suggestions:       result.Suggestions,
	}

	// Ensure slices are never nil for JSON serialization
	if response.Issues == nil {
		response.Issues = []string{}
	}
	if response.Suggestions == nil {
		response.Suggestions = []string{}
	}

	return c.JSON(http.StatusOK, response)
}

// getStatistics handles GET /statistics
func (s *Server) getStatistics(c echo.Context) error {
	result, err := s.qualityService.GetStatistics(c.Request().Context())
	if err != nil {
		return err
	}

	response := StatisticsResponse{
		TotalPersons:  result.TotalPersons,
		TotalFamilies: result.TotalFamilies,
		TopSurnames:   make([]SurnameCountResponse, len(result.TopSurnames)),
		GenderDistribution: GenderDistributionInfo{
			Male:    result.GenderDistribution.Male,
			Female:  result.GenderDistribution.Female,
			Unknown: result.GenderDistribution.Unknown,
		},
	}

	// Convert date range if it has values
	if result.DateRange.EarliestBirth != nil || result.DateRange.LatestBirth != nil {
		response.DateRange = &DateRangeInfo{
			EarliestBirth: result.DateRange.EarliestBirth,
			LatestBirth:   result.DateRange.LatestBirth,
		}
	}

	for i, surname := range result.TopSurnames {
		response.TopSurnames[i] = SurnameCountResponse{
			Surname: surname.Surname,
			Count:   surname.Count,
		}
	}

	return c.JSON(http.StatusOK, response)
}
