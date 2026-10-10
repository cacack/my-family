package api

import (
	"context"
	"errors"

	"github.com/cacack/my-family/internal/query"
	"github.com/cacack/my-family/internal/repository"
)

// The merge review's two soft checks (#838): the evidence-coverage warning
// and the branch health. Neither gates a merge.

// GetBranchEvidenceCoverage implements StrictServerInterface.
func (ss *StrictServer) GetBranchEvidenceCoverage(ctx context.Context, request GetBranchEvidenceCoverageRequestObject) (GetBranchEvidenceCoverageResponseObject, error) {
	if ss.server.branchService == nil {
		return GetBranchEvidenceCoverage503JSONResponse{BranchesUnavailableJSONResponse(errBranchesUnavailable)}, nil
	}

	coverage, err := ss.server.branchService.EvidenceCoverage(ctx, request.Id)
	if err != nil {
		if errors.Is(err, repository.ErrBranchNotFound) {
			return GetBranchEvidenceCoverage404JSONResponse{NotFoundJSONResponse{
				Code:    "not_found",
				Message: "Branch not found",
			}}, nil
		}
		return nil, err
	}

	out := GetBranchEvidenceCoverage200JSONResponse{
		ChangedFactCount: coverage.ChangedFactCount,
		HasMore:          coverage.HasMore,
		Uncovered:        make([]BranchChangedFact, 0, len(coverage.Uncovered)),
	}
	for _, fact := range coverage.Uncovered {
		changed := BranchChangedFact{
			Kind:        BranchChangedFactKind(fact.Kind),
			SubjectType: BranchChangedFactSubjectType(fact.SubjectType),
			SubjectId:   fact.SubjectID,
			SubjectName: fact.SubjectName,
			ChangeCount: fact.ChangeCount,
		}
		if fact.FactType != "" {
			factType := FactType(fact.FactType)
			changed.FactType = &factType
		}
		out.Uncovered = append(out.Uncovered, changed)
	}
	return out, nil
}

// GetBranchHealth implements StrictServerInterface.
func (ss *StrictServer) GetBranchHealth(ctx context.Context, request GetBranchHealthRequestObject) (GetBranchHealthResponseObject, error) {
	if ss.server.branchHealthService == nil {
		return GetBranchHealth503JSONResponse{BranchesUnavailableJSONResponse(errBranchesUnavailable)}, nil
	}

	// Only an active branch has a view to check: the same 404 a `?branch=`
	// read of an unknown or terminal branch gets.
	branch, err := ss.resolveBranchScope(ctx, &request.Id, branchScopeRead)
	if err != nil {
		return nil, err
	}

	health, err := ss.server.branchHealthService.BranchHealth(ctx, branchScopeID(branch))
	if err != nil {
		return nil, err
	}
	return GetBranchHealth200JSONResponse(convertBranchHealth(health)), nil
}

// convertBranchHealth maps the query result onto the API type; every list is
// [] rather than null.
func convertBranchHealth(health *query.BranchHealth) BranchHealth {
	out := BranchHealth{
		ValidationIssues: make([]BranchValidationIssue, 0, len(health.ValidationIssues)),
		QualityIssues:    make([]BranchQualityIssue, 0, len(health.QualityIssues)),
		Duplicates:       make([]BranchDuplicatePair, 0, len(health.Duplicates)),
		ErrorCount:       health.ErrorCount,
		WarningCount:     health.WarningCount,
		InfoCount:        health.InfoCount,
		ResolvedCount:    health.ResolvedCount,
	}
	for _, issue := range health.ValidationIssues {
		converted := BranchValidationIssue{
			Key:             issue.Key,
			Severity:        BranchValidationIssueSeverity(issue.Severity),
			Code:            issue.Code,
			Message:         issue.Message,
			RecordId:        issue.RecordID,
			RelatedRecordId: issue.RelatedRecordID,
		}
		if issue.RecordType != "" {
			recordType := BranchValidationIssueRecordType(issue.RecordType)
			converted.RecordType = &recordType
			name := issue.RecordName
			converted.RecordName = &name
		}
		if issue.RelatedRecordType != "" {
			relatedType := BranchValidationIssueRelatedRecordType(issue.RelatedRecordType)
			converted.RelatedRecordType = &relatedType
			name := issue.RelatedRecordName
			converted.RelatedRecordName = &name
		}
		out.ValidationIssues = append(out.ValidationIssues, converted)
	}
	for _, issue := range health.QualityIssues {
		out.QualityIssues = append(out.QualityIssues, BranchQualityIssue{
			Key:        issue.Key,
			PersonId:   issue.PersonID,
			PersonName: issue.PersonName,
			Issue:      issue.Issue,
		})
	}
	for _, pair := range health.Duplicates {
		reasons := pair.MatchReasons
		if reasons == nil {
			reasons = []string{}
		}
		out.Duplicates = append(out.Duplicates, BranchDuplicatePair{
			Key:          pair.Key,
			Person1Id:    pair.Person1ID,
			Person1Name:  pair.Person1Name,
			Person2Id:    pair.Person2ID,
			Person2Name:  pair.Person2Name,
			Confidence:   float32(pair.Confidence),
			MatchReasons: reasons,
		})
	}
	return out
}
