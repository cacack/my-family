package api

import (
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/labstack/echo/v4"

	"github.com/cacack/my-family/internal/command"
	"github.com/cacack/my-family/internal/exporter"
	"github.com/cacack/my-family/internal/gedcom"
	"github.com/cacack/my-family/internal/query"
)

// Person request/response types

type CreatePersonRequest struct {
	GivenName      string  `json:"given_name" validate:"required,min=1,max=100"`
	Surname        string  `json:"surname" validate:"required,min=1,max=100"`
	Gender         *string `json:"gender,omitempty"`
	BirthDate      *string `json:"birth_date,omitempty"`
	BirthPlace     *string `json:"birth_place,omitempty"`
	DeathDate      *string `json:"death_date,omitempty"`
	DeathPlace     *string `json:"death_place,omitempty"`
	Notes          *string `json:"notes,omitempty"`
	ResearchStatus *string `json:"research_status,omitempty"`
}

type UpdatePersonRequest struct {
	GivenName      *string `json:"given_name,omitempty"`
	Surname        *string `json:"surname,omitempty"`
	Gender         *string `json:"gender,omitempty"`
	BirthDate      *string `json:"birth_date,omitempty"`
	BirthPlace     *string `json:"birth_place,omitempty"`
	DeathDate      *string `json:"death_date,omitempty"`
	DeathPlace     *string `json:"death_place,omitempty"`
	Notes          *string `json:"notes,omitempty"`
	ResearchStatus *string `json:"research_status,omitempty"`
	Version        int64   `json:"version" validate:"required"`
}

type PersonResponse struct {
	ID             string  `json:"id"`
	GivenName      string  `json:"given_name"`
	Surname        string  `json:"surname"`
	Gender         *string `json:"gender,omitempty"`
	BirthDate      any     `json:"birth_date,omitempty"`
	BirthPlace     *string `json:"birth_place,omitempty"`
	DeathDate      any     `json:"death_date,omitempty"`
	DeathPlace     *string `json:"death_place,omitempty"`
	Notes          *string `json:"notes,omitempty"`
	ResearchStatus *string `json:"research_status,omitempty"`
	Version        int64   `json:"version"`
}

type PersonDetailResponse struct {
	PersonResponse
	Names             []PersonNameResponse    `json:"names,omitempty"`
	FamiliesAsPartner []FamilySummaryResponse `json:"families_as_partner,omitempty"`
	FamilyAsChild     *FamilySummaryResponse  `json:"family_as_child,omitempty"`
}

// PersonName request/response types

type PersonNameResponse struct {
	ID            string `json:"id"`
	PersonID      string `json:"person_id"`
	GivenName     string `json:"given_name"`
	Surname       string `json:"surname"`
	FullName      string `json:"full_name"`
	NamePrefix    string `json:"name_prefix,omitempty"`
	NameSuffix    string `json:"name_suffix,omitempty"`
	SurnamePrefix string `json:"surname_prefix,omitempty"`
	Nickname      string `json:"nickname,omitempty"`
	NameType      string `json:"name_type"`
	IsPrimary     bool   `json:"is_primary"`
}

type CreatePersonNameRequest struct {
	GivenName     string `json:"given_name" validate:"required,min=1,max=100"`
	Surname       string `json:"surname" validate:"max=100"`
	NamePrefix    string `json:"name_prefix,omitempty"`
	NameSuffix    string `json:"name_suffix,omitempty"`
	SurnamePrefix string `json:"surname_prefix,omitempty"`
	Nickname      string `json:"nickname,omitempty"`
	NameType      string `json:"name_type" validate:"required"`
	IsPrimary     *bool  `json:"is_primary,omitempty"`
}

type UpdatePersonNameRequest struct {
	GivenName     *string `json:"given_name,omitempty"`
	Surname       *string `json:"surname,omitempty"`
	NamePrefix    *string `json:"name_prefix,omitempty"`
	NameSuffix    *string `json:"name_suffix,omitempty"`
	SurnamePrefix *string `json:"surname_prefix,omitempty"`
	Nickname      *string `json:"nickname,omitempty"`
	NameType      *string `json:"name_type,omitempty"`
	IsPrimary     *bool   `json:"is_primary,omitempty"`
}

type PersonNameListResponse struct {
	Items []PersonNameResponse `json:"items"`
	Total int                  `json:"total"`
}

type FamilySummaryResponse struct {
	ID               string  `json:"id"`
	Partner1Name     *string `json:"partner1_name,omitempty"`
	Partner2Name     *string `json:"partner2_name,omitempty"`
	RelationshipType *string `json:"relationship_type,omitempty"`
}

type PersonListResponse struct {
	Items  []PersonResponse `json:"items"`
	Total  int              `json:"total"`
	Limit  int              `json:"limit"`
	Offset int              `json:"offset"`
}

// listPersons handles GET /persons
func (s *Server) listPersons(c echo.Context) error {
	limit, _ := strconv.Atoi(c.QueryParam("limit"))
	offset, _ := strconv.Atoi(c.QueryParam("offset"))
	sort := c.QueryParam("sort")
	order := c.QueryParam("order")

	result, err := s.personService.ListPersons(c.Request().Context(), query.ListPersonsInput{
		Limit:  limit,
		Offset: offset,
		Sort:   sort,
		Order:  order,
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

	for i, p := range result.Items {
		response.Items[i] = convertPersonToResponse(p)
	}

	return c.JSON(http.StatusOK, response)
}

// createPerson handles POST /persons
func (s *Server) createPerson(c echo.Context) error {
	var req CreatePersonRequest
	if err := c.Bind(&req); err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, "Invalid request body")
	}

	input := command.CreatePersonInput{
		GivenName: req.GivenName,
		Surname:   req.Surname,
	}
	if req.Gender != nil {
		input.Gender = *req.Gender
	}
	if req.BirthDate != nil {
		input.BirthDate = *req.BirthDate
	}
	if req.BirthPlace != nil {
		input.BirthPlace = *req.BirthPlace
	}
	if req.DeathDate != nil {
		input.DeathDate = *req.DeathDate
	}
	if req.DeathPlace != nil {
		input.DeathPlace = *req.DeathPlace
	}
	if req.Notes != nil {
		input.Notes = *req.Notes
	}
	if req.ResearchStatus != nil {
		input.ResearchStatus = *req.ResearchStatus
	}

	result, err := s.commandHandler.CreatePerson(c.Request().Context(), input)
	if err != nil {
		return err
	}

	// Create the primary name for this person
	// This creates the initial name entry associated with the person
	_, _ = s.commandHandler.AddName(c.Request().Context(), command.AddNameInput{
		PersonID:  result.ID,
		GivenName: req.GivenName,
		Surname:   req.Surname,
		IsPrimary: true,
	})

	// Fetch the created person to return full response
	person, err := s.personService.GetPerson(c.Request().Context(), result.ID)
	if err != nil {
		return err
	}

	return c.JSON(http.StatusCreated, convertPersonToResponse(person.Person))
}

