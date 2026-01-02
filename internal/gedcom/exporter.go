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
	BytesWritten      int64
	PersonsExported   int
	FamiliesExported  int
	SourcesExported   int
	CitationsExported int
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

		tags := toGedcomIndividualTags(p, sourceXrefs, birthCitations, deathCitations)
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

		tags := toGedcomFamilyTags(f, personXrefs, sourceXrefs, children, marriageCitations)
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
func toGedcomIndividualTags(p repository.PersonReadModel, sourceXrefs map[uuid.UUID]string, birthCitations, deathCitations []repository.CitationReadModel) []*gedcom.Tag {
	var tags []*gedcom.Tag

	// Name
	name := formatGedcomName(p.GivenName, p.Surname)
	tags = append(tags, &gedcom.Tag{Level: 1, Tag: "NAME", Value: name})

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
		}
		// Citations for death
		tags = append(tags, citationsToTags(deathCitations, sourceXrefs, 2)...)
	}

	// Notes with CONT for multiline
	if p.Notes != "" {
		tags = append(tags, notesToTags(p.Notes, 1)...)
	}

	return tags
}

// toGedcomFamilyTags converts a repository FamilyReadModel to gedcom.Tag slice.
func toGedcomFamilyTags(f repository.FamilyReadModel, personXrefs, sourceXrefs map[uuid.UUID]string, children []repository.FamilyChildReadModel, marriageCitations []repository.CitationReadModel) []*gedcom.Tag {
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
		}
		// Citations for marriage
		tags = append(tags, citationsToTags(marriageCitations, sourceXrefs, 2)...)
	}

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
func mapGPSToGedcomQuality(sourceQuality domain.SourceQuality, informantType domain.InformantType, evidenceType domain.EvidenceType) int {
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
