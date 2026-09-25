#!/bin/sh
# Empties the operational data and leaves ONLY the seeded user, so a demo can
# show the school being configured from nothing.
#
# It does NOT touch: users, roles, permissions, or schema_migrations. Dropping
# the owner would lock the demo out of its own system, and dropping the migration
# ledger would make the next `make migrate` try to re-apply everything.
#
# TRUNCATE ... CASCADE, in one statement, with RESTART IDENTITY so folio
# counters and sequences start over — a demo that opens on "U-2026-000042" has
# some explaining to do.
#
# TWO TRANSPORTS. Default is `docker exec` against the compose container. Set
# PGHOST to talk to a real host with a local psql instead — the header used to
# claim that worked while the code called docker exec unconditionally, so it
# never did.
#
# THIS SCRIPT DESTROYS DATA. Against anything that is not localhost it refuses
# unless you pass --i-know-this-wipes-remote. That guard exists because the one
# realistic way to lose a demo is to run a wipe against the cloud the night
# before showing it, and until now the only thing standing in the way was a
# sentence in a markdown file.
#
# Usage:  sh scripts/reset-demo-db.sh                         (compose)
#         PGHOST=127.0.0.1 PGPASSWORD=… sh scripts/reset-demo-db.sh
#         PGHOST=db.example PGPASSWORD=… sh scripts/reset-demo-db.sh --i-know-this-wipes-remote
set -e

: "${CONTAINER:=garcia-lorca-postgres-1}"
: "${PGUSER:=postgres}"
: "${PGDATABASE:=garcia_lorca}"

CONFIRMED=""
for arg in "$@"; do
  [ "$arg" = "--i-know-this-wipes-remote" ] && CONFIRMED="yes"
done

# CONFIRMATION IS REQUIRED FOR ANY NON-COMPOSE TRANSPORT.
#
# Two smarter designs were tried and both failed, so this one is deliberately
# dumb. Checking PGHOST does not work: the Cloud SQL Auth Proxy listens on
# 127.0.0.1, so a production instance looks local. Asking the server where it
# lives does not work either: through that proxy inet_server_addr() returns NULL,
# which is indistinguishable from a unix-socket connection to a local Postgres.
#
# So: `docker exec` against the compose container is the only transport that is
# unambiguously local. Anything else — any PGHOST at all — has to say so out loud.
# A guard that can be fooled is worse than one that asks twice.
if [ -n "${PGHOST:-}" ] && [ -z "$CONFIRMED" ]; then
  echo "[reset] REFUSING: PGHOST is set, so this is not the local compose database." >&2
  echo "[reset] (The Cloud SQL Auth Proxy makes a remote instance look like 127.0.0.1," >&2
  echo "[reset]  so the address alone cannot be trusted to tell you where you are.)" >&2
  echo "[reset] This TRUNCATEs every operational table. If you really mean it," >&2
  echo "[reset] re-run with --i-know-this-wipes-remote." >&2
  exit 1
fi
if [ -n "$CONFIRMED" ]; then
  echo "[reset] WARNING: destructive wipe confirmed on the command line"
fi

# psql_do runs one psql invocation against whichever transport applies.
psql_do() {
  if [ -n "${PGHOST:-}" ]; then
    psql -h "$PGHOST" -p "${PGPORT:-5432}" -U "$PGUSER" -d "$PGDATABASE" "$@"
  else
    docker exec -i "$CONTAINER" psql -U "$PGUSER" -d "$PGDATABASE" "$@"
  fi
}

# WHAT IS KEPT is what login needs, and nothing else. Every other table in the
# schema is emptied — derived from pg_tables, never hand-listed: the first
# version kept a list, and deposits, their folio counter, the finance outbox,
# the school's details and the matrícula counter were all added after it and
# survived every "wipe" (scripts/reset-demo-db_test.sh guards this).
KEEP_TABLES="users roles user_roles user_invitations schema_migrations"

KEEP_SQL="$(printf "'%s'," $KEEP_TABLES | sed 's/,$//')"
SQL="
DO \$\$
DECLARE
  targets text;
BEGIN
  SELECT string_agg(format('%I', tablename), ', ' ORDER BY tablename)
    INTO targets
    FROM pg_tables
   WHERE schemaname = 'public' AND tablename NOT IN (${KEEP_SQL});
  IF targets IS NOT NULL THEN
    EXECUTE 'TRUNCATE ' || targets || ' RESTART IDENTITY CASCADE';
  END IF;
END
\$\$;
"

echo "[reset] emptying operational data in ${PGDATABASE}@${PGHOST:-$CONTAINER} (users and roles are kept)"
psql_do -v ON_ERROR_STOP=1 -c "$SQL"

# Every one of these tables has FORCE ROW LEVEL SECURITY, which filters even the
# table owner. Without a tenant bound, this summary reports 0 for everything and
# reads exactly like "the reset deleted your users" — it does not.
echo "[reset] what is left:"
psql_do -tAc "
SET app.current_tenant = '00000000-0000-0000-0000-000000000001';
SELECT 'users: '||count(*) FROM users
UNION ALL SELECT 'roles: '||count(*) FROM roles
UNION ALL SELECT 'students: '||count(*) FROM students
UNION ALL SELECT 'cycles: '||count(*) FROM cycles
UNION ALL SELECT 'uniform_items: '||count(*) FROM uniform_items;"
