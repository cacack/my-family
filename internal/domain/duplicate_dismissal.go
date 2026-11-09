package domain

import (
	"errors"

	"github.com/google/uuid"
)

// Duplicate-pair dismissal (#1036): a researcher's decision that two persons the
// duplicate detector pairs up are NOT the same person (a father and son whose
// "Jr." was never entered, say). The decision is an aggregate of its own, keyed
// by the pair rather than by either person, so dismissing and restoring it never
// touches either person's stream.

var (
	// ErrDuplicatePairSamePerson is returned for a pair naming one person twice.
	ErrDuplicatePairSamePerson = errors.New("a duplicate pair must name two different persons")
	// ErrDuplicatePairNilPerson is returned for a pair with a nil person id.
	ErrDuplicatePairNilPerson = errors.New("a duplicate pair must name two persons")
	// ErrDismissalReasonTooLong is returned for a dismissal reason over
	// MaxDismissalReasonLength characters.
	ErrDismissalReasonTooLong = errors.New("dismissal reason must be 500 characters or less")
)

// MaxDismissalReasonLength bounds a dismissal's free-text reason.
const MaxDismissalReasonLength = 500

// duplicatePairNamespace is the UUIDv5 namespace pair ids are derived in. It is
// a fixed constant: changing it would re-key every recorded dismissal.
var duplicatePairNamespace = uuid.MustParse("6f1d5c2e-3b8a-4c1e-9a57-0d2f4b8e1c36")

// DuplicatePair is an unordered pair of distinct persons, stored in canonical
// order (Person1ID sorts before Person2ID), so (a, b) and (b, a) are the same
// pair with the same ID.
type DuplicatePair struct {
	Person1ID uuid.UUID
	Person2ID uuid.UUID
}

// NewDuplicatePair returns the canonical pair of a and b.
func NewDuplicatePair(a, b uuid.UUID) (DuplicatePair, error) {
	if a == uuid.Nil || b == uuid.Nil {
		return DuplicatePair{}, ErrDuplicatePairNilPerson
	}
	if a == b {
		return DuplicatePair{}, ErrDuplicatePairSamePerson
	}
	if a.String() > b.String() {
		a, b = b, a
	}
	return DuplicatePair{Person1ID: a, Person2ID: b}, nil
}

// ID is the pair's stable aggregate id: a name-based UUID of the two person
// ids in canonical order, so every scope and every request names the same
// stream for the same two persons.
func (p DuplicatePair) ID() uuid.UUID {
	return uuid.NewSHA1(duplicatePairNamespace, []byte(p.Person1ID.String()+p.Person2ID.String()))
}

// ValidateDismissalReason checks a dismissal's optional reason.
func ValidateDismissalReason(reason string) error {
	if len([]rune(reason)) > MaxDismissalReasonLength {
		return ErrDismissalReasonTooLong
	}
	return nil
}
