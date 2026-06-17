#!/usr/bin/env bash
# Hot-reload entrypoint for a single Go service under docker compose.
#
# The whole monorepo is mounted at /workspace (so go.work + every module is
# visible). `air` watches only this service's dir + pkg + gen/go and rebuilds
# ./services/$SERVICE/cmd on change. Used ONLY for local dev — prod images are
# the distroless multi-stage Dockerfiles under services/*/Dockerfile.
set -euo pipefail

: "${SERVICE:?set SERVICE to the service name, e.g. iam}"

# go.work lists ./gen/go, which is gitignored and produced by `buf generate`.
# Until codegen runs, drop a stub module so the workspace loads offline. Real
# generated code (make gen) overwrites this directory.
if [ ! -f /workspace/gen/go/go.mod ]; then
  mkdir -p /workspace/gen/go
  printf 'module github.com/garcia-lorca/gen/go\n\ngo 1.24\n' > /workspace/gen/go/go.mod
fi

TMP="/tmp/air-${SERVICE}"
CONF="/tmp/air-${SERVICE}.toml"
mkdir -p "$TMP"

# Per-service air config. tmp_dir lives outside /workspace so the build binary
# never lands in the mounted repo; include_dir scopes the watcher to this
# service (+ shared pkg/gen) so node_modules and the other apps are ignored.
cat > "$CONF" <<TOML
root = "/workspace"
tmp_dir = "${TMP}"

[build]
  cmd = "go build -o ${TMP}/server ./services/${SERVICE}/cmd"
  bin = "${TMP}/server"
  full_bin = "${TMP}/server"
  include_dir = ["services/${SERVICE}", "pkg", "gen/go"]
  exclude_dir = ["node_modules", "apps", "packages", ".git", ".turbo", "deploy", "docker"]
  include_ext = ["go", "mod", "sum"]
  delay = 600
  stop_on_error = true
  send_interrupt = true
  kill_delay = "2s"

[log]
  main_only = true
TOML

echo "[dev-entrypoint] starting air for service '${SERVICE}' (cmd: ./services/${SERVICE}/cmd)"
exec air -c "$CONF"
