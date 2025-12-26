# Stage 9: GEDCOM Integration

<objective>
Extend the GEDCOM importer to parse OBJE (multimedia object) records and create Media entries linked to persons and families.
</objective>

<context>
Reference existing patterns in:
- @file:internal/gedcom/importer.go - Import method, parseIndividual, parseFamily patterns

Dependencies:
- Stages 1-8 complete (full media CRUD working)
- gedcom-go library: check if MediaObject type is available

GEDCOM OBJE records contain:
- File reference (FILE tag with path)
- Format (FORM tag - jpeg, gif, etc.)
- Title (TITL tag)
- Optional notes
</context>

<requirements>

## 1. Update ImportResult

Add media tracking to `internal/gedcom/importer.go`:

```go
type ImportResult struct {
    PersonsImported   int
    FamiliesImported  int
    SourcesImported   int
    CitationsImported int
    MediaImported     int  // NEW
    Warnings          []string
    Errors            []string

    PersonXrefToID map[string]uuid.UUID
    FamilyXrefToID map[string]uuid.UUID
    SourceXrefToID map[string]uuid.UUID
    MediaXrefToID  map[string]uuid.UUID  // NEW
}
```

## 2. Add MediaData Struct

```go
// MediaData contains parsed media data ready for creation.
type MediaData struct {
    ID          uuid.UUID
    GedcomXref  string
    EntityType  string    // "person" or "family"
    EntityID    uuid.UUID // Will be resolved after person/family import
    Title       string
    Filename    string
    MimeType    string
    Format      string    // GEDCOM format (jpeg, gif, etc.)
    FileRef     string    // File path from GEDCOM (for external files)
    Notes       string
}
```

## 3. Parse OBJE Records from Individuals

GEDCOM multimedia can be:
1. Embedded in individual records (inline OBJE)
2. Standalone records referenced by @XREF@

```go
// extractMediaFromIndividual extracts media references from an individual.
func extractMediaFromIndividual(indi *gedcom.Individual, personID uuid.UUID, result *ImportResult) []MediaData {
    var mediaItems []MediaData

    // Check for OBJE tags in individual's Tags
    for _, tag := range indi.Tags {
        if tag.Tag == "OBJE" {
            media := parseMediaTag(tag, "person", personID, result)
            if media != nil {
                mediaItems = append(mediaItems, *media)
            }
        }
    }

    // Also check for media references via XRef
    // In gedcom-go, check if indi.Media or similar exists
    // This depends on the library structure

    return mediaItems
}

// parseMediaTag parses an OBJE tag into MediaData.
func parseMediaTag(tag gedcom.Tag, entityType string, entityID uuid.UUID, result *ImportResult) *MediaData {
    media := &MediaData{
        ID:         uuid.New(),
        EntityType: entityType,
        EntityID:   entityID,
    }

    // Parse OBJE value - could be XRef (@M1@) or inline
    if strings.HasPrefix(tag.Value, "@") && strings.HasSuffix(tag.Value, "@") {
        // This is a reference to a standalone OBJE record
        media.GedcomXref = tag.Value
        // Will be resolved when processing standalone OBJE records
        return nil // Skip for now, handle in main OBJE pass
    }

    // Inline OBJE - parse child tags
    for _, child := range tag.Children {
        switch child.Tag {
        case "FILE":
            media.FileRef = child.Value
            media.Filename = filepath.Base(child.Value)
            // Check for FORM subtag
            for _, formTag := range child.Children {
                if formTag.Tag == "FORM" {
                    media.Format = strings.ToLower(formTag.Value)
                    media.MimeType = gedcomFormatToMime(media.Format)
                }
            }
        case "TITL":
            media.Title = child.Value
        case "NOTE":
            media.Notes = child.Value
        case "FORM":
            // FORM can also be direct child of OBJE
            media.Format = strings.ToLower(child.Value)
            media.MimeType = gedcomFormatToMime(media.Format)
        }
    }

    // Default title if not provided
    if media.Title == "" {
        media.Title = media.Filename
    }
    if media.Title == "" {
        media.Title = "Untitled Media"
    }

    return media
}

// gedcomFormatToMime converts GEDCOM format values to MIME types.
func gedcomFormatToMime(format string) string {
    switch format {
    case "jpeg", "jpg":
        return "image/jpeg"
    case "gif":
        return "image/gif"
    case "png":
        return "image/png"
    case "bmp":
        return "image/bmp"
    case "tiff", "tif":
        return "image/tiff"
    case "pdf":
        return "application/pdf"
    case "wav":
        return "audio/wav"
    case "mp3":
        return "audio/mpeg"
    default:
        return "application/octet-stream"
    }
}
```

