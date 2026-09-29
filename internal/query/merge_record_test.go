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

// mergedScenario is a branch forked after two people exist on main. The branch
// renames Ada (which main also renames: an edit/edit conflict) and Allegra,
// and creates Clara. merge() then plays a merge the way the merge command
// writes one: the claim on the branch's own stream, the kept streams replayed
// onto main, and the registry marked merged.
type mergedScenario struct {
	f                    *branchTestFixture
	branch               *domain.Branch
	ada, allegra, clara  uuid.UUID
	branchEventsByStream map[uuid.UUID][]repository.StoredEvent
}

func newMergedScenario(t *testing.T) *mergedScenario {
	t.Helper()
	f := newBranchTestFixture(t)
	s := &mergedScenario{f: f, ada: uuid.New(), allegra: uuid.New(), clara: uuid.New()}
	f.appendMain(t, s.ada, domain.NewPersonCreated(&domain.Person{ID: s.ada, GivenName: "Ada", Surname: "Lovelace"}))
	f.appendMain(t, s.allegra, domain.NewPersonCreated(&domain.Person{ID: s.allegra, GivenName: "Allegra", Surname: "Byron"}))
	f.projectMainPerson(t, s.ada, "Ada", "Lovelace")
	f.projectMainPerson(t, s.allegra, "Allegra", "Byron")

	s.branch = f.forkBranch(t, "Byron theory")
	require.NoError(t, f.eventStore.Append(f.ctx, s.branch.ID, "branch",
		[]domain.Event{domain.NewBranchCreated(s.branch)}, anyVersion,
		repository.AppendScope{BranchID: domain.BranchID(s.branch.ID)}))
	f.appendBranch(t, s.branch, s.ada, "person", domain.NewPersonUpdated(s.ada, map[string]any{"surname": "Byron"}))
	f.appendBranch(t, s.branch, s.allegra, "person", domain.NewPersonUpdated(s.allegra, map[string]any{"surname": "Clairmont"}))
	f.appendBranch(t, s.branch, s.clara, "person", domain.NewPersonCreated(&domain.Person{ID: s.clara, GivenName: "Clara", Surname: "Clairmont"}))
	f.appendMain(t, s.ada, domain.NewPersonUpdated(s.ada, map[string]any{"surname": "King"}))

	own, err := f.eventStore.ReadBranch(f.ctx, domain.BranchID(s.branch.ID), 0, 100)
	require.NoError(t, err)
	s.branchEventsByStream = groupEventsByStreamID(withoutBranchLifecycleEvents(own))
	return s
}

// projectMainPerson gives a person a mainline read-model row, so names resolve.
func (f *branchTestFixture) projectMainPerson(t *testing.T, id uuid.UUID, given, surname string) {
	t.Helper()
	require.NoError(t, f.readStore.SavePerson(f.ctx, domain.MainBranchID,
		&repository.PersonReadModel{ID: id, GivenName: given, Surname: surname, FullName: given + " " + surname, Version: 1}))
}

// merge writes the claim and replays every stream but the kept ones. stamp
// false replays the way a merge before #832 did: no provenance.
func (s *mergedScenario) merge(t *testing.T, claim domain.BranchMerged, stamp bool, kept ...uuid.UUID) domain.BranchMerged {
	t.Helper()
	f := s.f
	scope := repository.AppendScope{BranchID: domain.BranchID(s.branch.ID)}
	require.NoError(t, f.eventStore.Append(f.ctx, s.branch.ID, "branch", []domain.Event{claim}, anyVersion, scope))
	skip := make(map[uuid.UUID]bool)
	for _, id := range kept {
		skip[id] = true
	}
	prov := &domain.MergeProvenance{BranchID: s.branch.ID, BranchName: s.branch.Name, ClaimID: claim.ID,
		MergedAtPosition: claim.MergedAtPosition, MergedAt: claim.Timestamp, Note: claim.Note}
	for _, streamID := range []uuid.UUID{s.ada, s.allegra, s.clara} {
		if skip[streamID] {
			continue
		}
		for i := range s.branchEventsByStream[streamID] {
			decoded, err := s.branchEventsByStream[streamID][i].DecodeEvent()
			require.NoError(t, err)
			if stamp {
				decoded = domain.Stamp(decoded, domain.EventMetadata{MergedFromBranch: prov}, prov.MergedAt)
			}
			f.appendMain(t, streamID, decoded)
		}
	}
	require.NoError(t, f.branchStore.MarkMerged(f.ctx, s.branch.ID, claim.Timestamp, claim.Note))
	return claim
}

