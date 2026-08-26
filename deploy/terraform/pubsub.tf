# Pub/Sub with the dead-letter (DLQ) pattern. payment-events is the canonical
# example; finance publishes overdue/payment events consumed by comms (push).
# Other domain topics follow the same topic + subscription + DLQ shape.

resource "google_pubsub_topic" "payment_events" {
  project = var.project_id
  name    = "payment-events"
  labels  = local.labels

  depends_on = [google_project_service.apis]
}

# Dead-letter topic: messages that exhaust delivery attempts land here for
# inspection/replay instead of being lost or retried forever.
resource "google_pubsub_topic" "payment_events_dlq" {
  project = var.project_id
  name    = "payment-events-dlq"
  labels  = local.labels

  depends_on = [google_project_service.apis]
}

# NOTE: the subscription NAME must match what the comms consumer subscribes to
# (services/comms/cmd/main.go consumedTopics: "payment-events" ->
# "comms-payment-events"). Pre-creating it here means the runtime finds it
# existing instead of auto-creating it, so the runtime SA needs only
# publisher/subscriber — never pubsub.subscriptions.create.
resource "google_pubsub_subscription" "payment_events_comms" {
  project = var.project_id
  name    = "comms-payment-events"
  topic   = google_pubsub_topic.payment_events.id

  ack_deadline_seconds = 30

  # Authenticated PUSH: Pub/Sub POSTs each message to comms /pubsub and wakes it
  # from scale-to-zero, so comms no longer runs a pull loop. The oidc_token lets
  # comms verify the caller is Pub/Sub (audience = the service URI; the push SA
  # carries run.invoker on comms — see main.tf). Presence of push_config makes
  # this a push subscription; the pull semantics are dropped by that alone.
  push_config {
    push_endpoint = "${module.comms.uri}/pubsub"
    oidc_token {
      service_account_email = google_service_account.pubsub_push.email
      audience              = module.comms.uri
    }
  }

  # Redeliver with backoff; after max_delivery_attempts, route to the DLQ.
  retry_policy {
    minimum_backoff = "10s"
    maximum_backoff = "600s"
  }

  dead_letter_policy {
    dead_letter_topic     = google_pubsub_topic.payment_events_dlq.id
    max_delivery_attempts = 5
  }

  # Replay window for at-least-once consumers that need to recover.
  message_retention_duration = "604800s" # 7 days
  expiration_policy {
    ttl = "" # never expire the subscription
  }
}

# A subscription on the DLQ so operators can drain/inspect failed messages.
resource "google_pubsub_subscription" "payment_events_dlq_drain" {
  project = var.project_id
  name    = "payment-events-dlq-drain"
  topic   = google_pubsub_topic.payment_events_dlq.id

  ack_deadline_seconds       = 60
  message_retention_duration = "604800s" # 7 days
}

# Pub/Sub's service agent needs publish rights on the DLQ and subscribe rights
# on the source subscription for dead-lettering to work.
data "google_project" "current" {
  project_id = var.project_id
}

locals {
  pubsub_sa = "serviceAccount:service-${data.google_project.current.number}@gcp-sa-pubsub.iam.gserviceaccount.com"
}

resource "google_pubsub_topic_iam_member" "dlq_publisher" {
  project = var.project_id
  topic   = google_pubsub_topic.payment_events_dlq.id
  role    = "roles/pubsub.publisher"
  member  = local.pubsub_sa
}

resource "google_pubsub_subscription_iam_member" "dlq_subscriber" {
  project      = var.project_id
  subscription = google_pubsub_subscription.payment_events_comms.id
  role         = "roles/pubsub.subscriber"
  member       = local.pubsub_sa
}

# ---- attendance-events (attendance publishes; comms consumes) --------------
# Pre-declared so the attendance publisher and comms consumer never rely on
# runtime auto-creation (keeps the runtime SA at publisher/subscriber only).
# Mirrors the payment-events topic + DLQ + comms-subscription shape above.
resource "google_pubsub_topic" "attendance_events" {
  project = var.project_id
  name    = "attendance-events"
  labels  = local.labels

  depends_on = [google_project_service.apis]
}

resource "google_pubsub_topic" "attendance_events_dlq" {
  project = var.project_id
  name    = "attendance-events-dlq"
  labels  = local.labels

  depends_on = [google_project_service.apis]
}

