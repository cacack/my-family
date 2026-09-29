package repository

import (
	"testing"

	"github.com/google/uuid"

	"github.com/cacack/my-family/internal/domain"
)

// TestRelinkEdgeParent covers relinkChildEdges' per-edge rules: swap the
// merged person for the survivor, and fill a parent slot a link left empty
// only when the edge still agrees with the family (#834).
func TestRelinkEdgeParent(t *testing.T) {
	merged, partner, stranger := uuid.New(), uuid.New(), uuid.New()
	male := &PersonReadModel{ID: uuid.New(), FullName: "Sam Survivor", Gender: domain.GenderMale}
	female := &PersonReadModel{ID: uuid.New(), FullName: "Sue Survivor", Gender: domain.GenderFemale}
	familyWith := func(survivor *PersonReadModel) *FamilyReadModel {
		return &FamilyReadModel{ID: uuid.New(), Partner1ID: &survivor.ID, Partner2ID: &partner}
	}
	ptr := func(id uuid.UUID) *uuid.UUID { return &id }

	tests := []struct {
		name        string
		edge        PedigreeEdge
		survivor    *PersonReadModel
		family      *FamilyReadModel
		wantChanged bool
		wantFather  *uuid.UUID
		wantMother  *uuid.UUID
	}{
		{"swaps merged father", PedigreeEdge{FatherID: ptr(merged), MotherID: ptr(partner)}, male, familyWith(male), true, &male.ID, &partner},
		{"swaps merged mother", PedigreeEdge{FatherID: ptr(partner), MotherID: ptr(merged)}, female, familyWith(female), true, &partner, &female.ID},
		{"fills empty father", PedigreeEdge{MotherID: ptr(partner)}, male, familyWith(male), true, &male.ID, &partner},
		{"fills empty mother", PedigreeEdge{FatherID: ptr(partner)}, female, familyWith(female), true, &partner, &female.ID},
		{"survivor already named", PedigreeEdge{FatherID: ptr(male.ID)}, male, familyWith(male), false, &male.ID, nil},
		{"edge from another family", PedigreeEdge{MotherID: ptr(stranger)}, male, familyWith(male), false, nil, &stranger},
		{"father slot taken", PedigreeEdge{FatherID: ptr(partner)}, male, familyWith(male), false, &partner, nil},
		{"mother slot taken", PedigreeEdge{MotherID: ptr(partner)}, female, familyWith(female), false, nil, &partner},
		{"survivor not a partner", PedigreeEdge{}, male, &FamilyReadModel{ID: uuid.New(), Partner2ID: &partner}, false, nil, nil},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			edge := tt.edge
			if got := relinkEdgeParent(&edge, tt.family, merged, tt.survivor); got != tt.wantChanged {
				t.Errorf("changed = %v, want %v", got, tt.wantChanged)
			}
			if !sameID(edge.FatherID, tt.wantFather) || !sameID(edge.MotherID, tt.wantMother) {
				t.Errorf("edge = father %v, mother %v; want father %v, mother %v",
					edge.FatherID, edge.MotherID, tt.wantFather, tt.wantMother)
			}
		})
	}
}

func sameID(a, b *uuid.UUID) bool {
	if a == nil || b == nil {
		return a == b
	}
	return *a == *b
}
