//! `cargo run --example webhook-verify --features webhook`
//!
//! Verifies a delivery the way a receiver's HTTP handler would: the raw body
//! bytes, the two signature headers, then the answer. Framework-agnostic; in
//! axum or actix pass the header values and the body bytes the same way.

use std::time::{SystemTime, UNIX_EPOCH};

use flowcatalyst_sdk::webhook::{MediationResponse, WebhookError, WebhookVerifier, sign};

fn handle(
    verifier: &WebhookVerifier,
    body: &[u8],
    signature: Option<&str>,
    timestamp: Option<&str>,
) -> (u16, String) {
    match verifier.verify(body, signature, timestamp) {
        Ok(()) => (
            200,
            serde_json::to_string(&MediationResponse::ack()).unwrap_or_default(),
        ),
        Err(e @ (WebhookError::TimestampExpired | WebhookError::TimestampInFuture)) => {
            (401, format!("stale: {}", e.code()))
        }
        Err(e) => (401, e.code().to_owned()),
    }
}

fn main() {
    let secret = "the-service-account-signing-secret";
    let verifier = WebhookVerifier::new(secret);
    let now = SystemTime::now()
        .duration_since(UNIX_EPOCH)
        .map(|d| d.as_secs())
        .unwrap_or(0);
    let timestamp = now.to_string();
    let body =
        br#"{"id":"0HZXEQ5Y8JY5Z","type":"shop:orders:order:placed","attemptNumber":1,"data":{}}"#;
    let signature = sign(secret, &timestamp, body);

    println!(
        "{:?}",
        handle(&verifier, body, Some(&signature), Some(&timestamp))
    );
    println!(
        "{:?}",
        handle(&verifier, b"tampered", Some(&signature), Some(&timestamp))
    );
    println!("{:?}", handle(&verifier, body, None, Some(&timestamp)));
}
