package repository_test

import (
	"context"
	"reflect"
	"testing"

	"github.com/google/uuid"

	"github.com/cacack/my-family/internal/domain"
	"github.com/cacack/my-family/internal/repository"
	"github.com/cacack/my-family/internal/repository/memory"
)

// TestCanonicalizeChanges_MatchesDecodedEvent pins the #848 contract: after
// canonicalization a live *Updated event carries exactly the changes map
// DecodeEvent yields for the same event read back out of the log.
func TestCanonicalizeChanges_MatchesDecodedEvent(t *testing.T) {
	partner := uuid.New()
	md := domain.ParseGenDate("ABT 1880")
	events := []domain.Event{
		domain.NewFamilyUpdated(uuid.New(), map[string]any{
			"partner1_id":       &partner,
			"relationship_type": domain.RelationPartnership,
			"marriage_date":     &md,
			"marriage_place":    "Hall",
		}),
		domain.NewMediaUpdated(uuid.New(), map[string]any{"crop_left": 3, "title": "t"}),
		domain.NewSubmitterUpdated(uuid.New(), map[string]any{
			"address": &domain.Address{City: "Town"},
			"phone":   []string{"1", "2"},
			"media_id": func() *uuid.UUID {
				id := uuid.New()
				return &id
			}(),
		}),
		domain.NewAssociationUpdated(uuid.New(), map[string]any{"note_ids": []uuid.UUID{uuid.New()}}),
		domain.NewCitationUpdated(uuid.New(), map[string]any{"fields": map[string]string{"a": "b"}}),
	}

	for _, event := range events {
		t.Run(event.EventType(), func(t *testing.T) {
			canonical, err := repository.CanonicalizeChanges(event)
			if err != nil {
				t.Fatalf("CanonicalizeChanges: %v", err)
			}
			stored, err := repository.EncodeEvent(event.AggregateID(), "test", event, 1, 1)
			if err != nil {
				t.Fatalf("EncodeEvent: %v", err)
			}
			decoded, err := stored.DecodeEvent()
			if err != nil {
				t.Fatalf("DecodeEvent: %v", err)
			}
			got := reflect.ValueOf(canonical).FieldByName("Changes").Interface()
			want := reflect.ValueOf(decoded).FieldByName("Changes").Interface()
			if !reflect.DeepEqual(got, want) {
				t.Errorf("canonical changes = %#v\nwant decoded      %#v", got, want)
			}
			// The event's identity is untouched.
			if canonical.EventType() != event.EventType() || canonical.AggregateID() != event.AggregateID() || !canonical.OccurredAt().Equal(event.OccurredAt()) {
				t.Errorf("canonicalization changed the event's identity")
			}
		})
	}
}

func TestCanonicalizeChanges_LeavesOtherEventsAlone(t *testing.T) {
	person := domain.NewPerson("Test", "Person")
	created := domain.NewPersonCreated(person)
	got, err := repository.CanonicalizeChanges(created)
	if err != nil {
		t.Fatalf("CanonicalizeChanges: %v", err)
	}
	if !reflect.DeepEqual(got, created) {
		t.Errorf("a PersonCreated was altered")
	}

	// A nil changes map stays nil rather than becoming an empty map.
	update := domain.NewPersonUpdated(person.ID, nil)
	got, err = repository.CanonicalizeChanges(update)
	if err != nil {
		t.Fatalf("CanonicalizeChanges(nil changes): %v", err)
	}
	if changes := got.(domain.PersonUpdated).Changes; changes != nil {
		t.Errorf("nil changes became %v", changes)
	}

	// A pointer event is not a struct value; it passes through.
	ptr := &created
	if got, err := repository.CanonicalizeChanges(ptr); err != nil || got != domain.Event(ptr) {
		t.Errorf("pointer event: got %v, %v", got, err)
	}
}

func TestCanonicalizeChanges_UnencodableValue(t *testing.T) {
	event := domain.NewPersonUpdated(uuid.New(), map[string]any{"notes": make(chan int)})
	if _, err := repository.CanonicalizeChanges(event); err == nil {
		t.Fatal("expected an error for a value JSON cannot encode")
	}
}

