package query

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/cacack/my-family/internal/domain"
	"github.com/cacack/my-family/internal/repository"
	"github.com/cacack/my-family/internal/repository/memory"
)

// storedEvent builds a StoredEvent as the store would hold it.
func storedEvent(t *testing.T, streamID uuid.UUID, position int64, e domain.Event) repository.StoredEvent {
	t.Helper()
	data, err := json.Marshal(e)
	require.NoError(t, err)
	return repository.StoredEvent{ID: uuid.New(), StreamID: streamID, EventType: e.EventType(), Data: data, Position: position, Version: position}
}

// A person merge is reported on the survivor as "merged", with the merged
// person's name and every field the merge actually changed.
func TestTransform_PersonMerged(t *testing.T) {
	ctx := context.Background()
	es := memory.NewEventStore()
	survivor, merged := uuid.New(), uuid.New()
	require.NoError(t, es.Append(ctx, survivor, "Person", []domain.Event{
		domain.PersonCreated{BaseEvent: domain.NewBaseEvent(), PersonID: survivor, GivenName: "Ada", Surname: "Lovelace", BirthPlace: "London"},
		domain.NewPersonMerged(survivor, merged,
			map[string]any{"full_name": "Ada Byron", "given_name": "Ada"},
			map[string]any{"given_name": "Ada", "birth_place": "Marylebone"},
			nil, nil, nil, nil, nil),
	}, -1, repository.MainScope))

	service := NewHistoryService(es, memory.NewReadModelStore())
	history, err := service.GetEntityHistory(ctx, "person", survivor, 10, 0)
	require.NoError(t, err)
	require.Len(t, history.Entries, 2)

	merge := history.Entries[1]
	assert.Equal(t, "person", merge.EntityType)
	assert.Equal(t, "merged", merge.Action)
	assert.Equal(t, "Ada Lovelace", merge.EntityName)
	assert.Equal(t, map[string]FieldChange{
		"merged_person": {NewValue: "Ada Byron"},
		"birth_place":   {OldValue: "London", NewValue: "Marylebone"},
	}, merge.Changes, "an unchanged resolved field is not a change")
}

// When the state read hits its cap, the entries past the last event read carry
// no old value rather than a stale one.
func TestTransform_TruncatedStateReadOmitsOldValues(t *testing.T) {
	ctx := context.Background()
	es := memory.NewEventStore()
	person := uuid.New()
	require.NoError(t, es.Append(ctx, person, "Person", []domain.Event{
		domain.PersonCreated{BaseEvent: domain.NewBaseEvent(), PersonID: person, GivenName: "Ada", Surname: "Lovelace", BirthPlace: "London"},
		domain.NewPersonUpdated(person, map[string]any{"birth_place": "Marylebone"}),
		domain.NewPersonUpdated(person, map[string]any{"birth_place": "Ockham"}),
	}, -1, repository.MainScope))

	saved := maxHistoryStateEvents
	maxHistoryStateEvents = 2
	t.Cleanup(func() { maxHistoryStateEvents = saved })

	service := NewHistoryService(es, memory.NewReadModelStore())
	history, err := service.GetEntityHistory(ctx, "person", person, 10, 0)
	require.NoError(t, err)
	require.Len(t, history.Entries, 3)
	assert.Equal(t, FieldChange{OldValue: "London", NewValue: "Marylebone"}, history.Entries[1].Changes["birth_place"])
	assert.Equal(t, FieldChange{NewValue: "Ockham"}, history.Entries[2].Changes["birth_place"], "past the cap the prior value is unknown")
}

