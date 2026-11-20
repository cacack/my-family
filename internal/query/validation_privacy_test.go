package query_test

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/cacack/my-family/internal/domain"
	"github.com/cacack/my-family/internal/query"
	"github.com/cacack/my-family/internal/repository"
	"github.com/cacack/my-family/internal/repository/memory"
)

// A principal who may not see some of a family's members gets no validation
// issue for that family the owner does not also get, and none naming the
// people withheld (#1518).
func TestValidation_WithheldFamilyMembers(t *testing.T) {
	ctx, main, now := context.Background(), domain.MainBranchID, time.Now().UTC()
	raw := memory.NewReadModelStore()
	must := func(err error) {
		t.Helper()
		if err != nil {
			t.Fatal(err)
		}
	}
	person := func(given string, dead bool) uuid.UUID {
		p := &repository.PersonReadModel{ID: uuid.New(), GivenName: given, Surname: "Smith", FullName: given + " Smith",
			Gender: domain.GenderFemale, DeathEvidence: dead, Version: 1, UpdatedAt: now}
		must(raw.SavePerson(ctx, main, p))
		return p.ID
	}
	family := func(p1, p2 uuid.UUID, children ...uuid.UUID) uuid.UUID {
		f := &repository.FamilyReadModel{ID: uuid.New(), Partner1ID: &p1, Partner2ID: &p2,
			RelationshipType: domain.RelationMarriage, Version: 1, UpdatedAt: now}
		must(raw.SaveFamily(ctx, main, f))
		for _, child := range children {
			must(raw.SaveFamilyChild(ctx, main, &repository.FamilyChildReadModel{FamilyID: f.ID, PersonID: child, RelationshipType: domain.ChildBiological}))
		}
		return f.ID
	}
	dead := person("Dora", true)
	living := []uuid.UUID{person("Lena", false), person("Liam", false), person("Kit", false)}
	allLiving := family(living[0], living[1], living[2])
	oneLiving := family(dead, living[0])
	families := map[uuid.UUID]string{allLiving: "every member withheld", oneLiving: "one partner withheld"}

	service := query.NewValidationService(query.NewPrivacyStore(raw))
	issuesAs := func(t *testing.T, ctx context.Context) []query.ValidationIssueResult {
		t.Helper()
		page, err := service.GetValidationIssues(ctx, "", 0, 0)
		if err != nil {
			t.Fatal(err)
		}
		return page.Issues
	}
	type recordCode struct {
		record uuid.UUID
		code   string
	}
	ownerHas := map[recordCode]bool{}
	for _, issue := range issuesAs(t, ownerCtx) {
		if issue.RecordID != nil {
			ownerHas[recordCode{*issue.RecordID, issue.Code}] = true
		}
	}

	for _, tt := range []struct {
		name string
		ctx  context.Context
	}{
		{"viewer without living", strictViewerCtx},
		{"visitor", visitorCtx},
	} {
		t.Run(tt.name, func(t *testing.T) {
			for _, issue := range issuesAs(t, tt.ctx) {
				if strings.Contains(issue.Message, "@VOID@") {
					t.Errorf("issue %s shows the null pointer: %s", issue.Code, issue.Message)
				}
				if issue.RecordID != nil {
					if label, ok := families[*issue.RecordID]; ok && !ownerHas[recordCode{*issue.RecordID, issue.Code}] {
						t.Errorf("family with %s: %s (%s) is not an issue the owner gets", label, issue.Code, issue.Message)
					}
				}
				for _, id := range living {
					if (issue.RecordID != nil && *issue.RecordID == id) || (issue.RelatedRecordID != nil && *issue.RelatedRecordID == id) {
						t.Errorf("issue %s names withheld person %s", issue.Code, id)
					}
				}
			}
		})
	}
}
