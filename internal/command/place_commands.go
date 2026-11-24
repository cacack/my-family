package command

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/google/uuid"

	"github.com/cacack/my-family/internal/domain"
	"github.com/cacack/my-family/internal/repository"
)

// ErrPlaceNotFound is returned for a place ID that no record on main names
// and no fact has been stated about.
var ErrPlaceNotFound = errors.New("place not found")

// placeStreamType is the stream type of a place's own stream (ADR-009 §2).
const placeStreamType = "Place"

// placeCoordinateNames are the API fields and change keys of a place's pair.
var placeCoordinateNames = coordinateNames{
	place: "name", lat: "latitude", long: "longitude", origin: "coordinates_origin",
	latKey: "latitude", longKey: "longitude", originKey: "coordinates_origin",
}

// UpdatePlaceInput sets or clears facts about a place (ADR-009 §2). Version
// is the place's stream version as last read, 0 before any fact is stated.
// Name is its display name, which a clear or a blank value removes, so the
// place shows its most-used spelling again; Coordinates is its pair, with
// the origin vocabulary of #1563.
type UpdatePlaceInput struct {
	ID          uuid.UUID
	Version     int64
	Name        Patch[string]
	Coordinates CoordinatesPatch
}

// UpdatePlaceResult is the place's stream version after the update.
type UpdatePlaceResult struct {
	Version int64
}

// UpdatePlace appends one PlaceUpdated to the place's own stream, and to no
// person or family stream, when the input changes a fact. Place facts are
// main-only (PL-005): on a branch-scoped handler the append is refused with
// ErrEventTypeNotBranchAware. A place exists once main names it or a fact
// about it has been stated; any other ID is ErrPlaceNotFound.
func (h *Handler) UpdatePlace(ctx context.Context, input UpdatePlaceInput) (*UpdatePlaceResult, error) {
	facts, err := h.readStore.GetPlaceFacts(ctx, input.ID)
	if err != nil {
		return nil, fmt.Errorf("getting place facts: %w", err)
	}
	key, text, err := h.placeKeyAndText(ctx, input.ID)
	if err != nil {
		return nil, err
	}
	current := repository.PlaceFactsReadModel{ID: input.ID, Key: key}
	if facts != nil {
		current = *facts
	}
	if current.Version != input.Version {
		return nil, repository.ErrConcurrencyConflict
	}

	changes := make(map[string]any)
	if name, ok := input.Name.Change(); ok {
		if name = strings.TrimSpace(name); name != current.Name {
			changes["name"] = name
		}
	}
	if err := recordPair(changes, placeCoordinateNames, text, input.Coordinates,
		placeCoordinates{Lat: current.Latitude, Long: current.Longitude, Origin: current.Origin}); err != nil {
		return nil, err
	}
	if len(changes) == 0 {
		return &UpdatePlaceResult{Version: current.Version}, nil
	}

	event := domain.NewPlaceUpdated(input.ID, key, text, changes)
	version, err := h.execute(ctx, input.ID.String(), placeStreamType, []domain.Event{event}, input.Version)
	if err != nil {
		return nil, fmt.Errorf("executing update place command: %w", err)
	}
	return &UpdatePlaceResult{Version: version}, nil
}

// placeKeyAndText finds the key of the place with id and the text a place
// event records beside it, the name the place shows on main. ListPlaces holds
// every place main names and, for a principal who sees everything, every
// place with facts.
func (h *Handler) placeKeyAndText(ctx context.Context, id uuid.UUID) (key, text string, err error) {
	places, err := h.readStore.ListPlaces(ctx, domain.MainBranchID, repository.Privacy{})
	if err != nil {
		return "", "", fmt.Errorf("listing places: %w", err)
	}
	for _, p := range places {
		if p.ID == id {
			return p.Key, p.Name, nil
		}
	}
	return "", "", ErrPlaceNotFound
}
