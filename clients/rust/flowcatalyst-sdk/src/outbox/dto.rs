//! Outbox DTOs

use chrono::{DateTime, Utc};
use serde::{Deserialize, Serialize};
use std::collections::HashMap;

/// Message type in the outbox
#[derive(Debug, Clone, Copy, PartialEq, Eq, Serialize, Deserialize)]
#[serde(rename_all = "SCREAMING_SNAKE_CASE")]
pub enum MessageType {
    /// Domain event for subscription matching
    Event,
    /// Direct webhook dispatch without subscription matching
    DispatchJob,
}

/// Context data entry
#[derive(Debug, Clone, Serialize, Deserialize)]
pub struct ContextData {
    pub key: String,
    pub value: String,
}

/// DTO for creating an event in the outbox
#[derive(Debug, Clone)]
pub struct CreateEventDto {
    /// Event type code (e.g., "app:domain:aggregate:event")
    pub event_type: String,
    /// Event payload data
    pub data: serde_json::Value,
    /// Partition ID for ordering
    pub partition_id: String,
    /// Event source identifier
    pub source: Option<String>,
    /// Event subject
    pub subject: Option<String>,
    /// Correlation ID for tracing
    pub correlation_id: Option<String>,
    /// Causation ID for event chains
    pub causation_id: Option<String>,
    /// Idempotency key
    pub deduplication_id: Option<String>,
    /// Message ordering group
    pub message_group: Option<String>,
    /// Additional context data
    pub context_data: Vec<ContextData>,
    /// Custom headers
    pub headers: HashMap<String, String>,
}

impl CreateEventDto {
    /// Create a new event DTO
    pub fn new(
        event_type: impl Into<String>,
        data: serde_json::Value,
        partition_id: impl Into<String>,
    ) -> Self {
        Self {
            event_type: event_type.into(),
            data,
            partition_id: partition_id.into(),
            source: None,
            subject: None,
            correlation_id: None,
            causation_id: None,
            deduplication_id: None,
            message_group: None,
            context_data: Vec::new(),
            headers: HashMap::new(),
        }
    }

    /// Set the event source
    pub fn with_source(mut self, source: impl Into<String>) -> Self {
        self.source = Some(source.into());
        self
    }

    /// Set the event subject
    pub fn with_subject(mut self, subject: impl Into<String>) -> Self {
        self.subject = Some(subject.into());
        self
    }

    /// Set the correlation ID
    pub fn with_correlation_id(mut self, id: impl Into<String>) -> Self {
        self.correlation_id = Some(id.into());
        self
    }

    /// Set the causation ID
    pub fn with_causation_id(mut self, id: impl Into<String>) -> Self {
        self.causation_id = Some(id.into());
        self
    }

    /// Set the deduplication ID
    pub fn with_deduplication_id(mut self, id: impl Into<String>) -> Self {
        self.deduplication_id = Some(id.into());
        self
    }

    /// Set the message group
    pub fn with_message_group(mut self, group: impl Into<String>) -> Self {
        self.message_group = Some(group.into());
        self
    }

    /// Add context data
    pub fn with_context(mut self, key: impl Into<String>, value: impl Into<String>) -> Self {
        self.context_data.push(ContextData {
            key: key.into(),
            value: value.into(),
        });
        self
    }

    /// Add a header
    pub fn with_header(mut self, key: impl Into<String>, value: impl Into<String>) -> Self {
        self.headers.insert(key.into(), value.into());
        self
    }

    /// Convert to outbox payload
    pub fn to_payload(&self) -> serde_json::Value {
        let mut payload = serde_json::json!({
            "specVersion": "1.0",
            "type": self.event_type,
            "data": self.data.to_string(),
        });

        if let Some(ref source) = self.source {
            payload["source"] = serde_json::json!(source);
        }
        if let Some(ref subject) = self.subject {
            payload["subject"] = serde_json::json!(subject);
        }
        if let Some(ref id) = self.correlation_id {
            payload["correlationId"] = serde_json::json!(id);
        }
        if let Some(ref id) = self.causation_id {
            payload["causationId"] = serde_json::json!(id);
        }
        if let Some(ref id) = self.deduplication_id {
            payload["deduplicationId"] = serde_json::json!(id);
        }
        if let Some(ref group) = self.message_group {
            payload["messageGroup"] = serde_json::json!(group);
        }
        if !self.context_data.is_empty() {
            payload["contextData"] = serde_json::json!(self.context_data);
        }

        payload
    }
}

