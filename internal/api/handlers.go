package api

import (
	"net/http"
	"strconv"
	"strings"

	"github.com/google/uuid"
	"github.com/labstack/echo/v4"

	"github.com/cacack/my-family/internal/command"
	"github.com/cacack/my-family/internal/exporter"
	"github.com/cacack/my-family/internal/gedcom"
	"github.com/cacack/my-family/internal/query"
)

// Person request/response types

type CreatePersonRequest struct {
	GivenName  string  `json:"given_name" validate:"required,min=1,max=100"`
	Surname    string  `json:"surname" validate:"required,min=1,max=100"`
	Gender     *string `json:"gender,omitempty"`
	BirthDate  *string `json:"birth_date,omitempty"`
	BirthPlace *string `json:"birth_place,omitempty"`
	DeathDate  *string `json:"death_date,omitempty"`
	DeathPlace *string `json:"death_place,omitempty"`
	Notes      *string `json:"notes,omitempty"`
}

type UpdatePersonRequest struct {
	GivenName  *string `json:"given_name,omitempty"`
	Surname    *string `json:"surname,omitempty"`
	Gender     *string `json:"gender,omitempty"`
	BirthDate  *string `json:"birth_date,omitempty"`
	BirthPlace *string `json:"birth_place,omitempty"`
	DeathDate  *string `json:"death_date,omitempty"`
	DeathPlace *string `json:"death_place,omitempty"`
	Notes      *string `json:"notes,omitempty"`
	Version    int64   `json:"version" validate:"required"`
}

type PersonResponse struct {
	ID         string  `json:"id"`
	GivenName  string  `json:"given_name"`
	Surname    string  `json:"surname"`
	Gender     *string `json:"gender,omitempty"`
	BirthDate  any     `json:"birth_date,omitempty"`
	BirthPlace *string `json:"birth_place,omitempty"`
	DeathDate  any     `json:"death_date,omitempty"`
	DeathPlace *string `json:"death_place,omitempty"`
	Notes      *string `json:"notes,omitempty"`
	Version    int64   `json:"version"`
}

type PersonDetailResponse struct {
	PersonResponse
	FamiliesAsPartner []FamilySummaryResponse `json:"families_as_partner,omitempty"`
	FamilyAsChild     *FamilySummaryResponse  `json:"family_as_child,omitempty"`
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

	result, err := s.commandHandler.CreatePerson(c.Request().Context(), input)
	if err != nil {
		return err
	}

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
		ID:         id,
		GivenName:  req.GivenName,
		Surname:    req.Surname,
		Gender:     req.Gender,
		BirthDate:  req.BirthDate,
		BirthPlace: req.BirthPlace,
		DeathDate:  req.DeathDate,
		DeathPlace: req.DeathPlace,
		Notes:      req.Notes,
		Version:    req.Version,
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
	exporter := gedcom.NewExporter(s.readStore)

	// Set response headers for file download
	c.Response().Header().Set("Content-Type", "application/x-gedcom")
	c.Response().Header().Set("Content-Disposition", "attachment; filename=export.ged")

	result, err := exporter.Export(c.Request().Context(), c.Response().Writer)
	if err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, "Failed to export GEDCOM: "+err.Error())
	}

	// Log export statistics
	c.Logger().Infof("GEDCOM export: %d persons, %d families, %d bytes",
		result.PersonsExported, result.FamiliesExported, result.BytesWritten)

	return nil
}

// exportTree exports the complete family tree as JSON.
func (s *Server) exportTree(c echo.Context) error {
	exp := exporter.NewDataExporter(s.readStore)

	opts := exporter.ExportOptions{
		Format:     exporter.FormatJSON,
		EntityType: exporter.EntityTypeAll,
	}

	c.Response().Header().Set("Content-Type", "application/json")
	c.Response().Header().Set("Content-Disposition", "attachment; filename=export-tree.json")

	result, err := exp.Export(c.Request().Context(), c.Response().Writer, opts)
	if err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, "Failed to export tree: "+err.Error())
	}

	c.Logger().Infof("Tree export: %d persons, %d families, %d bytes",
		result.PersonsExported, result.FamiliesExported, result.BytesWritten)

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

	c.Logger().Infof("Persons export: %d persons, %d bytes",
		result.PersonsExported, result.BytesWritten)

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

	c.Logger().Infof("Families export: %d families, %d bytes",
		result.FamiliesExported, result.BytesWritten)

	return nil
}
