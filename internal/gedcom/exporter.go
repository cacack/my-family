package gedcom

import (
	"context"
	"fmt"
	"io"
	"sort"
	"strings"

	"github.com/cacack/gedcom-go/encoder"
	"github.com/cacack/gedcom-go/gedcom"
	"github.com/google/uuid"

	"github.com/cacack/my-family/internal/domain"
	"github.com/cacack/my-family/internal/repository"
)

// ExportResult contains the results of a GEDCOM export operation.
type ExportResult struct {
	BytesWritten          int64
	PersonsExported       int
	FamiliesExported      int
	SourcesExported       int
	CitationsExported     int
	EventsExported        int
	AttributesExported    int
	NotesExported         int
	SubmittersExported    int
	AssociationsExported  int
	LDSOrdinancesExported int
}

// ExportProgress represents the current progress of an export operation.
type ExportProgress struct {
	Phase      string  `json:"phase"`      // Current phase: "sources", "persons", "families", "notes", "submitters", "encoding"
	Current    int     `json:"current"`    // Current item number in this phase
	Total      int     `json:"total"`      // Total items in this phase
	Percentage float64 `json:"percentage"` // Overall progress percentage (0-100)
}

// ProgressCallback is called during export to report progress.
// Return an error to cancel the export.
type ProgressCallback func(progress ExportProgress) error

// Exporter handles GEDCOM file generation from repository data.
type Exporter struct {
	readStore repository.ReadModelStore
}

// NewExporter creates a new GEDCOM exporter.
func NewExporter(readStore repository.ReadModelStore) *Exporter {
	return &Exporter{readStore: readStore}
}

// Export generates a GEDCOM 5.5 file from all data in the repository.
// For progress tracking during large exports, use ExportWithProgress instead.
func (exp *Exporter) Export(ctx context.Context, w io.Writer) (*ExportResult, error) {
	return exp.ExportWithProgress(ctx, w, nil)
}

