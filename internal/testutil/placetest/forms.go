package placetest

import (
	"context"
	"reflect"
	"testing"

	"github.com/google/uuid"

	"github.com/cacack/my-family/internal/domain"
	"github.com/cacack/my-family/internal/repository"
)

// RunPlaceFormsAndExternalIDs is the #1738 scenario: a PlaceUpdated's
// "forms" and "external_ids" changes replace a place's rows of place_forms
// (one form per positional key) and place_external_ids (in order) on every
// store, leave another place's rows alone, move the place's version, and a
// nil change clears them (ADR-009 §2, §8). store must start empty.
func RunPlaceFormsAndExternalIDs(t *testing.T, store repository.ReadModelStore) {
	t.Helper()
	ctx := context.Background()
	projector := repository.NewProjector(store, nil)
	darmstadt, kiel := repository.PlaceID("darmstadt,germany"), repository.PlaceID("kiel,germany")
	project := func(id uuid.UUID, key, text string, version int64, changes map[string]any) {
		t.Helper()
		// Changes as the log decodes them (repository.CanonicalizeChanges).
		event, err := repository.CanonicalizeChanges(domain.NewPlaceUpdated(id, key, text, changes))
		if err != nil {
			t.Fatal(err)
		}
		if err := projector.Project(ctx, event, version, domain.MainBranchID); err != nil {
			t.Fatalf("project PlaceUpdated(%s): %v", key, err)
		}
	}
	gov := domain.ExternalIdentifier{Value: "DARTADJO49EV", Type: "https://gov.genealogy.net"}
	wiki := domain.ExternalIdentifier{Value: "Q2973", Type: "https://www.wikidata.org/entity"}
	project(darmstadt, "darmstadt,germany", "Darmstadt,,,Germany", 1, map[string]any{
		"forms":        map[string]string{"darmstadt,,,germany": "City, County, State, Country", "darmstadt,germany": "City, Country"},
		"external_ids": []domain.ExternalIdentifier{wiki, gov},
	})
	project(kiel, "kiel,germany", "Kiel, Germany", 1, map[string]any{
		"forms":        map[string]string{"kiel,germany": "City, Country"},
		"external_ids": []domain.ExternalIdentifier{{Value: "Q1707"}},
	})

	check := func(label string, wantForms map[uuid.UUID][]repository.PlaceFormReadModel, wantIDs map[uuid.UUID][]repository.PlaceExternalIDReadModel) {
		t.Helper()
		forms, err := store.ListPlaceForms(ctx)
		if err != nil {
			t.Fatalf("%s: ListPlaceForms: %v", label, err)
		}
		if len(forms) == 0 {
			forms = nil
		}
		if !reflect.DeepEqual(forms, wantForms) {
			t.Errorf("%s: ListPlaceForms = %+v, want %+v", label, forms, wantForms)
		}
		ids, err := store.ListPlaceExternalIDs(ctx)
		if err != nil {
			t.Fatalf("%s: ListPlaceExternalIDs: %v", label, err)
		}
		if len(ids) == 0 {
			ids = nil
		}
		if !reflect.DeepEqual(ids, wantIDs) {
			t.Errorf("%s: ListPlaceExternalIDs = %+v, want %+v", label, ids, wantIDs)
		}
	}
	kielForms := []repository.PlaceFormReadModel{{PlaceID: kiel, PositionalKey: "kiel,germany", Form: "City, Country"}}
	kielIDs := []repository.PlaceExternalIDReadModel{{PlaceID: kiel, Sequence: 0, Value: "Q1707"}}
	check("after setting", map[uuid.UUID][]repository.PlaceFormReadModel{
		darmstadt: {
			{PlaceID: darmstadt, PositionalKey: "darmstadt,,,germany", Form: "City, County, State, Country"},
			{PlaceID: darmstadt, PositionalKey: "darmstadt,germany", Form: "City, Country"},
		},
		kiel: kielForms,
	}, map[uuid.UUID][]repository.PlaceExternalIDReadModel{
		darmstadt: {
			{PlaceID: darmstadt, Sequence: 0, Value: wiki.Value, Type: wiki.Type},
			{PlaceID: darmstadt, Sequence: 1, Value: gov.Value, Type: gov.Type},
		},
		kiel: kielIDs,
	})
	if f, err := store.GetPlaceFacts(ctx, darmstadt); err != nil || f == nil || f.Version != 1 || f.Key != "darmstadt,germany" {
		t.Errorf("GetPlaceFacts(darmstadt) = %+v, %v; want a row at version 1", f, err)
	}

	// The whole set is replaced: one form left, then both changes cleared.
	project(darmstadt, "darmstadt,germany", "Darmstadt,,,Germany", 2, map[string]any{
		"forms": map[string]string{"darmstadt,germany": "Town, Country"},
	})
	check("after replacing", map[uuid.UUID][]repository.PlaceFormReadModel{
		darmstadt: {{PlaceID: darmstadt, PositionalKey: "darmstadt,germany", Form: "Town, Country"}},
		kiel:      kielForms,
	}, map[uuid.UUID][]repository.PlaceExternalIDReadModel{
		darmstadt: {
			{PlaceID: darmstadt, Sequence: 0, Value: wiki.Value, Type: wiki.Type},
			{PlaceID: darmstadt, Sequence: 1, Value: gov.Value, Type: gov.Type},
		},
		kiel: kielIDs,
	})
	project(darmstadt, "darmstadt,germany", "Darmstadt,,,Germany", 3, map[string]any{"forms": nil, "external_ids": nil})
	check("after clearing", map[uuid.UUID][]repository.PlaceFormReadModel{kiel: kielForms},
		map[uuid.UUID][]repository.PlaceExternalIDReadModel{kiel: kielIDs})
	if f, err := store.GetPlaceFacts(ctx, darmstadt); err != nil || f == nil || f.Version != 3 {
		t.Errorf("GetPlaceFacts(darmstadt) = %+v, %v; want version 3", f, err)
	}
}
