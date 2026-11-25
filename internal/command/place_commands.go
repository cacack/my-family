package command

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"strings"

	"github.com/google/uuid"

	"github.com/cacack/my-family/internal/domain"
	"github.com/cacack/my-family/internal/gedcom"
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
// the origin vocabulary of #1563. Parent is the ID of its explicit parent
// (ADR-009 §4), which a clear removes, so the place sits under its default
// parent again.
type UpdatePlaceInput struct {
	ID          uuid.UUID
	Version     int64
	Name        Patch[string]
	Coordinates CoordinatesPatch
	Parent      Patch[uuid.UUID]
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
	if err := h.recordPlaceParent(ctx, changes, key, current.ParentKey, input.Parent); err != nil {
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

// recordPlaceParent records a change of the place with key's explicit parent
// under "parent", as the parent's key ("" to clear), when parent changes it
// from current. A parent that is not a place in use on main, and one that
// would close a cycle counted through aliases and default parents, the place
// itself included, are ErrInvalidInput (ADR-009 §4). Two concurrent commands
// can both pass; the projector then decides, in log order (PL-004).
func (h *Handler) recordPlaceParent(ctx context.Context, changes map[string]any, key, current string, parent Patch[uuid.UUID]) error {
	if parent.Omitted() {
		return nil
	}
	parentKey := ""
	if id, ok := parent.Value(); ok {
		p, err := h.mainPlace(ctx, id)
		if errors.Is(err, ErrPlaceNotFound) {
			return fmt.Errorf("%w: parent place %s not found", ErrInvalidInput, id)
		}
		if err != nil {
			return err
		}
		parentKey = p.Key
		parents, aliases, err := h.placeGraph(ctx)
		if err != nil {
			return err
		}
		if repository.PlaceParentClosesCycle(parents, aliases, key, parentKey) {
			return fmt.Errorf("%w: putting this place under %s would place it beneath itself", ErrInvalidInput, p.Spelling)
		}
	}
	if parentKey != current {
		changes["parent"] = parentKey
	}
	return nil
}

// placeGraph returns main's explicit place parents and place aliases, what a
// place's parent is read from (ADR-009 §4).
func (h *Handler) placeGraph(ctx context.Context) (repository.PlaceParents, repository.PlaceAliases, error) {
	facts, err := h.readStore.ListPlaceFacts(ctx, domain.MainBranchID)
	if err != nil {
		return nil, nil, fmt.Errorf("listing place facts: %w", err)
	}
	aliases, err := h.readStore.ListPlaceAliases(ctx)
	if err != nil {
		return nil, nil, fmt.Errorf("listing place aliases: %w", err)
	}
	return repository.PlaceParentsByKey(facts), repository.PlaceAliasesByKey(aliases), nil
}

// placeKeyAndText finds the key of the place with id and the text a place
// event records beside it: the place's recorded spelling on main, never its
// display name, so the text is what the key was computed from.
func (h *Handler) placeKeyAndText(ctx context.Context, id uuid.UUID) (key, text string, err error) {
	p, err := h.mainPlace(ctx, id)
	if err != nil {
		return "", "", err
	}
	return p.Key, p.Spelling, nil
}

// mainPlace returns the place with id as main reads it for a principal who
// sees everything, or ErrPlaceNotFound. ListPlaces holds every place main
// names, absorbed ones included, and every place with facts.
func (h *Handler) mainPlace(ctx context.Context, id uuid.UUID) (*repository.PlaceReadModel, error) {
	places, err := h.readStore.ListPlaces(ctx, domain.MainBranchID, repository.Privacy{})
	if err != nil {
		return nil, fmt.Errorf("listing places: %w", err)
	}
	for i := range places {
		if places[i].ID == id {
			return &places[i], nil
		}
	}
	return nil, ErrPlaceNotFound
}

// AliasPlaceInput says that the place with ID means the place with TargetID
// (ADR-009 §3). Version is the absorbed place's stream version as last read.
type AliasPlaceInput struct {
	ID       uuid.UUID
	TargetID uuid.UUID
	Version  int64
}

// AliasPlace appends one PlaceAliased to the stream of the place being
// absorbed, and to no other stream; no record's text changes (PL-001). It
// rejects, with ErrInvalidInput, a place that is already an alias, a target
// that is itself an alias, a target that is not a place, and an alias that
// would close a cycle, the place itself included, or would put the target
// beneath itself in the place hierarchy (ADR-009 §4). Places already aliased to
// this one follow it to the target. Two concurrent commands
// can both pass these checks; the projector then decides, in log order
// (PL-004). Main-only (PL-005).
func (h *Handler) AliasPlace(ctx context.Context, input AliasPlaceInput) (*UpdatePlaceResult, error) {
	place, err := h.mainPlace(ctx, input.ID)
	if err != nil {
		return nil, err
	}
	target, err := h.mainPlace(ctx, input.TargetID)
	if errors.Is(err, ErrPlaceNotFound) {
		return nil, fmt.Errorf("%w: target place %s not found", ErrInvalidInput, input.TargetID)
	}
	if err != nil {
		return nil, err
	}
	if place.Version != input.Version {
		return nil, repository.ErrConcurrencyConflict
	}
	if place.ResolvesTo != nil {
		return nil, fmt.Errorf("%w: %s is already an alias of another place; separate it first", ErrInvalidInput, place.Spelling)
	}
	if target.ResolvesTo != nil {
		return nil, fmt.Errorf("%w: %s is itself an alias of another place", ErrInvalidInput, target.Spelling)
	}
	aliases, err := h.readStore.ListPlaceAliases(ctx)
	if err != nil {
		return nil, fmt.Errorf("listing place aliases: %w", err)
	}
	if repository.PlaceAliasClosesCycle(aliases, place.Key, target.Key) {
		return nil, fmt.Errorf("%w: aliasing %s to %s would close a cycle", ErrInvalidInput, place.Spelling, target.Spelling)
	}
	parents, _, err := h.placeGraph(ctx)
	if err != nil {
		return nil, err
	}
	stated, _ := repository.StatePlaceAlias(aliases, place.Key, target.Key)
	if repository.PlaceAliasClosesParentCycle(parents, repository.PlaceAliasesByKey(stated), target.Key) {
		return nil, fmt.Errorf("%w: aliasing %s to %s would place %s beneath itself", ErrInvalidInput, place.Spelling, target.Spelling, target.Spelling)
	}

	event := domain.NewPlaceAliased(place.Key, place.Spelling, target.Key, target.Spelling, place.ID, target.ID)
	version, err := h.execute(ctx, input.ID.String(), placeStreamType, []domain.Event{event}, input.Version)
	if err != nil {
		return nil, fmt.Errorf("executing alias place command: %w", err)
	}
	return &UpdatePlaceResult{Version: version}, nil
}

// UnaliasPlaceInput separates the place with ID from the place it was
// aliased to. Version is its stream version as last read.
type UnaliasPlaceInput struct {
	ID      uuid.UUID
	Version int64
}

// UnaliasPlace appends one PlaceUnaliased to the place's stream, so its key
// names the place itself again (ADR-009 §3). A place that is not an alias is
// ErrInvalidInput. Main-only (PL-005).
func (h *Handler) UnaliasPlace(ctx context.Context, input UnaliasPlaceInput) (*UpdatePlaceResult, error) {
	place, err := h.mainPlace(ctx, input.ID)
	if err != nil {
		return nil, err
	}
	if place.Version != input.Version {
		return nil, repository.ErrConcurrencyConflict
	}
	if place.ResolvesTo == nil {
		return nil, fmt.Errorf("%w: %s is not an alias of another place", ErrInvalidInput, place.Spelling)
	}

	event := domain.NewPlaceUnaliased(place.ID, place.Key, place.Spelling)
	version, err := h.execute(ctx, input.ID.String(), placeStreamType, []domain.Event{event}, input.Version)
	if err != nil {
		return nil, fmt.Errorf("executing unalias place command: %w", err)
	}
	return &UpdatePlaceResult{Version: version}, nil
}

// importedPlace is what an import has gathered for one place: its forms by
// positional key and its EXIDs, the stored ones first, and whether the file
// changed either.
type importedPlace struct {
	key, text                string
	forms                    map[string]string
	ids                      []domain.ExternalIdentifier
	formsChanged, idsChanged bool
}

// importPlaceFacts stores the forms and EXIDs an import read from its PLAC
// lines on the place each text's key names, before alias resolution, with one
// PlaceUpdated per place it changes (ADR-009 §8). A form is filed under the
// positional key of the text it came with. It is set for a part layout that
// has none; a different form for a layout that has one, stored or set earlier
// in the file, is reported as a warning and not applied, so the first form in
// file order wins (places is in file order). An EXID the place does not hold
// yet is added after the ones it does.
func (h *Handler) importPlaceFacts(ctx context.Context, places []gedcom.PlaceData, result *ImportGedcomResult) error {
	if len(places) == 0 {
		return nil
	}
	storedForms, err := h.readStore.ListPlaceForms(ctx)
	if err != nil {
		return fmt.Errorf("listing place forms: %w", err)
	}
	storedIDs, err := h.readStore.ListPlaceExternalIDs(ctx)
	if err != nil {
		return fmt.Errorf("listing place external IDs: %w", err)
	}

	byID := make(map[uuid.UUID]*importedPlace)
	var order []uuid.UUID
	warned := make(map[string]bool)
	for _, pd := range places {
		key := repository.PlaceKey(pd.Text)
		id := repository.PlaceID(key)
		p := byID[id]
		if p == nil {
			p = &importedPlace{key: key, text: pd.Text, forms: make(map[string]string)}
			for _, f := range storedForms[id] {
				p.forms[f.PositionalKey] = f.Form
			}
			for _, x := range storedIDs[id] {
				p.ids = append(p.ids, domain.ExternalIdentifier{Value: x.Value, Type: x.Type})
			}
			byID[id] = p
			order = append(order, id)
		}
		if pd.Form != "" {
			positional := repository.PositionalPlaceKey(pd.Text)
			switch current, ok := p.forms[positional]; {
			case !ok:
				p.forms[positional] = pd.Form
				p.formsChanged = true
			case current != pd.Form && !warned[positional+"\x00"+pd.Form]:
				warned[positional+"\x00"+pd.Form] = true
				result.Warnings = append(result.Warnings, fmt.Sprintf(
					"%s: %s place %q has the form %q, but %q is already the form of places written that way; not applied",
					pd.Record, pd.Tag, pd.Text, pd.Form, current))
			}
		}
		for _, x := range pd.ExternalIDs {
			if !slices.Contains(p.ids, x) {
				p.ids = append(p.ids, x)
				p.idsChanged = true
			}
		}
	}

	for _, id := range order {
		p := byID[id]
		changes := make(map[string]any)
		if p.formsChanged {
			changes["forms"] = p.forms
		}
		if p.idsChanged {
			changes["external_ids"] = p.ids
		}
		if len(changes) == 0 {
			continue
		}
		if err := h.appendImportedPlace(ctx, id, p, changes); err != nil {
			result.Warnings = append(result.Warnings,
				fmt.Sprintf("Failed to store the form or external IDs of place %q: %v", p.text, err))
		}
	}
	return nil
}

// appendImportedPlace appends one PlaceUpdated with changes to the stream of
// the place with id, at the version its facts were last projected at.
func (h *Handler) appendImportedPlace(ctx context.Context, id uuid.UUID, p *importedPlace, changes map[string]any) error {
	facts, err := h.readStore.GetPlaceFacts(ctx, id)
	if err != nil {
		return fmt.Errorf("getting place facts: %w", err)
	}
	var version int64
	if facts != nil {
		version = facts.Version
	}
	event := domain.NewPlaceUpdated(id, p.key, p.text, changes)
	_, err = h.execute(ctx, id.String(), placeStreamType, []domain.Event{event}, version)
	return err
}
