package query

import (
	"context"
	"encoding/json"
	"fmt"
	"math"
	"reflect"
	"sort"
	"strings"
	"unicode/utf8"

	"github.com/google/uuid"

	"github.com/cacack/my-family/internal/domain"
	"github.com/cacack/my-family/internal/repository"
)

// Describing a batch of change entries (#827, #739).
//
// A change entry needs three things its own event does not carry: a display
// name for the entity, the values an update REPLACED (old_value), and for a
// sub-record (a life event, a citation, a media item) the entity whose page
// shows it. All three come from the same place — the state of the entity's
// stream just before (and after) the event — so a batch is described in one
// pass:
//
//  1. The streams the batch touches are read with ONE set-based event-store
//     read on main (ReadStreamsForBranch) and, on a branch, one more for the
//     branch's own events — never one read per entry.
//  2. Each stream's events, filtered to what the scope actually sees
//     (branchVisibleStreamEvents), are folded in position order. Every
//     batched event's field changes are taken against the state folded so
//     far, which is exactly the prior value; the final fold is the entity's
//     current state in the scope, which names it.
//  3. Person/family/source/citation names still come from the read model in
//     one batched lookup per type (resolveEntityNamesOn), including the
//     people an association or a merge refers to.
//
// The read-model query count therefore stays bounded by the number of entity
// types, and the event-store read count by two, whatever the batch size.

// maxHistoryStateEvents caps each event-store read that rebuilds the state of
// a batch's streams. A batch whose streams hold more events than this still
// renders: entries past the last event read simply carry no old_value, never
// a wrong one. A variable only so tests can reach the cap cheaply.
var maxHistoryStateEvents = 20000

// noteExcerptLength is how many characters of free text (a note, a
// conclusion, a search description) a display name keeps.
const noteExcerptLength = 60

// streamState is the folded state of one stream as a scope sees it.
type streamState struct {
	// fields are the entity's current field values, keyed by the JSON field
	// names of its Created event (which its Updated events' change keys share),
	// dates collapsed to their raw text.
	fields map[string]any
	// names are a person's name variants by name id (NameAdded/NameUpdated).
	names map[string]map[string]any
	// created reports that the entity's creation event was folded, so its
	// fields describe the whole entity rather than a few updated ones.
	created bool
	// mergedName is the display name of a person PersonMerged folded into
	// this one, from the event's snapshot.
	mergedName string
}

func newStreamState() *streamState {
	return &streamState{fields: map[string]any{}, names: map[string]map[string]any{}}
}

func (st *streamState) str(field string) string {
	if st == nil {
		return ""
	}
	if v, ok := st.fields[field].(string); ok {
		return v
	}
	return ""
}

func (st *streamState) id(field string) (uuid.UUID, bool) {
	id, err := uuid.Parse(st.str(field))
	if err != nil || id == uuid.Nil {
		return uuid.Nil, false
	}
	return id, true
}

// historyDescription is everything describeEvents resolved for a batch.
type historyDescription struct {
	names   *entityNames
	states  map[uuid.UUID]*streamState
	changes map[uuid.UUID]map[string]FieldChange // by event id
}

