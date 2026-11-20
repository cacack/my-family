# ADR-008: Privacy Model — Living People, Private Records and Redaction

**Status:** Proposed
**Date:** 2026-11-18
**Decision Makers:** Project maintainer
**Related Features:** [#1411](https://github.com/cacack/my-family/issues/1411),
[#1412](https://github.com/cacack/my-family/issues/1412),
[#488](https://github.com/cacack/my-family/issues/488)

## Context

[ADR-007](./007-access-model.md) gives every request a principal (anonymous, viewer, editor,
owner) and settles who may call each operation. It leaves open which records a principal may
*see*. Anonymous viewing (#1414) and viewer accounts must not ship until living people are
protected.

Nothing in the code protects anyone today:

- There is no living-person concept, no private flag and no GEDCOM `RESN` handling. The only
  related logic is the quality score's "born over 100 years ago" heuristic
  (`internal/query/quality_service.go`, used by the completeness score and missing-date
  suggestions).
- gedcom-go v3.6.0, the version this repo pins, parses `RESN` into a typed `Restriction` on
  events, attributes and media, but keeps record-level `RESN` on `INDI` and `FAM` only as raw
  tags. v3.7.0 types it on `Individual` and `Family` too
  ([cacack/gedcom-go#739](https://github.com/cacack/gedcom-go/issues/739)).
- Tree data reaches the API by two routes. Most operations read the read model through
  `repository.ReadModelStore`. History, snapshot comparison, branch comparison, merge review,
  the branch research archive and rollback read the event log and describe past event payloads.

This ADR decides who counts as living, how records are marked private, what each principal sees
instead of a protected record, and where that is enforced. The [read-surface
table](#read-surface-table) is the checklist #1412 implements against.

## Decision Drivers

- Fail safe. When the data cannot show someone is dead, treat them as living.
- Build it once. A rule repeated per screen or per handler is a rule that is missing somewhere.
- New read paths must be covered without anyone remembering to cover them.
- The event log is append-only (ES-002); privacy cannot be applied by rewriting it.
- Privacy marks must survive GEDCOM export and re-import, and must not alter what a researcher
  recorded (`RESN locked` included).
- Both SQL backends behave the same (DB-001).
- Branch views follow the same rules as the mainline (ADR-005).

## Considered Options

### Sub-decision 1 — Who counts as living?

#### Option 1A: Fixed 100-year rule on birth year

**Pros:** matches the existing quality heuristic; trivial.
**Cons:** 100 years exposes living centenarians; no answer for people with no birth date; no way
for an editor to correct a wrong guess.

#### Option 1B: Evidence of death, else a configurable birth threshold, with an editor override (chosen)

**Pros:** uses what the record says before guessing; the threshold is the owner's choice; an
editor fixes the cases the rule gets wrong.
**Cons:** status changes as time passes, so it cannot be a projected column.

#### Option 1C: Also infer from relatives (a parent born 200 years ago, a child who died in 1900)

**Pros:** fewer false "living" results on sparse trees.
**Cons:** a graph walk per person, and a wrong date on one relative changes the status of
others. The override covers the same cases.

#### Option 1D: Only an explicit per-person flag

**Cons:** fails open: an imported tree is fully exposed until someone marks every living person.

### Sub-decision 2 — How is a record marked private?

#### Option 2A: An event-sourced `restriction` on the record (chosen)

**Pros:** audited like any other change; replays, branches and merges handle it with no new
machinery; maps directly onto GEDCOM `RESN`.
**Cons:** one more field on six record types and their events.

#### Option 2B: A separate access-control list outside the event log

**Pros:** no event changes.
**Cons:** does not travel with exports or branches; a second store to keep consistent with the
records it names.

### Sub-decision 3 — What does a protected person look like?

#### Option 3A: Omit protected people entirely

**Cons:** charts and family pages break: a child appears with no parent, and the gap misleads.

#### Option 3B: Placeholder keeping the surname ("Living Smith")

**Cons:** the surname plus the chart position is often enough to identify the person.

#### Option 3C: Anonymous placeholder "Living" (chosen)

**Pros:** charts keep their shape; no identifying detail is shown.
**Cons:** the placeholder's position still shows that someone exists there (see Consequences).

### Sub-decision 4 — Where is it enforced?

#### Option 4A: Checks in each handler or query service

**Cons:** a check in every operation and query service; a forgotten one fails open.

#### Option 4B: Filter the JSON response in HTTP middleware

**Cons:** dozens of response shapes; names are already baked into strings (relationship
summaries, history descriptions) and counts are already aggregated by the time a response exists.

#### Option 4C: Database row-level security

**Cons:** PostgreSQL only; SQLite has none (DB-001). The living rule depends on today's date and
tree settings, which RLS policies handle poorly.

#### Option 4D: A redacting `ReadModelStore` decorator only

**Cons:** event-log surfaces never touch the read model's person rows for the values they show,
so history and comparisons would leak every past name and date.

#### Option 4E: Two choke points: the decorator plus an event-output redactor (chosen)

**Pros:** every read-model consumer is covered by construction; the event-log consumers share
one output type (`query.ChangeEntry`), so one redactor covers them.
**Cons:** two places to keep in step; mitigated by both calling one policy function.

## Decision

We chose **1B + 2A + 3C + 4E**, with the principal visibility of section 4 and the settings of
section 6.

### 1. Living rule

A person is **living** unless one of these holds, evaluated on the person's current read-model
state in the branch being viewed:

1. a death, burial or cremation event exists that is not negated, with or without a date; or
2. the birth date, or the christening or baptism date when there is no birth date, is more than
   **N** years before today. N defaults to **110** and is a tree setting.

A person with no qualifying dates at all is living. For date ranges and approximations the
comparison uses the latest date the value allows (`BET 1900 AND 1920` → 1920, `BEF 1900` → 1900);
a value with no upper bound (`AFT 1900`) counts as no date.

An editor can override per person: **auto** (the rule), **living** or **deceased**. The override
is event-sourced (a field on the person's events; absent means auto) and appears in history.

Status is **computed at read time**, not stored by the projection, because it changes as time
passes with no new event. The projection may store the time-independent inputs (whether death
evidence exists, the comparison date, the override) so the read-time test is one comparison.

Rejected: inferring from relatives' dates (1C), and a 100-year threshold (1A).

### 2. Private flag

`person`, `family`, `source`, `citation`, `note` and `media` gain an event-sourced
`restriction`: the record's GEDCOM `RESN` values, kept as recorded. A record is **private** when
its restriction contains `confidential` or `privacy`. `locked` is an edit lock, not a visibility
mark: it does not make a record private and round-trips untouched. Old events have no
restriction and decode to none (no upcaster, per [ADR-006](./006-schema-evolution.md)). The UI's
"private" toggle adds or removes `confidential` and leaves other values alone.

GEDCOM import reads `RESN` on each of these records and export writes it back, so the flag
survives export → re-import. Record-level `RESN` on `INDI` and `FAM` uses the typed field from
gedcom-go v3.7.0 (cacack/gedcom-go#739), so #1412 upgrades the dependency rather than reading the
raw tag. Where a GEDCOM version defines no `RESN` on
a record type, the exporter writes the same values as an extension tag and the importer reads it
back; #1412 confirms the tag per record type against GEDCOM 5.5.1 and 7.0.

Event- and attribute-level `RESN` (already typed in gedcom-go) is out of scope here; such facts
round-trip but do not drive visibility yet.

### 3. Placeholder

A **protected person** is shown as a placeholder: same ID, same family links, given name
"Living", no surname, its sex (so charts lay out and relationship labels read "grandson"), and
a `redacted` marker the UI uses to render it without links to edit or detail. Hidden: names and name variants, dates, places, events, attributes, notes, media,
citations, associations, LDS ordinances, external IDs, quality scores and the evidence records
whose subject is the person.

- Charts, family pages, group sheets and relationship paths show the placeholder.
- Protected people never match search and never appear in browse, surname, place, cemetery, map,
  brick-wall, duplicate, quality or discovery lists. Grand totals (person and family counts) may
  include them; anything that names or locates them may not.
- A family with a protected partner keeps its structure; its own facts (marriage date and place,
  family events, notes, media) are hidden.
- An association, citation or evidence record that attaches to a protected person is hidden.
- A shared note is hidden from a principal when every record that refers to it is protected or
  private for that principal.
- **Private** records other than people are omitted: absent from lists, 404 on direct fetch. A
  private person is hidden the same way, except where a chart or family needs the slot, where it
  is the placeholder.
- In charts, a private ancestor reached through a family the principal may see is the
  placeholder, not a gap, so the chart keeps its shape. A **private family** is a gap: its
  child-to-parents link is hidden from pedigree and relationship walks as it is from family
  pages and descendancy, so the child shows no parents through it. A pedigree edge carries no
  family ID, so the link is matched by the child and the family's partners.
- A record that cannot be resolved against the read model (deleted, or on a purged branch) is
  treated as protected.

Rejected: "Living Smith" (3B) and omitting people (3A).

### 4. Who sees what

| Principal | Living people | Private records |
|-----------|---------------|-----------------|
| Anonymous | Placeholder | Hidden |
| Viewer | Shown (default); placeholder when the owner turns off "viewers see living" | Hidden |
| Editor | Shown | Shown |
| Owner | Shown | Shown |

Two kinds of record outside the person rules:

- **Submitters** hold a living researcher's name, address and contact details and have no
  restriction field. They are hidden from anonymous principals and shown to viewers and up.
- **Change attribution** (`user_id` on history entries, `merged_by` on merge records, #1407) is
  account data showing who edited what and when. Anonymous principals are refused the change
  log that carries it (history, restore points, snapshots and their comparisons, branch compare)
  and must sign in (#1441); elsewhere it is removed for them by whichever choke point serves the
  surface. Viewers and up see it.
- **Research branches** hold work in progress, not the tree the owner publishes. Anonymous
  principals read the mainline only: a `?branch=` scope on any request, and a branch's own
  view (comparison, health, evidence coverage), are refused and need sign-in (#1513). They
  may still list branches and read their names, status and drift counts, and the research
  archive of a closed branch. The app offers them no branch to switch to and no link that
  leads only to a sign-in wall. Viewers and up may scope reads to a branch.

Open mode (`AUTH_MODE=open`, ADR-007) maps to owner. A context carrying no principal is treated
as anonymous, so a missing principal fails safe.

The owner may preview the tree as an anonymous principal or a viewer (#1413): the sign-in
middleware then puts that principal, and its role, on the owner's requests, so both choke points
apply to them unchanged and the preview cannot drift from what that principal is served.

### 5. Enforcement: two choke points

One policy function decides, for a principal, tree settings and today's date, whether a record
is **visible**, a **placeholder** or **hidden**. It has two callers and nothing else repeats its
logic.

**(a) Read-model decorator.** A `ReadModelStore` implementation that wraps the real store and
applies the policy for the principal on the context. It covers everything that reads the read
model: person and family pages, browse, search, charts, map, relationship, quality and reports,
evidence, media, and JSON/CSV/GEDCOM export.

- It does not embed the wrapped interface, so a new read method is a compile error until someone
  decides how it is redacted.
- Writes pass through. Projections, command handlers, `myfamily rebuild` and the CLI are wired to
  the raw store; only the read path (query services and read handlers) gets the decorator.
- List, search and aggregate methods take the visibility filter as a query parameter so the
  database excludes protected rows. Filtering a returned page is not allowed: it gives short
  pages, wrong totals and leaked counts. The rule therefore has a Go form (single records) and a
  SQL form (lists and aggregates) in both backends, checked against one shared test table.

**(b) Event-output redactor.** Every query that reads the event log returns `ChangeEntry` values
(history, snapshot comparison, branch comparison, merge plan and conflicts, research archive,
evidence coverage, restore points and rollback preview). One redactor runs on them before they
leave the query layer: it drops entries whose entity, or parent entity, is protected or private
for the principal, and blanks field values that name a protected person. A test fails if a
service that reads `EventStore` returns without it.

**Why one choke point is not enough.** The read model holds current state; the event log holds
every past value: a name before it was corrected, a death date later removed, a record since
deleted. Event-log queries fold payloads themselves and take nothing but existence checks from
the read model, so the decorator never sees the values they show. The redactor, in turn, works
on finished entries and cannot exclude rows from a paged SQL query. Each covers what the other
cannot.

**Leaks a naive decorator misses**, each of which #1412 must test:

- **Search on a hidden name.** Post-filtering a page drops the row but leaves the total, and a
  placeholder in results would confirm a living Smith exists. Matching (names, name variants,
  place and date filters, fuzzy and soundex) must exclude protected people in the query.
- **Counts.** Surname, place and cemetery indexes, map pins, statistics (top surnames, latest
  birth date) and quality counts are aggregated in SQL; the decorator only sees the numbers.
- **Media content and thumbnails.** `GetMediaWithData` and `GetMediaThumbnail` return bytes with
  no owner; the decorator must load the media row and check its owner and restriction first.
- **Relationship path labels.** Path names and the summary are strings the service builds from
  names it read; they are safe only because the service reads through the decorator.
- **Home widgets.** The discovery feed, recent people and the quality "lowest scoring" list name
  people; any widget built on recent changes is an event-log surface.
- **Exports and side tables.** The GEDCOM and CSV/JSON exporters walk events, attributes, names,
  citations, external IDs and family-child links by owner ID, not through person rows. Every
  side-table method applies its owner's protection.

### 6. Settings

N (`living_threshold_years`, default 110) and "viewers see living" (default on) are **tree
settings** in the ADR-007 split. Their store arrives with #1408; until then the defaults apply
and cannot be changed.

### 7. Read-surface table

The [table at the end](#read-surface-table) assigns every operation that returns tree data to a
choke point, with its special handling. It is the checklist #1412 implements against. It is a
snapshot of `openapi.yaml` as of this ADR's date, so #1412 adds a test that fails when an
`operationId` in the spec has no row, which keeps a new endpoint from going unclassified.

## Consequences

### Positive

- One policy, two callers: a new read method or event-log query is covered or fails to compile
  or test.
- Living people are protected by default on every install, including freshly imported trees.
- Privacy marks are ordinary audited events and round-trip through GEDCOM.
- Branch views apply the same rules on the branch's overlay, so a branch that records a death
  shows that person as deceased in that branch only. A comparison treats a person as protected
  if either side does.

### Negative

- Living status is computed on every read. On a large tree a list or aggregate query evaluates
  it for every row.
  - Mitigation: the projection stores the time-independent inputs, so the read-time test is one
    indexed date comparison in SQL; single records use the same inputs in Go.
- Placeholder IDs are stable, so chart structure shows that a person exists, their position and
  their parentage. Accepted: hiding it breaks charts (3A), and the rule hides who they are.
- Trees with sparse dates will show many "Living" placeholders to the public.
  - Mitigation: the per-person override, and the threshold setting once #1408 lands.
- The rule exists in Go and in SQL for two backends.
  - Mitigation: one shared table of cases run against both, as for other DB-001 code.
- A read-model schema change (the stored inputs) needs a version bump and `myfamily rebuild`
  under ADR-006.

### Neutral

- Editors and owners see everything, so mutation responses and editor-only operations need no
  redaction; they still pass through the choke points, so a lower role granted later is covered.
- Free text an editor writes outside tree records (branch and snapshot names and descriptions)
  is not redacted.
- The living override has no GEDCOM tag and is not exported.

## Read-surface table

A snapshot of `internal/api/openapi.yaml` as of 2026-11-18 (see §7). Paths are under `/api/v1`.
**Source:** RM = read model, RM agg = aggregate computed in the
store, RM blob = file bytes held in the read model, EL = event log. **Choke point:** a =
decorator, b = event-output redactor, — = no tree data. "Protected" means a placeholder person;
"hidden" means omitted or 404.

### Persons

| Operation | Path | Returns | Source | Choke | Special handling |
|-----------|------|---------|--------|-------|------------------|
| listPersons | GET /persons | Person list (also the home page's recent people) | RM | a | Protected excluded in the query; total is the filtered count |
| getPerson | GET /persons/{id} | Person detail | RM | a | Protected → placeholder; private → 404 |
| getPersonNames | GET /persons/{id}/names | Name variants | RM | a | Protected → empty |
| getCitationsForPerson | GET /persons/{id}/citations | Citations | RM | a | Protected → empty; private citations dropped |
| listPersonMedia | GET /persons/{id}/media | Media metadata | RM | a | Protected → empty; private media dropped |
| listAssociationsForPerson | GET /persons/{id}/associations | Associations | RM | a | Protected → empty; drop when the other end is protected |
| listLDSOrdinancesForPerson | GET /persons/{id}/lds-ordinances | LDS ordinances | RM | a | Protected → empty |
| getPersonQuality | GET /quality/persons/{id} | Completeness score | RM | a | Protected or private → 404 |
| getPersonHistory | GET /persons/{id}/history | Change entries | EL | b | Protected → empty; private → hidden. Anonymous: refused (sign-in required) |
| getPersonRestorePoints | GET /persons/{id}/restore-points | Restore points | EL (+RM existence) | b | Protected → empty; private → hidden. Anonymous: refused (sign-in required) |

### Families

| Operation | Path | Returns | Source | Choke | Special handling |
|-----------|------|---------|--------|-------|------------------|
| listFamilies | GET /families | Family list (also home page) | RM | a | Partners as placeholders; private families excluded in the query |
| getFamily | GET /families/{id} | Family detail | RM | a | Protected partner → placeholder and family facts hidden; private → 404 |
| getFamilyGroupSheet | GET /families/{id}/group-sheet | Group sheet with members' events and sources | RM | a | Protected members as placeholders with no events or sources |
| listLDSOrdinancesForFamily | GET /families/{id}/lds-ordinances | LDS ordinances | RM | a | Hidden when a partner is protected |
| getFamilyHistory | GET /families/{id}/history | Change entries | EL | b | Hidden when private or a partner is protected. Anonymous: refused (sign-in required) |
| getFamilyRestorePoints | GET /families/{id}/restore-points | Restore points | EL (+RM existence) | b | As getFamilyHistory. Anonymous: refused (sign-in required) |

### Charts and relationships

| Operation | Path | Returns | Source | Choke | Special handling |
|-----------|------|---------|--------|-------|------------------|
| getPedigree | GET /pedigree/{id} | Ancestor tree | RM | a | Placeholders keep IDs and links and are marked redacted; a private family's parentage is a gap |
| getDescendancy | GET /descendancy/{id} | Descendant tree | RM | a | As getPedigree |
| getAhnentafel | GET /ahnentafel/{id} | Numbered ancestor report | RM | a | Numbering kept; placeholder entries carry no facts and are marked redacted |
| getRelationship | GET /relationship/{personId1}/{personId2} | Paths, labels, summary | RM | a | Path nodes and summary built from placeholder names; no path through a private family |

### Search, browse and map

| Operation | Path | Returns | Source | Choke | Special handling |
|-----------|------|---------|--------|-------|------------------|
| searchPersons | GET /search | Matching people | RM | a | Protected never match, including name variants, place and date filters, fuzzy and soundex |
| browseSurnames | GET /browse/surnames | Surname index with counts | RM agg | a | Counts exclude protected; a surname with only protected people is absent |
| getPersonsBySurname | GET /browse/surnames/{surname}/persons | People | RM | a | Protected excluded in the query |
| browsePlaces | GET /browse/places | Place hierarchy with counts | RM agg | a | Counts exclude protected |
| getPersonsByPlace | GET /browse/places/{place}/persons | People | RM | a | Protected excluded in the query |
| browseCemeteries | GET /browse/cemeteries | Cemetery index with counts | RM agg | a | Counts exclude protected |
| getPersonsByCemetery | GET /browse/cemeteries/{place}/persons | People | RM | a | Protected excluded in the query |
| getMapLocations | GET /map/locations | Pins with counts and people | RM agg | a | Protected removed from pins, counts recomputed, empty pins dropped |
| getBrickWalls | GET /browse/brick-walls | Brick-wall people | RM agg | a | Protected excluded |

### Sources, citations and templates

| Operation | Path | Returns | Source | Choke | Special handling |
|-----------|------|---------|--------|-------|------------------|
| listSources | GET /sources | Sources with citation counts | RM | a | Private excluded in the query; counts exclude hidden citations |
| searchSources | GET /sources/search | Sources | RM | a | Private never match |
| getSource | GET /sources/{id} | Source detail | RM | a | Private → 404 |
| getCitationsForSource | GET /sources/{id}/citations | Citations | RM | a | Drop private citations and those on a protected person's facts |
| getCitation | GET /citations/{id} | Citation | RM | a | 404 when private or attached to a protected person |
| formatCitation | GET /citations/{id}/format | Formatted citation text | RM | a | As getCitation |
| getSourceHistory | GET /sources/{id}/history | Change entries | EL | b | Private → empty. Anonymous: refused (sign-in required) |
| getSourceRestorePoints | GET /sources/{id}/restore-points | Restore points | EL (+RM existence) | b | Private → empty. Anonymous: refused (sign-in required) |
| getCitationRestorePoints | GET /citations/{id}/restore-points | Restore points | EL (+RM existence) | b | As getCitation. Anonymous: refused (sign-in required) |
| listCitationTemplates | GET /citation-templates | Static templates | none | — | |
| getCitationTemplate | GET /citation-templates/{id} | Static template | none | — | |
| previewCitationTemplate | POST /citation-templates/{id}/preview | Formats the request's own fields | none | — | |

### Media

| Operation | Path | Returns | Source | Choke | Special handling |
|-----------|------|---------|--------|-------|------------------|
| getMedia | GET /media/{id} | Media metadata | RM | a | 404 when private or the owner is hidden or protected |
| downloadMedia | GET /media/{id}/content | File bytes | RM blob | a | Owner and restriction checked before bytes are read |
| getMediaThumbnail | GET /media/{id}/thumbnail | Thumbnail bytes | RM blob | a | Store method returns bytes only: load the media row and check first |

### Notes, submitters, repositories

| Operation | Path | Returns | Source | Choke | Special handling |
|-----------|------|---------|--------|-------|------------------|
| listNotes | GET /notes | Shared notes | RM | a | Private excluded in the query; a shared note is hidden when every record referring to it is protected or private (§3) |
| getNote | GET /notes/{id} | Note | RM | a | Private → 404 |
| listSubmitters | GET /submitters | Submitters (name, address, contact) | RM | a | Living researchers' contact details: hidden from anonymous, shown to viewers and up (§4) |
| getSubmitter | GET /submitters/{id} | Submitter | RM | a | As listSubmitters |
| listRepositories | GET /repositories | Archives and libraries | RM | a | No person data; no restriction field |
| getRepository | GET /repositories/{id} | Repository | RM | a | As listRepositories |

### Associations and LDS ordinances

| Operation | Path | Returns | Source | Choke | Special handling |
|-----------|------|---------|--------|-------|------------------|
| listAssociations | GET /associations | Associations | RM | a | Drop when either end is protected |
| getAssociation | GET /associations/{id} | Association | RM | a | 404 when either end is protected |
| listLDSOrdinances | GET /lds-ordinances | Ordinances | RM | a | Drop when the person or a family partner is protected |
| getLDSOrdinance | GET /lds-ordinances/{id} | Ordinance | RM | a | As listLDSOrdinances, 404 |

### Evidence and research (GPS)

| Operation | Path | Returns | Source | Choke | Special handling |
|-----------|------|---------|--------|-------|------------------|
| listEvidenceAnalyses | GET /evidence-analyses | Analyses | RM | a | Drop when the subject is protected or private |
| getEvidenceAnalysis | GET /evidence-analyses/{id} | Analysis | RM | a | As above, 404 |
| getAnalysesByFact | GET /evidence-analyses/by-fact | Analyses | RM | a | As above |
| listEvidenceConflicts | GET /evidence-conflicts | Conflicts with values | RM | a | As above |
| getEvidenceConflict | GET /evidence-conflicts/{id} | Conflict | RM | a | As above, 404 |
| getConflictsBySubject | GET /evidence-conflicts/by-subject/{subjectId} | Conflicts | RM | a | Protected or private subject → empty |
| listResearchLogs | GET /research-logs | Research log entries | RM | a | Drop when the subject is protected or private |
| getResearchLog | GET /research-logs/{id} | Entry | RM | a | As above, 404 |
| getResearchLogsBySubject | GET /research-logs/by-subject/{subjectId} | Entries | RM | a | Protected or private subject → empty |
| listProofSummaries | GET /proof-summaries | Proof summaries | RM | a | Drop when the subject is protected or private |
| getProofSummary | GET /proof-summaries/{id} | Proof summary | RM | a | As above, 404 |
| getProofSummaryByFact | GET /proof-summaries/by-fact | Proof summary | RM | a | As above |

### Quality, statistics and home widgets

| Operation | Path | Returns | Source | Choke | Special handling |
|-----------|------|---------|--------|-------|------------------|
| getQualityOverview | GET /quality/overview | Totals, top issues, lowest-scoring people | RM | a | Lowest-scoring list excludes protected; totals may count them |
| getQualityReport | GET /quality/report | Per-person report | RM | a | Protected excluded |
| getValidationIssues | GET /quality/validation | Issues naming records | RM | a | Issues about protected or private records dropped |
| getStatistics | GET /statistics | Totals, birth date range, top surnames, gender split | RM | a | Totals may count protected; date range and top surnames exclude them |
| getDiscoveryFeed | GET /analytics/discovery | Suggestions naming people (home page) | RM | a | Suggestions about protected or private records dropped |
| getPersonsDuplicates | GET /persons/duplicates | Candidate pairs | RM | a | Pairs with a protected person dropped |
| listDismissedDuplicates | GET /persons/duplicates/dismissed | Dismissed pairs | RM | a | As getPersonsDuplicates |

### Export (editor and owner under ADR-007)

| Operation | Path | Returns | Source | Choke | Special handling |
|-----------|------|---------|--------|-------|------------------|
| exportGedcom | GET /gedcom/export | GEDCOM file | RM | a | Writes `RESN`; a protected or private person an exported family links to is written as the placeholder (an `INDI` with the name "Living", the sex and the family links); side tables follow owner protection |
| previewGedcomExport | GET /gedcom/export/preview | GEDCOM preview | RM | a | As exportGedcom |
| exportTree | GET /export/tree | JSON tree | RM | a | Side tables follow owner protection |
| exportPersons | GET /export/persons | CSV/JSON people | RM | a | As exportTree |
| exportFamilies | GET /export/families | CSV/JSON families | RM | a | As exportTree |
| exportSources | GET /export/sources | CSV/JSON sources | RM | a | Private dropped |
| exportCitations | GET /export/citations | CSV/JSON citations | RM | a | As getCitationsForSource |
| exportEvents | GET /export/events | CSV/JSON events | RM | a | Events of protected owners dropped (separate store method) |
| exportAttributes | GET /export/attributes | CSV/JSON attributes | RM | a | As exportEvents |
| getExportEstimate | GET /export/estimate | Counts | RM agg | a | Counts match what the export would emit |

### History and snapshots

| Operation | Path | Returns | Source | Choke | Special handling |
|-----------|------|---------|--------|-------|------------------|
| listHistory | GET /history | Global change log | EL | b | Entries for protected or private entities dropped; names blanked. Anonymous: refused (sign-in required) (§4) |
| listSnapshots | GET /snapshots | Snapshot names and positions | snapshot registry | — | Editor-written free text, not redacted. Anonymous: refused (sign-in required) |
| getSnapshot | GET /snapshots/{id} | Snapshot | snapshot registry | — | As listSnapshots. Anonymous: refused (sign-in required) |
| compareSnapshotToCurrent | GET /snapshots/{id}/compare-current | Change entries | EL | b | As listHistory. Anonymous: refused (sign-in required) |
| compareSnapshots | GET /snapshots/{id1}/compare/{id2} | Change entries | EL | b | As listHistory. Anonymous: refused (sign-in required) |

### Branches

| Operation | Path | Returns | Source | Choke | Special handling |
|-----------|------|---------|--------|-------|------------------|
| listBranches | GET /branches | Branch metadata, drift counts, linked proof summaries | branch store + RM | a | Linked proof summaries redacted by the decorator; names and descriptions are free text |
| getBranch | GET /branches/{id} | As above | branch store + RM | a | As listBranches |
| getBranchDrift | GET /branches/{id}/drift | Event counts | EL (counts) | — | No record content |
| compareBranch | GET /branches/{id}/compare | Branch and main change entries, and the merge record of a merged branch | EL | b | Protected if protected on either side. Anonymous: refused (sign-in required) (§4) |
| getBranchResearch | GET /branches/{id}/research | Research archive rebuilt from branch events | EL | b | Overlay may be purged: unresolved subjects treated as protected |
| getBranchEvidenceCoverage | GET /branches/{id}/evidence-coverage | Changed facts without evidence | EL + RM | b | Protected if protected on either side. Anonymous: refused (sign-in required) (§4) |
| getBranchHealth | GET /branches/{id}/health | Validation and quality on the branch overlay | RM | a | As getValidationIssues. Anonymous: refused (sign-in required) (§4) |
| precheckBranchMerge | POST /branches/{id}/merge/precheck | Merge plan and conflict values | EL | b | Editor-only; redactor is identity for editors |
| mergeBranch | POST /branches/{id}/merge | Merge result and conflicts | EL | b | As precheckBranchMerge |
| resumeBranchMerge | POST /branches/{id}/merge/resume | Merge result and conflicts | EL | b | As precheckBranchMerge |

### Rollback (owner under ADR-007)

| Operation | Path | Returns | Source | Choke | Special handling |
|-----------|------|---------|--------|-------|------------------|
| rollbackPerson | POST /persons/{id}/rollback | Rolled-back changes | EL | b | Owner-only; redactor is identity. Across a person merge it keeps the more protective of the current and target privacy marks, as the merge does (#1447) |
| rollbackFamily | POST /families/{id}/rollback | As above | EL | b | As rollbackPerson |
| rollbackSource | POST /sources/{id}/rollback | As above | EL | b | As rollbackPerson |
| rollbackCitation | POST /citations/{id}/rollback | As above | EL | b | As rollbackPerson |

### Settings and accounts

Added after this ADR's date, when #1412 made the table a test (§7).

| Operation | Path | Returns | Source | Choke | Special handling |
|-----------|------|---------|--------|-------|------------------|
| getHomePerson | GET /settings/home-person | The account's home person (the site's in open mode) | settings + RM | a | Person read like getPerson: protected → placeholder; private or gone → none. Anonymous has no home person, so none (#1443) |
| setHomePerson | PUT /settings/home-person | The new home person | settings + RM | a | As getHomePerson; a private person the caller cannot see is 404. Any signed-in account; anonymous refused (401) |
| clearHomePerson | DELETE /settings/home-person | Empty body | settings | — | No tree data. Any signed-in account; anonymous refused (401) |
| getPrivacySettings | GET /settings/privacy | Tree privacy settings | settings | — | No tree data |
| getAuthStatus | GET /auth/status | Mode, setup state, the caller's account | accounts | — | No tree data |
| setupOwner | POST /auth/setup | The new owner's account | accounts | — | No tree data |
| signIn | POST /auth/sign-in | The caller's account | accounts | — | No tree data |
| signOut | POST /auth/sign-out | Empty | accounts | — | No tree data |
| getCurrentAccount | GET /auth/me | The caller's account | accounts | — | No tree data |
| updateCurrentAccount | PATCH /auth/me | The caller's account | accounts | — | No tree data |
| acceptInvite | POST /auth/invites/accept | The new account | accounts | — | No tree data |
| resetPassword | POST /auth/password-reset | The caller's account | accounts | — | No tree data |
| listUsers | GET /users | Accounts and roles | accounts | — | Owner-only; no tree data |
| listAuthAudit | GET /auth-audit | Sign-in and account events | accounts | — | Owner-only; no tree data |
| listInvites | GET /invites | Pending invite links | accounts | — | Owner-only; no tree data |
| listPasswordResets | GET /password-resets | Pending password reset links | accounts | — | Owner-only; no tree data |

### Writes and operations outside the spec

| Operation | Path | Returns | Source | Choke | Special handling |
|-----------|------|---------|--------|-------|------------------|
| Every create, update, delete, add, remove, merge, dismiss, restore, brick-wall, close, promote, resolve and batch operation | POST, PUT, PATCH, DELETE | The written record or an empty body | RM | a | Editor or owner only, so nothing is redacted; still wired through the decorator |
| importGedcom | POST /gedcom/import | Import counts and warnings | — | — | Owner-only |
| (not in spec) | POST /gedcom/import/stream | Import progress | — | — | Owner-only; must get a role declaration under ADR-007 |
| (not in spec) | GET /health, GET /config, GET /openapi.yaml, GET /docs | Server status, config flags, API description | none | — | No tree data |
| (not in spec) | POST /demo/reset | Re-seeds demo data | — | — | Demo mode is open mode |

## References

- [ADR-001](./001-event-sourcing-cqrs.md) - event sourcing; the log the redactor reads
- [ADR-002](./002-dual-database-strategy.md) - both backends implement the SQL form of the rule
- [ADR-005](./005-research-branch-data-model.md) - branch overlays, which the rule evaluates on
- [ADR-006](./006-schema-evolution.md) - added event fields and the read-model rebuild
- [ADR-007](./007-access-model.md) - principals, open mode, tree settings
- `internal/repository/readmodel.go` (`ReadModelStore`), `internal/query/history_queries.go`
  (`ChangeEntry`), `internal/query/quality_service.go` (the existing 100-year heuristic)
- [#1411](https://github.com/cacack/my-family/issues/1411) - this spike;
  [#1412](https://github.com/cacack/my-family/issues/1412) - implementation;
  #1408 - settings store; #1414 - anonymous viewing
