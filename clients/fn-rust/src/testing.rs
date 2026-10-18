//! Test helpers for handler authors: run a request through the same path a
//! real invocation takes (frame in, route, handler, frame out) on the host
//! target, with host calls going to the fake host (`set_fake_host`).
//! Compiled out of `wasm32` builds.
//!
//! Define the registration as a function so the test and `export!` share it:
//!
//! ```ignore
//! fn register(app: &mut App) { app.open("GET /healthz", health); }
//! flowcatalyst_fn::export!(register);
//!
//! #[test]
//! fn healthz() {
//!     let r = testing::invoke(register, TestRequest::new("GET", "/healthz"));
//!     assert_eq!(r.status, 200);
//! }
//! ```

use crate::app::App;
use crate::dispatch::handle_frame;
use crate::frame::{decode_frame, encode_frame};
use crate::types::ResponseMeta;
use std::collections::BTreeMap;

/// One request for [`invoke`]. The caller defaults to anonymous (auth "none").
#[derive(Debug, Clone)]
pub struct TestRequest {
    pub method: String,
    pub path: String,
    pub raw_query: String,
    pub headers: BTreeMap<String, Vec<String>>,
    pub body: Vec<u8>,
    pub caller: serde_json::Value,
}

impl TestRequest {
    pub fn new(method: &str, path: &str) -> Self {
        Self {
            method: method.to_string(),
            path: path.to_string(),
            raw_query: String::new(),
            headers: BTreeMap::new(),
            body: Vec::new(),
            caller: serde_json::json!({ "kind": "anonymous" }),
        }
    }

    pub fn query(mut self, raw_query: &str) -> Self {
        self.raw_query = raw_query.to_string();
        self
    }

    pub fn header(mut self, name: &str, value: &str) -> Self {
        self.headers
            .entry(name.to_string())
            .or_default()
            .push(value.to_string());
        self
    }

    pub fn body(mut self, body: impl Into<Vec<u8>>) -> Self {
        self.body = body.into();
        self
    }

    /// A verified principal holding `permissions` (what a "platform"
    /// endpoint's handler sees).
    pub fn principal(mut self, id: &str, permissions: &[&str]) -> Self {
        self.caller = serde_json::json!({
            "kind": "principal", "id": id, "type": "SERVICE", "tier": "CLIENT",
            "permissions": permissions,
        });
        self
    }

    /// A signature-verified webhook delivery (what a "webhook" endpoint sees).
    pub fn webhook(mut self) -> Self {
        self.caller = serde_json::json!({ "kind": "webhook" });
        self
    }
}

/// What the handler (or the router's 404/405) produced.
#[derive(Debug, Clone)]
pub struct TestResponse {
    pub status: u16,
    pub headers: BTreeMap<String, Vec<String>>,
    pub body: Vec<u8>,
}

impl TestResponse {
    pub fn text(&self) -> String {
        String::from_utf8_lossy(&self.body).to_string()
    }

    pub fn json(&self) -> serde_json::Value {
        serde_json::from_slice(&self.body).unwrap_or(serde_json::Value::Null)
    }

    pub fn header(&self, name: &str) -> Option<&str> {
        self.headers
            .iter()
            .find(|(k, _)| k.eq_ignore_ascii_case(name))
            .and_then(|(_, v)| v.first())
            .map(|s| s.as_str())
    }
}

/// What a fake host closure returns (see `set_fake_host`).
pub use crate::host::HostResult;

/// A successful host reply: the result frame's meta (JSON text) and body.
pub fn host_ok(meta: &str, body: &[u8]) -> HostResult {
    Ok((meta.as_bytes().to_vec(), body.to_vec()))
}

/// A host refusal with `code` (see `error_code`) and `message`.
pub fn host_error(code: &str, message: &str) -> HostResult {
    Err(crate::host::HostOutcome::Host(crate::host::HostError {
        code: code.to_string(),
        message: message.to_string(),
    }))
}

fn build<F: FnOnce(&mut App)>(f: F) -> App {
    let mut app = App::new();
    f(&mut app);
    app
}

/// Runs `req` against the app `register` builds.
pub fn invoke<F: FnOnce(&mut App)>(register: F, req: TestRequest) -> TestResponse {
    let meta = serde_json::json!({
        "id": "inv_test", "address": "test.fn", "version": 1,
        "method": req.method, "path": req.path, "rawQuery": req.raw_query,
        "headers": req.headers, "caller": req.caller,
        "deadlineUnixMs": 4_102_444_800_000_i64,
    });
    let frame = encode_frame(&serde_json::to_vec(&meta).unwrap(), &req.body);
    let out = handle_frame(&build(register), &frame);
    let (rm, body) = decode_frame(&out).expect("response frame");
    let rm: ResponseMeta = serde_json::from_slice(rm).expect("response meta");
    TestResponse {
        status: rm.status,
        headers: rm.headers,
        body: body.to_vec(),
    }
}

/// The describe document `register` produces, as publish would read it.
pub fn describe<F: FnOnce(&mut App)>(register: F) -> serde_json::Value {
    serde_json::from_slice(&build(register).describe_json()).expect("describe json")
}

#[cfg(test)]
mod tests {
    use super::*;
    use crate::{Context, Error, Request, Response};

    fn who(_: &Request, ctx: &Context) -> Result<Response, Error> {
        let id = ctx
            .caller()
            .principal()
            .map(|p| p.id.clone())
            .unwrap_or_default();
        Ok(Response::new(202)
            .with_header("X-Id", id)
            .with_body(b"ok".to_vec()))
    }

    fn register(app: &mut App) {
        app.platform("POST /who", who).open("GET /h", who);
    }

    #[test]
    fn invoke_routes_with_a_principal() {
        let r = invoke(
            register,
            TestRequest::new("POST", "/who")
                .principal("prn_1", &["a:b:c:d"])
                .body("x"),
        );
        assert_eq!(
            (r.status, r.header("x-id"), r.text().as_str()),
            (202, Some("prn_1"), "ok")
        );
        assert_eq!(
            invoke(register, TestRequest::new("GET", "/nope")).status,
            404
        );
        assert_eq!(
            invoke(register, TestRequest::new("PUT", "/who")).status,
            405
        );
    }

    #[test]
    fn host_helpers_round_trip() {
        crate::set_fake_host(|op, _, _| {
            if op == 3 {
                host_ok(r#"{"found":true}"#, b"v")
            } else {
                host_error("UNAVAILABLE", "down")
            }
        });
        let ctx = crate::Context {
            caller: Default::default(),
            invocation: Default::default(),
        };
        assert_eq!(ctx.secret_get("K").unwrap().as_deref(), Some("v"));
        assert!(ctx.config_get("K").unwrap_err().is_retryable());
    }

    #[test]
    fn describe_lists_endpoints() {
        let d = describe(register);
        assert_eq!(d["endpoints"].as_array().unwrap().len(), 2);
    }
}
