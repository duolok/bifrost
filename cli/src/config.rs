use std::env;
use std::fs;
use std::path::PathBuf;

pub struct Config {
    pub gateway_url: String,
    pub realtime_url: String,
}

pub fn load() -> Config {
    Config {
        gateway_url: env::var("BIFROST_GATEWAY_URL")
            .unwrap_or_else(|_| "http://localhost:8080".into()),
        realtime_url: env::var("BIFROST_REALTIME_URL")
            .unwrap_or_else(|_| "http://localhost:4000".into()),
    }
}

fn token_path() -> PathBuf {
    dirs::config_dir()
        .unwrap_or_else(|| PathBuf::from("."))
        .join("bifrost")
        .join("token")
}

pub fn load_token() -> Option<String> {
    fs::read_to_string(token_path()).ok().map(|s| s.trim().to_string()).filter(|s| !s.is_empty())
}

pub fn save_token(token: &str) -> anyhow::Result<()> {
    let path = token_path();
    if let Some(parent) = path.parent() {
        fs::create_dir_all(parent)?;
    }
    fs::write(&path, token)?;
    Ok(())
}

pub fn clear_token() -> anyhow::Result<()> {
    let path = token_path();
    if path.exists() {
        fs::remove_file(&path)?;
    }
    Ok(())
}
