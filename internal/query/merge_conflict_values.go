package query

import (
	"context"
	"encoding/json"
	"fmt"
	"sort"
	"strconv"
	"strings"
	"unicode"

	"github.com/google/uuid"

	"github.com/cacack/my-family/internal/domain"
	"github.com/cacack/my-family/internal/repository"
)

// What each side of a merge conflict says, in words (#828).
//
// A conflict names the fields the two sides disagree on; a reviewer choosing a
// side needs the values themselves. For every contested field this file
// reports three: the value at the fork (the branch's base position), the value
// the branch now asserts and the value the mainline now asserts, with ids of
// referenced people, families, sources and citations resolved to names.
//
// The work is set-based, whatever the number of conflicts:
//
//   - ONE event-store read of main for every conflicted stream (the base
//     state is main's events up to the base position). The two sides' own
//     events are already in hand — they are the diff the verdict was computed
//     from — so each side is the base plus its own events, folded in memory.
//   - ONE batched read-model lookup per entity type for the referenced names
//     (resolveEntityNamesOn), plus one more set-based read only when a
//     referenced person has since been deleted (foldRelatedPeople).

// MergeConflictField is one contested field of a conflict, valued on each side.
// A nil value means the field is not set on that side (or, for delete_edit, the
// side deleted the entity; see MergeConflict.DeletedBy).
type MergeConflictField struct {
	// Field is the raw field key the conflict reports, e.g. "birth_place" or
	// "children[<person-id>]".
	Field string `json:"field"`
	// Label is the field's readable name, e.g. "Birth place" or "Child: Ada
	// Lovelace".
	Label       string  `json:"label"`
	BaseValue   *string `json:"base_value"`
	BranchValue *string `json:"branch_value"`
	MainValue   *string `json:"main_value"`
}

// Display values of a child-of-family relationship.
const (
	childLinkedText   = "Linked as a child"
	childUnlinkedText = "Not linked"
)

// conflictState is one side's view of a conflicted stream: the folded fields
// and names (streamState), plus the child links streamState does not track.
type conflictState struct {
	st       *streamState
	children map[uuid.UUID]string
}

func newConflictState() *conflictState {
	return &conflictState{st: newStreamState(), children: map[uuid.UUID]string{}}
}

// clone copies the state so a side can fold its own events on top of the base
// without disturbing it. Inner maps are replaced, never mutated, by apply, so
// a shallow copy of each map is enough.
func (c *conflictState) clone() *conflictState {
	out := newConflictState()
	for k, v := range c.st.fields {
		out.st.fields[k] = v
	}
	for k, v := range c.st.names {
		out.st.names[k] = v
	}
	out.st.created = c.st.created
	for k, v := range c.children {
		out.children[k] = v
	}
	return out
}

func (c *conflictState) apply(evt *repository.StoredEvent) {
	switch evt.EventType {
	case "ChildLinkedToFamily", "ChildUnlinkedFromFamily":
		var link struct {
			PersonID uuid.UUID `json:"person_id"`
		}
		if err := json.Unmarshal(evt.Data, &link); err == nil && link.PersonID != uuid.Nil {
			direction := relationLinked
			if evt.EventType == "ChildUnlinkedFromFamily" {
				direction = relationUnlinked
			}
			c.children[link.PersonID] = direction
		}
		return
	}
	c.st.apply(evt)
}

