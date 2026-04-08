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
use std::time::Duration;
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
            connect_timeout: Duration::from_secs(10),
        }
    }

    /// Create config for production mode (HTTP/2, long timeout)
    pub fn production() -> Self {
        Self::default()
    }
}

/// HTTP-based message mediator.
/// Circuit breaking is handled by the per-endpoint CircuitBreakerRegistry in ProcessPool.
pub struct HttpMediator {
    client: Client,
    config: HttpMediatorConfig,
    warning_service: Arc<WarningService>,
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

        info!(
            timeout_secs = config.timeout.as_secs(),
            http_version = ?config.http_version,
            "HttpMediator initialized"
        );

        Self { client, config, warning_service: Arc::new(WarningService::noop()) }
    }

    /// Set the warning service for generating configuration warnings
    pub fn with_warning_service(mut self, warning_service: Arc<WarningService>) -> Self {
        self.warning_service = warning_service;
        self
    }

    /// Set warning service after construction
    pub fn set_warning_service(&mut self, warning_service: Arc<WarningService>) {
        self.warning_service = warning_service;
    }

    /// Generate a configuration warning
    fn warn_config(&self, message_id: &str, target: &str, status_code: u16, description: &str) {
        let severity = if status_code == 501 {
            WarningSeverity::Critical
        } else {
            WarningSeverity::Error
        };
        self.warning_service.add_warning(
            WarningCategory::Configuration,
            severity,
            format!("HTTP {} {} for message {}: Target: {}", status_code, description, message_id, target),
            "HttpMediator".to_string(),
        );
    }

    async fn mediate_once(&self, message: &Message) -> MediationOutcome {
        if message.mediation_type != MediationType::HTTP {
            return MediationOutcome::error_config(
                0,
                format!("Unsupported mediation type: {:?}", message.mediation_type),
            );
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

                    // Parse response body for ack and delaySeconds
                    if let Ok(body) = response.text().await {
                        if let Ok(resp) = serde_json::from_str::<MediationResponse>(&body) {
                            if !resp.ack {
                                // Target says not ready yet - use custom delay if provided
                                let delay = resp.delay_seconds.unwrap_or(30);
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

                    debug!(
                        message_id = %message.id,
                        status_code = status_code,
                        "Message delivered successfully"
                    );
                    MediationOutcome::success()
                } else if status_code == 400 {
                    // Bad request - configuration error

                    warn!(
                        message_id = %message.id,
                        status_code = status_code,
                        "Bad request - configuration error"
                    );
                    self.warn_config(&message.id, &message.mediation_target, status_code, "Bad Request");
                    MediationOutcome::error_config(status_code, "HTTP 400: Bad request".to_string())
                } else if status_code == 401 || status_code == 403 {
                    // Auth errors - configuration error

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

                    warn!(
                        message_id = %message.id,
                        status_code = status_code,
                        "Endpoint not found"
                    );
                    self.warn_config(&message.id, &message.mediation_target, status_code, "Not Found");
                    MediationOutcome::error_config(status_code, "HTTP 404: Not found".to_string())
                } else if status_code == 429 {
                    // Too Many Requests — destination is healthy but throttling us.
                    // Returned as RateLimited (not ErrorProcess) so the pool nacks
                    // with the Retry-After delay WITHOUT recording a circuit-breaker
                    // failure or consuming the dispatch retry budget.

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
                    MediationOutcome::rate_limited(retry_after)
                } else if status_code == 501 {
                    // Not implemented - configuration error (CRITICAL)

                    warn!(
                        message_id = %message.id,
                        status_code = status_code,
                        "Not implemented"
                    );
                    self.warn_config(&message.id, &message.mediation_target, status_code, "Not Implemented");
                    MediationOutcome::error_config(status_code, "HTTP 501: Not implemented".to_string())
                } else if status.is_client_error() {
                    // Other 4xx - treat as config error (but NOT 429 which is handled above)

                    warn!(
                        message_id = %message.id,
                        status_code = status_code,
                        "Client error"
                    );
                    MediationOutcome::error_config(status_code, format!("HTTP {}: Client error", status_code))
                } else if status.is_server_error() {
                    // 5xx - Transient error, retry

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

            // Don't retry on success, config errors, or rate-limit responses.
            // For 429 we want the queue to apply the Retry-After delay rather
            // than blocking this worker on in-process backoff.
            if outcome.result == MediationResult::Success ||
               outcome.result == MediationResult::ErrorConfig ||
               outcome.result == MediationResult::RateLimited {
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

// Circuit breaker tests are in circuit_breaker_registry.rs
