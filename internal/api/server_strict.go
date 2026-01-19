// Package api provides the HTTP API server for the genealogy application.
package api

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/url"
	"strings"
	"time"

	"github.com/google/uuid"

	"github.com/cacack/my-family/internal/command"
	"github.com/cacack/my-family/internal/domain"
	"github.com/cacack/my-family/internal/gedcom"
	"github.com/cacack/my-family/internal/query"
	"github.com/cacack/my-family/internal/repository"
)

// StrictServer wraps the Server and implements StrictServerInterface for type-safe API handlers.
type StrictServer struct {
	server *Server
}

// Compile-time check that StrictServer implements StrictServerInterface.
var _ StrictServerInterface = (*StrictServer)(nil)

// NewStrictServer creates a new StrictServer wrapping the given Server.
func NewStrictServer(server *Server) *StrictServer {
	return &StrictServer{server: server}
}

// ============================================================================
// Ahnentafel endpoints
// ============================================================================

// GetAhnentafel implements StrictServerInterface.
func (ss *StrictServer) GetAhnentafel(ctx context.Context, request GetAhnentafelRequestObject) (GetAhnentafelResponseObject, error) {
	// Validate format enum if provided
	if request.Params.Format != nil {
		switch *request.Params.Format {
		case Json, Text:
			// Valid formats
		default:
			return GetAhnentafel400JSONResponse{BadRequestJSONResponse{
				Code:    "invalid_format",
				Message: "Invalid format: must be 'json' or 'text'",
			}}, nil
		}
	}

	maxGen := 5
	if request.Params.Generations != nil {
		maxGen = *request.Params.Generations
		if maxGen > 10 {
			maxGen = 10
		}
	}

	result, err := ss.server.ahnentafelService.GetAhnentafel(ctx, query.GetAhnentafelInput{
		PersonID:       request.Id,
		MaxGenerations: maxGen,
	})
	if err != nil {
		if errors.Is(err, query.ErrNotFound) {
			return GetAhnentafel404JSONResponse{NotFoundJSONResponse{
				Code:    "not_found",
				Message: "Person not found",
			}}, nil
		}
		return nil, err
	}

	// Find subject (entry number 1)
	var subject AhnentafelSubject
	for _, entry := range result.Entries {
		if entry.Number == 1 {
			subject = AhnentafelSubject{
				Id:        entry.ID,
				GivenName: entry.GivenName,
				Surname:   entry.Surname,
			}
			break
		}
	}

	// Check format - if text, return text response
	if request.Params.Format != nil && *request.Params.Format == Text {
		var sb strings.Builder
		sb.WriteString("AHNENTAFEL REPORT\n")
		sb.WriteString("=================\n")
		sb.WriteString(fmt.Sprintf("Subject: %s %s\n\n", subject.GivenName, subject.Surname))

		for _, entry := range result.Entries {
			relationLabel := getRelationLabel(entry.Number)
			if relationLabel != "" {
				sb.WriteString(fmt.Sprintf("%d. %s %s (%s)\n", entry.Number, entry.GivenName, entry.Surname, relationLabel))
			} else {
				sb.WriteString(fmt.Sprintf("%d. %s %s\n", entry.Number, entry.GivenName, entry.Surname))
			}

			var birthDateStr, deathDateStr string
			if entry.BirthDate != nil {
				birthDateStr = entry.BirthDate.String()
			}
			if entry.DeathDate != nil {
				deathDateStr = entry.DeathDate.String()
			}
			sb.WriteString(fmt.Sprintf("   b. %s\n", formatEventLineStr(birthDateStr, entry.BirthPlace)))
			sb.WriteString(fmt.Sprintf("   d. %s\n\n", formatEventLineStr(deathDateStr, entry.DeathPlace)))
		}

		sb.WriteString(fmt.Sprintf("Generated: %s\n", time.Now().Format("2006-01-02")))
		sb.WriteString(fmt.Sprintf("Total ancestors: %d\n", result.TotalEntries))
		sb.WriteString(fmt.Sprintf("Generations: %d\n", result.MaxGeneration))

		return GetAhnentafel200TextResponse(sb.String()), nil
	}

	// JSON format
	entries := make([]AhnentafelEntry, len(result.Entries))
	knownCount := 0
	for i, entry := range result.Entries {
		entries[i] = convertQueryAhnentafelEntryToGenerated(entry)
		if entry.ID != uuid.Nil {
			knownCount++
		}
	}

	return GetAhnentafel200JSONResponse{
		Subject:     subject,
		Entries:     entries,
		Generations: result.MaxGeneration,
		TotalCount:  result.TotalEntries,
		KnownCount:  knownCount,
	}, nil
}

// formatEventLineStr formats a date and place for text output.
func formatEventLineStr(date string, place *string) string {
	dateStr := "-"
	if date != "" {
		dateStr = date
	}
	if place != nil && *place != "" {
		return fmt.Sprintf("%s, %s", dateStr, *place)
	}
	return dateStr
}

// ============================================================================
// Browse endpoints
// ============================================================================

// BrowsePlaces implements StrictServerInterface.
func (ss *StrictServer) BrowsePlaces(ctx context.Context, request BrowsePlacesRequestObject) (BrowsePlacesResponseObject, error) {
	parent := ""
	if request.Params.Parent != nil {
		parent = *request.Params.Parent
	}

	result, err := ss.server.browseService.GetPlaceHierarchy(ctx, query.GetPlaceHierarchyInput{
		Parent: parent,
	})
	if err != nil {
		return nil, err
	}

	response := PlaceIndexResponse{
		Items: make([]PlaceEntry, len(result.Items)),
		Total: result.Total,
	}

	if len(result.Breadcrumb) > 0 {
		response.Breadcrumb = &result.Breadcrumb
	}

	for i, item := range result.Items {
		fullName := item.FullName
		hasChildren := item.HasChildren
		response.Items[i] = PlaceEntry{
			Name:        item.Name,
			FullName:    &fullName,
			Count:       item.Count,
			HasChildren: &hasChildren,
		}
	}

	return BrowsePlaces200JSONResponse(response), nil
}

// GetPersonsByPlace implements StrictServerInterface.
func (ss *StrictServer) GetPersonsByPlace(ctx context.Context, request GetPersonsByPlaceRequestObject) (GetPersonsByPlaceResponseObject, error) {
	place, err := url.PathUnescape(request.Place)
	if err != nil {
		// Since there's no 400 response defined, return a generic error
		return nil, err
	}

	limit := 20
	offset := 0
	if request.Params.Limit != nil {
		limit = *request.Params.Limit
	}
	if request.Params.Offset != nil {
		offset = *request.Params.Offset
	}

	result, err := ss.server.browseService.GetPersonsByPlace(ctx, query.GetPersonsByPlaceInput{
		Place:  place,
		Limit:  limit,
		Offset: offset,
	})
	if err != nil {
		return nil, err
	}

	items := make([]Person, len(result.Items))
	for i, p := range result.Items {
		items[i] = convertQueryPersonToGenerated(p)
	}

	limitVal := result.Limit
	offsetVal := result.Offset
	return GetPersonsByPlace200JSONResponse{
		Items:  items,
		Total:  result.Total,
		Limit:  &limitVal,
		Offset: &offsetVal,
	}, nil
}

// BrowseSurnames implements StrictServerInterface.
func (ss *StrictServer) BrowseSurnames(ctx context.Context, request BrowseSurnamesRequestObject) (BrowseSurnamesResponseObject, error) {
	letter := ""
	if request.Params.Letter != nil {
		letter = *request.Params.Letter
	}

	result, err := ss.server.browseService.GetSurnameIndex(ctx, query.GetSurnameIndexInput{
		Letter: letter,
	})
	if err != nil {
		return nil, err
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
		letterCounts := make([]LetterCount, len(result.LetterCounts))
		for i, lc := range result.LetterCounts {
			letterCounts[i] = LetterCount{
				Letter: lc.Letter,
				Count:  lc.Count,
			}
		}
		response.LetterCounts = &letterCounts
	}

	return BrowseSurnames200JSONResponse(response), nil
}

// GetPersonsBySurname implements StrictServerInterface.
func (ss *StrictServer) GetPersonsBySurname(ctx context.Context, request GetPersonsBySurnameRequestObject) (GetPersonsBySurnameResponseObject, error) {
	surname, err := url.PathUnescape(request.Surname)
	if err != nil {
		// Since there's no 400 response defined, return a generic error
		return nil, err
	}

	limit := 20
	offset := 0
	if request.Params.Limit != nil {
		limit = *request.Params.Limit
	}
	if request.Params.Offset != nil {
		offset = *request.Params.Offset
	}

	result, err := ss.server.browseService.GetPersonsBySurname(ctx, query.GetPersonsBySurnameInput{
		Surname: surname,
		Limit:   limit,
		Offset:  offset,
	})
	if err != nil {
		return nil, err
	}

	items := make([]Person, len(result.Items))
	for i, p := range result.Items {
		items[i] = convertQueryPersonToGenerated(p)
	}

	limitVal := result.Limit
	offsetVal := result.Offset
	return GetPersonsBySurname200JSONResponse{
		Items:  items,
		Total:  result.Total,
		Limit:  &limitVal,
		Offset: &offsetVal,
	}, nil
}

// ============================================================================
// Citation endpoints
// ============================================================================

// CreateCitation implements StrictServerInterface.
func (ss *StrictServer) CreateCitation(ctx context.Context, request CreateCitationRequestObject) (CreateCitationResponseObject, error) {
	input := command.CreateCitationInput{
		SourceID:    request.Body.SourceId,
		FactType:    request.Body.FactType,
		FactOwnerID: request.Body.FactOwnerId,
	}

	if request.Body.Page != nil {
		input.Page = *request.Body.Page
	}
	if request.Body.Volume != nil {
		input.Volume = *request.Body.Volume
	}
	if request.Body.SourceQuality != nil {
		input.SourceQuality = *request.Body.SourceQuality
	}
	if request.Body.InformantType != nil {
		input.InformantType = *request.Body.InformantType
	}
	if request.Body.EvidenceType != nil {
		input.EvidenceType = *request.Body.EvidenceType
	}
	if request.Body.QuotedText != nil {
		input.QuotedText = *request.Body.QuotedText
	}
	if request.Body.Analysis != nil {
		input.Analysis = *request.Body.Analysis
	}
	if request.Body.TemplateId != nil {
		input.TemplateID = *request.Body.TemplateId
	}

	result, err := ss.server.commandHandler.CreateCitation(ctx, input)
	if err != nil {
		return nil, err
	}

	citation, err := ss.server.sourceService.GetCitation(ctx, result.ID)
	if err != nil {
		return nil, err
	}

	return CreateCitation201JSONResponse(convertQueryCitationToGenerated(*citation)), nil
}

// GetCitation implements StrictServerInterface.
func (ss *StrictServer) GetCitation(ctx context.Context, request GetCitationRequestObject) (GetCitationResponseObject, error) {
	citation, err := ss.server.sourceService.GetCitation(ctx, request.Id)
	if err != nil {
		if errors.Is(err, query.ErrNotFound) {
			return GetCitation404JSONResponse{NotFoundJSONResponse{
				Code:    "not_found",
				Message: "Citation not found",
			}}, nil
		}
		return nil, err
	}

	return GetCitation200JSONResponse(convertQueryCitationToGenerated(*citation)), nil
}

// UpdateCitation implements StrictServerInterface.
func (ss *StrictServer) UpdateCitation(ctx context.Context, request UpdateCitationRequestObject) (UpdateCitationResponseObject, error) {
	input := command.UpdateCitationInput{
		ID:      request.Id,
		Version: request.Body.Version,
	}

	if request.Body.Page != nil {
		input.Page = request.Body.Page
	}
	if request.Body.Volume != nil {
		input.Volume = request.Body.Volume
	}
	if request.Body.SourceQuality != nil {
		input.SourceQuality = request.Body.SourceQuality
	}
	if request.Body.InformantType != nil {
		input.InformantType = request.Body.InformantType
	}
	if request.Body.EvidenceType != nil {
		input.EvidenceType = request.Body.EvidenceType
	}
	if request.Body.QuotedText != nil {
		input.QuotedText = request.Body.QuotedText
	}
	if request.Body.Analysis != nil {
		input.Analysis = request.Body.Analysis
	}
	if request.Body.TemplateId != nil {
		input.TemplateID = request.Body.TemplateId
	}

	_, err := ss.server.commandHandler.UpdateCitation(ctx, input)
	if err != nil {
		if errors.Is(err, repository.ErrConcurrencyConflict) {
			return UpdateCitation409JSONResponse{ConflictJSONResponse{
				Code:    "conflict",
				Message: "Version conflict - entity was modified",
			}}, nil
		}
		if errors.Is(err, query.ErrNotFound) {
			return UpdateCitation404JSONResponse{NotFoundJSONResponse{
				Code:    "not_found",
				Message: "Citation not found",
			}}, nil
		}
		return nil, err
	}

	citation, err := ss.server.sourceService.GetCitation(ctx, request.Id)
	if err != nil {
		return nil, err
	}

	return UpdateCitation200JSONResponse(convertQueryCitationToGenerated(*citation)), nil
}

