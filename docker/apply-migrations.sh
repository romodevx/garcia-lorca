#!/bin/sh
# Applies every migrations/**/*.sql in lexical order (iam/0001, iam/0002,
# students/0001, …) as the Postgres superuser, so tables are owned by the
# migrator (not the app role). A schema_migrations ledger records applied files
# by their path relative to /migrations, so re-runs skip already-applied
# migrations (idempotent). Used by the compose `migrate` one-shot and by
# `make migrate`.
#
# NOTE: files are applied in lexical order of their full path, so all of iam/
# precedes students/. This is a single shared database; a future cross-module
# dependency whose order does not match alphabetical directory order would need
# a rename or an explicit ordering scheme.
set -e

: "${PGHOST:=postgres}"
: "${PGUSER:=postgres}"
: "${PGDATABASE:=garcia_lorca}"
export PGPASSWORD="${POSTGRES_PASSWORD:?POSTGRES_PASSWORD must be set}"

psql_do() {
  psql -v ON_ERROR_STOP=1 -h "$PGHOST" -U "$PGUSER" -d "$PGDATABASE" "$@"
}

echo "[migrate] applying migrations to ${PGDATABASE}@${PGHOST} as ${PGUSER}"

# Ledger of applied migrations (idempotency). filename is the path relative to
# /migrations, e.g. 'iam/0001_init.sql'.
psql_do -c "CREATE TABLE IF NOT EXISTS schema_migrations (filename text PRIMARY KEY, applied_at timestamptz NOT NULL DEFAULT now());"

find /migrations -name '*.sql' | sort | while IFS= read -r f; do
  name="${f#/migrations/}"
  already="$(psql_do -tAc "SELECT 1 FROM schema_migrations WHERE filename = '${name}';")"
  if [ "$already" = "1" ]; then
    echo "[migrate] skip (already applied) -> ${name}"
    continue
  fi
  echo "[migrate] apply -> ${name}"
  psql_do -f "$f"
  psql_do -c "INSERT INTO schema_migrations (filename) VALUES ('${name}');"
done

# The while loop runs in a subshell (pipeline), so re-derive the count here to
# fail fast when no migration files were found (wrong mount / empty dir).
if [ "$(find /migrations -name '*.sql' | wc -l | tr -d ' ')" -eq 0 ]; then
  echo "[migrate] ERROR: no *.sql files found under /migrations" >&2
  exit 1
fi

# Owner/director user seed. Runs only when OWNER_TEMP_PASSWORD is set; the seed
# hashes it with pgcrypto (bcrypt) at apply time, so no credential is committed.
# It is idempotent (ON CONFLICT DO NOTHING), so re-runs are a no-op; it is NOT
# recorded in schema_migrations (it is an env-driven seed, not schema).
if [ -n "${OWNER_TEMP_PASSWORD:-}" ]; then
  echo "[migrate] seeding owner user from OWNER_TEMP_PASSWORD"
  psql_do -v owner_temp_password="$OWNER_TEMP_PASSWORD" -f /seed/owner.sql
else
  echo "[migrate] OWNER_TEMP_PASSWORD not set — skipping owner seed" >&2
fi

echo "[migrate] done"
