//! SDK Batch APIs — batch event ingest.
//!
//! The handler durably stores events; fan-out (subscription matching →
//! dispatch jobs → queue) runs out-of-band in the stream processor's
//! `EventFanOutService` (fc-stream). The request returns as soon as the
//! events are committed; the fan-out service picks them up off the
//! partial `idx_msg_events_unfanned` index.

use axum::{
    routing::post,
    extract::{State, DefaultBodyLimit},
    Json, Router,
};
use utoipa::ToSchema;
use serde::{Deserialize, Serialize};
use std::sync::Arc;

use crate::event::entity::Event;
use crate::event::repository::EventRepository;
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
}

async fn batch_events(
    State(state): State<SdkEventsState>,
    _auth: Authenticated,
    Json(req): Json<BatchEventsRequest>,
) -> Result<Json<BatchResponse>, PlatformError> {
    if req.items.len() > 500 {
        return Err(PlatformError::validation("Maximum 500 items per batch"));
    }

    let mut inserted_events = Vec::with_capacity(req.items.len());

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

        inserted_events.push(event);
    }

    state.event_repo.insert_many(&inserted_events).await?;

    let results: Vec<BatchResultItem> = inserted_events
        .iter()
        .map(|e| BatchResultItem { id: e.id.clone(), status: "SUCCESS".to_string() })
        .collect();

    Ok(Json(BatchResponse { results }))
}

pub fn sdk_events_batch_router(state: SdkEventsState) -> Router {
    Router::new()
        .route("/batch", post(batch_events))
        .layer(DefaultBodyLimit::max(32 * 1024 * 1024))
        .with_state(state)
}
