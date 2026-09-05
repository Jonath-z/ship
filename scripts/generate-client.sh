#!/usr/bin/env bash
# Regenerate the API client schema from the Go-owned spec.
set -euo pipefail

repo_root="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
spec="$repo_root/server/internal/platform/httpx/openapi.yaml"

cd "$repo_root"
pnpm exec openapi-typescript "$spec" --output packages/api-client/src/generated/schema.ts
