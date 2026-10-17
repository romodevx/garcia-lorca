#!/usr/bin/env bash
# The whole García Lorca system LOCALLY, "as in production".
#
#   scripts/prod-local.sh build            build the 10 production images, ONE AT A TIME
#   scripts/prod-local.sh up               build if missing, stop the dev app containers,
#                                          migrate, start, and wait until every port answers
#   scripts/prod-local.sh status           probe every port (exit 1 if any is down)
#   scripts/prod-local.sh logs [service]   follow logs
#   scripts/prod-local.sh down             stop it and start again the dev containers `up` stopped
#
# Read deploy/PROD-LOCAL.md first. The short version of the design is in the
# header of docker-compose.prod-local.yml.
#
# Health is decided by PROBING PORTS, never by `docker ps`: a Go service whose
# binary died at boot still shows "Up" there (see the dev stack's history).
set -euo pipefail

REPO_ROOT=$(CDPATH='' cd -- "$(dirname "$0")/.." && pwd)
cd "$REPO_ROOT"

readonly PROJECT=garcia-lorca-prod
readonly DEV_PROJECT=garcia-lorca
readonly COMPOSE_FILE=docker-compose.prod-local.yml
readonly ENV_FILE=.env
readonly SECRETS_FILE=.env.prod-local
readonly STATE_DIR=.prod-local
readonly STOPPED_DEV_FILE="$STATE_DIR/stopped-dev-containers"
readonly IMAGE_PREFIX=garcia-lorca-prod
readonly BUF_IMAGE=bufbuild/buf:1.50.0
readonly HEALTH_TIMEOUT_SECONDS=150
readonly PROBE_TIMEOUT_SECONDS=3

# Build order = deploy/cloudbuild/cloudbuild.yaml. SEQUENTIAL ON PURPOSE: parallel
# image builds on one Docker daemon die with "unexpected EOF" and are slower.
readonly GO_SERVICES=(gateway iam students config finance grades attendance comms uniforms)

# service:host-port:path. Every one must answer 200.
readonly PROBES=(
  "iam:18091:/healthz"
  "students:18092:/healthz"
  "finance:18093:/healthz"
  "grades:18094:/healthz"
  "attendance:18095:/healthz"
  "comms:18096:/healthz"
  "uniforms:18099:/healthz"
  "config:18101:/healthz"
  "gateway:18080:/readyz"
  "web:13000:/healthz"
)

# The dev stack's app containers. Postgres stays up: it IS the shared database.
readonly DEV_APP_SERVICES=(web gateway iam students config finance grades attendance comms docs uniforms billing)

# Flags dev GCP has ON that terraform does not list yet: they were switched on
# with `gcloud run services update` (ui_refresh 2026-10-12, announcements_view
# 2026-10-13). Override with PROD_LOCAL_EXTRA_MODULES="" to match terraform only.
readonly DEFAULT_EXTRA_MODULES="ui_refresh,announcements_view"

die() { echo "[prod-local] ERROR: $*" >&2; exit 1; }
say() { echo "[prod-local] $*"; }

compose() {
  docker compose -p "$PROJECT" -f "$COMPOSE_FILE" \
    --env-file "$ENV_FILE" --env-file "$SECRETS_FILE" "$@"
}

env_value() { # env_value KEY FILE -> value, or empty
  sed -n "s/^$1=//p" "$2" | tail -n 1
}

# ---- configuration ----------------------------------------------------------

require_env() {
  [ -f "$ENV_FILE" ] || die "$ENV_FILE not found. It holds the local DB passwords: cp .env.example .env (or run 'make run' once)."
  [ -n "$(env_value APP_DB_PASSWORD "$ENV_FILE")" ] || die "APP_DB_PASSWORD missing in $ENV_FILE"
  [ -n "$(env_value POSTGRES_PASSWORD "$ENV_FILE")" ] || die "POSTGRES_PASSWORD missing in $ENV_FILE"
}

