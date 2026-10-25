//! Ingestion management operations: ingestions, their versions, sets of them,
//! the source kinds and connection tests.
//!
//! An ingestion is `(application, client, code)` plus a source, a split and a
//! target. The source and the split are tagged unions on `kind`; a source kind
//! a runtime extension supplies is [`Source::Custom`]. Secrets in a definition
//! are [`SecretRef`] names, never values: nothing here handles a secret value.
//! The runtime operations (`/api/ingestion-runtime/v1/*`) are the runtime's
//! own client and are not part of this SDK.
//!
//! Without the promote permission a create, update or version save keeps the
//! definition as a **draft**: the answer's `save.outcome` is `DRAFT` and
//! `save.awaiting_promote` is true until someone who holds promote makes it
//! live ([`Ingestions::make_version_live`]). A definition an existing version
//! already says is not saved again (`UNCHANGED`, `MADE_LIVE`, `DRAFT_EXISTS`);
//! versions carry `draft` and a `digest`. `warnings` names what will not run
//! as meant, such as an sftp source with neither `known_hosts` nor
//! `accept_any_host_key`.
//!
//! Notices are failures the platform refused (category `rejected`); an
//! ingestion counts the open ones (`notices`, `latest_notice`).
//! [`Ingestions::list_failures`] with `category: Some("rejected")` and
//! `undismissed: true` lists them, [`Ingestions::dismiss_failure`] dismisses
//! one (it stays listed), and [`Ingestions::list_secrets`] lists the secret
//! names the live definition binds with each runtime's last fetch, never a
//! value.

use super::{ClientError, FlowCatalystClient};
use crate::client::seg;
use serde::{Deserialize, Serialize};
use serde_json::{Map, Value};

/// The name of a secret, resolved by the runtime against the ingestion's client.
#[derive(Debug, Clone, PartialEq, Eq, Serialize, Deserialize)]
pub struct SecretRef {
    pub name: String,
}

impl SecretRef {
    pub fn new(name: impl Into<String>) -> Self {
        Self { name: name.into() }
    }
}

/// Where an ingestion's data comes from. Durations are ISO 8601 (`PT5M`).
// A definition is built once and sent; boxing the api-poll auth would only cost
// every caller a `Box::new`.
#[allow(clippy::large_enum_variant)]
#[derive(Debug, Clone, PartialEq, Serialize, Deserialize)]
#[serde(tag = "kind", rename_all = "kebab-case")]
pub enum Source {
    /// JSON posted to the endpoint the runtime exposes for the ingestion.
    #[serde(rename_all = "camelCase")]
    Http {
        auth: HttpAuth,
        #[serde(default, skip_serializing_if = "Option::is_none")]
        rate_limit: Option<SourceRateLimit>,
    },
    /// Files polled in a directory on an SFTP server.
    #[serde(rename_all = "camelCase")]
    Sftp {
        host: String,
        port: u16,
        directory: String,
        username: String,
        credential: SecretRef,
        /// The server's host key as a known_hosts line. Without it the runtime
        /// refuses to start the source unless `accept_any_host_key` is set.
        #[serde(default, skip_serializing_if = "Option::is_none")]
        known_hosts: Option<String>,
        /// Connect without checking who the server is: whoever answers at the
        /// address receives the credential, so turning it on needs the
        /// bind-secrets permission.
        #[serde(default, skip_serializing_if = "std::ops::Not::not")]
        accept_any_host_key: bool,
        /// A file name pattern such as `*.csv`; absent takes every file.
        #[serde(default, skip_serializing_if = "Option::is_none")]
        include: Option<String>,
        every: String,
    },
    /// Objects directly under a prefix of a bucket.
    S3 {
        region: String,
        bucket: String,
        #[serde(default, skip_serializing_if = "Option::is_none")]
        prefix: Option<String>,
        every: String,
        /// Absent reads with the runtime's own identity.
        #[serde(default, skip_serializing_if = "Option::is_none")]
        credential: Option<AwsCredential>,
    },
    /// Messages from a queue, named, not its URL.
    Sqs {
        region: String,
        queue: String,
        #[serde(default, skip_serializing_if = "Option::is_none")]
        credential: Option<AwsCredential>,
    },
    /// Messages from a queue; `uri` is `amqp://` or `amqps://` without a login.
    #[serde(rename = "rabbitmq")]
    RabbitMq {
        uri: String,
        queue: String,
        username: String,
        password: SecretRef,
    },
    /// Rows of a table or view; each row is a record, so the split is `none`.
    #[serde(rename_all = "camelCase")]
    DatabasePoll {
        jdbc_url: String,
        username: String,
        password: SecretRef,
        table: String,
        track: DatabaseTrack,
        max_rows: u32,
        every: String,
    },
    /// One request per poll to an HTTP API, the answer split.
    #[serde(rename_all = "camelCase")]
    ApiPoll {
        url: String,
        method: String,
        #[serde(default, skip_serializing_if = "Vec::is_empty")]
        headers: Vec<ApiHeader>,
        #[serde(default, skip_serializing_if = "Option::is_none")]
        body: Option<String>,
        auth: ApiAuth,
        results: PollResults,
        #[serde(default, skip_serializing_if = "Option::is_none")]
        cursor: Option<ApiCursor>,
        /// The wait after an answer with results.
        every: String,
        /// The wait after an answer without.
        idle_delay: String,
    },
    /// A kind a runtime extension supplies (or `camel-route`): its `type` and
    /// free-form settings. A secret setting is the secret's name.
    Custom {
        #[serde(rename = "type")]
        kind_type: String,
        #[serde(default, skip_serializing_if = "Map::is_empty")]
        settings: Map<String, Value>,
    },
}

/// At most `requests` per `per` (ISO 8601), counted per runtime.
#[derive(Debug, Clone, PartialEq, Serialize, Deserialize)]
pub struct SourceRateLimit {
    pub requests: u32,
    pub per: String,
}

/// An AWS access key; the secret half is a name.
#[derive(Debug, Clone, PartialEq, Serialize, Deserialize)]
#[serde(rename_all = "camelCase")]
pub struct AwsCredential {
    pub access_key_id: String,
    pub secret_access_key: SecretRef,
}

/// A header sent on every api-poll request; none of them is secret.
#[derive(Debug, Clone, PartialEq, Serialize, Deserialize)]
pub struct ApiHeader {
    pub name: String,
    pub value: String,
}

/// What in an api-poll answer moves the next request on.
#[derive(Debug, Clone, PartialEq, Serialize, Deserialize)]
pub struct ApiCursor {
    /// A path such as `$.next`.
    pub from: String,
    /// The query parameter it is sent in.
    pub param: String,
    #[serde(default, skip_serializing_if = "Option::is_none")]
    pub initial: Option<String>,
}

/// How a database poll tells rows read from rows not yet read.
#[derive(Debug, Clone, PartialEq, Serialize, Deserialize)]
#[serde(tag = "kind", rename_all = "kebab-case")]
pub enum DatabaseTrack {
    /// A column that only grows; `column_type` is `NUMBER` or `TIMESTAMP`.
    Watermark {
        column: String,
        #[serde(rename = "type")]
        column_type: String,
    },
    /// Rows whose status column holds `pending`, set to `processed` once read.
    #[serde(rename_all = "camelCase")]
    Status {
        column: String,
        key_column: String,
        pending: String,
        processed: String,
    },
}

/// How the caller of an `http` source authenticates.
#[derive(Debug, Clone, PartialEq, Serialize, Deserialize)]
#[serde(tag = "kind", rename_all = "kebab-case")]
pub enum HttpAuth {
    /// The caller's own platform access token; it must reach the client and
    /// the application. `principal_id` narrows it to one principal.
    #[serde(rename_all = "camelCase")]
    FlowcatalystToken {
        application: String,
        #[serde(default, skip_serializing_if = "Option::is_none")]
        principal_id: Option<String>,
    },
    Basic {
        username: String,
        password: SecretRef,
    },
    Bearer {
        token: SecretRef,
    },
}

