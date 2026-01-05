<objective>
Implement Ancestry (#38) and FamilySearch (#39) GEDCOM import support by wiring vendor extensions from the gedcom-go library through the my-family import pipeline.

The gedcom-go library already parses vendor-specific tags into structured fields. This task connects that parsed data to the my-family domain structures so vendor identifiers are preserved for future sync capabilities.
</objective>

<context>
The gedcom-go library (linked via replace directive) now provides:
- `doc.Vendor` - Detected vendor (ancestry, familysearch, etc.)
- `indi.FamilySearchID` - FamilySearch Family Tree ID from `_FSFTID` tag
- `cite.AncestryAPID` - Ancestry Permanent Identifier with Database, Record, URL()

Read CLAUDE.md for project conventions.

Key files to examine:
@internal/gedcom/importer.go - Main import pipeline with data structures
@internal/gedcom/importer_test.go - Existing test patterns
</context>

<requirements>
1. **Add vendor fields to ImportResult**:
   - `Vendor string` - The detected vendor (from doc.Vendor)

2. **Add vendor fields to PersonData**:
   - `FamilySearchID string` - FamilySearch Family Tree ID

3. **Add vendor fields to CitationData**:
   - Create `AncestryAPIDData` struct with Database, Record, RawValue string fields
   - Add `AncestryAPID *AncestryAPIDData` to CitationData

4. **Extract vendor data in Import()**:
   - Set `result.Vendor = string(doc.Vendor)` after decode

5. **Extract FamilySearchID in parseIndividual()**:
   - Set `person.FamilySearchID = indi.FamilySearchID`

6. **Extract AncestryAPID in extractCitationsFromIndividual/Family()**:
   - When source citation has AncestryAPID, copy to CitationData

7. **Add sample GEDCOM test files**:
   - Copy or create `testdata/gedcom-5.5/ancestry-sample.ged` with _APID tags
   - Copy or create `testdata/gedcom-5.5/familysearch-sample.ged` with _FSFTID tags
   - Check gedcom-go testdata for existing samples to reuse

8. **Add unit tests**:
   - Test vendor detection flows to ImportResult.Vendor
   - Test FamilySearchID extraction to PersonData
   - Test AncestryAPID extraction to CitationData
</requirements>

<implementation>
Follow existing patterns in importer.go:
- Data structs defined at top of file
- Extraction happens in parse* functions
- Tests use table-driven format with comprehensive.ged or custom test files

For AncestryAPIDData struct, keep it simple:
```go
type AncestryAPIDData struct {
    Database string
    Record   string
    RawValue string
}
```

When extracting from gedcom-go's AncestryAPID:
```go
if cite.AncestryAPID != nil {
    citationData.AncestryAPID = &AncestryAPIDData{
        Database: cite.AncestryAPID.Database,
        Record:   cite.AncestryAPID.Record,
        RawValue: cite.AncestryAPID.Raw,
    }
}
```
</implementation>

<output>
Files to modify:
- `./internal/gedcom/importer.go` - Add vendor fields and extraction logic
- `./internal/gedcom/importer_test.go` - Add vendor extraction tests

Files to create:
- `./testdata/gedcom-5.5/ancestry-sample.ged` - Test file with _APID tags
- `./testdata/gedcom-5.5/familysearch-sample.ged` - Test file with _FSFTID tags
</output>

<verification>
Before completing:
1. Run `go build ./...` - Must compile without errors
2. Run `go test ./internal/gedcom/...` - All tests must pass
3. Run `make check-coverage` - Must meet 85% threshold
4. Verify vendor field populated when importing ancestry-sample.ged
5. Verify FamilySearchID populated when importing familysearch-sample.ged
</verification>

<success_criteria>
- [ ] ImportResult.Vendor contains "ancestry" for Ancestry GEDCOM files
- [ ] ImportResult.Vendor contains "familysearch" for FamilySearch GEDCOM files
- [ ] PersonData.FamilySearchID extracted from _FSFTID tag
- [ ] CitationData.AncestryAPID extracted with Database/Record/RawValue
- [ ] All existing tests continue to pass
- [ ] New tests cover vendor extraction paths
- [ ] Coverage threshold met (85%)
</success_criteria>