// getPerson handles GET /persons/:id
func (s *Server) getPerson(c echo.Context) error {
	id, err := uuid.Parse(c.Param("id"))
	if err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, "Invalid person ID")
	}

	person, err := s.personService.GetPerson(c.Request().Context(), id)
	if err != nil {
		return err
	}

	response := PersonDetailResponse{
		PersonResponse: convertPersonToResponse(person.Person),
	}

	// Include names in the response
	for _, n := range person.Names {
		response.Names = append(response.Names, convertPersonNameToResponse(n))
	}

	for _, f := range person.FamiliesAsPartner {
		response.FamiliesAsPartner = append(response.FamiliesAsPartner, convertFamilySummaryToResponse(f))
	}
	if person.FamilyAsChild != nil {
		fs := convertFamilySummaryToResponse(*person.FamilyAsChild)
		response.FamilyAsChild = &fs
	}

	return c.JSON(http.StatusOK, response)
}

// updatePerson handles PUT /persons/:id
func (s *Server) updatePerson(c echo.Context) error {
	id, err := uuid.Parse(c.Param("id"))
	if err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, "Invalid person ID")
	}

	var req UpdatePersonRequest
	if err := c.Bind(&req); err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, "Invalid request body")
	}

	input := command.UpdatePersonInput{
		ID:             id,
		GivenName:      req.GivenName,
		Surname:        req.Surname,
		Gender:         req.Gender,
		BirthDate:      req.BirthDate,
		BirthPlace:     req.BirthPlace,
		DeathDate:      req.DeathDate,
		DeathPlace:     req.DeathPlace,
		Notes:          req.Notes,
		ResearchStatus: req.ResearchStatus,
		Version:        req.Version,
	}

	_, err = s.commandHandler.UpdatePerson(c.Request().Context(), input)
	if err != nil {
		return err
	}

	// Fetch the updated person to return full response
	person, err := s.personService.GetPerson(c.Request().Context(), id)
	if err != nil {
		return err
	}

	return c.JSON(http.StatusOK, convertPersonToResponse(person.Person))
}

// deletePerson handles DELETE /persons/:id
func (s *Server) deletePerson(c echo.Context) error {
	id, err := uuid.Parse(c.Param("id"))
	if err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, "Invalid person ID")
	}

	// Get version from query param or body
	version, _ := strconv.ParseInt(c.QueryParam("version"), 10, 64)
	if version == 0 {
		// Try to get current version
		person, err := s.personService.GetPerson(c.Request().Context(), id)
		if err != nil {
			return err
		}
		version = person.Version
	}

	err = s.commandHandler.DeletePerson(c.Request().Context(), command.DeletePersonInput{
		ID:      id,
		Version: version,
	})
	if err != nil {
		return err
	}

	return c.NoContent(http.StatusNoContent)
}

// searchPersons handles GET /search
func (s *Server) searchPersons(c echo.Context) error {
	q := c.QueryParam("q")
	if len(q) < 2 {
		return echo.NewHTTPError(http.StatusBadRequest, "Search query must be at least 2 characters")
	}

	fuzzy := c.QueryParam("fuzzy") == "true"
	limit, _ := strconv.Atoi(c.QueryParam("limit"))

	result, err := s.personService.SearchPersons(c.Request().Context(), query.SearchPersonsInput{
		Query: q,
		Fuzzy: fuzzy,
		Limit: limit,
	})
	if err != nil {
		return err
	}

	type SearchResultResponse struct {
		PersonResponse
		Score float64 `json:"score"`
	}

	response := struct {
		Items []SearchResultResponse `json:"items"`
		Total int                    `json:"total"`
		Query string                 `json:"query"`
	}{
		Items: make([]SearchResultResponse, len(result.Items)),
		Total: result.Total,
		Query: result.Query,
	}

	for i, r := range result.Items {
		response.Items[i] = SearchResultResponse{
			PersonResponse: convertPersonToResponse(r.Person),
			Score:          r.Score,
		}
	}

	return c.JSON(http.StatusOK, response)
}

// Helper function to convert query result to response.
func convertPersonToResponse(p query.Person) PersonResponse {
	resp := PersonResponse{
		ID:        p.ID.String(),
		GivenName: p.GivenName,
		Surname:   p.Surname,
		Gender:    p.Gender,
		Version:   p.Version,
	}

	if p.BirthDate != nil {
		resp.BirthDate = p.BirthDate
	}
	if p.BirthPlace != nil {
		resp.BirthPlace = p.BirthPlace
	}
	if p.DeathDate != nil {
		resp.DeathDate = p.DeathDate
	}
	if p.DeathPlace != nil {
		resp.DeathPlace = p.DeathPlace
	}
	if p.Notes != nil {
		resp.Notes = p.Notes
	}
	if p.ResearchStatus != nil {
		resp.ResearchStatus = p.ResearchStatus
	}

	return resp
}

func convertFamilySummaryToResponse(f query.FamilySummary) FamilySummaryResponse {
	return FamilySummaryResponse{
		ID:               f.ID.String(),
		Partner1Name:     f.Partner1Name,
		Partner2Name:     f.Partner2Name,
		RelationshipType: f.RelationshipType,
	}
}

func convertPersonNameToResponse(n query.PersonName) PersonNameResponse {
	return PersonNameResponse{
		ID:            n.ID.String(),
		PersonID:      "", // PersonID is not available in query.PersonName, omit or handle differently
		GivenName:     n.GivenName,
		Surname:       n.Surname,
		FullName:      n.FullName,
		NamePrefix:    n.NamePrefix,
		NameSuffix:    n.NameSuffix,
		SurnamePrefix: n.SurnamePrefix,
		Nickname:      n.Nickname,
		NameType:      n.NameType,
		IsPrimary:     n.IsPrimary,
	}
}

// Family request/response types

