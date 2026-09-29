package query

import (
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/cacack/my-family/internal/domain"
	"github.com/cacack/my-family/internal/repository"
)

// Value extraction for merge conflicts (#828), in memory. The same shapes run
// through the HTTP API on every backend in internal/integration.

func (f *branchTestFixture) person(t *testing.T, given, surname string) uuid.UUID {
	t.Helper()
	id := uuid.New()
	f.appendMain(t, id, domain.NewPersonCreated(&domain.Person{ID: id, GivenName: given, Surname: surname}))
	return id
}

// onlyConflict compares the branch and returns its single conflict.
func (f *branchTestFixture) onlyConflict(t *testing.T, branch *domain.Branch) MergeConflict {
	t.Helper()
	result, err := f.service.CompareBranch(f.ctx, branch.ID)
	require.NoError(t, err)
	require.Len(t, result.Conflicts, 1)
	return result.Conflicts[0]
}

func ptr(s string) *string { return &s }

func TestConflictValues_EditEditResolvesReferencedPeople(t *testing.T) {
	f := newBranchTestFixture(t)
	original := f.person(t, "Alex", "Original")
	branchPick := f.person(t, "Blair", "Branchpick")
	mainPick := f.person(t, "Casey", "Mainpick")
	familyID := uuid.New()
	f.appendMain(t, familyID, domain.NewFamilyCreated(&domain.Family{ID: familyID, Partner1ID: &original, MarriagePlace: "Old Chapel"}))

	branch := f.forkBranch(t, "partner")
	f.appendBranch(t, branch, familyID, "family", domain.NewFamilyUpdated(familyID, map[string]any{"partner1_id": branchPick.String()}))
	f.appendMain(t, familyID, domain.NewFamilyUpdated(familyID, map[string]any{"partner1_id": mainPick.String()}))

	conflict := f.onlyConflict(t, branch)
	assert.Equal(t, ConflictEditEdit, conflict.Kind)
	require.Len(t, conflict.FieldValues, 1)
	assert.Equal(t, MergeConflictField{
		Field:       "partner1_id",
		Label:       "Partner 1",
		BaseValue:   ptr("Alex Original"),
		BranchValue: ptr("Blair Branchpick"),
		MainValue:   ptr("Casey Mainpick"),
	}, conflict.FieldValues[0])
	assert.Equal(t, "The branch and main disagree on Partner 1", conflict.Detail)
}

func TestConflictValues_ChildLinkIsReadable(t *testing.T) {
	f := newBranchTestFixture(t)
	child := f.person(t, "Quinn", "Household")
	familyID := uuid.New()
	f.appendMain(t, familyID, domain.NewFamilyCreated(&domain.Family{ID: familyID}))

	branch := f.forkBranch(t, "child")
	link := &domain.FamilyChild{FamilyID: familyID, PersonID: child, RelationshipType: domain.ChildBiological}
	f.appendBranch(t, branch, familyID, "family", domain.NewChildLinkedToFamily(link))
	f.appendMain(t, familyID, domain.NewChildLinkedToFamily(link), domain.NewChildUnlinkedFromFamily(familyID, child))

	conflict := f.onlyConflict(t, branch)
	require.Len(t, conflict.FieldValues, 1)
	assert.Equal(t, MergeConflictField{
		Field:       childFieldKey(child),
		Label:       "Child: Quinn Household",
		BaseValue:   ptr(childUnlinkedText),
		BranchValue: ptr(childLinkedText),
		MainValue:   ptr(childUnlinkedText),
	}, conflict.FieldValues[0])
}

func TestConflictValues_NameVariant(t *testing.T) {
	f := newBranchTestFixture(t)
	personID := f.person(t, "Ada", "Byron")
	name := &domain.PersonName{ID: uuid.New(), PersonID: personID, GivenName: "Ada", Surname: "King", NameType: domain.NameTypeMarried}
	f.appendMain(t, personID, domain.NewNameAdded(name))

	branch := f.forkBranch(t, "name")
	renamed := *name
	renamed.Surname = "Lovelace"
	f.appendBranch(t, branch, personID, "person", domain.NewNameUpdated(&renamed))
	f.appendMain(t, personID, domain.NewNameRemoved(personID, name.ID))

	conflict := f.onlyConflict(t, branch)
	require.Len(t, conflict.FieldValues, 1)
	got := conflict.FieldValues[0]
	assert.Equal(t, "Name", got.Label)
	assert.Equal(t, ptr("Ada King"), got.BaseValue)
	assert.Equal(t, ptr("Ada Lovelace"), got.BranchValue)
	assert.Nil(t, got.MainValue, "main removed the name")
}

