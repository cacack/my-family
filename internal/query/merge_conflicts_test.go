package query

import (
	"context"
	"errors"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/cacack/my-family/internal/domain"
	"github.com/cacack/my-family/internal/repository"
	"github.com/cacack/my-family/internal/repository/memory"
)

// eventLog builds one side's ordered event slice for the pure-classifier tests,
// so a test reads as the sequence of things that happened on that side. The
// classifier only requires ascending position order, which appending gives.
type eventLog struct {
	t      *testing.T
	events []repository.StoredEvent
}

func newEventLog(t *testing.T) *eventLog {
	t.Helper()
	return &eventLog{t: t}
}

func (l *eventLog) add(streamID uuid.UUID, streamType string, event domain.Event) *eventLog {
	l.t.Helper()
	position := int64(len(l.events) + 1)
	stored, err := repository.EncodeEvent(streamID, streamType, event, position, position)
	require.NoError(l.t, err)
	l.events = append(l.events, stored)
	return l
}

func childLinked(familyID, personID uuid.UUID) domain.ChildLinkedToFamily {
	return domain.NewChildLinkedToFamily(&domain.FamilyChild{
		FamilyID:         familyID,
		PersonID:         personID,
		RelationshipType: domain.ChildBiological,
	})
}

// personName builds a name on a person, with the surname the only field a test
// varies unless it says otherwise.
func personName(personID, nameID uuid.UUID, given, surname string) *domain.PersonName {
	return &domain.PersonName{
		ID:        nameID,
		PersonID:  personID,
		GivenName: given,
		Surname:   surname,
		NameType:  domain.NameTypeBirth,
		IsPrimary: true,
	}
}

// --- Pure classifier: person names ------------------------------------------
//
// Name events are the shape the classifier originally missed: NameUpdated ends
// in "Updated" but carries its fields flat with no Changes map, so folding it
// as a changes-map event read nothing and two divergent renames looked like
// agreement. Renaming is the most common genealogy edit and the most likely to
// be contested, so these cases guard the review gate where it matters most.

func TestClassifyConflicts_DivergentRenameIsEditEdit(t *testing.T) {
	person, nameID := uuid.New(), uuid.New()

	branch := newEventLog(t).add(person, "Person",
		domain.NewNameUpdated(personName(person, nameID, "Ada", "Lovelace")))
	main := newEventLog(t).add(person, "Person",
		domain.NewNameUpdated(personName(person, nameID, "Ada", "King")))

	conflicts := classifyConflicts(branch.events, main.events, nil)

	require.Len(t, conflicts, 1, "a branch rename over a concurrent main rename must be reported")
	assert.Equal(t, ConflictEditEdit, conflicts[0].Kind)
	assert.Equal(t, []string{nameFieldKey(nameID)}, conflicts[0].Fields)
}

func TestClassifyConflicts_SameRenameOnBothSidesIsClean(t *testing.T) {
	person, nameID := uuid.New(), uuid.New()

	branch := newEventLog(t).add(person, "Person",
		domain.NewNameUpdated(personName(person, nameID, "Ada", "Lovelace")))
	main := newEventLog(t).add(person, "Person",
		domain.NewNameUpdated(personName(person, nameID, "Ada", "Lovelace")))

	// Identical payloads differ in event id and timestamp; those are envelope,
	// not the asserted name, so the two sides agree.
	assert.Empty(t, classifyConflicts(branch.events, main.events, nil))
}

func TestClassifyConflicts_DifferentNamesOfSamePersonAreClean(t *testing.T) {
	person := uuid.New()
	branchName, mainName := uuid.New(), uuid.New()

	branch := newEventLog(t).add(person, "Person",
		domain.NewNameUpdated(personName(person, branchName, "Ada", "Lovelace")))
	main := newEventLog(t).add(person, "Person",
		domain.NewNameUpdated(personName(person, mainName, "Augusta", "Byron")))

	// Both are edits to the same PERSON stream, so they only merge cleanly if
	// names are compared per-name rather than per-person.
	assert.Empty(t, classifyConflicts(branch.events, main.events, nil))
}