// ExportWithProgress generates a GEDCOM 5.5 file with optional progress callback.
// The onProgress callback is called periodically to report export progress.
// Pass nil for onProgress to disable progress tracking.
func (exp *Exporter) ExportWithProgress(ctx context.Context, w io.Writer, onProgress ProgressCallback) (*ExportResult, error) {
	result := &ExportResult{}

	// Helper to report progress (no-op if onProgress is nil)
	reportProgress := func(phase string, current, total int, overallPct float64) error {
		if onProgress != nil {
			return onProgress(ExportProgress{
				Phase:      phase,
				Current:    current,
				Total:      total,
				Percentage: overallPct,
			})
		}
		return nil
	}

	// Get all persons
	persons, err := repository.ListAll(ctx, 1000, exp.readStore.ListPersons)
	if err != nil {
		return nil, fmt.Errorf("failed to list persons: %w", err)
	}

	// Get all families
	families, err := repository.ListAll(ctx, 1000, exp.readStore.ListFamilies)
	if err != nil {
		return nil, fmt.Errorf("failed to list families: %w", err)
	}

	// Get all sources
	sources, err := repository.ListAll(ctx, 1000, exp.readStore.ListSources)
	if err != nil {
		return nil, fmt.Errorf("failed to list sources: %w", err)
	}

	// Get notes and submitters counts for progress calculation
	notes, err := repository.ListAll(ctx, 1000, exp.readStore.ListNotes)
	if err != nil {
		return nil, fmt.Errorf("failed to list notes: %w", err)
	}

	submitters, err := repository.ListAll(ctx, 1000, exp.readStore.ListSubmitters)
	if err != nil {
		return nil, fmt.Errorf("failed to list submitters: %w", err)
	}

	// Calculate total items for progress tracking
	// Weight persons more heavily since they have the most processing
	totalItems := len(sources) + len(persons)*2 + len(families) + len(notes) + len(submitters) + 1 // +1 for encoding
	processedItems := 0

	// Create XREF mappings (UUID -> @Xn@)
	personXrefs := make(map[uuid.UUID]string)
	familyXrefs := make(map[uuid.UUID]string)
	sourceXrefs := make(map[uuid.UUID]string)

	// Sort persons by ID for stable output
	sort.Slice(persons, func(i, j int) bool {
		return persons[i].ID.String() < persons[j].ID.String()
	})
	for i, p := range persons {
		personXrefs[p.ID] = fmt.Sprintf("@I%d@", i+1)
	}

	// Sort families by ID for stable output
	sort.Slice(families, func(i, j int) bool {
		return families[i].ID.String() < families[j].ID.String()
	})
	for i, f := range families {
		familyXrefs[f.ID] = fmt.Sprintf("@F%d@", i+1)
	}

	// Sort sources by ID for stable output
	sort.Slice(sources, func(i, j int) bool {
		return sources[i].ID.String() < sources[j].ID.String()
	})
	for i, s := range sources {
		// Use GedcomXref if available (for round-trip), otherwise generate
		if s.GedcomXref != "" {
			sourceXrefs[s.ID] = s.GedcomXref
		} else {
			sourceXrefs[s.ID] = fmt.Sprintf("@S%d@", i+1)
		}
	}

	// Build GEDCOM document
	doc := &gedcom.Document{
		Header: &gedcom.Header{
			Version:      gedcom.Version55,
			Encoding:     gedcom.EncodingUTF8,
			SourceSystem: "MyFamily",
		},
		Records: make([]*gedcom.Record, 0),
	}

	// Add source records
	for i, s := range sources {
		xref := sourceXrefs[s.ID]
		src := toGedcomSource(s)
		doc.Records = append(doc.Records, &gedcom.Record{
			XRef:   xref,
			Type:   gedcom.RecordTypeSource,
			Entity: src, // Encoder converts Entity -> Tags automatically
		})
		result.SourcesExported++
		processedItems++

		// Report progress every 10 items or at the end
		if i%10 == 0 || i == len(sources)-1 {
			pct := float64(processedItems) / float64(totalItems) * 100
			if err := reportProgress("sources", i+1, len(sources), pct); err != nil {
				return result, err
			}
		}
	}

	// Add individual records
	for i, p := range persons {
		xref := personXrefs[p.ID]
		// Fetch citations for this person's events
		birthCitations, _ := exp.readStore.GetCitationsForFact(ctx, domain.FactPersonBirth, p.ID)
		deathCitations, _ := exp.readStore.GetCitationsForFact(ctx, domain.FactPersonDeath, p.ID)
		allCitations := append(birthCitations, deathCitations...)
		result.CitationsExported += len(allCitations)

		// Fetch events and attributes for this person
		events, _ := exp.readStore.ListEventsForPerson(ctx, p.ID)
		attributes, _ := exp.readStore.ListAttributesForPerson(ctx, p.ID)
		result.EventsExported += len(events)
		result.AttributesExported += len(attributes)

		// Fetch associations for this person (where this person is the PersonID)
		allAssocs, _ := exp.readStore.ListAssociationsForPerson(ctx, p.ID)
		// Filter to only associations where this person is the PersonID (the one who has the association)
		var associations []repository.AssociationReadModel
		for _, a := range allAssocs {
			if a.PersonID == p.ID {
				associations = append(associations, a)
			}
		}
		result.AssociationsExported += len(associations)

		// Fetch LDS ordinances for this person
		ldsOrdinances, _ := exp.readStore.ListLDSOrdinancesForPerson(ctx, p.ID)
		result.LDSOrdinancesExported += len(ldsOrdinances)

		indi := toGedcomIndividual(p, sourceXrefs, personXrefs, birthCitations, deathCitations, events, attributes, associations, ldsOrdinances, exp.readStore, ctx)
		doc.Records = append(doc.Records, &gedcom.Record{
			XRef:   xref,
			Type:   gedcom.RecordTypeIndividual,
			Entity: indi, // Encoder converts Entity -> Tags automatically
		})
		result.PersonsExported++
		processedItems += 2 // Persons count double due to extra processing

		// Report progress every 10 items or at the end
		if i%10 == 0 || i == len(persons)-1 {
			pct := float64(processedItems) / float64(totalItems) * 100
			if err := reportProgress("persons", i+1, len(persons), pct); err != nil {
				return result, err
			}
		}
	}

	// Add family records
	for i, f := range families {
		xref := familyXrefs[f.ID]
		children, _ := exp.readStore.GetFamilyChildren(ctx, f.ID)
		marriageCitations, _ := exp.readStore.GetCitationsForFact(ctx, domain.FactFamilyMarriage, f.ID)
		result.CitationsExported += len(marriageCitations)

		// Fetch events for this family
		familyEvents, _ := exp.readStore.ListEventsForFamily(ctx, f.ID)
		result.EventsExported += len(familyEvents)

		// Fetch LDS ordinances for this family
		familyLDSOrdinances, _ := exp.readStore.ListLDSOrdinancesForFamily(ctx, f.ID)
		result.LDSOrdinancesExported += len(familyLDSOrdinances)

		fam := toGedcomFamily(f, personXrefs, sourceXrefs, children, marriageCitations, familyEvents, familyLDSOrdinances, exp.readStore, ctx)
		doc.Records = append(doc.Records, &gedcom.Record{
			XRef:   xref,
			Type:   gedcom.RecordTypeFamily,
			Entity: fam, // Encoder converts Entity -> Tags automatically
		})
		result.FamiliesExported++
		processedItems++

		// Report progress every 10 items or at the end
		if i%10 == 0 || i == len(families)-1 {
			pct := float64(processedItems) / float64(totalItems) * 100
			if err := reportProgress("families", i+1, len(families), pct); err != nil {
				return result, err
			}
		}
	}

	// Sort notes by ID for stable output
	sort.Slice(notes, func(i, j int) bool {
		return notes[i].ID.String() < notes[j].ID.String()
	})

	for i, n := range notes {
		// Use GedcomXref if available (for round-trip), otherwise generate
		var xref string
		if n.GedcomXref != "" {
			xref = n.GedcomXref
		} else {
			xref = fmt.Sprintf("@N%d@", i+1)
		}

		note := toGedcomNote(n)
		doc.Records = append(doc.Records, &gedcom.Record{
			XRef:   xref,
			Type:   gedcom.RecordTypeNote,
			Entity: note,
		})
		result.NotesExported++
		processedItems++

		// Report progress every 10 items or at the end
		if i%10 == 0 || i == len(notes)-1 {
			pct := float64(processedItems) / float64(totalItems) * 100
			if err := reportProgress("notes", i+1, len(notes), pct); err != nil {
				return result, err
			}
		}
	}

	// Sort submitters by ID for stable output
	sort.Slice(submitters, func(i, j int) bool {
		return submitters[i].ID.String() < submitters[j].ID.String()
	})

	for i, s := range submitters {
		// Use GedcomXref if available (for round-trip), otherwise generate
		var xref string
		if s.GedcomXref != "" {
			xref = s.GedcomXref
		} else {
			xref = fmt.Sprintf("@U%d@", i+1)
		}

		subm := toGedcomSubmitter(s)
		doc.Records = append(doc.Records, &gedcom.Record{
			XRef:   xref,
			Type:   gedcom.RecordTypeSubmitter,
			Entity: subm,
		})
		result.SubmittersExported++
		processedItems++

		// Report progress every 10 items or at the end
		if i%10 == 0 || i == len(submitters)-1 {
			pct := float64(processedItems) / float64(totalItems) * 100
			if err := reportProgress("submitters", i+1, len(submitters), pct); err != nil {
				return result, err
			}
		}
	}

	// Report encoding phase
	if err := reportProgress("encoding", 0, 1, 99.0); err != nil {
		return result, err
	}

	// Use a counting writer to track bytes written
	cw := &countingWriter{w: w}

	// Encode using gedcom-go encoder with LF line endings
	opts := &encoder.EncodeOptions{LineEnding: "\n"}
	if err := encoder.EncodeWithOptions(cw, doc, opts); err != nil {
		return result, fmt.Errorf("failed to write GEDCOM: %w", err)
	}

	result.BytesWritten = cw.count

	// Report completion
	if err := reportProgress("complete", 1, 1, 100.0); err != nil {
		return result, err
	}

	return result, nil
}

