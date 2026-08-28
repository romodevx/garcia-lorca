#!/bin/sh
# Checks everything the deploy runbook silently assumes, BEFORE you start.
#
# The failures this prevents are the expensive kind: the ones that surface forty
# resources into a terraform apply, or twenty minutes into a build. Billing-account
# roles are the worst of them, because they are not project roles — the apply
# creates everything and then dies on the last resource, which happens to be the
# budget guardrail the whole environment exists to have.
#
# Read-only. Changes nothing. Exit 1 if anything required is missing.
#
# Usage:  sh scripts/deploy-preflight.sh
#         PROJECT_ID=other BILLING_ACCOUNT_ID=… sh scripts/deploy-preflight.sh
set -u

: "${PROJECT_ID:=garcia-lorca}"
: "${REGION:=us-central1}"
: "${BILLING_ACCOUNT_ID:=018CA3-FF15AB-FAA61D}"

FAIL=0
ok()   { printf '  \033[32m✓\033[0m %s\n' "$1"; }
bad()  { printf '  \033[31m✗\033[0m %s\n' "$1"; printf '      → %s\n' "$2"; FAIL=1; }
warn() { printf '  \033[33m!\033[0m %s\n' "$1"; printf '      → %s\n' "$2"; }

echo "preflight for project=$PROJECT_ID region=$REGION"

echo
echo "TOOLS"
check_bin() {
  if command -v "$1" >/dev/null 2>&1; then ok "$1"; else bad "$1 missing" "$2"; fi
}
check_bin terraform "https://developer.hashicorp.com/terraform/install"
check_bin gcloud    "https://cloud.google.com/sdk/docs/install"
check_bin docker    "needed to build images locally; Cloud Build can do it instead"
check_bin node      "brew install node"
check_bin pnpm      "corepack enable pnpm"
check_bin psql      "brew install libpq && brew link --force libpq  (migrations need it)"
check_bin openssl   "ships with macOS; needed to generate secrets"

echo
echo "AUTH"
ACCOUNT=$(gcloud config get-value account 2>/dev/null)
if [ -n "$ACCOUNT" ] && [ "$ACCOUNT" != "(unset)" ]; then
  ok "gcloud account: $ACCOUNT"
else
  bad "no active gcloud account" "gcloud auth login"
fi

echo
echo "PROJECT"
if gcloud projects describe "$PROJECT_ID" >/dev/null 2>&1; then
  ok "$PROJECT_ID exists and is reachable"
else
  bad "cannot read project $PROJECT_ID" "check the id, or gcloud auth login"
fi

if gcloud beta billing projects describe "$PROJECT_ID" --format='value(billingEnabled)' 2>/dev/null | grep -qi true; then
  ok "billing is enabled on $PROJECT_ID"
else
  bad "billing is not enabled on $PROJECT_ID" "link a billing account in the console"
fi

echo
echo "APIS"
# cloudresourcemanager is special: data.google_project is read during PLAN, before
# Terraform can enable anything, so a virgin project fails the very first plan.
ENABLED=$(gcloud services list --enabled --project="$PROJECT_ID" --format='value(config.name)' 2>/dev/null)
if printf '%s\n' "$ENABLED" | grep -q '^cloudresourcemanager.googleapis.com$'; then
  ok "cloudresourcemanager enabled (required BEFORE the first terraform plan)"
else
  bad "cloudresourcemanager.googleapis.com is NOT enabled" \
      "gcloud services enable cloudresourcemanager.googleapis.com --project=$PROJECT_ID"
fi
for api in run sqladmin artifactregistry cloudbuild secretmanager pubsub; do
  if printf '%s\n' "$ENABLED" | grep -q "^${api}.googleapis.com$"; then
    ok "$api enabled"
  else
    warn "$api not enabled yet" "terraform apply enables it; only cloudresourcemanager must precede the plan"
  fi
done

echo
echo "BILLING-ACCOUNT ROLES (not project roles — this is the one that bites late)"
if gcloud beta billing accounts describe "$BILLING_ACCOUNT_ID" >/dev/null 2>&1; then
  ok "can read billing account $BILLING_ACCOUNT_ID"
else
  bad "cannot read billing account $BILLING_ACCOUNT_ID" \
      "budget.tf needs roles/billing.costsManager (or Administrator) on it, or the apply dies on its LAST resource"
fi

echo
echo "STATE BUCKET"
BUCKET="${BUCKET:-${PROJECT_ID}-tfstate}"
if gcloud storage buckets describe "gs://$BUCKET" >/dev/null 2>&1; then
  ok "gs://$BUCKET exists"
else
  warn "gs://$BUCKET does not exist yet" \
       "gcloud storage buckets create gs://$BUCKET --project=$PROJECT_ID --location=$REGION --uniform-bucket-level-access && gcloud storage buckets update gs://$BUCKET --versioning"
fi

echo
echo "TFVARS"
if [ -f "deploy/terraform/environments/dev.tfvars" ]; then
  ok "deploy/terraform/environments/dev.tfvars exists"
else
  bad "deploy/terraform/environments/dev.tfvars missing" \
      "cp deploy/terraform/environments/dev.tfvars.example deploy/terraform/environments/dev.tfvars"
fi

echo
if [ "$FAIL" -eq 0 ]; then
  printf '\033[32mpreflight passed\033[0m — the blocking prerequisites are in place.\n'
else
  printf '\033[31mpreflight FAILED\033[0m — fix the ✗ items above before applying.\n'
fi
exit "$FAIL"
