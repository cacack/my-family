package query

import (
	"context"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/cacack/my-family/internal/domain"
	"github.com/cacack/my-family/internal/repository"
	"github.com/cacack/my-family/internal/repository/memory"
)

// redactorTree is a dead and a living person, a family of the two, and one
// record of every other type the redactor checks, attached to the dead
// person where it has an owner, saved straight into a memory store.
type redactorTree struct {
	redactor     changeRedactor
	dead, living uuid.UUID
	family       uuid.UUID
	records      map[string]uuid.UUID // by history entity type
}

func newRedactorTree(t *testing.T) *redactorTree {
	t.Helper()
	ctx := context.Background()
	raw := memory.NewReadModelStore()
	main := domain.MainBranchID
	now := time.Now().UTC()
	must := func(err error) {
		t.Helper()
		if err != nil {
			t.Fatal(err)
		}
	}
	tr := &redactorTree{redactor: redactorFor(NewPrivacyStore(raw)), records: map[string]uuid.UUID{}}
	person := func(given string, dead bool) uuid.UUID {
		p := &repository.PersonReadModel{ID: uuid.New(), GivenName: given, FullName: given, DeathEvidence: dead, Version: 1, UpdatedAt: now}
		must(raw.SavePerson(ctx, main, p))
		return p.ID
	}
	tr.dead, tr.living = person("Dora", true), person("Lena", false)
	dora, err := raw.GetPerson(ctx, main, tr.dead)
	must(err)
	dora.BirthPlace = "Boston, MA"
	must(raw.SavePerson(ctx, main, dora))
	tr.family = uuid.New()
	must(raw.SaveFamily(ctx, main, &repository.FamilyReadModel{ID: tr.family, Partner1ID: &tr.dead, Partner2ID: &tr.living, Version: 1, UpdatedAt: now}))

	id := func(entityType string) uuid.UUID {
		tr.records[entityType] = uuid.New()
		return tr.records[entityType]
	}
	must(raw.SaveSource(ctx, main, &repository.SourceReadModel{ID: id(entityTypeSource), Title: "Register", SourceType: domain.SourceOther, Version: 1, UpdatedAt: now}))
	must(raw.SaveCitation(ctx, main, &repository.CitationReadModel{ID: id(entityTypeCitation), SourceID: tr.records[entityTypeSource],
		FactType: domain.FactPersonBirth, FactOwnerID: tr.dead, Version: 1, CreatedAt: now}))
	must(raw.SaveMedia(ctx, main, &repository.MediaReadModel{ID: id(entityTypeMedia), EntityType: "person", EntityID: tr.dead, Title: "Photo", Version: 1, UpdatedAt: now}))
	must(raw.SaveNote(ctx, main, &repository.NoteReadModel{ID: id(entityTypeNote), Text: "note", Version: 1, UpdatedAt: now}))
	must(raw.SaveSubmitter(ctx, &repository.SubmitterReadModel{ID: id(entityTypeSubmitter), Name: "Researcher", Version: 1, UpdatedAt: now}))
	must(raw.SaveRepository(ctx, &repository.RepositoryReadModel{ID: id(entityTypeRepository), Name: "Archive", Version: 1, UpdatedAt: now}))
	must(raw.SaveAssociation(ctx, main, &repository.AssociationReadModel{ID: id(entityTypeAssociation), PersonID: tr.dead, AssociateID: tr.dead, Role: "witness", Version: 1, UpdatedAt: now}))
	must(raw.SaveEvent(ctx, main, &repository.EventReadModel{ID: id(entityTypeLifeEvent), OwnerType: "person", OwnerID: tr.dead, FactType: domain.FactPersonBurial, Version: 1, CreatedAt: now}))
	must(raw.SaveAttribute(ctx, main, &repository.AttributeReadModel{ID: id(entityTypeAttribute), PersonID: tr.dead, FactType: domain.FactPersonOccupation, Value: "Smith", Version: 1, CreatedAt: now}))
	must(raw.SaveLDSOrdinance(ctx, &repository.LDSOrdinanceReadModel{ID: id(entityTypeLDSOrdinance), Type: domain.LDSBaptism, PersonID: &tr.dead, Version: 1, UpdatedAt: now}))
	must(raw.SaveEvidenceAnalysis(ctx, main, &repository.EvidenceAnalysisReadModel{ID: id(entityTypeEvidenceAnalysis), FactType: domain.FactPersonBirth, SubjectID: tr.dead, Version: 1, CreatedAt: now, UpdatedAt: now}))
	must(raw.SaveEvidenceConflict(ctx, main, &repository.EvidenceConflictReadModel{ID: id(entityTypeEvidenceConflict), FactType: domain.FactPersonBirth, SubjectID: tr.dead, Status: domain.ConflictStatusOpen, Version: 1, CreatedAt: now, UpdatedAt: now}))
	must(raw.SaveResearchLog(ctx, main, &repository.ResearchLogReadModel{ID: id(entityTypeResearchLog), SubjectID: tr.dead, SubjectType: "person", Version: 1, CreatedAt: now, UpdatedAt: now}))
	must(raw.SaveProofSummary(ctx, main, &repository.ProofSummaryReadModel{ID: id(entityTypeProofSummary), FactType: domain.FactPersonBirth, SubjectID: tr.dead, Version: 1, CreatedAt: now, UpdatedAt: now}))
	must(raw.SaveDuplicateDismissal(ctx, main, &repository.DuplicateDismissalReadModel{ID: id(entityTypeDuplicatePair), Person1ID: tr.dead, Person2ID: tr.dead, CreatedAt: now, UpdatedAt: now}))
	// A place resolves from its use: Dora's birth place.
	tr.records[entityTypePlace] = repository.PlaceID("boston,ma")
	return tr
}