type CreateFamilyRequest struct {
	Partner1ID       *string `json:"partner1_id,omitempty"`
	Partner2ID       *string `json:"partner2_id,omitempty"`
	RelationshipType *string `json:"relationship_type,omitempty"`
	MarriageDate     *string `json:"marriage_date,omitempty"`
	MarriagePlace    *string `json:"marriage_place,omitempty"`
}

type UpdateFamilyRequest struct {
	Partner1ID       *string `json:"partner1_id,omitempty"`
	Partner2ID       *string `json:"partner2_id,omitempty"`
	RelationshipType *string `json:"relationship_type,omitempty"`
	MarriageDate     *string `json:"marriage_date,omitempty"`
	MarriagePlace    *string `json:"marriage_place,omitempty"`
	Version          int64   `json:"version" validate:"required"`
}

type AddChildRequest struct {
	ChildID          string  `json:"child_id" validate:"required"`
	RelationshipType *string `json:"relationship_type,omitempty"`
}

type FamilyResponse struct {
	ID               string  `json:"id"`
	Partner1ID       *string `json:"partner1_id,omitempty"`
	Partner1Name     *string `json:"partner1_name,omitempty"`
	Partner2ID       *string `json:"partner2_id,omitempty"`
	Partner2Name     *string `json:"partner2_name,omitempty"`
	RelationshipType *string `json:"relationship_type,omitempty"`
	MarriageDate     any     `json:"marriage_date,omitempty"`
	MarriagePlace    *string `json:"marriage_place,omitempty"`
	ChildCount       int     `json:"child_count"`
	Version          int64   `json:"version"`
}

type FamilyDetailResponse struct {
	FamilyResponse
	Children []FamilyChildResponse `json:"children,omitempty"`
}

type FamilyChildResponse struct {
	ID               string `json:"id"`
	Name             string `json:"name"`
	RelationshipType string `json:"relationship_type"`
}

type FamilyListResponse struct {
	Items  []FamilyResponse `json:"items"`
	Total  int              `json:"total"`
	Limit  int              `json:"limit"`
	Offset int              `json:"offset"`
}

// listFamilies handles GET /families
func (s *Server) listFamilies(c echo.Context) error {
	limit, _ := strconv.Atoi(c.QueryParam("limit"))
	offset, _ := strconv.Atoi(c.QueryParam("offset"))

	result, err := s.familyService.ListFamilies(c.Request().Context(), query.ListFamiliesInput{
		Limit:  limit,
		Offset: offset,
	})
	if err != nil {
		return err
	}

	response := FamilyListResponse{
		Items:  make([]FamilyResponse, len(result.Items)),
		Total:  result.Total,
		Limit:  result.Limit,
		Offset: result.Offset,
	}

	for i, f := range result.Items {
		response.Items[i] = convertFamilyToResponse(f)
	}

	return c.JSON(http.StatusOK, response)
}

// createFamily handles POST /families
func (s *Server) createFamily(c echo.Context) error {
	var req CreateFamilyRequest
	if err := c.Bind(&req); err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, "Invalid request body")
	}

	input := command.CreateFamilyInput{}
	if req.Partner1ID != nil {
		id, err := uuid.Parse(*req.Partner1ID)
		if err != nil {
			return echo.NewHTTPError(http.StatusBadRequest, "Invalid partner1_id")
		}
		input.Partner1ID = &id
	}
	if req.Partner2ID != nil {
		id, err := uuid.Parse(*req.Partner2ID)
		if err != nil {
			return echo.NewHTTPError(http.StatusBadRequest, "Invalid partner2_id")
		}
		input.Partner2ID = &id
	}
	if req.RelationshipType != nil {
		input.RelationshipType = *req.RelationshipType
	}
	if req.MarriageDate != nil {
		input.MarriageDate = *req.MarriageDate
	}
	if req.MarriagePlace != nil {
		input.MarriagePlace = *req.MarriagePlace
	}

	result, err := s.commandHandler.CreateFamily(c.Request().Context(), input)
	if err != nil {
		return err
	}

	// Fetch the created family to return full response
	family, err := s.familyService.GetFamily(c.Request().Context(), result.ID)
	if err != nil {
		return err
	}

	return c.JSON(http.StatusCreated, convertFamilyDetailToResponse(*family))
}

// getFamily handles GET /families/:id
func (s *Server) getFamily(c echo.Context) error {
	id, err := uuid.Parse(c.Param("id"))
	if err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, "Invalid family ID")
	}

	family, err := s.familyService.GetFamily(c.Request().Context(), id)
	if err != nil {
		return err
	}

	return c.JSON(http.StatusOK, convertFamilyDetailToResponse(*family))
}

// updateFamily handles PUT /families/:id
func (s *Server) updateFamily(c echo.Context) error {
	id, err := uuid.Parse(c.Param("id"))
	if err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, "Invalid family ID")
	}

	var req UpdateFamilyRequest
	if err := c.Bind(&req); err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, "Invalid request body")
	}

	input := command.UpdateFamilyInput{
		ID:            id,
		MarriageDate:  req.MarriageDate,
		MarriagePlace: req.MarriagePlace,
		Version:       req.Version,
	}
	if req.RelationshipType != nil {
		input.RelationshipType = req.RelationshipType
	}

	_, err = s.commandHandler.UpdateFamily(c.Request().Context(), input)
	if err != nil {
		return err
	}

	// Fetch the updated family to return full response
	family, err := s.familyService.GetFamily(c.Request().Context(), id)
	if err != nil {
		return err
	}

	return c.JSON(http.StatusOK, convertFamilyToResponse(family.Family))
}

// deleteFamily handles DELETE /families/:id
func (s *Server) deleteFamily(c echo.Context) error {
	id, err := uuid.Parse(c.Param("id"))
	if err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, "Invalid family ID")
	}

	// Get version from query param
	version, _ := strconv.ParseInt(c.QueryParam("version"), 10, 64)
	if version == 0 {
		// Try to get current version
		family, err := s.familyService.GetFamily(c.Request().Context(), id)
		if err != nil {
			return err
		}
		version = family.Version
	}

	err = s.commandHandler.DeleteFamily(c.Request().Context(), command.DeleteFamilyInput{
		ID:      id,
		Version: version,
	})
	if err != nil {
		return err
	}

	return c.NoContent(http.StatusNoContent)
}

