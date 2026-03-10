variable "project_id" {
  description = "GCP project ID"
  type        = string
}

variable "region" {
  description = "GCP region"
  type        = string
  default     = "europe-central2"
}

variable "db_password" {
  description = "Cloud SQL password"
  type        = string
  sensitive   = true
}
