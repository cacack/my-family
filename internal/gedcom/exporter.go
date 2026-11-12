package gedcom

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"slices"
	"sort"
	"strings"

	"github.com/cacack/gedcom-go/v3/converter"
	"github.com/cacack/gedcom-go/v3/decoder"
	"github.com/cacack/gedcom-go/v3/encoder"
	"github.com/cacack/gedcom-go/v3/gedcom"
	"github.com/google/uuid"

	"github.com/cacack/my-family/internal/domain"
	"github.com/cacack/my-family/internal/repository"
)

// DataLossDetail describes one feature that could not be represented when a
// GEDCOM export was downgraded to an older version (e.g. 7.0 data emitted as
// 5.5.1). It mirrors gedcom.DataLossItem in a JSON-friendly shape.
type DataLossDetail struct {
	// Feature is the name of the lost feature (e.g. "EXID external IDs").
	Feature string `json:"feature"`
	// Reason explains why it was lost (e.g. "Not supported in GEDCOM 5.5.1").
	Reason string `json:"reason"`
	// AffectedRecords lists the XREFs of records that were affected.
	AffectedRecords []string `json:"affectedRecords,omitempty"`
}

// ExportResult contains the results of a GEDCOM export operation.
type ExportResult struct {
	// Version is the GEDCOM version that was actually emitted.
	Version gedcom.Version
	// SourceVersion is the version the data naturally requires before any
	// downgrade conversion. It equals Version unless a downgrade occurred, in
	// which case it is the higher source version (e.g. 7.0 → 5.5.1).
	SourceVersion gedcom.Version
	// DataLoss lists features dropped when a downgrade conversion could not
	// represent them in the target version. Empty when nothing was lost.
	DataLoss []DataLossDetail
	// Warnings lists problems the export could not repair, such as a NOTE
	// pointer whose stored xref several NOTE records share (#1228) or that
	// points to no exported note and was dropped (#1353).
	Warnings              []string
	BytesWritten          int64
	PersonsExported       int
	FamiliesExported      int
	SourcesExported       int
	CitationsExported     int
	EventsExported        int
	AttributesExported    int
	NotesExported         int
	SubmittersExported    int
	RepositoriesExported  int
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

// ExportOptions configures a GEDCOM export.
type ExportOptions struct {
	// OnProgress, if non-nil, is called periodically to report progress.
	OnProgress ProgressCallback

	// TargetVersion selects the GEDCOM version to emit. When empty, the
	// exporter defaults to 5.5 and automatically upgrades to 7.0 if the data
	// uses 7.0-only structures. When set, the chosen version is emitted as-is
	// (the auto-upgrade rule is not applied, so the caller's choice wins).
	TargetVersion gedcom.Version
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
// For progress tracking during large exports, use ExportWithProgress instead.
func (exp *Exporter) Export(ctx context.Context, w io.Writer) (*ExportResult, error) {
	return exp.ExportWithProgress(ctx, w, nil)
}

// ExportWithProgress generates a GEDCOM 5.5 file with optional progress callback.
// The onProgress callback is called periodically to report export progress.
// Pass nil for onProgress to disable progress tracking.
func (exp *Exporter) ExportWithProgress(ctx context.Context, w io.Writer, onProgress ProgressCallback) (*ExportResult, error) {
	return exp.ExportWithOptions(ctx, w, ExportOptions{OnProgress: onProgress})
}

// ExportWithOptions generates a GEDCOM file using the supplied options.
// It is the underlying implementation for Export and ExportWithProgress.
func (exp *Exporter) ExportWithOptions(ctx context.Context, w io.Writer, opts ExportOptions) (*ExportResult, error) {
	onProgress := opts.OnProgress
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

	repositories, err := repository.ListAll(ctx, 1000, exp.readStore.ListRepositories)
	if err != nil {
		return nil, fmt.Errorf("failed to list repositories: %w", err)
	}

	externalIDs, err := loadExternalIDs(ctx, exp.readStore)
	if err != nil {
		return nil, err
	}

	// Calculate total items for progress tracking
	// Weight persons more heavily since they have the most processing
	totalItems := len(sources) + len(persons)*2 + len(families) + len(notes) + len(submitters) + len(repositories) + 1 // +1 for encoding
	processedItems := 0

	// Create XREF mappings (UUID -> @Xn@)
	personXrefs := make(map[uuid.UUID]string)
	familyXrefs := make(map[uuid.UUID]string)
	sourceXrefs := make(map[uuid.UUID]string)
	repositoryXrefs := make(map[uuid.UUID]string)
	noteXrefs := make(map[uuid.UUID]string)

	// Sort persons by ID for stable output
	sort.Slice(persons, func(i, j int) bool {
		return persons[i].ID.String() < persons[j].ID.String()
	})
	// Sort submitters by ID for stable output
	sort.Slice(submitters, func(i, j int) bool {
		return submitters[i].ID.String() < submitters[j].ID.String()
	})

	// Every stored xref is reserved before any is generated, so a generated
	// xref never duplicates another record's stored one.
	var storedXrefs []string
	for _, s := range sources {
		storedXrefs = append(storedXrefs, s.GedcomXref)
	}
	for _, r := range repositories {
		storedXrefs = append(storedXrefs, r.GedcomXref)
	}
	for _, n := range notes {
		storedXrefs = append(storedXrefs, n.GedcomXref)
	}
	for _, s := range submitters {
		storedXrefs = append(storedXrefs, s.GedcomXref)
	}
	xrefs := newXrefAllocator(storedXrefs)

	for i, p := range persons {
		personXrefs[p.ID] = xrefs.assign("", "I", i+1)
	}

	// Sort families by ID for stable output
	sort.Slice(families, func(i, j int) bool {
		return families[i].ID.String() < families[j].ID.String()
	})
	for i, f := range families {
		familyXrefs[f.ID] = xrefs.assign("", "F", i+1)
	}

	// Sort sources by ID for stable output
	sort.Slice(sources, func(i, j int) bool {
		return sources[i].ID.String() < sources[j].ID.String()
	})
	for i, s := range sources {
		// Use GedcomXref if available (for round-trip), otherwise generate
		sourceXrefs[s.ID] = xrefs.assign(s.GedcomXref, "S", i+1)
	}

	// Sort repositories by ID for stable output and assign xrefs.
	sort.Slice(repositories, func(i, j int) bool {
		return repositories[i].ID.String() < repositories[j].ID.String()
	})
	// repoNameToXref lets sources that reference a repository by name be linked
	// to the standalone REPO record via a SOUR.REPO cross-reference. repoIDToXref
	// is the authoritative ID-based map, preferred over name matching (issue #525).
	repoNameToXref := make(map[string]string)
	repoIDToXref := make(map[uuid.UUID]string)
	// A SOUR.REPO pointer kept in RepositoryName and a NOTE pointer kept in
	// Notes name their record by its raw stored xref; each resolves against
	// records of its own type only (#1228).
	repoPointers := newPointerTargets("REPO", &result.Warnings)
	for i, r := range repositories {
		// Use GedcomXref if available (for round-trip), otherwise generate.
		repositoryXrefs[r.ID] = xrefs.assign(r.GedcomXref, "R", i+1)
		repoIDToXref[r.ID] = repositoryXrefs[r.ID]
		repoPointers.add(r.GedcomXref, repositoryXrefs[r.ID])
		if r.Name != "" {
			repoNameToXref[r.Name] = repositoryXrefs[r.ID]
		}
	}

	// Sort notes by ID for stable output. Their xrefs are assigned before any
	// record is built so that every NOTE pointer can be resolved.
	sort.Slice(notes, func(i, j int) bool {
		return notes[i].ID.String() < notes[j].ID.String()
	})
	notePointers := newPointerTargets("NOTE", &result.Warnings)
	for i, n := range notes {
		// Use GedcomXref if available (for round-trip), otherwise generate
		noteXrefs[n.ID] = xrefs.assign(n.GedcomXref, "N", i+1)
		notePointers.add(n.GedcomXref, noteXrefs[n.ID])
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
		s.Notes = notePointers.remapNotes(s.Notes, noteJoin, "SOUR "+xref)
		s.RepositoryName = repoPointers.remap(s.RepositoryName, noteJoin)
		src := toGedcomSource(s, repoIDToXref, repoNameToXref)
		src.ExternalIDs = externalIDs.sources[s.ID]
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

	childLinks, err := pedigreeLinks(ctx, exp.readStore, familyXrefs)
	if err != nil {
		return nil, err
	}
	spouseFamilies := spouseLinks(families, familyXrefs)

	// Add individual records
	for i, p := range persons {
		xref := personXrefs[p.ID]
		// Fetch citations for this person's events
		birthCitations, _ := exp.readStore.GetCitationsForFact(ctx, domain.MainBranchID, domain.FactPersonBirth, p.ID)
		deathCitations, _ := exp.readStore.GetCitationsForFact(ctx, domain.MainBranchID, domain.FactPersonDeath, p.ID)
		nameCitations, _ := exp.readStore.GetCitationsForFact(ctx, domain.MainBranchID, domain.FactPersonName, p.ID)

		// Fetch this person's names, their NOTE pointers remapped like the person's
		names, _ := exp.readStore.GetPersonNames(ctx, domain.MainBranchID, p.ID)
		for j := range names {
			names[j].Notes = notePointers.remapNotes(names[j].Notes, noteJoin, "INDI "+xref)
		}

		// Fetch events and attributes for this person
		events, _ := exp.readStore.ListEventsForPerson(ctx, domain.MainBranchID, p.ID)
		notePointers.remapEvents(events, "INDI "+xref)
		attributes, _ := exp.readStore.ListAttributesForPerson(ctx, domain.MainBranchID, p.ID)
		for j := range attributes {
			attributes[j].Notes = notePointers.remapNotes(attributes[j].Notes, noteJoin, "INDI "+xref)
		}
		result.EventsExported += len(events)
		result.AttributesExported += len(attributes)

		// Fetch associations for this person (where this person is the PersonID)
		allAssocs, _ := exp.readStore.ListAssociationsForPerson(ctx, domain.MainBranchID, p.ID)
		// Filter to only associations where this person is the PersonID (the one who has the association)
		var associations []repository.AssociationReadModel
		for _, a := range allAssocs {
			if a.PersonID == p.ID {
				a.Notes = notePointers.remapNotes(a.Notes, associationNoteJoin, "INDI "+xref)
				associations = append(associations, a)
			}
		}
		result.AssociationsExported += len(associations)

		// Fetch LDS ordinances for this person
		ldsOrdinances, _ := exp.readStore.ListLDSOrdinancesForPerson(ctx, p.ID)
		result.LDSOrdinancesExported += len(ldsOrdinances)

		p.Notes = notePointers.remapNotes(p.Notes, noteJoin, "INDI "+xref)
		p.BirthNotes = notePointers.remapNotes(p.BirthNotes, noteJoin, "INDI "+xref)
		p.DeathNotes = notePointers.remapNotes(p.DeathNotes, noteJoin, "INDI "+xref)
		indi := toGedcomIndividual(p, names, sourceXrefs, personXrefs, nameCitations, birthCitations, deathCitations, events, attributes, associations, ldsOrdinances, externalIDs.citationAPIDs, exp.readStore, ctx)
		indi.ExternalIDs = externalIDs.persons[p.ID]
		indi.ChildInFamilies = childLinks[p.ID]
		indi.SpouseInFamilies = spouseFamilies[p.ID]
		result.CitationsExported += individualCitations(indi)
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
		children, _ := exp.readStore.GetFamilyChildren(ctx, domain.MainBranchID, f.ID)
		marriageCitations, _ := exp.readStore.GetCitationsForFact(ctx, domain.MainBranchID, domain.FactFamilyMarriage, f.ID)

		// Fetch events for this family
		familyEvents, _ := exp.readStore.ListEventsForFamily(ctx, domain.MainBranchID, f.ID)
		notePointers.remapEvents(familyEvents, "FAM "+xref)
		f.MarriageNotes = notePointers.remapNotes(f.MarriageNotes, noteJoin, "FAM "+xref)
		f.Notes = notePointers.remapNotes(f.Notes, noteJoin, "FAM "+xref)
		result.EventsExported += len(familyEvents)

		// Fetch LDS ordinances for this family
		familyLDSOrdinances, _ := exp.readStore.ListLDSOrdinancesForFamily(ctx, f.ID)
		result.LDSOrdinancesExported += len(familyLDSOrdinances)

		fam := toGedcomFamily(f, personXrefs, sourceXrefs, children, marriageCitations, familyEvents, familyLDSOrdinances, externalIDs.citationAPIDs, exp.readStore, ctx)
		fam.ExternalIDs = externalIDs.families[f.ID]
		result.CitationsExported += familyCitations(fam)
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

	for i, n := range notes {
		xref := noteXrefs[n.ID]

		// Notes carrying GEDCOM 7.0 metadata (MIME, language, or translations)
		// round-trip as SNOTE records; plain notes stay as NOTE records for
		// GEDCOM 5.5.1 compatibility, and become SNOTE records in a 7.0
		// export (shareNoteRecords).
		doc.Records = append(doc.Records, toGedcomNoteRecord(xref, n))
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

	for i, s := range submitters {
		// Use GedcomXref if available (for round-trip), otherwise generate
		xref := xrefs.assign(s.GedcomXref, "U", i+1)

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

	// Add repository records (REPO). Standalone records that sources reference
	// via SOUR.REPO cross-references.
	for i, r := range repositories {
		xref := repositoryXrefs[r.ID]
		r.Notes = notePointers.remapNotes(r.Notes, noteJoin, "REPO "+xref)
		repo := toGedcomRepository(r)
		repo.ExternalIDs = externalIDs.repositories[r.ID]
		doc.Records = append(doc.Records, &gedcom.Record{
			XRef:   xref,
			Type:   gedcom.RecordTypeRepository,
			Entity: repo,
		})
		result.RepositoriesExported++
		processedItems++

		// Report progress every 10 items or at the end
		if i%10 == 0 || i == len(repositories)-1 {
			pct := float64(processedItems) / float64(totalItems) * 100
			if err := reportProgress("repositories", i+1, len(repositories), pct); err != nil {
				return result, err
			}
		}
	}

	// Resolve the target GEDCOM version. An explicit caller choice always wins.
	// Otherwise default to 5.5 and upgrade to 7.0 if the document uses any
	// 7.0-only structures. The library's RequiresGEDCOM7 inspects the whole
	// document — negated events (NO), EXID, SNOTE, SCHMA, TRAN, association
	// PHRASE, media CROP, SDATE, CREA — so such documents are emitted as 7.0
	// instead of silently lossy 5.5.1 (issue #539). A FamilySearch EXID on a
	// person does not count: the downgrade path writes it as _FSFTID, so
	// FamilySearch trees keep defaulting to 5.5 (#1175).
	targetVersion := opts.TargetVersion
	if targetVersion == "" {
		targetVersion = gedcom.Version55
		if requiresGEDCOM7(doc) {
			targetVersion = gedcom.Version70
		}
	}
	result.Version = targetVersion
	result.SourceVersion = targetVersion

	// Report encoding phase
	if err := reportProgress("encoding", 0, 1, 99.0); err != nil {
		return result, err
	}

	// Use a counting writer to track bytes written
	cw := &countingWriter{w: w}

	if doc.RequiresGEDCOM7() && targetVersion != gedcom.Version70 {
		// Downgrade path (issue #189, Option B): the caller forced a version that
		// cannot represent the document's 7.0-only structures. Transform the
		// document via the converter so the emitted file is internally consistent
		// and report exactly what is dropped — rather than emitting a document
		// mislabeled as 5.5.x that still contains 7.0 structures.
		//
		// The converter operates on parsed tags, which the exporter's
		// entity-based document does not carry, so materialize it first by
		// encoding at 7.0 (lossless) and re-parsing.
		report, err := exp.encodeDowngraded(cw, doc, targetVersion)
		if err != nil {
			return result, err
		}
		result.SourceVersion = report.SourceVersion
		result.DataLoss = toDataLossDetails(report)
	} else {
		if targetVersion == gedcom.Version70 {
			upcasePedigrees(doc)
			shareNoteRecords(doc)
		}
		if err := encoder.EncodeWithOptions(cw, doc, encodeOptions(targetVersion)); err != nil {
			return result, fmt.Errorf("failed to write GEDCOM: %w", err)
		}
	}

	result.BytesWritten = cw.count

	// Report completion
	if err := reportProgress("complete", 1, 1, 100.0); err != nil {
		return result, err
	}

	return result, nil
}

// childPedigrees is the GEDCOM 5.5.1 PEDI value of each non-biological child
// relationship, the one childPedigreeType reads back.
var childPedigrees = map[domain.ChildRelationType]string{
	domain.ChildAdopted: "adopted",
	domain.ChildFoster:  "foster",
}

// pedigreeLinks returns, per person, a FAMC link with PEDI for each family
// the person is an adopted or foster child of, in family xref order. A
// biological child is linked by the family's CHIL alone, as before; without
// the FAMC an imported adopted child came back biological (#622).
func pedigreeLinks(ctx context.Context, readStore repository.ReadModelStore, familyXrefs map[uuid.UUID]string) (map[uuid.UUID][]gedcom.FamilyLink, error) {
	children, err := readStore.ListAllFamilyChildren(ctx, domain.MainBranchID)
	if err != nil {
		return nil, fmt.Errorf("failed to list family children: %w", err)
	}
	links := map[uuid.UUID][]gedcom.FamilyLink{}
	for _, c := range children {
		pedigree, ok := childPedigrees[c.RelationshipType]
		xref := familyXrefs[c.FamilyID]
		if !ok || xref == "" {
			continue
		}
		links[c.PersonID] = append(links[c.PersonID], gedcom.FamilyLink{FamilyXRef: xref, Pedigree: pedigree})
	}
	for _, l := range links {
		sort.Slice(l, func(i, j int) bool { return l[i].FamilyXRef < l[j].FamilyXRef })
	}
	return links, nil
}

// spouseLinks returns, per person, a FAMS link for each family the person is
// a partner in, in family record order. GEDCOM 5.5.1 links spouses in both
// directions, so each HUSB/WIFE has a FAMS pointing back (#1357). families
// are the exported family records; a family without an xref gets no link.
func spouseLinks(families []repository.FamilyReadModel, familyXrefs map[uuid.UUID]string) map[uuid.UUID][]gedcom.FamilyLink {
	links := map[uuid.UUID][]gedcom.FamilyLink{}
	for _, f := range families {
		xref := familyXrefs[f.ID]
		if xref == "" {
			continue
		}
		var linked uuid.UUID
		for _, partner := range []*uuid.UUID{f.Partner1ID, f.Partner2ID} {
			if partner == nil || *partner == linked {
				continue
			}
			links[*partner] = append(links[*partner], gedcom.FamilyLink{FamilyXRef: xref})
			linked = *partner
		}
	}
	return links
}

// upcasePedigrees writes each FAMC's PEDI in GEDCOM 7.0's upper-case
// enumeration form ("ADOPTED" rather than 5.5.1's "adopted").
func upcasePedigrees(doc *gedcom.Document) {
	for _, rec := range doc.Records {
		if indi, ok := rec.GetIndividual(); ok {
			for i := range indi.ChildInFamilies {
				indi.ChildInFamilies[i].Pedigree = strings.ToUpper(indi.ChildInFamilies[i].Pedigree)
			}
		}
	}
}

// shareNoteRecords turns each plain NOTE record into an SNOTE record with the
// same xref and text. GEDCOM 7.0 has no NOTE record, and its NOTE takes text,
// so a pointer left as "NOTE @N1@" is read as the text "@N1@" (#622). The
// encoder writes a 7.0 pointer to an SNOTE record as SNOTE.
func shareNoteRecords(doc *gedcom.Document) {
	for _, rec := range doc.Records {
		if note, ok := rec.Entity.(*gedcom.Note); ok && rec.Type == gedcom.RecordTypeNote {
			rec.Type = gedcom.RecordTypeSharedNote
			rec.Entity = &gedcom.SharedNote{XRef: rec.XRef, Text: note.Text}
		}
	}
}

// exportExternalIDs is every main-branch external identifier the export
// writes, keyed by the ID of the record that owns it. Export reads each kind
// once, before the record loops, rather than once per record (#617), and a
// failed read fails the export instead of silently dropping identifiers. The
// encoder emits each EXID as an `EXID <value>` / `TYPE <uri>` structure, which
// also upgrades the export to GEDCOM 7.0 via RequiresGEDCOM7.
type exportExternalIDs struct {
	persons, families, sources, repositories map[uuid.UUID][]*gedcom.ExternalID
	// citationAPIDs is each citation's Ancestry APID, written as _APID (#1175).
	citationAPIDs map[uuid.UUID]*gedcom.AncestryAPID
}

// loadExternalIDs reads every main-branch external identifier, one query per
// record type.
func loadExternalIDs(ctx context.Context, rs repository.ReadModelStore) (*exportExternalIDs, error) {
	persons, err := rs.ListPersonExternalIDs(ctx, domain.MainBranchID)
	if err != nil {
		return nil, fmt.Errorf("failed to list person external ids: %w", err)
	}
	families, err := rs.ListFamilyExternalIDs(ctx, domain.MainBranchID)
	if err != nil {
		return nil, fmt.Errorf("failed to list family external ids: %w", err)
	}
	sources, err := rs.ListSourceExternalIDs(ctx, domain.MainBranchID)
	if err != nil {
		return nil, fmt.Errorf("failed to list source external ids: %w", err)
	}
	citations, err := rs.ListCitationExternalIDs(ctx, domain.MainBranchID)
	if err != nil {
		return nil, fmt.Errorf("failed to list citation external ids: %w", err)
	}
	repositories, err := rs.ListRepositoryExternalIDs(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to list repository external ids: %w", err)
	}

	citationAPIDs := make(map[uuid.UUID]*gedcom.AncestryAPID, len(citations))
	for citationID, ids := range citations {
		if apid := citationAncestryAPID(ids); apid != nil {
			citationAPIDs[citationID] = apid
		}
	}
	return &exportExternalIDs{
		persons: toGedcomExternalIDs(persons, func(id repository.PersonExternalIDReadModel) (string, string) {
			return id.Value, id.Type
		}),
		families: toGedcomExternalIDs(families, func(id repository.FamilyExternalIDReadModel) (string, string) {
			return id.Value, id.Type
		}),
		sources: toGedcomExternalIDs(sources, func(id repository.SourceExternalIDReadModel) (string, string) {
			return id.Value, id.Type
		}),
		repositories: toGedcomExternalIDs(repositories, func(id repository.RepositoryExternalIDReadModel) (string, string) {
			return id.Value, id.Type
		}),
		citationAPIDs: citationAPIDs,
	}, nil
}

// toGedcomExternalIDs converts each owner's read-model external IDs, in
// sequence order, to GEDCOM EXIDs; field extracts an ID's value and type.
func toGedcomExternalIDs[T any](byOwner map[uuid.UUID][]T, field func(T) (value, typ string)) map[uuid.UUID][]*gedcom.ExternalID {
	out := make(map[uuid.UUID][]*gedcom.ExternalID, len(byOwner))
	for owner, ids := range byOwner {
		for _, id := range ids {
			value, typ := field(id)
			out[owner] = append(out[owner], &gedcom.ExternalID{Value: value, Type: typ})
		}
	}
	return out
}

// citationAncestryAPID returns a citation's Ancestry APID, kept as an
// external ID of type ancestryAPIDExternalIDType (#1175), for export as
// _APID; nil when it has none. The encoder writes only Raw, so a value
// ParseAPID rejects is still written back unchanged.
func citationAncestryAPID(ids []repository.CitationExternalIDReadModel) *gedcom.AncestryAPID {
	for _, ext := range ids {
		if ext.Type != ancestryAPIDExternalIDType {
			continue
		}
		if apid := gedcom.ParseAPID(ext.Value); apid != nil {
			return apid
		}
		return &gedcom.AncestryAPID{Raw: ext.Value}
	}
	return nil
}

// requiresGEDCOM7 reports whether doc needs GEDCOM 7.0 for anything other
// than FamilySearch EXIDs on individuals, which the converter maps losslessly
// to the 5.5.x vendor tag _FSFTID. doc itself is not modified.
func requiresGEDCOM7(doc *gedcom.Document) bool {
	view := *doc
	view.Records = make([]*gedcom.Record, len(doc.Records))
	for i, rec := range doc.Records {
		view.Records[i] = rec
		indi, ok := rec.Entity.(*gedcom.Individual)
		if !ok || len(indi.ExternalIDs) == 0 {
			continue
		}
		indiView := *indi
		indiView.ExternalIDs = nil
		for _, ext := range indi.ExternalIDs {
			if !isFamilySearchExternalIDType(ext.Type) {
				indiView.ExternalIDs = append(indiView.ExternalIDs, ext)
			}
		}
		recView := *rec
		recView.Entity = &indiView
		view.Records[i] = &recView
	}
	return view.RequiresGEDCOM7()
}

// isFamilySearchExternalIDType reports whether an EXID TYPE names a
// FamilySearch ARK, matching the converter's rule for mapping to _FSFTID:
// "ark" must be a whole path segment, so ".../arkansas" does not match.
func isFamilySearchExternalIDType(typeURI string) bool {
	const ark = "familysearch.org/ark"
	lower := strings.ToLower(typeURI)
	idx := strings.Index(lower, ark)
	if idx < 0 {
		return false
	}
	rest := lower[idx+len(ark):]
	return rest == "" || rest[0] == ':' || rest[0] == '/'
}

// encodeDowngraded emits doc to w at targetVersion (an older version than the
// document's 7.0 content requires) and returns the conversion report describing
// what was transformed or dropped. Because the gedcom-go converter works on
// parsed tags rather than the exporter's typed entities, the document is first
// encoded at 7.0 (lossless) and re-parsed, then converted to targetVersion, then
// encoded to w.
func (exp *Exporter) encodeDowngraded(w io.Writer, doc *gedcom.Document, targetVersion gedcom.Version) (*gedcom.ConversionReport, error) {
	var full bytes.Buffer
	if err := encoder.EncodeWithOptions(&full, doc, encodeOptions(gedcom.Version70)); err != nil {
		return nil, fmt.Errorf("failed to encode document for conversion: %w", err)
	}
	// StrictMode keeps this a real check: a lenient decode would silently
	// recover from a malformed line in our own encoder output.
	parsed, err := decoder.DecodeWithOptions(bytes.NewReader(full.Bytes()), &decoder.DecodeOptions{StrictMode: true})
	if err != nil {
		return nil, fmt.Errorf("failed to re-parse document for conversion: %w", err)
	}
	converted, report, err := converter.ConvertWithOptions(parsed, targetVersion, converter.DefaultOptions())
	if err != nil {
		return nil, fmt.Errorf("failed to convert export to %s: %w", targetVersion, err)
	}
	// The converter deliberately keeps vendor extensions through a downgrade
	// (and maps 7.0-only EXIDs to vendor tags like _FSFTID); encodeOptions
	// keeps them in the output. See issue #599.
	if err := encoder.EncodeWithOptions(w, converted, encodeOptions(targetVersion)); err != nil {
		return nil, fmt.Errorf("failed to write GEDCOM: %w", err)
	}
	return report, nil
}

// encodeOptions returns the options for every export encode: LF line endings
// and targetVersion, which selects the emitted version without mutating
// doc.Header.Version. DropUnknownTags is deliberately left unset: its zero
// value keeps vendor "_"-prefixed tags (_FSFTID, _APID, ...) in the output.
func encodeOptions(targetVersion gedcom.Version) *encoder.EncodeOptions {
	return &encoder.EncodeOptions{LineEnding: "\n", TargetVersion: targetVersion}
}

// PreviewConversion reports what a GEDCOM export at targetVersion would change,
// without writing any output. It builds the same document the exporter would
// emit and runs the downgrade conversion, so callers can warn about data loss
// before initiating a download.
//
// This costs the same as a full export (it reads the whole read model and, for
// downgrades, runs the encode/re-parse/convert round trip); only the output
// bytes are discarded. Call it on demand, not reactively. The returned
// ExportResult carries SourceVersion and DataLoss; its entity and byte counts
// are also accurate and safe to display.
func (exp *Exporter) PreviewConversion(ctx context.Context, targetVersion gedcom.Version) (*ExportResult, error) {
	return exp.ExportWithOptions(ctx, io.Discard, ExportOptions{TargetVersion: targetVersion})
}

// toDataLossDetails distills a conversion report's feature-level data loss into
// the JSON-friendly ExportResult representation.
func toDataLossDetails(report *gedcom.ConversionReport) []DataLossDetail {
	if report == nil || len(report.DataLoss) == 0 {
		return nil
	}
	details := make([]DataLossDetail, 0, len(report.DataLoss))
	for _, item := range report.DataLoss {
		details = append(details, DataLossDetail{
			Feature:         item.Feature,
			Reason:          item.Reason,
			AffectedRecords: item.AffectedRecords,
		})
	}
	return details
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
//
// repoIDToXref maps a Repository's ID to the xref of its standalone REPO record;
// it is the authoritative link and is preferred when the source carries a
// RepositoryID. repoNameToXref maps a Repository's name (and any @XREF@ used as a
// name) to the same xref, used as a fallback for legacy sources linked only by
// name. When neither resolves, fall back to an inline repository definition.
func toGedcomSource(s repository.SourceReadModel, repoIDToXref map[uuid.UUID]string, repoNameToXref map[string]string) *gedcom.Source {
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

	// Repository. Prefer the authoritative RepositoryID link (issue #525): it is
	// robust against duplicate or renamed repository names. Fall back to name
	// matching only when no ID is present (e.g. legacy data). Built as a
	// structured RepositoryLink so the CALN subordinate round-trips too.
	repoLink := &gedcom.SourceRepositoryLink{}
	switch {
	case s.RepositoryID != nil && repoIDToXref[*s.RepositoryID] != "":
		// Source links to a Repository entity by ID: emit a SOUR.REPO
		// cross-reference to the standalone REPO record.
		repoLink.XRef = repoIDToXref[*s.RepositoryID]
	case s.RepositoryName == "":
		// No repository link at all.
	case repoNameToXref[s.RepositoryName] != "":
		// Legacy: source references a Repository entity by name.
		repoLink.XRef = repoNameToXref[s.RepositoryName]
	case gedcom.IsPointerXRef(s.RepositoryName):
		// Already an XREF (e.g. an unresolved import reference): preserve it.
		repoLink.XRef = s.RepositoryName
	default:
		// Inline repository with NAME subordinate (name-only / unlinked source).
		repoLink.Inline = &gedcom.InlineRepository{Name: s.RepositoryName}
	}
	if s.CallNumber != "" {
		repoLink.CallNumbers = []*gedcom.CallNumber{{Value: s.CallNumber}}
	}
	if repoLink.XRef != "" || repoLink.Inline != nil || len(repoLink.CallNumbers) > 0 {
		src.RepositoryLinks = []*gedcom.SourceRepositoryLink{repoLink}
	}

	// Notes - encoder handles CONT/CONC automatically for multiline text
	src.NoteXRefs, src.InlineNotes = splitNotes(s.Notes, noteJoin)

	return src
}

// toGedcomIndividual converts a repository PersonReadModel to a gedcom.Individual entity.
// The encoder will automatically convert this to GEDCOM tags, handling CONT/CONC.
// names are the person's names and nameCitations its person_name citations;
// each citation is written under the NAME it is linked to (#1165).
func toGedcomIndividual(p repository.PersonReadModel, names []repository.PersonNameReadModel, sourceXrefs map[uuid.UUID]string, personXrefs map[uuid.UUID]string, nameCitations, birthCitations, deathCitations []repository.CitationReadModel, events []repository.EventReadModel, attributes []repository.AttributeReadModel, associations []repository.AssociationReadModel, ldsOrdinances []repository.LDSOrdinanceReadModel, citationAPIDs map[uuid.UUID]*gedcom.AncestryAPID, readStore repository.ReadModelStore, ctx context.Context) *gedcom.Individual {
	indi := &gedcom.Individual{}

	if len(names) == 0 {
		// Fallback to person's primary name fields if no names in person_names table
		indi.Names = []*gedcom.PersonalName{{
			Full: formatGedcomName(p.GivenName, p.Surname),
		}}
	} else {
		// Sort names: primary first, then others
		names = sortNamesByPrimary(names)
		for _, nm := range names {
			indi.Names = append(indi.Names, toGedcomPersonalName(nm))
		}
	}
	for i, cits := range nameCitationsByName(nameCitations, names, len(indi.Names)) {
		indi.Names[i].SourceCitations = toGedcomSourceCitations(cits, sourceXrefs, citationAPIDs)
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
	if hasBirthField(p) {
		birthEvent := &gedcom.Event{Type: gedcom.EventBirth, EventTypeDetail: p.BirthTypeDetail}
		birthEvent.NoteXRefs, birthEvent.InlineNotes = splitNotes(p.BirthNotes, noteJoin)
		if p.BirthDateRaw != "" {
			birthEvent.Date = p.BirthDateRaw
		}
		if p.BirthPlace != "" {
			birthEvent.SetPlaceName(p.BirthPlace)
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
		birthEvent.SourceCitations = toGedcomSourceCitations(fieldCitations(birthCitations, events), sourceXrefs, citationAPIDs)
		indi.Events = append(indi.Events, birthEvent)
	}

	// Death event
	if hasDeathField(p) {
		deathEvent := &gedcom.Event{Type: gedcom.EventDeath, EventTypeDetail: p.DeathTypeDetail}
		deathEvent.NoteXRefs, deathEvent.InlineNotes = splitNotes(p.DeathNotes, noteJoin)
		if p.DeathDateRaw != "" {
			deathEvent.Date = p.DeathDateRaw
		}
		if p.DeathPlace != "" {
			deathEvent.SetPlaceName(p.DeathPlace)
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
		deathEvent.SourceCitations = toGedcomSourceCitations(fieldCitations(deathCitations, events), sourceXrefs, citationAPIDs)
		indi.Events = append(indi.Events, deathEvent)
	}

	// Additional life events (burial, baptism, emigration, etc.)
	for _, event := range events {
		fieldOwnsUnlinked := (event.FactType == domain.FactPersonBirth && hasBirthField(p)) ||
			(event.FactType == domain.FactPersonDeath && hasDeathField(p))
		if gedcomEvent := toGedcomEvent(event, fieldOwnsUnlinked, sourceXrefs, citationAPIDs, readStore, ctx); gedcomEvent != nil {
			indi.Events = append(indi.Events, gedcomEvent)
		}
	}

	// Attributes (occupation, residence, education, etc.)
	for _, attr := range attributes {
		if gedcomAttr := toGedcomAttribute(attr, sourceXrefs, citationAPIDs, readStore, ctx); gedcomAttr != nil {
			indi.Attributes = append(indi.Attributes, gedcomAttr)
		}
	}

	// Notes - encoder handles CONT/CONC automatically for multiline text
	indi.NoteXRefs, indi.InlineNotes = splitNotes(p.Notes, noteJoin)

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

	assoc.NoteXRefs, assoc.InlineNotes = splitNotes(a.Notes, associationNoteJoin)

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

	pn.NoteXRefs, pn.InlineNotes = splitNotes(nm.Notes, noteJoin)

	return pn
}

// nameCitationsByName groups a person's person_name citations by the NAME
// they are written under: names, sorted primary first, in that order, and n
// NAMEs in all. A citation goes under the name its fact_id names; one with no
// fact_id (cited before #1165) or naming no current name goes under the first,
// primary NAME, so it is still exported.
func nameCitationsByName(citations []repository.CitationReadModel, names []repository.PersonNameReadModel, n int) [][]repository.CitationReadModel {
	groups := make([][]repository.CitationReadModel, n)
	if n == 0 {
		return groups
	}
	for _, c := range citations {
		i := 0
		if c.FactID != nil {
			if j := slices.IndexFunc(names, func(nm repository.PersonNameReadModel) bool { return nm.ID == *c.FactID }); j >= 0 {
				i = j
			}
		}
		groups[i] = append(groups[i], c)
	}
	return groups
}

// individualCitations counts the source citations written under indi: the
// count is taken from what is built, so a citation dropped on the way (its
// source not exported, or a name citation with no NAME to go under) is not
// counted, and one on a life event or attribute is (#1354).
func individualCitations(indi *gedcom.Individual) int {
	n := len(indi.SourceCitations)
	for _, nm := range indi.Names {
		n += len(nm.SourceCitations)
	}
	for _, e := range indi.Events {
		n += len(e.SourceCitations)
	}
	for _, a := range indi.Attributes {
		n += len(a.SourceCitations)
	}
	for _, a := range indi.Associations {
		n += len(a.SourceCitations)
	}
	return n
}

// familyCitations counts the source citations written under fam, as
// individualCitations does for an individual.
func familyCitations(fam *gedcom.Family) int {
	n := len(fam.SourceCitations)
	for _, e := range fam.Events {
		n += len(e.SourceCitations)
	}
	for _, a := range fam.Attributes {
		n += len(a.SourceCitations)
	}
	return n
}

// toGedcomSourceCitations converts a slice of CitationReadModel to gedcom.SourceCitation slice.
// citationAPIDs supplies each citation's Ancestry APID; it may be nil.
func toGedcomSourceCitations(citations []repository.CitationReadModel, sourceXrefs map[uuid.UUID]string, citationAPIDs map[uuid.UUID]*gedcom.AncestryAPID) []*gedcom.SourceCitation {
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

		// QUAY (quality) - only when a rating maps; a nil Quality writes no
		// QUAY line, whereas &0 would assert "unreliable evidence".
		if q, ok := mapGPSToGedcomQuality(cit.SourceQuality, cit.InformantType, cit.EvidenceType); ok {
			citation.Quality = &q
		}

		// DATA with TEXT (quoted text)
		if cit.QuotedText != "" {
			citation.Data = &gedcom.SourceCitationData{
				Text: []*gedcom.SourceText{{Value: cit.QuotedText}},
			}
		}

		citation.AncestryAPID = citationAPIDs[cit.ID]

		result = append(result, citation)
	}

	return result
}

// hasBirthField, hasDeathField and hasMarriageField report whether the
// person's birth or death, or the family's marriage, held on the record itself
// rather than as a life event, is exported.
func hasBirthField(p repository.PersonReadModel) bool {
	return p.BirthDateRaw != "" || p.BirthPlace != "" || p.BirthNotes != "" || p.BirthTypeDetail != ""
}

func hasDeathField(p repository.PersonReadModel) bool {
	return p.DeathDateRaw != "" || p.DeathPlace != "" || p.DeathNotes != "" || p.DeathTypeDetail != ""
}

func hasMarriageField(f repository.FamilyReadModel) bool {
	return f.RelationshipType == domain.RelationMarriage || f.MarriageDateRaw != "" || f.MarriagePlace != "" || f.MarriageNotes != "" || f.MarriageTypeDetail != ""
}

// fieldCitations returns the citations of a birth, death or marriage held on
// the person or family record: those of its fact type not linked to one of
// the owner's life events, such as a NO BIRT (#1300).
func fieldCitations(citations []repository.CitationReadModel, events []repository.EventReadModel) []repository.CitationReadModel {
	var result []repository.CitationReadModel
	for _, c := range citations {
		if c.FactID != nil && slices.ContainsFunc(events, func(e repository.EventReadModel) bool { return e.ID == *c.FactID }) {
			continue
		}
		result = append(result, c)
	}
	return result
}

// toGedcomEvent converts an EventReadModel to a gedcom.Event.
// fieldOwnsUnlinked is set when the owner also exports a birth, death or
// marriage of the event's fact type from its own record; citations with no
// fact_id (imported before #1294) are then written under that one only (#1300).
func toGedcomEvent(event repository.EventReadModel, fieldOwnsUnlinked bool, sourceXrefs map[uuid.UUID]string, citationAPIDs map[uuid.UUID]*gedcom.AncestryAPID, readStore repository.ReadModelStore, ctx context.Context) *gedcom.Event {
	tagName := mapFactTypeToGedcomTag(event.FactType)
	if tagName == "" {
		return nil // Skip unknown event types
	}

	ge := &gedcom.Event{
		Type: gedcom.EventType(tagName),
	}

	// Set negative assertion flag for GEDCOM 7.0 NO tags
	ge.IsNegative = event.IsNegated

	if event.DateRaw != "" {
		ge.Date = event.DateRaw
	}

	if event.Place != "" {
		// SetPlaceName allocates PlaceDetail, so the name is kept whether or not
		// coordinates are present.
		ge.SetPlaceName(event.Place)
		// Add coordinates if present
		if event.PlaceLat != nil && event.PlaceLong != nil && *event.PlaceLat != "" && *event.PlaceLong != "" {
			ge.PlaceDetail.Coordinates = &gedcom.Coordinates{
				Latitude:  *event.PlaceLat,
				Longitude: *event.PlaceLong,
			}
		}
	}

	// Add structured address if available
	if event.Address != nil && !event.Address.IsEmpty() {
		ge.Address = convertDomainAddressToGedcom(event.Address)
	}

	// Only a generic EVEN carries a descriptor on its own line; a named event
	// tag's payload is the [Y|<NULL>] flag (#1276), so GEDCOM has no slot for
	// a named event's description. It is appended to the event's notes and
	// written as (the end of) its trailing inline NOTE (#1283). Re-import
	// reads that NOTE into notes, not description, and the next export writes
	// the same NOTE, so the text moves field once and is never duplicated.
	notes := event.Notes
	if ge.Type == "EVEN" {
		ge.Description = event.Description
	} else {
		notes = joinNotes(notes, event.Description)
	}
	ge.EventTypeDetail = event.TypeDetail

	if event.Cause != "" {
		ge.Cause = event.Cause
	}

	if event.Age != "" {
		ge.Age = event.Age
	}

	ge.NoteXRefs, ge.InlineNotes = splitNotes(notes, noteJoin)

	// Fetch and add this event's citations: not those of another event of
	// the same fact type (#1294)
	if readStore != nil {
		citations, _ := readStore.GetCitationsForFactInstance(ctx, domain.MainBranchID, event.FactType, event.OwnerID, event.ID)
		if fieldOwnsUnlinked {
			citations = slices.DeleteFunc(citations, func(c repository.CitationReadModel) bool { return c.FactID == nil })
		}
		ge.SourceCitations = toGedcomSourceCitations(citations, sourceXrefs, citationAPIDs)
	}

	return ge
}

// convertDomainAddressToGedcom converts a domain.Address to a gedcom.Address.
func convertDomainAddressToGedcom(addr *domain.Address) *gedcom.Address {
	if addr == nil {
		return nil
	}
	// The address text goes in Full, the ADDR payload: the encoder writes the
	// ADDR value from Full alone, and GEDCOM 7.0 has no ADR1-ADR3.
	var lines []string
	for _, line := range []string{addr.Line1, addr.Line2, addr.Line3} {
		if line != "" {
			lines = append(lines, line)
		}
	}
	return &gedcom.Address{
		Full:       strings.Join(lines, "\n"),
		City:       addr.City,
		State:      addr.State,
		PostalCode: addr.PostalCode,
		Country:    addr.Country,
	}
}

// toGedcomAttribute converts an AttributeReadModel to a gedcom.Attribute.
func toGedcomAttribute(attr repository.AttributeReadModel, sourceXrefs map[uuid.UUID]string, citationAPIDs map[uuid.UUID]*gedcom.AncestryAPID, readStore repository.ReadModelStore, ctx context.Context) *gedcom.Attribute {
	tagName := mapFactTypeToGedcomTag(attr.FactType)
	if tagName == "" {
		return nil // Skip unknown attribute types
	}

	ga := &gedcom.Attribute{
		Type:       gedcom.AttributeType(tagName),
		Value:      attr.Value,
		TypeDetail: attr.TypeDetail,
	}

	if attr.DateRaw != "" {
		ga.Date = attr.DateRaw
	}

	if attr.Place != "" {
		ga.SetPlaceName(attr.Place)
		if attr.PlaceLat != nil && attr.PlaceLong != nil && *attr.PlaceLat != "" && *attr.PlaceLong != "" {
			ga.PlaceDetail.Coordinates = &gedcom.Coordinates{
				Latitude:  *attr.PlaceLat,
				Longitude: *attr.PlaceLong,
			}
		}
	}

	ga.NoteXRefs, ga.InlineNotes = splitNotes(attr.Notes, noteJoin)

	// The attribute's own citations, as a life event's: not those of another
	// attribute of the same fact type (#1294).
	if readStore != nil {
		citations, _ := readStore.GetCitationsForFactInstance(ctx, domain.MainBranchID, attr.FactType, attr.PersonID, attr.ID)
		ga.SourceCitations = toGedcomSourceCitations(citations, sourceXrefs, citationAPIDs)
	}

	return ga
}

// toGedcomFamily converts a repository FamilyReadModel to a gedcom.Family entity.
// The encoder will automatically convert this to GEDCOM tags, handling CONT/CONC.
func toGedcomFamily(f repository.FamilyReadModel, personXrefs, sourceXrefs map[uuid.UUID]string, children []repository.FamilyChildReadModel, marriageCitations []repository.CitationReadModel, events []repository.EventReadModel, ldsOrdinances []repository.LDSOrdinanceReadModel, citationAPIDs map[uuid.UUID]*gedcom.AncestryAPID, readStore repository.ReadModelStore, ctx context.Context) *gedcom.Family {
	fam := &gedcom.Family{}
	fam.NoteXRefs, fam.InlineNotes = splitNotes(f.Notes, noteJoin)

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
	if hasMarriageField(f) {
		marriageEvent := &gedcom.Event{Type: gedcom.EventMarriage, EventTypeDetail: f.MarriageTypeDetail}
		marriageEvent.NoteXRefs, marriageEvent.InlineNotes = splitNotes(f.MarriageNotes, noteJoin)
		if f.MarriageDateRaw != "" {
			marriageEvent.Date = f.MarriageDateRaw
		}
		if f.MarriagePlace != "" {
			marriageEvent.SetPlaceName(f.MarriagePlace)
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
		marriageEvent.SourceCitations = toGedcomSourceCitations(fieldCitations(marriageCitations, events), sourceXrefs, citationAPIDs)
		fam.Events = append(fam.Events, marriageEvent)
	}

	// Additional family events (divorce, annulment, engagement, etc.)
	for _, event := range events {
		fieldOwnsUnlinked := event.FactType == domain.FactFamilyMarriage && hasMarriageField(f)
		if gedcomEvent := toGedcomEvent(event, fieldOwnsUnlinked, sourceXrefs, citationAPIDs, readStore, ctx); gedcomEvent != nil {
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
// ok is false when no rating maps, so the caller writes no QUAY line.
// This is the reverse of mapGedcomQualityToGPS in importer.go
// TODO: sourceQuality is reserved for future use when GEDCOM quality mapping is enhanced
func mapGPSToGedcomQuality(_ domain.SourceQuality, informantType domain.InformantType, evidenceType domain.EvidenceType) (quay int, ok bool) {
	// Priority: evidenceType > informantType > sourceQuality
	// QUAY 3 = Direct and primary evidence
	if evidenceType == domain.EvidenceDirect && informantType == domain.InformantPrimary {
		return 3, true
	}
	// QUAY 2 = Secondary evidence (informant type)
	if informantType == domain.InformantSecondary {
		return 2, true
	}
	// QUAY 1 = Questionable/Indirect evidence
	if evidenceType == domain.EvidenceIndirect {
		return 1, true
	}
	// Negative evidence writes no QUAY (not QUAY 0): pre-v3 imports stored
	// every QUAY-less citation as EvidenceNegative, and those spurious
	// negatives are indistinguishable from a genuine QUAY 0.
	// Default: no quality specified
	return 0, false
}

// xrefAllocator gives every exported record a distinct xref. A record keeps
// its stored GedcomXref when that is a valid pointer no other record has
// taken; otherwise it gets a generated one that skips every stored xref of any
// record type and every xref already given out (#1177).
//
// A stored xref can be invalid: 5.5.1 imports keep `@N 1@` verbatim and an
// API-supplied gedcom_xref is not validated. A space would fail the strict 7.0
// re-parse in encodeDowngraded, and a line break would fail the encoder
// (encoder.ErrLineBreak, gedcom-go#466) or forge a GEDCOM line.
type xrefAllocator struct {
	reserved map[string]bool // valid stored xrefs of every record type
	taken    map[string]bool // xrefs already given to a record
}

// newXrefAllocator reserves every valid xref in stored.
func newXrefAllocator(stored []string) *xrefAllocator {
	a := &xrefAllocator{reserved: map[string]bool{}, taken: map[string]bool{}}
	for _, xref := range stored {
		if gedcom.IsPointerXRef(xref) {
			a.reserved[xref] = true
		}
	}
	return a
}

// assign returns the xref for a record with the given stored xref: stored
// itself when it is valid and still free, otherwise the first @<prefix><n>@,
// counting up from n, that is neither reserved nor taken.
func (a *xrefAllocator) assign(stored, prefix string, n int) string {
	if gedcom.IsPointerXRef(stored) && !a.taken[stored] {
		a.taken[stored] = true
		return stored
	}
	xref := fmt.Sprintf("@%s%d@", prefix, n)
	for a.reserved[xref] || a.taken[xref] {
		n++
		xref = fmt.Sprintf("@%s%d@", prefix, n)
	}
	a.taken[xref] = true
	return xref
}

// pointerTargets resolves pointers to one record type (NOTE or REPO) that the
// read model keeps as the target's raw stored xref, to the xref that target
// is exported under. A pointer resolves against records of that type only, so
// a NOTE pointer never lands on a SOUR that shares its stored xref (#1228).
// When several records of the type share a stored xref the pointer is
// ambiguous: it resolves to the first and the export records a warning.
type pointerTargets struct {
	recordType string
	exported   map[string]string // stored xref -> export xref of the first record holding it
	holders    map[string]int    // stored xref -> number of records holding it
	warned     map[string]bool
	warnings   *[]string
}

func newPointerTargets(recordType string, warnings *[]string) *pointerTargets {
	return &pointerTargets{
		recordType: recordType,
		exported:   map[string]string{},
		holders:    map[string]int{},
		warned:     map[string]bool{},
		warnings:   warnings,
	}
}

// add records that the record stored as stored is exported as exported.
func (t *pointerTargets) add(stored, exported string) {
	if stored == "" {
		return
	}
	if t.holders[stored] == 0 {
		t.exported[stored] = exported
	}
	t.holders[stored]++
}

// remap rewrites v, or each sep-separated part of v, that is the stored xref
// of a record of this type to that record's export xref. One Notes value can
// hold several NOTE pointers: the importer joins a record's with noteJoin and
// an association's with "\n".
func (t *pointerTargets) remap(v, sep string) string {
	return renameXrefs(v, sep, t.resolve)
}

// resolve returns the export xref of the record stored as stored.
func (t *pointerTargets) resolve(stored string) (string, bool) {
	xref, ok := t.exported[stored]
	if ok && t.holders[stored] > 1 && !t.warned[stored] {
		t.warned[stored] = true
		*t.warnings = append(*t.warnings, fmt.Sprintf(
			// %q: a stored xref is not validated and can hold a line break.
			"%d %s records share the xref %q; pointers to it were linked to the first of them (exported as %s)",
			t.holders[stored], t.recordType, stored, xref))
	}
	return xref, ok
}

// remapNotes is remap for a Notes value of the record exported as owner (for
// example "INDI @I1@"), and drops each part that is a pointer to no exported
// record with a warning naming owner and the pointer. Such a pointer comes
// from a broken NOTE reference kept by import or a note deleted since: left
// in, 5.5.1 writes a dangling NOTE pointer and 7.0, whose NOTE takes text,
// writes the text "@N9@" (#1353).
func (t *pointerTargets) remapNotes(v, sep, owner string) string {
	if xref, ok := t.resolve(v); ok {
		return xref
	}
	parts := strings.Split(v, sep)
	kept := parts[:0]
	for _, part := range parts {
		if xref, ok := t.resolve(part); ok {
			kept = append(kept, xref)
			continue
		}
		if gedcom.IsPointerXRef(part) {
			*t.warnings = append(*t.warnings, fmt.Sprintf(
				"%s points to %s %s, which is not in the export; the pointer was dropped",
				owner, t.recordType, part))
			continue
		}
		kept = append(kept, part)
	}
	return strings.Join(kept, sep)
}

// remapEvents remaps the NOTE pointers in each event's Notes, in place.
func (t *pointerTargets) remapEvents(events []repository.EventReadModel, owner string) {
	for i := range events {
		events[i].Notes = t.remapNotes(events[i].Notes, noteJoin, owner)
	}
}

// renameXrefs replaces v, or else each sep-separated part of v, for which
// rename reports a new value. v is tried whole first because an invalid
// stored xref can contain sep.
func renameXrefs(v, sep string, rename func(string) (string, bool)) string {
	if xref, ok := rename(v); ok {
		return xref
	}
	parts := strings.Split(v, sep)
	for i, part := range parts {
		if xref, ok := rename(part); ok {
			parts[i] = xref
		}
	}
	return strings.Join(parts, sep)
}

// splitNotes splits a stored Notes value back into the NOTE pointers and the
// inline note text that GEDCOM models as separate lists (#1201, #1178). The
// importer joins an entity's NOTE values with sep and keeps pointers
// verbatim, so each sep-separated part that is a valid pointer is a NOTE
// pointer. Each run of other parts between pointers, rejoined with sep, is
// one inline note, so a value with no pointer exports unchanged. The encoder
// writes every pointer ahead of the inline notes, so text interleaved with
// pointers keeps its own NOTE but not its place.
func splitNotes(notes, sep string) (xrefs, inline []string) {
	if notes == "" {
		return nil, nil
	}
	var text []string
	flush := func() {
		if len(text) > 0 {
			inline = append(inline, strings.Join(text, sep))
			text = nil
		}
	}
	for _, part := range strings.Split(notes, sep) {
		if gedcom.IsPointerXRef(part) {
			flush()
			xrefs = append(xrefs, part)
		} else {
			text = append(text, part)
		}
	}
	flush()
	return xrefs, inline
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
	case domain.FactPersonAdoption:
		return "ADOP"
	case domain.FactPersonBarMitzvah:
		return "BARM"
	case domain.FactPersonBasMitzvah:
		return "BASM"
	case domain.FactPersonBlessing:
		return "BLES"
	case domain.FactPersonAdultChristening:
		return "CHRA"
	case domain.FactPersonConfirmation:
		return "CONF"
	case domain.FactPersonFirstCommunion:
		return "FCOM"
	case domain.FactPersonGraduation:
		return "GRAD"
	case domain.FactPersonRetirement:
		return "RETI"
	case domain.FactPersonOrdination:
		return "ORDN"
	case domain.FactPersonProbate:
		return "PROB"
	case domain.FactPersonWill:
		return "WILL"
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
	case domain.FactPersonPhysicalDescription:
		return "DSCR"
	case domain.FactPersonNationality:
		return "NATI"
	case domain.FactPersonCaste:
		return "CAST"
	case domain.FactPersonProperty:
		return "PROP"
	case domain.FactPersonSocialSecurityNumber:
		return "SSN"
	case domain.FactPersonIdentificationNumber:
		return "IDNO"
	case domain.FactPersonChildrenCount:
		return "NCHI"
	case domain.FactPersonMarriageCount:
		return "NMR"
	case domain.FactPersonGenericFact:
		return "FACT"
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
	case domain.FactFamilyDivorceFiling:
		return "DIVF"
	case domain.FactFamilyCensus:
		return "CENS"
	case domain.FactFamilyResidence:
		return "RESI"
	case domain.FactFamilyGenericEvent:
		return "EVEN"
	default:
		return ""
	}
}

// toGedcomNoteRecord converts a repository NoteReadModel to a top-level GEDCOM
// note record. A note carrying GEDCOM 7.0 metadata (MIME media type, language
// tag, or translations) becomes an SNOTE record; a plain note becomes a NOTE
// record for GEDCOM 5.5.1 compatibility.
//
// The note text lives on the level-0 record line, with any additional lines
// emitted as CONT subordinates. SNOTE tags are built directly (rather than via
// the gedcom.SharedNote entity) so that the CONT continuation precedes the
// MIME/LANG/TRAN substructures, which the encoder's entity conversion cannot
// guarantee.
func toGedcomNoteRecord(xref string, n repository.NoteReadModel) *gedcom.Record {
	if n.MIME == "" && n.Language == "" && len(n.Translations) == 0 {
		// Value stays empty so the encoder derives the level-0 line and the
		// CONT/CONC continuation from Note.Text (enforcing MaxLineLength).
		return &gedcom.Record{
			XRef:   xref,
			Type:   gedcom.RecordTypeNote,
			Entity: &gedcom.Note{Text: n.Text},
		}
	}

	firstLine, contLines := splitNoteText(n.Text)
	var tags []*gedcom.Tag
	for _, line := range contLines {
		tags = append(tags, &gedcom.Tag{Level: 1, Tag: "CONT", Value: line})
	}
	if n.MIME != "" {
		tags = append(tags, &gedcom.Tag{Level: 1, Tag: "MIME", Value: n.MIME})
	}
	if n.Language != "" {
		tags = append(tags, &gedcom.Tag{Level: 1, Tag: "LANG", Value: n.Language})
	}
	for _, tran := range n.Translations {
		tranFirst, tranCont := splitNoteText(tran.Text)
		tags = append(tags, &gedcom.Tag{Level: 1, Tag: "TRAN", Value: tranFirst})
		for _, line := range tranCont {
			tags = append(tags, &gedcom.Tag{Level: 2, Tag: "CONT", Value: line})
		}
		if tran.MIME != "" {
			tags = append(tags, &gedcom.Tag{Level: 2, Tag: "MIME", Value: tran.MIME})
		}
		if tran.Language != "" {
			tags = append(tags, &gedcom.Tag{Level: 2, Tag: "LANG", Value: tran.Language})
		}
	}

	return &gedcom.Record{
		XRef:  xref,
		Type:  gedcom.RecordTypeSharedNote,
		Value: firstLine,
		Tags:  tags,
	}
}

// splitNoteText splits note text into the first line (written on the record's
// level-0 line) and the remaining lines (emitted as CONT continuation tags).
func splitNoteText(text string) (first string, rest []string) {
	lines := strings.Split(text, "\n")
	return lines[0], lines[1:]
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

// toGedcomRepository converts a repository RepositoryReadModel to a gedcom.Repository entity.
// The encoder will automatically convert this to GEDCOM tags (NAME, ADDR, NOTE).
func toGedcomRepository(r repository.RepositoryReadModel) *gedcom.Repository {
	repo := &gedcom.Repository{
		Name: r.Name,
	}

	// Convert address if present. PHON/EMAIL/WWW are siblings of ADDR on a
	// repository, so they are written from the Repository's own fields, and a
	// contact-only repository gets no empty ADDR line.
	if r.Address != nil && !r.Address.IsEmpty() {
		if addr := convertDomainAddressToGedcom(r.Address); *addr != (gedcom.Address{}) {
			repo.Address = addr
		}
		if r.Address.Phone != "" {
			repo.Phone = []string{r.Address.Phone}
		}
		if r.Address.Email != "" {
			repo.Email = []string{r.Address.Email}
		}
		if r.Address.Website != "" {
			repo.Website = []string{r.Address.Website}
		}
	}

	// Notes - encoder handles CONT/CONC automatically for multiline text.
	repo.NoteXRefs, repo.InlineNotes = splitNotes(r.Notes, noteJoin)

	return repo
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
