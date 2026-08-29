#!/usr/bin/env bash
# Regenerate TypeScript types and the API client from the Go API's OpenAPI spec.
# The Go API is the source of truth; TS is always downstream (spec §48).
set -euo pipefail
echo "TODO(SH-005): emit openapi.yaml from Go, then generate into"
echo "  packages/types/src and packages/api-client/src/generated"