// Entities the read model does not hold are named from their own events: a
// person by their primary name variant, a citation by its source's title, a
// long note by an excerpt.
func TestTransform_NamesFromEventStreams(t *testing.T) {
	ctx := context.Background()
	service := NewHistoryService(&mockEventStore{}, &mockReadModelStore{})

	person, source, citation, note := uuid.New(), uuid.New(), uuid.New(), uuid.New()
	nameless := storedEvent(t, person, 1, domain.PersonCreated{BaseEvent: domain.NewBaseEvent(), PersonID: person})
	longText := strings.Repeat("Parish records mention the family repeatedly. ", 3)
	events := []repository.StoredEvent{
		nameless,
		storedEvent(t, person, 2, domain.NameAdded{BaseEvent: domain.NewBaseEvent(), PersonID: person, NameID: uuid.New(), GivenName: "Jane", Surname: "Doe", Nickname: "Jenny", IsPrimary: true}),
		storedEvent(t, source, 3, domain.SourceCreated{BaseEvent: domain.NewBaseEvent(), SourceID: source, Title: "Parish Register"}),
		storedEvent(t, citation, 4, domain.CitationCreated{BaseEvent: domain.NewBaseEvent(), CitationID: citation, SourceID: source, FactType: domain.FactPersonBurial}),
		storedEvent(t, note, 5, domain.NoteCreated{BaseEvent: domain.NewBaseEvent(), NoteID: note, Text: longText}),
	}

	entries, err := service.transformStoredEvents(ctx, events)
	require.NoError(t, err)
	require.Len(t, entries, 5)
	assert.Equal(t, `Jane "Jenny" Doe`, entries[0].EntityName)
	assert.Equal(t, FieldChange{NewValue: `Jane "Jenny" Doe`}, entries[1].Changes["name"])
	assert.Equal(t, "Parish Register", entries[2].EntityName)
	assert.Equal(t, "Parish Register (Burial)", entries[3].EntityName)
	require.NotNil(t, entries[3].ParentEntityID)
	assert.Equal(t, source, *entries[3].ParentEntityID)
	assert.True(t, strings.HasSuffix(entries[4].EntityName, "…"), entries[4].EntityName)
	assert.LessOrEqual(t, len([]rune(entries[4].EntityName)), noteExcerptLength+1)
}

// A name variant whose earlier state is not in the batch's streams reports
// only what can be known: the new name, or nothing for a removal.
func TestTransform_NameChangesWithoutPriorState(t *testing.T) {
	service := NewHistoryService(&mockEventStore{}, &mockReadModelStore{})
	person, nameID := uuid.New(), uuid.New()

	entries, err := service.transformStoredEvents(context.Background(), []repository.StoredEvent{
		storedEvent(t, person, 1, domain.NameUpdated{BaseEvent: domain.NewBaseEvent(), PersonID: person, NameID: nameID, GivenName: "Ann", Surname: "Lee"}),
		storedEvent(t, person, 2, domain.NameRemoved{BaseEvent: domain.NewBaseEvent(), PersonID: person, NameID: uuid.New()}),
	})
	require.NoError(t, err)
	require.Len(t, entries, 2)
	assert.Equal(t, map[string]FieldChange{"name": {NewValue: "Ann Lee"}}, entries[0].Changes)
	assert.Empty(t, entries[1].Changes)
	assert.Equal(t, "person", entries[1].EntityType)
}

func TestConflictEntityType(t *testing.T) {
	assert.Equal(t, "life_event", conflictEntityType(&repository.StoredEvent{EventType: "LifeEventUpdated", StreamType: "event"}))
	assert.Equal(t, "person", conflictEntityType(&repository.StoredEvent{EventType: "NameAdded", StreamType: "Person"}))
	assert.Equal(t, "futurething", conflictEntityType(&repository.StoredEvent{EventType: "FutureThingCreated", StreamType: "FutureThing"}))
}

func TestFormatHelpers(t *testing.T) {
	assert.Equal(t, "Dr. John \"Jack\" van Smith Jr.", formatPersonNameFields(map[string]any{
		"name_prefix": "Dr.", "given_name": "John", "nickname": "Jack", "surname_prefix": "van", "surname": "Smith", "name_suffix": "Jr.",
	}))
	assert.Empty(t, formatPersonNameFields(nil))
	assert.Equal(t, "Marriage", factLabel("family_marriage"))
	assert.Equal(t, "Short note", excerpt("  Short\n note "))
	assert.Nil(t, emptyToNil(""))
	assert.Empty(t, parenthesize(""))
	assert.Equal(t, "Ada Byron", mergedPersonName(map[string]any{"given_name": "Ada", "surname": "Byron"}))
	assert.Equal(t, "raw text", normalizeFieldValue(map[string]any{"raw": "raw text", "year": 1850}))
	assert.Empty(t, (*streamState)(nil).str("anything"))
}