// describeEvents resolves names, field changes and owners for a batch of
// events as branchID sees them. See the comment at the top of this file.
func (s *HistoryService) describeEvents(ctx context.Context, branchID domain.BranchID, events []repository.StoredEvent) (*historyDescription, error) {
	streams, reliableUpTo, err := s.readBatchStreams(ctx, branchID, events)
	if err != nil {
		return nil, err
	}

	batch := make(map[uuid.UUID]bool, len(events))
	for i := range events {
		batch[events[i].ID] = true
	}

	desc := &historyDescription{
		states:  make(map[uuid.UUID]*streamState, len(streams)),
		changes: make(map[uuid.UUID]map[string]FieldChange),
	}
	for streamID, streamEvents := range streams {
		state := newStreamState()
		for i := range streamEvents {
			evt := &streamEvents[i]
			if batch[evt.ID] {
				desc.changes[evt.ID] = fieldChanges(evt, state, evt.Position <= reliableUpTo)
			}
			state.apply(evt)
		}
		desc.states[streamID] = state
	}

	refs := newEntityRefs()
	for i := range events {
		class, ok := classifyHistoryEvent(events[i].EventType)
		if !ok || class.Excluded() {
			continue
		}
		refs.addEvent(class.EntityType, events[i].StreamID, &events[i])
		desc.registerRelated(refs, class.EntityType, events[i].StreamID)
	}
	if desc.names, err = s.resolveEntityNamesOn(ctx, branchID, refs); err != nil {
		return nil, err
	}
	return desc, nil
}

// readBatchStreams reads every event of the streams the batch touches, as
// branchID sees them, grouped by stream and ordered by position. The batch's
// own events are always included, so a batch still describes itself when the
// store read comes back short. reliableUpTo is the position up to which the
// reads are complete: a read that hit its cap may be missing later events.
func (s *HistoryService) readBatchStreams(ctx context.Context, branchID domain.BranchID, events []repository.StoredEvent) (map[uuid.UUID][]repository.StoredEvent, int64, error) {
	var streamIDs []uuid.UUID
	seen := make(map[uuid.UUID]bool)
	for i := range events {
		if !seen[events[i].StreamID] {
			seen[events[i].StreamID] = true
			streamIDs = append(streamIDs, events[i].StreamID)
		}
	}
	if len(streamIDs) == 0 {
		return nil, math.MaxInt64, nil
	}

	reliableUpTo := int64(math.MaxInt64)
	read := func(scope domain.BranchID) ([]repository.StoredEvent, error) {
		got, err := s.eventStore.ReadStreamsForBranch(ctx, streamIDs, scope, 0, maxHistoryStateEvents)
		if err != nil {
			return nil, fmt.Errorf("read stream state: %w", err)
		}
		if len(got) >= maxHistoryStateEvents {
			reliableUpTo = min(reliableUpTo, got[len(got)-1].Position)
		}
		return got, nil
	}
	all, err := read(domain.MainBranchID)
	if err != nil {
		return nil, 0, err
	}
	if !branchID.IsMain() {
		own, err := read(branchID)
		if err != nil {
			return nil, 0, err
		}
		all = append(all, own...)
	}

	byID := make(map[uuid.UUID]bool, len(all))
	for i := range all {
		byID[all[i].ID] = true
	}
	for i := range events {
		if !byID[events[i].ID] {
			byID[events[i].ID] = true
			all = append(all, events[i])
		}
	}

	grouped := make(map[uuid.UUID][]repository.StoredEvent, len(streamIDs))
	for i := range all {
		grouped[all[i].StreamID] = append(grouped[all[i].StreamID], all[i])
	}
	for streamID, streamEvents := range grouped {
		grouped[streamID] = branchVisibleStreamEvents(streamEvents, branchID)
	}
	return grouped, reliableUpTo, nil
}

// registerRelated registers the other entities a sub-record's display name
// refers to, so the single batched read-model lookup covers them.
func (d *historyDescription) registerRelated(refs entityRefs, entityType string, streamID uuid.UUID) {
	state := d.states[streamID]
	if state == nil {
		return
	}
	switch entityType {
	case entityTypeAssociation:
		for _, field := range []string{"person_id", "associate_id"} {
			if id, ok := state.id(field); ok {
				refs.add(entityTypePerson, id)
			}
		}
	case entityTypeCitation:
		if id, ok := state.id("source_id"); ok {
			refs.add(entityTypeSource, id)
		}
	case entityTypeFamily:
		for _, field := range []string{"partner1_id", "partner2_id"} {
			if id, ok := state.id(field); ok {
				refs.add(entityTypePerson, id)
			}
		}
	}
}

