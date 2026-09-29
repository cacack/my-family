package query

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"strconv"

	"github.com/cacack/gedcom-go/v2/validator"
	"github.com/google/uuid"

	"github.com/cacack/my-family/internal/domain"
)

// Branch health (#838): the validation issues, quality issues and potential
// duplicates a research branch INTRODUCES — what the branch's view of the tree
// has that the mainline's does not. It is what a reviewer wants to see right
// before a merge: not the tree's standing problems, only the branch's.
//
// Both views are checked the same way (the /quality checks, read through the
// branch overlay for the branch, ADR-005), and each finding is given a KEY that
// is stable across the two scopes because it is built from record ids, never
// from positions or messages:
//
//   - a validation issue: its code, record and related record, and its
//     occurrence among the issues sharing those three (the validator can raise
//     one code on one record more than once), so the difference is a multiset
//     difference;
//   - a quality issue: the person and the issue text;
//   - a duplicate pair: the two person ids, in id order.
//
// Introduced = the branch's findings whose key main does not have. Resolved =
// main's findings whose key the branch no longer has; only counted, as a
// merge review lists what a merge would bring in.

// errBranchHealthOfMain refuses to compare the mainline with itself.
var errBranchHealthOfMain = errors.New("branch health needs a research branch, not the mainline")

// BranchHealthService compares a branch's quality checks with the mainline's.
type BranchHealthService struct {
	validation *ValidationService
	quality    *QualityService
}

// NewBranchHealthService creates a BranchHealthService running the checks of
// the given services.
func NewBranchHealthService(validation *ValidationService, quality *QualityService) *BranchHealthService {
	return &BranchHealthService{validation: validation, quality: quality}
}

// BranchHealth is what a branch introduces into the tree's checks.
type BranchHealth struct {
	// ValidationIssues are the validation issues the branch introduces,
	// errors first, then warnings, then information.
	ValidationIssues []BranchValidationIssue `json:"validation_issues"`
	// QualityIssues are the per-person quality issues the branch introduces.
	QualityIssues []BranchQualityIssue `json:"quality_issues"`
	// Duplicates are the potential duplicate pairs the branch introduces.
	Duplicates []BranchDuplicate `json:"duplicates"`

	// ErrorCount, WarningCount and InfoCount count ValidationIssues by
	// severity.
	ErrorCount   int `json:"error_count"`
	WarningCount int `json:"warning_count"`
	InfoCount    int `json:"info_count"`
	// ResolvedCount is how many of the mainline's findings (of all three
	// kinds) the branch no longer has.
	ResolvedCount int `json:"resolved_count"`
}

// BranchValidationIssue is a validation issue with its stable key and the
// display names of the records it is about.
type BranchValidationIssue struct {
	Key string `json:"key"`
	ValidationIssueResult
	RecordType        string `json:"record_type,omitempty"`
	RecordName        string `json:"record_name,omitempty"`
	RelatedRecordType string `json:"related_record_type,omitempty"`
	RelatedRecordName string `json:"related_record_name,omitempty"`
}

// BranchQualityIssue is one person's quality issue with its stable key.
type BranchQualityIssue struct {
	Key        string    `json:"key"`
	PersonID   uuid.UUID `json:"person_id"`
	PersonName string    `json:"person_name"`
	Issue      string    `json:"issue"`
}

// BranchDuplicate is a potential duplicate pair with its stable key.
type BranchDuplicate struct {
	Key string `json:"key"`
	DuplicateResult
}

// healthFindings is one scope's findings, by key.
type healthFindings struct {
	validation map[string]BranchValidationIssue
	quality    map[string]BranchQualityIssue
	duplicates map[string]BranchDuplicate
}

// BranchHealth returns what branchID introduces into the tree's checks,
// compared with the mainline as it is now. The caller checks that the branch
// exists and still has a view (is active).
func (s *BranchHealthService) BranchHealth(ctx context.Context, branchID domain.BranchID) (*BranchHealth, error) {
	if branchID.IsMain() {
		return nil, errBranchHealthOfMain
	}
	main, err := s.findings(ctx, domain.MainBranchID)
	if err != nil {
		return nil, fmt.Errorf("check mainline: %w", err)
	}
	branch, err := s.findings(ctx, branchID)
	if err != nil {
		return nil, fmt.Errorf("check branch: %w", err)
	}

	health := &BranchHealth{
		ValidationIssues: introduced(branch.validation, main.validation),
		QualityIssues:    introduced(branch.quality, main.quality),
		Duplicates:       introduced(branch.duplicates, main.duplicates),
		ResolvedCount: len(introduced(main.validation, branch.validation)) +
			len(introduced(main.quality, branch.quality)) +
			len(introduced(main.duplicates, branch.duplicates)),
	}
	health.orderAndCount()
	return health, nil
}

