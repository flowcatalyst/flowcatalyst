//! Dispatch-job rows: the item `POST /api/dispatch-jobs/batch` ingests.
//!
//! The platform's outbox processor sends each row's payload **unchanged** as
//! one element of `{"items":[…]}`, and a whole-batch `400` is terminal and
//! blocks the row's message group. So the payload is exactly the platform's
//! `BatchItem` (`internal/platform/shared/sdk/dispatch_jobs_batch.go`), with
//! only the fields it reads, and the builder refuses what the platform would
//! refuse (framework contract `write-path.md` section 7a):
//!
//! - `metadata` is an **array** of `{"key","value"}` in insertion order (an
//!   object fails the whole batch with `400 INVALID_JSON`);
//! - `headers`, `retryStrategy`, `scheduledFor`, `expiresAt`,
//!   `idempotencyKey`, `connectionId` and `causationId` are never written: the
//!   endpoint ignores them;
//! - `sequence`, `timeoutSeconds` and `maxRetries` are written only when set,
//!   and must then be at least 1 (the platform turns 0 into its default:
//!   99, 30 and 3).

use std::fmt;

use serde::{Serialize, Serializer};
use url::Url;

use super::error::{InvalidField, OutboxError};
use crate::tsid::Tsid;

/// `EVENT` (fanned out from an event) or `TASK` (created by the application).
#[derive(Debug, Clone, Copy, PartialEq, Eq, Serialize)]
#[serde(rename_all = "SCREAMING_SNAKE_CASE")]
pub enum DispatchKind {
    Event,
    Task,
}

/// How a failure affects the rest of the job's message group.
#[derive(Debug, Clone, Copy, PartialEq, Eq, Serialize)]
#[serde(rename_all = "SCREAMING_SNAKE_CASE")]
pub enum DispatchMode {
    Immediate,
    NextOnError,
    BlockOnError,
}

/// The job's own dispatch priority.
#[derive(Debug, Clone, Copy, PartialEq, Eq, Serialize)]
#[serde(rename_all = "SCREAMING_SNAKE_CASE")]
pub enum DispatchQueue {
    Default,
    HighPriority,
}

/// A validated dispatch job. Build with [`DispatchJob::builder`].
///
/// ```
/// use flowcatalyst_sdk::outbox::{DispatchJob, DispatchMode};
///
/// let job = DispatchJob::builder("orders:billing:export", "https://billing.example.com/hooks/export")
///     .json_payload(&serde_json::json!({"invoice": "inv_1"}))?
///     .message_group("invoice:inv_1")
///     .mode(DispatchMode::BlockOnError)
///     .metadata("invoice", "inv_1")
///     .build()?;
/// assert_eq!(job.id().len(), 13);
/// # Ok::<(), flowcatalyst_sdk::outbox::OutboxError>(())
/// ```
#[derive(Debug, Clone, PartialEq, Eq)]
pub struct DispatchJob {
    id: String,
    kind: DispatchKind,
    code: String,
    source: String,
    subject: Option<String>,
    target_url: String,
    payload: String,
    payload_content_type: String,
    data_only: bool,
    external_id: Option<String>,
    event_id: Option<String>,
    correlation_id: Option<String>,
    client_id: Option<String>,
    subscription_id: Option<String>,
    service_account_id: Option<String>,
    dispatch_pool_id: Option<String>,
    message_group: Option<String>,
    mode: Option<DispatchMode>,
    sequence: Option<u32>,
    timeout_seconds: Option<u32>,
    max_retries: Option<u32>,
    metadata: Vec<(String, String)>,
    queue: Option<DispatchQueue>,
    descriptor: Option<String>,
}

/// Builder for [`DispatchJob`]; [`build`](Self::build) validates.
#[derive(Debug, Clone)]
pub struct DispatchJobBuilder {
    job: DispatchJob,
    source: Option<String>,
    explicit_id: bool,
}

struct Metadata<'a>(&'a [(String, String)]);

impl Serialize for Metadata<'_> {
    fn serialize<S: Serializer>(&self, s: S) -> Result<S::Ok, S::Error> {
        #[derive(Serialize)]
        struct Entry<'a> {
            key: &'a str,
            value: &'a str,
        }
        s.collect_seq(self.0.iter().map(|(key, value)| Entry { key, value }))
    }
}