/// How an api-poll logs in to the API.
#[derive(Debug, Clone, PartialEq, Serialize, Deserialize)]
#[serde(tag = "kind", rename_all = "kebab-case")]
pub enum ApiAuth {
    None,
    Basic {
        username: String,
        password: SecretRef,
    },
    Bearer {
        token: SecretRef,
    },
    ApiKey {
        header: String,
        value: SecretRef,
    },
    /// The OAuth client credentials grant; `credentials_in` is `BASIC_HEADER`
    /// or `BODY`.
    #[serde(rename = "oauth-client-credentials", rename_all = "camelCase")]
    OauthClientCredentials {
        token_url: String,
        client_id: String,
        client_secret: SecretRef,
        #[serde(default, skip_serializing_if = "Option::is_none")]
        scope: Option<String>,
        #[serde(default, skip_serializing_if = "Option::is_none")]
        audience: Option<String>,
        credentials_in: String,
    },
    /// A username and password posted to a login endpoint; `format` is `FORM`
    /// or `JSON`, `lifetime` ISO 8601.
    #[serde(rename_all = "camelCase")]
    Login {
        url: String,
        format: String,
        username_field: String,
        password_field: String,
        username: String,
        password: SecretRef,
        #[serde(default, skip_serializing_if = "Map::is_empty")]
        extra_fields: Map<String, Value>,
        token_path: String,
        #[serde(default, skip_serializing_if = "Option::is_none")]
        expires_in_path: Option<String>,
        lifetime: String,
        header: String,
        prefix: String,
    },
}

/// How an api-poll answer is told to have results.
#[derive(Debug, Clone, PartialEq, Serialize, Deserialize)]
#[serde(tag = "kind", rename_all = "kebab-case")]
pub enum PollResults {
    /// The split makes at least one record of the answer.
    Records,
    /// `path` (`$.items`) holds a non-empty array or a value that is not
    /// null, false or empty.
    JsonPath { path: String },
    /// The answer carries the header, optionally with the value `equals`.
    Header {
        name: String,
        #[serde(default, skip_serializing_if = "Option::is_none")]
        equals: Option<String>,
    },
}

/// How a payload becomes records.
#[derive(Debug, Clone, PartialEq, Serialize, Deserialize)]
#[serde(tag = "kind", rename_all = "kebab-case")]
pub enum Split {
    /// One record per payload.
    None,
    /// One record per line; `header` drops the first.
    Csv { header: bool },
    /// One record per element of the array at `$` or `$.field.field`.
    #[serde(rename_all = "camelCase")]
    Json { array_path: String },
    /// One record per occurrence of `element`.
    Xml { element: String },
}

/// What happens to a group's later jobs when one fails.
#[derive(Debug, Clone, Copy, PartialEq, Eq, Serialize, Deserialize)]
#[serde(rename_all = "SCREAMING_SNAKE_CASE")]
pub enum TargetMode {
    Immediate,
    NextOnError,
    BlockOnError,
}

/// The dispatch job each record becomes. `connection` and `service_account`
/// are codes and may not both be set; neither is unsigned delivery.
#[derive(Debug, Clone, PartialEq, Serialize, Deserialize)]
#[serde(rename_all = "camelCase")]
pub struct Target {
    pub code: String,
    pub target_url: String,
    /// A dispatch pool code.
    pub pool: String,
    #[serde(default, skip_serializing_if = "Option::is_none")]
    pub connection: Option<String>,
    #[serde(default, skip_serializing_if = "Option::is_none")]
    pub service_account: Option<String>,
    pub mode: TargetMode,
    pub batch_size: i64,
}

/// A definition without its identity: what a test takes and a version holds.
#[derive(Debug, Clone, PartialEq, Serialize, Deserialize)]
#[serde(rename_all = "camelCase")]
pub struct IngestionDefinition {
    pub name: String,
    pub source: Source,
    pub split: Split,
    pub target: Target,
    #[serde(default, skip_serializing_if = "Option::is_none")]
    pub keep_failed_payloads: Option<bool>,
}

/// Request to create an ingestion — `POST /api/ingestions`.
#[derive(Debug, Clone, PartialEq, Serialize, Deserialize)]
#[serde(rename_all = "camelCase")]
pub struct CreateIngestionRequest {
    /// Code of the owning application, which must be of type INTEGRATION.
    pub application: String,
    /// The client's id (`clt_...`) or identifier.
    pub client: String,
    /// Lowercase letters, digits and hyphens, unique within the application
    /// and client.
    pub code: String,
    pub name: String,
    pub source: Source,
    pub split: Split,
    pub target: Target,
    #[serde(default, skip_serializing_if = "Option::is_none")]
    pub keep_failed_payloads: Option<bool>,
    /// Make the first version live (the platform's default is true).
    #[serde(default, skip_serializing_if = "Option::is_none")]
    pub live: Option<bool>,
    #[serde(default, skip_serializing_if = "Option::is_none")]
    pub note: Option<String>,
}

/// Request to update an ingestion — `PUT /api/ingestions/{id}`.
#[derive(Debug, Clone, PartialEq, Serialize, Deserialize)]
#[serde(rename_all = "camelCase")]
pub struct UpdateIngestionRequest {
    /// The live version the caller read (0 when none is live); a different
    /// current version is a 409.
    pub expected_version: i64,
    pub name: String,
    pub source: Source,
    pub split: Split,
    pub target: Target,
    #[serde(default, skip_serializing_if = "Option::is_none")]
    pub keep_failed_payloads: Option<bool>,
    #[serde(default, skip_serializing_if = "Option::is_none")]
    pub note: Option<String>,
}

/// Request to save a version — `POST /api/ingestions/{id}/versions`.
#[derive(Debug, Clone, PartialEq, Serialize, Deserialize)]
#[serde(rename_all = "camelCase")]
pub struct SaveIngestionVersionRequest {
    pub name: String,
    pub source: Source,
    pub split: Split,
    pub target: Target,
    #[serde(default, skip_serializing_if = "Option::is_none")]
    pub keep_failed_payloads: Option<bool>,
    /// Also make the version live (needs the promote permission).
    #[serde(default, skip_serializing_if = "Option::is_none")]
    pub live: Option<bool>,
    #[serde(default, skip_serializing_if = "Option::is_none")]
    pub note: Option<String>,
}

/// Filters and cursor of `GET /api/ingestions`; unset fields are not sent.
#[derive(Debug, Clone, Default)]
pub struct ListIngestionsQuery {
    pub application_code: Option<String>,
    /// A client's id or identifier.
    pub client_id: Option<String>,
    /// Matches the code or name, ignoring case.
    pub q: Option<String>,
    /// `ACTIVE` or `PAUSED`.
    pub status: Option<String>,
    pub kind: Option<String>,
    /// The previous page's `next_cursor`.
    pub after: Option<String>,
    /// Page size, 1 to 200 (default 50).
    pub limit: Option<u32>,
}

/// Ingestion as returned by the platform.
#[derive(Debug, Clone, PartialEq, Serialize, Deserialize)]
#[serde(rename_all = "camelCase")]
pub struct IngestionResponse {
    pub id: String,
    pub application_id: String,
    pub application_code: String,
    pub client_id: String,
    pub client_identifier: String,
    pub code: String,
    /// `ACTIVE` or `PAUSED`.
    pub status: String,
    pub version: i64,
    pub latest_version: i64,
    pub created_at: String,
    pub updated_at: String,
    pub name: String,
    pub source: Source,
    pub split: Split,
    pub target: Target,
    #[serde(default)]
    pub keep_failed_payloads: bool,
    /// Id of the set that owns the ingestion; read-only until it is released.
    #[serde(default)]
    pub managed_by: Option<String>,
    #[serde(default)]
    pub runtime_notice: Option<String>,
    /// Undismissed notices: failures the platform refused (category `rejected`).
    #[serde(default)]
    pub notices: i64,
    /// The newest undismissed notice's message.
    #[serde(default)]
    pub latest_notice: Option<String>,
    #[serde(default)]
    pub latest_notice_at: Option<String>,
    /// What the definition says that will not run as meant.
    #[serde(default)]
    pub warnings: Vec<String>,
    /// What this create or update did (set on those answers only).
    #[serde(default)]
    pub save: Option<IngestionSaveOutcome>,
}

