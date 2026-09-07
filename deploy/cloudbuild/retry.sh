#!/usr/bin/env bash
# Retrying the network, and ONLY the network.
#
# Every Cloud Build step runs in a fresh container with no shared module or
# package cache, so each one re-downloads its dependencies from proxy.golang.org
# and the npm registry. A single dropped connection there kills the whole build
# — twenty minutes of work and ten images — and the only symptom the owner sees
# is a "Deploy dev failed" email that looks exactly like a real breakage.
#
# Observed 2026-09-07: `read "https://proxy.golang.org/...zip": stream error;
# INTERNAL_ERROR; received from peer`. Nothing was wrong with the code; the
# re-run passed untouched.
#
# WHAT THIS MUST NEVER WRAP: tests, linters, vet, or the govulncheck analysis.
# Retrying those turns a flaky test into a green build, which is the opposite of
# what CI is for — and this pipeline has already caught a real one that way
# (a web test that read a count before waiting for it, 2026-09-06). Downloads
# are idempotent and have no verdict to launder; test runs are neither.

# retry runs a command until it succeeds, up to three attempts, backing off
# 5s then 10s. Returns the last failure's status so `set -e` still stops the
# step when the network is genuinely down rather than merely flaky.
retry() {
  local attempt=1
  local max=3
  local delay=5
  until "$@"; do
    if [ "$attempt" -ge "$max" ]; then
      echo "retry: '$*' failed $max times; giving up" >&2
      return 1
    fi
    echo "retry: '$*' failed (attempt $attempt/$max); retrying in ${delay}s" >&2
    sleep "$delay"
    attempt=$((attempt + 1))
    delay=$((delay * 2))
  done
}