func TestClassifyConflicts_RemoveVersusRenameIsEditEdit(t *testing.T) {
	person, nameID := uuid.New(), uuid.New()

	branch := newEventLog(t).add(person, "Person", domain.NewNameRemoved(person, nameID))
	main := newEventLog(t).add(person, "Person",
		domain.NewNameUpdated(personName(person, nameID, "Ada", "King")))

	conflicts := classifyConflicts(branch.events, main.events, nil)

	require.Len(t, conflicts, 1)
	assert.Equal(t, ConflictEditEdit, conflicts[0].Kind,
		"removing a name is a field-level change to the person, not a delete of the person")
	assert.Equal(t, []string{nameFieldKey(nameID)}, conflicts[0].Fields)
}

func TestClassifyConflicts_NameAddedIsComparable(t *testing.T) {
	person, nameID := uuid.New(), uuid.New()

	branch := newEventLog(t).add(person, "Person",
		domain.NewNameAdded(personName(person, nameID, "Ada", "Lovelace")))
	main := newEventLog(t).add(person, "Person",
		domain.NewNameAdded(personName(person, nameID, "Ada", "King")))

	conflicts := classifyConflicts(branch.events, main.events, nil)

	require.Len(t, conflicts, 1, "NameAdded matches no suffix rule and needs its own fold")
	assert.Equal(t, []string{nameFieldKey(nameID)}, conflicts[0].Fields)
}

func TestConflictComparable_CoversPersonNameEvents(t *testing.T) {
	// The predicate the internal/command drift test consumes. Asserted here too
	// so a regression names the cause rather than surfacing as a missing conflict.
	for _, eventType := range []string{"NameAdded", "NameUpdated", "NameRemoved"} {
		assert.True(t, ConflictComparable(eventType), "%s must be comparable", eventType)
	}
}

// --- Pure classifier: edit vs. edit -----------------------------------------

func TestClassifyConflicts_EditEdit(t *testing.T) {
	person := uuid.New()

	branch := newEventLog(t).add(person, "person", domain.NewPersonUpdated(person, map[string]any{"surname": "Lovelace"}))
	main := newEventLog(t).add(person, "person", domain.NewPersonUpdated(person, map[string]any{"surname": "King"}))

	conflicts := classifyConflicts(branch.events, main.events, nil)

	require.Len(t, conflicts, 1)
	assert.Equal(t, person, conflicts[0].StreamID)
	assert.Equal(t, ConflictEditEdit, conflicts[0].Kind)
	assert.Equal(t, []string{"surname"}, conflicts[0].Fields)
	assert.NotEmpty(t, conflicts[0].Detail)
}

// Only the FINAL value per field matters: a branch that edits a field and then
// reverts it agrees with a main that never moved off the original.
func TestClassifyConflicts_SameFinalValueIsClean(t *testing.T) {
	person := uuid.New()

	branch := newEventLog(t).
		add(person, "person", domain.NewPersonUpdated(person, map[string]any{"surname": "Byron"})).
		add(person, "person", domain.NewPersonUpdated(person, map[string]any{"surname": "Lovelace"}))
	main := newEventLog(t).add(person, "person", domain.NewPersonUpdated(person, map[string]any{"surname": "Lovelace"}))

	assert.Empty(t, classifyConflicts(branch.events, main.events, nil))
}

func TestClassifyConflicts_DisjointFieldsAreClean(t *testing.T) {
	person := uuid.New()

	branch := newEventLog(t).add(person, "person", domain.NewPersonUpdated(person, map[string]any{"surname": "Lovelace"}))
	main := newEventLog(t).add(person, "person", domain.NewPersonUpdated(person, map[string]any{"given_name": "Augusta Ada"}))

	assert.Empty(t, classifyConflicts(branch.events, main.events, nil))
}