// DeleteCitation implements StrictServerInterface.
func (ss *StrictServer) DeleteCitation(ctx context.Context, request DeleteCitationRequestObject) (DeleteCitationResponseObject, error) {
	var version int64
	if request.Params.Version != nil {
		version = *request.Params.Version
	} else {
		citation, err := ss.server.sourceService.GetCitation(ctx, request.Id)
		if err != nil {
			if errors.Is(err, query.ErrNotFound) {
				return DeleteCitation404JSONResponse{NotFoundJSONResponse{
					Code:    "not_found",
					Message: "Citation not found",
				}}, nil
			}
			return nil, err
		}
		version = citation.Version
	}

	err := ss.server.commandHandler.DeleteCitation(ctx, request.Id, version, "")
	if err != nil {
		if errors.Is(err, query.ErrNotFound) {
			return DeleteCitation404JSONResponse{NotFoundJSONResponse{
				Code:    "not_found",
				Message: "Citation not found",
			}}, nil
		}
		return nil, err
	}

	return DeleteCitation204Response{}, nil
}

// GetCitationRestorePoints implements StrictServerInterface.
func (ss *StrictServer) GetCitationRestorePoints(ctx context.Context, request GetCitationRestorePointsRequestObject) (GetCitationRestorePointsResponseObject, error) {
	_, err := ss.server.sourceService.GetCitation(ctx, request.Id)
	if err != nil {
		if errors.Is(err, query.ErrNotFound) {
			return GetCitationRestorePoints404JSONResponse{NotFoundJSONResponse{
				Code:    "not_found",
				Message: "Citation not found",
			}}, nil
		}
		return nil, err
	}

	limit := 20
	offset := 0
	if request.Params.Limit != nil {
		limit = *request.Params.Limit
	}
	if request.Params.Offset != nil {
		offset = *request.Params.Offset
	}

	result, err := ss.server.rollbackService.GetRestorePoints(ctx, "Citation", request.Id, limit, offset)
	if err != nil {
		if errors.Is(err, query.ErrNoEvents) {
			return GetCitationRestorePoints404JSONResponse{NotFoundJSONResponse{
				Code:    "not_found",
				Message: "No history found for this entity",
			}}, nil
		}
		return nil, err
	}

	return GetCitationRestorePoints200JSONResponse(convertRestorePointsResult(result)), nil
}

// RollbackCitation implements StrictServerInterface.
func (ss *StrictServer) RollbackCitation(ctx context.Context, request RollbackCitationRequestObject) (RollbackCitationResponseObject, error) {
	_, err := ss.server.sourceService.GetCitation(ctx, request.Id)
	if err != nil {
		if errors.Is(err, query.ErrNotFound) {
			return RollbackCitation404JSONResponse{NotFoundJSONResponse{
				Code:    "not_found",
				Message: "Citation not found",
			}}, nil
		}
		return nil, err
	}

	if request.Body.TargetVersion < 1 {
		return RollbackCitation400JSONResponse{BadRequestJSONResponse{
			Code:    "bad_request",
			Message: "target_version must be a positive integer",
		}}, nil
	}

	result, err := ss.server.commandHandler.RollbackCitation(ctx, request.Id, request.Body.TargetVersion)
	if err != nil {
		return handleRollbackErrorStrict[RollbackCitationResponseObject](err,
			func(e Error) RollbackCitationResponseObject {
				return RollbackCitation400JSONResponse{BadRequestJSONResponse(e)}
			},
			func(e Error) RollbackCitationResponseObject {
				return RollbackCitation404JSONResponse{NotFoundJSONResponse(e)}
			},
			func(e Error) RollbackCitationResponseObject { return RollbackCitation409JSONResponse(e) },
		)
	}

	return RollbackCitation200JSONResponse(convertRollbackResult(result, "Citation rolled back successfully")), nil
}

// ============================================================================
// Export endpoints
// ============================================================================

// ExportFamilies implements StrictServerInterface.
func (ss *StrictServer) ExportFamilies(ctx context.Context, _ ExportFamiliesRequestObject) (ExportFamiliesResponseObject, error) {
	// List all families from the read store
	result, err := ss.server.familyService.ListFamilies(ctx, query.ListFamiliesInput{
		Limit: 10000, // Export all
	})
	if err != nil {
		return nil, err
	}

	families := make([]Family, len(result.Items))
	for i, f := range result.Items {
		families[i] = convertQueryFamilyToGenerated(f)
	}

	total := len(families)
	return ExportFamilies200JSONResponse{
		Families: &families,
		Total:    &total,
	}, nil
}

// ExportPersons implements StrictServerInterface.
func (ss *StrictServer) ExportPersons(ctx context.Context, _ ExportPersonsRequestObject) (ExportPersonsResponseObject, error) {
	// List all persons from the read store
	result, err := ss.server.personService.ListPersons(ctx, query.ListPersonsInput{
		Limit: 10000, // Export all
	})
	if err != nil {
		return nil, err
	}

	persons := make([]Person, len(result.Items))
	for i, p := range result.Items {
		persons[i] = convertQueryPersonToGenerated(p)
	}

	total := len(persons)
	return ExportPersons200JSONResponse{
		Persons: &persons,
		Total:   &total,
	}, nil
}

// ExportTree implements StrictServerInterface.
func (ss *StrictServer) ExportTree(ctx context.Context, _ ExportTreeRequestObject) (ExportTreeResponseObject, error) {
	// Get all persons
	personsResult, err := ss.server.personService.ListPersons(ctx, query.ListPersonsInput{
		Limit: 10000,
	})
	if err != nil {
		return nil, err
	}

	persons := make([]Person, len(personsResult.Items))
	for i, p := range personsResult.Items {
		persons[i] = convertQueryPersonToGenerated(p)
	}

	// Get all families
	familiesResult, err := ss.server.familyService.ListFamilies(ctx, query.ListFamiliesInput{
		Limit: 10000,
	})
	if err != nil {
		return nil, err
	}

	families := make([]Family, len(familiesResult.Items))
	for i, f := range familiesResult.Items {
		families[i] = convertQueryFamilyToGenerated(f)
	}

	return ExportTree200JSONResponse{
		Persons:  &persons,
		Families: &families,
	}, nil
}

// ============================================================================
// Family endpoints
// ============================================================================

// ListFamilies implements StrictServerInterface.
func (ss *StrictServer) ListFamilies(ctx context.Context, request ListFamiliesRequestObject) (ListFamiliesResponseObject, error) {
	limit := 20
	offset := 0
	if request.Params.Limit != nil {
		limit = *request.Params.Limit
	}
	if request.Params.Offset != nil {
		offset = *request.Params.Offset
	}

	result, err := ss.server.familyService.ListFamilies(ctx, query.ListFamiliesInput{
		Limit:  limit,
		Offset: offset,
	})
	if err != nil {
		return nil, err
	}

	items := make([]FamilyDetail, len(result.Items))
	for i, f := range result.Items {
		items[i] = convertQueryFamilyToFamilyDetail(f)
	}

	limitVal := result.Limit
	offsetVal := result.Offset
	return ListFamilies200JSONResponse{
		Items:  items,
		Total:  result.Total,
		Limit:  &limitVal,
		Offset: &offsetVal,
	}, nil
}

// CreateFamily implements StrictServerInterface.
func (ss *StrictServer) CreateFamily(ctx context.Context, request CreateFamilyRequestObject) (CreateFamilyResponseObject, error) {
	input := command.CreateFamilyInput{}

	if request.Body.Partner1Id != nil {
		id := *request.Body.Partner1Id
		input.Partner1ID = &id
	}
	if request.Body.Partner2Id != nil {
		id := *request.Body.Partner2Id
		input.Partner2ID = &id
	}
	if request.Body.RelationshipType != nil {
		input.RelationshipType = string(*request.Body.RelationshipType)
	}
	if request.Body.MarriageDate != nil {
		input.MarriageDate = *request.Body.MarriageDate
	}
	if request.Body.MarriagePlace != nil {
		input.MarriagePlace = *request.Body.MarriagePlace
	}

	result, err := ss.server.commandHandler.CreateFamily(ctx, input)
	if err != nil {
		return nil, err
	}

	family, err := ss.server.familyService.GetFamily(ctx, result.ID)
	if err != nil {
		return nil, err
	}

	return CreateFamily201JSONResponse(convertQueryFamilyToGenerated(family.Family)), nil
}

// GetFamily implements StrictServerInterface.
func (ss *StrictServer) GetFamily(ctx context.Context, request GetFamilyRequestObject) (GetFamilyResponseObject, error) {
	family, err := ss.server.familyService.GetFamily(ctx, request.Id)
	if err != nil {
		if errors.Is(err, query.ErrNotFound) {
			return GetFamily404JSONResponse{NotFoundJSONResponse{
				Code:    "not_found",
				Message: "Family not found",
			}}, nil
		}
		return nil, err
	}

	return GetFamily200JSONResponse(convertQueryFamilyDetailToGenerated(*family)), nil
}

// UpdateFamily implements StrictServerInterface.
func (ss *StrictServer) UpdateFamily(ctx context.Context, request UpdateFamilyRequestObject) (UpdateFamilyResponseObject, error) {
	input := command.UpdateFamilyInput{
		ID:      request.Id,
		Version: request.Body.Version,
	}

	if request.Body.MarriageDate != nil {
		input.MarriageDate = request.Body.MarriageDate
	}
	if request.Body.MarriagePlace != nil {
		input.MarriagePlace = request.Body.MarriagePlace
	}
	if request.Body.RelationshipType != nil {
		relType := string(*request.Body.RelationshipType)
		input.RelationshipType = &relType
	}

	_, err := ss.server.commandHandler.UpdateFamily(ctx, input)
	if err != nil {
		if errors.Is(err, repository.ErrConcurrencyConflict) {
			return UpdateFamily400JSONResponse{BadRequestJSONResponse{
				Code:    "conflict",
				Message: "Version conflict - entity was modified",
			}}, nil
		}
		if errors.Is(err, query.ErrNotFound) {
			return UpdateFamily404JSONResponse{NotFoundJSONResponse{
				Code:    "not_found",
				Message: "Family not found",
			}}, nil
		}
		return nil, err
	}

	family, err := ss.server.familyService.GetFamily(ctx, request.Id)
	if err != nil {
		return nil, err
	}

	return UpdateFamily200JSONResponse(convertQueryFamilyToGenerated(family.Family)), nil
}

// DeleteFamily implements StrictServerInterface.
func (ss *StrictServer) DeleteFamily(ctx context.Context, request DeleteFamilyRequestObject) (DeleteFamilyResponseObject, error) {
	// Get current version since DELETE doesn't accept version parameter per OpenAPI spec
	family, err := ss.server.familyService.GetFamily(ctx, request.Id)
	if err != nil {
		if errors.Is(err, query.ErrNotFound) {
			return DeleteFamily404JSONResponse{NotFoundJSONResponse{
				Code:    "not_found",
				Message: "Family not found",
			}}, nil
		}
		return nil, err
	}

	err = ss.server.commandHandler.DeleteFamily(ctx, command.DeleteFamilyInput{
		ID:      request.Id,
		Version: family.Version,
	})
	if err != nil {
		if errors.Is(err, query.ErrNotFound) {
			return DeleteFamily404JSONResponse{NotFoundJSONResponse{
				Code:    "not_found",
				Message: "Family not found",
			}}, nil
		}
		return nil, err
	}

	return DeleteFamily204Response{}, nil
}

// AddChildToFamily implements StrictServerInterface.
func (ss *StrictServer) AddChildToFamily(ctx context.Context, request AddChildToFamilyRequestObject) (AddChildToFamilyResponseObject, error) {
	input := command.LinkChildInput{
		FamilyID: request.Id,
		ChildID:  request.Body.PersonId,
	}
	if request.Body.RelationshipType != nil {
		input.RelationType = string(*request.Body.RelationshipType)
	}

	_, err := ss.server.commandHandler.LinkChild(ctx, input)
	if err != nil {
		if errors.Is(err, query.ErrNotFound) {
			return AddChildToFamily404JSONResponse{NotFoundJSONResponse{
				Code:    "not_found",
				Message: "Family or person not found",
			}}, nil
		}
		return nil, err
	}

	// Return the linked child info
	relType := FamilyChildRelationshipType("biological")
	if request.Body.RelationshipType != nil {
		relType = FamilyChildRelationshipType(*request.Body.RelationshipType)
	}

	return AddChildToFamily201JSONResponse(FamilyChild{
		PersonId:         request.Body.PersonId,
		RelationshipType: relType,
	}), nil
}

