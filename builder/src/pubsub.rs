use std::sync::Arc;

use anyhow::Result;
use google_cloud_pubsub::client::{Publisher, Subscriber};
use google_cloud_pubsub::model::Message;

use crate::config::Config;
use crate::job;
use crate::message::BuildRequest;
use crate::realtime::RealtimeClient;

pub async fn run(cfg: Config) -> Result<()> {
    let cfg = Arc::new(cfg);

    let sub_resource = format!(
        "projects/{}/subscriptions/{}",
        cfg.gcp_project, cfg.subscription
    );
    let topic_resource = format!(
        "projects/{}/topics/{}",
        cfg.gcp_project, cfg.complete_topic
    );

    let subscriber = Subscriber::builder().build().await?;
    let publisher = Publisher::builder(&topic_resource).build().await?;

    let mut rt_client = match &cfg.realtime_url {
        Some(url) => match RealtimeClient::connect(url).await {
            Ok(c) => {
                tracing::info!(url = %url, "realtime client connected");
                Some(c)
            }
            Err(e) => {
                tracing::warn!(error = %e, "realtime client unavailable, log streaming disabled");
                None
            }
        },
        None => None,
    };

    tracing::info!("listening for build requests");

    let mut session = subscriber.streaming_pull(&sub_resource).start();

    while let Some(result) = session.next().await {
        let (msg, handle) = match result {
            Ok(pair) => pair,
            Err(e) => {
                tracing::error!(error = %e, "streaming pull error");
                continue;
            }
        };

        let data = msg.data.as_ref();

        // Parse the build request
        let req: BuildRequest = match serde_json::from_slice(data) {
            Ok(r) => r,
            Err(e) => {
                tracing::error!(error = %e, "malformed build request, acking to discard");
                handle.ack();
                continue;
            }
        };

        tracing::info!(deploy_id = %req.deploy_id, project = %req.project_name, "build started");

        let cfg = Arc::clone(&cfg);
        let publisher = publisher.clone();

        // Run the build
        let result = job::run(&cfg, req, rt_client.as_mut()).await;
        tracing::info!(deploy_id = %result.deploy_id, success = result.success, "build finished");

        match serde_json::to_vec(&result) {
            Ok(payload) => {
                let pubsub_msg = Message::new().set_data(payload);
                if let Err(e) = publisher.publish(pubsub_msg).await {
                    tracing::error!(deploy_id = %result.deploy_id, error = %e, "failed to publish build-complete");
                    continue;
                }
            }
            Err(e) => {
                tracing::error!(deploy_id = %result.deploy_id, error = %e, "failed to serialize build-complete");
            }
        }

        handle.ack();
    }

    Ok(())
}