// Only the intersection is reported, and it comes back sorted.
func TestClassifyConflicts_EditEditReportsOnlyDivergentFieldsSorted(t *testing.T) {
	person := uuid.New()

	branch := newEventLog(t).add(person, "person", domain.NewPersonUpdated(person, map[string]any{
		"surname":     "Lovelace",
		"given_name":  "Ada",
		"birth_place": "London",
		"notes":       "branch only",
	}))
	main := newEventLog(t).add(person, "person", domain.NewPersonUpdated(person, map[string]any{
		"surname":     "King",
		"given_name":  "Ada", // agrees
		"birth_place": "Piccadilly",
		"death_place": "main only",
	}))

	conflicts := classifyConflicts(branch.events, main.events, nil)

	require.Len(t, conflicts, 1)
	assert.Equal(t, []string{"birth_place", "surname"}, conflicts[0].Fields)
}

// A stream main never touched merges cleanly no matter what the branch did.
func TestClassifyConflicts_BranchOnlyStreamIsClean(t *testing.T) {
	person := uuid.New()

	branch := newEventLog(t).add(person, "person", domain.NewPersonUpdated(person, map[string]any{"surname": "Lovelace"}))

	assert.Empty(t, classifyConflicts(branch.events, nil, nil))
}

// --- Pure classifier: delete vs. edit ---------------------------------------

func TestClassifyConflicts_DeleteEdit(t *testing.T) {
	tests := []struct {
		name          string
		branchDeletes bool
	}{
		{"branch deletes, main edits", true},
		{"main deletes, branch edits", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			person := uuid.New()
			deleteEvent := domain.NewPersonDeleted(person, "duplicate")
			editEvent := domain.NewPersonUpdated(person, map[string]any{"surname": "Lovelace"})

			branchSide, mainSide := domain.Event(deleteEvent), domain.Event(editEvent)
			if !tt.branchDeletes {
				branchSide, mainSide = editEvent, deleteEvent
			}

			branch := newEventLog(t).add(person, "person", branchSide)
			main := newEventLog(t).add(person, "person", mainSide)

			conflicts := classifyConflicts(branch.events, main.events, nil)

			require.Len(t, conflicts, 1)
			assert.Equal(t, person, conflicts[0].StreamID)
			assert.Equal(t, ConflictDeleteEdit, conflicts[0].Kind)
			assert.Empty(t, conflicts[0].Fields, "delete_edit is an aggregate-level verdict")
		})
	}
}

// Both sides deleting is agreement, not conflict — even when they disagreed on
// fields first, because the entity is gone either way.
func TestClassifyConflicts_BothDeleteIsClean(t *testing.T) {
	person := uuid.New()

	branch := newEventLog(t).
		add(person, "person", domain.NewPersonUpdated(person, map[string]any{"surname": "Lovelace"})).
		add(person, "person", domain.NewPersonDeleted(person, "duplicate"))
	main := newEventLog(t).
		add(person, "person", domain.NewPersonUpdated(person, map[string]any{"surname": "King"})).
		add(person, "person", domain.NewPersonDeleted(person, "duplicate"))

	assert.Empty(t, classifyConflicts(branch.events, main.events, nil))
}

// --- Pure classifier: structural relationships ------------------------------

func TestClassifyConflicts_LinkVersusUnlinkIsEditEdit(t *testing.T) {
	family := uuid.New()
	child := uuid.New()

	branch := newEventLog(t).add(family, "family", childLinked(family, child))
	main := newEventLog(t).add(family, "family", domain.NewChildUnlinkedFromFamily(family, child))

	conflicts := classifyConflicts(branch.events, main.events, nil)

	require.Len(t, conflicts, 1)
	assert.Equal(t, family, conflicts[0].StreamID)
	assert.Equal(t, ConflictEditEdit, conflicts[0].Kind, "structural divergence folds into edit_edit, not a fourth kind")
	assert.Equal(t, []string{"children[" + child.String() + "]"}, conflicts[0].Fields)
}