// describeConflictValues fills MergeConflict.FieldValues for every edit_edit
// and delete_edit conflict. See the comment at the top of this file.
func (s *BranchService) describeConflictValues(ctx context.Context, diff *branchDiffSources, conflicts []MergeConflict) error {
	var streamIDs []uuid.UUID
	for i := range conflicts {
		if conflicts[i].Kind == ConflictEditEdit || conflicts[i].Kind == ConflictDeleteEdit {
			streamIDs = append(streamIDs, conflicts[i].StreamID)
		}
	}
	if len(streamIDs) == 0 {
		return nil
	}

	mainStreams, reliableUpTo, err := s.historyService.readStreams(ctx, domain.MainBranchID, streamIDs, nil)
	if err != nil {
		return err
	}
	// A capped read may be missing events at or before the fork, in which case
	// the base is unknown and is reported as absent rather than guessed.
	baseKnown := reliableUpTo >= diff.branch.BasePosition

	branchByStream := groupEventsByStreamID(diff.branchEvents)
	mainByStream := groupEventsByStreamID(diff.mainEvents)
	branchSides := summarizeStreams(diff.branchEvents)
	mainSides := summarizeStreams(diff.mainEvents)

	type sides struct {
		base, branch, main *conflictState
		fields             []string
	}
	valued := make(map[uuid.UUID]*sides, len(streamIDs))
	refs := newEntityRefs()
	for i := range conflicts {
		c := &conflicts[i]
		if c.Kind != ConflictEditEdit && c.Kind != ConflictDeleteEdit {
			continue
		}
		base := newConflictState()
		for j := range mainStreams[c.StreamID] {
			evt := &mainStreams[c.StreamID][j]
			if evt.Position <= diff.branch.BasePosition {
				base.apply(evt)
			}
		}
		v := &sides{base: base, branch: foldOnto(base, branchByStream[c.StreamID]), main: foldOnto(base, mainByStream[c.StreamID])}
		if !baseKnown {
			v.base = nil
		}
		if c.Kind == ConflictEditEdit {
			v.fields = c.Fields
		} else {
			editor := branchSides[c.StreamID]
			if c.DeletedBy == resolveBranchValue {
				editor = mainSides[c.StreamID]
			}
			v.fields = editedFields(editor)
		}
		for _, state := range []*conflictState{v.base, v.branch, v.main} {
			registerConflictRefs(refs, state, v.fields)
		}
		valued[c.StreamID] = v
	}

	branchID := domain.BranchID(diff.branch.ID)
	desc := &historyDescription{states: map[uuid.UUID]*streamState{}}
	if desc.names, err = s.historyService.resolveEntityNamesOn(ctx, branchID, refs); err != nil {
		return fmt.Errorf("resolve referenced names: %w", err)
	}
	if err := s.historyService.foldRelatedPeople(ctx, branchID, desc, refs); err != nil {
		return err
	}

	for i := range conflicts {
		c := &conflicts[i]
		v := valued[c.StreamID]
		if v == nil {
			continue
		}
		c.FieldValues = make([]MergeConflictField, 0, len(v.fields))
		labels := make([]string, 0, len(v.fields))
		for _, field := range v.fields {
			entry := MergeConflictField{
				Field:       field,
				Label:       desc.conflictFieldLabel(field),
				BaseValue:   desc.conflictValue(v.base, field),
				BranchValue: desc.conflictValue(v.branch, field),
				MainValue:   desc.conflictValue(v.main, field),
			}
			switch c.DeletedBy {
			case resolveBranchValue:
				entry.BranchValue = nil
			case resolveMainValue:
				entry.MainValue = nil
			}
			c.FieldValues = append(c.FieldValues, entry)
			labels = append(labels, entry.Label)
		}
		if c.Kind == ConflictEditEdit && len(labels) > 0 {
			c.Detail = "The branch and main disagree on " + strings.Join(labels, "; ")
		}
	}
	return nil
}

// foldOnto returns base with events folded on top, leaving base untouched.
func foldOnto(base *conflictState, events []repository.StoredEvent) *conflictState {
	state := base.clone()
	for i := range events {
		state.apply(&events[i])
	}
	return state
}

// groupEventsByStreamID groups events by stream, keeping their order.
func groupEventsByStreamID(events []repository.StoredEvent) map[uuid.UUID][]repository.StoredEvent {
	grouped := make(map[uuid.UUID][]repository.StoredEvent)
	for _, evt := range events {
		grouped[evt.StreamID] = append(grouped[evt.StreamID], evt)
	}
	return grouped
}

// editedFields is the sorted set of fields a side changed; nil side, none.
func editedFields(side *streamSide) []string {
	if side == nil {
		return nil
	}
	fields := make([]string, 0, len(side.fields))
	for field := range side.fields {
		fields = append(fields, field)
	}
	sort.Strings(fields)
	return fields
}

// Structural field keys: "children[<person-id>]" and "names[<name-id>]".
const (
	childFieldPrefix = "children["
	nameFieldPrefix  = "names["
)

// bracketID parses the id out of a structural field key with the given prefix.
func bracketID(field, prefix string) (uuid.UUID, bool) {
	if !strings.HasPrefix(field, prefix) || !strings.HasSuffix(field, "]") {
		return uuid.Nil, false
	}
	id, err := uuid.Parse(field[len(prefix) : len(field)-1])
	if err != nil {
		return uuid.Nil, false
	}
	return id, true
}

