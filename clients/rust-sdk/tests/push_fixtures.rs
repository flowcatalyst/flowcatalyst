//! The shared push vectors (pinned copies of the framework's
//! `conformance/outbox/outbox-push.json` and `conformance/push/tickets.json`,
//! which the Go, TypeScript and Laravel SDKs pin as well): the PUSH rows the
//! writer produces, the refusals of the builder, and tickets the helper
//! reproduces byte for byte.

#![cfg(all(feature = "outbox", feature = "push-tickets"))]
#![allow(
    clippy::unwrap_used,
    clippy::expect_used,
    clippy::panic,
    reason = "test helpers"
)]

use std::fs;
use std::path::Path;
use std::sync::Mutex;

use base64::Engine as _;
use base64::engine::general_purpose::URL_SAFE_NO_PAD;
use flowcatalyst_sdk::outbox::{
    CreatePush, OutboxDriver, OutboxError, OutboxManager, OutboxMessage,
};
use flowcatalyst_sdk::push::{MintTicket, TicketKey, TicketMinter};
use serde_json::Value;

fn read(name: &str) -> String {
    let path = format!("{}/tests/fixtures/push/{name}", env!("CARGO_MANIFEST_DIR"));
    fs::read_to_string(&path).unwrap_or_else(|e| panic!("read {path}: {e}"))
}

fn vector(name: &str) -> Value {
    serde_json::from_str(&read(name)).unwrap()
}

#[derive(Default)]
struct Recording(Mutex<Vec<OutboxMessage>>);

impl OutboxDriver for Recording {
    async fn insert(&self, messages: &[OutboxMessage]) -> Result<(), OutboxError> {
        self.0.lock().unwrap().extend_from_slice(messages);
        Ok(())
    }
}

/// `data` of a vector push as the vector spells it (member order kept).
#[derive(serde::Deserialize)]
struct RawCases {
    cases: Vec<RawCase>,
}
#[derive(serde::Deserialize)]
struct RawCase {
    pushes: Vec<RawPush>,
}
#[derive(serde::Deserialize)]
struct RawPush {
    data: Option<Box<serde_json::value::RawValue>>,
}

fn build(v: &Value, raw: Option<&str>) -> CreatePush {
    let topic = v["topic"].as_str().unwrap();
    let ty = v["type"].as_str().unwrap();
    let mut p = if let Some(text) = v.get("dataText").and_then(Value::as_str) {
        CreatePush::from_json_text(topic, ty, text)
    } else if let Some(pad) = v.get("padBytes").and_then(Value::as_u64) {
        CreatePush::new(
            topic,
            ty,
            &serde_json::json!({"blob": "x".repeat(pad as usize)}),
        )
    } else {
        CreatePush::from_json_text(
            topic,
            ty,
            raw.map_or_else(|| v["data"].to_string(), str::to_owned),
        )
    };
    if let Some(id) = v.get("id").and_then(Value::as_str) {
        p = p.id(id);
    }
    if let Some(c) = v.get("clientCode").and_then(Value::as_str) {
        p = p.client_code(c);
    }
    if v.get("ungrouped").and_then(Value::as_bool) == Some(true) {
        p = p.ungrouped();
    }
    if let Some(g) = v.get("messageGroup").and_then(Value::as_str) {
        p = p.message_group(g);
    }
    p
}