## 4. Update Import() Method

Modify the main Import() function to extract media:

```go
func (imp *Importer) Import(ctx context.Context, reader io.Reader) (*ImportResult, []PersonData, []FamilyData, []SourceData, []CitationData, []MediaData, error) {
    result := &ImportResult{
        PersonXrefToID: make(map[string]uuid.UUID),
        FamilyXrefToID: make(map[string]uuid.UUID),
        SourceXrefToID: make(map[string]uuid.UUID),
        MediaXrefToID:  make(map[string]uuid.UUID),
    }

    // ... existing parsing code ...

    // After persons and families are parsed, extract media
    var mediaItems []MediaData

    // Extract media from individuals
    for _, indi := range doc.Individuals() {
        personID := result.PersonXrefToID[indi.XRef]
        personMedia := extractMediaFromIndividual(indi, personID, result)
        mediaItems = append(mediaItems, personMedia...)
    }

    // Extract media from families
    for _, fam := range doc.Families() {
        familyID := result.FamilyXrefToID[fam.XRef]
        familyMedia := extractMediaFromFamily(fam, familyID, result)
        mediaItems = append(mediaItems, familyMedia...)
    }

    // Process standalone OBJE records if gedcom-go supports them
    // for _, obj := range doc.MediaObjects() {
    //     media := parseMediaObject(obj, result)
    //     mediaItems = append(mediaItems, media)
    // }

    result.MediaImported = len(mediaItems)

    return result, persons, families, sources, citations, mediaItems, nil
}
```

## 5. Handle External File References

GEDCOM files often reference external files. Note this as a warning:

```go
// In the import service that calls the importer
for _, mediaData := range mediaItems {
    if mediaData.FileRef != "" {
        // External file reference - can't import actual file data
        result.Warnings = append(result.Warnings,
            fmt.Sprintf("Media %s references external file: %s (file not imported)",
                mediaData.GedcomXref, mediaData.FileRef))

        // Create media entry with metadata only
        // Actual file would need to be uploaded separately
    }
}
```

## 6. Integration with Import Handler

In the GEDCOM import HTTP handler or service, create Media entries:

```go
// After importing persons, families, sources, citations...
for _, mediaData := range mediaItems {
    // Only create if we have actual file data
    // For external references, create placeholder or skip
    if mediaData.FileRef != "" {
        // Log warning about external file
        continue
    }

    input := command.UploadMediaInput{
        EntityType:  mediaData.EntityType,
        EntityID:    mediaData.EntityID,
        Title:       mediaData.Title,
        Description: mediaData.Notes,
        Filename:    mediaData.Filename,
        MimeType:    mediaData.MimeType,
        FileData:    nil, // No data for external references
    }

    // If we somehow have file data (embedded base64 in some GEDCOM variants)
    // set FileData here

    _, err := cmdHandler.UploadMedia(ctx, input)
    if err != nil {
        result.Warnings = append(result.Warnings,
            fmt.Sprintf("Failed to import media %s: %v", mediaData.GedcomXref, err))
    }
}
```

</requirements>

<implementation>

1. Update ImportResult struct in `internal/gedcom/importer.go`
2. Add MediaData struct
3. Add helper functions: extractMediaFromIndividual, extractMediaFromFamily, parseMediaTag, gedcomFormatToMime
4. Update Import() function signature and implementation
5. Update any callers of Import() to handle the new return value
6. Run `go build ./internal/gedcom/...`

</implementation>

<verification>
```bash
# Build gedcom package
go build ./internal/gedcom/...

# Run gedcom tests
go test ./internal/gedcom/... -v

# Test with sample GEDCOM file containing OBJE records
# (Create test file or use existing test data)
```
</verification>

<output>
After completing this stage, provide:
1. GEDCOM tags parsed
2. Limitations (external files)
3. MIME type mappings

Example output:
```
Stage 9 Complete: GEDCOM Integration

Updated ImportResult:
- Added MediaImported counter
- Added MediaXrefToID mapping

Added MediaData struct with 9 fields

GEDCOM parsing:
- Inline OBJE tags in INDI records
- Inline OBJE tags in FAM records
- FILE, TITL, FORM, NOTE subtags
- XRef references to standalone OBJE (tracked but not resolved)

MIME type mappings:
- jpeg/jpg -> image/jpeg
- gif -> image/gif
- png -> image/png
- pdf -> application/pdf
- etc.

Limitations:
- External file references logged as warnings
- Actual file data not imported (would need separate upload)
- Base64 embedded data not common in GEDCOM, not handled

Build verification: go build passed

Ready for Stage 10 (Tests)
```
</output>
