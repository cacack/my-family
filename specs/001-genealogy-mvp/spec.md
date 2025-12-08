# Feature Specification: My Family Genealogy MVP

**Feature Branch**: `001-genealogy-mvp`
**Created**: 2025-12-07
**Status**: Draft
**Input**: Self-hosted genealogy application with API-first design, GEDCOM import/export, person and family management, pedigree visualization, and search functionality.

## User Scenarios & Testing *(mandatory)*

### User Story 1 - Import Existing Research (Priority: P1)

As a family historian, I want to import a GEDCOM file so I can start with my existing research data without re-entering everything manually.

**Why this priority**: This is the primary entry point for most users who already have genealogy data. Without import capability, users face a prohibitive barrier to adoption requiring manual re-entry of potentially thousands of records.

**Independent Test**: Can be tested by importing a valid GEDCOM file and verifying all individuals, families, and relationships are correctly created in the system.

**Acceptance Scenarios**:

1. **Given** a valid GEDCOM 5.5 file, **When** user uploads it for import, **Then** all individuals are created with their names, dates, places, and gender preserved
2. **Given** a GEDCOM file with family records, **When** import completes, **Then** all family relationships (partners, children) are correctly linked
3. **Given** a GEDCOM file with various date formats (exact, circa, ranges, before/after), **When** imported, **Then** dates are preserved with their original precision indicators
4. **Given** a GEDCOM file with 5,000+ individuals, **When** imported, **Then** import completes successfully and all data is accessible

---

### User Story 2 - Manage Person Records (Priority: P2)

As a family historian, I want to add and edit individual person records so I can capture genealogical facts as I discover them.

**Why this priority**: After import, the most frequent activity is adding new people and updating existing records as research progresses. This is the core data entry workflow.

**Independent Test**: Can be tested by creating a new person, editing their details, and verifying all changes persist correctly.

**Acceptance Scenarios**:

1. **Given** an empty database, **When** user creates a new person with name, birth date/place, death date/place, gender, and notes, **Then** the person record is saved and retrievable
2. **Given** an existing person record, **When** user edits any field, **Then** changes are saved and previous values are not lost
3. **Given** a person with approximate dates (e.g., "circa 1850"), **When** saved, **Then** the approximate nature of the date is preserved
4. **Given** a person record, **When** user views it, **Then** all captured information is displayed including any family relationships

---

### User Story 3 - Create Family Units (Priority: P3)

As a family historian, I want to create family units that link partners and their children so I can model actual family structures including multiple marriages and blended families.

**Why this priority**: Families are the structural backbone of genealogy. Without family units, the system would only have disconnected individuals.

**Independent Test**: Can be tested by creating a family unit with two partners and children, then verifying the relationships from each person's perspective.

**Acceptance Scenarios**:

1. **Given** two existing person records, **When** user creates a family unit with them as partners, **Then** the relationship is established and visible from both persons' records
2. **Given** a family unit, **When** user adds a child, **Then** the child appears linked to both partners as parents
3. **Given** a person who was married multiple times, **When** viewing their record, **Then** all family units they belong to are visible
4. **Given** a child with unknown father, **When** creating the family unit, **Then** the system allows a single parent family

---

### User Story 4 - View Pedigree Chart (Priority: P4)

As a family historian, I want to view an interactive pedigree chart for any person so I can visualize lineage across generations.

**Why this priority**: Visualization is essential for understanding family relationships at a glance and identifying gaps in research. This transforms raw data into actionable research guidance.

**Independent Test**: Can be tested by selecting any person and viewing their ancestor tree, navigating through generations, and verifying accuracy against known relationships.

**Acceptance Scenarios**:

1. **Given** a person with known parents and grandparents, **When** user opens pedigree view, **Then** ancestors are displayed in a standard pedigree chart layout
2. **Given** a pedigree chart, **When** user clicks on any ancestor, **Then** they can navigate to that person's details or re-center the chart on them
3. **Given** a person with 4+ generations of ancestors, **When** viewing pedigree, **Then** user can navigate/scroll to view all generations
4. **Given** a person with unknown parents, **When** viewing pedigree, **Then** empty placeholders indicate where research is needed

---

### User Story 5 - Search People (Priority: P5)

As a family historian, I want to search for people by name with partial/fuzzy matching so I can quickly locate individuals in a growing tree.

**Why this priority**: As trees grow, finding specific individuals becomes increasingly difficult. Search is essential for daily research workflow efficiency.

**Independent Test**: Can be tested by searching for names with partial strings, common misspellings, and verifying relevant results are returned.

**Acceptance Scenarios**:

1. **Given** a tree with many individuals, **When** user searches by full name, **Then** exact matches appear first in results
2. **Given** a partial name search (e.g., "John Sm"), **When** submitted, **Then** results include all matching names (Smith, Smythe, etc.)
3. **Given** a search with common spelling variations (e.g., "Catherine" vs "Katherine"), **When** submitted, **Then** both variations appear in results
4. **Given** a tree with 10,000 individuals, **When** searching, **Then** results appear within 1 second

---

### User Story 6 - Export Data (Priority: P6)

As a family historian, I want to export my data as a GEDCOM file so I can back up my work, share with relatives, or migrate to other tools.

**Why this priority**: Data portability is a core value proposition - users must never feel locked into the system. This also enables backup and sharing workflows.

**Independent Test**: Can be tested by exporting a tree, then re-importing to verify round-trip data integrity.

**Acceptance Scenarios**:

