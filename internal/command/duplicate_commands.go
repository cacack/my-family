package command

import (
	"context"
	"errors"
	"fmt"

	"github.com/google/uuid"

	"github.com/cacack/my-family/internal/domain"
)

// ErrDuplicateNotDismissed is returned when restoring a pair that is not
// dismissed on the handler's scope.
var ErrDuplicateNotDismissed = errors.New("duplicate pair is not dismissed")

// duplicatePairStreamType is the stream type of a duplicate pair's dismissal
// aggregate (#1036).
const duplicatePairStreamType = "DuplicatePair"

// DismissDuplicateInput names a suggested duplicate pair to mark as not a
// duplicate, with an optional reason.
type DismissDuplicateInput struct {
	Person1ID uuid.UUID
	Person2ID uuid.UUID
	Reason    string
}

// DismissDuplicate records that two persons are not duplicates (#1036), on the
// handler's scope. Both persons must exist there. Dismissing a pair that is
// already dismissed is a no-op. The order of the two persons does not matter.
//
// Dismissals carry no expected version: the decision is idempotent per pair,
// so two concurrent dismissals of the same pair both mean the same thing.
func (h *Handler) DismissDuplicate(ctx context.Context, input DismissDuplicateInput) error {
	pair, err := duplicatePair(input.Person1ID, input.Person2ID)
	if err != nil {
		return err
	}
	if err := domain.ValidateDismissalReason(input.Reason); err != nil {
		return fmt.Errorf("%w: %v", ErrInvalidInput, err)
	}
	for _, id := range []uuid.UUID{input.Person1ID, input.Person2ID} {
		person, err := h.readStore.GetPerson(ctx, h.branchID, id)
		if err != nil {
			return fmt.Errorf("getting person: %w", err)
		}
		if person == nil {
			return fmt.Errorf("%w: %s", ErrPersonNotFound, id)
		}
	}

	existing, err := h.readStore.GetDuplicateDismissal(ctx, h.branchID, pair.ID())
	if err != nil {
		return fmt.Errorf("getting duplicate dismissal: %w", err)
	}
	if existing != nil {
		return nil
	}

	event := domain.NewDuplicateDismissed(pair, input.Reason)
	if _, err := h.execute(ctx, pair.ID().String(), duplicatePairStreamType, []domain.Event{event}, -1); err != nil {
		return fmt.Errorf("executing dismiss duplicate command: %w", err)
	}
	return nil
}

// RestoreDuplicate undoes a dismissal on the handler's scope, so the detector
// may suggest the pair again. It returns ErrDuplicateNotDismissed when the
// pair is not dismissed there. The persons need not exist any more.
func (h *Handler) RestoreDuplicate(ctx context.Context, person1ID, person2ID uuid.UUID) error {
	pair, err := duplicatePair(person1ID, person2ID)
	if err != nil {
		return err
	}
	existing, err := h.readStore.GetDuplicateDismissal(ctx, h.branchID, pair.ID())
	if err != nil {
		return fmt.Errorf("getting duplicate dismissal: %w", err)
	}
	if existing == nil {
		return ErrDuplicateNotDismissed
	}

	event := domain.NewDuplicateRestored(pair)
	if _, err := h.execute(ctx, pair.ID().String(), duplicatePairStreamType, []domain.Event{event}, -1); err != nil {
		return fmt.Errorf("executing restore duplicate command: %w", err)
	}
	return nil
}

// duplicatePair builds the canonical pair, reporting a malformed one as
// ErrInvalidInput.
func duplicatePair(a, b uuid.UUID) (domain.DuplicatePair, error) {
	pair, err := domain.NewDuplicatePair(a, b)
	if err != nil {
		return domain.DuplicatePair{}, fmt.Errorf("%w: %v", ErrInvalidInput, err)
	}
	return pair, nil
}
