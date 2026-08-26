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
# Usage:  sh scripts/reset-demo-db.sh            (compose)
#         PGHOST=… PGUSER=… sh scripts/…         (anywhere else)
set -e

: "${CONTAINER:=garcia-lorca-postgres-1}"
: "${PGUSER:=postgres}"
: "${PGDATABASE:=garcia_lorca}"

SQL="
BEGIN;
TRUNCATE
  uniform_sale_keys, uniform_stock_moves, uniform_order_lines, uniform_fulfillments,
  uniform_orders, uniform_stock, uniform_items, uniform_folio_counters, uniform_settings,
  payments, charges, charge_concepts,
  attendance,
  grade_entries,
  guardian_students, enrollments, students,
  teaching_assignments, teachers, groups, terms, subjects, grade_levels, levels, cycles,
  notifications, devices, processed_events
RESTART IDENTITY CASCADE;
COMMIT;
"

echo "[reset] emptying operational data in ${PGDATABASE} (users and roles are kept)"
docker exec -i "$CONTAINER" psql -v ON_ERROR_STOP=1 -U "$PGUSER" -d "$PGDATABASE" -c "$SQL"

echo "[reset] what is left:"
docker exec -i "$CONTAINER" psql -U "$PGUSER" -d "$PGDATABASE" -tAc "
SELECT 'users: '||count(*) FROM users
UNION ALL SELECT 'roles: '||count(*) FROM roles
UNION ALL SELECT 'students: '||count(*) FROM students
UNION ALL SELECT 'cycles: '||count(*) FROM cycles
UNION ALL SELECT 'uniform_items: '||count(*) FROM uniform_items;"
