//! OpenAPI spec sync against the platform's catalogue.
//!
//! Exactly one endpoint: `POST /api/applications/{appCode}/openapi/sync`.
//! Takes an arbitrary OpenAPI 3.x / Swagger 2.x JSON document as the body
//! (`{ spec: ... }`) and returns the platform's record of what was stored
//! (version, status, breaking-change flag, etc.).
//!
//! Unlike the other per-resource syncs the body isn't a list — each call
//! replaces this application's currently-published spec. Re-syncing the
//! same content is detected on the server side via `unchanged: true`.

use super::{ClientError, FlowCatalystClient};
use crate::client::seg;
use serde::{Deserialize, Serialize};

#[derive(Debug, Clone, Serialize, Deserialize)]
#[serde(rename_all = "camelCase")]
pub struct SyncOpenApiSpecRequest {
    pub spec: serde_json::Value,
}

#[derive(Debug, Clone, Serialize, Deserialize)]
#[serde(rename_all = "camelCase")]
pub struct SyncOpenApiSpecResponse {
    pub application_code: String,
    pub spec_id: String,
    pub version: String,
    pub status: String,
    #[serde(skip_serializing_if = "Option::is_none")]
    pub archived_prior_version: Option<String>,
    pub has_breaking: bool,
    /// True when the submitted spec is byte-identical to the currently
    /// published version. The platform short-circuits and returns the
    /// existing `spec_id` / `version` in that case.
    pub unchanged: bool,
}

/// OpenAPI resource accessor — created via [`FlowCatalystClient::openapi`].
#[derive(Debug, Clone, Copy)]
pub struct OpenApi<'a> {
    pub(crate) client: &'a FlowCatalystClient,
}

impl OpenApi<'_> {
    /// Publish or replace this application's OpenAPI document.
    pub async fn sync(
        &self,
        app_code: &str,
        spec: serde_json::Value,
    ) -> Result<SyncOpenApiSpecResponse, ClientError> {
        let req = SyncOpenApiSpecRequest { spec };
        self.client
            .post(
                &format!("/api/applications/{}/openapi/sync", seg(app_code)),
                &req,
            )
            .await
    }
}
