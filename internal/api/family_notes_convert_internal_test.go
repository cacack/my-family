package api

import (
	"testing"

	"github.com/google/uuid"

	"github.com/cacack/my-family/internal/query"
	"github.com/cacack/my-family/internal/repository"
)

// Every converter that builds a Family response carries the family's notes,
// marriage notes and marriage TYPE, and omits them when unset (#1350).
func TestConverters_FamilyNotes(t *testing.T) {
	keys := []string{"notes", "marriage_notes", "marriage_type_detail"}
	notes, marriageNotes, typeDetail := "@N1@", "Married at the town hall", "Civil"
	want := map[string]string{"notes": notes, "marriage_notes": marriageNotes, "marriage_type_detail": typeDetail}
	qf := query.Family{ID: uuid.New(), Notes: &notes, MarriageNotes: &marriageNotes, MarriageTypeDetail: &typeDetail}
	rm := repository.FamilyReadModel{ID: uuid.New(), Notes: notes, MarriageNotes: marriageNotes, MarriageTypeDetail: typeDetail}

	assertJSONCoordinates(t, "query family", convertQueryFamilyToGenerated(qf), keys, want)
	assertJSONCoordinates(t, "query family detail", convertQueryFamilyDetailToGenerated(query.FamilyDetail{Family: qf}), keys, want)
	assertJSONCoordinates(t, "query family as detail", convertQueryFamilyToFamilyDetail(qf), keys, want)
	assertJSONCoordinates(t, "read model family", convertReadModelFamilyToGenerated(rm), keys, want)

	assertJSONCoordinates(t, "query family without", convertQueryFamilyToGenerated(query.Family{}), keys, nil)
	assertJSONCoordinates(t, "query family detail without", convertQueryFamilyDetailToGenerated(query.FamilyDetail{}), keys, nil)
	assertJSONCoordinates(t, "query family as detail without", convertQueryFamilyToFamilyDetail(query.Family{}), keys, nil)
	assertJSONCoordinates(t, "read model family without", convertReadModelFamilyToGenerated(repository.FamilyReadModel{ID: uuid.New()}), keys, nil)
}