func TestCompareBranch_MergedBranchShowsItsRecord(t *testing.T) {
	s := newMergedScenario(t)
	claim := domain.NewBranchMerged(s.branch.ID, s.branch.BasePosition, s.f.maxPosition(t), "Byron theory holds",
		map[uuid.UUID]int64{s.ada: 2, s.clara: 0})
	count := 2
	claim.ReplayedEventCount = &count
	claim.SkippedStreamIDs = []uuid.UUID{s.allegra}
	claim.Resolutions = []domain.MergeDecision{{StreamID: s.ada, EntityType: "person", EntityName: "Ada Byron",
		Kind: "edit_edit", Fields: []string{"surname"}, Resolution: "branch", Rationale: "baptism register"}}
	claim.Exclusions = []domain.MergeExclusion{{StreamID: s.allegra, EntityType: "person", EntityName: "Allegra Clairmont"}}
	snapshotID := uuid.New()
	claim.PreMergeSnapshotID = &snapshotID
	s.merge(t, claim, true, s.allegra)
	replayedThrough := s.f.maxPosition(t)
	// Main moves on after the merge; the record still ends at the replay.
	later := uuid.New()
	s.f.appendMain(t, later, domain.NewPersonCreated(&domain.Person{ID: later, GivenName: "Later", Surname: "Mainline"}))

	result, err := s.f.service.CompareBranch(s.f.ctx, s.branch.ID)
	require.NoError(t, err)
	require.NotNil(t, result.MergeRecord)
	assert.Equal(t, &snapshotID, result.MergeRecord.PreMergeSnapshotID)
	require.NotNil(t, result.MergeRecord.ReplayedThroughPosition)
	assert.Equal(t, replayedThrough, *result.MergeRecord.ReplayedThroughPosition)

	// The replayed copies are not the mainline's own changes.
	assert.Equal(t, 2, result.ReplayedChangeCount)
	require.Len(t, result.MainChanges, 1, "only main's independent edit of Ada is a mainline change")
	assert.Equal(t, s.ada, result.MainChanges[0].EntityID)
	assert.Equal(t, []uuid.UUID{s.ada}, result.OverlappingStreamIDs)

	record := result.MergeRecord
	require.NotNil(t, record)
	assert.True(t, record.Recorded)
	assert.Equal(t, claim.ID, record.ClaimID)
	assert.Equal(t, "Byron theory holds", record.Note)
	assert.True(t, record.MergedAt.Equal(claim.Timestamp))
	require.NotNil(t, record.ReplayedEventCount)
	assert.Equal(t, 2, *record.ReplayedEventCount)
	assert.Equal(t, []uuid.UUID{s.allegra}, record.SkippedStreamIDs)
	require.Len(t, record.Decisions, 1)
	assert.Equal(t, MergeRecordDecision{StreamID: s.ada, EntityType: "person", EntityName: "Ada Byron", Kind: "edit_edit",
		Fields: []string{"surname"}, Resolution: "branch", Rationale: "baptism register", DecidedAt: MergeDecidedAtMerge}, record.Decisions[0])
	assert.Equal(t, []MergeRecordExclusion{{StreamID: s.allegra, EntityType: "person", EntityName: "Allegra Clairmont"}}, record.Exclusions)
	assert.Zero(t, record.ResumeCount)
}

