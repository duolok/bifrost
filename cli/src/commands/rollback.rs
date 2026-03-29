use anyhow::Result;
use colored::Colorize;

use crate::client::BifrostClient;
use crate::RollbackArgs;

pub async fn run(client: &BifrostClient, args: RollbackArgs) -> Result<()> {
    println!("{} Rolling back project {}...", "→".blue().bold(), args.project);

    let result = client.rollback(&args.project).await?;

    println!("{} Rollback successful", "✓".green().bold());
    println!("  Deployment:    {}", result.deployment_id);
    println!("  Rolled back to: {}", result.rolled_back_to);
    Ok(())
}
