package command_test

import (
	"context"
	"encoding/json"
	"errors"
	"testing"

	"github.com/google/uuid"

	"github.com/cacack/my-family/internal/command"
	"github.com/cacack/my-family/internal/domain"
	"github.com/cacack/my-family/internal/repository/memory"
)

// familyFixture is a family of two partners plus a spare person, on a fresh
// in-memory handler.
type familyFixture struct {
	ctx        context.Context
	events     *memory.EventStore
	read       *memory.ReadModelStore
	handler    *command.Handler
	p1, p2, p3 uuid.UUID
	familyID   uuid.UUID
	version    int64
}

func newFamilyFixture(t *testing.T) *familyFixture {
	t.Helper()
	f := &familyFixture{ctx: context.Background(), events: memory.NewEventStore(), read: memory.NewReadModelStore()}
	f.handler = command.NewHandler(f.events, f.read)
	for i, name := range []string{"Avery", "Blake", "Cameron"} {
		res, err := f.handler.CreatePerson(f.ctx, command.CreatePersonInput{GivenName: name, Surname: "Placeholder"})
		if err != nil {
			t.Fatalf("create person %d: %v", i, err)
		}
		switch i {
		case 0:
			f.p1 = res.ID
		case 1:
			f.p2 = res.ID
		default:
			f.p3 = res.ID
		}
	}
	fam, err := f.handler.CreateFamily(f.ctx, command.CreateFamilyInput{Partner1ID: &f.p1, Partner2ID: &f.p2, RelationshipType: "marriage", MarriageDate: "1 JUN 1875"})
	if err != nil {
		t.Fatalf("create family: %v", err)
	}
	f.familyID, f.version = fam.ID, fam.Version
	return f
}

// TestUpdateFamily_ReflectedInReadModel is the #848 regression: partners,
// relationship type and marriage date set through the command must land in the
// read model on the synchronous projection.
func TestUpdateFamily_ReflectedInReadModel(t *testing.T) {
	f := newFamilyFixture(t)
	rel, date := "partnership", "ABT 1880"
	if _, err := f.handler.UpdateFamily(f.ctx, command.UpdateFamilyInput{
		ID: f.familyID, Partner1ID: &f.p3, Partner2ID: &f.p1, RelationshipType: &rel, MarriageDate: &date, Version: f.version,
	}); err != nil {
		t.Fatalf("UpdateFamily: %v", err)
	}

	fam, err := f.read.GetFamily(f.ctx, domain.MainBranchID, f.familyID)
	if err != nil || fam == nil {
		t.Fatalf("GetFamily: %v", err)
	}
	if fam.Partner1ID == nil || *fam.Partner1ID != f.p3 || fam.Partner1GivenName != "Cameron" {
		t.Errorf("partner1 = %v %q, want %s Cameron", fam.Partner1ID, fam.Partner1GivenName, f.p3)
	}
	if fam.Partner2ID == nil || *fam.Partner2ID != f.p1 || fam.Partner2GivenName != "Avery" {
		t.Errorf("partner2 = %v %q, want %s Avery", fam.Partner2ID, fam.Partner2GivenName, f.p1)
	}
	if fam.RelationshipType != domain.RelationPartnership {
		t.Errorf("RelationshipType = %q, want partnership", fam.RelationshipType)
	}
	if fam.MarriageDateRaw != "ABT 1880" || fam.MarriageDateSort == nil {
		t.Errorf("marriage date = %q (sort %v), want ABT 1880 with a sort key", fam.MarriageDateRaw, fam.MarriageDateSort)
	}
}

// TestUpdateFamily_StoresJSONShapedChanges pins the stored form: IDs and the
// relationship type as strings and the marriage date as its raw text — the
// same shape the rollback service writes and branch merge's partner scan reads.
func TestUpdateFamily_StoresJSONShapedChanges(t *testing.T) {
	f := newFamilyFixture(t)
	rel, date := "unknown", "12 MAR 1881"
	if _, err := f.handler.UpdateFamily(f.ctx, command.UpdateFamilyInput{
		ID: f.familyID, Partner1ID: &f.p3, RelationshipType: &rel, MarriageDate: &date, Version: f.version,
	}); err != nil {
		t.Fatalf("UpdateFamily: %v", err)
	}

	stream, err := f.events.ReadStream(f.ctx, f.familyID)
	if err != nil {
		t.Fatalf("ReadStream: %v", err)
	}
	last := stream[len(stream)-1]
	var payload struct {
		Changes map[string]json.RawMessage `json:"changes"`
	}
	if err := json.Unmarshal(last.Data, &payload); err != nil {
		t.Fatalf("decode stored event: %v", err)
	}
	want := map[string]string{
		"partner1_id":       `"` + f.p3.String() + `"`,
		"relationship_type": `"unknown"`,
		"marriage_date":     `"12 MAR 1881"`,
	}
	for key, raw := range want {
		if got := string(payload.Changes[key]); got != raw {
			t.Errorf("stored %s = %s, want %s", key, got, raw)
		}
	}

	// Clearing stores an explicit null.
	empty := ""
	if _, err := f.handler.UpdateFamily(f.ctx, command.UpdateFamilyInput{ID: f.familyID, MarriageDate: &empty, Version: f.version + 1}); err != nil {
		t.Fatalf("clear date: %v", err)
	}
	stream, _ = f.events.ReadStream(f.ctx, f.familyID)
	if err := json.Unmarshal(stream[len(stream)-1].Data, &payload); err != nil {
		t.Fatalf("decode stored event: %v", err)
	}
	if got := string(payload.Changes["marriage_date"]); got != "null" {
		t.Errorf("stored cleared marriage_date = %s, want null", got)
	}
	fam, _ := f.read.GetFamily(f.ctx, domain.MainBranchID, f.familyID)
	if fam.MarriageDateRaw != "" || fam.MarriageDateSort != nil {
		t.Errorf("cleared marriage date = %q (sort %v), want empty", fam.MarriageDateRaw, fam.MarriageDateSort)
	}
}

