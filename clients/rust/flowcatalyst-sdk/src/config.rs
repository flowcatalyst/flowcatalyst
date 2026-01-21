//! SDK Configuration

use std::time::Duration;

/// Configuration for the FlowCatalyst SDK
#[derive(Debug, Clone)]
pub struct Config {
    /// Base URL for the FlowCatalyst API
    pub base_url: String,

    /// OAuth2 client ID
    pub client_id: Option<String>,

    /// OAuth2 client secret
    pub client_secret: Option<String>,

    /// OAuth2 token endpoint (defaults to {base_url}/oauth/token)
    pub token_url: Option<String>,

    /// Request timeout
    pub timeout: Duration,

    /// Maximum retry attempts
    pub retry_attempts: u32,

    /// Initial retry delay
    pub retry_delay: Duration,

    /// User agent string
    pub user_agent: String,
}

impl Config {
    /// Create a new configuration with the given base URL
    pub fn new(base_url: impl Into<String>) -> Self {
        Self {
            base_url: base_url.into(),
            client_id: None,
            client_secret: None,
            token_url: None,
            timeout: Duration::from_secs(30),
            retry_attempts: 3,
            retry_delay: Duration::from_millis(100),
            user_agent: format!("FlowCatalyst-Rust-SDK/{}", env!("CARGO_PKG_VERSION")),
        }
    }

    /// Set OAuth2 client credentials
    pub fn with_client_credentials(
        mut self,
        client_id: impl Into<String>,
        client_secret: impl Into<String>,
    ) -> Self {
        self.client_id = Some(client_id.into());
        self.client_secret = Some(client_secret.into());
        self
    }

    /// Set custom token URL
    pub fn with_token_url(mut self, token_url: impl Into<String>) -> Self {
        self.token_url = Some(token_url.into());
        self
    }

    /// Set request timeout
    pub fn with_timeout(mut self, timeout: Duration) -> Self {
        self.timeout = timeout;
        self
    }

    /// Set retry configuration
    pub fn with_retry(mut self, attempts: u32, delay: Duration) -> Self {
        self.retry_attempts = attempts;
        self.retry_delay = delay;
        self
    }

    /// Set custom user agent
    pub fn with_user_agent(mut self, user_agent: impl Into<String>) -> Self {
        self.user_agent = user_agent.into();
        self
    }

    /// Get the token URL (defaults to {base_url}/oauth/token)
    pub fn get_token_url(&self) -> String {
        self.token_url
            .clone()
            .unwrap_or_else(|| format!("{}/oauth/token", self.base_url))
    }
}

impl Default for Config {
    fn default() -> Self {
        Self::new("http://localhost:8080")
    }
}
