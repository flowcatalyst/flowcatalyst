//! Debug BFF API
//!
//! Raw/debug endpoints for admin access to transactional data.
//! These endpoints query the raw collections (events, dispatch_jobs)
//! rather than the optimized read projections.

use std::sync::Arc;
use axum::{
    extract::{Path, Query, State},
    routing::get,
    response::Json,
    Router,
};
use serde::Serialize;
use crate::{Event, DispatchJob};
use crate::{EventRepository, DispatchJobRepository};
use crate::shared::api_common::{CursorPage, CursorParams, decode_cursor, encode_cursor};
use crate::shared::error::{PlatformError, Result};

// ============================================================================
// State
// ============================================================================

#[derive(Clone)]
pub struct DebugState {
    pub event_repo: Arc<EventRepository>,
    pub dispatch_job_repo: Arc<DispatchJobRepository>,
}

// ============================================================================
// DTOs - Raw Events
// ============================================================================

#[derive(Debug, Serialize)]
#[serde(rename_all = "camelCase")]
pub struct RawEventResponse {
    pub id: String,
    pub spec_version: String,
    pub event_type: String,
    pub source: String,
    pub subject: Option<String>,
    pub time: String,
    pub data: serde_json::Value,
    pub message_group: Option<String>,
    pub correlation_id: Option<String>,
    pub causation_id: Option<String>,
    pub deduplication_id: Option<String>,
    pub context_data: Option<Vec<ContextDataResponse>>,
    pub client_id: Option<String>,
}

#[derive(Debug, Serialize)]
#[serde(rename_all = "camelCase")]
pub struct ContextDataResponse {
    pub key: String,
    pub value: String,
}

impl From<&Event> for RawEventResponse {
    fn from(event: &Event) -> Self {
        let context_data = if event.context_data.is_empty() {
            None
        } else {
            Some(event.context_data.iter().map(|cd| ContextDataResponse {
                key: cd.key.clone(),
                value: cd.value.clone(),
            }).collect())
        };

        Self {
            id: event.id.clone(),
            spec_version: event.spec_version.clone(),
            event_type: event.event_type.clone(),
            source: event.source.clone(),
            subject: event.subject.clone(),
            time: event.time.to_rfc3339(),
            data: event.data.clone(),
            message_group: event.message_group.clone(),
            correlation_id: event.correlation_id.clone(),
            causation_id: event.causation_id.clone(),
            deduplication_id: event.deduplication_id.clone(),
            context_data,
            client_id: event.client_id.clone(),
        }
    }
}


// ============================================================================
// DTOs - Raw Dispatch Jobs
// ============================================================================

#[derive(Debug, Serialize)]
#[serde(rename_all = "camelCase")]
pub struct RawDispatchJobResponse {
    pub id: String,
    pub external_id: Option<String>,
    pub source: Option<String>,
    pub kind: String,
    pub code: String,
    pub subject: Option<String>,
    pub event_id: Option<String>,
    pub correlation_id: Option<String>,
    pub target_url: String,
    pub protocol: String,
    pub client_id: Option<String>,
    pub subscription_id: Option<String>,
    pub service_account_id: Option<String>,
    pub dispatch_pool_id: Option<String>,
    pub message_group: Option<String>,
    pub mode: String,
    pub sequence: i32,
    pub status: String,
    pub attempt_count: u32,
    pub max_retries: u32,
    pub last_error: Option<String>,
    pub timeout_seconds: u32,
    pub retry_strategy: String,
    pub idempotency_key: Option<String>,
    pub created_at: String,
    pub updated_at: String,
    pub scheduled_for: Option<String>,
    pub completed_at: Option<String>,
    // Include payload info for debug (but not full payload for large payloads)
    pub payload_content_type: String,
    pub payload_length: usize,
    pub attempt_history_count: usize,
}

