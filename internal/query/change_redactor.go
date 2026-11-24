package query

import (
	"context"
	"maps"
	"slices"

	"github.com/google/uuid"

	"github.com/cacack/my-family/internal/domain"
	"github.com/cacack/my-family/internal/repository"
)

// changeRedactor is the event-output choke point of ADR-008 §5b. Every query
// service that reads the event log runs what it returns through it before the
// result leaves the query layer: change entries (history, snapshot and branch
// comparisons), merge conflicts and merge records (the merge review), restore
// points, the changed facts of the evidence-coverage warning and the research
// archive. For the principal on the context it
//
//   - drops what is about a record that is protected or private for them, or
//     whose parent record is (a life event's person, a citation's source),
//     and what is about a record that no longer resolves (ADR-008 §3);
//   - names a person who does not resolve as the placeholder, as the read
//     model names a protected one (see historyDescription.placeholderUnresolved);
//   - removes who made each change for an anonymous principal (ADR-008 §4).
//
// It applies the policy through the privacy decorator, so the two choke points
// share one rule. A service that reads through the decorator redacts; one
// wired to the real store (command handlers, the CLI, the merge and rollback
// commands' own reads) does not, since those run for editors and owners, who
// see everything. A principal who sees everything gets results unchanged.
type changeRedactor struct {
	store *PrivacyStore
}

// redactorFor returns the redactor of a service that reads readStore: active
// when readStore is the privacy decorator, else one that changes nothing.
// Wrapping the decorator in another store therefore turns redaction off; the
// server's wiring is pinned by TestPrivacyRedaction_PersonSurfaces, whose
// protected person's history and restore points must come back empty.
func redactorFor(readStore repository.ReadModelStore) changeRedactor {
	store, _ := readStore.(*PrivacyStore)
	return changeRedactor{store: store}
}

// privacy is what the principal on ctx may not see, and whether anything is
// to be redacted for them.
func (r changeRedactor) privacy(ctx context.Context) (repository.Privacy, bool) {
	if r.store == nil {
		return repository.Privacy{}, false
	}
	v := r.store.privacy(ctx)
	return v, !v.SeesAll()
}

// hidesRecords reports whether v hides any tree record, so a record that does
// not resolve is protected too.
func hidesRecords(v repository.Privacy) bool {
	return v.HidePrivate || v.HideLiving
}

// placeholderUnresolved reports whether a person the read model does not
// hold is named as the placeholder for the principal on ctx.
func (r changeRedactor) placeholderUnresolved(ctx context.Context) bool {
	v, active := r.privacy(ctx)
	return active && hidesRecords(v)
}

// shownCheck answers, once per record, whether a principal may see a record
// in every one of scopes (see shown).
type shownCheck struct {
	store  *PrivacyStore
	v      repository.Privacy
	scopes []domain.BranchID
	memo   map[shownKey]bool
}

type shownKey struct {
	entityType string
	id         uuid.UUID
}

func (r changeRedactor) check(v repository.Privacy, scopes []domain.BranchID) *shownCheck {
	return &shownCheck{store: r.store, v: v, scopes: scopes, memo: map[shownKey]bool{}}
}

// shown reports whether the record entityType and id names resolves in at
// least one scope and is shown in each scope it resolves in: a comparison
// treats a record as protected if either side does (ADR-008, Consequences).
func (c *shownCheck) shown(ctx context.Context, entityType string, id uuid.UUID) (bool, error) {
	key := shownKey{entityType, id}
	if shown, ok := c.memo[key]; ok {
		return shown, nil
	}
	found := false
	shown := true
	for _, scope := range c.scopes {
		inScope, shownInScope, err := c.store.recordShown(ctx, c.v, scope, entityType, id)
		if err != nil {
			return false, err
		}
		found = found || inScope
		shown = shown && (!inScope || shownInScope)
	}
	c.memo[key] = found && shown
	return c.memo[key], nil
}

