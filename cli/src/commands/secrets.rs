use anyhow::Result;
use colored::Colorize;

use crate::client::BifrostClient;
use crate::SecretAction;

pub async fn run(client: &BifrostClient, action: SecretAction) -> Result<()> {
    match action {
        SecretAction::List { project } => {
            let secrets = client.list_secrets(&project).await?;
            if secrets.is_empty() {
                println!("{}", "No secrets configured.".dimmed());
                return Ok(());
            }
            println!("{:<20} {}", "KEY", "SECRET REF");
            println!("{}", "-".repeat(60));
            for s in secrets {
                println!("{:<20} {}", s.key_name, s.secret_ref.dimmed());
            }
        }
        SecretAction::Set { project, key, secret_ref } => {
            client.set_secret(&project, &key, &secret_ref).await?;
            println!("{} Secret {} set", "✓".green().bold(), key);
        }
        SecretAction::Delete { project, key } => {
            client.delete_secret(&project, &key).await?;
            println!("{} Secret {} deleted", "✓".green().bold(), key);
        }
    }
    Ok(())
}
