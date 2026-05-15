//! Low-level per-category sync wrappers.
//!
//! Sync endpoints reconcile application-scoped resources with a declarative
//! manifest. Each method here covers exactly one category and is a thin
//! POST to `/api/applications/{app}/{resource}/sync`.
//!
//! For bundled, multi-category sync against a single application, use the
//! orchestrator at [`crate::sync`] (the `DefinitionSynchronizer`) — it
//! delegates to these methods in deterministic order and collects
//! per-category results.
//!
//! Exception: `sync_processes` lives on `super::processes` because the
//! processes sync endpoint is not app-scoped in the URL — the application
//! code travels in the body.

use super::event_types::CreateEventTypeRequest;
use super::{ClientError, FlowCatalystClient};
use serde::{Deserialize, Serialize};

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
///
/// The platform's app-scoped endpoint expects `{ pools: [...] }`, NOT
/// `{ dispatchPools: [...] }` (see `shared/sdk_sync_api.rs`).
#[derive(Debug, Clone, Serialize, Deserialize)]
#[serde(rename_all = "camelCase")]
pub struct SyncDispatchPoolsRequest {
    pub pools: Vec<SyncDispatchPoolItem>,
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
    #[serde(skip_serializing_if = "Option::is_none")]
    pub description: Option<String>,
}

/// Request to sync principals.
#[derive(Debug, Clone, Serialize, Deserialize)]
#[serde(rename_all = "camelCase")]
pub struct SyncPrincipalsRequest {
    pub principals: Vec<SyncPrincipalItem>,
}

/// A principal item for sync. Matched by email.
#[derive(Debug, Clone, Serialize, Deserialize)]
#[serde(rename_all = "camelCase")]
pub struct SyncPrincipalItem {
    pub email: String,
    pub name: String,
    /// Role short names (without the `<app>:` prefix — the platform adds it).
    #[serde(default)]
    pub roles: Vec<String>,
    /// Defaults to `true` server-side when omitted.
    #[serde(default = "default_true")]
    pub active: bool,
}

fn default_true() -> bool {
    true
}

impl FlowCatalystClient {
    /// Sync roles for an application.
    pub async fn sync_roles(
        &self,
        app_code: &str,
        req: &SyncRolesRequest,
        remove_unlisted: bool,
    ) -> Result<SyncResult, ClientError> {
        let query = if remove_unlisted {
            "?removeUnlisted=true"
        } else {
            ""
        };
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
        let query = if remove_unlisted {
            "?removeUnlisted=true"
        } else {
            ""
        };
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
        let query = if remove_unlisted {
            "?removeUnlisted=true"
        } else {
            ""
        };
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
        let query = if remove_unlisted {
            "?removeUnlisted=true"
        } else {
            ""
        };
        self.post(
            &format!(
                "/api/applications/{}/dispatch-pools/sync{}",
                app_code, query
            ),
            req,
        )
        .await
    }

    /// Sync principals (users + role assignments) for an application.
    ///
    /// When `remove_unlisted` is true the platform strips SDK-sourced role
    /// assignments from principals not in the list (principals themselves
    /// are never deleted by sync).
    pub async fn sync_principals(
        &self,
        app_code: &str,
        req: &SyncPrincipalsRequest,
        remove_unlisted: bool,
    ) -> Result<SyncResult, ClientError> {
        let query = if remove_unlisted {
            "?removeUnlisted=true"
        } else {
            ""
        };
        self.post(
            &format!("/api/applications/{}/principals/sync{}", app_code, query),
            req,
        )
        .await
    }
}
