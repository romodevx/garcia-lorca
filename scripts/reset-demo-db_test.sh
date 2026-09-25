#!/bin/sh
# Tests for scripts/reset-demo-db.sh.
#
# The rule this guards: **a wipe empties every operational table, including the
# ones added after the script was written.** The first version hand-listed its
# tables, and by September 2026 deposits, their folio counter, the finance
# outbox, the school's details and the matrícula counter had all been added
# without it — a "wiped" local database still held them, and nothing said so.
# Same silent failure as ci-shards: a list a human maintains drifts. So the
# script derives the list from the catalog, and keeps only what login needs.
#
# Usage:  sh scripts/reset-demo-db_test.sh
set -u

PASS=0
FAIL=0
ok()  { printf '  \033[32m✓\033[0m %s\n' "$1"; PASS=$((PASS + 1)); }
bad() { printf '  \033[31m✗\033[0m %s\n' "$1"; printf '      → %s\n' "$2"; FAIL=$((FAIL + 1)); }

DIR="$(dirname "$0")"
SCRIPT="$DIR/reset-demo-db.sh"
MIGRATIONS="$DIR/../migrations"
[ -f "$SCRIPT" ] || { echo "ERROR: $SCRIPT does not exist." >&2; exit 1; }

echo "reset-demo-db.sh"

# --- 1. The table list comes from the catalog, not from a hand-kept list ----
if grep -q "FROM pg_tables" "$SCRIPT"; then
  ok "truncates what pg_tables lists, so a new table cannot be forgotten"
else
  bad "truncates a hand-kept list" "derive the tables from pg_tables and keep only what login needs"
fi

# --- 2. What it keeps is exactly what login needs --------------------------
KEEP="$(sed -n 's/^KEEP_TABLES="\(.*\)"$/\1/p' "$SCRIPT")"
WANT="schema_migrations user_invitations user_roles roles users"
sorted() { printf '%s\n' $1 | sort | tr '\n' ' '; }
if [ "$(sorted "$KEEP")" = "$(sorted "$WANT")" ]; then
  ok "keeps only users, roles, their links, invitations and the migration ledger"
else
  bad "keeps [$KEEP]" "want exactly [$WANT]: anything more survives a wipe"
fi

# --- 3. Every kept table exists (a rename must not keep a ghost) -----------
for t in $WANT; do
  [ "$t" = "schema_migrations" ] && continue
  if grep -rqiE "CREATE TABLE( IF NOT EXISTS)? $t[ (]" "$MIGRATIONS"; then
    ok "kept table $t exists in the migrations"
  else
    bad "kept table $t is in no migration" "a renamed table would be kept under a name that no longer exists"
  fi
done

echo
echo "$PASS passed, $FAIL failed"
[ "$FAIL" -eq 0 ]
