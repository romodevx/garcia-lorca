#!/bin/sh
# Applies every migrations/**/*.sql in lexical order (iam/0001, iam/0002,
# students/0001, …) as the Postgres superuser, so tables are owned by the
# migrator (not the app role). Idempotent SQL is the migration author's job;
# this runner just orders and applies. Used by the compose `migrate` one-shot
# and by `make migrate`.
set -e

: "${PGHOST:=postgres}"
: "${PGUSER:=postgres}"
: "${PGDATABASE:=garcia_lorca}"
export PGPASSWORD="${POSTGRES_PASSWORD:?POSTGRES_PASSWORD must be set}"

echo "[migrate] applying migrations to ${PGDATABASE}@${PGHOST} as ${PGUSER}"
for f in $(find /migrations -name '*.sql' | sort); do
  echo "[migrate] -> $f"
  psql -v ON_ERROR_STOP=1 -h "$PGHOST" -U "$PGUSER" -d "$PGDATABASE" -f "$f"
done
echo "[migrate] done"