// addChildToFamily handles POST /families/:id/children
func (s *Server) addChildToFamily(c echo.Context) error {
	familyID, err := uuid.Parse(c.Param("id"))
	if err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, "Invalid family ID")
	}

	var req AddChildRequest
	if err := c.Bind(&req); err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, "Invalid request body")
	}

	childID, err := uuid.Parse(req.ChildID)
	if err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, "Invalid child_id")
	}

	input := command.LinkChildInput{
		FamilyID: familyID,
		ChildID:  childID,
	}
	if req.RelationshipType != nil {
		input.RelationType = *req.RelationshipType
	}

	_, err = s.commandHandler.LinkChild(c.Request().Context(), input)
	if err != nil {
		return err
	}

	// Fetch the updated family to return response
	family, err := s.familyService.GetFamily(c.Request().Context(), familyID)
	if err != nil {
		return err
	}

	return c.JSON(http.StatusCreated, convertFamilyDetailToResponse(*family))
}

// removeChildFromFamily handles DELETE /families/:id/children/:personId
func (s *Server) removeChildFromFamily(c echo.Context) error {
	familyID, err := uuid.Parse(c.Param("id"))
	if err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, "Invalid family ID")
	}

	childID, err := uuid.Parse(c.Param("personId"))
	if err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, "Invalid person ID")
	}

	err = s.commandHandler.UnlinkChild(c.Request().Context(), command.UnlinkChildInput{
		FamilyID: familyID,
		ChildID:  childID,
	})
	if err != nil {
		return err
	}

	return c.NoContent(http.StatusNoContent)
}

// Group Sheet response types

// GroupSheetEventResponse represents an event in the group sheet.
type GroupSheetEventResponse struct {
	Date      string                       `json:"date,omitempty"`
	Place     string                       `json:"place,omitempty"`
	Citations []GroupSheetCitationResponse `json:"citations,omitempty"`
}

// GroupSheetCitationResponse represents a citation in the group sheet.
type GroupSheetCitationResponse struct {
	ID          string `json:"id"`
	SourceID    string `json:"source_id"`
	SourceTitle string `json:"source_title"`
	Page        string `json:"page,omitempty"`
	Detail      string `json:"detail,omitempty"`
}

// GroupSheetPersonResponse represents a person in the group sheet.
type GroupSheetPersonResponse struct {
	ID         string                   `json:"id"`
	GivenName  string                   `json:"given_name"`
	Surname    string                   `json:"surname"`
	Gender     string                   `json:"gender,omitempty"`
	Birth      *GroupSheetEventResponse `json:"birth,omitempty"`
	Death      *GroupSheetEventResponse `json:"death,omitempty"`
	FatherName string                   `json:"father_name,omitempty"`
	FatherID   *string                  `json:"father_id,omitempty"`
	MotherName string                   `json:"mother_name,omitempty"`
	MotherID   *string                  `json:"mother_id,omitempty"`
}

// GroupSheetChildResponse represents a child in the group sheet.
type GroupSheetChildResponse struct {
	ID               string                   `json:"id"`
	GivenName        string                   `json:"given_name"`
	Surname          string                   `json:"surname"`
	Gender           string                   `json:"gender,omitempty"`
	RelationshipType string                   `json:"relationship_type,omitempty"`
	Sequence         *int                     `json:"sequence,omitempty"`
	Birth            *GroupSheetEventResponse `json:"birth,omitempty"`
	Death            *GroupSheetEventResponse `json:"death,omitempty"`
	SpouseName       string                   `json:"spouse_name,omitempty"`
	SpouseID         *string                  `json:"spouse_id,omitempty"`
}

// GroupSheetResponse represents the full family group sheet.
type GroupSheetResponse struct {
	ID       string                    `json:"id"`
	Husband  *GroupSheetPersonResponse `json:"husband,omitempty"`
	Wife     *GroupSheetPersonResponse `json:"wife,omitempty"`
	Marriage *GroupSheetEventResponse  `json:"marriage,omitempty"`
	Children []GroupSheetChildResponse `json:"children,omitempty"`
}

// getFamilyGroupSheet handles GET /families/:id/group-sheet
func (s *Server) getFamilyGroupSheet(c echo.Context) error {
	familyID, err := uuid.Parse(c.Param("id"))
	if err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, "Invalid family ID")
	}

	gs, err := s.familyService.GetGroupSheet(c.Request().Context(), familyID)
	if err != nil {
		return err
	}

	return c.JSON(http.StatusOK, convertGroupSheetToResponse(gs))
}

// convertGroupSheetToResponse converts the service GroupSheet to API response.
func convertGroupSheetToResponse(gs *query.GroupSheet) GroupSheetResponse {
	resp := GroupSheetResponse{
		ID: gs.ID.String(),
	}

	if gs.Husband != nil {
		resp.Husband = convertGroupSheetPersonToResponse(gs.Husband)
	}
	if gs.Wife != nil {
		resp.Wife = convertGroupSheetPersonToResponse(gs.Wife)
	}
	if gs.Marriage != nil {
		resp.Marriage = convertGroupSheetEventToResponse(gs.Marriage)
	}

	for _, child := range gs.Children {
		resp.Children = append(resp.Children, convertGroupSheetChildToResponse(&child))
	}

	return resp
}

func convertGroupSheetPersonToResponse(p *query.GroupSheetPerson) *GroupSheetPersonResponse {
	resp := &GroupSheetPersonResponse{
		ID:         p.ID.String(),
		GivenName:  p.GivenName,
		Surname:    p.Surname,
		Gender:     p.Gender,
		FatherName: p.FatherName,
		MotherName: p.MotherName,
	}

	if p.Birth != nil {
		resp.Birth = convertGroupSheetEventToResponse(p.Birth)
	}
	if p.Death != nil {
		resp.Death = convertGroupSheetEventToResponse(p.Death)
	}
	if p.FatherID != nil {
		s := p.FatherID.String()
		resp.FatherID = &s
	}
	if p.MotherID != nil {
		s := p.MotherID.String()
		resp.MotherID = &s
	}

	return resp
}

