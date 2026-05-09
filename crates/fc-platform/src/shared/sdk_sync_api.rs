//! SDK Sync API — application-scoped sync endpoints
//!
//! Provides sync routes scoped under /api/applications/:appCode for
//! roles, event types, subscriptions, dispatch pools, and principals.

use axum::{
    routing::post,
    extract::{State, Path, Query},
    Json, Router,
};
use utoipa::ToSchema;
use serde::{Deserialize, Serialize};
use std::sync::Arc;

use crate::role::operations::{
    SyncRolesCommand, SyncRolesUseCase, SyncRoleInput,
};
use crate::event_type::operations::{
    SyncEventTypesCommand, SyncEventTypesUseCase, SyncEventTypeInput,
};
use crate::subscription::operations::{
    SyncSubscriptionsCommand, SyncSubscriptionsUseCase, SyncSubscriptionInput,
    EventTypeBindingInput,
};
use crate::dispatch_pool::operations::{
    SyncDispatchPoolsCommand, SyncDispatchPoolsUseCase, SyncDispatchPoolInput,
};
use crate::principal::operations::{
    SyncPrincipalsCommand, SyncPrincipalsUseCase, SyncPrincipalInput,
};
use crate::scheduled_job::operations::{
    ScheduledJobSyncEntry, SyncScheduledJobsCommand, SyncScheduledJobsUseCase,
};
use crate::usecase::{ExecutionContext, UseCase, UseCaseResult};
use crate::shared::error::PlatformError;
use crate::shared::middleware::Authenticated;

// ---------------------------------------------------------------------------
// Shared types
// ---------------------------------------------------------------------------

/// Sync query parameters (shared across all sync endpoints)
#[derive(Debug, Default, Deserialize)]
#[serde(rename_all = "camelCase")]
pub struct SyncQuery {
    /// Remove items not in the sync list
    #[serde(default)]
    pub remove_unlisted: bool,
}

/// Sync result response (shared across all sync endpoints)
#[derive(Debug, Serialize, ToSchema)]
#[serde(rename_all = "camelCase")]
pub struct SyncResultResponse {
    pub application_code: String,
    pub created: u32,
    pub updated: u32,
    pub deleted: u32,
    pub synced_codes: Vec<String>,
}

// ---------------------------------------------------------------------------
// Roles sync
// ---------------------------------------------------------------------------

/// Sync roles request body
#[derive(Debug, Deserialize, ToSchema)]
#[serde(rename_all = "camelCase")]
pub struct SyncRolesRequest {
    pub roles: Vec<SyncRoleInputRequest>,
}

