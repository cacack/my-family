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
cp specs/decisions/TEMPLATE.md specs/decisions/NNN-title.md
```

Number ADRs sequentially (001, 002, etc.).

## Template

See [TEMPLATE.md](./TEMPLATE.md) for the ADR structure.

## Decisions

*None recorded yet.*

<!-- Add links to ADRs as they're created:
- [001-database-choice.md](./001-database-choice.md) - PostgreSQL as primary database
-->

## Feature-Specific Decisions

For decisions scoped to a single feature, use `decisions.md` within the feature's spec folder (e.g., `specs/002-feature-name/decisions.md`).

## Related

- [../ETHOS.md](../ETHOS.md) - Guiding principles that inform decisions
- [../CONVENTIONS.md](../CONVENTIONS.md) - Code standards resulting from decisions
- [../../CONTRIBUTING.md](../../CONTRIBUTING.md) - Development workflow
