//! High-level FlowCatalyst client

use crate::auth::AuthenticatedClient;
use crate::config::Config;
use crate::error::{Error, Result};
use std::sync::Arc;

/// FlowCatalyst API client
///
/// Provides a high-level interface to the FlowCatalyst Platform API with
/// automatic authentication, retries, and error handling.
#[derive(Debug, Clone)]
pub struct Client {
    config: Arc<Config>,
    auth_client: Arc<AuthenticatedClient>,
    http_client: reqwest::Client,
}

impl Client {
    /// Create a new FlowCatalyst client with the given configuration
    pub async fn new(config: Config) -> Result<Self> {
        let http_client = reqwest::Client::builder()
            .timeout(config.timeout)
            .user_agent(&config.user_agent)
            .build()?;

        let auth_client = if let (Some(client_id), Some(client_secret)) =
            (&config.client_id, &config.client_secret)
        {
            AuthenticatedClient::new(
                http_client.clone(),
                client_id.clone(),
                client_secret.clone(),
                config.get_token_url(),
            )
        } else {
            AuthenticatedClient::unauthenticated(http_client.clone())
        };

        Ok(Self {
            config: Arc::new(config),
            auth_client: Arc::new(auth_client),
            http_client,
        })
    }

    /// Get the base URL
    pub fn base_url(&self) -> &str {
        &self.config.base_url
    }

    /// Get the underlying HTTP client
    pub fn http_client(&self) -> &reqwest::Client {
        &self.http_client
    }

    /// Execute a request with automatic retries and error handling
    pub async fn execute<T>(&self, request: reqwest::Request) -> Result<T>
    where
        T: serde::de::DeserializeOwned,
    {
        let mut last_error = None;

        for attempt in 0..self.config.retry_attempts {
            if attempt > 0 {
                let delay = self.config.retry_delay * (1 << (attempt - 1));
                tokio::time::sleep(delay).await;
            }

            let request = request
                .try_clone()
                .ok_or_else(|| Error::Other("Request cannot be cloned".into()))?;

            match self.http_client.execute(request).await {
                Ok(response) => {
                    let status = response.status();

                    if status.is_success() {
                        let body: T = response.json().await?;
                        return Ok(body);
                    }

                    // Handle 401 - try token refresh
                    if status == reqwest::StatusCode::UNAUTHORIZED && attempt == 0 {
                        self.auth_client.invalidate_token().await;
                        continue;
                    }

                    let body = response.text().await.unwrap_or_default();
                    let error = Error::from_status(status, body);

                    if !error.is_retryable() {
                        return Err(error);
                    }

                    last_error = Some(error);
                }
                Err(e) => {
                    last_error = Some(Error::Http(e));
                }
            }
        }

        Err(last_error.unwrap_or_else(|| Error::Other("Request failed".into())))
    }

    /// Get an authenticated request builder
    pub async fn request(
        &self,
        method: reqwest::Method,
        path: &str,
    ) -> Result<reqwest::RequestBuilder> {
        let url = format!("{}{}", self.config.base_url, path);
        self.auth_client.request(method, &url).await
    }
}
