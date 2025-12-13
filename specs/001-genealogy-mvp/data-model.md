# Data Model: My Family Genealogy MVP

**Branch**: `001-genealogy-mvp` | **Date**: 2025-12-07

## Overview

This document defines the domain entities, event store schema, and read model structures for the genealogy MVP. The architecture follows event sourcing with CQRS-lite pattern.

---

## Domain Entities

### Person

The core entity representing an individual in the family tree.

| Field | Type | Required | Description |
|-------|------|----------|-------------|
| id | UUID | Yes | Unique identifier |
| given_name | string | Yes | First/given name(s) |
| surname | string | Yes | Family/last name |
| gender | Gender | No | male, female, unknown |
| birth_date | GenDate | No | Birth date with precision |
| birth_place | string | No | Birth location |
| death_date | GenDate | No | Death date with precision |
| death_place | string | No | Death location |
| notes | string | No | Free-form notes |
| gedcom_xref | string | No | Original GEDCOM @XREF@ for round-trip |

**Validation Rules**:
- `given_name` and `surname` must be non-empty, max 100 characters each
- `gender` must be one of: `male`, `female`, `unknown`, or null
- If `death_date` is set, it must be after or equal to `birth_date`

**State Transitions**:
- Created → Active (default state)
- Active → Deleted (soft delete, preserves audit trail)

### Family

A family unit linking partners and their children.

| Field | Type | Required | Description |
|-------|------|----------|-------------|
| id | UUID | Yes | Unique identifier |
| partner1_id | UUID | No | First partner reference |
| partner2_id | UUID | No | Second partner reference |
| relationship_type | RelationType | No | marriage, partnership, unknown |
| marriage_date | GenDate | No | Marriage/partnership date |
| marriage_place | string | No | Marriage/partnership location |
| gedcom_xref | string | No | Original GEDCOM @XREF@ |

**Validation Rules**:
- At least one partner must be set (single-parent families allowed)
- `partner1_id` and `partner2_id` must be different if both set
- Partners must exist as Person records

### FamilyChild

Junction entity linking children to families with relationship metadata.

| Field | Type | Required | Description |
|-------|------|----------|-------------|
| family_id | UUID | Yes | Parent family reference |
| person_id | UUID | Yes | Child person reference |
| relationship_type | ChildRelationType | Yes | biological, adopted, foster |
| sequence | int | No | Birth order (optional) |

**Validation Rules**:
- `relationship_type` defaults to `biological`
- `person_id` must not be the same as either partner in the family (prevents circular ancestry)
- Person can be a child in at most one family (birth family)

### GenDate (Value Object)

Genealogical date supporting flexible precision per GEDCOM 5.5 spec.

| Field | Type | Required | Description |
|-------|------|----------|-------------|
| raw | string | Yes | Original input string |
| qualifier | DateQualifier | Yes | exact, abt, cal, est, bef, aft, bet, from |
| year | int | No | Year (nil if unknown) |
| month | int | No | Month 1-12 (nil if unknown) |
| day | int | No | Day 1-31 (nil if unknown) |
| year2 | int | No | End year for ranges |
| month2 | int | No | End month for ranges |
| day2 | int | No | End day for ranges |
| calendar | string | No | DGREGORIAN (default), DJULIAN, etc. |

**DateQualifier Enum**:
- `exact` - No qualifier, precise date
- `abt` - About/approximately (ABT)
- `cal` - Calculated (CAL)
- `est` - Estimated (EST)
- `bef` - Before (BEF)
- `aft` - After (AFT)
- `bet` - Between (BET ... AND ...)
- `from` - From/to range (FROM ... TO ...)

**Examples**:
- `1 JAN 1850` → `{raw: "1 JAN 1850", qualifier: exact, year: 1850, month: 1, day: 1}`
- `ABT 1850` → `{raw: "ABT 1850", qualifier: abt, year: 1850}`
- `BET 1850 AND 1860` → `{raw: "BET 1850 AND 1860", qualifier: bet, year: 1850, year2: 1860}`

### Place (Value Object)

Location string stored as-is to preserve hierarchical format.

| Field | Type | Required | Description |
|-------|------|----------|-------------|
| name | string | Yes | Full place name (e.g., "Springfield, IL, USA") |

---

## Enumerations

### Gender
```go
type Gender string
const (
    GenderMale    Gender = "male"
    GenderFemale  Gender = "female"
    GenderUnknown Gender = "unknown"
)
```

### RelationType
```go
type RelationType string
const (
    RelationMarriage    RelationType = "marriage"
    RelationPartnership RelationType = "partnership"
    RelationUnknown     RelationType = "unknown"
)
```

### ChildRelationType
```go
type ChildRelationType string
const (
    ChildBiological ChildRelationType = "biological"
    ChildAdopted    ChildRelationType = "adopted"
    ChildFoster     ChildRelationType = "foster"
)
```