// RemoveChildFromFamily implements StrictServerInterface.
func (ss *StrictServer) RemoveChildFromFamily(ctx context.Context, request RemoveChildFromFamilyRequestObject) (RemoveChildFromFamilyResponseObject, error) {
	err := ss.server.commandHandler.UnlinkChild(ctx, command.UnlinkChildInput{
		FamilyID: request.Id,
		ChildID:  request.PersonId,
	})
	if err != nil {
		if errors.Is(err, query.ErrNotFound) {
			return RemoveChildFromFamily404JSONResponse{NotFoundJSONResponse{
				Code:    "not_found",
				Message: "Family or child not found",
			}}, nil
		}
		return nil, err
	}

	return RemoveChildFromFamily204Response{}, nil
}

// GetFamilyGroupSheet implements StrictServerInterface.
func (ss *StrictServer) GetFamilyGroupSheet(ctx context.Context, request GetFamilyGroupSheetRequestObject) (GetFamilyGroupSheetResponseObject, error) {
	gs, err := ss.server.familyService.GetGroupSheet(ctx, request.Id)
	if err != nil {
		if errors.Is(err, query.ErrNotFound) {
			return GetFamilyGroupSheet404JSONResponse{NotFoundJSONResponse{
				Code:    "not_found",
				Message: "Family not found",
			}}, nil
		}
		return nil, err
	}

	return GetFamilyGroupSheet200JSONResponse(convertQueryGroupSheetToGenerated(gs)), nil
}

// GetFamilyHistory implements StrictServerInterface.
func (ss *StrictServer) GetFamilyHistory(ctx context.Context, request GetFamilyHistoryRequestObject) (GetFamilyHistoryResponseObject, error) {
	_, err := ss.server.familyService.GetFamily(ctx, request.Id)
	if err != nil {
		if errors.Is(err, query.ErrNotFound) {
			return GetFamilyHistory404JSONResponse{NotFoundJSONResponse{
				Code:    "not_found",
				Message: "Family not found",
			}}, nil
		}
		return nil, err
	}

	limit := 20
	offset := 0
	if request.Params.Limit != nil {
		limit = *request.Params.Limit
	}
	if request.Params.Offset != nil {
		offset = *request.Params.Offset
	}

	result, err := ss.server.historyService.GetEntityHistory(ctx, "family", request.Id, limit, offset)
	if err != nil {
		return nil, err
	}

	return GetFamilyHistory200JSONResponse(convertHistoryResult(result)), nil
}

// GetFamilyRestorePoints implements StrictServerInterface.
func (ss *StrictServer) GetFamilyRestorePoints(ctx context.Context, request GetFamilyRestorePointsRequestObject) (GetFamilyRestorePointsResponseObject, error) {
	_, err := ss.server.familyService.GetFamily(ctx, request.Id)
	if err != nil {
		if errors.Is(err, query.ErrNotFound) {
			return GetFamilyRestorePoints404JSONResponse{NotFoundJSONResponse{
				Code:    "not_found",
				Message: "Family not found",
			}}, nil
		}
		return nil, err
	}

	limit := 20
	offset := 0
	if request.Params.Limit != nil {
		limit = *request.Params.Limit
	}
	if request.Params.Offset != nil {
		offset = *request.Params.Offset
	}

	result, err := ss.server.rollbackService.GetRestorePoints(ctx, "Family", request.Id, limit, offset)
	if err != nil {
		if errors.Is(err, query.ErrNoEvents) {
			return GetFamilyRestorePoints404JSONResponse{NotFoundJSONResponse{
				Code:    "not_found",
				Message: "No history found for this entity",
			}}, nil
		}
		return nil, err
	}

	return GetFamilyRestorePoints200JSONResponse(convertRestorePointsResult(result)), nil
}

// RollbackFamily implements StrictServerInterface.
func (ss *StrictServer) RollbackFamily(ctx context.Context, request RollbackFamilyRequestObject) (RollbackFamilyResponseObject, error) {
	_, err := ss.server.familyService.GetFamily(ctx, request.Id)
	if err != nil {
		if errors.Is(err, query.ErrNotFound) {
			return RollbackFamily404JSONResponse{NotFoundJSONResponse{
				Code:    "not_found",
				Message: "Family not found",
			}}, nil
		}
		return nil, err
	}

	if request.Body.TargetVersion < 1 {
		return RollbackFamily400JSONResponse{BadRequestJSONResponse{
			Code:    "bad_request",
			Message: "target_version must be a positive integer",
		}}, nil
	}

	result, err := ss.server.commandHandler.RollbackFamily(ctx, request.Id, request.Body.TargetVersion)
	if err != nil {
		return handleRollbackErrorStrict[RollbackFamilyResponseObject](err,
			func(e Error) RollbackFamilyResponseObject {
				return RollbackFamily400JSONResponse{BadRequestJSONResponse(e)}
			},
			func(e Error) RollbackFamilyResponseObject {
				return RollbackFamily404JSONResponse{NotFoundJSONResponse(e)}
			},
			func(e Error) RollbackFamilyResponseObject { return RollbackFamily409JSONResponse(e) },
		)
	}

	return RollbackFamily200JSONResponse(convertRollbackResult(result, "Family rolled back successfully")), nil
}

// ============================================================================
// GEDCOM endpoints
// ============================================================================

// ExportGedcom implements StrictServerInterface.
func (ss *StrictServer) ExportGedcom(ctx context.Context, _ ExportGedcomRequestObject) (ExportGedcomResponseObject, error) {
	gedcomExporter := gedcom.NewExporter(ss.server.readStore)

	var sb strings.Builder
	_, err := gedcomExporter.Export(ctx, &sb)
	if err != nil {
		return nil, err
	}

	return ExportGedcom200ApplicationxGedcomResponse{
		Body:          strings.NewReader(sb.String()),
		ContentLength: int64(sb.Len()),
		Headers: ExportGedcom200ResponseHeaders{
			ContentDisposition: "attachment; filename=export.ged",
		},
	}, nil
}

// ImportGedcom implements StrictServerInterface.
func (ss *StrictServer) ImportGedcom(ctx context.Context, request ImportGedcomRequestObject) (ImportGedcomResponseObject, error) {
	if request.Body == nil {
		return ImportGedcom400JSONResponse{
			Code:    "bad_request",
			Message: "No file uploaded",
		}, nil
	}

	// Read the first part from multipart reader
	part, err := request.Body.NextPart()
	if err != nil {
		return ImportGedcom400JSONResponse{
			Code:    "bad_request",
			Message: "Failed to read uploaded file",
		}, nil
	}
	defer part.Close()

	result, err := ss.server.commandHandler.ImportGedcom(ctx, command.ImportGedcomInput{
		Filename: part.FileName(),
		FileSize: 0, // Size not available from multipart part
		Reader:   part,
	})
	if err != nil {
		return ImportGedcom400JSONResponse{
			Code:    "bad_request",
			Message: err.Error(),
		}, nil
	}

	warnings := make([]ImportWarning, len(result.Warnings))
	for i, w := range result.Warnings {
		warnings[i] = ImportWarning{
			Line:    0, // Not tracked in current implementation
			Message: w,
		}
	}

	return ImportGedcom200JSONResponse{
		FamiliesImported: result.FamiliesImported,
		PersonsImported:  result.PersonsImported,
		Success:          true,
		Warnings:         &warnings,
	}, nil
}

// ============================================================================
// History endpoints
// ============================================================================

// ListHistory implements StrictServerInterface.
func (ss *StrictServer) ListHistory(ctx context.Context, request ListHistoryRequestObject) (ListHistoryResponseObject, error) {
	fromTime := time.Time{}
	toTime := time.Now().Add(24 * time.Hour)

	if request.Params.From != nil {
		fromTime = *request.Params.From
	}
	if request.Params.To != nil {
		toTime = *request.Params.To
	}

	var eventTypes []string
	if request.Params.EntityType != nil {
		eventTypes = mapEntityTypeToEventTypes(string(*request.Params.EntityType))
	}

	limit := 20
	offset := 0
	if request.Params.Limit != nil {
		limit = *request.Params.Limit
	}
	if request.Params.Offset != nil {
		offset = *request.Params.Offset
	}

	result, err := ss.server.historyService.GetGlobalHistory(ctx, query.GetGlobalHistoryInput{
		FromTime:   fromTime,
		ToTime:     toTime,
		EventTypes: eventTypes,
		Limit:      limit,
		Offset:     offset,
	})
	if err != nil {
		return nil, err
	}

	return ListHistory200JSONResponse(convertHistoryResult(result)), nil
}

// ============================================================================
// Media endpoints
// ============================================================================

// DeleteMedia implements StrictServerInterface.
func (ss *StrictServer) DeleteMedia(ctx context.Context, request DeleteMediaRequestObject) (DeleteMediaResponseObject, error) {
	// Get version from params or fetch current version
	var version int64
	if request.Params.Version != nil {
		version = *request.Params.Version
	} else {
		// Fetch current version if not provided
		media, err := ss.server.readStore.GetMedia(ctx, request.Id)
		if err != nil {
			return nil, err
		}
		if media == nil {
			return DeleteMedia404JSONResponse{NotFoundJSONResponse{
				Code:    "not_found",
				Message: "Media not found",
			}}, nil
		}
		version = media.Version
	}

	if err := ss.server.commandHandler.DeleteMedia(ctx, request.Id, version, "user request"); err != nil {
		if errors.Is(err, query.ErrNotFound) {
			return DeleteMedia404JSONResponse{NotFoundJSONResponse{
				Code:    "not_found",
				Message: "Media not found",
			}}, nil
		}
		return nil, err
	}

	return DeleteMedia204Response{}, nil
}

// GetMedia implements StrictServerInterface.
func (ss *StrictServer) GetMedia(ctx context.Context, request GetMediaRequestObject) (GetMediaResponseObject, error) {
	media, err := ss.server.readStore.GetMedia(ctx, request.Id)
	if err != nil {
		return nil, err
	}
	if media == nil {
		return GetMedia404JSONResponse{NotFoundJSONResponse{
			Code:    "not_found",
			Message: "Media not found",
		}}, nil
	}

	return GetMedia200JSONResponse(convertMediaReadModelToGenerated(*media)), nil
}

// UpdateMedia implements StrictServerInterface.
func (ss *StrictServer) UpdateMedia(ctx context.Context, request UpdateMediaRequestObject) (UpdateMediaResponseObject, error) {
	var mediaType *string
	if request.Body.MediaType != nil {
		mt := string(*request.Body.MediaType)
		mediaType = &mt
	}

	result, err := ss.server.commandHandler.UpdateMedia(ctx, command.UpdateMediaInput{
		ID:          request.Id,
		Title:       request.Body.Title,
		Description: request.Body.Description,
		MediaType:   mediaType,
		CropLeft:    request.Body.CropLeft,
		CropTop:     request.Body.CropTop,
		CropWidth:   request.Body.CropWidth,
		CropHeight:  request.Body.CropHeight,
		Version:     request.Body.Version,
	})
	if err != nil {
		if errors.Is(err, repository.ErrConcurrencyConflict) {
			return UpdateMedia409JSONResponse{ConflictJSONResponse{
				Code:    "conflict",
				Message: "Version conflict - entity was modified",
			}}, nil
		}
		if errors.Is(err, query.ErrNotFound) {
			return UpdateMedia404JSONResponse{NotFoundJSONResponse{
				Code:    "not_found",
				Message: "Media not found",
			}}, nil
		}
		return nil, err
	}

	media, err := ss.server.readStore.GetMedia(ctx, request.Id)
	if err != nil {
		return nil, err
	}
	if media == nil {
		return UpdateMedia404JSONResponse{NotFoundJSONResponse{
			Code:    "not_found",
			Message: "Media not found",
		}}, nil
	}
	media.Version = result.Version

	return UpdateMedia200JSONResponse(convertMediaReadModelToGenerated(*media)), nil
}

