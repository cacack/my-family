package query

import "sort"

// The authoritative event-type table for every change-log view (#739, #827):
// global history, entity history (mainline and branch-scoped), branch compare
// and merge review, and snapshot compare.
//
// Every event type the event store can hold (the cases of
// repository.StoredEvent.DecodeEvent) is listed here exactly once, either
//
//   - MAPPED to the entity type and action a ChangeEntry reports for it, or
//   - EXCLUDED, with the reason it is not a change to anyone's family tree.
//
// There is no third outcome. The table is used both to render an event and to
// filter the global history AT THE STORE (GetGlobalHistory passes the excluded
// types to repository.EventStore.ReadGlobalHistory), so a page and its total
// are computed over the same set of events. TestHistoryCatalog_CoversEveryEventType
// fails the build of any new event type that is not classified here, which is
// what keeps "unknown" out of the ChangeEntry contract.
//
// docs/HISTORY-EVENT-TYPES.md records the same decisions for readers.

// ChangeEntry.EntityType vocabulary. It matches the OpenAPI ChangeEntry
// entity_type enum and MergeConflict.entity_type.
const (
	entityTypePerson           = "person"
	entityTypeFamily           = "family"
	entityTypeSource           = "source"
	entityTypeCitation         = "citation"
	entityTypeMedia            = "media"
	entityTypeNote             = "note"
	entityTypeSubmitter        = "submitter"
	entityTypeRepository       = "repository"
	entityTypeAssociation      = "association"
	entityTypeLifeEvent        = "life_event"
	entityTypeAttribute        = "attribute"
	entityTypeLDSOrdinance     = "lds_ordinance"
	entityTypeEvidenceAnalysis = "evidence_analysis"
	entityTypeEvidenceConflict = "evidence_conflict"
	entityTypeResearchLog      = "research_log"
	entityTypeProofSummary     = "proof_summary"
	// entityTypeBranch is a research branch's lifecycle (#832): created,
	// merged into the mainline, deleted (archived). Only the global history
	// shows it; its events live on each branch's own scope, and the store
	// keeps them in the mainline's page (HistoryBranchLifecycleEventTypes).
	entityTypeBranch = "branch"
)

// ChangeEntry.Action vocabulary.
const (
	actionCreated = "created"
	actionUpdated = "updated"
	actionDeleted = "deleted"
	// actionMerged is PersonMerged on the survivor's stream: another person was
	// folded into this one.
	actionMerged = "merged"
)

// historyEventClass is one row of the table: either EntityType and Action are
// set (mapped), or ExcludeReason is (excluded).
type historyEventClass struct {
	EntityType    string
	Action        string
	ExcludeReason string
}

// Excluded reports whether the event is deliberately left out of every
// change-log view.
func (c historyEventClass) Excluded() bool { return c.ExcludeReason != "" }

func mapped(entityType, action string) historyEventClass {
	return historyEventClass{EntityType: entityType, Action: action}
}

func excluded(reason string) historyEventClass {
	return historyEventClass{ExcludeReason: reason}
}

const (
	reasonImport   = "import audit record: the persons, families and sources it created each have their own events"
	reasonSnapshot = "research-artifact marker (#624): a snapshot names a position in the log, it changes no genealogical data"
	reasonResume   = "merge resume record (#685): part of the merge it finishes, which the history shows once as BranchMerged; its decisions are on the branch's merge record"
)