// eventFields decodes an event's payload into a generic field map with the
// envelope and binary fields dropped and dates collapsed to their raw text.
func eventFields(evt *repository.StoredEvent) map[string]any {
	var fields map[string]any
	if err := json.Unmarshal(evt.Data, &fields); err != nil || fields == nil {
		return map[string]any{}
	}
	for _, key := range []string{"id", "timestamp", "file_data", "thumbnail_data"} {
		delete(fields, key)
	}
	for key, value := range fields {
		fields[key] = normalizeFieldValue(value)
	}
	return fields
}

// normalizeFieldValue collapses a GenDate (an object with a "raw" text) to
// that text, the form an update's change map carries dates in.
func normalizeFieldValue(value any) any {
	if m, ok := value.(map[string]any); ok {
		if raw, ok := m["raw"].(string); ok {
			return raw
		}
	}
	return value
}

// apply folds one event into the state.
func (st *streamState) apply(evt *repository.StoredEvent) {
	switch evt.EventType {
	case "NameAdded", "NameUpdated":
		fields := eventFields(evt)
		if nameID, ok := fields["name_id"].(string); ok {
			st.names[nameID] = fields
		}
		return
	case "NameRemoved":
		fields := eventFields(evt)
		if nameID, ok := fields["name_id"].(string); ok {
			delete(st.names, nameID)
		}
		return
	case "ChildLinkedToFamily", "ChildUnlinkedFromFamily":
		return
	case "PersonMerged":
		fields := eventFields(evt)
		if resolved, ok := fields["resolved_fields"].(map[string]any); ok {
			for key, value := range resolved {
				st.fields[key] = normalizeFieldValue(value)
			}
		}
		if snapshot, ok := fields["merged_person_snapshot"].(map[string]any); ok {
			st.mergedName = mergedPersonName(snapshot)
		}
		return
	case "EvidenceConflictResolved":
		fields := eventFields(evt)
		for _, key := range []string{"resolution", "status"} {
			if value, ok := fields[key]; ok {
				st.fields[key] = value
			}
		}
		return
	}

	class, ok := classifyHistoryEvent(evt.EventType)
	if !ok || class.Excluded() {
		return
	}
	switch class.Action {
	case actionCreated:
		st.fields = eventFields(evt)
		st.created = true
	case actionUpdated:
		fields := eventFields(evt)
		if changes, ok := fields["changes"].(map[string]any); ok {
			for key, value := range changes {
				st.fields[key] = normalizeFieldValue(value)
			}
		}
	}
}

// fieldChanges returns the field-level changes evt makes to prior, the state
// folded from every earlier event of its stream. withOld is false when that
// fold may be incomplete (see readBatchStreams), in which case no old value is
// reported rather than a possibly wrong one. Created and deleted events carry
// no field changes.
func fieldChanges(evt *repository.StoredEvent, prior *streamState, withOld bool) map[string]FieldChange {
	if special, ok := specialFieldChanges[evt.EventType]; ok {
		return special(eventFields(evt), prior, withOld)
	}
	class, ok := classifyHistoryEvent(evt.EventType)
	if !ok || class.Action != actionUpdated {
		return nil
	}
	changesMap, ok := eventFields(evt)["changes"].(map[string]any)
	if !ok {
		return nil
	}
	changes := make(map[string]FieldChange, len(changesMap))
	for key, value := range changesMap {
		changes[key] = FieldChange{OldValue: oldValue(prior.fields[key], withOld), NewValue: normalizeFieldValue(value)}
	}
	return changes
}

// oldValue reports value as a change's old value only when the fold that
// produced it is complete.
func oldValue(value any, withOld bool) any {
	if !withOld {
		return nil
	}
	return value
}