// TestProjector_FamilyUpdated_DateForms covers every shape a marriage_date
// change can arrive in: the canonical raw string, nil to clear, a legacy
// serialized GenDate object (events stored before #848), and the typed forms.
func TestProjector_FamilyUpdated_DateForms(t *testing.T) {
	legacy := domain.ParseGenDate("ABT 1880")
	tests := []struct {
		name     string
		value    any
		wantRaw  string
		wantSort bool
	}{
		{"raw string", "12 MAR 1881", "12 MAR 1881", true},
		{"nil clears", nil, "", false},
		{"empty string clears", "", "", false},
		{"legacy object", map[string]any{"raw": "ABT 1880", "qualifier": "about", "year": float64(1880)}, "ABT 1880", true},
		{"legacy object without raw", map[string]any{"qualifier": "exact", "year": float64(1882)}, "1882", true},
		{"typed pointer", &legacy, "ABT 1880", true},
		{"typed nil pointer", (*domain.GenDate)(nil), "", false},
		{"typed value", legacy, "ABT 1880", true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctx := context.Background()
			readStore := memory.NewReadModelStore()
			projector := repository.NewProjector(readStore, nil)
			family := domain.NewFamily()
			family.SetMarriageDate("1 JAN 1870")
			if err := projector.Project(ctx, domain.NewFamilyCreated(family), 1, domain.MainBranchID); err != nil {
				t.Fatalf("create: %v", err)
			}
			update := domain.NewFamilyUpdated(family.ID, map[string]any{"marriage_date": tt.value})
			if err := projector.Project(ctx, update, 2, domain.MainBranchID); err != nil {
				t.Fatalf("update: %v", err)
			}
			rm, _ := readStore.GetFamily(ctx, domain.MainBranchID, family.ID)
			if rm.MarriageDateRaw != tt.wantRaw {
				t.Errorf("MarriageDateRaw = %q, want %q", rm.MarriageDateRaw, tt.wantRaw)
			}
			if (rm.MarriageDateSort != nil) != tt.wantSort {
				t.Errorf("MarriageDateSort = %v, want set=%v", rm.MarriageDateSort, tt.wantSort)
			}
		})
	}
}

func TestProjector_FamilyUpdated_RejectsUnsupportedDate(t *testing.T) {
	ctx := context.Background()
	readStore := memory.NewReadModelStore()
	projector := repository.NewProjector(readStore, nil)
	family := domain.NewFamily()
	if err := projector.Project(ctx, domain.NewFamilyCreated(family), 1, domain.MainBranchID); err != nil {
		t.Fatalf("create: %v", err)
	}
	for _, bad := range []any{42, map[string]any{"year": "not a number"}} {
		update := domain.NewFamilyUpdated(family.ID, map[string]any{"marriage_date": bad})
		if err := projector.Project(ctx, update, 2, domain.MainBranchID); err == nil {
			t.Errorf("marriage_date %v (%T): expected an error", bad, bad)
		}
	}
}

// TestProjector_FamilyUpdated_TypedValues keeps the projection tolerant of a
// typed map handed to it directly (not through a command), the pre-#848 shape.
func TestProjector_FamilyUpdated_TypedValues(t *testing.T) {
	ctx := context.Background()
	readStore := memory.NewReadModelStore()
	projector := repository.NewProjector(readStore, nil)
	partner := domain.NewPerson("Partner", "Typed")
	if err := projector.Project(ctx, domain.NewPersonCreated(partner), 1, domain.MainBranchID); err != nil {
		t.Fatalf("create person: %v", err)
	}
	family := domain.NewFamily()
	if err := projector.Project(ctx, domain.NewFamilyCreated(family), 1, domain.MainBranchID); err != nil {
		t.Fatalf("create family: %v", err)
	}
	update := domain.NewFamilyUpdated(family.ID, map[string]any{
		"partner1_id":       &partner.ID,
		"relationship_type": domain.RelationPartnership,
	})
	if err := projector.Project(ctx, update, 2, domain.MainBranchID); err != nil {
		t.Fatalf("update: %v", err)
	}
	rm, _ := readStore.GetFamily(ctx, domain.MainBranchID, family.ID)
	if rm.Partner1ID == nil || *rm.Partner1ID != partner.ID || rm.Partner1GivenName != "Partner" {
		t.Errorf("partner1 = %v %q, want %s Partner", rm.Partner1ID, rm.Partner1GivenName, partner.ID)
	}
	if rm.RelationshipType != domain.RelationPartnership {
		t.Errorf("RelationshipType = %q, want partnership", rm.RelationshipType)
	}
}