// orderAndCount puts each list in its review order — validation issues by
// severity (errors first), code, record name and key; quality issues by person
// name and key; duplicates by descending confidence and key — and counts the
// validation issues by severity.
func (health *BranchHealth) orderAndCount() {
	sort.SliceStable(health.ValidationIssues, func(i, j int) bool {
		a, b := health.ValidationIssues[i], health.ValidationIssues[j]
		if ra, rb := severityRank(a.Severity), severityRank(b.Severity); ra != rb {
			return ra < rb
		}
		if a.Code != b.Code {
			return a.Code < b.Code
		}
		if a.RecordName != b.RecordName {
			return a.RecordName < b.RecordName
		}
		return a.Key < b.Key
	})
	sort.SliceStable(health.QualityIssues, func(i, j int) bool {
		a, b := health.QualityIssues[i], health.QualityIssues[j]
		if a.PersonName != b.PersonName {
			return a.PersonName < b.PersonName
		}
		return a.Key < b.Key
	})
	sort.SliceStable(health.Duplicates, func(i, j int) bool {
		a, b := health.Duplicates[i], health.Duplicates[j]
		if a.Confidence != b.Confidence {
			return a.Confidence > b.Confidence
		}
		return a.Key < b.Key
	})
	for _, issue := range health.ValidationIssues {
		switch issue.Severity {
		case "error":
			health.ErrorCount++
		case "warning":
			health.WarningCount++
		default:
			health.InfoCount++
		}
	}
}

// findings runs every check over one scope's view and keys what it finds.
func (s *BranchHealthService) findings(ctx context.Context, scope domain.BranchID) (*healthFindings, error) {
	// Each record list is read once and shared by the validation view and the
	// quality checks.
	records, err := loadScopeRecords(ctx, s.validation.readStore, scope)
	if err != nil {
		return nil, err
	}
	view := s.validation.gedcomViewOf(records)
	people, err := s.quality.personIssuesOf(ctx, scope, records)
	if err != nil {
		return nil, err
	}

	found := &healthFindings{
		validation: make(map[string]BranchValidationIssue),
		quality:    make(map[string]BranchQualityIssue),
		duplicates: make(map[string]BranchDuplicate),
	}

	// One validation pass: it also runs duplicate detection, whose pairs come
	// back as POTENTIAL_DUPLICATE issues and are reported as duplicates.
	occurrences := make(map[string]int)
	for _, issue := range strictValidator().ValidateAll(view.doc) {
		if issue.Code == validator.CodePotentialDuplicate {
			if dup, ok := view.duplicateFromIssue(issue); ok {
				found.duplicates[dup.Key] = dup
			}
			continue
		}
		result := view.issueResult(issue)
		base := fmt.Sprintf("validation:%s:%s:%s", issue.Code, optionalID(result.RecordID), optionalID(result.RelatedRecordID))
		occurrences[base]++
		keyed := BranchValidationIssue{
			Key:                   fmt.Sprintf("%s:%d", base, occurrences[base]),
			ValidationIssueResult: result,
		}
		if result.RecordID != nil {
			label := view.labels[*result.RecordID]
			keyed.RecordType, keyed.RecordName = label.kind, label.name
		}
		if result.RelatedRecordID != nil {
			label := view.labels[*result.RelatedRecordID]
			keyed.RelatedRecordType, keyed.RelatedRecordName = label.kind, label.name
		}
		found.validation[keyed.Key] = keyed
	}

	for personID, person := range people {
		for _, issue := range person.issues {
			key := fmt.Sprintf("quality:%s:%s", personID, issue)
			found.quality[key] = BranchQualityIssue{Key: key, PersonID: personID, PersonName: person.name, Issue: issue}
		}
	}
	return found, nil
}

// duplicateFromIssue reads a duplicate pair back from its POTENTIAL_DUPLICATE
// issue (validator.DuplicatePair.ToIssue): the two records, the confidence
// and the match reasons in order.
func (v *gedcomView) duplicateFromIssue(issue validator.Issue) (BranchDuplicate, bool) {
	first, ok1 := v.xrefMap[issue.RecordXRef]
	second, ok2 := v.xrefMap[issue.RelatedXRef]
	if !ok1 || !ok2 {
		return BranchDuplicate{}, false
	}
	if second.String() < first.String() {
		first, second = second, first
	}
	confidence, err := strconv.ParseFloat(issue.Details["confidence"], 64)
	if err != nil {
		confidence = 0
	}
	reasons := []string{}
	for i := 1; ; i++ {
		reason, ok := issue.Details[fmt.Sprintf("reason_%d", i)]
		if !ok {
			break
		}
		reasons = append(reasons, reason)
	}
	return BranchDuplicate{
		Key: fmt.Sprintf("duplicate:%s:%s", first, second),
		DuplicateResult: DuplicateResult{
			Person1ID:    first,
			Person1Name:  v.labels[first].name,
			Person2ID:    second,
			Person2Name:  v.labels[second].name,
			Confidence:   confidence,
			MatchReasons: reasons,
		},
	}, true
}

// introduced returns the entries of have whose key is not in base, unordered.
func introduced[T any](have, base map[string]T) []T {
	out := make([]T, 0)
	for key, entry := range have {
		if _, ok := base[key]; !ok {
			out = append(out, entry)
		}
	}
	return out
}

func optionalID(id *uuid.UUID) string {
	if id == nil {
		return ""
	}
	return id.String()
}

// severityRank orders severities most severe first.
func severityRank(severity string) int {
	switch severity {
	case "error":
		return 0
	case "warning":
		return 1
	default:
		return 2
	}
}