/// The batch item, in the platform's (and the framework's) key order.
#[derive(Serialize)]
#[serde(rename_all = "camelCase")]
struct BatchItem<'a> {
    id: &'a str,
    kind: DispatchKind,
    code: &'a str,
    source: &'a str,
    #[serde(skip_serializing_if = "Option::is_none")]
    subject: Option<&'a str>,
    target_url: &'a str,
    payload: &'a str,
    payload_content_type: &'a str,
    data_only: bool,
    #[serde(skip_serializing_if = "Option::is_none")]
    external_id: Option<&'a str>,
    #[serde(skip_serializing_if = "Option::is_none")]
    event_id: Option<&'a str>,
    #[serde(skip_serializing_if = "Option::is_none")]
    correlation_id: Option<&'a str>,
    #[serde(skip_serializing_if = "Option::is_none")]
    client_id: Option<&'a str>,
    #[serde(skip_serializing_if = "Option::is_none")]
    subscription_id: Option<&'a str>,
    #[serde(skip_serializing_if = "Option::is_none")]
    service_account_id: Option<&'a str>,
    #[serde(skip_serializing_if = "Option::is_none")]
    dispatch_pool_id: Option<&'a str>,
    #[serde(skip_serializing_if = "Option::is_none")]
    message_group: Option<&'a str>,
    #[serde(skip_serializing_if = "Option::is_none")]
    mode: Option<DispatchMode>,
    #[serde(skip_serializing_if = "Option::is_none")]
    sequence: Option<u32>,
    #[serde(skip_serializing_if = "Option::is_none")]
    timeout_seconds: Option<u32>,
    #[serde(skip_serializing_if = "Option::is_none")]
    max_retries: Option<u32>,
    #[serde(skip_serializing_if = "Option::is_none")]
    metadata: Option<Metadata<'a>>,
    #[serde(skip_serializing_if = "Option::is_none")]
    queue: Option<DispatchQueue>,
    #[serde(skip_serializing_if = "Option::is_none")]
    descriptor: Option<&'a str>,
}

impl DispatchJob {
    /// Start a job with its code (`{application}:{rest}`) and target URL
    /// (absolute `http`/`https`, with a host and no user info).
    ///
    /// Defaults: `kind` `TASK`, a generated 13-character TSID id, `source` the
    /// code's application segment, payload `{}` as `application/json`,
    /// `dataOnly` false.
    pub fn builder(code: impl Into<String>, target_url: impl Into<String>) -> DispatchJobBuilder {
        DispatchJobBuilder {
            job: DispatchJob {
                id: Tsid::generate().to_string(),
                kind: DispatchKind::Task,
                code: code.into(),
                source: String::new(),
                subject: None,
                target_url: target_url.into(),
                payload: "{}".to_owned(),
                payload_content_type: "application/json".to_owned(),
                data_only: false,
                external_id: None,
                event_id: None,
                correlation_id: None,
                client_id: None,
                subscription_id: None,
                service_account_id: None,
                dispatch_pool_id: None,
                message_group: None,
                mode: None,
                sequence: None,
                timeout_seconds: None,
                max_retries: None,
                metadata: Vec::new(),
                queue: None,
                descriptor: None,
            },
            source: None,
            explicit_id: false,
        }
    }

    /// The job id (also the outbox row's id).
    pub fn id(&self) -> &str {
        &self.id
    }

    pub fn code(&self) -> &str {
        &self.code
    }

    /// The message group, when set (also the outbox row's `message_group`).
    pub fn group(&self) -> Option<&str> {
        self.message_group.as_deref()
    }

