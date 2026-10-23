//! Platform configuration: list, get, set, delete and reveal properties.
//!
//! Properties of `valueType` `SECRET` are always returned as the literal
//! `"***"` by [`PlatformConfig::list`], [`PlatformConfig::get`] and
//! [`PlatformConfig::set`], for every caller. The only way to read a secret's
//! plaintext is [`PlatformConfig::reveal_secret`], which the platform audits.

use super::{ClientError, FlowCatalystClient};
use crate::client::seg;
use serde::{Deserialize, Serialize};
use std::fmt;

/// A configuration property. For `value_type == "SECRET"` the `value` is
/// always `"***"`; use [`PlatformConfig::reveal_secret`] for the plaintext.
#[derive(Debug, Clone, Serialize, Deserialize)]
#[serde(rename_all = "camelCase")]
pub struct ConfigResponse {
    pub id: String,
    pub application_code: String,
    pub section: String,
    pub property: String,
    pub scope: String,
    /// `PLAIN` or `SECRET`.
    pub value_type: String,
    /// The value; the literal `"***"` for a `SECRET` property.
    pub value: String,
    #[serde(default, skip_serializing_if = "Option::is_none")]
    pub client_id: Option<String>,
    #[serde(default, skip_serializing_if = "Option::is_none")]
    pub description: Option<String>,
    pub created_at: String,
    pub updated_at: String,
}

/// `GET /api/platform-config/{app}`.
#[derive(Debug, Clone, Serialize, Deserialize)]
pub struct ConfigListResponse {
    pub items: Vec<ConfigResponse>,
}

/// Request body for [`PlatformConfig::set`].
#[derive(Debug, Clone, Serialize, Deserialize, Default)]
#[serde(rename_all = "camelCase")]
pub struct SetPropertyRequest {
    pub value: String,
    /// `PLAIN` (default) or `SECRET`.
    #[serde(skip_serializing_if = "Option::is_none")]
    pub value_type: Option<String>,
    #[serde(skip_serializing_if = "Option::is_none")]
    pub description: Option<String>,
}

/// A secret's plaintext. `Debug` and `Display` print `***`; read it with
/// [`expose`](Self::expose). It is deliberately not `Serialize`.
#[derive(Clone, Deserialize, PartialEq, Eq)]
#[serde(transparent)]
pub struct SecretValue(String);

impl SecretValue {
    /// The plaintext. Do not log it.
    pub fn expose(&self) -> &str {
        &self.0
    }
}

impl fmt::Debug for SecretValue {
    fn fmt(&self, f: &mut fmt::Formatter<'_>) -> fmt::Result {
        f.write_str("SecretValue(***)")
    }
}

impl fmt::Display for SecretValue {
    fn fmt(&self, f: &mut fmt::Formatter<'_>) -> fmt::Result {
        f.write_str("***")
    }
}

/// The revealed secret — `GET /api/config/{app}/{section}/{property}/secret`.
#[derive(Debug, Clone, Deserialize)]
#[serde(rename_all = "camelCase")]
pub struct SecretResponse {
    pub application_code: String,
    pub section: String,
    pub property: String,
    #[serde(default)]
    pub client_id: Option<String>,
    /// The plaintext (redacted in `Debug`).
    pub value: SecretValue,
    /// The value's version; changes whenever the secret is set.
    pub updated_at: String,
}

/// Platform-config resource accessor — created via
/// [`FlowCatalystClient::platform_config`].
#[derive(Debug, Clone, Copy)]
pub struct PlatformConfig<'a> {
    pub(crate) client: &'a FlowCatalystClient,
}

fn prop_path(app: &str, section: &str, property: &str) -> String {
    format!(
        "/api/config/{}/{}/{}",
        seg(app),
        seg(section),
        seg(property)
    )
}

fn client_query(client_id: Option<&str>) -> String {
    let params: Vec<(&str, String)> = client_id
        .map(|v| vec![("clientId", v.to_string())])
        .unwrap_or_default();
    FlowCatalystClient::query_string(&params)
}

impl PlatformConfig<'_> {
    /// List an application's properties. `SECRET` values are `"***"`.
    pub async fn list(&self, app: &str) -> Result<ConfigListResponse, ClientError> {
        self.client
            .get(&format!("/api/platform-config/{}", seg(app)))
            .await
    }

    /// Get one property. A `SECRET` value is `"***"`; see
    /// [`reveal_secret`](Self::reveal_secret).
    pub async fn get(
        &self,
        app: &str,
        section: &str,
        property: &str,
        client_id: Option<&str>,
    ) -> Result<ConfigResponse, ClientError> {
        self.client
            .get(&format!(
                "{}{}",
                prop_path(app, section, property),
                client_query(client_id)
            ))
            .await
    }

    /// Set (create or replace) a property.
    pub async fn set(
        &self,
        app: &str,
        section: &str,
        property: &str,
        client_id: Option<&str>,
        req: &SetPropertyRequest,
    ) -> Result<ConfigResponse, ClientError> {
        self.client
            .put(
                &format!(
                    "{}{}",
                    prop_path(app, section, property),
                    client_query(client_id)
                ),
                req,
            )
            .await
    }

    /// Delete a property (204).
    pub async fn delete(
        &self,
        app: &str,
        section: &str,
        property: &str,
        client_id: Option<&str>,
    ) -> Result<(), ClientError> {
        self.client
            .delete_req(&format!(
                "{}{}",
                prop_path(app, section, property),
                client_query(client_id)
            ))
            .await
    }

    /// Reveal a `SECRET` property's plaintext. Every call is audited
    /// server-side and is never cached by the SDK. The value is sensitive:
    /// do not log it.
    ///
    /// Errors: `403` (needs `platform:admin:config:reveal`, or the
    /// application's own service account), `404` (no such property), `400`
    /// code `NOT_A_SECRET` (the property is `PLAIN`; use [`get`](Self::get)).
    pub async fn reveal_secret(
        &self,
        app: &str,
        section: &str,
        property: &str,
        client_id: Option<&str>,
    ) -> Result<SecretResponse, ClientError> {
        self.client
            .get(&format!(
                "{}/secret{}",
                prop_path(app, section, property),
                client_query(client_id)
            ))
            .await
    }
}

