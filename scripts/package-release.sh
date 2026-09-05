#!/usr/bin/env bash
set -euo pipefail

repo_root="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
# shellcheck disable=SC1091
source "$repo_root/infra/versions.env"

version="${1:-$SHIP_DEFAULT_VERSION}"
output_dir="${2:-$repo_root/dist}"
[[ "$version" =~ ^v[0-9]+\.[0-9]+\.[0-9]+([.-][A-Za-z0-9._-]+)?$ ]] || {
  echo "version must look like v1.2.3" >&2
  exit 1
}

mkdir -p "$output_dir"
temporary="$(mktemp -d)"
trap 'rm -rf -- "$temporary"' EXIT
install -m 0644 "$repo_root/infra/compose/docker-compose.yml" "$temporary/compose.yml"

for architecture in amd64 arm64; do
  (
    cd "$repo_root"
    CGO_ENABLED=0 GOOS=linux GOARCH="$architecture" \
      go build -trimpath -ldflags '-s -w' -o "$temporary/ship" ./server/cmd/ship
  )
  chmod 0755 "$temporary/ship"
  archive="ship-$version-$architecture.tar.gz"
  tar -C "$temporary" -czf "$output_dir/$archive" compose.yml ship
  (
    cd "$output_dir"
    sha256sum "$archive" >"$archive.sha256"
  )
done

printf 'Release bundle written to %s\n' "$output_dir"