func TestConflictValues_DeleteEditValuesTheEditor(t *testing.T) {
	f := newBranchTestFixture(t)
	personID := f.person(t, "Jordan", "Doomed")

	branch := f.forkBranch(t, "delete")
	f.appendBranch(t, branch, personID, "person", domain.NewPersonDeleted(personID, "duplicate"))
	f.appendMain(t, personID, domain.NewPersonUpdated(personID, map[string]any{"surname": "Revised", "living": true}))

	conflict := f.onlyConflict(t, branch)
	assert.Equal(t, ConflictDeleteEdit, conflict.Kind)
	assert.Equal(t, resolveBranchValue, conflict.DeletedBy)
	require.Len(t, conflict.FieldValues, 2)
	assert.Equal(t, MergeConflictField{Field: "living", Label: "Living", MainValue: ptr("Yes")}, conflict.FieldValues[0])
	assert.Equal(t, MergeConflictField{
		Field: "surname", Label: "Surname", BaseValue: ptr("Doomed"), MainValue: ptr("Revised"),
	}, conflict.FieldValues[1])
}

func TestConflictValues_MainDeleterNamed(t *testing.T) {
	f := newBranchTestFixture(t)
	personID := f.person(t, "Jordan", "Doomed")

	branch := f.forkBranch(t, "delete")
	f.appendBranch(t, branch, personID, "person", domain.NewPersonUpdated(personID, map[string]any{"surname": "Revised"}))
	f.appendMain(t, personID, domain.NewPersonDeleted(personID, "duplicate"))

	conflict := f.onlyConflict(t, branch)
	assert.Equal(t, resolveMainValue, conflict.DeletedBy)
	require.Len(t, conflict.FieldValues, 1)
	assert.Equal(t, ptr("Revised"), conflict.FieldValues[0].BranchValue)
	assert.Nil(t, conflict.FieldValues[0].MainValue)
}

// When the read that rebuilds the fork state comes back capped, the base is
// reported as unknown rather than guessed.
func TestConflictValues_BaseUnknownWhenReadIsCapped(t *testing.T) {
	f := newBranchTestFixture(t)
	personID := f.person(t, "Robin", "Original")
	// A second pre-fork event, so a read capped at one event stops short of
	// the fork.
	f.appendMain(t, personID, domain.NewPersonUpdated(personID, map[string]any{"surname": "Forkside"}))
	branch := f.forkBranch(t, "capped")
	f.appendBranch(t, branch, personID, "person", domain.NewPersonUpdated(personID, map[string]any{"surname": "Branchside"}))
	f.appendMain(t, personID, domain.NewPersonUpdated(personID, map[string]any{"surname": "Mainside"}))

	saved := maxHistoryStateEvents
	maxHistoryStateEvents = 1
	t.Cleanup(func() { maxHistoryStateEvents = saved })

	conflict := f.onlyConflict(t, branch)
	require.Len(t, conflict.FieldValues, 1)
	assert.Nil(t, conflict.FieldValues[0].BaseValue)
	assert.True(t, conflict.FieldValues[0].BaseUnknown, "an unread fork is unknown, not unset")
	assert.Equal(t, ptr("Branchside"), conflict.FieldValues[0].BranchValue)
	assert.Equal(t, ptr("Mainside"), conflict.FieldValues[0].MainValue)
}

// savedCitation puts a citation in the read model, which names it
// "<source title> (<fact>)".
func (f *branchTestFixture) savedCitation(t *testing.T, sourceTitle string) uuid.UUID {
	t.Helper()
	id := uuid.New()
	require.NoError(t, f.readStore.SaveCitation(f.ctx, domain.MainBranchID, &repository.CitationReadModel{
		ID: id, SourceID: uuid.New(), SourceTitle: sourceTitle, FactType: domain.FactPersonBirth,
	}))
	return id
}

// savedFamily puts a family in the read model, which names it by its partners.
func (f *branchTestFixture) savedFamily(t *testing.T, given1, given2, surname string) uuid.UUID {
	t.Helper()
	id := uuid.New()
	require.NoError(t, f.readStore.SaveFamily(f.ctx, domain.MainBranchID, &repository.FamilyReadModel{
		ID: id, Partner1GivenName: given1, Partner1Surname: surname, Partner2GivenName: given2, Partner2Surname: surname,
	}))
	return id
}

