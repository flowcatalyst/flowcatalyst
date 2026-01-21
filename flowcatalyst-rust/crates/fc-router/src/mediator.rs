//! Mediator - HTTP-based message delivery
//!
//! Mirrors the Java HttpMediator with:
//! - HTTP POST to mediation target
//! - Auth token handling
//! - HMAC-SHA256 webhook signing (X-FLOWCATALYST-SIGNATURE, X-FLOWCATALYST-TIMESTAMP)
//! - Response code classification
//! - Retry with exponential backoff
//! - Circuit breaker pattern
//! - Custom delay parsing from response

use async_trait::async_trait;
use chrono::Utc;
use fc_common::{Message, MediationType, MediationResult, MediationOutcome, WarningCategory, WarningSeverity};
use hmac::{Hmac, Mac};
use reqwest::Client;
use serde::{Deserialize, Serialize};
use sha2::Sha256;
use std::sync::Arc;
use std::sync::atomic::{AtomicU32, Ordering};
use std::time::{Duration, Instant};
use parking_lot::RwLock;
use tracing::{info, warn, error, debug};

use crate::warning::WarningService;

/// FlowCatalyst webhook signature header (matches Java: X-FLOWCATALYST-SIGNATURE)
pub const SIGNATURE_HEADER: &str = "X-FLOWCATALYST-SIGNATURE";
/// FlowCatalyst webhook timestamp header (matches Java: X-FLOWCATALYST-TIMESTAMP)
pub const TIMESTAMP_HEADER: &str = "X-FLOWCATALYST-TIMESTAMP";

type HmacSha256 = Hmac<Sha256>;

/// Generate HMAC-SHA256 signature for webhook payload.
///
/// Matches Java WebhookSigner.sign():
/// - Signature payload = timestamp + body
/// - HMAC-SHA256 with signing_secret
/// - Returns hex-encoded signature
fn sign_webhook(payload: &str, signing_secret: &str) -> (String, String) {
    // Generate ISO8601 timestamp with millisecond precision (matches Java)
    let timestamp = Utc::now().format("%Y-%m-%dT%H:%M:%S%.3fZ").to_string();

    // Create signature payload: timestamp + body (matches Java: signaturePayload = timestamp + payload)
    let signature_payload = format!("{}{}", timestamp, payload);

    // Generate HMAC-SHA256
    let mut mac = HmacSha256::new_from_slice(signing_secret.as_bytes())
        .expect("HMAC can take key of any size");
    mac.update(signature_payload.as_bytes());
    let result = mac.finalize();

    // Return as lowercase hex (matches Java: HexFormat.of().formatHex())
    let signature = hex::encode(result.into_bytes());

    (signature, timestamp)
}

/// Trait for message mediation
#[async_trait]
pub trait Mediator: Send + Sync {
    async fn mediate(&self, message: &Message) -> MediationOutcome;
}

/// Payload sent to mediation target (matches Java format)
/// Java sends: {"messageId":"<id>"}
#[derive(Debug, Serialize)]
struct MediationPayload<'a> {
    #[serde(rename = "messageId")]
    message_id: &'a str,
}

/// Response from mediation target
#[derive(Debug, Deserialize, Default)]
struct MediationResponse {
    #[serde(default = "default_ack")]
    ack: bool,
    #[serde(rename = "delaySeconds")]
    delay_seconds: Option<u32>,
}

fn default_ack() -> bool {
    true
}

/// Circuit breaker state
#[derive(Debug, Clone, Copy, PartialEq, Eq)]
pub enum CircuitState {
    Closed,
    Open,
    HalfOpen,
}

/// Circuit breaker for protecting downstream services
pub struct CircuitBreaker {
    state: RwLock<CircuitState>,
    failure_count: AtomicU32,
    success_count: AtomicU32,
    last_failure_time: RwLock<Option<Instant>>,

    /// Number of failures to trip the breaker
    failure_threshold: u32,
    /// Number of successes in half-open to close
    success_threshold: u32,
    /// Time to wait before half-opening
    reset_timeout: Duration,
}

impl CircuitBreaker {
    pub fn new(failure_threshold: u32, success_threshold: u32, reset_timeout: Duration) -> Self {
        Self {
            state: RwLock::new(CircuitState::Closed),
            failure_count: AtomicU32::new(0),
            success_count: AtomicU32::new(0),
            last_failure_time: RwLock::new(None),
            failure_threshold,
            success_threshold,
            reset_timeout,
        }
    }

