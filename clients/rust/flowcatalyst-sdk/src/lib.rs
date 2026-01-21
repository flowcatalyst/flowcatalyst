//! # FlowCatalyst SDK for Rust
//!
//! Official Rust SDK for the FlowCatalyst Platform - a multi-tenant event routing platform
//! with webhook delivery, FIFO ordering, and comprehensive subscription management.
//!
//! ## Features
//!
//! - **API Client**: Auto-generated from OpenAPI spec with full type safety
//! - **Authentication**: OIDC client credentials flow with automatic token refresh
//! - **Webhook Validation**: HMAC-SHA256 signature verification for incoming webhooks
//! - **Outbox Pattern**: Transactional outbox for reliable event publishing
//! - **TSID Support**: Time-sorted ID generation in Crockford Base32 format
//!
//! ## Quick Start
//!
//! ```rust,no_run
//! use flowcatalyst_sdk::{Client, Config};
//!
//! #[tokio::main]
//! async fn main() -> Result<(), Box<dyn std::error::Error>> {
//!     let config = Config::new("https://api.flowcatalyst.tech")
//!         .with_client_credentials("client_id", "client_secret");
//!
//!     let client = Client::new(config).await?;
//!
//!     // List event types
//!     let event_types = client.event_types().list(None, None, None).await?;
//!     println!("Found {} event types", event_types.len());
//!
//!     Ok(())
//! }
//! ```

pub mod auth;
pub mod client;
pub mod config;
pub mod error;
pub mod outbox;
pub mod tsid;
pub mod webhook;

// Re-export main types
pub use client::Client;
pub use config::Config;
pub use error::{Error, Result};

// Include generated API client
mod generated {
    include!(concat!(env!("OUT_DIR"), "/generated_client.rs"));
}

pub use generated::*;
