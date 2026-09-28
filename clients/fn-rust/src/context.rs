//! `Context`: the request-scoped handle to every host capability (plan
//! §5.3) plus the caller/invocation accessors handlers need.

use crate::host::{host_call, op, HostError, HostOutcome};
use crate::types::{Caller, Invocation};
use std::collections::BTreeMap;

/// A row from `db_query`. With `serde_json`'s `preserve_order` feature
/// (enabled by this crate), `Map` iterates in insertion order, so a row's
/// key order matches the host's SQL column order (plan §5.3 op 6: "the
/// host will emit keys in select order").
pub type Row = serde_json::Map<String, serde_json::Value>;

/// A capability failure: either the host reported one (`Host`), or a local
/// transport problem occurred (`Transport`, e.g. a malformed result frame —
/// should not happen against a correct host).
#[derive(Debug)]
pub enum CapabilityError {
    Host(HostError),
    Transport(String),
}

impl core::fmt::Display for CapabilityError {
    fn fmt(&self, f: &mut core::fmt::Formatter<'_>) -> core::fmt::Result {
        match self {
            CapabilityError::Host(e) => write!(f, "{e}"),
            CapabilityError::Transport(s) => write!(f, "fn: transport: {s}"),
        }
    }
}

impl CapabilityError {
    /// True when the host reported UNAVAILABLE ("retryable" per plan
    /// §5.3): platform trouble, as opposed to a permanent refusal such as
    /// NOT_ALLOWED.
    pub fn is_retryable(&self) -> bool {
        matches!(self, CapabilityError::Host(e) if e.is_retryable())
    }

    pub fn code(&self) -> Option<&str> {
        match self {
            CapabilityError::Host(e) => Some(&e.code),
            CapabilityError::Transport(_) => None,
        }
    }
}

impl From<HostOutcome> for CapabilityError {
    fn from(o: HostOutcome) -> Self {
        match o {
            HostOutcome::Host(e) => CapabilityError::Host(e),
            HostOutcome::Transport(e) => CapabilityError::Transport(e.0),
        }
    }
}

impl From<CapabilityError> for crate::types::Error {
    fn from(e: CapabilityError) -> Self {
        crate::types::Error(e.to_string())
    }
}

/// An outbound HTTP request (host op 4, http.fetch). `timeout_ms == 0`
/// means "use the host's default": unlike the Go SDK (which runs under
/// WASI and can read the request context's deadline off a real clock), a
/// `wasm32-unknown-unknown` guest has no clock capability at all, so this
/// crate cannot derive a timeout from "now" — callers that want a specific
/// budget must set it explicitly.
#[derive(Debug, Clone, Default)]
pub struct HttpRequest {
    pub method: String,
    pub url: String,
    pub headers: BTreeMap<String, Vec<String>>,
    pub timeout_ms: i64,
    pub body: Vec<u8>,
}

#[derive(Debug, Clone, Default)]
pub struct HttpResponse {
    pub status: u16,
    pub headers: BTreeMap<String, Vec<String>>,
    pub body: Vec<u8>,
}

/// An event to publish via `Context::emit` (host op 5, event.emit). `event_type`
/// must be one declared with `App::emits`.
#[derive(Debug, Clone, Default)]
pub struct Event {
    pub event_type: String,
    pub source: String,
    pub subject: String,
    pub dedup_id: String,
    pub correlation_id: String,
    pub causation_id: String,
    pub message_group: String,
    pub content_type: String,
    pub data: Vec<u8>,
}

/// A request-scoped handle to the host and to the caller/invocation
/// identity. Handlers receive `&Context` alongside `&Request`.
pub struct Context {
    pub(crate) caller: Caller,
    pub(crate) invocation: Invocation,
}

impl Context {
    pub fn caller(&self) -> &Caller {
        &self.caller
    }

    pub fn invocation(&self) -> &Invocation {
        &self.invocation
    }

    // --- op 1: log ----------------------------------------------------

