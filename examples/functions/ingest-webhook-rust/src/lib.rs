//! ingest-webhook: a public endpoint for a third party that signs what it sends.
//! It verifies an HMAC-SHA256 signature over the raw body against a declared
//! secret, then creates one DISPATCH JOB that delivers the payload to a webhook
//! function of the same application (see `names.rs`).
//!
//! ```text
//! sender --POST /ingest  X-Signature: sha256=<hex>--> ingest-webhook
//!   --ctx.dispatch--> DISPATCH JOB (pool + connection declared below)
//!   --> <target function> POST /deliver
//! ```
//!
//! The endpoint is auth "none": the runner cannot know the sender's scheme, so
//! the signature is this function's whole authentication. A request that fails
//! it is answered 401 and nothing is dispatched.

mod names;

use flowcatalyst_fn::{App, Context, DispatchOpts, Error, Job, Request, Response};
use hmac::{Hmac, Mac};
use names::*;
use sha2::Sha256;
use std::collections::BTreeMap;

fn register(app: &mut App) {
    app.open("GET /healthz", health)
        .open(INGEST_PATH, ingest)
        .secret(SIGNING_SECRET_KEY)
        .dispatches_with(
            &target_address(),
            DELIVER_PATH,
            DispatchOpts::new(DISPATCH_CONNECTION).pool(DISPATCH_POOL),
        );
}

flowcatalyst_fn::export!(register);

fn health(_req: &Request, _ctx: &Context) -> Result<Response, Error> {
    Response::json(200, &serde_json::json!({ "status": "ok" }))
}

fn error(status: u16, code: &str) -> Result<Response, Error> {
    Response::json(status, &serde_json::json!({ "error": code }))
}

fn hex_decode(s: &str) -> Option<Vec<u8>> {
    if s.len() % 2 != 0 {
        return None;
    }
    (0..s.len())
        .step_by(2)
        .map(|i| u8::from_str_radix(s.get(i..i + 2)?, 16).ok())
        .collect()
}

/// True when `header` is `sha256=` + hex(HMAC-SHA256(secret, body)); the
/// comparison is constant-time (`verify_slice`).
fn valid_signature(secret: &str, body: &[u8], header: &str) -> bool {
    let Some(hex) = header.strip_prefix("sha256=") else {
        return false;
    };
    let Some(got) = hex_decode(hex) else {
        return false;
    };
    let Ok(mut mac) = Hmac::<Sha256>::new_from_slice(secret.as_bytes()) else {
        return false;
    };
    mac.update(body);
    mac.verify_slice(&got).is_ok()
}

fn ingest(req: &Request, ctx: &Context) -> Result<Response, Error> {
    // The runner has already enforced the 1 MiB body limit (413).
    let secret = match ctx.secret_get(SIGNING_SECRET_KEY) {
        Ok(Some(s)) if !s.is_empty() => s,
        other => {
            // Misconfiguration or platform trouble, not the sender's fault; say nothing more.
            ctx.log_error(&format!("signing secret unavailable (found: {})", matches!(other, Ok(Some(_)))));
            return error(500, "NOT_CONFIGURED");
        }
    };
    if !valid_signature(&secret, &req.body, req.header(SIGNATURE_HEADER).unwrap_or("")) {
        return error(401, "INVALID_SIGNATURE");
    }

    match serde_json::from_slice::<serde_json::Value>(&req.body) {
        Ok(v) if v.is_object() || v.is_array() => {}
        _ => return error(400, "PAYLOAD_MUST_BE_A_JSON_OBJECT_OR_ARRAY"),
    }

    let group = match req.header("X-Message-Group") {
        Some(g) if g.len() > 128 => return error(400, "INVALID_MESSAGE_GROUP"),
        Some(g) if !g.is_empty() => g.to_string(),
        _ => DEFAULT_MESSAGE_GROUP.to_string(),
    };
    let mut metadata = BTreeMap::new();
    metadata.insert("source".to_string(), "webhook".to_string());
    if let Some(id) = req.header("X-Delivery-Id") {
        metadata.insert("deliveryId".to_string(), id.to_string());
    }

    // The correlation id defaults to the request's X-Correlation-Id.
    match ctx.dispatch(Job {
        target: target_address(),
        path: DELIVER_PATH.to_string(),
        payload: req.body.clone(),
        message_group: group,
        descriptor: DESCRIPTOR.to_string(),
        metadata,
        ..Default::default()
    }) {
        Ok(job_id) => {
            ctx.log_info(&format!("payload dispatched: {job_id} ({} bytes)", req.body.len()));
            Response::json(202, &serde_json::json!({ "jobId": job_id }))
        }
        Err(e) if e.is_retryable() => Ok(Response::new(429).with_header("Retry-After", "5")),
        Err(e) => {
            ctx.log_error(&format!("dispatch refused: {e}"));
            error(502, "DISPATCH_REFUSED")
        }
    }
}

#[cfg(test)]
mod tests {
    use super::*;
    use flowcatalyst_fn::testing::{describe, host_error, host_ok, invoke, TestRequest, TestResponse};
    use flowcatalyst_fn::{error_code, fake_host_calls, set_fake_host};

    const SECRET: &str = "s3cr3t";

    fn sign_with(secret: &str, body: &str) -> String {
        let mut mac = Hmac::<Sha256>::new_from_slice(secret.as_bytes()).unwrap();
        mac.update(body.as_bytes());
        let hex: String = mac.finalize().into_bytes().iter().map(|b| format!("{b:02x}")).collect();
        format!("sha256={hex}")
    }