// subjectShown is shown for the subject of a GPS record or changed fact: a
// person or a family, or either when subjectType does not say.
func (c *shownCheck) subjectShown(ctx context.Context, subjectType string, id uuid.UUID) (bool, error) {
	if subjectType == entityTypePerson || subjectType == entityTypeFamily {
		return c.shown(ctx, subjectType, id)
	}
	for _, typ := range []string{entityTypePerson, entityTypeFamily} {
		if shown, err := c.shown(ctx, typ, id); err != nil || shown {
			return shown, err
		}
	}
	return false, nil
}

// recordShown reports whether the record entityType and id names resolves in
// branchID (found) and whether v may see it there (shown): a person in full,
// a family with its own facts (no protected partner), and any other record
// as the privacy decorator returns it. A record type the history does not
// know is never shown.
func (s *PrivacyStore) recordShown(ctx context.Context, v repository.Privacy, branchID domain.BranchID, entityType string, id uuid.UUID) (found, shown bool, err error) {
	switch entityType {
	case entityTypeBranch:
		// A branch is not tree data.
		return true, true, nil
	case entityTypePerson:
		p, err := s.inner.GetPerson(ctx, branchID, id)
		if err != nil || p == nil {
			return false, false, err
		}
		return true, v.Person(p) == repository.Visible, nil
	case entityTypeFamily:
		f, err := s.inner.GetFamily(ctx, branchID, id)
		if err != nil || f == nil {
			return false, false, err
		}
		shown, err := s.familyShown(ctx, v, branchID, id)
		return true, shown, err
	}
	get, ok := recordGetters[entityType]
	if !ok {
		return false, false, nil
	}
	if found, err = get(ctx, s.inner, branchID, id); err != nil || !found {
		return false, false, err
	}
	shown, err = get(ctx, s, branchID, id)
	return true, shown, err
}

// recordGetters report whether a store returns the record of each history
// entity type other than people, families and branches. Read through the
// real store they say whether the record exists; through the decorator,
// whether the principal may see it.
var recordGetters = map[string]func(context.Context, repository.ReadModelStore, domain.BranchID, uuid.UUID) (bool, error){
	entityTypeSource: func(ctx context.Context, s repository.ReadModelStore, b domain.BranchID, id uuid.UUID) (bool, error) {
		return returned(s.GetSource(ctx, b, id))
	},
	entityTypeCitation: func(ctx context.Context, s repository.ReadModelStore, b domain.BranchID, id uuid.UUID) (bool, error) {
		return returned(s.GetCitation(ctx, b, id))
	},
	entityTypeMedia: func(ctx context.Context, s repository.ReadModelStore, b domain.BranchID, id uuid.UUID) (bool, error) {
		return returned(s.GetMedia(ctx, b, id))
	},
	entityTypeNote: func(ctx context.Context, s repository.ReadModelStore, b domain.BranchID, id uuid.UUID) (bool, error) {
		return returned(s.GetNote(ctx, b, id))
	},
	entityTypeSubmitter: func(ctx context.Context, s repository.ReadModelStore, _ domain.BranchID, id uuid.UUID) (bool, error) {
		return returned(s.GetSubmitter(ctx, id))
	},
	entityTypeRepository: func(ctx context.Context, s repository.ReadModelStore, _ domain.BranchID, id uuid.UUID) (bool, error) {
		return returned(s.GetRepository(ctx, id))
	},
	entityTypeAssociation: func(ctx context.Context, s repository.ReadModelStore, b domain.BranchID, id uuid.UUID) (bool, error) {
		return returned(s.GetAssociation(ctx, b, id))
	},
	entityTypeLifeEvent: func(ctx context.Context, s repository.ReadModelStore, b domain.BranchID, id uuid.UUID) (bool, error) {
		return returned(s.GetEvent(ctx, b, id))
	},
	entityTypeAttribute: func(ctx context.Context, s repository.ReadModelStore, b domain.BranchID, id uuid.UUID) (bool, error) {
		return returned(s.GetAttribute(ctx, b, id))
	},
	entityTypeLDSOrdinance: func(ctx context.Context, s repository.ReadModelStore, _ domain.BranchID, id uuid.UUID) (bool, error) {
		return returned(s.GetLDSOrdinance(ctx, id))
	},
	entityTypeEvidenceAnalysis: func(ctx context.Context, s repository.ReadModelStore, b domain.BranchID, id uuid.UUID) (bool, error) {
		return returned(s.GetEvidenceAnalysis(ctx, b, id))
	},
	entityTypeEvidenceConflict: func(ctx context.Context, s repository.ReadModelStore, b domain.BranchID, id uuid.UUID) (bool, error) {
		return returned(s.GetEvidenceConflict(ctx, b, id))
	},
	entityTypeResearchLog: func(ctx context.Context, s repository.ReadModelStore, b domain.BranchID, id uuid.UUID) (bool, error) {
		return returned(s.GetResearchLog(ctx, b, id))
	},
	entityTypeProofSummary: func(ctx context.Context, s repository.ReadModelStore, b domain.BranchID, id uuid.UUID) (bool, error) {
		return returned(s.GetProofSummary(ctx, b, id))
	},
	entityTypeDuplicatePair: func(ctx context.Context, s repository.ReadModelStore, b domain.BranchID, id uuid.UUID) (bool, error) {
		return returned(s.GetDuplicateDismissal(ctx, b, id))
	},
	// A place resolves while a record names it or a fact about it is stated,
	// and is shown when a record the principal may see names it (ADR-009
	// §10, PL-006): ListPlaces holds exactly those for each store.
	entityTypePlace: func(ctx context.Context, s repository.ReadModelStore, b domain.BranchID, id uuid.UUID) (bool, error) {
		places, err := s.ListPlaces(ctx, b, repository.Privacy{})
		if err != nil {
			return false, err
		}
		return slices.ContainsFunc(places, func(p repository.PlaceReadModel) bool { return p.ID == id }), nil
	},
}