/// DTO for creating a dispatch job in the outbox
#[derive(Debug, Clone)]
pub struct CreateDispatchJobDto {
    /// Job source identifier
    pub source: String,
    /// Unique job code
    pub code: String,
    /// Webhook target URL
    pub target_url: String,
    /// Request payload
    pub payload: serde_json::Value,
    /// Dispatch pool ID
    pub dispatch_pool_id: String,
    /// Partition ID for ordering
    pub partition_id: String,
    /// Correlation ID
    pub correlation_id: Option<String>,
    /// Custom HTTP headers
    pub headers: HashMap<String, String>,
    /// Dispatch mode
    pub mode: Option<String>,
    /// Maximum retry attempts
    pub max_retries: Option<u32>,
    /// Request timeout in seconds
    pub timeout_seconds: Option<u32>,
}

impl CreateDispatchJobDto {
    /// Create a new dispatch job DTO
    pub fn new(
        source: impl Into<String>,
        code: impl Into<String>,
        target_url: impl Into<String>,
        payload: serde_json::Value,
        dispatch_pool_id: impl Into<String>,
        partition_id: impl Into<String>,
    ) -> Self {
        Self {
            source: source.into(),
            code: code.into(),
            target_url: target_url.into(),
            payload,
            dispatch_pool_id: dispatch_pool_id.into(),
            partition_id: partition_id.into(),
            correlation_id: None,
            headers: HashMap::new(),
            mode: None,
            max_retries: None,
            timeout_seconds: None,
        }
    }

    /// Set the correlation ID
    pub fn with_correlation_id(mut self, id: impl Into<String>) -> Self {
        self.correlation_id = Some(id.into());
        self
    }

    /// Add a header
    pub fn with_header(mut self, key: impl Into<String>, value: impl Into<String>) -> Self {
        self.headers.insert(key.into(), value.into());
        self
    }

    /// Set the dispatch mode
    pub fn with_mode(mut self, mode: impl Into<String>) -> Self {
        self.mode = Some(mode.into());
        self
    }

    /// Set max retries
    pub fn with_max_retries(mut self, retries: u32) -> Self {
        self.max_retries = Some(retries);
        self
    }

    /// Set timeout
    pub fn with_timeout(mut self, seconds: u32) -> Self {
        self.timeout_seconds = Some(seconds);
        self
    }

    /// Convert to outbox payload
    pub fn to_payload(&self) -> serde_json::Value {
        let mut payload = serde_json::json!({
            "source": self.source,
            "code": self.code,
            "targetUrl": self.target_url,
            "payload": self.payload,
            "dispatchPoolId": self.dispatch_pool_id,
        });

        if let Some(ref id) = self.correlation_id {
            payload["correlationId"] = serde_json::json!(id);
        }
        if let Some(ref mode) = self.mode {
            payload["mode"] = serde_json::json!(mode);
        }
        if let Some(retries) = self.max_retries {
            payload["maxRetries"] = serde_json::json!(retries);
        }
        if let Some(timeout) = self.timeout_seconds {
            payload["timeoutSeconds"] = serde_json::json!(timeout);
        }

        payload
    }
}

/// Outbox message stored in the database
#[derive(Debug, Clone, Serialize, Deserialize)]
pub struct OutboxMessage {
    /// Message ID (TSID)
    pub id: String,
    /// Tenant ID
    pub tenant_id: String,
    /// Partition ID for ordering
    pub partition_id: String,
    /// Message type
    pub message_type: MessageType,
    /// Message payload
    pub payload: serde_json::Value,
    /// Custom headers
    #[serde(default)]
    pub headers: HashMap<String, String>,
    /// Creation timestamp
    pub created_at: DateTime<Utc>,
}