/// A single role input for sync
#[derive(Debug, Deserialize, ToSchema)]
#[serde(rename_all = "camelCase")]
pub struct SyncRoleInputRequest {
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

// ---------------------------------------------------------------------------
// Event types sync
// ---------------------------------------------------------------------------

/// Sync event types request body
#[derive(Debug, Deserialize, ToSchema)]
#[serde(rename_all = "camelCase")]
pub struct SyncEventTypesRequest {
    pub event_types: Vec<SyncEventTypeInputRequest>,
}

/// A single event type input for sync
#[derive(Debug, Deserialize, ToSchema)]
#[serde(rename_all = "camelCase")]
pub struct SyncEventTypeInputRequest {
    /// Full code (application:subdomain:aggregate:event)
    pub code: String,
    pub name: String,
    #[serde(skip_serializing_if = "Option::is_none")]
    pub description: Option<String>,
}

// ---------------------------------------------------------------------------
// Subscriptions sync
// ---------------------------------------------------------------------------

/// Sync subscriptions request body
#[derive(Debug, Deserialize, ToSchema)]
#[serde(rename_all = "camelCase")]
pub struct SyncSubscriptionsRequest {
    pub subscriptions: Vec<SyncSubscriptionInputRequest>,
}

/// A single subscription input for sync
#[derive(Debug, Deserialize, ToSchema)]
#[serde(rename_all = "camelCase")]
pub struct SyncSubscriptionInputRequest {
    pub code: String,
    pub name: String,
    #[serde(skip_serializing_if = "Option::is_none")]
    pub description: Option<String>,
    pub target: String,
    #[serde(skip_serializing_if = "Option::is_none")]
    pub connection_id: Option<String>,
    pub event_types: Vec<SyncSubscriptionEventTypeRequest>,
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

/// Event type binding for sync subscription input
#[derive(Debug, Deserialize, ToSchema)]
#[serde(rename_all = "camelCase")]
pub struct SyncSubscriptionEventTypeRequest {
    pub event_type_code: String,
    #[serde(skip_serializing_if = "Option::is_none")]
    pub filter: Option<String>,
}

// ---------------------------------------------------------------------------
// Dispatch pools sync
// ---------------------------------------------------------------------------

/// Sync dispatch pools request body
#[derive(Debug, Deserialize, ToSchema)]
#[serde(rename_all = "camelCase")]
pub struct SyncDispatchPoolsRequest {
    pub pools: Vec<SyncDispatchPoolInputRequest>,
}

/// A single dispatch pool input for sync
#[derive(Debug, Deserialize, ToSchema)]
#[serde(rename_all = "camelCase")]
pub struct SyncDispatchPoolInputRequest {
    pub code: String,
    pub name: String,
    #[serde(skip_serializing_if = "Option::is_none")]
    pub description: Option<String>,
    /// Optional. `None` / omitted = concurrency-only (no rate limit).
    #[serde(default, skip_serializing_if = "Option::is_none")]
    pub rate_limit: Option<u32>,
    #[serde(default = "default_concurrency")]
    pub concurrency: u32,
}

fn default_concurrency() -> u32 { 10 }

// ---------------------------------------------------------------------------
// Principals sync
// ---------------------------------------------------------------------------

/// Sync principals request body
#[derive(Debug, Deserialize, ToSchema)]
#[serde(rename_all = "camelCase")]
pub struct SyncPrincipalsRequest {
    pub principals: Vec<SyncPrincipalInputRequest>,
}

/// A single principal input for sync
#[derive(Debug, Deserialize, ToSchema)]
#[serde(rename_all = "camelCase")]
pub struct SyncPrincipalInputRequest {
    /// User's email address (unique identifier for matching)
    pub email: String,
    /// Display name
    pub name: String,
    /// Role short names to assign (prefixed with applicationCode)
    #[serde(default)]
    pub roles: Vec<String>,
    /// Whether the user is active (default: true)
    #[serde(default = "default_active")]
    pub active: bool,
}

fn default_active() -> bool { true }

// ---------------------------------------------------------------------------
// State
// ---------------------------------------------------------------------------

/// SDK Sync service state
#[derive(Clone)]
pub struct SdkSyncState {
    pub sync_roles_use_case: Arc<SyncRolesUseCase<crate::usecase::PgUnitOfWork>>,
    pub sync_event_types_use_case: Arc<SyncEventTypesUseCase<crate::usecase::PgUnitOfWork>>,
    pub sync_subscriptions_use_case: Arc<SyncSubscriptionsUseCase<crate::usecase::PgUnitOfWork>>,
    pub sync_dispatch_pools_use_case: Arc<SyncDispatchPoolsUseCase<crate::usecase::PgUnitOfWork>>,
    pub sync_principals_use_case: Arc<SyncPrincipalsUseCase<crate::usecase::PgUnitOfWork>>,
    pub sync_scheduled_jobs_use_case:
        Arc<SyncScheduledJobsUseCase<crate::usecase::PgUnitOfWork>>,
}

// ---------------------------------------------------------------------------
// Scheduled jobs sync
// ---------------------------------------------------------------------------

#[derive(Debug, Deserialize, ToSchema)]
#[serde(rename_all = "camelCase")]
pub struct SyncScheduledJobsRequest {
    /// None = sync platform-scoped jobs (anchor only).
    #[serde(skip_serializing_if = "Option::is_none")]
    pub client_id: Option<String>,
    pub jobs: Vec<SyncScheduledJobInputRequest>,
    #[serde(default)]
    pub archive_unlisted: bool,
}

#[derive(Debug, Deserialize, ToSchema)]
#[serde(rename_all = "camelCase")]
pub struct SyncScheduledJobInputRequest {
    pub code: String,
    pub name: String,
    #[serde(skip_serializing_if = "Option::is_none")]
    pub description: Option<String>,
    pub crons: Vec<String>,
    #[serde(default = "default_tz")]
    pub timezone: String,
    #[serde(skip_serializing_if = "Option::is_none")]
    pub payload: Option<serde_json::Value>,
    #[serde(default)]
    pub concurrent: bool,
    #[serde(default)]
    pub tracks_completion: bool,
    #[serde(skip_serializing_if = "Option::is_none")]
    pub timeout_seconds: Option<i32>,
    #[serde(default = "default_attempts")]
    pub delivery_max_attempts: i32,
    #[serde(skip_serializing_if = "Option::is_none")]
    pub target_url: Option<String>,
}
fn default_tz() -> String { "UTC".into() }
fn default_attempts() -> i32 { 3 }

#[derive(Debug, Serialize, ToSchema)]
#[serde(rename_all = "camelCase")]
pub struct SyncScheduledJobsResultResponse {
    pub application_code: String,
    pub created: Vec<String>,
    pub updated: Vec<String>,
    pub archived: Vec<String>,
}

// ---------------------------------------------------------------------------
// Handlers
// ---------------------------------------------------------------------------

/// Sync roles for an application
#[utoipa::path(
    post,
    path = "/{app_code}/roles/sync",
    tag = "sdk-sync",
    operation_id = "postApiApplicationsByAppCodeRolesSync",
    params(
        ("app_code" = String, Path, description = "Application code"),
        ("remove_unlisted" = Option<bool>, Query, description = "Remove SDK roles not in list")
    ),
    request_body = SyncRolesRequest,
    responses(
        (status = 200, description = "Roles synced", body = SyncResultResponse),
        (status = 400, description = "Validation error"),
        (status = 404, description = "Application not found")
    ),
    security(("bearer_auth" = []))
)]
async fn sync_roles(
    State(state): State<SdkSyncState>,
    auth: Authenticated,
    Path(app_code): Path<String>,
    Query(query): Query<SyncQuery>,
    Json(req): Json<SyncRolesRequest>,
) -> Result<Json<SyncResultResponse>, PlatformError> {
    let command = SyncRolesCommand {
        application_code: app_code,
        roles: req.roles.into_iter().map(|r| SyncRoleInput {
            name: r.name,
            display_name: r.display_name,
            description: r.description,
            permissions: r.permissions,
            client_managed: r.client_managed,
        }).collect(),
        remove_unlisted: query.remove_unlisted,
    };

    let ctx = ExecutionContext::create(auth.0.principal_id.clone());

    match state.sync_roles_use_case.run(command, ctx).await {
        UseCaseResult::Success(event) => {
            Ok(Json(SyncResultResponse {
                application_code: event.application_code,
                created: event.created,
                updated: event.updated,
                deleted: event.deleted,
                synced_codes: event.synced_names,
            }))
        }
        UseCaseResult::Failure(err) => Err(err.into()),
    }
}

