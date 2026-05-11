//! MCP server configuration.
//!
//! Resolution order, top to bottom:
//!   1. Explicit env vars (`FLOWCATALYST_URL`, `FLOWCATALYST_CLIENT_ID`,
//!      `FLOWCATALYST_CLIENT_SECRET`) — power-user / CI escape hatch.
//!   2. `~/.cache/flowcatalyst-dev/mcp-credentials.json` — written by
//!      `fc-dev`'s mcp-bootstrap step on startup.
//!   3. Hard-coded defaults — `base_url` only (`http://localhost:8080`).
//!
//! Missing `client_id`/`client_secret` after walking those is fatal:
//! we surface a "start fc-dev first" message rather than silently
//! sending empty credentials.

use std::path::PathBuf;

use anyhow::{anyhow, Context, Result};
use serde::Deserialize;

const DEFAULT_BASE_URL: &str = "http://localhost:8080";

#[derive(Clone, Debug)]
pub struct Config {
    pub base_url: String,
    pub client_id: String,
    pub client_secret: String,
}

#[derive(Deserialize)]
struct CredentialsFile {
    client_id: String,
    client_secret: String,
    #[serde(default)]
    base_url: Option<String>,
}

impl Config {
    /// Read env first, fall back to the credentials file, and finally
    /// surface a clear error pointing at fc-dev if no creds were found.
    pub fn from_env() -> Result<Self> {
        let env_base_url = env_opt("FLOWCATALYST_URL");
        let env_client_id = env_opt("FLOWCATALYST_CLIENT_ID");
        let env_client_secret = env_opt("FLOWCATALYST_CLIENT_SECRET");

        // Only look at the file if either credential is missing — env
        // values always win.
        let file = if env_client_id.is_none() || env_client_secret.is_none() {
            read_credentials_file()?
        } else {
            None
        };

        let client_id = env_client_id
            .or_else(|| file.as_ref().map(|f| f.client_id.clone()))
            .ok_or_else(missing_creds_error)?;
        let client_secret = env_client_secret
            .or_else(|| file.as_ref().map(|f| f.client_secret.clone()))
            .ok_or_else(missing_creds_error)?;
        let base_url = env_base_url
            .or_else(|| file.as_ref().and_then(|f| f.base_url.clone()))
            .unwrap_or_else(|| DEFAULT_BASE_URL.to_string())
            .trim_end_matches('/')
            .to_owned();

        Ok(Self {
            base_url,
            client_id,
            client_secret,
        })
    }
}

fn env_opt(name: &str) -> Option<String> {
    std::env::var(name).ok().filter(|s| !s.is_empty())
}

/// Where `fc-dev`'s `mcp_bootstrap::write_credentials_file` writes them.
/// Returns `None` only on platforms where `dirs::cache_dir()` is
/// unreachable; on those, the user must use env vars.
fn credentials_path() -> Option<PathBuf> {
    Some(
        dirs::cache_dir()?
            .join("flowcatalyst-dev")
            .join("mcp-credentials.json"),
    )
}

fn read_credentials_file() -> Result<Option<CredentialsFile>> {
    let Some(path) = credentials_path() else {
        return Ok(None);
    };
    if !path.exists() {
        return Ok(None);
    }
    let bytes = std::fs::read(&path).with_context(|| format!("reading {}", path.display()))?;
    let parsed: CredentialsFile = serde_json::from_slice(&bytes).with_context(|| {
        format!(
            "parsing {} (regenerate with `fc-dev` restart)",
            path.display()
        )
    })?;
    Ok(Some(parsed))
}

fn missing_creds_error() -> anyhow::Error {
    let hint = credentials_path()
        .map(|p| p.display().to_string())
        .unwrap_or_else(|| "<unknown>".to_string());
    anyhow!(
        "no MCP credentials found.\n\n\
         Start the fc-dev server in another terminal first:\n  \
           ./fc-dev\n\n\
         It will provision credentials at {hint} and the MCP server will pick \
         them up automatically on the next launch.\n\n\
         Alternatively, set FLOWCATALYST_CLIENT_ID and FLOWCATALYST_CLIENT_SECRET \
         (and optionally FLOWCATALYST_URL) directly.",
    )
}
