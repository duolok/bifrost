use anyhow::{Context, Result};
use std::env;

pub struct Config {
    pub gcp_project: String,
    pub subscription: String,           // Pub/Sub subscription for build-requests
    pub complete_topic: String,         // Pub/Sub topic for build-complete
    pub workspace_dir: String,          // temp dir for git clones
    pub realtime_url: Option<String>
}

impl Config {
    pub fn from_env() -> Result<Self> {
        Ok(Self {
            gcp_project: env::var("GCP_PROJECT").context("GCP_PROJECT required")?,
            subscription: env::var("PUBSUB_SUBSCRIPTION").unwrap_or_else(|_| "builder-subscription".into()),
            complete_topic: env::var("PUBSUB_COMPLETE_TOPIC").unwrap_or_else(|_| "build-complete".into()),
            workspace_dir: env::var("WORKSPACE_DIR").unwrap_or_else(|_| "/tmp/builder".into()),
            realtime_url: env::var("BF_REALTIME_URL").ok(),
        })
    }
}