// countingWriter wraps an io.Writer and counts bytes written.
type countingWriter struct {
	w     io.Writer
	count int64
}

func (cw *countingWriter) Write(p []byte) (n int, err error) {
	n, err = cw.w.Write(p)
	cw.count += int64(n)
	return n, err
}

// toGedcomSource converts a repository SourceReadModel to a gedcom.Source entity.
// The encoder will automatically convert this to GEDCOM tags, handling CONT/CONC.
func toGedcomSource(s repository.SourceReadModel) *gedcom.Source {
	src := &gedcom.Source{}

	// Title
	if s.Title != "" {
		src.Title = s.Title
	}

	// Author
	if s.Author != "" {
		src.Author = s.Author
	}

	// Publisher (as PUBL)
	if s.Publisher != "" {
		src.Publication = s.Publisher
	}

	// Repository
	if s.RepositoryName != "" {
		// If it looks like an XREF, use RepositoryRef
		if strings.HasPrefix(s.RepositoryName, "@") && strings.HasSuffix(s.RepositoryName, "@") {
			src.RepositoryRef = s.RepositoryName
		} else {
			// Inline repository with NAME subordinate
			src.Repository = &gedcom.InlineRepository{Name: s.RepositoryName}
		}
	}

	// Notes - encoder handles CONT/CONC automatically for multiline text
	if s.Notes != "" {
		src.Notes = []string{s.Notes}
	}

	return src
}