func convertGroupSheetChildToResponse(c *query.GroupSheetChild) GroupSheetChildResponse {
	resp := GroupSheetChildResponse{
		ID:               c.ID.String(),
		GivenName:        c.GivenName,
		Surname:          c.Surname,
		Gender:           c.Gender,
		RelationshipType: c.RelationshipType,
		Sequence:         c.Sequence,
		SpouseName:       c.SpouseName,
	}

	if c.Birth != nil {
		resp.Birth = convertGroupSheetEventToResponse(c.Birth)
	}
	if c.Death != nil {
		resp.Death = convertGroupSheetEventToResponse(c.Death)
	}
	if c.SpouseID != nil {
		s := c.SpouseID.String()
		resp.SpouseID = &s
	}

	return resp
}

func convertGroupSheetEventToResponse(e *query.GroupSheetEvent) *GroupSheetEventResponse {
	resp := &GroupSheetEventResponse{
		Date:  e.Date,
		Place: e.Place,
	}

	for _, cit := range e.Citations {
		resp.Citations = append(resp.Citations, GroupSheetCitationResponse{
			ID:          cit.ID.String(),
			SourceID:    cit.SourceID.String(),
			SourceTitle: cit.SourceTitle,
			Page:        cit.Page,
			Detail:      cit.Detail,
		})
	}

	return resp
}

// Helper function to convert query result to response
func convertFamilyToResponse(f query.Family) FamilyResponse {
	resp := FamilyResponse{
		ID:               f.ID.String(),
		Partner1Name:     f.Partner1Name,
		Partner2Name:     f.Partner2Name,
		RelationshipType: f.RelationshipType,
		MarriagePlace:    f.MarriagePlace,
		ChildCount:       f.ChildCount,
		Version:          f.Version,
	}
	if f.Partner1ID != nil {
		s := f.Partner1ID.String()
		resp.Partner1ID = &s
	}
	if f.Partner2ID != nil {
		s := f.Partner2ID.String()
		resp.Partner2ID = &s
	}
	if f.MarriageDate != nil {
		resp.MarriageDate = f.MarriageDate
	}
	return resp
}

func convertFamilyDetailToResponse(fd query.FamilyDetail) FamilyDetailResponse {
	resp := FamilyDetailResponse{
		FamilyResponse: convertFamilyToResponse(fd.Family),
	}
	for _, c := range fd.Children {
		resp.Children = append(resp.Children, FamilyChildResponse{
			ID:               c.ID.String(),
			Name:             c.Name,
			RelationshipType: c.RelationshipType,
		})
	}
	return resp
}

// PedigreeNodeResponse represents a person in the pedigree tree.
type PedigreeNodeResponse struct {
	ID         string                `json:"id"`
	GivenName  string                `json:"given_name"`
	Surname    string                `json:"surname"`
	Gender     string                `json:"gender,omitempty"`
	BirthDate  *string               `json:"birth_date,omitempty"`
	BirthPlace *string               `json:"birth_place,omitempty"`
	DeathDate  *string               `json:"death_date,omitempty"`
	DeathPlace *string               `json:"death_place,omitempty"`
	Generation int                   `json:"generation"`
	Father     *PedigreeNodeResponse `json:"father,omitempty"`
	Mother     *PedigreeNodeResponse `json:"mother,omitempty"`
}

// PedigreeResponse represents the pedigree tree for a person.
type PedigreeResponse struct {
	Root           *PedigreeNodeResponse `json:"root"`
	TotalAncestors int                   `json:"total_ancestors"`
	MaxGeneration  int                   `json:"max_generation"`
}

func (s *Server) getPedigree(c echo.Context) error {
	idStr := c.Param("id")
	id, err := uuid.Parse(idStr)
	if err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, "Invalid person ID")
	}

	// Get max generations from query param (default 5)
	maxGen := 5
	if mg := c.QueryParam("generations"); mg != "" {
		if parsed, err := strconv.Atoi(mg); err == nil && parsed > 0 {
			maxGen = parsed
		}
	}

	result, err := s.pedigreeService.GetPedigree(c.Request().Context(), query.GetPedigreeInput{
		PersonID:       id,
		MaxGenerations: maxGen,
	})
	if err != nil {
		return err
	}

	response := PedigreeResponse{
		Root:           convertPedigreeNode(result.Root),
		TotalAncestors: result.TotalAncestors,
		MaxGeneration:  result.MaxGeneration,
	}

	return c.JSON(http.StatusOK, response)
}

// convertPedigreeNode converts a query PedigreeNode to API response format.
func convertPedigreeNode(node *query.PedigreeNode) *PedigreeNodeResponse {
	if node == nil {
		return nil
	}

	resp := &PedigreeNodeResponse{
		ID:         node.ID.String(),
		GivenName:  node.GivenName,
		Surname:    node.Surname,
		Gender:     node.Gender,
		Generation: node.Generation,
	}

	if node.BirthDate != nil {
		bd := node.BirthDate.String()
		resp.BirthDate = &bd
	}
	if node.BirthPlace != nil {
		resp.BirthPlace = node.BirthPlace
	}
	if node.DeathDate != nil {
		dd := node.DeathDate.String()
		resp.DeathDate = &dd
	}
	if node.DeathPlace != nil {
		resp.DeathPlace = node.DeathPlace
	}

	// Recursively convert ancestors
	resp.Father = convertPedigreeNode(node.Father)
	resp.Mother = convertPedigreeNode(node.Mother)

	return resp
}

// AhnentafelEntryResponse represents a single entry in an Ahnentafel report.
type AhnentafelEntryResponse struct {
	Number       int    `json:"number"`
	Generation   int    `json:"generation"`
	ID           string `json:"id,omitempty"`
	GivenName    string `json:"given_name,omitempty"`
	Surname      string `json:"surname,omitempty"`
	Gender       string `json:"gender,omitempty"`
	BirthDate    any    `json:"birth_date,omitempty"`
	BirthPlace   string `json:"birth_place,omitempty"`
	DeathDate    any    `json:"death_date,omitempty"`
	DeathPlace   string `json:"death_place,omitempty"`
	Relationship string `json:"relationship"`
}

// AhnentafelSubjectResponse represents the subject person in an Ahnentafel report.
type AhnentafelSubjectResponse struct {
	ID        string `json:"id"`
	GivenName string `json:"given_name"`
	Surname   string `json:"surname"`
}

// AhnentafelResponse represents the complete Ahnentafel report.
type AhnentafelResponse struct {
	Subject     AhnentafelSubjectResponse `json:"subject"`
	Entries     []AhnentafelEntryResponse `json:"entries"`
	Generations int                       `json:"generations"`
	TotalCount  int                       `json:"total_count"`
	KnownCount  int                       `json:"known_count"`
}

