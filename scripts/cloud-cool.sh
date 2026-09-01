#!/bin/sh
# Return the demo to scale-to-zero after a meeting.
#
# Undoes scripts/cloud-warm.sh. Safe to run at any time and safe to run twice.
# If you forget, the next `terraform apply` does it for you — modules/cloud_run
# refuses to plan min-instances > 0 outside prod — but do not rely on that: the
# whole point is that warmth is temporary and deliberate.
#
# Usage:  sh scripts/cloud-cool.sh
SCRIPT_NAME=cloud-cool
. "$(dirname "$0")/cloud-common.sh"

banner "returning $ALL_SERVICES to min-instances=0"

for svc in $ALL_SERVICES; do
  if gcloud run services describe "$svc" --project="$PROJECT_ID" --region="$REGION" >/dev/null 2>&1; then
    say "  $svc -> min-instances=0"
    run_update "$svc" --min-instances=0
  fi
done

say "verifying nothing is left warm:"
gcloud run services list \
  --project="$PROJECT_ID" --region="$REGION" \
  --format='table(metadata.name,spec.template.metadata.annotations["autoscaling.knative.dev/minScale"]:label=MIN)' 2>/dev/null || true