---

## Event Store Schema

### streams Table

Aggregate stream metadata.

| Column | Type | Constraints | Description |
|--------|------|-------------|-------------|
| id | UUID | PK | Stream identifier (= aggregate ID) |
| type | VARCHAR(50) | NOT NULL | "Person" or "Family" |
| created_at | TIMESTAMPTZ | NOT NULL | Stream creation time |
| metadata | JSONB/BLOB | | Optional stream metadata |

### events Table

Immutable event log.

| Column | Type | Constraints | Description |
|--------|------|-------------|-------------|
| id | UUID | PK | Event unique identifier |
| stream_id | UUID | FK → streams.id | Aggregate this event belongs to |
| version | BIGINT | NOT NULL | Aggregate version (1-based) |
| event_type | VARCHAR(100) | NOT NULL | Event type name |
| data | JSONB/BLOB | NOT NULL | Event payload (JSON) |
| metadata | JSONB/BLOB | | Correlation ID, user, etc. |
| timestamp | TIMESTAMPTZ | NOT NULL | Event timestamp |
| position | BIGSERIAL | UNIQUE | Global event ordering |

**Indexes**:
- `UNIQUE(stream_id, version)` - Optimistic locking
- `INDEX(stream_id, version)` - Stream replay
- `INDEX(position)` - Global ordering for projections
- `INDEX(event_type, timestamp)` - Event type queries

---

## Domain Events

### Person Events

**PersonCreated**
```json
{
  "event_type": "PersonCreated",
  "data": {
    "id": "uuid",
    "given_name": "string",
    "surname": "string",
    "gender": "male|female|unknown|null",
    "birth_date": "GenDate|null",
    "birth_place": "string|null",
    "death_date": "GenDate|null",
    "death_place": "string|null",
    "notes": "string|null",
    "gedcom_xref": "string|null"
  }
}
```

**PersonUpdated**
```json
{
  "event_type": "PersonUpdated",
  "data": {
    "id": "uuid",
    "changes": {
      "given_name": "string|null",
      "surname": "string|null",
      "gender": "male|female|unknown|null",
      "birth_date": "GenDate|null",
      "birth_place": "string|null",
      "death_date": "GenDate|null",
      "death_place": "string|null",
      "notes": "string|null"
    }
  }
}
```

**PersonDeleted**
```json
{
  "event_type": "PersonDeleted",
  "data": {
    "id": "uuid",
    "reason": "string|null"
  }
}
```

### Family Events

**FamilyCreated**
```json
{
  "event_type": "FamilyCreated",
  "data": {
    "id": "uuid",
    "partner1_id": "uuid|null",
    "partner2_id": "uuid|null",
    "relationship_type": "marriage|partnership|unknown|null",
    "marriage_date": "GenDate|null",
    "marriage_place": "string|null",
    "gedcom_xref": "string|null"
  }
}
```

**FamilyUpdated**
```json
{
  "event_type": "FamilyUpdated",
  "data": {
    "id": "uuid",
    "changes": {
      "partner1_id": "uuid|null",
      "partner2_id": "uuid|null",
      "relationship_type": "marriage|partnership|unknown|null",
      "marriage_date": "GenDate|null",
      "marriage_place": "string|null"
    }
  }
}
```

**ChildLinkedToFamily**
```json
{
  "event_type": "ChildLinkedToFamily",
  "data": {
    "family_id": "uuid",
    "person_id": "uuid",
    "relationship_type": "biological|adopted|foster",
    "sequence": "int|null"
  }
}
```

**ChildUnlinkedFromFamily**
```json
{
  "event_type": "ChildUnlinkedFromFamily",
  "data": {
    "family_id": "uuid",
    "person_id": "uuid"
  }
}
```

**FamilyDeleted**
```json
{
  "event_type": "FamilyDeleted",
  "data": {
    "id": "uuid",
    "reason": "string|null"
  }
}
```

### GEDCOM Events

**GedcomImported**
```json
{
  "event_type": "GedcomImported",
  "data": {
    "filename": "string",
    "file_size": "int",
    "persons_imported": "int",
    "families_imported": "int",
    "warnings": ["string"],
    "errors": ["string"]
  }
}
```

---

## Read Model Schema

### persons Table (Materialized View)

Denormalized person records for fast retrieval.

| Column | Type | Constraints | Description |
|--------|------|-------------|-------------|
| id | UUID | PK | Person ID |
| given_name | VARCHAR(100) | NOT NULL | First/given name |
| surname | VARCHAR(100) | NOT NULL | Family name |
| full_name | VARCHAR(200) | GENERATED | given_name + surname |
| gender | VARCHAR(10) | | male, female, unknown |
| birth_date_raw | VARCHAR(100) | | Original date string |
| birth_date_sort | DATE | | Sortable date (approx) |
| birth_place | VARCHAR(255) | | Birth location |
| death_date_raw | VARCHAR(100) | | Original date string |
| death_date_sort | DATE | | Sortable date (approx) |
| death_place | VARCHAR(255) | | Death location |
| notes | TEXT | | Free-form notes |
| search_vector | TSVECTOR | | Full-text search (PostgreSQL) |
| version | BIGINT | NOT NULL | Last applied event version |
| updated_at | TIMESTAMPTZ | NOT NULL | Last update timestamp |

