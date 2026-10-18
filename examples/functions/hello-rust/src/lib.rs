//! Minimal FlowCatalyst function example for the Rust guest SDK: an open
//! health check, a platform-authenticated greeting that reads a declared
//! config value and the caller's principal, and a webhook endpoint that
//! emits an event. Build it to wasm with:
//!
//!   cargo build --release --target wasm32-unknown-unknown
//!
//! Tests run on the host: `cargo test`.

use flowcatalyst_fn::{App, Context, DispatchOpts, Error, Event, Job, Request, Response};

fn register(app: &mut App) {
    app.open("GET /healthz", health)
        .platform("GET /hello/{name}", hello)
        .webhook("POST /events/greeting", on_greeting_event)
        .open("POST /dispatch", on_dispatch)
        .config("GREETING")
        .emits("hello:greeting:greeting:sent")
        .dispatches_with("hello.logger", "/deliver", DispatchOpts::new("hello-conn").pool("hello-pool"));
}

flowcatalyst_fn::export!(register);

fn health(_req: &Request, _ctx: &Context) -> Result<Response, Error> {
    Ok(Response::new(200)
        .with_header("Content-Type", "application/json")
        .with_body(br#"{"status":"ok"}"#.to_vec()))
}

fn hello(req: &Request, ctx: &Context) -> Result<Response, Error> {
    let name = req.path_param("name").unwrap_or("");
    let greeting = match ctx.config_get("GREETING") {
        Ok(Some(v)) => v,
        Ok(None) => "Hello".to_string(),
        Err(e) => {
            ctx.log_error(&format!("config.get failed: {e}"));
            return Ok(Response::new(500)
                .with_header("Content-Type", "application/json")
                .with_body(br#"{"error":"CONFIG_UNAVAILABLE"}"#.to_vec()));
        }
    };

    let mut body = serde_json::json!({ "message": format!("{greeting}, {name}") });
    if let Some(p) = ctx.caller().principal() {
        body["caller"] = serde_json::json!(p.id);
    }
    Response::json(200, &body)
}

fn on_greeting_event(req: &Request, ctx: &Context) -> Result<Response, Error> {
    // Derive the dedup id from the delivered event's id, so a redelivery of
    // the same event emits the same follow-up exactly once.
    let inbound: serde_json::Value = serde_json::from_slice(&req.body).unwrap_or_default();
    let id = inbound.get("id").and_then(|v| v.as_str()).unwrap_or(&req.id);
    match ctx.emit(Event {
        event_type: "hello:greeting:greeting:sent".to_string(),
        dedup_id: format!("greeting-sent-{id}"),
        source: "hello.example".to_string(),
        content_type: "application/json".to_string(),
        data: br#"{"ok":true}"#.to_vec(),
        ..Default::default()
    }) {
        Ok(event_id) => {
            ctx.log_info(&format!("greeting event emitted: {event_id}"));
            Ok(Response::new(202))
        }
        Err(e) if e.is_retryable() => Ok(Response::new(429).with_header("Retry-After", "5")),
        Err(e) => {
            ctx.log_error(&format!("emit failed: {e}"));
            Ok(Response::new(422)
                .with_header("FlowCatalyst-Outcome", "reject")
                .with_body(b"emit refused".to_vec()))
        }
    }
}

/// Creates a dispatch job for the hello.logger function's /deliver webhook.
/// The target and path are declared above; the platform builds the URL and
/// signs the delivery, the function holds no credential.
fn on_dispatch(_req: &Request, ctx: &Context) -> Result<Response, Error> {
    let mut metadata = std::collections::BTreeMap::new();
    metadata.insert("from".to_string(), "hello".to_string());
    match ctx.dispatch(Job {
        target: "hello.logger".to_string(),
        path: "/deliver".to_string(),
        payload: br#"{"hello":"world"}"#.to_vec(),
        metadata,
        message_group: "hello".to_string(),
        ..Default::default()
    }) {
        Ok(job_id) => {
            ctx.log_info(&format!("job dispatched: {job_id}"));
            Response::json(202, &serde_json::json!({ "jobId": job_id }))
        }
        Err(e) if e.is_retryable() => Ok(Response::new(429).with_header("Retry-After", "5")),
        Err(e) => {
            ctx.log_error(&format!("dispatch failed: {e}"));
            Ok(Response::new(422)
                .with_header("FlowCatalyst-Outcome", "reject")
                .with_body(b"dispatch refused".to_vec()))
        }
    }
}

#[cfg(test)]
mod tests {
    use super::*;
    use flowcatalyst_fn::testing::{describe, host_error, host_ok, invoke, TestRequest};
    use flowcatalyst_fn::{error_code, fake_host_calls, set_fake_host};

    /// The platform side: config values, and the results of emit and dispatch.
    fn platform(greeting: Option<&'static str>) {
        set_fake_host(move |op, meta, _body| match op {
            2 => {
                assert!(String::from_utf8_lossy(meta).contains("GREETING"));
                match greeting {
                    Some(g) => host_ok(r#"{"found":true}"#, g.as_bytes()),
                    None => host_ok(r#"{"found":false}"#, b""),
                }
            }
            5 => host_ok(r#"{"eventId":"evt_1"}"#, b""),
            11 => host_ok(r#"{"jobId":"djob_1"}"#, b""),
            _ => host_ok("{}", b""),
        });
    }

    #[test]
    fn describe_declares_the_dispatch_target() {
        let d = describe(register);
        assert_eq!(d["endpoints"].as_array().unwrap().len(), 4);
        assert_eq!(d["emits"][0], "hello:greeting:greeting:sent");
        let g = &d["dispatches"][0];
        assert_eq!(
            (g["target"].as_str(), g["pool"].as_str(), g["connection"].as_str()),
            (Some("hello.logger"), Some("hello-pool"), Some("hello-conn"))
        );
    }

    #[test]
    fn healthz() {
        platform(None);
        let r = invoke(register, TestRequest::new("GET", "/healthz"));
        assert_eq!((r.status, r.text().as_str()), (200, r#"{"status":"ok"}"#));
    }

    #[test]
    fn hello_uses_config_and_principal() {
        platform(Some("Kia ora"));
        let r = invoke(register, TestRequest::new("GET", "/hello/Aroha").principal("prn_1", &[]));
        assert_eq!(r.status, 200);
        assert_eq!(r.json()["message"], "Kia ora, Aroha");
        assert_eq!(r.json()["caller"], "prn_1");
    }

    #[test]
    fn hello_defaults_without_config() {
        platform(None);
        let r = invoke(register, TestRequest::new("GET", "/hello/Aroha").principal("prn_1", &[]));
        assert_eq!(r.json()["message"], "Hello, Aroha");
    }

    #[test]
    fn webhook_emits_keyed_by_the_event_id() {
        platform(None);
        let r = invoke(
            register,
            TestRequest::new("POST", "/events/greeting").webhook().body(r#"{"id":"evt_9"}"#),
        );
        assert_eq!(r.status, 202);
        let emit = fake_host_calls().into_iter().find(|c| c.0 == 5).expect("emit call");
        assert!(String::from_utf8_lossy(&emit.1).contains(r#""dedupId":"greeting-sent-evt_9""#));
    }

    #[test]
    fn dispatch_sends_the_payload_and_array_metadata() {
        platform(None);
        let r = invoke(register, TestRequest::new("POST", "/dispatch"));
        assert_eq!((r.status, r.json()["jobId"].as_str()), (202, Some("djob_1")));
        let call = fake_host_calls().into_iter().find(|c| c.0 == 11).expect("dispatch call");
        assert_eq!(call.2, br#"{"hello":"world"}"#);
        assert!(String::from_utf8_lossy(&call.1).contains(r#""metadata":[{"key":"from","value":"hello"}]"#));
    }

    #[test]
    fn dispatch_refusal_rejects_and_platform_trouble_retries() {
        for (code, want) in [(error_code::NOT_ALLOWED, 422), (error_code::UNAVAILABLE, 429)] {
            set_fake_host(move |op, _, _| if op == 11 { host_error(code, "x") } else { host_ok("{}", b"") });
            assert_eq!(invoke(register, TestRequest::new("POST", "/dispatch")).status, want, "{code}");
        }
    }
}