    /// The batch-item JSON written as the outbox row's payload.
    pub fn to_payload(&self) -> Result<String, OutboxError> {
        Ok(serde_json::to_string(&BatchItem {
            id: &self.id,
            kind: self.kind,
            code: &self.code,
            source: &self.source,
            subject: self.subject.as_deref(),
            target_url: &self.target_url,
            payload: &self.payload,
            payload_content_type: &self.payload_content_type,
            data_only: self.data_only,
            external_id: self.external_id.as_deref(),
            event_id: self.event_id.as_deref(),
            correlation_id: self.correlation_id.as_deref(),
            client_id: self.client_id.as_deref(),
            subscription_id: self.subscription_id.as_deref(),
            service_account_id: self.service_account_id.as_deref(),
            dispatch_pool_id: self.dispatch_pool_id.as_deref(),
            message_group: self.message_group.as_deref(),
            mode: self.mode,
            sequence: self.sequence,
            timeout_seconds: self.timeout_seconds,
            max_retries: self.max_retries,
            metadata: (!self.metadata.is_empty()).then_some(Metadata(&self.metadata)),
            queue: self.queue,
            descriptor: self.descriptor.as_deref(),
        })?)
    }
}

impl DispatchJobBuilder {
    /// The job id, `[0-9A-Za-z_-]{1,17}`. A job sent twice (a lost response)
    /// keeps its id, so the receiver can run the work once.
    pub fn id(mut self, id: impl Into<String>) -> Self {
        self.job.id = id.into();
        self.explicit_id = true;
        self
    }

    pub fn kind(mut self, kind: DispatchKind) -> Self {
        self.job.kind = kind;
        self
    }

    /// 1 to 255 characters; default the code's application segment.
    pub fn source(mut self, source: impl Into<String>) -> Self {
        self.source = Some(source.into());
        self
    }

    pub fn subject(mut self, subject: impl Into<String>) -> Self {
        self.job.subject = Some(subject.into());
        self
    }

    /// A payload of any content type, sent as given.
    pub fn payload(mut self, payload: impl Into<String>, content_type: impl Into<String>) -> Self {
        self.job.payload = payload.into();
        self.job.payload_content_type = content_type.into();
        self
    }

    /// A JSON payload (`application/json`).
    pub fn json_payload(mut self, value: &impl Serialize) -> Result<Self, OutboxError> {
        self.job.payload = serde_json::to_string(value)?;
        self.job.payload_content_type = "application/json".to_owned();
        Ok(self)
    }

    /// `true`: the target receives the raw payload, not the platform envelope.
    pub fn data_only(mut self, data_only: bool) -> Self {
        self.job.data_only = data_only;
        self
    }

    pub fn external_id(mut self, id: impl Into<String>) -> Self {
        self.job.external_id = Some(id.into());
        self
    }

    pub fn event_id(mut self, id: impl Into<String>) -> Self {
        self.job.event_id = Some(id.into());
        self
    }

    pub fn correlation_id(mut self, id: impl Into<String>) -> Self {
        self.job.correlation_id = Some(id.into());
        self
    }

    /// The platform client **id** (`clientId`, at most 17 characters). The
    /// dispatch-jobs batch does not resolve a client code the way the events
    /// batch does.
    pub fn platform_client_id(mut self, id: impl Into<String>) -> Self {
        self.job.client_id = Some(id.into());
        self
    }

    pub fn subscription_id(mut self, id: impl Into<String>) -> Self {
        self.job.subscription_id = Some(id.into());
        self
    }

    pub fn service_account_id(mut self, id: impl Into<String>) -> Self {
        self.job.service_account_id = Some(id.into());
        self
    }

    /// Without one the platform uses its default pool.
    pub fn dispatch_pool_id(mut self, id: impl Into<String>) -> Self {
        self.job.dispatch_pool_id = Some(id.into());
        self
    }

    pub fn message_group(mut self, group: impl Into<String>) -> Self {
        self.job.message_group = Some(group.into());
        self
    }

    pub fn mode(mut self, mode: DispatchMode) -> Self {
        self.job.mode = Some(mode);
        self
    }

    /// At least 1.
    pub fn sequence(mut self, sequence: u32) -> Self {
        self.job.sequence = Some(sequence);
        self
    }

    /// At least 1.
    pub fn timeout_seconds(mut self, seconds: u32) -> Self {
        self.job.timeout_seconds = Some(seconds);
        self
    }

    /// At least 1 (the platform's default is 3).
    pub fn max_retries(mut self, retries: u32) -> Self {
        self.job.max_retries = Some(retries);
        self
    }

    /// Add a metadata entry; a repeated key replaces the earlier value in place.
    pub fn metadata(mut self, key: impl Into<String>, value: impl Into<String>) -> Self {
        let (key, value) = (key.into(), value.into());
        match self.job.metadata.iter_mut().find(|(k, _)| *k == key) {
            Some(entry) => entry.1 = value,
            None => self.job.metadata.push((key, value)),
        }
        self
    }

