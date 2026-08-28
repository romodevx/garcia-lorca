#!/bin/sh
# Empties the operational data and leaves ONLY the seeded user, so a demo can
# show the school being configured from nothing.
#
# It does NOT touch: users, roles, permissions, or schema_migrations. Dropping
# the owner would lock the demo out of its own system, and dropping the migration
# ledger would make the next `make migrate` try to re-apply everything.
#
# TRUNCATE ... CASCADE, in one transaction, with RESTART IDENTITY so folio
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

if [ -n "${PGHOST:-}" ]; then
  case "$PGHOST" in
    localhost|127.0.0.1|::1|"") ;;
    *)
      if [ -z "$CONFIRMED" ]; then
        echo "[reset] REFUSING: PGHOST=$PGHOST is not local." >&2
        echo "[reset] This TRUNCATEs every operational table. If you really mean it," >&2
        echo "[reset] re-run with --i-know-this-wipes-remote." >&2
        exit 1
      fi
      echo "[reset] WARNING: wiping REMOTE host $PGHOST (confirmed on the command line)"
      ;;
  esac
fi

# psql_do runs one psql invocation against whichever transport applies.
psql_do() {
  if [ -n "${PGHOST:-}" ]; then
    psql -h "$PGHOST" -p "${PGPORT:-5432}" -U "$PGUSER" -d "$PGDATABASE" "$@"
  else
    docker exec -i "$CONTAINER" psql -U "$PGUSER" -d "$PGDATABASE" "$@"
  fi
}

SQL="
BEGIN;
TRUNCATE
  uniform_sale_keys, uniform_stock_moves, uniform_order_lines, uniform_fulfillments,
  uniform_orders, uniform_stock, uniform_items, uniform_folio_counters, uniform_settings,
  payments, charges, charge_concepts, tuition_plans,
  attendance,
  grade_entries,
  guardian_students, enrollments, students,
  teaching_assignments, teachers, groups, terms, subjects, grade_levels, levels, cycles,
  notifications, devices, processed_events
RESTART IDENTITY CASCADE;
COMMIT;
"

echo "[reset] emptying operational data in ${PGDATABASE}@${PGHOST:-$CONTAINER} (users and roles are kept)"
psql_do -v ON_ERROR_STOP=1 -c "$SQL"

echo "[reset] what is left:"
psql_do -tAc "
SELECT 'users: '||count(*) FROM users
UNION ALL SELECT 'roles: '||count(*) FROM roles
UNION ALL SELECT 'students: '||count(*) FROM students
UNION ALL SELECT 'cycles: '||count(*) FROM cycles
UNION ALL SELECT 'uniform_items: '||count(*) FROM uniform_items;"
