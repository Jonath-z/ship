#!/usr/bin/env bash
set -Eeuo pipefail

readonly DEFAULT_VERSION="v0.1.0"
readonly DEFAULT_RELEASE_BASE="https://github.com/Jonath-z/ship/releases/download"

install_dir="${SHIP_INSTALL_DIR:-/opt/ship}"
bin_dir="${SHIP_BIN_DIR:-/usr/local/bin}"
hostname_value="${SHIP_HOSTNAME:-}"
public_url="${SHIP_PUBLIC_URL:-}"
ship_version="${SHIP_VERSION:-$DEFAULT_VERSION}"
hostname_explicit=false
public_url_explicit=false
version_explicit=false
if [[ -n "${SHIP_HOSTNAME:-}" ]]; then
  hostname_explicit=true
fi
if [[ -n "${SHIP_VERSION:-}" ]]; then
  version_explicit=true
fi
if [[ -n "${SHIP_PUBLIC_URL:-}" ]]; then
  public_url_explicit=true
fi
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
  install.sh [--hostname HOST_OR_IP] [--public-url URL] [--version vX.Y.Z]

Environment:
  SHIP_INSTALL_DIR          installation directory (default: /opt/ship)
  SHIP_INSTALL_SOURCE_DIR  local repository/bundle directory for development
  SHIP_RELEASE_BASE_URL    release download base URL
  SHIP_PUBLIC_URL          externally reachable URL (HTTPS for production)
  SHIP_SKIP_SYSTEM_PACKAGES=1  skip Docker package installation
  SHIP_SKIP_START=1        prepare files without starting the stack
EOF
}