# Subscription NAME matches the comms consumer (consumedTopics:
# "attendance-events" -> "comms-attendance-events").
resource "google_pubsub_subscription" "attendance_events_comms" {
  project = var.project_id
  name    = "comms-attendance-events"
  topic   = google_pubsub_topic.attendance_events.id

  ack_deadline_seconds = 30

  # Authenticated PUSH (same shape as comms-payment-events above): Pub/Sub POSTs
  # to comms /pubsub with an OIDC token, waking it from scale-to-zero.
  push_config {
    push_endpoint = "${module.comms.uri}/pubsub"
    oidc_token {
      service_account_email = google_service_account.pubsub_push.email
      audience              = module.comms.uri
    }
  }

  retry_policy {
    minimum_backoff = "10s"
    maximum_backoff = "600s"
  }

  dead_letter_policy {
    dead_letter_topic     = google_pubsub_topic.attendance_events_dlq.id
    max_delivery_attempts = 5
  }

  message_retention_duration = "604800s" # 7 days
  expiration_policy {
    ttl = "" # never expire the subscription
  }
}

resource "google_pubsub_subscription" "attendance_events_dlq_drain" {
  project = var.project_id
  name    = "attendance-events-dlq-drain"
  topic   = google_pubsub_topic.attendance_events_dlq.id

  ack_deadline_seconds       = 60
  message_retention_duration = "604800s" # 7 days
}

resource "google_pubsub_topic_iam_member" "attendance_dlq_publisher" {
  project = var.project_id
  topic   = google_pubsub_topic.attendance_events_dlq.id
  role    = "roles/pubsub.publisher"
  member  = local.pubsub_sa
}

resource "google_pubsub_subscription_iam_member" "attendance_dlq_subscriber" {
  project      = var.project_id
  subscription = google_pubsub_subscription.attendance_events_comms.id
  role         = "roles/pubsub.subscriber"
  member       = local.pubsub_sa
}

# ---- student-events (students publishes; finance consumes) -----------------
# comms deliberately does NOT subscribe here (it renders nothing from
# student-events). finance does, since M15: StudentEnrolled is what charges a
# pupil's inscription the moment they take a place in a group.
resource "google_pubsub_topic" "student_events" {
  project = var.project_id
  name    = "student-events"
  labels  = local.labels

  depends_on = [google_project_service.apis]
}

resource "google_pubsub_topic" "student_events_dlq" {
  project = var.project_id
  name    = "student-events-dlq"
  labels  = local.labels

  depends_on = [google_project_service.apis]
}

# NOTE: the name must match what finance subscribes to
# (services/finance/cmd/main.go studentEventsSubscription). Pre-creating it here
# means the runtime finds it existing, so the runtime SA needs only
# publisher/subscriber — never pubsub.subscriptions.create.
#
# finance keeps NO idempotency ledger for this, unlike comms: the inscription
# charge is keyed by its external_ref, so a redelivery inserts nothing. That is
# also why redelivery here is cheap rather than dangerous.
resource "google_pubsub_subscription" "student_events_finance" {
  project = var.project_id
  name    = "finance-student-events"
  topic   = google_pubsub_topic.student_events.id

  ack_deadline_seconds = 30

  push_config {
    push_endpoint = "${module.finance.uri}/pubsub"
    oidc_token {
      service_account_email = google_service_account.pubsub_push.email
      audience              = module.finance.uri
    }
  }

  retry_policy {
    minimum_backoff = "10s"
    maximum_backoff = "600s"
  }

  dead_letter_policy {
    dead_letter_topic     = google_pubsub_topic.student_events_dlq.id
    max_delivery_attempts = 5
  }

  message_retention_duration = "604800s" # 7 days
  expiration_policy {
    ttl = "" # never expire the subscription
  }
}

resource "google_pubsub_subscription" "student_events_dlq_drain" {
  project = var.project_id
  name    = "student-events-dlq-drain"
  topic   = google_pubsub_topic.student_events_dlq.id

  message_retention_duration = "604800s"
  expiration_policy {
    ttl = ""
  }
}

# The DLQ machinery needs Pub/Sub's own service agent to be able to publish into
# the dead-letter topic and to ack on the source subscription.
resource "google_pubsub_topic_iam_member" "student_events_dlq_publisher" {
  project = var.project_id
  topic   = google_pubsub_topic.student_events_dlq.id
  role    = "roles/pubsub.publisher"
  member  = local.pubsub_sa
}

resource "google_pubsub_subscription_iam_member" "student_events_dlq_subscriber" {
  project      = var.project_id
  subscription = google_pubsub_subscription.student_events_finance.id
  role         = "roles/pubsub.subscriber"
  member       = local.pubsub_sa
}
