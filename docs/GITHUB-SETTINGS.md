# GitHub Settings Checklist

Manual configuration required for repository settings to enable semi-linear history (GitLab-style rebasing).

## Branch Protection Rules (main branch)

- [ ] **Require status checks**: Enable only "CI Success" check (not individual job names)
  - Location: Settings → Branches → Branch protection rules → main → Require status checks to pass before merging
  - Check: "Require status checks to pass before merging"
  - Select: "CI Success" (from the list of available checks)
  - Why: Allows adding/removing CI jobs without updating branch protection rules

- [ ] **Require branches to be up to date**: Force PRs to be rebased on main before merging
  - Location: Settings → Branches → Branch protection rules → main → Require status checks to pass before merging
  - Check: "Require branches to be up to date before merging"
  - Why: Ensures semi-linear history and prevents stale branches from being merged

## Repository Settings

- [ ] **Always suggest updating pull request branches**: Show rebase/merge button on PR page
  - Location: Settings → General → Pull Requests
  - Check: "Always suggest updating pull request branches"
  - Why: Provides UI for contributors to update their branches before merge

## Notes

- GitHub cannot enforce rebase-only updates; both merge and rebase options are shown
- Team discipline is required to use "Rebase and merge" instead of "Merge commit" when updating branches
- For strict enforcement, consider automation tools like Mergify or GitHub Actions

## References

- HANDOFF.md: Developer Experience Improvements from gedcom-go
- Related: CI Success gate job in `.github/workflows/ci.yml`
