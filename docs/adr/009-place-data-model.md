# ADR-009: Place Data Model — Place Records Resolved from Event Text by a Matching Key

**Status:** Accepted
**Date:** 2026-11-22
**Decision Makers:** Project maintainer
**Related Features:** [#1602](https://github.com/cacack/my-family/issues/1602) (this spike),
[#540](https://github.com/cacack/my-family/issues/540) (epic)

## Context

A place is a line of text on each event. `domain.Place` is a value: a name and an optional latitude
and longitude as GEDCOM strings. Nothing connects two events in the same town, and two spellings of
one town are two places.

What the code does today:

- **Text.** Place text is a field on `PersonCreated` (birth, death), `FamilyCreated` (marriage),
  `LifeEventCreated` and `AttributeCreated`, changed afterwards through the `changes` map of the
  matching update event. It projects to `persons.birth_place`, `persons.death_place`,
  `families.marriage_place`, `life_events.place` and `attributes.place`.
- **Coordinates.** A latitude and longitude sit beside each of those five texts, on the same four
  creation events and on `ImportedValuesBackfilled`. Only GEDCOM import sets them.
  [#1604](https://github.com/cacack/my-family/issues/1604) will let the API set them and
  [#1563](https://github.com/cacack/my-family/issues/1563) will record how each pair was obtained.
- **Matching.** `repository.NormalizePlace` trims spaces around commas and drops empty parts. It is
  case-sensitive. SQLite calls it through a registered function, the memory store calls it
  directly, and PostgreSQL has its own SQL expression (`placeNormSQL`) held to it by a parity test.
- **Hierarchy.** The place browser splits the normalized text on commas, right to left, inside
  each backend's SQL. It reads birth and death places only. The memory store returns a flat list
  ([#926](https://github.com/cacack/my-family/issues/926)).
- **Map.** `GET /map/locations` reads birth and death rows that have coordinates and groups them by
  exact place text and event type. The first pair seen in a group places the pin.
- **GEDCOM.** Import takes the name (`PlaceName()`) and `MAP` coordinates. Export writes them back.
  `PLAC.FORM`, the file default `HEAD.PLAC.FORM` and the GEDCOM 7.0 `PLAC.EXID` are decoded by
  gedcom-go (v3.10.0: `PlaceDetail.Form`, `Header.PlaceForm`, `PlaceDetail.ExternalIDs`) and
  ignored by the importer, so none of them round-trips.

Coordinate work is starting against this model (the *Place Coordinates* milestone), and dedup,
historical names ([#1564](https://github.com/cacack/my-family/issues/1564)) and migration maps
([#49](https://github.com/cacack/my-family/issues/49)) all depend on what a place is.

The maintainer set three constraints on the issue before this ADR was written:

1. A place becomes a record of its own. Text plus a matching key with no record is ruled out.
2. The preferred shape keeps each event's place text exactly as recorded and resolves it to a place
   record through a normalized key, with variant spellings as aliases and a per-event coordinate
   override. Events carrying a place ID stay open if the key shape does not hold against branch
   overlays, `myfamily rebuild` or the GEDCOM round trip.
3. There is one matching rule, used everywhere. It ignores letter case and spacing around commas,
   and nothing looser. Variant spellings are aliases the user chooses.

## Decision Drivers

- **What was recorded comes back out** ([ETHOS](../ETHOS.md), "Your Research Is Yours"). A place
  name is evidence. Merging two spellings must not change what any record says.
- **Events are append-only (ES-002).** Existing events carry text and coordinates and no place ID.
  Whatever is decided has to read them as they are.
- **Branches overlay every read model** ([ADR-005](./005-research-branch-data-model.md)), and a
  cross-stream reference is what made branch merges hard there.
- **Three stores must agree (DB-001).** A rule written once in Go and once in SQL drifts.
- **A new entity type is expensive**: 20 items across 7 layers
  ([INTEGRATION-MATRIX](../INTEGRATION-MATRIX.md)), more when it is branch-scoped.
- **Room for later features** without a second migration: dedup, dated names, migration maps.

## Considered Options

### Sub-decision 1 — What is a place?

#### Option 1A: A value on each event with a shared matching key, no record

**Pros:** no new entity.
**Cons:** nowhere to keep a fact about the place itself: coordinates, an alias, a parent, a dated
name. Ruled out by the maintainer.

#### Option 1B: A place entity; events carry a place ID

**Pros:** an exact reference. Two different towns with the same name can be told apart without
changing the text.
**Cons:**
- No stored event has an ID and none may be rewritten, so reads must resolve old events from their
  text for good. The key resolution of Option 1C has to exist anyway, and the ID is a second path
  beside it.
- Every writer has to find or create a place before it can write an event: the forms, the API, and
  GEDCOM import for every `PLAC` line. Finding one is a key lookup again.
- An event on a branch would refer to a place created on that branch. A merge then has to replay
  the place before the event that names it, and refuse when it cannot. ADR-005 records that class
  of dangling reference three times over (family links, citations and sources, media and owners).
- GEDCOM has no place record. An ID cannot be exported or imported, so the round trip runs on the
  text either way.

#### Option 1C: A place entity; events keep their text and resolve to it through a key (chosen)

**Pros:** existing events need nothing. Writers keep writing text. A reference cannot dangle,
because every text has a key and every key has a place. Nothing crosses streams at merge time.
**Cons:** two different places recorded with identical text are one place until the user makes one
text more specific. GEDCOM has the same limit, since its only place identity is the text.

### Sub-decision 2 — How does a place record come to exist, and what identifies it?

#### Option 2A: An explicit `PlaceCreated` with a random ID on first use

**Pros:** the usual entity shape.
**Cons:** uniqueness of a key across streams cannot be enforced by an event store, so two
concurrent writers, or `main` and a branch, can create two places for one key. An import appends
one event per distinct place. Existing data needs a backfill event per place at rebuild.

#### Option 2B: A place exists once its key is in use; its ID is derived from the key (chosen)

**Pros:** no creation event, so no backfill and no race: the ID is a function of the key, the same
on every branch, after every rebuild and in every install. Events are appended only when someone
states a fact about the place.
**Cons:** the matching rule is frozen once place events exist, because their stream IDs depend on
it. Departs from the `XCreated`/`XDeleted` pattern.

### Sub-decision 3 — How is hierarchy represented?

#### Option 3A: Split the text on commas at read time, in each backend's SQL (today)

**Cons:** three implementations to keep in step; no place to correct a wrong level; no ID for a
level.

#### Option 3B: Jurisdiction levels from `PLAC.FORM`

**Pros:** uses what the file says.
**Cons:** most files give no form, hand-entered places never do, and a part's position says
nothing reliable about its jurisdiction (cacack/gedcom-go#135). A model built on labels would be
empty for most data.

#### Option 3C: Each place has one parent place; the default parent comes from the name (chosen)

**Pros:** containment is the one thing position does give: GEDCOM lists jurisdictions smallest to
largest. Every level is a place with an ID. The user can correct a parent. Labels stay optional.
**Cons:** a place is contained in one parent at a time; a jurisdiction that changed over time needs
the dated extension noted under *Room for later features*.

### Sub-decision 4 — Where do coordinates live?

#### Option 4A: On each event only (today)

**Cons:** one town, a hundred copies; fixing a pin means editing every record
([#1606](https://github.com/cacack/my-family/issues/1606) exists to work around this).

#### Option 4B: On the place only

**Cons:** existing events carry their own pairs and cannot be rewritten, and an event can
legitimately be more exact than its place (a farm recorded under its parish).

#### Option 4C: On the place, with an optional pair on the event that overrides it (chosen)

**Pros:** existing pairs are already valid overrides, so nothing migrates. One edit maps a place.
**Cons:** two sources for one answer; reads must say which one they used.

### Sub-decision 5 — Are facts about a place branch-scoped?

#### Option 5A: Branch-scoped, like persons and families

**Cons:** a `branch_id`, composite key, tombstones, overlay resolution on three backends and merge
and conflict rules, for an entity whose facts describe the world and not a hypothesis.

#### Option 5B: Main-only (chosen)

**Pros:** the same reasoning ADR-005 applies to repositories. What a branch does fork, which place
a record names, is the event's own text, and that is branch-scoped already.
**Cons:** a place cannot be given trial coordinates or a trial alias on a branch.

## Decision

We chose **1C + 2B + 3C + 4C + 5B**. The key-linked shape holds against the three checks the
maintainer named; the findings are in sections 6, 7 and 8.

### 1. The matching key

One function, `PlaceKey`, turns place text into its key. It is the only place-matching rule in the
project and replaces `NormalizePlace` and `placeNormSQL`.

1. Split the text on `,`.
2. Trim Unicode white space from both ends of each part.
3. Fold letter case code point by code point: Unicode simple case folding (the orbit of
   `unicode.SimpleFold`), with no locale and no full folding, so `ß` does not become `ss`.
4. Join the parts with `,`.

Text whose parts are all empty has no key and no place.

| Recorded text | Key | |
|---|---|---|
| `Boston, MA` | `boston,ma` | |
| ` BOSTON ,ma ` | `boston,ma` | same place |
| `Boston,MA` | `boston,ma` | same place |
| `Boston MA` | `boston ma` | different: no comma |
| `New  York, NY` (two spaces) | `new  york,ny` | different from `New York, NY` |
| `Köln, Germany` | `köln,germany` | different from `Koln, Germany` |
| `Straße 1, Wien` | `straße 1,wien` | different from `Strasse 1, Wien` |
| `Darmstadt,,,Germany` | `darmstadt,,,germany` | different from `Darmstadt, Germany` |

Nothing looser is applied: no accent folding, no Unicode normalization, no collapsing of inner
spaces, no abbreviations. Those are variant spellings, and the user joins them with an alias.

**Empty parts are kept.** `NormalizePlace` drops them today (#925). The key does not, for two
reasons. An empty part holds a position in GEDCOM (`, , Utah, USA` leaves the two smallest
jurisdictions unknown), and a place's `FORM` labels pair with its parts by position, so two texts
with one key must have the same number of parts. The browser still never shows an empty level
(section 4).

**The key is computed in Go and stored.** Each of the five place-bearing rows gains a key column
beside its text, filled by the projector. No SQL statement implements the rule, so the stores
cannot disagree on it and case folding does not depend on a database's collation.

### 2. The place record

A place exists as soon as its key is in use: by an event's text, as the parent of a place in use,
or by an event on its own stream. Its ID is `UUIDv5(namespace, key)`, with one fixed namespace UUID
defined beside `PlaceKey`.

A place carries, all optional:

- a **name** to display. Without one, the place shows its most-used recorded spelling (ties broken
  bytewise);
- **coordinates**, with the origin vocabulary of #1563;
- a **form**: the `PLAC.FORM` labels for its parts;
- a **parent** place, overriding the default of section 4;
- **external IDs** (GEDCOM 7.0 `EXID`, such as a gazetteer entry).

These facts are event-sourced on the place's own stream (stream ID = place ID, stream type
`place`):

- `PlaceUpdated` sets or clears facts through a `changes` map, as `PersonUpdated` does.
- `PlaceAliased` and `PlaceUnaliased` (section 3).

Every place event carries the place's key and the text it was computed from, so the log can be read
without the hash. There is no `PlaceCreated` or `PlaceDeleted`: existence follows from use, and
removing a fact is clearing it. This is a stated departure from item 5 of the entity checklist; the
other items apply (`DecodeEvent`, projection handler, history catalog, both SQL stores, API).

**Read model.** Two kinds of data, kept apart:

- *Facts*: tables `places` (ID, key, name, coordinates and origin, form, parent, version),
  `place_aliases` (alias key to place ID) and `place_external_ids`. They store their own rows.
- *Use*: which places are in use, and by how many records, is a view over the key columns of the
  five place-bearing rows. It stores nothing.

The list of places is the union of the two. Resolving a key is one step:
`place_aliases[key]` if present, otherwise `UUIDv5(namespace, key)`.

### 3. Variant spellings: aliases

An alias says that one key means another place. `PlaceAliased{place, target}` is appended to the
stream of the place that is being absorbed. From then on its key resolves to the target, and every
event recorded under either spelling belongs to one place. `PlaceUnaliased` on the same stream
undoes it.

- **No event text changes.** Each record keeps the spelling it was given, in the UI, in history and
  in export.
- **Aliases are one level deep.** The command rejects a target that is itself an alias, and
  re-points the aliases of a place that becomes one. Resolution never follows a chain.
- **Facts on an absorbed place are kept and ignored** while the alias stands; the target's facts
  apply. The merge screen shows both so the user can copy one across first.
- **Only the whole key matches.** Aliasing `Hesse, Germany` to `Hessen, Germany` does not rewrite
  `Darmstadt, Hesse, Germany`. That place moves under Hessen in the browser, beside
  `Darmstadt, Hessen, Germany`, where the user can join the two.

### 4. Hierarchy

A place has at most one parent place.

- **Default parent:** the place whose key is this key with its leftmost part removed, then any
  leading empty parts removed, resolved through aliases. `darmstadt,hessen,germany` is in
  `hessen,germany`, which is in `germany`. `darmstadt,,,germany` is in `germany`. A one-part key
  has no parent.
- **Explicit parent:** a `PlaceUpdated` may set one. The command rejects a cycle.
- **Jurisdiction label:** optional per level. It comes from the place's form when it has one, and
  is never inferred from position. A level with no label shows none.

The browser walks parents, in Go, over places resolved by key. A parent that no record names
directly (`Hessen, Germany` when only its towns were recorded) is a place like any other. A place's
count is the records that resolve to it or to a place beneath it. The right-to-left SQL in the two
SQL stores and the flat list in the memory store are replaced by this one walk.

### 5. Coordinates

The place's coordinates are the default; an event's own pair overrides them.

> **Effective coordinates of a record** = its own pair when both halves are set, otherwise its
> place's pair, otherwise none.

- The existing per-record fields (`birth_place_latitude` and the rest) keep meaning *the record's
  own pair*. Responses add the resolved place (ID, name, its pair) beside them, so a form can show
  an inherited pair without saving it back as an override.
- Each pair has its own origin: the record's is the one #1563 defines, and the place's uses the
  same vocabulary.
- A parent's coordinates are not inherited. A town with no pair is unmapped, not drawn at its
  country.
- Coordinates on other records that share the place are suggestions (#1561), never applied
  silently.

### 6. Branches

**Finding: the key shape holds.** An event on a branch carries text, as on `main`. Its shadow row
gets a key column like any other row, so the *use* view inherits the overlay and needs no
`branch_id` of its own. This is the rule ADR-005 states for the browse and map aggregates. A place
named only on a branch is in use on that branch alone, with the same ID it will have after a merge.
A merge replays person, family and event edits and has nothing place-shaped to order or reconcile.

**Place facts are main-only.** `places`, `place_aliases` and `place_external_ids` have no
`branch_id`, and `PlaceUpdated`, `PlaceAliased` and `PlaceUnaliased` are not on the BR-006
allowlist. A branch reads `main`'s facts live. This adds Place to the decided main-only set of
ADR-005, which is amended to say so. The hypothesis a branch can fork is "this record means that
place", and that is the record's place text. As in ADR-005, the choice is reversible.

### 7. Migration and `myfamily rebuild`

**No event is rewritten and none is appended.**

- **Place text** on existing events is read as it is. The projector computes each key.
- **Coordinates** on existing events (the four creation events and `ImportedValuesBackfilled`) stay
  where they are and are read as the record's own pair, which is exactly what an override is.
- **Pairs entered through #1604** arrive as change keys on `PersonUpdated` and `FamilyUpdated` and
  project to the same columns. They are the record's own pair too.
- **Origins from #1563** describe the record's pair and are unchanged: a pair stored before #1563
  still reads as `imported`, one written through #1604 as `manual`.
- **Place records** need no migration. Each place in use exists by derivation, with no facts.
  Nothing promotes a record's pair to its place; the user does that from the worklist.

**Result for `myfamily rebuild`.** The key columns and the three fact tables change the read-model
schema, so `ReadModelSchemaVersion` is bumped and every install rebuilds once
([ADR-006](./006-schema-evolution.md)). The rebuild replays the log and appends nothing for
places. Afterwards every place-bearing row has its key, every record has the coordinates it had
before, the map shows what it showed before, and each place has the ID it will always have.
Rebuilding again changes nothing. Once place events exist, their streams are replayed like any
other and the fact tables come back from them.

One visible change comes with the first rebuild: the browser joins spellings that differ only in
letter case, and stops joining texts that differ only in empty parts.

### 8. GEDCOM round trip

- **`PLAC`.** The event's text, verbatim, both ways. Export never substitutes a place's display
  name or an alias target.
- **`MAP`.** Import stores `LATI`/`LONG` as the record's own pair, as today. Export writes the
  record's *effective* pair. GEDCOM has no shared place record, so a pair set once on a place is
  written under every event that inherits it; leaving it out would lose it. Re-importing that file
  gives each record its own pair and the same map.
- **`PLAC.FORM`.** Import stores a form on the place: the `PLAC`'s own `FORM`, otherwise the
  file's `HEAD.PLAC.FORM`. A form is set on a place that has none; a different form for a place
  that already has one is reported as an import warning and not applied. Export writes `PLAC.FORM`
  under each event whose place has a form.
- **`HEAD.PLAC.FORM`.** Not written yet. The file default comes back as a `FORM` under each place,
  which means the same and is not the same bytes. Writing the most common form once in the header,
  and a per-place `FORM` only where it differs, needs
  [cacack/gedcom-go#875](https://github.com/cacack/gedcom-go/issues/875): the encoder does not
  write `Header.PlaceForm` for a document assembled in memory.
- **`PLAC.EXID`** (GEDCOM 7.0). Import stores them on the place; export writes them under each of
  its events when the file being written is a version that has `EXID`.

**Form-labelled parts from gedcom-go: needed for labels only.** Keys, aliases, containment,
coordinates and the round trip above need nothing new upstream. Showing a jurisdiction label
(section 4) needs each part paired with its label, with the header default, empty parts and a
count mismatch handled. That is GEDCOM interpretation and belongs in the library, which said in
cacack/gedcom-go#135 that it would take a form-aware accessor. Requested as
[cacack/gedcom-go#874](https://github.com/cacack/gedcom-go/issues/874). my-family does not
implement the pairing itself; labels wait for it.

### 9. What the map and migration maps read

- **`GET /map/locations`** reads every place-bearing record that has effective coordinates,
  through the branch and privacy overlay it uses today, and groups by resolved place, event type
  and effective pair. Each location carries the place ID and name. Two spellings of one place
  become one pin, and a record with its own, more exact pair gets its own. Which event types are
  read is still [#1303](https://github.com/cacack/my-family/issues/1303)'s question.
- **Migration maps** ([#49](https://github.com/cacack/my-family/issues/49), fed by
  [#1623](https://github.com/cacack/my-family/issues/1623)). A stop is a dated record with
  effective coordinates. It carries its resolved place ID, so two consecutive stops at one place,
  however spelled, are recognizably no move, and a place mapped once gives every ancestor recorded
  there a stop.
- **The no-coordinates worklist** (#1561) lists places with at least one record that has no
  effective coordinates. Setting the place's pair clears all of them with one event.

### 10. Privacy

The *use* view reads through the privacy overlay of [ADR-008](./008-privacy-model.md), as the place
browser does. A place whose only uses are hidden from a principal is absent for that principal,
facts included, so a name or a pair cannot leak through the fact tables. The read-surface table of
ADR-008 gains a row for each place operation in the PR that adds it.

### 11. Room for later features

- **Dedup** is an alias. Suggesting candidates (keys that differ by accents, abbreviations or
  edit distance) is a read over the place list and needs no model change.
- **Dated historical names** (#1564): a place gains a list of names, each with an optional date
  range and language, and each name's key becomes an alias. Event text and export are untouched.
- **Dated parents**, for a town whose county or country changed: the parent of section 4 gains a
  date range the same way.
- **Gazetteers**: an external ID on the place is the link.
- **Aliasing a whole subtree** by its suffix was left out to keep resolution to one lookup. It can
  be added as a second kind of alias.

## Consequences

### Positive

- No stored event changes meaning and none is touched. Existing coordinates keep working as
  overrides.
- One matching rule, written once. `placeNormSQL` and its parity test go away.
- A place is mapped, renamed or merged with one event, where #1606 needed one per record.
- Every hierarchy level has an ID, and the memory store gets the same browser as the SQL stores.
- Branch merges gain no new cross-stream dependency.
- Aliases are reversible, and the record's spelling is never lost.

### Negative

- The matching rule is frozen. Changing it changes place IDs and strands the streams of places
  that have facts.
  - Mitigation: place events carry their key and text, so a later ADR can re-key them; test vectors
    for `PlaceKey` are pinned with its implementation.
- Two different places recorded with the same text are one place.
  - Mitigation: make one text more specific. A per-record place override can be added later
    without disturbing the key path.
- Texts that differ only in empty parts stop being joined automatically, which #925 did.
  - Mitigation: an alias, and a dedup suggestion for exactly that case.
- Every install rebuilds once (ADR-006).
- Export repeats a place's coordinates and form under each event, and a `HEAD.PLAC.FORM` default
  comes back as per-place lines until cacack/gedcom-go#875.
- Place facts cannot be tried on a branch.
- Jurisdiction labels wait for cacack/gedcom-go#874.

### Neutral

- Place streams are ordinary `main` streams, so history, snapshots and rollback see them as they
  see any other main-only entity.
- LDS ordinance place text and `PLAC.NOTE` are outside this decision. Neither is in the browser or
  on the map today, and `PLAC.NOTE` is not imported.
- The *Place Coordinates* work in flight is compatible: what it stores per record is the override
  half of this model. Its effect on each open issue is listed below.

## New Invariants

A **Place (PL)** category, added to
[ARCHITECTURAL-INVARIANTS.md](../ARCHITECTURAL-INVARIANTS.md) with its verification methods by the
PRs that make each one enforceable:

- **PL-001** Place text on an event is never rewritten by a place operation.
- **PL-002** `PlaceKey` is the only place-matching rule and no SQL implements it.
- **PL-003** A place's ID is `UUIDv5(namespace, key)`; the rule and the namespace do not change
  without an ADR.
- **PL-004** An alias target is never itself an alias.
- **PL-005** Place events are main-only (not on the BR-006 allowlist).

## Implementation Plan

To be filed in the *Places & Geography* milestone and added to #540 once this ADR is approved. Each
is one PR.

1. **Key.** `PlaceKey`, key columns on the five rows in all three stores, schema version bump;
   the browser and persons-by-place read the key. Removes `NormalizePlace` and `placeNormSQL`.
2. **Place list.** Derived places with IDs and use counts; `GET` operations for the list and one
   place; privacy rows.
3. **Place facts.** `PlaceUpdated`, the `places` table, name and coordinates with origin; effective
   coordinates on person, family, life-event and attribute reads and on `GET /map/locations`.
4. **Aliases.** `PlaceAliased`/`PlaceUnaliased`, resolution, and a screen to join two places.
5. **Hierarchy.** Parent walk in Go on all three stores, explicit parent, browser on place IDs.
6. **GEDCOM.** Import `PLAC.FORM`, the header default and `EXID` onto the place; export the
   effective `MAP`, `FORM` and `EXID`; a round-trip test.
7. **Header form on export**, after cacack/gedcom-go#875.
8. **Jurisdiction labels** in the browser, after cacack/gedcom-go#874.
9. **Dedup suggestions** on the place list.

### Open children of #540 under this decision

| Issue | Verdict |
|---|---|
| #1604 accept coordinates on updates | Holds. What it writes is the record's own pair. |
| #1033 coordinates in the forms | Holds. Showing the place's inherited pair beside the inputs comes with step 3. |
| #1563 origin of each pair | Holds. The origin describes the record's pair; the place's pair reuses the vocabulary. |
| #1561 no-coordinates worklist | Revise. Matching is `PlaceKey`, so `Boston,MA` and `Boston, MA` are one entry (its second criterion says two). After step 3, "no coordinates" means no effective coordinates. |
| #1606 apply a pair to every record | Superseded by step 3. One event on the place replaces one per record, so its per-record history criterion no longer applies. Hold until this ADR is approved. |
| #1562 geocoder endpoint | Holds. It looks up a name and stores nothing. |
| #1607 geocoder candidates | Holds, except that a pick in the worklist is saved on the place (step 3), not through #1606. |
| #1609 queued lookups | Holds. |
| #52 historical place context | Holds. Dated names and external IDs have a home on the place record. |
| #1564 gazetteer spike | Holds. Its follow-up issues should build on the place record. |
| #926 memory store hierarchy | Superseded by step 5, which gives all three stores one walk. Worth doing first only if step 5 is far off. |
| #927 hierarchy parity cases | Holds in intent. Its fixtures move to the key-based hierarchy of steps 1 and 5, and the multibyte case should cover case folding. |

## References

- [ADR-001](./001-event-sourcing-cqrs.md) - append-only events (ES-002)
- [ADR-005](./005-research-branch-data-model.md) - branch overlays, derived aggregates, the
  main-only set
- [ADR-006](./006-schema-evolution.md) - schema version and rebuild; added event fields
- [ADR-008](./008-privacy-model.md) - the privacy overlay place reads go through
- [INTEGRATION-MATRIX](../INTEGRATION-MATRIX.md) - the entity checklist
- `internal/domain/place.go`, `internal/repository/place.go` (`NormalizePlace`),
  `GetPlaceHierarchy` and `GetMapLocations` in each store, `internal/gedcom/importer.go`
- [#1602](https://github.com/cacack/my-family/issues/1602) - this spike, with the maintainer's
  direction in its comments; [#540](https://github.com/cacack/my-family/issues/540) - the epic
- cacack/gedcom-go#135 - positional place helpers declined;
  [cacack/gedcom-go#874](https://github.com/cacack/gedcom-go/issues/874) and
  [cacack/gedcom-go#875](https://github.com/cacack/gedcom-go/issues/875) - the two upstream
  requests this decision makes
