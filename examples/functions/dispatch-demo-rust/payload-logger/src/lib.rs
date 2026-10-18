//! payload-logger: the delivery target for dispatch-creator's jobs. The
//! platform POSTs each job's payload here (the job is dataOnly, so the body is
//! exactly what dispatch-creator was given), signed; the runner verifies the
//! signature (webhook auth) before this handler runs. It logs the payload and
//! acks.
//!
//! It declares no subscription: the job is created directly by dispatch-creator
//! (`ctx.dispatch`), which names this function's address and this path.

use flowcatalyst_fn::{App, Context, Error, Request, Response};

fn register(app: &mut App) {
    app.webhook("POST /deliver", deliver);
}

flowcatalyst_fn::export!(register);

fn deliver(req: &Request, ctx: &Context) -> Result<Response, Error> {
    ctx.log_info(&format!(
        "payload received: {} (invocation {})",
        String::from_utf8_lossy(&req.body),
        ctx.invocation().id
    ));
    Response::json(200, &serde_json::json!({ "ack": true }))
}

#[cfg(test)]
mod tests {
    use super::*;
    use flowcatalyst_fn::fake_host_calls;
    use flowcatalyst_fn::testing::{describe, host_ok, invoke, TestRequest};
    use flowcatalyst_fn::set_fake_host;

    #[test]
    fn describe_is_one_signed_webhook_and_nothing_else() {
        let d = describe(register);
        assert_eq!(d["endpoints"], serde_json::json!([{ "method": "POST", "path": "/deliver", "auth": "webhook" }]));
        // No subscription: jobs are dispatched to it directly by dispatch-creator.
        for key in ["subscriptions", "schedules", "emits", "dispatches"] {
            assert!(d.get(key).is_none(), "{key}");
        }
    }

    #[test]
    fn a_delivery_is_logged_and_acked() {
        set_fake_host(|_, _, _| host_ok("{}", b""));
        let r = invoke(register, TestRequest::new("POST", "/deliver").webhook().body(r#"{"order":42}"#));
        assert_eq!((r.status, r.json()), (200, serde_json::json!({ "ack": true })));
        let logged = fake_host_calls().into_iter().any(|c| {
            let m = String::from_utf8_lossy(&c.1).to_string();
            c.0 == 1 && m.contains("payload received") && m.contains("order")
        });
        assert!(logged);
    }
}
