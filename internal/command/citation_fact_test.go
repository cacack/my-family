package command_test

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/google/uuid"

	"github.com/cacack/my-family/internal/command"
	"github.com/cacack/my-family/internal/domain"
	"github.com/cacack/my-family/internal/repository"
	"github.com/cacack/my-family/internal/repository/memory"
)

const citedFactsGedcom = `0 HEAD
1 GEDC
2 VERS 5.5.1
1 CHAR UTF-8
0 @S1@ SOUR
1 TITL Census
0 @I1@ INDI
1 NAME A /B/
1 CENS
2 DATE 1850
1 OCCU Farmer
0 @I2@ INDI
1 NAME C /D/
0 TRLR
`

// A citation may name the life event or attribute it cites (#1294): one of
// the citation's fact type and owner. Moving the citation to another owner
// unlinks it, and a rollback relinks it.
func TestCreateCitation_FactID(t *testing.T) {
	ctx := context.Background()
	readStore := memory.NewReadModelStore()
	handler := command.NewHandler(memory.NewEventStore(), readStore)
	if _, err := handler.ImportGedcom(ctx, command.ImportGedcomInput{Filename: "t.ged", Reader: strings.NewReader(citedFactsGedcom)}); err != nil {
		t.Fatalf("import: %v", err)
	}
	persons, err := repository.ListAll(ctx, 10, readStore.ListPersons)
	if err != nil || len(persons) != 2 {
		t.Fatalf("persons = %v, %v", persons, err)
	}
	a, c := persons[0].ID, persons[1].ID
	if persons[0].GivenName != "A" {
		a, c = c, a
	}
	events, _ := readStore.ListEventsForPerson(ctx, domain.MainBranchID, a)
	attrs, _ := readStore.ListAttributesForPerson(ctx, domain.MainBranchID, a)
	sources, _ := repository.ListAll(ctx, 10, readStore.ListSources)
	if len(events) != 1 || len(attrs) != 1 || len(sources) != 1 {
		t.Fatalf("events %d, attributes %d, sources %d; want one each", len(events), len(attrs), len(sources))
	}
	census, occupation, sourceID := events[0].ID, attrs[0].ID, sources[0].ID

	input := func(factType domain.FactType, owner, factID uuid.UUID) command.CreateCitationInput {
		return command.CreateCitationInput{SourceID: sourceID, FactType: string(factType), FactOwnerID: owner, FactID: &factID}
	}
	for name, bad := range map[string]command.CreateCitationInput{
		"attribute of another fact type":  input(domain.FactPersonCensus, a, occupation),
		"attribute of another person":     input(domain.FactPersonOccupation, c, occupation),
		"life event of another person":    input(domain.FactPersonCensus, c, census),
		"life event of another fact type": input(domain.FactPersonOccupation, a, census),
		"no such fact":                    input(domain.FactPersonCensus, a, uuid.New()),
	} {
		if _, err := handler.CreateCitation(ctx, bad); !errors.Is(err, command.ErrInvalidInput) {
			t.Errorf("%s: err = %v, want ErrInvalidInput", name, err)
		}
	}

	created, err := handler.CreateCitation(ctx, input(domain.FactPersonOccupation, a, occupation))
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	factIDOf := func() *uuid.UUID {
		t.Helper()
		got, err := readStore.GetCitation(ctx, domain.MainBranchID, created.ID)
		if err != nil || got == nil {
			t.Fatalf("get citation: %+v, %v", got, err)
		}
		return got.FactID
	}
	if got := factIDOf(); got == nil || *got != occupation {
		t.Fatalf("fact_id = %v, want %s", got, occupation)
	}

	// A page edit keeps the link; a move to another owner drops it.
	page := "p. 3"
	updated, err := handler.UpdateCitation(ctx, command.UpdateCitationInput{ID: created.ID, Page: &page, Version: created.Version})
	if err != nil {
		t.Fatalf("update page: %v", err)
	}
	if got := factIDOf(); got == nil || *got != occupation {
		t.Errorf("fact_id after page edit = %v, want %s", got, occupation)
	}
	if _, err := handler.UpdateCitation(ctx, command.UpdateCitationInput{ID: created.ID, FactOwnerID: &c, Version: updated.Version}); err != nil {
		t.Fatalf("update owner: %v", err)
	}
	if got := factIDOf(); got != nil {
		t.Errorf("fact_id after owner change = %s, want none", got)
	}

	// Rolling back to the first version relinks it.
	if _, err := handler.RollbackCitation(ctx, created.ID, 1); err != nil {
		t.Fatalf("rollback: %v", err)
	}
	if got := factIDOf(); got == nil || *got != occupation {
		t.Errorf("fact_id after rollback = %v, want %s", got, occupation)
	}
}