// getAhnentafel handles GET /ahnentafel/:id
func (s *Server) getAhnentafel(c echo.Context) error {
	idStr := c.Param("id")
	id, err := uuid.Parse(idStr)
	if err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, "Invalid person ID")
	}

	// Get generations from query param (default 5, max 10)
	maxGen := 5
	if mg := c.QueryParam("generations"); mg != "" {
		if parsed, err := strconv.Atoi(mg); err == nil && parsed > 0 {
			if parsed > 10 {
				parsed = 10
			}
			maxGen = parsed
		}
	}

	// Get format from query param (default "json")
	format := c.QueryParam("format")
	if format == "" {
		format = "json"
	}
	if format != "json" && format != "text" {
		return echo.NewHTTPError(http.StatusBadRequest, "Invalid format: "+format+"; valid formats are 'json' or 'text'")
	}

	result, err := s.ahnentafelService.GetAhnentafel(c.Request().Context(), query.GetAhnentafelInput{
		PersonID:       id,
		MaxGenerations: maxGen,
	})
	if err != nil {
		return err
	}

	// Find subject (entry number 1)
	var subject AhnentafelSubjectResponse
	for _, entry := range result.Entries {
		if entry.Number == 1 {
			subject = AhnentafelSubjectResponse{
				ID:        entry.ID.String(),
				GivenName: entry.GivenName,
				Surname:   entry.Surname,
			}
			break
		}
	}

	if format == "text" {
		return s.formatAhnentafelText(c, result, subject)
	}

	// JSON format - convert entries and count known ancestors
	entries := make([]AhnentafelEntryResponse, len(result.Entries))
	knownCount := 0
	for i, entry := range result.Entries {
		entries[i] = convertAhnentafelEntry(entry)
		if entry.ID != uuid.Nil {
			knownCount++
		}
	}

	response := AhnentafelResponse{
		Subject:     subject,
		Entries:     entries,
		Generations: result.MaxGeneration,
		TotalCount:  result.TotalEntries,
		KnownCount:  knownCount,
	}

	return c.JSON(http.StatusOK, response)
}

// convertAhnentafelEntry converts a query AhnentafelEntry to API response format.
func convertAhnentafelEntry(entry query.AhnentafelEntry) AhnentafelEntryResponse {
	resp := AhnentafelEntryResponse{
		Number:       entry.Number,
		Generation:   entry.Generation,
		Relationship: getRelationLabel(entry.Number),
	}

	// Only include ID if the person is known (not nil UUID)
	if entry.ID != uuid.Nil {
		resp.ID = entry.ID.String()
		resp.GivenName = entry.GivenName
		resp.Surname = entry.Surname
		resp.Gender = entry.Gender

		if entry.BirthDate != nil {
			resp.BirthDate = entry.BirthDate
		}
		if entry.BirthPlace != nil {
			resp.BirthPlace = *entry.BirthPlace
		}
		if entry.DeathDate != nil {
			resp.DeathDate = entry.DeathDate
		}
		if entry.DeathPlace != nil {
			resp.DeathPlace = *entry.DeathPlace
		}
	}

	return resp
}

// formatAhnentafelText formats the Ahnentafel report as plain text.
func (s *Server) formatAhnentafelText(c echo.Context, result *query.AhnentafelResult, subject AhnentafelSubjectResponse) error {
	c.Response().Header().Set("Content-Type", "text/plain; charset=utf-8")

	var sb strings.Builder

	// Header
	sb.WriteString("AHNENTAFEL REPORT\n")
	sb.WriteString("=================\n")
	sb.WriteString(fmt.Sprintf("Subject: %s %s\n\n", subject.GivenName, subject.Surname))

	// Entries
	for _, entry := range result.Entries {
		// Ahnentafel number and name
		relationLabel := getRelationLabel(entry.Number)
		if relationLabel != "" {
			sb.WriteString(fmt.Sprintf("%d. %s %s (%s)\n", entry.Number, entry.GivenName, entry.Surname, relationLabel))
		} else {
			sb.WriteString(fmt.Sprintf("%d. %s %s\n", entry.Number, entry.GivenName, entry.Surname))
		}

		// Birth
		var birthDateStr string
		if entry.BirthDate != nil {
			birthDateStr = entry.BirthDate.String()
		}
		birthStr := formatEventLine("b.", birthDateStr, entry.BirthPlace)
		sb.WriteString(fmt.Sprintf("   %s\n", birthStr))

		// Death
		var deathDateStr string
		if entry.DeathDate != nil {
			deathDateStr = entry.DeathDate.String()
		}
		deathStr := formatEventLine("d.", deathDateStr, entry.DeathPlace)
		sb.WriteString(fmt.Sprintf("   %s\n", deathStr))

		sb.WriteString("\n")
	}

	// Footer
	sb.WriteString(fmt.Sprintf("Generated: %s\n", time.Now().Format("2006-01-02")))
	sb.WriteString(fmt.Sprintf("Total ancestors: %d\n", result.TotalEntries))
	sb.WriteString(fmt.Sprintf("Generations: %d\n", result.MaxGeneration))

	return c.String(http.StatusOK, sb.String())
}

// getRelationLabel returns the relationship label for a given Ahnentafel number.
func getRelationLabel(num int) string {
	if num == 1 {
		return ""
	}
	if num == 2 {
		return "Father"
	}
	if num == 3 {
		return "Mother"
	}

	// For higher numbers, build the relationship string
	// Start from the person and work backwards to find the path
	var path []string
	n := num
	for n > 1 {
		if n%2 == 0 {
			path = append([]string{"Father"}, path...)
		} else {
			path = append([]string{"Mother"}, path...)
		}
		n /= 2
	}

	// Convert path to a label like "Father's Father" or "Mother's Mother"
	if len(path) == 0 {
		return ""
	}

	result := path[0]
	for i := 1; i < len(path); i++ {
		result += "'s " + path[i]
	}
	return result
}

// formatEventLine formats a birth or death event line.
func formatEventLine(prefix string, date string, place *string) string {
	dateStr := "-"
	if date != "" {
		dateStr = date
	}

	if place != nil && *place != "" {
		return fmt.Sprintf("%s %s, %s", prefix, dateStr, *place)
	}
	return fmt.Sprintf("%s %s", prefix, dateStr)
}

