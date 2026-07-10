package gedcom

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"sort"
	"strings"

	"github.com/cacack/gedcom-go/v2/converter"
	"github.com/cacack/gedcom-go/v2/decoder"
	"github.com/cacack/gedcom-go/v2/encoder"
	"github.com/cacack/gedcom-go/v2/gedcom"
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
	DataLoss              []DataLossDetail
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

	// Calculate total items for progress tracking
	// Weight persons more heavily since they have the most processing
	totalItems := len(sources) + len(persons)*2 + len(families) + len(notes) + len(submitters) + len(repositories) + 1 // +1 for encoding
	processedItems := 0

	// Create XREF mappings (UUID -> @Xn@)
	personXrefs := make(map[uuid.UUID]string)
	familyXrefs := make(map[uuid.UUID]string)
	sourceXrefs := make(map[uuid.UUID]string)
	repositoryXrefs := make(map[uuid.UUID]string)

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

	// Sort repositories by ID for stable output and assign xrefs.
	sort.Slice(repositories, func(i, j int) bool {
		return repositories[i].ID.String() < repositories[j].ID.String()
	})
	// repoNameToXref lets sources that reference a repository by name be linked
	// to the standalone REPO record via a SOUR.REPO cross-reference. repoIDToXref
	// is the authoritative ID-based map, preferred over name matching (issue #525).
	repoNameToXref := make(map[string]string)
	repoIDToXref := make(map[uuid.UUID]string)
	for i, r := range repositories {
		// Use GedcomXref if available (for round-trip), otherwise generate.
		if r.GedcomXref != "" {
			repositoryXrefs[r.ID] = r.GedcomXref
		} else {
			repositoryXrefs[r.ID] = fmt.Sprintf("@R%d@", i+1)
		}
		repoIDToXref[r.ID] = repositoryXrefs[r.ID]
		if r.Name != "" {
			repoNameToXref[r.Name] = repositoryXrefs[r.ID]
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
		src := toGedcomSource(s, repoIDToXref, repoNameToXref, exp.readStore, ctx)
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

		// Notes carrying GEDCOM 7.0 metadata (MIME, language, or translations)
		// round-trip as SNOTE records; plain notes stay as NOTE records for
		// GEDCOM 5.5.1 compatibility.
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

	// Add repository records (REPO). Standalone records that sources reference
	// via SOUR.REPO cross-references.
	for i, r := range repositories {
		xref := repositoryXrefs[r.ID]
		repo := toGedcomRepository(r, exp.readStore, ctx)
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
	// instead of silently lossy 5.5.1 (issue #539).
	targetVersion := opts.TargetVersion
	if targetVersion == "" {
		targetVersion = gedcom.Version55
		if doc.RequiresGEDCOM7() {
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
		// Encode using gedcom-go encoder with LF line endings. TargetVersion
		// selects the emitted version without mutating doc.Header.Version.
		// PreserveUnknownTags mirrors the downgrade path (and the library's own
		// nil-opts default) so vendor "_"-prefixed tags are never silently
		// stripped by the bare-literal zero value.
		encOpts := &encoder.EncodeOptions{LineEnding: "\n", TargetVersion: targetVersion, PreserveUnknownTags: true}
		if err := encoder.EncodeWithOptions(cw, doc, encOpts); err != nil {
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

// encodeDowngraded emits doc to w at targetVersion (an older version than the
// document's 7.0 content requires) and returns the conversion report describing
// what was transformed or dropped. Because the gedcom-go converter works on
// parsed tags rather than the exporter's typed entities, the document is first
// encoded at 7.0 (lossless) and re-parsed, then converted to targetVersion, then
// encoded to w.
func (exp *Exporter) encodeDowngraded(w io.Writer, doc *gedcom.Document, targetVersion gedcom.Version) (*gedcom.ConversionReport, error) {
	var full bytes.Buffer
	if err := encoder.EncodeWithOptions(&full, doc, &encoder.EncodeOptions{LineEnding: "\n", TargetVersion: gedcom.Version70, PreserveUnknownTags: true}); err != nil {
		return nil, fmt.Errorf("failed to encode document for conversion: %w", err)
	}
	parsed, err := decoder.Decode(bytes.NewReader(full.Bytes()))
	if err != nil {
		return nil, fmt.Errorf("failed to re-parse document for conversion: %w", err)
	}
	converted, report, err := converter.ConvertWithOptions(parsed, targetVersion, converter.DefaultOptions())
	if err != nil {
		return nil, fmt.Errorf("failed to convert export to %s: %w", targetVersion, err)
	}
	// Preserve custom "_"-prefixed tags: the converter deliberately keeps vendor
	// extensions through a downgrade (and maps 7.0-only EXIDs to vendor tags like
	// _FSFTID), but EncodeOptions only defaults PreserveUnknownTags to true when
	// opts is nil — a bare literal would silently strip them. See issue #599.
	if err := encoder.EncodeWithOptions(w, converted, &encoder.EncodeOptions{LineEnding: "\n", TargetVersion: targetVersion, PreserveUnknownTags: true}); err != nil {
		return nil, fmt.Errorf("failed to write GEDCOM: %w", err)
	}
	return report, nil
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
func toGedcomSource(s repository.SourceReadModel, repoIDToXref map[uuid.UUID]string, repoNameToXref map[string]string, readStore repository.ReadModelStore, ctx context.Context) *gedcom.Source {
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
	case strings.HasPrefix(s.RepositoryName, "@") && strings.HasSuffix(s.RepositoryName, "@"):
		// Already an XREF (e.g. an unresolved import reference): preserve it.
		repoLink.XRef = s.RepositoryName
	default:
		// Inline repository with NAME subordinate (name-only / unlinked source).
		repoLink.Inline = &gedcom.InlineRepository{Name: s.RepositoryName}
	}
	if s.CallNumber != "" {
		repoLink.CallNumbers = []string{s.CallNumber}
	}
	if repoLink.XRef != "" || repoLink.Inline != nil || len(repoLink.CallNumbers) > 0 {
		src.RepositoryLink = repoLink
	}

	// Notes - encoder handles CONT/CONC automatically for multiline text
	if s.Notes != "" {
		src.InlineNotes = []string{s.Notes}
	}

	// External identifiers (GEDCOM 7.0 EXID), re-emitted from the read model.
	if externalIDs, err := readStore.GetSourceExternalIDs(ctx, s.ID); err == nil {
		for _, ext := range externalIDs {
			src.ExternalIDs = append(src.ExternalIDs, &gedcom.ExternalID{
				Value: ext.Value,
				Type:  ext.Type,
			})
		}
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
		indi.InlineNotes = []string{p.Notes}
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

	// External identifiers (GEDCOM 7.0 EXID). The encoder emits each as an
	// `EXID <value>` / `TYPE <uri>` structure on the individual (gedcom-go
	// v2.2.1+), which also upgrades the export to GEDCOM 7.0 via RequiresGEDCOM7.
	if externalIDs, err := readStore.GetPersonExternalIDs(ctx, p.ID); err == nil {
		for _, ext := range externalIDs {
			indi.ExternalIDs = append(indi.ExternalIDs, &gedcom.ExternalID{
				Value: ext.Value,
				Type:  ext.Type,
			})
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

	// Set negative assertion flag for GEDCOM 7.0 NO tags
	ge.IsNegative = event.IsNegated

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

	// External identifiers (GEDCOM 7.0 EXID), re-emitted from the read model.
	if externalIDs, err := readStore.GetFamilyExternalIDs(ctx, f.ID); err == nil {
		for _, ext := range externalIDs {
			fam.ExternalIDs = append(fam.ExternalIDs, &gedcom.ExternalID{
				Value: ext.Value,
				Type:  ext.Type,
			})
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
	firstLine, contLines := splitNoteText(n.Text)

	if n.MIME == "" && n.Language == "" && len(n.Translations) == 0 {
		return &gedcom.Record{
			XRef:   xref,
			Type:   gedcom.RecordTypeNote,
			Value:  firstLine,
			Entity: &gedcom.Note{Text: firstLine, Continuation: contLines},
		}
	}

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
func toGedcomRepository(r repository.RepositoryReadModel, readStore repository.ReadModelStore, ctx context.Context) *gedcom.Repository {
	repo := &gedcom.Repository{
		Name: r.Name,
	}

	// Convert address if present (includes phone/email/website).
	if r.Address != nil && !r.Address.IsEmpty() {
		repo.Address = convertDomainAddressToGedcom(r.Address)
	}

	// Notes - encoder handles CONT/CONC automatically for multiline text.
	if r.Notes != "" {
		repo.InlineNotes = []string{r.Notes}
	}

	// External identifiers (GEDCOM 7.0 EXID), re-emitted from the read model.
	if externalIDs, err := readStore.GetRepositoryExternalIDs(ctx, r.ID); err == nil {
		for _, ext := range externalIDs {
			repo.ExternalIDs = append(repo.ExternalIDs, &gedcom.ExternalID{
				Value: ext.Value,
				Type:  ext.Type,
			})
		}
	}

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
