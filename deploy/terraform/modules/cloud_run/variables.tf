# Reusable Cloud Run service module.
#
# Cost lever (see deploy/README.md): warm services (gateway, iam) pass
# min_instances=1 + cpu_idle=false + startup_cpu_boost=true. Scale-to-zero
# services pass min_instances=0 + cpu_idle=true (CPU billed only per request).

# NOTE: deliberately NOT called "env" — that name is already taken by the map of
# container environment variables below. A precondition comparing a map to "dev"
# would silently never fire, which for a cost guardrail is worse than not having
# one at all.
variable "environment" {
  description = "Deployment environment (dev/staging/prod). Drives the always-on cost guardrail below."
  type        = string
}

variable "name" {
  description = "Service name (also the Cloud Run service id), e.g. \"gateway\"."
  type        = string
}

variable "project_id" {
  description = "GCP project id the service is deployed into."
  type        = string
}

variable "region" {
  description = "Region for the Cloud Run service."
  type        = string
}

variable "image" {
  description = "Fully-qualified container image, e.g. REGION-docker.pkg.dev/PROJECT/REPO/NAME:TAG."
  type        = string
}

variable "service_account_email" {
  description = "Runtime service account the revision runs as (least privilege)."
  type        = string
}

variable "min_instances" {
  description = "Minimum instances. 1 = always warm (gateway/iam); 0 = scale-to-zero."
  type        = number
  default     = 0
}

variable "max_instances" {
  description = "Maximum instances (cost ceiling)."
  type        = number
  default     = 10
}

variable "cpu_idle" {
  description = "true = CPU only allocated during requests (cheap, scale-to-zero). false = CPU always allocated (warm services)."
  type        = bool
  default     = true
}

variable "startup_cpu_boost" {
  description = "Extra CPU during cold start. Enable on warm/critical services to cut tail latency."
  type        = bool
  default     = false
}

variable "cpu" {
  description = "CPU per instance (e.g. \"1\")."
  type        = string
  default     = "1"
}

variable "memory" {
  description = "Memory per instance (e.g. \"512Mi\")."
  type        = string
  default     = "512Mi"
}

variable "concurrency" {
  description = "Max concurrent requests per instance. High value packs more work per instance (cheaper)."
  type        = number
  default     = 80
}

variable "ingress" {
  description = "Ingress setting. \"INGRESS_TRAFFIC_ALL\" for the public gateway; \"INGRESS_TRAFFIC_INTERNAL_ONLY\" for internal gRPC services."
  type        = string
  default     = "INGRESS_TRAFFIC_INTERNAL_ONLY"
}

variable "allow_unauthenticated" {
  description = "Whether to grant roles/run.invoker to allUsers (only the public gateway should)."
  type        = bool
  default     = false
}

variable "env" {
  description = "Plain environment variables injected into the container."
  type        = map(string)
  default     = {}
}

variable "secret_env" {
  description = "Environment variables sourced from Secret Manager. Map of ENV_VAR_NAME => { secret_id, version }."
  type = map(object({
    secret_id = string
    version   = optional(string, "latest")
  }))
  default = {}
}

variable "cloudsql_instances" {
  description = "Cloud SQL connection names to mount via the built-in proxy, e.g. [\"project:region:instance\"]."
  type        = list(string)
  default     = []
}

variable "vpc_connector" {
  description = "Optional Serverless VPC Access connector id for private egress. null = none."
  type        = string
  default     = null
}
