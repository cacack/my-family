package query

import (
	"errors"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/cacack/my-family/internal/domain"
	"github.com/cacack/my-family/internal/repository"
)

// coverageFacts reduces a coverage result to comparable (kind, fact, subject)
// triples.
func coverageFacts(coverage *BranchEvidenceCoverage) map[string]bool {
	out := make(map[string]bool)
	for _, fact := range coverage.Uncovered {
		out[fact.Kind+"/"+string(fact.FactType)+"/"+fact.SubjectID.String()] = true
	}
	return out
}

func TestEvidenceCoverage_ClassifiesChangesAndDocumentation(t *testing.T) {
	f := newBranchTestFixture(t)
	pat, quinn, robin, gone := uuid.New(), uuid.New(), uuid.New(), uuid.New()
	family, census, occupation, divorce := uuid.New(), uuid.New(), uuid.New(), uuid.New()
	mainAnalysis, nameAnalysis := uuid.New(), uuid.New()

	// The mainline: Pat and Quinn, their family, Pat's census and occupation,
	// and an occupation analysis about Quinn.
	f.appendMain(t, pat, domain.NewPersonCreated(&domain.Person{ID: pat, GivenName: "Pat", Surname: "Sample"}))
	f.appendMain(t, quinn, domain.NewPersonCreated(&domain.Person{ID: quinn, GivenName: "Quinn", Surname: "Sample"}))
	f.projectMainPerson(t, pat, "Pat", "Sample")
	f.projectMainPerson(t, quinn, "Quinn", "Sample")
	f.appendMain(t, robin, domain.NewPersonCreated(&domain.Person{ID: robin, GivenName: "Robin", Surname: "Sample"}))
	f.projectMainPerson(t, robin, "Robin", "Sample")
	f.appendMain(t, family, domain.NewFamilyCreated(&domain.Family{ID: family, Partner1ID: &pat, Partner2ID: &quinn}))
	f.appendMain(t, census, domain.NewLifeEventCreatedFromModel(&domain.LifeEvent{ID: census, PersonID: &pat, FactType: domain.FactPersonCensus}))
	f.appendMain(t, occupation, domain.NewAttributeCreatedFromModel(&domain.Attribute{ID: occupation, PersonID: pat, FactType: domain.FactPersonOccupation, Value: "Clerk"}))
	f.appendMain(t, mainAnalysis, domain.NewEvidenceAnalysisCreated(&domain.EvidenceAnalysis{ID: mainAnalysis, FactType: domain.FactPersonOccupation, SubjectID: quinn, Conclusion: "Clerk"}))

	branch := f.forkBranch(t, "coverage")
	on := func(streamID uuid.UUID, streamType string, events ...domain.Event) {
		f.appendBranch(t, branch, streamID, streamType, events...)
	}
	// Changed, undocumented: a name variant, and the marriage.
	on(pat, "Person", domain.NewNameAdded(&domain.PersonName{ID: uuid.New(), PersonID: pat, GivenName: "Patricia", Surname: "Sample"}))
	on(family, "Family", domain.NewFamilyUpdated(family, map[string]any{"marriage_date": "1870"}))
	// Changed and documented: the census (an update, whose fact type and owner
	// come from the main fold), the occupation (deleted on the branch) and the
	// partners.
	on(census, "LifeEvent", domain.NewLifeEventUpdated(census, map[string]any{"place": "Springfield"}))
	on(occupation, "Attribute", domain.NewAttributeDeleted(occupation, "wrong person"))
	on(family, "Family", domain.NewFamilyUpdated(family, map[string]any{"partner2_id": robin.String()}))
	// Not claims: bookkeeping fields.
	on(pat, "Person", domain.NewPersonUpdated(pat, map[string]any{"notes": "check the register", "research_status": "probable"}))
	// Nothing left to document: a fact created and deleted again, and a person
	// the branch created and deleted.
	on(divorce, "LifeEvent",
		domain.NewLifeEventCreatedFromModel(&domain.LifeEvent{ID: divorce, FamilyID: &family, FactType: domain.FactFamilyDivorce}),
		domain.NewLifeEventDeleted(divorce, "mistake"))
	on(gone, "Person",
		domain.NewPersonCreated(&domain.Person{ID: gone, GivenName: "Gone", Surname: "Sample", Gender: domain.GenderFemale}),
		domain.NewPersonDeleted(gone, "mistake"))
	// The documentation written on the branch.
	on(uuid.New(), "EvidenceAnalysis", domain.NewEvidenceAnalysisCreated(&domain.EvidenceAnalysis{ID: uuid.New(), FactType: domain.FactPersonCensus, SubjectID: pat, Conclusion: "Springfield"}))
	on(mainAnalysis, "EvidenceAnalysis", domain.NewEvidenceAnalysisUpdated(mainAnalysis, map[string]any{"subject_id": pat.String()}))
	summaryID := uuid.New()
	on(summaryID, "ProofSummary", domain.NewProofSummaryCreated(&domain.ProofSummary{ID: summaryID, FactType: domain.FactFamilyDivorce, SubjectID: family, Conclusion: "c", Argument: "a"}))
	// Written and deleted again: documents nothing.
	on(nameAnalysis, "EvidenceAnalysis",
		domain.NewEvidenceAnalysisCreated(&domain.EvidenceAnalysis{ID: nameAnalysis, FactType: domain.FactPersonName, SubjectID: pat, Conclusion: "Patricia"}),
		domain.NewEvidenceAnalysisDeleted(nameAnalysis, "premature"))

	coverage, err := f.service.EvidenceCoverage(f.ctx, branch.ID)
	require.NoError(t, err)

	assert.Equal(t, 5, coverage.ChangedFactCount, "name, marriage, census, occupation, partners")
	assert.Equal(t, map[string]bool{
		"fact/person_name/" + pat.String():        true,
		"fact/family_marriage/" + family.String(): true,
	}, coverageFacts(coverage))
	assert.False(t, coverage.HasMore)
	for _, fact := range coverage.Uncovered {
		assert.NotEmpty(t, fact.SubjectName, "subject %s is named", fact.SubjectID)
		assert.Equal(t, 1, fact.ChangeCount)
	}
	// Ordered by subject name; the family is named by its partners as the
	// branch has them.
	require.Len(t, coverage.Uncovered, 2)
	assert.Equal(t, "Pat Sample", coverage.Uncovered[0].SubjectName)
	assert.Equal(t, "Pat Sample & Robin Sample", coverage.Uncovered[1].SubjectName)
}

