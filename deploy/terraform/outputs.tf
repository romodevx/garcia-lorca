output "gateway_uri" {
  description = "Public URL of the gateway (front-end entry point)."
  value       = module.gateway.uri
}

output "iam_uri" {
  description = "Internal URI of the iam service."
  value       = module.iam.uri
}

output "students_uri" {
  description = "Internal URI of the students service."
  value       = module.students.uri
}

output "config_uri" {
  description = "Internal URI of the config service."
  value       = module.config.uri
}

output "finance_uri" {
  description = "Internal URI of the finance service."
  value       = module.finance.uri
}

output "grades_uri" {
  description = "Internal URI of the grades service."
  value       = module.grades.uri
}

output "attendance_uri" {
  description = "Internal URI of the attendance service."
  value       = module.attendance.uri
}

output "comms_uri" {
  description = "Internal URI of the comms service."
  value       = module.comms.uri
}

output "artifact_registry_path" {
  description = "Base path for service images, e.g. REGION-docker.pkg.dev/PROJECT/REPO."
  value       = local.registry_path
}

output "db_connection_name" {
  description = "Cloud SQL connection name (project:region:instance) for the proxy. Empty when use_cloud_sql = false."
  value       = var.use_cloud_sql ? google_sql_database_instance.main[0].connection_name : ""
}

output "db_app_password_secret" {
  description = "Secret Manager secret id holding the app DB password. Empty when use_cloud_sql = false — the whole DSN lives in the database-url secret instead."
  value       = var.use_cloud_sql ? google_secret_manager_secret.db_app_password[0].secret_id : ""
}

output "database_url_secret" {
  description = "Secret Manager secret id holding the external Postgres DSN. Empty when use_cloud_sql = true."
  value       = var.use_cloud_sql ? "" : google_secret_manager_secret.database_url[0].secret_id
}

output "iam_jwt_secret_name" {
  description = "Secret Manager secret id holding the iam token signing key. iam refuses to boot without it outside a local environment."
  value       = google_secret_manager_secret.iam_jwt_secret.secret_id
}

output "web_url" {
  description = "Public URL of the web front end — the one link to hand to the school."
  value       = module.web.uri
}

output "payment_events_topic" {
  description = "Pub/Sub topic id for payment events."
  value       = google_pubsub_topic.payment_events.id
}

output "attendance_events_topic" {
  description = "Pub/Sub topic id for attendance events."
  value       = google_pubsub_topic.attendance_events.id
}

output "student_events_topic" {
  description = "Pub/Sub topic id for student events."
  value       = google_pubsub_topic.student_events.id
}

output "build_service_account" {
  description = "Pass this to Cloud Build: gcloud builds submit --service-account=projects/PROJECT/serviceAccounts/THIS"
  value       = google_service_account.build.email
}

output "billing_killswitch_sa" {
  description = "Service account that must hold roles/billing.projectManager ON THE BILLING ACCOUNT for the kill switch to work. Empty when the kill switch is disabled."
  value       = var.enable_billing_killswitch ? google_service_account.billing_killswitch[0].email : ""
}
