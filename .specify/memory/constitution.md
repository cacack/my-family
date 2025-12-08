<!--
SYNC IMPACT REPORT
==================
Version change: N/A → 1.0.0
Modified principles: N/A (initial creation)
Added sections:
  - Core Principles (4 principles)
  - Performance Standards
  - Development Workflow
  - Governance
Removed sections: N/A
Templates requiring updates:
  - .specify/templates/plan-template.md ✅ (Constitution Check section compatible)
  - .specify/templates/spec-template.md ✅ (Requirements section aligned)
  - .specify/templates/tasks-template.md ✅ (Test phases align with testing principle)
  - .specify/templates/checklist-template.md ✅ (No changes needed)
  - .specify/templates/agent-file-template.md ✅ (No changes needed)
Follow-up TODOs: None
==================
-->

# my-family Constitution

## Core Principles

### I. Code Quality

All code MUST be idiomatic Go following established conventions.

- Code MUST pass `go vet` and `go fmt` without warnings or changes
- All exported functions, types, and packages MUST have documentation comments
- Functions MUST have a single, clear responsibility
- Error handling MUST be explicit; never ignore returned errors
- Dependencies MUST be minimal and justified; prefer standard library

**Rationale**: Idiomatic Go code is easier to maintain, review, and onboard new contributors.
Genealogy data spans generations; code quality ensures long-term maintainability.

### II. Testing Standards

All features MUST have corresponding tests that verify behavior.

- Unit tests MUST cover core business logic (models, services)
- Integration tests MUST verify data persistence and retrieval
- Tests MUST be deterministic and not depend on external services
- Test names MUST describe the scenario being tested (e.g., `TestPerson_AddChild_DuplicateReturnsError`)
- Coverage target: 80% for core packages; best effort for CLI/UI layers

**Rationale**: Genealogy data is irreplaceable family history. Comprehensive tests
prevent data corruption and ensure import/export operations preserve integrity.

### III. User Experience Consistency

The user interface MUST be intuitive and consistent across all interactions.

- CLI commands MUST follow the pattern: `my-family <noun> <verb> [options]`
- All user-facing output MUST support both human-readable and JSON formats (`--json` flag)
- Error messages MUST be actionable: state what went wrong and suggest resolution
- Destructive operations MUST require confirmation unless `--force` is provided
- GEDCOM import/export MUST preserve standard fields without data loss

**Rationale**: Users may have varying technical skills. Consistent, predictable
interactions reduce errors when handling sensitive family data.

### IV. Performance Requirements

Operations MUST complete within acceptable time bounds for typical datasets.

- Single record operations (add, view, edit): <100ms response time
- Bulk imports (1000 records): <10 seconds
- Search operations: <500ms for databases up to 10,000 individuals
- Memory usage: <100MB for typical family trees (<5000 individuals)
- Database queries MUST use indexes; full table scans are prohibited for user operations

**Rationale**: Performance degrades user experience and trust. Family trees grow
over time; operations must remain responsive as data scales.

## Performance Standards

### Benchmarking

- Performance-critical paths MUST have Go benchmarks (`func BenchmarkXxx`)
- Benchmark results SHOULD be tracked across releases for regression detection
- Database schema changes MUST include query plan analysis for common operations

### Resource Limits

| Operation | Time Limit | Memory Limit |
|-----------|------------|--------------|
| Single record CRUD | 100ms | 10MB |
| Bulk import (1000 records) | 10s | 100MB |
| Full tree traversal | 2s | 50MB |
| Search/filter | 500ms | 25MB |
| GEDCOM export (full tree) | 5s | 100MB |

## Development Workflow

### Pre-Commit Requirements

Before any commit, code MUST:
1. Pass `go build ./...` without errors
2. Pass `go test ./...` without failures
3. Pass `go vet ./...` without warnings
4. Be formatted with `go fmt ./...`

### Code Review Standards

- All changes MUST be reviewed before merging to main
- Reviews MUST verify adherence to constitution principles
- Performance-impacting changes MUST include benchmark results
- Database schema changes MUST include migration scripts

### Documentation Requirements

- README.md MUST stay current with installation and usage instructions
- Breaking changes MUST be documented in a changelog
- API changes MUST update relevant documentation before merge

## Governance

This constitution is the authoritative guide for all technical decisions in the
my-family project.

### Authority

- Constitution principles MUST be followed in all development work
- Conflicts between convenience and principles MUST resolve in favor of principles
- Exceptions require explicit documentation with justification in the relevant
  plan.md or PR description

### Amendment Process

1. Propose amendment via pull request modifying this file
2. Document rationale for change
3. Amendment requires maintainer approval
4. Update version number according to semantic versioning:
   - MAJOR: Principle removal or fundamental redefinition
   - MINOR: New principle or significant expansion
   - PATCH: Clarifications, typo fixes, minor refinements
5. Update LAST_AMENDED_DATE to the merge date

### Compliance Verification

- All PRs MUST pass automated checks (build, test, vet, fmt)
- Code reviews MUST include constitution compliance check
- plan.md files MUST include "Constitution Check" section
- Violations discovered post-merge MUST be addressed in follow-up commits

### Guidance Files

- Use CLAUDE.md for runtime development guidance specific to AI assistants
- Use README.md for human developer onboarding and reference

**Version**: 1.0.0 | **Ratified**: 2025-12-07 | **Last Amended**: 2025-12-07