**Indexes**:
- `INDEX(surname, given_name)` - Name sorting
- `INDEX(birth_date_sort)` - Date range queries
- `GIN(search_vector)` - Full-text search (PostgreSQL)
- `GIN(surname gin_trgm_ops)` - Fuzzy search (PostgreSQL)

### families Table (Materialized View)

| Column | Type | Constraints | Description |
|--------|------|-------------|-------------|
| id | UUID | PK | Family ID |
| partner1_id | UUID | FK | First partner |
| partner1_name | VARCHAR(200) | | Denormalized name |
| partner2_id | UUID | FK | Second partner |
| partner2_name | VARCHAR(200) | | Denormalized name |
| relationship_type | VARCHAR(20) | | marriage, partnership, unknown |
| marriage_date_raw | VARCHAR(100) | | Original date string |
| marriage_date_sort | DATE | | Sortable date |
| marriage_place | VARCHAR(255) | | Marriage location |
| child_count | INT | DEFAULT 0 | Number of children |
| version | BIGINT | NOT NULL | Last applied event version |
| updated_at | TIMESTAMPTZ | NOT NULL | Last update timestamp |

### family_children Table (Materialized View)

| Column | Type | Constraints | Description |
|--------|------|-------------|-------------|
| family_id | UUID | PK, FK | Parent family |
| person_id | UUID | PK, FK | Child person |
| person_name | VARCHAR(200) | | Denormalized name |
| relationship_type | VARCHAR(20) | NOT NULL | biological, adopted, foster |
| sequence | INT | | Birth order |

**Indexes**:
- `INDEX(person_id)` - Find families for a person

### pedigree_edges Table (Graph Structure)

Optimized for ancestry traversal queries.

| Column | Type | Constraints | Description |
|--------|------|-------------|-------------|
| person_id | UUID | PK | The person |
| father_id | UUID | | Father reference |
| mother_id | UUID | | Mother reference |
| father_name | VARCHAR(200) | | Denormalized |
| mother_name | VARCHAR(200) | | Denormalized |

**Indexes**:
- `INDEX(father_id)` - Find children of father
- `INDEX(mother_id)` - Find children of mother

---

## SQLite Adaptations

For SQLite compatibility:

| PostgreSQL | SQLite |
|------------|--------|
| UUID | TEXT (stored as string) |
| TIMESTAMPTZ | TEXT (ISO 8601 format) |
| JSONB | TEXT (JSON string) |
| BIGSERIAL | INTEGER PRIMARY KEY AUTOINCREMENT |
| TSVECTOR | FTS5 virtual table |
| gin_trgm_ops | Application-level fuzzy matching |

### SQLite FTS5 for Search

```sql
CREATE VIRTUAL TABLE persons_fts USING fts5(
    given_name,
    surname,
    content='persons',
    content_rowid='rowid'
);

-- Triggers to keep FTS in sync
CREATE TRIGGER persons_fts_insert AFTER INSERT ON persons BEGIN
    INSERT INTO persons_fts(rowid, given_name, surname)
    VALUES (new.rowid, new.given_name, new.surname);
END;
```

---

## Entity Relationships

```
Person (1) ←──────────────────── (*) FamilyChild
   │                                     │
   │                                     │
   │ as partner                          │ as child
   │                                     │
   ▼                                     ▼
Family (*) ────────────────────→ (1) Family
   │
   │ partner1_id, partner2_id
   │
   ▼
Person (0..2)
```

**Cardinality**:
- Person can be a child in 0..1 Family (birth family)
- Person can be a partner in 0..* Families (multiple marriages)
- Family has 0..2 partners
- Family has 0..* children

---

## GEDCOM Mapping

| GEDCOM Tag | Domain Entity | Field |
|------------|---------------|-------|
| INDI | Person | - |
| NAME | Person | given_name, surname |
| SEX | Person | gender |
| BIRT.DATE | Person | birth_date |
| BIRT.PLAC | Person | birth_place |
| DEAT.DATE | Person | death_date |
| DEAT.PLAC | Person | death_place |
| NOTE | Person | notes |
| FAM | Family | - |
| HUSB | Family | partner1_id |
| WIFE | Family | partner2_id |
| MARR.DATE | Family | marriage_date |
| MARR.PLAC | Family | marriage_place |
| CHIL | FamilyChild | person_id |
| @XREF@ | Person/Family | gedcom_xref |
