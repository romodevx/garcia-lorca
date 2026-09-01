#!/bin/sh
# Panic button: make the demo unreachable and bound any runaway compute.
#
# READ THIS FIRST — what it does and does NOT save.
#
# Since the database moved to an external Postgres that auto-suspends, this
# environment costs ~$0.38/month at rest, and all of that is Artifact Registry
# and Secret Manager. Every Cloud Run service already sits at zero instances.
# So there is nothing here to "switch off" for idle cost: it is already off.
#
# What this script is actually for:
#   * cutting off traffic you did not expect (a crawler, a loop, a shared link)
#   * bounding compute if something starts hammering the gateway
#   * taking the demo out of public view without destroying it
#
# It does NOT delete anything. The database, the images and the secrets all
# survive; scripts/cloud-start.sh puts it back in one command.
#
# NOTE on --max-instances: 0 does NOT mean "stopped" in Cloud Run (gcloud
# documents 'default' as the way to remove the cap), which is why this caps at 1
# rather than pretending 0 halts anything.
#
# Usage:  sh scripts/cloud-stop.sh
SCRIPT_NAME=cloud-stop
. "$(dirname "$0")/cloud-common.sh"

banner "taking the demo offline (nothing is deleted)"

say "revoking public access on web — the link stops working immediately"
gcloud run services remove-iam-policy-binding web \
  --project="$PROJECT_ID" --region="$REGION" \
  --member=allUsers --role=roles/run.invoker --quiet >/dev/null 2>&1 \
  && say "  web is no longer publicly invokable" \
  || say "  (web already had no public binding)"

say "capping instances so nothing can scale up while you look into it"
for svc in $ALL_SERVICES; do
  if gcloud run services describe "$svc" --project="$PROJECT_ID" --region="$REGION" >/dev/null 2>&1; then
    run_update "$svc" --min-instances=0 --max-instances=1
    say "  $svc -> min=0 max=1"
  fi
done

say "done. Restore with: sh scripts/cloud-start.sh"
