# ADR-007: Access Model — Accounts, Roles, Sessions and Anonymous Access

**Status:** Accepted — implemented in v0.14 (#1407 to #1441)
**Date:** 2026-11-18
**Decision Makers:** Project maintainer
**Related Features:** [#488](https://github.com/cacack/my-family/issues/488)

## Context

The app has no authentication. The HTTP middleware is recover, request ID, logging and CORS with
`AllowOrigins: ["*"]`; the OpenAPI spec declares no security schemes across its 115 paths. Anyone
who can reach the port can read, edit, import, roll back and export the tree.

Two things the app already promises depend on identity. The audit trail is meant to say *who*
changed what ([ETHOS](../ETHOS.md)): `EventMetadata.UserID` exists and the history API returns
`user_id`, but no command writes it. And the planned visitor-facing features (#1020, #1021, #1035)
assume a difference between the site owner and a visitor.

This ADR settles who the principals are, how they sign in, where account data lives and how access
is enforced. Which records a principal may *see* (living people, private records) is a separate
decision, to be recorded in ADR-008.

## Decision Drivers

- One code path. A second "auth off" path doubles the surface to test and is where gaps hide.
- Private by default: an upgrade must never make a tree more exposed than the operator chose.
- Self-hosting stays easy ([ADR-004](./004-single-binary-deployment.md)): no mail server, identity
  provider or reverse proxy required.
- The event log is append-only (ES-002) and flows into history, snapshots, rollback, exports and
  branches. Nothing secret, and nothing that is not tree data, belongs in it.
- Both SQL backends behave the same (DB-001).
- One tree per instance now, without closing the door on several later.

## Considered Options

### Sub-decision 1 — Is authentication optional?

#### Option 1A: A switch that turns authentication off

**Pros:** today's behaviour is one setting away.
**Cons:** two code paths; every handler and test has to hold under both; an unauthenticated request
has no principal, so authorization and attribution need a second rule.

#### Option 1B: Always on, with anonymous as a principal (chosen)

Every request resolves to a principal. A request without a session is the *anonymous* principal,
whose role is configured.

**Pros:** one path; "who may do this" has one answer everywhere; open and public instances are
configurations, not modes.
**Cons:** an upgraded install must create an owner before it is usable again.

### Sub-decision 2 — Roles and tenancy

#### Option 2A: A role column on the user

**Pros:** simplest schema.
**Cons:** a second tree would need the role moved, a data migration on the security path.

#### Option 2B: Roles as memberships of a tree, with one tree (chosen)

A membership is `(user, tree, role)`. There is exactly one tree, with a constant ID.

**Pros:** multi-tree later adds rows, not a migration; costs one join.
**Cons:** a table that looks over-built while there is one tree.

#### Option 2C: Full multi-tree now

**Pros:** no later change.
**Cons:** needs a tree dimension on every event and read-model row, or a database per tree, with no
user asking for it yet.

### Sub-decision 3 — Where account data lives

#### Option 3A: Account events in the event log

**Pros:** reuses the audit machinery; matches the original wording of #488.
**Cons:** account events would appear in change history, snapshot comparisons, rollback, exports
and branch overlays unless each one excluded them, and a missed exclusion is a leak. Credentials
could not go in the log anyway, so accounts would be split across two stores.

#### Option 3B: Plain tables plus an append-only auth audit table (chosen)

**Pros:** accounts never touch tree machinery; credentials, sessions and invites sit together; the
tree's audit trail still gets "who" through event metadata.
**Cons:** a third kind of storage, neither event log nor derived read model; no point-in-time
replay of account state.

### Sub-decision 4 — How people sign in and join

#### Option 4A: Local passwords, server-side sessions, invite links (chosen)

**Pros:** works on a fresh install with nothing else configured; sessions can be revoked.
**Cons:** the app stores password hashes; no single sign-on.

#### Option 4B: Delegate to OIDC or a trusted reverse-proxy header

**Pros:** no credentials stored; fits operators who already run an identity provider.
**Cons:** makes extra infrastructure a prerequisite for a second user.

#### Option 4C: Open self-registration with owner approval

**Pros:** relatives can ask to join.
**Cons:** an unauthenticated write endpoint to protect from abuse, and an approval queue, where an
invite link gives the owner the same control.

### Sub-decision 5 — Where access is enforced

#### Option 5A: Checks inside each handler

**Cons:** 115 paths; a forgotten check fails open.

#### Option 5B: Declared per operation in the OpenAPI spec, enforced in one middleware (chosen)

**Pros:** default-deny; the spec documents the role each operation needs; a test can assert that
every operation declares one.
**Cons:** the declaration mechanism has to survive code generation.

## Decision

We chose **1B + 2B + 3B + 4A + 5B**.

**Principals and roles.** Four principals, each including the abilities of the one before:

| Principal | May |
|-----------|-----|
| Anonymous | Whatever the anonymous-access setting grants |
| Viewer | Read the tree |
| Editor | Change tree data; create, merge and close research branches; export |
| Owner | Manage users and settings; GEDCOM import; rollback |

**Anonymous access.** A tree setting the owner changes in the UI: `none` (default) or `viewer`.
An anonymous viewer never sees more than ADR-008 allows for the public. A visitor may not read the
change log (history, restore points, snapshots, comparisons), whose entries name who made each
change, or a research branch's view (ADR-008 §4), and reads whose cost grows with the tree are rate-limited per client address.

**Open mode.** `AUTH_MODE=open` maps the anonymous principal to owner, for a trusted network, demo
mode and end-to-end tests. It is settable only from the environment, never from the UI, logs a
warning at startup, and records no actor. `DEMO_MODE` implies it.

**First owner.** With no owner account the server is in setup mode and serves only the setup page.
Creating the owner needs a one-time token written to the server log, or the CLI
(`myfamily user create`). An upgraded install starts here.

**Joining and recovery.** The owner issues single-use, expiring invite links that carry a role.
There is no self-registration. A forgotten password is reset by an owner-issued link, or from the
CLI for an owner. No email is sent.

**Credentials and sessions.** Passwords are hashed with argon2id. A session is an opaque random
token, stored hashed server-side with idle and absolute expiry, in an `HttpOnly`, `SameSite=Lax`
cookie that is `Secure` over TLS. State-changing requests whose `Origin` does not match the host
are refused, and CORS no longer allows `*`. Sign-in attempts are rate-limited.

**Sign-in limits.** Failed sign-ins are limited per username (5 in 15 minutes, from anyone) and
per client address (20). A correct password is refused while either limit is reached, so anyone
who knows a username can keep it locked out. We accept that over letting a correct password from a
client with no recent failures through, because that bypass makes the per-username limit a
per-address one: a guesser with many addresses (one IPv6 prefix holds billions) gets 5 guesses per
address, and the per-username limit is the only guard against guessing from many addresses.
Behind a reverse proxy every client shares one address, so the bypass would not even help the
owner there. The lockout costs little: it refuses only new sign-ins, sessions last up to 30 days,
and the counts are in memory, so a restart clears them. Rejected: a bypass for a browser that has
signed in before (a second stored credential to issue, expire and revoke), and longer or growing
lockouts (they make the denial worse, not better).

**Storage.** `users`, `tree_memberships`, `sessions`, `invites` and an append-only `auth_audit`
table, in PostgreSQL, SQLite and memory. They are not in the event log and are not read model:
`myfamily rebuild` leaves them alone.

**Attribution.** A command carries the acting principal, and the handler writes its user ID to
`EventMetadata.UserID`. Events written before accounts existed, or in open mode, have none; they
are read as "the owner, before accounts" and the log is not rewritten.

**Enforcement.** Each operation in `openapi.yaml` declares its minimum role, and one middleware
resolves the principal and refuses the request before any handler runs. An operation with no
declaration is refused. Every check takes the tree as an argument.

**Multi-tree seams.** Memberships carry a tree ID; settings are split into instance settings
(auth mode) and tree settings (anonymous access, privacy rules); authorization takes a tree.
Events, read-model rows and URLs carry no tree dimension.

**Deferred.** API tokens, OIDC and proxy-header sign-in, a suggest-only contributor role (#1035),
permissions scoped to a family line, email delivery, and multi-tree itself.

## Consequences

### Positive

- Every request has a principal, so authorization and attribution each have one rule.
- A tree is private until its owner says otherwise.
- The audit trail records who made each change from the first signed-in edit.
- No account data can leak through history, snapshots, exports or branches.
- A new endpoint is unreachable until someone decides who may call it.

### Negative

- Upgrading is a breaking change: the operator must create an owner or set `AUTH_MODE=open`.
  Mitigation: release notes, a startup log line that says which, and API versioning (#393).
- Scripts that call the API need a signed-in session until API tokens exist. Mitigation: open mode
  on a trusted network; tokens are the first deferred item.
- Account tables are a third storage category that backups (#15) must include and `rebuild` must
  not drop. Mitigation: an invariant and a test for each.
- Whoever reads the server log during setup can claim the instance. Accepted: log access already
  implies control of the deployment.
- Adding `/trees/{id}` to URLs later breaks the API. Mitigation: decide alongside #393.

### Neutral

- Demo mode becomes a named use of open mode, not a special case.
- New invariants (default-deny, accounts outside the event log, actor on every command) are added
  to [ARCHITECTURAL-INVARIANTS.md](../ARCHITECTURAL-INVARIANTS.md) as each is implemented.

## References

- [ADR-001](./001-event-sourcing-cqrs.md) - event sourcing; the audit trail this adds "who" to
- [ADR-005](./005-research-branch-data-model.md) - branches; any editor may create and merge them
- [ADR-006](./006-schema-evolution.md) - `myfamily rebuild`, which must not touch account tables
- [#488](https://github.com/cacack/my-family/issues/488) - multi-user collaboration epic
- [#393](https://github.com/cacack/my-family/issues/393) - API versioning and stability
