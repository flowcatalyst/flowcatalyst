//! SDK Batch APIs — batch event and dispatch job ingest
//!
//! After inserting events, performs fan-out: matches events against active
//! subscriptions, batch-creates dispatch jobs, and batch-publishes to the
//! message queue. Mirrors the TypeScript UnitOfWork + EventDispatchService
//! pattern.

use axum::{
    routing::post,
    extract::State,
    Json, Router,
};
use utoipa::ToSchema;
use serde::{Deserialize, Serialize};
use std::sync::Arc;
use tracing::{debug, info, warn};

use fc_common::{Message, MediationType};
use fc_queue::QueuePublisher;

use crate::event::entity::Event;
use crate::event::repository::EventRepository;
use crate::dispatch_job::entity::DispatchJob;
use crate::dispatch_job::repository::DispatchJobRepository;
use crate::subscription::repository::SubscriptionRepository;
use crate::shared::error::PlatformError;
use crate::shared::middleware::Authenticated;

// ── Batch Events ─────────────────────────────────────────────────────────

#[derive(Debug, Deserialize, ToSchema)]
#[serde(rename_all = "camelCase")]
pub struct BatchEventItem {
    pub spec_version: Option<String>,
    /// Event type — accepts both `type` (camelCase API) and `event_type` (SDK outbox payload).
    #[serde(alias = "event_type")]
    pub r#type: String,
    pub source: Option<String>,
    pub subject: Option<String>,
    pub data: Option<serde_json::Value>,
    #[serde(alias = "correlation_id")]
    pub correlation_id: Option<String>,
    #[serde(alias = "causation_id")]
    pub causation_id: Option<String>,
    #[serde(alias = "deduplication_id")]
    pub deduplication_id: Option<String>,
    #[serde(alias = "message_group")]
    pub message_group: Option<String>,
    #[serde(alias = "client_id")]
    pub client_id: Option<String>,
    #[serde(alias = "context_data")]
    pub context_data: Option<serde_json::Value>,
}

#[derive(Debug, Deserialize, ToSchema)]
#[serde(rename_all = "camelCase")]
pub struct BatchEventsRequest {
    pub items: Vec<BatchEventItem>,
}

#[derive(Debug, Serialize, ToSchema)]
#[serde(rename_all = "camelCase")]
pub struct BatchResultItem {
    pub id: String,
    pub status: String,
}

#[derive(Debug, Serialize, ToSchema)]
#[serde(rename_all = "camelCase")]
pub struct BatchResponse {
    pub results: Vec<BatchResultItem>,
}

#[derive(Clone)]
pub struct SdkEventsState {
    pub event_repo: Arc<EventRepository>,
    /// Optional — when present, enables event fan-out (subscription matching → dispatch jobs → queue).
    /// None in lightweight/test contexts where fan-out isn't needed.
    pub dispatch: Option<EventDispatchDeps>,
}

/// Dependencies for event fan-out (subscription matching + dispatch job creation + queue publish).
#[derive(Clone)]
pub struct EventDispatchDeps {
    pub subscription_repo: Arc<SubscriptionRepository>,
    pub dispatch_job_repo: Arc<DispatchJobRepository>,
    pub queue_publisher: Arc<dyn QueuePublisher>,
    /// Platform callback URL for dispatch processing (e.g. "http://localhost:8080/api/dispatch/process").
    /// The router sends `{ messageId }` here; the processing endpoint loads the job and delivers the webhook.
    pub dispatch_process_url: String,
}

async fn batch_events(
    State(state): State<SdkEventsState>,
    _auth: Authenticated,
    Json(req): Json<BatchEventsRequest>,
) -> Result<Json<BatchResponse>, PlatformError> {
    if req.items.len() > 100 {
        return Err(PlatformError::validation("Maximum 100 items per batch"));
    }

    let mut results = Vec::with_capacity(req.items.len());
    let mut inserted_events = Vec::with_capacity(req.items.len());

    // Phase 1: Insert all events
    for item in req.items {
        let mut event = Event::new(
            item.r#type,
            item.source.unwrap_or_default(),
            item.data.unwrap_or(serde_json::Value::Null),
        );
        event.subject = item.subject;
        event.correlation_id = item.correlation_id;
        event.causation_id = item.causation_id;
        event.deduplication_id = item.deduplication_id;
        event.message_group = item.message_group;
        event.client_id = item.client_id;

        let id = event.id.clone();
        state.event_repo.insert(&event).await?;
        results.push(BatchResultItem { id: id.clone(), status: "SUCCESS".to_string() });

        inserted_events.push(event);
    }

    // Phase 2: Fan-out — match events against subscriptions, create dispatch jobs, publish to queue
    if let Some(ref dispatch) = state.dispatch {
        fan_out_events(&inserted_events, dispatch).await;
    }

    Ok(Json(BatchResponse { results }))
}

