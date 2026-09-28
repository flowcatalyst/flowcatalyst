//! Request/response/caller shapes (plan §5.2).

use crate::host::HostError;
use std::collections::BTreeMap;

/// Deserializes an explicit JSON `null` as the type's default, as
/// `#[serde(default)]` does for a missing field.
fn null_default<'de, D, T>(d: D) -> Result<T, D::Error>
where
    D: serde::Deserializer<'de>,
    T: Default + serde::Deserialize<'de>,
{
    Ok(<Option<T> as serde::Deserialize>::deserialize(d)?.unwrap_or_default())
}

/// A verified caller identity's principal (plan §5.2, `caller.kind ==
/// "principal"`). Semantics mirror the platform's
/// `internal/platform/shared/auth.AuthContext`: `tier` is "ANCHOR",
/// "PARTNER" or "CLIENT"; permissions may carry `*` segment wildcards
/// matched by `has_permission`.
#[derive(Debug, Clone, Default, serde::Deserialize)]
pub struct Principal {
    #[serde(default)]
    pub id: String,
    #[serde(default, rename = "type")]
    pub kind: String,
    #[serde(default)]
    pub tier: String,
    #[serde(default)]
    pub clients: Vec<String>,
    #[serde(default)]
    pub roles: Vec<String>,
    #[serde(default)]
    pub applications: Vec<String>,
    #[serde(default)]
    pub all_applications: bool,
    #[serde(default)]
    pub permissions: Vec<String>,
}

impl Principal {
    pub fn is_anchor(&self) -> bool {
        self.tier == "ANCHOR"
    }

    pub fn has_role(&self, role: &str) -> bool {
        self.roles.iter().any(|r| r == role)
    }

    pub fn can_access_client(&self, client_id: &str) -> bool {
        self.is_anchor() || self.clients.iter().any(|c| c == client_id)
    }

    pub fn can_access_application(&self, application_id: &str) -> bool {
        self.all_applications || self.applications.iter().any(|a| a == application_id)
    }

    /// A held permission may contain `*` segment wildcards (e.g.
    /// "platform:messaging:*:*" or the super-admin "platform:*:*:*"): such
    /// a permission matches any code with the same colon-separated segment
    /// count whose non-wildcard segments are equal. Mirrors the Go SDK's
    /// (and the platform's) `permissionMatches` exactly.
    pub fn has_permission(&self, code: &str) -> bool {
        self.permissions
            .iter()
            .any(|held| permission_matches(held, code))
    }
}

fn permission_matches(held: &str, required: &str) -> bool {
    if held == required {
        return true;
    }
    let h: Vec<&str> = held.split(':').collect();
    let r: Vec<&str> = required.split(':').collect();
    if h.len() != r.len() {
        return false;
    }
    h.iter().zip(r.iter()).all(|(hs, rs)| *hs == "*" || hs == rs)
}

/// Who made this invocation (plan §5.2).
#[derive(Debug, Clone, Default, serde::Deserialize)]
pub struct Caller {
    #[serde(default)]
    pub kind: String,
    #[serde(flatten, default)]
    principal_fields: Principal,
}

impl Caller {
    pub fn kind(&self) -> &str {
        &self.kind
    }

    /// The verified principal, or `None` unless `kind() == "principal"`
    /// (i.e. the endpoint's auth is "platform").
    pub fn principal(&self) -> Option<&Principal> {
        if self.kind == "principal" {
            Some(&self.principal_fields)
        } else {
            None
        }
    }
}

/// The in-flight invocation's identity (plan §5.2 request meta).
#[derive(Debug, Clone, Default)]
pub struct Invocation {
    pub id: String,
    pub address: String,
    pub version: i64,
    pub deadline_unix_ms: i64,
}