/// What a create, update or version save did. `outcome` is `UNCHANGED` (the
/// live version already says it, nothing written), `SAVED` (saved and made
/// live), `MADE_LIVE` (a saved version that was not live already says it and
/// was made live), `DRAFT` (saved as a version that is not live: no promote
/// permission, or not asked to be live) or `DRAFT_EXISTS` (a saved draft
/// already says it, nothing saved). `version` is the live version or the draft.
#[derive(Debug, Clone, PartialEq, Eq, Serialize, Deserialize)]
#[serde(rename_all = "camelCase")]
pub struct IngestionSaveOutcome {
    pub outcome: String,
    pub version: i64,
    /// The definition waits in a version that is not live for someone who
    /// holds promote to make it live.
    pub awaiting_promote: bool,
    #[serde(default)]
    pub message: String,
}

/// One page of ingestions — `GET /api/ingestions`.
#[derive(Debug, Clone, Serialize, Deserialize)]
#[serde(rename_all = "camelCase")]
pub struct IngestionListResponse {
    pub ingestions: Vec<IngestionResponse>,
    pub has_more: bool,
    /// Pass as `after` for the next page; absent on the last.
    #[serde(default)]
    pub next_cursor: Option<String>,
}

/// A saved version of an ingestion.
#[derive(Debug, Clone, PartialEq, Serialize, Deserialize)]
#[serde(rename_all = "camelCase")]
pub struct IngestionVersionResponse {
    pub id: String,
    pub version: i64,
    pub live: bool,
    /// Saved but not live: waits for someone who holds promote to make it live.
    #[serde(default)]
    pub draft: bool,
    pub created_at: String,
    pub name: String,
    pub source: Source,
    pub split: Split,
    pub target: Target,
    #[serde(default)]
    pub keep_failed_payloads: bool,
    /// Id of the set apply that produced the version.
    #[serde(default)]
    pub applied_from: Option<String>,
    #[serde(default)]
    pub created_by: Option<String>,
    #[serde(default)]
    pub note: Option<String>,
    /// sha256 (hex) of the definition's canonical form; equal definitions have
    /// equal digests.
    #[serde(default)]
    pub digest: Option<String>,
    /// What this save did (the answer of `save_version` only).
    #[serde(default)]
    pub save: Option<IngestionSaveOutcome>,
}

/// Versions of an ingestion — `GET /api/ingestions/{id}/versions`.
#[derive(Debug, Clone, Serialize, Deserialize)]
pub struct IngestionVersionListResponse {
    pub versions: Vec<IngestionVersionResponse>,
}

/// One operation on an ingestion, from the audit log.
#[derive(Debug, Clone, Serialize, Deserialize)]
#[serde(rename_all = "camelCase")]
pub struct IngestionHistoryEntry {
    pub id: String,
    pub operation: String,
    pub performed_at: String,
    /// The command the operation ran with.
    #[serde(default)]
    pub details: Option<Value>,
    #[serde(default)]
    pub principal_id: Option<String>,
    #[serde(default)]
    pub principal_name: Option<String>,
}

/// `GET /api/ingestions/{id}/history`.
#[derive(Debug, Clone, Serialize, Deserialize)]
#[serde(rename_all = "camelCase")]
pub struct IngestionHistoryResponse {
    pub entries: Vec<IngestionHistoryEntry>,
    pub has_more: bool,
}

/// A failure a runtime reported.
#[derive(Debug, Clone, Serialize, Deserialize)]
#[serde(rename_all = "camelCase")]
pub struct IngestionFailureResponse {
    pub id: String,
    pub kind: String,
    /// `payload`: a record or payload the runtime could not take. `rejected`:
    /// the platform refused the runtime's jobs permanently, shown on the
    /// ingestion as a notice until someone dismisses it.
    #[serde(default)]
    pub category: String,
    pub message: String,
    pub after_records: i64,
    pub occurred_at: String,
    #[serde(default)]
    pub created_at: Option<String>,
    /// The start of the payload, only for an ingestion that keeps failed payloads.
    #[serde(default)]
    pub excerpt: Option<String>,
    #[serde(default)]
    pub runtime_id: Option<String>,
    #[serde(default)]
    pub version: Option<i64>,
    #[serde(default)]
    pub dismissed_at: Option<String>,
    /// The principal who dismissed it.
    #[serde(default)]
    pub dismissed_by: Option<String>,
}

/// Filters for [`Ingestions::list_failures`]; every field is optional.
#[derive(Debug, Clone, Default)]
pub struct ListIngestionFailuresQuery {
    /// The previous page's `next_cursor`.
    pub after: Option<String>,
    pub limit: Option<u32>,
    /// `payload` or `rejected` (the notices).
    pub category: Option<String>,
    /// Only the failures nobody has dismissed.
    pub undismissed: bool,
}

/// When a runtime last fetched a bound secret.
#[derive(Debug, Clone, PartialEq, Eq, Serialize, Deserialize)]
#[serde(rename_all = "camelCase")]
pub struct IngestionSecretFetch {
    pub runtime_id: String,
    /// The live version the last fetch was made under.
    pub version: i64,
    /// The runtime's service account.
    #[serde(default)]
    pub principal_id: Option<String>,
    pub last_fetched_at: String,
    pub fetch_count: i64,
}

/// A secret *name* the ingestion binds (`bound`), or bound under an earlier
/// live version; never its value.
#[derive(Debug, Clone, PartialEq, Eq, Serialize, Deserialize)]
#[serde(rename_all = "camelCase")]
pub struct IngestionSecretResponse {
    pub name: String,
    pub bound: bool,
    /// Per runtime, newest first; empty when no runtime has fetched it.
    #[serde(default)]
    pub fetches: Vec<IngestionSecretFetch>,
}

/// `GET /api/ingestions/{id}/secrets`.
#[derive(Debug, Clone, Serialize, Deserialize)]
pub struct IngestionSecretListResponse {
    pub secrets: Vec<IngestionSecretResponse>,
}

/// One page of failures — `GET /api/ingestions/{id}/failures`.
#[derive(Debug, Clone, Serialize, Deserialize)]
#[serde(rename_all = "camelCase")]
pub struct IngestionFailureListResponse {
    pub failures: Vec<IngestionFailureResponse>,
    pub has_more: bool,
    #[serde(default)]
    pub next_cursor: Option<String>,
}

/// A source kind the form can offer.
#[derive(Debug, Clone, Serialize, Deserialize)]
#[serde(rename_all = "camelCase")]
pub struct IngestionSourceKindResponse {
    pub kind: String,
    pub built_in: bool,
    pub single_owner: bool,
    #[serde(default)]
    pub description: Option<String>,
    /// Creating or changing an ingestion of this kind needs this permission.
    #[serde(default)]
    pub required_permission: Option<String>,
    /// The kind's settings as the runtime describes them.
    #[serde(default)]
    pub settings: Option<Value>,
}

/// `GET /api/ingestion-source-kinds`.
#[derive(Debug, Clone, Serialize, Deserialize)]
pub struct IngestionSourceKindListResponse {
    pub kinds: Vec<IngestionSourceKindResponse>,
}

