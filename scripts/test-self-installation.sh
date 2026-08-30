#!/usr/bin/env bash
set -euo pipefail

repo_root="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
temporary="$(mktemp -d)"
trap 'rm -rf -- "$temporary"' EXIT

bash -n \
  "$repo_root/infra/installer/install.sh" \
  "$repo_root/infra/installer/ship" \
  "$repo_root/scripts/build-production-images.sh" \
  "$repo_root/scripts/generate-client.sh" \
  "$repo_root/scripts/package-release.sh" \
  "$repo_root/scripts/seed.sh" \
  "$repo_root/scripts/test-generated-client.sh" \
  "$repo_root/scripts/test-image.sh"

compose=(docker compose --env-file "$repo_root/.env.example" -f "$repo_root/infra/compose/docker-compose.yml")
services="$("${compose[@]}" config --services | sort | tr '\n' ' ')"
expected="ship-api ship-postgres ship-redis ship-web ship-worker "
[[ "$services" == "$expected" ]] || {
  printf 'expected exactly five Ship services; got: %s\n' "$services" >&2
  exit 1
}

"${compose[@]}" config --format json >"$temporary/compose.json"
jq -e '
  (.services | length) == 5 and
  (.services["ship-web"].ports[0].published == "3000") and
  (.services["ship-web"].ports[0].host_ip == null) and
  (.services["ship-api"].ports[0].host_ip == "127.0.0.1") and
  (.services["ship-postgres"].ports == null) and
  (.services["ship-redis"].ports == null) and
  (.services["ship-worker"].ports == null) and
  (.networks["ship-data"].internal == true) and
  (.volumes.ship_pgdata != null) and
  (.volumes.ship_data != null)
' "$temporary/compose.json" >/dev/null

install_root="$temporary/install"
export SHIP_INSTALL_SOURCE_DIR="$repo_root"
export SHIP_INSTALL_DIR="$install_root"
export SHIP_BIN_DIR="$temporary/bin"
export SHIP_SKIP_SYSTEM_PACKAGES=1
export SHIP_SKIP_START=1
export SHIP_ALLOW_NON_ROOT=1
export SHIP_ALLOW_UNSUPPORTED_OS=1

bash "$repo_root/infra/installer/install.sh" --hostname 127.0.0.1 >"$temporary/first.log"
first_keyring="$(sha256sum "$install_root/keys/keyring" | awk '{print $1}')"
bash "$repo_root/infra/installer/install.sh" --hostname ship.example.com >"$temporary/second.log"
second_keyring="$(sha256sum "$install_root/keys/keyring" | awk '{print $1}')"
bash "$repo_root/infra/installer/install.sh" >"$temporary/third.log"

[[ "$first_keyring" == "$second_keyring" ]]
if env_mode="$(stat -c '%a' "$install_root/.env" 2>/dev/null)"; then
  :
else
  env_mode="$(stat -f '%Lp' "$install_root/.env")"
fi
[[ "$env_mode" == "600" ]]
if keyring_mode="$(stat -c '%a' "$install_root/keys/keyring" 2>/dev/null)"; then
  :
else
  keyring_mode="$(stat -f '%Lp' "$install_root/keys/keyring")"
fi
[[ "$keyring_mode" == "600" ]]
grep -q 'First-run token (shown once)' "$temporary/first.log"
grep -q 'Ship is ready at http://127.0.0.1:3000' "$temporary/first.log"
if grep -q 'First-run token (shown once)' "$temporary/second.log"; then
  echo "idempotent install printed a new first-run token" >&2
  exit 1
fi
if grep -q 'First-run token (shown once)' "$temporary/third.log"; then
  echo "idempotent install printed a new first-run token" >&2
  exit 1
fi
grep -q '^SHIP_HOSTNAME=ship.example.com$' "$install_root/.env"
grep -q '^SHIP_PUBLIC_URL=http://ship.example.com:3000$' "$install_root/.env"
grep -q '^SHIP_ALLOW_INSECURE_HTTP=true$' "$install_root/.env"
grep -q '^active [a-f0-9]\{16\}$' "$install_root/keys/keyring"

"$repo_root/scripts/package-release.sh" v0.1.0 "$temporary/dist"
(
  cd "$temporary/dist"
  sha256sum -c ./*.sha256
)

echo "Self-installation contract passed"