// toGedcomIndividual converts a repository PersonReadModel to a gedcom.Individual entity.
// The encoder will automatically convert this to GEDCOM tags, handling CONT/CONC.
func toGedcomIndividual(p repository.PersonReadModel, sourceXrefs map[uuid.UUID]string, personXrefs map[uuid.UUID]string, birthCitations, deathCitations []repository.CitationReadModel, events []repository.EventReadModel, attributes []repository.AttributeReadModel, associations []repository.AssociationReadModel, ldsOrdinances []repository.LDSOrdinanceReadModel, readStore repository.ReadModelStore, ctx context.Context) *gedcom.Individual {
	indi := &gedcom.Individual{}

	// Fetch all names for this person
	names, err := readStore.GetPersonNames(ctx, p.ID)
	if err != nil || len(names) == 0 {
		// Fallback to person's primary name fields if no names in person_names table
		indi.Names = []*gedcom.PersonalName{{
			Full: formatGedcomName(p.GivenName, p.Surname),
		}}
	} else {
		// Sort names: primary first, then others
		sortedNames := sortNamesByPrimary(names)
		for _, nm := range sortedNames {
			indi.Names = append(indi.Names, toGedcomPersonalName(nm))
		}
	}

	// Sex
	if p.Gender != "" {
		switch p.Gender {
		case domain.GenderMale:
			indi.Sex = "M"
		case domain.GenderFemale:
			indi.Sex = "F"
		}
	}

	// Birth event
	if p.BirthDateRaw != "" || p.BirthPlace != "" {
		birthEvent := &gedcom.Event{Type: gedcom.EventBirth}
		if p.BirthDateRaw != "" {
			birthEvent.Date = p.BirthDateRaw
		}
		if p.BirthPlace != "" {
			birthEvent.Place = p.BirthPlace
			// Add coordinates if present
			if p.BirthPlaceLat != nil && p.BirthPlaceLong != nil && *p.BirthPlaceLat != "" && *p.BirthPlaceLong != "" {
				birthEvent.PlaceDetail = &gedcom.PlaceDetail{
					Name: p.BirthPlace,
					Coordinates: &gedcom.Coordinates{
						Latitude:  *p.BirthPlaceLat,
						Longitude: *p.BirthPlaceLong,
					},
				}
			}
		}
		// Citations for birth
		birthEvent.SourceCitations = toGedcomSourceCitations(birthCitations, sourceXrefs)
		indi.Events = append(indi.Events, birthEvent)
	}

	// Death event
	if p.DeathDateRaw != "" || p.DeathPlace != "" {
		deathEvent := &gedcom.Event{Type: gedcom.EventDeath}
		if p.DeathDateRaw != "" {
			deathEvent.Date = p.DeathDateRaw
		}
		if p.DeathPlace != "" {
			deathEvent.Place = p.DeathPlace
			// Add coordinates if present
			if p.DeathPlaceLat != nil && p.DeathPlaceLong != nil && *p.DeathPlaceLat != "" && *p.DeathPlaceLong != "" {
				deathEvent.PlaceDetail = &gedcom.PlaceDetail{
					Name: p.DeathPlace,
					Coordinates: &gedcom.Coordinates{
						Latitude:  *p.DeathPlaceLat,
						Longitude: *p.DeathPlaceLong,
					},
				}
			}
		}
		// Citations for death
		deathEvent.SourceCitations = toGedcomSourceCitations(deathCitations, sourceXrefs)
		indi.Events = append(indi.Events, deathEvent)
	}

	// Additional life events (burial, baptism, emigration, etc.)
	for _, event := range events {
		if gedcomEvent := toGedcomEvent(event, sourceXrefs, readStore, ctx); gedcomEvent != nil {
			indi.Events = append(indi.Events, gedcomEvent)
		}
	}

	// Attributes (occupation, residence, education, etc.)
	for _, attr := range attributes {
		if gedcomAttr := toGedcomAttribute(attr); gedcomAttr != nil {
			indi.Attributes = append(indi.Attributes, gedcomAttr)
		}
	}

	// Notes - encoder handles CONT/CONC automatically for multiline text
	if p.Notes != "" {
		indi.Notes = []string{p.Notes}
	}

	// Associations (godparents, witnesses, etc.)
	for _, assoc := range associations {
		if gedcomAssoc := toGedcomAssociation(assoc, personXrefs); gedcomAssoc != nil {
			indi.Associations = append(indi.Associations, gedcomAssoc)
		}
	}

	// LDS ordinances (BAPL, CONL, ENDL, SLGC)
	for _, ord := range ldsOrdinances {
		// Only individual-level ordinances (not SLGS which is family-level)
		if ord.Type == domain.LDSBaptism || ord.Type == domain.LDSConfirmation ||
			ord.Type == domain.LDSEndowment || ord.Type == domain.LDSSealingChild {
			indi.LDSOrdinances = append(indi.LDSOrdinances, toGedcomLDSOrdinance(ord))
		}
	}

	return indi
}

