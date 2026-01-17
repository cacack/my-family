package gedcom

import (
	"context"
	"fmt"
	"io"
	"sort"
	"strconv"
	"strings"

	"github.com/cacack/gedcom-go/encoder"
	"github.com/cacack/gedcom-go/gedcom"
	"github.com/google/uuid"

	"github.com/cacack/my-family/internal/domain"
	"github.com/cacack/my-family/internal/repository"
)

// ExportResult contains the results of a GEDCOM export operation.
type ExportResult struct {
	BytesWritten       int64
	PersonsExported    int
	FamiliesExported   int
	SourcesExported    int
	CitationsExported  int
	EventsExported     int
	AttributesExported int
}

// Exporter handles GEDCOM file generation from repository data.
type Exporter struct {
	readStore repository.ReadModelStore
}

// NewExporter creates a new GEDCOM exporter.
func NewExporter(readStore repository.ReadModelStore) *Exporter {
	return &Exporter{readStore: readStore}
}

// Export generates a GEDCOM 5.5 file from all data in the repository.
func (exp *Exporter) Export(ctx context.Context, w io.Writer) (*ExportResult, error) {
	result := &ExportResult{}

	// Get all persons
	persons, _, err := exp.readStore.ListPersons(ctx, repository.ListOptions{
		Limit: 100000, // Large limit to get all
	})
	if err != nil {
		return nil, fmt.Errorf("failed to list persons: %w", err)
	}

	// Get all families
	families, _, err := exp.readStore.ListFamilies(ctx, repository.ListOptions{
		Limit: 100000,
	})
	if err != nil {
		return nil, fmt.Errorf("failed to list families: %w", err)
	}

	// Get all sources
	sources, _, err := exp.readStore.ListSources(ctx, repository.ListOptions{
		Limit: 100000,
	})
	if err != nil {
		return nil, fmt.Errorf("failed to list sources: %w", err)
	}

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
	for _, s := range sources {
		xref := sourceXrefs[s.ID]
		tags := toGedcomSourceTags(s)
		doc.Records = append(doc.Records, &gedcom.Record{
			XRef: xref,
			Type: gedcom.RecordTypeSource,
			Tags: tags,
		})
		result.SourcesExported++
	}

	// Add individual records
	for _, p := range persons {
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

		tags := toGedcomIndividualTags(p, sourceXrefs, birthCitations, deathCitations, events, attributes, exp.readStore, ctx)
		doc.Records = append(doc.Records, &gedcom.Record{
			XRef: xref,
			Type: gedcom.RecordTypeIndividual,
			Tags: tags,
		})
		result.PersonsExported++
	}

	// Add family records
	for _, f := range families {
		xref := familyXrefs[f.ID]
		children, _ := exp.readStore.GetFamilyChildren(ctx, f.ID)
		marriageCitations, _ := exp.readStore.GetCitationsForFact(ctx, domain.FactFamilyMarriage, f.ID)
		result.CitationsExported += len(marriageCitations)

		// Fetch events for this family
		familyEvents, _ := exp.readStore.ListEventsForFamily(ctx, f.ID)
		result.EventsExported += len(familyEvents)

		tags := toGedcomFamilyTags(f, personXrefs, sourceXrefs, children, marriageCitations, familyEvents, exp.readStore, ctx)
		doc.Records = append(doc.Records, &gedcom.Record{
			XRef: xref,
			Type: gedcom.RecordTypeFamily,
			Tags: tags,
		})
		result.FamiliesExported++
	}

	// Use a counting writer to track bytes written
	cw := &countingWriter{w: w}

	// Encode using gedcom-go encoder with LF line endings
	opts := &encoder.EncodeOptions{LineEnding: "\n"}
	if err := encoder.EncodeWithOptions(cw, doc, opts); err != nil {
		return result, fmt.Errorf("failed to write GEDCOM: %w", err)
	}

	result.BytesWritten = cw.count

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

// toGedcomSourceTags converts a repository SourceReadModel to gedcom.Tag slice.
func toGedcomSourceTags(s repository.SourceReadModel) []*gedcom.Tag {
	var tags []*gedcom.Tag

	// Title
	if s.Title != "" {
		tags = append(tags, &gedcom.Tag{Level: 1, Tag: "TITL", Value: s.Title})
	}

	// Author
	if s.Author != "" {
		tags = append(tags, &gedcom.Tag{Level: 1, Tag: "AUTH", Value: s.Author})
	}

	// Publisher (as PUBL)
	if s.Publisher != "" {
		tags = append(tags, &gedcom.Tag{Level: 1, Tag: "PUBL", Value: s.Publisher})
	}

	// Repository
	if s.RepositoryName != "" {
		// If it looks like an XREF, use directly
		if strings.HasPrefix(s.RepositoryName, "@") && strings.HasSuffix(s.RepositoryName, "@") {
			tags = append(tags, &gedcom.Tag{Level: 1, Tag: "REPO", Value: s.RepositoryName})
		} else {
			// Inline repository with NAME subordinate
			tags = append(tags, &gedcom.Tag{Level: 1, Tag: "REPO"})
			tags = append(tags, &gedcom.Tag{Level: 2, Tag: "NAME", Value: s.RepositoryName})
		}
	}

	// Notes with CONT for multiline
	if s.Notes != "" {
		tags = append(tags, notesToTags(s.Notes, 1)...)
	}

	return tags
}

// toGedcomIndividualTags converts a repository PersonReadModel to gedcom.Tag slice.
func toGedcomIndividualTags(p repository.PersonReadModel, sourceXrefs map[uuid.UUID]string, birthCitations, deathCitations []repository.CitationReadModel, events []repository.EventReadModel, attributes []repository.AttributeReadModel, readStore repository.ReadModelStore, ctx context.Context) []*gedcom.Tag {
	var tags []*gedcom.Tag

	// Fetch all names for this person
	names, err := readStore.GetPersonNames(ctx, p.ID)
	if err != nil || len(names) == 0 {
		// Fallback to person's primary name fields if no names in person_names table
		name := formatGedcomName(p.GivenName, p.Surname)
		tags = append(tags, &gedcom.Tag{Level: 1, Tag: "NAME", Value: name})
	} else {
		// Sort names: primary first, then others
		sortedNames := sortNamesByPrimary(names)
		for _, nm := range sortedNames {
			tags = append(tags, nameToTags(nm)...)
		}
	}

	// Sex
	if p.Gender != "" {
		switch p.Gender {
		case domain.GenderMale:
			tags = append(tags, &gedcom.Tag{Level: 1, Tag: "SEX", Value: "M"})
		case domain.GenderFemale:
			tags = append(tags, &gedcom.Tag{Level: 1, Tag: "SEX", Value: "F"})
		}
	}

	// Birth event
	if p.BirthDateRaw != "" || p.BirthPlace != "" {
		tags = append(tags, &gedcom.Tag{Level: 1, Tag: "BIRT"})
		if p.BirthDateRaw != "" {
			tags = append(tags, &gedcom.Tag{Level: 2, Tag: "DATE", Value: p.BirthDateRaw})
		}
		if p.BirthPlace != "" {
			tags = append(tags, &gedcom.Tag{Level: 2, Tag: "PLAC", Value: p.BirthPlace})
			// Add MAP structure with coordinates if present
			tags = append(tags, placeCoordinatesToTags(p.BirthPlaceLat, p.BirthPlaceLong, 3)...)
		}
		// Citations for birth
		tags = append(tags, citationsToTags(birthCitations, sourceXrefs, 2)...)
	}

	// Death event
	if p.DeathDateRaw != "" || p.DeathPlace != "" {
		tags = append(tags, &gedcom.Tag{Level: 1, Tag: "DEAT"})
		if p.DeathDateRaw != "" {
			tags = append(tags, &gedcom.Tag{Level: 2, Tag: "DATE", Value: p.DeathDateRaw})
		}
		if p.DeathPlace != "" {
			tags = append(tags, &gedcom.Tag{Level: 2, Tag: "PLAC", Value: p.DeathPlace})
			// Add MAP structure with coordinates if present
			tags = append(tags, placeCoordinatesToTags(p.DeathPlaceLat, p.DeathPlaceLong, 3)...)
		}
		// Citations for death
		tags = append(tags, citationsToTags(deathCitations, sourceXrefs, 2)...)
	}

	// Additional life events (burial, baptism, emigration, etc.)
	tags = append(tags, eventsToTags(events, sourceXrefs, 1, readStore, ctx)...)

	// Attributes (occupation, residence, education, etc.)
	tags = append(tags, attributesToTags(attributes, sourceXrefs, 1)...)

	// Notes with CONT for multiline
	if p.Notes != "" {
		tags = append(tags, notesToTags(p.Notes, 1)...)
	}

	return tags
}

// toGedcomFamilyTags converts a repository FamilyReadModel to gedcom.Tag slice.
func toGedcomFamilyTags(f repository.FamilyReadModel, personXrefs, sourceXrefs map[uuid.UUID]string, children []repository.FamilyChildReadModel, marriageCitations []repository.CitationReadModel, events []repository.EventReadModel, readStore repository.ReadModelStore, ctx context.Context) []*gedcom.Tag {
	var tags []*gedcom.Tag

	// Husband (Partner1)
	if f.Partner1ID != nil {
		if pXref, ok := personXrefs[*f.Partner1ID]; ok {
			tags = append(tags, &gedcom.Tag{Level: 1, Tag: "HUSB", Value: pXref})
		}
	}

	// Wife (Partner2)
	if f.Partner2ID != nil {
		if pXref, ok := personXrefs[*f.Partner2ID]; ok {
			tags = append(tags, &gedcom.Tag{Level: 1, Tag: "WIFE", Value: pXref})
		}
	}

	// Marriage event
	if f.RelationshipType == domain.RelationMarriage || f.MarriageDateRaw != "" || f.MarriagePlace != "" {
		tags = append(tags, &gedcom.Tag{Level: 1, Tag: "MARR"})
		if f.MarriageDateRaw != "" {
			tags = append(tags, &gedcom.Tag{Level: 2, Tag: "DATE", Value: f.MarriageDateRaw})
		}
		if f.MarriagePlace != "" {
			tags = append(tags, &gedcom.Tag{Level: 2, Tag: "PLAC", Value: f.MarriagePlace})
			// Add MAP structure with coordinates if present
			tags = append(tags, placeCoordinatesToTags(f.MarriagePlaceLat, f.MarriagePlaceLong, 3)...)
		}
		// Citations for marriage
		tags = append(tags, citationsToTags(marriageCitations, sourceXrefs, 2)...)
	}

	// Additional family events (divorce, annulment, engagement, etc.)
	tags = append(tags, eventsToTags(events, sourceXrefs, 1, readStore, ctx)...)

	// Children
	for _, c := range children {
		if pXref, ok := personXrefs[c.PersonID]; ok {
			tags = append(tags, &gedcom.Tag{Level: 1, Tag: "CHIL", Value: pXref})
		}
	}

	return tags
}

// citationsToTags converts repository citations to gedcom.Tag slice.
func citationsToTags(citations []repository.CitationReadModel, sourceXrefs map[uuid.UUID]string, level int) []*gedcom.Tag {
	var tags []*gedcom.Tag

	for _, cit := range citations {
		srcXref, ok := sourceXrefs[cit.SourceID]
		if !ok {
			continue
		}

		// SOUR tag with source XRef
		tags = append(tags, &gedcom.Tag{Level: level, Tag: "SOUR", Value: srcXref})

		// PAGE
		if cit.Page != "" {
			tags = append(tags, &gedcom.Tag{Level: level + 1, Tag: "PAGE", Value: cit.Page})
		}

		// QUAY (quality) - always output if any GPS quality info present
		if cit.SourceQuality != "" || cit.InformantType != "" || cit.EvidenceType != "" {
			quay := mapGPSToGedcomQuality(cit.SourceQuality, cit.InformantType, cit.EvidenceType)
			tags = append(tags, &gedcom.Tag{Level: level + 1, Tag: "QUAY", Value: strconv.Itoa(quay)})
		}

		// DATA with TEXT (quoted text)
		if cit.QuotedText != "" {
			tags = append(tags, &gedcom.Tag{Level: level + 1, Tag: "DATA"})
			// Handle multiline quoted text with CONT
			lines := strings.Split(cit.QuotedText, "\n")
			tags = append(tags, &gedcom.Tag{Level: level + 2, Tag: "TEXT", Value: lines[0]})
			for _, line := range lines[1:] {
				tags = append(tags, &gedcom.Tag{Level: level + 3, Tag: "CONT", Value: line})
			}
		}
	}

	return tags
}

// notesToTags converts notes text to tags with CONT for multiline.
func notesToTags(notes string, level int) []*gedcom.Tag {
	var tags []*gedcom.Tag
	lines := strings.Split(notes, "\n")
	tags = append(tags, &gedcom.Tag{Level: level, Tag: "NOTE", Value: lines[0]})
	for _, line := range lines[1:] {
		tags = append(tags, &gedcom.Tag{Level: level + 1, Tag: "CONT", Value: line})
	}
	return tags
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

// nameToTags converts a PersonNameReadModel to GEDCOM tags.
func nameToTags(nm repository.PersonNameReadModel) []*gedcom.Tag {
	var tags []*gedcom.Tag

	// Main NAME tag with value in GEDCOM format
	name := formatGedcomName(nm.GivenName, nm.Surname)
	tags = append(tags, &gedcom.Tag{Level: 1, Tag: "NAME", Value: name})

	// TYPE - only add if not birth (birth is the default)
	if nm.NameType != "" && nm.NameType != domain.NameTypeBirth {
		tags = append(tags, &gedcom.Tag{Level: 2, Tag: "TYPE", Value: mapNameTypeToGedcom(nm.NameType)})
	}

	// NPFX - Name prefix (Dr., Rev., Sir)
	if nm.NamePrefix != "" {
		tags = append(tags, &gedcom.Tag{Level: 2, Tag: "NPFX", Value: nm.NamePrefix})
	}

	// NSFX - Name suffix (Jr., III, PhD)
	if nm.NameSuffix != "" {
		tags = append(tags, &gedcom.Tag{Level: 2, Tag: "NSFX", Value: nm.NameSuffix})
	}

	// SPFX - Surname prefix (von, de, van)
	if nm.SurnamePrefix != "" {
		tags = append(tags, &gedcom.Tag{Level: 2, Tag: "SPFX", Value: nm.SurnamePrefix})
	}

	// NICK - Nickname
	if nm.Nickname != "" {
		tags = append(tags, &gedcom.Tag{Level: 2, Tag: "NICK", Value: nm.Nickname})
	}

	return tags
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

// eventsToTags converts a slice of EventReadModel to gedcom.Tag slice.
func eventsToTags(events []repository.EventReadModel, sourceXrefs map[uuid.UUID]string, level int, readStore repository.ReadModelStore, ctx context.Context) []*gedcom.Tag {
	var tags []*gedcom.Tag

	for _, event := range events {
		tagName := mapFactTypeToGedcomTag(event.FactType)
		if tagName == "" {
			continue // Skip unknown event types
		}

		// Add event tag
		tags = append(tags, &gedcom.Tag{Level: level, Tag: tagName})

		// Add DATE subordinate if present
		if event.DateRaw != "" {
			tags = append(tags, &gedcom.Tag{Level: level + 1, Tag: "DATE", Value: event.DateRaw})
		}

		// Add PLAC subordinate if present
		if event.Place != "" {
			tags = append(tags, &gedcom.Tag{Level: level + 1, Tag: "PLAC", Value: event.Place})
			// Add MAP structure with coordinates if present
			tags = append(tags, placeCoordinatesToTags(event.PlaceLat, event.PlaceLong, level+2)...)
		}

		// Add CAUS subordinate if present (for death/burial events)
		if event.Cause != "" {
			tags = append(tags, &gedcom.Tag{Level: level + 1, Tag: "CAUS", Value: event.Cause})
		}

		// Add AGE subordinate if present
		if event.Age != "" {
			tags = append(tags, &gedcom.Tag{Level: level + 1, Tag: "AGE", Value: event.Age})
		}

		// Fetch and add citations for this event
		if readStore != nil {
			citations, _ := readStore.GetCitationsForFact(ctx, event.FactType, event.OwnerID)
			tags = append(tags, citationsToTags(citations, sourceXrefs, level+1)...)
		}
	}

	return tags
}

// attributesToTags converts a slice of AttributeReadModel to gedcom.Tag slice.
// TODO: sourceXrefs is reserved for linking attributes to source citations
func attributesToTags(attributes []repository.AttributeReadModel, _ map[uuid.UUID]string, level int) []*gedcom.Tag {
	var tags []*gedcom.Tag

	for _, attr := range attributes {
		tagName := mapFactTypeToGedcomTag(attr.FactType)
		if tagName == "" {
			continue // Skip unknown attribute types
		}

		// Add attribute tag with value
		tags = append(tags, &gedcom.Tag{Level: level, Tag: tagName, Value: attr.Value})

		// Add DATE subordinate if present
		if attr.DateRaw != "" {
			tags = append(tags, &gedcom.Tag{Level: level + 1, Tag: "DATE", Value: attr.DateRaw})
		}

		// Add PLAC subordinate if present
		if attr.Place != "" {
			tags = append(tags, &gedcom.Tag{Level: level + 1, Tag: "PLAC", Value: attr.Place})
		}
	}

	return tags
}

// placeCoordinatesToTags generates MAP/LATI/LONG tags if coordinates are present.
// The level parameter is the level at which MAP should be written (subordinate to PLAC).
func placeCoordinatesToTags(lat, long *string, level int) []*gedcom.Tag {
	// Only generate MAP structure if both coordinates are present
	if lat == nil || long == nil || *lat == "" || *long == "" {
		return nil
	}

	return []*gedcom.Tag{
		{Level: level, Tag: "MAP"},
		{Level: level + 1, Tag: "LATI", Value: *lat},
		{Level: level + 1, Tag: "LONG", Value: *long},
	}
}
