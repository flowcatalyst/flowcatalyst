//! Authentication support for FlowCatalyst SDK
//!
//! Provides OIDC client credentials flow with automatic token refresh.

use crate::error::{Error, Result};
use serde::{Deserialize, Serialize};
use std::sync::Arc;
use std::time::{Duration, Instant};
use tokio::sync::RwLock;

/// Token response from the OAuth2 token endpoint
#[derive(Debug, Clone, Deserialize)]
pub struct TokenResponse {
    pub access_token: String,
    pub token_type: String,
    #[serde(default)]
    pub expires_in: Option<u64>,
    #[serde(default)]
    pub refresh_token: Option<String>,
    #[serde(default)]
    pub scope: Option<String>,
}

/// Token request for client credentials grant
#[derive(Debug, Serialize)]
struct TokenRequest<'a> {
    grant_type: &'a str,
    client_id: &'a str,
    client_secret: &'a str,
    #[serde(skip_serializing_if = "Option::is_none")]
    scope: Option<&'a str>,
}

/// Cached token with expiration tracking
#[derive(Debug, Clone)]
struct CachedToken {
    access_token: String,
    expires_at: Instant,
}

impl CachedToken {
    /// Check if the token is still valid (with 60-second buffer)
    fn is_valid(&self) -> bool {
        self.expires_at > Instant::now() + Duration::from_secs(60)
    }
}

/// Token manager for OIDC authentication
#[derive(Debug)]
pub struct TokenManager {
    client_id: String,
    client_secret: String,
    token_url: String,
    http_client: reqwest::Client,
    cached_token: Arc<RwLock<Option<CachedToken>>>,
}

impl TokenManager {
    /// Create a new token manager
    pub fn new(
        client_id: String,
        client_secret: String,
        token_url: String,
        http_client: reqwest::Client,
    ) -> Self {
        Self {
            client_id,
            client_secret,
            token_url,
            http_client,
            cached_token: Arc::new(RwLock::new(None)),
        }
    }

    /// Get a valid access token, refreshing if necessary
    pub async fn get_token(&self) -> Result<String> {
        // Check if we have a valid cached token
        {
            let cached = self.cached_token.read().await;
            if let Some(ref token) = *cached {
                if token.is_valid() {
                    return Ok(token.access_token.clone());
                }
            }
        }

        // Need to refresh - acquire write lock
        let mut cached = self.cached_token.write().await;

        // Double-check in case another task refreshed while we waited
        if let Some(ref token) = *cached {
            if token.is_valid() {
                return Ok(token.access_token.clone());
            }
        }

        // Fetch new token
        let new_token = self.fetch_token().await?;
        let access_token = new_token.access_token.clone();

        // Calculate expiration time
        let expires_in = new_token.expires_in.unwrap_or(3600);
        let expires_at = Instant::now() + Duration::from_secs(expires_in);

        *cached = Some(CachedToken {
            access_token: access_token.clone(),
            expires_at,
        });

        Ok(access_token)
    }

    /// Fetch a new token from the token endpoint
    async fn fetch_token(&self) -> Result<TokenResponse> {
        let request = TokenRequest {
            grant_type: "client_credentials",
            client_id: &self.client_id,
            client_secret: &self.client_secret,
            scope: None,
        };

        let response = self
            .http_client
            .post(&self.token_url)
            .form(&request)
            .send()
            .await?;

        if !response.status().is_success() {
            let status = response.status();
            let body = response.text().await.unwrap_or_default();
            return Err(Error::Authentication(format!(
                "Token request failed with status {}: {}",
                status, body
            )));
        }

        let token_response: TokenResponse = response.json().await?;
        Ok(token_response)
    }

    /// Invalidate the cached token (force refresh on next request)
    pub async fn invalidate(&self) {
        let mut cached = self.cached_token.write().await;
        *cached = None;
    }
}

/// Authenticated HTTP client wrapper
#[derive(Debug, Clone)]
pub struct AuthenticatedClient {
    http_client: reqwest::Client,
    token_manager: Option<Arc<TokenManager>>,
}

impl AuthenticatedClient {
    /// Create an authenticated client with token management
    pub fn new(
        http_client: reqwest::Client,
        client_id: String,
        client_secret: String,
        token_url: String,
    ) -> Self {
        let token_manager = Arc::new(TokenManager::new(
            client_id,
            client_secret,
            token_url,
            http_client.clone(),
        ));

        Self {
            http_client,
            token_manager: Some(token_manager),
        }
    }

    /// Create an unauthenticated client
    pub fn unauthenticated(http_client: reqwest::Client) -> Self {
        Self {
            http_client,
            token_manager: None,
        }
    }

    /// Get a request builder with authentication header
    pub async fn request(
        &self,
        method: reqwest::Method,
        url: &str,
    ) -> Result<reqwest::RequestBuilder> {
        let mut builder = self.http_client.request(method, url);

        if let Some(ref manager) = self.token_manager {
            let token = manager.get_token().await?;
            builder = builder.bearer_auth(token);
        }

        Ok(builder)
    }

    /// Invalidate the current token
    pub async fn invalidate_token(&self) {
        if let Some(ref manager) = self.token_manager {
            manager.invalidate().await;
        }
    }
}
