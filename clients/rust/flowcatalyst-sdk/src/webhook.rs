//! Webhook signature validation
//!
//! Validates incoming webhooks from FlowCatalyst using HMAC-SHA256 signatures.
//!
//! ## Example
//!
//! ```rust
//! use flowcatalyst_sdk::webhook::WebhookValidator;
//!
//! let validator = WebhookValidator::new("your-signing-secret");
//!
//! // Validate incoming webhook
//! let signature = "abc123..."; // From X-FlowCatalyst-Signature header
//! let timestamp = "1704067200"; // From X-FlowCatalyst-Timestamp header
//! let body = r#"{"event": "data"}"#;
//!
//! match validator.validate(signature, timestamp, body.as_bytes()) {
//!     Ok(()) => println!("Valid webhook!"),
//!     Err(e) => println!("Invalid webhook: {}", e),
//! }
//! ```

use crate::error::{Error, Result};
use hmac::{Hmac, Mac};
use sha2::Sha256;
use std::time::{Duration, SystemTime, UNIX_EPOCH};

type HmacSha256 = Hmac<Sha256>;

/// Header name for the webhook signature
pub const SIGNATURE_HEADER: &str = "X-FlowCatalyst-Signature";

/// Header name for the webhook timestamp
pub const TIMESTAMP_HEADER: &str = "X-FlowCatalyst-Timestamp";

/// Webhook signature validator
#[derive(Debug, Clone)]
pub struct WebhookValidator {
    secret: Vec<u8>,
    /// Maximum age for webhook timestamps (default: 300 seconds)
    pub tolerance: Duration,
    /// Maximum future timestamp grace period (default: 60 seconds)
    pub future_grace: Duration,
}

impl WebhookValidator {
    /// Create a new webhook validator with the given signing secret
    pub fn new(secret: impl AsRef<[u8]>) -> Self {
        Self {
            secret: secret.as_ref().to_vec(),
            tolerance: Duration::from_secs(300),
            future_grace: Duration::from_secs(60),
        }
    }

    /// Set the maximum age tolerance for webhook timestamps
    pub fn with_tolerance(mut self, tolerance: Duration) -> Self {
        self.tolerance = tolerance;
        self
    }

    /// Set the future timestamp grace period
    pub fn with_future_grace(mut self, grace: Duration) -> Self {
        self.future_grace = grace;
        self
    }

    /// Validate a webhook request
    ///
    /// # Arguments
    ///
    /// * `signature` - The value of the X-FlowCatalyst-Signature header (hex-encoded)
    /// * `timestamp` - The value of the X-FlowCatalyst-Timestamp header (Unix timestamp in seconds)
    /// * `body` - The raw request body
    ///
    /// # Returns
    ///
    /// `Ok(())` if the signature is valid, or an `Error` describing the validation failure.
    pub fn validate(&self, signature: &str, timestamp: &str, body: &[u8]) -> Result<()> {
        // Parse and validate timestamp
        let ts: u64 = timestamp.parse().map_err(|_| {
            Error::InvalidSignature(format!("Invalid timestamp format: {}", timestamp))
        })?;

        self.validate_timestamp(ts)?;

        // Compute expected signature
        let expected = self.compute_signature(timestamp, body);

        // Constant-time comparison
        if !constant_time_compare(&expected, signature) {
            return Err(Error::InvalidSignature(
                "Signature mismatch".to_string(),
            ));
        }

        Ok(())
    }

    /// Validate webhook timestamp
    fn validate_timestamp(&self, timestamp: u64) -> Result<()> {
        let now = SystemTime::now()
            .duration_since(UNIX_EPOCH)
            .map_err(|_| Error::Other("System time error".to_string()))?
            .as_secs();

        // Check if timestamp is too old
        if timestamp + self.tolerance.as_secs() < now {
            return Err(Error::TimestampExpired);
        }

        // Check if timestamp is too far in the future
        if timestamp > now + self.future_grace.as_secs() {
            return Err(Error::TimestampInFuture);
        }

        Ok(())
    }

    /// Compute the expected signature for a webhook
    pub fn compute_signature(&self, timestamp: &str, body: &[u8]) -> String {
        let mut mac =
            HmacSha256::new_from_slice(&self.secret).expect("HMAC can take key of any size");

        // Message format: {timestamp}{body}
        mac.update(timestamp.as_bytes());
        mac.update(body);

        let result = mac.finalize();
        hex::encode(result.into_bytes())
    }

    /// Generate a signature for testing purposes
    ///
    /// Returns (signature, timestamp) tuple.
    pub fn sign(&self, body: &[u8]) -> (String, String) {
        let timestamp = SystemTime::now()
            .duration_since(UNIX_EPOCH)
            .expect("Time went backwards")
            .as_secs()
            .to_string();

        let signature = self.compute_signature(&timestamp, body);
        (signature, timestamp)
    }
}

/// Constant-time string comparison to prevent timing attacks
fn constant_time_compare(a: &str, b: &str) -> bool {
    if a.len() != b.len() {
        return false;
    }

    let mut result = 0u8;
    for (x, y) in a.bytes().zip(b.bytes()) {
        result |= x ^ y;
    }
    result == 0
}

/// Helper for extracting webhook headers from HTTP requests
pub struct WebhookHeaders {
    pub signature: String,
    pub timestamp: String,
}

impl WebhookHeaders {
    /// Extract webhook headers from a header map
    ///
    /// Works with any header map that implements the required trait.
    pub fn from_headers<F>(get_header: F) -> Result<Self>
    where
        F: Fn(&str) -> Option<String>,
    {
        let signature = get_header(SIGNATURE_HEADER)
            .ok_or_else(|| Error::MissingHeader(SIGNATURE_HEADER.to_string()))?;

        let timestamp = get_header(TIMESTAMP_HEADER)
            .ok_or_else(|| Error::MissingHeader(TIMESTAMP_HEADER.to_string()))?;

        Ok(Self {
            signature,
            timestamp,
        })
    }
}

#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn test_signature_computation() {
        let validator = WebhookValidator::new("test-secret");
        let timestamp = "1704067200";
        let body = b"test body";

        let sig1 = validator.compute_signature(timestamp, body);
        let sig2 = validator.compute_signature(timestamp, body);

        assert_eq!(sig1, sig2);
    }

    #[test]
    fn test_sign_and_validate() {
        let validator = WebhookValidator::new("test-secret");
        let body = b"test webhook payload";

        let (signature, timestamp) = validator.sign(body);

        assert!(validator.validate(&signature, &timestamp, body).is_ok());
    }

    #[test]
    fn test_invalid_signature() {
        let validator = WebhookValidator::new("test-secret");
        let body = b"test body";

        let (_, timestamp) = validator.sign(body);

        let result = validator.validate("invalid-signature", &timestamp, body);
        assert!(matches!(result, Err(Error::InvalidSignature(_))));
    }

    #[test]
    fn test_constant_time_compare() {
        assert!(constant_time_compare("abc", "abc"));
        assert!(!constant_time_compare("abc", "abd"));
        assert!(!constant_time_compare("abc", "ab"));
    }
}
