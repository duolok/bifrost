use anyhow::{Context, Result};
use std::env;

pub struct Config {
    pub gcp_project: String,
    pub subscription: String,           // Pub/Sub subscription for build-requests
    pub complete_topic: String,         // Pub/Sub topic for build-complete
    pub buildkitd_addr: String,         // buildkitd socket address
    pub workspace_dir: String,          // temp dir for git clones
}

impl Config {
    pub fn from_env() -> Result<Self> {
        Ok(Self {
            gcp_project: env::var("GCP_PROJECT").context("GCP_PROJECT required")?,
            subscription: env::var("PUBSUB_SUBSCRIPTION").unwrap_or_else(|_| "builder-subscription".into()),
            complete_topic: env::var("PUBSUB_COMPLETE_TOPIC").unwrap_or_else(|_| "build-complete".into()),
            buildkitd_addr: env::var("BUILDKITD_ADDR").unwrap_or_else(|_| "unix:///run/buildkit/buildkitd.sock".into()),
            workspace_dir: env::var("WORKSPACE_DIR").unwrap_or_else(|_| "/tmp/builder".into()),
        })
    }
}
