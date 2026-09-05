#!/usr/bin/env bash
set -Eeuo pipefail

readonly DEFAULT_VERSION="v0.1.0"
readonly DEFAULT_RELEASE_BASE="https://github.com/Jonath-z/ship/releases/download"

install_dir="${SHIP_INSTALL_DIR:-/opt/ship}"
bin_dir="${SHIP_BIN_DIR:-/usr/local/bin}"
hostname_flag="${SHIP_HOSTNAME:-}"
version_flag="${SHIP_VERSION:-}"
first_run_token=""

log() {
  printf 'ship: %s\n' "$*"
}

die() {
  printf 'ship: error: %s\n' "$*" >&2
  exit 1
}

usage() {
  cat <<'EOF'
Install Ship on Ubuntu 22.04/24.04 or Debian 12.

Usage:
  install.sh [--hostname HOST_OR_IP] [--version vX.Y.Z]

The dashboard starts on http://HOST:3000. Configure HTTPS afterwards with
"ship public-url https://ship.example.com".

Environment:
  SHIP_INSTALL_DIR         installation directory (default: /opt/ship)
  SHIP_INSTALL_SOURCE_DIR  local repository/bundle directory for development
  SHIP_RELEASE_BASE_URL    release download base URL
  SHIP_SKIP_SYSTEM_PACKAGES=1  skip Docker package installation
  SHIP_SKIP_START=1        prepare files without starting the stack
EOF
}

while (( $# > 0 )); do
  case "$1" in
    --hostname)
      (( $# >= 2 )) || die "--hostname requires a value"
      hostname_flag="$2"
      shift 2
      ;;
    --version)
      (( $# >= 2 )) || die "--version requires a value"
      version_flag="$2"
      shift 2
      ;;
    --help|-h)
      usage
      exit 0
      ;;
    *)
      die "unknown option: $1"
      ;;
  esac
done

if [[ "${EUID}" -ne 0 && "${SHIP_ALLOW_NON_ROOT:-0}" != "1" ]]; then
  die "run the installer as root (curl -fsSL https://get.ship.dev | sudo bash)"
fi

random_hex() {
  openssl rand -hex "$1"
}

# env_get KEY prints the stored value from an existing installation, or nothing.
env_get() {
  [[ -f "$install_dir/.env" ]] || return 0
  awk -F= -v key="$1" '$1 == key {print $2; exit}' "$install_dir/.env"
}

# env_get_or KEY DEFAULT keeps operator-tuned values across re-installs.
env_get_or() {
  local value
  value="$(env_get "$1")"
  printf '%s' "${value:-$2}"
}

os_id=""
os_version=""
architecture=""

detect_platform() {
  if [[ -r /etc/os-release ]]; then
    # shellcheck disable=SC1091
    source /etc/os-release
    os_id="${ID:-}"
    os_version="${VERSION_ID:-}"
  elif [[ "${SHIP_ALLOW_UNSUPPORTED_OS:-0}" == "1" ]]; then
    os_id="unsupported"
    os_version="unknown"
  else
    die "cannot identify this operating system"
  fi

  case "$os_id:$os_version" in
    ubuntu:22.04|ubuntu:24.04|debian:12) ;;
    *)
      if [[ "${SHIP_ALLOW_UNSUPPORTED_OS:-0}" != "1" ]]; then
        die "supported systems are Ubuntu 22.04/24.04 and Debian 12; found $os_id $os_version"
      fi
      ;;
  esac

  case "$(uname -m)" in
    x86_64|amd64) architecture="amd64" ;;
    aarch64|arm64) architecture="arm64" ;;
    *) die "supported architectures are amd64 and arm64" ;;
  esac
}

install_system_packages() {
  if [[ "${SHIP_SKIP_SYSTEM_PACKAGES:-0}" == "1" ]]; then
    command -v docker >/dev/null || die "Docker is required when package installation is skipped"
    return
  fi

  export DEBIAN_FRONTEND=noninteractive
  apt-get update
  apt-get install -y ca-certificates curl gnupg openssl

  if ! command -v docker >/dev/null || ! docker compose version >/dev/null 2>&1; then
    log "installing Docker from Docker's official APT repository"
    install -m 0755 -d /etc/apt/keyrings
    curl -fsSL "https://download.docker.com/linux/$os_id/gpg" |
      gpg --dearmor --yes -o /etc/apt/keyrings/docker.gpg
    chmod a+r /etc/apt/keyrings/docker.gpg

    local codename
    codename="$(. /etc/os-release && printf '%s' "${VERSION_CODENAME}")"
    printf 'deb [arch=%s signed-by=/etc/apt/keyrings/docker.gpg] https://download.docker.com/linux/%s %s stable\n' "$architecture" "$os_id" "$codename" >/etc/apt/sources.list.d/docker.list
    apt-get update
    apt-get install -y docker-ce docker-ce-cli containerd.io docker-buildx-plugin docker-compose-plugin
  fi

  systemctl enable --now docker
}