// aliasTargetShown reports whether the place a place alias entry names
// beside its own place is shown too (ADR-009 §10, PL-006): the alias's
// change holds that place's recorded spelling, whose key is the place's key.
// An entry with no alias change is shown.
func aliasTargetShown(ctx context.Context, check *shownCheck, entry ChangeEntry) (bool, error) {
	change, ok := entry.Changes[placeAliasField]
	if !ok || entry.EntityType != entityTypePlace {
		return true, nil
	}
	for _, value := range []any{change.OldValue, change.NewValue} {
		text, ok := value.(string)
		if !ok || repository.PlaceKey(text) == "" {
			continue
		}
		if shown, err := check.shown(ctx, entityTypePlace, repository.PlaceID(repository.PlaceKey(text))); err != nil || !shown {
			return false, err
		}
	}
	return true, nil
}

func returned[T any](row *T, err error) (bool, error) {
	return row != nil, err
}

// mergedPersonField is the change of a person merge that names the person
// merged away, who no longer resolves.
const mergedPersonField = "merged_person"

// entries redacts change entries as scopes see them (see changeRedactor).
func (r changeRedactor) entries(ctx context.Context, scopes []domain.BranchID, entries []ChangeEntry) ([]ChangeEntry, error) {
	v, active := r.privacy(ctx)
	if !active {
		return entries, nil
	}
	check := r.check(v, scopes)
	out := make([]ChangeEntry, 0, len(entries))
	for _, entry := range entries {
		if v.HideAttribution {
			entry.UserID, entry.UserName = nil, nil
		}
		if hidesRecords(v) {
			shown, err := check.shown(ctx, entry.EntityType, entry.EntityID)
			if err == nil && shown && entry.ParentEntityID != nil {
				shown, err = check.shown(ctx, entry.ParentEntityType, *entry.ParentEntityID)
			}
			if err == nil && shown {
				shown, err = aliasTargetShown(ctx, check, entry)
			}
			if err != nil {
				return nil, err
			}
			if !shown {
				continue
			}
			if _, ok := entry.Changes[mergedPersonField]; ok {
				entry.Changes = maps.Clone(entry.Changes)
				entry.Changes[mergedPersonField] = FieldChange{NewValue: repository.PlaceholderGivenName}
			}
		}
		out = append(out, entry)
	}
	return out, nil
}