// historyEventCatalog is the table itself. Keep it in the order of
// DecodeEvent's switch so the two are easy to compare.
var historyEventCatalog = map[string]historyEventClass{
	"PersonCreated":           mapped(entityTypePerson, actionCreated),
	"PersonUpdated":           mapped(entityTypePerson, actionUpdated),
	"PersonDeleted":           mapped(entityTypePerson, actionDeleted),
	"FamilyCreated":           mapped(entityTypeFamily, actionCreated),
	"FamilyUpdated":           mapped(entityTypeFamily, actionUpdated),
	"ChildLinkedToFamily":     mapped(entityTypeFamily, actionUpdated),
	"ChildUnlinkedFromFamily": mapped(entityTypeFamily, actionUpdated),
	"FamilyDeleted":           mapped(entityTypeFamily, actionDeleted),
	"GedcomImported":          excluded(reasonImport),
	"SourceCreated":           mapped(entityTypeSource, actionCreated),
	"SourceUpdated":           mapped(entityTypeSource, actionUpdated),
	"SourceDeleted":           mapped(entityTypeSource, actionDeleted),
	"CitationCreated":         mapped(entityTypeCitation, actionCreated),
	"CitationUpdated":         mapped(entityTypeCitation, actionUpdated),
	"CitationDeleted":         mapped(entityTypeCitation, actionDeleted),
	"MediaCreated":            mapped(entityTypeMedia, actionCreated),
	"MediaUpdated":            mapped(entityTypeMedia, actionUpdated),
	"MediaDeleted":            mapped(entityTypeMedia, actionDeleted),
	// A person's names live on the person's stream, so a name change is an
	// update of the person; the change map carries the name itself.
	"NameAdded":       mapped(entityTypePerson, actionUpdated),
	"NameUpdated":     mapped(entityTypePerson, actionUpdated),
	"NameRemoved":     mapped(entityTypePerson, actionUpdated),
	"SnapshotCreated": excluded(reasonSnapshot),
	"SnapshotDeleted": excluded(reasonSnapshot),
	// The branch lifecycle (#832): mapped so the mainline's history shows
	// when research branched off and when (and why) it came back. The
	// replayed changes of a merge still appear as their own entries.
	"BranchCreated":            mapped(entityTypeBranch, actionCreated),
	"BranchDeleted":            mapped(entityTypeBranch, actionDeleted),
	"BranchMerged":             mapped(entityTypeBranch, actionMerged),
	"BranchMergeResumed":       excluded(reasonResume),
	"PersonMerged":             mapped(entityTypePerson, actionMerged),
	"NoteCreated":              mapped(entityTypeNote, actionCreated),
	"NoteUpdated":              mapped(entityTypeNote, actionUpdated),
	"NoteDeleted":              mapped(entityTypeNote, actionDeleted),
	"SubmitterCreated":         mapped(entityTypeSubmitter, actionCreated),
	"SubmitterUpdated":         mapped(entityTypeSubmitter, actionUpdated),
	"SubmitterDeleted":         mapped(entityTypeSubmitter, actionDeleted),
	"AssociationCreated":       mapped(entityTypeAssociation, actionCreated),
	"AssociationUpdated":       mapped(entityTypeAssociation, actionUpdated),
	"AssociationDeleted":       mapped(entityTypeAssociation, actionDeleted),
	"LDSOrdinanceCreated":      mapped(entityTypeLDSOrdinance, actionCreated),
	"LDSOrdinanceUpdated":      mapped(entityTypeLDSOrdinance, actionUpdated),
	"LDSOrdinanceDeleted":      mapped(entityTypeLDSOrdinance, actionDeleted),
	"RepositoryCreated":        mapped(entityTypeRepository, actionCreated),
	"RepositoryUpdated":        mapped(entityTypeRepository, actionUpdated),
	"RepositoryDeleted":        mapped(entityTypeRepository, actionDeleted),
	"LifeEventCreated":         mapped(entityTypeLifeEvent, actionCreated),
	"LifeEventUpdated":         mapped(entityTypeLifeEvent, actionUpdated),
	"LifeEventDeleted":         mapped(entityTypeLifeEvent, actionDeleted),
	"AttributeCreated":         mapped(entityTypeAttribute, actionCreated),
	"AttributeUpdated":         mapped(entityTypeAttribute, actionUpdated),
	"AttributeDeleted":         mapped(entityTypeAttribute, actionDeleted),
	"EvidenceAnalysisCreated":  mapped(entityTypeEvidenceAnalysis, actionCreated),
	"EvidenceAnalysisUpdated":  mapped(entityTypeEvidenceAnalysis, actionUpdated),
	"EvidenceAnalysisDeleted":  mapped(entityTypeEvidenceAnalysis, actionDeleted),
	"EvidenceConflictDetected": mapped(entityTypeEvidenceConflict, actionCreated),
	"EvidenceConflictResolved": mapped(entityTypeEvidenceConflict, actionUpdated),
	"ResearchLogCreated":       mapped(entityTypeResearchLog, actionCreated),
	"ResearchLogUpdated":       mapped(entityTypeResearchLog, actionUpdated),
	"ResearchLogDeleted":       mapped(entityTypeResearchLog, actionDeleted),
	"ProofSummaryCreated":      mapped(entityTypeProofSummary, actionCreated),
	"ProofSummaryUpdated":      mapped(entityTypeProofSummary, actionUpdated),
	"ProofSummaryDeleted":      mapped(entityTypeProofSummary, actionDeleted),
}

// classifyHistoryEvent looks an event type up in the table. ok is false only
// for a type the table does not know, which the coverage test rules out for
// every type the store can decode.
func classifyHistoryEvent(eventType string) (class historyEventClass, ok bool) {
	class, ok = historyEventCatalog[eventType]
	return class, ok
}

// HistoryExcludedEventTypes returns the event types no change-log view shows,
// sorted. The global history filters them out in the store, before pagination.
func HistoryExcludedEventTypes() []string {
	var types []string
	for eventType, class := range historyEventCatalog {
		if class.Excluded() {
			types = append(types, eventType)
		}
	}
	sort.Strings(types)
	return types
}

// HistoryEventTypesForEntity returns, sorted, the event types whose change
// entries report entityType — the store-level filter behind the global
// history's entity_type parameter. An entity type the table does not use
// returns nil.
func HistoryEventTypesForEntity(entityType string) []string {
	var types []string
	for eventType, class := range historyEventCatalog {
		if !class.Excluded() && class.EntityType == entityType {
			types = append(types, eventType)
		}
	}
	sort.Strings(types)
	return types
}

// HistoryBranchLifecycleEventTypes returns, sorted, the mapped event types
// of a research branch's lifecycle. They are written on each branch's own
// scope, so the mainline's global history asks the store to keep them from
// every branch (GlobalHistoryQuery.AnyBranchEventTypes).
func HistoryBranchLifecycleEventTypes() []string {
	return HistoryEventTypesForEntity(entityTypeBranch)
}

// HistoryEntityTypes returns every entity type a ChangeEntry can report,
// sorted.
func HistoryEntityTypes() []string {
	seen := map[string]bool{}
	var types []string
	for _, class := range historyEventCatalog {
		if !class.Excluded() && !seen[class.EntityType] {
			seen[class.EntityType] = true
			types = append(types, class.EntityType)
		}
	}
	sort.Strings(types)
	return types
}
