//! Platform configuration: list, get, set, delete and reveal properties.
//!
//! Properties of `valueType` `SECRET` are always returned as the literal
//! `"***"` by [`PlatformConfig::list`], [`PlatformConfig::get`] and
//! [`PlatformConfig::set`], for every caller. The only way to read a secret's
//! plaintext is [`PlatformConfig::reveal_secret`], which the platform audits.
//!
//! Reveal is always a permission, never implied by being the application's
//! service account: `platform:admin:config:reveal` (a platform administrator)
//! or `platform:application-service:config:reveal` (an application's service
//! account; the `platform:application-service` role carries it). The platform
//! confines an application's service account to its own applications and to
//! the clients it is linked to (its own client list, else the clients its
//! application is enabled for), plus the application-wide value.
//!
//! A service account acting for one of its application's signed-in users says
//! so with [`PlatformConfig::on_behalf_of`]: set, delete and reveal then send
//! `X-On-Behalf-Of`, which the platform records in the audit row.

use super::{ClientError, FlowCatalystClient};
use crate::client::seg;
use reqwest::header::HeaderValue;
use serde::{Deserialize, Serialize};
use std::fmt;

/// The header that says who a calling service account acts for (for example
/// the application's signed-in user) on set, delete and reveal. Honoured only
/// from a service account and ignored from anyone else; at most 256 printable
/// characters (no control characters), else `400 INVALID_ON_BEHALF_OF`.
pub const ON_BEHALF_OF_HEADER: &str = "X-On-Behalf-Of";

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
    /// The client's identifier (e.g. `acme`) beside the stored `client_id`, so
    /// a caller holding either can match the row.
    #[serde(default, skip_serializing_if = "Option::is_none")]
    pub client_identifier: Option<String>,
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

/// Request body for [`PlatformConfig::set`]. A `SECRET`'s value is required
/// and the mask is not a value: `"***"` is refused (`400 SECRET_VALUE_MASKED`),
/// as is an empty secret (`400 SECRET_VALUE_REQUIRED`). To leave a secret
/// unchanged, do not set it.
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
    pub(crate) on_behalf_of: Option<&'a str>,
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

impl<'a> PlatformConfig<'a> {
    /// The same resource, whose set, delete and reveal calls say who the
    /// calling service account acts for ([`ON_BEHALF_OF_HEADER`]). A blank
    /// `who` sends nothing; one that cannot be a header value (a control
    /// character) fails the call with [`ClientError::InvalidHeader`] before
    /// anything is sent.
    #[must_use]
    pub fn on_behalf_of(self, who: &'a str) -> Self {
        Self {
            on_behalf_of: Some(who),
            ..self
        }
    }

    fn actor(&self) -> Result<Vec<(&'static str, HeaderValue)>, ClientError> {
        match self.on_behalf_of.map(str::trim).filter(|w| !w.is_empty()) {
            None => Ok(Vec::new()),
            Some(who) => HeaderValue::from_str(who)
                .map(|v| vec![(ON_BEHALF_OF_HEADER, v)])
                .map_err(|_| ClientError::InvalidHeader(ON_BEHALF_OF_HEADER)),
        }
    }

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
        let headers = self.actor()?;
        self.client
            .put_with(
                &format!(
                    "{}{}",
                    prop_path(app, section, property),
                    client_query(client_id)
                ),
                req,
                &headers,
            )
            .await
    }

    /// Delete a property (204; audited; one that is not there is not an error).
    pub async fn delete(
        &self,
        app: &str,
        section: &str,
        property: &str,
        client_id: Option<&str>,
    ) -> Result<(), ClientError> {
        let headers = self.actor()?;
        self.client
            .delete_with(
                &format!(
                    "{}{}",
                    prop_path(app, section, property),
                    client_query(client_id)
                ),
                &headers,
            )
            .await
    }

    /// Reveal a `SECRET` property's plaintext. Every call is audited
    /// server-side (with the [`on_behalf_of`](Self::on_behalf_of) principal
    /// when one is given) and is never cached by the SDK. The value is
    /// sensitive: do not log it.
    ///
    /// Errors: `403` (needs `platform:admin:config:reveal`, or
    /// `platform:application-service:config:reveal` for an application's
    /// service account, which reaches only its own applications and the
    /// clients it is linked to: its own client list, else the clients its
    /// application is enabled for, plus the application-wide value), `404`
    /// (no such property), `400` code `NOT_A_SECRET` (the property is `PLAIN`;
    /// use [`get`](Self::get)).
    pub async fn reveal_secret(
        &self,
        app: &str,
        section: &str,
        property: &str,
        client_id: Option<&str>,
    ) -> Result<SecretResponse, ClientError> {
        let headers = self.actor()?;
        self.client
            .get_with(
                &format!(
                    "{}/secret{}",
                    prop_path(app, section, property),
                    client_query(client_id)
                ),
                &headers,
            )
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
        assert_eq!(reqs[1].header("x-on-behalf-of"), None);
    }

    #[tokio::test]
    async fn on_behalf_of_is_sent_on_set_delete_and_reveal_only_when_given() {
        let cfg = r#"{"id":"c1","applicationCode":"orders","section":"smtp","property":"pw","scope":"CLIENT","clientId":"clt_1","clientIdentifier":"acme","valueType":"SECRET","value":"***","createdAt":"t","updatedAt":"t"}"#;
        let stub = MockPlatform::start(&[
            ("PUT", "/api/config/orders/smtp/pw", 200, cfg),
            ("DELETE", "/api/config/orders/smtp/pw", 204, ""),
            ("GET", "/api/config/orders/smtp/pw/secret", 200, SECRET),
        ])
        .await;
        let c = stub.client();
        let req = SetPropertyRequest {
            value: "x".into(),
            value_type: Some("SECRET".into()),
            ..Default::default()
        };
        let set = c
            .platform_config()
            .on_behalf_of("usr:alice@example.com")
            .set("orders", "smtp", "pw", None, &req)
            .await
            .unwrap();
        assert_eq!(set.client_identifier.as_deref(), Some("acme"));
        c.platform_config()
            .on_behalf_of("bob")
            .delete("orders", "smtp", "pw", Some("clt_1"))
            .await
            .unwrap();
        c.platform_config()
            .on_behalf_of("  ")
            .delete("orders", "smtp", "pw", None)
            .await
            .unwrap();
        c.platform_config()
            .on_behalf_of("carol")
            .reveal_secret("orders", "smtp", "pw", None)
            .await
            .unwrap();
        let reqs = stub.requests();
        assert_eq!(
            reqs[0].header("x-on-behalf-of").as_deref(),
            Some("usr:alice@example.com")
        );
        assert_eq!(reqs[1].header("x-on-behalf-of").as_deref(), Some("bob"));
        assert_eq!(reqs[1].query, "clientId=clt_1");
        assert_eq!(reqs[2].header("x-on-behalf-of"), None);
        assert_eq!(reqs[3].header("x-on-behalf-of").as_deref(), Some("carol"));

        let err = c
            .platform_config()
            .on_behalf_of("bad\nvalue")
            .reveal_secret("orders", "smtp", "pw", None)
            .await
            .unwrap_err();
        assert!(matches!(
            err,
            ClientError::InvalidHeader(ON_BEHALF_OF_HEADER)
        ));
        assert_eq!(
            stub.requests().len(),
            4,
            "nothing sent for an invalid header"
        );
    }
}
