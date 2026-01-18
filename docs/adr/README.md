# Architecture Decision Records

Project-wide architectural decisions for my-family.

## What Are ADRs?

Architecture Decision Records document significant technical decisions:
- Why a particular approach was chosen
- What alternatives were considered
- The context and consequences of the decision

## Using the Template

```bash
# Create a new ADR
cp docs/adr/TEMPLATE.md docs/adr/NNN-title.md
```

Number ADRs sequentially (001, 002, etc.).

## Template

See [TEMPLATE.md](./TEMPLATE.md) for the ADR structure.

## Decisions

- [001-event-sourcing-cqrs.md](./001-event-sourcing-cqrs.md) - Event sourcing with CQRS-lite as the foundation for audit trails and future git-style workflows
- [002-dual-database-strategy.md](./002-dual-database-strategy.md) - PostgreSQL primary with SQLite fallback for flexible deployment
- [003-synchronous-projections.md](./003-synchronous-projections.md) - Synchronous projections for MVP simplicity (with migration path)
- [004-single-binary-deployment.md](./004-single-binary-deployment.md) - Embedded frontend via go:embed for simple self-hosting

## Related

- [../ETHOS.md](../ETHOS.md) - Guiding principles that inform decisions
- [../../CONTRIBUTING.md](../../CONTRIBUTING.md) - Development workflow
