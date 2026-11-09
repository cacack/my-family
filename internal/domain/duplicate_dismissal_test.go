package domain

import (
	"errors"
	"strings"
	"testing"

	"github.com/google/uuid"
)

func TestNewDuplicatePair_IsOrderIndependent(t *testing.T) {
	a := uuid.MustParse("bbbbbbbb-0000-0000-0000-000000000000")
	b := uuid.MustParse("aaaaaaaa-0000-0000-0000-000000000000")
	ab, err := NewDuplicatePair(a, b)
	if err != nil {
		t.Fatalf("NewDuplicatePair(a, b): %v", err)
	}
	ba, err := NewDuplicatePair(b, a)
	if err != nil {
		t.Fatalf("NewDuplicatePair(b, a): %v", err)
	}
	if ab != ba {
		t.Errorf("pairs differ: %+v vs %+v", ab, ba)
	}
	if ab.Person1ID != b || ab.Person2ID != a {
		t.Errorf("pair not in canonical order: %+v", ab)
	}
	if ab.ID() != ba.ID() || ab.ID() == uuid.Nil {
		t.Errorf("pair ids %s / %s, want one stable non-nil id", ab.ID(), ba.ID())
	}
	other, _ := NewDuplicatePair(a, uuid.New())
	if other.ID() == ab.ID() {
		t.Error("different pairs share an id")
	}
}

func TestNewDuplicatePair_Rejects(t *testing.T) {
	a := uuid.New()
	if _, err := NewDuplicatePair(a, a); !errors.Is(err, ErrDuplicatePairSamePerson) {
		t.Errorf("same person: %v", err)
	}
	if _, err := NewDuplicatePair(a, uuid.Nil); !errors.Is(err, ErrDuplicatePairNilPerson) {
		t.Errorf("nil person: %v", err)
	}
}

func TestValidateDismissalReason(t *testing.T) {
	if err := ValidateDismissalReason(strings.Repeat("é", MaxDismissalReasonLength)); err != nil {
		t.Errorf("max-length reason rejected: %v", err)
	}
	if err := ValidateDismissalReason(strings.Repeat("x", MaxDismissalReasonLength+1)); !errors.Is(err, ErrDismissalReasonTooLong) {
		t.Errorf("long reason: %v", err)
	}
}

func TestDuplicateEvents(t *testing.T) {
	pair, _ := NewDuplicatePair(uuid.New(), uuid.New())
	dismissed := NewDuplicateDismissed(pair, "father and son")
	if dismissed.EventType() != "DuplicateDismissed" || dismissed.AggregateID() != pair.ID() || dismissed.OccurredAt().IsZero() {
		t.Errorf("DuplicateDismissed = %+v", dismissed)
	}
	restored := NewDuplicateRestored(pair)
	if restored.EventType() != "DuplicateRestored" || restored.AggregateID() != pair.ID() {
		t.Errorf("DuplicateRestored = %+v", restored)
	}
}
