use anyhow::{Context, Result};

#[derive(Clone, Debug)]
pub struct Config {
    pub base_url: String,
    pub client_id: String,
    pub client_secret: String,
}

impl Config {
    pub fn from_env() -> Result<Self> {
        let base_url = require_env("FLOWCATALYST_URL")?
            .trim_end_matches('/')
            .to_owned();
        let client_id = require_env("FLOWCATALYST_CLIENT_ID")?;
        let client_secret = require_env("FLOWCATALYST_CLIENT_SECRET")?;
        Ok(Self {
            base_url,
            client_id,
            client_secret,
        })
    }
}

fn require_env(name: &str) -> Result<String> {
    std::env::var(name).with_context(|| format!("missing required environment variable: {name}"))
}
