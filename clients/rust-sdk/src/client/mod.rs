//! The FlowCatalyst platform HTTP API client.
//!
//! Operations are grouped by resource behind accessor methods:
//!
//! ```no_run
//! use flowcatalyst_sdk::client::{ClientCredentials, FlowCatalystClient};
//!
//! # async fn demo() -> Result<(), flowcatalyst_sdk::client::ClientError> {
//! let client = FlowCatalystClient::new("https://platform.example.com")
//!     .with_token_source(ClientCredentials::new(
//!         "https://platform.example.com",
//!         "svc-orders",
//!         "secret",
//!     ));
//! let order_created = client.event_types().get_by_code("orders:sales:order:created").await?;
//! # Ok(()) }
//! ```
//!
//! Transport (the Java SDK's rules): `Accept: application/json` on every
//! request, `Content-Type` only with a body; a request answered `408`, `429`,
//! `502`, `503` or `504`, or failing to connect, is retried with exponential
//! backoff (default 3 retries from 100 ms); a `401` refreshes a
//! client-credentials token once; any other non-2xx is a
//! [`ClientError::Api`] carrying the platform's `{error, message}` envelope.

pub mod applications;
pub mod audit_logs;
pub mod clients;
pub mod connections;
pub mod dispatch_pools;
pub mod docs;
pub mod event_types;
pub mod me;
pub mod openapi;
pub mod permissions;
pub mod principals;
pub mod processes;
pub mod roles;
pub mod router;
pub mod scheduled_jobs;
pub mod service_accounts;
pub mod subscriptions;
pub mod token;

#[cfg(test)]
pub(crate) mod test_support;

use std::fmt;
use std::sync::Arc;
use std::time::Duration;

use reqwest::header::{ACCEPT, AUTHORIZATION, CONTENT_TYPE, HeaderValue};
use reqwest::{Method, StatusCode};
use serde::de::DeserializeOwned;
use serde::{Deserialize, Serialize};

pub use token::{ClientCredentials, StaticToken, TokenError, TokenSource};

/// Result body returned by every per-resource sync endpoint.
#[derive(Debug, Clone, Serialize, Deserialize)]
#[serde(rename_all = "camelCase")]
pub struct SyncResult {
    pub application_code: String,
    pub created: u32,
    pub updated: u32,
    pub deleted: u32,
    pub synced_codes: Vec<String>,
    /// Legacy field: the Go platform never sends it (it applies a
    /// `passwordHash` to existing users too), so it is always empty against
    /// Go. Kept for compatibility with platforms that report it.
    #[serde(default, skip_serializing_if = "Vec::is_empty")]
    pub password_hash_ignored: Vec<String>,
}

// Re-export request/response DTOs at the module root.
pub use applications::*;
pub use audit_logs::*;
pub use clients::*;
pub use connections::*;
pub use dispatch_pools::*;
pub use docs::*;
pub use event_types::*;
pub use me::*;
pub use openapi::*;
pub use permissions::*;
pub use principals::*;
pub use processes::*;
pub use roles::*;
pub use router::*;
pub use scheduled_jobs::*;
pub use service_accounts::*;
pub use subscriptions::*;

/// Percent-encode one path segment (Java SDK `Transport.enc`): everything but
/// the RFC 3986 unreserved characters and `:` / `@` (legal in a segment, and
/// in every platform code) is escaped, so a caller-supplied code or name
/// containing `/`, `?`, `#`, `%` or a space cannot change the route.
pub(crate) fn seg<S: AsRef<str> + ?Sized>(value: &S) -> String {
    let mut out = String::new();
    for b in value.as_ref().bytes() {
        if b.is_ascii_alphanumeric() || matches!(b, b'-' | b'.' | b'_' | b'~' | b':' | b'@') {
            out.push(char::from(b));
        } else {
            out.push_str(&format!("%{b:02X}"));
        }
    }
    out
}

/// Statuses retried with backoff (Java SDK `Transport.RETRYABLE`).
const RETRYABLE: [u16; 5] = [408, 429, 502, 503, 504];

