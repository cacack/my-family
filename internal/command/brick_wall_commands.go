package command

import (
	"context"
	"fmt"

	"github.com/google/uuid"

	"github.com/cacack/my-family/internal/domain"
)

// SetBrickWall marks a person as a brick wall with a note (#802), replacing
// any earlier brick wall, resolved or not. It returns ErrPersonNotFound when
// the person does not exist.
//
// Brick walls are main-only for now: BrickWallSet is not branch-aware, so a
// branch-scoped handler refuses it (ErrEventTypeNotBranchAware). Scoping them
// to branches is follow-up work under #676.
func (h *Handler) SetBrickWall(ctx context.Context, personID uuid.UUID, note string) error {
	person, err := h.readStore.GetPerson(ctx, h.branchID, personID)
	if err != nil {
		return fmt.Errorf("getting person: %w", err)
	}
	if person == nil {
		return ErrPersonNotFound
	}
	event := domain.NewBrickWallSet(personID, note)
	if _, err := h.execute(ctx, personID.String(), "Person", []domain.Event{event}, person.Version); err != nil {
		return fmt.Errorf("executing set brick wall command: %w", err)
	}
	return nil
}

// ResolveBrickWall marks a person's brick wall as broken through (#802). A
// person with no unresolved brick wall is left as it is. It returns
// ErrPersonNotFound when the person does not exist. Main-only, like
// SetBrickWall.
func (h *Handler) ResolveBrickWall(ctx context.Context, personID uuid.UUID) error {
	person, err := h.readStore.GetPerson(ctx, h.branchID, personID)
	if err != nil {
		return fmt.Errorf("getting person: %w", err)
	}
	if person == nil {
		return ErrPersonNotFound
	}
	if person.BrickWallSince == nil || person.BrickWallResolvedAt != nil {
		return nil
	}
	event := domain.NewBrickWallResolved(personID, person.BrickWallNote, *person.BrickWallSince)
	if _, err := h.execute(ctx, personID.String(), "Person", []domain.Event{event}, person.Version); err != nil {
		return fmt.Errorf("executing resolve brick wall command: %w", err)
	}
	return nil
}
