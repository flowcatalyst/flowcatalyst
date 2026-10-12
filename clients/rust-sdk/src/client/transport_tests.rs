//! Transport behaviour: headers, retries, the 401 refresh, error mapping.

use std::sync::atomic::{AtomicUsize, Ordering};
use std::sync::{Arc, Mutex};
use std::time::Duration;

use tokio::io::{AsyncReadExt, AsyncWriteExt};
use tokio::net::TcpListener;

use super::*;

/// A server answering each connection with the next scripted `(status, body)`
/// (the last one repeats), recording each request's head.
async fn scripted(script: Vec<(u16, &'static str)>) -> (String, Arc<Mutex<Vec<String>>>) {
    let listener = TcpListener::bind("127.0.0.1:0").await.unwrap();
    let base = format!("http://{}", listener.local_addr().unwrap());
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
                let (status, body) = script[i];
                let resp = format!(
                    "HTTP/1.1 {status} X\r\nContent-Type: application/json\r\nContent-Length: {}\r\nConnection: close\r\n\r\n{body}",
                    body.len()
                );
                socket.write_all(resp.as_bytes()).await.ok();
            });
        }
    });
    (base, heads)
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
