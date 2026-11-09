//! Push rows: a signal to the clients of an application, delivered through
//! the platform's push gateway (`POST /api/push/batch`).
//!
//! The platform's outbox processor sends each `PUSH` row's payload, unchanged,
//! to the gateway (docs/push-gateway-spec.md section 6.1). Pushes are signals:
//! the clients read the change again. Durable work is events and dispatch jobs.
//!
//! The rules and the refusal codes are the push contract's section 1 and the
//! Go SDK's `pkg/fcsdk/outbox/push.go`; the shared vector `outbox-push.json`
//! pins them for every SDK.

use std::fmt;

use serde::Serialize;

use super::error::OutboxError;

/// The Postgres notification channel a `PUSH` row's writer notifies in the
/// same statement as the insert, so a processor that listens on it delivers
/// the push at once instead of at its next poll.
pub const PUSH_NOTIFY_CHANNEL: &str = "fc_outbox_push";

/// The client code that means "the platform": left out of the payload.
pub const PLATFORM_CLIENT_CODE: &str = "_platform";

/// Longest topic, in characters.
pub const MAX_PUSH_TOPIC_LENGTH: usize = 255;
/// Longest type, in characters.
pub const MAX_PUSH_TYPE_LENGTH: usize = 100;
/// Default data limit in bytes (`push.max-data-bytes`).
pub const DEFAULT_PUSH_MAX_DATA_BYTES: usize = 16384;
/// The most the data limit may be set to.
pub const MAX_PUSH_DATA_BYTES: usize = 65536;

/// The code of a refused push (the codes of the framework's
/// `conformance/push/topics.json`).
#[derive(Debug, Clone, Copy, PartialEq, Eq, Hash)]
pub enum PushErrorCode {
    TopicInvalid,
    TypeInvalid,
    TypeReserved,
    DataInvalid,
    DataNotObject,
    DataTooLarge,
    PushIdInvalid,
    ClientInvalid,
}

impl PushErrorCode {
    /// The code as the contract spells it, e.g. `TOPIC_INVALID`.
    pub const fn as_str(self) -> &'static str {
        match self {
            Self::TopicInvalid => "TOPIC_INVALID",
            Self::TypeInvalid => "TYPE_INVALID",
            Self::TypeReserved => "TYPE_RESERVED",
            Self::DataInvalid => "DATA_INVALID",
            Self::DataNotObject => "DATA_NOT_OBJECT",
            Self::DataTooLarge => "DATA_TOO_LARGE",
            Self::PushIdInvalid => "PUSH_ID_INVALID",
            Self::ClientInvalid => "CLIENT_INVALID",
        }
    }
}

impl fmt::Display for PushErrorCode {
    fn fmt(&self, f: &mut fmt::Formatter<'_>) -> fmt::Result {
        f.write_str(self.as_str())
    }
}

/// A push the writer refuses, so a row the platform would refuse terminally is
/// never stored.
#[derive(Debug, Clone, PartialEq, Eq, thiserror::Error)]
#[error("invalid push: {code}: {message}")]
pub struct PushError {
    pub code: PushErrorCode,
    pub message: String,
}

impl PushError {
    fn new(code: PushErrorCode, message: impl Into<String>) -> Self {
        Self {
            code,
            message: message.into(),
        }
    }
}

fn is_application(s: &str) -> bool {
    let mut b = s.bytes();
    b.next().is_some_and(|c| c.is_ascii_lowercase())
        && b.all(|c| c.is_ascii_lowercase() || c.is_ascii_digit() || c == b'-')
}

fn is_segment(s: &str) -> bool {
    !s.is_empty()
        && s.bytes()
            .all(|c| c.is_ascii_alphanumeric() || matches!(c, b'_' | b'.' | b'-'))
}

fn is_row_id(s: &str) -> bool {
    (1..=26).contains(&s.len())
        && s.bytes()
            .all(|c| c.is_ascii_alphanumeric() || matches!(c, b'_' | b'-'))
}

/// Whitespace-strip JSON text without re-serialising its values (so member
/// order and number spelling are kept).
fn compact_json(text: &str) -> String {
    let mut out = String::with_capacity(text.len());
    let (mut in_string, mut escaped) = (false, false);
    for ch in text.chars() {
        if in_string {
            out.push(ch);
            if escaped {
                escaped = false;
            } else if ch == '\\' {
                escaped = true;
            } else if ch == '"' {
                in_string = false;
            }
        } else if ch == '"' {
            in_string = true;
            out.push(ch);
        } else if !matches!(ch, ' ' | '\t' | '\n' | '\r') {
            out.push(ch);
        }
    }
    out
}

