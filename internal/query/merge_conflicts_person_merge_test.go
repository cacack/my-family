package query

import (
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/cacack/my-family/internal/domain"
	"github.com/cacack/my-family/internal/repository"
)

// --- Pure classifier: person merges (#834) ---------------------------------
//
// A PersonMerged lands on the survivor's stream but also ends the merged
// person, whose own stream the branch never writes. These cases pin how the
// classifier compares both halves.

// personMerged builds a merge of merged into survivor resolving the given
// fields.
func personMerged(survivor, merged uuid.UUID, resolved map[string]any) domain.PersonMerged {
	return domain.NewPersonMerged(survivor, merged,
		map[string]any{"id": merged.String()}, resolved, nil, nil, nil, nil, nil)
}

func TestConflictComparable_CoversPersonMerged(t *testing.T) {
	assert.True(t, ConflictComparable("PersonMerged"),
		"a branch person merge must be compared, or main's edits to either person merge without review")
}

func TestClassifyConflicts_MainEditedMergedPersonIsDeleteEdit(t *testing.T) {
	survivor, merged := uuid.New(), uuid.New()

	branch := newEventLog(t).add(survivor, "Person", personMerged(survivor, merged, nil))
	main := newEventLog(t).add(merged, "Person",
		domain.NewPersonUpdated(merged, map[string]any{"surname": "Revised"}))

	conflicts := classifyConflicts(branch.events, main.events, nil)

	require.Len(t, conflicts, 1)
	assert.Equal(t, survivor, conflicts[0].StreamID, "the conflict is decided on the survivor's stream")
	assert.Equal(t, ConflictDeleteEdit, conflicts[0].Kind)
	assert.Equal(t, []string{"branch", "main"}, conflicts[0].SupportedResolutions)
	assert.Contains(t, conflicts[0].Detail, merged.String())
}

func TestClassifyConflicts_MainDeletedMergedPersonIsClean(t *testing.T) {
	survivor, merged := uuid.New(), uuid.New()

	branch := newEventLog(t).add(survivor, "Person", personMerged(survivor, merged, nil))
	main := newEventLog(t).
		add(merged, "Person", domain.NewPersonUpdated(merged, map[string]any{"surname": "Revised"})).
		add(merged, "Person", domain.NewPersonDeleted(merged, "duplicate"))

	assert.Empty(t, classifyConflicts(branch.events, main.events, nil),
		"both sides removed the merged person, so there is nothing to disagree about")
}

func TestClassifyConflicts_MergeResolvedFieldVersusMainEditIsEditEdit(t *testing.T) {
	survivor, merged := uuid.New(), uuid.New()

	branch := newEventLog(t).add(survivor, "Person",
		personMerged(survivor, merged, map[string]any{"birth_place": "Riverton"}))
	main := newEventLog(t).add(survivor, "Person",
		domain.NewPersonUpdated(survivor, map[string]any{"birth_place": "Lakeside"}))

	conflicts := classifyConflicts(branch.events, main.events, nil)

	require.Len(t, conflicts, 1)
	assert.Equal(t, ConflictEditEdit, conflicts[0].Kind)
	assert.Equal(t, []string{"birth_place"}, conflicts[0].Fields)
	assert.NotContains(t, conflicts[0].Detail, merged.String(), "main never changed the merged person")
}

func TestClassifyConflicts_SurvivorConflictMentionsMergedPersonEdits(t *testing.T) {
	survivor, merged := uuid.New(), uuid.New()

	branch := newEventLog(t).add(survivor, "Person",
		personMerged(survivor, merged, map[string]any{"birth_place": "Riverton"}))
	main := newEventLog(t).
		add(survivor, "Person", domain.NewPersonUpdated(survivor, map[string]any{"birth_place": "Lakeside"})).
		add(merged, "Person", domain.NewPersonUpdated(merged, map[string]any{"notes": "Seen in 1850"}))

	conflicts := classifyConflicts(branch.events, main.events, nil)

	require.Len(t, conflicts, 1, "one conflict per stream: the merged person's edit joins the survivor's")
	assert.Equal(t, ConflictEditEdit, conflicts[0].Kind)
	assert.Contains(t, conflicts[0].Detail, merged.String())
}

func TestClassifyConflicts_MainMergeIntoSameSurvivorIsCompared(t *testing.T) {
	survivor, branchMerged, mainMerged := uuid.New(), uuid.New(), uuid.New()

	branch := newEventLog(t).add(survivor, "Person",
		personMerged(survivor, branchMerged, map[string]any{"surname": "Duplicate"}))
	main := newEventLog(t).add(survivor, "Person",
		personMerged(survivor, mainMerged, map[string]any{"surname": "Duplicat"}))

	conflicts := classifyConflicts(branch.events, main.events, nil)

	require.Len(t, conflicts, 1)
	assert.Equal(t, []string{"surname"}, conflicts[0].Fields)
}

func TestClassifyConflicts_SeveralMergedPersonsEdited(t *testing.T) {
	survivor, first, second := uuid.New(), uuid.New(), uuid.New()

	branch := newEventLog(t).
		add(survivor, "Person", personMerged(survivor, first, nil)).
		add(survivor, "Person", personMerged(survivor, second, nil))
	main := newEventLog(t).
		add(first, "Person", domain.NewPersonUpdated(first, map[string]any{"notes": "a"})).
		add(second, "Person", domain.NewPersonUpdated(second, map[string]any{"notes": "b"}))

	conflicts := classifyConflicts(branch.events, main.events, nil)

	require.Len(t, conflicts, 1)
	assert.Contains(t, conflicts[0].Detail, first.String())
	assert.Contains(t, conflicts[0].Detail, second.String())
	assert.Contains(t, conflicts[0].Detail, "those persons")
}

func TestComparedStreamIDs_AddsMergedPersonsOnce(t *testing.T) {
	survivor, merged, other := uuid.New(), uuid.New(), uuid.New()

	events := newEventLog(t).
		add(merged, "Person", domain.NewPersonUpdated(merged, map[string]any{"notes": "edited first"})).
		add(survivor, "Person", personMerged(survivor, merged, nil)).
		add(survivor, "Person", personMerged(survivor, other, nil)).
		add(survivor, "Person", personMerged(survivor, other, nil)).events

	assert.Equal(t, []uuid.UUID{merged, survivor, other}, comparedStreamIDs(events),
		"a merged person the branch also wrote is not listed twice; one it did not write is appended")
	assert.Equal(t, []uuid.UUID{merged, other}, mergedPersonIDs(events))
}

func TestApplyPersonMerged_IgnoresMalformedPayload(t *testing.T) {
	side := &streamSide{fields: map[string]any{}}
	events := newEventLog(t).add(uuid.New(), "Person", personMerged(uuid.New(), uuid.New(), nil)).events
	evt := events[0]
	evt.Data = []byte("not json")

	applyPersonMerged(side, evt)
	assert.Empty(t, side.fields)
	assert.Empty(t, side.merged)
	assert.Empty(t, mergedPersonIDs([]repository.StoredEvent{evt}))
}