#[tokio::test]
async fn the_push_rows_are_the_vectors_rows() {
    let v = vector("outbox-push.json");
    let raw: RawCases = serde_json::from_str(&read("outbox-push.json")).unwrap();
    let cases = v["cases"].as_array().unwrap();
    assert!(!cases.is_empty());
    for (c, rc) in cases.iter().zip(&raw.cases) {
        let name = c["name"].as_str().unwrap();
        let client = c["context"]["client"].as_str().unwrap();
        let m = OutboxManager::new(Recording::default(), client)
            .unwrap()
            .with_client_code(client);
        for (p, rp) in c["pushes"].as_array().unwrap().iter().zip(&rc.pushes) {
            let raw = rp.data.as_ref().map(|d| d.get());
            m.create_push(&build(p, raw)).await.unwrap();
        }
        let want: Vec<&Value> = c["rows"]
            .as_array()
            .unwrap()
            .iter()
            .filter(|r| r["type"] == "PUSH")
            .collect();
        let got = m.driver().0.lock().unwrap().clone();
        assert_eq!(got.len(), want.len(), "{name}: row count");
        for (g, w) in got.iter().zip(want) {
            assert_eq!(g.id.as_str(), w["id"].as_str().unwrap(), "{name}: id");
            assert_eq!(g.message_type.as_str(), "PUSH");
            assert_eq!(
                g.message_group.as_deref(),
                w["message_group"].as_str(),
                "{name}: message_group"
            );
            assert_eq!(g.status_code(), w["status"].as_i64().unwrap() as i16);
            assert_eq!(
                g.client_id,
                w["client_id"].as_str().unwrap(),
                "{name}: client_id"
            );
            assert!(w["headers"].is_null());
            assert_eq!(g.headers, None);
            assert_eq!(
                g.payload,
                w["payloadText"].as_str().unwrap(),
                "{name}: payload bytes, members in order"
            );
            assert_eq!(g.payload_size as i64, w["payload_size"].as_i64().unwrap());
        }
    }
}

#[tokio::test]
async fn the_builder_refuses_the_vectors_invalid_pushes() {
    let v = vector("outbox-push.json");
    let invalid = v["invalidPushes"].as_array().unwrap();
    assert!(invalid.len() >= 16);
    for bad in invalid {
        let want = bad["code"].as_str().unwrap();
        let m = OutboxManager::new(Recording::default(), "cli_1").unwrap();
        let err = m.create_push(&build(bad, None)).await.unwrap_err();
        match err {
            OutboxError::Push(e) => assert_eq!(e.code.as_str(), want, "{bad}"),
            other => panic!("{bad}: {other}"),
        }
        assert!(
            m.driver().0.lock().unwrap().is_empty(),
            "{bad}: a refused push left a row"
        );
    }
}

#[test]
fn the_pinned_vectors_are_the_framework_copies() {
    // Compared when the framework checkout is beside this repository (or
    // FC_FRAMEWORK_DIR names it), so a vector change there fails here.
    let root = std::env::var("FC_FRAMEWORK_DIR").unwrap_or_else(|_| {
        format!(
            "{}/../../../flowcatalyst-framework",
            env!("CARGO_MANIFEST_DIR")
        )
    });
    let conf = Path::new(&root).join("conformance");
    for (rel, name) in [
        ("outbox/outbox-push.json", "outbox-push.json"),
        ("push/tickets.json", "tickets.json"),
    ] {
        let f = conf.join(rel);
        if f.is_file() {
            assert_eq!(
                fs::read(&f).unwrap(),
                read(name).into_bytes(),
                "tests/fixtures/push/{name} differs from the framework's: copy it again"
            );
        }
    }
}

// ── tickets ─────────────────────────────────────────────────────────────

fn decode(token: &str) -> Option<(Value, Value)> {
    let mut it = token.split('.');
    let (h, c, _s) = (it.next()?, it.next()?, it.next()?);
    let part = |s: &str| serde_json::from_slice::<Value>(&URL_SAFE_NO_PAD.decode(s).ok()?).ok();
    Some((part(h)?, part(c)?))
}

#[test]
fn the_jwks_from_the_configured_keys_is_the_vectors() {
    let v = vector("tickets.json");
    let k = &v["keys"];
    let cfg = |n: &str| {
        format!(
            "{}:{}",
            k[n]["kid"].as_str().unwrap(),
            k[n]["seed"].as_str().unwrap()
        )
    };
    let m = TicketMinter::from_config(
        v["application"].as_str().unwrap(),
        &cfg("current"),
        &[&cfg("previous")],
    )
    .unwrap();
    assert_eq!(serde_json::to_value(m.jwks().unwrap()).unwrap(), v["jwks"]);
}

