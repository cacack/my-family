package api

import (
	"encoding/json"
	"testing"

	"github.com/google/uuid"

	"github.com/cacack/my-family/internal/repository"
)

// The events export carries is_negated for a negated event and omits it
// otherwise (#1302).
func TestConvertEventToExport_IsNegated(t *testing.T) {
	tests := []struct {
		name    string
		negated bool
		want    any
	}{
		{name: "negated", negated: true, want: true},
		{name: "not negated", negated: false, want: nil},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			b, err := json.Marshal(convertEventToExport(repository.EventReadModel{ID: uuid.New(), IsNegated: tt.negated}))
			if err != nil {
				t.Fatalf("marshal: %v", err)
			}
			var got map[string]any
			if err := json.Unmarshal(b, &got); err != nil {
				t.Fatalf("unmarshal: %v", err)
			}
			if got["is_negated"] != tt.want {
				t.Errorf("is_negated = %v, want %v", got["is_negated"], tt.want)
			}
		})
	}
}
