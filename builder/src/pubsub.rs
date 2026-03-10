use anyhow::Result;

use crate::config::Config;
use crate::message::{BuildComplete, BuildRequest};

/// Main loop: subscribe to build-requests, run jobs, publish build-complete.
///
/// 1. Create google_cloud_pubsub::client::Client (use ClientConfig::default().with_auth().await)
/// 2. Get subscription handle (cfg.subscription)
/// 3. Get topic handle for publishing (cfg.complete_topic)
/// 4. Create a publisher from the topic
/// 5. Loop: subscription.receive(|message, cancel| async { ... })
///    a. Deserialize message.data as BuildRequest
///    b. Call job::run(cfg, request) -> BuildComplete
///    c. Serialize BuildComplete to JSON, publish to complete_topic
///    d. Ack the message
///
/// On malformed messages: log error, ack (don't let them loop).
/// On job failure: BuildComplete.success=false handles it, still ack.
pub async fn run(cfg: Config) -> Result<()> {
    todo!()
}