    /// Check if request should be allowed
    pub fn allow_request(&self) -> bool {
        let state = *self.state.read();

        match state {
            CircuitState::Closed => true,
            CircuitState::Open => {
                // Check if we should transition to half-open
                if let Some(last_failure) = *self.last_failure_time.read() {
                    if last_failure.elapsed() >= self.reset_timeout {
                        *self.state.write() = CircuitState::HalfOpen;
                        self.success_count.store(0, Ordering::SeqCst);
                        debug!("Circuit breaker transitioning to half-open");
                        return true;
                    }
                }
                false
            }
            CircuitState::HalfOpen => true,
        }
    }

    /// Record a successful request
    pub fn record_success(&self) {
        let state = *self.state.read();

        match state {
            CircuitState::HalfOpen => {
                let count = self.success_count.fetch_add(1, Ordering::SeqCst) + 1;
                if count >= self.success_threshold {
                    *self.state.write() = CircuitState::Closed;
                    self.failure_count.store(0, Ordering::SeqCst);
                    info!("Circuit breaker closed after {} successes", count);
                }
            }
            CircuitState::Closed => {
                // Reset failure count on success
                self.failure_count.store(0, Ordering::SeqCst);
            }
            _ => {}
        }
    }

    /// Record a failed request
    pub fn record_failure(&self) {
        let state = *self.state.read();

        match state {
            CircuitState::Closed => {
                let count = self.failure_count.fetch_add(1, Ordering::SeqCst) + 1;
                if count >= self.failure_threshold {
                    *self.state.write() = CircuitState::Open;
                    *self.last_failure_time.write() = Some(Instant::now());
                    warn!("Circuit breaker opened after {} failures", count);
                }
            }
            CircuitState::HalfOpen => {
                // Any failure in half-open immediately opens
                *self.state.write() = CircuitState::Open;
                *self.last_failure_time.write() = Some(Instant::now());
                self.success_count.store(0, Ordering::SeqCst);
                warn!("Circuit breaker re-opened on failure in half-open state");
            }
            _ => {}
        }
    }

    /// Get current state
    pub fn state(&self) -> CircuitState {
        *self.state.read()
    }

    /// Get failure rate (approximate)
    pub fn failure_count(&self) -> u32 {
        self.failure_count.load(Ordering::SeqCst)
    }
}

impl Default for CircuitBreaker {
    fn default() -> Self {
        Self::new(10, 5, Duration::from_secs(5))
    }
}

/// HTTP version to use for mediation requests
#[derive(Debug, Clone, Copy, PartialEq, Eq, Default)]
pub enum HttpVersion {
    /// HTTP/1.1 - better for development/debugging
    Http1,
    /// HTTP/2 - better for production (multiplexing, header compression)
    #[default]
    Http2,
}

/// Configuration for HTTP mediator
#[derive(Debug, Clone)]
pub struct HttpMediatorConfig {
    /// Request timeout (Java default: 900s / 15 minutes)
    pub timeout: Duration,
    /// HTTP version to use
    pub http_version: HttpVersion,
    pub max_retries: u32,
    pub retry_delays: Vec<Duration>,
    /// Circuit breaker failure threshold
    pub circuit_breaker_threshold: u32,
    /// Circuit breaker reset timeout
    pub circuit_breaker_timeout: Duration,
    /// Connection timeout
    pub connect_timeout: Duration,
}

impl Default for HttpMediatorConfig {
    fn default() -> Self {
        Self {
            timeout: Duration::from_secs(900), // 15 minutes - matches Java default
            http_version: HttpVersion::Http2,  // Production default
            max_retries: 3,
            retry_delays: vec![
                Duration::from_secs(1),
                Duration::from_secs(2),
                Duration::from_secs(3),
            ],
            circuit_breaker_threshold: 10,
            circuit_breaker_timeout: Duration::from_secs(5),
            connect_timeout: Duration::from_secs(30),
        }
    }
}