/// A push to the clients of an application, written to the outbox in the same
/// transaction as the change it announces.
///
/// ```
/// use flowcatalyst_sdk::outbox::CreatePush;
///
/// let push = CreatePush::new("shop:orders:ord_2", "order.updated", &serde_json::json!({"status": "shipped"}))
///     .client_code("acme");
/// assert!(push.validate().is_ok());
/// ```
#[derive(Debug, Clone, PartialEq, Eq)]
pub struct CreatePush {
    id: Option<String>,
    topic: String,
    push_type: String,
    /// The data as JSON text, or the reason it could not be produced.
    data: Result<String, String>,
    client_code: Option<String>,
    message_group: Option<String>,
    ungrouped: bool,
    max_data_bytes: Option<usize>,
}

/// The payload of a `PUSH` row, members in the framework's order.
#[derive(Debug, Clone, PartialEq, Eq)]
pub(crate) struct PushPayload {
    pub(crate) json: String,
    pub(crate) client_code: String,
}

#[derive(Serialize)]
struct Str<'a>(&'a str);

impl CreatePush {
    /// A push of `push_type` to `topic` carrying `data`: anything that
    /// serialises to a JSON object (a struct, a map, a `serde_json::Value`).
    /// Anything else is refused by [`validate`](Self::validate) with
    /// `DATA_NOT_OBJECT`.
    pub fn new<T: Serialize + ?Sized>(
        topic: impl Into<String>,
        push_type: impl Into<String>,
        data: &T,
    ) -> Self {
        Self::with_data(
            topic,
            push_type,
            serde_json::to_string(data).map_err(|e| e.to_string()),
        )
    }

    /// As [`new`](Self::new) with the data given as JSON text, whose member
    /// order and number spelling are kept.
    pub fn from_json_text(
        topic: impl Into<String>,
        push_type: impl Into<String>,
        data: impl Into<String>,
    ) -> Self {
        Self::with_data(topic, push_type, Ok(data.into()))
    }

    fn with_data(
        topic: impl Into<String>,
        push_type: impl Into<String>,
        data: Result<String, String>,
    ) -> Self {
        Self {
            id: None,
            topic: topic.into(),
            push_type: push_type.into(),
            data,
            client_code: None,
            message_group: None,
            ungrouped: false,
            max_data_bytes: None,
        }
    }

    /// The row id and the payload's `pushId`; a TSID is generated when unset.
    /// At most 26 characters `[0-9A-Za-z_-]`.
    pub fn id(mut self, id: impl Into<String>) -> Self {
        self.id = Some(id.into());
        self
    }

    /// The client the push is for, by identifier. Unset: the manager's client
    /// code (none for `_platform`, which leaves the credential to decide).
    pub fn client_code(mut self, client_code: impl Into<String>) -> Self {
        self.client_code = Some(client_code.into());
        self
    }

    /// The outbox message group, replacing the topic. Ignored when
    /// [`ungrouped`](Self::ungrouped).
    pub fn message_group(mut self, group: impl Into<String>) -> Self {
        self.message_group = Some(group.into());
        self
    }

    /// Write the row with no message group: batched, concurrent, unordered.
    /// For signal-only topics.
    pub fn ungrouped(mut self) -> Self {
        self.ungrouped = true;
        self
    }

    /// The data limit (default 16 KiB, at most 64 KiB): the application's
    /// `push.max-data-bytes`.
    pub fn max_data_bytes(mut self, n: usize) -> Self {
        self.max_data_bytes = Some(n);
        self
    }

    pub fn topic(&self) -> &str {
        &self.topic
    }

    pub fn push_type(&self) -> &str {
        &self.push_type
    }

    pub(crate) fn row_id(&self) -> Option<&str> {
        self.id.as_deref()
    }

    /// The message group the row is written with: none when ungrouped, else the
    /// override, else the topic.
    pub fn resolved_message_group(&self) -> Option<String> {
        if self.ungrouped {
            return None;
        }
        match self.message_group.as_deref() {
            Some(g) if !g.is_empty() => Some(g.to_owned()),
            _ => Some(self.topic.clone()),
        }
    }

    fn limit(&self) -> usize {
        match self.max_data_bytes {
            Some(n) if n > 0 => n.min(MAX_PUSH_DATA_BYTES),
            _ => DEFAULT_PUSH_MAX_DATA_BYTES,
        }
    }

    /// Apply the contract's section 1 (topic, type, data) and the row's own
    /// rules (id, client code, group), in the Go SDK's order.
    pub fn validate(&self) -> Result<(), PushError> {
        self.compact_data().map(|_| ())
    }