func TestClassifyConflicts_SameDirectionLinkIsClean(t *testing.T) {
	family := uuid.New()
	child := uuid.New()

	branch := newEventLog(t).add(family, "family", childLinked(family, child))
	main := newEventLog(t).add(family, "family", childLinked(family, child))

	assert.Empty(t, classifyConflicts(branch.events, main.events, nil))
}

// Divergent links on different children do not collide with each other.
func TestClassifyConflicts_DifferentChildrenAreClean(t *testing.T) {
	family := uuid.New()

	branch := newEventLog(t).add(family, "family", childLinked(family, uuid.New()))
	main := newEventLog(t).add(family, "family", domain.NewChildUnlinkedFromFamily(family, uuid.New()))

	assert.Empty(t, classifyConflicts(branch.events, main.events, nil))
}

// --- Pure classifier: create vs. create -------------------------------------

func TestClassifyConflicts_CreateCreate(t *testing.T) {
	branchStream := uuid.New()
	mainStream := uuid.New()

	branch := newEventLog(t).add(branchStream, "person", domain.NewPersonCreated(&domain.Person{
		ID: branchStream, GivenName: "Ada", Surname: "Lovelace", GedcomXref: "@I42@",
	}))
	mainTail := newEventLog(t).add(mainStream, "person", domain.NewPersonCreated(&domain.Person{
		ID: mainStream, GivenName: "Augusta", Surname: "Byron", GedcomXref: "@I42@",
	}))

	conflicts := classifyConflicts(branch.events, nil, mainTail.events)

	require.Len(t, conflicts, 1)
	assert.Equal(t, branchStream, conflicts[0].StreamID)
	assert.Equal(t, ConflictCreateCreate, conflicts[0].Kind)
	assert.Contains(t, conflicts[0].Detail, "@I42@")
}

func TestClassifyConflicts_CreateCreate_NonCollidingXrefIsClean(t *testing.T) {
	branchStream := uuid.New()
	mainStream := uuid.New()

	branch := newEventLog(t).add(branchStream, "person", domain.NewPersonCreated(&domain.Person{
		ID: branchStream, GivenName: "Ada", GedcomXref: "@I42@",
	}))
	mainTail := newEventLog(t).add(mainStream, "person", domain.NewPersonCreated(&domain.Person{
		ID: mainStream, GivenName: "Grace", GedcomXref: "@I43@",
	}))

	assert.Empty(t, classifyConflicts(branch.events, nil, mainTail.events))
}

// A create with no xref has no identity two sides could independently resolve
// to, so it never collides.
func TestClassifyConflicts_CreateWithoutXrefIsClean(t *testing.T) {
	branchStream := uuid.New()
	mainStream := uuid.New()

	branch := newEventLog(t).add(branchStream, "person", domain.NewPersonCreated(&domain.Person{
		ID: branchStream, GivenName: "Ada",
	}))
	mainTail := newEventLog(t).add(mainStream, "person", domain.NewPersonCreated(&domain.Person{
		ID: mainStream, GivenName: "Grace",
	}))

	assert.Empty(t, classifyConflicts(branch.events, nil, mainTail.events))
}

// --- Pure classifier: ordering ----------------------------------------------

func TestClassifyConflicts_OrderedByBranchFirstTouch(t *testing.T) {
	first := uuid.New()
	second := uuid.New()
	third := uuid.New()

	branch := newEventLog(t).
		add(first, "person", domain.NewPersonUpdated(first, map[string]any{"surname": "A"})).
		add(second, "person", domain.NewPersonUpdated(second, map[string]any{"surname": "B"})).
		add(third, "person", domain.NewPersonDeleted(third, "duplicate")).
		add(first, "person", domain.NewPersonUpdated(first, map[string]any{"notes": "revisited"}))

	// Main touches them in the opposite order, which must not influence the
	// reported order.
	main := newEventLog(t).
		add(third, "person", domain.NewPersonUpdated(third, map[string]any{"surname": "C"})).
		add(second, "person", domain.NewPersonUpdated(second, map[string]any{"surname": "B-main"})).
		add(first, "person", domain.NewPersonUpdated(first, map[string]any{"surname": "A-main"}))

	conflicts := classifyConflicts(branch.events, main.events, nil)

	require.Len(t, conflicts, 3)
	assert.Equal(t, []uuid.UUID{first, second, third}, []uuid.UUID{
		conflicts[0].StreamID, conflicts[1].StreamID, conflicts[2].StreamID,
	})
	assert.Equal(t, ConflictDeleteEdit, conflicts[2].Kind)
}