func fieldValue(t *testing.T, conflict MergeConflict, field string) MergeConflictField {
	t.Helper()
	for _, v := range conflict.FieldValues {
		if v.Field == field {
			return v
		}
	}
	t.Fatalf("no field_values entry for %s in %+v", field, conflict.FieldValues)
	return MergeConflictField{}
}

// The GPS artifacts reference people and evidence by id; a reviewer weighing
// them sees names, never UUIDs or JSON arrays.
func TestConflictValues_EvidenceAnalysisSubjectAndCitations(t *testing.T) {
	f := newBranchTestFixture(t)
	forkSubject := f.person(t, "Fork", "Subject")
	branchSubject := f.person(t, "Branch", "Subject")
	census := f.savedCitation(t, "1850 Census")
	parish := f.savedCitation(t, "Parish register")
	analysisID := uuid.New()
	f.appendMain(t, analysisID, domain.NewEvidenceAnalysisCreated(&domain.EvidenceAnalysis{
		ID: analysisID, FactType: domain.FactPersonBirth, SubjectID: forkSubject,
		CitationIDs: []uuid.UUID{census}, Conclusion: "Born 1820",
	}))

	branch := f.forkBranch(t, "evidence")
	f.appendBranch(t, branch, analysisID, "evidence_analysis", domain.NewEvidenceAnalysisUpdated(analysisID, map[string]any{
		"subject_id":   branchSubject.String(),
		"citation_ids": []uuid.UUID{census, parish},
	}))
	f.appendMain(t, analysisID, domain.NewEvidenceAnalysisUpdated(analysisID, map[string]any{
		"subject_id":   forkSubject.String(),
		"citation_ids": []uuid.UUID{parish},
	}))

	conflict := f.onlyConflict(t, branch)
	assert.Equal(t, ConflictEditEdit, conflict.Kind)
	assert.Equal(t, MergeConflictField{
		Field:       "citation_ids",
		Label:       "Citations",
		BaseValue:   ptr("1850 Census (Birth)"),
		BranchValue: ptr("1850 Census (Birth); Parish register (Birth)"),
		MainValue:   ptr("Parish register (Birth)"),
	}, fieldValue(t, conflict, "citation_ids"))
	assert.Equal(t, MergeConflictField{
		Field:       "subject_id",
		Label:       "Subject",
		BaseValue:   ptr("Fork Subject"),
		BranchValue: ptr("Branch Subject"),
		MainValue:   ptr("Fork Subject"),
	}, fieldValue(t, conflict, "subject_id"))
	assert.Equal(t, "The branch and main disagree on Citations; Subject", conflict.Detail)
}

// Evidence analyses have no read-model name; they are named from their own
// folded streams, "<fact>: <conclusion>".
func TestConflictValues_ProofSummaryAnalysesNamedFromStreams(t *testing.T) {
	f := newBranchTestFixture(t)
	subject := f.person(t, "Proof", "Subject")
	analysis := func(conclusion string) uuid.UUID {
		id := uuid.New()
		f.appendMain(t, id, domain.NewEvidenceAnalysisCreated(&domain.EvidenceAnalysis{
			ID: id, FactType: domain.FactPersonBirth, SubjectID: subject, Conclusion: conclusion,
		}))
		return id
	}
	early := analysis("Born 1820")
	late := analysis("Born 1822")
	summaryID := uuid.New()
	f.appendMain(t, summaryID, domain.NewProofSummaryCreated(&domain.ProofSummary{
		ID: summaryID, FactType: domain.FactPersonBirth, SubjectID: subject, Conclusion: "Born 1820",
		AnalysisIDs: []uuid.UUID{early},
	}))

	branch := f.forkBranch(t, "proof")
	f.appendBranch(t, branch, summaryID, "proof_summary", domain.NewProofSummaryUpdated(summaryID, map[string]any{
		"analysis_ids": []uuid.UUID{late},
	}))
	f.appendMain(t, summaryID, domain.NewProofSummaryUpdated(summaryID, map[string]any{
		"analysis_ids": []uuid.UUID{},
	}))

	got := fieldValue(t, f.onlyConflict(t, branch), "analysis_ids")
	assert.Equal(t, "Evidence analyses", got.Label)
	assert.Equal(t, ptr("Birth: Born 1820"), got.BaseValue)
	assert.Equal(t, ptr("Birth: Born 1822"), got.BranchValue)
	assert.Nil(t, got.MainValue, "an emptied list is no value")
}

