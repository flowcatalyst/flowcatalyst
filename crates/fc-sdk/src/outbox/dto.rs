//! Outbox Builder DTOs
//!
//! Lightweight builder types for creating outbox messages without
//! implementing the full `DomainEvent` trait. These match the TS and
//! Laravel SDK patterns.
//!
//! # Examples
//!
//! ```ignore
//! use fc_sdk::outbox::{CreateEventDto, CreateDispatchJobDto, CreateAuditLogDto};
//!
//! // Create an event
//! let event = CreateEventDto::new("user.registered", serde_json::json!({"userId": "123"}))
//!     .source("user-service")
//!     .subject("users.user.123")
//!     .correlation_id("corr-456")
//!     .message_group("users:user:123");
//!
//! // Create a dispatch job
//! let job = CreateDispatchJobDto::new(
//!     "user-service",
//!     "user.registered",
//!     "https://webhook.example.com/users",
//!     r#"{"userId":"123"}"#,
//!     "pool_0HZXEQ5Y8JY5Z",
//! ).correlation_id("corr-456")
//!  .timeout_seconds(60);
//!
//! // Create an audit log
//! let audit = CreateAuditLogDto::new("User", "usr_123", "CREATE")
//!     .operation_data(serde_json::json!({"email": "user@example.com"}))
//!     .principal_id("usr_456")
//!     .source("user-service");
//! ```

use chrono::{DateTime, Utc};
use serde::Serialize;
use std::collections::HashMap;

// ─── CreateEventDto ─────────────────────────────────────────────────────────

/// Builder for creating an event in the outbox.
#[derive(Debug, Clone)]
pub struct CreateEventDto {
    pub event_type: String,
    pub data: serde_json::Value,
    pub source: Option<String>,
    pub subject: Option<String>,
    pub correlation_id: Option<String>,
    pub causation_id: Option<String>,
    pub deduplication_id: Option<String>,
    pub message_group: Option<String>,
    pub context_data: Vec<ContextDataEntry>,
    pub headers: HashMap<String, String>,
}

/// Key-value context data entry attached to events.
#[derive(Debug, Clone, Serialize)]
pub struct ContextDataEntry {
    pub key: String,
    pub value: String,
}

