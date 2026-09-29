package query

import (
	"context"
	"regexp"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/cacack/my-family/internal/domain"
	"github.com/cacack/my-family/internal/repository"
	"github.com/cacack/my-family/internal/repository/memory"
)

// A branch that touches every branch-writable entity type (#827), and the
// mainline it forked from, written straight to the event store so each
// event type appears exactly as its command would append it.

// everyTypeFixture is the written log plus what each entry must read as.
type everyTypeFixture struct {
	branch *domain.Branch
	// branchEventTypes are the event types the branch wrote — the external
	// coverage test checks they include every branch-aware type.
	branchEventTypes map[string]bool
	mainEventCount   int
	branchCount      int

	person, associate, family, source, citation, note, media  uuid.UUID
	lifeEvent, attribute, association                         uuid.UUID
	analysis, conflict, researchLog, proofSummary, branchConf uuid.UUID
	primaryName                                               uuid.UUID
}

// BranchCompareFixtureEventTypes runs the fixture on a memory store and
// returns the event types its branch writes. Exported for the external
// coverage test (history_catalog_external_test.go), which may import the
// command package's branch-aware allowlist where this package cannot.
func BranchCompareFixtureEventTypes(t *testing.T) map[string]bool {
	t.Helper()
	f := writeEveryTypeFixture(t, context.Background(), memory.NewEventStore(), memory.NewReadModelStore(), memory.NewBranchStore())
	return f.branchEventTypes
}