// --- PlanMerge --------------------------------------------------------------

func TestBranchService_PlanMerge_NotFound(t *testing.T) {
	f := newBranchTestFixture(t)

	_, err := f.service.PlanMerge(f.ctx, uuid.New())
	assert.ErrorIs(t, err, repository.ErrBranchNotFound)
}

// The replay set is the branch's mutation events in position order, with the
// lifecycle events stripped (ADR-005 §Merge).
func TestBranchService_PlanMerge_ReplaySetExcludesLifecycleEvents(t *testing.T) {
	f := newBranchTestFixture(t)

	person := uuid.New()
	f.appendMain(t, person, domain.NewPersonCreated(&domain.Person{ID: person, GivenName: "Ada", Surname: "Byron"}))

	branch := f.forkBranch(t, "Byron parentage")
	f.appendBranch(t, branch, branch.ID, "branch", domain.NewBranchCreated(branch))
	f.appendBranch(t, branch, person, "person", domain.NewPersonUpdated(person, map[string]any{"surname": "Lovelace"}))
	f.appendBranch(t, branch, person, "person", domain.NewPersonUpdated(person, map[string]any{"notes": "married 1835"}))

	plan, err := f.service.PlanMerge(f.ctx, branch.ID)
	require.NoError(t, err)

	assert.Equal(t, branch.ID, plan.Branch.ID)
	assert.False(t, plan.BranchTruncated)
	assert.False(t, plan.MainTruncated)
	assert.Empty(t, plan.Conflicts)

	require.Len(t, plan.ReplayEvents, 2)
	for i, evt := range plan.ReplayEvents {
		assert.Equal(t, "PersonUpdated", evt.EventType)
		assert.Equal(t, person, evt.StreamID)
		if i > 0 {
			assert.Greater(t, evt.Position, plan.ReplayEvents[i-1].Position, "replay must be in position order")
		}
	}
}

func TestBranchService_PlanMerge_ReportsConflictWithEntityName(t *testing.T) {
	f := newBranchTestFixture(t)

	person := uuid.New()
	f.appendMain(t, person, domain.NewPersonCreated(&domain.Person{ID: person, GivenName: "Ada", Surname: "Byron"}))
	require.NoError(t, f.readStore.SavePerson(f.ctx, domain.MainBranchID, &repository.PersonReadModel{
		ID: person, GivenName: "Ada", Surname: "Byron", FullName: "Ada Byron",
	}))

	branch := f.forkBranch(t, "Byron parentage")
	f.appendBranch(t, branch, person, "person", domain.NewPersonUpdated(person, map[string]any{"surname": "Lovelace"}))
	f.appendMain(t, person, domain.NewPersonUpdated(person, map[string]any{"surname": "King"}))

	plan, err := f.service.PlanMerge(f.ctx, branch.ID)
	require.NoError(t, err)

	require.Len(t, plan.Conflicts, 1)
	assert.Equal(t, ConflictEditEdit, plan.Conflicts[0].Kind)
	assert.Equal(t, "person", plan.Conflicts[0].EntityType)
	assert.Equal(t, "Ada Byron", plan.Conflicts[0].EntityName)
}

