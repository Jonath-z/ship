#!/usr/bin/env bash
# Verify that the committed TypeScript clients match the Go-owned OpenAPI spec.
set -euo pipefail

repo_root="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
spec="$repo_root/server/internal/platform/httpx/openapi.yaml"
types_client="$repo_root/packages/types/src/generated.ts"
api_client="$repo_root/packages/api-client/src/generated/schema.ts"
temporary_directory="$(mktemp -d "${TMPDIR:-/tmp}/ship-generated-client.XXXXXX")"

cleanup() {
  rm -rf "$temporary_directory"
}
trap cleanup EXIT

cd "$repo_root"
pnpm exec openapi-typescript "$spec" --output "$temporary_directory/types.ts"
pnpm exec openapi-typescript "$spec" --output "$temporary_directory/schema.ts"

stale=0
if ! cmp -s "$types_client" "$temporary_directory/types.ts"; then
  echo "packages/types/src/generated.ts is stale; run: pnpm generate" >&2
  diff -u "$types_client" "$temporary_directory/types.ts" || true
  stale=1
fi

if ! cmp -s "$api_client" "$temporary_directory/schema.ts"; then
  echo "packages/api-client/src/generated/schema.ts is stale; run: pnpm generate" >&2
  diff -u "$api_client" "$temporary_directory/schema.ts" || true
  stale=1
fi

exit "$stale"
