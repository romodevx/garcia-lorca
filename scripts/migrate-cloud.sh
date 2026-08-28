#!/bin/sh
# Applies the schema to a deployed environment's Postgres, from your machine.
#
# Mirrors docker/apply-migrations.sh (same ledger, same lexical order) but talks
# to a remote host over TLS with a local psql instead of `docker exec`.
#
# ONE ROLE, ONE CONNECTION, ON PURPOSE. setup_app_role.sql ends with
# ALTER DEFAULT PRIVILEGES, which is PER-GRANTOR: it only affects tables created
# by the role that ran it. Running the role setup as one identity and the
# migrations as another leaves `app` with no rights on any new table, and every
# query fails with "permission denied for table" the first time it is used. So
# this script does both steps itself, with one DATABASE_URL.
#
# With an external Postgres (Neon) that is all you need. Against Cloud SQL with
# no public IP you would first start the Cloud SQL Auth Proxy and point
# DATABASE_URL at 127.0.0.1.
#
# Usage:
#   DATABASE_URL='postgresql://USER:PASS@HOST/garcia_lorca?sslmode=require' \
#     sh scripts/migrate-cloud.sh
#
#   # also seed the owner login (first run only; the owner rotates it at login):
#   DATABASE_URL=... OWNER_TEMP_PASSWORD='...' sh scripts/migrate-cloud.sh
set -eu

REPO_ROOT=$(CDPATH= cd -- "$(dirname "$0")/.." && pwd)
MIGRATIONS_DIR="$REPO_ROOT/migrations"
ROLE_SQL="$REPO_ROOT/deploy/terraform/db/setup_app_role.sql"
OWNER_SQL="$REPO_ROOT/docker/seed-owner.sql"

# ---------------------------------------------------------------------------
# Connection. Two shapes, and the script works out which one applies.
#
#   1. DATABASE_URL set  -> use it verbatim (external Postgres, or a proxy you
#      started yourself).
#   2. Otherwise         -> Cloud SQL. Read the instance and the migration
#      credentials from Terraform + Secret Manager, start the Cloud SQL Auth
#      Proxy on a local port, and tear it down on exit.
#
# The Cloud SQL instance has a public IP with NO authorized networks, so a direct
# psql from this machine cannot connect and is not supposed to. The proxy
# authenticates through the Admin API as roles/cloudsql.client instead.
# ---------------------------------------------------------------------------
PROXY_PID=""
cleanup() { [ -n "$PROXY_PID" ] && kill "$PROXY_PID" 2>/dev/null || true; }
trap cleanup EXIT INT TERM

if ! command -v psql >/dev/null 2>&1; then
  echo "[migrate] psql not found. Install it:  brew install libpq && brew link --force libpq" >&2
  exit 1
fi

