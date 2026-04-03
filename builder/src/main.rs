mod buildkit;
mod config;
mod git;
mod job;
mod message;
mod pubsub;
mod realtime;
mod telemetry;

use anyhow::Result;

#[tokio::main]
async fn main() -> Result<()> {
    telemetry::init()?;

    let cfg = config::Config::from_env()?;
    tracing::info!(
        project = %cfg.gcp_project,
        subscription = %cfg.subscription,
        "builder starting"
    );

    let result = pubsub::run(cfg).await;
    telemetry::shutdown();
    result
}