/// Sync event types for an application
#[utoipa::path(
    post,
    path = "/{app_code}/event-types/sync",
    tag = "sdk-sync",
    operation_id = "postApiApplicationsByAppCodeEventTypesSync",
    params(
        ("app_code" = String, Path, description = "Application code"),
        ("remove_unlisted" = Option<bool>, Query, description = "Remove API-sourced event types not in list")
    ),
    request_body = SyncEventTypesRequest,
    responses(
        (status = 200, description = "Event types synced", body = SyncResultResponse),
        (status = 400, description = "Validation error")
    ),
    security(("bearer_auth" = []))
)]
async fn sync_event_types(
    State(state): State<SdkSyncState>,
    auth: Authenticated,
    Path(app_code): Path<String>,
    Query(query): Query<SyncQuery>,
    Json(req): Json<SyncEventTypesRequest>,
) -> Result<Json<SyncResultResponse>, PlatformError> {
    let command = SyncEventTypesCommand {
        application_code: app_code,
        event_types: req.event_types.into_iter().map(|et| SyncEventTypeInput {
            code: et.code,
            name: et.name,
            description: et.description,
            schema: None,
        }).collect(),
        remove_unlisted: query.remove_unlisted,
    };

    let ctx = ExecutionContext::create(auth.0.principal_id.clone());

    match state.sync_event_types_use_case.run(command, ctx).await {
        UseCaseResult::Success(event) => {
            Ok(Json(SyncResultResponse {
                application_code: event.application_code,
                created: event.created,
                updated: event.updated,
                deleted: event.deleted,
                synced_codes: event.synced_codes,
            }))
        }
        UseCaseResult::Failure(err) => Err(err.into()),
    }
}

