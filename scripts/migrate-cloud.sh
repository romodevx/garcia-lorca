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

if [ -z "${DATABASE_URL:-}" ]; then
  echo "[migrate] DATABASE_URL is required." >&2
  echo "[migrate]   DATABASE_URL='postgresql://user:pass@host/garcia_lorca?sslmode=require' sh $0" >&2
  exit 1
fi

if ! command -v psql >/dev/null 2>&1; then
  echo "[migrate] psql not found. Install it:  brew install libpq && brew link --force libpq" >&2
  exit 1
fi

# Everything goes through this one connection string, so every statement runs as
# the same role — which is what makes ALTER DEFAULT PRIVILEGES mean anything.
psql_do() { psql "$DATABASE_URL" -v ON_ERROR_STOP=1 "$@"; }

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
