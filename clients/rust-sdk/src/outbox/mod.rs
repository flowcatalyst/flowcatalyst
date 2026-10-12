//! The transactional outbox, the producer side.
//!
//! An application writes events, dispatch jobs and audit entries as rows of
//! its own `outbox_messages` table, in the same transaction as the business
//! change. The platform's outbox processor (`fcdev outbox`, or inside
//! `fc-server`) reads the table with the application's credentials and posts
//! each row's payload, unchanged, to:
//!
//! | `type` | endpoint | payload |
//! |---|---|---|
//! | `EVENT` | `/api/events/batch` | [`CreateEventDto`] |
//! | `DISPATCH_JOB` | `/api/dispatch-jobs/batch` | [`DispatchJob`] |
//! | `AUDIT_LOG` | `/api/audit-logs/batch` | [`CreateAuditLogDto`] |
//!
//! Rows, payloads and the table are those of the Java SDK (the framework's
//! reference), except dispatch jobs, which follow the platform's batch item
//! (see [`dispatch_job`]). [`OutboxManager`] turns the builders into
//! [`OutboxMessage`] rows; a driver writes them:
//! [`PgOutboxDriver`] (feature `outbox-postgres`) and [`MySqlOutboxDriver`]
//! (feature `outbox-mysql`), or your own [`OutboxDriver`].

pub mod audit_log;
pub mod dispatch_job;
pub mod error;
pub mod event;
pub mod manager;
pub mod message;
#[cfg(feature = "outbox-mysql")]
pub mod mysql;
#[cfg(feature = "outbox-postgres")]
pub mod postgres;
pub mod redaction;
pub mod schema;
pub mod status;

pub use audit_log::CreateAuditLogDto;
pub use dispatch_job::{
    DispatchJob, DispatchJobBuilder, DispatchKind, DispatchMode, DispatchQueue,
};
pub use error::{InvalidField, OutboxError};
pub use event::{ContextEntry, CreateEventDto};
pub use manager::{OutboxDriver, OutboxManager};
pub use message::{MessageId, OutboxMessage};
#[cfg(feature = "outbox-mysql")]
pub use mysql::MySqlOutboxDriver;
#[cfg(feature = "outbox-postgres")]
pub use postgres::PgOutboxDriver;
pub use status::{OutboxItemType, OutboxStatus};