impl CreateEventDto {
    pub fn new(event_type: impl Into<String>, data: serde_json::Value) -> Self {
        Self {
            event_type: event_type.into(),
            data,
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

    pub fn source(mut self, source: impl Into<String>) -> Self {
        self.source = Some(source.into());
        self
    }

    pub fn subject(mut self, subject: impl Into<String>) -> Self {
        self.subject = Some(subject.into());
        self
    }

    pub fn correlation_id(mut self, id: impl Into<String>) -> Self {
        self.correlation_id = Some(id.into());
        self
    }

    pub fn causation_id(mut self, id: impl Into<String>) -> Self {
        self.causation_id = Some(id.into());
        self
    }

    pub fn deduplication_id(mut self, id: impl Into<String>) -> Self {
        self.deduplication_id = Some(id.into());
        self
    }

    pub fn message_group(mut self, group: impl Into<String>) -> Self {
        self.message_group = Some(group.into());
        self
    }

    pub fn context_data(mut self, entries: Vec<ContextDataEntry>) -> Self {
        self.context_data.extend(entries);
        self
    }

    pub fn add_context(mut self, key: impl Into<String>, value: impl Into<String>) -> Self {
        self.context_data.push(ContextDataEntry {
            key: key.into(),
            value: value.into(),
        });
        self
    }

    pub fn headers(mut self, headers: HashMap<String, String>) -> Self {
        self.headers.extend(headers);
        self
    }

    /// Build the event payload JSON for the outbox.
    pub fn to_payload(&self) -> serde_json::Value {
        let mut payload = serde_json::json!({
            "specVersion": "1.0",
            "type": self.event_type,
            "data": serde_json::to_string(&self.data).unwrap_or_else(|_| "{}".to_string()),
        });

        let obj = payload.as_object_mut().unwrap();
        if let Some(ref v) = self.source {
            obj.insert("source".into(), serde_json::json!(v));
        }
        if let Some(ref v) = self.subject {
            obj.insert("subject".into(), serde_json::json!(v));
        }
        if let Some(ref v) = self.correlation_id {
            obj.insert("correlationId".into(), serde_json::json!(v));
        }
        if let Some(ref v) = self.causation_id {
            obj.insert("causationId".into(), serde_json::json!(v));
        }
        if let Some(ref v) = self.deduplication_id {
            obj.insert("deduplicationId".into(), serde_json::json!(v));
        }
        if let Some(ref v) = self.message_group {
            obj.insert("messageGroup".into(), serde_json::json!(v));
        }
        if !self.context_data.is_empty() {
            obj.insert("contextData".into(), serde_json::json!(self.context_data));
        }

        payload
    }
}

// ─── CreateDispatchJobDto ───────────────────────────────────────────────────

/// Builder for creating a dispatch job in the outbox.
#[derive(Debug, Clone)]
pub struct CreateDispatchJobDto {
    pub source: String,
    pub code: String,
    pub target_url: String,
    pub payload: String,
    pub dispatch_pool_id: String,
    pub subject: Option<String>,
    pub correlation_id: Option<String>,
    pub event_id: Option<String>,
    pub metadata: HashMap<String, String>,
    pub headers: HashMap<String, String>,
    pub payload_content_type: String,
    pub data_only: bool,
    pub message_group: Option<String>,
    pub sequence: Option<i64>,
    pub timeout_seconds: u32,
    pub max_retries: u32,
    pub retry_strategy: Option<String>,
    pub scheduled_for: Option<DateTime<Utc>>,
    pub expires_at: Option<DateTime<Utc>>,
    pub idempotency_key: Option<String>,
    pub external_id: Option<String>,
    pub connection_id: Option<String>,
}

impl CreateDispatchJobDto {
    pub fn new(
        source: impl Into<String>,
        code: impl Into<String>,
        target_url: impl Into<String>,
        payload: impl Into<String>,
        dispatch_pool_id: impl Into<String>,
    ) -> Self {
        Self {
            source: source.into(),
            code: code.into(),
            target_url: target_url.into(),
            payload: payload.into(),
            dispatch_pool_id: dispatch_pool_id.into(),
            subject: None,
            correlation_id: None,
            event_id: None,
            metadata: HashMap::new(),
            headers: HashMap::new(),
            payload_content_type: "application/json".to_string(),
            data_only: true,
            message_group: None,
            sequence: None,
            timeout_seconds: 30,
            max_retries: 5,
            retry_strategy: None,
            scheduled_for: None,
            expires_at: None,
            idempotency_key: None,
            external_id: None,
            connection_id: None,
        }
    }

    /// Create from a JSON-serializable payload (auto-stringifies).
    pub fn from_json(
        source: impl Into<String>,
        code: impl Into<String>,
        target_url: impl Into<String>,
        payload: &impl Serialize,
        dispatch_pool_id: impl Into<String>,
    ) -> Self {
        Self::new(
            source,
            code,
            target_url,
            serde_json::to_string(payload).unwrap_or_else(|_| "{}".to_string()),
            dispatch_pool_id,
        )
    }

    pub fn subject(mut self, subject: impl Into<String>) -> Self {
        self.subject = Some(subject.into());
        self
    }

    pub fn correlation_id(mut self, id: impl Into<String>) -> Self {
        self.correlation_id = Some(id.into());
        self
    }

    pub fn event_id(mut self, id: impl Into<String>) -> Self {
        self.event_id = Some(id.into());
        self
    }

    pub fn metadata(mut self, metadata: HashMap<String, String>) -> Self {
        self.metadata.extend(metadata);
        self
    }

    pub fn add_metadata(mut self, key: impl Into<String>, value: impl Into<String>) -> Self {
        self.metadata.insert(key.into(), value.into());
        self
    }

    pub fn headers(mut self, headers: HashMap<String, String>) -> Self {
        self.headers.extend(headers);
        self
    }

    pub fn payload_content_type(mut self, ct: impl Into<String>) -> Self {
        self.payload_content_type = ct.into();
        self
    }

    pub fn data_only(mut self, data_only: bool) -> Self {
        self.data_only = data_only;
        self
    }

    pub fn message_group(mut self, group: impl Into<String>) -> Self {
        self.message_group = Some(group.into());
        self
    }

    pub fn sequence(mut self, seq: i64) -> Self {
        self.sequence = Some(seq);
        self
    }

    pub fn timeout_seconds(mut self, secs: u32) -> Self {
        self.timeout_seconds = secs;
        self
    }

    pub fn max_retries(mut self, retries: u32) -> Self {
        self.max_retries = retries;
        self
    }

    pub fn retry_strategy(mut self, strategy: impl Into<String>) -> Self {
        self.retry_strategy = Some(strategy.into());
        self
    }

    pub fn scheduled_for(mut self, at: DateTime<Utc>) -> Self {
        self.scheduled_for = Some(at);
        self
    }

    pub fn expires_at(mut self, at: DateTime<Utc>) -> Self {
        self.expires_at = Some(at);
        self
    }

    pub fn idempotency_key(mut self, key: impl Into<String>) -> Self {
        self.idempotency_key = Some(key.into());
        self
    }

    pub fn external_id(mut self, id: impl Into<String>) -> Self {
        self.external_id = Some(id.into());
        self
    }

    pub fn connection_id(mut self, id: impl Into<String>) -> Self {
        self.connection_id = Some(id.into());
        self
    }

    /// Build the dispatch job payload JSON for the outbox.
    pub fn to_payload(&self) -> serde_json::Value {
        let mut payload = serde_json::json!({
            "source": self.source,
            "code": self.code,
            "targetUrl": self.target_url,
            "payload": self.payload,
            "payloadContentType": self.payload_content_type,
            "dispatchPoolId": self.dispatch_pool_id,
            "dataOnly": self.data_only,
            "timeoutSeconds": self.timeout_seconds,
            "maxRetries": self.max_retries,
        });

        let obj = payload.as_object_mut().unwrap();
        if let Some(ref v) = self.subject {
            obj.insert("subject".into(), serde_json::json!(v));
        }
        if let Some(ref v) = self.correlation_id {
            obj.insert("correlationId".into(), serde_json::json!(v));
        }
        if let Some(ref v) = self.event_id {
            obj.insert("eventId".into(), serde_json::json!(v));
        }
        if !self.metadata.is_empty() {
            obj.insert("metadata".into(), serde_json::json!(self.metadata));
        }
        if !self.headers.is_empty() {
            obj.insert("headers".into(), serde_json::json!(self.headers));
        }
        if let Some(ref v) = self.message_group {
            obj.insert("messageGroup".into(), serde_json::json!(v));
        }
        if let Some(v) = self.sequence {
            obj.insert("sequence".into(), serde_json::json!(v));
        }
        if let Some(ref v) = self.retry_strategy {
            obj.insert("retryStrategy".into(), serde_json::json!(v));
        }
        if let Some(ref v) = self.scheduled_for {
            obj.insert("scheduledFor".into(), serde_json::json!(v.to_rfc3339()));
        }
        if let Some(ref v) = self.expires_at {
            obj.insert("expiresAt".into(), serde_json::json!(v.to_rfc3339()));
        }
        if let Some(ref v) = self.idempotency_key {
            obj.insert("idempotencyKey".into(), serde_json::json!(v));
        }
        if let Some(ref v) = self.external_id {
            obj.insert("externalId".into(), serde_json::json!(v));
        }
        if let Some(ref v) = self.connection_id {
            obj.insert("connectionId".into(), serde_json::json!(v));
        }

        payload
    }
}

// ─── CreateAuditLogDto ──────────────────────────────────────────────────────

/// Builder for creating an audit log entry in the outbox.
#[derive(Debug, Clone)]
pub struct CreateAuditLogDto {
    pub entity_type: String,
    pub entity_id: String,
    pub operation: String,
    pub operation_data: Option<serde_json::Value>,
    pub principal_id: Option<String>,
    pub performed_at: Option<DateTime<Utc>>,
    pub source: Option<String>,
    pub correlation_id: Option<String>,
    pub metadata: HashMap<String, String>,
    pub headers: HashMap<String, String>,
}

impl CreateAuditLogDto {
    pub fn new(
        entity_type: impl Into<String>,
        entity_id: impl Into<String>,
        operation: impl Into<String>,
    ) -> Self {
        Self {
            entity_type: entity_type.into(),
            entity_id: entity_id.into(),
            operation: operation.into(),
            operation_data: None,
            principal_id: None,
            performed_at: None,
            source: None,
            correlation_id: None,
            metadata: HashMap::new(),
            headers: HashMap::new(),
        }
    }

    pub fn operation_data(mut self, data: serde_json::Value) -> Self {
        self.operation_data = Some(data);
        self
    }

    pub fn principal_id(mut self, id: impl Into<String>) -> Self {
        self.principal_id = Some(id.into());
        self
    }

    pub fn performed_at(mut self, at: DateTime<Utc>) -> Self {
        self.performed_at = Some(at);
        self
    }

    pub fn source(mut self, source: impl Into<String>) -> Self {
        self.source = Some(source.into());
        self
    }

    pub fn correlation_id(mut self, id: impl Into<String>) -> Self {
        self.correlation_id = Some(id.into());
        self
    }

    pub fn metadata(mut self, metadata: HashMap<String, String>) -> Self {
        self.metadata.extend(metadata);
        self
    }

    pub fn headers(mut self, headers: HashMap<String, String>) -> Self {
        self.headers.extend(headers);
        self
    }

    /// Build the audit log payload JSON for the outbox.
    pub fn to_payload(&self) -> serde_json::Value {
        let performed = self
            .performed_at
            .unwrap_or_else(Utc::now)
            .to_rfc3339();

        let mut payload = serde_json::json!({
            "entityType": self.entity_type,
            "entityId": self.entity_id,
            "operation": self.operation,
            "performedAt": performed,
        });

        let obj = payload.as_object_mut().unwrap();
        if let Some(ref v) = self.operation_data {
            obj.insert(
                "operationData".into(),
                serde_json::json!(serde_json::to_string(v).unwrap_or_else(|_| "{}".to_string())),
            );
        }
        if let Some(ref v) = self.principal_id {
            obj.insert("principalId".into(), serde_json::json!(v));
        }
        if let Some(ref v) = self.source {
            obj.insert("source".into(), serde_json::json!(v));
        }
        if let Some(ref v) = self.correlation_id {
            obj.insert("correlationId".into(), serde_json::json!(v));
        }
        if !self.metadata.is_empty() {
            obj.insert("metadata".into(), serde_json::json!(self.metadata));
        }

        payload
    }
}
