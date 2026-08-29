terraform {
  required_version = ">= 1.7.0"

  required_providers {
    archive = {
      source  = "hashicorp/archive"
      version = "~> 2.4"
    }
    random = {
      source  = "hashicorp/random"
      version = "~> 3.6"
    }
    google = {
      source  = "hashicorp/google"
      version = ">= 5.30.0, < 6.0.0"
    }
  }

  # Remote state lives in a GCS bucket per environment. The bucket name is
  # supplied at init time so it is not hardcoded here:
  #   terraform init -backend-config="bucket=garcia-lorca-tfstate-dev" \
  #                  -backend-config="prefix=cloud-run/dev"
  backend "gcs" {}
}

provider "google" {
  project = var.project_id
  region  = var.region

  # Required for the billing-budget API. When Terraform authenticates with local
  # Application Default Credentials there is no quota project attached, and
  # billingbudgets.googleapis.com refuses the call with a 403 that reads like a
  # permissions problem but is not:
  #   "requires a quota project, which is not set by default"
  #
  # Setting it here rather than relying on
  # `gcloud auth application-default set-quota-project` means the apply works on
  # a fresh machine without anyone having to know that command — and the budget is
  # the guardrail this whole environment exists to have, so it must not be the one
  # resource that quietly fails to create.
  user_project_override = true
  billing_project       = var.project_id
}