// A merge recorded before #832 has neither a record on its claim nor
// provenance on its replay: the record is derived from the plan, and the
// copies are still recognised by their payload ids.
// A merge that replays nothing changed nothing: its effect ends at its own
// claim, so later mainline work stays out of the "what the merge changed"
// range rather than falling back to everything since the snapshot (#833).
func TestCompareBranch_MergeThatReplayedNothingEndsAtItsClaim(t *testing.T) {
	s := newMergedScenario(t)
	claim := domain.NewBranchMerged(s.branch.ID, s.branch.BasePosition, s.f.maxPosition(t), "", map[uuid.UUID]int64{})
	count := 0
	claim.ReplayedEventCount = &count
	snapshotID := uuid.New()
	claim.PreMergeSnapshotID = &snapshotID
	s.merge(t, claim, true, s.ada, s.allegra, s.clara)
	claimPosition := s.f.maxPosition(t)
	later := uuid.New()
	s.f.appendMain(t, later, domain.NewPersonCreated(&domain.Person{ID: later, GivenName: "Later", Surname: "Mainline"}))

	result, err := s.f.service.CompareBranch(s.f.ctx, s.branch.ID)
	require.NoError(t, err)
	require.NotNil(t, result.MergeRecord)
	require.NotNil(t, result.MergeRecord.ReplayedThroughPosition)
	assert.Equal(t, claimPosition, *result.MergeRecord.ReplayedThroughPosition)
	assert.Zero(t, result.ReplayedChangeCount)
}

func TestCompareBranch_MergedBeforeTheRecord(t *testing.T) {
	s := newMergedScenario(t)
	claim := domain.NewBranchMerged(s.branch.ID, s.branch.BasePosition, s.f.maxPosition(t), "",
		map[uuid.UUID]int64{s.ada: 2, s.clara: 0})
	claim.ResolutionRationales = map[uuid.UUID]string{s.allegra: "unproven"}
	s.merge(t, claim, false, s.allegra)

	result, err := s.f.service.CompareBranch(s.f.ctx, s.branch.ID)
	require.NoError(t, err)
	assert.Equal(t, 2, result.ReplayedChangeCount)
	require.Len(t, result.MainChanges, 1)

	record := result.MergeRecord
	require.NotNil(t, record)
	assert.False(t, record.Recorded)
	assert.Nil(t, record.ReplayedEventCount)
	assert.Empty(t, record.Decisions)
	assert.Equal(t, []MergeRecordExclusion{{StreamID: s.allegra, EntityType: "person", EntityName: "Allegra Byron", Rationale: "unproven"}},
		record.Exclusions, "named from the log, as the mainline knows her")
	assert.Equal(t, []uuid.UUID{s.allegra}, record.SkippedStreamIDs)
}

// A resume's decisions supersede the claim's for the same entity, and the
// skipped list follows the plan as the last resume left it.
func TestCompareBranch_MergeRecordIncludesResumeDecisions(t *testing.T) {
	s := newMergedScenario(t)
	claim := domain.NewBranchMerged(s.branch.ID, s.branch.BasePosition, s.f.maxPosition(t), "",
		map[uuid.UUID]int64{s.ada: 2, s.allegra: 1, s.clara: 0})
	count := 3
	claim.ReplayedEventCount = &count
	claim.Resolutions = []domain.MergeDecision{{StreamID: s.ada, EntityType: "person", EntityName: "Ada Byron",
		Kind: "edit_edit", Fields: []string{"surname"}, Resolution: "branch"}}
	claim.Exclusions = []domain.MergeExclusion{}
	s.merge(t, claim, true, s.allegra, s.ada)

	resumed := domain.NewBranchMergeResumed(s.branch.ID, claim.MergedAtPosition,
		map[uuid.UUID]int64{s.allegra: 1, s.clara: 0},
		map[uuid.UUID]string{s.ada: "main"})
	resumed.Rationales = map[uuid.UUID]string{s.ada: "main moved on"}
	require.NoError(t, s.f.eventStore.Append(s.f.ctx, s.branch.ID, "branch", []domain.Event{resumed}, anyVersion,
		repository.AppendScope{BranchID: domain.BranchID(s.branch.ID)}))

	result, err := s.f.service.CompareBranch(s.f.ctx, s.branch.ID)
	require.NoError(t, err)
	record := result.MergeRecord
	require.NotNil(t, record)
	assert.Equal(t, 1, record.ResumeCount)
	require.Len(t, record.Decisions, 1, "the resume's decision replaces the claim's")
	assert.Equal(t, MergeRecordDecision{StreamID: s.ada, EntityType: "person", EntityName: "Ada Byron", Kind: "edit_edit",
		Fields: []string{"surname"}, Resolution: "main", Rationale: "main moved on", DecidedAt: MergeDecidedAtResume}, record.Decisions[0])
	assert.Equal(t, []uuid.UUID{s.ada}, record.SkippedStreamIDs)
}

