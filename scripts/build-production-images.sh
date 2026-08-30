#!/usr/bin/env bash
set -euo pipefail

repo_root="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
# shellcheck disable=SC1091
source "$repo_root/infra/versions.env"

tag="${1:-local}"
build_sha="${BUILD_SHA:-development}"
source_url="${SOURCE_URL:-https://github.com/Jonath-z/ship}"

docker build \
  --build-arg "GO_BUILDER_IMAGE=$GO_BUILDER_IMAGE" \
  --build-arg "BUILD_SHA=$build_sha" \
  --build-arg "VERSION=$tag" \
  --build-arg "SOURCE_URL=$source_url" \
  -f "$repo_root/infra/docker/api.Dockerfile" \
  -t "ship-api:$tag" "$repo_root"

docker build \
  --build-arg "GO_BUILDER_IMAGE=$GO_BUILDER_IMAGE" \
  --build-arg "RUBY_IMAGE=$RUBY_IMAGE" \
  --build-arg "KAMAL_VERSION=$KAMAL_VERSION" \
  --build-arg "BUILD_SHA=$build_sha" \
  --build-arg "VERSION=$tag" \
  --build-arg "SOURCE_URL=$source_url" \
  -f "$repo_root/infra/docker/worker.Dockerfile" \
  -t "ship-worker:$tag" "$repo_root"

docker build \
  --build-arg "NODE_IMAGE=$NODE_IMAGE" \
  --build-arg "BUILD_SHA=$build_sha" \
  --build-arg "VERSION=$tag" \
  --build-arg "SOURCE_URL=$source_url" \
  -f "$repo_root/infra/docker/web.Dockerfile" \
  -t "ship-web:$tag" "$repo_root"