    pub fn queue(mut self, queue: DispatchQueue) -> Self {
        self.job.queue = Some(queue);
        self
    }

    /// What the job is, in words (the dispatch-jobs grid); 1 to 255 characters.
    pub fn descriptor(mut self, descriptor: impl Into<String>) -> Self {
        self.job.descriptor = Some(descriptor.into());
        self
    }

    /// Validate and build. The first refusal is returned.
    pub fn build(mut self) -> Result<DispatchJob, InvalidField> {
        let job = &mut self.job;
        let app = validate_code(&job.code)?;
        job.source = self.source.take().unwrap_or_else(|| app.to_owned());
        text("source", &job.source, 255)?;
        validate_target_url(&job.target_url)?;
        if self.explicit_id {
            platform_id("id", &job.id)?;
        }
        for (field, value) in [
            ("eventId", &job.event_id),
            ("clientId", &job.client_id),
            ("subscriptionId", &job.subscription_id),
            ("serviceAccountId", &job.service_account_id),
            ("dispatchPoolId", &job.dispatch_pool_id),
        ] {
            if let Some(v) = value {
                platform_id(field, v)?;
            }
        }
        for (field, value) in [
            ("subject", &job.subject),
            ("externalId", &job.external_id),
            ("correlationId", &job.correlation_id),
            ("messageGroup", &job.message_group),
            ("descriptor", &job.descriptor),
        ] {
            if let Some(v) = value {
                text(field, v, 255)?;
            }
        }
        text("payloadContentType", &job.payload_content_type, 100)?;
        for (field, value) in [
            ("sequence", job.sequence),
            ("timeoutSeconds", job.timeout_seconds),
            ("maxRetries", job.max_retries),
        ] {
            if value == Some(0) {
                return Err(InvalidField::new(
                    field,
                    "must be at least 1 when set (the platform reads 0 as its default)",
                ));
            }
        }
        if job.metadata.len() > 100 {
            return Err(InvalidField::new("metadata", "at most 100 entries"));
        }
        if job.metadata.iter().any(|(k, _)| k.trim().is_empty()) {
            return Err(InvalidField::new("metadata", "keys must not be blank"));
        }
        Ok(self.job)
    }
}

impl fmt::Display for DispatchKind {
    fn fmt(&self, f: &mut fmt::Formatter<'_>) -> fmt::Result {
        f.write_str(match self {
            DispatchKind::Event => "EVENT",
            DispatchKind::Task => "TASK",
        })
    }
}

/// `{application}:{rest}`, the application segment `[a-z][a-z0-9-]*`, at most
/// 255 characters. Returns the application segment.
fn validate_code(code: &str) -> Result<&str, InvalidField> {
    let bad = || {
        InvalidField::new(
            "code",
            format!(
                "{code:?} must be {{application}}:{{rest}} with an application segment [a-z][a-z0-9-]*, at most 255 characters"
            ),
        )
    };
    if code.len() > 255 {
        return Err(bad());
    }
    let (app, rest) = code.split_once(':').ok_or_else(bad)?;
    let mut chars = app.bytes();
    let first_ok = chars.next().is_some_and(|c| c.is_ascii_lowercase());
    if !first_ok
        || !chars.all(|c| c.is_ascii_lowercase() || c.is_ascii_digit() || c == b'-')
        || rest.trim().is_empty()
    {
        return Err(bad());
    }
    Ok(app)
}

fn validate_target_url(target: &str) -> Result<(), InvalidField> {
    let url = Url::parse(target)
        .map_err(|e| InvalidField::new("targetUrl", format!("not an absolute URL: {e}")))?;
    if !matches!(url.scheme(), "http" | "https") {
        return Err(InvalidField::new(
            "targetUrl",
            "scheme must be http or https",
        ));
    }
    if url.host_str().is_none_or(str::is_empty) {
        return Err(InvalidField::new("targetUrl", "must have a host"));
    }
    if !url.username().is_empty() || url.password().is_some() {
        return Err(InvalidField::new("targetUrl", "must not carry user info"));
    }
    Ok(())
}