func TestEvidenceCoverage_CreatedPeopleAndRelationships(t *testing.T) {
	f := newBranchTestFixture(t)
	parent, child, family := uuid.New(), uuid.New(), uuid.New()
	f.appendMain(t, parent, domain.NewPersonCreated(&domain.Person{ID: parent, GivenName: "Pat", Surname: "Sample"}))
	f.projectMainPerson(t, parent, "Pat", "Sample")

	branch := f.forkBranch(t, "new people")
	birth := domain.ParseGenDate("1850")
	// A new person with a name, an unknown gender (no claim) and a birth set
	// by two fields of one event: one change to one fact.
	f.appendBranch(t, branch, child, "Person", domain.NewPersonCreated(&domain.Person{
		ID: child, GivenName: "Cal", Surname: "Sample", Gender: domain.GenderUnknown, BirthDate: &birth, BirthPlace: "Springfield",
	}))
	// A new family: its partner is the relationship, its partnership type
	// alone is not; linking a child is a second change to it.
	f.appendBranch(t, branch, family, "Family",
		domain.NewFamilyCreated(&domain.Family{ID: family, Partner1ID: &parent, RelationshipType: domain.RelationMarriage}),
		domain.NewChildLinkedToFamily(&domain.FamilyChild{FamilyID: family, PersonID: child}))
	// A family life event, documented on the branch.
	event := uuid.New()
	f.appendBranch(t, branch, event, "LifeEvent", domain.NewLifeEventCreatedFromModel(&domain.LifeEvent{ID: event, FamilyID: &family, FactType: domain.FactFamilyMarriage}))
	analysis := uuid.New()
	f.appendBranch(t, branch, analysis, "EvidenceAnalysis", domain.NewEvidenceAnalysisCreated(&domain.EvidenceAnalysis{ID: analysis, FactType: domain.FactFamilyMarriage, SubjectID: family, Conclusion: "c"}))

	coverage, err := f.service.EvidenceCoverage(f.ctx, branch.ID)
	require.NoError(t, err)
	// The analysis about the family documents both its marriage and, having
	// the family as its subject, the relationship.
	assert.Equal(t, map[string]bool{
		"fact/person_name/" + child.String():  true,
		"fact/person_birth/" + child.String(): true,
	}, coverageFacts(coverage))
	assert.Equal(t, 4, coverage.ChangedFactCount)
	for _, fact := range coverage.Uncovered {
		assert.Equal(t, 1, fact.ChangeCount, "%s counted once per event", fact.FactType)
		assert.Equal(t, "Cal Sample", fact.SubjectName)
	}
}

func TestEvidenceCoverage_RelationshipUndocumented(t *testing.T) {
	f := newBranchTestFixture(t)
	family, child := uuid.New(), uuid.New()
	branch := f.forkBranch(t, "child link")
	f.appendBranch(t, branch, family, "Family",
		domain.NewChildLinkedToFamily(&domain.FamilyChild{FamilyID: family, PersonID: child}),
		domain.NewChildUnlinkedFromFamily(family, child))

	coverage, err := f.service.EvidenceCoverage(f.ctx, branch.ID)
	require.NoError(t, err)
	require.Len(t, coverage.Uncovered, 1)
	fact := coverage.Uncovered[0]
	assert.Equal(t, ChangedFactKindRelationship, fact.Kind)
	assert.Empty(t, fact.FactType)
	assert.Equal(t, entityTypeFamily, fact.SubjectType)
	assert.Equal(t, family, fact.SubjectID)
	assert.Equal(t, 2, fact.ChangeCount)
}

