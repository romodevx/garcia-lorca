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
}