/// The request frame's meta JSON (plan §5.2), as deserialized.
#[derive(Debug, Clone, Default, serde::Deserialize)]
pub(crate) struct RequestMeta {
    #[serde(default)]
    pub id: String,
    #[serde(default)]
    pub address: String,
    #[serde(default)]
    pub version: i64,
    #[serde(default)]
    pub method: String,
    #[serde(default)]
    pub path: String,
    #[serde(default, rename = "rawQuery")]
    pub raw_query: String,
    #[serde(default, deserialize_with = "null_default")]
    pub headers: BTreeMap<String, Vec<String>>,
    // route/path_params are parsed for completeness but intentionally
    // unused: dispatch.rs re-derives the matched endpoint and its path
    // params from App::route (the same "let the router own matching"
    // decision the Go SDK makes via http.ServeMux), rather than trusting
    // the host's own route match.
    #[serde(default)]
    #[allow(dead_code)]
    pub route: String,
    #[serde(default, rename = "pathParams", deserialize_with = "null_default")]
    #[allow(dead_code)]
    pub path_params: BTreeMap<String, String>,
    #[serde(default, deserialize_with = "null_default")]
    pub caller: Caller,
    #[serde(default, rename = "deadlineUnixMs")]
    pub deadline_unix_ms: i64,
}

/// A dispatched request, built from the request frame (plan §5.2).
#[derive(Debug, Clone, Default)]
pub struct Request {
    pub id: String,
    pub address: String,
    pub version: i64,
    pub method: String,
    pub path: String,
    pub raw_query: String,
    pub headers: BTreeMap<String, Vec<String>>,
    pub route: String,
    pub path_params: BTreeMap<String, String>,
    pub caller: Caller,
    pub deadline_unix_ms: i64,
    pub body: Vec<u8>,
}

impl Request {
    pub fn header(&self, name: &str) -> Option<&str> {
        self.headers
            .iter()
            .find(|(k, _)| k.eq_ignore_ascii_case(name))
            .and_then(|(_, v)| v.first())
            .map(|s| s.as_str())
    }

    pub fn path_param(&self, name: &str) -> Option<&str> {
        self.path_params.get(name).map(|s| s.as_str())
    }

    pub fn query(&self) -> &str {
        &self.raw_query
    }

    pub fn body(&self) -> &[u8] {
        &self.body
    }
}

/// The response frame's meta JSON (plan §5.2). Deserialize is derived too
/// (not just Serialize) so tests can decode a response frame the same way
/// a real host would, rather than re-implementing that parsing.
#[derive(Debug, Clone, Default, serde::Serialize, serde::Deserialize)]
pub(crate) struct ResponseMeta {
    pub status: u16,
    #[serde(default, skip_serializing_if = "BTreeMap::is_empty")]
    pub headers: BTreeMap<String, Vec<String>>,
}

/// A handler's HTTP response.
#[derive(Debug, Clone, Default)]
pub struct Response {
    pub status: u16,
    pub headers: BTreeMap<String, Vec<String>>,
    pub body: Vec<u8>,
}

impl Response {
    pub fn new(status: u16) -> Self {
        Response {
            status,
            headers: BTreeMap::new(),
            body: Vec::new(),
        }
    }

    pub fn with_header(mut self, name: impl Into<String>, value: impl Into<String>) -> Self {
        self.headers
            .entry(name.into())
            .or_default()
            .push(value.into());
        self
    }

    pub fn with_body(mut self, body: impl Into<Vec<u8>>) -> Self {
        self.body = body.into();
        self
    }

    pub fn json<T: serde::Serialize>(status: u16, value: &T) -> Result<Self, Error> {
        let body =
            serde_json::to_vec(value).map_err(|e| Error(format!("json encode: {e}")))?;
        Ok(Response::new(status)
            .with_header("Content-Type", "application/json")
            .with_body(body))
    }
}

/// A handler's failure (plan §9: "Handler Err -> 500 with fixed body,
/// logged."). This is a guest-side error distinct from `HostError`
/// (`?`-convertible into `Error` for handlers that call capability
/// methods).
#[derive(Debug, Clone)]
pub struct Error(pub String);

impl core::fmt::Display for Error {
    fn fmt(&self, f: &mut core::fmt::Formatter<'_>) -> core::fmt::Result {
        write!(f, "{}", self.0)
    }
}

impl From<&str> for Error {
    fn from(s: &str) -> Self {
        Error(s.to_string())
    }
}

impl From<String> for Error {
    fn from(s: String) -> Self {
        Error(s)
    }
}

impl From<HostError> for Error {
    fn from(e: HostError) -> Self {
        Error(format!("{}: {}", e.code, e.message))
    }
}

/// The fixed body written on a handler `Err` (plan §9).
pub const HANDLER_ERROR_BODY: &str = r#"{"error":"FUNCTION_FAILED"}"#;

