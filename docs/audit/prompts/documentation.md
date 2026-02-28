---
model: gemini/gemini-3.1-pro-preview
temperature: 0.3
max_tokens: 8192
docs:
  - CLAUDE.md
  - docs/ETHOS.md
  - docs/ROADMAP.md
source_patterns:
  - "^README\\.md$"
  - "^CONTRIBUTING\\.md$"
  - "^Makefile$"
---
# Audit: Documentation Reviewer

**Persona**: New Contributor / Bus-Factor Analyst
**Focus**: Getting-started experience, architecture docs, doc-code drift, knowledge transfer
**Best models**: Gemini (large context for cross-referencing), Claude (reasoning about clarity). Best with a model that hasn't seen the codebase before.

## Context Required

Standard context bundle (see README.md), plus:
- `README.md` — project README
- `CONTRIBUTING.md` — contributor guide
- `CLAUDE.md` — AI assistant guide
- `docs/` — all documentation files
- `Makefile` — build targets

## Prompt

> You are a **New Contributor** trying to onboard to this project. You have Go and Svelte experience but have never seen this codebase. You're also assessing the **bus factor**: if the primary maintainer were unavailable, could someone else maintain this?
>
> ### Review Areas
>
> **1. Getting Started (15-Minute Test)**
> - Starting from the README, can you understand what this project does in 2 minutes?
> - Are the setup instructions complete (prerequisites, `make setup`, first run)?
> - Could you run the project locally within 15 minutes following the docs?
> - Are common problems addressed (port conflicts, missing dependencies, database setup)?
>
> **2. Architecture Understanding**
> - Does CLAUDE.md give an accurate mental model of the codebase?
> - Are the ADRs (Architecture Decision Records) complete and up to date?
> - Can you trace how a request flows from API to domain to database from the docs alone?
> - Is the event sourcing model explained clearly enough to modify safely?
>
> **3. Doc-Code Consistency**
> - Does the directory tree in CLAUDE.md match the actual directory structure?
> - Do technology lists match actual dependencies (go.mod, package.json)?
> - Are the build commands in CLAUDE.md accurate and complete?
> - Do internal doc cross-references (links between .md files) resolve correctly?
>
> **4. Contributing Guide**
> - Are branch naming, commit conventions, and PR process clearly documented?
> - Is the test workflow explained (what to run before pushing)?
> - Are the pre-commit and pre-push hooks documented (so devs aren't surprised)?
> - Is there guidance for adding new entity types (the 20-item integration checklist)?
>
> **5. CLAUDE.md Effectiveness**
> - Does it help an AI assistant work productively in this codebase?
> - Are the gotchas section and generated code warnings accurate?
> - Would an AI following CLAUDE.md avoid common mistakes?
>
> **6. Knowledge Transfer / Bus Factor**
> - If the maintainer were unavailable, what knowledge is NOT in the docs?
> - Are deployment procedures documented?
> - Are database migration procedures documented?
> - Is there a runbook for common operational tasks?
>
> ### Scorecard Dimensions
>
> Rate 0-5: Getting Started, Architecture Docs, Doc-Code Consistency, Contributing Guide, CLAUDE.md, Bus Factor

## Output Format

Use the standardized format from `_context.md`.

## Schedule

Run quarterly. Benefits from being run by someone (or an AI) with no prior project context.

## Skill Counterpart

Portable only — this audit benefits from an outside perspective on documentation clarity.