/// Platform ids are `VARCHAR(17)`: `[0-9A-Za-z_-]{1,17}`.
fn platform_id(field: &'static str, value: &str) -> Result<(), InvalidField> {
    if (1..=17).contains(&value.len())
        && value
            .bytes()
            .all(|b| b.is_ascii_alphanumeric() || b == b'_' || b == b'-')
    {
        Ok(())
    } else {
        Err(InvalidField::new(field, "must be [0-9A-Za-z_-]{1,17}"))
    }
}

/// 1 to `max` characters (the column is `VARCHAR(max)`).
fn text(field: &'static str, value: &str, max: usize) -> Result<(), InvalidField> {
    let len = value.chars().count();
    if len == 0 || len > max {
        Err(InvalidField::new(
            field,
            format!("must be 1 to {max} characters"),
        ))
    } else {
        Ok(())
    }
}

#[cfg(test)]
mod tests {
    use super::*;
    use serde_json::Value;

    const TARGET: &str = "https://billing.example.com/hooks/export";

    #[test]
    fn minimal_job_writes_only_the_always_fields() {
        let job = DispatchJob::builder("orders:billing:export", TARGET)
            .build()
            .unwrap();
        let v: Value = serde_json::from_str(&job.to_payload().unwrap()).unwrap();
        let keys: Vec<&str> = v.as_object().unwrap().keys().map(String::as_str).collect();
        let mut expected = vec![
            "code",
            "dataOnly",
            "id",
            "kind",
            "payload",
            "payloadContentType",
            "source",
            "targetUrl",
        ];
        expected.sort_unstable();
        assert_eq!(keys, expected);
        assert_eq!(v["kind"], "TASK");
        assert_eq!(v["source"], "orders");
        assert_eq!(v["payload"], "{}");
        assert_eq!(v["dataOnly"], false);
        assert_eq!(v["id"].as_str().unwrap().len(), 13);
    }

    #[test]
    fn metadata_is_an_array_in_insertion_order() {
        let job = DispatchJob::builder("orders:billing:export", TARGET)
            .metadata("b", "1")
            .metadata("a", "2")
            .metadata("b", "3")
            .build()
            .unwrap();
        let v: Value = serde_json::from_str(&job.to_payload().unwrap()).unwrap();
        assert_eq!(
            v["metadata"],
            serde_json::json!([{"key":"b","value":"3"},{"key":"a","value":"2"}])
        );
    }

    #[test]
    fn refuses_what_the_platform_would_refuse() {
        let cases: Vec<(DispatchJobBuilder, &str)> = vec![
            (DispatchJob::builder("export", TARGET), "code"),
            (DispatchJob::builder("Orders:export", TARGET), "code"),
            (DispatchJob::builder("orders:", TARGET), "code"),
            (DispatchJob::builder("orders:x", "/relative"), "targetUrl"),
            (DispatchJob::builder("orders:x", "ftp://h/x"), "targetUrl"),
            (
                DispatchJob::builder("orders:x", "https://u:p@h/x"),
                "targetUrl",
            ),
            (
                DispatchJob::builder("orders:x", TARGET).id("way-too-long-for-the-column"),
                "id",
            ),
            (DispatchJob::builder("orders:x", TARGET).id("bad id"), "id"),
            (
                DispatchJob::builder("orders:x", TARGET).dispatch_pool_id(""),
                "dispatchPoolId",
            ),
            (
                DispatchJob::builder("orders:x", TARGET).max_retries(0),
                "maxRetries",
            ),
            (
                DispatchJob::builder("orders:x", TARGET).timeout_seconds(0),
                "timeoutSeconds",
            ),
            (
                DispatchJob::builder("orders:x", TARGET).sequence(0),
                "sequence",
            ),
            (
                DispatchJob::builder("orders:x", TARGET).message_group("g".repeat(256)),
                "messageGroup",
            ),
            (
                DispatchJob::builder("orders:x", TARGET).source(""),
                "source",
            ),
            (
                DispatchJob::builder("orders:x", TARGET).metadata(" ", "v"),
                "metadata",
            ),
        ];
        for (builder, field) in cases {
            let err = builder.build().unwrap_err();
            assert_eq!(err.field, field, "{err}");
        }
    }
}
