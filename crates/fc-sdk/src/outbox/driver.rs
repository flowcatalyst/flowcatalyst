//! Outbox Driver Trait
//!
//! Pluggable persistence backend for outbox messages.
//! Implement this trait to store outbox messages in your database.
//!
//! # Example
//!
//! ```ignore
//! use fc_sdk::outbox::{OutboxDriver, OutboxMessage};
//!
//! struct MyPgDriver { pool: sqlx::PgPool }
//!
//! #[async_trait::async_trait]
//! impl OutboxDriver for MyPgDriver {
//!     async fn insert(&self, message: OutboxMessage) -> anyhow::Result<()> {
//!         sqlx::query("INSERT INTO outbox_messages ...")
//!             .bind(&message.id).execute(&self.pool).await?;
//!         Ok(())
//!     }
//!     async fn insert_batch(&self, messages: Vec<OutboxMessage>) -> anyhow::Result<()> {
//!         for m in messages { self.insert(m).await?; }
//!         Ok(())
//!     }
//! }
//! ```

use async_trait::async_trait;
use serde::{Deserialize, Serialize};
use std::collections::HashMap;

/// Message types supported by the outbox.
#[derive(Debug, Clone, Copy, PartialEq, Eq, Serialize, Deserialize)]
pub enum MessageType {
    #[serde(rename = "EVENT")]
    Event,
    #[serde(rename = "DISPATCH_JOB")]
    DispatchJob,
    #[serde(rename = "AUDIT_LOG")]
    AuditLog,
}

impl MessageType {
    pub fn as_str(&self) -> &'static str {
        match self {
            Self::Event => "EVENT",
            Self::DispatchJob => "DISPATCH_JOB",
            Self::AuditLog => "AUDIT_LOG",
        }
    }
}

/// Outbox status codes matching the outbox-processor.
///
/// Only `PENDING` (0) is written by the SDK; all others are managed by the processor.
pub struct OutboxStatus;

impl OutboxStatus {
    pub const PENDING: i32 = 0;
    pub const SUCCESS: i32 = 1;
    pub const BAD_REQUEST: i32 = 2;
    pub const INTERNAL_ERROR: i32 = 3;
    pub const UNAUTHORIZED: i32 = 4;
    pub const FORBIDDEN: i32 = 5;
    pub const GATEWAY_ERROR: i32 = 6;
    pub const IN_PROGRESS: i32 = 9;
}

/// An outbox message record to be persisted by the driver.
#[derive(Debug, Clone, Serialize, Deserialize)]
pub struct OutboxMessage {
    pub id: String,
    #[serde(rename = "type")]
    pub message_type: String,
    pub message_group: Option<String>,
    pub payload: String,
    pub status: i32,
    pub created_at: String,
    pub updated_at: String,
    pub client_id: String,
    pub payload_size: i32,
    pub headers: Option<HashMap<String, String>>,
}

/// Driver interface for outbox persistence.
///
/// Implement this with your database client to participate in the same
/// transaction as your business logic. The SDK provides a built-in
/// [`SqlxPgDriver`](super::SqlxPgDriver) for PostgreSQL via sqlx.
#[async_trait]
pub trait OutboxDriver: Send + Sync {
    /// Insert a single message into the outbox.
    async fn insert(&self, message: OutboxMessage) -> anyhow::Result<()>;

    /// Insert multiple messages into the outbox (batch).
    async fn insert_batch(&self, messages: Vec<OutboxMessage>) -> anyhow::Result<()>;
}