func writeEveryTypeFixture(t *testing.T, ctx context.Context, es repository.EventStore, rs repository.ReadModelStore, branches repository.BranchStore) *everyTypeFixture {
	t.Helper()
	f := &everyTypeFixture{branchEventTypes: map[string]bool{}}
	projector := repository.NewProjector(rs, branches)
	versions := map[[2]uuid.UUID]int64{}
	// write appends and projects, as a command handler does, so names resolve
	// through the read model exactly as they do in production.
	write := func(streamID uuid.UUID, streamType string, scope repository.AppendScope, events []domain.Event) {
		t.Helper()
		require.NoError(t, es.Append(ctx, streamID, streamType, events, -1, scope))
		key := [2]uuid.UUID{streamID, scope.BranchID.UUID()}
		for _, e := range events {
			versions[key]++
			require.NoError(t, projector.Project(ctx, e, versions[key], scope.BranchID), "project %s", e.EventType())
		}
	}
	for _, id := range []*uuid.UUID{
		&f.person, &f.associate, &f.family, &f.source, &f.citation, &f.note, &f.media,
		&f.lifeEvent, &f.attribute, &f.association, &f.analysis, &f.conflict,
		&f.researchLog, &f.proofSummary, &f.branchConf, &f.primaryName,
	} {
		*id = uuid.New()
	}
	base := domain.NewBaseEvent
	date := func(s string) *domain.GenDate { d := domain.ParseGenDate(s); return &d }

	main := func(streamID uuid.UUID, streamType string, events ...domain.Event) {
		t.Helper()
		write(streamID, streamType, repository.MainScope, events)
		f.mainEventCount += len(events)
	}

	// --- The mainline: one of everything -----------------------------------
	main(f.person, "Person",
		domain.PersonCreated{BaseEvent: base(), PersonID: f.person, GivenName: "Ada", Surname: "Lovelace", BirthPlace: "London"},
		domain.NameAdded{BaseEvent: base(), PersonID: f.person, NameID: f.primaryName, GivenName: "Ada", Surname: "Lovelace", IsPrimary: true})
	main(f.associate, "Person", domain.PersonCreated{BaseEvent: base(), PersonID: f.associate, GivenName: "Charles", Surname: "Babbage"})
	main(f.family, "Family", domain.FamilyCreated{BaseEvent: base(), FamilyID: f.family, Partner1ID: &f.person, Partner2ID: &f.associate, MarriagePlace: "London"})
	main(f.source, "Source", domain.SourceCreated{BaseEvent: base(), SourceID: f.source, SourceType: domain.SourceArchive, Title: "1850 Census"})
	main(f.citation, "Citation", domain.CitationCreated{BaseEvent: base(), CitationID: f.citation, SourceID: f.source, FactType: domain.FactPersonBirth, FactOwnerID: f.person, Page: "p. 12"})
	main(f.note, "Note", domain.NoteCreated{BaseEvent: base(), NoteID: f.note, Text: "Ada wrote the first published algorithm."})
	main(f.media, "Media", domain.MediaCreated{BaseEvent: base(), MediaID: f.media, EntityType: "person", EntityID: f.person, Title: "Portrait", Filename: "ada.jpg", MimeType: "image/jpeg", FileData: []byte{1, 2, 3}})
	main(f.lifeEvent, "LifeEvent", domain.LifeEventCreated{BaseEvent: base(), EventID: f.lifeEvent, PersonID: &f.person, FactType: domain.FactPersonBirth, Date: date("10 DEC 1815"), Place: "London"})
	main(f.attribute, "Attribute", domain.AttributeCreated{BaseEvent: base(), AttributeID: f.attribute, PersonID: f.person, FactType: domain.FactPersonOccupation, Value: "Mathematician"})
	main(f.association, "Association", domain.AssociationCreated{BaseEvent: base(), AssociationID: f.association, PersonID: f.person, AssociateID: f.associate, Role: "colleague"})
	main(f.analysis, "EvidenceAnalysis", domain.EvidenceAnalysisCreated{BaseEvent: base(), AnalysisID: f.analysis, FactType: domain.FactPersonBirth, SubjectID: f.person, Conclusion: "Born 10 Dec 1815 in London"})
	main(f.conflict, "EvidenceConflict", domain.EvidenceConflictDetected{BaseEvent: base(), ConflictID: f.conflict, FactType: domain.FactPersonBirth, SubjectID: f.person, Description: "Two birth dates disagree", Status: domain.ConflictStatusOpen})
	main(f.researchLog, "ResearchLog", domain.ResearchLogCreated{BaseEvent: base(), LogID: f.researchLog, SubjectID: f.person, SubjectType: "person", Repository: "National Archives", SearchDescription: "Searched baptism registers", Outcome: domain.ResearchOutcomeInconclusive, SearchDate: time.Now()})
	main(f.proofSummary, "ProofSummary", domain.ProofSummaryCreated{BaseEvent: base(), SummaryID: f.proofSummary, FactType: domain.FactPersonBirth, SubjectID: f.person, Conclusion: "Born 1815", Argument: "Census and baptism agree"})

	// --- The branch: change every one of them -----------------------------
	events, err := es.ReadAll(ctx, 0, 1000)
	require.NoError(t, err)
	branch, err := domain.NewBranch("Every type", "", events[len(events)-1].Position)
	require.NoError(t, err)
	require.NoError(t, branches.Create(ctx, branch))
	f.branch = branch
	scope := repository.AppendScope{BranchID: domain.BranchID(branch.ID)}
	onBranch := func(streamID uuid.UUID, streamType string, events ...domain.Event) {
		t.Helper()
		write(streamID, streamType, scope, events)
		for _, e := range events {
			f.branchEventTypes[e.EventType()] = true
		}
		f.branchCount += len(events)
	}
	// createDelete writes a branch-only entity and deletes it again.
	createDelete := func(streamType string, create func(uuid.UUID) domain.Event, del func(uuid.UUID) domain.Event) {
		id := uuid.New()
		onBranch(id, streamType, create(id), del(id))
	}

	secondName := uuid.New()
	onBranch(f.person, "Person",
		domain.PersonUpdated{BaseEvent: base(), PersonID: f.person, Changes: map[string]any{"birth_place": "Marylebone"}},
		domain.NameUpdated{BaseEvent: base(), PersonID: f.person, NameID: f.primaryName, GivenName: "Augusta Ada", Surname: "Lovelace", IsPrimary: true},
		domain.NameAdded{BaseEvent: base(), PersonID: f.person, NameID: secondName, GivenName: "Ada", Surname: "Byron", NameType: domain.NameTypeBirth},
		domain.NameRemoved{BaseEvent: base(), PersonID: f.person, NameID: secondName})
	createDelete("Person",
		func(id uuid.UUID) domain.Event {
			return domain.PersonCreated{BaseEvent: base(), PersonID: id, GivenName: "Allegra", Surname: "Byron"}
		},
		func(id uuid.UUID) domain.Event { return domain.PersonDeleted{BaseEvent: base(), PersonID: id} })
	onBranch(f.family, "Family",
		domain.FamilyUpdated{BaseEvent: base(), FamilyID: f.family, Changes: map[string]any{"marriage_place": "Ockham"}},
		domain.ChildLinkedToFamily{BaseEvent: base(), FamilyID: f.family, PersonID: f.associate},
		domain.ChildUnlinkedFromFamily{BaseEvent: base(), FamilyID: f.family, PersonID: f.associate})
	createDelete("Family",
		func(id uuid.UUID) domain.Event {
			return domain.FamilyCreated{BaseEvent: base(), FamilyID: id, Partner1ID: &f.associate}
		},
		func(id uuid.UUID) domain.Event { return domain.FamilyDeleted{BaseEvent: base(), FamilyID: id} })
	onBranch(f.source, "Source", domain.SourceUpdated{BaseEvent: base(), SourceID: f.source, Changes: map[string]any{"title": "1851 Census"}})
	createDelete("Source",
		func(id uuid.UUID) domain.Event {
			return domain.SourceCreated{BaseEvent: base(), SourceID: id, SourceType: domain.SourceArchive, Title: "Parish Register"}
		},
		func(id uuid.UUID) domain.Event { return domain.SourceDeleted{BaseEvent: base(), SourceID: id} })
	onBranch(f.citation, "Citation", domain.CitationUpdated{BaseEvent: base(), CitationID: f.citation, Changes: map[string]any{"page": "p. 13"}})
	createDelete("Citation",
		func(id uuid.UUID) domain.Event {
			return domain.CitationCreated{BaseEvent: base(), CitationID: id, SourceID: f.source, FactType: domain.FactPersonDeath, FactOwnerID: f.person}
		},
		func(id uuid.UUID) domain.Event { return domain.CitationDeleted{BaseEvent: base(), CitationID: id} })
	onBranch(f.note, "Note", domain.NoteUpdated{BaseEvent: base(), NoteID: f.note, Changes: map[string]any{"text": "Ada wrote the first algorithm for a machine."}})
	createDelete("Note",
		func(id uuid.UUID) domain.Event {
			return domain.NoteCreated{BaseEvent: base(), NoteID: id, Text: "A passing thought"}
		},
		func(id uuid.UUID) domain.Event { return domain.NoteDeleted{BaseEvent: base(), NoteID: id} })
	onBranch(f.media, "Media", domain.MediaUpdated{BaseEvent: base(), MediaID: f.media, Changes: map[string]any{"title": "Portrait (1840)"}})
	createDelete("Media",
		func(id uuid.UUID) domain.Event {
			return domain.MediaCreated{BaseEvent: base(), MediaID: id, EntityType: "family", EntityID: f.family, Title: "Wedding sketch", Filename: "w.png", MimeType: "image/png"}
		},
		func(id uuid.UUID) domain.Event { return domain.MediaDeleted{BaseEvent: base(), MediaID: id} })
	onBranch(f.lifeEvent, "LifeEvent", domain.LifeEventUpdated{BaseEvent: base(), EventID: f.lifeEvent, Changes: map[string]any{"place": "Marylebone"}})
	createDelete("LifeEvent",
		func(id uuid.UUID) domain.Event {
			return domain.LifeEventCreated{BaseEvent: base(), EventID: id, FamilyID: &f.family, FactType: domain.FactFamilyMarriage, Date: date("8 JUL 1835")}
		},
		func(id uuid.UUID) domain.Event { return domain.LifeEventDeleted{BaseEvent: base(), EventID: id} })
	onBranch(f.attribute, "Attribute", domain.AttributeUpdated{BaseEvent: base(), AttributeID: f.attribute, Changes: map[string]any{"value": "Analyst"}})
	createDelete("Attribute",
		func(id uuid.UUID) domain.Event {
			return domain.AttributeCreated{BaseEvent: base(), AttributeID: id, PersonID: f.person, FactType: domain.FactPersonOccupation, Value: "Writer"}
		},
		func(id uuid.UUID) domain.Event { return domain.AttributeDeleted{BaseEvent: base(), AttributeID: id} })
	onBranch(f.association, "Association", domain.AssociationUpdated{BaseEvent: base(), AssociationID: f.association, Changes: map[string]any{"role": "friend"}})
	createDelete("Association",
		func(id uuid.UUID) domain.Event {
			return domain.AssociationCreated{BaseEvent: base(), AssociationID: id, PersonID: f.associate, AssociateID: f.person, Role: "mentor"}
		},
		func(id uuid.UUID) domain.Event {
			return domain.AssociationDeleted{BaseEvent: base(), AssociationID: id}
		})
	onBranch(f.analysis, "EvidenceAnalysis", domain.EvidenceAnalysisUpdated{BaseEvent: base(), AnalysisID: f.analysis, Changes: map[string]any{"conclusion": "Born 10 Dec 1815 in Marylebone"}})
	createDelete("EvidenceAnalysis",
		func(id uuid.UUID) domain.Event {
			return domain.EvidenceAnalysisCreated{BaseEvent: base(), AnalysisID: id, FactType: domain.FactPersonDeath, SubjectID: f.person, Conclusion: "Died 1852"}
		},
		func(id uuid.UUID) domain.Event {
			return domain.EvidenceAnalysisDeleted{BaseEvent: base(), AnalysisID: id}
		})
	onBranch(f.conflict, "EvidenceConflict", domain.EvidenceConflictResolved{BaseEvent: base(), ConflictID: f.conflict, Resolution: "The baptism register wins", Status: domain.ConflictStatusResolved})
	onBranch(f.branchConf, "EvidenceConflict", domain.EvidenceConflictDetected{BaseEvent: base(), ConflictID: f.branchConf, FactType: domain.FactPersonDeath, SubjectID: f.person, Description: "Burial and death dates disagree", Status: domain.ConflictStatusOpen})
	onBranch(f.researchLog, "ResearchLog", domain.ResearchLogUpdated{BaseEvent: base(), LogID: f.researchLog, Changes: map[string]any{"outcome": "found"}})
	createDelete("ResearchLog",
		func(id uuid.UUID) domain.Event {
			return domain.ResearchLogCreated{BaseEvent: base(), LogID: id, SubjectID: f.person, SubjectType: "person", Repository: "Parish chest", SearchDescription: "Looked for burial", Outcome: domain.ResearchOutcomeNotFound, SearchDate: time.Now()}
		},
		func(id uuid.UUID) domain.Event { return domain.ResearchLogDeleted{BaseEvent: base(), LogID: id} })
	onBranch(f.proofSummary, "ProofSummary", domain.ProofSummaryUpdated{BaseEvent: base(), SummaryID: f.proofSummary, Changes: map[string]any{"argument": "Census, baptism and memoir agree"}})
	createDelete("ProofSummary",
		func(id uuid.UUID) domain.Event {
			return domain.ProofSummaryCreated{BaseEvent: base(), SummaryID: id, FactType: domain.FactPersonDeath, SubjectID: f.person, Conclusion: "Died 27 Nov 1852"}
		},
		func(id uuid.UUID) domain.Event { return domain.ProofSummaryDeleted{BaseEvent: base(), SummaryID: id} })
	return f
}

