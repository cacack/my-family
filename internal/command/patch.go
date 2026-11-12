package command

import (
	"slices"

	"github.com/google/uuid"
)

// Patch is the update of one optional field in an Update* command input. It
// has three states, matching the API's PATCH semantics (#1359):
//
//   - omitted (the zero value): leave the field unchanged;
//   - Set(v): give the field the value v;
//   - Clear(): remove the field's value.
//
// Every optional field of an Update* input is a Patch; a required field,
// which cannot be cleared, stays a pointer (nil leaves it unchanged).
type Patch[T any] struct {
	state patchState
	value T
}

type patchState uint8

const (
	patchOmitted patchState = iota
	patchSet
	patchClear
)

// Set returns a Patch that gives the field the value v.
func Set[T any](v T) Patch[T] { return Patch[T]{state: patchSet, value: v} }

// Clear returns a Patch that removes the field's value.
func Clear[T any]() Patch[T] { return Patch[T]{state: patchClear} }

// SetPtr returns Set(*p), or an omitted Patch when p is nil. It suits callers
// that hold a pointer meaning "absent leaves it unchanged".
func SetPtr[T any](p *T) Patch[T] {
	if p == nil {
		return Patch[T]{}
	}
	return Set(*p)
}

// Omitted reports whether the update leaves the field unchanged.
func (p Patch[T]) Omitted() bool { return p.state == patchOmitted }

// Cleared reports whether the update removes the field's value.
func (p Patch[T]) Cleared() bool { return p.state == patchClear }

// Value returns the value the update sets, and true; or the zero value and
// false when the field is omitted or cleared.
func (p Patch[T]) Value() (T, bool) { return p.value, p.state == patchSet }

// Change returns the field's value after the update, and true; or the zero
// value and false when the field is omitted. A clear yields the zero value,
// which is how a field stored as a plain value (text, an enum, a list) is
// cleared.
func (p Patch[T]) Change() (T, bool) {
	var zero T
	switch p.state {
	case patchSet:
		return p.value, true
	case patchClear:
		return zero, true
	default:
		return zero, false
	}
}

// recordListChange records under key a list field's new value when it differs
// from current. A clear records an empty list.
func recordListChange[T comparable](changes map[string]any, key string, input Patch[[]T], current []T) {
	list, ok := input.Change()
	if !ok || slices.Equal(list, current) {
		return
	}
	if list == nil {
		list = []T{}
	}
	changes[key] = list
}

// recordOptionalChange records under key the new value of a field stored as
// an optional value (current is nil when unset), or nil when the update
// clears a field that is set. Setting the current value again, or clearing an
// unset field, is not a change.
func recordOptionalChange[T comparable](changes map[string]any, key string, input Patch[T], current *T) {
	if input.Cleared() {
		if current != nil {
			changes[key] = nil
		}
		return
	}
	if v, ok := input.Value(); ok && (current == nil || *current != v) {
		changes[key] = v
	}
}

// recordIDChange is recordOptionalChange for a reference, recording a set ID
// as its string: the JSON shape every reader of the stored event decodes
// (#848).
func recordIDChange(changes map[string]any, key string, input Patch[uuid.UUID], current *uuid.UUID) {
	recordOptionalChange(changes, key, input, current)
	if id, ok := changes[key].(uuid.UUID); ok {
		changes[key] = id.String()
	}
}
