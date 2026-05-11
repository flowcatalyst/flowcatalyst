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

/// Run the MCP server over stdio. Blocks until the client disconnects.
pub async fn run_stdio(config: Config) -> Result<()> {
    let server = build_server(&config);
    tracing::info!("fc-mcp: stdio transport, base={}", config.base_url);
    let service = server.serve(stdio()).await?;
    service.waiting().await?;
    Ok(())
}

/// Run the MCP server as a streamable HTTP service on `addr` at `/mcp`.
pub async fn run_http(config: Config, addr: std::net::SocketAddr) -> Result<()> {
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