// specialFieldChanges derives the changes of the events that carry no
// "changes" map: they are described from their payload against the prior state.
var specialFieldChanges = map[string]func(fields map[string]any, prior *streamState, withOld bool) map[string]FieldChange{
	"NameAdded":                nameAddedChanges,
	"NameUpdated":              nameUpdatedChanges,
	"NameRemoved":              nameRemovedChanges,
	"PersonMerged":             personMergedChanges,
	"EvidenceConflictResolved": conflictResolvedChanges,
}

func nameAddedChanges(fields map[string]any, _ *streamState, _ bool) map[string]FieldChange {
	changes := map[string]FieldChange{"name": {NewValue: formatPersonNameFields(fields)}}
	if nameType, ok := fields["name_type"].(string); ok && nameType != "" {
		changes["name_type"] = FieldChange{NewValue: nameType}
	}
	return changes
}

func nameUpdatedChanges(fields map[string]any, prior *streamState, withOld bool) map[string]FieldChange {
	nameID, _ := fields["name_id"].(string)
	before := prior.names[nameID]
	newName := formatPersonNameFields(fields)
	if before == nil || !withOld {
		// The variant's earlier state is not in hand: report the name as it
		// now reads, without inventing what it replaced.
		return map[string]FieldChange{"name": {NewValue: newName}}
	}
	changes := map[string]FieldChange{}
	if oldName := formatPersonNameFields(before); oldName != newName {
		changes["name"] = FieldChange{OldValue: emptyToNil(oldName), NewValue: newName}
	}
	for _, key := range []string{"name_type", "is_primary"} {
		if !reflect.DeepEqual(before[key], fields[key]) {
			changes[key] = FieldChange{OldValue: before[key], NewValue: fields[key]}
		}
	}
	return changes
}

func nameRemovedChanges(fields map[string]any, prior *streamState, withOld bool) map[string]FieldChange {
	nameID, _ := fields["name_id"].(string)
	if before := prior.names[nameID]; before != nil && withOld {
		return map[string]FieldChange{"name": {OldValue: formatPersonNameFields(before)}}
	}
	return nil
}

func personMergedChanges(fields map[string]any, prior *streamState, withOld bool) map[string]FieldChange {
	changes := map[string]FieldChange{}
	if snapshot, ok := fields["merged_person_snapshot"].(map[string]any); ok {
		if name := mergedPersonName(snapshot); name != "" {
			changes["merged_person"] = FieldChange{NewValue: name}
		}
	}
	resolved, _ := fields["resolved_fields"].(map[string]any)
	for key, value := range resolved {
		value = normalizeFieldValue(value)
		if withOld && reflect.DeepEqual(prior.fields[key], value) {
			continue
		}
		changes[key] = FieldChange{OldValue: oldValue(prior.fields[key], withOld), NewValue: value}
	}
	return changes
}

func conflictResolvedChanges(fields map[string]any, prior *streamState, withOld bool) map[string]FieldChange {
	changes := map[string]FieldChange{}
	for _, key := range []string{"resolution", "status"} {
		if value, ok := fields[key]; ok {
			changes[key] = FieldChange{OldValue: oldValue(prior.fields[key], withOld), NewValue: value}
		}
	}
	return changes
}

func emptyToNil(s string) any {
	if s == "" {
		return nil
	}
	return s
}

// formatPersonNameFields renders a name variant ("Dr. John 'Jack' van Smith
// Jr.") from NameAdded/NameUpdated fields.
func formatPersonNameFields(fields map[string]any) string {
	if fields == nil {
		return ""
	}
	get := func(key string) string {
		v, _ := fields[key].(string)
		return strings.TrimSpace(v)
	}
	parts := []string{get("name_prefix"), get("given_name")}
	if nick := get("nickname"); nick != "" {
		parts = append(parts, `"`+nick+`"`)
	}
	parts = append(parts, get("surname_prefix"), get("surname"), get("name_suffix"))
	var kept []string
	for _, p := range parts {
		if p != "" {
			kept = append(kept, p)
		}
	}
	return strings.Join(kept, " ")
}

