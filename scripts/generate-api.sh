#!/bin/bash
# Generate both Go and TypeScript code from OpenAPI spec
set -e

echo "=== Generating API code from OpenAPI spec ==="
echo ""

# Go types and server interface
echo "Generating Go code..."
go generate ./internal/api/...
echo "  internal/api/generated.go"

# TypeScript types
echo "Generating TypeScript types..."
cd web && npm run generate:types --silent
cd ..
echo "  web/src/lib/api/types.generated.ts"

echo ""
echo "Done. Remember to commit generated files."