// DownloadMedia implements StrictServerInterface.
func (ss *StrictServer) DownloadMedia(ctx context.Context, request DownloadMediaRequestObject) (DownloadMediaResponseObject, error) {
	media, err := ss.server.readStore.GetMediaWithData(ctx, request.Id)
	if err != nil {
		return nil, err
	}
	if media == nil {
		return DownloadMedia404JSONResponse{NotFoundJSONResponse{
			Code:    "not_found",
			Message: "Media not found",
		}}, nil
	}

	reader := io.NopCloser(strings.NewReader(string(media.FileData)))
	contentLength := int64(len(media.FileData))

	// Return appropriate response based on MIME type
	switch media.MimeType {
	case "image/jpeg":
		return DownloadMedia200ImagejpegResponse{Body: reader, ContentLength: contentLength}, nil
	case "image/png":
		return DownloadMedia200ImagepngResponse{Body: reader, ContentLength: contentLength}, nil
	case "image/gif":
		return DownloadMedia200ImagegifResponse{Body: reader, ContentLength: contentLength}, nil
	case "application/pdf":
		return DownloadMedia200ApplicationpdfResponse{Body: reader, ContentLength: contentLength}, nil
	default:
		return DownloadMedia200ApplicationoctetStreamResponse{Body: reader, ContentLength: contentLength}, nil
	}
}

// GetMediaThumbnail implements StrictServerInterface.
func (ss *StrictServer) GetMediaThumbnail(ctx context.Context, request GetMediaThumbnailRequestObject) (GetMediaThumbnailResponseObject, error) {
	thumbnail, err := ss.server.readStore.GetMediaThumbnail(ctx, request.Id)
	if err != nil {
		return nil, err
	}
	if len(thumbnail) == 0 {
		return GetMediaThumbnail404JSONResponse{NotFoundJSONResponse{
			Code:    "not_found",
			Message: "Thumbnail not found",
		}}, nil
	}

	return GetMediaThumbnail200ImagejpegResponse{
		Body: io.NopCloser(strings.NewReader(string(thumbnail))),
	}, nil
}

// ============================================================================
// Pedigree endpoint
// ============================================================================

// GetPedigree implements StrictServerInterface.
func (ss *StrictServer) GetPedigree(ctx context.Context, request GetPedigreeRequestObject) (GetPedigreeResponseObject, error) {
	maxGen := 5
	if request.Params.Generations != nil {
		maxGen = *request.Params.Generations
	}

	result, err := ss.server.pedigreeService.GetPedigree(ctx, query.GetPedigreeInput{
		PersonID:       request.Id,
		MaxGenerations: maxGen,
	})
	if err != nil {
		if errors.Is(err, query.ErrNotFound) {
			return GetPedigree404JSONResponse{NotFoundJSONResponse{
				Code:    "not_found",
				Message: "Person not found",
			}}, nil
		}
		return nil, err
	}

	generations := result.MaxGeneration
	totalAncestors := result.TotalAncestors
	maxGeneration := result.MaxGeneration
	return GetPedigree200JSONResponse{
		Root:           convertQueryPedigreeNodeToGenerated(result.Root),
		Generations:    &generations,
		TotalAncestors: &totalAncestors,
		MaxGeneration:  &maxGeneration,
	}, nil
}

// ============================================================================
// Descendancy endpoint
// ============================================================================

// GetDescendancy implements StrictServerInterface.
func (ss *StrictServer) GetDescendancy(ctx context.Context, request GetDescendancyRequestObject) (GetDescendancyResponseObject, error) {
	maxGen := 4
	if request.Params.Generations != nil {
		maxGen = *request.Params.Generations
	}

	result, err := ss.server.descendancyService.GetDescendancy(ctx, query.GetDescendancyInput{
		PersonID:       request.Id,
		MaxGenerations: maxGen,
	})
	if err != nil {
		if errors.Is(err, query.ErrNotFound) {
			return GetDescendancy404JSONResponse{NotFoundJSONResponse{
				Code:    "not_found",
				Message: "Person not found",
			}}, nil
		}
		return nil, err
	}

	generations := result.MaxGeneration
	totalDescendants := result.TotalDescendants
	maxGeneration := result.MaxGeneration
	return GetDescendancy200JSONResponse{
		Root:             convertQueryDescendancyNodeToGenerated(result.Root),
		Generations:      &generations,
		TotalDescendants: &totalDescendants,
		MaxGeneration:    &maxGeneration,
	}, nil
}

// ============================================================================
// Person endpoints
// ============================================================================

// ListPersons implements StrictServerInterface.
func (ss *StrictServer) ListPersons(ctx context.Context, request ListPersonsRequestObject) (ListPersonsResponseObject, error) {
	limit := 20
	offset := 0
	sort := ""
	order := ""

	if request.Params.Limit != nil {
		limit = *request.Params.Limit
	}
	if request.Params.Offset != nil {
		offset = *request.Params.Offset
	}
	if request.Params.Sort != nil {
		sort = string(*request.Params.Sort)
	}
	if request.Params.Order != nil {
		order = string(*request.Params.Order)
	}

	input := query.ListPersonsInput{
		Limit:  limit,
		Offset: offset,
		Sort:   sort,
		Order:  order,
	}
	if request.Params.ResearchStatus != nil {
		rs := string(*request.Params.ResearchStatus)
		input.ResearchStatus = &rs
	}

	result, err := ss.server.personService.ListPersons(ctx, input)
	if err != nil {
		return nil, err
	}

	items := make([]Person, len(result.Items))
	for i, p := range result.Items {
		items[i] = convertQueryPersonToGenerated(p)
	}

	limitVal := result.Limit
	offsetVal := result.Offset
	return ListPersons200JSONResponse{
		Items:  items,
		Total:  result.Total,
		Limit:  &limitVal,
		Offset: &offsetVal,
	}, nil
}

// CreatePerson implements StrictServerInterface.
func (ss *StrictServer) CreatePerson(ctx context.Context, request CreatePersonRequestObject) (CreatePersonResponseObject, error) {
	input := command.CreatePersonInput{
		GivenName: request.Body.GivenName,
		Surname:   request.Body.Surname,
	}
	if request.Body.Gender != nil {
		input.Gender = string(*request.Body.Gender)
	}
	if request.Body.BirthDate != nil {
		input.BirthDate = *request.Body.BirthDate
	}
	if request.Body.BirthPlace != nil {
		input.BirthPlace = *request.Body.BirthPlace
	}
	if request.Body.DeathDate != nil {
		input.DeathDate = *request.Body.DeathDate
	}
	if request.Body.DeathPlace != nil {
		input.DeathPlace = *request.Body.DeathPlace
	}
	if request.Body.Notes != nil {
		input.Notes = *request.Body.Notes
	}
	if request.Body.ResearchStatus != nil {
		input.ResearchStatus = string(*request.Body.ResearchStatus)
	}

	result, err := ss.server.commandHandler.CreatePerson(ctx, input)
	if err != nil {
		return nil, err
	}

	// Create the primary name
	_, _ = ss.server.commandHandler.AddName(ctx, command.AddNameInput{
		PersonID:  result.ID,
		GivenName: request.Body.GivenName,
		Surname:   request.Body.Surname,
		IsPrimary: true,
	})

	person, err := ss.server.personService.GetPerson(ctx, result.ID)
	if err != nil {
		return nil, err
	}

	return CreatePerson201JSONResponse(convertQueryPersonToGenerated(person.Person)), nil
}

// GetPerson implements StrictServerInterface.
func (ss *StrictServer) GetPerson(ctx context.Context, request GetPersonRequestObject) (GetPersonResponseObject, error) {
	person, err := ss.server.personService.GetPerson(ctx, request.Id)
	if err != nil {
		if errors.Is(err, query.ErrNotFound) {
			return GetPerson404JSONResponse{NotFoundJSONResponse{
				Code:    "not_found",
				Message: "Person not found",
			}}, nil
		}
		return nil, err
	}

	return GetPerson200JSONResponse(convertQueryPersonDetailToGenerated(person)), nil
}

// UpdatePerson implements StrictServerInterface.
func (ss *StrictServer) UpdatePerson(ctx context.Context, request UpdatePersonRequestObject) (UpdatePersonResponseObject, error) {
	input := command.UpdatePersonInput{
		ID:      request.Id,
		Version: request.Body.Version,
	}

	if request.Body.GivenName != nil {
		input.GivenName = request.Body.GivenName
	}
	if request.Body.Surname != nil {
		input.Surname = request.Body.Surname
	}
	if request.Body.Gender != nil {
		g := string(*request.Body.Gender)
		input.Gender = &g
	}
	if request.Body.BirthDate != nil {
		input.BirthDate = request.Body.BirthDate
	}
	if request.Body.BirthPlace != nil {
		input.BirthPlace = request.Body.BirthPlace
	}
	if request.Body.DeathDate != nil {
		input.DeathDate = request.Body.DeathDate
	}
	if request.Body.DeathPlace != nil {
		input.DeathPlace = request.Body.DeathPlace
	}
	if request.Body.Notes != nil {
		input.Notes = request.Body.Notes
	}
	if request.Body.ResearchStatus != nil {
		rs := string(*request.Body.ResearchStatus)
		input.ResearchStatus = &rs
	}

	_, err := ss.server.commandHandler.UpdatePerson(ctx, input)
	if err != nil {
		if errors.Is(err, repository.ErrConcurrencyConflict) {
			return UpdatePerson409JSONResponse{ConflictJSONResponse{
				Code:    "conflict",
				Message: "Version conflict - entity was modified",
			}}, nil
		}
		if errors.Is(err, query.ErrNotFound) {
			return UpdatePerson404JSONResponse{NotFoundJSONResponse{
				Code:    "not_found",
				Message: "Person not found",
			}}, nil
		}
		return nil, err
	}

	person, err := ss.server.personService.GetPerson(ctx, request.Id)
	if err != nil {
		return nil, err
	}

	return UpdatePerson200JSONResponse(convertQueryPersonToGenerated(person.Person)), nil
}

// DeletePerson implements StrictServerInterface.
func (ss *StrictServer) DeletePerson(ctx context.Context, request DeletePersonRequestObject) (DeletePersonResponseObject, error) {
	// Get current version since DELETE doesn't accept version parameter per OpenAPI spec
	person, err := ss.server.personService.GetPerson(ctx, request.Id)
	if err != nil {
		if errors.Is(err, query.ErrNotFound) {
			return DeletePerson404JSONResponse{NotFoundJSONResponse{
				Code:    "not_found",
				Message: "Person not found",
			}}, nil
		}
		return nil, err
	}

	err = ss.server.commandHandler.DeletePerson(ctx, command.DeletePersonInput{
		ID:      request.Id,
		Version: person.Version,
	})
	if err != nil {
		if errors.Is(err, query.ErrNotFound) {
			return DeletePerson404JSONResponse{NotFoundJSONResponse{
				Code:    "not_found",
				Message: "Person not found",
			}}, nil
		}
		return nil, err
	}

	return DeletePerson204Response{}, nil
}

// GetCitationsForPerson implements StrictServerInterface.
func (ss *StrictServer) GetCitationsForPerson(ctx context.Context, request GetCitationsForPersonRequestObject) (GetCitationsForPersonResponseObject, error) {
	citations, err := ss.server.sourceService.GetCitationsForPerson(ctx, request.Id)
	if err != nil {
		if errors.Is(err, query.ErrNotFound) {
			return GetCitationsForPerson404JSONResponse{NotFoundJSONResponse{
				Code:    "not_found",
				Message: "Person not found",
			}}, nil
		}
		return nil, err
	}

	items := make([]Citation, len(citations))
	for i, c := range citations {
		items[i] = convertQueryCitationToGenerated(c)
	}

	return GetCitationsForPerson200JSONResponse{
		Citations: items,
		Total:     len(items),
	}, nil
}

// GetPersonHistory implements StrictServerInterface.
func (ss *StrictServer) GetPersonHistory(ctx context.Context, request GetPersonHistoryRequestObject) (GetPersonHistoryResponseObject, error) {
	_, err := ss.server.personService.GetPerson(ctx, request.Id)
	if err != nil {
		if errors.Is(err, query.ErrNotFound) {
			return GetPersonHistory404JSONResponse{NotFoundJSONResponse{
				Code:    "not_found",
				Message: "Person not found",
			}}, nil
		}
		return nil, err
	}

	limit := 20
	offset := 0
	if request.Params.Limit != nil {
		limit = *request.Params.Limit
	}
	if request.Params.Offset != nil {
		offset = *request.Params.Offset
	}

	result, err := ss.server.historyService.GetEntityHistory(ctx, "person", request.Id, limit, offset)
	if err != nil {
		return nil, err
	}

	return GetPersonHistory200JSONResponse(convertHistoryResult(result)), nil
}

