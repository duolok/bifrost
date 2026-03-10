resource "google_sql_database_instance" "main" {
  name             = "bifrost-db"
  database_version = "POSTGRES_15"
  region           = var.region

  settings {
    tier = "db-f1-micro"

    ip_configuration {
      ipv4_enabled = true

      authorized_networks {
        name  = "allow-all-dev"
        value = "0.0.0.0/0"
      }
    }

    backup_configuration {
      enabled    = true
      start_time = "03:00"
    }
  }

  deletion_protection = false

  depends_on = [google_project_service.apis]
}

resource "google_sql_database" "bifrost" {
  name     = "bifrost"
  instance = google_sql_database_instance.main.name
}

resource "google_sql_user" "bifrost" {
  name     = "bifrost"
  instance = google_sql_database_instance.main.name
  password = var.db_password
}