impl HttpMediatorConfig {
    /// Create config for development mode (HTTP/1.1, shorter timeout)
    pub fn dev() -> Self {
        Self {
            timeout: Duration::from_secs(30),
            http_version: HttpVersion::Http1,
            max_retries: 3,
            retry_delays: vec![
                Duration::from_secs(1),
                Duration::from_secs(2),
                Duration::from_secs(3),
            ],
            circuit_breaker_threshold: 10,
            circuit_breaker_timeout: Duration::from_secs(5),
            connect_timeout: Duration::from_secs(10),
        }
    }

    /// Create config for production mode (HTTP/2, long timeout)
    pub fn production() -> Self {
        Self::default()
    }
}

/// HTTP-based message mediator with circuit breaker
pub struct HttpMediator {
    client: Client,
    config: HttpMediatorConfig,
    circuit_breaker: CircuitBreaker,
    warning_service: Option<Arc<WarningService>>,
}

impl HttpMediator {
    pub fn new() -> Self {
        Self::with_config(HttpMediatorConfig::default())
    }

    /// Create mediator with dev mode configuration (HTTP/1.1)
    pub fn dev() -> Self {
        Self::with_config(HttpMediatorConfig::dev())
    }

    /// Create mediator with production configuration (HTTP/2)
    pub fn production() -> Self {
        Self::with_config(HttpMediatorConfig::production())
    }

    pub fn with_config(config: HttpMediatorConfig) -> Self {
        let mut builder = Client::builder()
            .timeout(config.timeout)
            .connect_timeout(config.connect_timeout)
            .pool_max_idle_per_host(10);

        // Configure HTTP version
        match config.http_version {
            HttpVersion::Http1 => {
                // Force HTTP/1.1 only
                builder = builder.http1_only();
                info!("HttpMediator configured for HTTP/1.1");
            }
            HttpVersion::Http2 => {
                // For HTTPS: let ALPN negotiate HTTP/2 (this is the default behavior)
                // Do NOT use http2_prior_knowledge() for HTTPS - that's for h2c (cleartext)
                // reqwest will automatically negotiate HTTP/2 via ALPN for HTTPS
                info!("HttpMediator configured for HTTP/2 (ALPN negotiation)");
            }
        }

        let client = builder.build().expect("Failed to build HTTP client");

        let circuit_breaker = CircuitBreaker::new(
            config.circuit_breaker_threshold,
            5,
            config.circuit_breaker_timeout,
        );

        info!(
            timeout_secs = config.timeout.as_secs(),
            http_version = ?config.http_version,
            "HttpMediator initialized"
        );

        Self { client, config, circuit_breaker, warning_service: None }
    }

    /// Set the warning service for generating configuration warnings
    pub fn with_warning_service(mut self, warning_service: Arc<WarningService>) -> Self {
        self.warning_service = Some(warning_service);
        self
    }

    /// Set warning service after construction
    pub fn set_warning_service(&mut self, warning_service: Arc<WarningService>) {
        self.warning_service = Some(warning_service);
    }

    /// Generate a configuration warning
    fn warn_config(&self, message_id: &str, target: &str, status_code: u16, description: &str) {
        if let Some(ref ws) = self.warning_service {
            let severity = if status_code == 501 {
                WarningSeverity::Critical
            } else {
                WarningSeverity::Error
            };
            ws.add_warning(
                WarningCategory::Configuration,
                severity,
                format!("HTTP {} {} for message {}: Target: {}", status_code, description, message_id, target),
                "HttpMediator".to_string(),
            );
        }
    }

    /// Get circuit breaker state
    pub fn circuit_state(&self) -> CircuitState {
        self.circuit_breaker.state()
    }