// TestProjector_SubmitterUpdated_JSONForms: a submitter update decoded from the
// log carries a map address, []any string lists and a string media ID. Before
// #848 the projection only understood the typed forms, so a replay dropped them.
func TestProjector_SubmitterUpdated_JSONForms(t *testing.T) {
	ctx := context.Background()
	readStore := memory.NewReadModelStore()
	projector := repository.NewProjector(readStore, nil)
	sub := domain.NewSubmitter("Submitter")
	if err := projector.Project(ctx, domain.NewSubmitterCreated(sub), 1, domain.MainBranchID); err != nil {
		t.Fatalf("create: %v", err)
	}
	mediaID := uuid.New()
	update := domain.NewSubmitterUpdated(sub.ID, map[string]any{
		"address":  map[string]any{"city": "Town"},
		"phone":    []any{"555"},
		"email":    []any{"a@example.test", "b@example.test"},
		"media_id": mediaID.String(),
	})
	if err := projector.Project(ctx, update, 2, domain.MainBranchID); err != nil {
		t.Fatalf("update: %v", err)
	}
	rm, _ := readStore.GetSubmitter(ctx, sub.ID)
	if rm.Address == nil || rm.Address.City != "Town" {
		t.Errorf("Address = %+v, want city Town", rm.Address)
	}
	if !reflect.DeepEqual(rm.Phone, []string{"555"}) || !reflect.DeepEqual(rm.Email, []string{"a@example.test", "b@example.test"}) {
		t.Errorf("Phone/Email = %v / %v", rm.Phone, rm.Email)
	}
	if rm.MediaID == nil || *rm.MediaID != mediaID {
		t.Errorf("MediaID = %v, want %s", rm.MediaID, mediaID)
	}

	// nil clears the address and the media link.
	clearing := domain.NewSubmitterUpdated(sub.ID, map[string]any{"address": nil, "media_id": nil})
	if err := projector.Project(ctx, clearing, 3, domain.MainBranchID); err != nil {
		t.Fatalf("clear: %v", err)
	}
	rm, _ = readStore.GetSubmitter(ctx, sub.ID)
	if rm.Address != nil || rm.MediaID != nil {
		t.Errorf("after clear Address=%v MediaID=%v, want both nil", rm.Address, rm.MediaID)
	}

	for key, bad := range map[string]any{"address": "not an address", "phone": 7, "email": map[string]any{"x": 1}} {
		update := domain.NewSubmitterUpdated(sub.ID, map[string]any{key: bad})
		if err := projector.Project(ctx, update, 4, domain.MainBranchID); err == nil {
			t.Errorf("%s = %v: expected an error", key, bad)
		}
	}
}

func TestProjector_AddressChanges_RejectUndecodable(t *testing.T) {
	ctx := context.Background()
	readStore := memory.NewReadModelStore()
	projector := repository.NewProjector(readStore, nil)

	repo := domain.NewRepository("Archive")
	if err := projector.Project(ctx, domain.NewRepositoryCreated(repo), 1, domain.MainBranchID); err != nil {
		t.Fatalf("create repository: %v", err)
	}
	bad := domain.NewRepositoryUpdated(repo.ID, map[string]any{"address": "nowhere"})
	if err := projector.Project(ctx, bad, 2, domain.MainBranchID); err == nil {
		t.Error("repository address: expected an error for an undecodable value")
	}
}