// ImportGedcomResponse represents the response from a GEDCOM import.
type ImportGedcomResponse struct {
	ImportID         string   `json:"import_id"`
	PersonsImported  int      `json:"persons_imported"`
	FamiliesImported int      `json:"families_imported"`
	Warnings         []string `json:"warnings,omitempty"`
	Errors           []string `json:"errors,omitempty"`
}

// importGedcom handles POST /gedcom/import
func (s *Server) importGedcom(c echo.Context) error {
	// Get the uploaded file
	file, err := c.FormFile("file")
	if err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, "No file uploaded. Use multipart form with 'file' field.")
	}

	// Validate file extension
	if !strings.HasSuffix(strings.ToLower(file.Filename), ".ged") {
		return echo.NewHTTPError(http.StatusBadRequest, "File must have .ged extension")
	}

	// Open the file
	src, err := file.Open()
	if err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, "Failed to open uploaded file")
	}
	defer src.Close()

	// Import the GEDCOM file
	result, err := s.commandHandler.ImportGedcom(c.Request().Context(), command.ImportGedcomInput{
		Filename: file.Filename,
		FileSize: file.Size,
		Reader:   src,
	})
	if err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, err.Error())
	}

	response := ImportGedcomResponse{
		ImportID:         result.ImportID.String(),
		PersonsImported:  result.PersonsImported,
		FamiliesImported: result.FamiliesImported,
		Warnings:         result.Warnings,
		Errors:           result.Errors,
	}

	return c.JSON(http.StatusOK, response)
}

func (s *Server) exportGedcom(c echo.Context) error {
	gedcomExporter := gedcom.NewExporter(s.readStore)

	// Set response headers for file download
	c.Response().Header().Set("Content-Type", "application/x-gedcom")
	c.Response().Header().Set("Content-Disposition", "attachment; filename=export.ged")

	result, err := gedcomExporter.Export(c.Request().Context(), c.Response().Writer)
	if err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, "Failed to export GEDCOM: "+err.Error())
	}

	// Log export statistics using strconv to break CodeQL taint analysis chain (CWE-117)
	c.Logger().Infof("GEDCOM export: %s persons, %s families, %s bytes",
		strconv.Itoa(result.PersonsExported), strconv.Itoa(result.FamiliesExported), strconv.FormatInt(result.BytesWritten, 10))

	return nil
}

// exportTree exports the complete family tree as JSON.
func (s *Server) exportTree(c echo.Context) error {
	dataExporter := exporter.NewDataExporter(s.readStore)

	opts := exporter.ExportOptions{
		Format:     exporter.FormatJSON,
		EntityType: exporter.EntityTypeAll,
	}

	c.Response().Header().Set("Content-Type", "application/json")
	c.Response().Header().Set("Content-Disposition", "attachment; filename=export-tree.json")

	result, err := dataExporter.Export(c.Request().Context(), c.Response().Writer, opts)
	if err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, "Failed to export tree: "+err.Error())
	}

	// Log export statistics using strconv to break CodeQL taint analysis chain (CWE-117)
	c.Logger().Infof("Tree export: %s persons, %s families, %s bytes",
		strconv.Itoa(result.PersonsExported), strconv.Itoa(result.FamiliesExported), strconv.FormatInt(result.BytesWritten, 10))

	return nil
}

// exportPersons exports persons in JSON or CSV format.
func (s *Server) exportPersons(c echo.Context) error {
	format := c.QueryParam("format")
	if format == "" {
		format = "json"
	}

	fieldsParam := c.QueryParam("fields")

	// Validate format
	var exportFormat exporter.Format
	switch format {
	case "json":
		exportFormat = exporter.FormatJSON
	case "csv":
		exportFormat = exporter.FormatCSV
	default:
		return echo.NewHTTPError(http.StatusBadRequest, "Invalid format: "+format+"; valid formats are 'json' or 'csv'")
	}

	// Parse fields for CSV
	var fields []string
	if fieldsParam != "" && exportFormat == exporter.FormatCSV {
		fields = strings.Split(fieldsParam, ",")
		// Trim whitespace from field names
		for i, f := range fields {
			fields[i] = strings.TrimSpace(f)
		}
		// Validate fields
		for _, f := range fields {
			if !exporter.AvailablePersonFields[f] {
				validFields := make([]string, 0, len(exporter.AvailablePersonFields))
				for k := range exporter.AvailablePersonFields {
					validFields = append(validFields, k)
				}
				return echo.NewHTTPError(http.StatusBadRequest,
					"Invalid field '"+f+"'; available fields: "+strings.Join(validFields, ", "))
			}
		}
	}

	exp := exporter.NewDataExporter(s.readStore)
	opts := exporter.ExportOptions{
		Format:     exportFormat,
		EntityType: exporter.EntityTypePersons,
		Fields:     fields,
	}

	// Set response headers
	if exportFormat == exporter.FormatJSON {
		c.Response().Header().Set("Content-Type", "application/json")
		c.Response().Header().Set("Content-Disposition", "attachment; filename=export-persons.json")
	} else {
		c.Response().Header().Set("Content-Type", "text/csv")
		c.Response().Header().Set("Content-Disposition", "attachment; filename=export-persons.csv")
	}

	result, err := exp.Export(c.Request().Context(), c.Response().Writer, opts)
	if err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, "Failed to export persons: "+err.Error())
	}

	// Log export statistics using strconv to break CodeQL taint analysis chain (CWE-117)
	c.Logger().Infof("Persons export: %s persons, %s bytes",
		strconv.Itoa(result.PersonsExported), strconv.FormatInt(result.BytesWritten, 10))

	return nil
}