// toGedcomAssociation converts a repository AssociationReadModel to a gedcom.Association.
func toGedcomAssociation(a repository.AssociationReadModel, personXrefs map[uuid.UUID]string) *gedcom.Association {
	// Look up the associate's XREF
	associateXref, found := personXrefs[a.AssociateID]
	if !found {
		// Can't export without a valid XREF
		return nil
	}

	assoc := &gedcom.Association{
		IndividualXRef: associateXref,
		Role:           mapRoleToGedcom(a.Role),
	}

	// PHRASE (GEDCOM 7.0)
	if a.Phrase != "" {
		assoc.Phrase = a.Phrase
	}

	// Notes
	if a.Notes != "" {
		assoc.Notes = []string{a.Notes}
	}

	return assoc
}

// mapRoleToGedcom converts internal role names to GEDCOM RELA values.
func mapRoleToGedcom(role string) string {
	switch role {
	case domain.RoleGodparent:
		return "GODP"
	case domain.RoleWitness:
		return "WITN"
	default:
		// Return custom role as-is (GEDCOM 5.5.1 allows free text)
		return strings.ToUpper(role)
	}
}

// toGedcomPersonalName converts a PersonNameReadModel to a gedcom.PersonalName.
func toGedcomPersonalName(nm repository.PersonNameReadModel) *gedcom.PersonalName {
	pn := &gedcom.PersonalName{
		Full: formatGedcomName(nm.GivenName, nm.Surname),
	}

	// TYPE - only add if not birth (birth is the default)
	if nm.NameType != "" && nm.NameType != domain.NameTypeBirth {
		pn.Type = mapNameTypeToGedcom(nm.NameType)
	}

	// Name components
	if nm.NamePrefix != "" {
		pn.Prefix = nm.NamePrefix
	}
	if nm.NameSuffix != "" {
		pn.Suffix = nm.NameSuffix
	}
	if nm.SurnamePrefix != "" {
		pn.SurnamePrefix = nm.SurnamePrefix
	}
	if nm.Nickname != "" {
		pn.Nickname = nm.Nickname
	}

	return pn
}