// history redacts a page of change entries. Entries dropped from the page
// leave its total as the store counted it: the redactor works on finished
// entries and cannot exclude rows from a paged read (ADR-008 §5).
func (r changeRedactor) history(ctx context.Context, scopes []domain.BranchID, result *ChangeHistoryResult) (*ChangeHistoryResult, error) {
	entries, err := r.entries(ctx, scopes, result.Entries)
	if err != nil {
		return nil, err
	}
	result.Entries = entries
	return result, nil
}

// entityHistory redacts one record's history: none at all, total included,
// when the principal may not see the record, else its entries (history).
func (r changeRedactor) entityHistory(ctx context.Context, scopes []domain.BranchID, entityType string, entityID uuid.UUID, result *ChangeHistoryResult) (*ChangeHistoryResult, error) {
	if v, active := r.privacy(ctx); active && hidesRecords(v) {
		shown, err := r.check(v, scopes).shown(ctx, entityType, entityID)
		if err != nil {
			return nil, err
		}
		if !shown {
			return &ChangeHistoryResult{Entries: []ChangeEntry{}, Limit: result.Limit, Offset: result.Offset}, nil
		}
	}
	return r.history(ctx, scopes, result)
}

// conflicts drops the merge conflicts about records the principal may not see.
func (r changeRedactor) conflicts(ctx context.Context, scopes []domain.BranchID, conflicts []MergeConflict) ([]MergeConflict, error) {
	v, active := r.privacy(ctx)
	if !active || !hidesRecords(v) {
		return conflicts, nil
	}
	check := r.check(v, scopes)
	out := make([]MergeConflict, 0, len(conflicts))
	for _, c := range conflicts {
		shown, err := check.shown(ctx, c.EntityType, c.StreamID)
		if err != nil {
			return nil, err
		}
		if shown {
			out = append(out, c)
		}
	}
	return out, nil
}

// mergeRecord drops the decisions and exclusions about records the principal
// may not see, and who merged the branch from an anonymous principal.
func (r changeRedactor) mergeRecord(ctx context.Context, scopes []domain.BranchID, record *MergeRecord) error {
	v, active := r.privacy(ctx)
	if !active || record == nil {
		return nil
	}
	if v.HideAttribution {
		record.MergedBy, record.MergedByName = "", ""
	}
	if !hidesRecords(v) {
		return nil
	}
	check := r.check(v, scopes)
	decisions := make([]MergeRecordDecision, 0, len(record.Decisions))
	for _, d := range record.Decisions {
		shown, err := check.shown(ctx, d.EntityType, d.StreamID)
		if err != nil {
			return err
		}
		if shown {
			decisions = append(decisions, d)
		}
	}
	exclusions := make([]MergeRecordExclusion, 0, len(record.Exclusions))
	for _, e := range record.Exclusions {
		shown, err := check.shown(ctx, e.EntityType, e.StreamID)
		if err != nil {
			return err
		}
		if shown {
			exclusions = append(exclusions, e)
		}
	}
	record.Decisions, record.Exclusions = decisions, exclusions
	return nil
}

