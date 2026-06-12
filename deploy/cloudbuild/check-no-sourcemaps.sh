#!/usr/bin/env bash
# Build-hardening gate (fallback).
#
# FAILS the build if production web client artifacts contain source maps:
#   * any *.map file, or
#   * a `sourceMappingURL` comment inside a shipped JS/CSS bundle.
#
# This is the pipeline-owned fallback. When apps/web/scripts/check-no-sourcemaps.sh
# exists (frontend-owned), cloudbuild.yaml invokes that one instead.
#
# Scans the standard Next.js production output locations.
set -euo pipefail

# Candidate build-output directories for the web app.
SCAN_DIRS=(
  "apps/web/.next"
  "apps/web/out"
  "apps/web/dist"
)

found_any_dir=false
violations=0

for dir in "${SCAN_DIRS[@]}"; do
  [ -d "$dir" ] || continue
  found_any_dir=true
  echo "Scanning $dir for source-map leaks..."

  # 1. Any .map file shipped.
  while IFS= read -r f; do
    echo "VIOLATION: source map file present: $f"
    violations=$((violations + 1))
  done < <(find "$dir" -type f -name '*.map')

  # 2. sourceMappingURL comments inside js/css bundles.
  while IFS= read -r f; do
    echo "VIOLATION: sourceMappingURL comment in: $f"
    violations=$((violations + 1))
  done < <(grep -rIl --include='*.js' --include='*.css' 'sourceMappingURL' "$dir" 2>/dev/null || true)
done

if [ "$found_any_dir" = false ]; then
  echo "ERROR: no web build output found in: ${SCAN_DIRS[*]}"
  echo "Did 'turbo run build' run before this gate?"
  exit 1
fi

if [ "$violations" -gt 0 ]; then
  echo "FAILED: $violations source-map leak(s) in production artifacts."
  exit 1
fi

echo "OK: no source maps in production web artifacts."
