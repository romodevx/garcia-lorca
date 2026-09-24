# ---- Pupils' photos (task 5) ------------------------------------------------
#
# A PRIVATE bucket. No object is ever public and no signed URL is ever minted:
# the students service reads the bytes and the gateway serves them only after
# its own permission check (students:read), so the bucket's access policy is
# the one below and nothing else.
#
#   * public_access_prevention = "enforced": no allUsers/allAuthenticatedUsers
#     binding can be added, by mistake or otherwise.
#   * uniform bucket-level access: IAM only, no per-object ACLs.
#   * versioning: a replaced or deleted photo can be recovered for 7 days,
#     then the lifecycle rule removes the noncurrent version. Short on purpose:
#     "Quitar foto" should mean gone, not kept for a month.
#   * NOT swept by any rule: a LIVE object nothing references (a replace whose
#     cleanup failed). The students service logs "photo object not deleted"
#     with its key; that line is how such an object is found and removed.
resource "google_storage_bucket" "student_photos" {
  project                     = var.project_id
  name                        = "${var.project_id}-student-photos-${var.env}"
  location                    = var.region
  uniform_bucket_level_access = true
  public_access_prevention    = "enforced"
  force_destroy               = false
  labels                      = local.labels

  versioning {
    enabled = true
  }

  lifecycle_rule {
    condition {
      days_since_noncurrent_time = 7
    }
    action {
      type = "Delete"
    }
  }

  depends_on = [google_project_service.apis]
}

# Object read/write on THIS bucket only, for the runtime service account the
# students service runs as. The account is shared with other scale-to-zero
# services, so the grant is bucket-scoped (not project-wide) and only the
# students code opens the bucket. A dedicated service account for students is
# the tighter follow-up (see identity_platform.tf for the pattern iam uses).
resource "google_storage_bucket_iam_member" "student_photos_runtime" {
  bucket = google_storage_bucket.student_photos.name
  role   = "roles/storage.objectUser"
  member = "serviceAccount:${google_service_account.runtime.email}"
}
