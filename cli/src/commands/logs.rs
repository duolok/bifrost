use anyhow::{Context, Result};
use colored::Colorize;
use futures_util::{SinkExt, StreamExt};
use serde_json::json;
use tokio_tungstenite::{connect_async, tungstenite::Message};

use crate::LogsArgs;

pub async fn run(realtime_url: &str, args: LogsArgs) -> Result<()> {
    let deploy_id = &args.deployment_id;

    let ws_url = format!(
        "{}/socket/websocket?vsn=2.0.0",
        realtime_url
            .replace("http://", "ws://")
            .replace("https://", "wss://")
            .trim_end_matches('/')
    );

    println!(
        "{} Connecting to logs for deployment {}...",
        "→".blue().bold(),
        &deploy_id[..8.min(deploy_id.len())]
    );

    let (ws_stream, _) = connect_async(&ws_url)
        .await
        .context("failed to connect to realtime service")?;

    let (mut write, mut read) = ws_stream.split();

    // Join build:logs channel (Phoenix v2 protocol)
    // Format: [join_ref, ref, topic, event, payload]
    let join_logs = json!(["1", "1", format!("build:logs:{}", deploy_id), "phx_join", {}]);
    write.send(Message::Text(join_logs.to_string())).await?;

    // Join deploy:status channel
    let join_status = json!(["2", "2", format!("deploy:status:{}", deploy_id), "phx_join", {}]);
    write.send(Message::Text(join_status.to_string())).await?;

    println!("{} Streaming logs (Ctrl+C to stop)\n", "✓".green().bold());

    // Spawn heartbeat task to keep connection alive
    let (tx, mut rx) = tokio::sync::mpsc::channel::<()>(1);
    tokio::spawn(async move {
        loop {
            tokio::select! {
                _ = tokio::time::sleep(std::time::Duration::from_secs(30)) => {
                    // Phoenix heartbeat
                    let hb = json!([null, "hb", "phoenix", "heartbeat", {}]);
                    // If send fails, channel is closed
                    let _ = tx.send(()).await;
                    drop(hb);
                }
            }
        }
    });

    loop {
        tokio::select! {
            msg = read.next() => {
                match msg {
                    Some(Ok(Message::Text(text))) => {
                        if let Ok(payload) = serde_json::from_str::<serde_json::Value>(&text) {
                            handle_message(&payload);
                        }
                    }
                    Some(Ok(Message::Close(_))) | None => {
                        println!("\n{} Connection closed", "✗".red().bold());
                        break;
                    }
                    _ => {}
                }
            }
            _ = rx.recv() => {
                let hb = json!([null, "hb", "phoenix", "heartbeat", {}]);
                write.send(Message::Text(hb.to_string())).await?;
            }
            _ = tokio::signal::ctrl_c() => {
                println!("\n{} Disconnected", "✓".green().bold());
                break;
            }
        }
    }

    Ok(())
}

fn handle_message(msg: &serde_json::Value) {
    let event = msg.get(3).and_then(|e| e.as_str()).unwrap_or("");
    let payload = &msg[4];

    match event {
        "new_log" => {
            if let Some(line) = payload.get("line").and_then(|l| l.as_str()) {
                println!("{}", line);
            }
        }
        "status_changed" => {
            let status = payload
                .get("event_type")
                .and_then(|s| s.as_str())
                .unwrap_or("unknown");
            let message = payload
                .get("message")
                .and_then(|m| m.as_str())
                .unwrap_or("");
            let colored_status = match status {
                s if s.contains("running") => s.green().to_string(),
                s if s.contains("failed") => s.red().to_string(),
                s => s.yellow().to_string(),
            };
            println!(
                "\n{} {} {}",
                "▶".bold(),
                colored_status,
                message.dimmed()
            );
        }
        "phx_reply" => {
        }
        "phx_error" => {
            eprintln!("{} Channel error", "✗".red().bold());
        }
        _ => {}
    }
}
