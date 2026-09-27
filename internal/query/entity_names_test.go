package query

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/cacack/my-family/internal/domain"
	"github.com/cacack/my-family/internal/repository"
	"github.com/cacack/my-family/internal/repository/memory"
)

// countingReadStore wraps a real ReadModelStore and counts the entity lookups
// that name resolution can issue — single-row and batched alike — so a test can
// assert the query count stays flat as the number of entries grows (#697).
type countingReadStore struct {
	repository.ReadModelStore

	mu      sync.Mutex
	single  int // GetPerson/GetFamily/GetSource/GetCitation
	batched int // Get*ByIDs
	failAll error
}

func (c *countingReadStore) countSingle() {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.single++
}

func (c *countingReadStore) countBatch() error {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.batched++
	return c.failAll
}

func (c *countingReadStore) counts() (single, batched int) {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.single, c.batched
}

func (c *countingReadStore) reset() {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.single, c.batched = 0, 0
}

func (c *countingReadStore) GetPerson(ctx context.Context, b domain.BranchID, id uuid.UUID) (*repository.PersonReadModel, error) {
	c.countSingle()
	return c.ReadModelStore.GetPerson(ctx, b, id)
}

func (c *countingReadStore) GetFamily(ctx context.Context, b domain.BranchID, id uuid.UUID) (*repository.FamilyReadModel, error) {
	c.countSingle()
	return c.ReadModelStore.GetFamily(ctx, b, id)
}

func (c *countingReadStore) GetSource(ctx context.Context, b domain.BranchID, id uuid.UUID) (*repository.SourceReadModel, error) {
	c.countSingle()
	return c.ReadModelStore.GetSource(ctx, b, id)
}

func (c *countingReadStore) GetCitation(ctx context.Context, b domain.BranchID, id uuid.UUID) (*repository.CitationReadModel, error) {
	c.countSingle()
	return c.ReadModelStore.GetCitation(ctx, b, id)
}

func (c *countingReadStore) GetPersonsByIDs(ctx context.Context, b domain.BranchID, ids []uuid.UUID) ([]repository.PersonReadModel, error) {
	if err := c.countBatch(); err != nil {
		return nil, err
	}
	return c.ReadModelStore.GetPersonsByIDs(ctx, b, ids)
}

func (c *countingReadStore) GetFamiliesByIDs(ctx context.Context, b domain.BranchID, ids []uuid.UUID) ([]repository.FamilyReadModel, error) {
	if err := c.countBatch(); err != nil {
		return nil, err
	}
	return c.ReadModelStore.GetFamiliesByIDs(ctx, b, ids)
}

func (c *countingReadStore) GetSourcesByIDs(ctx context.Context, b domain.BranchID, ids []uuid.UUID) ([]repository.SourceReadModel, error) {
	if err := c.countBatch(); err != nil {
		return nil, err
	}
	return c.ReadModelStore.GetSourcesByIDs(ctx, b, ids)
}

func (c *countingReadStore) GetCitationsByIDs(ctx context.Context, b domain.BranchID, ids []uuid.UUID) ([]repository.CitationReadModel, error) {
	if err := c.countBatch(); err != nil {
		return nil, err
	}
	return c.ReadModelStore.GetCitationsByIDs(ctx, b, ids)
}

// namedEventSet is n entities of every named type, each with a projected
// read-model row and a stored event, plus a child link per family so the change
// summary also needs a person name.
type namedEventSet struct {
	events    []repository.StoredEvent
	wantNames map[uuid.UUID]string // entry entity id -> expected name
}