ship_version=""
hostname_value=""
public_url=""

# resolve_choices decides version, hostname, and public URL: a flag wins, then
# the existing installation, then detection or the default. A new --hostname
# resets the URL to bootstrap HTTP; otherwise a URL set later through
# "ship public-url" survives re-installs.
resolve_choices() {
  ship_version="${version_flag:-$(env_get SHIP_VERSION)}"
  ship_version="${ship_version:-$DEFAULT_VERSION}"
  [[ "$ship_version" =~ ^v[0-9]+\.[0-9]+\.[0-9]+([.-][A-Za-z0-9._-]+)?$ ]] ||
    die "version must look like v1.2.3"

  hostname_value="${hostname_flag:-$(env_get SHIP_HOSTNAME)}"
  if [[ -z "$hostname_value" ]]; then
    hostname_value="$(curl -4fsS --max-time 5 https://api.ipify.org || true)"
  fi
  if [[ -z "$hostname_value" ]]; then
    hostname_value="$(hostname -I 2>/dev/null | awk '{print $1}')"
  fi
  [[ -n "$hostname_value" ]] ||
    die "could not detect a public IP; re-run with --hostname ship.example.com"
  [[ "$hostname_value" =~ ^([A-Za-z0-9]([A-Za-z0-9.-]*[A-Za-z0-9])?|[0-9]+(\.[0-9]+){3})$ ]] ||
    die "hostname must be a DNS name or IPv4 address"

  public_url="$(env_get SHIP_PUBLIC_URL)"
  if [[ -n "$hostname_flag" || -z "$public_url" ]]; then
    public_url="http://$hostname_value:3000"
  fi
}

copy_bundle() {
  local source_dir="${SHIP_INSTALL_SOURCE_DIR:-}"
  local release_base="${SHIP_RELEASE_BASE_URL:-$DEFAULT_RELEASE_BASE}"
  local temporary
  temporary="$(mktemp -d)"

  install -d -m 0755 "$install_dir"
  install -d -m 0700 "$install_dir/backups"

  if [[ -n "$source_dir" ]]; then
    if [[ -f "$source_dir/infra/compose/docker-compose.yml" ]]; then
      # Source checkout: the ship CLI is a Go binary, so this path needs a
      # local Go toolchain. Release bundles carry a prebuilt binary instead.
      command -v go >/dev/null || die "installing from a source checkout requires Go to build the ship CLI"
      install -m 0644 "$source_dir/infra/compose/docker-compose.yml" "$install_dir/compose.yml"
      (cd "$source_dir" && CGO_ENABLED=0 go build -o "$install_dir/ship" ./server/cmd/ship)
      chmod 0755 "$install_dir/ship"
    else
      install -m 0644 "$source_dir/compose.yml" "$install_dir/compose.yml"
      install -m 0755 "$source_dir/ship" "$install_dir/ship"
    fi
  else
    local archive="ship-$ship_version-$architecture.tar.gz"
    log "downloading Ship $ship_version for $architecture"
    curl -fsSL "$release_base/$ship_version/$archive" -o "$temporary/$archive"
    curl -fsSL "$release_base/$ship_version/$archive.sha256" -o "$temporary/$archive.sha256"
    (
      cd "$temporary"
      sha256sum -c "$archive.sha256"
      tar -xzf "$archive"
    )
    install -m 0644 "$temporary/compose.yml" "$install_dir/compose.yml"
    install -m 0755 "$temporary/ship" "$install_dir/ship"
  fi

  install -d -m 0755 "$bin_dir"
  install -m 0755 "$install_dir/ship" "$bin_dir/ship"
  rm -rf -- "$temporary"
}

ensure_keyring() {
  local key_directory="$install_dir/keys"
  local keyring="$key_directory/keyring"
  install -d -m 0700 "$key_directory"
  if [[ ! -f "$keyring" ]]; then
    local key_id
    key_id="$(random_hex 8)"
    umask 077
    {
      printf 'active %s\n' "$key_id"
      printf 'key %s %s\n' "$key_id" "$(random_hex 32)"
    } >"$keyring"
  fi
  chmod 0600 "$keyring"
  if [[ "$EUID" -eq 0 ]]; then
    chown 10001:10001 "$key_directory" "$keyring"
  fi
}

