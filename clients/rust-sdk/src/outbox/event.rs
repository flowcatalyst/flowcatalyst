//! Event rows: the item `POST /api/events/batch` ingests.

use std::collections::BTreeMap;

use serde::Serialize;

use super::error::{InvalidField, OutboxError};

/// A `contextData` entry: a key/value tag stored with the event.
#[derive(Debug, Clone, PartialEq, Eq, Serialize)]
pub struct ContextEntry {
    pub key: String,
    pub value: String,
}

/// An event to write to the outbox, built like the Java SDK's `CreateEventDto`.
///
/// The payload is the Java SDK's byte for byte (key order, `data` embedded as
/// a JSON **string**, absent fields omitted, never `null`):
///
/// ```json
/// {"specVersion":"1.0","type":"orders:sales:order:created","source":"orders",
///  "subject":"orders.order.123","data":"{\"orderId\":\"123\"}",
///  "correlationId":"…","causationId":"…","deduplicationId":"…",
///  "messageGroup":"orders:order:123","clientCode":"acme",
///  "contextData":[{"key":"principalId","value":"prn_…"}]}
/// ```
///
/// ```
/// use flowcatalyst_sdk::outbox::CreateEventDto;
///
/// let event = CreateEventDto::new("orders:sales:order:created", &serde_json::json!({"orderId": "123"}))?
///     .source("orders")
///     .subject("orders.order.123")
///     .message_group("orders:order:123")
///     .client_code("acme");
/// # Ok::<(), flowcatalyst_sdk::outbox::OutboxError>(())
/// ```
#[derive(Debug, Clone, PartialEq, Eq)]
pub struct CreateEventDto {
    event_type: String,
    data: String,
    source: Option<String>,
    subject: Option<String>,
    correlation_id: Option<String>,
    causation_id: Option<String>,
    deduplication_id: Option<String>,
    message_group: Option<String>,
    client_code: Option<String>,
    context_data: Vec<ContextEntry>,
    headers: BTreeMap<String, String>,
}

/// The serialised payload (field order is the wire order).
#[derive(Serialize)]
#[serde(rename_all = "camelCase")]
struct EventPayload<'a> {
    spec_version: &'static str,
    #[serde(rename = "type")]
    event_type: &'a str,
    #[serde(skip_serializing_if = "Option::is_none")]
    source: Option<&'a str>,
    #[serde(skip_serializing_if = "Option::is_none")]
    subject: Option<&'a str>,
    data: &'a str,
    #[serde(skip_serializing_if = "Option::is_none")]
    correlation_id: Option<&'a str>,
    #[serde(skip_serializing_if = "Option::is_none")]
    causation_id: Option<&'a str>,
    #[serde(skip_serializing_if = "Option::is_none")]
    deduplication_id: Option<&'a str>,
    #[serde(skip_serializing_if = "Option::is_none")]
    message_group: Option<&'a str>,
    #[serde(skip_serializing_if = "Option::is_none")]
    client_code: Option<&'a str>,
    #[serde(skip_serializing_if = "<[ContextEntry]>::is_empty")]
    context_data: &'a [ContextEntry],
}

