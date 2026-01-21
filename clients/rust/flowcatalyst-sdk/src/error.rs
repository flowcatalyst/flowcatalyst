//! Error types for the FlowCatalyst SDK

use thiserror::Error;

/// Result type alias for FlowCatalyst SDK operations
pub type Result<T> = std::result::Result<T, Error>;

/// Error types for the FlowCatalyst SDK
#[derive(Error, Debug)]
pub enum Error {
    /// Authentication failed
    #[error("Authentication failed: {0}")]
    Authentication(String),

    /// Authorization failed (403)
    #[error("Forbidden: {0}")]
    Forbidden(String),

    /// Resource not found (404)
    #[error("Not found: {0}")]
    NotFound(String),

    /// Validation error (422)
    #[error("Validation error: {0}")]
    Validation(String),

    /// Rate limit exceeded (429)
    #[error("Rate limit exceeded: retry after {retry_after:?}")]
    RateLimited {
        retry_after: Option<std::time::Duration>,
    },

    /// Server error (5xx)
    #[error("Server error: {0}")]
    Server(String),

    /// HTTP request failed
    #[error("HTTP error: {0}")]
    Http(#[from] reqwest::Error),

    /// JSON serialization/deserialization error
    #[error("JSON error: {0}")]
    Json(#[from] serde_json::Error),

    /// Configuration error
    #[error("Configuration error: {0}")]
    Config(String),

    /// Webhook signature validation failed
    #[error("Invalid webhook signature: {0}")]
    InvalidSignature(String),

    /// Webhook timestamp expired
    #[error("Webhook timestamp expired")]
    TimestampExpired,

    /// Webhook timestamp in future
    #[error("Webhook timestamp in future")]
    TimestampInFuture,

    /// Missing required header
    #[error("Missing required header: {0}")]
    MissingHeader(String),

    /// Outbox error
    #[error("Outbox error: {0}")]
    Outbox(String),

    /// Database error
    #[error("Database error: {0}")]
    Database(String),

    /// Generic error
    #[error("{0}")]
    Other(String),
}

impl Error {
    /// Check if the error is retryable
    pub fn is_retryable(&self) -> bool {
        matches!(self, Error::RateLimited { .. } | Error::Server(_))
    }

    /// Create an error from an HTTP status code and message
    pub fn from_status(status: reqwest::StatusCode, message: String) -> Self {
        match status.as_u16() {
            401 => Error::Authentication(message),
            403 => Error::Forbidden(message),
            404 => Error::NotFound(message),
            422 => Error::Validation(message),
            429 => Error::RateLimited { retry_after: None },
            500..=599 => Error::Server(message),
            _ => Error::Other(format!("HTTP {}: {}", status, message)),
        }
    }
}
