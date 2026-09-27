package query

import (
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/cacack/my-family/internal/domain"
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
	assert.Equal(t, ptr("Branchside"), conflict.FieldValues[0].BranchValue)
	assert.Equal(t, ptr("Mainside"), conflict.FieldValues[0].MainValue)
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
