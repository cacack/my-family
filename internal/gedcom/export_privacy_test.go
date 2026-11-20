package gedcom_test

import (
	"bytes"
	"context"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/cacack/my-family/internal/domain"
	"github.com/cacack/my-family/internal/gedcom"
	"github.com/cacack/my-family/internal/query"
	"github.com/cacack/my-family/internal/repository"
	"github.com/cacack/my-family/internal/repository/memory"
)

// A family whose partner and child the exporting principal may not see keeps
// its shape: the person list leaves them out, so the export writes each as
// the ADR-008 placeholder, an INDI holding the name "Living", the sex and the
// family links, and nothing of who they are (#1516).
func TestExport_FamilyMembersHiddenFromThePrincipalAreWrittenAsPlaceholders(t *testing.T) {
	ctx := context.Background()
	store := memory.NewReadModelStore()
	main := domain.MainBranchID
	now := time.Now().UTC()
	person := func(given string, gender domain.Gender, dead bool) uuid.UUID {
		p := &repository.PersonReadModel{ID: uuid.New(), GivenName: given, Surname: "Hollis", FullName: given + " Hollis",
			Gender: gender, BirthDateRaw: "1990", BirthPlace: given + "ton", DeathEvidence: dead, Version: 1, UpdatedAt: now}
		if err := store.SavePerson(ctx, main, p); err != nil {
			t.Fatal(err)
		}
		return p.ID
	}
	dead := person("Dorian", domain.GenderMale, true)
	living := person("Lumi", domain.GenderFemale, false)
	child := person("Kai", domain.GenderMale, false)
	family := &repository.FamilyReadModel{ID: uuid.New(), Partner1ID: &dead, Partner2ID: &living, MarriagePlace: "Chapelry", Version: 1, UpdatedAt: now}
	if err := store.SaveFamily(ctx, main, family); err != nil {
		t.Fatal(err)
	}
	if err := store.SaveFamilyChild(ctx, main, &repository.FamilyChildReadModel{FamilyID: family.ID, PersonID: child, RelationshipType: domain.ChildBiological}); err != nil {
		t.Fatal(err)
	}

	visitor := repository.ContextWithPrincipal(ctx, repository.Principal{Role: repository.RoleViewer, Anonymous: true})
	var buf bytes.Buffer
	result, err := gedcom.NewExporter(query.NewPrivacyStore(store)).Export(visitor, &buf)
	if err != nil {
		t.Fatalf("export: %v", err)
	}
	out := buf.String()
	for _, hidden := range []string{"Lumi", "Kai", "Lumiton", "Kaiton", "Chapelry"} {
		if strings.Contains(out, hidden) {
			t.Errorf("export holds %q:\n%s", hidden, out)
		}
	}
	if got := strings.Count(out, "1 NAME Living //"); got != 2 || result.PersonsExported != 3 {
		t.Errorf("export writes %d placeholders of %d people, want 2 of 3:\n%s", got, result.PersonsExported, out)
	}
	for _, want := range []string{"1 HUSB @I", "1 WIFE @I", "1 CHIL @I", "1 FAMC @F", "Dorian /Hollis/"} {
		if !strings.Contains(out, want) {
			t.Errorf("export has no %q:\n%s", want, out)
		}
	}
}