#[cfg(test)]
mod tests {
    use super::*;
    use crate::client::test_support::MockPlatform;

    const SECRET: &str = r#"{"applicationCode":"orders","section":"smtp","property":"pass word","value":"hunter2-plain","updatedAt":"2026-01-01T00:00:00Z"}"#;

    #[tokio::test]
    async fn reveal_encodes_segments_and_sends_client_id_only_when_given() {
        let stub = MockPlatform::start(&[(
            "GET",
            "/api/config/orders/smtp%2Fx/pass%20word/secret",
            200,
            SECRET,
        )])
        .await;
        let c = stub.client();
        let r = c
            .platform_config()
            .reveal_secret("orders", "smtp/x", "pass word", None)
            .await
            .unwrap();
        assert_eq!(r.value.expose(), "hunter2-plain");
        c.platform_config()
            .reveal_secret("orders", "smtp/x", "pass word", Some("clt 1"))
            .await
            .unwrap();
        let reqs = stub.requests();
        assert_eq!(reqs[0].method, "GET");
        assert_eq!(reqs[0].query, "");
        assert_eq!(
            reqs[1].query_pairs(),
            vec![("clientId".to_string(), "clt 1".to_string())]
        );
    }

    #[tokio::test]
    async fn reveal_error_statuses_map_to_api_errors() {
        for (status, body, code) in [
            (403, r#"{"error":"FORBIDDEN","message":"no"}"#, "FORBIDDEN"),
            (404, r#"{"error":"NOT_FOUND","message":"no"}"#, "NOT_FOUND"),
            (
                400,
                r#"{"error":"NOT_A_SECRET","message":"plain"}"#,
                "NOT_A_SECRET",
            ),
        ] {
            let stub =
                MockPlatform::start(&[("GET", "/api/config/a/s/p/secret", status, body)]).await;
            let err = stub
                .client()
                .platform_config()
                .reveal_secret("a", "s", "p", None)
                .await
                .unwrap_err();
            assert_eq!(err.status(), Some(status));
            match err {
                ClientError::Api { code: c, .. } => assert_eq!(c.as_deref(), Some(code)),
                other => panic!("expected Api error, got {other:?}"),
            }
        }
    }

    #[tokio::test]
    async fn the_secret_is_redacted_in_debug_and_display() {
        let stub =
            MockPlatform::start(&[("GET", "/api/config/orders/smtp/pw/secret", 200, SECRET)]).await;
        let r = stub
            .client()
            .platform_config()
            .reveal_secret("orders", "smtp", "pw", None)
            .await
            .unwrap();
        assert!(!format!("{r:?}").contains("hunter2-plain"));
        assert!(!format!("{r:#?}").contains("hunter2-plain"));
        assert!(!r.value.to_string().contains("hunter2-plain"));
        assert!(format!("{r:?}").contains("***"));
    }

    #[tokio::test]
    async fn get_set_delete_list_use_the_config_paths() {
        let cfg = r#"{"id":"c1","applicationCode":"orders","section":"smtp","property":"pw","scope":"GLOBAL","valueType":"SECRET","value":"***","createdAt":"t","updatedAt":"t"}"#;
        let list = format!(r#"{{"items":[{cfg}]}}"#);
        let stub = MockPlatform::start(&[
            ("GET", "/api/config/orders/smtp/pw", 200, cfg),
            ("PUT", "/api/config/orders/smtp/pw", 200, cfg),
            ("DELETE", "/api/config/orders/smtp/pw", 204, ""),
            ("GET", "/api/platform-config/orders", 200, &list),
        ])
        .await;
        let c = stub.client();
        let p = c.platform_config();
        assert_eq!(
            p.get("orders", "smtp", "pw", None).await.unwrap().value,
            "***"
        );
        p.set(
            "orders",
            "smtp",
            "pw",
            Some("clt_1"),
            &SetPropertyRequest {
                value: "x".into(),
                value_type: Some("SECRET".into()),
                ..Default::default()
            },
        )
        .await
        .unwrap();
        p.delete("orders", "smtp", "pw", None).await.unwrap();
        assert_eq!(p.list("orders").await.unwrap().items.len(), 1);
        let reqs = stub.requests();
        assert_eq!(reqs[1].method, "PUT");
        assert_eq!(reqs[1].query, "clientId=clt_1");
        assert_eq!(
            reqs[1].json(),
            serde_json::json!({"value":"x","valueType":"SECRET"})
        );
        assert_eq!(reqs[2].method, "DELETE");
    }
}
