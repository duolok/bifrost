use anyhow::Result;
use colored::Colorize;

use crate::client::BifrostClient;
use crate::ProjectAction;

pub async fn run(client: &BifrostClient, action: ProjectAction) -> Result<()> {
    match action {
        ProjectAction::List => {
            let projects = client.list_projects().await?;
            if projects.is_empty() {
                println!("{}", "No projects found.".dimmed());
                return Ok(());
            }

            println!("{:<38} {:<20} {:<10} {}", "ID", "NAME", "STATUS", "REPO");
            println!("{}", "-".repeat(90));

            for p in projects {
                let status = match p.status.as_str() {
                    "active" => p.status.green(),
                    "archived" => p.status.red(),
                    _ => p.status.yellow(),
                };
                println!("{:<38} {:<20} {:<10} {}", p.id, p.name, status, p.repo_url);
            }
        }
        ProjectAction::Create { name, repo_url } => {
            let project = client.create_project(&name, &repo_url).await?;
            println!("{} Project created", "✓".green().bold());
            println!("  ID:     {}", project.id);
            println!("  Name:   {}", project.name);
            println!("  Repo:   {}", project.repo_url);
            println!("  Secret: {}", project.webhook_secret.unwrap_or_default().dimmed());
        }
        ProjectAction::Get { id } => {
            let p = client.get_project(&id).await?;
            println!("  ID:       {}", p.id);
            println!("  Name:     {}", p.name);
            println!("  Repo:     {}", p.repo_url);
            println!("  Branch:   {}", p.default_branch);
            println!("  Status:   {}", p.status);
            println!("  Created:  {}", p.created_at);
        }
        ProjectAction::Delete { id } => {
            client.delete_project(&id).await?;
            println!("{} Project {} deleted", "✓".green().bold(), id);
        }
    }
    Ok(())
}
