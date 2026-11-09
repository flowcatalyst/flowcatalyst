//! The EVENT rows of the shared outbox vector (the Go SDK's `outbox.json`, read in place;
//! the framework's `conformance/outbox/outbox.json` is the same file) carry the event's id as
//! `deduplicationId`. The Rust SDK writes the row id there unless the event sets one.
//!
//! `serde_json::Value` sorts object keys here, so the `data` text is compared decoded; the
//! outer members' order is checked on the emitted text.

#![cfg(feature = "outbox")]
#![allow(
    clippy::unwrap_used,
    clippy::expect_used,
    clippy::panic,
    reason = "test helpers"
)]

use std::fs;
use std::path::Path;

use flowcatalyst_sdk::outbox::CreateEventDto;
use serde_json::Value;

fn text<'a>(v: &'a Value, k: &str) -> Option<&'a str> {
    v.get(k).and_then(Value::as_str).filter(|s| !s.is_empty())
}

fn build(ev: &Value, ctx: &Value) -> CreateEventDto {
    let ty = ev["type"].as_str().unwrap();
    let seg: Vec<&str> = ty.split(':').collect();
    let subject_id = ev["subjectId"].as_str().unwrap();
    let data: serde_json::Map<String, Value> = ev["fields"]
        .as_object()
        .unwrap()
        .iter()
        .filter(|(_, v)| !v.is_null())
        .map(|(k, v)| (k.clone(), v.clone()))
        .collect();
    let group = text(ev, "messageGroup")
        .map(str::to_owned)
        .unwrap_or_else(|| format!("{}:{}:{subject_id}", seg[1], seg[2]));
    let mut e = CreateEventDto::new(ty, &Value::Object(data))
        .unwrap()
        .source(format!("{}:{}", seg[0], seg[1]))
        .subject(format!("{}.{}.{subject_id}", seg[1], seg[2]))
        .message_group(group);
    if let Some(v) = text(ctx, "correlation") {
        e = e.correlation_id(v);
    }
    if let Some(v) = text(ctx, "causation") {
        e = e.causation_id(v);
    }
    if let Some(v) = text(ctx, "client") {
        e = e.client_code(v);
    }
    if let Some(v) = text(ctx, "principal") {
        e = e.context("principalId", v);
    }
    if let Some(v) = text(ctx, "execution") {
        e = e.context("executionId", v);
    }
    e
}

/// The payload with its embedded `data` text decoded.
fn decoded(payload: &str) -> Value {
    let mut v: Value = serde_json::from_str(payload).unwrap();
    let data: Value = serde_json::from_str(v["data"].as_str().unwrap()).unwrap();
    v["data"] = data;
    v
}

#[test]
fn the_vectors_event_rows_are_reproduced() {
    let path = Path::new(env!("CARGO_MANIFEST_DIR"))
        .join("../../pkg/fcsdk/outbox/testdata/conformance/outbox.json");
    if !path.exists() {
        eprintln!("SDK mirrored without the Go SDK: skipped");
        return;
    }
    let vector: Value = serde_json::from_str(&fs::read_to_string(&path).unwrap()).unwrap();
    let mut checked = 0;
    for case in vector["cases"].as_array().unwrap() {
        let want: Vec<&Value> = case["rows"]
            .as_array()
            .unwrap()
            .iter()
            .filter(|r| r["type"] == "EVENT")
            .collect();
        let events = case["events"].as_array().unwrap();
        assert_eq!(want.len(), events.len());
        for (ev, row) in events.iter().zip(want) {
            let id = ev["eventId"].as_str().unwrap();
            let want_text = row["payloadText"].as_str().unwrap();
            let got = build(ev, &case["context"]).to_payload_for_row(id).unwrap();
            assert_eq!(decoded(&got), decoded(want_text), "{}", case["name"]);
            // Same members in the same order.
            let order = |s: &str| -> Vec<usize> {
                [
                    "\"specVersion\"",
                    "\"type\"",
                    "\"source\"",
                    "\"subject\"",
                    "\"data\"",
                    "\"correlationId\"",
                    "\"causationId\"",
                    "\"deduplicationId\"",
                    "\"messageGroup\"",
                    "\"clientCode\"",
                    "\"contextData\"",
                ]
                .iter()
                .map(|k| s.find(k).unwrap_or(usize::MAX))
                .collect()
            };
            let (a, b) = (order(&got), order(want_text));
            let rank = |o: &[usize]| {
                let mut i: Vec<usize> = (0..o.len()).filter(|&i| o[i] != usize::MAX).collect();
                i.sort_by_key(|&i| o[i]);
                i
            };
            assert_eq!(rank(&a), rank(&b));
            assert!(got.contains(&format!("\"deduplicationId\":\"{id}\"")));

            // An explicit value is written as given.
            let explicit = build(ev, &case["context"])
                .deduplication_id(id)
                .to_payload_for_row("another-row")
                .unwrap();
            assert_eq!(explicit, got);
            checked += 1;
        }
    }
    assert!(checked >= 4, "{checked} EVENT rows checked");
}