/// HTTP client for the FlowCatalyst platform API. Cheap to clone.
#[derive(Clone)]
pub struct FlowCatalystClient {
    base_url: String,
    pub(crate) http: reqwest::Client,
    tokens: Option<Arc<dyn TokenSource>>,
    /// The message router's base URL for its monitoring endpoints (a separate
    /// process, usually its own host). `None`: the platform's base URL.
    pub(crate) router_base_url: Option<String>,
    timeout: Duration,
    retry_attempts: u32,
    retry_delay: Duration,
}

impl fmt::Debug for FlowCatalystClient {
    fn fmt(&self, f: &mut fmt::Formatter<'_>) -> fmt::Result {
        f.debug_struct("FlowCatalystClient")
            .field("base_url", &self.base_url)
            .field("router_base_url", &self.router_base_url)
            .field("tokens", &self.tokens)
            .field("timeout", &self.timeout)
            .field("retry_attempts", &self.retry_attempts)
            .finish()
    }
}

impl FlowCatalystClient {
    /// A client for the platform at `base_url` (no authentication yet).
    pub fn new(base_url: impl Into<String>) -> Self {
        Self {
            base_url: base_url.into().trim_end_matches('/').to_string(),
            http: reqwest::Client::new(),
            tokens: None,
            router_base_url: None,
            timeout: Duration::from_secs(30),
            retry_attempts: 3,
            retry_delay: Duration::from_millis(100),
        }
    }

    /// Authenticate with a fixed bearer token.
    pub fn with_token(self, token: impl Into<String>) -> Self {
        self.with_token_source(StaticToken::new(token))
    }

    /// Authenticate with a token source, e.g. [`ClientCredentials`].
    pub fn with_token_source(mut self, source: impl TokenSource + 'static) -> Self {
        self.tokens = Some(Arc::new(source));
        self
    }

    /// Use this HTTP client (custom TLS, proxies, connection pool).
    pub fn with_http_client(mut self, client: reqwest::Client) -> Self {
        self.http = client;
        self
    }

    /// Per-attempt timeout (default 30 s).
    pub fn with_timeout(mut self, timeout: Duration) -> Self {
        self.timeout = timeout;
        self
    }

    /// Retries after the first attempt for the retryable statuses and for
    /// connection failures, and the base delay (doubled each retry). Default
    /// 3 and 100 ms; `0` disables retrying.
    pub fn with_retry(mut self, retry_attempts: u32, base_delay: Duration) -> Self {
        self.retry_attempts = retry_attempts;
        self.retry_delay = base_delay;
        self
    }

    /// The message router's base URL for `/monitoring/in-flight-messages/*`.
    pub fn with_router_url(mut self, url: impl Into<String>) -> Self {
        self.router_base_url = Some(url.into().trim_end_matches('/').to_string());
        self
    }

    /// The platform base URL.
    pub fn base_url(&self) -> &str {
        &self.base_url
    }

    // ── Resource accessors ──────────────────────────────────────────────