func seedNamedEvents(t *testing.T, ctx context.Context, store repository.ReadModelStore, n int) namedEventSet {
	t.Helper()
	set := namedEventSet{wantNames: make(map[uuid.UUID]string)}
	position := int64(0)
	add := func(streamID uuid.UUID, streamType string, event domain.Event) {
		position++
		stored, err := repository.EncodeEvent(streamID, streamType, event, position, position)
		require.NoError(t, err)
		set.events = append(set.events, stored)
	}

	for i := range n {
		person, family, source, citation := uuid.New(), uuid.New(), uuid.New(), uuid.New()
		given, surname := fmt.Sprintf("Given%d", i), fmt.Sprintf("Surname%d", i)
		title := fmt.Sprintf("Register %d", i)

		require.NoError(t, store.SavePerson(ctx, domain.MainBranchID, &repository.PersonReadModel{
			ID: person, GivenName: given, Surname: surname, FullName: given + " " + surname,
		}))
		require.NoError(t, store.SaveFamily(ctx, domain.MainBranchID, &repository.FamilyReadModel{
			ID: family, Partner1ID: &person, Partner1GivenName: given, Partner1Surname: surname,
		}))
		require.NoError(t, store.SaveSource(ctx, domain.MainBranchID, &repository.SourceReadModel{
			ID: source, SourceType: domain.SourceBook, Title: title,
		}))
		require.NoError(t, store.SaveCitation(ctx, domain.MainBranchID, &repository.CitationReadModel{
			ID: citation, SourceID: source, SourceTitle: title,
			FactType: domain.FactPersonBirth, FactOwnerID: person,
		}))

		add(person, "Person", domain.NewPersonUpdated(person, map[string]any{"notes": "x"}))
		add(family, "Family", domain.NewFamilyUpdated(family, map[string]any{"marriage_place": "y"}))
		add(family, "Family", childLinked(family, person))
		add(source, "Source", domain.NewSourceUpdated(source, map[string]any{"notes": "z"}))
		add(citation, "Citation", domain.NewCitationUpdated(citation, map[string]any{"page": "1"}))

		set.wantNames[person] = given + " " + surname
		set.wantNames[family] = given + " " + surname
		set.wantNames[source] = title
		set.wantNames[citation] = title + " (Birth)"
	}
	return set
}

// overlayOnBranch gives a seeded set a branch view: every person is renamed on
// the branch, and person, family, source and citation 0 are deleted there
// (tombstoned). It returns the branch and the names a branch-scoped resolution
// must produce: the branch's name where the branch has a row, and main's name
// for each entity the branch deleted — so every entity type needs the one
// fallback lookup on main.
func overlayOnBranch(t *testing.T, ctx context.Context, store repository.ReadModelStore, set namedEventSet) (domain.BranchID, map[uuid.UUID]string) {
	t.Helper()
	branch := domain.BranchID(uuid.New())
	want := make(map[uuid.UUID]string, len(set.wantNames))
	for id, name := range set.wantNames {
		want[id] = name
	}
	var first [4]uuid.UUID // person, family, source, citation of entity 0
	for i := range set.events {
		evt := set.events[i]
		if i < 5 {
			switch evt.StreamType {
			case "Person":
				first[0] = evt.StreamID
			case "Family":
				first[1] = evt.StreamID
			case "Source":
				first[2] = evt.StreamID
			case "Citation":
				first[3] = evt.StreamID
			}
		}
		if evt.StreamType != "Person" || evt.StreamID == first[0] {
			continue
		}
		surname := set.wantNames[evt.StreamID]
		name := "Branch " + surname
		require.NoError(t, store.SavePerson(ctx, branch, &repository.PersonReadModel{
			ID: evt.StreamID, GivenName: "Branch", Surname: surname, FullName: name, Version: 2,
		}))
		want[evt.StreamID] = name
	}
	require.NoError(t, store.DeleteCitation(ctx, branch, first[3]))
	require.NoError(t, store.DeleteSource(ctx, branch, first[2]))
	require.NoError(t, store.DeleteFamily(ctx, branch, first[1]))
	require.NoError(t, store.DeletePerson(ctx, branch, first[0]))
	return branch, want
}