impl From<&DispatchJob> for RawDispatchJobResponse {
    fn from(job: &DispatchJob) -> Self {
        Self {
            id: job.id.clone(),
            external_id: job.external_id.clone(),
            source: job.source.clone(),
            kind: format!("{:?}", job.kind).to_uppercase(),
            code: job.code.clone(),
            subject: job.subject.clone(),
            event_id: job.event_id.clone(),
            correlation_id: job.correlation_id.clone(),
            target_url: job.target_url.clone(),
            protocol: format!("{:?}", job.protocol).to_uppercase(),
            client_id: job.client_id.clone(),
            subscription_id: job.subscription_id.clone(),
            service_account_id: job.service_account_id.clone(),
            dispatch_pool_id: job.dispatch_pool_id.clone(),
            message_group: job.message_group.clone(),
            mode: format!("{:?}", job.mode).to_uppercase(),
            sequence: job.sequence,
            status: format!("{:?}", job.status).to_uppercase(),
            attempt_count: job.attempt_count,
            max_retries: job.max_retries,
            last_error: job.last_error.clone(),
            timeout_seconds: job.timeout_seconds,
            retry_strategy: format!("{:?}", job.retry_strategy).to_uppercase(),
            idempotency_key: job.idempotency_key.clone(),
            created_at: job.created_at.to_rfc3339(),
            updated_at: job.updated_at.to_rfc3339(),
            scheduled_for: job.scheduled_for.map(|t| t.to_rfc3339()),
            completed_at: job.completed_at.map(|t| t.to_rfc3339()),
            payload_content_type: job.payload_content_type.clone(),
            payload_length: job.payload.as_ref().map(|p| p.len()).unwrap_or(0),
            attempt_history_count: job.attempts.len(),
        }
    }
}

// ============================================================================
// Handlers - Raw Events
// ============================================================================

/// List raw events (debug/admin only). Cursor-paginated; `msg_events` is
/// unbounded so we keyset on `(created_at, id) DESC` and never count.
async fn list_raw_events(
    State(state): State<DebugState>,
    Query(params): Query<CursorParams>,
) -> Result<Json<CursorPage<RawEventResponse>>> {
    let size = params.size() as usize;
    let cursor = match params.after.as_deref() {
        Some(c) => Some(decode_cursor(c).map_err(|_| PlatformError::validation("Invalid cursor"))?),
        None => None,
    };
    let mut events = state.event_repo
        .find_recent_with_cursor(cursor.as_ref(), params.fetch_limit())
        .await?;

    let has_more = events.len() > size;
    if has_more { events.truncate(size); }
    let next_cursor = if has_more {
        events.last().map(|e| encode_cursor(e.created_at, &e.id))
    } else {
        None
    };
    let items = events.iter().map(RawEventResponse::from).collect();
    Ok(Json(CursorPage { items, has_more, next_cursor }))
}

/// Get a single raw event by ID (debug/admin only)
async fn get_raw_event(
    State(state): State<DebugState>,
    Path(id): Path<String>,
) -> Result<Json<RawEventResponse>> {
    let event = state.event_repo.find_by_id(&id).await?
        .ok_or_else(|| PlatformError::not_found("Event", &id))?;

    Ok(Json(RawEventResponse::from(&event)))
}

// ============================================================================
// Handlers - Raw Dispatch Jobs
// ============================================================================

/// List raw dispatch jobs (debug/admin only). Cursor-paginated;
/// `msg_dispatch_jobs` is unbounded so we keyset on `(created_at, id) DESC`
/// and never count.
async fn list_raw_dispatch_jobs(
    State(state): State<DebugState>,
    Query(params): Query<CursorParams>,
) -> Result<Json<CursorPage<RawDispatchJobResponse>>> {
    let size = params.size() as usize;
    let cursor = match params.after.as_deref() {
        Some(c) => Some(decode_cursor(c).map_err(|_| PlatformError::validation("Invalid cursor"))?),
        None => None,
    };
    let mut jobs = state.dispatch_job_repo
        .find_recent_with_cursor(cursor.as_ref(), params.fetch_limit())
        .await?;

    let has_more = jobs.len() > size;
    if has_more { jobs.truncate(size); }
    let next_cursor = if has_more {
        jobs.last().map(|j| encode_cursor(j.created_at, &j.id))
    } else {
        None
    };
    let items = jobs.iter().map(RawDispatchJobResponse::from).collect();
    Ok(Json(CursorPage { items, has_more, next_cursor }))
}

/// Get a single raw dispatch job by ID (debug/admin only)
async fn get_raw_dispatch_job(
    State(state): State<DebugState>,
    Path(id): Path<String>,
) -> Result<Json<RawDispatchJobResponse>> {
    let job = state.dispatch_job_repo.find_by_id(&id).await?
        .ok_or_else(|| PlatformError::not_found("DispatchJob", &id))?;

    Ok(Json(RawDispatchJobResponse::from(&job)))
}

// ============================================================================
// Router
// ============================================================================

/// Create debug events router
pub fn debug_events_router(state: DebugState) -> Router {
    Router::new()
        .route("/", get(list_raw_events))
        .route("/{id}", get(get_raw_event))
        .with_state(state)
}

/// Create debug dispatch jobs router
pub fn debug_dispatch_jobs_router(state: DebugState) -> Router {
    Router::new()
        .route("/", get(list_raw_dispatch_jobs))
        .route("/{id}", get(get_raw_dispatch_job))
        .with_state(state)
}
