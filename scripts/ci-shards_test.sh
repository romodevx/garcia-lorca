#!/bin/sh
# Table tests for scripts/ci-shards.sh.
#
# The rule this guards is the one that makes sharding safe to adopt at all:
# **every module in go.work lands in exactly one shard.** Collapsing the matrix
# trades a list Go maintains for a list a human maintains, and the failure mode
# is silent — a service added six weeks from now simply stops being tested, and
# nothing goes red to say so. That is worth more than the minutes sharding saves,
# so it is asserted here rather than trusted.
#
# Usage:  sh scripts/ci-shards_test.sh
set -u

PASS=0
FAIL=0
ok()   { printf '  \033[32m✓\033[0m %s\n' "$1"; PASS=$((PASS + 1)); }
bad()  { printf '  \033[31m✗\033[0m %s\n' "$1"; printf '      → %s\n' "$2"; FAIL=$((FAIL + 1)); }

SCRIPT="$(dirname "$0")/ci-shards.sh"
if [ ! -f "$SCRIPT" ]; then
  echo "ERROR: $SCRIPT does not exist yet." >&2
  exit 1
fi

command -v jq >/dev/null 2>&1 || { echo "ERROR: jq is required." >&2; exit 2; }

echo "ci-shards.sh"

# --- 1. Partition: every module in, exactly once ----------------------------
# The whole point. Uses an injected list so the test does not depend on what
# go.work happens to contain today.
out=$(MODULES="./a ./b ./c ./d ./e ./f ./g" SHARDS=3 sh "$SCRIPT" 2>/dev/null)
got=$(printf '%s' "$out" | jq -r '[.[].modules] | join(" ")' 2>/dev/null | tr ' ' '\n' | grep -v '^$' | sort | tr '\n' ' ')
want="./a ./b ./c ./d ./e ./f ./g "
if [ "$got" = "$want" ]; then
  ok "every module lands in exactly one shard"
else
  bad "partition is wrong" "want [$want] got [$got]"
fi

# --- 2. Shard count is honoured ---------------------------------------------
for n in 1 2 4 7; do
  cnt=$(MODULES="./a ./b ./c ./d ./e ./f ./g" SHARDS="$n" sh "$SCRIPT" 2>/dev/null | jq 'length' 2>/dev/null)
  if [ "$cnt" = "$n" ]; then
    ok "SHARDS=$n produces $n shards"
  else
    bad "SHARDS=$n produced '$cnt'" "the matrix must have exactly the requested number of legs"
  fi
done

# --- 3. More shards than modules does not emit an empty leg -----------------
# An empty leg is a job that costs a whole billed minute to check nothing.
empties=$(MODULES="./a ./b" SHARDS=5 sh "$SCRIPT" 2>/dev/null \
          | jq '[.[] | select(.modules == "")] | length' 2>/dev/null)
if [ "$empties" = "0" ]; then
  ok "never emits an empty shard"
else
  bad "emitted $empties empty shard(s)" "a leg with no modules bills a minute to do nothing"
fi

# --- 4. A module nobody weighed still gets placed ---------------------------
# This is the drift case: somebody adds services/newthing and never touches the
# weight table. It must still be tested, just possibly in an unbalanced shard.
placed=$(MODULES="./services/iam ./services/brand-new-unweighed" SHARDS=2 sh "$SCRIPT" 2>/dev/null \
         | jq -r '[.[].modules] | join(" ")' 2>/dev/null | grep -c 'brand-new-unweighed')
if [ "$placed" = "1" ]; then
  ok "an unweighed module is still assigned"
else
  bad "unweighed module was dropped" "a service added later must never silently stop being tested"
fi

# --- 5. Deterministic ---------------------------------------------------------
a=$(MODULES="./a ./b ./c ./d ./e" SHARDS=3 sh "$SCRIPT" 2>/dev/null)
b=$(MODULES="./a ./b ./c ./d ./e" SHARDS=3 sh "$SCRIPT" 2>/dev/null)
if [ "$a" = "$b" ]; then
  ok "same input, same output"
else
  bad "output is not deterministic" "a matrix that reshuffles makes job names useless as checks"
fi

# --- 6. Shard names are stable and usable as check names --------------------
names=$(MODULES="./a ./b ./c" SHARDS=3 sh "$SCRIPT" 2>/dev/null | jq -r '[.[].shard] | join(",")' 2>/dev/null)
if [ "$names" = "1,2,3" ]; then
  ok "shard ids are stable (1..N)"
else
  bad "shard ids are '$names'" "they name the CI jobs; they cannot depend on the module list"
fi

# --- 7. Balance: the heaviest shard is not absurdly worse than the lightest --
# Not a strict bound — bin packing on lumpy input cannot promise one. This only
# catches an assignment that ignores weight entirely (e.g. plain round-robin
# over an alphabetical list), which is what makes sharding cost more than it saves.
bal=$(MODULES="./services/iam ./services/finance ./services/comms ./services/uniforms ./services/students ./services/attendance ./services/grades ./services/gateway ./services/config ./pkg ./gen/go ./services/billing ./services/docs ./services/scholarships" \
      SHARDS=4 sh "$SCRIPT" 2>/dev/null | jq '[.[].weight] | (max / (min + 1))' 2>/dev/null)
if [ -n "$bal" ] && [ "$(printf '%s' "$bal" | cut -d. -f1)" -le 1 ]; then
  ok "shards are balanced by measured weight (max/min ratio $bal)"
else
  bad "shards are unbalanced (max/min ratio $bal)" "weights are being ignored; the slowest leg sets the wall clock"
fi

# --- 8. Real go.work: nothing is lost ----------------------------------------
# The production case. Runs against whatever go.work says TODAY.
if command -v go >/dev/null 2>&1 && [ -f go.work ]; then
  real=$(go work edit -json | jq -r '.Use[].DiskPath' | sort | tr '\n' ' ')
  sharded=$(SHARDS=4 sh "$SCRIPT" 2>/dev/null | jq -r '[.[].modules] | join(" ")' 2>/dev/null \
            | tr ' ' '\n' | grep -v '^$' | sort | tr '\n' ' ')
  if [ "$real" = "$sharded" ]; then
    ok "go.work's $(printf '%s' "$real" | wc -w | tr -d ' ') modules are all sharded, none twice"
  else
    bad "go.work and the shards disagree" "in go.work: [$real]   in shards: [$sharded]"
  fi
else
  printf '  \033[33m!\033[0m skipped the go.work check (no go or no go.work here)\n'
fi

echo
echo "passed: $PASS   failed: $FAIL"
[ "$FAIL" -eq 0 ]
