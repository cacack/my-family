package api

import (
	"testing"

	"github.com/google/uuid"

	"github.com/cacack/my-family/internal/query"
)

// A fork value the server could not read travels as base_unknown, so the
// review says "Unknown" rather than "Not set" (#828).
func TestConvertMergeConflicts_BaseUnknown(t *testing.T) {
	branch := "Lovelace"
	out := convertQueryMergeConflictsToGenerated([]query.MergeConflict{{
		StreamID: uuid.New(),
		Kind:     query.ConflictEditEdit,
		Fields:   []string{"surname", "given_name"},
		FieldValues: []query.MergeConflictField{
			{Field: "surname", Label: "Surname", BranchValue: &branch, BaseUnknown: true},
			{Field: "given_name", Label: "Given name"},
		},
	}})
	if len(out) != 1 || out[0].FieldValues == nil || len(*out[0].FieldValues) != 2 {
		t.Fatalf("converted = %+v, want one conflict with two field values", out)
	}
	values := *out[0].FieldValues
	if values[0].BaseUnknown == nil || !*values[0].BaseUnknown {
		t.Errorf("surname base_unknown = %v, want true", values[0].BaseUnknown)
	}
	if values[1].BaseUnknown != nil {
		t.Errorf("given_name base_unknown = %v, want absent", *values[1].BaseUnknown)
	}
}