/// Request to test a connection — `POST /api/ingestion-tests`: a saved
/// ingestion's live definition (`ingestion_id`), or a `definition` with its
/// `application` and `client`.
#[derive(Debug, Clone, Default, Serialize, Deserialize)]
#[serde(rename_all = "camelCase")]
pub struct TestIngestionRequest {
    #[serde(default, skip_serializing_if = "Option::is_none")]
    pub ingestion_id: Option<String>,
    #[serde(default, skip_serializing_if = "Option::is_none")]
    pub definition: Option<IngestionDefinition>,
    #[serde(default, skip_serializing_if = "Option::is_none")]
    pub application: Option<String>,
    #[serde(default, skip_serializing_if = "Option::is_none")]
    pub client: Option<String>,
}

/// A connection test and, once a runtime answered, its result.
#[derive(Debug, Clone, Serialize, Deserialize)]
#[serde(rename_all = "camelCase")]
pub struct IngestionTestResponse {
    pub id: String,
    pub kind: String,
    /// `PENDING`, `RUNNING`, `DONE` or `EXPIRED`.
    pub status: String,
    pub created_at: String,
    pub expires_at: String,
    /// `RESULTS`, `NO_RESULTS`, `REFUSED`, `FAILED` or `NOT_SUPPORTED`, once done.
    #[serde(default)]
    pub outcome: Option<String>,
    #[serde(default)]
    pub completed_at: Option<String>,
    #[serde(default)]
    pub excerpt: Option<String>,
    #[serde(default)]
    pub ingestion_id: Option<String>,
    /// Set while no live runtime carries the kind: the test waits.
    #[serde(default)]
    pub notice: Option<String>,
    /// How many records the split makes of the answer.
    #[serde(default)]
    pub records: Option<i64>,
    #[serde(default)]
    pub runtime_id: Option<String>,
    /// What the runtime did, step by step.
    #[serde(default)]
    pub steps: Vec<String>,
}

/// One ingestion of a set — a definition with its identity.
#[derive(Debug, Clone, PartialEq, Serialize, Deserialize)]
#[serde(rename_all = "camelCase")]
pub struct IngestionSetItem {
    pub application: String,
    pub client: String,
    pub code: String,
    pub name: String,
    pub source: Source,
    pub split: Split,
    pub target: Target,
    /// Whether the ingestion runs (the platform's default is true); an apply
    /// sets it, so a pause made since is undone.
    #[serde(default, skip_serializing_if = "Option::is_none")]
    pub active: Option<bool>,
    #[serde(default, skip_serializing_if = "Option::is_none")]
    pub keep_failed_payloads: Option<bool>,
}

/// A file a set's items were read from, kept with the apply.
#[derive(Debug, Clone, PartialEq, Eq, Serialize, Deserialize)]
pub struct SetFile {
    pub path: String,
    pub content: String,
}

/// Request to apply a set — `PUT /api/ingestion-sets/{name}`.
#[derive(Debug, Clone, Default, Serialize, Deserialize)]
pub struct ApplyIngestionSetRequest {
    pub items: Vec<IngestionSetItem>,
    /// At most 500 files and 2 MB in all.
    #[serde(default, skip_serializing_if = "Vec::is_empty")]
    pub files: Vec<SetFile>,
}

/// What an apply did to one ingestion.
#[derive(Debug, Clone, Serialize, Deserialize)]
#[serde(rename_all = "camelCase")]
pub struct SetChange {
    pub application: String,
    pub client: String,
    pub code: String,
    /// `CREATE`, `CHANGE`, `ADOPT`, `UNCHANGED` or `REMOVE`.
    pub action: String,
    pub changed: Vec<String>,
    #[serde(default)]
    pub ingestion_id: Option<String>,
}

/// The changes of an apply or a dry run.
#[derive(Debug, Clone, Serialize, Deserialize)]
#[serde(rename_all = "camelCase")]
pub struct IngestionSetPlanResponse {
    pub name: String,
    pub dry_run: bool,
    pub changes: Vec<SetChange>,
    /// The recorded apply; absent for a dry run and for an apply that changed nothing.
    #[serde(default)]
    pub apply_id: Option<String>,
}

/// A set with the number of ingestions it owns.
#[derive(Debug, Clone, Serialize, Deserialize)]
#[serde(rename_all = "camelCase")]
pub struct IngestionSetResponse {
    pub id: String,
    pub name: String,
    pub ingestions: i64,
    pub created_at: String,
    pub updated_at: String,
}

/// `GET /api/ingestion-sets`.
#[derive(Debug, Clone, Serialize, Deserialize)]
pub struct IngestionSetListResponse {
    pub sets: Vec<IngestionSetResponse>,
}

/// One recorded apply of a set.
#[derive(Debug, Clone, Serialize, Deserialize)]
#[serde(rename_all = "camelCase")]
pub struct IngestionSetApplyResponse {
    pub id: String,
    pub set: String,
    pub applied_at: String,
    /// How many items it was given.
    pub items: i64,
    /// How many files they were read from.
    pub files: i64,
    pub changes: Vec<SetChange>,
    #[serde(default)]
    pub applied_by: Option<String>,
    /// The earlier apply it applied again.
    #[serde(default)]
    pub going_back_to: Option<String>,
}

/// `GET /api/ingestion-sets/{name}/applies`.
#[derive(Debug, Clone, Serialize, Deserialize)]
pub struct IngestionSetApplyListResponse {
    pub applies: Vec<IngestionSetApplyResponse>,
}

/// An archived apply: the items and files it was given.
#[derive(Debug, Clone, Serialize, Deserialize)]
#[serde(rename_all = "camelCase")]
pub struct IngestionSetArchiveResponse {
    pub set: String,
    pub apply_id: String,
    pub applied_at: String,
    pub applied_by: String,
    pub items: Vec<IngestionSetItem>,
    pub files: Vec<SetFile>,
    #[serde(default)]
    pub going_back_to: Option<String>,
}

/// `POST /api/ingestion-sets/{name}/release`.
#[derive(Debug, Clone, Serialize, Deserialize)]
pub struct IngestionSetReleaseResponse {
    pub name: String,
    pub released: i64,
}

/// `dryRun=true` only when asked for; the platform's default is a real apply.
fn dry_run_query(dry_run: bool) -> String {
    let params: Vec<(&str, String)> = if dry_run {
        vec![("dryRun", "true".to_string())]
    } else {
        Vec::new()
    };
    FlowCatalystClient::query_string(&params)
}

/// Ingestions resource accessor — created via [`FlowCatalystClient::ingestions`].
#[derive(Debug, Clone, Copy)]
pub struct Ingestions<'a> {
    pub(crate) client: &'a FlowCatalystClient,
}

