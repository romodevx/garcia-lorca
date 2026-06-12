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

output "artifact_registry_path" {
  description = "Base path for service images, e.g. REGION-docker.pkg.dev/PROJECT/REPO."
  value       = local.registry_path
}

output "db_connection_name" {
  description = "Cloud SQL connection name (project:region:instance) for the proxy."
  value       = google_sql_database_instance.main.connection_name
}

output "db_app_password_secret" {
  description = "Secret Manager secret id holding the app DB password."
  value       = google_secret_manager_secret.db_app_password.secret_id
}

output "payment_events_topic" {
  description = "Pub/Sub topic id for payment events."
  value       = google_pubsub_topic.payment_events.id
}
