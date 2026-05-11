//! FlowCatalyst MCP server.
//!
//! Read-only access to FlowCatalyst event types and subscriptions for AI
//! agents. The library exposes `run_stdio` and `run_http` entrypoints; the
//! `fc-mcp-server` binary and `fc-dev mcp` both call into here.
//!
//! Codegen is intentionally not offered as a tool — LLM clients are perfectly
//! capable of producing typed code from a JSON Schema (use `get_schema`).

mod api_client;
mod auth;
mod config;
mod server;

use std::sync::Arc;

use anyhow::Result;
use rmcp::{
    transport::{
        stdio,
        streamable_http_server::{
            session::local::LocalSessionManager, StreamableHttpServerConfig, StreamableHttpService,
        },
    },
    ServiceExt,
};

pub use config::Config;
pub use server::FcMcpServer;

fn build_server(config: &Config) -> FcMcpServer {
    let http = reqwest::Client::builder()
        .user_agent(concat!("fc-mcp/", env!("CARGO_PKG_VERSION")))
        .build()
        .expect("reqwest client");
    let tokens = Arc::new(auth::TokenManager::new(config, http.clone()));
    let api = Arc::new(api_client::ApiClient::new(config, http, tokens));
    FcMcpServer::new(api)
}

/// Probe `base_url` for liveness before booting the MCP server. Without
/// this, a missing fc-dev surfaces as a confusing OAuth token failure
/// later; with it, the user sees an actionable "start fc-dev first"
/// message and exits cleanly.
async fn assert_platform_reachable(base_url: &str) -> Result<()> {
    // `/q/ready` is the canonical readiness probe; it doesn't require
    // auth and returns 200 once migrations are done and the server is
    // accepting traffic. 3s is generous for a localhost call but won't
    // make the user wait forever if they typoed the URL.
    let url = format!("{}/q/ready", base_url.trim_end_matches('/'));
    let client = reqwest::Client::builder()
        .timeout(std::time::Duration::from_secs(3))
        .user_agent(concat!("fc-mcp/", env!("CARGO_PKG_VERSION")))
        .build()?;

    match client.get(&url).send().await {
        Ok(resp) if resp.status().is_success() => Ok(()),
        Ok(resp) => Err(anyhow::anyhow!(
            "FlowCatalyst at {base_url} replied {} to /q/ready — the platform is reachable but not ready",
            resp.status()
        )),
        Err(e) => Err(anyhow::anyhow!(
            "Cannot reach FlowCatalyst at {base_url}: {e}\n\n\
             Start the dev server in another terminal first:\n  ./fc-dev",
        )),
    }
}

/// Run the MCP server over stdio. Blocks until the client disconnects.
pub async fn run_stdio(config: Config) -> Result<()> {
    assert_platform_reachable(&config.base_url).await?;
    let server = build_server(&config);
    tracing::info!("fc-mcp: stdio transport, base={}", config.base_url);
    let service = server.serve(stdio()).await?;
    service.waiting().await?;
    Ok(())
}

/// Run the MCP server as a streamable HTTP service on `addr` at `/mcp`.
pub async fn run_http(config: Config, addr: std::net::SocketAddr) -> Result<()> {
    assert_platform_reachable(&config.base_url).await?;
    let cancel = tokio_util::sync::CancellationToken::new();
    let factory_config = config.clone();
    let service = StreamableHttpService::new(
        move || Ok(build_server(&factory_config)),
        LocalSessionManager::default().into(),
        StreamableHttpServerConfig::default().with_cancellation_token(cancel.child_token()),
    );

    let router = axum::Router::new().nest_service("/mcp", service);
    let listener = tokio::net::TcpListener::bind(addr).await?;
    tracing::info!("fc-mcp: http transport listening at http://{addr}/mcp");

    axum::serve(listener, router)
        .with_graceful_shutdown(async move {
            let _ = tokio::signal::ctrl_c().await;
            cancel.cancel();
        })
        .await?;
    Ok(())
}