# Stable signing keys, generated ONCE and kept in a gitignored file. A key that
# changed on every start would log everyone out on every restart, which is the
# exact production failure (scale-to-zero + ephemeral key) this stack must not hide.
ensure_secrets() {
  if [ ! -f "$SECRETS_FILE" ]; then
    say "generating $SECRETS_FILE (stable local signing keys; values are never printed)"
    umask 077
    {
      echo "# Local-only keys for scripts/prod-local.sh. Gitignored. Never reuse in a shared env."
      echo "PROD_LOCAL_IAM_JWT_SECRET=$(openssl rand -hex 32)"
      echo "PROD_LOCAL_LEVEL_DELETION_TOKEN_KEY=$(openssl rand -hex 32)"
    } >"$SECRETS_FILE"
  fi
  for key in PROD_LOCAL_IAM_JWT_SECRET PROD_LOCAL_LEVEL_DELETION_TOKEN_KEY; do
    [ -n "$(env_value "$key" "$SECRETS_FILE")" ] || die "$key empty in $SECRETS_FILE (delete the file to regenerate)"
  done
}

# ENABLED_MODULES exactly as terraform gives the dev gateway, plus the flags dev
# enabled out of band. Read from the file, not from gcloud.
resolve_modules() {
  if [ -n "${PROD_LOCAL_ENABLED_MODULES:-}" ]; then
    export PROD_LOCAL_ENABLED_MODULES
    return
  fi
  local tf
  tf=$(sed -n 's/^[[:space:]]*ENABLED_MODULES[[:space:]]*=[[:space:]]*"\([^"]*\)".*/\1/p' deploy/terraform/services.tf | head -n 1)
  [ -n "$tf" ] || die "could not read ENABLED_MODULES from deploy/terraform/services.tf"
  local extra="${PROD_LOCAL_EXTRA_MODULES-$DEFAULT_EXTRA_MODULES}"
  PROD_LOCAL_ENABLED_MODULES="$tf${extra:+,$extra}"
  export PROD_LOCAL_ENABLED_MODULES
}

# ---- build ------------------------------------------------------------------

ensure_generated_code() {
  if [ -d gen/go/garcialorca ]; then
    return
  fi
  say "gen/go is empty: running buf generate (pinned $BUF_IMAGE, in Docker)"
  docker run --rm -v "$REPO_ROOT":/workspace -w /workspace "$BUF_IMAGE" generate
  [ -d gen/go/garcialorca ] || die "buf generate produced no gen/go/garcialorca"
}

build_images() {
  ensure_generated_code
  export DOCKER_BUILDKIT=1
  local svc
  for svc in "${GO_SERVICES[@]}"; do
    say "building $IMAGE_PREFIX/$svc:local (services/$svc/Dockerfile)"
    docker build -f "services/$svc/Dockerfile" -t "$IMAGE_PREFIX/$svc:local" . \
      || die "image $svc failed to build"
  done
  # Same build args Cloud Build passes, with the identity provider OFF (empty),
  # so the web signs in with the gateway's password login.
  say "building $IMAGE_PREFIX/web:local (apps/web/Dockerfile, next build standalone)"
  docker build -f apps/web/Dockerfile \
    --build-arg NEXT_PUBLIC_API_URL=/ \
    --build-arg NEXT_PUBLIC_AUTH_PROVIDER= \
    -t "$IMAGE_PREFIX/web:local" . || die "image web failed to build"
  check_web_image_has_no_sourcemaps
}