    async fn mediate_once(&self, message: &Message) -> MediationOutcome {
        if message.mediation_type != MediationType::HTTP {
            return MediationOutcome::error_config(
                0,
                format!("Unsupported mediation type: {:?}", message.mediation_type),
            );
        }

        // Check circuit breaker
        if !self.circuit_breaker.allow_request() {
            debug!(
                message_id = %message.id,
                "Circuit breaker open, rejecting request"
            );
            return MediationOutcome {
                result: MediationResult::ErrorConnection,
                delay_seconds: Some(5),
                status_code: None,
                error_message: Some("Circuit breaker open".to_string()),
            };
        }

        // Build payload matching Java format: {"messageId":"<id>"}
        let payload = MediationPayload {
            message_id: &message.id,
        };

        debug!(
            message_id = %message.id,
            target = %message.mediation_target,
            has_auth_token = message.auth_token.is_some(),
            auth_token_preview = message.auth_token.as_ref().map(|t| if t.len() > 20 { format!("{}...", &t[..20]) } else { t.clone() }),
            "Mediating message"
        );

        // Serialize payload for signing
        let payload_json = serde_json::to_string(&payload)
            .expect("Failed to serialize payload");

        let mut request = self.client
            .post(&message.mediation_target)
            .header("Content-Type", "application/json")
            .header("Accept", "application/json");

        // Add webhook signing headers if signing_secret is present
        if let Some(ref signing_secret) = message.signing_secret {
            let (signature, timestamp) = sign_webhook(&payload_json, signing_secret);
            request = request
                .header(SIGNATURE_HEADER, signature)
                .header(TIMESTAMP_HEADER, timestamp);
        }

        if let Some(token) = &message.auth_token {
            request = request.bearer_auth(token);
        }

        // Add the body after all headers are set
        request = request.body(payload_json);

        match request.send().await {
            Ok(response) => {
                let status = response.status();
                let status_code = status.as_u16();

                if status.is_success() {
                    self.circuit_breaker.record_success();

                    // Parse response body for ack and delaySeconds
                    if let Ok(body) = response.text().await {
                        if let Ok(resp) = serde_json::from_str::<MediationResponse>(&body) {
                            if !resp.ack {
                                // Target says not ready yet - use custom delay if provided
                                let delay = resp.delay_seconds.unwrap_or(5);
                                debug!(
                                    message_id = %message.id,
                                    delay_seconds = delay,
                                    "Target returned ack=false with delay"
                                );
                                return MediationOutcome {
                                    result: MediationResult::ErrorProcess,
                                    delay_seconds: Some(delay),
                                    status_code: Some(status_code),
                                    error_message: Some("Target returned ack=false".to_string()),
                                };
                            }
                        }
                    }

                    info!(
                        message_id = %message.id,
                        status_code = status_code,
                        "Message delivered successfully"
                    );
                    MediationOutcome::success()
                } else if status_code == 400 {
                    // Bad request - configuration error
                    self.circuit_breaker.record_success(); // Don't count as failure
                    warn!(
                        message_id = %message.id,
                        status_code = status_code,
                        "Bad request - configuration error"
                    );
                    self.warn_config(&message.id, &message.mediation_target, status_code, "Bad Request");
                    MediationOutcome::error_config(status_code, "HTTP 400: Bad request".to_string())
                } else if status_code == 401 || status_code == 403 {
                    // Auth errors - configuration error
                    self.circuit_breaker.record_success();
                    let desc = if status_code == 401 { "Unauthorized" } else { "Forbidden" };
                    warn!(
                        message_id = %message.id,
                        status_code = status_code,
                        "Authentication/authorization error"
                    );
                    self.warn_config(&message.id, &message.mediation_target, status_code, desc);
                    MediationOutcome::error_config(status_code, format!("HTTP {}: Auth error", status_code))
                } else if status_code == 404 {
                    // Not found - configuration error
                    self.circuit_breaker.record_success();
                    warn!(
                        message_id = %message.id,
                        status_code = status_code,
                        "Endpoint not found"
                    );
                    self.warn_config(&message.id, &message.mediation_target, status_code, "Not Found");
                    MediationOutcome::error_config(status_code, "HTTP 404: Not found".to_string())
                } else if status_code == 429 {
                    // Too Many Requests - TRANSIENT error, respect Retry-After
                    // Don't count as circuit breaker failure (it's rate limiting, not a real error)
                    self.circuit_breaker.record_success();

                    // Parse Retry-After header if present, default to 30 seconds
                    let retry_after = response.headers()
                        .get("Retry-After")
                        .and_then(|v| v.to_str().ok())
                        .and_then(|s| s.parse::<u32>().ok())
                        .unwrap_or(30);

                    warn!(
                        message_id = %message.id,
                        status_code = status_code,
                        retry_after = retry_after,
                        "Rate limited (429) - will retry"
                    );
                    MediationOutcome {
                        result: MediationResult::ErrorProcess,
                        delay_seconds: Some(retry_after),
                        status_code: Some(status_code),
                        error_message: Some("HTTP 429: Too Many Requests".to_string()),
                    }
                } else if status_code == 501 {
                    // Not implemented - configuration error (CRITICAL)
                    self.circuit_breaker.record_success();
                    warn!(
                        message_id = %message.id,
                        status_code = status_code,
                        "Not implemented"
                    );
                    self.warn_config(&message.id, &message.mediation_target, status_code, "Not Implemented");
                    MediationOutcome::error_config(status_code, "HTTP 501: Not implemented".to_string())
                } else if status.is_client_error() {
                    // Other 4xx - treat as config error (but NOT 429 which is handled above)
                    self.circuit_breaker.record_success();
                    warn!(
                        message_id = %message.id,
                        status_code = status_code,
                        "Client error"
                    );
                    MediationOutcome::error_config(status_code, format!("HTTP {}: Client error", status_code))
                } else if status.is_server_error() {
                    // 5xx - Transient error, retry
                    self.circuit_breaker.record_failure();
                    warn!(
                        message_id = %message.id,
                        status_code = status_code,
                        "Server error - will retry"
                    );
                    MediationOutcome {
                        result: MediationResult::ErrorProcess,
                        delay_seconds: Some(30),
                        status_code: Some(status_code),
                        error_message: Some(format!("HTTP {}: Server error", status_code)),
                    }
                } else {
                    // Other status codes
                    warn!(
                        message_id = %message.id,
                        status_code = status_code,
                        "Unexpected status code"
                    );
                    MediationOutcome::error_process(
                        Some(30),
                        format!("HTTP {}: Unexpected status", status_code),
                    )
                }
            }
            Err(e) => {
                self.circuit_breaker.record_failure();

                if e.is_timeout() {
                    warn!(
                        message_id = %message.id,
                        error = %e,
                        "Request timeout"
                    );
                    MediationOutcome::error_connection("Request timeout".to_string())
                } else if e.is_connect() {
                    warn!(
                        message_id = %message.id,
                        error = %e,
                        "Connection error"
                    );
                    MediationOutcome::error_connection(format!("Connection error: {}", e))
                } else {
                    error!(
                        message_id = %message.id,
                        target = %message.mediation_target,
                        error = %e,
                        error_debug = ?e,
                        is_request = e.is_request(),
                        is_redirect = e.is_redirect(),
                        is_status = e.is_status(),
                        is_body = e.is_body(),
                        is_decode = e.is_decode(),
                        "Request failed"
                    );
                    MediationOutcome::error_connection(format!("Request failed: {}", e))
                }
            }
        }
    }
}