func TestEvidenceCoverage_EmptyAndUnknownBranch(t *testing.T) {
	f := newBranchTestFixture(t)
	branch := f.forkBranch(t, "empty")
	coverage, err := f.service.EvidenceCoverage(f.ctx, branch.ID)
	require.NoError(t, err)
	assert.Zero(t, coverage.ChangedFactCount)
	assert.NotNil(t, coverage.Uncovered)
	assert.Empty(t, coverage.Uncovered)

	_, err = f.service.EvidenceCoverage(f.ctx, uuid.New())
	assert.True(t, errors.Is(err, repository.ErrBranchNotFound), "err = %v", err)
}

func TestStatesClaim(t *testing.T) {
	assert.False(t, statesClaim("gender", nil))
	assert.False(t, statesClaim("gender", "unknown"))
	assert.True(t, statesClaim("gender", "female"))
	assert.False(t, statesClaim("birth_place", ""))
	assert.True(t, statesClaim("partner1_id", "x"))
	assert.True(t, statesClaim("sequence", float64(1)))
}

// A branch that deletes a person or family the mainline has concludes the
// person, or the partnership, never existed: that is a change to document
// (#838 review). Its earlier edits to the record fold into the deletion.
func TestEvidenceCoverage_DeletingMainlineRecords(t *testing.T) {
	f := newBranchTestFixture(t)
	pat, quinn, dee, eli := uuid.New(), uuid.New(), uuid.New(), uuid.New()
	family, kept := uuid.New(), uuid.New()
	for _, p := range []struct {
		id    uuid.UUID
		given string
	}{{pat, "Pat"}, {quinn, "Quinn"}, {dee, "Dee"}, {eli, "Eli"}} {
		f.appendMain(t, p.id, domain.NewPersonCreated(&domain.Person{ID: p.id, GivenName: p.given, Surname: "Sample"}))
		f.projectMainPerson(t, p.id, p.given, "Sample")
	}
	f.appendMain(t, family, domain.NewFamilyCreated(&domain.Family{ID: family, Partner1ID: &pat, Partner2ID: &quinn}))
	f.appendMain(t, kept, domain.NewFamilyCreated(&domain.Family{ID: kept, Partner1ID: &eli}))

	branch := f.forkBranch(t, "never married")
	on := func(streamID uuid.UUID, streamType string, events ...domain.Event) {
		f.appendBranch(t, branch, streamID, streamType, events...)
	}
	// The family is edited, then deleted: one deletion, not a marriage change.
	on(family, "Family",
		domain.NewFamilyUpdated(family, map[string]any{"marriage_date": "1870"}),
		domain.NewFamilyDeleted(family, "never married"))
	// Dee is edited, then deleted: one deletion.
	on(dee, "Person",
		domain.NewPersonUpdated(dee, map[string]any{"birth_date": "1850"}),
		domain.NewPersonDeleted(dee, "conflated record"))
	// Eli is deleted with an analysis on the branch about him: documented.
	on(eli, "Person", domain.NewPersonDeleted(eli, "never existed"))
	analysis := uuid.New()
	on(analysis, "EvidenceAnalysis", domain.NewEvidenceAnalysisCreated(&domain.EvidenceAnalysis{ID: analysis, FactType: domain.FactPersonBirth, SubjectID: eli, Conclusion: "No such person"}))

	coverage, err := f.service.EvidenceCoverage(f.ctx, branch.ID)
	require.NoError(t, err)
	assert.Equal(t, 3, coverage.ChangedFactCount, "three deletions")
	assert.Equal(t, map[string]bool{
		"deletion//" + family.String(): true,
		"deletion//" + dee.String():    true,
	}, coverageFacts(coverage))
	names := map[uuid.UUID]string{}
	for _, fact := range coverage.Uncovered {
		assert.Equal(t, 1, fact.ChangeCount)
		assert.Empty(t, fact.FactType)
		names[fact.SubjectID] = fact.SubjectName
	}
	// A deleted record is named as the mainline has it.
	assert.Equal(t, "Dee Sample", names[dee])
	assert.NotEmpty(t, names[family])
}

// A family the branch changed and then deleted keeps its deletion apart from
// its relationship change: both have no fact type.
func TestEvidenceCoverage_DeletionKeyedApartFromRelationship(t *testing.T) {
	f := newBranchTestFixture(t)
	family, child := uuid.New(), uuid.New()
	f.appendMain(t, family, domain.NewFamilyCreated(&domain.Family{ID: family}))
	branch := f.forkBranch(t, "unlink then delete")
	f.appendBranch(t, branch, family, "Family",
		domain.NewChildLinkedToFamily(&domain.FamilyChild{FamilyID: family, PersonID: child}),
		domain.NewFamilyDeleted(family, "duplicate"))

	coverage, err := f.service.EvidenceCoverage(f.ctx, branch.ID)
	require.NoError(t, err)
	require.Len(t, coverage.Uncovered, 1)
	assert.Equal(t, ChangedFactKindDeletion, coverage.Uncovered[0].Kind)
	assert.Equal(t, 1, coverage.Uncovered[0].ChangeCount)
	assert.Equal(t, 1, coverage.ChangedFactCount)
}