// streams keeps the stream IDs (a comparison's overlap hint, a merge's
// skipped streams) that one of shown, the redacted change entries, is about:
// a bare ID still says the record changed. Nothing is dropped for a
// principal who sees every record.
func (r changeRedactor) streams(ctx context.Context, ids []uuid.UUID, shown ...[]ChangeEntry) []uuid.UUID {
	if v, active := r.privacy(ctx); !active || !hidesRecords(v) || ids == nil {
		return ids
	}
	kept := map[uuid.UUID]bool{}
	for _, entries := range shown {
		for i := range entries {
			kept[entries[i].EntityID] = true
		}
	}
	out := make([]uuid.UUID, 0, len(ids))
	for _, id := range ids {
		if kept[id] {
			out = append(out, id)
		}
	}
	return out
}

// restorePoints empties the restore points of a record the principal may not
// see on the mainline, where restore points live (ADR-005).
func (r changeRedactor) restorePoints(ctx context.Context, entityType string, entityID uuid.UUID, result *RestorePointsResult) (*RestorePointsResult, error) {
	v, active := r.privacy(ctx)
	if !active || !hidesRecords(v) {
		return result, nil
	}
	shown, err := r.check(v, []domain.BranchID{domain.MainBranchID}).shown(ctx, entityType, entityID)
	if err != nil || shown {
		return result, err
	}
	return &RestorePointsResult{RestorePoints: []RestorePoint{}, Limit: result.Limit, Offset: result.Offset}, nil
}

// changedFacts drops the changed facts whose subject the principal may not see.
func (r changeRedactor) changedFacts(ctx context.Context, scopes []domain.BranchID, facts []ChangedFact) ([]ChangedFact, error) {
	v, active := r.privacy(ctx)
	if !active || !hidesRecords(v) {
		return facts, nil
	}
	check := r.check(v, scopes)
	out := make([]ChangedFact, 0, len(facts))
	for _, f := range facts {
		shown, err := check.subjectShown(ctx, f.SubjectType, f.SubjectID)
		if err != nil {
			return nil, err
		}
		if shown {
			out = append(out, f)
		}
	}
	return out, nil
}

// archive drops the archived research whose subject the principal may not
// see. The branch's overlay may have been purged, so a subject only it held
// does not resolve and is treated as protected (ADR-008 read-surface table).
func (r changeRedactor) archive(ctx context.Context, scopes []domain.BranchID, archive *BranchResearchArchive) error {
	v, active := r.privacy(ctx)
	if !active || !hidesRecords(v) {
		return nil
	}
	check := r.check(v, scopes)
	var err error
	if archive.ResearchLogs, err = keepSubjects(ctx, check, archive.ResearchLogs, func(l *ArchivedResearchLog) (string, uuid.UUID) {
		return l.SubjectType, l.SubjectID
	}); err != nil {
		return err
	}
	if archive.EvidenceAnalyses, err = keepSubjects(ctx, check, archive.EvidenceAnalyses, func(a *ArchivedEvidenceAnalysis) (string, uuid.UUID) {
		return "", a.SubjectID
	}); err != nil {
		return err
	}
	archive.ProofSummaries, err = keepSubjects(ctx, check, archive.ProofSummaries, func(p *ArchivedProofSummary) (string, uuid.UUID) {
		return "", p.SubjectID
	})
	return err
}

func keepSubjects[T any](ctx context.Context, check *shownCheck, rows []T, subject func(*T) (string, uuid.UUID)) ([]T, error) {
	out := make([]T, 0, len(rows))
	for i := range rows {
		typ, id := subject(&rows[i])
		shown, err := check.subjectShown(ctx, typ, id)
		if err != nil {
			return nil, err
		}
		if shown {
			out = append(out, rows[i])
		}
	}
	return out, nil
}

// branchScopes are the scopes a branch comparison checks a record in: the
// branch's and the mainline's.
func branchScopes(branchID uuid.UUID) []domain.BranchID {
	return []domain.BranchID{domain.BranchID(branchID), domain.MainBranchID}
}

// redactor is the redactor of the history service the branch service
// describes events through; one that changes nothing without it.
func (s *BranchService) redactor() changeRedactor {
	if s.historyService == nil {
		return changeRedactor{}
	}
	return s.historyService.redactor
}
