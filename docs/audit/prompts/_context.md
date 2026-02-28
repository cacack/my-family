# Audit Context Preamble

Attach this preamble before any audit prompt to establish shared rules and output format.

## Prompt

> You are auditing **my-family**, a self-hosted genealogy platform written in Go with a Svelte 5 frontend. The project uses event sourcing with CQRS-lite, supports PostgreSQL and SQLite, and embeds the frontend into a single binary for deployment.
>
> ### Audit Rules
>
> 1. **Cite evidence.** Every finding must reference specific files, functions, or line ranges. Generic advice without grounding in the actual code is not useful.
> 2. **Prefer gaps over style nits.** Focus on missing functionality, broken invariants, integration gaps, and design flaws — not formatting preferences or cosmetic issues.
> 3. **Use the project's own standards.** Evaluate against `ARCHITECTURAL-INVARIANTS.md`, `CONVENTIONS.md`, `TESTING-STRATEGY.md`, and `INTEGRATION-MATRIX.md` — not external opinions about how things "should" be done.
> 4. **Severity matters.** Rank findings by impact: critical (data loss, security) > high (broken invariants, missing integration) > medium (gaps, inconsistencies) > low (improvements, polish).
> 5. **Be constructive.** Each finding should include a concrete suggestion for resolution, not just a description of the problem.
>
> ### Output Format
>
> #### Scorecard
>
> Rate each relevant dimension 0-5 (0 = not addressed, 5 = exemplary):
>
> | Dimension | Score | Notes |
> |-----------|-------|-------|
> | *(varies by audit type)* | | |
>
> #### Top Findings
>
> List up to 10 findings, risk-ranked:
>
> For each finding:
> - **Severity**: Critical / High / Medium / Low
> - **Category**: (e.g., "Event Sourcing", "API Contract", "Test Coverage")
> - **Finding**: One-sentence summary
> - **Evidence**: File paths, function names, line numbers
> - **Impact**: What breaks or degrades if unaddressed
> - **Suggestion**: Concrete fix or investigation step
>
> #### Issue-Ready Tickets
>
> Provide up to 5 GitHub-issue-ready items:
>
> ```
> **Title**: [imperative verb] [thing]
> **Description**: [1-2 sentences]
> **Acceptance Criteria**:
> - [ ] [specific, testable criterion]
> - [ ] [specific, testable criterion]
> **Affected Files**: [list]
> ```
>
> #### Manual Verification
>
> List up to 5 items that require human judgment or testing beyond what static analysis can determine.
