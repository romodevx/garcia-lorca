output "service_name" {
  description = "Name of the Cloud Run service."
  value       = google_cloud_run_v2_service.this.name
}

output "uri" {
  description = "Public/internal URI of the deployed service."
  value       = google_cloud_run_v2_service.this.uri
}