    /// The platform side: the signing secret (None = not set) and the dispatch result.
    fn platform(secret: Option<&'static str>, refusal: Option<&'static str>) {
        set_fake_host(move |op, _, _| match (op, secret, refusal) {
            (3, Some(s), _) => host_ok(r#"{"found":true}"#, s.as_bytes()),
            (3, None, _) => host_ok(r#"{"found":false}"#, b""),
            (11, _, Some(code)) => host_error(code, "x"),
            (11, _, None) => host_ok(r#"{"jobId":"djob_1"}"#, b""),
            _ => host_ok("{}", b""),
        });
    }

    fn send(body: &str, signature: Option<String>) -> TestResponse {
        let mut r = TestRequest::new("POST", "/ingest").body(body);
        if let Some(s) = signature {
            r = r.header("X-Signature", &s);
        }
        invoke(register, r)
    }

    fn dispatched() -> Option<(Vec<u8>, Vec<u8>)> {
        fake_host_calls().into_iter().find(|c| c.0 == 11).map(|c| (c.1, c.2))
    }

    #[test]
    fn describe_declares_secret_target_and_open_ingest() {
        let d = describe(register);
        assert_eq!(d["secrets"], serde_json::json!([SIGNING_SECRET_KEY]));
        let g = &d["dispatches"][0];
        assert_eq!(g["target"], target_address().as_str());
        assert_eq!((g["pool"].as_str(), g["connection"].as_str()), (Some(DISPATCH_POOL), Some(DISPATCH_CONNECTION)));
        let ep = d["endpoints"].as_array().unwrap().iter().find(|e| e["path"] == "/ingest").unwrap();
        assert_eq!(ep["auth"], "none");
    }

    #[test]
    fn a_signed_payload_is_dispatched_as_received() {
        platform(Some(SECRET), None);
        let body = r#"{"order": 42,  "note":"keep my bytes"}"#;
        let r = invoke(
            register,
            TestRequest::new("POST", "/ingest")
                .body(body)
                .header("X-Signature", &sign_with(SECRET, body))
                .header("X-Message-Group", "trip-7")
                .header("X-Delivery-Id", "d-1"),
        );
        assert_eq!((r.status, r.json()["jobId"].as_str()), (202, Some("djob_1")));
        let (meta, sent) = dispatched().expect("dispatch call");
        assert_eq!(sent, body.as_bytes());
        let meta = String::from_utf8(meta).unwrap();
        for want in [
            format!(r#""target":"{}""#, target_address()),
            r#""messageGroup":"trip-7""#.to_string(),
            r#""metadata":[{"key":"deliveryId","value":"d-1"},{"key":"source","value":"webhook"}]"#.to_string(),
        ] {
            assert!(meta.contains(&want), "{meta} lacks {want}");
        }
    }

    #[test]
    fn bad_signatures_are_refused_and_nothing_is_dispatched() {
        platform(Some(SECRET), None);
        let body = r#"{"a":1}"#;
        let good = sign_with(SECRET, body);
        let cases = [
            ("missing", None),
            ("no prefix", Some(good.trim_start_matches("sha256=").to_string())),
            ("not hex", Some("sha256=zz".to_string())),
            ("other body", Some(sign_with(SECRET, r#"{"a":2}"#))),
            ("other secret", Some(sign_with("nope", body))),
            ("truncated", Some(good[..20].to_string())),
            ("uppercase algo", Some(good.replacen("sha256", "SHA256", 1))),
        ];
        for (name, sig) in cases {
            assert_eq!(send(body, sig).status, 401, "{name}");
        }
        assert!(dispatched().is_none());
    }

    #[test]
    fn signed_but_not_json_is_400() {
        platform(Some(SECRET), None);
        for body in ["not json", "42", r#""s""#, ""] {
            assert_eq!(send(body, Some(sign_with(SECRET, body))).status, 400, "{body:?}");
        }
        assert!(dispatched().is_none());
    }

    #[test]
    fn a_missing_secret_is_500_not_an_open_door() {
        platform(None, None);
        assert_eq!(send("{}", Some(sign_with(SECRET, "{}"))).status, 500);
        assert_eq!(send("{}", None).status, 500);
        platform(Some(""), None);
        assert_eq!(send("{}", Some(sign_with("", "{}"))).status, 500);
        assert!(dispatched().is_none());
    }

    #[test]
    fn refusal_is_502_and_platform_trouble_asks_for_a_retry() {
        platform(Some(SECRET), Some(error_code::NOT_ALLOWED));
        assert_eq!(send("{}", Some(sign_with(SECRET, "{}"))).status, 502);
        platform(Some(SECRET), Some(error_code::UNAVAILABLE));
        let r = send("{}", Some(sign_with(SECRET, "{}")));
        assert_eq!((r.status, r.header("retry-after")), (429, Some("5")));
    }

    #[test]
    fn an_overlong_message_group_is_400() {
        platform(Some(SECRET), None);
        let r = invoke(
            register,
            TestRequest::new("POST", "/ingest")
                .body("{}")
                .header("X-Signature", &sign_with(SECRET, "{}"))
                .header("X-Message-Group", &"g".repeat(129)),
        );
        assert_eq!(r.status, 400);
    }
}
