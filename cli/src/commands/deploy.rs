use anyhow::Result;
use colored::Colorize;

use crate::client::BifrostClient;
use crate::DeployArgs;

pub async fn run(client: &BifrostClient, args: DeployArgs) -> Result<()> {
    let commit = match args.commit {
        Some(sha) => sha,
        None => {
            let output = std::process::Command::new("git")
                .args(["rev-parse", "HEAD"])
                .output()?;
            String::from_utf8(output.stdout)?.trim().to_string()
        }
    };

    println!("{} Deploying commit {} ...", "→".blue().bold(), &commit[..8.min(commit.len())]);

    let deployment = client.trigger_deploy(&args.project, &commit).await?;

    println!("{} Deployment queued", "✓".green().bold());
    println!("  ID:      {}", deployment.id);
    println!("  Status:  {}", deployment.status);
    println!("  Commit:  {}", deployment.commit_sha);
    Ok(())
}