while (( $# > 0 )); do
  case "$1" in
    --hostname)
      (( $# >= 2 )) || die "--hostname requires a value"
      hostname_value="$2"
      hostname_explicit=true
      shift 2
      ;;
    --version)
      (( $# >= 2 )) || die "--version requires a value"
      ship_version="$2"
      version_explicit=true
      shift 2
      ;;
    --public-url)
      (( $# >= 2 )) || die "--public-url requires a value"
      public_url="$2"
      public_url_explicit=true
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

[[ "$ship_version" =~ ^v[0-9]+\.[0-9]+\.[0-9]+([.-][A-Za-z0-9._-]+)?$ ]] ||
  die "version must look like v1.2.3"

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

detect_hostname() {
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
}

# Re-running the installer keeps whatever the operator chose before, unless a
# flag overrides it. One subtlety: a new --hostname without --public-url clears
# the stored URL so it is re-derived from the new hostname.
load_existing_choices() {
  local env_file="$install_dir/.env"
  if [[ ! -f "$env_file" ]]; then
    return 0
  fi
  if [[ "$version_explicit" == "false" ]]; then
    ship_version="$(awk -F= '$1 == "SHIP_VERSION" {print $2; exit}' "$env_file")"
  fi
  if [[ "$hostname_explicit" == "false" ]]; then
    hostname_value="$(awk -F= '$1 == "SHIP_HOSTNAME" {print $2; exit}' "$env_file")"
  fi
  if [[ "$public_url_explicit" == "false" ]]; then
    if [[ "$hostname_explicit" == "true" ]]; then
      public_url=""
    else
      public_url="$(awk -F= '$1 == "SHIP_PUBLIC_URL" {print $2; exit}' "$env_file")"
    fi
  fi
}

configure_public_url() {
  if [[ -z "$public_url" ]]; then
    public_url="http://$hostname_value:3000"
  fi
  [[ "$public_url" =~ ^https?://([A-Za-z0-9]([A-Za-z0-9.-]*[A-Za-z0-9])?|[0-9]+(\.[0-9]+){3})(:[0-9]{1,5})?$ ]] ||
    die "public URL must look like https://ship.example.com or http://SERVER_IP:3000"
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
      install -m 0644 "$source_dir/infra/compose/docker-compose.yml" "$install_dir/compose.yml"
      install -m 0755 "$source_dir/infra/installer/ship" "$install_dir/ship"
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

random_hex() {
  openssl rand -hex "$1"
}

sha256_value() {
  printf '%s' "$1" | sha256sum | awk '{print $1}'
}

set_env_value() {
  local key="$1"
  local value="$2"
  local file="$install_dir/.env"
  local temporary
  temporary="$(mktemp "$install_dir/.env.XXXXXX")"
  awk -v key="$key" -v value="$value" '
    BEGIN { replaced = 0 }
    $0 ~ ("^" key "=") {
      print key "=" value
      replaced = 1
      next
    }
    { print }
    END {
      if (!replaced) {
        print key "=" value
      }
    }
  ' "$file" >"$temporary"
  chmod 0600 "$temporary"
  mv "$temporary" "$file"
}

ensure_env_value() {
  local key="$1"
  local value="$2"
  if ! awk -F= -v key="$key" '$1 == key && length($2) > 0 { found = 1 } END { exit !found }' "$install_dir/.env"; then
    set_env_value "$key" "$value"
  fi
}

ensure_keyring() {
  local key_directory="$install_dir/keys"
  local keyring="$key_directory/keyring"
  install -d -m 0700 "$key_directory"
  if [[ -f "$keyring" ]]; then
    chmod 0600 "$keyring"
    if [[ "$EUID" -eq 0 ]]; then
      chown 10001:10001 "$key_directory" "$keyring"
    fi
    return
  fi

  local key_value
  local key_id
  key_value="$(random_hex 32)"
  key_id="$(random_hex 8)"
  umask 077
  {
    printf 'active %s\n' "$key_id"
    printf 'key %s %s\n' "$key_id" "$key_value"
  } >"$keyring"
  chmod 0600 "$keyring"
  if [[ "$EUID" -eq 0 ]]; then
    chown 10001:10001 "$key_directory" "$keyring"
  fi
}

write_environment() {
  local env_file="$install_dir/.env"
  local docker_gid
  docker_gid="$(stat -c '%g' /var/run/docker.sock 2>/dev/null || printf '0')"

  if [[ ! -f "$env_file" ]]; then
    umask 077
    first_run_token="$(random_hex 24)"
    {
      printf 'SHIP_VERSION=%s\n' "$ship_version"
      printf 'SHIP_IMAGE_REGISTRY=ghcr.io/jonath-z\n'
      printf 'SHIP_COMPOSE_PROJECT_NAME=ship\n'
      printf 'SHIP_HOSTNAME=%s\n' "$hostname_value"
      printf 'SHIP_PUBLIC_URL=%s\n' "$public_url"
      if [[ "$public_url" == https://* ]]; then
        printf 'SHIP_ALLOW_INSECURE_HTTP=false\n'
        printf 'SHIP_TRUST_FORWARDED_IP=true\n'
      else
        printf 'SHIP_ALLOW_INSECURE_HTTP=true\n'
        printf 'SHIP_TRUST_FORWARDED_IP=false\n'
      fi
      printf 'SHIP_WEB_PORT=3000\n'
      printf 'SHIP_API_PORT=8080\n'
      printf 'SHIP_LOG_LEVEL=info\n'
      printf 'SHIP_KEYRING_PATH=/run/ship/keys/keyring\n'
      printf 'SHIP_SESSION_SECRET=%s\n' "$(random_hex 32)"
      printf 'SHIP_SESSION_IDLE_TTL=24h\n'
      printf 'SHIP_SESSION_ABSOLUTE_TTL=168h\n'
      printf 'SHIP_FIRST_RUN_TOKEN_HASH=%s\n' "$(sha256_value "$first_run_token")"
      printf 'POSTGRES_USER=ship\n'
      printf 'POSTGRES_PASSWORD=%s\n' "$(random_hex 24)"
      printf 'POSTGRES_DB=ship\n'
      printf 'POSTGRES_IMAGE=postgres:16.14-alpine3.23\n'
      printf 'REDIS_IMAGE=redis:7.4.11-alpine3.21\n'
      printf 'DOCKER_GID=%s\n' "$docker_gid"
    } >"$env_file"
    chmod 0600 "$env_file"
  else
    log "preserving the existing database and secrets"
    set_env_value SHIP_VERSION "$ship_version"
    set_env_value SHIP_HOSTNAME "$hostname_value"
    set_env_value SHIP_PUBLIC_URL "$public_url"
    if [[ "$public_url" == https://* ]]; then
      set_env_value SHIP_ALLOW_INSECURE_HTTP false
      set_env_value SHIP_TRUST_FORWARDED_IP true
    else
      set_env_value SHIP_ALLOW_INSECURE_HTTP true
      set_env_value SHIP_TRUST_FORWARDED_IP false
    fi
    set_env_value DOCKER_GID "$docker_gid"
    ensure_env_value SHIP_KEYRING_PATH /run/ship/keys/keyring
    ensure_env_value SHIP_SESSION_SECRET "$(random_hex 32)"
    ensure_env_value SHIP_SESSION_IDLE_TTL 24h
    ensure_env_value SHIP_SESSION_ABSOLUTE_TTL 168h
  fi
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
  load_existing_choices
  [[ "$ship_version" =~ ^v[0-9]+\.[0-9]+\.[0-9]+([.-][A-Za-z0-9._-]+)?$ ]] ||
    die "existing SHIP_VERSION is invalid"
  detect_hostname
  configure_public_url
  copy_bundle
  write_environment
  start_ship

  local installed_web_port
  installed_web_port="$(awk -F= '$1 == "SHIP_WEB_PORT" {print $2; exit}' "$install_dir/.env")"
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
