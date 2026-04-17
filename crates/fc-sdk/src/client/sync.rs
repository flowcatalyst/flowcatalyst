//! Sync operations for bulk reconciliation.
//!
//! Sync endpoints reconcile application-scoped resources with a declarative manifest.
//! They create, update, and optionally delete resources to match the provided list.

use serde::{Deserialize, Serialize};
use super::{FlowCatalystClient, ClientError};
use super::event_types::CreateEventTypeRequest;

/// Result of a sync operation.
#[derive(Debug, Clone, Serialize, Deserialize)]
#[serde(rename_all = "camelCase")]
pub struct SyncResult {
    pub application_code: String,
    pub created: u32,
    pub updated: u32,
    pub deleted: u32,
    pub synced_codes: Vec<String>,
}

/// Request to sync roles.
#[derive(Debug, Clone, Serialize, Deserialize)]
pub struct SyncRolesRequest {
    pub roles: Vec<SyncRoleItem>,
}

/// A role item for sync.
#[derive(Debug, Clone, Serialize, Deserialize)]
#[serde(rename_all = "camelCase")]
pub struct SyncRoleItem {
    pub name: String,
    #[serde(skip_serializing_if = "Option::is_none")]
    pub display_name: Option<String>,
    #[serde(skip_serializing_if = "Option::is_none")]
    pub description: Option<String>,
    #[serde(default)]
    pub permissions: Vec<String>,
    #[serde(default)]
    pub client_managed: bool,
}

/// Request to sync event types.
#[derive(Debug, Clone, Serialize, Deserialize)]
#[serde(rename_all = "camelCase")]
pub struct SyncEventTypesRequest {
    pub event_types: Vec<CreateEventTypeRequest>,
}

/// Request to sync subscriptions.
#[derive(Debug, Clone, Serialize, Deserialize)]
pub struct SyncSubscriptionsRequest {
    pub subscriptions: Vec<SyncSubscriptionItem>,
}

/// A subscription item for sync — matches platform's SyncSubscriptionInput.
#[derive(Debug, Clone, Serialize, Deserialize)]
#[serde(rename_all = "camelCase")]
pub struct SyncSubscriptionItem {
    pub code: String,
    pub name: String,
    #[serde(skip_serializing_if = "Option::is_none")]
    pub description: Option<String>,
    /// Webhook endpoint URL
    pub target: String,
    #[serde(skip_serializing_if = "Option::is_none")]
    pub connection_id: Option<String>,
    pub event_types: Vec<SyncEventTypeBinding>,
    #[serde(skip_serializing_if = "Option::is_none")]
    pub dispatch_pool_code: Option<String>,
    #[serde(skip_serializing_if = "Option::is_none")]
    pub mode: Option<String>,
    #[serde(skip_serializing_if = "Option::is_none")]
    pub max_retries: Option<u32>,
    #[serde(skip_serializing_if = "Option::is_none")]
    pub timeout_seconds: Option<u32>,
    #[serde(default)]
    pub data_only: bool,
}

/// Event type binding for subscription sync.
#[derive(Debug, Clone, Serialize, Deserialize)]
#[serde(rename_all = "camelCase")]
pub struct SyncEventTypeBinding {
    pub event_type_code: String,
    #[serde(skip_serializing_if = "Option::is_none")]
    pub filter: Option<String>,
}

/// Request to sync dispatch pools.
#[derive(Debug, Clone, Serialize, Deserialize)]
#[serde(rename_all = "camelCase")]
pub struct SyncDispatchPoolsRequest {
    pub dispatch_pools: Vec<SyncDispatchPoolItem>,
}

/// A dispatch pool item for sync.
#[derive(Debug, Clone, Serialize, Deserialize)]
#[serde(rename_all = "camelCase")]
pub struct SyncDispatchPoolItem {
    pub code: String,
    #[serde(skip_serializing_if = "Option::is_none")]
    pub name: Option<String>,
    #[serde(skip_serializing_if = "Option::is_none")]
    pub concurrency: Option<u32>,
    /// Messages per minute. The backend's camelCase field is `rateLimit`.
    #[serde(skip_serializing_if = "Option::is_none")]
    pub rate_limit: Option<u32>,
}

impl FlowCatalystClient {
    /// Sync roles for an application.
    pub async fn sync_roles(
        &self,
        app_code: &str,
        req: &SyncRolesRequest,
        remove_unlisted: bool,
    ) -> Result<SyncResult, ClientError> {
        let query = if remove_unlisted { "?removeUnlisted=true" } else { "" };
        self.post(
            &format!("/api/applications/{}/roles/sync{}", app_code, query),
            req,
        )
        .await
    }

    /// Sync event types for an application.
    pub async fn sync_event_types(
        &self,
        app_code: &str,
        req: &SyncEventTypesRequest,
        remove_unlisted: bool,
    ) -> Result<SyncResult, ClientError> {
        let query = if remove_unlisted { "?removeUnlisted=true" } else { "" };
        self.post(
            &format!("/api/applications/{}/event-types/sync{}", app_code, query),
            req,
        )
        .await
    }

    /// Sync subscriptions for an application.
    pub async fn sync_subscriptions(
        &self,
        app_code: &str,
        req: &SyncSubscriptionsRequest,
        remove_unlisted: bool,
    ) -> Result<SyncResult, ClientError> {
        let query = if remove_unlisted { "?removeUnlisted=true" } else { "" };
        self.post(
            &format!("/api/applications/{}/subscriptions/sync{}", app_code, query),
            req,
        )
        .await
    }

    /// Sync dispatch pools for an application.
    pub async fn sync_dispatch_pools(
        &self,
        app_code: &str,
        req: &SyncDispatchPoolsRequest,
        remove_unlisted: bool,
    ) -> Result<SyncResult, ClientError> {
        let query = if remove_unlisted { "?removeUnlisted=true" } else { "" };
        self.post(
            &format!("/api/applications/{}/dispatch-pools/sync{}", app_code, query),
            req,
        )
        .await
    }
}