    fn compact_data(&self) -> Result<String, PushError> {
        use PushErrorCode as C;
        if self.topic.len() > MAX_PUSH_TOPIC_LENGTH {
            return Err(PushError::new(
                C::TopicInvalid,
                format!("a topic is at most {MAX_PUSH_TOPIC_LENGTH} characters"),
            ));
        }
        let mut segs = self.topic.split(':');
        let app = segs.next().unwrap_or("");
        let rest: Vec<&str> = segs.collect();
        if rest.is_empty() || !is_application(app) {
            return Err(PushError::new(
                C::TopicInvalid,
                format!(
                    "a topic is {{application}}:{{segment}}(:{{segment}})*: {:?}",
                    self.topic
                ),
            ));
        }
        if rest.iter().any(|s| !is_segment(s)) {
            return Err(PushError::new(
                C::TopicInvalid,
                format!("a topic segment is [0-9A-Za-z_.-]+: {:?}", self.topic),
            ));
        }
        let ty = &self.push_type;
        if ty.len() > MAX_PUSH_TYPE_LENGTH {
            return Err(PushError::new(
                C::TypeInvalid,
                format!("a type is at most {MAX_PUSH_TYPE_LENGTH} characters"),
            ));
        }
        if ty.split('.').any(|p| !is_application(p)) {
            return Err(PushError::new(
                C::TypeInvalid,
                format!("a type is [a-z][a-z0-9-]*(.[a-z][a-z0-9-]*)*: {ty:?}"),
            ));
        }
        if matches!(ty.as_str(), "reset" | "auth-expired" | "subscribed") || ty.starts_with("fw.") {
            return Err(PushError::new(
                C::TypeReserved,
                format!("the type {ty:?} is reserved"),
            ));
        }
        let text = match &self.data {
            Err(e) => {
                return Err(PushError::new(
                    C::DataInvalid,
                    format!("the data does not marshal to JSON: {e}"),
                ));
            }
            Ok(t) => t.trim(),
        };
        if text.is_empty() || serde_json::from_str::<serde::de::IgnoredAny>(text).is_err() {
            return Err(PushError::new(C::DataInvalid, "the data is not valid JSON"));
        }
        if !text.starts_with('{') {
            return Err(PushError::new(
                C::DataNotObject,
                "the data is not a JSON object",
            ));
        }
        let compact = compact_json(text);
        if compact.len() > self.limit() {
            return Err(PushError::new(
                C::DataTooLarge,
                format!(
                    "the data is {} bytes, more than {}",
                    compact.len(),
                    self.limit()
                ),
            ));
        }
        if let Some(id) = self.id.as_deref()
            && !id.is_empty()
            && !is_row_id(id)
        {
            return Err(PushError::new(
                C::PushIdInvalid,
                "id must match [0-9A-Za-z_-]{1,26}",
            ));
        }
        if let Some(code) = self.client_code.as_deref()
            && !code.is_empty()
            && (code.chars().count() > 100 || code.trim().is_empty())
        {
            return Err(PushError::new(
                C::ClientInvalid,
                "clientCode must be 1 to 100 characters",
            ));
        }
        if let Some(g) = self.message_group.as_deref()
            && g.len() > 255
        {
            return Err(PushError::new(
                C::TopicInvalid,
                "messageGroup is at most 255 characters",
            ));
        }
        Ok(compact)
    }

    /// Validate and serialise as the row's payload, members in the framework's
    /// order: `topic`, `type`, `data`, `clientCode`, `pushId`. `data` is the
    /// object itself, never encoded JSON in a string.
    pub(crate) fn to_payload(
        &self,
        push_id: &str,
        default_client_code: Option<&str>,
    ) -> Result<PushPayload, OutboxError> {
        let data = self.compact_data()?;
        let mut code = self.client_code.as_deref().unwrap_or("").trim().to_owned();
        if code.is_empty() {
            let c = default_client_code.unwrap_or("").trim();
            if c != PLATFORM_CLIENT_CODE {
                code = c.to_owned();
            }
        }
        let s = |v: &str| serde_json::to_string(&Str(v));
        let mut json = format!(
            "{{\"topic\":{},\"type\":{},\"data\":{data}",
            s(&self.topic)?,
            s(&self.push_type)?
        );
        if !code.is_empty() {
            json.push_str(",\"clientCode\":");
            json.push_str(&s(&code)?);
        }
        json.push_str(",\"pushId\":");
        json.push_str(&s(push_id)?);
        json.push('}');
        Ok(PushPayload {
            json,
            client_code: code,
        })
    }
}
