package api

import (
	"net/http"
	"net/url"
	"strconv"

	"github.com/labstack/echo/v4"

	"github.com/cacack/my-family/internal/query"
)

// Browse response types

// SurnameIndexResponse contains the surname index data.
type SurnameIndexResponse struct {
	Items        []SurnameEntry `json:"items"`
	Total        int            `json:"total"`
	LetterCounts []LetterCount  `json:"letter_counts,omitempty"`
}

// SurnameEntry represents a surname with count.
type SurnameEntry struct {
	Surname string `json:"surname"`
	Count   int    `json:"count"`
}

// LetterCount represents count of surnames by starting letter.
type LetterCount struct {
	Letter string `json:"letter"`
	Count  int    `json:"count"`
}

// PlaceIndexResponse contains the place hierarchy data.
type PlaceIndexResponse struct {
	Items      []PlaceEntry `json:"items"`
	Total      int          `json:"total"`
	Breadcrumb []string     `json:"breadcrumb,omitempty"`
}

// PlaceEntry represents a place with count and hierarchy info.
type PlaceEntry struct {
	Name        string `json:"name"`
	FullName    string `json:"full_name"`
	Count       int    `json:"count"`
	HasChildren bool   `json:"has_children"`
}

// browseSurnames handles GET /browse/surnames
func (s *Server) browseSurnames(c echo.Context) error {
	letter := c.QueryParam("letter")

	result, err := s.browseService.GetSurnameIndex(c.Request().Context(), query.GetSurnameIndexInput{
		Letter: letter,
	})
	if err != nil {
		return err
	}

	response := SurnameIndexResponse{
		Items: make([]SurnameEntry, len(result.Items)),
		Total: result.Total,
	}

	for i, item := range result.Items {
		response.Items[i] = SurnameEntry{
			Surname: item.Surname,
			Count:   item.Count,
		}
	}

	if result.LetterCounts != nil {
		response.LetterCounts = make([]LetterCount, len(result.LetterCounts))
		for i, lc := range result.LetterCounts {
			response.LetterCounts[i] = LetterCount{
				Letter: lc.Letter,
				Count:  lc.Count,
			}
		}
	}

	return c.JSON(http.StatusOK, response)
}

// getPersonsBySurname handles GET /browse/surnames/:surname/persons
func (s *Server) getPersonsBySurname(c echo.Context) error {
	// URL decode the surname parameter
	surname, err := url.PathUnescape(c.Param("surname"))
	if err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, "Invalid surname parameter")
	}

	limit, _ := strconv.Atoi(c.QueryParam("limit"))
	offset, _ := strconv.Atoi(c.QueryParam("offset"))

	result, err := s.browseService.GetPersonsBySurname(c.Request().Context(), query.GetPersonsBySurnameInput{
		Surname: surname,
		Limit:   limit,
		Offset:  offset,
	})
	if err != nil {
		return err
	}

	response := PersonListResponse{
		Items:  make([]PersonResponse, len(result.Items)),
		Total:  result.Total,
		Limit:  result.Limit,
		Offset: result.Offset,
	}

	for i, person := range result.Items {
		response.Items[i] = convertPersonToResponse(person)
	}

	return c.JSON(http.StatusOK, response)
}

// browsePlaces handles GET /browse/places
func (s *Server) browsePlaces(c echo.Context) error {
	// URL decode the parent parameter
	parent, err := url.QueryUnescape(c.QueryParam("parent"))
	if err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, "Invalid parent parameter")
	}

	result, err := s.browseService.GetPlaceHierarchy(c.Request().Context(), query.GetPlaceHierarchyInput{
		Parent: parent,
	})
	if err != nil {
		return err
	}

	response := PlaceIndexResponse{
		Items:      make([]PlaceEntry, len(result.Items)),
		Total:      result.Total,
		Breadcrumb: result.Breadcrumb,
	}

	for i, item := range result.Items {
		response.Items[i] = PlaceEntry{
			Name:        item.Name,
			FullName:    item.FullName,
			Count:       item.Count,
			HasChildren: item.HasChildren,
		}
	}

	return c.JSON(http.StatusOK, response)
}

// getPersonsByPlace handles GET /browse/places/:place/persons
func (s *Server) getPersonsByPlace(c echo.Context) error {
	// URL decode the place parameter
	place, err := url.PathUnescape(c.Param("place"))
	if err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, "Invalid place parameter")
	}

	limit, _ := strconv.Atoi(c.QueryParam("limit"))
	offset, _ := strconv.Atoi(c.QueryParam("offset"))

	result, err := s.browseService.GetPersonsByPlace(c.Request().Context(), query.GetPersonsByPlaceInput{
		Place:  place,
		Limit:  limit,
		Offset: offset,
	})
	if err != nil {
		return err
	}

	response := PersonListResponse{
		Items:  make([]PersonResponse, len(result.Items)),
		Total:  result.Total,
		Limit:  result.Limit,
		Offset: result.Offset,
	}

	for i, person := range result.Items {
		response.Items[i] = convertPersonToResponse(person)
	}

	return c.JSON(http.StatusOK, response)
}
