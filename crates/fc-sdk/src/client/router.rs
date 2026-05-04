//! Message-router monitoring endpoints.
//!
//! These call the **router** (a separate process from the platform) at the
//! `router_base_url` configured on the client via `.with_router_url(...)`.
//! If no router URL is configured, calls fall back to the platform's
//! `base_url`, which is correct only when the router and platform are
//! co-located (e.g. `fc-dev`).
//!
//! Designed for an external recovery / replay process that maintains its
//! own list of "messages that look stuck" and wants to confirm whether the
//! router is still actively processing each one before re-enqueueing.

use std::collections::HashMap;
use serde::{Deserialize, Serialize};

use super::{FlowCatalystClient, ClientError};

/// Response from `GET /monitoring/in-flight-messages/check`.
///
/// `inPipeline=true` → the router currently holds the message; the caller
/// should not re-enqueue. `inPipeline=false` → safe to resend.
#[derive(Debug, Clone, Serialize, Deserialize)]
#[serde(rename_all = "camelCase")]
pub struct InPipelineCheckResponse {
    pub message_id: String,
    pub in_pipeline: bool,
    /// Populated only when `in_pipeline = true`.
    #[serde(default, skip_serializing_if = "Option::is_none")]
    pub detail: Option<InPipelineDetail>,
}

#[derive(Debug, Clone, Serialize, Deserialize)]
#[serde(rename_all = "camelCase")]
pub struct InPipelineDetail {
    pub message_id: String,
    pub broker_message_id: Option<String>,
    pub queue_id: String,
    pub pool_code: String,
    pub elapsed_time_ms: u64,
    pub added_to_in_pipeline_at: String,
}

/// Body of `POST /monitoring/in-flight-messages/check-batch`.
///
/// Capped at 5000 ids per request server-side.
#[derive(Debug, Clone, Serialize, Deserialize)]
#[serde(rename_all = "camelCase")]
pub struct InPipelineBatchRequest {
    pub message_ids: Vec<String>,
}

impl FlowCatalystClient {
    fn router_url(&self, path: &str) -> String {
        let base = self
            .router_base_url
            .as_deref()
            .unwrap_or(&self.base_url);
        format!("{}{}", base, path)
    }

    /// Check whether a single application message ID is currently held in
    /// the router's in-pipeline map. O(1) on the server side.
    pub async fn is_message_in_pipeline(
        &self,
        message_id: &str,
    ) -> Result<InPipelineCheckResponse, ClientError> {
        let url = self.router_url("/monitoring/in-flight-messages/check");
        let resp = self
            .http
            .get(&url)
            .headers(self.headers())
            .query(&[("messageId", message_id)])
            .send()
            .await
            .map_err(ClientError::Request)?;

        let status = resp.status();
        if !status.is_success() {
            let body = resp.text().await.unwrap_or_default();
            return Err(ClientError::Api { status: status.as_u16(), body });
        }
        resp.json().await.map_err(ClientError::Request)
    }

    /// Batch-check whether each of the given application message IDs is
    /// currently held in the router's in-pipeline map. Returns
    /// `messageId → bool`. The server caps the batch at 5000 ids.
    pub async fn are_messages_in_pipeline(
        &self,
        message_ids: &[String],
    ) -> Result<HashMap<String, bool>, ClientError> {
        let url = self.router_url("/monitoring/in-flight-messages/check-batch");
        let body = InPipelineBatchRequest {
            message_ids: message_ids.to_vec(),
        };
        let resp = self
            .http
            .post(&url)
            .headers(self.headers())
            .json(&body)
            .send()
            .await
            .map_err(ClientError::Request)?;

        let status = resp.status();
        if !status.is_success() {
            let body = resp.text().await.unwrap_or_default();
            return Err(ClientError::Api { status: status.as_u16(), body });
        }
        resp.json().await.map_err(ClientError::Request)
    }
}