// ListPersonMedia implements StrictServerInterface.
func (ss *StrictServer) ListPersonMedia(ctx context.Context, request ListPersonMediaRequestObject) (ListPersonMediaResponseObject, error) {
	person, err := ss.server.readStore.GetPerson(ctx, request.Id)
	if err != nil {
		return nil, err
	}
	if person == nil {
		return ListPersonMedia404JSONResponse{NotFoundJSONResponse{
			Code:    "not_found",
			Message: "Person not found",
		}}, nil
	}

	limit := 20
	offset := 0
	if request.Params.Limit != nil {
		limit = *request.Params.Limit
	}
	if request.Params.Offset != nil {
		offset = *request.Params.Offset
	}

	items, total, err := ss.server.readStore.ListMediaForEntity(ctx, "person", request.Id, repository.ListOptions{
		Limit:  limit,
		Offset: offset,
	})
	if err != nil {
		return nil, err
	}

	mediaItems := make([]Media, len(items))
	for i, m := range items {
		mediaItems[i] = convertMediaReadModelToGenerated(m)
	}

	return ListPersonMedia200JSONResponse{
		Items: mediaItems,
		Total: total,
	}, nil
}

// UploadPersonMedia implements StrictServerInterface.
func (ss *StrictServer) UploadPersonMedia(ctx context.Context, request UploadPersonMediaRequestObject) (UploadPersonMediaResponseObject, error) {
	person, err := ss.server.readStore.GetPerson(ctx, request.Id)
	if err != nil {
		return nil, err
	}
	if person == nil {
		return UploadPersonMedia404JSONResponse{NotFoundJSONResponse{
			Code:    "not_found",
			Message: "Person not found",
		}}, nil
	}

	if request.Body == nil {
		return UploadPersonMedia400JSONResponse{BadRequestJSONResponse{
			Code:    "bad_request",
			Message: "No file uploaded",
		}}, nil
	}

	// Read the first part from multipart reader (the file)
	part, err := request.Body.NextPart()
	if err != nil {
		return UploadPersonMedia400JSONResponse{BadRequestJSONResponse{
			Code:    "bad_request",
			Message: "Failed to read uploaded file",
		}}, nil
	}
	defer part.Close()

	fileData, err := io.ReadAll(part)
	if err != nil {
		return nil, err
	}

	if int64(len(fileData)) > domain.MaxMediaFileSize {
		return UploadPersonMedia400JSONResponse{BadRequestJSONResponse{
			Code:    "bad_request",
			Message: "File too large (max 10MB)",
		}}, nil
	}

	// For now, use the part name and filename as title if available
	title := part.FileName()
	if title == "" {
		title = "Uploaded media"
	}

	result, err := ss.server.commandHandler.UploadMedia(ctx, command.UploadMediaInput{
		EntityType:  "person",
		EntityID:    request.Id,
		Title:       title,
		Description: "",
		MediaType:   "",
		Filename:    part.FileName(),
		FileData:    fileData,
	})
	if err != nil {
		return nil, err
	}

	media, err := ss.server.readStore.GetMedia(ctx, result.ID)
	if err != nil {
		return nil, err
	}
	if media == nil {
		return nil, errors.New("failed to retrieve created media")
	}

	return UploadPersonMedia201JSONResponse(convertMediaReadModelToGenerated(*media)), nil
}

// GetPersonNames implements StrictServerInterface.
func (ss *StrictServer) GetPersonNames(ctx context.Context, request GetPersonNamesRequestObject) (GetPersonNamesResponseObject, error) {
	names, err := ss.server.readStore.GetPersonNames(ctx, request.Id)
	if err != nil {
		return nil, err
	}

	items := make([]PersonName, len(names))
	for i, n := range names {
		items[i] = convertPersonNameReadModelToGenerated(n)
	}

	return GetPersonNames200JSONResponse{
		Items: items,
		Total: len(items),
	}, nil
}

// AddPersonName implements StrictServerInterface.
func (ss *StrictServer) AddPersonName(ctx context.Context, request AddPersonNameRequestObject) (AddPersonNameResponseObject, error) {
	input := command.AddNameInput{
		PersonID:  request.Id,
		GivenName: request.Body.GivenName,
		Surname:   request.Body.Surname,
	}
	if request.Body.NamePrefix != nil {
		input.NamePrefix = *request.Body.NamePrefix
	}
	if request.Body.NameSuffix != nil {
		input.NameSuffix = *request.Body.NameSuffix
	}
	if request.Body.SurnamePrefix != nil {
		input.SurnamePrefix = *request.Body.SurnamePrefix
	}
	if request.Body.Nickname != nil {
		input.Nickname = *request.Body.Nickname
	}
	// NameType is required (not a pointer) per OpenAPI spec
	input.NameType = string(request.Body.NameType)
	if request.Body.IsPrimary != nil {
		input.IsPrimary = *request.Body.IsPrimary
	}

	result, err := ss.server.commandHandler.AddName(ctx, input)
	if err != nil {
		if errors.Is(err, query.ErrNotFound) {
			return AddPersonName404JSONResponse{NotFoundJSONResponse{
				Code:    "not_found",
				Message: "Person not found",
			}}, nil
		}
		return nil, err
	}

	name, err := ss.server.readStore.GetPersonName(ctx, result.ID)
	if err != nil {
		return nil, err
	}

	return AddPersonName201JSONResponse(convertPersonNameReadModelToGenerated(*name)), nil
}

// UpdatePersonName implements StrictServerInterface.
func (ss *StrictServer) UpdatePersonName(ctx context.Context, request UpdatePersonNameRequestObject) (UpdatePersonNameResponseObject, error) {
	var nameType *string
	if request.Body.NameType != nil {
		nt := string(*request.Body.NameType)
		nameType = &nt
	}

	input := command.UpdateNameInput{
		PersonID:      request.Id,
		NameID:        request.NameId,
		GivenName:     request.Body.GivenName,
		Surname:       request.Body.Surname,
		NamePrefix:    request.Body.NamePrefix,
		NameSuffix:    request.Body.NameSuffix,
		SurnamePrefix: request.Body.SurnamePrefix,
		Nickname:      request.Body.Nickname,
		NameType:      nameType,
		IsPrimary:     request.Body.IsPrimary,
	}

	_, err := ss.server.commandHandler.UpdateName(ctx, input)
	if err != nil {
		if errors.Is(err, query.ErrNotFound) {
			return UpdatePersonName404JSONResponse{NotFoundJSONResponse{
				Code:    "not_found",
				Message: "Person or name not found",
			}}, nil
		}
		return nil, err
	}

	name, err := ss.server.readStore.GetPersonName(ctx, request.NameId)
	if err != nil {
		return nil, err
	}

	return UpdatePersonName200JSONResponse(convertPersonNameReadModelToGenerated(*name)), nil
}

// DeletePersonName implements StrictServerInterface.
func (ss *StrictServer) DeletePersonName(ctx context.Context, request DeletePersonNameRequestObject) (DeletePersonNameResponseObject, error) {
	err := ss.server.commandHandler.DeleteName(ctx, command.DeleteNameInput{
		PersonID: request.Id,
		NameID:   request.NameId,
	})
	if err != nil {
		if errors.Is(err, query.ErrNotFound) {
			return DeletePersonName404JSONResponse{NotFoundJSONResponse{
				Code:    "not_found",
				Message: "Person or name not found",
			}}, nil
		}
		return nil, err
	}

	return DeletePersonName204Response{}, nil
}

// GetPersonRestorePoints implements StrictServerInterface.
func (ss *StrictServer) GetPersonRestorePoints(ctx context.Context, request GetPersonRestorePointsRequestObject) (GetPersonRestorePointsResponseObject, error) {
	_, err := ss.server.personService.GetPerson(ctx, request.Id)
	if err != nil {
		if errors.Is(err, query.ErrNotFound) {
			return GetPersonRestorePoints404JSONResponse{NotFoundJSONResponse{
				Code:    "not_found",
				Message: "Person not found",
			}}, nil
		}
		return nil, err
	}

	limit := 20
	offset := 0
	if request.Params.Limit != nil {
		limit = *request.Params.Limit
	}
	if request.Params.Offset != nil {
		offset = *request.Params.Offset
	}

	result, err := ss.server.rollbackService.GetRestorePoints(ctx, "Person", request.Id, limit, offset)
	if err != nil {
		if errors.Is(err, query.ErrNoEvents) {
			return GetPersonRestorePoints404JSONResponse{NotFoundJSONResponse{
				Code:    "not_found",
				Message: "No history found for this entity",
			}}, nil
		}
		return nil, err
	}

	return GetPersonRestorePoints200JSONResponse(convertRestorePointsResult(result)), nil
}

// RollbackPerson implements StrictServerInterface.
func (ss *StrictServer) RollbackPerson(ctx context.Context, request RollbackPersonRequestObject) (RollbackPersonResponseObject, error) {
	_, err := ss.server.personService.GetPerson(ctx, request.Id)
	if err != nil {
		if errors.Is(err, query.ErrNotFound) {
			return RollbackPerson404JSONResponse{NotFoundJSONResponse{
				Code:    "not_found",
				Message: "Person not found",
			}}, nil
		}
		return nil, err
	}

	if request.Body.TargetVersion < 1 {
		return RollbackPerson400JSONResponse{BadRequestJSONResponse{
			Code:    "bad_request",
			Message: "target_version must be a positive integer",
		}}, nil
	}

	result, err := ss.server.commandHandler.RollbackPerson(ctx, request.Id, request.Body.TargetVersion)
	if err != nil {
		return handleRollbackErrorStrict[RollbackPersonResponseObject](err,
			func(e Error) RollbackPersonResponseObject {
				return RollbackPerson400JSONResponse{BadRequestJSONResponse(e)}
			},
			func(e Error) RollbackPersonResponseObject {
				return RollbackPerson404JSONResponse{NotFoundJSONResponse(e)}
			},
			func(e Error) RollbackPersonResponseObject { return RollbackPerson409JSONResponse(e) },
		)
	}

	return RollbackPerson200JSONResponse(convertRollbackResult(result, "Person rolled back successfully")), nil
}

// ============================================================================
// Quality endpoints
// ============================================================================

// GetQualityOverview implements StrictServerInterface.
func (ss *StrictServer) GetQualityOverview(ctx context.Context, request GetQualityOverviewRequestObject) (GetQualityOverviewResponseObject, error) {
	result, err := ss.server.qualityService.GetQualityOverview(ctx)
	if err != nil {
		return nil, err
	}

	topIssues := make([]QualityIssue, len(result.TopIssues))
	for i, issue := range result.TopIssues {
		topIssues[i] = QualityIssue{
			Issue: issue.Issue,
			Count: issue.Count,
		}
	}

	return GetQualityOverview200JSONResponse{
		TotalPersons:        result.TotalPersons,
		AverageCompleteness: float32(result.AverageCompleteness),
		RecordsWithIssues:   result.RecordsWithIssues,
		TopIssues:           topIssues,
	}, nil
}

// GetPersonQuality implements StrictServerInterface.
func (ss *StrictServer) GetPersonQuality(ctx context.Context, request GetPersonQualityRequestObject) (GetPersonQualityResponseObject, error) {
	result, err := ss.server.qualityService.GetPersonQuality(ctx, request.Id)
	if err != nil {
		if errors.Is(err, query.ErrNotFound) {
			return GetPersonQuality404JSONResponse{NotFoundJSONResponse{
				Code:    "not_found",
				Message: "Person not found",
			}}, nil
		}
		return nil, err
	}

	issues := result.Issues
	if issues == nil {
		issues = []string{}
	}
	suggestions := result.Suggestions
	if suggestions == nil {
		suggestions = []string{}
	}

	return GetPersonQuality200JSONResponse{
		PersonId:          result.PersonID,
		CompletenessScore: float32(result.CompletenessScore),
		Issues:            issues,
		Suggestions:       suggestions,
	}, nil
}

// GetQualityReport implements StrictServerInterface.
func (ss *StrictServer) GetQualityReport(ctx context.Context, request GetQualityReportRequestObject) (GetQualityReportResponseObject, error) {
	result, err := ss.server.validationService.GetQualityReport(ctx)
	if err != nil {
		return nil, err
	}

	// Map domain type to API type
	topIssues := make([]QualityReportIssue, len(result.TopIssues))
	for i, issue := range result.TopIssues {
		topIssues[i] = QualityReportIssue{
			Code:  issue.Code,
			Count: issue.Count,
		}
	}

	return GetQualityReport200JSONResponse{
		TotalIndividuals:  result.TotalIndividuals,
		TotalFamilies:     result.TotalFamilies,
		TotalSources:      result.TotalSources,
		BirthDateCoverage: float32(result.BirthDateCoverage),
		DeathDateCoverage: float32(result.DeathDateCoverage),
		SourceCoverage:    float32(result.SourceCoverage),
		ErrorCount:        result.ErrorCount,
		WarningCount:      result.WarningCount,
		InfoCount:         result.InfoCount,
		TopIssues:         topIssues,
	}, nil
}

