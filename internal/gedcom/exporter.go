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
	BytesWritten     int64
	PersonsExported  int
	FamiliesExported int
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

	// Create XREF mappings (UUID -> @Xn@)
	personXrefs := make(map[uuid.UUID]string)
	familyXrefs := make(map[uuid.UUID]string)

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

	// Buffer for building GEDCOM
	buf := &bytes.Buffer{}

	// Write header
	exp.writeHeader(buf)

	// Write individuals
	for _, p := range persons {
		exp.writeIndividual(ctx, buf, p, personXrefs)
		result.PersonsExported++
	}

	// Write families
	for _, f := range families {
		exp.writeFamily(ctx, buf, f, personXrefs, familyXrefs)
		result.FamiliesExported++
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

// writeIndividual writes an individual (INDI) record.
func (exp *Exporter) writeIndividual(ctx context.Context, buf *bytes.Buffer, p repository.PersonReadModel, xrefs map[uuid.UUID]string) {
	xref := xrefs[p.ID]
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

	// Birth event
	if p.BirthDateRaw != "" || p.BirthPlace != "" {
		buf.WriteString("1 BIRT\n")
		if p.BirthDateRaw != "" {
			buf.WriteString(fmt.Sprintf("2 DATE %s\n", p.BirthDateRaw))
		}
		if p.BirthPlace != "" {
			buf.WriteString(fmt.Sprintf("2 PLAC %s\n", p.BirthPlace))
		}
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
}

// writeFamily writes a family (FAM) record.
func (exp *Exporter) writeFamily(ctx context.Context, buf *bytes.Buffer, f repository.FamilyReadModel, personXrefs, familyXrefs map[uuid.UUID]string) {
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

	// Marriage event
	if f.RelationshipType == domain.RelationMarriage || f.MarriageDateRaw != "" || f.MarriagePlace != "" {
		buf.WriteString("1 MARR\n")
		if f.MarriageDateRaw != "" {
			buf.WriteString(fmt.Sprintf("2 DATE %s\n", f.MarriageDateRaw))
		}
		if f.MarriagePlace != "" {
			buf.WriteString(fmt.Sprintf("2 PLAC %s\n", f.MarriagePlace))
		}
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
}

// formatGedcomName formats a name in GEDCOM format (Given /Surname/).
// GEDCOM requires surname delimiters even when surname is empty.
func formatGedcomName(givenName, surname string) string {
	// Always include surname delimiters per GEDCOM spec
	return fmt.Sprintf("%s /%s/", givenName, surname)
}