// mergedPersonName is the display name recorded in a PersonMerged snapshot.
func mergedPersonName(snapshot map[string]any) string {
	if full, ok := snapshot["full_name"].(string); ok && strings.TrimSpace(full) != "" {
		return strings.TrimSpace(full)
	}
	given, _ := snapshot["given_name"].(string)
	surname, _ := snapshot["surname"].(string)
	return fullName(given, surname)
}

// excerpt shortens free text to a one-line display name.
func excerpt(text string) string {
	text = strings.Join(strings.Fields(text), " ")
	if utf8.RuneCountInString(text) <= noteExcerptLength {
		return text
	}
	runes := []rune(text)
	return strings.TrimSpace(string(runes[:noteExcerptLength])) + "…"
}

// factLabel renders a fact type ("person_birth") as "Birth".
func factLabel(factType string) string {
	factType = strings.TrimPrefix(strings.TrimPrefix(factType, "person_"), "family_")
	words := strings.Fields(strings.ReplaceAll(strings.ToLower(factType), "_", " "))
	for i, w := range words {
		words[i] = strings.ToUpper(w[:1]) + w[1:]
	}
	return strings.Join(words, " ")
}

// joinLabel joins the non-empty parts with sep.
func joinLabel(sep string, parts ...string) string {
	var kept []string
	for _, p := range parts {
		if p = strings.TrimSpace(p); p != "" {
			kept = append(kept, p)
		}
	}
	return strings.Join(kept, sep)
}

// name returns the display name of the entity a change entry is about: the
// read-model name where the read model holds the entity (person, family,
// source, citation), otherwise a label built from its folded stream, falling
// back to the event payload and finally the id (entityNames.name).
func (d *historyDescription) name(entityType string, entityID uuid.UUID, evt *repository.StoredEvent) string {
	if label := d.stateLabel(entityType, entityID, evt); label != "" {
		return label
	}
	return d.names.name(entityType, entityID, evt)
}

// stateLabel labels an entity from its folded stream, or returns "" when the
// read model names it (or nothing in the stream does).
func (d *historyDescription) stateLabel(entityType string, entityID uuid.UUID, evt *repository.StoredEvent) string {
	state := d.states[entityID]
	switch entityType {
	case entityTypePerson:
		if d.names.persons[entityID] == nil {
			return d.personFromState(state)
		}
	case entityTypeFamily:
		if d.names.families[entityID] == nil && (evt == nil || evt.EventType != "FamilyCreated") {
			return d.familyFromState(state)
		}
	case entityTypeSource:
		if d.names.sources[entityID] == nil {
			return state.str("title")
		}
	case entityTypeCitation:
		if d.names.citations[entityID] == nil && state != nil {
			return joinLabel(" ", d.sourceTitle(state), parenthesize(factLabel(state.str("fact_type"))))
		}
	case entityTypeAssociation:
		return d.associationName(state)
	default:
		if label, ok := stateLabels[entityType]; ok {
			return label(state)
		}
	}
	return ""
}

// stateLabels build the display name of the entity types the read model does
// not name, from their folded fields.
var stateLabels = map[string]func(*streamState) string{
	entityTypeMedia: func(st *streamState) string {
		if title := st.str("title"); title != "" {
			return title
		}
		return st.str("filename")
	},
	entityTypeNote:       func(st *streamState) string { return excerpt(st.str("text")) },
	entityTypeSubmitter:  func(st *streamState) string { return st.str("name") },
	entityTypeRepository: func(st *streamState) string { return st.str("name") },
	entityTypeLifeEvent: func(st *streamState) string {
		return joinLabel(", ", factLabel(st.str("fact_type")), st.str("date"), st.str("place"))
	},
	entityTypeAttribute: func(st *streamState) string {
		return joinLabel(", ", joinLabel(": ", factLabel(st.str("fact_type")), st.str("value")), st.str("date"))
	},
	entityTypeLDSOrdinance: func(st *streamState) string {
		return joinLabel(", ", st.str("type"), st.str("date"), st.str("temple"))
	},
	entityTypeEvidenceAnalysis: factAndText("conclusion"),
	entityTypeProofSummary:     factAndText("conclusion"),
	entityTypeEvidenceConflict: factAndText("description"),
	entityTypeResearchLog: func(st *streamState) string {
		return joinLabel(" ", excerpt(st.str("search_description")), parenthesize(st.str("repository")))
	},
}

