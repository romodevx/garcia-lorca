# Shared preamble for the scripts that drive the deployed environment.
# Sourced, not executed.
#
# Every gcloud call in these scripts passes --project and --region EXPLICITLY.
# The owner runs several GCP projects from one machine; a destructive command
# that inherits whatever `gcloud config set project` last pointed at is the wrong
# shape for something you reach for under pressure.

set -eu

: "${PROJECT_ID:=garcia-lorca}"
: "${REGION:=us-central1}"

# The public front end plus the two services on the login path. Warming these
# three covers the click the visitor actually experiences first.
WARM_SERVICES="web gateway iam"
# Everything Terraform manages, for the operations that must cover the fleet.
ALL_SERVICES="web gateway iam students config finance grades attendance comms uniforms"

say() { printf '\033[36m[%s]\033[0m %s\n' "${SCRIPT_NAME:-cloud}" "$*"; }

# Announce before acting. Under pressure the useful thing is knowing WHICH
# project you are about to change.
banner() {
  say "project=$PROJECT_ID region=$REGION"
  say "$1"
}

run_update() { # run_update <service> <flags...>
  svc="$1"; shift
  gcloud run services update "$svc" \
    --project="$PROJECT_ID" \
    --region="$REGION" \
    --quiet \
    "$@" >/dev/null
}
