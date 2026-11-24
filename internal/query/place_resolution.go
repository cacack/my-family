package query

import (
	"context"

	"github.com/google/uuid"

	"github.com/cacack/my-family/internal/domain"
	"github.com/cacack/my-family/internal/repository"
)

// ResolvedPlace is the place a record's place text resolves to (ADR-009 §5),
// returned beside the record's own pair so a reader can tell the two apart:
// its ID, its display name when one is set, and its own pair with the pair's
// origin when it has one. The record's effective coordinates are its own pair
// when both halves are set, otherwise this pair (repository.ResolveCoordinates).
type ResolvedPlace struct {
	ID        uuid.UUID                `json:"id"`
	Name      string                   `json:"name,omitempty"`
	Latitude  *string                  `json:"latitude,omitempty"`
	Longitude *string                  `json:"longitude,omitempty"`
	Origin    *domain.CoordinateOrigin `json:"coordinates_origin,omitempty"`
}

// PlaceResolver resolves place texts to places from one read of the place
// facts a branch sees (main's, PL-005). Read through the privacy decorator it
// holds only the facts of places visible to the principal (PL-006).
type PlaceResolver struct {
	facts map[string]*repository.PlaceFactsReadModel
}

// NewPlaceResolver reads the place facts branchID sees from store.
func NewPlaceResolver(ctx context.Context, store repository.ReadModelStore, branchID domain.BranchID) (*PlaceResolver, error) {
	facts, err := store.ListPlaceFacts(ctx, branchID)
	if err != nil {
		return nil, err
	}
	return &PlaceResolver{facts: repository.PlaceFactsByKey(facts)}, nil
}

// Resolve returns the place text resolves to, or nil for a text that names
// no place.
func (r *PlaceResolver) Resolve(text *string) *ResolvedPlace {
	if text == nil {
		return nil
	}
	key := repository.PlaceKey(*text)
	if key == "" {
		return nil
	}
	resolved := &ResolvedPlace{ID: repository.PlaceID(key)}
	if f := r.facts[key]; f != nil {
		resolved.Name = f.Name
		if f.HasCoordinates() {
			resolved.Latitude, resolved.Longitude = f.Latitude, f.Longitude
			resolved.Origin = domain.EffectiveCoordinateOrigin(f.Latitude, f.Longitude, f.Origin)
		}
	}
	return resolved
}

// resolvePerson sets the resolved birth and death places on p.
func (r *PlaceResolver) resolvePerson(p *Person) {
	p.BirthPlaceResolved = r.Resolve(p.BirthPlace)
	p.DeathPlaceResolved = r.Resolve(p.DeathPlace)
}

// resolveFamily sets the resolved marriage place on f.
func (r *PlaceResolver) resolveFamily(f *Family) {
	f.MarriagePlaceResolved = r.Resolve(f.MarriagePlace)
}
