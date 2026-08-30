#!/usr/bin/env bash
set -euo pipefail

image="${1:?usage: test-image.sh IMAGE api|worker|web}"
kind="${2:?usage: test-image.sh IMAGE api|worker|web}"
max_size=$((200 * 1024 * 1024))

size="$(docker image inspect "$image" --format '{{.Size}}')"
user="$(docker image inspect "$image" --format '{{.Config.User}}')"
health="$(docker image inspect "$image" --format '{{json .Config.Healthcheck.Test}}')"
source_label="$(docker image inspect "$image" --format '{{index .Config.Labels "org.opencontainers.image.source"}}')"

if (( size >= max_size )); then
  echo "$image is $size bytes; the limit is $max_size" >&2
  exit 1
fi
case "$user" in
  ""|0|0:0|root|root:root)
    echo "$image runs as root" >&2
    exit 1
    ;;
esac
if [[ "$health" == "null" || -z "$health" ]]; then
  echo "$image has no container health check" >&2
  exit 1
fi
if [[ "$source_label" != "https://github.com/Jonath-z/ship" ]]; then
  echo "$image has an invalid source label: $source_label" >&2
  exit 1
fi

case "$kind" in
  api)
    docker run --rm "$image" -version | grep -q '^ship-api '
    ;;
  worker)
    docker run --rm "$image" -version | grep -q '^ship-worker '
    docker run --rm --entrypoint kamal "$image" version | grep -Eq '^2\.12\.0$'
    docker run --rm --entrypoint docker "$image" --version
    docker run --rm --entrypoint ssh "$image" -V
    ;;
  web)
    docker run --rm --entrypoint node "$image" --version
    ;;
  *)
    echo "unknown image kind: $kind" >&2
    exit 1
    ;;
esac

echo "$image: size=$size user=$user health=present"
