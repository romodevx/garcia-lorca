#!/bin/sh
# What CI and the deploy actually COST, in billed GitHub Actions minutes.
#
# This is the failing test of .claude/docs/PLAN-minutos-ci.md. Today it prints
# ~48 min per PR run of CI and ~58 min per deploy, against a 3,000 min/cycle
# allowance that the owner deliberately caps at a $0 spending limit — so running
# out is not a bill, it is CI and the deploy going dark mid-week with daily
# demos for the school and delivery on 2026-11-30.
#
# WHY IT RE-DERIVES THE NUMBER INSTEAD OF ASKING GITHUB
#   /repos/{owner}/{repo}/actions/workflows/{id}/timing returns total_ms: 0 in
#   this repo and is useless. GitHub bills each JOB rounded UP to a whole
#   minute, so the only faithful figure is sum(ceil(job_seconds / 60)) — which
#   is exactly what this computes. Measured against the 2026-08-19..09-18 cycle
#   it printed 2,739 min against a 2,700 min invoice: 1.4% off, and the
#   difference is runs aged out of the API window.
#
# WHY IT REPORTS RATES, NOT JUST A TOTAL
#   The baseline was measured during a month with commits deliberately withheld
#   to save minutes. A total compares two different workloads, so it cannot tell
#   "we got faster" from "we did less". Minutes PER CI RUN and PER DEPLOY can.
#
# Read-only. Changes nothing. Exit 1 if a rate is above its target.
#
# Usage:  sh scripts/ci-minutes.sh
#         DAYS=7 sh scripts/ci-minutes.sh
#         REPO=owner/name DAYS=30 sh scripts/ci-minutes.sh
#
# Requires: gh (authenticated), jq. Needs only the `repo` scope — the billing
# endpoints are NOT used, precisely so this runs without `gh auth refresh`.
set -u

: "${DAYS:=30}"
: "${MAX_RUNS:=400}"
# Targets from the plan's §Verificación. Etapa 1 is <=18 (18 jobs x a 1-minute
# billing floor is the arithmetic floor); <=8 only becomes reachable after T3
# collapses the 14-leg matrix into shards.
: "${TARGET_CI_MIN:=18}"
: "${TARGET_DEPLOY_MIN:=2}"

FAIL=0
ok()   { printf '  \033[32m✓\033[0m %s\n' "$1"; }
bad()  { printf '  \033[31m✗\033[0m %s\n' "$1"; printf '      → %s\n' "$2"; FAIL=1; }
warn() { printf '  \033[33m!\033[0m %s\n' "$1"; printf '      → %s\n' "$2"; }

for bin in gh jq; do
  command -v "$bin" >/dev/null 2>&1 || {
    echo "ERROR: $bin is required but not installed." >&2
    [ "$bin" = gh ] && echo "  → brew install gh && gh auth login" >&2
    [ "$bin" = jq ] && echo "  → brew install jq" >&2
    exit 2
  }
done

# The repo under measurement. Explicit REPO wins so this can run from anywhere.
if [ -z "${REPO:-}" ]; then
  REPO=$(gh repo view --json nameWithOwner -q .nameWithOwner 2>/dev/null) || REPO=""
fi
if [ -z "$REPO" ]; then
  echo "ERROR: could not determine the repository." >&2
  echo "  → run inside the repo, or pass REPO=owner/name" >&2
  exit 2
fi

# BSD date (macOS) and GNU date (Linux/CI) disagree on relative arithmetic.
if SINCE=$(date -u -v-"${DAYS}"d +%Y-%m-%dT%H:%M:%SZ 2>/dev/null); then
  :
elif SINCE=$(date -u -d "${DAYS} days ago" +%Y-%m-%dT%H:%M:%SZ 2>/dev/null); then
  :
else
  echo "ERROR: neither BSD nor GNU date accepted a relative offset." >&2
  exit 2
fi

echo "billed Actions minutes — repo=$REPO window=${DAYS}d (since $SINCE)"
echo "counting sum(ceil(job_seconds / 60)) per job, which is how GitHub bills."
echo

TMP=$(mktemp -t ci-minutes.XXXXXXXX) || exit 2
RUNS=$(mktemp -t ci-runs.XXXXXXXX) || exit 2
trap 'rm -f "$TMP" "$RUNS" "$TMP.jobs"' EXIT INT TERM

# --- 1. The runs in the window -----------------------------------------------
# A 403 here is almost always a missing/expired token rather than a scope
# problem, because this endpoint needs only `repo`. Name the fix either way:
# the plan's DX review asked for the exact command, not a stack trace.
if ! gh api --paginate \
      "repos/${REPO}/actions/runs?created=%3E%3D${SINCE}&per_page=100" \
      --jq '.workflow_runs[] | {id, name, event, conclusion, created_at}' \
      > "$RUNS" 2>"$TMP.err"; then
  echo "ERROR: could not list workflow runs." >&2
  sed 's/^/  /' "$TMP.err" >&2
  echo "  → 403 or 404: gh auth status, then gh auth login --scopes repo" >&2
  echo "  → if this repo is in an org with SSO: gh auth refresh -s repo" >&2
  rm -f "$TMP.err"
  exit 2
fi
rm -f "$TMP.err"

