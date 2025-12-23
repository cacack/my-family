<objective>
Implement the domain layer for sources and citations to support GPS-compliant genealogical research.

This is Phase 1 of implementing GitHub issue #31 (Sources and citations foundation). The domain layer establishes the core entities, enums, and events that all other layers depend on.
</objective>

<context>
Project: Self-hosted genealogy software written in Go with event sourcing architecture.
Issue: #31 - Sources and citations foundation (GPS)
Tech stack: Go 1.22+, Event sourcing with CQRS pattern

The codebase follows Domain-Driven Design patterns. Review existing domain entities to match conventions:
@internal/domain/person.go - Person aggregate pattern
@internal/domain/family.go - Family aggregate pattern
@internal/domain/enums.go - Enum definition pattern
@internal/domain/events.go - Event definition pattern
@internal/domain/date.go - GenDate value object for flexible dates

GPS (Genealogical Proof Standard) requires tracking:
- Source quality: original vs derivative vs authored
- Informant type: primary (witnessed) vs secondary (heard from others)
- Evidence quality: direct vs indirect vs negative
</context>

<requirements>
1. Add source-related enums to `internal/domain/enums.go`:
   - SourceType: book, archive, webpage, census, vital_record, church_record, newspaper, photograph, interview, correspondence, other
   - SourceQuality: original, derivative, authored (per GPS terminology)
   - InformantType: primary, secondary, indeterminate
   - EvidenceType: direct, indirect, negative
   - FactType: person_birth, person_death, person_name, person_gender, family_marriage, family_divorce (what citations attach to)

2. Create `internal/domain/source.go` with:
   - Source aggregate: ID (UUID), SourceType, Title (required), Author, Publisher, PublishDate (*GenDate), URL, RepositoryName, CollectionName, CallNumber, Notes, GedcomXref, Version
   - NewSource(title, sourceType) factory
   - Validate() method (title required, valid sourceType)
   - Citation struct: ID, SourceID, FactType, FactOwnerID (person or family), Page, Volume, SourceQuality, InformantType, EvidenceType, QuotedText, Analysis, TemplateID (for future Evidence Explained), GedcomXref, Version
   - NewCitation(sourceID, factType, factOwnerID) factory
   - Validate() method (sourceID and factOwnerID required, valid enums)

3. Add events to `internal/domain/events.go`:
   - SourceCreated, SourceUpdated, SourceDeleted
   - CitationCreated, CitationUpdated, CitationDeleted
   - Follow BaseEvent pattern with EventType(), AggregateID(), OccurredAt() methods
   - Include factory functions: NewSourceCreated(source), etc.

4. Create corresponding test files with table-driven tests following existing patterns.
</requirements>

<implementation>
Follow these existing patterns exactly:
- Use `github.com/google/uuid` for IDs
- Use pointers for optional fields (e.g., `*GenDate`, `*string` for optional strings)
- Validation returns joined errors using `errors.Join()`
- Enums have IsValid() methods that accept empty string as valid (for optional fields)
- Events embed BaseEvent and implement the Event interface
- Factory functions like NewPersonCreated(person) create events from entities

For FactType, use a string-based approach similar to RelationType - this allows citations to attach to specific facts on persons/families without creating a separate Fact entity yet.
</implementation>

<output>
Create/modify files:
- `./internal/domain/enums.go` - Add new enums
- `./internal/domain/source.go` - NEW: Source and Citation entities
- `./internal/domain/events.go` - Add source/citation events
- `./internal/domain/enums_test.go` - Add enum validation tests
- `./internal/domain/source_test.go` - NEW: Entity and validation tests
- `./internal/domain/events_test.go` - Add event tests
</output>

<verification>
Before completing:
1. Run `go build ./...` - must compile without errors
2. Run `go test ./internal/domain/...` - all tests must pass
3. Verify Source.Validate() rejects empty title
4. Verify Citation.Validate() rejects empty SourceID and FactOwnerID
5. Verify all enum IsValid() methods work correctly
</verification>

<success_criteria>
- All new types compile and follow existing patterns
- Source and Citation have working validation
- All events implement the Event interface
- Test coverage for validation edge cases
- No changes to unrelated code
</success_criteria>