impl CreateEventDto {
    /// An event of `event_type` (`application:subdomain:aggregate:action`, four
    /// non-empty segments, as every SDK requires) carrying `data`, which must
    /// serialise to a JSON object.
    pub fn new(event_type: impl Into<String>, data: &impl Serialize) -> Result<Self, OutboxError> {
        let event_type = event_type.into();
        require_qualified_code(&event_type, "type")?;
        let value = serde_json::to_value(data)?;
        if !value.is_object() {
            return Err(InvalidField::new("data", "must serialise to a JSON object").into());
        }
        Ok(Self {
            event_type,
            data: serde_json::to_string(data)?,
            source: None,
            subject: None,
            correlation_id: None,
            causation_id: None,
            deduplication_id: None,
            message_group: None,
            client_code: None,
            context_data: Vec::new(),
            headers: BTreeMap::new(),
        })
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

    /// The platform drops a second event with the same deduplication id.
    /// Without one, [`OutboxManager`](super::OutboxManager) writes the outbox
    /// row's id, so a batch the outbox processor sends again after a lost
    /// response does not create a second event; a value set here wins.
    pub fn deduplication_id(mut self, id: impl Into<String>) -> Self {
        self.deduplication_id = Some(id.into());
        self
    }

    /// Per-group FIFO key; also the outbox row's `message_group`.
    pub fn message_group(mut self, group: impl Into<String>) -> Self {
        self.message_group = Some(group.into());
        self
    }

    /// The FlowCatalyst client (by identifier) this event belongs to; the
    /// platform resolves it to a client id at ingest.
    pub fn client_code(mut self, code: impl Into<String>) -> Self {
        self.client_code = Some(code.into());
        self
    }

    /// Append a `contextData` entry (e.g. `principalId`, `executionId`).
    pub fn context(mut self, key: impl Into<String>, value: impl Into<String>) -> Self {
        self.context_data.push(ContextEntry {
            key: key.into(),
            value: value.into(),
        });
        self
    }

    /// Set a row header (the `headers` column; the processor does not read it).
    pub fn header(mut self, key: impl Into<String>, value: impl Into<String>) -> Self {
        self.headers.insert(key.into(), value.into());
        self
    }

    /// The event type.
    pub fn event_type(&self) -> &str {
        &self.event_type
    }

    /// The message group, when set.
    pub fn group(&self) -> Option<&str> {
        self.message_group.as_deref()
    }

    pub(crate) fn headers(&self) -> &BTreeMap<String, String> {
        &self.headers
    }

    /// The payload JSON as built, without a default `deduplicationId`: it is
    /// present only when [`deduplication_id`](Self::deduplication_id) was set.
    /// The outbox manager writes [`to_payload_for_row`](Self::to_payload_for_row).
    pub fn to_payload(&self) -> Result<String, OutboxError> {
        self.payload(None)
    }

    /// The payload JSON written to the outbox row `row_id`: the
    /// `deduplicationId` is the one set on the event, else `row_id` (which is
    /// stable across a resend of the row).
    pub fn to_payload_for_row(&self, row_id: &str) -> Result<String, OutboxError> {
        self.payload(Some(row_id))
    }

    fn payload(&self, row_id: Option<&str>) -> Result<String, OutboxError> {
        let dedup = self
            .deduplication_id
            .as_deref()
            .filter(|d| !d.trim().is_empty())
            .or(row_id);
        Ok(serde_json::to_string(&EventPayload {
            spec_version: "1.0",
            event_type: &self.event_type,
            source: self.source.as_deref(),
            subject: self.subject.as_deref(),
            data: &self.data,
            correlation_id: self.correlation_id.as_deref(),
            causation_id: self.causation_id.as_deref(),
            deduplication_id: dedup,
            message_group: self.message_group.as_deref(),
            client_code: self.client_code.as_deref(),
            context_data: &self.context_data,
        })?)
    }
}

/// Four non-empty (after trimming) colon-separated segments (Java
/// `QualifiedCode.assertQualified`).
pub(crate) fn require_qualified_code(value: &str, field: &'static str) -> Result<(), InvalidField> {
    let segments: Vec<&str> = value.split(':').collect();
    if segments.len() == 4 && segments.iter().all(|s| !s.trim().is_empty()) {
        Ok(())
    } else {
        Err(InvalidField::new(
            field,
            format!(
                "{value:?} must be a fully qualified code with four non-empty colon-separated \
                 segments: application:subdomain:aggregate:action"
            ),
        ))
    }
}

#[cfg(test)]
mod tests {
    use super::*;
    use serde_json::json;

    #[test]
    fn minimal_payload_has_only_the_always_fields() {
        let e =
            CreateEventDto::new("orders:sales:order:created", &json!({"orderId": "123"})).unwrap();
        assert_eq!(
            e.to_payload().unwrap(),
            r#"{"specVersion":"1.0","type":"orders:sales:order:created","data":"{\"orderId\":\"123\"}"}"#
        );
    }

    #[test]
    fn bare_or_partial_codes_are_refused() {
        for code in [
            "created",
            "orders:created",
            "orders::order:created",
            "a:b:c:d:e",
            "a:b:c: ",
        ] {
            let err = CreateEventDto::new(code, &json!({})).unwrap_err();
            assert!(
                matches!(err, OutboxError::Invalid(ref f) if f.field == "type"),
                "{code}"
            );
        }
    }

    #[test]
    fn data_must_be_an_object() {
        for data in [json!(null), json!([1]), json!("x"), json!(3)] {
            assert!(CreateEventDto::new("a:b:c:d", &data).is_err(), "{data}");
        }
    }

    #[test]
    fn context_entries_keep_insertion_order() {
        let e = CreateEventDto::new("a:b:c:d", &json!({}))
            .unwrap()
            .context("principalId", "prn_1")
            .context("executionId", "exe_1");
        let v: serde_json::Value = serde_json::from_str(&e.to_payload().unwrap()).unwrap();
        assert_eq!(
            v["contextData"],
            json!([{"key":"principalId","value":"prn_1"},{"key":"executionId","value":"exe_1"}])
        );
    }

    #[test]
    fn the_row_id_is_the_default_deduplication_id_and_an_explicit_one_wins() {
        let base = CreateEventDto::new("a:b:c:d", &json!({})).unwrap();
        let dedup = |e: &CreateEventDto, row: &str| -> Option<String> {
            let v: serde_json::Value =
                serde_json::from_str(&e.to_payload_for_row(row).unwrap()).unwrap();
            v.get("deduplicationId")
                .map(|d| d.as_str().unwrap().to_owned())
        };
        assert_eq!(dedup(&base, "row_1").as_deref(), Some("row_1"));
        // The same row written again (a resend) carries the same id.
        assert_eq!(dedup(&base, "row_1"), dedup(&base, "row_1"));
        assert_eq!(
            dedup(&base.clone().deduplication_id("mine"), "row_1").as_deref(),
            Some("mine")
        );
        assert_eq!(
            dedup(&base.clone().deduplication_id("  "), "row_1").as_deref(),
            Some("row_1")
        );
        // `to_payload` alone has no row to take it from.
        assert!(!base.to_payload().unwrap().contains("deduplicationId"));
    }
}