// GetValidationIssues implements StrictServerInterface.
func (ss *StrictServer) GetValidationIssues(ctx context.Context, request GetValidationIssuesRequestObject) (GetValidationIssuesResponseObject, error) {
	// Extract severity filter from params (optional)
	var severityFilter string
	if request.Params.Severity != nil {
		severityFilter = string(*request.Params.Severity)
	}

	results, err := ss.server.validationService.GetValidationIssues(ctx, severityFilter)
	if err != nil {
		return nil, err
	}

	// Map to API types and count issues by severity
	issues := make([]ValidationIssue, len(results))
	errorCount, warningCount, infoCount := 0, 0, 0
	for i, r := range results {
		issues[i] = ValidationIssue{
			Severity: ValidationIssueSeverity(r.Severity),
			Code:     r.Code,
			Message:  r.Message,
			RecordId: r.RecordID,
		}
		if r.RelatedRecordID != nil {
			issues[i].RelatedRecordId = r.RelatedRecordID
		}

		// Count by severity
		switch r.Severity {
		case "error":
			errorCount++
		case "warning":
			warningCount++
		case "info":
			infoCount++
		}
	}

	return GetValidationIssues200JSONResponse{
		Issues:       issues,
		ErrorCount:   errorCount,
		WarningCount: warningCount,
		InfoCount:    infoCount,
	}, nil
}

// GetPersonsDuplicates implements StrictServerInterface.
func (ss *StrictServer) GetPersonsDuplicates(ctx context.Context, request GetPersonsDuplicatesRequestObject) (GetPersonsDuplicatesResponseObject, error) {
	// Extract limit/offset from params with defaults
	limit := 100
	offset := 0
	if request.Params.Limit != nil {
		limit = *request.Params.Limit
	}
	if request.Params.Offset != nil {
		offset = *request.Params.Offset
	}

	results, total, err := ss.server.validationService.FindDuplicates(ctx, limit, offset)
	if err != nil {
		return nil, err
	}

	// Map to API types
	duplicates := make([]DuplicatePair, len(results))
	for i, r := range results {
		duplicates[i] = DuplicatePair{
			Person1Id:    r.Person1ID,
			Person1Name:  r.Person1Name,
			Person2Id:    r.Person2ID,
			Person2Name:  r.Person2Name,
			Confidence:   float32(r.Confidence),
			MatchReasons: r.MatchReasons,
		}
	}

	return GetPersonsDuplicates200JSONResponse{
		Duplicates: duplicates,
		Total:      total,
	}, nil
}

// ============================================================================
// Search endpoint
// ============================================================================

// SearchPersons implements StrictServerInterface.
func (ss *StrictServer) SearchPersons(ctx context.Context, request SearchPersonsRequestObject) (SearchPersonsResponseObject, error) {
	if len(request.Params.Q) < 2 {
		return SearchPersons400JSONResponse{BadRequestJSONResponse{
			Code:    "bad_request",
			Message: "Search query must be at least 2 characters",
		}}, nil
	}

	fuzzy := false
	if request.Params.Fuzzy != nil {
		fuzzy = *request.Params.Fuzzy
	}

	limit := 20
	if request.Params.Limit != nil {
		limit = *request.Params.Limit
	}

	result, err := ss.server.personService.SearchPersons(ctx, query.SearchPersonsInput{
		Query: request.Params.Q,
		Fuzzy: fuzzy,
		Limit: limit,
	})
	if err != nil {
		return nil, err
	}

	items := make([]SearchResult, len(result.Items))
	for i, r := range result.Items {
		score := float32(r.Score)
		items[i] = SearchResult{
			Id:        r.ID,
			GivenName: r.GivenName,
			Surname:   r.Surname,
			Score:     &score,
		}
		if r.BirthDate != nil {
			items[i].BirthDate = convertDomainGenDateToGenerated(r.BirthDate)
		}
		if r.DeathDate != nil {
			items[i].DeathDate = convertDomainGenDateToGenerated(r.DeathDate)
		}
	}

	queryStr := result.Query
	return SearchPersons200JSONResponse{
		Items: items,
		Total: result.Total,
		Query: &queryStr,
	}, nil
}

// ============================================================================
// Source endpoints
// ============================================================================

// ListSources implements StrictServerInterface.
func (ss *StrictServer) ListSources(ctx context.Context, request ListSourcesRequestObject) (ListSourcesResponseObject, error) {
	limit := 20
	offset := 0
	sortBy := ""
	sortOrder := ""

	if request.Params.Limit != nil {
		limit = *request.Params.Limit
	}
	if request.Params.Offset != nil {
		offset = *request.Params.Offset
	}
	if request.Params.Sort != nil {
		sortBy = string(*request.Params.Sort)
	}
	if request.Params.Order != nil {
		sortOrder = string(*request.Params.Order)
	}

	result, err := ss.server.sourceService.ListSources(ctx, query.ListSourcesInput{
		Limit:     limit,
		Offset:    offset,
		SortBy:    sortBy,
		SortOrder: sortOrder,
	})
	if err != nil {
		return nil, err
	}

	items := make([]Source, len(result.Sources))
	for i, s := range result.Sources {
		items[i] = convertQuerySourceToGenerated(s)
	}

	limitVal := result.Limit
	offsetVal := result.Offset
	return ListSources200JSONResponse{
		Sources: items,
		Total:   result.Total,
		Limit:   &limitVal,
		Offset:  &offsetVal,
	}, nil
}

// CreateSource implements StrictServerInterface.
func (ss *StrictServer) CreateSource(ctx context.Context, request CreateSourceRequestObject) (CreateSourceResponseObject, error) {
	input := command.CreateSourceInput{
		SourceType: request.Body.SourceType,
		Title:      request.Body.Title,
	}

	if request.Body.Author != nil {
		input.Author = *request.Body.Author
	}
	if request.Body.Publisher != nil {
		input.Publisher = *request.Body.Publisher
	}
	if request.Body.PublishDate != nil {
		input.PublishDate = *request.Body.PublishDate
	}
	if request.Body.Url != nil {
		input.URL = *request.Body.Url
	}
	if request.Body.RepositoryName != nil {
		input.RepositoryName = *request.Body.RepositoryName
	}
	if request.Body.CollectionName != nil {
		input.CollectionName = *request.Body.CollectionName
	}
	if request.Body.CallNumber != nil {
		input.CallNumber = *request.Body.CallNumber
	}
	if request.Body.Notes != nil {
		input.Notes = *request.Body.Notes
	}

	result, err := ss.server.commandHandler.CreateSource(ctx, input)
	if err != nil {
		return nil, err
	}

	source, err := ss.server.sourceService.GetSource(ctx, result.ID)
	if err != nil {
		return nil, err
	}

	return CreateSource201JSONResponse(convertQuerySourceDetailToGenerated(*source)), nil
}

// SearchSources implements StrictServerInterface.
func (ss *StrictServer) SearchSources(ctx context.Context, request SearchSourcesRequestObject) (SearchSourcesResponseObject, error) {
	if len(request.Params.Q) < 2 {
		return SearchSources400JSONResponse{BadRequestJSONResponse{
			Code:    "bad_request",
			Message: "Search query must be at least 2 characters",
		}}, nil
	}

	limit := 20
	if request.Params.Limit != nil {
		limit = *request.Params.Limit
	}

	sources, err := ss.server.sourceService.SearchSources(ctx, request.Params.Q, limit)
	if err != nil {
		return nil, err
	}

	items := make([]Source, len(sources))
	for i, s := range sources {
		items[i] = convertQuerySourceToGenerated(s)
	}

	return SearchSources200JSONResponse{
		Sources: items,
		Total:   len(items),
	}, nil
}

// GetSource implements StrictServerInterface.
func (ss *StrictServer) GetSource(ctx context.Context, request GetSourceRequestObject) (GetSourceResponseObject, error) {
	source, err := ss.server.sourceService.GetSource(ctx, request.Id)
	if err != nil {
		if errors.Is(err, query.ErrNotFound) {
			return GetSource404JSONResponse{NotFoundJSONResponse{
				Code:    "not_found",
				Message: "Source not found",
			}}, nil
		}
		return nil, err
	}

	return GetSource200JSONResponse(convertQuerySourceDetailToGenerated(*source)), nil
}

// UpdateSource implements StrictServerInterface.
func (ss *StrictServer) UpdateSource(ctx context.Context, request UpdateSourceRequestObject) (UpdateSourceResponseObject, error) {
	input := command.UpdateSourceInput{
		ID:      request.Id,
		Version: request.Body.Version,
	}

	if request.Body.SourceType != nil {
		input.SourceType = request.Body.SourceType
	}
	if request.Body.Title != nil {
		input.Title = request.Body.Title
	}
	if request.Body.Author != nil {
		input.Author = request.Body.Author
	}
	if request.Body.Publisher != nil {
		input.Publisher = request.Body.Publisher
	}
	if request.Body.PublishDate != nil {
		input.PublishDate = request.Body.PublishDate
	}
	if request.Body.Url != nil {
		input.URL = request.Body.Url
	}
	if request.Body.RepositoryName != nil {
		input.RepositoryName = request.Body.RepositoryName
	}
	if request.Body.CollectionName != nil {
		input.CollectionName = request.Body.CollectionName
	}
	if request.Body.CallNumber != nil {
		input.CallNumber = request.Body.CallNumber
	}
	if request.Body.Notes != nil {
		input.Notes = request.Body.Notes
	}

	_, err := ss.server.commandHandler.UpdateSource(ctx, input)
	if err != nil {
		if errors.Is(err, repository.ErrConcurrencyConflict) {
			return UpdateSource409JSONResponse{ConflictJSONResponse{
				Code:    "conflict",
				Message: "Version conflict - entity was modified",
			}}, nil
		}
		if errors.Is(err, query.ErrNotFound) {
			return UpdateSource404JSONResponse{NotFoundJSONResponse{
				Code:    "not_found",
				Message: "Source not found",
			}}, nil
		}
		return nil, err
	}

	source, err := ss.server.sourceService.GetSource(ctx, request.Id)
	if err != nil {
		return nil, err
	}

	return UpdateSource200JSONResponse(convertQuerySourceToGenerated(source.Source)), nil
}

// DeleteSource implements StrictServerInterface.
func (ss *StrictServer) DeleteSource(ctx context.Context, request DeleteSourceRequestObject) (DeleteSourceResponseObject, error) {
	var version int64
	if request.Params.Version != nil {
		version = *request.Params.Version
	} else {
		source, err := ss.server.sourceService.GetSource(ctx, request.Id)
		if err != nil {
			if errors.Is(err, query.ErrNotFound) {
				return DeleteSource404JSONResponse{NotFoundJSONResponse{
					Code:    "not_found",
					Message: "Source not found",
				}}, nil
			}
			return nil, err
		}
		version = source.Version
	}

	err := ss.server.commandHandler.DeleteSource(ctx, request.Id, version, "")
	if err != nil {
		if errors.Is(err, query.ErrNotFound) {
			return DeleteSource404JSONResponse{NotFoundJSONResponse{
				Code:    "not_found",
				Message: "Source not found",
			}}, nil
		}
		return nil, err
	}

	return DeleteSource204Response{}, nil
}

// GetCitationsForSource implements StrictServerInterface.
func (ss *StrictServer) GetCitationsForSource(ctx context.Context, request GetCitationsForSourceRequestObject) (GetCitationsForSourceResponseObject, error) {
	source, err := ss.server.sourceService.GetSource(ctx, request.Id)
	if err != nil {
		if errors.Is(err, query.ErrNotFound) {
			return GetCitationsForSource404JSONResponse{NotFoundJSONResponse{
				Code:    "not_found",
				Message: "Source not found",
			}}, nil
		}
		return nil, err
	}

	items := make([]Citation, len(source.Citations))
	for i, c := range source.Citations {
		items[i] = convertQueryCitationToGenerated(c)
	}

	return GetCitationsForSource200JSONResponse{
		Citations: items,
		Total:     len(items),
	}, nil
}

