# My Family

[![CI](https://github.com/cacack/my-family/actions/workflows/ci.yml/badge.svg)](https://github.com/cacack/my-family/actions/workflows/ci.yml)
[![codecov](https://codecov.io/gh/cacack/my-family/graph/badge.svg)](https://codecov.io/gh/cacack/my-family)
[![Go Report Card](https://goreportcard.com/badge/github.com/cacack/my-family)](https://goreportcard.com/report/github.com/cacack/my-family)
[![Go Version](https://img.shields.io/github/go-mod/go-version/cacack/my-family)](https://github.com/cacack/my-family)
[![License: AGPL-3.0](https://img.shields.io/badge/License-AGPL--3.0-blue.svg)](https://opensource.org/licenses/AGPL-3.0)
[![OpenSSF Scorecard](https://api.scorecard.dev/projects/github.com/cacack/my-family/badge)](https://scorecard.dev/viewer/?uri=github.com/cacack/my-family)

Self-hosted genealogy software written in Go with an embedded Svelte frontend.

## Features

A genealogy platform designed for research rigor and data ownership.

- **GEDCOM 5.5 import/export** - Full round-trip fidelity with your existing data
- **Flexible date handling** - Supports exact, approximate, ranges, and "before/after"
- **Historical calendars** - Parses, displays, and round-trips Julian, Hebrew, and French Republican dates, with optional Gregorian conversion for comparison
- **Family relationships** - Biological, adopted, step, and foster qualifiers
- **Research branches** - Explore an unproven hypothesis in isolation, record the question it tests and the outcome it reached, then review the diff against the mainline and merge it with a per-conflict decision. A branch covers people, families, sources, citations, notes, media, associations, facts, evidence analyses, research logs and proof summaries, and search, browse, the map, the charts and reports all follow it
- **Snapshots, history and rollback** - Tag research milestones and compare them with each other or with the current state, browse the change history of the whole tree or of one record, and roll a person, family, source or citation back to an earlier version
- **Geographic heat map** - Interactive world map showing family locations with zoom/pan
- **Interactive pedigree chart** - D3.js visualization with pan/zoom and keyboard navigation
- **Home person** - Mark who you are in the tree; every person page then names their relationship to you, linking to the full path, and the charts open on you by default. Each account has its own; in open mode there is one for the site
- **Full-text search** - Fast fuzzy matching with keyboard-navigable results
- **Keyboard shortcuts** - Power user navigation (press `?` for help)
- **Accessibility** - High contrast, font scaling, screen reader support
- **API-first design** - Complete REST API with OpenAPI documentation
- **Event sourcing** - Full audit trail of all changes
- **Easy deployment** - Single binary or Docker, SQLite or PostgreSQL

See [FEATURES.md](./FEATURES.md) for the complete list.

## Quick Start

### Using Docker

```bash
# Run with SQLite (default) - data persists in the myfamily_data volume
docker compose up -d

# Access the application
open http://localhost:8080
```

To use PostgreSQL instead, uncomment the `postgres` service and the
`DATABASE_URL` line in `docker-compose.yml`.

### Building from Source

Prerequisites:
- Go 1.26+
- Node.js 24 (`nvm use` reads the repo's `.nvmrc`). The app itself also runs on 22.22.2+ and 26+
  (see `web/package.json` `engines`), but the test suite is held at 24 — see below.

```bash
# Install dependencies
go mod download
npm ci --prefix web

# Build the frontend, stage it for embedding, and build the binary
make binary
./myfamily serve      # stores data in ./myfamily.db
```

The binary embeds the frontend from `internal/web/dist`, so a plain `go build` fails with
`pattern all:dist: no matching files found` until that directory is filled. Without `make`, the
equivalent steps are:

```bash
npm run build --prefix web                  # SvelteKit writes web/build
mkdir -p internal/web/dist
cp -r web/build/* internal/web/dist/        # what `make embed-assets` does
go build -o myfamily ./cmd/myfamily
```

A source build reports `my-family dev (commit: none, built: unknown)` from `./myfamily version`.
Release builds set these values with linker flags; to stamp your own build, pass them to `go build`:

```bash
go build -ldflags "-X main.version=$(git describe --tags --always) \
  -X main.commit=$(git rev-parse --short HEAD) \
  -X main.date=$(date -u +%Y-%m-%dT%H:%M:%SZ)" -o myfamily ./cmd/myfamily
```

**Why Node is pinned to 24.** `.nvmrc`, CI and the Dockerfile all read the same version so they
cannot drift apart. It is held at 24 (Active LTS) rather than 26 because Node 25 enabled the Web
Storage API by default, and its `localStorage`/`sessionStorage` globals shadow the ones jsdom
installs — every storage-touching test fails. This is unresolved upstream
([vitest-dev/vitest#8757](https://github.com/vitest-dev/vitest/issues/8757)) and is not fixed by
newer vitest or jsdom. Running the suite on 25+ needs `NODE_OPTIONS=--no-webstorage`. Revisit the
pin once that lands upstream.

## Configuration

Environment variables:

| Variable | Default | Description |
|----------|---------|-------------|
| `DATABASE_URL` | (none) | PostgreSQL connection string, e.g. `postgres://user:pass@host:5432/myfamily?sslmode=disable`. When set, all data is stored in PostgreSQL |
| `SQLITE_PATH` | `./myfamily.db` | SQLite database file, used when `DATABASE_URL` is unset. Created if missing; its directory must exist |
| `PORT` | `8080` | HTTP server port |
| `LOG_LEVEL` | `info` | Logging level (debug, info, warn, error) |
| `LOG_FORMAT` | `text` | Log format (text, json) |
| `DEMO_MODE` | `false` | Run in memory with a sample family tree and a reset button. Nothing is persisted; overrides `DATABASE_URL` and `SQLITE_PATH`, and implies `AUTH_MODE=open` |
| `AUTH_MODE` | `accounts` | `accounts`: everyone signs in. `open`: every request is treated as the owner, with no sign-in; only for a trusted network. See [Accounts and sign-in](#accounts-and-sign-in) |

### Accounts and sign-in

Every request needs a signed-in account. The first time the server starts on a database with no
owner account, it serves only a setup page and writes a one-time setup token to its log:

```
Setup required: there is no owner account, so the server answers only its setup page. Open the web UI and enter the setup token 3f9c..., or run `myfamily user create`. ...
```

Open the web UI, enter the token, and choose the owner's username, display name and password. Or
create the owner from the command line, with the same `DATABASE_URL` / `SQLITE_PATH` as `serve`
(it works while the server runs):

```bash
myfamily user create --username ada --display-name "Ada Lovelace"   # asks for the password
echo "$PASSWORD" | myfamily user create --username ada                # or reads it from stdin
```

Sessions last 7 days without use and 30 days at most; signing out ends the session on the server.
The session cookie is `HttpOnly` and `SameSite=Lax`, and `Secure` when the request arrived over
TLS (directly, or through a proxy that sets `X-Forwarded-Proto: https`). Repeated failed sign-ins
are refused for 15 minutes: 5 per username, from anyone, and 20 per client address. So anyone who
can reach the server can keep a known username locked out, and behind a reverse proxy every client
shares the proxy's address and so the per-client limit. The API accepts no cross-origin browser
requests: a write whose `Origin` is not the server's host is refused, so a proxy must pass the
original `Host` through or set `X-Forwarded-Host`.

`AUTH_MODE=open` skips sign-in and treats every request as the owner, for a trusted home network,
demo mode and end-to-end tests. It can only be set in the environment, never from the UI, the
server logs a warning when it starts in it, and changes made in it record no user. `DEMO_MODE`
always runs open.

Accounts are kept in their own tables (`users`, `tree_memberships`, `sessions`, `invites`,
`password_resets` and `auth_audit`), not in the event log, so `myfamily rebuild` leaves them alone; back them up with the rest of the database. See
[ADR-007](./docs/adr/007-access-model.md).

#### Roles

Each account holds one role in the tree, and each role can do everything the one before it can:

| Role | Can |
|------|-----|
| Viewer | Read the tree |
| Editor | Change tree data; create, merge and close research branches; export |
| Owner | Manage users; import GEDCOM; roll records back |

The server refuses anything above a user's role with `403 FORBIDDEN`, and the web UI hides what a
role cannot use. Every API operation declares the role it needs as `x-min-role` in the
[OpenAPI spec](./internal/api/openapi.yaml). Each user may start 5 GEDCOM exports or export
previews at once and one more every 10 seconds; beyond that the API answers
`429 RATE_LIMITED`. Open mode treats every request as the owner and does not limit exports.

#### Managing users

There is no self-registration: an owner adds people from **Users** in the account menu. No email
is sent, so each link below is shown once, for the owner to copy and pass on.

- **Invite**: choose a role and create an invite link (`/join#token=...`). It works once, for 7
  days, and creates an account with that role, signed in.
- **Change a role, disable, re-enable**: disabling ends the user's sessions and refuses their
  sign-in until an owner re-enables the account. Disabling or demoting an owner also cancels the
  invite and reset links they created. The tree's last active owner can be neither demoted nor
  disabled.
- **Reset a password**: create a reset link (`/reset-password#token=...`). It works once, for 24
  hours, and setting the password ends all of that user's sessions. From the command line, with
  the same storage settings as `serve`:

  ```bash
  myfamily user reset-password --username ada --url https://tree.example
  ```

- **Revoke a link**: **Users** lists the invite and reset links still pending, with who created
  each and when it expires (never the link itself). Revoking one makes it fail as an expired link
  does.

Each user changes their own display name and password on **Account**; a new password needs the
current one and signs them out everywhere else. Sign-ins, failed sign-ins, invites, revoked
links, role changes, disables, re-enables, password resets and changes, and changes to visitor
access are written to
an append-only audit log, which owners read under **Users > Audit log**.

#### Living people and private records

Each person has a computed `living` flag: a recorded death (including a bare GEDCOM `DEAT Y`, which
round-trips through import and export), burial or cremation makes them deceased; otherwise they count as living unless their birth (else christening or baptism) is more
than 110 years ago. Owners change that threshold (50 to 150 years) and whether viewers see living
people under **Settings**. Editors set `living_override` (`auto`, `living`, `deceased`) to decide
for the rule, and mark a record private, when editing it. Persons, families, sources, citations, notes and media carry a `private` mark,
which is their GEDCOM restriction notice (`RESN`); it round-trips through GEDCOM import and export
for persons, families, sources, notes and media. GEDCOM media records (`OBJE`) import with their
title, file references and restriction, as one item per person, family or source that links to
them; a media record nothing links to is skipped with a warning. Export writes an `OBJE` record per
item, naming an uploaded item's file by its filename; the file content is not included.

Viewers do not see private people, families or media, and see a living person as a placeholder
named "Living" (same place in charts and families, no names, dates, places, facts, citations or
media, marked "Details hidden" in the UI) when the owner turns **Viewers see living people** off;
editors and owners see everything. A private family's parentage is hidden from charts.
A family with a protected partner keeps its shape but hides its own facts. Protected people are
left out of person lists and searches, totals included, and media of protected people and private
media return 404, file and thumbnail included, and so does the change history of a private person
or of a family with a protected partner; a living placeholder's history is empty. The surname,
place and cemetery indexes, the map, brick walls, statistics, the discovery feed and the quality
overview and report leave protected people out, counts included. Private sources, citations and
notes, evidence, duplicate dismissals, exports and the global change history are not redacted
yet: every role still sees them ([ADR-008](./docs/adr/008-privacy-model.md), #1412).

The app marks a living or private person with a **Living** or **Private** badge on their page and
person cards, in search results, on the pedigree and descendancy charts, on the family group
sheet and in the map's person lists. Search results, chart nodes, group sheet entries and map
persons carry the same `living` and `private` fields as a person.

#### Visitors

Owners choose under **Settings** whether people who are not signed in may browse the tree:
**No** (the default) or **Yes, as viewers**. With it on, a visitor can call the read operations
a viewer can, except the account ones and the change log (history, restore points, snapshots and
comparisons, which name who made each change); sees a **Sign in** link instead of the account
menu, and gets no editing controls. Reads whose cost grows with the tree (lists, search, charts,
browse indexes and their people, the map, statistics, reports and media files) are limited to 30
at once and one more per second per client address. The change applies to the next request and
is written to the audit log. Until living people and private records are protected from visitors
(#1412), the server refuses to turn it on.

#### Upgrading from a version without accounts

An upgraded server starts in setup mode: until an owner exists, the API answers every request
with `503 SETUP_REQUIRED`. Either create the owner (setup page or `myfamily user create`), or set
`AUTH_MODE=open` to keep the old behaviour on a trusted network. Scripts that call the API need a
signed-in session cookie, or open mode, until API tokens exist. Changes made before accounts are
shown in history as the owner's.

### Storage

`serve` picks one storage backend at startup and logs it (`Database: ...`):

1. `DEMO_MODE=true` → in memory (sample data, lost on restart)
2. `DATABASE_URL` set → PostgreSQL
3. otherwise → SQLite at `SQLITE_PATH`

Everything — the event log, the read model, snapshots and research branches — lives in that one
database, and the schema is created or migrated automatically at startup. If the chosen database
cannot be opened, the server exits with an error instead of falling back to memory. Back up a
SQLite deployment by copying the database file while the server is stopped (it also keeps
`-wal`/`-shm` sidecar files while running). See
[ADR-002](./docs/adr/002-dual-database-strategy.md) for the design. The SQLite driver is pure Go,
so the Docker image, the prebuilt release archives and a plain `go build` all run on SQLite with
no C toolchain.

#### Repairing the read model

The read model is derived from the event log. If projecting an event fails after it was saved,
the request still succeeds, the failure is logged, and every later `serve` start warns that the
read model is out of sync. To repair it, stop the server, back up the database, and run
`myfamily rebuild` with the same `DATABASE_URL` / `SQLITE_PATH`. It drops and re-creates the
read-model tables and replays the whole event log into them.
The rebuild refuses to start while a server is running against the same database, and the
server refuses to start while a rebuild is running; on SQLite the lock is a `.lock` file next to
the database file, and on PostgreSQL a session advisory lock, so `DATABASE_URL` must not go
through PgBouncer in transaction or statement pooling mode. If the rebuild is interrupted, `serve` keeps warning; run `myfamily rebuild`
again. Brick walls set by a version from before they were kept in the event log are logged by
the first rebuild before its reset, so it keeps them. GEDCOM place
coordinates and external IDs are in the log only for files imported by a version that has
`myfamily rebuild`. For entities imported by an earlier version, the first rebuild logs them
before its reset, so it keeps them, and later rebuilds replay them from the log. Snapshots created by a version
from before they were kept in the event log are logged by the first rebuild too.

The rebuild is also how a new version changes the read model's schema. The database records the
read-model schema version, and when it does not match the version the binary needs, `serve`
refuses to start and asks for `myfamily rebuild`; it never rebuilds on its own. A database
created by a version from before this check has no recorded version, so the first `serve` of a
newer version asks for one rebuild.

## API Endpoints

- `GET /api/v1/persons` - List persons
- `POST /api/v1/persons` - Create person
- `GET /api/v1/persons/{id}` - Get person
- `PUT /api/v1/persons/{id}` - Update person
- `DELETE /api/v1/persons/{id}` - Delete person
- `GET /api/v1/families` - List families
- `POST /api/v1/families` - Create family
- `GET /api/v1/families/{id}` - Get family
- `PUT /api/v1/families/{id}` - Update family
- `DELETE /api/v1/families/{id}` - Delete family
- `POST /api/v1/families/{id}/children` - Add child to family
- `DELETE /api/v1/families/{id}/children/{personId}` - Remove child
- `GET /api/v1/pedigree/{id}` - Get pedigree chart data
- `GET /api/v1/settings/home-person` / `PUT` / `DELETE` - Get, set or clear the signed-in account's home person, or the site's in open mode (always the mainline)
- `GET /api/v1/history` - Global change history of the mainline
- `GET /api/v1/{persons,families,sources}/{id}/history` - Change history of one record
- `GET /api/v1/{persons,families,sources,citations}/{id}/restore-points` - List the versions a record can be rolled back to
- `POST /api/v1/{persons,families,sources,citations}/{id}/rollback` - Roll a record back to an earlier version (mainline only)
- `GET /api/v1/snapshots` / `POST /api/v1/snapshots` - List or create research snapshots (tags)
- `GET /api/v1/snapshots/{id}` / `DELETE /api/v1/snapshots/{id}` - Get or delete a snapshot
- `GET /api/v1/snapshots/{id1}/compare/{id2}` - Changes between two snapshots
- `GET /api/v1/snapshots/{id}/compare-current` - Changes since a snapshot
- `GET /api/v1/branches` - List research branches
- `POST /api/v1/branches` - Create a research branch, optionally with its research question, subjects and outcome
- `GET /api/v1/branches/{id}` - Get a branch with its research record
- `PATCH /api/v1/branches/{id}` - Edit a branch's description and research record
- `POST /api/v1/branches/{id}/close` - Close a branch without merging, recording its outcome and why; its events are retained
- `DELETE /api/v1/branches/{id}` - Close (archive) a branch without recording why; its events are retained
- `GET /api/v1/branches/{id}/research` - A branch's research logs, evidence analyses and proof summaries, rebuilt from its events (works on a closed branch)
- `POST /api/v1/branches/{id}/research-logs/promote` - Copy a closed branch's research logs to the mainline
- `GET /api/v1/branches/{id}/drift` - How far the mainline has moved under a branch
- `GET /api/v1/branches/{id}/compare` - Compare a branch against the mainline, with the conflict verdict
- `GET /api/v1/branches/{id}/evidence-coverage` - Facts a branch changed without documenting them
- `GET /api/v1/branches/{id}/health` - Validation issues, quality issues and duplicates a branch introduces
- `POST /api/v1/branches/{id}/merge/precheck` - List what would block a merge, writing nothing
- `POST /api/v1/branches/{id}/merge` - Merge a branch into the mainline
- `POST /api/v1/branches/{id}/merge/resume` - Finish a merge whose replay onto the mainline was interrupted
- Most genealogy reads and writes (people, families, sources, citations, notes, media, associations, evidence, search, browse, map, charts, reports, snapshots and person merges) accept an optional `?branch={id}` scope; omit it for the mainline. `openapi.yaml` marks exactly which operations take it
- `GET /api/v1/map/locations` - Get geographic locations for map
- `GET /api/v1/search?q=...` - Search persons
- `POST /api/v1/gedcom/import` - Import GEDCOM file
- `GET /api/v1/gedcom/export` - Export as GEDCOM (optional `?version=5.5|5.5.1|7.0`; defaults to 5.5, auto-upgraded to 7.0 when the data uses 7.0-only features). When exporting 7.0 external identifiers (EXID) down to 5.5/5.5.1, a FamilySearch ARK identifier on a person is preserved as the `_FSFTID` vendor tag; other external IDs have no 5.5.x equivalent and are reported as data loss.
- `GET /api/v1/gedcom/export/preview` - Preview an export conversion (optional `?version=`); reports data loss without producing a file
- `GET /api/v1/export/tree` - Export complete tree as JSON
- `GET /api/v1/export/{persons,families,sources,citations,events,attributes}` - Export one entity type as JSON (default) or CSV (`?format=csv`); for CSV, `?fields=id,given_name,...` picks the columns. Unknown formats or fields return 400. CSV cells beginning with `=`, `+`, `-`, `@`, tab or carriage return are prefixed with `'` so spreadsheets show them as text rather than evaluating a formula

API documentation: http://localhost:8080/api/v1/docs

## Development

```bash
# Run tests
go test ./...

# Run frontend tests
cd web && npm test

# Format code
go fmt ./...

# Static analysis
go vet ./...
```

## Documentation

- [ETHOS.md](./docs/ETHOS.md) - Project vision and guiding principles
- [ROADMAP.md](./docs/ROADMAP.md) - Phased feature plan and success factors
- [CONVENTIONS.md](./docs/CONVENTIONS.md) - Code patterns and standards
- [Architecture Decisions](./docs/adr/) - Key technical decisions with rationale
- [FEATURES.md](./FEATURES.md) - Complete feature list
- [CONTRIBUTING.md](./CONTRIBUTING.md) - Development workflow and code standards

## License

AGPL-3.0