#[cfg(test)]
mod tests {
    use super::*;

    fn principal(permissions: &[&str]) -> Principal {
        Principal {
            permissions: permissions.iter().map(|s| s.to_string()).collect(),
            ..Default::default()
        }
    }

    #[test]
    fn has_permission_exact_match() {
        assert!(principal(&["platform:function:view"]).has_permission("platform:function:view"));
    }

    #[test]
    fn has_permission_different_segment_no_match() {
        assert!(!principal(&["platform:function:view"]).has_permission("platform:function:manage"));
    }

    #[test]
    fn has_permission_trailing_wildcard() {
        assert!(principal(&["platform:messaging:*:*"]).has_permission("platform:messaging:queue:read"));
    }

    #[test]
    fn has_permission_super_admin_wildcard() {
        assert!(principal(&["platform:*:*:*"]).has_permission("platform:messaging:queue:read"));
    }

    #[test]
    fn has_permission_requires_same_segment_count_fewer() {
        assert!(!principal(&["platform:*"]).has_permission("platform:messaging:queue:read"));
    }

    #[test]
    fn has_permission_requires_same_segment_count_more() {
        assert!(!principal(&["platform:*:*:*:*"]).has_permission("platform:messaging:queue"));
    }

    #[test]
    fn has_permission_middle_wildcard() {
        assert!(principal(&["platform:*:queue:read"]).has_permission("platform:messaging:queue:read"));
        assert!(!principal(&["platform:*:queue:read"]).has_permission("platform:messaging:topic:read"));
    }

    #[test]
    fn has_permission_empty_permissions() {
        assert!(!principal(&[]).has_permission("platform:function:view"));
    }

    #[test]
    fn has_permission_one_of_several() {
        assert!(principal(&["a:b", "platform:function:*"]).has_permission("platform:function:view"));
    }

    #[test]
    fn has_role_is_exact_only() {
        let p = Principal {
            roles: vec!["operant:admin".to_string(), "billing:viewer".to_string()],
            ..Default::default()
        };
        assert!(p.has_role("operant:admin"));
        assert!(!p.has_role("operant:*"), "has_role must not wildcard");
    }

    #[test]
    fn can_access_client_anchor_always_true() {
        let p = Principal {
            tier: "ANCHOR".to_string(),
            ..Default::default()
        };
        assert!(p.can_access_client("any-client"));
    }

    #[test]
    fn can_access_client_scoped() {
        let p = Principal {
            tier: "CLIENT".to_string(),
            clients: vec!["clt_a".to_string()],
            ..Default::default()
        };
        assert!(p.can_access_client("clt_a"));
        assert!(!p.can_access_client("clt_z"));
    }

    #[test]
    fn can_access_application() {
        let all = Principal {
            all_applications: true,
            ..Default::default()
        };
        assert!(all.can_access_application("app_1"));

        let scoped = Principal {
            applications: vec!["app_1".to_string()],
            ..Default::default()
        };
        assert!(scoped.can_access_application("app_1"));
        assert!(!scoped.can_access_application("app_2"));
    }

    #[test]
    fn caller_deserializes_principal_fields() {
        let json = r#"{
            "kind": "principal", "id": "prn_1", "type": "USER", "tier": "CLIENT",
            "clients": ["clt_1"], "roles": ["operant:admin"], "applications": ["app_1"],
            "allApplications": false, "permissions": ["platform:function:view"]
        }"#;
        let c: Caller = serde_json::from_str(json).unwrap();
        assert_eq!(c.kind(), "principal");
        let p = c.principal().expect("principal caller must expose a Principal");
        assert_eq!(p.id, "prn_1");
        assert_eq!(p.tier, "CLIENT");
        assert!(p.has_role("operant:admin"));
        assert!(p.has_permission("platform:function:view"));
    }

    #[test]
    fn caller_webhook_has_no_principal() {
        let c: Caller = serde_json::from_str(r#"{"kind":"webhook"}"#).unwrap();
        assert_eq!(c.kind(), "webhook");
        assert!(c.principal().is_none());
    }

    #[test]
    fn caller_anonymous_has_no_principal() {
        let c: Caller = serde_json::from_str(r#"{"kind":"anonymous"}"#).unwrap();
        assert!(c.principal().is_none());
    }
}