# write_environment rewrites .env as one document on every run. Secrets are
# read back first, so re-running the installer never regenerates them.
write_environment() {
  local session_secret postgres_password token_hash docker_gid
  if [[ -f "$install_dir/.env" ]]; then
    log "preserving the existing database and secrets"
  fi
  session_secret="$(env_get SHIP_SESSION_SECRET)"
  postgres_password="$(env_get POSTGRES_PASSWORD)"
  token_hash="$(env_get SHIP_FIRST_RUN_TOKEN_HASH)"
  [[ -n "$session_secret" ]] || session_secret="$(random_hex 32)"
  [[ -n "$postgres_password" ]] || postgres_password="$(random_hex 24)"
  if [[ -z "$token_hash" ]]; then
    first_run_token="$(random_hex 24)"
    token_hash="$(printf '%s' "$first_run_token" | sha256sum | awk '{print $1}')"
  fi
  docker_gid="$(stat -c '%g' /var/run/docker.sock 2>/dev/null || printf '0')"

  local allow_insecure_http=true trust_forwarded_ip=false
  if [[ "$public_url" == https://* ]]; then
    allow_insecure_http=false
    trust_forwarded_ip=true
  fi

  umask 077
  {
    printf 'SHIP_VERSION=%s\n' "$ship_version"
    printf 'SHIP_IMAGE_REGISTRY=ghcr.io/jonath-z\n'
    printf 'SHIP_COMPOSE_PROJECT_NAME=ship\n'
    printf 'SHIP_HOSTNAME=%s\n' "$hostname_value"
    printf 'SHIP_PUBLIC_URL=%s\n' "$public_url"
    printf 'SHIP_ALLOW_INSECURE_HTTP=%s\n' "$allow_insecure_http"
    printf 'SHIP_TRUST_FORWARDED_IP=%s\n' "$trust_forwarded_ip"
    printf 'SHIP_WEB_PORT=%s\n' "$(env_get_or SHIP_WEB_PORT 3000)"
    printf 'SHIP_API_PORT=%s\n' "$(env_get_or SHIP_API_PORT 8080)"
    printf 'SHIP_LOG_LEVEL=%s\n' "$(env_get_or SHIP_LOG_LEVEL info)"
    printf 'SHIP_KEYRING_PATH=/run/ship/keys/keyring\n'
    printf 'SHIP_SESSION_SECRET=%s\n' "$session_secret"
    printf 'SHIP_SESSION_IDLE_TTL=%s\n' "$(env_get_or SHIP_SESSION_IDLE_TTL 24h)"
    printf 'SHIP_SESSION_ABSOLUTE_TTL=%s\n' "$(env_get_or SHIP_SESSION_ABSOLUTE_TTL 168h)"
    printf 'SHIP_FIRST_RUN_TOKEN_HASH=%s\n' "$token_hash"
    printf 'POSTGRES_USER=ship\n'
    printf 'POSTGRES_PASSWORD=%s\n' "$postgres_password"
    printf 'POSTGRES_DB=ship\n'
    printf 'POSTGRES_IMAGE=postgres:16.14-alpine3.23\n'
    printf 'REDIS_IMAGE=redis:7.4.11-alpine3.21\n'
    printf 'DOCKER_GID=%s\n' "$docker_gid"
  } >"$install_dir/.env.next"
  chmod 0600 "$install_dir/.env.next"
  mv "$install_dir/.env.next" "$install_dir/.env"
  ensure_keyring
}

start_ship() {
  if [[ "${SHIP_SKIP_START:-0}" == "1" ]]; then
    log "prepared installation files without starting Ship"
    return
  fi
  (
    cd "$install_dir"
    docker compose --env-file .env -f compose.yml config --quiet
    docker compose --env-file .env -f compose.yml pull
    docker compose --env-file .env -f compose.yml up -d --wait --remove-orphans
  )
}

main() {
  detect_platform
  log "detected $os_id $os_version on $architecture"
  install_system_packages
  resolve_choices
  copy_bundle
  write_environment
  start_ship

  printf '\nShip is ready at %s\n' "$public_url"
  if [[ -n "$first_run_token" ]]; then
    printf 'First-run token (shown once): %s\n' "$first_run_token"
    printf 'Open %s/setup to create the owner account.\n' "$public_url"
  else
    printf 'Existing installation updated without changing its secrets or data.\n'
  fi
  if [[ "$public_url" != https://* ]]; then
    printf 'Warning: this is temporary insecure bootstrap access. Configure external HTTPS before production use.\n'
  fi
  printf 'Run "ship status" to inspect the control plane.\n'
}

main