    pub fn log(&self, level: &str, msg: &str, attrs: BTreeMap<String, serde_json::Value>) {
        #[derive(serde::Serialize)]
        struct LogMeta<'a> {
            level: &'a str,
            msg: &'a str,
            #[serde(skip_serializing_if = "BTreeMap::is_empty")]
            attrs: BTreeMap<String, serde_json::Value>,
        }
        if let Ok(mb) = serde_json::to_vec(&LogMeta { level, msg, attrs }) {
            let _ = host_call(op::LOG, &mb, &[]);
        }
    }

    pub fn log_info(&self, msg: &str) {
        self.log("INFO", msg, BTreeMap::new());
    }
    pub fn log_warn(&self, msg: &str) {
        self.log("WARN", msg, BTreeMap::new());
    }
    pub fn log_error(&self, msg: &str) {
        self.log("ERROR", msg, BTreeMap::new());
    }

    // --- op 2/3: config.get / secret.get -------------------------------

    pub fn config_get(&self, key: &str) -> Result<Option<String>, CapabilityError> {
        self.kv_get(op::CONFIG_GET, key)
    }

    pub fn secret_get(&self, key: &str) -> Result<Option<String>, CapabilityError> {
        self.kv_get(op::SECRET_GET, key)
    }

    fn kv_get(&self, op: i32, key: &str) -> Result<Option<String>, CapabilityError> {
        #[derive(serde::Serialize)]
        struct KVMeta<'a> {
            key: &'a str,
        }
        #[derive(serde::Deserialize)]
        struct KVResult {
            found: bool,
        }
        let mb = serde_json::to_vec(&KVMeta { key })
            .map_err(|e| CapabilityError::Transport(e.to_string()))?;
        let (rm, rb) = host_call(op, &mb, &[])?;
        let result: KVResult =
            serde_json::from_slice(&rm).map_err(|e| CapabilityError::Transport(e.to_string()))?;
        if !result.found {
            return Ok(None);
        }
        Ok(Some(String::from_utf8_lossy(&rb).into_owned()))
    }

    // --- op 4: http.fetch -----------------------------------------------

    pub fn http_fetch(&self, req: HttpRequest) -> Result<HttpResponse, CapabilityError> {
        #[derive(serde::Serialize)]
        struct FetchMeta<'a> {
            method: &'a str,
            url: &'a str,
            #[serde(skip_serializing_if = "BTreeMap::is_empty")]
            headers: &'a BTreeMap<String, Vec<String>>,
            #[serde(skip_serializing_if = "is_zero")]
            #[serde(rename = "timeoutMs")]
            timeout_ms: i64,
        }
        fn is_zero(v: &i64) -> bool {
            *v == 0
        }
        #[derive(serde::Deserialize, Default)]
        struct FetchRespMeta {
            status: u16,
            #[serde(default)]
            headers: BTreeMap<String, Vec<String>>,
        }
        let mb = serde_json::to_vec(&FetchMeta {
            method: &req.method,
            url: &req.url,
            headers: &req.headers,
            timeout_ms: req.timeout_ms,
        })
        .map_err(|e| CapabilityError::Transport(e.to_string()))?;
        let (rm, rb) = host_call(op::HTTP_FETCH, &mb, &req.body)?;
        let rmeta: FetchRespMeta = if rm.is_empty() {
            FetchRespMeta::default()
        } else {
            serde_json::from_slice(&rm).map_err(|e| CapabilityError::Transport(e.to_string()))?
        };
        Ok(HttpResponse {
            status: rmeta.status,
            headers: rmeta.headers,
            body: rb,
        })
    }

    // --- op 5: event.emit -------------------------------------------------

    pub fn emit(&self, event: Event) -> Result<String, CapabilityError> {
        #[derive(serde::Serialize)]
        struct EmitMeta<'a> {
            #[serde(rename = "type")]
            event_type: &'a str,
            #[serde(skip_serializing_if = "str::is_empty")]
            source: &'a str,
            #[serde(skip_serializing_if = "str::is_empty")]
            subject: &'a str,
            #[serde(skip_serializing_if = "str::is_empty", rename = "dedupId")]
            dedup_id: &'a str,
            #[serde(skip_serializing_if = "str::is_empty", rename = "correlationId")]
            correlation_id: &'a str,
            #[serde(skip_serializing_if = "str::is_empty", rename = "causationId")]
            causation_id: &'a str,
            #[serde(skip_serializing_if = "str::is_empty", rename = "messageGroup")]
            message_group: &'a str,
            #[serde(skip_serializing_if = "str::is_empty", rename = "contentType")]
            content_type: &'a str,
        }
        #[derive(serde::Deserialize, Default)]
        struct EmitResult {
            #[serde(default, rename = "eventId")]
            event_id: String,
        }
        let mb = serde_json::to_vec(&EmitMeta {
            event_type: &event.event_type,
            source: &event.source,
            subject: &event.subject,
            dedup_id: &event.dedup_id,
            correlation_id: &event.correlation_id,
            causation_id: &event.causation_id,
            message_group: &event.message_group,
            content_type: &event.content_type,
        })
        .map_err(|e| CapabilityError::Transport(e.to_string()))?;
        let (rm, _rb) = host_call(op::EVENT_EMIT, &mb, &event.data)?;
        let result: EmitResult = if rm.is_empty() {
            EmitResult::default()
        } else {
            serde_json::from_slice(&rm).map_err(|e| CapabilityError::Transport(e.to_string()))?
        };
        Ok(result.event_id)
    }

    // --- ops 6-10: db -----------------------------------------------------

    pub fn db_query(
        &self,
        db: &str,
        sql: &str,
        params: &[serde_json::Value],
        tx: Option<&str>,
    ) -> Result<Vec<Row>, CapabilityError> {
        let mb = db_query_meta(db, sql, params, tx)
            .map_err(|e| CapabilityError::Transport(e.to_string()))?;
        let (_rm, rb) = host_call(op::DB_QUERY, &mb, &[])?;
        if rb.is_empty() {
            return Ok(Vec::new());
        }
        serde_json::from_slice(&rb).map_err(|e| CapabilityError::Transport(e.to_string()))
    }

    pub fn db_exec(
        &self,
        db: &str,
        sql: &str,
        params: &[serde_json::Value],
        tx: Option<&str>,
    ) -> Result<i64, CapabilityError> {
        #[derive(serde::Deserialize, Default)]
        struct ExecResult {
            #[serde(default, rename = "rowsAffected")]
            rows_affected: i64,
        }
        let mb = db_query_meta(db, sql, params, tx)
            .map_err(|e| CapabilityError::Transport(e.to_string()))?;
        let (rm, _rb) = host_call(op::DB_EXEC, &mb, &[])?;
        let result: ExecResult = if rm.is_empty() {
            ExecResult::default()
        } else {
            serde_json::from_slice(&rm).map_err(|e| CapabilityError::Transport(e.to_string()))?
        };
        Ok(result.rows_affected)
    }

    pub fn db_begin(&self, db: &str) -> Result<String, CapabilityError> {
        #[derive(serde::Serialize)]
        struct BeginMeta<'a> {
            db: &'a str,
        }
        #[derive(serde::Deserialize, Default)]
        struct BeginResult {
            #[serde(default)]
            tx: String,
        }
        let mb = serde_json::to_vec(&BeginMeta { db })
            .map_err(|e| CapabilityError::Transport(e.to_string()))?;
        let (rm, _rb) = host_call(op::DB_BEGIN, &mb, &[])?;
        let result: BeginResult = if rm.is_empty() {
            BeginResult::default()
        } else {
            serde_json::from_slice(&rm).map_err(|e| CapabilityError::Transport(e.to_string()))?
        };
        Ok(result.tx)
    }

    pub fn db_commit(&self, tx: &str) -> Result<(), CapabilityError> {
        self.db_tx_op(op::DB_COMMIT, tx)
    }

    pub fn db_rollback(&self, tx: &str) -> Result<(), CapabilityError> {
        self.db_tx_op(op::DB_ROLLBACK, tx)
    }

    fn db_tx_op(&self, op: i32, tx: &str) -> Result<(), CapabilityError> {
        #[derive(serde::Serialize)]
        struct TxMeta<'a> {
            tx: &'a str,
        }
        let mb = serde_json::to_vec(&TxMeta { tx })
            .map_err(|e| CapabilityError::Transport(e.to_string()))?;
        host_call(op, &mb, &[])?;
        Ok(())
    }
}