/// Match events against active subscriptions, batch-create dispatch jobs,
/// and batch-publish to the message queue.
///
/// Best-effort: failures are logged but don't fail the batch response
/// (events are already persisted). The scheduler will pick up any missed
/// jobs via its stale-job poller.
async fn fan_out_events(events: &[Event], deps: &EventDispatchDeps) {
    // Load all active subscriptions — matching is done per-event below
    let all_subscriptions = match deps.subscription_repo.find_active().await {
        Ok(subs) => subs,
        Err(e) => {
            warn!(error = %e, "Failed to load subscriptions for fan-out, dispatch jobs will not be created");
            return;
        }
    };

    if all_subscriptions.is_empty() {
        return;
    }

    let mut dispatch_jobs = Vec::new();
    let mut queue_messages = Vec::new();

    for event in events {
        for sub in &all_subscriptions {
            // Match event type
            if !sub.matches_event_type(&event.event_type) {
                continue;
            }

            // Match client
            if !sub.matches_client(event.client_id.as_deref()) {
                continue;
            }

            // Build dispatch job
            let payload = serde_json::to_string(&event.data).unwrap_or_default();
            let mut job = DispatchJob::for_event(
                &event.id,
                &event.event_type,
                &event.source,
                &sub.endpoint,
                &payload,
            );

            // Set status to QUEUED since we're publishing directly to the queue
            // (not going through the scheduler's PENDING → QUEUED flow)
            job.status = crate::dispatch_job::entity::DispatchStatus::Queued;

            if let Some(ref s) = event.subject {
                job.subject = Some(s.clone());
            }
            if let Some(ref c) = event.correlation_id {
                job = job.with_correlation_id(c);
            }
            if let Some(ref g) = event.message_group {
                job = job.with_message_group(g);
            }
            if let Some(ref c) = event.client_id {
                job = job.with_client_id(c);
            }

            job = job
                .with_subscription_id(&sub.id)
                .with_mode(sub.mode)
                .with_data_only(sub.data_only);

            if let Some(ref pool_id) = sub.dispatch_pool_id {
                job = job.with_dispatch_pool_id(pool_id);
            }
            if let Some(ref sa_id) = sub.service_account_id {
                job = job.with_service_account_id(sa_id);
            }

            job.max_retries = sub.max_retries as u32;
            job.timeout_seconds = sub.timeout_seconds as u32;

            // Build queue message
            let pool_code = sub.dispatch_pool_id.clone()
                .unwrap_or_else(|| "DEFAULT".to_string());
            let msg_group = job.message_group.clone()
                .unwrap_or_else(|| "default".to_string());

            queue_messages.push(Message {
                id: job.id.clone(),
                pool_code,
                auth_token: None,
                signing_secret: None,
                mediation_type: MediationType::HTTP,
                mediation_target: deps.dispatch_process_url.clone(),
                message_group_id: Some(msg_group),
                high_priority: false,
                dispatch_mode: sub.mode,
            });

            dispatch_jobs.push(job);
        }
    }

    if dispatch_jobs.is_empty() {
        return;
    }

    let job_count = dispatch_jobs.len();

    // Batch-insert dispatch jobs
    if let Err(e) = deps.dispatch_job_repo.insert_many(&dispatch_jobs).await {
        warn!(error = %e, count = job_count, "Failed to batch-insert dispatch jobs");
        return;
    }

    debug!(count = job_count, "Created dispatch jobs from event fan-out");

    // Batch-publish to queue (best-effort, post-commit style)
    match deps.queue_publisher.publish_batch(queue_messages).await {
        Ok(ids) => {
            info!(count = ids.len(), "Published dispatch jobs to queue");
        }
        Err(e) => {
            warn!(
                error = %e,
                count = job_count,
                "Failed to publish dispatch jobs to queue (scheduler will recover)"
            );
        }
    }
}

pub fn sdk_events_batch_router(state: SdkEventsState) -> Router {
    Router::new()
        .route("/batch", post(batch_events))
        .with_state(state)
}