#[test]
fn the_helper_reproduces_the_vectors_tickets_byte_for_byte() {
    let v = vector("tickets.json");
    let seeds: Vec<&str> = ["current", "previous", "stranger"]
        .iter()
        .map(|n| v["keys"][*n]["seed"].as_str().unwrap())
        .collect();
    let (mut reproduced, mut refused) = (0, Vec::new());
    for c in v["cases"].as_array().unwrap() {
        for (i, token) in c["tickets"].as_array().unwrap().iter().enumerate() {
            let token = token.as_str().unwrap();
            let Some((header, claims)) = decode(token) else {
                continue;
            };
            if header["alg"] != "EdDSA" {
                continue;
            }
            let reason = c["expect"]["ticketReasons"]
                .get(i)
                .cloned()
                .unwrap_or(Value::Null);
            let mut minted = None;
            for seed in &seeds {
                let key = TicketKey::from_seed(header["kid"].as_str().unwrap(), seed).unwrap();
                let m = TicketMinter::new(claims["iss"].as_str().unwrap(), key, vec![]).unwrap();
                let mut t = MintTicket::new(
                    claims["client"].as_str().unwrap(),
                    claims["topics"]
                        .as_array()
                        .unwrap()
                        .iter()
                        .map(|x| x.as_str().unwrap().to_owned()),
                )
                .now(claims["iat"].as_i64().unwrap());
                let ttl = claims["exp"].as_i64().unwrap() - claims["iat"].as_i64().unwrap();
                if !(1..=300).contains(&ttl) {
                    refused.push(reason.clone());
                    break;
                }
                t = t.ttl_seconds(ttl as u32);
                if let Some(s) = claims.get("sub").and_then(Value::as_str) {
                    t = t.principal(s);
                }
                if let Some(j) = claims["cnf"]["jkt"].as_str() {
                    t = t.jkt(j);
                }
                if let Some(se) = claims.get("stream-exp").and_then(Value::as_i64) {
                    t = t.stream_exp(se);
                }
                match m.mint(&t) {
                    Ok(out) if out == token => {
                        minted = Some(out);
                        break;
                    }
                    Ok(_) => {}
                    Err(_) => {
                        refused.push(reason.clone());
                        break;
                    }
                }
            }
            if minted.is_some() {
                reproduced += 1;
            } else if reason.is_null() {
                panic!("{}: a valid ticket is not reproduced", c["name"]);
            }
        }
    }
    assert!(reproduced >= 12, "reproduced {reproduced}");
    assert!(refused.iter().any(|r| r == "times"), "{refused:?}");
    assert!(
        refused.iter().any(|r| r == "topic-outside-iss"),
        "{refused:?}"
    );
}

#[test]
fn ticket_helper_rules() {
    let seed = "5BgJptHbkz4uIeRqgLz3Ws_38Ovwi1uCpSdyW45yMdc";
    let m = TicketMinter::from_config("shop", &format!("k1:{seed}"), &[]).unwrap();
    let ok = |topics: Vec<String>| MintTicket::new("acme", topics);
    let one = || vec!["shop:orders:{id}".to_owned()];
    assert!(m.mint(&ok(vec!["orders:order:{id}".into()])).is_err());
    assert!(m.mint(&ok(vec!["shop".into()])).is_err());
    assert!(m.mint(&ok(vec![])).is_err());
    let many = |n: usize| (0..n).map(|i| format!("shop:t:{i}")).collect::<Vec<_>>();
    assert!(m.mint(&ok(many(101))).is_err());
    assert!(m.mint(&ok(many(100))).is_ok());
    assert!(m.mint(&ok(one()).ttl_seconds(301)).is_err());
    assert!(m.mint(&ok(one()).ttl_seconds(0)).is_err());
    assert!(m.mint(&ok(one()).ttl_seconds(300)).is_ok());
    assert!(m.mint(&MintTicket::new("a b", one())).is_err());
    assert!(TicketKey::parse("noseed").is_err());
    assert!(TicketKey::parse(":abc").is_err());
    assert!(TicketKey::parse("k:AAAA").is_err());
    let t = m.mint(&ok(one()).now(1000)).unwrap();
    let (h, c) = decode(&t).unwrap();
    assert_eq!(c["exp"].as_i64().unwrap() - c["iat"].as_i64().unwrap(), 60);
    assert_eq!(h["alg"], "EdDSA");
    let jwk = serde_json::to_value(&m.jwks().unwrap().keys[0]).unwrap();
    let mut names: Vec<_> = jwk.as_object().unwrap().keys().cloned().collect();
    names.sort();
    assert_eq!(names, ["alg", "crv", "kid", "kty", "use", "x"]);
}