// An active branch has no merge record and nothing suppressed.
func TestCompareBranch_ActiveBranchHasNoRecord(t *testing.T) {
	s := newMergedScenario(t)
	result, err := s.f.service.CompareBranch(s.f.ctx, s.branch.ID)
	require.NoError(t, err)
	assert.Nil(t, result.MergeRecord)
	assert.Zero(t, result.ReplayedChangeCount)
}

// The history reports a replayed change with the merge it came with (#832)
// and the branch lifecycle as entries of its own, named from the registry.
func TestHistory_MergeProvenanceAndLifecycle(t *testing.T) {
	s := newMergedScenario(t)
	claim := domain.NewBranchMerged(s.branch.ID, s.branch.BasePosition, s.f.maxPosition(t), "Byron theory holds",
		map[uuid.UUID]int64{s.ada: 2, s.allegra: 1, s.clara: 0})
	claim.Timestamp = time.Now().UTC().Add(time.Hour)
	s.merge(t, claim, true)
	history := s.f.service.historyService
	history.UseBranchStore(s.f.branchStore)

	ada, err := history.GetEntityHistory(s.f.ctx, "person", s.ada, 10, 0)
	require.NoError(t, err)
	require.Len(t, ada.Entries, 3)
	assert.Nil(t, ada.Entries[1].MergedFrom, "main's own edit came with no merge")
	replayed := ada.Entries[2]
	require.NotNil(t, replayed.MergedFrom)
	assert.Equal(t, s.branch.ID, replayed.MergedFrom.BranchID)
	assert.Equal(t, "Byron theory", replayed.MergedFrom.BranchName)
	assert.Equal(t, "Byron theory holds", replayed.MergedFrom.Note)
	assert.True(t, replayed.MergedFrom.MergedAt.Equal(claim.Timestamp))
	assert.True(t, replayed.Timestamp.Equal(claim.Timestamp), "shown when it reached the mainline")
	assert.True(t, replayed.MergedFrom.OriginalTimestamp.Before(claim.Timestamp), "and when it was made on the branch")

	// The window after the merge holds the replayed changes and the
	// lifecycle, though the changes were made on the branch before it.
	window, err := history.GetGlobalHistory(s.f.ctx, GetGlobalHistoryInput{FromTime: claim.Timestamp.Add(-time.Minute), Limit: 20})
	require.NoError(t, err)
	var merged, replays int
	for _, entry := range window.Entries {
		switch {
		case entry.EntityType == entityTypeBranch && entry.Action == actionMerged:
			merged++
			assert.Equal(t, "Byron theory", entry.EntityName)
			assert.Equal(t, "Byron theory holds", entry.Changes["merge_note"].NewValue)
		case entry.MergedFrom != nil:
			replays++
		}
	}
	assert.Equal(t, 1, merged, "the merge itself is an entry: %+v", window.Entries)
	assert.Equal(t, 3, replays)

	all, err := history.GetGlobalHistory(s.f.ctx, GetGlobalHistoryInput{EventTypes: HistoryEventTypesForEntity("branch"), Limit: 20})
	require.NoError(t, err)
	require.Len(t, all.Entries, 2, "created and merged")
	assert.Equal(t, actionCreated, all.Entries[0].Action)
	assert.Equal(t, "Byron theory", all.Entries[0].EntityName)
}

// Without the registry a branch is named by its BranchCreated event; a merge
// or delete event of an unknown branch still renders, unnamed.
func TestHistory_LifecycleWithoutRegistry(t *testing.T) {
	f := newBranchTestFixture(t)
	branch := f.forkBranch(t, "Clairmont line")
	created := domain.NewBranchCreated(branch)
	created.Description = "Mary Jane's children"
	require.NoError(t, f.eventStore.Append(f.ctx, branch.ID, "branch", []domain.Event{created, domain.NewBranchDeleted(branch.ID)}, anyVersion,
		repository.AppendScope{BranchID: domain.BranchID(branch.ID)}))

	page, err := f.service.historyService.GetGlobalHistory(f.ctx, GetGlobalHistoryInput{Limit: 10})
	require.NoError(t, err)
	require.Len(t, page.Entries, 2)
	assert.Equal(t, "Clairmont line", page.Entries[0].EntityName)
	assert.Equal(t, "Mary Jane's children", page.Entries[0].Changes["description"].NewValue)
	assert.Equal(t, actionDeleted, page.Entries[1].Action)
	assert.Equal(t, "", page.Entries[1].EntityName)
}

