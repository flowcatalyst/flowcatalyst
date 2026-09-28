//! Minimal FlowCatalyst function example for the Rust guest SDK: an open
//! health check, a platform-authenticated greeting that reads a declared
//! config value and the caller's principal, and a webhook endpoint that
//! emits an event. Build it to wasm with:
//!
//!   cargo build --release --target wasm32-unknown-unknown

use flowcatalyst_fn::{App, Context, Error, Event, Request, Response};

flowcatalyst_fn::export!(|app: &mut App| {
    app.open("GET /healthz", health)
        .platform("GET /hello/{name}", hello)
        .webhook("POST /events/greeting", on_greeting_event)
        .config("GREETING")
        .emits("hello:greeting:greeting:sent");
});

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