    /// Applications — `/api/applications/*`.
    pub fn applications(&self) -> applications::Applications<'_> {
        applications::Applications { client: self }
    }

    /// Audit logs — `/api/audit-logs/*`.
    pub fn audit_logs(&self) -> audit_logs::AuditLogs<'_> {
        audit_logs::AuditLogs { client: self }
    }

    /// Clients (tenants) — `/api/clients/*`.
    pub fn clients(&self) -> clients::Clients<'_> {
        clients::Clients { client: self }
    }

    /// Connections — `/api/connections/*`.
    pub fn connections(&self) -> connections::Connections<'_> {
        connections::Connections { client: self }
    }

    /// Dispatch pools — `/api/dispatch-pools/*`.
    pub fn dispatch_pools(&self) -> dispatch_pools::DispatchPools<'_> {
        dispatch_pools::DispatchPools { client: self }
    }

    /// Event types — `/api/event-types/*`.
    pub fn event_types(&self) -> event_types::EventTypes<'_> {
        event_types::EventTypes { client: self }
    }

    /// Current user context — `/api/me/*`.
    pub fn me(&self) -> me::Me<'_> {
        me::Me { client: self }
    }

    /// Permissions catalogue — `/api/roles/permissions/*`.
    pub fn permissions(&self) -> permissions::Permissions<'_> {
        permissions::Permissions { client: self }
    }

    /// Principals (users + service accounts) — `/api/principals/*`.
    pub fn principals(&self) -> principals::Principals<'_> {
        principals::Principals { client: self }
    }

    /// Process documentation — `/api/processes/*`.
    pub fn processes(&self) -> processes::Processes<'_> {
        processes::Processes { client: self }
    }

    /// OpenAPI specs — `/api/applications/{appCode}/openapi/sync`.
    pub fn openapi(&self) -> openapi::OpenApi<'_> {
        openapi::OpenApi { client: self }
    }

    /// Application documentation pages — `/api/applications/{appCode}/docs/sync`.
    pub fn docs(&self) -> docs::Docs<'_> {
        docs::Docs { client: self }
    }

    /// Roles — `/api/roles/*`.
    pub fn roles(&self) -> roles::Roles<'_> {
        roles::Roles { client: self }
    }

    /// Message-router monitoring — `/monitoring/in-flight-messages/*`.
    pub fn router(&self) -> router::Router<'_> {
        router::Router { client: self }
    }

    /// Scheduled jobs — `/api/scheduled-jobs/*`.
    pub fn scheduled_jobs(&self) -> scheduled_jobs::ScheduledJobs<'_> {
        scheduled_jobs::ScheduledJobs { client: self }
    }

    /// Service accounts — `/api/service-accounts/*`.
    pub fn service_accounts(&self) -> service_accounts::ServiceAccounts<'_> {
        service_accounts::ServiceAccounts { client: self }
    }

    /// Subscriptions — `/api/subscriptions/*`.
    pub fn subscriptions(&self) -> subscriptions::Subscriptions<'_> {
        subscriptions::Subscriptions { client: self }
    }

    // ── Transport ───────────────────────────────────────────────────────

    pub(crate) fn url(&self, path: &str) -> String {
        format!("{}{}", self.base_url, path)
    }

    /// Build a `?k=v&...` query string with every value form-encoded (an
    /// email's `+` or a CSV's `,` survive intact). Empty when `params` is.
    pub(crate) fn query_string(params: &[(&str, String)]) -> String {
        if params.is_empty() {
            return String::new();
        }
        let encoded: String = url::form_urlencoded::Serializer::new(String::new())
            .extend_pairs(params.iter().map(|(k, v)| (*k, v.as_str())))
            .finish();
        format!("?{encoded}")
    }

    /// One request with auth, retry, the one-shot 401 refresh and error
    /// mapping; returns the successful response's status and body.
    pub(crate) async fn send(
        &self,
        method: Method,
        url: &str,
        body: Option<&[u8]>,
    ) -> Result<(StatusCode, bytes_body::Body), ClientError> {
        let mut token = match &self.tokens {
            Some(source) => Some(source.token().await?),
            None => None,
        };
        let mut refreshed = false;
        let mut attempt: u32 = 0;
        loop {
            let mut req = self
                .http
                .request(method.clone(), url)
                .timeout(self.timeout)
                .header(ACCEPT, HeaderValue::from_static("application/json"));
            if let Some(t) = &token {
                let value = HeaderValue::from_str(&format!("Bearer {t}"))
                    .map_err(|_| ClientError::InvalidToken)?;
                req = req.header(AUTHORIZATION, value);
            }
            if let Some(b) = body {
                req = req
                    .header(CONTENT_TYPE, HeaderValue::from_static("application/json"))
                    .body(b.to_vec());
            }
            let resp = match req.send().await {
                Ok(resp) => resp,
                Err(err) => {
                    if (err.is_connect() || err.is_timeout()) && attempt < self.retry_attempts {
                        self.backoff(attempt).await;
                        attempt += 1;
                        continue;
                    }
                    return Err(ClientError::Request(err));
                }
            };
            let status = resp.status();
            let text = resp.bytes().await.map_err(ClientError::Request)?;
            if status.is_success() {
                return Ok((status, bytes_body::Body(text.to_vec())));
            }
            if status == StatusCode::UNAUTHORIZED
                && !refreshed
                && let Some(source) = &self.tokens
                && source.invalidate().await
            {
                refreshed = true;
                token = Some(source.token().await?);
                continue;
            }
            if RETRYABLE.contains(&status.as_u16()) && attempt < self.retry_attempts {
                self.backoff(attempt).await;
                attempt += 1;
                continue;
            }
            return Err(ClientError::api(
                status.as_u16(),
                String::from_utf8_lossy(&text).into_owned(),
            ));
        }
    }

    async fn backoff(&self, attempt: u32) {
        let factor = 1u32.checked_shl(attempt).unwrap_or(u32::MAX);
        tokio::time::sleep(self.retry_delay.saturating_mul(factor)).await;
    }

    async fn call<B: Serialize + ?Sized, T: DeserializeOwned>(
        &self,
        method: Method,
        url: &str,
        body: Option<&B>,
    ) -> Result<T, ClientError> {
        let encoded = body
            .map(serde_json::to_vec)
            .transpose()
            .map_err(ClientError::Encode)?;
        let (status, body) = self.send(method, url, encoded.as_deref()).await?;
        body.decode(status)
    }

    async fn call_no_content<B: Serialize + ?Sized>(
        &self,
        method: Method,
        url: &str,
        body: Option<&B>,
    ) -> Result<(), ClientError> {
        let encoded = body
            .map(serde_json::to_vec)
            .transpose()
            .map_err(ClientError::Encode)?;
        self.send(method, url, encoded.as_deref()).await.map(|_| ())
    }

    pub(crate) async fn get<T: DeserializeOwned>(&self, path: &str) -> Result<T, ClientError> {
        self.call::<(), T>(Method::GET, &self.url(path), None).await
    }

    /// GET against an absolute URL (the router).
    pub(crate) async fn get_absolute<T: DeserializeOwned>(
        &self,
        url: &str,
    ) -> Result<T, ClientError> {
        self.call::<(), T>(Method::GET, url, None).await
    }

    /// POST against an absolute URL (the router).
    pub(crate) async fn post_absolute<B: Serialize + ?Sized, T: DeserializeOwned>(
        &self,
        url: &str,
        body: &B,
    ) -> Result<T, ClientError> {
        self.call(Method::POST, url, Some(body)).await
    }

    pub(crate) async fn post<B: Serialize + ?Sized, T: DeserializeOwned>(
        &self,
        path: &str,
        body: &B,
    ) -> Result<T, ClientError> {
        self.call(Method::POST, &self.url(path), Some(body)).await
    }

    pub(crate) async fn put<B: Serialize + ?Sized, T: DeserializeOwned>(
        &self,
        path: &str,
        body: &B,
    ) -> Result<T, ClientError> {
        self.call(Method::PUT, &self.url(path), Some(body)).await
    }

    pub(crate) async fn delete_req(&self, path: &str) -> Result<(), ClientError> {
        self.call_no_content::<()>(Method::DELETE, &self.url(path), None)
            .await
    }

    /// DELETE that returns a parsed response body (e.g. the updated resource).
    pub(crate) async fn delete_with_response<T: DeserializeOwned>(
        &self,
        path: &str,
    ) -> Result<T, ClientError> {
        self.call::<(), T>(Method::DELETE, &self.url(path), None)
            .await
    }

    /// POST without a body, parsing the response.
    pub(crate) async fn post_action<T: DeserializeOwned>(
        &self,
        path: &str,
    ) -> Result<T, ClientError> {
        self.call::<(), T>(Method::POST, &self.url(path), None)
            .await
    }

    /// POST without a body, discarding the response.
    pub(crate) async fn post_empty(&self, path: &str) -> Result<(), ClientError> {
        self.call_no_content::<()>(Method::POST, &self.url(path), None)
            .await
    }

    /// POST with a JSON body, discarding the response body (204 endpoints).
    pub(crate) async fn post_no_content<B: Serialize + ?Sized>(
        &self,
        path: &str,
        body: &B,
    ) -> Result<(), ClientError> {
        self.call_no_content(Method::POST, &self.url(path), Some(body))
            .await
    }

    /// PUT with a JSON body, discarding the response body (204 endpoints).
    pub(crate) async fn put_empty<B: Serialize + ?Sized>(
        &self,
        path: &str,
        body: &B,
    ) -> Result<(), ClientError> {
        self.call_no_content(Method::PUT, &self.url(path), Some(body))
            .await
    }
}