// An unresolvable name degrades to empty rather than leaking the UUID twice.
func TestBranchService_PlanMerge_UnresolvableNameIsEmpty(t *testing.T) {
	f := newBranchTestFixture(t)

	person := uuid.New()
	branch := f.forkBranch(t, "Ghost")
	f.appendBranch(t, branch, person, "person", domain.NewPersonUpdated(person, map[string]any{"surname": "Lovelace"}))
	f.appendMain(t, person, domain.NewPersonUpdated(person, map[string]any{"surname": "King"}))

	plan, err := f.service.PlanMerge(f.ctx, branch.ID)
	require.NoError(t, err)

	require.Len(t, plan.Conflicts, 1)
	assert.Empty(t, plan.Conflicts[0].EntityName)
}

func TestBranchService_PlanMerge_TruncatedPropagates(t *testing.T) {
	t.Run("branch side hits the cap", func(t *testing.T) {
		f := newBranchTestFixture(t)

		person := uuid.New()
		branch := f.forkBranch(t, "Prolific")

		edits := make([]domain.Event, maxComparisonEvents)
		for i := range edits {
			edits[i] = domain.NewPersonUpdated(person, map[string]any{"note": i})
		}
		f.appendBranch(t, branch, person, "person", edits...)

		plan, err := f.service.PlanMerge(f.ctx, branch.ID)
		require.NoError(t, err)

		// The BRANCH is the oversized side; main never moved.
		assert.True(t, plan.BranchTruncated)
		assert.False(t, plan.MainTruncated)
		assert.Len(t, plan.ReplayEvents, maxComparisonEvents)
	})

	t.Run("main side hits the cap", func(t *testing.T) {
		f := newBranchTestFixture(t)

		person := uuid.New()
		branch := f.forkBranch(t, "Quiet")
		f.appendBranch(t, branch, person, "person", domain.NewPersonUpdated(person, map[string]any{"surname": "Teslic"}))

		edits := make([]domain.Event, maxComparisonEvents+1)
		for i := range edits {
			edits[i] = domain.NewPersonUpdated(person, map[string]any{"note": i})
		}
		f.appendMain(t, person, edits...)

		plan, err := f.service.PlanMerge(f.ctx, branch.ID)
		require.NoError(t, err)

		// Main is the oversized side. The branch has a single event, so
		// reporting this as "branch too large" would send the user after a
		// fix that cannot exist.
		assert.True(t, plan.MainTruncated)
		assert.False(t, plan.BranchTruncated)
	})
}

func TestBranchService_PlanMerge_ReadErrors(t *testing.T) {
	readErr := errors.New("boom")

	tests := []struct {
		name  string
		store *failingEventStore
	}{
		{"branch side", &failingEventStore{readBranchErr: readErr}},
		{"main side", &failingEventStore{readStreamErr: readErr}},
		// The version pin is a merge-only read (CompareBranch never takes it), so
		// nothing else covers its failure. A plan built with the pin silently
		// missing is worse than no plan: the staleness guard would compare
		// against a version that was never observed.
		{"version pin", &failingEventStore{streamVersionErr: readErr}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			branchStore := memory.NewBranchStore()
			branch, err := domain.NewBranch("Broken", "", 0)
			require.NoError(t, err)
			require.NoError(t, branchStore.Create(context.Background(), branch))

			service := NewBranchService(branchStore, tt.store, NewHistoryService(tt.store, memory.NewReadModelStore()))

			_, err = service.PlanMerge(context.Background(), branch.ID)
			assert.ErrorIs(t, err, readErr)
		})
	}
}

// --- The create-vs-create gate ----------------------------------------------

// mainTailSpy counts the reads of main's OWN tail — the scan ADR-005's
// Implementation Notes forbid issuing unconditionally.
type mainTailSpy struct {
	repository.EventStore
	mainTailReads int
}

func (s *mainTailSpy) ReadBranch(ctx context.Context, branchID domain.BranchID, fromPosition int64, limit int) ([]repository.StoredEvent, error) {
	if branchID == domain.MainBranchID {
		s.mainTailReads++
	}
	return s.EventStore.ReadBranch(ctx, branchID, fromPosition, limit)
}

