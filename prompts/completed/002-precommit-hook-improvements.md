<objective>
Enhance the pre-commit hook with two improvements from the gedcom-go project:
1. Make tool discovery more robust (check multiple PATH locations)
2. Add per-package test coverage enforcement

These changes prevent commit failures due to PATH issues and ensure consistent test coverage across all packages.
</objective>

<context>
This is a Go project. The pre-commit hook runs linting and tests before commits.

Read @HANDOFF.md for detailed implementation examples.
Examine existing pre-commit hook: @scripts/pre-commit or @.git/hooks/pre-commit
</context>

<requirements>
1. **Robust Tool Discovery**
   - Check multiple locations for golangci-lint:
     1. `command -v golangci-lint` (PATH)
     2. `$HOME/go/bin/golangci-lint`
     3. `$(go env GOPATH)/bin/golangci-lint`
   - Provide clear error message if not found anywhere
   - Why: Developers often install Go tools to ~/go/bin which may not be in PATH

2. **Per-Package Coverage Enforcement**
   - Check each package individually for 85% minimum coverage
   - List all packages that fail the threshold
   - Show coverage percentage for each package
   - Exit with error if any package is below threshold
   - Why: Overall coverage can hide undertested packages
</requirements>

<implementation>
Reference the exact implementations in HANDOFF.md:
- Tool discovery pattern (lines 56-66)
- Per-package coverage loop (lines 145-157)

Adapt the package list in the coverage loop to match this project's structure.
Use `go list ./...` or examine the project to identify packages.
</implementation>

<output>
Modify or create: `./scripts/pre-commit`

If creating new, ensure it's executable and document installation in the file header.
</output>

<verification>
Before completing:
- Verify script syntax: `bash -n scripts/pre-commit`
- Confirm the script is executable: `chmod +x scripts/pre-commit`
- List packages that will be checked: `go list ./...`
- Test tool discovery logic works for common installations
</verification>

<success_criteria>
- Pre-commit hook finds golangci-lint in PATH, ~/go/bin, or GOPATH/bin
- Per-package coverage check runs on each package
- Clear output showing pass/fail status for each package
- Script exits non-zero if any package below 85% coverage
</success_criteria>
