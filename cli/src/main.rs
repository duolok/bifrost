use clap::{Parser, Subcommand, Args};

#[derive(Parser)]
#[command(name = "bifrost", about = "Bifrost deployment CLI")]
struct Cli {
  /// Gateway URL (overrides config file)
  #[arg(long, global = true)]
  gateway: Option<String>,

  #[command(subcommand)]
  command: Commands,
}

#[derive(Subcommand)]
enum Commands {
  /// Trigger a deployment
  Deploy(DeployArgs),
  /// Show status
  Status(StatusArgs),
  /// Stream build logs
  Logs(LogsArgs),
  /// Manage projects
  Projects {
      #[command(subcommand)]
      action: ProjectAction,
  },
}

#[derive(Args)]
pub struct DeployArgs {
  /// Project name or ID
  #[arg(short, long)]
  pub project: String,

  /// Commit SHA (defaults to git HEAD)
  #[arg(short, long)]
  pub commit: Option<String>,
}

#[derive(Args)]
pub struct StatusArgs {
  /// Project name or ID (omit for all)
  #[arg(short, long)]
  pub project: Option<String>,
}

#[derive(Args)]
pub struct LogsArgs {
  /// Deployment ID to stream logs for
  pub deployment_id: String,
}

#[derive(Subcommand)]
pub enum ProjectAction {
  /// List all projects
  List,
  /// Create a new project
  Create {
      #[arg(short, long)]
      name: String,
      #[arg(short, long)]
      repo_url: String,
  },
  /// Get project details
  Get {
      /// Project ID
      id: String,
  },
  /// Delete a project
  Delete {
      /// Project ID
      id: String,
  },
}

fn main() {
    println!("Hello, world!");
}
