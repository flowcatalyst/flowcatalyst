//! The declaration builder (plan §9): `App` accumulates endpoints,
//! subscriptions, schedules and declared config/secret/db/http-allow/emits,
//! then produces both the describe document (plan §5.4) and the router used
//! to dispatch a request frame.

use crate::context::Context;
use crate::route::{compile_pattern, match_segments, split_path, Seg};
use crate::types::{Error, Request, Response};
use std::collections::BTreeMap;

/// A registered handler function. Guest handlers are plain `fn` items (no
/// captured state), matching the macro-generated, per-call `App` rebuild.
pub type Handler = fn(&Request, &Context) -> Result<Response, Error>;

#[derive(Debug, Clone, Copy, PartialEq, Eq)]
pub(crate) enum Auth {
    Webhook,
    Platform,
    None,
}

impl Auth {
    fn as_str(self) -> &'static str {
        match self {
            Auth::Webhook => "webhook",
            Auth::Platform => "platform",
            Auth::None => "none",
        }
    }
}

pub(crate) struct Endpoint {
    pub method: String,
    pub segs: Vec<Seg>,
    pub path: String,
    pub auth: Auth,
    pub handler: Handler,
}

pub(crate) struct Sub {
    pub event_type: String,
    pub path: String,
}

pub(crate) struct Sched {
    pub cron: String,
    pub path: String,
}

/// The declaration builder. Guest code populates it once per `fc_handle`/
/// `fc_describe` call inside the closure passed to `flowcatalyst_fn::export!`.
#[derive(Default)]
pub struct App {
    pub(crate) endpoints: Vec<Endpoint>,
    pub(crate) subs: Vec<Sub>,
    pub(crate) scheds: Vec<Sched>,
    pub(crate) config: Vec<String>,
    pub(crate) secret: Vec<String>,
    pub(crate) db: Vec<String>,
    pub(crate) http_allow: Vec<String>,
    pub(crate) emits: Vec<String>,
}

impl App {
    pub fn new() -> Self {
        Self::default()
    }

    fn register(&mut self, auth: Auth, pattern: &str, handler: Handler) -> &mut Self {
        let (method, segs, path) = compile_pattern(pattern);
        if auth == Auth::Webhook && method != "POST" {
            panic!("fn: webhook endpoint {pattern:?} must be POST-only, got method {method:?}");
        }
        self.endpoints.push(Endpoint {
            method,
            segs,
            path,
            auth,
            handler,
        });
        self
    }

    /// Declares a webhook-authenticated endpoint (signature verified by the
    /// runner). Must be POST-only; any other method panics.
    pub fn webhook(&mut self, pattern: &str, handler: Handler) -> &mut Self {
        self.register(Auth::Webhook, pattern, handler)
    }

    /// Declares a platform-authenticated endpoint.
    pub fn platform(&mut self, pattern: &str, handler: Handler) -> &mut Self {
        self.register(Auth::Platform, pattern, handler)
    }

    /// Declares an endpoint with no authentication.
    pub fn open(&mut self, pattern: &str, handler: Handler) -> &mut Self {
        self.register(Auth::None, pattern, handler)
    }

    /// Declares that this function handles `event_type` deliveries at
    /// `path`, which must name a webhook endpoint.
    pub fn subscribe(&mut self, event_type: &str, path: &str) -> &mut Self {
        self.subs.push(Sub {
            event_type: event_type.to_string(),
            path: path.to_string(),
        });
        self
    }

    /// Declares a cron-triggered firing at `path`.
    pub fn schedule(&mut self, cron: &str, path: &str) -> &mut Self {
        self.scheds.push(Sched {
            cron: cron.to_string(),
            path: path.to_string(),
        });
        self
    }

    pub fn config(&mut self, key: &str) -> &mut Self {
        self.config.push(key.to_string());
        self
    }

    pub fn secret(&mut self, key: &str) -> &mut Self {
        self.secret.push(key.to_string());
        self
    }

    pub fn db(&mut self, name: &str) -> &mut Self {
        self.db.push(name.to_string());
        self
    }

    pub fn http_allow(&mut self, host: &str) -> &mut Self {
        self.http_allow.push(host.to_string());
        self
    }

    pub fn emits(&mut self, event_type: &str) -> &mut Self {
        self.emits.push(event_type.to_string());
        self
    }

