resource "google_artifact_registry_repository" "apps" {
  location      = var.region
  repository_id = "bifrost-apps"
  description   = "Container images for apps deployed through bifrost"
  format        = "DOCKER"

  cleanup_policies {
    id     = "keep-recent"
    action = "KEEP"
    most_recent_versions {
      keep_count = 10
    }
  }

  depends_on = [google_project_service.apis]
}

resource "google_artifact_registry_repository" "platform" {
  location      = var.region
  repository_id = "bifrost-platform"
  description   = "Container images for bifrost platform services"
  format        = "DOCKER"

  depends_on = [google_project_service.apis]
}

