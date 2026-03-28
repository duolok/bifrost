use anyhow::Result;
use colored::Colorize;

use crate::client::BifrostClient;
use crate::StatusArgs;

pub async fn run(client: &BifrostClient, args: StatusArgs) -> Result<()> {
    match args.project {
        Some(id) => {
            let deployments = client.list_deployments(&id).await?;
            if deployments.is_empty() {
                println!("{}", "No deployments found.".dimmed());
                return Ok(());
            }
            println!("{:<38} {:<12} {:<12} {}", "ID", "STATUS", "TRIGGERED", "COMMIT");
            println!("{}", "-".repeat(80));
            for d in deployments {
                let status = match d.status.as_str() {
                    "running" | "healthy" => d.status.green(),
                    "failed" => d.status.red(),
                    "building" | "deploying" => d.status.yellow(),
                    _ => d.status.normal(),
                };
                println!("{:<38} {:<12} {:<12} {}", d.id, status, d.triggered_by, &d.commit_sha[..8.min(d.commit_sha.len())]);
            }
        }
        None => {
            let projects = client.list_projects().await?;
            if projects.is_empty() {
                println!("{}", "No projects found.".dimmed());
                return Ok(());
            }
            for p in projects {
                println!("{} ({})", p.name.bold(), p.status);
                let deployments = client.list_deployments(&p.id).await?;
                if deployments.is_empty() {
                    println!("  No deployments\n");
                    continue;
                }
                for d in deployments.iter().take(3) {
                    let status = match d.status.as_str() {
                        "running" | "healthy" => d.status.green(),
                        "failed" => d.status.red(),
                        _ => d.status.yellow(),
                    };
                    println!("  {} {} {}", &d.commit_sha[..8.min(d.commit_sha.len())], status, d.triggered_by);
                }
                println!();
            }
        }
    }
    Ok(())
}