func strictViewerCtx(anonymous bool) context.Context {
	settings := repository.DefaultTreeSettings()
	settings.ViewersSeeLiving = false
	return repository.ContextWithTreeSettings(
		repository.ContextWithPrincipal(context.Background(), repository.Principal{Role: repository.RoleViewer, Anonymous: anonymous}), settings)
}

var mainScope = []domain.BranchID{domain.MainBranchID}

// Every record type the history knows resolves and is shown when it belongs
// to a dead person; one that does not resolve, or of a type the history does
// not know, is not.
func TestChangeRedactor_RecordShownForEveryType(t *testing.T) {
	tr := newRedactorTree(t)
	ctx := strictViewerCtx(false)
	v, _ := tr.redactor.privacy(ctx)
	if len(tr.records) != len(recordGetters) {
		t.Fatalf("the tree has %d record types, the redactor checks %d", len(tr.records), len(recordGetters))
	}
	check := tr.redactor.check(v, mainScope)
	for entityType, id := range tr.records {
		if shown, err := check.shown(ctx, entityType, id); err != nil || !shown {
			t.Errorf("%s of the dead person: shown = %v, %v; want shown", entityType, shown, err)
		}
		if shown, err := check.shown(ctx, entityType, uuid.New()); err != nil || shown {
			t.Errorf("unresolved %s: shown = %v, %v; want hidden", entityType, shown, err)
		}
	}
	for _, c := range []struct {
		entityType string
		id         uuid.UUID
		want       bool
	}{
		{entityTypePerson, tr.dead, true},
		{entityTypePerson, tr.living, false},
		{entityTypeFamily, tr.family, false},
		{entityTypeFamily, uuid.New(), false},
		{entityTypeBranch, uuid.New(), true},
		{"unknown", tr.dead, false},
	} {
		if shown, err := check.shown(ctx, c.entityType, c.id); err != nil || shown != c.want {
			t.Errorf("%s %s: shown = %v, %v; want %v", c.entityType, c.id, shown, err, c.want)
		}
	}
	for _, c := range []struct {
		subjectType string
		id          uuid.UUID
		want        bool
	}{{"", tr.dead, true}, {"", tr.family, false}, {"", tr.living, false}, {"person", tr.living, false}} {
		if shown, err := check.subjectShown(ctx, c.subjectType, c.id); err != nil || shown != c.want {
			t.Errorf("subject %q %s: shown = %v, %v; want %v", c.subjectType, c.id, shown, err, c.want)
		}
	}
}