impl Ingestions<'_> {
    /// List ingestions, one page; pass `next_cursor` back as `after`.
    pub async fn list(
        &self,
        query: &ListIngestionsQuery,
    ) -> Result<IngestionListResponse, ClientError> {
        let mut params = Vec::new();
        if let Some(v) = &query.application_code {
            params.push(("applicationCode", v.clone()));
        }
        if let Some(v) = &query.client_id {
            params.push(("clientId", v.clone()));
        }
        if let Some(v) = &query.q {
            params.push(("q", v.clone()));
        }
        if let Some(v) = &query.status {
            params.push(("status", v.clone()));
        }
        if let Some(v) = &query.kind {
            params.push(("kind", v.clone()));
        }
        if let Some(v) = &query.after {
            params.push(("after", v.clone()));
        }
        if let Some(v) = query.limit {
            params.push(("limit", v.to_string()));
        }
        let query = FlowCatalystClient::query_string(&params);
        self.client.get(&format!("/api/ingestions{query}")).await
    }

    /// Create an ingestion (`201` with the full record). Without the promote
    /// permission (or with `live: Some(false)`) the first version is saved as a
    /// draft: `save.outcome` is `DRAFT` and `save.awaiting_promote` is true.
    pub async fn create(
        &self,
        req: &CreateIngestionRequest,
    ) -> Result<IngestionResponse, ClientError> {
        self.client.post("/api/ingestions", req).await
    }

    /// Get an ingestion by ID.
    pub async fn get(&self, id: &str) -> Result<IngestionResponse, ClientError> {
        self.client
            .get(&format!("/api/ingestions/{}", seg(id)))
            .await
    }

    /// Save a new definition; a stale `expected_version` is a 409. With promote
    /// it is made live (`SAVED`); without, it is a draft awaiting promote
    /// (`DRAFT`). A definition an existing version already says is not saved
    /// again (`UNCHANGED`, `MADE_LIVE`, `DRAFT_EXISTS`); `save` says which.
    pub async fn update(
        &self,
        id: &str,
        req: &UpdateIngestionRequest,
    ) -> Result<IngestionResponse, ClientError> {
        self.client
            .put(&format!("/api/ingestions/{}", seg(id)), req)
            .await
    }

    /// Delete an ingestion (204).
    pub async fn delete(&self, id: &str) -> Result<(), ClientError> {
        self.client
            .delete_req(&format!("/api/ingestions/{}", seg(id)))
            .await
    }

    /// Pause an ingestion.
    pub async fn pause(&self, id: &str) -> Result<IngestionResponse, ClientError> {
        self.client
            .post_action(&format!("/api/ingestions/{}/pause", seg(id)))
            .await
    }

    /// Resume a paused ingestion.
    pub async fn resume(&self, id: &str) -> Result<IngestionResponse, ClientError> {
        self.client
            .post_action(&format!("/api/ingestions/{}/resume", seg(id)))
            .await
    }

    /// List the saved versions of an ingestion.
    pub async fn list_versions(
        &self,
        id: &str,
    ) -> Result<IngestionVersionListResponse, ClientError> {
        self.client
            .get(&format!("/api/ingestions/{}/versions", seg(id)))
            .await
    }

    /// Save a version (`201`); it runs only once it is live, and making it
    /// live needs promote. A definition an existing version already says is
    /// not saved again; `save` on the answer says what happened.
    pub async fn save_version(
        &self,
        id: &str,
        req: &SaveIngestionVersionRequest,
    ) -> Result<IngestionVersionResponse, ClientError> {
        self.client
            .post(&format!("/api/ingestions/{}/versions", seg(id)), req)
            .await
    }

    /// Make a saved version the live one.
    pub async fn make_version_live(
        &self,
        id: &str,
        version: i64,
    ) -> Result<IngestionResponse, ClientError> {
        self.client
            .post_action(&format!(
                "/api/ingestions/{}/versions/{version}/live",
                seg(id)
            ))
            .await
    }

    /// The definition file (YAML) that `fcdev ingest apply` reads back.
    pub async fn definition(&self, id: &str) -> Result<String, ClientError> {
        self.client
            .get_text(&format!("/api/ingestions/{}/definition", seg(id)))
            .await
    }

    /// The operations run on an ingestion, newest first.
    pub async fn list_history(
        &self,
        id: &str,
        limit: Option<u32>,
        offset: Option<u32>,
    ) -> Result<IngestionHistoryResponse, ClientError> {
        let mut params = Vec::new();
        if let Some(v) = limit {
            params.push(("limit", v.to_string()));
        }
        if let Some(v) = offset {
            params.push(("offset", v.to_string()));
        }
        let query = FlowCatalystClient::query_string(&params);
        self.client
            .get(&format!("/api/ingestions/{}/history{query}", seg(id)))
            .await
    }

    /// One page of the failures, newest first; pass `next_cursor` as `after`.
    /// `category: Some("rejected")` with `undismissed: true` is the
    /// ingestion's open notices (what `notices` counts).
    pub async fn list_failures(
        &self,
        id: &str,
        query: &ListIngestionFailuresQuery,
    ) -> Result<IngestionFailureListResponse, ClientError> {
        let mut params = Vec::new();
        if let Some(v) = &query.after {
            params.push(("after", v.clone()));
        }
        if let Some(v) = query.limit {
            params.push(("limit", v.to_string()));
        }
        if let Some(v) = &query.category {
            params.push(("category", v.clone()));
        }
        if query.undismissed {
            params.push(("undismissed", "true".to_string()));
        }
        let query = FlowCatalystClient::query_string(&params);
        self.client
            .get(&format!("/api/ingestions/{}/failures{query}", seg(id)))
            .await
    }

    /// Dismiss a failure: a notice stops showing on the ingestion; the failure
    /// stays listed.
    pub async fn dismiss_failure(
        &self,
        id: &str,
        failure_id: &str,
    ) -> Result<IngestionFailureResponse, ClientError> {
        self.client
            .post_action(&format!(
                "/api/ingestions/{}/failures/{}/dismiss",
                seg(id),
                seg(failure_id)
            ))
            .await
    }

    /// The secret names the live definition binds and, per runtime, when it
    /// last fetched each one. Names only: no secret value is ever returned.
    pub async fn list_secrets(&self, id: &str) -> Result<IngestionSecretListResponse, ClientError> {
        self.client
            .get(&format!("/api/ingestions/{}/secrets", seg(id)))
            .await
    }

    /// The source kinds: the built-in ones and those live runtimes report.
    pub async fn list_source_kinds(&self) -> Result<IngestionSourceKindListResponse, ClientError> {
        self.client.get("/api/ingestion-source-kinds").await
    }

    /// Queue a connection test (`202`); poll [`get_test`](Self::get_test).
    pub async fn test_connection(
        &self,
        req: &TestIngestionRequest,
    ) -> Result<IngestionTestResponse, ClientError> {
        self.client.post("/api/ingestion-tests", req).await
    }

    /// A connection test and its result so far.
    pub async fn get_test(&self, id: &str) -> Result<IngestionTestResponse, ClientError> {
        self.client
            .get(&format!("/api/ingestion-tests/{}", seg(id)))
            .await
    }

    /// Apply a set; with `dry_run` the platform checks and reports the changes
    /// and writes nothing. An ingestion the set owns but no longer lists is deleted.
    pub async fn apply_set(
        &self,
        name: &str,
        req: &ApplyIngestionSetRequest,
        dry_run: bool,
    ) -> Result<IngestionSetPlanResponse, ClientError> {
        self.client
            .put(
                &format!(
                    "/api/ingestion-sets/{}{}",
                    seg(name),
                    dry_run_query(dry_run)
                ),
                req,
            )
            .await
    }

    /// List the sets (needs a caller that reaches all clients and applications).
    pub async fn list_sets(&self) -> Result<IngestionSetListResponse, ClientError> {
        self.client.get("/api/ingestion-sets").await
    }

    /// The recorded applies of a set, newest first.
    pub async fn list_set_applies(
        &self,
        name: &str,
    ) -> Result<IngestionSetApplyListResponse, ClientError> {
        self.client
            .get(&format!("/api/ingestion-sets/{}/applies", seg(name)))
            .await
    }

    /// An archived apply with the items and files it was given.
    pub async fn get_set_apply(
        &self,
        name: &str,
        apply_id: &str,
    ) -> Result<IngestionSetArchiveResponse, ClientError> {
        self.client
            .get(&format!(
                "/api/ingestion-sets/{}/applies/{}",
                seg(name),
                seg(apply_id)
            ))
            .await
    }

    /// Apply an archived apply's items again as a new apply.
    pub async fn reapply_set(
        &self,
        name: &str,
        apply_id: &str,
        dry_run: bool,
    ) -> Result<IngestionSetPlanResponse, ClientError> {
        self.client
            .post_action(&format!(
                "/api/ingestion-sets/{}/applies/{}/reapply{}",
                seg(name),
                seg(apply_id),
                dry_run_query(dry_run)
            ))
            .await
    }

    /// End a set's ownership of its ingestions; nothing else changes.
    pub async fn release_set(
        &self,
        name: &str,
    ) -> Result<IngestionSetReleaseResponse, ClientError> {
        self.client
            .post_action(&format!("/api/ingestion-sets/{}/release", seg(name)))
            .await
    }
}

