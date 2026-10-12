//! # FlowCatalyst SDK for Rust
//!
//! The Rust member of the FlowCatalyst SDK family (Go `pkg/fcsdk`, Java,
//! TypeScript, Laravel), wire-compatible with them and with the platform's
//! contract (`docs/wire-contract.md` in the platform repository):
//!
//! | Module | Feature | What |
//! |---|---|---|
//! | [`tsid`] | always | time-sorted ids, typed prefixes |
//! | [`outbox`] | `outbox` (default) | event, dispatch-job and audit rows for `outbox_messages`; the canonical DDL; `OutboxManager` |
//! | `outbox::postgres` / `outbox::mysql` | `outbox-postgres` / `outbox-mysql` | sqlx drivers |
//! | [`webhook`] | `webhook` (default) | delivery signature verification, mediation responses |
//! | [`client`] | `client` (default) | the platform HTTP API, client-credentials tokens, retries |
//! | [`sync`] | `client` | declarative definition sync (roles, event types, subscriptions, ...) |
//! | `auth` | `auth` | access-token claims, JWKS/HS256 validation, the OAuth client |
//!
//! The crate never panics on input: every fallible operation returns a
//! `Result` with a `thiserror` error.
//!
//! ```
//! use flowcatalyst_sdk::outbox::{CreateEventDto, DispatchJob};
//!
//! let event = CreateEventDto::new("orders:sales:order:created", &serde_json::json!({"orderId": "123"}))?
//!     .message_group("orders:order:123");
//! let job = DispatchJob::builder("orders:billing:export", "https://billing.example.com/hooks/export")
//!     .build()?;
//! # Ok::<(), Box<dyn std::error::Error>>(())
//! ```

pub mod tsid;

#[cfg(feature = "outbox")]
pub mod outbox;

#[cfg(feature = "webhook")]
pub mod webhook;

#[cfg(feature = "client")]
pub mod client;

#[cfg(feature = "client")]
pub mod sync;

#[cfg(feature = "auth")]
pub mod auth;

pub use tsid::{EntityType, Tsid};
