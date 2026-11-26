package query_test

import (
	"context"
	"strings"
	"testing"

	"github.com/google/uuid"

	"github.com/cacack/my-family/internal/domain"
	"github.com/cacack/my-family/internal/repository"
)

// The validation view records each family link on both sides (HUSB/WIFE/CHIL
// and FAMS/FAMC), so gedcom-go's both-sides rule (v3.15.0+) neither reports
// the tree's links as one-way nor hides its families from the family checks.
func TestValidationService_FamilyLinksBothSides(t *testing.T) {
	service, store := setupValidationService()
	ctx := context.Background()
	addChild := func(familyID, personID uuid.UUID) {
		if err := store.SaveFamilyChild(ctx, domain.MainBranchID, &repository.FamilyChildReadModel{FamilyID: familyID, PersonID: personID}); err != nil {
			t.Fatalf("SaveFamilyChild: %v", err)
		}
	}

	// A married couple and a child, all consistent.
	husband := addPersonWithGender(store, "John", "Doe", domain.GenderMale, "1920")
	wife := addPersonWithGender(store, "Jane", "Doe", domain.GenderFemale, "1925")
	kid := addPersonWithGender(store, "Kid", "Doe", domain.GenderMale, "1950")
	consistentFamily := addFamily(store, &husband, &wife, "1945")
	addChild(consistentFamily, kid)
	consistent := map[uuid.UUID]bool{husband: true, wife: true, kid: true, consistentFamily: true}

	// A child born before its father, and a marriage before the father's birth.
	father := addPersonWithGender(store, "Early", "Smith", domain.GenderMale, "1920")
	addChild(addFamily(store, &father, nil, "1910"), addPersonWithGender(store, "Earlier", "Smith", domain.GenderFemale, "1900"))

	page, err := service.GetValidationIssues(ctx, "", 1000, 0)
	if err != nil {
		t.Fatalf("GetValidationIssues returned error: %v", err)
	}
	counts := map[string]int{}
	for _, issue := range page.Issues {
		counts[issue.Code]++
		for _, id := range []*uuid.UUID{issue.RecordID, issue.RelatedRecordID} {
			if id != nil && consistent[*id] {
				t.Errorf("issue on the consistent family: %s %s", issue.Code, issue.Message)
			}
		}
		if strings.HasPrefix(issue.Code, "UNRECIPROCATED_") {
			t.Errorf("unexpected one-way link issue: %s %s", issue.Code, issue.Message)
		}
	}
	for _, code := range []string{"CHILD_BEFORE_PARENT", "MARRIAGE_BEFORE_BIRTH"} {
		if counts[code] == 0 {
			t.Errorf("%s not reported; issues by code: %v", code, counts)
		}
	}
}
