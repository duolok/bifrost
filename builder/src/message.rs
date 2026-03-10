use serde::{Deserialize, Serialize};

/// Received from the "build-requests" Pub/Sub topic (published by Go gateway).
#[derive(Debug, Deserialize)]
pub struct BuildRequest {
    pub deploy_id: String,
    pub project_name: String,
    pub repo_url: String,
    pub commit_sha: String,
    pub image_uri: String,
}

/// Published to the "build-complete" Pub/Sub topic (consumed by Go gateway).
#[derive(Debug, Serialize)]
pub struct BuildComplete {
    pub deploy_id: String,
    pub image_uri: String,
    pub success: bool,
    pub error_message: String,
}

impl BuildComplete {
    pub fn success(deploy_id: &str, image_uri: &str) -> Self {
        Self {
            deploy_id: deploy_id.to_string(),
            image_uri: image_uri.to_string(),
            success: true,
            error_message: String::new(),
        }
    }

    pub fn failure(deploy_id: &str, error: &str) -> Self {
        Self {
            deploy_id: deploy_id.to_string(),
            image_uri: String::new(),
            success: false,
            error_message: error.to_string(),
        }
    }
}
