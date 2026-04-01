use anyhow::Result;
use colored::Colorize;
use std::io::{self, Write};

use crate::client::BifrostClient;
use crate::config;
use crate::AuthAction;

pub async fn run(client: &BifrostClient, action: AuthAction) -> Result<()> {
    match action {
        AuthAction::Login => login(client).await,
        AuthAction::Register => register(client).await,
        AuthAction::Logout => logout(),
        AuthAction::Me => me(client).await,
    }
}

async fn login(client: &BifrostClient) -> Result<()> {
    let email = prompt("Email: ")?;
    let password = prompt_password("Password: ")?;

    let resp = client.login(&email, &password).await?;
    config::save_token(&resp.token)?;

    println!("{}", "Logged in successfully!".green());
    println!("  User:  {}", resp.user_id);
    println!("  Team:  {}", resp.team_id);
    println!("  Role:  {}", resp.role.cyan());
    Ok(())
}

async fn register(client: &BifrostClient) -> Result<()> {
    let name = prompt("Name: ")?;
    let email = prompt("Email: ")?;
    let password = prompt_password("Password (min 8 chars): ")?;

    let resp = client.register(&email, &password, &name).await?;
    config::save_token(&resp.token)?;

    println!("{}", "Account created and logged in!".green());
    println!("  User:  {}", resp.user_id);
    println!("  Team:  {}", resp.team_id);
    println!("  Role:  {}", resp.role.cyan());
    Ok(())
}

fn logout() -> Result<()> {
    config::clear_token()?;
    println!("{}", "Logged out.".green());
    Ok(())
}

async fn me(client: &BifrostClient) -> Result<()> {
    let resp = client.get_me().await?;
    println!("{}", "Current user:".bold());
    println!("  Email: {}", resp.user.email);
    if let Some(name) = &resp.user.name {
        println!("  Name:  {}", name);
    }
    println!("  Team:  {} ({})", resp.team.name, resp.team.id);
    println!("  Role:  {}", resp.role.cyan());
    Ok(())
}

fn prompt(label: &str) -> Result<String> {
    print!("{}", label);
    io::stdout().flush()?;
    let mut input = String::new();
    io::stdin().read_line(&mut input)?;
    Ok(input.trim().to_string())
}

fn prompt_password(label: &str) -> Result<String> {
    Ok(rpassword::read_password_from_tty(Some(label))?)
}