fn db_query_meta(
    db: &str,
    sql: &str,
    params: &[serde_json::Value],
    tx: Option<&str>,
) -> serde_json::Result<Vec<u8>> {
    #[derive(serde::Serialize)]
    struct QueryMeta<'a> {
        db: &'a str,
        sql: &'a str,
        #[serde(skip_serializing_if = "<[_]>::is_empty")]
        params: &'a [serde_json::Value],
        #[serde(skip_serializing_if = "Option::is_none")]
        tx: Option<&'a str>,
    }
    serde_json::to_vec(&QueryMeta { db, sql, params, tx })
}

#[cfg(test)]
mod tests {
    use super::*;
    use crate::host::{op, set_fake_host, HostOutcome};
    use crate::types::{Caller, Invocation};

    fn ctx() -> Context {
        Context {
            caller: Caller::default(),
            invocation: Invocation::default(),
        }
    }

    #[test]
    fn config_get_found() {
        set_fake_host(|o, meta, _body| {
            assert_eq!(o, op::CONFIG_GET);
            let m: serde_json::Value = serde_json::from_slice(meta).unwrap();
            assert_eq!(m["key"], "GREETING");
            Ok((br#"{"found":true}"#.to_vec(), b"hello".to_vec()))
        });
        let got = ctx().config_get("GREETING").unwrap();
        assert_eq!(got.as_deref(), Some("hello"));
    }

    #[test]
    fn config_get_not_found() {
        set_fake_host(|_o, _m, _b| Ok((br#"{"found":false}"#.to_vec(), Vec::new())));
        let got = ctx().config_get("MISSING").unwrap();
        assert_eq!(got, None);
    }

    #[test]
    fn secret_get_uses_secret_op() {
        set_fake_host(|o, _m, _b| {
            assert_eq!(o, op::SECRET_GET);
            Ok((br#"{"found":true}"#.to_vec(), b"sk_live_123".to_vec()))
        });
        let got = ctx().secret_get("STRIPE_KEY").unwrap();
        assert_eq!(got.as_deref(), Some("sk_live_123"));
    }

    #[test]
    fn config_get_host_error() {
        set_fake_host(|_o, _m, _b| {
            Err(HostOutcome::Host(HostError {
                code: crate::host::error_code::NOT_DECLARED.to_string(),
                message: "key not declared".to_string(),
            }))
        });
        let err = ctx().config_get("X").unwrap_err();
        assert_eq!(err.code(), Some(crate::host::error_code::NOT_DECLARED));
        assert!(!err.is_retryable());
    }

    #[test]
    fn emit_returns_event_id() {
        set_fake_host(|o, meta, body| {
            assert_eq!(o, op::EVENT_EMIT);
            let m: serde_json::Value = serde_json::from_slice(meta).unwrap();
            assert_eq!(m["type"], "orders:order:order:shipped");
            assert_eq!(body, br#"{"orderId":1}"#);
            Ok((br#"{"eventId":"evt_1"}"#.to_vec(), Vec::new()))
        });
        let id = ctx()
            .emit(Event {
                event_type: "orders:order:order:shipped".to_string(),
                data: br#"{"orderId":1}"#.to_vec(),
                ..Default::default()
            })
            .unwrap();
        assert_eq!(id, "evt_1");
    }

    #[test]
    fn emit_unavailable_is_retryable() {
        set_fake_host(|_o, _m, _b| {
            Err(HostOutcome::Host(HostError {
                code: crate::host::error_code::UNAVAILABLE.to_string(),
                message: "platform busy".to_string(),
            }))
        });
        let err = ctx()
            .emit(Event {
                event_type: "a:b:c:d".to_string(),
                ..Default::default()
            })
            .unwrap_err();
        assert!(err.is_retryable());
    }

    #[test]
    fn emit_not_allowed_is_not_retryable() {
        set_fake_host(|_o, _m, _b| {
            Err(HostOutcome::Host(HostError {
                code: crate::host::error_code::NOT_ALLOWED.to_string(),
                message: "type not owned".to_string(),
            }))
        });
        let err = ctx()
            .emit(Event {
                event_type: "a:b:c:d".to_string(),
                ..Default::default()
            })
            .unwrap_err();
        assert!(!err.is_retryable());
    }

    #[test]
    fn http_fetch_round_trip() {
        set_fake_host(|o, meta, _body| {
            assert_eq!(o, op::HTTP_FETCH);
            let m: serde_json::Value = serde_json::from_slice(meta).unwrap();
            assert_eq!(m["method"], "GET");
            assert_eq!(m["url"], "https://api.stripe.com/v1/charges");
            let rm = serde_json::json!({"status": 200, "headers": {"Content-Type": ["application/json"]}});
            Ok((serde_json::to_vec(&rm).unwrap(), br#"{"ok":true}"#.to_vec()))
        });
        let resp = ctx()
            .http_fetch(HttpRequest {
                method: "GET".to_string(),
                url: "https://api.stripe.com/v1/charges".to_string(),
                ..Default::default()
            })
            .unwrap();
        assert_eq!(resp.status, 200);
        assert_eq!(resp.body, br#"{"ok":true}"#);
        assert_eq!(resp.headers.get("Content-Type").unwrap()[0], "application/json");
    }

    #[test]
    fn http_fetch_host_error() {
        set_fake_host(|_o, _m, _b| {
            Err(HostOutcome::Host(HostError {
                code: crate::host::error_code::NOT_ALLOWED.to_string(),
                message: "host not allowlisted".to_string(),
            }))
        });
        let err = ctx()
            .http_fetch(HttpRequest {
                method: "GET".to_string(),
                url: "https://evil.example.com".to_string(),
                ..Default::default()
            })
            .unwrap_err();
        assert_eq!(err.code(), Some(crate::host::error_code::NOT_ALLOWED));
    }

    #[test]
    fn db_query_preserves_column_order() {
        set_fake_host(|o, meta, _body| {
            assert_eq!(o, op::DB_QUERY);
            let m: serde_json::Value = serde_json::from_slice(meta).unwrap();
            assert_eq!(m["db"], "main");
            assert_eq!(m["sql"], "select id, name from widgets where id = ?");
            assert_eq!(m["params"][0], 7);
            let rows = br#"[{"z_last":"z","id":1,"name":"a"},{"z_last":"y","id":2,"name":"b"}]"#;
            Ok((br#"{"truncated":false}"#.to_vec(), rows.to_vec()))
        });
        let rows = ctx()
            .db_query(
                "main",
                "select id, name from widgets where id = ?",
                &[serde_json::json!(7)],
                None,
            )
            .unwrap();
        assert_eq!(rows.len(), 2);
        let keys: Vec<&String> = rows[0].keys().collect();
        assert_eq!(keys, vec!["z_last", "id", "name"], "row key order must follow the host's column order");
        assert_eq!(rows[0]["id"], 1);
        assert_eq!(rows[1]["name"], "b");
    }

    #[test]
    fn db_query_empty_result() {
        set_fake_host(|_o, _m, _b| Ok((Vec::new(), b"[]".to_vec())));
        let rows = ctx().db_query("main", "select 1 where false", &[], None).unwrap();
        assert!(rows.is_empty());
    }

    #[test]
    fn db_exec_rows_affected() {
        set_fake_host(|o, _m, _b| {
            assert_eq!(o, op::DB_EXEC);
            Ok((br#"{"rowsAffected":3}"#.to_vec(), Vec::new()))
        });
        let n = ctx()
            .db_exec("main", "update widgets set active = ?", &[serde_json::json!(true)], None)
            .unwrap();
        assert_eq!(n, 3);
    }

    #[test]
    fn db_transaction_commit_carries_tx_id() {
        let mut seen_tx = String::new();
        set_fake_host(move |o, meta, _body| match o {
            x if x == op::DB_BEGIN => Ok((br#"{"tx":"tx_1"}"#.to_vec(), Vec::new())),
            x if x == op::DB_EXEC => {
                let m: serde_json::Value = serde_json::from_slice(meta).unwrap();
                assert_eq!(m["tx"], "tx_1");
                Ok((br#"{"rowsAffected":1}"#.to_vec(), Vec::new()))
            }
            x if x == op::DB_COMMIT => {
                let m: serde_json::Value = serde_json::from_slice(meta).unwrap();
                assert_eq!(m["tx"], "tx_1");
                Ok((Vec::new(), Vec::new()))
            }
            _ => panic!("unexpected op {o}"),
        });
        let _ = &mut seen_tx;
        let c = ctx();
        let tx = c.db_begin("main").unwrap();
        assert_eq!(tx, "tx_1");
        c.db_exec("main", "update x set y = 1", &[], Some(&tx)).unwrap();
        c.db_commit(&tx).unwrap();
    }

    #[test]
    fn db_rollback() {
        set_fake_host(|o, _m, _b| match o {
            x if x == op::DB_BEGIN => Ok((br#"{"tx":"tx_2"}"#.to_vec(), Vec::new())),
            x if x == op::DB_ROLLBACK => Ok((Vec::new(), Vec::new())),
            _ => panic!("unexpected op {o}"),
        });
        let c = ctx();
        let tx = c.db_begin("main").unwrap();
        c.db_rollback(&tx).unwrap();
    }

    #[test]
    fn db_error_classified() {
        set_fake_host(|_o, _m, _b| {
            Err(HostOutcome::Host(HostError {
                code: crate::host::error_code::DB_CONSTRAINT.to_string(),
                message: "unique violation".to_string(),
            }))
        });
        let err = ctx().db_exec("main", "insert into x values (1)", &[], None).unwrap_err();
        assert_eq!(err.code(), Some(crate::host::error_code::DB_CONSTRAINT));
    }

    #[test]
    fn log_sends_op1() {
        set_fake_host(|o, meta, _body| {
            assert_eq!(o, op::LOG);
            let m: serde_json::Value = serde_json::from_slice(meta).unwrap();
            assert_eq!(m["level"], "INFO");
            assert_eq!(m["msg"], "hello");
            Ok((Vec::new(), Vec::new()))
        });
        ctx().log_info("hello");
        assert_eq!(crate::host::fake_host_calls().len(), 1);
    }
}