// A close (#836) shows in the global history with its outcome and reason; a
// delete written before #836 carries no changes.
func TestHistory_BranchCloseCarriesOutcomeAndReason(t *testing.T) {
	f := newBranchTestFixture(t)
	closed := f.forkBranch(t, "Byron theory")
	legacy := f.forkBranch(t, "Clairmont line")
	closeEvt := domain.NewBranchClosed(closed.ID, domain.BranchOutcomeDisproved, "The will names other heirs")
	closeEvt.Timestamp = time.Now().UTC().Add(time.Hour)
	reasonless := domain.NewBranchClosed(legacy.ID, domain.BranchOutcomeDisproved, "")
	reasonless.Timestamp = closeEvt.Timestamp.Add(time.Minute)
	require.NoError(t, f.eventStore.Append(f.ctx, closed.ID, "branch", []domain.Event{domain.NewBranchCreated(closed), closeEvt}, anyVersion,
		repository.AppendScope{BranchID: domain.BranchID(closed.ID)}))
	require.NoError(t, f.eventStore.Append(f.ctx, legacy.ID, "branch", []domain.Event{domain.NewBranchCreated(legacy), domain.NewBranchDeleted(legacy.ID), reasonless}, anyVersion,
		repository.AppendScope{BranchID: domain.BranchID(legacy.ID)}))

	history := f.service.historyService
	history.UseBranchStore(f.branchStore)
	page, err := history.GetGlobalHistory(f.ctx, GetGlobalHistoryInput{EventTypes: []string{"BranchDeleted"}, Limit: 10})
	require.NoError(t, err)
	require.Len(t, page.Entries, 3)
	byName := map[string][]ChangeEntry{}
	for _, e := range page.Entries {
		assert.Equal(t, actionDeleted, e.Action)
		byName[e.EntityName] = append(byName[e.EntityName], e)
	}
	require.Len(t, byName["Byron theory"], 1)
	changes := byName["Byron theory"][0].Changes
	assert.Equal(t, string(domain.BranchOutcomeDisproved), changes["outcome"].NewValue)
	assert.Equal(t, "The will names other heirs", changes["close_reason"].NewValue)

	require.Len(t, byName["Clairmont line"], 2)
	var sawLegacy, sawReasonless bool
	for _, e := range byName["Clairmont line"] {
		if e.Changes == nil {
			sawLegacy = true
			continue
		}
		sawReasonless = true
		assert.Equal(t, string(domain.BranchOutcomeDisproved), e.Changes["outcome"].NewValue)
		_, hasReason := e.Changes["close_reason"]
		assert.False(t, hasReason, "no reason given, no reason change")
	}
	assert.True(t, sawLegacy, "a pre-#836 delete carries no changes")
	assert.True(t, sawReasonless)
}

func TestIsReplayedCopy_MetadataDecides(t *testing.T) {
	branchID := uuid.New()
	original := domain.NewPersonUpdated(uuid.New(), map[string]any{"surname": "Byron"})
	data, meta, _, err := domain.EncodeForStore(domain.Stamp(original, domain.EventMetadata{
		MergedFromBranch: &domain.MergeProvenance{BranchID: uuid.New()}}, time.Time{}))
	require.NoError(t, err)
	other := repository.StoredEvent{Data: data, Metadata: meta}
	originals := map[uuid.UUID]bool{original.ID: true}
	assert.False(t, isReplayedCopy(branchID, originals, &other), "another branch's merge replayed it")

	unreadable := repository.StoredEvent{Data: data, Metadata: []byte("{")}
	assert.True(t, isReplayedCopy(branchID, originals, &unreadable), "falls back to the payload id")
	_, ok := payloadID(&repository.StoredEvent{Data: []byte("{")})
	assert.False(t, ok)
}
