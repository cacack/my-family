package repository

import (
	"encoding/json"
	"fmt"
	"reflect"

	"github.com/cacack/my-family/internal/domain"
)

// changesMapType is the type of the Changes field every *Updated event with a
// partial-update payload carries.
var changesMapType = reflect.TypeFor[map[string]any]()

// CanonicalizeChanges returns event with its Changes map rewritten into the
// exact shape DecodeEvent produces for it: the map is round-tripped through
// JSON, so a *uuid.UUID becomes a string, a named string type (RelationType,
// …) a plain string, an int a float64, a slice a []any and a struct a
// map[string]any.
//
// A command builds its changes map from typed Go values, but every other reader
// of the event — replay, branch merge, resume repair, history, rollback — sees
// it decoded from its stored JSON. Projecting the typed map live and the
// decoded map on replay is how the read model and the event log came to
// disagree (issue #848). Canonicalizing before the append and the synchronous
// projection makes the live event identical to the one every later reader
// decodes, so a projection only ever has to understand the JSON shape.
//
// Events without a map[string]any Changes field (every *Created, NameUpdated,
// …) are returned unchanged. The check is structural rather than a list of
// event types so a *Updated event added later is covered without anyone having
// to remember this function.
func CanonicalizeChanges(event domain.Event) (domain.Event, error) {
	v := reflect.ValueOf(event)
	if v.Kind() != reflect.Struct {
		return event, nil
	}
	field, ok := v.Type().FieldByName("Changes")
	if !ok || field.Type != changesMapType {
		return event, nil
	}
	changes, _ := v.FieldByIndex(field.Index).Interface().(map[string]any)
	if changes == nil {
		return event, nil
	}

	canonical, err := canonicalChanges(changes)
	if err != nil {
		return nil, fmt.Errorf("canonicalize %s changes: %w", event.EventType(), err)
	}

	cp := reflect.New(v.Type()).Elem()
	cp.Set(v)
	cp.FieldByIndex(field.Index).Set(reflect.ValueOf(canonical))
	out, ok := cp.Interface().(domain.Event)
	if !ok {
		return nil, fmt.Errorf("canonicalize %s changes: copy is not an event", event.EventType())
	}
	return out, nil
}

// canonicalChanges round-trips a changes map through JSON, the same encoding
// EncodeEvent writes and DecodeEvent reads.
func canonicalChanges(changes map[string]any) (map[string]any, error) {
	b, err := json.Marshal(changes)
	if err != nil {
		return nil, err
	}
	var out map[string]any
	if err := json.Unmarshal(b, &out); err != nil {
		return nil, err
	}
	return out, nil
}
