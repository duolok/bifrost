mod buildkit;
mod config;
mod error;
mod git;
mod job;
mod message;
mod pubsub;

use anyhow::Result;
use tracing_subscriber::EnvFilter;

#[tokio::main]
async fn main() -> Result<()> {
    tracing_subscriber::fmt()
        .with_env_filter(EnvFilter::from_default_env())
        .json()
        .init();

    let cfg = config::Config::from_env()?;
    tracing::info!(
        project = %cfg.gcp_project,
        subscription = %cfg.subscription,
        "builder starting"
    );

    pubsub::run(cfg).await
}
