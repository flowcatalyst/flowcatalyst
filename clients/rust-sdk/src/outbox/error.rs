//! Outbox errors.

use std::error::Error as StdError;

/// A value an outbox builder refused, so a row the platform would reject
/// terminally (a `400` that blocks the row's message group) is never written.
#[derive(Debug, Clone, PartialEq, Eq, thiserror::Error)]
#[error("{field}: {reason}")]
pub struct InvalidField {
    /// The builder field (wire name), e.g. `targetUrl`.
    pub field: &'static str,
    /// What is wrong with it.
    pub reason: String,
}

impl InvalidField {
    pub(crate) fn new(field: &'static str, reason: impl Into<String>) -> Self {
        Self {
            field,
            reason: reason.into(),
        }
    }
}

/// Errors from building or writing outbox rows.
#[derive(Debug, thiserror::Error)]
#[non_exhaustive]
pub enum OutboxError {
    /// The manager was built without a client id; every row carries one.
    #[error("outbox: a client id is required (the client_id column of every row)")]
    MissingClientId,

    /// A builder value the platform would refuse.
    #[error("outbox: invalid {0}")]
    Invalid(#[from] InvalidField),

    /// A payload or the headers could not be serialised.
    #[error("outbox: payload serialisation failed: {0}")]
    Serialize(#[from] serde_json::Error),

    /// The outbox table name is not a plain (optionally schema-qualified) SQL
    /// identifier.
    #[error("outbox: invalid table name {0:?}")]
    InvalidTableName(String),

    /// The database refused the insert.
    #[cfg(any(feature = "outbox-postgres", feature = "outbox-mysql"))]
    #[error("outbox: database error: {0}")]
    Database(#[from] sqlx::Error),

    /// A custom driver's failure.
    #[error("outbox: driver error: {0}")]
    Driver(#[source] Box<dyn StdError + Send + Sync>),
}