// toGedcomSourceCitations converts a slice of CitationReadModel to gedcom.SourceCitation slice.
func toGedcomSourceCitations(citations []repository.CitationReadModel, sourceXrefs map[uuid.UUID]string) []*gedcom.SourceCitation {
	var result []*gedcom.SourceCitation

	for _, cit := range citations {
		srcXref, ok := sourceXrefs[cit.SourceID]
		if !ok {
			continue
		}

		citation := &gedcom.SourceCitation{
			SourceXRef: srcXref,
		}

		if cit.Page != "" {
			citation.Page = cit.Page
		}

		// QUAY (quality) - always output if any GPS quality info present
		if cit.SourceQuality != "" || cit.InformantType != "" || cit.EvidenceType != "" {
			citation.Quality = mapGPSToGedcomQuality(cit.SourceQuality, cit.InformantType, cit.EvidenceType)
		}

		// DATA with TEXT (quoted text)
		if cit.QuotedText != "" {
			citation.Data = &gedcom.SourceCitationData{
				Text: cit.QuotedText,
			}
		}

		result = append(result, citation)
	}

	return result
}

// toGedcomEvent converts an EventReadModel to a gedcom.Event.
func toGedcomEvent(event repository.EventReadModel, sourceXrefs map[uuid.UUID]string, readStore repository.ReadModelStore, ctx context.Context) *gedcom.Event {
	tagName := mapFactTypeToGedcomTag(event.FactType)
	if tagName == "" {
		return nil // Skip unknown event types
	}

	ge := &gedcom.Event{
		Type: gedcom.EventType(tagName),
	}

	if event.DateRaw != "" {
		ge.Date = event.DateRaw
	}

	if event.Place != "" {
		ge.Place = event.Place
		// Add coordinates if present
		if event.PlaceLat != nil && event.PlaceLong != nil && *event.PlaceLat != "" && *event.PlaceLong != "" {
			ge.PlaceDetail = &gedcom.PlaceDetail{
				Name: event.Place,
				Coordinates: &gedcom.Coordinates{
					Latitude:  *event.PlaceLat,
					Longitude: *event.PlaceLong,
				},
			}
		}
	}

	// Add structured address if available
	if event.Address != nil && !event.Address.IsEmpty() {
		ge.Address = convertDomainAddressToGedcom(event.Address)
	}

	if event.Cause != "" {
		ge.Cause = event.Cause
	}

	if event.Age != "" {
		ge.Age = event.Age
	}

	// Fetch and add citations for this event
	if readStore != nil {
		citations, _ := readStore.GetCitationsForFact(ctx, event.FactType, event.OwnerID)
		ge.SourceCitations = toGedcomSourceCitations(citations, sourceXrefs)
	}

	return ge
}

// convertDomainAddressToGedcom converts a domain.Address to a gedcom.Address.
func convertDomainAddressToGedcom(addr *domain.Address) *gedcom.Address {
	if addr == nil {
		return nil
	}
	return &gedcom.Address{
		Line1:      addr.Line1,
		Line2:      addr.Line2,
		Line3:      addr.Line3,
		City:       addr.City,
		State:      addr.State,
		PostalCode: addr.PostalCode,
		Country:    addr.Country,
		Phone:      addr.Phone,
		Email:      addr.Email,
		Website:    addr.Website,
	}
}

// toGedcomAttribute converts an AttributeReadModel to a gedcom.Attribute.
func toGedcomAttribute(attr repository.AttributeReadModel) *gedcom.Attribute {
	tagName := mapFactTypeToGedcomTag(attr.FactType)
	if tagName == "" {
		return nil // Skip unknown attribute types
	}

	ga := &gedcom.Attribute{
		Type:  tagName,
		Value: attr.Value,
	}

	if attr.DateRaw != "" {
		ga.Date = attr.DateRaw
	}

	if attr.Place != "" {
		ga.Place = attr.Place
	}

	return ga
}