# The build-hardening gate, applied to what the image actually SERVES: the
# browser bundle (.next/static) and public/. Same scope as the CI gate
# (apps/web/scripts/check-no-sourcemaps.mjs). Next also writes server-side maps
# under .next/server; those are never sent to a browser (`up` checks that over
# HTTP), so they are counted, not failed.
check_web_image_has_no_sourcemaps() {
  local found server_maps
  # A failing `docker run` (no image, daemon down) must stop the build, so no
  # `|| true` here. Inside, grep exits 1 when it finds NOTHING (the good case),
  # so only exit codes above 1 are errors and the OUTPUT is the verdict.
  found=$(docker run --rm --entrypoint sh "$IMAGE_PREFIX/web:local" -c \
    'cd /app/apps/web || exit 2
     find .next/static public -name "*.map" || exit 2
     grep -rlE "sourceMappingURL=" .next/static public; [ $? -le 1 ] || exit 2') \
    || die "could not inspect the web image (docker run failed)"
  if [ -n "$found" ]; then
    echo "$found" >&2
    die "the web image's CLIENT bundle contains source maps or sourceMappingURL comments"
  fi
  server_maps=$(docker run --rm --entrypoint sh "$IMAGE_PREFIX/web:local" -c \
    'find /app/apps/web/.next/server -name "*.map" | wc -l' | tr -d ' ')
  say "web image: client bundle has no *.map and no sourceMappingURL (server-only maps, never served: $server_maps)"
}

# Over HTTP, which is what a browser can actually reach: every chunk the login
# page references must come without a sourceMappingURL, and asking for its .map
# must not return a source map. NOT "must 404": the optional catch-all route
# app/[[...slug]] answers ANY unknown path, a missing asset included, with the
# HTML shell and a 200. So the test is on the body: a v3 map is JSON that starts
# with {"version":3.
check_web_serves_no_sourcemaps() {
  local base=http://127.0.0.1:13000 chunks chunk body chunk_body
  chunks=$(curl -s --max-time 10 "$base/login" | grep -oE '/_next/static/[^"]+\.js' | sort -u)
  [ -n "$chunks" ] || die "could not list the web's JS chunks from /login"
  for chunk in $chunks; do
    # Captured first: `curl | grep -q` under pipefail fails when grep exits early.
    chunk_body=$(curl -s --max-time 10 "$base$chunk") || die "could not fetch $chunk"
    if grep -q 'sourceMappingURL=' <<<"$chunk_body"; then
      die "served chunk $chunk carries a sourceMappingURL comment"
    fi
    body=$(curl -s --max-time 10 "$base$chunk.map" | head -c 64 || true)
    case "$body" in
      *'"version"'*|*'"mappings"'*) die "$chunk.map is served as a source map" ;;
    esac
  done
  say "web serves no source maps ($(echo "$chunks" | wc -l | tr -d ' ') chunks checked; no chunk links a map, no .map URL returns one)"
}

images_present() {
  local svc
  for svc in "${GO_SERVICES[@]}" web; do
    docker image inspect "$IMAGE_PREFIX/$svc:local" >/dev/null 2>&1 || return 1
  done
}

# ---- the shared database ----------------------------------------------------

port_open() { nc -z -w "$PROBE_TIMEOUT_SECONDS" 127.0.0.1 "$1" >/dev/null 2>&1; }

require_database() {
  local pg_port
  pg_port=$(env_value POSTGRES_HOST_PORT "$ENV_FILE")
  pg_port=${pg_port:-5432}
  if ! port_open "$pg_port"; then
    say "Postgres is not answering on :$pg_port; starting the dev stack's postgres container"
    docker start "$DEV_PROJECT-postgres-1" >/dev/null 2>&1 \
      || die "no $DEV_PROJECT-postgres-1 container. Create the dev database once: docker compose up -d postgres"
  fi
  local waited=0
  until docker exec "$DEV_PROJECT-postgres-1" pg_isready -U postgres -d garcia_lorca >/dev/null 2>&1 \
    && port_open "$pg_port"; do
    [ "$waited" -lt 60 ] || die "Postgres on :$pg_port did not become ready in 60 s"
    sleep 2; waited=$((waited + 2))
  done
  say "database: dev stack Postgres on :$pg_port (volume ${DEV_PROJECT}_pgdata)"
}

stop_dev_app_containers() {
  mkdir -p "$STATE_DIR"
  local svc name stopped=()
  for svc in "${DEV_APP_SERVICES[@]}"; do
    name="$DEV_PROJECT-$svc-1"
    if [ "$(docker inspect -f '{{.State.Running}}' "$name" 2>/dev/null || true)" = "true" ]; then
      docker stop "$name" >/dev/null
      stopped+=("$name")
    fi
  done
  if [ "${#stopped[@]}" -gt 0 ]; then
    printf '%s\n' "${stopped[@]}" >>"$STOPPED_DEV_FILE"
    sort -u "$STOPPED_DEV_FILE" -o "$STOPPED_DEV_FILE"
    say "stopped dev app containers (restored by 'down'): ${stopped[*]}"
  fi
}

