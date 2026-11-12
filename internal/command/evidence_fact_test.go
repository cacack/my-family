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

const evidenceCensusesGedcom = `0 HEAD
1 GEDC
2 VERS 5.5.1
1 CHAR UTF-8
0 @I1@ INDI
1 NAME A /B/
1 CENS
2 DATE 1850
1 CENS
2 DATE 1860
0 @I2@ INDI
1 NAME C /D/
0 TRLR
`

// An evidence analysis or proof summary may name the life event it is about
// (#1304). An edit keeps the link, a move to another subject drops it, and a
// fact_id that is not a fact of the subject is rejected.
func TestEvidenceArtifacts_FactID(t *testing.T) {
	ctx := context.Background()
	readStore := memory.NewReadModelStore()
	handler := command.NewHandler(memory.NewEventStore(), readStore)
	if _, err := handler.ImportGedcom(ctx, command.ImportGedcomInput{Filename: "t.ged", Reader: strings.NewReader(evidenceCensusesGedcom)}); err != nil {
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
	if len(events) != 2 {
		t.Fatalf("events = %d, want 2", len(events))
	}
	first, second := events[0].ID, events[1].ID
	census := string(domain.FactPersonCensus)

	t.Run("analysis", func(t *testing.T) {
		created, err := handler.CreateEvidenceAnalysis(ctx, command.CreateEvidenceAnalysisInput{
			FactType: census, SubjectID: a, FactID: &first, Conclusion: "Ohio",
		})
		if err != nil {
			t.Fatalf("create: %v", err)
		}
		factIDOf := func() *uuid.UUID {
			t.Helper()
			got, err := readStore.GetEvidenceAnalysis(ctx, domain.MainBranchID, created.ID)
			if err != nil || got == nil {
				t.Fatalf("get analysis: %+v, %v", got, err)
			}
			return got.FactID
		}
		version := created.Version
		update := func(input command.UpdateEvidenceAnalysisInput) error {
			t.Helper()
			input.ID, input.Version = created.ID, version
			result, err := handler.UpdateEvidenceAnalysis(ctx, input)
			if err == nil {
				version = result.Version
			}
			return err
		}

		if err := update(command.UpdateEvidenceAnalysisInput{FactID: command.Set(first)}); err != nil || version != created.Version {
			t.Errorf("unchanged fact_id: version %d, err %v; want no new version", version, err)
		}
		conclusion := "Ohio, Franklin County"
		if err := update(command.UpdateEvidenceAnalysisInput{Conclusion: &conclusion}); err != nil {
			t.Fatalf("update conclusion: %v", err)
		}
		if got := factIDOf(); got == nil || *got != first {
			t.Errorf("fact_id after conclusion edit = %v, want %s", got, first)
		}
		if err := update(command.UpdateEvidenceAnalysisInput{FactID: command.Set(second)}); err != nil {
			t.Fatalf("update fact_id: %v", err)
		}
		if got := factIDOf(); got == nil || *got != second {
			t.Errorf("fact_id after relink = %v, want %s", got, second)
		}
		if err := update(command.UpdateEvidenceAnalysisInput{SubjectID: &c, FactID: command.Set(second)}); !errors.Is(err, command.ErrInvalidInput) {
			t.Errorf("move to another subject keeping a fact of the old one: err = %v, want ErrInvalidInput", err)
		}
		if err := update(command.UpdateEvidenceAnalysisInput{SubjectID: &c}); err != nil {
			t.Fatalf("update subject: %v", err)
		}
		if got := factIDOf(); got != nil {
			t.Errorf("fact_id after subject change = %s, want none", got)
		}
	})

	t.Run("proof summary", func(t *testing.T) {
		missing := uuid.New()
		if _, err := handler.CreateProofSummary(ctx, command.CreateProofSummaryInput{
			FactType: census, SubjectID: a, FactID: &missing, Conclusion: "x", Argument: "y",
		}); !errors.Is(err, command.ErrInvalidInput) {
			t.Errorf("create with no such fact: err = %v, want ErrInvalidInput", err)
		}
		created, err := handler.CreateProofSummary(ctx, command.CreateProofSummaryInput{
			FactType: census, SubjectID: a, FactID: &first, Conclusion: "Ohio", Argument: "Two records agree",
		})
		if err != nil {
			t.Fatalf("create: %v", err)
		}
		birth := string(domain.FactPersonBirth)
		if _, err := handler.UpdateProofSummary(ctx, command.UpdateProofSummaryInput{
			ID: created.ID, FactType: &birth, Version: created.Version,
		}); err != nil {
			t.Fatalf("update fact type: %v", err)
		}
		got, err := readStore.GetProofSummary(ctx, domain.MainBranchID, created.ID)
		if err != nil || got == nil || got.FactID != nil {
			t.Errorf("proof after fact type change = %+v, %v; want no fact_id", got, err)
		}
	})
}

// Analyses of two censuses do not conflict; a second analysis of one census
// that disagrees raises a conflict about that census, apart from any open
// conflict about the census fact as a whole (#1304).
func TestDetectEvidenceConflicts_FactInstance(t *testing.T) {
	ctx := context.Background()
	readStore := memory.NewReadModelStore()
	handler := command.NewHandler(memory.NewEventStore(), readStore)
	if _, err := handler.ImportGedcom(ctx, command.ImportGedcomInput{Filename: "t.ged", Reader: strings.NewReader(evidenceCensusesGedcom)}); err != nil {
		t.Fatalf("import: %v", err)
	}
	persons, _ := repository.ListAll(ctx, 10, readStore.ListPersons)
	a := persons[0].ID
	if persons[0].GivenName != "A" {
		a = persons[1].ID
	}
	events, _ := readStore.ListEventsForPerson(ctx, domain.MainBranchID, a)
	first, second := events[0].ID, events[1].ID
	analyze := func(conclusion string, factID *uuid.UUID) *uuid.UUID {
		t.Helper()
		result, err := handler.CreateEvidenceAnalysis(ctx, command.CreateEvidenceAnalysisInput{
			FactType: string(domain.FactPersonCensus), SubjectID: a, FactID: factID, Conclusion: conclusion,
		})
		if err != nil {
			t.Fatalf("create %s: %v", conclusion, err)
		}
		return result.ConflictID
	}

	analyze("Ohio", &first)
	if id := analyze("Indiana", &second); id != nil {
		t.Errorf("analyses of two censuses raised conflict %s", id)
	}
	wholeConflict := analyze("Midwest", nil)
	if wholeConflict == nil {
		t.Fatal("whole-fact analysis that disagrees raised no conflict")
	}
	instanceConflict := analyze("Kentucky", &first)
	if instanceConflict == nil || *instanceConflict == *wholeConflict {
		t.Fatalf("conflict about the 1850 census = %v, want a new one (whole-fact conflict %s)", instanceConflict, wholeConflict)
	}
	conflict, err := readStore.GetEvidenceConflict(ctx, domain.MainBranchID, *instanceConflict)
	if err != nil || conflict == nil || conflict.FactID == nil || *conflict.FactID != first {
		t.Errorf("conflict = %+v, %v; want fact_id %s", conflict, err, first)
	}
	if again := analyze("Tennessee", &first); again == nil || *again != *instanceConflict {
		t.Errorf("another disagreement about the 1850 census = %v, want the open conflict %s", again, instanceConflict)
	}
}