// A research log says what its subject is; the id is named as that type.
func TestConflictValues_ResearchLogSubjectFollowsSubjectType(t *testing.T) {
	f := newBranchTestFixture(t)
	person := f.person(t, "Solo", "Researched")
	family := f.savedFamily(t, "Ann", "Bert", "Household")
	logID := uuid.New()
	f.appendMain(t, logID, domain.NewResearchLogCreated(&domain.ResearchLog{
		ID: logID, SubjectID: person, SubjectType: "person", Repository: "Archive",
		SearchDescription: "Parish books", Outcome: domain.ResearchOutcomeFound, SearchDate: time.Now(),
	}))

	branch := f.forkBranch(t, "log")
	f.appendBranch(t, branch, logID, "research_log", domain.NewResearchLogUpdated(logID, map[string]any{
		"subject_id": family.String(), "subject_type": "family",
	}))
	f.appendMain(t, logID, domain.NewResearchLogUpdated(logID, map[string]any{
		"subject_id": uuid.New().String(),
	}))

	got := fieldValue(t, f.onlyConflict(t, branch), "subject_id")
	assert.Equal(t, ptr("Solo Researched"), got.BaseValue)
	assert.Equal(t, ptr("Ann Household & Bert Household"), got.BranchValue)
	// An id nothing knows still falls back to the id rather than vanishing.
	require.NotNil(t, got.MainValue)
	_, err := uuid.Parse(*got.MainValue)
	assert.NoError(t, err)
}

// A citation's fact owner is a person or a family, with nothing saying which.
func TestConflictValues_CitationFactOwner(t *testing.T) {
	f := newBranchTestFixture(t)
	owner := f.person(t, "Fact", "Owner")
	family := f.savedFamily(t, "Cara", "Dan", "Owners")
	other := f.person(t, "Other", "Owner")
	citationID := uuid.New()
	f.appendMain(t, citationID, domain.NewCitationCreated(&domain.Citation{
		ID: citationID, SourceID: uuid.New(), FactType: domain.FactPersonBirth, FactOwnerID: owner,
	}))

	branch := f.forkBranch(t, "owner")
	f.appendBranch(t, branch, citationID, "citation", domain.NewCitationUpdated(citationID, map[string]any{
		"fact_owner_id": family.String(),
	}))
	f.appendMain(t, citationID, domain.NewCitationUpdated(citationID, map[string]any{
		"fact_owner_id": other.String(),
	}))

	assert.Equal(t, MergeConflictField{
		Field:       "fact_owner_id",
		Label:       "Fact owner",
		BaseValue:   ptr("Fact Owner"),
		BranchValue: ptr("Cara Owners & Dan Owners"),
		MainValue:   ptr("Other Owner"),
	}, fieldValue(t, f.onlyConflict(t, branch), "fact_owner_id"))
}

// Notes and media are named from their streams too.
func TestConflictValues_LinkedNotesAndMedia(t *testing.T) {
	f := newBranchTestFixture(t)
	note := func(text string) uuid.UUID {
		id := uuid.New()
		f.appendMain(t, id, domain.NewNoteCreated(&domain.Note{ID: id, Text: text}))
		return id
	}
	first := note("Godmother per baptism entry")
	second := note("Witness at the wedding")
	a := f.person(t, "Ann", "Associated")
	b := f.person(t, "Bea", "Associate")
	assocID := uuid.New()
	f.appendMain(t, assocID, domain.NewAssociationCreated(&domain.Association{
		ID: assocID, PersonID: a, AssociateID: b, Role: "godparent", NoteIDs: []uuid.UUID{first},
	}))

	photo := uuid.New()
	f.appendMain(t, photo, domain.NewMediaCreated(&domain.Media{ID: photo, Title: "Portrait, 1901"}))
	scan := uuid.New()
	f.appendMain(t, scan, domain.NewMediaCreated(&domain.Media{ID: scan, Filename: "scan.jpg"}))
	submitterID := uuid.New()
	f.appendMain(t, submitterID, domain.NewSubmitterCreated(&domain.Submitter{ID: submitterID, Name: "Researcher"}))

	branch := f.forkBranch(t, "links")
	f.appendBranch(t, branch, assocID, "association", domain.NewAssociationUpdated(assocID, map[string]any{
		"note_ids": []uuid.UUID{first, second},
	}))
	f.appendMain(t, assocID, domain.NewAssociationUpdated(assocID, map[string]any{
		"note_ids": []uuid.UUID{second},
	}))
	f.appendBranch(t, branch, submitterID, "submitter", domain.NewSubmitterUpdated(submitterID, map[string]any{
		"media_id": &photo,
	}))
	f.appendMain(t, submitterID, domain.NewSubmitterUpdated(submitterID, map[string]any{
		"media_id": &scan,
	}))

	result, err := f.service.CompareBranch(f.ctx, branch.ID)
	require.NoError(t, err)
	require.Len(t, result.Conflicts, 2)
	byStream := map[uuid.UUID]MergeConflict{}
	for _, c := range result.Conflicts {
		byStream[c.StreamID] = c
	}
	assert.Equal(t, MergeConflictField{
		Field:       "note_ids",
		Label:       "Linked notes",
		BaseValue:   ptr("Godmother per baptism entry"),
		BranchValue: ptr("Godmother per baptism entry; Witness at the wedding"),
		MainValue:   ptr("Witness at the wedding"),
	}, fieldValue(t, byStream[assocID], "note_ids"))
	assert.Equal(t, MergeConflictField{
		Field:       "media_id",
		Label:       "Media",
		BranchValue: ptr("Portrait, 1901"),
		MainValue:   ptr("scan.jpg"),
	}, fieldValue(t, byStream[submitterID], "media_id"))
}

