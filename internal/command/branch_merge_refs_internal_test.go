package command

// personReferences decodes stored payloads directly, so the shapes it must read
// the way the projection does — a FamilyUpdated that clears a partner, or
// carries a value the projection stores as "no partner" — are tested here on
// hand-built events: no command writes them today.

import (
	"slices"
	"testing"

	"github.com/google/uuid"

	"github.com/cacack/my-family/internal/repository"
)

func TestPersonReferences_FamilyPartners(t *testing.T) {
	p1, p2 := uuid.New(), uuid.New()
	cases := []struct {
		name      string
		eventType string
		data      string
		want      []uuid.UUID
	}{
		{"created with both partners", "FamilyCreated",
			`{"family_id":"` + uuid.NewString() + `","partner1_id":"` + p1.String() + `","partner2_id":"` + p2.String() + `"}`,
			[]uuid.UUID{p1, p2}},
		{"created with one partner", "FamilyCreated", `{"partner2_id":"` + p2.String() + `"}`, []uuid.UUID{p2}},
		{"created with no partner", "FamilyCreated", `{"family_id":"` + uuid.NewString() + `"}`, nil},
		{"update sets both partners", "FamilyUpdated",
			`{"changes":{"partner1_id":"` + p1.String() + `","partner2_id":"` + p2.String() + `"}}`,
			[]uuid.UUID{p1, p2}},
		{"update clears a partner with null", "FamilyUpdated", `{"changes":{"partner1_id":null}}`, nil},
		{"update clears a partner with an empty string", "FamilyUpdated", `{"changes":{"partner2_id":""}}`, nil},
		{"update with a value the projection ignores", "FamilyUpdated", `{"changes":{"partner1_id":"not-a-uuid","partner2_id":7}}`, nil},
		{"update not touching partners", "FamilyUpdated", `{"changes":{"marriage_place":"Paris"}}`, nil},
		{"update with no changes", "FamilyUpdated", `{}`, nil},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, err := personReferences(repository.StoredEvent{EventType: tc.eventType, Data: []byte(tc.data)})
			if err != nil {
				t.Fatalf("personReferences failed: %v", err)
			}
			if !slices.Equal(got, tc.want) {
				t.Errorf("personReferences = %v, want %v", got, tc.want)
			}
		})
	}
}

func TestPersonReferences_MalformedFamilyPayload(t *testing.T) {
	for _, eventType := range []string{"FamilyCreated", "FamilyUpdated"} {
		if _, err := personReferences(repository.StoredEvent{EventType: eventType, Data: []byte(`{`)}); err == nil {
			t.Errorf("%s: personReferences accepted a malformed payload", eventType)
		}
	}
}
