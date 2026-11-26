package gedcom

import (
	"context"
	"fmt"

	"github.com/cacack/gedcom-go/v3/gedcom"
	"github.com/google/uuid"

	"github.com/cacack/my-family/internal/domain"
	"github.com/cacack/my-family/internal/repository"
)

// exportPlaces is what an export writes under each PLAC beyond the record's
// own text and pair (ADR-009 §8): the forms and EXIDs of the place each text's
// key names, before alias resolution, and the facts of the place it resolves
// to, whose pair a record without its own inherits. Read through the
// export's store, it holds only places the principal may see (ADR-009 §10).
type exportPlaces struct {
	facts   map[string]*repository.PlaceFactsReadModel
	aliases repository.PlaceAliases
	forms   map[uuid.UUID][]repository.PlaceFormReadModel
	ids     map[uuid.UUID][]repository.PlaceExternalIDReadModel
}

// loadExportPlaces reads main's place facts, aliases, forms and EXIDs.
func loadExportPlaces(ctx context.Context, rs repository.ReadModelStore) (*exportPlaces, error) {
	facts, err := rs.ListPlaceFacts(ctx, domain.MainBranchID)
	if err != nil {
		return nil, fmt.Errorf("failed to list place facts: %w", err)
	}
	aliases, err := rs.ListPlaceAliases(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to list place aliases: %w", err)
	}
	forms, err := rs.ListPlaceForms(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to list place forms: %w", err)
	}
	ids, err := rs.ListPlaceExternalIDs(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to list place external IDs: %w", err)
	}
	return &exportPlaces{
		facts:   repository.PlaceFactsByKey(facts),
		aliases: repository.PlaceAliasesByKey(aliases),
		forms:   forms,
		ids:     ids,
	}, nil
}

// addTo writes the place facts under every PLAC of the document's persons'
// and families' events and attributes, then moves the most common form into
// the header (see writeHeaderForm). The document holds only what the
// export's privacy filter let through, so nothing is written under a record
// it left out.
func (p *exportPlaces) addTo(doc *gedcom.Document) {
	var details []*gedcom.PlaceDetail
	forEachPlaceDetail(doc, func(_, _ string, detail *gedcom.PlaceDetail) {
		p.write(detail)
		if detail != nil && repository.PlaceKey(detail.Name) != "" {
			details = append(details, detail)
		}
	})
	writeHeaderForm(doc.Header, details)
}

// writeHeaderForm writes the form most PLACs carry as HEAD.PLAC.FORM and
// clears it from those PLACs, so a per-place FORM is left only where it
// differs (ADR-009 §8). A tie goes to the form that sorts first. Import gives
// the header form to every PLAC that names a place and has no FORM of its
// own, so no default is written while any such PLAC has no form: re-importing
// would give that place a form it never had.
func writeHeaderForm(header *gedcom.Header, details []*gedcom.PlaceDetail) {
	counts := make(map[string]int)
	for _, d := range details {
		if d.Form == "" {
			return
		}
		counts[d.Form]++
	}
	def := ""
	for form, n := range counts {
		if n > counts[def] || (n == counts[def] && form < def) {
			def = form
		}
	}
	if def == "" {
		return
	}
	header.PlaceForm = def
	for _, d := range details {
		if d.Form == def {
			d.Form = ""
		}
	}
}

// write adds to a PLAC whose text names a place the form its place holds
// for the text's positional key, the place's EXIDs, and, when the record has
// no coordinate of its own (not even half a pair), the pair of the place it
// resolves to (ADR-009 §5, §8). A form is never written against a text with a
// different number of parts.
func (p *exportPlaces) write(detail *gedcom.PlaceDetail) {
	if detail == nil {
		return
	}
	key := repository.PlaceKey(detail.Name)
	if key == "" {
		return
	}
	id := repository.PlaceID(key)
	positional := repository.PositionalPlaceKey(detail.Name)
	for _, f := range p.forms[id] {
		if f.PositionalKey == positional {
			detail.Form = f.Form
		}
	}
	for _, x := range p.ids[id] {
		detail.ExternalIDs = append(detail.ExternalIDs, &gedcom.ExternalID{Value: x.Value, Type: x.Type})
	}
	if detail.Coordinates == nil {
		if f := p.facts[p.aliases.Resolve(key)]; f.HasCoordinates() {
			detail.Coordinates = &gedcom.Coordinates{Latitude: *f.Latitude, Longitude: *f.Longitude}
		}
	}
}