#[cfg(test)]
mod tests {
    use super::*;
    use crate::client::test_support::MockPlatform;
    use serde_json::json;

    fn target() -> Target {
        Target {
            code: "orders".into(),
            target_url: "https://hooks.example.com/orders".into(),
            pool: "default".into(),
            connection: None,
            service_account: None,
            mode: TargetMode::NextOnError,
            batch_size: 10,
        }
    }

    fn target_json() -> Value {
        json!({"code":"orders","targetUrl":"https://hooks.example.com/orders","pool":"default",
            "mode":"NEXT_ON_ERROR","batchSize":10})
    }

    fn ingestion_json(id: &str) -> String {
        json!({"id":id,"applicationId":"app_1","applicationCode":"erp","clientId":"clt_1",
            "clientIdentifier":"acme","code":"orders","status":"ACTIVE","version":2,
            "latestVersion":3,"createdAt":"t","updatedAt":"t","name":"Orders",
            "source":{"kind":"http","auth":{"kind":"bearer","token":{"name":"orders-token"}}},
            "split":{"kind":"json","arrayPath":"$.items"},"target":target_json()})
        .to_string()
    }

    fn create_request() -> CreateIngestionRequest {
        CreateIngestionRequest {
            application: "erp".into(),
            client: "acme".into(),
            code: "orders".into(),
            name: "Orders".into(),
            source: Source::Http {
                auth: HttpAuth::Bearer {
                    token: SecretRef::new("orders-token"),
                },
                rate_limit: None,
            },
            split: Split::Json {
                array_path: "$.items".into(),
            },
            target: target(),
            keep_failed_payloads: None,
            live: None,
            note: None,
        }
    }

    #[tokio::test]
    async fn create_sends_the_tagged_definition_and_reads_it_back() {
        let stub =
            MockPlatform::start(&[("POST", "/api/ingestions", 201, &ingestion_json("ing_1"))])
                .await;
        let created = stub
            .client()
            .ingestions()
            .create(&create_request())
            .await
            .unwrap();
        assert_eq!(created.id, "ing_1");
        assert_eq!(created.latest_version, 3);
        assert_eq!(created.target.mode, TargetMode::NextOnError);
        assert!(matches!(created.source, Source::Http { .. }));
        let req = stub.single();
        assert_eq!(req.method, "POST");
        assert_eq!(
            req.json(),
            json!({"application":"erp","client":"acme","code":"orders","name":"Orders",
                "source":{"kind":"http","auth":{"kind":"bearer","token":{"name":"orders-token"}}},
                "split":{"kind":"json","arrayPath":"$.items"},"target":target_json()})
        );
    }

    #[tokio::test]
    async fn list_sends_only_the_filters_given_and_follows_the_cursor() {
        let page1 = format!(
            r#"{{"ingestions":[{}],"hasMore":true,"nextCursor":"cur 1"}}"#,
            ingestion_json("ing_1")
        );
        let page2 = format!(
            r#"{{"ingestions":[{}],"hasMore":false}}"#,
            ingestion_json("ing_2")
        );
        let stub = MockPlatform::start(&[("GET", "/api/ingestions", 200, &page1)]).await;
        let c = stub.client();
        let first = c
            .ingestions()
            .list(&ListIngestionsQuery::default())
            .await
            .unwrap();
        assert!(first.has_more);
        assert_eq!(first.next_cursor.as_deref(), Some("cur 1"));
        assert_eq!(stub.requests()[0].query, "");

        c.ingestions()
            .list(&ListIngestionsQuery {
                client_id: Some("acme".into()),
                after: first.next_cursor,
                limit: Some(50),
                ..Default::default()
            })
            .await
            .unwrap();
        assert_eq!(
            stub.requests()[1].query_pairs(),
            vec![
                ("clientId".to_string(), "acme".to_string()),
                ("after".to_string(), "cur 1".to_string()),
                ("limit".to_string(), "50".to_string())
            ]
        );

        let last = MockPlatform::start(&[("GET", "/api/ingestions", 200, &page2)]).await;
        let end = last
            .client()
            .ingestions()
            .list(&ListIngestionsQuery::default())
            .await
            .unwrap();
        assert!(!end.has_more && end.next_cursor.is_none());
    }

    #[tokio::test]
    async fn paths_encode_ids_and_use_the_contract_verbs() {
        let ing = ingestion_json("ing_1");
        let stub = MockPlatform::start(&[
            ("GET", "/api/ingestions/ing%2F1", 200, &ing),
            ("PUT", "/api/ingestions/ing_1", 200, &ing),
            ("DELETE", "/api/ingestions/ing_1", 204, ""),
            ("POST", "/api/ingestions/ing_1/pause", 200, &ing),
            ("POST", "/api/ingestions/ing_1/resume", 200, &ing),
            ("POST", "/api/ingestions/ing_1/versions/3/live", 200, &ing),
        ])
        .await;
        let c = stub.client();
        let i = c.ingestions();
        i.get("ing/1").await.unwrap();
        i.update(
            "ing_1",
            &UpdateIngestionRequest {
                expected_version: 2,
                name: "Orders".into(),
                source: Source::Http {
                    auth: HttpAuth::Bearer {
                        token: SecretRef::new("orders-token"),
                    },
                    rate_limit: None,
                },
                split: Split::None,
                target: target(),
                keep_failed_payloads: Some(true),
                note: None,
            },
        )
        .await
        .unwrap();
        i.delete("ing_1").await.unwrap();
        i.pause("ing_1").await.unwrap();
        i.resume("ing_1").await.unwrap();
        i.make_version_live("ing_1", 3).await.unwrap();
        let sent: Vec<_> = stub
            .requests()
            .iter()
            .map(|r| (r.method.clone(), r.path.clone()))
            .collect();
        assert_eq!(sent.len(), 6);
        assert_eq!(sent[0], ("GET".into(), "/api/ingestions/ing%2F1".into()));
        assert_eq!(stub.requests()[1].json()["expectedVersion"], 2);
        assert_eq!(stub.requests()[1].json()["keepFailedPayloads"], true);
        assert!(stub.requests().iter().all(|r| r.query.is_empty()));
    }

