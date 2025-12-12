# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## Project Overview

Self-hosted genealogy software written in Go.

## Build Commands

```bash
go build ./...          # Build all packages
go test ./...           # Run all tests
go test -v ./... -run TestName  # Run a specific test
go fmt ./...            # Format code
go vet ./...            # Static analysis
```

## Architecture

*To be documented as the codebase develops.*

## Active Technologies
- Go 1.22+ + Echo (HTTP router), Ent (data layer), oapi-codegen (OpenAPI), github.com/cacack/gedcom-go (GEDCOM processing), Svelte 5 + Vite + D3.js + Tailwind CSS (frontend) (001-genealogy-mvp)
- PostgreSQL (primary, required for future pgvector/PostGIS), SQLite (local/demo fallback) (001-genealogy-mvp)

## Linked Library Development

This project uses `github.com/cacack/gedcom-go` via a `replace` directive pointing to `/Users/chris/devel/home/gedcom-go`. When changes to gedcom-go are needed:

1. **Keep changes atomic**: Each gedcom-go enhancement should be a single logical unit (e.g., "add entity parsing", not mixed with unrelated fixes)
2. **Always add tests**: Any new gedcom-go functionality must include tests in that repo
3. **Run both test suites**: After gedcom-go changes, run `go test ./...` in both repos
4. **Commit separately**: gedcom-go changes should be committed to that repo independently, with their own descriptive commit message
5. **Document the dependency**: If adding new gedcom-go features, note what my-family feature required them

## Recent Changes
- 001-genealogy-mvp: Added Go 1.22+ + Echo (HTTP router), Ent (data layer), oapi-codegen (OpenAPI), github.com/cacack/gedcom-go (GEDCOM processing), Svelte 5 + Vite + D3.js + Tailwind CSS (frontend)