mod bytes_body {
    use super::{ClientError, StatusCode};
    use serde::de::DeserializeOwned;

    /// A successful response's body.
    #[derive(Debug)]
    pub(crate) struct Body(pub(crate) Vec<u8>);

    impl Body {
        /// Decode JSON; an empty body or a `204` decodes as JSON `null`, so
        /// `()` and `Option<T>` accept it.
        pub(crate) fn decode<T: DeserializeOwned>(
            self,
            status: StatusCode,
        ) -> Result<T, ClientError> {
            let bytes: &[u8] =
                if self.0.iter().all(u8::is_ascii_whitespace) || status == StatusCode::NO_CONTENT {
                    b"null"
                } else {
                    &self.0
                };
            serde_json::from_slice(bytes).map_err(|source| ClientError::Decode {
                status: status.as_u16(),
                source,
                body: String::from_utf8_lossy(&self.0)
                    .chars()
                    .take(2000)
                    .collect(),
            })
        }
    }
}

/// The platform's error envelope: `{"error": "<CODE>", "message": "…"}`
/// (OAuth endpoints: `error` / `error_description`).
#[derive(Debug, Default, Deserialize)]
struct Envelope {
    error: Option<String>,
    code: Option<String>,
    message: Option<String>,
    error_description: Option<String>,
}

