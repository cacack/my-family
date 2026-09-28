package command_test

import (
	"encoding/json"
	"errors"
	"testing"

	"github.com/google/uuid"

	"github.com/cacack/my-family/internal/command"
	"github.com/cacack/my-family/internal/domain"
)

// TestUpdateFamily_ClearsPartner (#826): the family form can remove a partner.
// The change is stored as an explicit null and the read model drops the
// partner and its denormalized name.
func TestUpdateFamily_ClearsPartner(t *testing.T) {
	f := newFamilyFixture(t)
	if _, err := f.handler.UpdateFamily(f.ctx, command.UpdateFamilyInput{
		ID: f.familyID, ClearPartner2: true, Version: f.version,
	}); err != nil {
		t.Fatalf("UpdateFamily: %v", err)
	}

	fam, err := f.read.GetFamily(f.ctx, domain.MainBranchID, f.familyID)
	if err != nil || fam == nil {
		t.Fatalf("GetFamily: %v", err)
	}
	if fam.Partner2ID != nil || fam.Partner2GivenName != "" || fam.Partner2Surname != "" {
		t.Errorf("partner2 = %v %q %q, want cleared", fam.Partner2ID, fam.Partner2GivenName, fam.Partner2Surname)
	}
	if fam.Partner1ID == nil || *fam.Partner1ID != f.p1 {
		t.Errorf("partner1 = %v, want untouched %s", fam.Partner1ID, f.p1)
	}

	stream, err := f.events.ReadStream(f.ctx, f.familyID)
	if err != nil {
		t.Fatalf("ReadStream: %v", err)
	}
	var payload struct {
		Changes map[string]json.RawMessage `json:"changes"`
	}
	if err := json.Unmarshal(stream[len(stream)-1].Data, &payload); err != nil {
		t.Fatalf("decode stored event: %v", err)
	}
	if got := string(payload.Changes["partner2_id"]); got != "null" {
		t.Errorf("stored partner2_id = %s, want null", got)
	}
	if _, ok := payload.Changes["partner1_id"]; ok {
		t.Errorf("stored changes carry partner1_id, want only partner2_id")
	}

	// Clearing an already-empty slot is a no-op: nothing is appended.
	if _, err := f.handler.UpdateFamily(f.ctx, command.UpdateFamilyInput{
		ID: f.familyID, ClearPartner2: true, Version: f.version + 1,
	}); err != nil {
		t.Fatalf("second clear: %v", err)
	}
	after, err := f.events.ReadStream(f.ctx, f.familyID)
	if err != nil {
		t.Fatalf("ReadStream after no-op clear: %v", err)
	}
	if len(after) != len(stream) {
		t.Errorf("stream has %d events after a no-op clear, want %d", len(after), len(stream))
	}
}

// TestUpdateFamily_ClearAndSwapPartners: clearing partner1 while moving that
// person to partner2 is one valid update — the distinct-partner check runs on
// the state after the update, not before.
func TestUpdateFamily_ClearAndSwapPartners(t *testing.T) {
	f := newFamilyFixture(t)
	if _, err := f.handler.UpdateFamily(f.ctx, command.UpdateFamilyInput{
		ID: f.familyID, ClearPartner1: true, Partner2ID: &f.p1, Version: f.version,
	}); err != nil {
		t.Fatalf("UpdateFamily: %v", err)
	}
	fam, err := f.read.GetFamily(f.ctx, domain.MainBranchID, f.familyID)
	if err != nil || fam == nil {
		t.Fatalf("GetFamily: %v (family %v)", err, fam)
	}
	if fam.Partner1ID != nil {
		t.Errorf("partner1 = %v, want cleared", fam.Partner1ID)
	}
	if fam.Partner2ID == nil || *fam.Partner2ID != f.p1 || fam.Partner2GivenName != "Avery" {
		t.Errorf("partner2 = %v %q, want %s Avery", fam.Partner2ID, fam.Partner2GivenName, f.p1)
	}
}