func TestBranchService_PlanMerge_MainTailScanIsGated(t *testing.T) {
	t.Run("not issued when no branch create carries an xref", func(t *testing.T) {
		f := newBranchTestFixture(t)

		person := uuid.New()
		f.appendMain(t, person, domain.NewPersonCreated(&domain.Person{ID: person, GivenName: "Ada", Surname: "Byron"}))

		branch := f.forkBranch(t, "No import")
		// A create WITHOUT an xref, plus an edit — neither may open the gate.
		created := uuid.New()
		f.appendBranch(t, branch, created, "person", domain.NewPersonCreated(&domain.Person{ID: created, GivenName: "Ada", Surname: "Lovelace"}))
		f.appendBranch(t, branch, person, "person", domain.NewPersonUpdated(person, map[string]any{"surname": "Lovelace"}))

		spy := &mainTailSpy{EventStore: f.eventStore}
		service := NewBranchService(f.branchStore, spy, NewHistoryService(spy, memory.NewReadModelStore()))

		plan, err := service.PlanMerge(f.ctx, branch.ID)
		require.NoError(t, err)

		assert.Empty(t, plan.Conflicts)
		assert.Zero(t, spy.mainTailReads, "main's full tail must not be scanned without an xref-bearing branch create")
	})

	t.Run("issued and detects a collision when a branch create carries an xref", func(t *testing.T) {
		f := newBranchTestFixture(t)

		branch := f.forkBranch(t, "Imported on a branch")

		branchPerson := uuid.New()
		f.appendBranch(t, branch, branchPerson, "person", domain.NewPersonCreated(&domain.Person{
			ID: branchPerson, GivenName: "Ada", Surname: "Lovelace", GedcomXref: "@I42@",
		}))

		mainPerson := uuid.New()
		f.appendMain(t, mainPerson, domain.NewPersonCreated(&domain.Person{
			ID: mainPerson, GivenName: "Augusta", Surname: "Byron", GedcomXref: "@I42@",
		}))

		spy := &mainTailSpy{EventStore: f.eventStore}
		service := NewBranchService(f.branchStore, spy, NewHistoryService(spy, memory.NewReadModelStore()))

		plan, err := service.PlanMerge(f.ctx, branch.ID)
		require.NoError(t, err)

		assert.Equal(t, 1, spy.mainTailReads, "the gated scan is one read, not one per created stream")
		require.Len(t, plan.Conflicts, 1)
		assert.Equal(t, ConflictCreateCreate, plan.Conflicts[0].Kind)
		assert.Equal(t, branchPerson, plan.Conflicts[0].StreamID)
	})
}

func TestBranchService_PlanMerge_MainTailReadError(t *testing.T) {
	f := newBranchTestFixture(t)

	branch := f.forkBranch(t, "Imported on a branch")
	branchPerson := uuid.New()
	f.appendBranch(t, branch, branchPerson, "person", domain.NewPersonCreated(&domain.Person{
		ID: branchPerson, GivenName: "Ada", GedcomXref: "@I42@",
	}))

	readErr := errors.New("boom")
	store := &mainTailFailingStore{EventStore: f.eventStore, err: readErr}
	service := NewBranchService(f.branchStore, store, NewHistoryService(store, memory.NewReadModelStore()))

	_, err := service.PlanMerge(f.ctx, branch.ID)
	assert.ErrorIs(t, err, readErr)
}

// mainTailFailingStore fails only the gated main-tail read, leaving the branch
// side working so the failure is attributable.
type mainTailFailingStore struct {
	repository.EventStore
	err error
}

func (s *mainTailFailingStore) ReadBranch(ctx context.Context, branchID domain.BranchID, fromPosition int64, limit int) ([]repository.StoredEvent, error) {
	if branchID == domain.MainBranchID {
		return nil, s.err
	}
	return s.EventStore.ReadBranch(ctx, branchID, fromPosition, limit)
}
