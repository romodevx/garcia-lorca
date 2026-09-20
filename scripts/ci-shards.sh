#!/bin/sh
# Assign every go.work module to a CI shard, balanced by measured cost.
#
# WHY THIS EXISTS
#   GitHub bills every job rounded UP to a whole minute. Measured on the warm
#   run 35462895886: the 14-leg Go matrix did 1,558 s of real work and was
#   billed 33 minutes — and with 18 jobs in the workflow, a CI run cannot cost
#   less than 18 billed minutes however good the cache is. Across a cycle that
#   floor alone is ~1,044 min, the single largest line in the budget. Four
#   shards bill ~22 min for the same work.
#
#   It is NOT about doing less work. Every module still runs every gate.
#
# WHY LPT
#   Packing modules into shards to minimise the slowest shard is multiprocessor
#   scheduling — NP-hard. Longest-Processing-Time-first is the best-known simple
#   approximation (Graham 1969), bounded at 4/3 - 1/(3m) of optimal. On the real
#   weights below it lands within 13% (408 s vs 360 s), which is well inside the
#   one-minute billing granularity that is all this needs to beat.
#
# WHY THE WEIGHTS ARE A LIST AND THAT IS OK
#   go.work stays the single source of truth for WHICH modules exist — this
#   script reads it and never keeps its own copy. The weights below only affect
#   BALANCE, so a module missing from them is slower, never skipped.
#   scripts/ci-shards_test.sh asserts exactly that, because the failure mode of
#   getting it wrong is silent: a service added later that quietly stops being
#   tested, with nothing going red to say so.
#
# Usage:  sh scripts/ci-shards.sh                 # shards go.work, SHARDS=4
#         SHARDS=2 sh scripts/ci-shards.sh
#         MODULES="./a ./b" sh scripts/ci-shards.sh     # injected, for tests
#
# Output: a GitHub Actions matrix, one object per shard:
#   [{"shard":"1","modules":"./services/iam ./pkg","weight":408}, ...]
set -u

: "${SHARDS:=4}"

# Measured seconds per leg on the warm pull-request run 35462895886
# (2026-09-19), which is what a shard's cost actually is: the gates plus the
# per-job overhead of checkout, setup-go, artifact download and cache restore.
# Refresh with:  make ci-minutes  (or the per-job breakdown of any warm run).
weight_for() {
  case "$1" in
    ./services/iam)          echo 186 ;;
    ./services/finance)      echo 154 ;;
    ./services/comms)        echo 137 ;;
    ./services/uniforms)     echo 135 ;;
    ./services/students)     echo 135 ;;
    ./services/attendance)   echo 132 ;;
    ./services/grades)       echo 129 ;;
    ./services/gateway)      echo 116 ;;
    ./services/config)       echo 116 ;;
    ./pkg)                   echo  87 ;;
    ./gen/go)                echo  77 ;;
    ./services/billing)      echo  54 ;;
    ./services/docs)         echo  52 ;;
    ./services/scholarships) echo  48 ;;
    # Unknown module: assume the most expensive one we have ever measured.
    # Guessing cheap would let a new service pile onto an already-full shard and
    # set the wall clock for everyone; guessing expensive only costs a little
    # imbalance until somebody measures it.
    *)                       echo 186 ;;
  esac
}

if [ -z "${MODULES:-}" ]; then
  command -v go >/dev/null 2>&1 || { echo "ERROR: go is required to read go.work" >&2; exit 2; }
  command -v jq >/dev/null 2>&1 || { echo "ERROR: jq is required" >&2; exit 2; }
  MODULES="$(go work edit -json | jq -r '.Use[].DiskPath' | tr '\n' ' ')"
fi

# Never emit an empty shard: a leg with no modules still bills a whole minute to
# check nothing at all.
count=0
for m in $MODULES; do count=$((count + 1)); done
[ "$count" -eq 0 ] && { echo "[]"; exit 0; }
[ "$SHARDS" -gt "$count" ] && SHARDS="$count"
[ "$SHARDS" -lt 1 ] && SHARDS=1

# Longest first, ties broken by name so the output never reshuffles between runs
# — the shard ids name the CI jobs, and a check whose contents move is a check
# nobody can reason about.
for m in $MODULES; do
  printf '%s\t%s\n' "$(weight_for "$m")" "$m"
done \
| sort -k1,1nr -k2,2 \
| awk -v n="$SHARDS" '
    { w[NR] = $1; mod[NR] = $2 }
    END {
      for (b = 1; b <= n; b++) { load[b] = 0; items[b] = "" }
      for (i = 1; i <= NR; i++) {
        # Place on the lightest shard so far. Ties go to the lowest id, which is
        # what keeps the whole thing deterministic.
        best = 1
        for (b = 2; b <= n; b++) if (load[b] < load[best]) best = b
        load[best] += w[i]
        items[best] = (items[best] == "" ? mod[i] : items[best] " " mod[i])
      }
      printf "["
      for (b = 1; b <= n; b++) {
        printf "%s{\"shard\":\"%d\",\"modules\":\"%s\",\"weight\":%d}", \
               (b == 1 ? "" : ","), b, items[b], load[b]
      }
      printf "]\n"
    }
  '