// The merge itself never needs the values, so planning does not compute them;
// the refusal path asks for them explicitly.
func TestBranchService_PlanMergeLeavesValuesToDescribe(t *testing.T) {
	f := newBranchTestFixture(t)
	personID := f.person(t, "Robin", "Original")
	branch := f.forkBranch(t, "plan")
	f.appendBranch(t, branch, personID, "person", domain.NewPersonUpdated(personID, map[string]any{"surname": "Branchside"}))
	f.appendMain(t, personID, domain.NewPersonUpdated(personID, map[string]any{"surname": "Mainside"}))

	plan, err := f.service.PlanMerge(f.ctx, branch.ID)
	require.NoError(t, err)
	require.Len(t, plan.Conflicts, 1)
	assert.Nil(t, plan.Conflicts[0].FieldValues, "planning does no display work")

	require.NoError(t, f.service.DescribeConflictValues(f.ctx, plan))
	assert.Equal(t, []MergeConflictField{{
		Field: "surname", Label: "Surname",
		BaseValue: ptr("Original"), BranchValue: ptr("Branchside"), MainValue: ptr("Mainside"),
	}}, plan.Conflicts[0].FieldValues)

	// A plan PlanMerge did not build has nothing to describe from.
	require.NoError(t, f.service.DescribeConflictValues(f.ctx, nil))
	require.NoError(t, f.service.DescribeConflictValues(f.ctx, &MergePlan{Conflicts: plan.Conflicts}))
}

func TestReferenceIDs(t *testing.T) {
	id := uuid.New()
	assert.Equal(t, []uuid.UUID{id}, referenceIDs(id.String()))
	assert.Equal(t, []uuid.UUID{id}, referenceIDs([]any{id.String(), "junk", 7, uuid.Nil.String()}))
	assert.Empty(t, referenceIDs("junk"))
	assert.Empty(t, referenceIDs(nil))
}

func TestHumanizeField(t *testing.T) {
	for field, want := range map[string]string{
		"birth_place": "Birth place",
		"partner1_id": "Partner 1",
		"surname":     "Surname",
		"_id":         "_id",
		"source_id":   "Source",
	} {
		assert.Equal(t, want, humanizeField(field), field)
	}
}

func TestFormatConflictScalar(t *testing.T) {
	assert.Nil(t, formatConflictScalar(nil))
	assert.Nil(t, formatConflictScalar("  "))
	assert.Equal(t, ptr("No"), formatConflictScalar(false))
	assert.Equal(t, ptr("3"), formatConflictScalar(float64(3)))
	assert.Equal(t, ptr("2.5"), formatConflictScalar(2.5))
	assert.Equal(t, ptr(`["a","b"]`), formatConflictScalar([]any{"a", "b"}))
	assert.Equal(t, ptr("text"), formatConflictScalar("text"))
}

func TestBracketID(t *testing.T) {
	id := uuid.New()
	got, ok := bracketID(childFieldKey(id), childFieldPrefix)
	assert.True(t, ok)
	assert.Equal(t, id, got)
	_, ok = bracketID("children[not-a-uuid]", childFieldPrefix)
	assert.False(t, ok)
	_, ok = bracketID("surname", childFieldPrefix)
	assert.False(t, ok)
}