    /// Matches `method`/`path` against every registered endpoint. Returns
    /// `RouteMatch::Found` on a full match, `MethodMismatch` if some
    /// endpoint's path matches but not its method (-> 405), or `NoMatch`
    /// (-> 404).
    pub(crate) fn route(&self, method: &str, path: &str) -> RouteMatch<'_> {
        let path_segs = split_path(path);
        let mut path_matched = false;
        for ep in &self.endpoints {
            if let Some(params) = match_segments(&ep.segs, &path_segs) {
                if ep.method == method {
                    return RouteMatch::Found(ep, params);
                }
                path_matched = true;
            }
        }
        if path_matched {
            RouteMatch::MethodMismatch
        } else {
            RouteMatch::NoMatch
        }
    }

    /// Builds the describe document (plan §5.4). Must not call any host
    /// import, and doesn't: it only reads in-memory registration state.
    pub(crate) fn describe_json(&self) -> Vec<u8> {
        let endpoints: Vec<serde_json::Value> = self
            .endpoints
            .iter()
            .map(|e| {
                serde_json::json!({
                    "method": e.method,
                    "path": e.path,
                    "auth": e.auth.as_str(),
                })
            })
            .collect();

        let mut doc = serde_json::Map::new();
        doc.insert("abi".into(), serde_json::json!(1));
        doc.insert("endpoints".into(), serde_json::Value::Array(endpoints));

        if !self.subs.is_empty() {
            let subs: Vec<_> = self
                .subs
                .iter()
                .map(|s| serde_json::json!({"eventType": s.event_type, "path": s.path}))
                .collect();
            doc.insert("subscriptions".into(), serde_json::Value::Array(subs));
        }
        if !self.scheds.is_empty() {
            let scheds: Vec<_> = self
                .scheds
                .iter()
                .map(|s| serde_json::json!({"cron": s.cron, "path": s.path}))
                .collect();
            doc.insert("schedules".into(), serde_json::Value::Array(scheds));
        }
        insert_if_nonempty(&mut doc, "config", &self.config);
        insert_if_nonempty(&mut doc, "secrets", &self.secret);
        insert_if_nonempty(&mut doc, "db", &self.db);
        insert_if_nonempty(&mut doc, "httpAllow", &self.http_allow);
        insert_if_nonempty(&mut doc, "emits", &self.emits);

        serde_json::to_vec(&serde_json::Value::Object(doc)).expect("fn: describe: serialization is infallible for this shape")
    }
}

fn insert_if_nonempty(doc: &mut serde_json::Map<String, serde_json::Value>, key: &str, values: &[String]) {
    if !values.is_empty() {
        doc.insert(key.to_string(), serde_json::json!(values));
    }
}

pub(crate) enum RouteMatch<'a> {
    Found(&'a Endpoint, BTreeMap<String, String>),
    MethodMismatch,
    NoMatch,
}

#[cfg(test)]
mod tests {
    use super::*;
    use crate::types::Response;

    fn ok(_req: &Request, _ctx: &Context) -> Result<Response, Error> {
        Ok(Response::new(200))
    }

    #[test]
    fn webhook_requires_post() {
        let result = std::panic::catch_unwind(|| {
            let mut app = App::new();
            app.webhook("GET /events/x", ok);
        });
        assert!(result.is_err());
    }

    #[test]
    fn webhook_post_ok() {
        let mut app = App::new();
        app.webhook("POST /events/x", ok);
        assert_eq!(app.endpoints.len(), 1);
    }

    #[test]
    fn route_found() {
        let mut app = App::new();
        app.open("GET /hello/{name}", ok);
        match app.route("GET", "/hello/world") {
            RouteMatch::Found(ep, params) => {
                assert_eq!(ep.path, "/hello/{name}");
                assert_eq!(params.get("name").map(String::as_str), Some("world"));
            }
            _ => panic!("expected a match"),
        }
    }

    #[test]
    fn route_method_mismatch_is_distinct_from_no_match() {
        let mut app = App::new();
        app.open("POST /only-post", ok);
        assert!(matches!(app.route("GET", "/only-post"), RouteMatch::MethodMismatch));
        assert!(matches!(app.route("GET", "/nope"), RouteMatch::NoMatch));
    }

    #[test]
    fn describe_omits_empty_optional_fields() {
        let mut app = App::new();
        app.open("GET /healthz", ok);
        let doc: serde_json::Value = serde_json::from_slice(&app.describe_json()).unwrap();
        let obj = doc.as_object().unwrap();
        for key in ["subscriptions", "schedules", "config", "secrets", "db", "httpAllow", "emits"] {
            assert!(!obj.contains_key(key), "expected {key} to be omitted when empty");
        }
        assert!(obj.contains_key("endpoints"));
    }

    #[test]
    fn describe_full_shape() {
        let mut app = App::new();
        app.webhook("POST /events/order-created", ok)
            .platform("GET /hello/{name}", ok)
            .open("GET /healthz", ok)
            .subscribe("orders:order:order:created", "/events/order-created")
            .schedule("*/5 * * * *", "/events/tick")
            .config("GREETING")
            .secret("STRIPE_KEY")
            .db("main")
            .http_allow("api.stripe.com")
            .emits("orders:order:order:shipped");

        let doc: serde_json::Value = serde_json::from_slice(&app.describe_json()).unwrap();
        assert_eq!(doc["abi"], 1);
        assert_eq!(doc["endpoints"].as_array().unwrap().len(), 3);
        assert_eq!(doc["subscriptions"][0]["eventType"], "orders:order:order:created");
        assert_eq!(doc["schedules"][0]["cron"], "*/5 * * * *");
        assert_eq!(doc["config"][0], "GREETING");
        assert_eq!(doc["secrets"][0], "STRIPE_KEY");
        assert_eq!(doc["db"][0], "main");
        assert_eq!(doc["httpAllow"][0], "api.stripe.com");
        assert_eq!(doc["emits"][0], "orders:order:order:shipped");
    }
}