var uuidPattern = regexp.MustCompile(`[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}`)

// validEntityTypes and validActions are the OpenAPI ChangeEntry enums.
var (
	validEntityTypes = map[string]bool{
		"person": true, "family": true, "source": true, "citation": true, "media": true, "note": true,
		"submitter": true, "repository": true, "association": true, "life_event": true, "attribute": true,
		"lds_ordinance": true, "evidence_analysis": true, "evidence_conflict": true, "research_log": true,
		"proof_summary": true, "branch": true,
	}
	validActions = map[string]bool{"created": true, "updated": true, "deleted": true, "merged": true}
)

// assertReadableEntries checks the ChangeEntry contract on every entry: a
// real type and action, a name that is words rather than an id, and on every
// update the fields it changed.
func assertReadableEntries(t *testing.T, entries []ChangeEntry) {
	t.Helper()
	for _, e := range entries {
		assert.True(t, validEntityTypes[e.EntityType], "entry %s: entity_type %q", e.EntityID, e.EntityType)
		assert.True(t, validActions[e.Action], "entry %s: action %q", e.EntityID, e.Action)
		assert.NotEmpty(t, e.EntityName, "entry %s (%s %s) has no name", e.EntityID, e.EntityType, e.Action)
		assert.False(t, uuidPattern.MatchString(e.EntityName), "entry %s (%s %s) is named by an id: %q", e.EntityID, e.EntityType, e.Action, e.EntityName)
		if e.Action == actionUpdated {
			assert.NotEmpty(t, e.Changes, "update of %s %q carries no field changes", e.EntityType, e.EntityName)
		}
	}
}