1. **Given** a populated family tree, **When** user initiates GEDCOM export, **Then** a valid GEDCOM 5.5 file is generated
2. **Given** exported GEDCOM, **When** imported into another GEDCOM-compatible tool, **Then** all individuals, families, and relationships are readable
3. **Given** a tree with complex date formats, **When** exported, **Then** date precision and qualifiers are preserved in GEDCOM format
4. **Given** a large tree (10,000+ individuals), **When** exported, **Then** export completes within reasonable time and produces valid output

---

### User Story 7 - REST API Access (Priority: P7)

As a developer or power user, I want all functionality exposed through a well-documented REST API so I can build alternative interfaces, automate tasks, or integrate with other tools.

**Why this priority**: API-first design is an architectural requirement. All other user stories implicitly depend on the API existing. This story ensures the API is documented and usable by third parties.

**Independent Test**: Can be tested by performing all user operations via API calls alone, verifying responses match expected formats.

**Acceptance Scenarios**:

1. **Given** API documentation, **When** developer reviews it, **Then** all endpoints for CRUD operations on persons, families, and imports/exports are documented
2. **Given** API endpoints, **When** called with valid parameters, **Then** responses follow consistent JSON structure with appropriate status codes
3. **Given** an API request, **When** an error occurs, **Then** response includes actionable error message and appropriate HTTP status
4. **Given** API documentation, **When** developer attempts to integrate, **Then** endpoint paths, request/response formats, and examples are clearly documented

---

### Edge Cases

- What happens when importing a malformed GEDCOM file? System provides clear error messages indicating the problem location and nature, imports valid portions where possible, and reports which records could not be imported.
- How does the system handle circular relationships (e.g., data entry error where person is their own ancestor)? System detects and prevents circular ancestry relationships, displaying a clear error message.
- What happens when deleting a person who is linked to families? System warns user about dependent relationships, requiring confirmation, and removes the person from family units while preserving other family members.
- How does the system handle duplicate person detection during import? System flags potential duplicates based on name and date similarity, allowing user to merge or keep separate.
- What happens when importing a GEDCOM with encoding issues (e.g., special characters)? System attempts to detect encoding and convert to UTF-8, preserving special characters in names and places.

## Requirements *(mandatory)*

### Functional Requirements

- **FR-001**: System MUST import GEDCOM 5.5 format files, preserving all standard individual and family data
- **FR-002**: System MUST support flexible date representations: exact dates, circa/approximate dates, date ranges, "before/after" qualifiers, and unknown dates
- **FR-003**: System MUST allow creation, reading, updating, and deletion of person records with: given name, surname, birth date/place, death date/place, gender, and notes
- **FR-004**: System MUST support family units linking two partners (or one for single-parent families) with zero or more children
- **FR-005**: System MUST allow a person to belong to multiple family contexts (as child in birth family, as partner in formed families)
- **FR-006**: System MUST track relationship types for children (biological, adopted, foster) with biological as the default
- **FR-007**: System MUST provide a pedigree chart visualization showing ancestors for any selected person
- **FR-008**: System MUST support navigation within the pedigree chart to re-center on any displayed person
- **FR-009**: System MUST provide name search with partial matching (prefix and contains) capability
- **FR-010**: System MUST provide fuzzy name matching to handle common spelling variations
- **FR-011**: System MUST export data as valid GEDCOM 5.5 format files
- **FR-012**: System MUST expose all functionality via REST API with JSON responses
- **FR-013**: System MUST support human-readable and JSON output formats for all API responses
- **FR-014**: System MUST operate without external service dependencies for core functionality
- **FR-015**: System MUST persist all data locally under user control
- **FR-016**: System MUST support single-binary deployment and Docker container deployment
- **FR-017**: System MUST provide a web-based user interface that consumes the REST API

### Key Entities

- **Person**: An individual in the family tree. Attributes include: unique identifier, given name(s), surname, birth date, birth place, death date, death place, gender, and notes. A person can be a child in one family and a partner in zero or more families.

- **Family**: A family unit consisting of partners and their children. Attributes include: unique identifier, partner references (0-2 persons), child references (0 or more persons with relationship type), marriage/partnership date, and marriage/partnership place.

- **Date**: A genealogical date with flexible precision. Supports: exact dates, approximate dates (circa), date ranges (between X and Y), bounded dates (before/after), and unknown. Preserves original input format for GEDCOM round-trip fidelity.

- **Place**: A location associated with an event (birth, death, marriage). Stores the place name as provided, supporting hierarchical location strings (e.g., "City, County, State, Country").

## Success Criteria *(mandatory)*

### Measurable Outcomes

- **SC-001**: Users can import a 5,000-person GEDCOM file and begin working within 30 seconds
- **SC-002**: Users can add a new person record in under 1 minute including all basic fields
- **SC-003**: Users can create a family unit linking existing persons in under 30 seconds
- **SC-004**: Pedigree chart displays and responds to navigation within 1 second for trees up to 10,000 individuals
- **SC-005**: Search returns results within 1 second for trees up to 10,000 individuals
- **SC-006**: GEDCOM export completes within 30 seconds for trees up to 10,000 individuals
- **SC-007**: Exported GEDCOM files successfully import into at least 2 other major genealogy tools without data loss
- **SC-008**: All seven user stories can be completed using only API calls (no web UI required)
- **SC-009**: System runs successfully as both a single binary and Docker container without code changes
- **SC-010**: A user unfamiliar with the system can import data and view a pedigree chart within 5 minutes of first launch

## Assumptions

- Single-user mode is sufficient for MVP; no authentication/authorization required
- SQLite or embedded database is acceptable for local data persistence
- Web UI will be responsive design suitable for desktop browsers; mobile-native apps are out of scope
- GEDCOM 5.5 is the primary format; GEDCOM 7.0 awareness is a future enhancement
- Users are comfortable with command-line tools for initial setup and deployment
- Pedigree chart shows ancestors only (not descendants) for MVP