    #[tokio::test]
    async fn versions_save_and_list() {
        let version = json!({"id":"ver_1","version":3,"live":false,"createdAt":"t","name":"Orders",
            "source":{"kind":"custom","type":"mix","settings":{"password":"mix-pass"}},
            "split":{"kind":"none"},"target":target_json()})
        .to_string();
        let list = format!(r#"{{"versions":[{version}]}}"#);
        let stub = MockPlatform::start(&[
            ("GET", "/api/ingestions/ing_1/versions", 200, &list),
            ("POST", "/api/ingestions/ing_1/versions", 201, &version),
        ])
        .await;
        let c = stub.client();
        let listed = c.ingestions().list_versions("ing_1").await.unwrap();
        assert!(
            matches!(&listed.versions[0].source, Source::Custom { kind_type, .. } if kind_type == "mix")
        );
        c.ingestions()
            .save_version(
                "ing_1",
                &SaveIngestionVersionRequest {
                    name: "Orders".into(),
                    source: listed.versions[0].source.clone(),
                    split: Split::None,
                    target: target(),
                    keep_failed_payloads: None,
                    live: Some(true),
                    note: Some("promote".into()),
                },
            )
            .await
            .unwrap();
        let body = stub.requests()[1].json();
        assert_eq!(body["live"], true);
        assert_eq!(body["note"], "promote");
        assert_eq!(
            body["source"],
            json!({"kind":"custom","type":"mix","settings":{"password":"mix-pass"}})
        );
    }

    #[tokio::test]
    async fn definition_is_returned_as_text_and_history_failures_page() {
        let yaml = "application: erp\nclient: acme\ncode: orders\n";
        let history = r#"{"entries":[{"id":"h1","operation":"CreateIngestion","performedAt":"t",
            "details":{"code":"orders"}}],"hasMore":false}"#;
        let failures = r#"{"failures":[{"id":"f1","kind":"SOURCE","message":"boom","afterRecords":3,
            "occurredAt":"t","version":2}],"hasMore":true,"nextCursor":"f1"}"#;
        let stub = MockPlatform::start(&[
            ("GET", "/api/ingestions/ing_1/definition", 200, yaml),
            ("GET", "/api/ingestions/ing_1/history", 200, history),
            ("GET", "/api/ingestions/ing_1/failures", 200, failures),
        ])
        .await;
        let c = stub.client();
        assert_eq!(c.ingestions().definition("ing_1").await.unwrap(), yaml);
        let h = c
            .ingestions()
            .list_history("ing_1", Some(20), Some(40))
            .await
            .unwrap();
        assert_eq!(h.entries[0].operation, "CreateIngestion");
        assert_eq!(
            stub.requests()[1].query_pairs(),
            vec![
                ("limit".to_string(), "20".to_string()),
                ("offset".to_string(), "40".to_string())
            ]
        );
        let f = c
            .ingestions()
            .list_failures("ing_1", &ListIngestionFailuresQuery::default())
            .await
            .unwrap();
        assert_eq!(f.failures[0].after_records, 3);
        assert_eq!(f.next_cursor.as_deref(), Some("f1"));
        assert_eq!(stub.requests()[2].query, "");
        c.ingestions()
            .list_failures(
                "ing_1",
                &ListIngestionFailuresQuery {
                    after: f.next_cursor.clone(),
                    limit: Some(5),
                    category: Some("rejected".into()),
                    undismissed: true,
                },
            )
            .await
            .unwrap();
        assert_eq!(
            stub.requests()[3].query_pairs(),
            vec![
                ("after".to_string(), "f1".to_string()),
                ("limit".to_string(), "5".to_string()),
                ("category".to_string(), "rejected".to_string()),
                ("undismissed".to_string(), "true".to_string())
            ]
        );
    }

    #[tokio::test]
    async fn dismiss_failure_and_secrets() {
        let failure = r#"{"id":"igf_1","kind":"JOBS_REJECTED","category":"rejected","message":"refused",
            "afterRecords":0,"occurredAt":"t","createdAt":"t","dismissedAt":"t2","dismissedBy":"prn_1"}"#;
        let secrets = r#"{"secrets":[{"name":"orders-token","bound":true,"fetches":[{"runtimeId":"rt-1",
            "version":3,"principalId":"prn_rt","lastFetchedAt":"t","fetchCount":7}]}]}"#;
        let stub = MockPlatform::start(&[
            (
                "POST",
                "/api/ingestions/ing_1/failures/igf_1/dismiss",
                200,
                failure,
            ),
            ("GET", "/api/ingestions/ing_1/secrets", 200, secrets),
        ])
        .await;
        let c = stub.client();
        let f = c
            .ingestions()
            .dismiss_failure("ing_1", "igf_1")
            .await
            .unwrap();
        assert_eq!(f.category, "rejected");
        assert_eq!(f.dismissed_by.as_deref(), Some("prn_1"));
        assert_eq!(stub.requests()[0].method, "POST");
        let s = c.ingestions().list_secrets("ing_1").await.unwrap();
        assert_eq!(s.secrets[0].name, "orders-token");
        assert!(s.secrets[0].bound);
        assert_eq!(s.secrets[0].fetches[0].fetch_count, 7);
    }

    #[tokio::test]
    async fn a_create_without_promote_answers_a_draft() {
        let mut answer: Value = serde_json::from_str(&ingestion_json("ing_1")).unwrap();
        let extra = json!({"notices":2,"latestNotice":"jobs refused","latestNoticeAt":"t",
            "warnings":["no host key"],
            "save":{"outcome":"DRAFT","version":1,"awaitingPromote":true,"message":"saved as a draft"}});
        answer
            .as_object_mut()
            .unwrap()
            .extend(extra.as_object().unwrap().clone());
        let stub =
            MockPlatform::start(&[("POST", "/api/ingestions", 201, &answer.to_string())]).await;
        let created = stub
            .client()
            .ingestions()
            .create(&create_request())
            .await
            .unwrap();
        let save = created.save.unwrap();
        assert_eq!(save.outcome, "DRAFT");
        assert!(save.awaiting_promote);
        assert_eq!(created.notices, 2);
        assert_eq!(created.warnings, vec!["no host key".to_string()]);
        // an answer without the new fields still reads
        let old: IngestionResponse = serde_json::from_str(&ingestion_json("ing_2")).unwrap();
        assert_eq!(old.notices, 0);
        assert!(old.save.is_none());
        let version: IngestionVersionResponse = serde_json::from_value(json!({"id":"igv_1",
            "version":1,"live":false,"draft":true,"digest":"ab12","createdAt":"t","name":"n",
            "source":{"kind":"sqs","region":"r","queue":"q"},"split":{"kind":"none"},
            "target":target_json()}))
        .unwrap();
        assert!(version.draft);
        assert_eq!(version.digest.as_deref(), Some("ab12"));
    }

    #[tokio::test]
    async fn source_kinds_and_tests() {
        let kinds = r#"{"kinds":[{"kind":"http","builtIn":true,"singleOwner":false},
            {"kind":"camel-route","builtIn":false,"singleOwner":true,
             "requiredPermission":"platform:ingestion:ingestion:route","settings":{"yaml":"string"}}]}"#;
        let test = r#"{"id":"itt_1","kind":"http","status":"PENDING","createdAt":"t","expiresAt":"t",
            "notice":"no runtime carries http"}"#;
        let done = r#"{"id":"itt_1","kind":"http","status":"DONE","createdAt":"t","expiresAt":"t",
            "outcome":"RESULTS","records":4,"steps":["connected","read"]}"#;
        let stub = MockPlatform::start(&[
            ("GET", "/api/ingestion-source-kinds", 200, kinds),
            ("POST", "/api/ingestion-tests", 202, test),
            ("GET", "/api/ingestion-tests/itt_1", 200, done),
        ])
        .await;
        let c = stub.client();
        let k = c.ingestions().list_source_kinds().await.unwrap();
        assert_eq!(
            k.kinds[1].required_permission.as_deref(),
            Some("platform:ingestion:ingestion:route")
        );
        let queued = c
            .ingestions()
            .test_connection(&TestIngestionRequest {
                ingestion_id: Some("ing_1".into()),
                ..Default::default()
            })
            .await
            .unwrap();
        assert_eq!(queued.status, "PENDING");
        assert_eq!(queued.notice.as_deref(), Some("no runtime carries http"));
        assert_eq!(stub.requests()[1].json(), json!({"ingestionId":"ing_1"}));
        let got = c.ingestions().get_test("itt_1").await.unwrap();
        assert_eq!(got.outcome.as_deref(), Some("RESULTS"));
        assert_eq!(got.records, Some(4));
        assert_eq!(got.steps.len(), 2);
    }

    #[tokio::test]
    async fn sets_send_dry_run_only_when_asked() {
        let plan = r#"{"name":"inhance","dryRun":true,"changes":[
            {"application":"erp","client":"acme","code":"orders","action":"CREATE","changed":[]}]}"#;
        let archive = r#"{"set":"inhance","applyId":"sap_1","appliedAt":"t","appliedBy":"me",
            "items":[],"files":[{"path":"a.yaml","content":"x"}]}"#;
        let stub = MockPlatform::start(&[
            ("PUT", "/api/ingestion-sets/in%20hance", 200, plan),
            ("GET", "/api/ingestion-sets", 200, r#"{"sets":[{"id":"set_1","name":"inhance","ingestions":2,"createdAt":"t","updatedAt":"t"}]}"#),
            ("GET", "/api/ingestion-sets/inhance/applies", 200, r#"{"applies":[{"id":"sap_1","set":"inhance","appliedAt":"t","items":1,"files":1,"changes":[],"goingBackTo":"sap_0"}]}"#),
            ("GET", "/api/ingestion-sets/inhance/applies/sap_1", 200, archive),
            ("POST", "/api/ingestion-sets/inhance/applies/sap_1/reapply", 200, plan),
            ("POST", "/api/ingestion-sets/inhance/release", 200, r#"{"name":"inhance","released":2}"#),
        ])
        .await;
        let c = stub.client();
        let i = c.ingestions();
        let req = ApplyIngestionSetRequest::default();
        i.apply_set("in hance", &req, false).await.unwrap();
        let planned = i.apply_set("in hance", &req, true).await.unwrap();
        assert_eq!(planned.changes[0].action, "CREATE");
        assert!(planned.apply_id.is_none());
        assert_eq!(stub.requests()[0].query, "");
        assert_eq!(stub.requests()[1].query, "dryRun=true");
        assert_eq!(stub.requests()[0].json(), json!({"items":[]}));
        assert_eq!(i.list_sets().await.unwrap().sets[0].ingestions, 2);
        let applies = i.list_set_applies("inhance").await.unwrap();
        assert_eq!(applies.applies[0].going_back_to.as_deref(), Some("sap_0"));
        assert_eq!(
            i.get_set_apply("inhance", "sap_1").await.unwrap().files[0].path,
            "a.yaml"
        );
        i.reapply_set("inhance", "sap_1", false).await.unwrap();
        i.reapply_set("inhance", "sap_1", true).await.unwrap();
        assert_eq!(stub.requests()[5].query, "");
        assert_eq!(stub.requests()[6].query, "dryRun=true");
        assert_eq!(i.release_set("inhance").await.unwrap().released, 2);
    }

    #[tokio::test]
    async fn error_statuses_carry_the_platform_codes() {
        for (status, code) in [
            (400, "VALIDATION_ERROR"),
            (403, "FORBIDDEN"),
            (404, "INGESTION_NOT_FOUND"),
            (409, "INGESTION_CODE_EXISTS"),
        ] {
            let body = format!(r#"{{"error":"{code}","message":"no"}}"#);
            let stub = MockPlatform::start(&[("POST", "/api/ingestions", status, &body)]).await;
            let err = stub
                .client()
                .ingestions()
                .create(&create_request())
                .await
                .unwrap_err();
            assert_eq!(err.status(), Some(status));
            match &err {
                ClientError::Api { code: got, .. } => assert_eq!(got.as_deref(), Some(code)),
                other => panic!("expected an API error, got {other:?}"),
            }
            assert_eq!(err.is_conflict(), status == 409);
            assert_eq!(err.is_not_found(), status == 404);
        }
    }

    #[test]
    fn every_source_and_split_kind_round_trips_on_the_wire() {
        let secret = json!({"name":"s"});
        let sources = vec![
            json!({"kind":"http","auth":{"kind":"flowcatalyst-token","application":"erp","principalId":"prn_1"},
                "rateLimit":{"requests":100,"per":"PT1M"}}),
            json!({"kind":"http","auth":{"kind":"basic","username":"u","password":secret}}),
            json!({"kind":"sftp","host":"h","port":22,"directory":"/in","username":"u","credential":secret,
                "include":"*.csv","every":"PT5M"}),
            json!({"kind":"sftp","host":"h","port":22,"directory":"/in","username":"u","credential":secret,
                "acceptAnyHostKey":true,"every":"PT5M"}),
            json!({"kind":"s3","region":"eu-west-1","bucket":"b","prefix":"in/","every":"PT5M",
                "credential":{"accessKeyId":"AK","secretAccessKey":secret}}),
            json!({"kind":"sqs","region":"eu-west-1","queue":"q"}),
            json!({"kind":"rabbitmq","uri":"amqps://mq","queue":"q","username":"u","password":secret}),
            json!({"kind":"database-poll","jdbcUrl":"jdbc:postgresql://db/x","username":"u","password":secret,
                "table":"t","track":{"kind":"status","column":"s","keyColumn":"id","pending":"N","processed":"Y"},
                "maxRows":100,"every":"PT1M"}),
            json!({"kind":"database-poll","jdbcUrl":"j","username":"u","password":secret,"table":"t",
                "track":{"kind":"watermark","column":"c","type":"TIMESTAMP"},"maxRows":1,"every":"PT1M"}),
            json!({"kind":"api-poll","url":"https://api/x","method":"GET","headers":[{"name":"A","value":"b"}],
                "auth":{"kind":"oauth-client-credentials","tokenUrl":"https://t","clientId":"c","clientSecret":secret,
                    "credentialsIn":"BODY"},
                "results":{"kind":"header","name":"X-More","equals":"yes"},
                "cursor":{"from":"$.next","param":"after"},"every":"PT1M","idleDelay":"PT5M"}),
            json!({"kind":"api-poll","url":"https://api/x","method":"POST","body":"{}",
                "auth":{"kind":"login","url":"https://l","format":"JSON","usernameField":"u","passwordField":"p",
                    "username":"me","password":secret,"extraFields":{"tenant":"t"},"tokenPath":"$.token",
                    "lifetime":"PT1H","header":"Authorization","prefix":"Bearer "},
                "results":{"kind":"json-path","path":"$.items"},"every":"PT1M","idleDelay":"PT5M"}),
            json!({"kind":"api-poll","url":"https://api/x","method":"GET","auth":{"kind":"none"},
                "results":{"kind":"records"},"every":"PT1M","idleDelay":"PT5M"}),
            json!({"kind":"api-poll","url":"https://api/x","method":"GET",
                "auth":{"kind":"api-key","header":"X-Key","value":secret},
                "results":{"kind":"records"},"every":"PT1M","idleDelay":"PT5M"}),
            json!({"kind":"custom","type":"camel-route","settings":{"yaml":"- from: {}"}}),
        ];
        for wire in sources {
            let typed: Source = serde_json::from_value(wire.clone()).unwrap();
            assert_eq!(serde_json::to_value(&typed).unwrap(), wire);
        }
        for wire in [
            json!({"kind":"none"}),
            json!({"kind":"csv","header":true}),
            json!({"kind":"json","arrayPath":"$"}),
            json!({"kind":"xml","element":"order"}),
        ] {
            let typed: Split = serde_json::from_value(wire.clone()).unwrap();
            assert_eq!(serde_json::to_value(&typed).unwrap(), wire);
        }
    }

    #[test]
    fn a_definition_with_a_connection_target_round_trips() {
        let wire = json!({"name":"Orders","source":{"kind":"sqs","region":"r","queue":"q"},
            "split":{"kind":"none"},
            "target":{"code":"c","targetUrl":"https://x","pool":"p","connection":"billing",
                "mode":"BLOCK_ON_ERROR","batchSize":1},
            "keepFailedPayloads":true});
        let def: IngestionDefinition = serde_json::from_value(wire.clone()).unwrap();
        assert_eq!(def.target.connection.as_deref(), Some("billing"));
        assert_eq!(def.target.mode, TargetMode::BlockOnError);
        assert_eq!(serde_json::to_value(&def).unwrap(), wire);
        assert_eq!(
            serde_json::to_value(TargetMode::Immediate).unwrap(),
            json!("IMMEDIATE")
        );
    }
}