func TestUpdateFamily_ValidatesPartnersAndType(t *testing.T) {
	missing := uuid.New()
	badType := "situationship"
	tests := []struct {
		name    string
		input   func(f *familyFixture) command.UpdateFamilyInput
		wantErr error
	}{
		{
			name: "unknown partner1",
			input: func(f *familyFixture) command.UpdateFamilyInput {
				return command.UpdateFamilyInput{Partner1ID: &missing}
			},
			wantErr: command.ErrInvalidFamilyInput,
		},
		{
			name: "unknown partner2",
			input: func(f *familyFixture) command.UpdateFamilyInput {
				return command.UpdateFamilyInput{Partner2ID: &missing}
			},
			wantErr: command.ErrInvalidFamilyInput,
		},
		{
			name:    "partner1 set to the existing partner2",
			input:   func(f *familyFixture) command.UpdateFamilyInput { return command.UpdateFamilyInput{Partner1ID: &f.p2} },
			wantErr: command.ErrInvalidFamilyInput,
		},
		{
			name: "both partners the same person",
			input: func(f *familyFixture) command.UpdateFamilyInput {
				return command.UpdateFamilyInput{Partner1ID: &f.p3, Partner2ID: &f.p3}
			},
			wantErr: command.ErrInvalidFamilyInput,
		},
		{
			name: "invalid relationship type",
			input: func(f *familyFixture) command.UpdateFamilyInput {
				return command.UpdateFamilyInput{RelationshipType: &badType}
			},
			wantErr: command.ErrInvalidFamilyInput,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			f := newFamilyFixture(t)
			input := tt.input(f)
			input.ID, input.Version = f.familyID, f.version
			_, err := f.handler.UpdateFamily(f.ctx, input)
			if !errors.Is(err, tt.wantErr) {
				t.Fatalf("UpdateFamily error = %v, want %v", err, tt.wantErr)
			}
			// Nothing was appended.
			stream, _ := f.events.ReadStream(f.ctx, f.familyID)
			if len(stream) != 1 {
				t.Errorf("stream has %d events, want only the create", len(stream))
			}
		})
	}
}

// TestUpdateFamily_PartnerCannotBeDescendant: making a family's own child (or
// the child's descendant) one of its partners would make them their own
// ancestor, the cycle LinkChild refuses from the other direction.
func TestUpdateFamily_PartnerCannotBeDescendant(t *testing.T) {
	f := newFamilyFixture(t)
	if _, err := f.handler.LinkChild(f.ctx, command.LinkChildInput{FamilyID: f.familyID, ChildID: f.p3}); err != nil {
		t.Fatalf("LinkChild: %v", err)
	}
	fam, _ := f.read.GetFamily(f.ctx, domain.MainBranchID, f.familyID)

	// The child itself.
	if _, err := f.handler.UpdateFamily(f.ctx, command.UpdateFamilyInput{ID: f.familyID, Partner2ID: &f.p3, Version: fam.Version}); !errors.Is(err, command.ErrCircularAncestry) {
		t.Fatalf("child as partner: error = %v, want ErrCircularAncestry", err)
	}

	// A grandchild: p3 has a family whose child is g.
	g, err := f.handler.CreatePerson(f.ctx, command.CreatePersonInput{GivenName: "Dana", Surname: "Placeholder"})
	if err != nil {
		t.Fatalf("create grandchild: %v", err)
	}
	sub, err := f.handler.CreateFamily(f.ctx, command.CreateFamilyInput{Partner1ID: &f.p3})
	if err != nil {
		t.Fatalf("create child's family: %v", err)
	}
	if _, err := f.handler.LinkChild(f.ctx, command.LinkChildInput{FamilyID: sub.ID, ChildID: g.ID}); err != nil {
		t.Fatalf("link grandchild: %v", err)
	}
	if _, err := f.handler.UpdateFamily(f.ctx, command.UpdateFamilyInput{ID: f.familyID, Partner1ID: &g.ID, Version: fam.Version}); !errors.Is(err, command.ErrCircularAncestry) {
		t.Fatalf("grandchild as partner: error = %v, want ErrCircularAncestry", err)
	}

	// An unrelated person is fine.
	other, err := f.handler.CreatePerson(f.ctx, command.CreatePersonInput{GivenName: "Emery", Surname: "Sample"})
	if err != nil {
		t.Fatalf("create unrelated person: %v", err)
	}
	if _, err := f.handler.UpdateFamily(f.ctx, command.UpdateFamilyInput{ID: f.familyID, Partner2ID: &other.ID, Version: fam.Version}); err != nil {
		t.Fatalf("unrelated partner: %v", err)
	}
}
