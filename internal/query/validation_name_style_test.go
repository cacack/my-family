package query_test

import (
	"context"
	"strings"
	"testing"
)

// TestValidationService_GetValidationIssues_NameStyle checks the name style
// warnings the quality page lists (#1881): each style problem yields one
// warning linked to its person, and a tidy name yields none.
func TestValidationService_GetValidationIssues_NameStyle(t *testing.T) {
	tests := []struct {
		name, given, surname string
		wantCode             string // "" means no name style warning
	}{
		{"all caps surname", "John", "SMITH", "NAME_ALL_CAPS"},
		{"double space", "John  Paul", "Smith", "NAME_SPACING"},
		{"quoted nickname", `John "Jack"`, "Smith", "NAME_QUOTES_OR_PARENS"},
		{"alternative surnames", "Mary", "Smith or Smyth", "NAME_UNCERTAIN_MARK"},
		{"tidy name", "John", "Smith", ""},
		{"initials", "J. R.", "Smith", ""},
		{"surname only", "", "Smith", ""},
		{"given name only", "John", "", ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			service, store := setupValidationService()
			personID := addPerson(store, tt.given, tt.surname, "1 JAN 1900")

			page, err := service.GetValidationIssues(context.Background(), "warning", 0, 0)
			if err != nil {
				t.Fatalf("GetValidationIssues returned error: %v", err)
			}

			var codes []string
			for _, issue := range page.Issues {
				if !strings.HasPrefix(issue.Code, "NAME_") {
					continue
				}
				codes = append(codes, issue.Code)
				if issue.Severity != "warning" {
					t.Errorf("%s severity = %q, want warning", issue.Code, issue.Severity)
				}
				if issue.RecordID == nil || *issue.RecordID != personID || issue.RecordType != "person" {
					t.Errorf("%s record = (%v, %q), want (%s, person)", issue.Code, issue.RecordID, issue.RecordType, personID)
				}
			}
			var want []string
			if tt.wantCode != "" {
				want = []string{tt.wantCode}
			}
			if strings.Join(codes, ",") != strings.Join(want, ",") {
				t.Errorf("name style codes = %v, want %v", codes, want)
			}
		})
	}
}

// TestValidationService_GetQualityReport_NameStyle checks that the quality
// report counts the same name style warnings as the issues list.
func TestValidationService_GetQualityReport_NameStyle(t *testing.T) {
	service, store := setupValidationService()
	addPerson(store, "John", "SMITH", "1 JAN 1900")
	ctx := context.Background()

	page, err := service.GetValidationIssues(ctx, "", 0, 0)
	if err != nil {
		t.Fatalf("GetValidationIssues returned error: %v", err)
	}
	report, err := service.GetQualityReport(ctx)
	if err != nil {
		t.Fatalf("GetQualityReport returned error: %v", err)
	}
	if report.WarningCount != page.WarningCount {
		t.Errorf("report warning count = %d, issues page = %d", report.WarningCount, page.WarningCount)
	}
	found := false
	for _, top := range report.TopIssues {
		if top.Code == "NAME_ALL_CAPS" && top.Count == 1 {
			found = true
		}
	}
	if !found {
		t.Errorf("top issues %v lack NAME_ALL_CAPS x1", report.TopIssues)
	}
}