// factAndText labels a GPS artifact "Fact: <excerpt of field>".
func factAndText(field string) func(*streamState) string {
	return func(st *streamState) string {
		return joinLabel(": ", factLabel(st.str("fact_type")), excerpt(st.str(field)))
	}
}

func parenthesize(s string) string {
	if s == "" {
		return ""
	}
	return "(" + s + ")"
}

// personFromState names a person the read model does not hold from its
// folded stream: its own name fields, else its primary name variant.
func (d *historyDescription) personFromState(state *streamState) string {
	if state == nil || !state.created {
		// A few updated fields ("surname: Lovelace") are not a name.
		return ""
	}
	if name := fullName(state.str("given_name"), state.str("surname")); name != "" {
		return name
	}
	keys := make([]string, 0, len(state.names))
	for key := range state.names {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	for _, key := range keys {
		if primary, _ := state.names[key]["is_primary"].(bool); primary {
			return formatPersonNameFields(state.names[key])
		}
	}
	return ""
}

func (d *historyDescription) familyFromState(state *streamState) string {
	if state == nil {
		return ""
	}
	var partners []string
	for _, field := range []string{"partner1_id", "partner2_id"} {
		if id, ok := state.id(field); ok {
			partners = append(partners, d.names.personName(id, nil))
		}
	}
	return strings.Join(partners, " & ")
}

func (d *historyDescription) sourceTitle(state *streamState) string {
	id, ok := state.id("source_id")
	if !ok {
		return ""
	}
	if title := d.names.sourceName(id, nil); title != id.String() {
		return title
	}
	if src := d.states[id]; src != nil {
		return src.str("title")
	}
	return ""
}

func (d *historyDescription) associationName(state *streamState) string {
	if state == nil {
		return ""
	}
	var people []string
	for _, field := range []string{"person_id", "associate_id"} {
		if id, ok := state.id(field); ok {
			people = append(people, d.names.personName(id, nil))
		}
	}
	return joinLabel(": ", state.str("role"), strings.Join(people, " and "))
}

// parent returns the entity whose page presents a sub-record — a life
// event's person or family, a citation's source, a media item's owner — so a
// change entry can link to it. ok is false for entities that have a page of
// their own or none at all.
func (d *historyDescription) parent(entityType string, entityID uuid.UUID) (parentType string, parentID uuid.UUID, ok bool) {
	state := d.states[entityID]
	if state == nil {
		return "", uuid.Nil, false
	}
	switch entityType {
	case entityTypeLifeEvent, entityTypeLDSOrdinance:
		if id, ok := state.id("person_id"); ok {
			return entityTypePerson, id, true
		}
		if id, ok := state.id("family_id"); ok {
			return entityTypeFamily, id, true
		}
	case entityTypeAttribute, entityTypeAssociation:
		if id, ok := state.id("person_id"); ok {
			return entityTypePerson, id, true
		}
	case entityTypeCitation:
		if id, ok := state.id("source_id"); ok {
			return entityTypeSource, id, true
		}
	case entityTypeMedia:
		owner := strings.ToLower(state.str("entity_type"))
		if id, ok := state.id("entity_id"); ok {
			switch owner {
			case entityTypePerson, entityTypeFamily, entityTypeSource:
				return owner, id, true
			}
		}
	}
	return "", uuid.Nil, false
}
