mod client;
mod commands;
mod config;

use anyhow::Result;
use clap::{Parser, Subcommand, Args};

#[derive(Parser)]
#[command(name = "bifrost", about = "Bifrost deployment CLI")]
struct Cli {
  #[arg(long, global = true)]
  gateway: Option<String>,

  #[command(subcommand)]
  command: Commands,
}

#[derive(Subcommand)]
enum Commands {
  Auth {
      #[command(subcommand)]
      action: AuthAction,
  },
  Deploy(DeployArgs),
  Status(StatusArgs),
  Logs(LogsArgs),
  Rollback(RollbackArgs),
  Projects {
      #[command(subcommand)]
      action: ProjectAction,
  },
  Secrets {
      #[command(subcommand)]
      action: SecretAction,
  },
}

#[derive(Subcommand)]
pub enum AuthAction {
  /// Log in with email and password
  Login,
  /// Create a new account
  Register,
  /// Log out and clear stored token
  Logout,
  /// Show current user info
  Me,
}

#[derive(Args)]
pub struct DeployArgs {
  #[arg(short, long)]
  pub project: String,

  #[arg(short, long)]
  pub commit: Option<String>,
}

#[derive(Args)]
pub struct StatusArgs {
  #[arg(short, long)]
  pub project: Option<String>,
}

#[derive(Args)]
pub struct LogsArgs {
  pub deployment_id: String,
}

#[derive(Args)]
pub struct RollbackArgs {
  #[arg(short, long)]
  pub project: String,
}

#[derive(Subcommand)]
pub enum SecretAction {
  List {
      #[arg(short, long)]
      project: String,
  },

  Set {
      #[arg(short, long)]
      project: String,
      #[arg(short, long)]
      key: String,
      #[arg(long)]
      secret_ref: String,
  },

  Delete {
      #[arg(short, long)]
      project: String,
      #[arg(short, long)]
      key: String,
  },
}

#[derive(Subcommand)]
pub enum ProjectAction {
  List,
  Create {
      #[arg(short, long)]
      name: String,
      #[arg(short, long)]
      repo_url: String,
  },

  Get {
      id: String,
  },

  Delete {
      id: String,
  },
}

#[tokio::main]
async fn main() -> Result<()> {
    let cli = Cli::parse();
    let cfg = config::load();
    let token = config::load_token();
    let gateway_url = cli.gateway.unwrap_or(cfg.gateway_url);
    let realtime_url = cfg.realtime_url;
    let client = client::BifrostClient::new(&gateway_url, token);

    match cli.command {
        Commands::Auth { action } => commands::auth::run(&client, action).await,
        Commands::Deploy(args) => commands::deploy::run(&client, args).await,
        Commands::Status(args) => commands::status::run(&client, args).await,
        Commands::Logs(args) => commands::logs::run(&realtime_url, args).await,
        Commands::Rollback(args) => commands::rollback::run(&client, args).await,
        Commands::Projects { action } => commands::projects::run(&client, action).await,
        Commands::Secrets { action } => commands::secrets::run(&client, action).await,
    }
}