/// Errors from platform calls.
#[derive(Debug, thiserror::Error)]
#[non_exhaustive]
pub enum ClientError {
    /// The request could not be sent or the response not read.
    #[error("HTTP request failed: {0}")]
    Request(#[source] reqwest::Error),

    /// The platform answered with a non-2xx status.
    #[error("API error (HTTP {status}){}{}", code.as_deref().map(|c| format!(" {c}")).unwrap_or_default(), message.as_deref().map(|m| format!(": {m}")).unwrap_or_default())]
    Api {
        status: u16,
        /// The envelope's `error` code (e.g. `VALIDATION_ERROR`), when present.
        code: Option<String>,
        /// The envelope's `message`, when present.
        message: Option<String>,
        /// The raw body.
        body: String,
    },

    /// A 2xx body that is not the expected JSON.
    #[error("unexpected response body (HTTP {status}): {source}")]
    Decode {
        status: u16,
        #[source]
        source: serde_json::Error,
        body: String,
    },

    /// A request body could not be serialised.
    #[error("request body serialisation failed: {0}")]
    Encode(#[source] serde_json::Error),

    /// The token source failed.
    #[error(transparent)]
    Token(#[from] TokenError),

    /// The token contains characters not allowed in a header.
    #[error("the bearer token is not a valid header value")]
    InvalidToken,
}

impl ClientError {
    pub(crate) fn api(status: u16, body: String) -> Self {
        let env: Envelope = serde_json::from_str(&body).unwrap_or_default();
        ClientError::Api {
            status,
            code: env.error.or(env.code),
            message: env.message.or(env.error_description),
            body,
        }
    }

    /// The HTTP status of an [`Api`](Self::Api) error.
    pub fn status(&self) -> Option<u16> {
        match self {
            ClientError::Api { status, .. } | ClientError::Decode { status, .. } => Some(*status),
            ClientError::Request(e) => e.status().map(|s| s.as_u16()),
            ClientError::Encode(_) | ClientError::Token(_) | ClientError::InvalidToken => None,
        }
    }

    /// `404`.
    pub fn is_not_found(&self) -> bool {
        self.status() == Some(404)
    }

    /// `409`.
    pub fn is_conflict(&self) -> bool {
        self.status() == Some(409)
    }
}

#[cfg(test)]
mod transport_tests;
