package command

import (
	"reflect"
	"testing"

	"github.com/google/uuid"
)

func TestPatch_States(t *testing.T) {
	var omitted Patch[string]
	if !omitted.Omitted() || omitted.Cleared() {
		t.Errorf("zero Patch: Omitted=%v Cleared=%v, want omitted", omitted.Omitted(), omitted.Cleared())
	}
	if _, ok := omitted.Change(); ok {
		t.Error("omitted Patch reports a change")
	}

	set := Set("x")
	if v, ok := set.Value(); !ok || v != "x" {
		t.Errorf("Set(x).Value() = %q, %v", v, ok)
	}
	if v, ok := set.Change(); !ok || v != "x" {
		t.Errorf("Set(x).Change() = %q, %v", v, ok)
	}

	cleared := Clear[string]()
	if !cleared.Cleared() || cleared.Omitted() {
		t.Errorf("Clear: Cleared=%v Omitted=%v", cleared.Cleared(), cleared.Omitted())
	}
	if _, ok := cleared.Value(); ok {
		t.Error("Clear().Value() reports a value")
	}
	if v, ok := cleared.Change(); !ok || v != "" {
		t.Errorf("Clear().Change() = %q, %v, want the zero value", v, ok)
	}

	if !SetPtr[string](nil).Omitted() {
		t.Error("SetPtr(nil) is not omitted")
	}
	s := "y"
	if v, ok := SetPtr(&s).Value(); !ok || v != "y" {
		t.Errorf("SetPtr(&y).Value() = %q, %v", v, ok)
	}
}

func TestRecordOptionalChange(t *testing.T) {
	one, two := 1, 2
	for _, tc := range []struct {
		name    string
		input   Patch[int]
		current *int
		want    map[string]any
	}{
		{"omitted", Patch[int]{}, &one, map[string]any{}},
		{"same value", Set(1), &one, map[string]any{}},
		{"new value", Set(2), &one, map[string]any{"k": 2}},
		{"set from unset", Set(2), nil, map[string]any{"k": 2}},
		{"clear set", Clear[int](), &two, map[string]any{"k": nil}},
		{"clear unset", Clear[int](), nil, map[string]any{}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			changes := map[string]any{}
			recordOptionalChange(changes, "k", tc.input, tc.current)
			if !reflect.DeepEqual(changes, tc.want) {
				t.Errorf("changes = %v, want %v", changes, tc.want)
			}
		})
	}

	id := uuid.New()
	changes := map[string]any{}
	recordIDChange(changes, "id", Set(id), nil)
	if changes["id"] != id.String() {
		t.Errorf("recordIDChange set = %v, want the id string", changes["id"])
	}
}

func TestRecordListChange(t *testing.T) {
	changes := map[string]any{}
	recordListChange(changes, "l", Clear[[]string](), []string{"a"})
	if got, ok := changes["l"].([]string); !ok || got == nil || len(got) != 0 {
		t.Errorf("cleared list recorded as %#v, want an empty list", changes["l"])
	}
	changes = map[string]any{}
	recordListChange(changes, "l", Clear[[]string](), nil)
	if len(changes) != 0 {
		t.Errorf("clearing an empty list recorded %v", changes)
	}
}