// exportFamilies exports families in JSON or CSV format.
func (s *Server) exportFamilies(c echo.Context) error {
	format := c.QueryParam("format")
	if format == "" {
		format = "json"
	}

	fieldsParam := c.QueryParam("fields")

	// Validate format
	var exportFormat exporter.Format
	switch format {
	case "json":
		exportFormat = exporter.FormatJSON
	case "csv":
		exportFormat = exporter.FormatCSV
	default:
		return echo.NewHTTPError(http.StatusBadRequest, "Invalid format: "+format+"; valid formats are 'json' or 'csv'")
	}

	// Parse fields for CSV
	var fields []string
	if fieldsParam != "" && exportFormat == exporter.FormatCSV {
		fields = strings.Split(fieldsParam, ",")
		// Trim whitespace from field names
		for i, f := range fields {
			fields[i] = strings.TrimSpace(f)
		}
		// Validate fields
		for _, f := range fields {
			if !exporter.AvailableFamilyFields[f] {
				validFields := make([]string, 0, len(exporter.AvailableFamilyFields))
				for k := range exporter.AvailableFamilyFields {
					validFields = append(validFields, k)
				}
				return echo.NewHTTPError(http.StatusBadRequest,
					"Invalid field '"+f+"'; available fields: "+strings.Join(validFields, ", "))
			}
		}
	}

	exp := exporter.NewDataExporter(s.readStore)
	opts := exporter.ExportOptions{
		Format:     exportFormat,
		EntityType: exporter.EntityTypeFamilies,
		Fields:     fields,
	}

	// Set response headers
	if exportFormat == exporter.FormatJSON {
		c.Response().Header().Set("Content-Type", "application/json")
		c.Response().Header().Set("Content-Disposition", "attachment; filename=export-families.json")
	} else {
		c.Response().Header().Set("Content-Type", "text/csv")
		c.Response().Header().Set("Content-Disposition", "attachment; filename=export-families.csv")
	}

	result, err := exp.Export(c.Request().Context(), c.Response().Writer, opts)
	if err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, "Failed to export families: "+err.Error())
	}

	// Log export statistics using strconv to break CodeQL taint analysis chain (CWE-117)
	c.Logger().Infof("Families export: %s families, %s bytes",
		strconv.Itoa(result.FamiliesExported), strconv.FormatInt(result.BytesWritten, 10))

	return nil
}

// getPersonNames handles GET /persons/:id/names
func (s *Server) getPersonNames(c echo.Context) error {
	id, err := uuid.Parse(c.Param("id"))
	if err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, "Invalid person ID")
	}

	names, err := s.readStore.GetPersonNames(c.Request().Context(), id)
	if err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, "Failed to get person names: "+err.Error())
	}

	response := PersonNameListResponse{
		Items: make([]PersonNameResponse, len(names)),
		Total: len(names),
	}

	for i, n := range names {
		response.Items[i] = PersonNameResponse{
			ID:            n.ID.String(),
			PersonID:      n.PersonID.String(),
			GivenName:     n.GivenName,
			Surname:       n.Surname,
			FullName:      n.FullName,
			NamePrefix:    n.NamePrefix,
			NameSuffix:    n.NameSuffix,
			SurnamePrefix: n.SurnamePrefix,
			Nickname:      n.Nickname,
			NameType:      string(n.NameType),
			IsPrimary:     n.IsPrimary,
		}
	}

	return c.JSON(http.StatusOK, response)
}

// addPersonName handles POST /persons/:id/names
func (s *Server) addPersonName(c echo.Context) error {
	personID, err := uuid.Parse(c.Param("id"))
	if err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, "Invalid person ID")
	}

	var req CreatePersonNameRequest
	if err := c.Bind(&req); err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, "Invalid request body")
	}

	input := command.AddNameInput{
		PersonID:      personID,
		GivenName:     req.GivenName,
		Surname:       req.Surname,
		NamePrefix:    req.NamePrefix,
		NameSuffix:    req.NameSuffix,
		SurnamePrefix: req.SurnamePrefix,
		Nickname:      req.Nickname,
		NameType:      req.NameType,
	}
	if req.IsPrimary != nil {
		input.IsPrimary = *req.IsPrimary
	}

	result, err := s.commandHandler.AddName(c.Request().Context(), input)
	if err != nil {
		return err
	}

	// Fetch the created name to return full response
	name, err := s.readStore.GetPersonName(c.Request().Context(), result.ID)
	if err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, "Failed to get created name: "+err.Error())
	}

	return c.JSON(http.StatusCreated, PersonNameResponse{
		ID:            name.ID.String(),
		PersonID:      name.PersonID.String(),
		GivenName:     name.GivenName,
		Surname:       name.Surname,
		FullName:      name.FullName,
		NamePrefix:    name.NamePrefix,
		NameSuffix:    name.NameSuffix,
		SurnamePrefix: name.SurnamePrefix,
		Nickname:      name.Nickname,
		NameType:      string(name.NameType),
		IsPrimary:     name.IsPrimary,
	})
}

// updatePersonName handles PUT /persons/:id/names/:nameId
func (s *Server) updatePersonName(c echo.Context) error {
	personID, err := uuid.Parse(c.Param("id"))
	if err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, "Invalid person ID")
	}

	nameID, err := uuid.Parse(c.Param("nameId"))
	if err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, "Invalid name ID")
	}

	var req UpdatePersonNameRequest
	if err := c.Bind(&req); err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, "Invalid request body")
	}

	input := command.UpdateNameInput{
		PersonID:      personID,
		NameID:        nameID,
		GivenName:     req.GivenName,
		Surname:       req.Surname,
		NamePrefix:    req.NamePrefix,
		NameSuffix:    req.NameSuffix,
		SurnamePrefix: req.SurnamePrefix,
		Nickname:      req.Nickname,
		NameType:      req.NameType,
		IsPrimary:     req.IsPrimary,
	}

	_, err = s.commandHandler.UpdateName(c.Request().Context(), input)
	if err != nil {
		return err
	}

	// Fetch the updated name to return full response
	name, err := s.readStore.GetPersonName(c.Request().Context(), nameID)
	if err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, "Failed to get updated name: "+err.Error())
	}

	return c.JSON(http.StatusOK, PersonNameResponse{
		ID:            name.ID.String(),
		PersonID:      name.PersonID.String(),
		GivenName:     name.GivenName,
		Surname:       name.Surname,
		FullName:      name.FullName,
		NamePrefix:    name.NamePrefix,
		NameSuffix:    name.NameSuffix,
		SurnamePrefix: name.SurnamePrefix,
		Nickname:      name.Nickname,
		NameType:      string(name.NameType),
		IsPrimary:     name.IsPrimary,
	})
}

// deletePersonName handles DELETE /persons/:id/names/:nameId
func (s *Server) deletePersonName(c echo.Context) error {
	personID, err := uuid.Parse(c.Param("id"))
	if err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, "Invalid person ID")
	}

	nameID, err := uuid.Parse(c.Param("nameId"))
	if err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, "Invalid name ID")
	}

	err = s.commandHandler.DeleteName(c.Request().Context(), command.DeleteNameInput{
		PersonID: personID,
		NameID:   nameID,
	})
	if err != nil {
		return err
	}

	return c.NoContent(http.StatusNoContent)
}
