//! The SDK produces the golden fixtures in `tests/fixtures/wire` byte for
//! byte. The same files are checked against the platform's Go decoders and
//! the Go SDK by `clients/rust-sdk/conformance/wire_test.go` in the platform
//! repository, so the two sides cannot drift apart silently.

#![cfg(all(feature = "outbox", feature = "webhook"))]
#![allow(
    clippy::unwrap_used,
    clippy::expect_used,
    clippy::panic,
    reason = "test helpers"
)]

use std::fs;

use flowcatalyst_sdk::outbox::{
    CreateAuditLogDto, CreateEventDto, DispatchJob, DispatchKind, DispatchMode, DispatchQueue,
};
use flowcatalyst_sdk::tsid;
use flowcatalyst_sdk::webhook::sign;
use serde::{Deserialize, Serialize};

fn fixture(name: &str) -> String {
    let path = format!("{}/tests/fixtures/wire/{name}", env!("CARGO_MANIFEST_DIR"));
    fs::read_to_string(&path)
        .unwrap_or_else(|e| panic!("read {path}: {e}"))
        .trim_end()
        .to_owned()
}

/// The framework's Java golden (`DispatchJobOutboxTest`): every field set.
#[test]
fn dispatch_job_full_matches_the_framework_golden() {
    let job = DispatchJob::builder(
        "orders:billing:export",
        "https://billing.example.com/hooks/export",
    )
    .id("0HZXEQ5Y8JY5Z")
    .kind(DispatchKind::Event)
    .source("orders:billing")
    .subject("billing.invoice.inv_1")
    .payload(r#"<invoice id="inv_1"/>"#, "application/xml")
    .data_only(true)
    .external_id("export-inv_1")
    .event_id("0HZXEQ5Y8JY50")
    .correlation_id("corr-42")
    .platform_client_id("0HZXEQ5Y8JY51")
    .subscription_id("0HZXEQ5Y8JY52")
    .service_account_id("0HZXEQ5Y8JY53")
    .dispatch_pool_id("0HZXEQ5Y8JY54")
    .message_group("invoice:inv_1")
    .mode(DispatchMode::BlockOnError)
    .sequence(2)
    .timeout_seconds(45)
    .max_retries(5)
    .metadata("invoice", "inv_1")
    .metadata("batch", "nightly")
    .queue(DispatchQueue::HighPriority)
    .descriptor("invoice export")
    .build()
    .unwrap();
    assert_eq!(job.to_payload().unwrap(), fixture("dispatch-job-full.json"));
}

#[test]
fn dispatch_job_minimal_writes_only_the_always_fields() {
    let job = DispatchJob::builder(
        "orders:billing:export",
        "https://billing.example.com/hooks/export",
    )
    .id("0HZXEQ5Y8JY5Z")
    .build()
    .unwrap();
    assert_eq!(
        job.to_payload().unwrap(),
        fixture("dispatch-job-minimal.json")
    );
}

#[derive(Serialize)]
#[serde(rename_all = "camelCase")]
struct OrderPlaced {
    order_id: &'static str,
    total: u32,
}

/// The Java SDK's `CreateEventDto.toPayload` for the same inputs.
#[test]
fn event_full_matches_the_java_sdk_shape() {
    let event = CreateEventDto::new(
        "orders:sales:order:created",
        &OrderPlaced {
            order_id: "ord_1",
            total: 1500,
        },
    )
    .unwrap()
    .source("orders")
    .subject("orders.order.ord_1")
    .correlation_id("corr-1")
    .causation_id("cause-1")
    .deduplication_id("0HZXEQ5Y8JY5Z")
    .message_group("orders:order:ord_1")
    .client_code("acme")
    .context("principalId", "prn_0HZXEQ5Y8JY5A")
    .context("executionId", "0HZXEQ5Y8JY5B");
    assert_eq!(event.to_payload().unwrap(), fixture("event-full.json"));
}

#[test]
fn event_minimal_matches_the_java_sdk_shape() {
    let event = CreateEventDto::new("orders:sales:order:created", &serde_json::json!({})).unwrap();
    assert_eq!(event.to_payload().unwrap(), fixture("event-minimal.json"));
}

/// The Java SDK's `CreateAuditLogDto.toPayload`, with operation data redacted.
#[test]
fn audit_log_full_matches_the_java_sdk_shape() {
    let at = chrono::DateTime::parse_from_rfc3339("2026-08-07T09:32:12.123Z")
        .unwrap()
        .with_timezone(&chrono::Utc);
    let audit = CreateAuditLogDto::new("Order", "ord_1", "PlaceOrder", "prn_0HZXEQ5Y8JY5A")
        .unwrap()
        .operation_data(&serde_json::json!({"total": 1500, "apiKey": "k-123"}))
        .unwrap()
        .performed_at(at)
        .source("orders")
        .correlation_id("corr-1")
        .application_code("orders")
        .client_code("acme")
        .metadata("channel", "web");
    assert_eq!(audit.to_payload().unwrap(), fixture("audit-log-full.json"));
}

#[derive(Deserialize)]
struct SignatureVector {
    name: String,
    secret: String,
    timestamp: String,
    body: String,
    signature: String,
}

#[test]
fn webhook_signature_vectors() {
    let vectors: Vec<SignatureVector> =
        serde_json::from_str(&fixture("webhook-signatures.json")).unwrap();
    assert!(vectors.len() >= 3);
    for v in vectors {
        assert_eq!(
            sign(&v.secret, &v.timestamp, v.body.as_bytes()),
            v.signature,
            "{}",
            v.name
        );
    }
}

#[derive(Deserialize)]
struct TsidVector {
    value: i64,
    encoded: String,
}

#[test]
fn tsid_vectors() {
    let vectors: Vec<TsidVector> = serde_json::from_str(&fixture("tsid-vectors.json")).unwrap();
    for v in vectors {
        assert_eq!(tsid::from_long(v.value), v.encoded);
        assert_eq!(tsid::to_long(&v.encoded), Some(v.value));
    }
}