/// Sync subscriptions for an application
#[utoipa::path(
    post,
    path = "/{app_code}/subscriptions/sync",
    tag = "sdk-sync",
    operation_id = "postApiApplicationsByAppCodeSubscriptionsSync",
    params(
        ("app_code" = String, Path, description = "Application code"),
        ("remove_unlisted" = Option<bool>, Query, description = "Remove API-sourced subscriptions not in list")
    ),
    request_body = SyncSubscriptionsRequest,
    responses(
        (status = 200, description = "Subscriptions synced", body = SyncResultResponse),
        (status = 400, description = "Validation error"),
        (status = 404, description = "Connection not found")
    ),
    security(("bearer_auth" = []))
)]
async fn sync_subscriptions(
    State(state): State<SdkSyncState>,
    auth: Authenticated,
    Path(app_code): Path<String>,
    Query(query): Query<SyncQuery>,
    Json(req): Json<SyncSubscriptionsRequest>,
) -> Result<Json<SyncResultResponse>, PlatformError> {
    let command = SyncSubscriptionsCommand {
        application_code: app_code,
        subscriptions: req.subscriptions.into_iter().map(|s| SyncSubscriptionInput {
            code: s.code,
            name: s.name,
            description: s.description,
            target: s.target,
            connection_id: s.connection_id,
            event_types: s.event_types.into_iter().map(|et| EventTypeBindingInput {
                event_type_code: et.event_type_code,
                filter: et.filter,
            }).collect(),
            dispatch_pool_code: s.dispatch_pool_code,
            mode: s.mode,
            max_retries: s.max_retries,
            timeout_seconds: s.timeout_seconds,
            data_only: s.data_only,
        }).collect(),
        remove_unlisted: query.remove_unlisted,
    };

    let ctx = ExecutionContext::create(auth.0.principal_id.clone());

    match state.sync_subscriptions_use_case.run(command, ctx).await {
        UseCaseResult::Success(event) => {
            Ok(Json(SyncResultResponse {
                application_code: event.application_code,
                created: event.created,
                updated: event.updated,
                deleted: event.deleted,
                synced_codes: event.synced_codes,
            }))
        }
        UseCaseResult::Failure(err) => Err(err.into()),
    }
}

/// Sync dispatch pools for an application
#[utoipa::path(
    post,
    path = "/{app_code}/dispatch-pools/sync",
    tag = "sdk-sync",
    operation_id = "postApiApplicationsByAppCodeDispatchPoolsSync",
    params(
        ("app_code" = String, Path, description = "Application code"),
        ("remove_unlisted" = Option<bool>, Query, description = "Archive pools not in list")
    ),
    request_body = SyncDispatchPoolsRequest,
    responses(
        (status = 200, description = "Dispatch pools synced", body = SyncResultResponse),
        (status = 400, description = "Validation error")
    ),
    security(("bearer_auth" = []))
)]
async fn sync_dispatch_pools(
    State(state): State<SdkSyncState>,
    auth: Authenticated,
    Path(app_code): Path<String>,
    Query(query): Query<SyncQuery>,
    Json(req): Json<SyncDispatchPoolsRequest>,
) -> Result<Json<SyncResultResponse>, PlatformError> {
    let command = SyncDispatchPoolsCommand {
        application_code: app_code,
        pools: req.pools.into_iter().map(|p| SyncDispatchPoolInput {
            code: p.code,
            name: p.name,
            description: p.description,
            rate_limit: p.rate_limit,
            concurrency: p.concurrency,
        }).collect(),
        remove_unlisted: query.remove_unlisted,
    };

    let ctx = ExecutionContext::create(auth.0.principal_id.clone());

    match state.sync_dispatch_pools_use_case.run(command, ctx).await {
        UseCaseResult::Success(event) => {
            Ok(Json(SyncResultResponse {
                application_code: event.application_code,
                created: event.created,
                updated: event.updated,
                deleted: event.deleted,
                synced_codes: event.synced_codes,
            }))
        }
        UseCaseResult::Failure(err) => Err(err.into()),
    }
}

/// Sync principals for an application
#[utoipa::path(
    post,
    path = "/{app_code}/principals/sync",
    tag = "sdk-sync",
    operation_id = "postApiApplicationsByAppCodePrincipalsSync",
    params(
        ("app_code" = String, Path, description = "Application code"),
        ("remove_unlisted" = Option<bool>, Query, description = "Remove SDK_SYNC roles from unlisted principals")
    ),
    request_body = SyncPrincipalsRequest,
    responses(
        (status = 200, description = "Principals synced", body = SyncResultResponse),
        (status = 400, description = "Validation error"),
        (status = 404, description = "Application not found")
    ),
    security(("bearer_auth" = []))
)]
async fn sync_principals(
    State(state): State<SdkSyncState>,
    auth: Authenticated,
    Path(app_code): Path<String>,
    Query(query): Query<SyncQuery>,
    Json(req): Json<SyncPrincipalsRequest>,
) -> Result<Json<SyncResultResponse>, PlatformError> {
    let command = SyncPrincipalsCommand {
        application_code: app_code,
        principals: req.principals.into_iter().map(|p| SyncPrincipalInput {
            email: p.email,
            name: p.name,
            roles: p.roles,
            active: p.active,
        }).collect(),
        remove_unlisted: query.remove_unlisted,
    };

    let ctx = ExecutionContext::create(auth.0.principal_id.clone());

    match state.sync_principals_use_case.run(command, ctx).await {
        UseCaseResult::Success(event) => {
            Ok(Json(SyncResultResponse {
                application_code: event.application_code,
                created: event.created,
                updated: event.updated,
                deleted: event.deactivated,
                synced_codes: event.synced_emails,
            }))
        }
        UseCaseResult::Failure(err) => Err(err.into()),
    }
}

