<objective>
Enhance the CI workflow with three improvements from the gedcom-go project:
1. Add a CI Success gate job for simplified branch protection
2. Add per-package coverage enforcement
3. Add PR title check to prevent conventional commit format in PR titles

These changes improve developer experience by simplifying branch protection rules and ensuring consistent commit/PR conventions.
</objective>

<context>
This is a Go project using GitHub Actions. The improvements were successfully implemented in the sibling project `gedcom-go` and should be ported here.

Read @HANDOFF.md for detailed implementation examples.
Read @.github/workflows/ci.yml to understand the current CI structure.
</context>

<requirements>
1. **CI Success Gate Job**
   - Add a job named "CI Success" that depends on all other jobs
   - Use `if: always()` to run even if dependencies fail
   - Check for failures and cancellations in dependent jobs
   - Exit 1 if any job failed or was cancelled

2. **Per-Package Coverage Check**
   - Add coverage enforcement using `vladopajic/go-test-coverage@v2`
   - Set `threshold-package: 85` and `threshold-total: 85`
   - Requires a coverage.out profile from test step

3. **PR Title Check**
   - Add a job that runs only on pull_request events
   - Reject PR titles that match conventional commit format: `^(feat|fix|docs|style|refactor|perf|test|build|ci|chore|revert)(\(.+\))?!?:`
   - Exempt release-please PRs (titles starting with `chore(main): release`)
   - Why: Prevents duplicate changelog entries when using merge commits with release-please
</requirements>

<implementation>
Reference the exact implementations in HANDOFF.md:
- CI Success gate job pattern (lines 16-35)
- PR title check pattern (lines 81-102)
- Coverage check with vladopajic action (lines 162-168)

Ensure the `needs` array in CI Success job includes ALL jobs in the workflow.
</implementation>

<output>
Modify: `./.github/workflows/ci.yml`
</output>

<verification>
Before completing:
- Validate YAML syntax: `python -c "import yaml; yaml.safe_load(open('.github/workflows/ci.yml'))"`
- Verify CI Success job lists all other jobs in its `needs` array
- Confirm PR title regex correctly identifies conventional commit format
- Ensure coverage job has access to coverage.out artifact
</verification>

<success_criteria>
- CI workflow has three new jobs: ci-success, coverage, pr-title
- ci-success depends on ALL other jobs and uses if: always()
- pr-title exempts release-please PRs
- Coverage threshold set to 85% per-package
</success_criteria>