if [ -z "${DATABASE_URL:-}" ]; then
  : "${PROJECT_ID:=garcia-lorca}"
  : "${ENV_NAME:=dev}"
  : "${DB_NAME:=garcia_lorca}"
  : "${PROXY_PORT:=5433}"

  if ! command -v cloud-sql-proxy >/dev/null 2>&1; then
    echo "[migrate] cloud-sql-proxy not found. Install it:" >&2
    echo "[migrate]   brew install cloud-sql-proxy" >&2
    echo "[migrate] (or set DATABASE_URL yourself and skip the proxy)" >&2
    exit 1
  fi

  CONNECTION_NAME="${INSTANCE_CONNECTION_NAME:-$(terraform -chdir="$REPO_ROOT/deploy/terraform" output -raw db_connection_name 2>/dev/null || true)}"
  if [ -z "$CONNECTION_NAME" ]; then
    echo "[migrate] cannot resolve the Cloud SQL instance." >&2
    echo "[migrate] Run terraform apply first, or set INSTANCE_CONNECTION_NAME=project:region:instance" >&2
    exit 1
  fi

  echo "[migrate] reading the migration credentials from Secret Manager"
  MIGRATOR_PASSWORD=$(gcloud secrets versions access latest \
    --secret="garcia-lorca-db-migrator-password-${ENV_NAME}" \
    --project="$PROJECT_ID" 2>/dev/null || true)
  if [ -z "$MIGRATOR_PASSWORD" ]; then
    echo "[migrate] could not read garcia-lorca-db-migrator-password-${ENV_NAME}." >&2
    echo "[migrate] Terraform creates it; run the apply first." >&2
    exit 1
  fi

  echo "[migrate] starting the Cloud SQL Auth Proxy on 127.0.0.1:${PROXY_PORT}"
  cloud-sql-proxy --port "$PROXY_PORT" "$CONNECTION_NAME" >/tmp/cloud-sql-proxy.log 2>&1 &
  PROXY_PID=$!

  # The proxy needs a moment; poll rather than sleep a fixed amount.
  READY=""
  i=0
  while [ "$i" -lt 30 ]; do
    if psql "postgresql://migrator:${MIGRATOR_PASSWORD}@127.0.0.1:${PROXY_PORT}/${DB_NAME}?sslmode=disable" \
         -tAc 'SELECT 1' >/dev/null 2>&1; then
      READY=yes
      break
    fi
    i=$((i + 1))
    sleep 1
  done
  if [ -z "$READY" ]; then
    echo "[migrate] the proxy never became usable. Its log:" >&2
    tail -20 /tmp/cloud-sql-proxy.log >&2
    echo "[migrate] Common cause: your account lacks roles/cloudsql.client on $PROJECT_ID." >&2
    exit 1
  fi

  # sslmode=disable is correct HERE and only here: the hop is to a local proxy
  # process, and the proxy itself holds the encrypted connection to the instance.
  DATABASE_URL="postgresql://migrator:${MIGRATOR_PASSWORD}@127.0.0.1:${PROXY_PORT}/${DB_NAME}?sslmode=disable"
fi

# Redact credentials before echoing anything.
SAFE_TARGET=$(printf '%s' "$DATABASE_URL" | sed -E 's|://[^@]*@|://***@|')
echo "[migrate] target: $SAFE_TARGET"

echo "[migrate] checking connectivity"
psql_do -tAc 'SELECT current_user, current_database();'

echo "[migrate] applying role posture (deploy/terraform/db/setup_app_role.sql)"
psql_do -f "$ROLE_SQL"

echo "[migrate] ledger"
psql_do -c "CREATE TABLE IF NOT EXISTS schema_migrations (filename text PRIMARY KEY, applied_at timestamptz NOT NULL DEFAULT now());"

COUNT=$(find "$MIGRATIONS_DIR" -name '*.sql' | wc -l | tr -d ' ')
if [ "$COUNT" -eq 0 ]; then
  echo "[migrate] ERROR: no *.sql under $MIGRATIONS_DIR" >&2
  exit 1
fi
echo "[migrate] $COUNT migration files found"

# Lexical order over the full relative path, same as the compose runner: all of
# iam/ precedes students/. There are no cross-domain foreign keys, so this holds.
find "$MIGRATIONS_DIR" -name '*.sql' | sort | while IFS= read -r f; do
  name="${f#"$MIGRATIONS_DIR"/}"
  already=$(psql_do -tAc "SELECT 1 FROM schema_migrations WHERE filename = '${name}';")
  if [ "$already" = "1" ]; then
    echo "[migrate] skip  -> ${name}"
    continue
  fi
  echo "[migrate] apply -> ${name}"
  psql_do -f "$f"
  psql_do -c "INSERT INTO schema_migrations (filename) VALUES ('${name}');"
done

if [ -n "${OWNER_TEMP_PASSWORD:-}" ]; then
  echo "[migrate] seeding the owner user"
  psql_do -v owner_temp_password="$OWNER_TEMP_PASSWORD" -f "$OWNER_SQL"
else
  echo "[migrate] OWNER_TEMP_PASSWORD not set — skipping the owner seed."
  echo "[migrate] Without it nobody can log in; read it from Secret Manager:"
  echo "[migrate]   gcloud secrets versions access latest --secret=owner-temp-password-dev --project=garcia-lorca"
fi

echo "[migrate] done"
