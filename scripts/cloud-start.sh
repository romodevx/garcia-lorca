#!/bin/sh
# Undo scripts/cloud-stop.sh: put the demo back online.
#
# Restores public access to the web service and lifts the emergency instance cap
# back to what Terraform declares. Running `terraform apply` afterwards is the
# authoritative way to get every setting back to its declared value; this is the
# fast path for the common case.
#
# Usage:  sh scripts/cloud-start.sh
SCRIPT_NAME=cloud-start
. "$(dirname "$0")/cloud-common.sh"

banner "putting the demo back online"

say "restoring public access on web"
gcloud run services add-iam-policy-binding web \
  --project="$PROJECT_ID" --region="$REGION" \
  --member=allUsers --role=roles/run.invoker --quiet >/dev/null

say "lifting the emergency instance cap (Terraform owns the real values)"
run_update web --max-instances=2
run_update gateway --max-instances=1
for svc in iam students config finance grades attendance comms uniforms; do
  if gcloud run services describe "$svc" --project="$PROJECT_ID" --region="$REGION" >/dev/null 2>&1; then
    run_update "$svc" --max-instances=3
  fi
done

WEB_URL=$(gcloud run services describe web --project="$PROJECT_ID" --region="$REGION" --format='value(status.url)' 2>/dev/null || true)
say "done. ${WEB_URL:-run terraform output web_url}"
say "run 'make tf-apply' to return every setting to its declared value."
