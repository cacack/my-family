# HANDOFF: Developer Experience Improvements from gedcom-go

Captured: 2025-12-21

## Summary

Developer and pipeline improvements implemented in `gedcom-go` that should be considered for `my-family`.

## Improvements Implemented in gedcom-go

### 1. CI Gate Job for Branch Protection

**Problem**: GitHub branch protection requires enumerating every status check by name. Adding/removing CI jobs requires updating branch protection rules.

**Solution**: Single "CI Success" gate job that depends on all other jobs.

```yaml
ci-success:
  name: CI Success
  runs-on: ubuntu-latest
  needs: [test, coverage, lint, format, security, build-examples]
  if: always()
  steps:
    - name: Check all jobs passed
      run: |
        if [[ "${{ contains(needs.*.result, 'failure') }}" == "true" ]]; then
          echo "❌ Some jobs failed"
          exit 1
        fi
        if [[ "${{ contains(needs.*.result, 'cancelled') }}" == "true" ]]; then
          echo "❌ Some jobs were cancelled"
          exit 1
        fi
        echo "✅ All CI jobs passed"
```

**Branch protection**: Only require "CI Success" check instead of individual jobs.

### 2. Semi-Linear History (GitLab-style)

**Goal**: Require PRs to be rebased on main before merging.

**GitHub Settings**:
1. **Branch protection** → "Require branches to be up to date before merging"
2. **Repo settings** → "Always suggest updating pull request branches" (`allow_update_branch: true`)

**Limitation**: GitHub can't force rebase-only updates; both merge and rebase options are shown. Team discipline or automation (Mergify, GitHub Actions) needed for strict enforcement.

### 3. Pre-commit Hook Robustness

**Problem**: Pre-commit hook failed when `golangci-lint` not in PATH (installed to `~/go/bin`).

**Solution**: Check multiple locations:

```bash
if command -v golangci-lint &> /dev/null; then
  GOLANGCI_LINT="golangci-lint"
elif [ -x "$HOME/go/bin/golangci-lint" ]; then
  GOLANGCI_LINT="$HOME/go/bin/golangci-lint"
elif [ -x "$(go env GOPATH)/bin/golangci-lint" ]; then
  GOLANGCI_LINT="$(go env GOPATH)/bin/golangci-lint"
else
  echo "Error: golangci-lint not found. Run 'make install-tools' first."
  exit 1
fi
```

### 4. PR Title Convention (Release-Please Fix)

**Problem**: Using merge commits (for semi-linear history) + release-please causes duplicate changelog entries. Release-please picks up both PR titles and commit messages when both use conventional commit format.

**Solution**: Different formats for commits vs PR titles:

| Where | Format | Example |
|-------|--------|---------|
| Commit | `type(scope): desc` | `feat(parser): add date support` |
| PR title | Descriptive | `Add date support` |

**CI Enforcement** (in `ci.yml`):

```yaml
pr-title:
  name: PR Title Check
  runs-on: ubuntu-latest
  if: github.event_name == 'pull_request'
  steps:
    - name: Check PR title is not conventional commit format
      run: |
        TITLE="${{ github.event.pull_request.title }}"

        # Exempt release-please PRs
        if echo "$TITLE" | grep -qE "^chore\(main\): release [0-9]"; then
          echo "✅ Release-please PR exempt"
          exit 0
        fi

        PATTERN="^(feat|fix|docs|style|refactor|perf|test|build|ci|chore|revert)(\(.+\))?!?:"
        if echo "$TITLE" | grep -qE "$PATTERN"; then
          echo "::error::PR title should NOT use conventional commit format."
          exit 1
        fi
```

### 5. Defined Commit Types

**Problem**: Inconsistent use of conventional commit types. `feat`/`fix` used for tooling changes, polluting changelog.

**Solution**: Document specific types with clear boundaries:

| Type | Use for | Changelog? |
|------|---------|------------|
| `feat` | Library/app capabilities | ✅ |
| `fix` | Bug fixes in library/app | ✅ |
| `perf` | Performance improvements | ✅ |
| `refactor` | Code restructuring | ❌ |
| `test` | Test changes | ❌ |
| `docs` | Documentation | ❌ |
| `ci` | Dev infrastructure, deps, tooling | ❌ |
| `chore` | Miscellaneous maintenance | ❌ |

**Key rule**: `feat`/`fix` reserved for what users consume, not development tooling.

**Release-please config** (`release-please-config.json`):

```json
{
  "changelog-sections": [
    {"type": "feat", "section": "Features"},
    {"type": "fix", "section": "Bug Fixes"},
    {"type": "perf", "section": "Performance"}
  ]
}
```

### 6. Per-Package Coverage Enforcement

**Problem**: Overall coverage can hide undertested packages. A 90% total might include one package at 50%.

**Solution**: Enforce per-package minimums (85%) in both pre-commit and CI.

**Pre-commit hook** (in `scripts/pre-commit`):

```bash
echo "→ Checking per-package test coverage..."
FAILED=0
for pkg in charset decoder encoder gedcom parser validator version; do
  COV=$(go test -cover "./$pkg" 2>/dev/null | grep -oE '[0-9]+\.[0-9]+%' | head -1)
  PCT=$(echo "$COV" | sed 's/%//')
  if (( $(echo "$PCT < 85.0" | bc -l) )); then
    echo "  ✗ ./$pkg: $COV (below 85%)"
    FAILED=1
  else
    echo "  ✓ ./$pkg: $COV"
  fi
done
[ $FAILED -eq 1 ] && exit 1
```

**CI** (using `vladopajic/go-test-coverage` action):

```yaml
- name: Check coverage thresholds
  uses: vladopajic/go-test-coverage@v2
  with:
    profile: coverage.out
    threshold-package: 85
    threshold-total: 85
```

## TODO for my-family

- [ ] Add CI Success gate job to `.github/workflows/ci.yml`
- [ ] Update branch protection to require only "CI Success"
- [ ] Enable "Always suggest updating pull request branches" in repo settings
- [ ] Enable "Require branches to be up to date before merging" in branch protection
- [x] Update pre-commit hook to find tools in common locations
- [x] Add per-package coverage enforcement to pre-commit hook
- [ ] Add per-package coverage check to CI (vladopajic/go-test-coverage)
- [ ] Add PR title check to CI (reject conventional commit format in titles)
- [x] Document commit types in CONTRIBUTING.md (already present)
- [ ] Configure release-please changelog-sections to match documented types

## References

- gedcom-go PR #55: CI gate job and pre-commit fix
- gedcom-go PR #53: Coverage enforcement
- gedcom-go PR #57: PR title enforcement and commit type definitions
