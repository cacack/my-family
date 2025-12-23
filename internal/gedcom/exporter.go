package gedcom

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"sort"
	"strings"
	"time"

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

	// Buffer for building GEDCOM
	buf := &bytes.Buffer{}

	// Write header
	exp.writeHeader(buf)

	// Write sources
	for _, s := range sources {
		exp.writeSource(buf, s, sourceXrefs)
		result.SourcesExported++
	}

	// Write individuals
	for _, p := range persons {
		citationCount := exp.writeIndividual(ctx, buf, p, personXrefs, sourceXrefs)
		result.PersonsExported++
		result.CitationsExported += citationCount
	}

	// Write families
	for _, f := range families {
		citationCount := exp.writeFamily(ctx, buf, f, personXrefs, familyXrefs, sourceXrefs)
		result.FamiliesExported++
		result.CitationsExported += citationCount
	}

	// Write trailer
	buf.WriteString("0 TRLR\n")

	// Write to output
	n, err := w.Write(buf.Bytes())
	result.BytesWritten = int64(n)
	if err != nil {
		return result, fmt.Errorf("failed to write GEDCOM: %w", err)
	}

	return result, nil
}

// writeHeader writes the GEDCOM header.
func (exp *Exporter) writeHeader(buf *bytes.Buffer) {
	buf.WriteString("0 HEAD\n")
	buf.WriteString("1 SOUR MyFamily\n")
	buf.WriteString("2 VERS 1.0\n")
	buf.WriteString("2 NAME My Family Genealogy\n")
	buf.WriteString("1 GEDC\n")
	buf.WriteString("2 VERS 5.5\n")
	buf.WriteString("2 FORM LINEAGE-LINKED\n")
	buf.WriteString("1 CHAR UTF-8\n")
	buf.WriteString(fmt.Sprintf("1 DATE %s\n", time.Now().Format("2 Jan 2006")))
}

// writeIndividual writes an individual (INDI) record and returns citation count.
func (exp *Exporter) writeIndividual(ctx context.Context, buf *bytes.Buffer, p repository.PersonReadModel, personXrefs, sourceXrefs map[uuid.UUID]string) int {
	xref := personXrefs[p.ID]
	buf.WriteString(fmt.Sprintf("0 %s INDI\n", xref))

	// Name
	name := formatGedcomName(p.GivenName, p.Surname)
	buf.WriteString(fmt.Sprintf("1 NAME %s\n", name))

	// Sex
	if p.Gender != "" {
		switch p.Gender {
		case domain.GenderMale:
			buf.WriteString("1 SEX M\n")
		case domain.GenderFemale:
			buf.WriteString("1 SEX F\n")
		}
	}

	citationCount := 0

	// Birth event
	if p.BirthDateRaw != "" || p.BirthPlace != "" {
		buf.WriteString("1 BIRT\n")
		if p.BirthDateRaw != "" {
			buf.WriteString(fmt.Sprintf("2 DATE %s\n", p.BirthDateRaw))
		}
		if p.BirthPlace != "" {
			buf.WriteString(fmt.Sprintf("2 PLAC %s\n", p.BirthPlace))
		}
		// Write citations for birth
		citationCount += exp.writeCitationsForFact(ctx, buf, domain.FactPersonBirth, p.ID, sourceXrefs, 2)
	}

	// Death event
	if p.DeathDateRaw != "" || p.DeathPlace != "" {
		buf.WriteString("1 DEAT\n")
		if p.DeathDateRaw != "" {
			buf.WriteString(fmt.Sprintf("2 DATE %s\n", p.DeathDateRaw))
		}
		if p.DeathPlace != "" {
			buf.WriteString(fmt.Sprintf("2 PLAC %s\n", p.DeathPlace))
		}
		// Write citations for death
		citationCount += exp.writeCitationsForFact(ctx, buf, domain.FactPersonDeath, p.ID, sourceXrefs, 2)
	}

	// Notes
	if p.Notes != "" {
		// GEDCOM notes need to handle line breaks
		lines := strings.Split(p.Notes, "\n")
		buf.WriteString("1 NOTE ")
		buf.WriteString(lines[0])
		buf.WriteString("\n")
		for _, line := range lines[1:] {
			buf.WriteString("2 CONT ")
			buf.WriteString(line)
			buf.WriteString("\n")
		}
	}

	return citationCount
}

