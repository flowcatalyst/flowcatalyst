//! One `outbox_messages` row, ready to insert.

use std::collections::BTreeMap;
use std::fmt;

use chrono::{DateTime, Utc};

use super::status::{OutboxItemType, OutboxStatus};

/// The id of an outbox row (an event's or audit entry's generated TSID, or a
/// dispatch job's id).
#[derive(Debug, Clone, PartialEq, Eq, PartialOrd, Ord, Hash)]
pub struct MessageId(String);

impl MessageId {
    pub(crate) fn new(id: String) -> Self {
        Self(id)
    }

    pub fn as_str(&self) -> &str {
        &self.0
    }

    pub fn into_string(self) -> String {
        self.0
    }
}

impl fmt::Display for MessageId {
    fn fmt(&self, f: &mut fmt::Formatter<'_>) -> fmt::Result {
        f.write_str(&self.0)
    }
}

impl AsRef<str> for MessageId {
    fn as_ref(&self) -> &str {
        &self.0
    }
}

/// A row of the canonical `outbox_messages` table (see [`super::schema`]).
///
/// Only the producer's columns are here: `retry_count` (default 0) and
/// `error_message` belong to the outbox processor.
#[derive(Debug, Clone, PartialEq, Eq)]
pub struct OutboxMessage {
    /// `id VARCHAR(26)`.
    pub id: MessageId,
    /// `type VARCHAR(20)`: `EVENT`, `DISPATCH_JOB` or `AUDIT_LOG`.
    pub message_type: OutboxItemType,
    /// `message_group VARCHAR(255)`, per-group FIFO for the processor.
    pub message_group: Option<String>,
    /// `payload TEXT`: the exact JSON the processor forwards, unchanged.
    pub payload: String,
    /// `status SMALLINT`: always [`OutboxStatus::Pending`] when written.
    pub status: OutboxStatus,
    pub created_at: DateTime<Utc>,
    pub updated_at: DateTime<Utc>,
    /// `client_id VARCHAR(26)`: the tenant that owns the row.
    pub client_id: String,
    /// `payload_size INTEGER`: the payload's UTF-8 byte length.
    pub payload_size: i32,
    /// `headers JSONB`: `None` when there are none.
    pub headers: Option<BTreeMap<String, String>>,
}

impl OutboxMessage {
    pub(crate) fn pending(
        id: MessageId,
        message_type: OutboxItemType,
        message_group: Option<String>,
        payload: String,
        client_id: String,
        headers: &BTreeMap<String, String>,
    ) -> Self {
        let now = Utc::now();
        let payload_size = i32::try_from(payload.len()).unwrap_or(i32::MAX);
        Self {
            id,
            message_type,
            message_group,
            payload,
            status: OutboxStatus::Pending,
            created_at: now,
            updated_at: now,
            client_id,
            payload_size,
            headers: (!headers.is_empty()).then(|| headers.clone()),
        }
    }

    /// The headers as the JSON written to the `headers` column.
    pub fn headers_json(&self) -> Result<Option<String>, serde_json::Error> {
        self.headers.as_ref().map(serde_json::to_string).transpose()
    }

    /// `status` as the `SMALLINT` stored.
    pub fn status_code(&self) -> i16 {
        // Every OutboxStatus code is 0..=9.
        self.status.code() as i16
    }
}