// toGedcomFamily converts a repository FamilyReadModel to a gedcom.Family entity.
// The encoder will automatically convert this to GEDCOM tags, handling CONT/CONC.
func toGedcomFamily(f repository.FamilyReadModel, personXrefs, sourceXrefs map[uuid.UUID]string, children []repository.FamilyChildReadModel, marriageCitations []repository.CitationReadModel, events []repository.EventReadModel, ldsOrdinances []repository.LDSOrdinanceReadModel, readStore repository.ReadModelStore, ctx context.Context) *gedcom.Family {
	fam := &gedcom.Family{}

	// Husband (Partner1)
	if f.Partner1ID != nil {
		if pXref, ok := personXrefs[*f.Partner1ID]; ok {
			fam.Husband = pXref
		}
	}

	// Wife (Partner2)
	if f.Partner2ID != nil {
		if pXref, ok := personXrefs[*f.Partner2ID]; ok {
			fam.Wife = pXref
		}
	}

	// Marriage event
	if f.RelationshipType == domain.RelationMarriage || f.MarriageDateRaw != "" || f.MarriagePlace != "" {
		marriageEvent := &gedcom.Event{Type: gedcom.EventMarriage}
		if f.MarriageDateRaw != "" {
			marriageEvent.Date = f.MarriageDateRaw
		}
		if f.MarriagePlace != "" {
			marriageEvent.Place = f.MarriagePlace
			// Add coordinates if present
			if f.MarriagePlaceLat != nil && f.MarriagePlaceLong != nil && *f.MarriagePlaceLat != "" && *f.MarriagePlaceLong != "" {
				marriageEvent.PlaceDetail = &gedcom.PlaceDetail{
					Name: f.MarriagePlace,
					Coordinates: &gedcom.Coordinates{
						Latitude:  *f.MarriagePlaceLat,
						Longitude: *f.MarriagePlaceLong,
					},
				}
			}
		}
		// Citations for marriage
		marriageEvent.SourceCitations = toGedcomSourceCitations(marriageCitations, sourceXrefs)
		fam.Events = append(fam.Events, marriageEvent)
	}

	// Additional family events (divorce, annulment, engagement, etc.)
	for _, event := range events {
		if gedcomEvent := toGedcomEvent(event, sourceXrefs, readStore, ctx); gedcomEvent != nil {
			fam.Events = append(fam.Events, gedcomEvent)
		}
	}

	// Children
	for _, c := range children {
		if pXref, ok := personXrefs[c.PersonID]; ok {
			fam.Children = append(fam.Children, pXref)
		}
	}

	// LDS ordinances (SLGS - sealing to spouse)
	for _, ord := range ldsOrdinances {
		if ord.Type == domain.LDSSealingSpouse {
			fam.LDSOrdinances = append(fam.LDSOrdinances, toGedcomLDSOrdinance(ord))
		}
	}

	return fam
}

// mapGPSToGedcomQuality maps GPS quality terms to GEDCOM QUAY values (0-3).
// This is the reverse of mapGedcomQualityToGPS in importer.go
// TODO: sourceQuality is reserved for future use when GEDCOM quality mapping is enhanced
func mapGPSToGedcomQuality(_ domain.SourceQuality, informantType domain.InformantType, evidenceType domain.EvidenceType) int {
	// Priority: evidenceType > informantType > sourceQuality
	// QUAY 3 = Direct and primary evidence
	if evidenceType == domain.EvidenceDirect && informantType == domain.InformantPrimary {
		return 3
	}
	// QUAY 2 = Secondary evidence (informant type)
	if informantType == domain.InformantSecondary {
		return 2
	}
	// QUAY 1 = Questionable/Indirect evidence
	if evidenceType == domain.EvidenceIndirect {
		return 1
	}
	// QUAY 0 = Unreliable/Negative evidence
	if evidenceType == domain.EvidenceNegative {
		return 0
	}
	// Default: no quality specified
	return 0
}

// formatGedcomName formats a name in GEDCOM format (Given /Surname/).
// GEDCOM requires surname delimiters even when surname is empty.
func formatGedcomName(givenName, surname string) string {
	// Always include surname delimiters per GEDCOM spec
	return fmt.Sprintf("%s /%s/", givenName, surname)
}

