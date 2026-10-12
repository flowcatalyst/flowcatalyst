//! Audit-log rows: the item `POST /api/audit-logs/batch` ingests.

use std::collections::BTreeMap;

use chrono::{DateTime, SecondsFormat, Utc};
use serde::Serialize;
use serde_json::Value;

use super::error::{InvalidField, OutboxError};
use super::redaction;

/// An audit entry to write to the outbox, built like the Java SDK's
/// `CreateAuditLogDto`.
///
/// The payload uses the Java SDK's keys and order. `operationData` is
/// redacted (passwords, secrets, tokens become `"***"`) and embedded as a JSON
/// **string**, as every SDK does. `principalId` is required: the platform
/// refuses an audit entry without an actor (`BAD_REQUEST`, terminal).
///
/// ```
/// use flowcatalyst_sdk::outbox::CreateAuditLogDto;
///
/// let audit = CreateAuditLogDto::new("Order", "ord_0HZXEQ5Y8JY5Z", "CreateOrder", "prn_0HZXEQ5Y8JY5A")?
///     .operation_data(&serde_json::json!({"total": 1500, "apiKey": "k"}))?
///     .application_code("orders")
///     .client_code("acme");
/// # Ok::<(), flowcatalyst_sdk::outbox::OutboxError>(())
/// ```
#[derive(Debug, Clone, PartialEq)]
pub struct CreateAuditLogDto {
    entity_type: String,
    entity_id: String,
    operation: String,
    operation_data: Option<String>,
    principal_id: String,
    performed_at: Option<DateTime<Utc>>,
    source: Option<String>,
    correlation_id: Option<String>,
    application_code: Option<String>,
    client_code: Option<String>,
    metadata: BTreeMap<String, String>,
    headers: BTreeMap<String, String>,
}

#[derive(Serialize)]
#[serde(rename_all = "camelCase")]
struct AuditPayload<'a> {
    entity_type: &'a str,
    entity_id: &'a str,
    operation: &'a str,
    #[serde(skip_serializing_if = "Option::is_none")]
    operation_data: Option<&'a str>,
    principal_id: &'a str,
    performed_at: String,
    #[serde(skip_serializing_if = "Option::is_none")]
    source: Option<&'a str>,
    #[serde(skip_serializing_if = "Option::is_none")]
    correlation_id: Option<&'a str>,
    #[serde(skip_serializing_if = "Option::is_none")]
    application_code: Option<&'a str>,
    #[serde(skip_serializing_if = "Option::is_none")]
    client_code: Option<&'a str>,
    #[serde(skip_serializing_if = "BTreeMap::is_empty")]
    metadata: &'a BTreeMap<String, String>,
}

impl CreateAuditLogDto {
    /// An entry for `operation` on `entity_type`/`entity_id`, performed by
    /// `principal_id` (not blank).
    pub fn new(
        entity_type: impl Into<String>,
        entity_id: impl Into<String>,
        operation: impl Into<String>,
        principal_id: impl Into<String>,
    ) -> Result<Self, OutboxError> {
        let principal_id = principal_id.into();
        if principal_id.trim().is_empty() {
            return Err(InvalidField::new(
                "principalId",
                "is required: the platform refuses an audit entry without an actor",
            )
            .into());
        }
        Ok(Self {
            entity_type: entity_type.into(),
            entity_id: entity_id.into(),
            operation: operation.into(),
            operation_data: None,
            principal_id,
            performed_at: None,
            source: None,
            correlation_id: None,
            application_code: None,
            client_code: None,
            metadata: BTreeMap::new(),
            headers: BTreeMap::new(),
        })
    }

    /// The command or change being audited. Secret-named keys are masked.
    pub fn operation_data(self, data: &impl Serialize) -> Result<Self, OutboxError> {
        self.operation_data_masked(data, &[])
    }

    /// As [`operation_data`](Self::operation_data), also masking these
    /// top-level fields whatever their names.
    pub fn operation_data_masked(
        mut self,
        data: &impl Serialize,
        masked: &[&str],
    ) -> Result<Self, OutboxError> {
        let value: Value = serde_json::to_value(data)?;
        let redacted = redaction::redact(&value, masked);
        self.operation_data = Some(serde_json::to_string(&redacted)?);
        Ok(self)
    }

    /// When it happened; default the time the payload is built.
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

    /// The FlowCatalyst application (by code). Required by the platform when
    /// the outbox is delivered with an application-scoped credential.
    pub fn application_code(mut self, code: impl Into<String>) -> Self {
        self.application_code = Some(code.into());
        self
    }

    /// The FlowCatalyst client (by identifier); resolved at ingest.
    pub fn client_code(mut self, code: impl Into<String>) -> Self {
        self.client_code = Some(code.into());
        self
    }

    pub fn metadata(mut self, key: impl Into<String>, value: impl Into<String>) -> Self {
        self.metadata.insert(key.into(), value.into());
        self
    }

    /// Set a row header (the `headers` column; the processor does not read it).
    pub fn header(mut self, key: impl Into<String>, value: impl Into<String>) -> Self {
        self.headers.insert(key.into(), value.into());
        self
    }

    pub(crate) fn headers(&self) -> &BTreeMap<String, String> {
        &self.headers
    }

    /// The payload JSON written to the outbox row. `performedAt` is RFC 3339
    /// UTC formatted as Java's `Instant.toString` (0, 3, 6 or 9 fractional
    /// digits, `Z`), so the bytes equal the Java SDK's.
    pub fn to_payload(&self) -> Result<String, OutboxError> {
        let performed_at = self
            .performed_at
            .unwrap_or_else(Utc::now)
            .to_rfc3339_opts(SecondsFormat::AutoSi, true);
        Ok(serde_json::to_string(&AuditPayload {
            entity_type: &self.entity_type,
            entity_id: &self.entity_id,
            operation: &self.operation,
            operation_data: self.operation_data.as_deref(),
            principal_id: &self.principal_id,
            performed_at,
            source: self.source.as_deref(),
            correlation_id: self.correlation_id.as_deref(),
            application_code: self.application_code.as_deref(),
            client_code: self.client_code.as_deref(),
            metadata: &self.metadata,
        })?)
    }
}

#[cfg(test)]
mod tests {
    use super::*;
    use chrono::TimeZone;
    use serde_json::json;

    #[test]
    fn principal_is_required() {
        assert!(CreateAuditLogDto::new("Order", "o1", "Create", " ").is_err());
    }

    #[test]
    fn operation_data_is_redacted_and_stringified() {
        let at = Utc.with_ymd_and_hms(2026, 8, 7, 9, 32, 12).unwrap();
        let a = CreateAuditLogDto::new("Order", "o1", "Create", "prn_1")
            .unwrap()
            .operation_data(&json!({"total": 1, "password": "hunter2"}))
            .unwrap()
            .performed_at(at);
        assert_eq!(
            a.to_payload().unwrap(),
            r#"{"entityType":"Order","entityId":"o1","operation":"Create","operationData":"{\"password\":\"***\",\"total\":1}","principalId":"prn_1","performedAt":"2026-08-07T09:32:12Z"}"#
        );
    }
}