#[async_trait]
impl Mediator for HttpMediator {
    async fn mediate(&self, message: &Message) -> MediationOutcome {
        let mut attempts = 0;

        loop {
            let outcome = self.mediate_once(message).await;

            // Don't retry on success or config errors
            if outcome.result == MediationResult::Success ||
               outcome.result == MediationResult::ErrorConfig {
                return outcome;
            }

            attempts += 1;
            if attempts >= self.config.max_retries {
                return outcome;
            }

            // Use configured delay or exponential backoff
            let delay = self.config.retry_delays
                .get(attempts as usize - 1)
                .copied()
                .unwrap_or(Duration::from_secs(3));

            debug!(
                message_id = %message.id,
                attempt = attempts,
                delay_ms = delay.as_millis(),
                "Retrying mediation"
            );
            tokio::time::sleep(delay).await;
        }
    }
}

impl Default for HttpMediator {
    fn default() -> Self {
        Self::new()
    }
}

#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn test_circuit_breaker_trips() {
        let cb = CircuitBreaker::new(3, 2, Duration::from_secs(1));

        assert_eq!(cb.state(), CircuitState::Closed);
        assert!(cb.allow_request());

        // Record failures to trip
        cb.record_failure();
        cb.record_failure();
        assert_eq!(cb.state(), CircuitState::Closed);

        cb.record_failure();
        assert_eq!(cb.state(), CircuitState::Open);
        assert!(!cb.allow_request());
    }

    #[test]
    fn test_circuit_breaker_resets_on_success() {
        let cb = CircuitBreaker::new(3, 2, Duration::from_secs(1));

        cb.record_failure();
        cb.record_failure();
        assert_eq!(cb.failure_count(), 2);

        cb.record_success();
        assert_eq!(cb.failure_count(), 0);
    }
}
