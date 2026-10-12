//! Bearer tokens for the platform client.
//!
//! [`ClientCredentials`] is the service-to-service source: the OAuth2
//! `client_credentials` grant against `{base}/oauth/token` (credentials in the
//! form body, as the Go and Java SDKs send them), cached until 60 s before
//! expiry, fetched single-flight (concurrent callers wait for one request),
//! and dropped on a `401` so the client refreshes once (Java SDK behaviour).

use std::fmt;
use std::future::Future;
use std::pin::Pin;
use std::time::{Duration, Instant};

use serde::Deserialize;
use tokio::sync::Mutex;

use super::ClientError;

/// A boxed, `Send` future (object-safe async in [`TokenSource`]).
pub type BoxFuture<'a, T> = Pin<Box<dyn Future<Output = T> + Send + 'a>>;

/// Supplies the bearer token for each request.
pub trait TokenSource: Send + Sync + fmt::Debug {
    /// The token to send (without the `Bearer ` prefix).
    fn token(&self) -> BoxFuture<'_, Result<String, ClientError>>;

    /// The platform answered `401` with the last token. Return `true` when a
    /// different token can now be obtained (the request is then retried once).
    fn invalidate(&self) -> BoxFuture<'_, bool> {
        Box::pin(async { false })
    }
}

/// A fixed token (a user's access token, a test token).
#[derive(Clone)]
pub struct StaticToken(String);

impl StaticToken {
    pub fn new(token: impl Into<String>) -> Self {
        Self(token.into())
    }
}

impl fmt::Debug for StaticToken {
    fn fmt(&self, f: &mut fmt::Formatter<'_>) -> fmt::Result {
        f.write_str("StaticToken(<redacted>)")
    }
}

impl TokenSource for StaticToken {
    fn token(&self) -> BoxFuture<'_, Result<String, ClientError>> {
        let t = self.0.clone();
        Box::pin(async move { Ok(t) })
    }
}

/// Why a token could not be obtained.
#[derive(Debug, thiserror::Error)]
#[non_exhaustive]
pub enum TokenError {
    #[error("client id and secret are required")]
    MissingCredentials,
    #[error("the token endpoint refused the client credentials (HTTP {status})")]
    InvalidCredentials { status: u16 },
    #[error("token fetch failed (HTTP {status}){}", message.as_deref().map(|m| format!(": {m}")).unwrap_or_default())]
    Rejected {
        status: u16,
        message: Option<String>,
    },
    #[error("token fetch failed: {0}")]
    Transport(#[source] reqwest::Error),
    #[error("token response had no access_token")]
    NoAccessToken,
}

/// Seconds before expiry at which a cached token is replaced.
pub const EXPIRY_BUFFER: Duration = Duration::from_secs(60);

/// OAuth2 client-credentials token source with caching.
pub struct ClientCredentials {
    token_url: String,
    client_id: String,
    client_secret: String,
    http: reqwest::Client,
    cached: Mutex<Option<(String, Instant)>>,
}

impl fmt::Debug for ClientCredentials {
    fn fmt(&self, f: &mut fmt::Formatter<'_>) -> fmt::Result {
        f.debug_struct("ClientCredentials")
            .field("token_url", &self.token_url)
            .field("client_id", &self.client_id)
            .field("client_secret", &"<redacted>")
            .finish()
    }
}

#[derive(Deserialize)]
struct TokenResponse {
    access_token: Option<String>,
    expires_in: Option<u64>,
}

#[derive(Deserialize)]
struct OAuthErrorBody {
    error: Option<String>,
    error_description: Option<String>,
}

impl ClientCredentials {
    /// Credentials for the platform at `base_url` (token endpoint
    /// `{base_url}/oauth/token`).
    pub fn new(
        base_url: impl AsRef<str>,
        client_id: impl Into<String>,
        client_secret: impl Into<String>,
    ) -> Self {
        let base = base_url.as_ref().trim_end_matches('/');
        Self::with_token_url(format!("{base}/oauth/token"), client_id, client_secret)
    }

    /// Credentials for an explicit token endpoint.
    pub fn with_token_url(
        token_url: impl Into<String>,
        client_id: impl Into<String>,
        client_secret: impl Into<String>,
    ) -> Self {
        Self {
            token_url: token_url.into(),
            client_id: client_id.into(),
            client_secret: client_secret.into(),
            http: reqwest::Client::new(),
            cached: Mutex::new(None),
        }
    }

    /// Use this HTTP client for the token request.
    pub fn with_http_client(mut self, http: reqwest::Client) -> Self {
        self.http = http;
        self
    }

    /// Drop the cached token without fetching a new one.
    pub async fn clear(&self) {
        *self.cached.lock().await = None;
    }

    async fn fetch(&self) -> Result<(String, Instant), TokenError> {
        if self.client_id.is_empty() || self.client_secret.is_empty() {
            return Err(TokenError::MissingCredentials);
        }
        let resp = self
            .http
            .post(&self.token_url)
            .header(reqwest::header::ACCEPT, "application/json")
            .form(&[
                ("grant_type", "client_credentials"),
                ("client_id", self.client_id.as_str()),
                ("client_secret", self.client_secret.as_str()),
            ])
            .send()
            .await
            .map_err(TokenError::Transport)?;
        let status = resp.status().as_u16();
        let body = resp.text().await.map_err(TokenError::Transport)?;
        if status == 401 || status == 403 {
            return Err(TokenError::InvalidCredentials { status });
        }
        if !(200..300).contains(&status) {
            let message = serde_json::from_str::<OAuthErrorBody>(&body)
                .ok()
                .and_then(|e| e.error_description.or(e.error));
            return Err(TokenError::Rejected { status, message });
        }
        let parsed: TokenResponse =
            serde_json::from_str(&body).map_err(|_| TokenError::NoAccessToken)?;
        let token = parsed
            .access_token
            .filter(|t| !t.is_empty())
            .ok_or(TokenError::NoAccessToken)?;
        let expires_at = Instant::now() + Duration::from_secs(parsed.expires_in.unwrap_or(0));
        Ok((token, expires_at))
    }
}

impl TokenSource for ClientCredentials {
    fn token(&self) -> BoxFuture<'_, Result<String, ClientError>> {
        Box::pin(async move {
            // Held across the fetch: concurrent callers wait for one request.
            let mut cached = self.cached.lock().await;
            if let Some((token, expires_at)) = cached.as_ref()
                && Instant::now() + EXPIRY_BUFFER < *expires_at
            {
                return Ok(token.clone());
            }
            let (token, expires_at) = self.fetch().await.map_err(ClientError::Token)?;
            *cached = Some((token.clone(), expires_at));
            Ok(token)
        })
    }

    fn invalidate(&self) -> BoxFuture<'_, bool> {
        Box::pin(async move {
            *self.cached.lock().await = None;
            true
        })
    }
}