// Change entries about a protected record, or under a protected parent, are
// dropped; a merged-away person is the placeholder; a visitor does not see
// who made a change. A principal who sees everything gets entries as they
// were, and so does a service that does not read through the decorator.
func TestChangeRedactor_Entries(t *testing.T) {
	tr := newRedactorTree(t)
	user, name := "user-1", "Researcher One"
	parent := tr.living
	entries := []ChangeEntry{
		{EntityType: entityTypePerson, EntityID: tr.dead, UserID: &user, UserName: &name,
			Changes: map[string]FieldChange{mergedPersonField: {NewValue: "Lena"}}},
		{EntityType: entityTypePerson, EntityID: tr.living},
		{EntityType: entityTypeLifeEvent, EntityID: tr.records[entityTypeLifeEvent], ParentEntityType: entityTypePerson, ParentEntityID: &parent},
		{EntityType: entityTypeBranch, EntityID: uuid.New()},
	}

	for _, anonymous := range []bool{false, true} {
		got, err := tr.redactor.entries(strictViewerCtx(anonymous), mainScope, entries)
		if err != nil {
			t.Fatal(err)
		}
		if len(got) != 2 || got[0].EntityID != tr.dead || got[1].EntityType != entityTypeBranch {
			t.Fatalf("anonymous %v: entries = %+v, want the dead person's and the branch's", anonymous, got)
		}
		if got[0].Changes[mergedPersonField].NewValue != repository.PlaceholderGivenName {
			t.Errorf("merged person = %v, want the placeholder", got[0].Changes[mergedPersonField])
		}
		if (got[0].UserID == nil) != anonymous || (got[0].UserName == nil) != anonymous {
			t.Errorf("anonymous %v: attribution = %v %v", anonymous, got[0].UserID, got[0].UserName)
		}
	}
	if entries[0].Changes[mergedPersonField].NewValue != "Lena" {
		t.Error("redacting changed the caller's entries")
	}

	editor := repository.ContextWithPrincipal(context.Background(), repository.Principal{Role: repository.RoleEditor})
	for name, r := range map[string]changeRedactor{"editor": tr.redactor, "real store": redactorFor(memory.NewReadModelStore())} {
		ctx := editor
		if name == "real store" {
			ctx = strictViewerCtx(true)
		}
		if got, err := r.entries(ctx, mainScope, entries); err != nil || len(got) != len(entries) {
			t.Errorf("%s: entries = %d, %v; want all %d", name, len(got), err, len(entries))
		}
		if r.placeholderUnresolved(ctx) {
			t.Errorf("%s: unresolved people named as the placeholder", name)
		}
	}
	if !tr.redactor.placeholderUnresolved(strictViewerCtx(false)) {
		t.Error("viewer without living: unresolved people not named as the placeholder")
	}
}