// On a branch, names come through the overlay, and an entity the branch
// deleted falls back to main's name in ONE further batched lookup per type —
// never one per entry.
func TestTransformStoredEventsOn_BranchScopeQueryCountDoesNotScale(t *testing.T) {
	ctx := context.Background()
	for _, n := range []int{2, 60} {
		t.Run(fmt.Sprintf("%d per type", n), func(t *testing.T) {
			store := &countingReadStore{ReadModelStore: memory.NewReadModelStore()}
			set := seedNamedEvents(t, ctx, store, n)
			branch, want := overlayOnBranch(t, ctx, store, set)
			service := NewHistoryService(memory.NewEventStore(), store)
			store.reset()

			entries, err := service.transformStoredEventsOn(ctx, branch, set.events)
			require.NoError(t, err)
			require.Len(t, entries, 5*n)

			single, batched := store.counts()
			assert.Zero(t, single, "no per-entry single-row lookups")
			assert.Equal(t, 8, batched, "one branch lookup and one main fallback per entity type, independent of entry count")
			for _, entry := range entries {
				assert.Equal(t, want[entry.EntityID], entry.EntityName, "entry %s", entry.EntityID)
			}
		})
	}
}

// Without anything to fall back for, a branch scope costs exactly what main
// does.
func TestTransformStoredEventsOn_BranchWithoutTombstonesNeedsNoFallback(t *testing.T) {
	ctx := context.Background()
	store := &countingReadStore{ReadModelStore: memory.NewReadModelStore()}
	set := seedNamedEvents(t, ctx, store, 3)
	service := NewHistoryService(memory.NewEventStore(), store)
	store.reset()

	entries, err := service.transformStoredEventsOn(ctx, domain.BranchID(uuid.New()), set.events)
	require.NoError(t, err)
	_, batched := store.counts()
	assert.Equal(t, 4, batched)
	for _, entry := range entries {
		assert.Equal(t, set.wantNames[entry.EntityID], entry.EntityName)
	}
}

// A failing main fallback surfaces like any other lookup failure.
func TestTransformStoredEventsOn_FallbackErrorPropagates(t *testing.T) {
	ctx := context.Background()
	boom := errors.New("main read model unavailable")
	store := &failMainReadStore{countingReadStore: countingReadStore{ReadModelStore: memory.NewReadModelStore()}, err: boom}
	set := seedNamedEvents(t, ctx, store, 1)
	branch, _ := overlayOnBranch(t, ctx, store, set)

	_, err := NewHistoryService(memory.NewEventStore(), store).transformStoredEventsOn(ctx, branch, set.events)
	require.ErrorIs(t, err, boom)
}

// failMainReadStore fails every batched person lookup on main.
type failMainReadStore struct {
	countingReadStore
	err error
}

func (f *failMainReadStore) GetPersonsByIDs(ctx context.Context, b domain.BranchID, ids []uuid.UUID) ([]repository.PersonReadModel, error) {
	if b == domain.MainBranchID {
		return nil, f.err
	}
	return f.countingReadStore.GetPersonsByIDs(ctx, b, ids)
}

// TestTransformStoredEvents_QueryCountDoesNotScale is the #697 acceptance test
// for history/compare: naming the entries costs one batched lookup per entity
// type and no single-row lookups, whether there are 2 entities per type or 60.
func TestTransformStoredEvents_QueryCountDoesNotScale(t *testing.T) {
	ctx := context.Background()
	for _, n := range []int{2, 60} {
		t.Run(fmt.Sprintf("%d per type", n), func(t *testing.T) {
			store := &countingReadStore{ReadModelStore: memory.NewReadModelStore()}
			set := seedNamedEvents(t, ctx, store, n)
			service := NewHistoryService(memory.NewEventStore(), store)
			store.reset()

			entries, err := service.transformStoredEvents(ctx, set.events)
			require.NoError(t, err)
			require.Len(t, entries, 5*n)

			single, batched := store.counts()
			assert.Zero(t, single, "no per-entry single-row lookups")
			assert.Equal(t, 4, batched, "one batched lookup per entity type, independent of entry count")

			for _, entry := range entries {
				assert.Equal(t, set.wantNames[entry.EntityID], entry.EntityName, "entry %s", entry.EntityID)
				if change, ok := entry.Changes["children"]; ok {
					assert.Contains(t, change.NewValue, "Child linked: Given")
				}
			}
		})
	}
}

