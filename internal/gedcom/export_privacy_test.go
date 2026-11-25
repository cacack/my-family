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

// An export for a viewer leaves out private media and the media of people the
// viewer may not see in full, listed in bulk through the privacy store
// (#1480); an owner's export keeps them all.
func TestExport_MediaHiddenFromThePrincipalIsLeftOut(t *testing.T) {
	ctx := context.Background()
	store := memory.NewReadModelStore()
	main := domain.MainBranchID
	now := time.Now().UTC()
	person := func(given string, dead bool) uuid.UUID {
		p := &repository.PersonReadModel{ID: uuid.New(), GivenName: given, Surname: "Hollis", FullName: given + " Hollis",
			BirthDateRaw: "1990", DeathEvidence: dead, Version: 1, UpdatedAt: now}
		if err := store.SavePerson(ctx, main, p); err != nil {
			t.Fatal(err)
		}
		return p.ID
	}
	dead := person("Dorian", true)
	living := person("Lumi", false)
	for _, m := range []struct {
		owner       uuid.UUID
		file        string
		restriction domain.Restriction
	}{
		{dead, "public.jpg", ""},
		{dead, "private.jpg", "confidential"},
		{living, "living.jpg", ""},
	} {
		item := &repository.MediaReadModel{ID: uuid.New(), EntityType: "person", EntityID: m.owner, Title: m.file,
			MediaType: domain.MediaPhoto, Filename: m.file, Restriction: m.restriction, Version: 1, CreatedAt: now, UpdatedAt: now}
		if err := store.SaveMedia(ctx, main, item); err != nil {
			t.Fatal(err)
		}
	}
	export := func(principal repository.Principal) string {
		t.Helper()
		var buf bytes.Buffer
		if _, err := gedcom.NewExporter(query.NewPrivacyStore(store)).Export(repository.ContextWithPrincipal(ctx, principal), &buf); err != nil {
			t.Fatalf("export: %v", err)
		}
		return buf.String()
	}

	out := export(repository.Principal{Role: repository.RoleViewer, Anonymous: true})
	if !strings.Contains(out, "1 FILE public.jpg") {
		t.Errorf("visitor export has no public photo:\n%s", out)
	}
	for _, hidden := range []string{"private.jpg", "living.jpg"} {
		if strings.Contains(out, hidden) {
			t.Errorf("visitor export holds %q:\n%s", hidden, out)
		}
	}

	out = export(repository.Principal{Role: repository.RoleOwner})
	for _, file := range []string{"public.jpg", "private.jpg", "living.jpg"} {
		if !strings.Contains(out, "1 FILE "+file) {
			t.Errorf("owner export has no %q:\n%s", file, out)
		}
	}
}

// A place's form, EXIDs and pair are written only under events the export's
// privacy filter lets through: under the visible person's birth, and nowhere
// for the place only a hidden person names (ADR-009 §10).
func TestExport_PlaceFactsOnlyUnderVisibleEvents(t *testing.T) {
	ctx := context.Background()
	store := memory.NewReadModelStore()
	main := domain.MainBranchID
	now := time.Now().UTC()
	for _, p := range []struct {
		given, place string
		dead         bool
	}{{"Dorian", "Darmstadt,,,Germany", true}, {"Lumi", "Kiel, Germany", false}, {"Kai", "Darmstadt,,,Germany", false}} {
		if err := store.SavePerson(ctx, main, &repository.PersonReadModel{ID: uuid.New(), GivenName: p.given, Surname: "Hollis",
			FullName: p.given + " Hollis", BirthDateRaw: "1990", BirthPlace: p.place, DeathEvidence: p.dead, Version: 1, UpdatedAt: now}); err != nil {
			t.Fatal(err)
		}
	}
	projector := repository.NewProjector(store, nil)
	for _, place := range []struct{ key, text, positional, form, exid, lat, long string }{
		{"darmstadt,germany", "Darmstadt,,,Germany", "darmstadt,,,germany", "City, County, State, Country", "Q2973", "N49.87", "E8.65"},
		{"kiel,germany", "Kiel, Germany", "kiel,germany", "Town, Country", "Q1707", "N54.32", "E10.14"},
	} {
		event, err := repository.CanonicalizeChanges(domain.NewPlaceUpdated(repository.PlaceID(place.key), place.key, place.text, map[string]any{
			"latitude": place.lat, "longitude": place.long, "forms": map[string]string{place.positional: place.form},
			"external_ids": []domain.ExternalIdentifier{{Value: place.exid}},
		}))
		if err != nil {
			t.Fatal(err)
		}
		if err := projector.Project(ctx, event, 1, main); err != nil {
			t.Fatal(err)
		}
	}
	export := func(principal repository.Principal) string {
		t.Helper()
		var buf bytes.Buffer
		if _, err := gedcom.NewExporter(query.NewPrivacyStore(store)).Export(repository.ContextWithPrincipal(ctx, principal), &buf); err != nil {
			t.Fatalf("export: %v", err)
		}
		return buf.String()
	}

	out := export(repository.Principal{Role: repository.RoleViewer, Anonymous: true})
	darmstadt := "2 PLAC Darmstadt,,,Germany\n3 FORM City, County, State, Country\n3 MAP\n4 LATI N49.87\n4 LONG E8.65\n3 EXID Q2973\n"
	if got := strings.Count(out, darmstadt); got != 1 {
		t.Errorf("visitor export writes Darmstadt's facts %d times, want once, under Dorian's birth:\n%s", got, out)
	}
	for _, hidden := range []string{"Kiel", "Town, Country", "Q1707", "N54.32"} {
		if strings.Contains(out, hidden) {
			t.Errorf("visitor export holds %q:\n%s", hidden, out)
		}
	}
	out = export(repository.Principal{Role: repository.RoleOwner})
	if got := strings.Count(out, darmstadt); got != 2 {
		t.Errorf("owner export writes Darmstadt's facts %d times, want twice:\n%s", got, out)
	}
	if !strings.Contains(out, "2 PLAC Kiel, Germany\n3 FORM Town, Country\n3 MAP\n4 LATI N54.32\n4 LONG E10.14\n3 EXID Q1707\n") {
		t.Errorf("owner export lacks Kiel's facts:\n%s", out)
	}
}