// The merge review, restore points, changed facts and the research archive
// keep only what is about a shown record; a visitor does not see who merged.
func TestChangeRedactor_OtherOutputs(t *testing.T) {
	tr := newRedactorTree(t)
	ctx := strictViewerCtx(true)

	conflicts, err := tr.redactor.conflicts(ctx, mainScope, []MergeConflict{
		{StreamID: tr.dead, EntityType: entityTypePerson}, {StreamID: tr.living, EntityType: entityTypePerson},
	})
	if err != nil || len(conflicts) != 1 || conflicts[0].StreamID != tr.dead {
		t.Errorf("conflicts = %+v, %v; want the dead person's", conflicts, err)
	}

	record := &MergeRecord{MergedBy: "user-1", MergedByName: "Researcher One",
		Decisions:  []MergeRecordDecision{{StreamID: tr.dead, EntityType: entityTypePerson}, {StreamID: tr.living, EntityType: entityTypePerson}},
		Exclusions: []MergeRecordExclusion{{StreamID: tr.living, EntityType: entityTypePerson}, {StreamID: tr.dead, EntityType: entityTypePerson}},
	}
	if err := tr.redactor.mergeRecord(ctx, mainScope, record); err != nil {
		t.Fatal(err)
	}
	if record.MergedBy != "" || record.MergedByName != "" || len(record.Decisions) != 1 || len(record.Exclusions) != 1 ||
		record.Decisions[0].StreamID != tr.dead || record.Exclusions[0].StreamID != tr.dead {
		t.Errorf("merge record = %+v, want the dead person's decision and exclusion and no attribution", record)
	}
	if err := tr.redactor.mergeRecord(ctx, mainScope, nil); err != nil {
		t.Errorf("nil merge record: %v", err)
	}

	if got := tr.redactor.streams(ctx, []uuid.UUID{tr.dead, tr.living}, []ChangeEntry{{EntityID: tr.dead}}); len(got) != 1 || got[0] != tr.dead {
		t.Errorf("streams = %v, want the dead person's", got)
	}

	points := &RestorePointsResult{RestorePoints: []RestorePoint{{Version: 1}}, TotalCount: 1, Limit: 20}
	for id, want := range map[uuid.UUID]int{tr.dead: 1, tr.living: 0} {
		got, err := tr.redactor.restorePoints(ctx, entityTypePerson, id, points)
		if err != nil || len(got.RestorePoints) != want || got.TotalCount != want || got.Limit != 20 {
			t.Errorf("restore points of %s = %+v, %v; want %d", id, got, err, want)
		}
	}

	facts, err := tr.redactor.changedFacts(ctx, mainScope, []ChangedFact{
		{SubjectType: entityTypePerson, SubjectID: tr.living}, {SubjectType: entityTypePerson, SubjectID: tr.dead},
	})
	if err != nil || len(facts) != 1 || facts[0].SubjectID != tr.dead {
		t.Errorf("changed facts = %+v, %v; want the dead person's", facts, err)
	}

	archive := &BranchResearchArchive{
		ResearchLogs: []ArchivedResearchLog{{ResearchLogEntry: ResearchLogEntry{SubjectID: tr.dead, SubjectType: "person"}},
			{ResearchLogEntry: ResearchLogEntry{SubjectID: tr.living, SubjectType: "person"}}},
		EvidenceAnalyses: []ArchivedEvidenceAnalysis{{EvidenceAnalysis: EvidenceAnalysis{SubjectID: tr.living}},
			{EvidenceAnalysis: EvidenceAnalysis{SubjectID: tr.dead}}},
		ProofSummaries: []ArchivedProofSummary{{ProofSummaryResult: ProofSummaryResult{SubjectID: uuid.New()}}},
	}
	if err := tr.redactor.archive(ctx, mainScope, archive); err != nil {
		t.Fatal(err)
	}
	if len(archive.ResearchLogs) != 1 || archive.ResearchLogs[0].SubjectID != tr.dead || len(archive.EvidenceAnalyses) != 1 ||
		archive.EvidenceAnalyses[0].SubjectID != tr.dead || len(archive.ProofSummaries) != 0 {
		t.Errorf("archive = %+v, want only the dead person's research", archive)
	}
}

// A person the read model does not hold is named as the placeholder when
// unresolved people are protected, else from their folded stream.
func TestHistoryDescription_UnresolvedPersonPlaceholder(t *testing.T) {
	id := uuid.New()
	state := newStreamState()
	state.created = true
	state.fields["given_name"], state.fields["surname"] = "Lena", "Gone"
	for _, protect := range []bool{false, true} {
		d := &historyDescription{names: &entityNames{}, states: map[uuid.UUID]*streamState{id: state}, placeholderUnresolved: protect}
		want := "Lena Gone"
		if protect {
			want = repository.PlaceholderGivenName
		}
		if got := d.personLabel(id); got != want {
			t.Errorf("protect %v: personLabel = %q, want %q", protect, got, want)
		}
		if got := d.stateLabel(entityTypePerson, id); got != want {
			t.Errorf("protect %v: stateLabel = %q, want %q", protect, got, want)
		}
	}
}

// A protected record's history is empty, total included; a shown one's keeps
// its entries and total.
func TestChangeRedactor_EntityHistory(t *testing.T) {
	tr := newRedactorTree(t)
	ctx := strictViewerCtx(false)
	for id, want := range map[uuid.UUID]int{tr.dead: 1, tr.living: 0} {
		result := &ChangeHistoryResult{Entries: []ChangeEntry{{EntityType: entityTypePerson, EntityID: id}}, TotalCount: 1, Limit: 20}
		got, err := tr.redactor.entityHistory(ctx, mainScope, entityTypePerson, id, result)
		if err != nil || len(got.Entries) != want || got.TotalCount != want || got.Limit != 20 {
			t.Errorf("history of %s = %+v, %v; want %d entries", id, got, err, want)
		}
	}
}
