use std::time::{SystemTime, UNIX_EPOCH};
use tonic::transport::Channel;

pub mod events {
    tonic::include_proto!("bifrost.events");
}

use events::event_ingress_client::EventIngressClient;
use events::{BuildLogLine, PlatformEvent};

pub struct RealtimeClient {
    client: EventIngressClient<Channel>,
}

impl RealtimeClient {
      pub async fn connect(addr: &str) -> anyhow::Result<Self> {
          let client = EventIngressClient::connect(addr.to_string()).await?;
          Ok(Self { client })
      }

      fn now() -> i64 {
          SystemTime::now()
              .duration_since(UNIX_EPOCH)
              .unwrap()
              .as_secs() as i64
      }

      pub async fn send_event(
          &mut self,
          event_type: &str,
          deploy_id: &str,
          project_name: &str,
          message: &str,
      ) {
          let result = self
              .client
              .send_event(PlatformEvent {
                  event_type: event_type.into(),
                  deploy_id: deploy_id.into(),
                  project_name: project_name.into(),
                  actor: "builder".into(),
                  message: message.into(),
                  timestamp: Self::now(),
                  metadata: Default::default(),
              })
              .await;

          if let Err(e) = result {
              tracing::warn!(event_type, deploy_id, error = %e, "failed to emit event");
          }
      }

      pub async fn send_log(&mut self, deploy_id: &str, line: String) {
          let log_line = BuildLogLine {
              deploy_id: deploy_id.into(),
              line,
              timestamp: Self::now(),
          };

          let stream = tokio_stream::iter(vec![log_line]);
          if let Err(e) = self.client.stream_build_logs(stream).await {
              tracing::warn!(deploy_id, error = %e, "failed to stream log line");
          }
      }
  }

