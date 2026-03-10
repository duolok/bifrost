resource "google_pubsub_topic" "build_requests" {
  name                       = "build-requests"
  message_retention_duration = "604800s"

  depends_on = [google_project_service.apis]
}

resource "google_pubsub_topic" "build_complete" {
  name                       = "build-complete"
  message_retention_duration = "604800s"

  depends_on = [google_project_service.apis]
}

resource "google_pubsub_topic" "deploy_events" {
  name                       = "deploy-events"
  message_retention_duration = "604800s"

  depends_on = [google_project_service.apis]
}

resource "google_pubsub_topic" "dead_letter" {
  name = "dead-letter"

  depends_on = [google_project_service.apis]
}

resource "google_pubsub_subscription" "builder_sub" { 
  name  = "builder-subscription"
  topic = google_pubsub_topic.build_requests.name
  ack_deadline_seconds = 600
  retry_policy {
    minimum_backoff = "10s"
    maximum_backoff = "600s"
  }
  dead_letter_policy {
    dead_letter_topic     = google_pubsub_topic.dead_letter.id
    max_delivery_attempts = 5
  }
}

resource "google_pubsub_subscription" "gateway_build_complete" {
  name                 = "gateway-build-complete"
  topic                = google_pubsub_topic.build_complete.name
  ack_deadline_seconds = 60
}

resource "google_pubsub_subscription" "realtime_events" {
  name                 = "realtime-events"
  topic                = google_pubsub_topic.deploy_events.name
  ack_deadline_seconds = 30
}

resource "google_pubsub_subscription" "dead_letter_monitor" {
  name                 = "dead-letter-monitor"
  topic                = google_pubsub_topic.dead_letter.name
  ack_deadline_seconds = 60
}
