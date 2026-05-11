//! Standalone `fc-mcp-server` binary.
//!
//! Distributed separately from `fc-dev` so external integrators don't need
//! to install the dev monolith (with its embedded Postgres) just to get an
//! MCP endpoint. `fc-dev mcp` calls into the same `fc-mcp` library, so both
//! surfaces produce identical behavior.

use std::net::SocketAddr;

use anyhow::Result;
use clap::Parser;
use tracing_subscriber::EnvFilter;

/// FlowCatalyst MCP server (read-only access to event types and subscriptions).
///
/// Default transport is stdio (for Claude Code, Cursor, Claude Desktop, etc.).
/// Pass `--http` to run as a streamable HTTP service for hosted deployments.
#[derive(Parser, Debug)]
#[command(name = "fc-mcp-server", version)]
struct Cli {
    /// Run as a streamable HTTP server instead of stdio.
    #[arg(long)]
    http: bool,

    /// Bind address for `--http` mode.
    #[arg(long, env = "FC_MCP_BIND", default_value = "127.0.0.1:3100")]
    bind: SocketAddr,
}

#[tokio::main]
async fn main() -> Result<()> {
    // MCP over stdio uses stdout for JSON-RPC, so logs MUST go to stderr.
    tracing_subscriber::fmt()
        .with_env_filter(EnvFilter::try_from_default_env().unwrap_or_else(|_| "info".into()))
        .with_writer(std::io::stderr)
        .with_ansi(false)
        .init();

    let cli = Cli::parse();
    let config = fc_mcp::Config::from_env()?;

    if cli.http {
        fc_mcp::run_http(config, cli.bind).await
    } else {
        fc_mcp::run_stdio(config).await
    }
}