// byEntity returns the entries about one entity, in order.
func byEntity(entries []ChangeEntry, id uuid.UUID) []ChangeEntry {
	var out []ChangeEntry
	for _, e := range entries {
		if e.EntityID == id {
			out = append(out, e)
		}
	}
	return out
}

// TestCompareBranch_EveryBranchWritableType is #827's "done when": a branch
// touching every branch-writable type compares with the right type, action,
// name, parent link and before/after values for each change — on every
// event-store backend.
func TestCompareBranch_EveryBranchWritableType(t *testing.T) {
	for _, backend := range historyBackends() {
		t.Run(backend.name, func(t *testing.T) {
			ctx := context.Background()
			es, rs := backend.open(t)
			branches := memory.NewBranchStore()
			f := writeEveryTypeFixture(t, ctx, es, rs, branches)
			service := NewBranchService(branches, es, NewHistoryService(es, rs))

			result, err := service.CompareBranch(ctx, f.branch.ID)
			require.NoError(t, err)
			require.Len(t, result.BranchChanges, f.branchCount, "every branch event is a change entry")
			assertReadableEntries(t, result.BranchChanges)

			type want struct {
				entityType, name     string
				field                string
				oldValue, newValue   any
				parentType           string
				parentID             uuid.UUID
				expectedEntriesCount int
			}
			wants := map[uuid.UUID]want{
				f.family: {entityType: "family", name: "Ada Lovelace & Charles Babbage", field: "marriage_place", oldValue: "London", newValue: "Ockham", expectedEntriesCount: 3},
				f.source: {entityType: "source", name: "1851 Census", field: "title", oldValue: "1850 Census", newValue: "1851 Census", expectedEntriesCount: 1},
				// A citation is named from its read-model row, whose source title
				// is denormalized when the citation row is written.
				f.citation:     {entityType: "citation", name: "1850 Census (Birth)", field: "page", oldValue: "p. 12", newValue: "p. 13", parentType: "source", parentID: f.source, expectedEntriesCount: 1},
				f.note:         {entityType: "note", name: "Ada wrote the first algorithm for a machine.", field: "text", oldValue: "Ada wrote the first published algorithm.", newValue: "Ada wrote the first algorithm for a machine.", expectedEntriesCount: 1},
				f.media:        {entityType: "media", name: "Portrait (1840)", field: "title", oldValue: "Portrait", newValue: "Portrait (1840)", parentType: "person", parentID: f.person, expectedEntriesCount: 1},
				f.lifeEvent:    {entityType: "life_event", name: "Birth, 10 DEC 1815, Marylebone", field: "place", oldValue: "London", newValue: "Marylebone", parentType: "person", parentID: f.person, expectedEntriesCount: 1},
				f.attribute:    {entityType: "attribute", name: "Occupation: Analyst", field: "value", oldValue: "Mathematician", newValue: "Analyst", parentType: "person", parentID: f.person, expectedEntriesCount: 1},
				f.association:  {entityType: "association", name: "friend: Ada Lovelace and Charles Babbage", field: "role", oldValue: "colleague", newValue: "friend", parentType: "person", parentID: f.person, expectedEntriesCount: 1},
				f.analysis:     {entityType: "evidence_analysis", name: "Birth: Born 10 Dec 1815 in Marylebone", field: "conclusion", oldValue: "Born 10 Dec 1815 in London", newValue: "Born 10 Dec 1815 in Marylebone", expectedEntriesCount: 1},
				f.conflict:     {entityType: "evidence_conflict", name: "Birth: Two birth dates disagree", field: "status", oldValue: "open", newValue: "resolved", expectedEntriesCount: 1},
				f.researchLog:  {entityType: "research_log", name: "Searched baptism registers (National Archives)", field: "outcome", oldValue: "inconclusive", newValue: "found", expectedEntriesCount: 1},
				f.proofSummary: {entityType: "proof_summary", name: "Birth: Born 1815", field: "argument", oldValue: "Census and baptism agree", newValue: "Census, baptism and memoir agree", expectedEntriesCount: 1},
			}
			for id, w := range wants {
				got := byEntity(result.BranchChanges, id)
				require.Len(t, got, w.expectedEntriesCount, "%s entries", w.entityType)
				e := got[0]
				assert.Equal(t, w.entityType, e.EntityType)
				assert.Equal(t, actionUpdated, e.Action, "%s", w.entityType)
				assert.Equal(t, w.name, e.EntityName, "%s name", w.entityType)
				require.Contains(t, e.Changes, w.field, "%s changes", w.entityType)
				assert.Equal(t, w.oldValue, e.Changes[w.field].OldValue, "%s %s old value", w.entityType, w.field)
				assert.Equal(t, w.newValue, e.Changes[w.field].NewValue, "%s %s new value", w.entityType, w.field)
				assert.Equal(t, w.parentType, e.ParentEntityType, "%s parent type", w.entityType)
				if w.parentType != "" {
					require.NotNil(t, e.ParentEntityID)
					assert.Equal(t, w.parentID, *e.ParentEntityID)
				} else {
					assert.Nil(t, e.ParentEntityID)
				}
			}

			// The person: a field edit, then its name variants.
			person := byEntity(result.BranchChanges, f.person)
			require.Len(t, person, 4)
			for _, e := range person {
				assert.Equal(t, "person", e.EntityType)
				assert.Equal(t, actionUpdated, e.Action)
				assert.Equal(t, "Ada Lovelace", e.EntityName)
			}
			assert.Equal(t, FieldChange{OldValue: "London", NewValue: "Marylebone"}, person[0].Changes["birth_place"])
			assert.Equal(t, FieldChange{OldValue: "Ada Lovelace", NewValue: "Augusta Ada Lovelace"}, person[1].Changes["name"])
			assert.Equal(t, FieldChange{NewValue: "Ada Byron"}, person[2].Changes["name"])
			assert.Equal(t, FieldChange{NewValue: "birth"}, person[2].Changes["name_type"])
			assert.Equal(t, FieldChange{OldValue: "Ada Byron"}, person[3].Changes["name"])

			family := byEntity(result.BranchChanges, f.family)
			assert.Equal(t, "Child linked: Charles Babbage", family[1].Changes["children"].NewValue)
			assert.Equal(t, "Child unlinked: Charles Babbage", family[2].Changes["children"].NewValue)

			// A conflict the branch detected is created there.
			detected := byEntity(result.BranchChanges, f.branchConf)
			require.Len(t, detected, 1)
			assert.Equal(t, actionCreated, detected[0].Action)
			assert.Equal(t, "Death: Burial and death dates disagree", detected[0].EntityName)

			// Branch-only entities: created then deleted, both named.
			names := map[string]bool{}
			for _, e := range result.BranchChanges {
				if e.Action == actionCreated || e.Action == actionDeleted {
					names[e.EntityType+": "+e.EntityName] = true
				}
			}
			for _, want := range []string{
				"person: Allegra Byron", "family: Charles Babbage", "source: Parish Register",
				"citation: 1851 Census (Death)", "note: A passing thought", "media: Wedding sketch",
				"life_event: Marriage, 8 JUL 1835", "attribute: Occupation: Writer",
				"association: mentor: Charles Babbage and Ada Lovelace", "evidence_analysis: Death: Died 1852",
				"research_log: Looked for burial (Parish chest)", "proof_summary: Death: Died 27 Nov 1852",
			} {
				assert.True(t, names[want], "missing created/deleted entry %q (have %v)", want, names)
			}

			// The mainline's full log reads the same way, and a branch's edits
			// are not part of it.
			history := NewHistoryService(es, rs)
			global, err := history.GetGlobalHistory(ctx, GetGlobalHistoryInput{Limit: 100})
			require.NoError(t, err)
			assert.Equal(t, f.mainEventCount, global.TotalCount, "branch events are excluded from the mainline history")
			assert.Len(t, global.Entries, f.mainEventCount)
			assertReadableEntries(t, global.Entries)
		})
	}
}
