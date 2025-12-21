<objective>
Update documentation and configuration to formalize commit conventions:
1. Document commit types in CONTRIBUTING.md
2. Configure release-please changelog sections
3. Create a checklist for GitHub settings that require manual configuration

These changes ensure consistent commit practices and proper changelog generation.
</objective>

<context>
This is a Go project using conventional commits and release-please for versioning.

Read @HANDOFF.md for the commit type definitions and release-please config.
Read @CONTRIBUTING.md to understand existing contribution guidelines.
Read @release-please-config.json if it exists.
</context>

<requirements>
1. **Document Commit Types in CONTRIBUTING.md**
   Add a section explaining the commit type conventions:

   | Type | Use for | Appears in Changelog? |
   |------|---------|----------------------|
   | `feat` | Library/app capabilities | Yes |
   | `fix` | Bug fixes in library/app | Yes |
   | `perf` | Performance improvements | Yes |
   | `refactor` | Code restructuring | No |
   | `test` | Test changes | No |
   | `docs` | Documentation | No |
   | `ci` | Dev infrastructure, deps, tooling | No |
   | `chore` | Miscellaneous maintenance | No |

   Key rule: `feat`/`fix` are reserved for user-facing changes, not development tooling.

   Also document the PR title convention:
   - Commit messages: Use conventional commit format (`type(scope): description`)
   - PR titles: Use descriptive format (NOT conventional commit)
   - Why: Prevents duplicate changelog entries with release-please

2. **Configure Release-Please Changelog Sections**
   Update `release-please-config.json` to specify which types appear in changelog:
   ```json
   "changelog-sections": [
     {"type": "feat", "section": "Features"},
     {"type": "fix", "section": "Bug Fixes"},
     {"type": "perf", "section": "Performance"}
   ]
   ```

3. **GitHub Settings Checklist**
   Create or append to a file documenting manual GitHub settings needed:

   - [ ] Branch protection: Require only "CI Success" check (not individual jobs)
   - [ ] Branch protection: Enable "Require branches to be up to date before merging"
   - [ ] Repository settings: Enable "Always suggest updating pull request branches"

   These enable semi-linear history (GitLab-style rebasing).
</requirements>

<output>
Modify: `./CONTRIBUTING.md` - Add commit conventions section
Modify or create: `./release-please-config.json` - Add changelog-sections
Create: `./docs/GITHUB-SETTINGS.md` - Manual settings checklist
</output>

<verification>
Before completing:
- Validate JSON syntax in release-please-config.json
- Ensure CONTRIBUTING.md additions are well-integrated with existing content
- Confirm the GitHub settings checklist is clear and actionable
</verification>

<success_criteria>
- CONTRIBUTING.md has complete commit type documentation
- CONTRIBUTING.md explains PR title vs commit message conventions
- release-please-config.json has changelog-sections for feat, fix, perf
- GitHub settings checklist exists with 3 action items
</success_criteria>
