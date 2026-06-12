# Cloud Run v2 service. All services share this module; behaviour differs only
# by the variables passed in (min_instances, cpu_idle, startup_cpu_boost, etc).

resource "google_cloud_run_v2_service" "this" {
  name     = var.name
  project  = var.project_id
  location = var.region
  ingress  = var.ingress

  template {
    service_account                  = var.service_account_email
    max_instance_request_concurrency = var.concurrency

    scaling {
      min_instance_count = var.min_instances
      max_instance_count = var.max_instances
    }

    # Mount Cloud SQL via the built-in proxy when connection names are provided.
    dynamic "volumes" {
      for_each = length(var.cloudsql_instances) > 0 ? [1] : []
      content {
        name = "cloudsql"
        cloud_sql_instance {
          instances = var.cloudsql_instances
        }
      }
    }

    dynamic "vpc_access" {
      for_each = var.vpc_connector == null ? [] : [1]
      content {
        connector = var.vpc_connector
        egress    = "PRIVATE_RANGES_ONLY"
      }
    }

    containers {
      image = var.image

      resources {
        # cpu_idle=false keeps CPU allocated between requests (warm services).
        # startup_cpu_boost shortens cold starts on the critical path.
        cpu_idle          = var.cpu_idle
        startup_cpu_boost = var.startup_cpu_boost
        limits = {
          cpu    = var.cpu
          memory = var.memory
        }
      }

      dynamic "volume_mounts" {
        for_each = length(var.cloudsql_instances) > 0 ? [1] : []
        content {
          name       = "cloudsql"
          mount_path = "/cloudsql"
        }
      }

      dynamic "env" {
        for_each = var.env
        content {
          name  = env.key
          value = env.value
        }
      }

      # Secrets are never inlined: they are referenced from Secret Manager and
      # injected at runtime. No secret value ever appears in Terraform state here.
      dynamic "env" {
        for_each = var.secret_env
        content {
          name = env.key
          value_source {
            secret_key_ref {
              secret  = env.value.secret_id
              version = env.value.version
            }
          }
        }
      }

      # Liveness/startup probes. The container must expose gRPC health or /healthz.
      startup_probe {
        http_get {
          path = "/healthz"
        }
        initial_delay_seconds = 2
        period_seconds        = 5
        failure_threshold     = 5
      }
    }
  }
}

# Only the public gateway should be invokable unauthenticated; internal gRPC
# services are reached via authenticated service-to-service identity.
resource "google_cloud_run_v2_service_iam_member" "public_invoker" {
  count    = var.allow_unauthenticated ? 1 : 0
  project  = var.project_id
  location = var.region
  name     = google_cloud_run_v2_service.this.name
  role     = "roles/run.invoker"
  member   = "allUsers"
}
