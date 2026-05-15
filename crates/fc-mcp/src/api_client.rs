use std::sync::Arc;

use anyhow::{anyhow, Result};
use reqwest::Client;
use serde_json::Value;

use crate::auth::TokenManager;
use crate::config::Config;

/// Thin HTTP wrapper. Responses come back as `serde_json::Value` so the MCP
/// tools can round-trip them as JSON text without re-modelling every DTO; the
/// platform's `EventTypeResponse` / `SubscriptionResponse` evolve, and we don't
/// want the MCP layer to be a second source of truth for those shapes.
pub struct ApiClient {
    http: Client,
    base_api: String,
    tokens: Arc<TokenManager>,
}

#[derive(Default, Debug)]
pub struct ListEventTypesFilters<'a> {
    pub status: Option<&'a str>,
    pub application: Option<&'a str>,
    pub client_id: Option<&'a str>,
    pub subdomain: Option<&'a str>,
    pub aggregate: Option<&'a str>,
}

impl ApiClient {
    pub fn new(config: &Config, http: Client, tokens: Arc<TokenManager>) -> Self {
        Self {
            base_api: format!("{}/api", config.base_url),
            http,
            tokens,
        }
    }

    pub async fn list_event_types(&self, filters: &ListEventTypesFilters<'_>) -> Result<Value> {
        let mut req = self.http.get(format!("{}/event-types", self.base_api));
        let mut query: Vec<(&str, &str)> = Vec::new();
        if let Some(s) = filters.status {
            query.push(("status", s));
        }
        if let Some(a) = filters.application {
            query.push(("application", a));
        }
        if let Some(c) = filters.client_id {
            query.push(("clientId", c));
        }
        if let Some(sd) = filters.subdomain {
            query.push(("subdomain", sd));
        }
        if let Some(ag) = filters.aggregate {
            query.push(("aggregate", ag));
        }
        if !query.is_empty() {
            req = req.query(&query);
        }
        self.send(req).await
    }

    pub async fn get_event_type(&self, id: &str) -> Result<Value> {
        let url = format!("{}/event-types/{}", self.base_api, urlencode(id));
        self.send(self.http.get(url)).await
    }

    pub async fn list_subscriptions(&self, client_id: Option<&str>) -> Result<Value> {
        let mut req = self.http.get(format!("{}/subscriptions", self.base_api));
        if let Some(c) = client_id {
            req = req.query(&[("clientId", c)]);
        }
        self.send(req).await
    }

    pub async fn get_subscription(&self, id: &str) -> Result<Value> {
        let url = format!("{}/subscriptions/{}", self.base_api, urlencode(id));
        self.send(self.http.get(url)).await
    }

    async fn send(&self, req: reqwest::RequestBuilder) -> Result<Value> {
        let token = self.tokens.get_access_token().await?;
        let resp = req
            .header("Accept", "application/json")
            .bearer_auth(token)
            .send()
            .await?;
        let status = resp.status();
        if !status.is_success() {
            let body = resp.text().await.unwrap_or_default();
            return Err(anyhow!("platform request failed ({status}): {body}"));
        }
        Ok(resp.json::<Value>().await?)
    }
}

fn urlencode(s: &str) -> String {
    // Only encodes path-segment-dangerous characters. IDs are TSIDs so this is
    // effectively a passthrough, but covers operators who paste a code with a
    // colon or slash.
    let mut out = String::with_capacity(s.len());
    for b in s.bytes() {
        match b {
            b'A'..=b'Z' | b'a'..=b'z' | b'0'..=b'9' | b'-' | b'_' | b'.' | b'~' => {
                out.push(b as char)
            }
            _ => out.push_str(&format!("%{:02X}", b)),
        }
    }
    out
}