/// Sync scheduled jobs for an application.
///
/// Body specifies the target client (or null for platform-scoped). Caller
/// must have access to that client (or be anchor for platform-scoped).
#[utoipa::path(
    post,
    path = "/{app_code}/scheduled-jobs/sync",
    tag = "sdk-sync",
    operation_id = "postApiApplicationsByAppCodeScheduledJobsSync",
    params(("app_code" = String, Path, description = "Application code")),
    request_body = SyncScheduledJobsRequest,
    responses(
        (status = 200, description = "Scheduled jobs synced", body = SyncScheduledJobsResultResponse),
        (status = 400, description = "Validation error"),
        (status = 403, description = "Forbidden")
    ),
    security(("bearer_auth" = []))
)]
async fn sync_scheduled_jobs(
    State(state): State<SdkSyncState>,
    auth: Authenticated,
    Path(app_code): Path<String>,
    Json(req): Json<SyncScheduledJobsRequest>,
) -> Result<Json<SyncScheduledJobsResultResponse>, PlatformError> {
    crate::shared::authorization_service::checks::can_sync_scheduled_jobs_app(&auth.0)?;

    // Resource-level scope check: caller must have access to the target client
    // (or be anchor when targeting platform-scoped jobs).
    match req.client_id.as_deref() {
        Some(cid) => {
            if !auth.0.can_access_client(cid) {
                return Err(PlatformError::forbidden(format!(
                    "No access to client: {}", cid
                )));
            }
        }
        None => {
            if !auth.0.is_anchor()
                && !auth.0.has_permission(crate::permissions::ADMIN_ALL)
            {
                return Err(PlatformError::forbidden(
                    "Only anchor users can sync platform-scoped scheduled jobs",
                ));
            }
        }
    }

    let command = SyncScheduledJobsCommand {
        scope: app_code.clone(),
        client_id: req.client_id,
        jobs: req.jobs.into_iter().map(|j| ScheduledJobSyncEntry {
            code: j.code,
            name: j.name,
            description: j.description,
            crons: j.crons,
            timezone: j.timezone,
            payload: j.payload,
            concurrent: j.concurrent,
            tracks_completion: j.tracks_completion,
            timeout_seconds: j.timeout_seconds,
            delivery_max_attempts: j.delivery_max_attempts,
            target_url: j.target_url,
        }).collect(),
        archive_unlisted: req.archive_unlisted,
    };

    let ctx = ExecutionContext::create(auth.0.principal_id.clone());

    match state.sync_scheduled_jobs_use_case.run(command, ctx).await {
        UseCaseResult::Success(event) => Ok(Json(SyncScheduledJobsResultResponse {
            application_code: app_code,
            created: event.created,
            updated: event.updated,
            archived: event.archived,
        })),
        UseCaseResult::Failure(err) => Err(err.into()),
    }
}

// ---------------------------------------------------------------------------
// Router
// ---------------------------------------------------------------------------

/// Create SDK sync router
///
/// Mounts application-scoped sync routes:
/// - POST /{app_code}/roles/sync
/// - POST /{app_code}/event-types/sync
/// - POST /{app_code}/subscriptions/sync
/// - POST /{app_code}/dispatch-pools/sync
/// - POST /{app_code}/principals/sync
/// - POST /{app_code}/scheduled-jobs/sync
pub fn sdk_sync_router(state: SdkSyncState) -> Router {
    Router::new()
        .route("/{app_code}/roles/sync", post(sync_roles))
        .route("/{app_code}/event-types/sync", post(sync_event_types))
        .route("/{app_code}/subscriptions/sync", post(sync_subscriptions))
        .route("/{app_code}/dispatch-pools/sync", post(sync_dispatch_pools))
        .route("/{app_code}/principals/sync", post(sync_principals))
        .route("/{app_code}/scheduled-jobs/sync", post(sync_scheduled_jobs))
        .with_state(state)
}