// writeFamily writes a family (FAM) record and returns citation count.
func (exp *Exporter) writeFamily(ctx context.Context, buf *bytes.Buffer, f repository.FamilyReadModel, personXrefs, familyXrefs, sourceXrefs map[uuid.UUID]string) int {
	xref := familyXrefs[f.ID]
	buf.WriteString(fmt.Sprintf("0 %s FAM\n", xref))

	// Husband (Partner1)
	if f.Partner1ID != nil {
		if pXref, ok := personXrefs[*f.Partner1ID]; ok {
			buf.WriteString(fmt.Sprintf("1 HUSB %s\n", pXref))
		}
	}

	// Wife (Partner2)
	if f.Partner2ID != nil {
		if pXref, ok := personXrefs[*f.Partner2ID]; ok {
			buf.WriteString(fmt.Sprintf("1 WIFE %s\n", pXref))
		}
	}

	citationCount := 0

	// Marriage event
	if f.RelationshipType == domain.RelationMarriage || f.MarriageDateRaw != "" || f.MarriagePlace != "" {
		buf.WriteString("1 MARR\n")
		if f.MarriageDateRaw != "" {
			buf.WriteString(fmt.Sprintf("2 DATE %s\n", f.MarriageDateRaw))
		}
		if f.MarriagePlace != "" {
			buf.WriteString(fmt.Sprintf("2 PLAC %s\n", f.MarriagePlace))
		}
		// Write citations for marriage
		citationCount += exp.writeCitationsForFact(ctx, buf, domain.FactFamilyMarriage, f.ID, sourceXrefs, 2)
	}

	// Children
	children, err := exp.readStore.GetFamilyChildren(ctx, f.ID)
	if err == nil {
		for _, c := range children {
			if pXref, ok := personXrefs[c.PersonID]; ok {
				buf.WriteString(fmt.Sprintf("1 CHIL %s\n", pXref))
			}
		}
	}

	return citationCount
}

// writeSource writes a source (SOUR) record.
func (exp *Exporter) writeSource(buf *bytes.Buffer, s repository.SourceReadModel, sourceXrefs map[uuid.UUID]string) {
	xref := sourceXrefs[s.ID]
	buf.WriteString(fmt.Sprintf("0 %s SOUR\n", xref))

	// Title (required)
	if s.Title != "" {
		buf.WriteString(fmt.Sprintf("1 TITL %s\n", s.Title))
	}

	// Author
	if s.Author != "" {
		buf.WriteString(fmt.Sprintf("1 AUTH %s\n", s.Author))
	}

	// Publisher
	if s.Publisher != "" {
		buf.WriteString(fmt.Sprintf("1 PUBL %s\n", s.Publisher))
	}

	// Repository (as reference or inline)
	if s.RepositoryName != "" {
		// If it looks like an XREF, write as reference; otherwise inline
		if strings.HasPrefix(s.RepositoryName, "@") && strings.HasSuffix(s.RepositoryName, "@") {
			buf.WriteString(fmt.Sprintf("1 REPO %s\n", s.RepositoryName))
		} else {
			buf.WriteString(fmt.Sprintf("1 REPO\n2 NAME %s\n", s.RepositoryName))
		}
	}

	// Notes
	if s.Notes != "" {
		lines := strings.Split(s.Notes, "\n")
		buf.WriteString("1 NOTE ")
		buf.WriteString(lines[0])
		buf.WriteString("\n")
		for _, line := range lines[1:] {
			buf.WriteString("2 CONT ")
			buf.WriteString(line)
			buf.WriteString("\n")
		}
	}
}

// writeCitationsForFact writes citations for a specific fact and returns count.
func (exp *Exporter) writeCitationsForFact(ctx context.Context, buf *bytes.Buffer, factType domain.FactType, factOwnerID uuid.UUID, sourceXrefs map[uuid.UUID]string, level int) int {
	citations, err := exp.readStore.GetCitationsForFact(ctx, factType, factOwnerID)
	if err != nil || len(citations) == 0 {
		return 0
	}

	levelStr := fmt.Sprintf("%d", level)
	subLevelStr := fmt.Sprintf("%d", level+1)

	for _, cit := range citations {
		// Get source XREF
		srcXref, ok := sourceXrefs[cit.SourceID]
		if !ok {
			continue
		}

		// Write SOUR tag with reference
		buf.WriteString(fmt.Sprintf("%s SOUR %s\n", levelStr, srcXref))

		// Write PAGE
		if cit.Page != "" {
			buf.WriteString(fmt.Sprintf("%s PAGE %s\n", subLevelStr, cit.Page))
		}

		// Write QUAY (quality)
		if cit.SourceQuality != "" || cit.InformantType != "" || cit.EvidenceType != "" {
			quay := mapGPSToGedcomQuality(cit.SourceQuality, cit.InformantType, cit.EvidenceType)
			buf.WriteString(fmt.Sprintf("%s QUAY %d\n", subLevelStr, quay))
		}

		// Write DATA with TEXT (quoted text)
		if cit.QuotedText != "" {
			buf.WriteString(fmt.Sprintf("%s DATA\n", subLevelStr))
			lines := strings.Split(cit.QuotedText, "\n")
			buf.WriteString(fmt.Sprintf("%d TEXT %s\n", level+2, lines[0]))
			for _, line := range lines[1:] {
				buf.WriteString(fmt.Sprintf("%d CONT %s\n", level+3, line))
			}
		}
	}

	return len(citations)
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
