<objective>
Implement GEDCOM import/export support for sources and citations, enabling round-trip compatibility with other genealogy software.

This is Phase 5 (final) of implementing GitHub issue #31. GEDCOM integration ensures users can import existing research with sources preserved and export their work to share with others.
</objective>

<context>
Project: Self-hosted genealogy software using github.com/cacack/gedcom-go library.
Issue: #31 - Sources and citations foundation (GPS)

Depends on: Phase 1-4 (domain, repository, command/query, API) must be complete.

Review existing GEDCOM patterns:
@internal/gedcom/importer.go - Import logic, two-pass approach (persons then families)
@internal/gedcom/exporter.go - Export logic, writes GEDCOM format
@internal/gedcom/importer_test.go - Test patterns

GEDCOM source structure:
```
0 @S1@ SOUR              <- Source record
1 TITL Census 1850
1 AUTH US Census Bureau
1 PUBL Government Printing Office
1 REPO @R1@              <- Repository reference (optional)

0 @I1@ INDI              <- Individual with citation
1 BIRT
2 DATE 1 JAN 1850
2 SOUR @S1@              <- Citation to source
3 PAGE 123               <- Page number
3 QUAY 3                 <- Quality (0-3)
3 TEXT Quoted text       <- Direct quote
```
</context>

<requirements>
1. Enhance importer in `internal/gedcom/importer.go`:

   Add SourceData struct:
   - ID uuid.UUID
   - GedcomXref string (e.g., "@S1@")
   - SourceType string
   - Title string
   - Author string
   - Publisher string
   - PublishDate string
   - RepositoryName string
   - Notes string

   Add CitationData struct:
   - ID uuid.UUID
   - SourceXref string (references SourceData)
   - FactType string
   - FactOwnerID uuid.UUID
   - Page string
   - Quality string (maps to QUAY 0-3)
   - QuotedText string
   - GedcomXref string

   Modify Import() to:
   - Add third pass: parse all 0-level SOUR records into SourceData
   - Build sourceXrefToID map (like personXrefToID)
   - When parsing INDI/FAM events, capture SOUR references as CitationData
   - Return sources and citations in ImportResult

2. Enhance exporter in `internal/gedcom/exporter.go`:

   Modify Export() to:
   - Write all sources as 0-level SOUR records with @S{n}@ XREFs
   - Build sourceIDToXref map
   - When writing INDI/FAM event tags (BIRT, DEAT, MARR), include SOUR citations
   - Write citation details: PAGE, QUAY, TEXT

3. Map GEDCOM QUAY (quality) values to GPS terms:
   - QUAY 0 = Unreliable -> confidence: "negative"
   - QUAY 1 = Questionable -> confidence: "indirect"
   - QUAY 2 = Secondary evidence -> informant_type: "secondary"
   - QUAY 3 = Direct evidence -> confidence: "direct", informant_type: "primary"

4. Update tests to verify source/citation round-trip:
   - Import GEDCOM with SOUR records
   - Verify sources created with correct data
   - Verify citations linked to correct persons/events
   - Export and verify SOUR tags present
</requirements>

<implementation>
Follow existing patterns:
- Use gedcom-go library's Document, Individual, Family, Source types
- The gedcom-go library should already parse SOUR records - check its API
- If gedcom-go doesn't fully support sources, parse manually from raw GEDCOM
- Maintain XREFs for round-trip: store GedcomXref, use it when exporting
- For new sources (created in app), generate sequential XREFs: @S1@, @S2@, etc.

GEDCOM 5.5 source tags to handle:
- TITL (title) - required
- AUTH (author)
- PUBL (publisher)
- REPO (repository) - may be reference or inline
- NOTE (notes)
- TEXT (source text)

Citation tags (under SOUR reference):
- PAGE (page/location)
- QUAY (quality 0-3)
- TEXT (quoted text)
- NOTE (citation notes)
</implementation>

<output>
Modify files:
- `./internal/gedcom/importer.go` - Add source/citation parsing
- `./internal/gedcom/exporter.go` - Add source/citation writing
- `./internal/gedcom/importer_test.go` - Add source import tests
- `./internal/gedcom/exporter_test.go` - Add source export tests
- `./internal/gedcom/integration_test.go` - Add round-trip tests
</output>

<verification>
Before completing:
1. Run `go build ./...` - must compile without errors
2. Run `go test ./internal/gedcom/...` - all tests must pass
3. Create test GEDCOM with sources, import, verify sources created
4. Create test GEDCOM with citations on BIRT event, verify citations linked to person_birth
5. Import then export, verify SOUR records preserved
6. Verify GedcomXref values survive round-trip
</verification>

<success_criteria>
- GEDCOM files with sources import correctly
- Sources preserve title, author, publisher, notes
- Citations link to correct person/family facts
- Export produces valid GEDCOM with SOUR records
- Round-trip import->export->import preserves all source data
- Existing GEDCOM tests continue to pass
</success_criteria>
