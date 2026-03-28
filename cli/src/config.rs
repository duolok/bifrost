use std::env;

pub struct Config {
    pub gateway_url: String,
}

pub fn load() -> Config {
    Config {
        gateway_url: env::var("BIFROST_GATEWAY_URL")
            .unwrap_or_else(|_| "http://localhost:8080".into()),
    }
}