restore_dev_app_containers() {
  [ -s "$STOPPED_DEV_FILE" ] || return 0
  local names
  names=$(tr '\n' ' ' <"$STOPPED_DEV_FILE")
  say "starting again the dev containers 'up' stopped: $names"
  # shellcheck disable=SC2086 # one name per word on purpose
  docker start $names >/dev/null
  rm -f "$STOPPED_DEV_FILE"
}

# ---- health -----------------------------------------------------------------

probe() { # probe PORT PATH -> HTTP code (000 when nothing answers)
  curl -s -o /dev/null -w '%{http_code}' --max-time "$PROBE_TIMEOUT_SECONDS" "http://127.0.0.1:$1$2" || true
}

all_healthy() {
  local entry svc port path
  for entry in "${PROBES[@]}"; do
    IFS=: read -r svc port path <<<"$entry"
    [ "$(probe "$port" "$path")" = "200" ] || return 1
  done
}

print_status() {
  local entry svc port path code bad=0
  for entry in "${PROBES[@]}"; do
    IFS=: read -r svc port path <<<"$entry"
    code=$(probe "$port" "$path")
    if [ "$code" = "200" ]; then
      printf '  %-11s :%-6s %-9s OK\n' "$svc" "$port" "$path"
    else
      printf '  %-11s :%-6s %-9s DOWN (HTTP %s)\n' "$svc" "$port" "$path" "$code"
      bad=1
    fi
  done
  return "$bad"
}

wait_healthy() {
  local waited=0
  until all_healthy; do
    if [ "$waited" -ge "$HEALTH_TIMEOUT_SECONDS" ]; then
      print_status || true
      local entry svc port path
      for entry in "${PROBES[@]}"; do
        IFS=: read -r svc port path <<<"$entry"
        if [ "$(probe "$port" "$path")" != "200" ]; then
          echo "---- last log lines of $svc ----" >&2
          compose logs --tail 20 "$svc" >&2 || true
        fi
      done
      die "not healthy after ${HEALTH_TIMEOUT_SECONDS} s (see above)"
    fi
    sleep 3; waited=$((waited + 3))
  done
}

# ---- commands ---------------------------------------------------------------

cmd_build() { require_env; build_images; }

cmd_up() {
  require_env
  ensure_secrets
  resolve_modules
  images_present || build_images
  require_database
  stop_dev_app_containers
  say "ENABLED_MODULES=$PROD_LOCAL_ENABLED_MODULES"
  # `run` (not up) for the one-shot, so a failed migration stops here, loudly.
  compose up -d pubsub fake-gcs
  if ! compose run --rm migrate; then
    restore_dev_app_containers
    die "migrations failed; nothing else was started (dev app containers restored)"
  fi
  compose up -d --no-deps gateway iam students config finance grades attendance comms uniforms web
  say "waiting for every port to answer (up to ${HEALTH_TIMEOUT_SECONDS} s)"
  wait_healthy
  print_status
  check_web_serves_no_sourcemaps
  say "UP: web http://localhost:13000  ·  gateway http://localhost:18080  ·  Android emulator: http://10.0.2.2:18080 (bound to ${PROD_LOCAL_BIND:-127.0.0.1})"
}

cmd_status() {
  require_env
  print_status || die "at least one service is down"
}

cmd_logs() {
  require_env
  ensure_secrets
  resolve_modules
  compose logs -f --tail 100 "$@"
}

cmd_down() {
  require_env
  ensure_secrets
  resolve_modules
  compose down --remove-orphans
  restore_dev_app_containers
}

case "${1:-}" in
  build) cmd_build ;;
  up) cmd_up ;;
  status) cmd_status ;;
  logs) shift; cmd_logs "$@" ;;
  down) cmd_down ;;
  *) echo "usage: $0 build|up|status|logs [service]|down" >&2; exit 2 ;;
esac