// Only the types actually present are looked up; an empty batch costs nothing.
func TestTransformStoredEvents_QueriesOnlyPresentTypes(t *testing.T) {
	ctx := context.Background()
	store := &countingReadStore{ReadModelStore: memory.NewReadModelStore()}
	service := NewHistoryService(memory.NewEventStore(), store)

	entries, err := service.transformStoredEvents(ctx, nil)
	require.NoError(t, err)
	assert.Empty(t, entries)
	_, batched := store.counts()
	assert.Zero(t, batched)

	person := uuid.New()
	stored, err := repository.EncodeEvent(person, "Person", domain.NewPersonUpdated(person, map[string]any{"notes": "x"}), 1, 1)
	require.NoError(t, err)
	_, err = service.transformStoredEvents(ctx, []repository.StoredEvent{stored})
	require.NoError(t, err)
	_, batched = store.counts()
	assert.Equal(t, 1, batched, "a person-only history reads persons only")
}

// A read-model failure surfaces instead of silently degrading every name to an id.
func TestTransformStoredEvents_LookupErrorPropagates(t *testing.T) {
	ctx := context.Background()
	boom := errors.New("read model unavailable")
	store := &countingReadStore{ReadModelStore: memory.NewReadModelStore()}
	set := seedNamedEvents(t, ctx, store, 1)
	store.failAll = boom

	_, err := NewHistoryService(memory.NewEventStore(), store).transformStoredEvents(ctx, set.events)
	require.ErrorIs(t, err, boom)
}

// TestEnrichConflictEntities_QueryCountDoesNotScale is the #697 acceptance test
// for merge review: naming N conflicts is one batched lookup per entity type.
func TestEnrichConflictEntities_QueryCountDoesNotScale(t *testing.T) {
	ctx := context.Background()
	for _, n := range []int{2, 60} {
		t.Run(fmt.Sprintf("%d per type", n), func(t *testing.T) {
			store := &countingReadStore{ReadModelStore: memory.NewReadModelStore()}
			set := seedNamedEvents(t, ctx, store, n)
			eventStore := memory.NewEventStore()
			service := NewBranchService(memory.NewBranchStore(), eventStore, NewHistoryService(eventStore, store))

			seen := make(map[uuid.UUID]bool)
			var conflicts []MergeConflict
			for _, evt := range set.events {
				if !seen[evt.StreamID] {
					seen[evt.StreamID] = true
					conflicts = append(conflicts, MergeConflict{StreamID: evt.StreamID, Kind: ConflictEditEdit})
				}
			}
			// A conflict on a stream the branch never touched stays untyped and
			// unnamed without costing a lookup.
			conflicts = append(conflicts, MergeConflict{StreamID: uuid.New(), Kind: ConflictEditEdit})
			store.reset()

			require.NoError(t, service.enrichConflictEntities(ctx, domain.MainBranchID, set.events, conflicts))

			single, batched := store.counts()
			assert.Zero(t, single, "no per-conflict single-row lookups")
			assert.Equal(t, 4, batched, "one batched lookup per entity type, independent of conflict count")

			for _, c := range conflicts[:len(conflicts)-1] {
				assert.NotEmpty(t, c.EntityType)
				assert.Equal(t, set.wantNames[c.StreamID], c.EntityName)
			}
			last := conflicts[len(conflicts)-1]
			assert.Empty(t, last.EntityType)
			assert.Empty(t, last.EntityName)
		})
	}
}

