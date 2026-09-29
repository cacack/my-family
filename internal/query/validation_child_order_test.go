package query

import (
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"

	"github.com/cacack/my-family/internal/repository"
)

// A family's children reach the validator in birth order whatever order the
// store returned the links in, so every backend validates the same document.
func TestSortChildrenByBirthOrder(t *testing.T) {
	one, two := 1, 2
	lowID, highID := uuid.MustParse("00000000-0000-4000-8000-000000000001"), uuid.MustParse("00000000-0000-4000-8000-000000000002")
	children := []repository.FamilyChildReadModel{
		{PersonID: highID, PersonGivenName: "Twin", PersonSurname: "A"},
		{PersonID: uuid.New(), PersonGivenName: "Second", PersonSurname: "B", Sequence: &two},
		{PersonID: lowID, PersonGivenName: "Twin", PersonSurname: "A"},
		{PersonID: uuid.New(), PersonGivenName: "First", PersonSurname: "C", Sequence: &one},
		{PersonID: uuid.New(), PersonGivenName: "Also", PersonSurname: "A"},
	}
	sortChildrenByBirthOrder(children)
	var got []string
	for _, c := range children {
		got = append(got, c.PersonGivenName)
	}
	assert.Equal(t, []string{"First", "Second", "Also", "Twin", "Twin"}, got)
	assert.Equal(t, lowID, children[3].PersonID, "a tie on name falls back to the id")
}
