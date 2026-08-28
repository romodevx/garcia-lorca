#!/bin/sh
# Warm the demo before a meeting, then let it go cold again afterwards.
#
# WHY THIS EXISTS. Every service runs at min-instances=0, which is what makes the
# environment cost ~nothing at rest. The price is the first click of the day:
# web, gateway and iam all cold-start in sequence, so the first login takes
# seconds and reads to a visitor as "unfinished".
#
# Warming those three for a three-hour window costs on the order of ten US cents.
# That is the whole trade: pay a dime, remove the one thing about this deployment
# a customer would actually notice.
#
# This is deliberately a SCRIPT and not a Terraform setting. terraform apply will
# put min-instances back to 0 — in fact modules/cloud_run refuses to plan a warm
# service outside prod — so warmth can never quietly persist into next month's
# bill. Run scripts/cloud-cool.sh when the meeting ends; if you forget, the next
# apply fixes it for you.
#
# Usage:  sh scripts/cloud-warm.sh
#         PROJECT_ID=other-project sh scripts/cloud-warm.sh
SCRIPT_NAME=cloud-warm
. "$(dirname "$0")/cloud-common.sh"

banner "warming $WARM_SERVICES (min-instances=1) — remember scripts/cloud-cool.sh afterwards"

for svc in $WARM_SERVICES; do
  say "  $svc -> min-instances=1"
  run_update "$svc" --min-instances=1
done

# Waking the rest is free and takes one request each: a cold module is a 3-8s
# pause the first time someone opens it, which is exactly when they are watching.
WEB_URL=$(gcloud run services describe web --project="$PROJECT_ID" --region="$REGION" --format='value(status.url)' 2>/dev/null || true)
if [ -n "$WEB_URL" ]; then
  say "waking every module through $WEB_URL"
  for path in /healthz /login /; do
    curl -s -o /dev/null -w "  %{http_code}  $path (%{time_total}s)\n" "$WEB_URL$path" || true
  done
else
  say "could not resolve the web URL; skipping the warm-up requests"
fi

say "done. Click through each module once before the meeting to warm the rest."