// referenceFieldTypes maps the fields whose value is another entity's id to
// that entity's type, so the value can be shown as its name.
var referenceFieldTypes = map[string]string{
	"partner1_id":  entityTypePerson,
	"partner2_id":  entityTypePerson,
	"person_id":    entityTypePerson,
	"associate_id": entityTypePerson,
	"family_id":    entityTypeFamily,
	"source_id":    entityTypeSource,
	"citation_id":  entityTypeCitation,
}

// registerConflictRefs registers every entity a state's contested values (and
// their labels) refer to, so one batched lookup names them all.
func registerConflictRefs(refs entityRefs, state *conflictState, fields []string) {
	for _, field := range fields {
		if id, ok := bracketID(field, childFieldPrefix); ok {
			refs.add(entityTypePerson, id)
			continue
		}
		if state == nil {
			continue
		}
		if entityType, ok := referenceFieldTypes[field]; ok {
			if id, ok := state.st.id(field); ok {
				refs.add(entityType, id)
			}
		}
	}
}

// conflictFieldLabel renders a field key readably: "Birth place",
// "Partner 1", "Child: Ada Lovelace", "Name".
func (d *historyDescription) conflictFieldLabel(field string) string {
	if id, ok := bracketID(field, childFieldPrefix); ok {
		return "Child: " + d.personLabel(id)
	}
	if _, ok := bracketID(field, nameFieldPrefix); ok {
		return "Name"
	}
	return humanizeField(field)
}

// humanizeField turns a snake_case field key into a sentence-case label,
// dropping an "_id" suffix and separating a trailing number: "partner1_id"
// becomes "Partner 1", "birth_place" becomes "Birth place".
func humanizeField(field string) string {
	var b strings.Builder
	prevLetter := false
	for _, r := range strings.TrimSuffix(field, "_id") {
		switch {
		case r == '_':
			b.WriteRune(' ')
			prevLetter = false
			continue
		case unicode.IsDigit(r) && prevLetter:
			b.WriteRune(' ')
		}
		b.WriteRune(r)
		prevLetter = unicode.IsLetter(r)
	}
	label := strings.TrimSpace(b.String())
	if label == "" {
		return field
	}
	runes := []rune(label)
	runes[0] = unicode.ToUpper(runes[0])
	return string(runes)
}

// conflictValue renders the value state holds for field, or nil when it holds
// none. A nil state (an unknown base) has no value.
func (d *historyDescription) conflictValue(state *conflictState, field string) *string {
	if state == nil {
		return nil
	}
	if id, ok := bracketID(field, childFieldPrefix); ok {
		if state.children[id] == relationLinked {
			return strPtr(childLinkedText)
		}
		return strPtr(childUnlinkedText)
	}
	if id, ok := bracketID(field, nameFieldPrefix); ok {
		name := formatPersonNameFields(state.st.names[id.String()])
		if name == "" {
			return nil
		}
		return &name
	}
	value, ok := state.st.fields[field]
	if !ok {
		return nil
	}
	if entityType, isRef := referenceFieldTypes[field]; isRef {
		if id, ok := state.st.id(field); ok {
			return strPtr(d.referenceLabel(entityType, id))
		}
	}
	return formatConflictScalar(value)
}

// referenceLabel names a referenced entity, falling back to its id.
func (d *historyDescription) referenceLabel(entityType string, id uuid.UUID) string {
	if entityType == entityTypePerson {
		return d.personLabel(id)
	}
	return d.names.name(entityType, id, nil)
}

// formatConflictScalar renders a folded field value as text: strings as they
// are, booleans as Yes/No, numbers without exponent noise, anything else as
// compact JSON. An empty string is no value.
func formatConflictScalar(value any) *string {
	switch v := value.(type) {
	case nil:
		return nil
	case string:
		if strings.TrimSpace(v) == "" {
			return nil
		}
		return &v
	case bool:
		if v {
			return strPtr("Yes")
		}
		return strPtr("No")
	case float64:
		return strPtr(strconv.FormatFloat(v, 'f', -1, 64))
	default:
		encoded, err := json.Marshal(v)
		if err != nil {
			return strPtr(fmt.Sprint(v))
		}
		return strPtr(string(encoded))
	}
}

func strPtr(s string) *string { return &s }