// sortNamesByPrimary sorts names with primary name first.
func sortNamesByPrimary(names []repository.PersonNameReadModel) []repository.PersonNameReadModel {
	// Simple stable sort: primary first
	sort.SliceStable(names, func(i, j int) bool {
		// Primary names come first
		if names[i].IsPrimary && !names[j].IsPrimary {
			return true
		}
		return false
	})
	return names
}

// mapNameTypeToGedcom converts a domain NameType to GEDCOM TYPE value.
func mapNameTypeToGedcom(nameType domain.NameType) string {
	switch nameType {
	case domain.NameTypeBirth:
		return "birth"
	case domain.NameTypeMarried:
		return "married"
	case domain.NameTypeAKA:
		return "aka"
	case domain.NameTypeImmigrant:
		return "immigrant"
	case domain.NameTypeReligious:
		return "religious"
	case domain.NameTypeProfessional:
		return "professional"
	default:
		return string(nameType)
	}
}

// mapFactTypeToGedcomTag maps a FactType to its corresponding GEDCOM tag string.
// Returns an empty string for unknown or unmappable types.
func mapFactTypeToGedcomTag(factType domain.FactType) string {
	switch factType {
	// Person events
	case domain.FactPersonBirth:
		return "BIRT"
	case domain.FactPersonDeath:
		return "DEAT"
	case domain.FactPersonBurial:
		return "BURI"
	case domain.FactPersonCremation:
		return "CREM"
	case domain.FactPersonBaptism:
		return "BAPM"
	case domain.FactPersonChristening:
		return "CHR"
	case domain.FactPersonEmigration:
		return "EMIG"
	case domain.FactPersonImmigration:
		return "IMMI"
	case domain.FactPersonNaturalization:
		return "NATU"
	case domain.FactPersonCensus:
		return "CENS"
	case domain.FactPersonGenericEvent:
		return "EVEN"
	// Person attributes
	case domain.FactPersonOccupation:
		return "OCCU"
	case domain.FactPersonResidence:
		return "RESI"
	case domain.FactPersonEducation:
		return "EDUC"
	case domain.FactPersonReligion:
		return "RELI"
	case domain.FactPersonTitle:
		return "TITL"
	// Family events
	case domain.FactFamilyMarriage:
		return "MARR"
	case domain.FactFamilyDivorce:
		return "DIV"
	case domain.FactFamilyMarriageBann:
		return "MARB"
	case domain.FactFamilyMarriageContract:
		return "MARC"
	case domain.FactFamilyMarriageLicense:
		return "MARL"
	case domain.FactFamilyMarriageSettlement:
		return "MARS"
	case domain.FactFamilyAnnulment:
		return "ANUL"
	case domain.FactFamilyEngagement:
		return "ENGA"
	default:
		return ""
	}
}

// toGedcomNote converts a repository NoteReadModel to a gedcom.Note entity.
// The encoder will automatically convert this to GEDCOM tags, handling CONT/CONC for multiline text.
func toGedcomNote(n repository.NoteReadModel) *gedcom.Note {
	return &gedcom.Note{
		Text: n.Text,
	}
}

// toGedcomSubmitter converts a repository SubmitterReadModel to a gedcom.Submitter entity.
// The encoder will automatically convert this to GEDCOM tags.
func toGedcomSubmitter(s repository.SubmitterReadModel) *gedcom.Submitter {
	subm := &gedcom.Submitter{
		Name:  s.Name,
		Phone: s.Phone,
		Email: s.Email,
	}

	// Convert address if present
	if s.Address != nil && !s.Address.IsEmpty() {
		subm.Address = convertDomainAddressToGedcom(s.Address)
	}

	// GEDCOM allows multiple languages, but we only store one
	if s.Language != "" {
		subm.Language = []string{s.Language}
	}

	return subm
}

// toGedcomLDSOrdinance converts a repository LDSOrdinanceReadModel to a gedcom.LDSOrdinance.
func toGedcomLDSOrdinance(ord repository.LDSOrdinanceReadModel) *gedcom.LDSOrdinance {
	return &gedcom.LDSOrdinance{
		Type:   gedcom.LDSOrdinanceType(ord.Type),
		Date:   ord.DateRaw,
		Temple: ord.Temple,
		Place:  ord.Place,
		Status: ord.Status,
	}
}