// GetSourceHistory implements StrictServerInterface.
func (ss *StrictServer) GetSourceHistory(ctx context.Context, request GetSourceHistoryRequestObject) (GetSourceHistoryResponseObject, error) {
	_, err := ss.server.sourceService.GetSource(ctx, request.Id)
	if err != nil {
		if errors.Is(err, query.ErrNotFound) {
			return GetSourceHistory404JSONResponse{NotFoundJSONResponse{
				Code:    "not_found",
				Message: "Source not found",
			}}, nil
		}
		return nil, err
	}

	limit := 20
	offset := 0
	if request.Params.Limit != nil {
		limit = *request.Params.Limit
	}
	if request.Params.Offset != nil {
		offset = *request.Params.Offset
	}

	result, err := ss.server.historyService.GetEntityHistory(ctx, "source", request.Id, limit, offset)
	if err != nil {
		return nil, err
	}

	return GetSourceHistory200JSONResponse(convertHistoryResult(result)), nil
}

// GetSourceRestorePoints implements StrictServerInterface.
func (ss *StrictServer) GetSourceRestorePoints(ctx context.Context, request GetSourceRestorePointsRequestObject) (GetSourceRestorePointsResponseObject, error) {
	_, err := ss.server.sourceService.GetSource(ctx, request.Id)
	if err != nil {
		if errors.Is(err, query.ErrNotFound) {
			return GetSourceRestorePoints404JSONResponse{NotFoundJSONResponse{
				Code:    "not_found",
				Message: "Source not found",
			}}, nil
		}
		return nil, err
	}

	limit := 20
	offset := 0
	if request.Params.Limit != nil {
		limit = *request.Params.Limit
	}
	if request.Params.Offset != nil {
		offset = *request.Params.Offset
	}

	result, err := ss.server.rollbackService.GetRestorePoints(ctx, "Source", request.Id, limit, offset)
	if err != nil {
		if errors.Is(err, query.ErrNoEvents) {
			return GetSourceRestorePoints404JSONResponse{NotFoundJSONResponse{
				Code:    "not_found",
				Message: "No history found for this entity",
			}}, nil
		}
		return nil, err
	}

	return GetSourceRestorePoints200JSONResponse(convertRestorePointsResult(result)), nil
}

// RollbackSource implements StrictServerInterface.
func (ss *StrictServer) RollbackSource(ctx context.Context, request RollbackSourceRequestObject) (RollbackSourceResponseObject, error) {
	_, err := ss.server.sourceService.GetSource(ctx, request.Id)
	if err != nil {
		if errors.Is(err, query.ErrNotFound) {
			return RollbackSource404JSONResponse{NotFoundJSONResponse{
				Code:    "not_found",
				Message: "Source not found",
			}}, nil
		}
		return nil, err
	}

	if request.Body.TargetVersion < 1 {
		return RollbackSource400JSONResponse{BadRequestJSONResponse{
			Code:    "bad_request",
			Message: "target_version must be a positive integer",
		}}, nil
	}

	result, err := ss.server.commandHandler.RollbackSource(ctx, request.Id, request.Body.TargetVersion)
	if err != nil {
		return handleRollbackErrorStrict[RollbackSourceResponseObject](err,
			func(e Error) RollbackSourceResponseObject {
				return RollbackSource400JSONResponse{BadRequestJSONResponse(e)}
			},
			func(e Error) RollbackSourceResponseObject {
				return RollbackSource404JSONResponse{NotFoundJSONResponse(e)}
			},
			func(e Error) RollbackSourceResponseObject { return RollbackSource409JSONResponse(e) },
		)
	}

	return RollbackSource200JSONResponse(convertRollbackResult(result, "Source rolled back successfully")), nil
}

// ============================================================================
// Statistics endpoint
// ============================================================================

// GetStatistics implements StrictServerInterface.
func (ss *StrictServer) GetStatistics(ctx context.Context, request GetStatisticsRequestObject) (GetStatisticsResponseObject, error) {
	result, err := ss.server.qualityService.GetStatistics(ctx)
	if err != nil {
		return nil, err
	}

	topSurnames := make([]SurnameCount, len(result.TopSurnames))
	for i, s := range result.TopSurnames {
		topSurnames[i] = SurnameCount{
			Surname: s.Surname,
			Count:   s.Count,
		}
	}

	dateRange := DateRange{
		EarliestBirth: result.DateRange.EarliestBirth,
		LatestBirth:   result.DateRange.LatestBirth,
	}

	return GetStatistics200JSONResponse{
		TotalPersons:  result.TotalPersons,
		TotalFamilies: result.TotalFamilies,
		TopSurnames:   topSurnames,
		DateRange:     dateRange,
		GenderDistribution: GenderDistribution{
			Male:    result.GenderDistribution.Male,
			Female:  result.GenderDistribution.Female,
			Unknown: result.GenderDistribution.Unknown,
		},
	}, nil
}

// ============================================================================
// Conversion helpers
// ============================================================================

// handleRollbackErrorStrict handles rollback errors and returns appropriate response types.
func handleRollbackErrorStrict[T any](err error, badReq func(Error) T, notFound func(Error) T, conflict func(Error) T) (T, error) {
	switch {
	case errors.Is(err, command.ErrRollbackInvalidVersion):
		return badReq(Error{Code: "bad_request", Message: "Invalid target version: must be positive and less than current version"}), nil
	case errors.Is(err, command.ErrRollbackDeletedEntity):
		return conflict(Error{Code: "conflict", Message: "Cannot rollback a deleted entity"}), nil
	case errors.Is(err, command.ErrRollbackNoChanges):
		return badReq(Error{Code: "bad_request", Message: "Target version matches current version, no rollback needed"}), nil
	case errors.Is(err, query.ErrNoEvents):
		return notFound(Error{Code: "not_found", Message: "No history found for this entity"}), nil
	case errors.Is(err, query.ErrInvalidVersion):
		return badReq(Error{Code: "bad_request", Message: "Invalid version specified"}), nil
	default:
		var zero T
		return zero, err
	}
}

// convertQueryPersonToGenerated converts a query.Person to the generated Person type.
func convertQueryPersonToGenerated(p query.Person) Person {
	resp := Person{
		Id:        p.ID,
		GivenName: p.GivenName,
		Surname:   p.Surname,
		Version:   p.Version,
	}

	if p.Gender != nil {
		g := PersonGender(*p.Gender)
		resp.Gender = &g
	}
	if p.BirthDate != nil {
		resp.BirthDate = convertDomainGenDateToGenerated(p.BirthDate)
	}
	if p.BirthPlace != nil {
		resp.BirthPlace = p.BirthPlace
	}
	if p.DeathDate != nil {
		resp.DeathDate = convertDomainGenDateToGenerated(p.DeathDate)
	}
	if p.DeathPlace != nil {
		resp.DeathPlace = p.DeathPlace
	}
	if p.Notes != nil {
		resp.Notes = p.Notes
	}
	if p.ResearchStatus != nil {
		rs := ResearchStatus(*p.ResearchStatus)
		resp.ResearchStatus = &rs
	}

	return resp
}

// convertQueryPersonDetailToGenerated converts a query.PersonDetail to the generated PersonDetail type.
func convertQueryPersonDetailToGenerated(pd *query.PersonDetail) PersonDetail {
	resp := PersonDetail{
		Id:        pd.ID,
		GivenName: pd.GivenName,
		Surname:   pd.Surname,
		Version:   pd.Version,
	}

	if pd.Gender != nil {
		g := PersonDetailGender(*pd.Gender)
		resp.Gender = &g
	}
	if pd.BirthDate != nil {
		resp.BirthDate = convertDomainGenDateToGenerated(pd.BirthDate)
	}
	if pd.BirthPlace != nil {
		resp.BirthPlace = pd.BirthPlace
	}
	if pd.DeathDate != nil {
		resp.DeathDate = convertDomainGenDateToGenerated(pd.DeathDate)
	}
	if pd.DeathPlace != nil {
		resp.DeathPlace = pd.DeathPlace
	}
	if pd.Notes != nil {
		resp.Notes = pd.Notes
	}
	if pd.ResearchStatus != nil {
		rs := ResearchStatus(*pd.ResearchStatus)
		resp.ResearchStatus = &rs
	}

	if len(pd.Names) > 0 {
		names := make([]PersonName, len(pd.Names))
		for i, n := range pd.Names {
			names[i] = convertQueryPersonNameToGenerated(n)
		}
		resp.Names = &names
	}

	if len(pd.FamiliesAsPartner) > 0 {
		families := make([]FamilySummary, len(pd.FamiliesAsPartner))
		for i, f := range pd.FamiliesAsPartner {
			families[i] = FamilySummary{
				Id:               f.ID,
				Partner1Name:     f.Partner1Name,
				Partner2Name:     f.Partner2Name,
				RelationshipType: f.RelationshipType,
			}
		}
		resp.FamiliesAsPartner = &families
	}

	if pd.FamilyAsChild != nil {
		fac := FamilySummary{
			Id:               pd.FamilyAsChild.ID,
			Partner1Name:     pd.FamilyAsChild.Partner1Name,
			Partner2Name:     pd.FamilyAsChild.Partner2Name,
			RelationshipType: pd.FamilyAsChild.RelationshipType,
		}
		resp.FamilyAsChild = &fac
	}

	return resp
}

// convertQueryPersonNameToGenerated converts a query.PersonName to the generated PersonName type.
func convertQueryPersonNameToGenerated(n query.PersonName) PersonName {
	return PersonName{
		Id:            n.ID,
		GivenName:     n.GivenName,
		Surname:       n.Surname,
		FullName:      &n.FullName,
		NamePrefix:    &n.NamePrefix,
		NameSuffix:    &n.NameSuffix,
		SurnamePrefix: &n.SurnamePrefix,
		Nickname:      &n.Nickname,
		NameType:      PersonNameNameType(n.NameType),
		IsPrimary:     n.IsPrimary,
	}
}

// convertPersonNameReadModelToGenerated converts a repository.PersonNameReadModel to the generated PersonName type.
func convertPersonNameReadModelToGenerated(n repository.PersonNameReadModel) PersonName {
	return PersonName{
		Id:            n.ID,
		GivenName:     n.GivenName,
		Surname:       n.Surname,
		FullName:      &n.FullName,
		NamePrefix:    &n.NamePrefix,
		NameSuffix:    &n.NameSuffix,
		SurnamePrefix: &n.SurnamePrefix,
		Nickname:      &n.Nickname,
		NameType:      PersonNameNameType(n.NameType),
		IsPrimary:     n.IsPrimary,
	}
}

// convertQueryFamilyToGenerated converts a query.Family to the generated Family type.
func convertQueryFamilyToGenerated(f query.Family) Family {
	resp := Family{
		Id:      f.ID,
		Version: f.Version,
	}

	if f.Partner1ID != nil {
		id := *f.Partner1ID
		resp.Partner1Id = &id
	}
	if f.Partner2ID != nil {
		id := *f.Partner2ID
		resp.Partner2Id = &id
	}
	if f.RelationshipType != nil {
		rt := FamilyRelationshipType(*f.RelationshipType)
		resp.RelationshipType = &rt
	}
	if f.MarriageDate != nil {
		resp.MarriageDate = convertDomainGenDateToGenerated(f.MarriageDate)
	}
	if f.MarriagePlace != nil {
		resp.MarriagePlace = f.MarriagePlace
	}

	return resp
}

// convertQueryFamilyDetailToGenerated converts a query.FamilyDetail to the generated FamilyDetail type.
func convertQueryFamilyDetailToGenerated(fd query.FamilyDetail) FamilyDetail {
	resp := FamilyDetail{
		Id:      fd.ID,
		Version: fd.Version,
	}

	if fd.Partner1ID != nil {
		id := *fd.Partner1ID
		resp.Partner1Id = &id
	}
	if fd.Partner2ID != nil {
		id := *fd.Partner2ID
		resp.Partner2Id = &id
	}
	if fd.RelationshipType != nil {
		rt := FamilyDetailRelationshipType(*fd.RelationshipType)
		resp.RelationshipType = &rt
	}
	if fd.MarriageDate != nil {
		resp.MarriageDate = convertDomainGenDateToGenerated(fd.MarriageDate)
	}
	if fd.MarriagePlace != nil {
		resp.MarriagePlace = fd.MarriagePlace
	}

	// Add partner details if available
	if fd.Partner1Name != nil {
		resp.Partner1 = &PersonSummary{
			GivenName: *fd.Partner1Name,
		}
	}
	if fd.Partner2Name != nil {
		resp.Partner2 = &PersonSummary{
			GivenName: *fd.Partner2Name,
		}
	}

	if len(fd.Children) > 0 {
		children := make([]FamilyChild, len(fd.Children))
		for i, c := range fd.Children {
			children[i] = FamilyChild{
				PersonId:         c.ID,
				RelationshipType: FamilyChildRelationshipType(c.RelationshipType),
			}
		}
		resp.Children = &children
	}

	return resp
}