RUN_COUNT=$(wc -l < "$RUNS" | tr -d ' ')
if [ "$RUN_COUNT" -eq 0 ]; then
  warn "no workflow runs in the last ${DAYS} days" "nothing to measure; widen DAYS="
  exit 0
fi
if [ "$RUN_COUNT" -gt "$MAX_RUNS" ]; then
  warn "$RUN_COUNT runs found, capping at $MAX_RUNS" "raise MAX_RUNS= to include them all"
  head -n "$MAX_RUNS" "$RUNS" > "$RUNS.capped" && mv "$RUNS.capped" "$RUNS"
  RUN_COUNT=$MAX_RUNS
fi

# --- 2. Billed minutes per run ------------------------------------------------
# One jobs call per run. Progress goes to stderr so stdout stays pipeable.
i=0
: > "$TMP"
while IFS= read -r run; do
  i=$((i + 1))
  printf '\r  reading jobs: %d/%d' "$i" "$RUN_COUNT" >&2
  run_id=$(printf '%s' "$run" | jq -r '.id')
  gh api --paginate "repos/${REPO}/actions/runs/${run_id}/jobs?per_page=100" \
      --jq '.jobs[] | select(.started_at != null and .completed_at != null) |
            ((.completed_at | fromdateiso8601) - (.started_at | fromdateiso8601))
            | (. + 59) / 60 | floor | if . < 1 then 1 else . end' \
      2>/dev/null \
    | jq -s --argjson run "$run" \
        '{id: $run.id, name: $run.name, event: $run.event,
          conclusion: $run.conclusion, jobs: length, minutes: (add // 0)}' \
    >> "$TMP"
done < "$RUNS"
printf '\r%*s\r' 40 '' >&2

TOTAL=$(jq -s 'map(.minutes) | add // 0' "$TMP")

# --- 3. Report ---------------------------------------------------------------
echo "TOTAL: ${TOTAL} billed min across ${RUN_COUNT} runs"
echo
echo "BY WORKFLOW"
jq -rs 'group_by(.name) | sort_by(-(map(.minutes) | add))
        | .[] | "  \(map(.minutes) | add | tostring | (" " * (6 - length)) + .) min  \(.[0].name)  (\(length) runs)"' "$TMP"

echo
echo "BY WORKFLOW + EVENT — this is the number that has to come down"
jq -rs 'group_by([.name, .event]) | sort_by(-(map(.minutes) | add))
        | .[] | "  \(map(.minutes) | add | tostring | (" " * (6 - length)) + .) min  \(.[0].name) / \(.[0].event)  \(length) runs  avg \((map(.minutes) | add / length) | .*10 | round / 10) min/run"' "$TMP"

echo
echo "RATES"

# CI: pull_request is the expensive path and the one the targets are written
# against. A cycle with no PRs is not a pass, it is no measurement.
CI_PR_RUNS=$(jq -s '[.[] | select(.name == "CI" and .event == "pull_request")] | length' "$TMP")
if [ "$CI_PR_RUNS" -gt 0 ]; then
  CI_PR_AVG=$(jq -s '[.[] | select(.name == "CI" and .event == "pull_request")] | (map(.minutes) | add / length) | .*10 | round / 10' "$TMP")
  if [ "$(jq -n "$CI_PR_AVG <= $TARGET_CI_MIN")" = "true" ]; then
    ok "CI / pull_request: ${CI_PR_AVG} min/run (target <= ${TARGET_CI_MIN})"
  else
    bad "CI / pull_request: ${CI_PR_AVG} min/run (target <= ${TARGET_CI_MIN})" \
        "T1 (the 7.6 KB cache tombstone) and T2 (vet compiled four times) are the levers"
  fi
else
  warn "no CI pull_request runs in the window" "the PR path is the expensive one; measure a cycle that has some"
fi

DEPLOY_OK_RUNS=$(jq -s '[.[] | select(.name == "Deploy dev" and .conclusion == "success")] | length' "$TMP")
if [ "$DEPLOY_OK_RUNS" -gt 0 ]; then
  DEPLOY_AVG=$(jq -s '[.[] | select(.name == "Deploy dev" and .conclusion == "success")] | (map(.minutes) | add / length) | .*10 | round / 10' "$TMP")
  if [ "$(jq -n "$DEPLOY_AVG <= $TARGET_DEPLOY_MIN")" = "true" ]; then
    ok "Deploy dev (successful): ${DEPLOY_AVG} min/deploy (target <= ${TARGET_DEPLOY_MIN})"
  else
    bad "Deploy dev (successful): ${DEPLOY_AVG} min/deploy (target <= ${TARGET_DEPLOY_MIN})" \
        "T7 — the runner is paid to watch a build that runs on Google's infrastructure"
  fi
else
  warn "no successful Deploy dev runs in the window" "a failed deploy dies early and understates the cost"
fi

# The total is a secondary, context-dependent signal: it moves with how much
# work the month had. Reported, never the pass/fail criterion.
echo
echo "CONTEXT (not a criterion — the total moves with the month's workload)"
printf '  %s min in %s days; the allowance is 3,000 min/cycle.\n' "$TOTAL" "$DAYS"
printf '  Staged goal from the plan: <= 900 min/cycle to call it a success, <= 700 the target.\n'

exit $FAIL
