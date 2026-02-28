# Audit Prompt Library

Portable, persona-based audit prompts for evaluating the my-family codebase. Each prompt is self-contained for copy-paste into any AI model, and several have Claude Code skill counterparts for automated execution.

## Quick Start

1. Copy the **Context Bundle** (below) into your AI model's context
2. Pick a prompt file from this directory
3. Copy the `## Prompt` section into the AI
4. Review the scored output

## Context Bundle

Every audit prompt expects these project documents as context. Attach them alongside the prompt:

| Document | Why |
|----------|-----|
| `docs/ETHOS.md` | Mission, differentiators, anti-patterns |
| `docs/ROADMAP.md` | Phased feature plan (what's in scope now) |
| `docs/CONVENTIONS.md` | Code patterns, commit types, naming |
| `docs/ARCHITECTURAL-INVARIANTS.md` | Rules that must always hold |
| `docs/TESTING-STRATEGY.md` | Test organization and scenarios |
| `docs/INTEGRATION-MATRIX.md` | Feature integration checklists |
| `CLAUDE.md` | Architecture overview and build commands |

Some prompts require additional files (noted in their `## Context Required` section).

## Prompts

| # | File | Persona | Claude Code Skill |
|---|------|---------|-------------------|
| 1 | `architect.md` | Software Architect | `/audit-architect` |
| 2 | `genealogist.md` | Domain Expert (Genealogy) | `/audit-domain` |
| 3 | `security.md` | Security Auditor | `/audit-security` |
| 4 | `go-purist.md` | Go Language Expert | `/audit-go` |
| 5 | `frontend.md` | Frontend Specialist | `/audit-frontend` |
| 6 | `devil-advocate.md` | Devil's Advocate | Portable only |
| 7 | `test-engineer.md` | Test Engineer | `/audit-tests` |
| 8 | `documentation.md` | Documentation Reviewer | Portable only |
| 9 | `api-cohesion.md` | API Reviewer | Portable only |
| 10 | `product-cohesion.md` | Product Manager | Portable only |

## Output Format

All prompts use a standardized output format (defined in `_context.md`):

1. **Scorecard** — 0-5 ratings across relevant dimensions
2. **Top Findings** — Risk-ranked, with file/line citations
3. **Issue-Ready Tickets** — Title, description, acceptance criteria, affected files
4. **Manual Verification** — Items that need human judgment

## Recommended Schedule

| Cadence | Prompts |
|---------|---------|
| Every release | `architect`, `go-purist`, `test-engineer` |
| Monthly | `security`, `frontend`, `api-cohesion` |
| Quarterly | `genealogist`, `devil-advocate`, `documentation`, `product-cohesion` |

## Claude Code Skills

Prompts 1-5 and 7 have Claude Code skill counterparts. Run them with:

```
/audit-architect    # Architecture audit
/audit-domain       # Domain/genealogy audit
/audit-security     # Security audit
/audit-go           # Go idioms audit
/audit-frontend     # Frontend audit
/audit-tests        # Test quality audit
/audit-full         # All 6 above in parallel
```

Skills are read-only and produce the same standardized output format.

## Saving Results

Save audit outputs to `docs/audit/results/` with the naming convention:

```
YYYY-MM-DD-{prompt-name}.md
```

Example: `2026-02-16-architect.md`

## Relationship to /health-check

| | `/health-check` | `/audit-full` |
|---|---|---|
| **Depth** | Grep-based pattern detection | Reasoning-based design assessment |
| **Speed** | ~2 minutes | ~10 minutes |
| **Cadence** | Weekly / pre-release | Monthly / at milestones |
| **Focus** | Drift, conventions, debt | Architecture, design, gaps |

They're complementary. Run `/health-check` often; run `/audit-full` for deeper analysis.

## n8n Automation

These prompts are automated via n8n workflows in [`../n8n/`](../n8n/). The pipeline:

```
n8n Orchestrator → GitHub API (fetch code) → LiteLLM → 10 personas in parallel
                                                          ↓
                                              Aggregate → GitHub Issue
```

Each persona runs on a different LLM (Claude, GPT-4o, Gemini) for diverse perspectives. See [`../n8n/README.md`](../n8n/README.md) for setup instructions.

The prompt format is designed for both manual use (copy-paste into any AI) and programmatic extraction by n8n. The `## Prompt` section is the payload, `## Context Required` lists files to attach, and `_context.md` standardizes output across models.
