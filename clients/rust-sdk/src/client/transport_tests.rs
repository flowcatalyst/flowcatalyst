//! Transport behaviour: headers, retries, the 401 refresh, error mapping.

use std::sync::atomic::{AtomicUsize, Ordering};
use std::sync::{Arc, Mutex};
use std::time::Duration;

use tokio::io::{AsyncReadExt, AsyncWriteExt};
use tokio::net::TcpListener;

use super::*;

/// What the scripted server does with one request.
#[derive(Clone, Copy, Debug)]
enum Step {
    /// Answer with this status and body.
    Answer(u16, &'static str),
    /// Read the request, then answer nothing for ten seconds (a read
    /// timeout for the client).
    Hang,
    /// Read the request, then close the connection without answering.
    Close,
    /// Answer this status with a `Content-Length` of 100, send four bytes of
    /// body and close (the body cannot be read).
    Truncated(u16),
}

/// A server answering each connection with the next scripted `(status, body)`
/// (the last one repeats), recording each request's head.
async fn scripted(script: Vec<(u16, &'static str)>) -> (String, Arc<Mutex<Vec<String>>>) {
    scripted_steps(
        script
            .into_iter()
            .map(|(status, body)| Step::Answer(status, body))
            .collect(),
    )
    .await
}

/// [`scripted`] with steps that can also hang, close or cut a body short.
async fn scripted_steps(script: Vec<Step>) -> (String, Arc<Mutex<Vec<String>>>) {
    let listener = TcpListener::bind("127.0.0.1:0").await.unwrap();
    let base = format!("http://{}", listener.local_addr().unwrap());
    let heads = serve(listener, script);
    (base, heads)
}

/// Serve `script` on `listener`, returning the log of request heads.
fn serve(listener: TcpListener, script: Vec<Step>) -> Arc<Mutex<Vec<String>>> {
    let heads = Arc::new(Mutex::new(Vec::new()));
    let log = heads.clone();
    let next = Arc::new(AtomicUsize::new(0));
    tokio::spawn(async move {
        while let Ok((mut socket, _)) = listener.accept().await {
            let script = script.clone();
            let log = log.clone();
            let next = next.clone();
            tokio::spawn(async move {
                let mut raw = Vec::new();
                let mut buf = [0u8; 4096];
                loop {
                    let n = socket.read(&mut buf).await.unwrap_or(0);
                    if n == 0 {
                        return;
                    }
                    raw.extend_from_slice(&buf[..n]);
                    let text = String::from_utf8_lossy(&raw).to_string();
                    if let Some(end) = text.find("\r\n\r\n") {
                        let len: usize = text[..end]
                            .lines()
                            .find_map(|l| {
                                let (k, v) = l.split_once(':')?;
                                k.trim()
                                    .eq_ignore_ascii_case("content-length")
                                    .then(|| v.trim().parse().ok())?
                            })
                            .unwrap_or(0);
                        if raw.len() >= end + 4 + len {
                            log.lock().unwrap().push(text);
                            break;
                        }
                    }
                }
                let i = next.fetch_add(1, Ordering::SeqCst).min(script.len() - 1);
                match script[i] {
                    Step::Answer(status, body) => {
                        let resp = format!(
                            "HTTP/1.1 {status} X\r\nContent-Type: application/json\r\nContent-Length: {}\r\nConnection: close\r\n\r\n{body}",
                            body.len()
                        );
                        socket.write_all(resp.as_bytes()).await.ok();
                    }
                    Step::Hang => tokio::time::sleep(Duration::from_secs(10)).await,
                    Step::Close => drop(socket),
                    Step::Truncated(status) => {
                        let resp = format!(
                            "HTTP/1.1 {status} X\r\nContent-Type: application/json\r\nContent-Length: 100\r\nConnection: close\r\n\r\n{{\"a\""
                        );
                        socket.write_all(resp.as_bytes()).await.ok();
                        socket.flush().await.ok();
                    }
                }
            });
        }
    });
    heads
}

fn client(base: &str) -> FlowCatalystClient {
    FlowCatalystClient::new(base).with_retry(3, Duration::from_millis(1))
}

#[tokio::test]
async fn retries_the_transient_statuses_then_succeeds() {
    let (base, heads) = scripted(vec![(503, ""), (429, ""), (200, r#"{"ok":true}"#)]).await;
    let v: serde_json::Value = client(&base).get("/api/x").await.unwrap();
    assert_eq!(v["ok"], true);
    assert_eq!(heads.lock().unwrap().len(), 3);
}

#[tokio::test]
async fn does_not_retry_a_500_or_a_400() {
    for status in [500, 400] {
        let (base, heads) = scripted(vec![(status, r#"{"error":"X","message":"m"}"#)]).await;
        let err = client(&base)
            .get::<serde_json::Value>("/api/x")
            .await
            .unwrap_err();
        assert_eq!(err.status(), Some(status));
        assert_eq!(heads.lock().unwrap().len(), 1, "{status}");
    }
}

#[tokio::test]
async fn gives_up_after_the_configured_retries() {
    let (base, heads) = scripted(vec![(502, "")]).await;
    let err = client(&base)
        .get::<serde_json::Value>("/api/x")
        .await
        .unwrap_err();
    assert_eq!(err.status(), Some(502));
    assert_eq!(
        heads.lock().unwrap().len(),
        4,
        "one attempt plus three retries"
    );
}

#[tokio::test]
async fn parses_the_error_envelope() {
    let (base, _) = scripted(vec![(
        409,
        r#"{"error":"DUPLICATE","message":"code exists"}"#,
    )])
    .await;
    match client(&base)
        .get::<serde_json::Value>("/api/x")
        .await
        .unwrap_err()
    {
        ClientError::Api {
            status,
            code,
            message,
            ..
        } => {
            assert_eq!(status, 409);
            assert_eq!(code.as_deref(), Some("DUPLICATE"));
            assert_eq!(message.as_deref(), Some("code exists"));
        }
        other => panic!("{other:?}"),
    }
}

#[tokio::test]
async fn sends_accept_always_and_content_type_only_with_a_body() {
    let (base, heads) = scripted(vec![(200, "{}")]).await;
    let c = client(&base).with_token("tok-1");
    let _: serde_json::Value = c.get("/api/x").await.unwrap();
    let _: serde_json::Value = c
        .post("/api/y", &serde_json::json!({"a": 1}))
        .await
        .unwrap();
    let heads = heads.lock().unwrap();
    let get = heads[0].to_ascii_lowercase();
    let post = heads[1].to_ascii_lowercase();
    assert!(get.contains("accept: application/json"));
    assert!(get.contains("authorization: bearer tok-1"));
    assert!(!get.contains("content-type"));
    assert!(post.contains("content-type: application/json"));
}

#[tokio::test]
async fn an_empty_2xx_decodes_as_unit_or_none() {
    let (base, _) = scripted(vec![(204, "")]).await;
    let c = client(&base);
    let (): () = c.post("/api/x", &()).await.unwrap();
    let none: Option<serde_json::Value> = c.get("/api/x").await.unwrap();
    assert!(none.is_none());
}

#[tokio::test]
async fn a_401_refreshes_client_credentials_once() {
    // Token endpoint and API on one scripted server: token, 401, token, 200.
    let (base, heads) = scripted(vec![
        (200, r#"{"access_token":"t1","expires_in":3600}"#),
        (401, ""),
        (200, r#"{"access_token":"t2","expires_in":3600}"#),
        (200, r#"{"ok":true}"#),
    ])
    .await;
    let c = client(&base).with_token_source(ClientCredentials::new(&base, "id", "secret"));
    let v: serde_json::Value = c.get("/api/x").await.unwrap();
    assert_eq!(v["ok"], true);
    let heads = heads.lock().unwrap();
    assert_eq!(heads.len(), 4);
    assert!(heads[0].starts_with("POST /oauth/token"));
    assert!(heads[0].contains("grant_type=client_credentials&client_id=id&client_secret=secret"));
    assert!(
        heads[1]
            .to_ascii_lowercase()
            .contains("authorization: bearer t1")
    );
    assert!(
        heads[3]
            .to_ascii_lowercase()
            .contains("authorization: bearer t2")
    );
}

#[tokio::test]
async fn client_credentials_are_cached_until_near_expiry() {
    let (base, heads) = scripted(vec![
        (200, r#"{"access_token":"t1","expires_in":3600}"#),
        (200, "{}"),
    ])
    .await;
    let cc = ClientCredentials::new(&base, "id", "secret");
    assert_eq!(cc.token().await.unwrap(), "t1");
    assert_eq!(cc.token().await.unwrap(), "t1");
    assert_eq!(heads.lock().unwrap().len(), 1);
}

#[tokio::test]
async fn refused_credentials_are_a_token_error() {
    let (base, _) = scripted(vec![(401, r#"{"error":"invalid_client"}"#)]).await;
    let err = ClientCredentials::new(&base, "id", "bad")
        .token()
        .await
        .unwrap_err();
    assert!(matches!(
        err,
        ClientError::Token(TokenError::InvalidCredentials { status: 401 })
    ));
    let err = ClientCredentials::new(&base, "", "")
        .token()
        .await
        .unwrap_err();
    assert!(matches!(
        err,
        ClientError::Token(TokenError::MissingCredentials)
    ));
}

#[test]
fn query_strings_are_form_encoded() {
    assert_eq!(
        FlowCatalystClient::query_string(&[("email", "a+b@x.io".into()), ("ids", "1,2".into())]),
        "?email=a%2Bb%40x.io&ids=1%2C2"
    );
    assert_eq!(FlowCatalystClient::query_string(&[]), "");
}

// ── The retry rule: repeatable requests and unknown outcomes ────────────

fn methods(heads: &Arc<Mutex<Vec<String>>>) -> Vec<String> {
    heads
        .lock()
        .unwrap()
        .iter()
        .map(|h| h.lines().next().unwrap_or_default().to_string())
        .collect()
}

#[tokio::test]
async fn a_get_answered_502_is_retried() {
    let (base, heads) = scripted(vec![(502, ""), (200, r#"{"ok":true}"#)]).await;
    let v: serde_json::Value = client(&base).get("/api/x").await.unwrap();
    assert_eq!(v["ok"], true);
    assert_eq!(heads.lock().unwrap().len(), 2);
}

#[tokio::test]
async fn a_put_answered_503_is_retried() {
    let (base, heads) = scripted(vec![(503, ""), (200, r#"{"ok":true}"#)]).await;
    let v: serde_json::Value = client(&base)
        .put("/api/x", &serde_json::json!({"a": 1}))
        .await
        .unwrap();
    assert_eq!(v["ok"], true);
    assert_eq!(heads.lock().unwrap().len(), 2);
}

#[tokio::test]
async fn a_delete_answered_502_then_404_has_succeeded() {
    let (base, heads) = scripted(vec![(502, ""), (404, r#"{"error":"NOT_FOUND"}"#)]).await;
    client(&base).delete_req("/api/x/1").await.unwrap();
    assert_eq!(
        methods(&heads),
        vec!["DELETE /api/x/1 HTTP/1.1", "DELETE /api/x/1 HTTP/1.1"]
    );
}

#[tokio::test]
async fn a_delete_cut_off_after_sending_then_404_has_succeeded() {
    let (base, heads) = scripted_steps(vec![
        Step::Close,
        Step::Answer(404, r#"{"error":"NOT_FOUND"}"#),
    ])
    .await;
    client(&base).delete_req("/api/x/1").await.unwrap();
    assert_eq!(heads.lock().unwrap().len(), 2);
}

#[tokio::test]
async fn a_delete_answered_404_first_is_not_found() {
    let (base, heads) = scripted(vec![(404, r#"{"error":"NOT_FOUND"}"#)]).await;
    let err = client(&base).delete_req("/api/x/1").await.unwrap_err();
    assert!(err.is_not_found(), "{err:?}");
    assert!(matches!(err, ClientError::Api { status: 404, .. }));
    assert_eq!(heads.lock().unwrap().len(), 1);
}

#[tokio::test]
async fn a_delete_answered_429_then_404_is_not_found() {
    // A 429 is a refusal, not an unknown outcome: the 404 is real.
    let (base, _) = scripted(vec![(429, ""), (404, "")]).await;
    let err = client(&base).delete_req("/api/x/1").await.unwrap_err();
    assert!(err.is_not_found(), "{err:?}");
}

#[tokio::test]
async fn a_post_answered_502_is_not_repeated_and_its_outcome_is_unknown() {
    for status in [502u16, 503, 504] {
        let (base, heads) = scripted(vec![(status, "bad gateway"), (200, "{}")]).await;
        let err = client(&base)
            .post::<_, serde_json::Value>("/api/x", &serde_json::json!({"a": 1}))
            .await
            .unwrap_err();
        assert_eq!(heads.lock().unwrap().len(), 1, "{status}");
        match &err {
            ClientError::OutcomeUnknown {
                method,
                url,
                status: got,
                body,
                source,
            } => {
                assert_eq!(*method, Method::POST);
                assert_eq!(url, &format!("{base}/api/x"));
                assert_eq!(*got, Some(status));
                assert_eq!(body, "bad gateway");
                assert!(source.is_none());
            }
            other => panic!("{other:?}"),
        }
        assert!(err.is_outcome_unknown());
        assert_eq!(err.status(), Some(status));
        let text = err.to_string();
        assert!(
            text.contains("may have been applied and was not repeated"),
            "{text}"
        );
        assert!(text.contains(&format!("HTTP {status}")), "{text}");
    }
}

#[tokio::test]
async fn a_post_answered_500_is_the_ordinary_error() {
    let (base, heads) = scripted(vec![(500, r#"{"error":"INTERNAL"}"#)]).await;
    let err = client(&base)
        .post::<_, serde_json::Value>("/api/x", &())
        .await
        .unwrap_err();
    assert!(
        matches!(err, ClientError::Api { status: 500, .. }),
        "{err:?}"
    );
    assert_eq!(heads.lock().unwrap().len(), 1);
}

#[tokio::test]
async fn a_post_to_a_sync_endpoint_answered_502_is_retried() {
    let (base, heads) = scripted(vec![
        (502, ""),
        (
            200,
            r#"{"applicationCode":"orders","created":0,"updated":0,"deleted":0,"syncedCodes":[]}"#,
        ),
    ])
    .await;
    let result = client(&base)
        .roles()
        .sync("orders", &SyncRolesRequest { roles: vec![] }, false)
        .await
        .unwrap();
    assert_eq!(result.application_code, "orders");
    assert_eq!(
        methods(&heads),
        vec![
            "POST /api/applications/orders/roles/sync HTTP/1.1",
            "POST /api/applications/orders/roles/sync HTTP/1.1"
        ]
    );
}

#[tokio::test]
async fn a_post_refused_with_429_or_408_is_retried() {
    for status in [429u16, 408] {
        let (base, heads) = scripted(vec![(status, ""), (200, r#"{"id":"x"}"#)]).await;
        let v: serde_json::Value = client(&base)
            .post("/api/x", &serde_json::json!({"a": 1}))
            .await
            .unwrap();
        assert_eq!(v["id"], "x");
        assert_eq!(heads.lock().unwrap().len(), 2, "{status}");
    }
}

#[tokio::test]
async fn a_post_that_could_not_connect_is_retried() {
    // Take a free port and close it: the first attempt is refused. The
    // server starts on that port while the client backs off.
    let probe = std::net::TcpListener::bind("127.0.0.1:0").unwrap();
    let addr = probe.local_addr().unwrap();
    drop(probe);
    let base = format!("http://{addr}");
    let (tx, rx) = tokio::sync::oneshot::channel();
    tokio::spawn(async move {
        tokio::time::sleep(Duration::from_millis(50)).await;
        let listener = TcpListener::bind(addr).await.unwrap();
        tx.send(serve(listener, vec![Step::Answer(201, r#"{"id":"x"}"#)]))
            .ok();
    });
    let v: serde_json::Value = FlowCatalystClient::new(&base)
        .with_retry(3, Duration::from_millis(300))
        .post("/api/x", &serde_json::json!({"a": 1}))
        .await
        .unwrap();
    assert_eq!(v["id"], "x");
    let heads = rx.await.unwrap();
    assert_eq!(
        heads.lock().unwrap().len(),
        1,
        "the refused attempt never arrived"
    );
}

#[tokio::test]
async fn a_post_that_timed_out_after_sending_is_not_repeated() {
    let (base, heads) = scripted_steps(vec![Step::Hang, Step::Answer(200, "{}")]).await;
    let err = client(&base)
        .with_timeout(Duration::from_millis(200))
        .post::<_, serde_json::Value>("/api/x", &serde_json::json!({"a": 1}))
        .await
        .unwrap_err();
    assert_eq!(heads.lock().unwrap().len(), 1);
    match &err {
        ClientError::OutcomeUnknown { status, source, .. } => {
            assert_eq!(*status, None);
            assert!(source.as_ref().is_some_and(reqwest::Error::is_timeout));
        }
        other => panic!("{other:?}"),
    }
    assert!(std::error::Error::source(&err).is_some());
}

#[tokio::test]
async fn a_post_cut_off_after_sending_is_not_repeated() {
    let (base, heads) = scripted_steps(vec![Step::Close, Step::Answer(200, "{}")]).await;
    let err = client(&base)
        .post::<_, serde_json::Value>("/api/x", &serde_json::json!({"a": 1}))
        .await
        .unwrap_err();
    assert_eq!(heads.lock().unwrap().len(), 1);
    assert!(
        matches!(
            &err,
            ClientError::OutcomeUnknown {
                status: None,
                source: Some(_),
                ..
            }
        ),
        "{err:?}"
    );
}

#[tokio::test]
async fn a_post_whose_answer_was_cut_short_is_not_repeated() {
    let (base, heads) = scripted_steps(vec![Step::Truncated(201), Step::Answer(201, "{}")]).await;
    let err = client(&base)
        .post::<_, serde_json::Value>("/api/x", &serde_json::json!({"a": 1}))
        .await
        .unwrap_err();
    assert_eq!(heads.lock().unwrap().len(), 1);
    assert!(
        matches!(
            &err,
            ClientError::OutcomeUnknown {
                status: Some(201),
                source: Some(_),
                ..
            }
        ),
        "{err:?}"
    );
}

#[tokio::test]
async fn a_get_that_timed_out_is_retried() {
    let (base, heads) = scripted_steps(vec![Step::Hang, Step::Answer(200, r#"{"ok":true}"#)]).await;
    let v: serde_json::Value = client(&base)
        .with_timeout(Duration::from_millis(200))
        .get("/api/x")
        .await
        .unwrap();
    assert_eq!(v["ok"], true);
    assert_eq!(heads.lock().unwrap().len(), 2);
}

#[tokio::test]
async fn provisioning_answered_502_is_sent_once_and_its_outcome_is_unknown() {
    // As reported: the platform committed the provisioning and the gateway
    // answered 502. A repeat would be refused 409 ALREADY_PROVISIONED and the
    // one-time secrets lost; the SDK must not send it again.
    let (base, heads) = scripted(vec![
        (502, "<html>502 Bad Gateway</html>"),
        (
            409,
            r#"{"error":"ALREADY_PROVISIONED","message":"application already has a service account"}"#,
        ),
    ])
    .await;
    let err = client(&base)
        .applications()
        .provision_service_account("app_1")
        .await
        .unwrap_err();
    assert_eq!(
        methods(&heads),
        vec!["POST /api/applications/app_1/provision-service-account HTTP/1.1"],
        "exactly one request; the scripted 409 is never requested"
    );
    assert!(err.is_outcome_unknown(), "{err:?}");
    assert!(!err.is_conflict());
    assert_eq!(err.status(), Some(502));
}

#[tokio::test]
async fn with_no_retries_a_get_answered_502_is_the_ordinary_error() {
    let (base, heads) = scripted(vec![(502, "")]).await;
    let err = FlowCatalystClient::new(&base)
        .with_retry(0, Duration::from_millis(1))
        .get::<serde_json::Value>("/api/x")
        .await
        .unwrap_err();
    assert!(
        matches!(err, ClientError::Api { status: 502, .. }),
        "{err:?}"
    );
    assert_eq!(heads.lock().unwrap().len(), 1);
}