// TestUpdateFamily_SetAndClearSamePartnerRefused: one update cannot both set
// and clear the same partner.
func TestUpdateFamily_SetAndClearSamePartnerRefused(t *testing.T) {
	for _, tc := range []struct {
		name  string
		input func(f *familyFixture) command.UpdateFamilyInput
	}{
		{"partner1", func(f *familyFixture) command.UpdateFamilyInput {
			return command.UpdateFamilyInput{Partner1ID: &f.p3, ClearPartner1: true}
		}},
		{"partner2", func(f *familyFixture) command.UpdateFamilyInput {
			return command.UpdateFamilyInput{Partner2ID: &f.p3, ClearPartner2: true}
		}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			f := newFamilyFixture(t)
			input := tc.input(f)
			input.ID, input.Version = f.familyID, f.version
			if _, err := f.handler.UpdateFamily(f.ctx, input); !errors.Is(err, command.ErrInvalidFamilyInput) {
				t.Fatalf("UpdateFamily error = %v, want ErrInvalidFamilyInput", err)
			}
			stream, err := f.events.ReadStream(f.ctx, f.familyID)
			if err != nil {
				t.Fatalf("ReadStream: %v", err)
			}
			if len(stream) != 1 {
				t.Errorf("stream has %d events, want only the create", len(stream))
			}
		})
	}
}

// TestUpdateFamily_CannotClearLastPartner (#826): a family must keep at least
// one partner, the rule CreateFamily and domain.Family.Validate enforce.
// Clearing both at once, or clearing the only remaining one, is refused and
// nothing is appended.
func TestUpdateFamily_CannotClearLastPartner(t *testing.T) {
	t.Run("both at once", func(t *testing.T) {
		f := newFamilyFixture(t)
		_, err := f.handler.UpdateFamily(f.ctx, command.UpdateFamilyInput{
			ID: f.familyID, ClearPartner1: true, ClearPartner2: true, Version: f.version,
		})
		if !errors.Is(err, command.ErrInvalidFamilyInput) {
			t.Fatalf("UpdateFamily error = %v, want ErrInvalidFamilyInput", err)
		}
		assertStreamLen(t, f, 1)
	})
	t.Run("last remaining", func(t *testing.T) {
		f := newFamilyFixture(t)
		if _, err := f.handler.UpdateFamily(f.ctx, command.UpdateFamilyInput{
			ID: f.familyID, ClearPartner2: true, Version: f.version,
		}); err != nil {
			t.Fatalf("clear partner2: %v", err)
		}
		_, err := f.handler.UpdateFamily(f.ctx, command.UpdateFamilyInput{
			ID: f.familyID, ClearPartner1: true, Version: f.version + 1,
		})
		if !errors.Is(err, command.ErrInvalidFamilyInput) {
			t.Fatalf("UpdateFamily error = %v, want ErrInvalidFamilyInput", err)
		}
		assertStreamLen(t, f, 2)
		fam, err := f.read.GetFamily(f.ctx, domain.MainBranchID, f.familyID)
		if err != nil || fam == nil {
			t.Fatalf("GetFamily: %v", err)
		}
		if fam.Partner1ID == nil || *fam.Partner1ID != f.p1 {
			t.Errorf("partner1 = %v, want kept %s", fam.Partner1ID, f.p1)
		}
	})
}

// TestCreateFamily_ValidationErrorsWrapSentinel: CreateFamily's input errors
// wrap ErrInvalidFamilyInput so the API maps them to 400, not 500.
func TestCreateFamily_ValidationErrorsWrapSentinel(t *testing.T) {
	f := newFamilyFixture(t)
	missing := uuid.New()
	for _, tc := range []struct {
		name  string
		input command.CreateFamilyInput
	}{
		{"no partners", command.CreateFamilyInput{RelationshipType: "marriage"}},
		{"unknown partner1", command.CreateFamilyInput{Partner1ID: &missing}},
		{"unknown partner2", command.CreateFamilyInput{Partner1ID: &f.p3, Partner2ID: &missing}},
		{"same partner twice", command.CreateFamilyInput{Partner1ID: &f.p3, Partner2ID: &f.p3}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if _, err := f.handler.CreateFamily(f.ctx, tc.input); !errors.Is(err, command.ErrInvalidFamilyInput) {
				t.Fatalf("CreateFamily error = %v, want ErrInvalidFamilyInput", err)
			}
		})
	}
}

func assertStreamLen(t *testing.T, f *familyFixture, want int) {
	t.Helper()
	stream, err := f.events.ReadStream(f.ctx, f.familyID)
	if err != nil {
		t.Fatalf("ReadStream: %v", err)
	}
	if len(stream) != want {
		t.Errorf("stream has %d events, want %d", len(stream), want)
	}
}