// Conflicts are named as the branch sees the entity, and an entity the branch
// deletes by main's name, in a bounded number of lookups.
func TestEnrichConflictEntities_BranchScope(t *testing.T) {
	ctx := context.Background()
	for _, n := range []int{2, 60} {
		t.Run(fmt.Sprintf("%d per type", n), func(t *testing.T) {
			store := &countingReadStore{ReadModelStore: memory.NewReadModelStore()}
			set := seedNamedEvents(t, ctx, store, n)
			branch, want := overlayOnBranch(t, ctx, store, set)
			eventStore := memory.NewEventStore()
			service := NewBranchService(memory.NewBranchStore(), eventStore, NewHistoryService(eventStore, store))

			seen := make(map[uuid.UUID]bool)
			var conflicts []MergeConflict
			for _, evt := range set.events {
				if !seen[evt.StreamID] {
					seen[evt.StreamID] = true
					conflicts = append(conflicts, MergeConflict{StreamID: evt.StreamID, Kind: ConflictEditEdit})
				}
			}
			store.reset()

			require.NoError(t, service.enrichConflictEntities(ctx, branch, set.events, conflicts))
			single, batched := store.counts()
			assert.Zero(t, single)
			assert.Equal(t, 8, batched)
			for _, c := range conflicts {
				assert.Equal(t, want[c.StreamID], c.EntityName)
			}
		})
	}
}

func TestEnrichConflictEntities_LookupErrorPropagates(t *testing.T) {
	ctx := context.Background()
	boom := errors.New("read model unavailable")
	store := &countingReadStore{ReadModelStore: memory.NewReadModelStore()}
	set := seedNamedEvents(t, ctx, store, 1)
	store.failAll = boom
	eventStore := memory.NewEventStore()
	service := NewBranchService(memory.NewBranchStore(), eventStore, NewHistoryService(eventStore, store))

	conflicts := []MergeConflict{{StreamID: set.events[0].StreamID, Kind: ConflictEditEdit}}
	require.ErrorIs(t, service.enrichConflictEntities(ctx, domain.MainBranchID, set.events, conflicts), boom)
}

// End to end: PlanMerge surfaces a naming failure rather than a plan full of
// silently unnamed conflicts.
func TestBranchService_PlanMerge_NameLookupErrorPropagates(t *testing.T) {
	f := newBranchTestFixture(t)
	boom := errors.New("read model unavailable")
	store := &countingReadStore{ReadModelStore: f.readStore, failAll: boom}
	f.service = NewBranchService(f.branchStore, f.eventStore, NewHistoryService(f.eventStore, store))

	person := uuid.New()
	f.appendMain(t, person, domain.NewPersonCreated(&domain.Person{ID: person, GivenName: "Ada", Surname: "Byron"}))
	branch := f.forkBranch(t, "Byron parentage")
	f.appendBranch(t, branch, person, "person", domain.NewPersonUpdated(person, map[string]any{"surname": "Lovelace"}))
	f.appendMain(t, person, domain.NewPersonUpdated(person, map[string]any{"surname": "King"}))

	_, err := f.service.PlanMerge(f.ctx, branch.ID)
	require.ErrorIs(t, err, boom)
}

// The family fallback names partners from the creation event; those partners
// are registered up front so a deleted family still costs no extra round.
func TestEntityNames_DeletedFamilyNamesPartnersInOneRound(t *testing.T) {
	ctx := context.Background()
	store := &countingReadStore{ReadModelStore: memory.NewReadModelStore()}
	service := NewHistoryService(memory.NewEventStore(), store)

	p1, p2 := uuid.New(), uuid.New()
	require.NoError(t, store.SavePerson(ctx, domain.MainBranchID, &repository.PersonReadModel{ID: p1, FullName: "Ada Byron"}))
	require.NoError(t, store.SavePerson(ctx, domain.MainBranchID, &repository.PersonReadModel{ID: p2, FullName: "William King"}))
	family := &domain.Family{ID: uuid.New(), Partner1ID: &p1, Partner2ID: &p2}
	stored, err := repository.EncodeEvent(family.ID, "Family", domain.NewFamilyCreated(family), 1, 1)
	require.NoError(t, err)
	store.reset()

	entries, err := service.transformStoredEvents(ctx, []repository.StoredEvent{stored})
	require.NoError(t, err)
	require.Len(t, entries, 1)
	assert.Equal(t, "Ada Byron & William King", entries[0].EntityName)
	single, batched := store.counts()
	assert.Zero(t, single)
	assert.Equal(t, 2, batched, "families + persons, one round")
}
