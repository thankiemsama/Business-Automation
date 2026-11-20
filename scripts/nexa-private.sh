#!/usr/bin/env bash
set -euo pipefail

usage() {
  echo 'Usage: bash nexa-private.sh pack BUNDLE_DIR [--with-ai] | install BUNDLE_DIR' >&2
  exit 2
}

[[ $# -ge 2 && $# -le 3 ]] || usage
action=$1
destination=$2
[[ "$action" == pack || "$action" == install ]] || usage
[[ $# -eq 2 || ( "$action" == pack && "$3" == --with-ai ) ]] || usage

command -v docker >/dev/null || { echo 'Docker is required.' >&2; exit 1; }
docker compose version >/dev/null

bundle_image() {
  local source=$1
  local local_name=$2
  docker pull "$source"
  docker tag "$source" "$local_name"
}

if [[ "$action" == pack ]]; then
  repo=$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)
  mkdir -- "$destination"
  bundle=$(cd "$destination" && pwd)
  cp -- "$repo/compose.private.yaml" "$bundle/"
  cp -- "$repo/scripts/nexa-private.sh" "$bundle/"
  docker build -t nexa-private/backend:bundle "$repo/BACKEND"
  docker build -t nexa-private/frontend:bundle "$repo/FRONTEND"
  bundle_image "${NEXA_PRIVATE_POSTGRES_IMAGE:-postgres:16}" nexa-private/postgres:bundle
  bundle_image "${NEXA_MINIO_IMAGE:-docker.io/pgsty/silo:RELEASE.2026-09-16T00-00-00Z@sha256:635197cb9f36d01bee221d34d1c7d7960f6a95c48b0b6c01d99cd13bdae51a46}" nexa-private/object-store:bundle
  bundle_image "${NEXA_MINIO_CLIENT_IMAGE:-docker.io/pgsty/mc:RELEASE.2026-09-16T00-00-00Z}" nexa-private/object-client:bundle
  images=(nexa-private/backend:bundle nexa-private/frontend:bundle nexa-private/postgres:bundle nexa-private/object-store:bundle nexa-private/object-client:bundle)
  if [[ "${3:-}" == --with-ai ]]; then
    bundle_image "${NEXA_PRIVATE_OLLAMA_IMAGE:-ollama/ollama:0.40.8}" nexa-private/ollama:bundle
    images+=(nexa-private/ollama:bundle)
  fi
  docker save --output "$bundle/images.tar" "${images[@]}"
  (cd "$bundle" && sha256sum compose.private.yaml nexa-private.sh images.tar > SHA256SUMS)
  printf 'Bundle ready: %s\n' "$bundle"
  exit 0
fi

cd "$destination"
sha256sum --strict --check SHA256SUMS
docker load --input images.tar
umask 077
mkdir -p -- models
if [[ ! -f private.env ]]; then
  command -v openssl >/dev/null || { echo 'OpenSSL is required for initial secret generation.' >&2; exit 1; }
  db_password=$(openssl rand -hex 24)
  storage_password=$(openssl rand -hex 24)
  (
    set -o noclobber
    printf 'NEXA_PRIVATE_DB_PASSWORD=%s\nNEXA_PRIVATE_STORAGE_PASSWORD=%s\n' "$db_password" "$storage_password" > private.env
  )
fi
docker compose --env-file private.env -f compose.private.yaml up -d --no-build --pull never --wait --wait-timeout 180
