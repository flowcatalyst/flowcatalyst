//! dispatch-creator: `POST /dispatch` with a JSON body (platform-authenticated)
//! creates one DISPATCH JOB that delivers that body to the payload-logger
//! function's `/deliver` webhook.
//!
//! A function holds no platform credential, so it cannot call the dispatch-job
//! API. It uses the `dispatch` capability instead: it declares the one
//! (function, path) it may dispatch to, names that target in `ctx.dispatch`, and
//! the runner asks the platform to create the job on its behalf. The platform
//! checks the target against this version's declared dispatches, requires it to
//! be a live function of the SAME application with a webhook endpoint at the
//! path, builds the delivery URL and signs it as the declared connection's
//! service account (the job carries the connection, so pausing it holds the jobs).

use flowcatalyst_fn::{App, Context, DispatchOpts, Error, Job, Request, Response};
use std::collections::BTreeMap;

/// The code of the application that owns BOTH functions: `APP_CODE=example
/// fcdev fn deploy ...` bakes it in at build time (default `dispatchdemo`).
const APP: &str = match option_env!("APP_CODE") {
    Some(a) => a,
    None => "dispatchdemo",
};
/// The webhook path payload-logger declares.
const DELIVER_PATH: &str = "/deliver";
/// The connection the jobs go through (required by every dispatches
/// declaration). Create it with the application's own service account so the
/// signature it puts on a delivery is the one payload-logger verifies. Pausing
/// it holds the jobs PENDING.
const CONNECTION_CODE: &str = "dispatch-demo-conn";

/// A function may only dispatch to a function of its own application.
fn logger_address() -> String {
    format!("{APP}.payload-logger")
}

/// A 4-segment `<application>:<context>:<resource>:<action>` code, carried by
/// a role of the application (see the README).
fn required_permission() -> String {
    format!("{APP}:dispatch:job:create")
}

fn register(app: &mut App) {
    app.open("GET /healthz", health)
        .platform("POST /dispatch", create)
        .dispatches_with(&logger_address(), DELIVER_PATH, DispatchOpts::new(CONNECTION_CODE));
}

flowcatalyst_fn::export!(register);

fn health(_req: &Request, _ctx: &Context) -> Result<Response, Error> {
    Response::json(200, &serde_json::json!({ "status": "ok" }))
}

fn error(status: u16, code: &str) -> Result<Response, Error> {
    Response::json(status, &serde_json::json!({ "error": code }))
}

fn create(req: &Request, ctx: &Context) -> Result<Response, Error> {
    // auth "platform": the runner has already verified the bearer token (401
    // otherwise). Authorisation is the function's own job: the verified
    // principal must hold the permission.
    let permission = required_permission();
    let principal = match ctx.caller().principal() {
        Some(p) if p.has_permission(&permission) => p,
        _ => {
            return Response::json(
                403,
                &serde_json::json!({ "error": "FORBIDDEN", "message": format!("requires permission {permission}") }),
            )
        }
    };

    match serde_json::from_slice::<serde_json::Value>(&req.body) {
        Err(_) => return error(400, "INVALID_JSON"),
        Ok(v) if !(v.is_object() || v.is_array()) => {
            return error(400, "PAYLOAD_MUST_BE_A_JSON_OBJECT_OR_ARRAY")
        }
        Ok(_) => {}
    }

    let mut metadata = BTreeMap::new();
    metadata.insert("requestedBy".to_string(), principal.id.clone());
    match ctx.dispatch(Job {
        target: logger_address(),
        path: DELIVER_PATH.to_string(),
        payload: req.body.clone(),
        message_group: "dispatch-demo".to_string(),
        descriptor: "dispatch-demo payload".to_string(),
        metadata,
        ..Default::default()
    }) {
        Ok(job_id) => {
            ctx.log_info(&format!("job dispatched: {job_id}"));
            Response::json(202, &serde_json::json!({ "jobId": job_id }))
        }
        Err(e) if e.is_retryable() => error(503, "PLATFORM_UNAVAILABLE"),
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

    fn platform(refusal: Option<&'static str>) {
        set_fake_host(move |op, _, _| match (op, refusal) {
            (11, Some(code)) => host_error(code, "x"),
            (11, None) => host_ok(r#"{"jobId":"djob_1"}"#, b""),
            _ => host_ok("{}", b""),
        });
    }

    fn post(permissions: &[&str], body: &str) -> TestResponse {
        invoke(register, TestRequest::new("POST", "/dispatch").principal("prn_1", permissions).body(body))
    }

    fn dispatched() -> Option<(Vec<u8>, Vec<u8>)> {
        fake_host_calls().into_iter().find(|c| c.0 == 11).map(|c| (c.1, c.2))
    }

    #[test]
    fn describe_declares_the_dispatch_target_and_emits_nothing() {
        let d = describe(register);
        let g = &d["dispatches"][0];
        assert_eq!(g["target"], logger_address().as_str());
        assert_eq!((g["path"].as_str(), g["connection"].as_str()), (Some("/deliver"), Some(CONNECTION_CODE)));
        assert!(d.get("emits").is_none());
        let eps = d["endpoints"].as_array().unwrap();
        assert!(eps.iter().any(|e| e["path"] == "/dispatch" && e["auth"] == "platform"));
    }

    #[test]
    fn names_derive_from_the_application_code() {
        if option_env!("APP_CODE").is_none() {
            assert_eq!(logger_address(), "dispatchdemo.payload-logger");
            assert_eq!(required_permission(), "dispatchdemo:dispatch:job:create");
        }
    }

    #[test]
    fn post_creates_a_job_carrying_the_payload() {
        platform(None);
        let r = post(&[&required_permission()], r#"{"order":42}"#);
        assert_eq!((r.status, r.json()["jobId"].as_str()), (202, Some("djob_1")));
        let (meta, body) = dispatched().expect("dispatch call");
        assert_eq!(body, br#"{"order":42}"#);
        let meta = String::from_utf8(meta).unwrap();
        for want in [
            format!(r#""target":"{}""#, logger_address()),
            r#""messageGroup":"dispatch-demo""#.to_string(),
            r#""metadata":[{"key":"requestedBy","value":"prn_1"}]"#.to_string(),
        ] {
            assert!(meta.contains(&want), "{meta} lacks {want}");
        }
    }

    #[test]
    fn bad_bodies_dispatch_nothing() {
        for (body, want) in [("not json", "INVALID_JSON"), ("", "INVALID_JSON"), ("42", "PAYLOAD_MUST_BE_A_JSON_OBJECT_OR_ARRAY")] {
            platform(None);
            let r = post(&[&required_permission()], body);
            assert_eq!((r.status, r.json()["error"].as_str()), (400, Some(want)));
            assert!(dispatched().is_none());
        }
    }

    #[test]
    fn refusal_is_502_and_platform_trouble_is_a_retryable_503() {
        platform(Some(error_code::NOT_ALLOWED));
        assert_eq!(post(&[&required_permission()], "{}").status, 502);
        platform(Some(error_code::UNAVAILABLE));
        assert_eq!(post(&[&required_permission()], "{}").status, 503);
    }

    #[test]
    fn without_the_permission_nothing_is_dispatched() {
        platform(None);
        assert_eq!(post(&["other:x:y:z"], "{}").status, 403);
        let anon = invoke(register, TestRequest::new("POST", "/dispatch").body("{}"));
        assert_eq!(anon.status, 403);
        assert!(dispatched().is_none());
    }
}