// convertQueryFamilyToFamilyDetail converts a query.Family to the generated FamilyDetail type.
// This is a simpler conversion when we don't have full FamilyDetail data.
func convertQueryFamilyToFamilyDetail(f query.Family) FamilyDetail {
	resp := FamilyDetail{
		Id:      f.ID,
		Version: f.Version,
	}

	if f.Partner1ID != nil {
		id := *f.Partner1ID
		resp.Partner1Id = &id
	}
	if f.Partner2ID != nil {
		id := *f.Partner2ID
		resp.Partner2Id = &id
	}
	if f.RelationshipType != nil {
		rt := FamilyDetailRelationshipType(*f.RelationshipType)
		resp.RelationshipType = &rt
	}
	if f.MarriageDate != nil {
		resp.MarriageDate = convertDomainGenDateToGenerated(f.MarriageDate)
	}
	if f.MarriagePlace != nil {
		resp.MarriagePlace = f.MarriagePlace
	}

	return resp
}

// convertQueryGroupSheetToGenerated converts a query.GroupSheet to the generated FamilyGroupSheet type.
func convertQueryGroupSheetToGenerated(gs *query.GroupSheet) FamilyGroupSheet {
	resp := FamilyGroupSheet{
		Id: gs.ID,
	}

	if gs.Husband != nil {
		resp.Husband = convertQueryGroupSheetPersonToGenerated(gs.Husband)
	}
	if gs.Wife != nil {
		resp.Wife = convertQueryGroupSheetPersonToGenerated(gs.Wife)
	}
	if gs.Marriage != nil {
		resp.Marriage = convertQueryGroupSheetEventToGenerated(gs.Marriage)
	}
	if len(gs.Children) > 0 {
		children := make([]GroupSheetChild, len(gs.Children))
		for i, c := range gs.Children {
			children[i] = convertQueryGroupSheetChildToGenerated(&c)
		}
		resp.Children = &children
	}

	return resp
}

// convertQueryGroupSheetPersonToGenerated converts a query.GroupSheetPerson to the generated GroupSheetPerson type.
func convertQueryGroupSheetPersonToGenerated(p *query.GroupSheetPerson) *GroupSheetPerson {
	if p == nil {
		return nil
	}
	resp := &GroupSheetPerson{
		Id:        p.ID,
		GivenName: p.GivenName,
		Surname:   p.Surname,
	}

	if p.Gender != "" {
		g := GroupSheetPersonGender(p.Gender)
		resp.Gender = &g
	}
	if p.Birth != nil {
		resp.Birth = convertQueryGroupSheetEventToGenerated(p.Birth)
	}
	if p.Death != nil {
		resp.Death = convertQueryGroupSheetEventToGenerated(p.Death)
	}
	if p.FatherID != nil {
		id := *p.FatherID
		resp.FatherId = &id
		resp.FatherName = &p.FatherName
	}
	if p.MotherID != nil {
		id := *p.MotherID
		resp.MotherId = &id
		resp.MotherName = &p.MotherName
	}

	return resp
}

// convertQueryGroupSheetEventToGenerated converts a query.GroupSheetEvent to the generated GroupSheetEvent type.
func convertQueryGroupSheetEventToGenerated(e *query.GroupSheetEvent) *GroupSheetEvent {
	if e == nil {
		return nil
	}
	resp := &GroupSheetEvent{}
	if e.Date != "" {
		resp.Date = &e.Date
	}
	if e.Place != "" {
		resp.Place = &e.Place
	}
	return resp
}

// convertQueryGroupSheetChildToGenerated converts a query.GroupSheetChild to the generated GroupSheetChild type.
func convertQueryGroupSheetChildToGenerated(c *query.GroupSheetChild) GroupSheetChild {
	resp := GroupSheetChild{
		Id:        c.ID,
		GivenName: c.GivenName,
		Surname:   c.Surname,
	}

	if c.Gender != "" {
		g := GroupSheetChildGender(c.Gender)
		resp.Gender = &g
	}
	if c.RelationshipType != "" {
		rt := GroupSheetChildRelationshipType(c.RelationshipType)
		resp.RelationshipType = &rt
	}
	if c.Sequence != nil {
		resp.Sequence = c.Sequence
	}
	if c.Birth != nil {
		resp.Birth = convertQueryGroupSheetEventToGenerated(c.Birth)
	}
	if c.Death != nil {
		resp.Death = convertQueryGroupSheetEventToGenerated(c.Death)
	}
	if c.SpouseID != nil {
		id := *c.SpouseID
		resp.SpouseId = &id
		resp.SpouseName = &c.SpouseName
	}

	return resp
}

// convertQuerySourceToGenerated converts a query.Source to the generated Source type.
func convertQuerySourceToGenerated(s query.Source) Source {
	citationCount := s.CitationCount
	return Source{
		Id:             s.ID,
		SourceType:     s.SourceType,
		Title:          s.Title,
		Author:         s.Author,
		Publisher:      s.Publisher,
		PublishDate:    s.PublishDate,
		Url:            s.URL,
		RepositoryName: s.RepositoryName,
		CollectionName: s.CollectionName,
		CallNumber:     s.CallNumber,
		Notes:          s.Notes,
		CitationCount:  &citationCount,
		Version:        s.Version,
	}
}

// convertQuerySourceDetailToGenerated converts a query.SourceDetail to the generated SourceDetail type.
func convertQuerySourceDetailToGenerated(sd query.SourceDetail) SourceDetail {
	citationCount := sd.CitationCount
	resp := SourceDetail{
		Id:             sd.ID,
		SourceType:     sd.SourceType,
		Title:          sd.Title,
		Author:         sd.Author,
		Publisher:      sd.Publisher,
		PublishDate:    sd.PublishDate,
		Url:            sd.URL,
		RepositoryName: sd.RepositoryName,
		CollectionName: sd.CollectionName,
		CallNumber:     sd.CallNumber,
		Notes:          sd.Notes,
		CitationCount:  &citationCount,
		Version:        sd.Version,
	}

	if len(sd.Citations) > 0 {
		citations := make([]Citation, len(sd.Citations))
		for i, c := range sd.Citations {
			citations[i] = convertQueryCitationToGenerated(c)
		}
		resp.Citations = &citations
	}

	return resp
}

// convertQueryCitationToGenerated converts a query.Citation to the generated Citation type.
func convertQueryCitationToGenerated(c query.Citation) Citation {
	return Citation{
		Id:            c.ID,
		SourceId:      c.SourceID,
		SourceTitle:   c.SourceTitle,
		FactType:      c.FactType,
		FactOwnerId:   c.FactOwnerID,
		Page:          c.Page,
		Volume:        c.Volume,
		SourceQuality: c.SourceQuality,
		InformantType: c.InformantType,
		EvidenceType:  c.EvidenceType,
		QuotedText:    c.QuotedText,
		Analysis:      c.Analysis,
		TemplateId:    c.TemplateID,
		Version:       c.Version,
	}
}

// convertQueryPedigreeNodeToGenerated converts a query.PedigreeNode to the generated PedigreeNode type.
func convertQueryPedigreeNodeToGenerated(node *query.PedigreeNode) PedigreeNode {
	if node == nil {
		return PedigreeNode{}
	}
	gen := node.Generation
	resp := PedigreeNode{
		Id:         node.ID,
		Generation: &gen,
	}

	if node.GivenName != "" {
		resp.GivenName = &node.GivenName
	}
	if node.Surname != "" {
		resp.Surname = &node.Surname
	}
	if node.Gender != "" {
		resp.Gender = &node.Gender
	}
	if node.BirthDate != nil {
		resp.BirthDate = convertDomainGenDateToGenerated(node.BirthDate)
	}
	if node.DeathDate != nil {
		resp.DeathDate = convertDomainGenDateToGenerated(node.DeathDate)
	}
	if node.Father != nil {
		f := convertQueryPedigreeNodeToGenerated(node.Father)
		resp.Father = &f
	}
	if node.Mother != nil {
		m := convertQueryPedigreeNodeToGenerated(node.Mother)
		resp.Mother = &m
	}

	return resp
}

// convertQueryDescendancyNodeToGenerated converts a query.DescendancyNode to the generated DescendancyNode type.
func convertQueryDescendancyNodeToGenerated(node *query.DescendancyNode) DescendancyNode {
	if node == nil {
		return DescendancyNode{}
	}
	gen := node.Generation
	resp := DescendancyNode{
		Id:         node.ID,
		Generation: &gen,
	}

	if node.GivenName != "" {
		resp.GivenName = &node.GivenName
	}
	if node.Surname != "" {
		resp.Surname = &node.Surname
	}
	if node.Gender != "" {
		resp.Gender = &node.Gender
	}
	if node.BirthDate != nil {
		resp.BirthDate = convertDomainGenDateToGenerated(node.BirthDate)
	}
	if node.DeathDate != nil {
		resp.DeathDate = convertDomainGenDateToGenerated(node.DeathDate)
	}
	if len(node.Spouses) > 0 {
		spouses := make([]SpouseInfo, len(node.Spouses))
		for i, s := range node.Spouses {
			spouses[i] = SpouseInfo{
				Id:   s.ID,
				Name: s.Name,
			}
			if s.MarriageDate != nil {
				spouses[i].MarriageDate = convertDomainGenDateToGenerated(s.MarriageDate)
			}
		}
		resp.Spouses = &spouses
	}
	if len(node.Children) > 0 {
		children := make([]DescendancyNode, len(node.Children))
		for i, c := range node.Children {
			children[i] = convertQueryDescendancyNodeToGenerated(c)
		}
		resp.Children = &children
	}

	return resp
}

// convertHistoryResult converts a query.ChangeHistoryResult to the generated ChangeHistoryResponse type.
func convertHistoryResult(result *query.ChangeHistoryResult) ChangeHistoryResponse {
	hasMore := result.HasMore
	limitVal := result.Limit
	offsetVal := result.Offset
	resp := ChangeHistoryResponse{
		Items:   make([]ChangeEntry, len(result.Entries)),
		Total:   result.TotalCount,
		Limit:   &limitVal,
		Offset:  &offsetVal,
		HasMore: &hasMore,
	}

	for i, entry := range result.Entries {
		resp.Items[i] = convertQueryChangeEntryToGenerated(entry)
	}

	return resp
}

// convertRestorePointsResult converts a query.RestorePointsResult to the generated RestorePointsResponse type.
func convertRestorePointsResult(result *query.RestorePointsResult) RestorePointsResponse {
	resp := RestorePointsResponse{
		Items:   make([]RestorePoint, len(result.RestorePoints)),
		Total:   result.TotalCount,
		HasMore: result.HasMore,
	}

	for i, rp := range result.RestorePoints {
		resp.Items[i] = RestorePoint{
			Version:   rp.Version,
			Timestamp: rp.Timestamp,
			Action:    RestorePointAction(rp.Action),
			Summary:   rp.Summary,
		}
	}

	return resp
}

// convertRollbackResult converts a command.RollbackResult to the generated RollbackResponse type.
func convertRollbackResult(result *command.RollbackResult, message string) RollbackResponse {
	return RollbackResponse{
		EntityId:   result.EntityID,
		EntityType: RollbackResponseEntityType(result.EntityType),
		NewVersion: result.NewVersion,
		Changes:    result.Changes,
		Message:    message,
	}
}

// convertMediaReadModelToGenerated converts a repository.MediaReadModel to the generated Media type.
func convertMediaReadModelToGenerated(m repository.MediaReadModel) Media {
	hasThumbnail := len(m.ThumbnailData) > 0
	resp := Media{
		Id:           m.ID,
		EntityType:   MediaEntityType(m.EntityType),
		EntityId:     m.EntityID,
		Title:        m.Title,
		MimeType:     m.MimeType,
		Filename:     m.Filename,
		FileSize:     m.FileSize,
		HasThumbnail: &hasThumbnail,
		Version:      m.Version,
	}

	if m.Description != "" {
		resp.Description = &m.Description
	}
	if m.MediaType != "" {
		mt := MediaMediaType(m.MediaType)
		resp.MediaType = &mt
	}
	if m.CropLeft != nil {
		resp.CropLeft = m.CropLeft
	}
	if m.CropTop != nil {
		resp.CropTop = m.CropTop
	}
	if m.CropWidth != nil {
		resp.CropWidth = m.CropWidth
	}
	if m.CropHeight != nil {
		resp.CropHeight = m.CropHeight
	}
	if !m.CreatedAt.IsZero() {
		resp.CreatedAt = &m.CreatedAt
	}
	if !m.UpdatedAt.IsZero() {
		resp.UpdatedAt = &m.UpdatedAt
	}

	return resp
}
