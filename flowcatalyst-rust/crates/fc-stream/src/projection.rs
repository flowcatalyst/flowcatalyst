//! Read Projections
//!
//! Creates denormalized read projections for events and dispatch jobs.

use std::sync::Arc;
use serde::{Deserialize, Serialize};
use chrono::{DateTime, Utc};
use bson::serde_helpers::chrono_datetime_as_bson_datetime;
use async_trait::async_trait;
use tracing::debug;

/// Event read projection
#[derive(Debug, Clone, Serialize, Deserialize)]
#[serde(rename_all = "camelCase")]
pub struct EventReadProjection {
    #[serde(rename = "_id")]
    pub id: String,
    pub event_type_code: String,
    pub event_type_name: String,
    pub application: String,
    pub subdomain: String,
    pub subject: String,
    pub action: String,
    pub client_id: Option<String>,
    pub client_name: Option<String>,
    pub source_id: Option<String>,
    pub source_type: Option<String>,
    pub correlation_id: Option<String>,
    pub data_summary: Option<String>,
    pub dispatch_job_count: u32,
    #[serde(with = "chrono_datetime_as_bson_datetime")]
    pub created_at: DateTime<Utc>,
}

/// Dispatch job read projection
#[derive(Debug, Clone, Serialize, Deserialize)]
#[serde(rename_all = "camelCase")]
pub struct DispatchJobReadProjection {
    #[serde(rename = "_id")]
    pub id: String,
    pub event_id: String,
    pub event_type_code: String,
    pub event_type_name: String,
    pub subscription_id: String,
    pub subscription_name: String,
    pub client_id: Option<String>,
    pub client_name: Option<String>,
    pub target: String,
    pub status: String,
    pub attempt_count: u32,
    pub max_retries: u32,
    pub last_error: Option<String>,
    #[serde(skip_serializing_if = "Option::is_none", default, with = "bson::serde_helpers::chrono_datetime_as_bson_datetime_optional")]
    pub last_attempt_at: Option<DateTime<Utc>>,
    #[serde(skip_serializing_if = "Option::is_none", default, with = "bson::serde_helpers::chrono_datetime_as_bson_datetime_optional")]
    pub next_retry_at: Option<DateTime<Utc>>,
    #[serde(skip_serializing_if = "Option::is_none", default, with = "bson::serde_helpers::chrono_datetime_as_bson_datetime_optional")]
    pub completed_at: Option<DateTime<Utc>>,
    pub correlation_id: Option<String>,
    pub dispatch_pool_id: Option<String>,
    pub dispatch_pool_name: Option<String>,
    #[serde(with = "chrono_datetime_as_bson_datetime")]
    pub created_at: DateTime<Utc>,
    #[serde(with = "chrono_datetime_as_bson_datetime")]
    pub updated_at: DateTime<Utc>,
}

/// Lookup service for resolving names
#[async_trait]
pub trait ProjectionLookup: Send + Sync {
    async fn get_event_type_name(&self, code: &str) -> Option<String>;
    async fn get_client_name(&self, id: &str) -> Option<String>;
    async fn get_subscription_name(&self, id: &str) -> Option<String>;
    async fn get_dispatch_pool_name(&self, id: &str) -> Option<String>;
}

/// Result of a batch write operation
#[derive(Debug, Clone, Default)]
pub struct BatchWriteResult {
    /// Number of documents inserted
    pub inserted: usize,
    /// Number of documents updated
    pub updated: usize,
    /// Number of documents that already existed (idempotent skips)
    pub skipped: usize,
    /// Number of failed operations
    pub failed: usize,
    /// Error messages for failed operations
    pub errors: Vec<String>,
}

impl BatchWriteResult {
    pub fn success(inserted: usize, updated: usize) -> Self {
        Self { inserted, updated, skipped: 0, failed: 0, errors: Vec::new() }
    }

    pub fn is_success(&self) -> bool {
        self.failed == 0
    }

    pub fn total_processed(&self) -> usize {
        self.inserted + self.updated + self.skipped
    }
}

/// Storage for projections
#[async_trait]
pub trait ProjectionStore: Send + Sync {
    async fn save_event_projection(&self, projection: &EventReadProjection) -> Result<(), String>;
    async fn save_dispatch_job_projection(&self, projection: &DispatchJobReadProjection) -> Result<(), String>;
    async fn update_dispatch_job_projection(&self, projection: &DispatchJobReadProjection) -> Result<(), String>;
    async fn increment_event_dispatch_count(&self, event_id: &str) -> Result<(), String>;

    /// Batch save event projections with idempotency
    async fn save_event_projections_batch(&self, projections: &[EventReadProjection]) -> BatchWriteResult {
        let mut result = BatchWriteResult::default();
        for projection in projections {
            match self.save_event_projection(projection).await {
                Ok(_) => result.inserted += 1,
                Err(e) => {
                    result.failed += 1;
                    result.errors.push(format!("{}: {}", projection.id, e));
                }
            }
        }
        result
    }

    /// Batch save dispatch job projections with idempotency
    async fn save_dispatch_job_projections_batch(&self, projections: &[DispatchJobReadProjection]) -> BatchWriteResult {
        let mut result = BatchWriteResult::default();
        for projection in projections {
            match self.save_dispatch_job_projection(projection).await {
                Ok(_) => result.inserted += 1,
                Err(e) => {
                    result.failed += 1;
                    result.errors.push(format!("{}: {}", projection.id, e));
                }
            }
        }
        result
    }

    /// Batch update dispatch job projections
    async fn update_dispatch_job_projections_batch(&self, projections: &[DispatchJobReadProjection]) -> BatchWriteResult {
        let mut result = BatchWriteResult::default();
        for projection in projections {
            match self.update_dispatch_job_projection(projection).await {
                Ok(_) => result.updated += 1,
                Err(e) => {
                    result.failed += 1;
                    result.errors.push(format!("{}: {}", projection.id, e));
                }
            }
        }
        result
    }
}

/// Event data for projection creation
#[derive(Debug, Clone)]
pub struct EventData {
    pub id: String,
    pub event_type_code: String,
    pub client_id: Option<String>,
    pub source_id: Option<String>,
    pub source_type: Option<String>,
    pub correlation_id: Option<String>,
    pub data: serde_json::Value,
    pub created_at: DateTime<Utc>,
}

/// Dispatch job data for projection creation
#[derive(Debug, Clone)]
pub struct DispatchJobData {
    pub id: String,
    pub event_id: String,
    pub event_type_code: String,
    pub subscription_id: String,
    pub client_id: Option<String>,
    pub target: String,
    pub status: String,
    pub attempt_count: u32,
    pub max_retries: u32,
    pub last_error: Option<String>,
    pub last_attempt_at: Option<DateTime<Utc>>,
    pub next_retry_at: Option<DateTime<Utc>>,
    pub completed_at: Option<DateTime<Utc>>,
    pub correlation_id: Option<String>,
    pub dispatch_pool_id: Option<String>,
    pub created_at: DateTime<Utc>,
    pub updated_at: DateTime<Utc>,
}

/// Projection builder
pub struct ProjectionBuilder {
    lookup: Arc<dyn ProjectionLookup>,
    store: Arc<dyn ProjectionStore>,
}

impl ProjectionBuilder {
    pub fn new(lookup: Arc<dyn ProjectionLookup>, store: Arc<dyn ProjectionStore>) -> Self {
        Self { lookup, store }
    }

    /// Create projection for a new event
    pub async fn create_event_projection(&self, event: &EventData) -> Result<EventReadProjection, String> {
        // Parse event type code (app:subdomain:subject:action)
        let parts: Vec<&str> = event.event_type_code.split(':').collect();
        let (application, subdomain, subject, action) = if parts.len() == 4 {
            (
                parts[0].to_string(),
                parts[1].to_string(),
                parts[2].to_string(),
                parts[3].to_string(),
            )
        } else {
            (
                "unknown".to_string(),
                "unknown".to_string(),
                "unknown".to_string(),
                "unknown".to_string(),
            )
        };

        let event_type_name = self
            .lookup
            .get_event_type_name(&event.event_type_code)
            .await
            .unwrap_or_else(|| event.event_type_code.clone());

        let client_name = match &event.client_id {
            Some(id) => self.lookup.get_client_name(id).await,
            None => None,
        };

        // Create data summary (first 200 chars of JSON)
        let data_summary = {
            let json = serde_json::to_string(&event.data).unwrap_or_default();
            if json.len() > 200 {
                Some(format!("{}...", &json[..200]))
            } else if json != "{}" && json != "null" {
                Some(json)
            } else {
                None
            }
        };

        let projection = EventReadProjection {
            id: event.id.clone(),
            event_type_code: event.event_type_code.clone(),
            event_type_name,
            application,
            subdomain,
            subject,
            action,
            client_id: event.client_id.clone(),
            client_name,
            source_id: event.source_id.clone(),
            source_type: event.source_type.clone(),
            correlation_id: event.correlation_id.clone(),
            data_summary,
            dispatch_job_count: 0,
            created_at: event.created_at,
        };

        self.store.save_event_projection(&projection).await?;
        debug!("Created event projection: {}", event.id);

        Ok(projection)
    }

    /// Create projection for a new dispatch job
    pub async fn create_dispatch_job_projection(
        &self,
        job: &DispatchJobData,
    ) -> Result<DispatchJobReadProjection, String> {
        let event_type_name = self
            .lookup
            .get_event_type_name(&job.event_type_code)
            .await
            .unwrap_or_else(|| job.event_type_code.clone());

        let subscription_name = self
            .lookup
            .get_subscription_name(&job.subscription_id)
            .await
            .unwrap_or_else(|| job.subscription_id.clone());

        let client_name = match &job.client_id {
            Some(id) => self.lookup.get_client_name(id).await,
            None => None,
        };

        let dispatch_pool_name = match &job.dispatch_pool_id {
            Some(id) => self.lookup.get_dispatch_pool_name(id).await,
            None => None,
        };

        let projection = DispatchJobReadProjection {
            id: job.id.clone(),
            event_id: job.event_id.clone(),
            event_type_code: job.event_type_code.clone(),
            event_type_name,
            subscription_id: job.subscription_id.clone(),
            subscription_name,
            client_id: job.client_id.clone(),
            client_name,
            target: job.target.clone(),
            status: job.status.clone(),
            attempt_count: job.attempt_count,
            max_retries: job.max_retries,
            last_error: job.last_error.clone(),
            last_attempt_at: job.last_attempt_at,
            next_retry_at: job.next_retry_at,
            completed_at: job.completed_at,
            correlation_id: job.correlation_id.clone(),
            dispatch_pool_id: job.dispatch_pool_id.clone(),
            dispatch_pool_name,
            created_at: job.created_at,
            updated_at: job.updated_at,
        };

        self.store.save_dispatch_job_projection(&projection).await?;

        // Increment event's dispatch job count
        self.store.increment_event_dispatch_count(&job.event_id).await?;

        debug!("Created dispatch job projection: {}", job.id);

        Ok(projection)
    }

    /// Update projection for an existing dispatch job
    pub async fn update_dispatch_job_projection(
        &self,
        job: &DispatchJobData,
    ) -> Result<DispatchJobReadProjection, String> {
        let event_type_name = self
            .lookup
            .get_event_type_name(&job.event_type_code)
            .await
            .unwrap_or_else(|| job.event_type_code.clone());

        let subscription_name = self
            .lookup
            .get_subscription_name(&job.subscription_id)
            .await
            .unwrap_or_else(|| job.subscription_id.clone());

        let client_name = match &job.client_id {
            Some(id) => self.lookup.get_client_name(id).await,
            None => None,
        };

        let dispatch_pool_name = match &job.dispatch_pool_id {
            Some(id) => self.lookup.get_dispatch_pool_name(id).await,
            None => None,
        };

        let projection = DispatchJobReadProjection {
            id: job.id.clone(),
            event_id: job.event_id.clone(),
            event_type_code: job.event_type_code.clone(),
            event_type_name,
            subscription_id: job.subscription_id.clone(),
            subscription_name,
            client_id: job.client_id.clone(),
            client_name,
            target: job.target.clone(),
            status: job.status.clone(),
            attempt_count: job.attempt_count,
            max_retries: job.max_retries,
            last_error: job.last_error.clone(),
            last_attempt_at: job.last_attempt_at,
            next_retry_at: job.next_retry_at,
            completed_at: job.completed_at,
            correlation_id: job.correlation_id.clone(),
            dispatch_pool_id: job.dispatch_pool_id.clone(),
            dispatch_pool_name,
            created_at: job.created_at,
            updated_at: job.updated_at,
        };

        self.store.update_dispatch_job_projection(&projection).await?;
        debug!("Updated dispatch job projection: {}", job.id);

        Ok(projection)
    }
}

/// MongoDB implementation of projection store
pub struct MongoProjectionStore {
    events_read: mongodb::Collection<mongodb::bson::Document>,
    dispatch_jobs_read: mongodb::Collection<mongodb::bson::Document>,
}

impl MongoProjectionStore {
    pub fn new(db: &mongodb::Database) -> Self {
        Self {
            events_read: db.collection("events_read"),
            dispatch_jobs_read: db.collection("dispatch_jobs_read"),
        }
    }

    /// Ensure all required indexes exist on the projection collections.
    /// Should be called on startup.
    pub async fn ensure_indexes(&self) -> Result<(), String> {
        use mongodb::bson::doc;
        use mongodb::options::IndexOptions;
        use mongodb::IndexModel;
        use tracing::info;

        // Events read collection indexes
        let event_indexes = vec![
            // Query by client and time (UI list view)
            IndexModel::builder()
                .keys(doc! { "clientId": 1, "createdAt": -1 })
                .options(IndexOptions::builder().name("idx_client_time".to_string()).build())
                .build(),
            // Query by event type
            IndexModel::builder()
                .keys(doc! { "eventTypeCode": 1, "createdAt": -1 })
                .options(IndexOptions::builder().name("idx_event_type".to_string()).build())
                .build(),
            // Query by correlation ID (tracing)
            IndexModel::builder()
                .keys(doc! { "correlationId": 1 })
                .options(IndexOptions::builder().name("idx_correlation".to_string()).sparse(true).build())
                .build(),
            // Query by source (entity-specific views)
            IndexModel::builder()
                .keys(doc! { "sourceType": 1, "sourceId": 1, "createdAt": -1 })
                .options(IndexOptions::builder().name("idx_source".to_string()).sparse(true).build())
                .build(),
            // Full-text search on data summary
            IndexModel::builder()
                .keys(doc! { "dataSummary": "text" })
                .options(IndexOptions::builder().name("idx_data_search".to_string()).build())
                .build(),
        ];

        self.events_read
            .create_indexes(event_indexes)
            .await
            .map_err(|e| format!("Failed to create event indexes: {}", e))?;

        info!("Created indexes on events_read collection");

        // Dispatch jobs read collection indexes
        let job_indexes = vec![
            // Query by client and time (UI list view)
            IndexModel::builder()
                .keys(doc! { "clientId": 1, "createdAt": -1 })
                .options(IndexOptions::builder().name("idx_client_time".to_string()).build())
                .build(),
            // Query by status (filter by pending, failed, etc.)
            IndexModel::builder()
                .keys(doc! { "status": 1, "createdAt": -1 })
                .options(IndexOptions::builder().name("idx_status".to_string()).build())
                .build(),
            // Query by event ID (show dispatch jobs for an event)
            IndexModel::builder()
                .keys(doc! { "eventId": 1 })
                .options(IndexOptions::builder().name("idx_event".to_string()).build())
                .build(),
            // Query by subscription (analytics)
            IndexModel::builder()
                .keys(doc! { "subscriptionId": 1, "createdAt": -1 })
                .options(IndexOptions::builder().name("idx_subscription".to_string()).build())
                .build(),
            // Query by correlation ID (tracing)
            IndexModel::builder()
                .keys(doc! { "correlationId": 1 })
                .options(IndexOptions::builder().name("idx_correlation".to_string()).sparse(true).build())
                .build(),
            // Query pending retries (scheduler)
            IndexModel::builder()
                .keys(doc! { "status": 1, "nextRetryAt": 1 })
                .options(IndexOptions::builder().name("idx_pending_retry".to_string()).build())
                .build(),
            // Query by dispatch pool (analytics)
            IndexModel::builder()
                .keys(doc! { "dispatchPoolId": 1, "createdAt": -1 })
                .options(IndexOptions::builder().name("idx_pool".to_string()).sparse(true).build())
                .build(),
        ];

        self.dispatch_jobs_read
            .create_indexes(job_indexes)
            .await
            .map_err(|e| format!("Failed to create dispatch job indexes: {}", e))?;

        info!("Created indexes on dispatch_jobs_read collection");

        Ok(())
    }
}

/// Check if a MongoDB error is a duplicate key error (code 11000)
fn is_duplicate_key_error(error: &mongodb::error::Error) -> bool {
    if let mongodb::error::ErrorKind::Write(mongodb::error::WriteFailure::WriteError(write_error)) =
        error.kind.as_ref()
    {
        return write_error.code == 11000;
    }
    false
}

#[async_trait]
impl ProjectionStore for MongoProjectionStore {
    async fn save_event_projection(&self, projection: &EventReadProjection) -> Result<(), String> {
        use mongodb::bson::doc;
        use mongodb::options::UpdateOptions;

        let doc = mongodb::bson::to_document(projection)
            .map_err(|e| format!("Serialization error: {}", e))?;

        // Use upsert with $setOnInsert for idempotency - only insert if not exists
        let result = self.events_read
            .update_one(
                doc! { "_id": &projection.id },
                doc! { "$setOnInsert": doc },
            )
            .with_options(UpdateOptions::builder().upsert(true).build())
            .await;

        match result {
            Ok(_) => Ok(()),
            Err(e) if is_duplicate_key_error(&e) => {
                // Already exists - idempotent success
                tracing::debug!("Event projection {} already exists (idempotent)", projection.id);
                Ok(())
            }
            Err(e) => Err(format!("MongoDB upsert error: {}", e)),
        }
    }

    async fn save_dispatch_job_projection(
        &self,
        projection: &DispatchJobReadProjection,
    ) -> Result<(), String> {
        use mongodb::bson::doc;
        use mongodb::options::UpdateOptions;

        let doc = mongodb::bson::to_document(projection)
            .map_err(|e| format!("Serialization error: {}", e))?;

        // Use upsert with $setOnInsert for idempotency - only insert if not exists
        let result = self.dispatch_jobs_read
            .update_one(
                doc! { "_id": &projection.id },
                doc! { "$setOnInsert": doc },
            )
            .with_options(UpdateOptions::builder().upsert(true).build())
            .await;

        match result {
            Ok(_) => Ok(()),
            Err(e) if is_duplicate_key_error(&e) => {
                // Already exists - idempotent success
                tracing::debug!("Dispatch job projection {} already exists (idempotent)", projection.id);
                Ok(())
            }
            Err(e) => Err(format!("MongoDB upsert error: {}", e)),
        }
    }

    async fn update_dispatch_job_projection(
        &self,
        projection: &DispatchJobReadProjection,
    ) -> Result<(), String> {
        use mongodb::bson::doc;
        use mongodb::options::ReplaceOptions;

        let doc = mongodb::bson::to_document(projection)
            .map_err(|e| format!("Serialization error: {}", e))?;

        // Use upsert for idempotency - create if not exists, replace if exists
        self.dispatch_jobs_read
            .replace_one(doc! { "_id": &projection.id }, doc)
            .with_options(ReplaceOptions::builder().upsert(true).build())
            .await
            .map_err(|e| format!("MongoDB replace error: {}", e))?;

        Ok(())
    }

    async fn increment_event_dispatch_count(&self, event_id: &str) -> Result<(), String> {
        use mongodb::bson::doc;

        self.events_read
            .update_one(
                doc! { "_id": event_id },
                doc! { "$inc": { "dispatchJobCount": 1 } },
            )
            .await
            .map_err(|e| format!("MongoDB update error: {}", e))?;

        Ok(())
    }

    /// Optimized batch save for event projections using MongoDB bulk write
    async fn save_event_projections_batch(&self, projections: &[EventReadProjection]) -> BatchWriteResult {
        if projections.is_empty() {
            return BatchWriteResult::default();
        }

        use mongodb::bson::doc;
        use mongodb::options::UpdateOptions;

        let mut result = BatchWriteResult::default();

        // MongoDB Rust driver doesn't have a true bulk upsert, so we use parallel upserts
        // with join_all for efficiency
        let futures: Vec<_> = projections.iter().map(|projection| {
            let collection = self.events_read.clone();
            let projection = projection.clone();
            async move {
                let doc = match mongodb::bson::to_document(&projection) {
                    Ok(d) => d,
                    Err(e) => return Err(format!("Serialization error for {}: {}", projection.id, e)),
                };

                let res = collection
                    .update_one(
                        doc! { "_id": &projection.id },
                        doc! { "$setOnInsert": doc },
                    )
                    .with_options(UpdateOptions::builder().upsert(true).build())
                    .await;

                match res {
                    Ok(update_result) => {
                        if update_result.upserted_id.is_some() {
                            Ok((1, 0, 0)) // inserted
                        } else {
                            Ok((0, 0, 1)) // skipped (already existed)
                        }
                    }
                    Err(e) if is_duplicate_key_error(&e) => {
                        Ok((0, 0, 1)) // skipped
                    }
                    Err(e) => Err(format!("{}: {}", projection.id, e)),
                }
            }
        }).collect();

        let results = futures::future::join_all(futures).await;

        for res in results {
            match res {
                Ok((inserted, updated, skipped)) => {
                    result.inserted += inserted;
                    result.updated += updated;
                    result.skipped += skipped;
                }
                Err(e) => {
                    result.failed += 1;
                    result.errors.push(e);
                }
            }
        }

        result
    }

    /// Optimized batch save for dispatch job projections using parallel upserts
    async fn save_dispatch_job_projections_batch(&self, projections: &[DispatchJobReadProjection]) -> BatchWriteResult {
        if projections.is_empty() {
            return BatchWriteResult::default();
        }

        use mongodb::bson::doc;
        use mongodb::options::UpdateOptions;

        let mut result = BatchWriteResult::default();

        let futures: Vec<_> = projections.iter().map(|projection| {
            let collection = self.dispatch_jobs_read.clone();
            let projection = projection.clone();
            async move {
                let doc = match mongodb::bson::to_document(&projection) {
                    Ok(d) => d,
                    Err(e) => return Err(format!("Serialization error for {}: {}", projection.id, e)),
                };

                let res = collection
                    .update_one(
                        doc! { "_id": &projection.id },
                        doc! { "$setOnInsert": doc },
                    )
                    .with_options(UpdateOptions::builder().upsert(true).build())
                    .await;

                match res {
                    Ok(update_result) => {
                        if update_result.upserted_id.is_some() {
                            Ok((1, 0, 0))
                        } else {
                            Ok((0, 0, 1))
                        }
                    }
                    Err(e) if is_duplicate_key_error(&e) => {
                        Ok((0, 0, 1))
                    }
                    Err(e) => Err(format!("{}: {}", projection.id, e)),
                }
            }
        }).collect();

        let results = futures::future::join_all(futures).await;

        for res in results {
            match res {
                Ok((inserted, updated, skipped)) => {
                    result.inserted += inserted;
                    result.updated += updated;
                    result.skipped += skipped;
                }
                Err(e) => {
                    result.failed += 1;
                    result.errors.push(e);
                }
            }
        }

        result
    }

    /// Optimized batch update for dispatch job projections using parallel replaces
    async fn update_dispatch_job_projections_batch(&self, projections: &[DispatchJobReadProjection]) -> BatchWriteResult {
        if projections.is_empty() {
            return BatchWriteResult::default();
        }

        use mongodb::bson::doc;
        use mongodb::options::ReplaceOptions;

        let mut result = BatchWriteResult::default();

        let futures: Vec<_> = projections.iter().map(|projection| {
            let collection = self.dispatch_jobs_read.clone();
            let projection = projection.clone();
            async move {
                let doc = match mongodb::bson::to_document(&projection) {
                    Ok(d) => d,
                    Err(e) => return Err(format!("Serialization error for {}: {}", projection.id, e)),
                };

                collection
                    .replace_one(doc! { "_id": &projection.id }, doc)
                    .with_options(ReplaceOptions::builder().upsert(true).build())
                    .await
                    .map(|_| ())
                    .map_err(|e| format!("{}: {}", projection.id, e))
            }
        }).collect();

        let results = futures::future::join_all(futures).await;

        for res in results {
            match res {
                Ok(_) => result.updated += 1,
                Err(e) => {
                    result.failed += 1;
                    result.errors.push(e);
                }
            }
        }

        result
    }
}

/// In-memory implementation for testing
#[derive(Clone)]
pub struct InMemoryProjectionStore {
    events: Arc<tokio::sync::RwLock<std::collections::HashMap<String, EventReadProjection>>>,
    jobs: Arc<tokio::sync::RwLock<std::collections::HashMap<String, DispatchJobReadProjection>>>,
}

impl InMemoryProjectionStore {
    pub fn new() -> Self {
        Self {
            events: Arc::new(tokio::sync::RwLock::new(std::collections::HashMap::new())),
            jobs: Arc::new(tokio::sync::RwLock::new(std::collections::HashMap::new())),
        }
    }

    pub async fn get_event(&self, id: &str) -> Option<EventReadProjection> {
        self.events.read().await.get(id).cloned()
    }

    pub async fn get_job(&self, id: &str) -> Option<DispatchJobReadProjection> {
        self.jobs.read().await.get(id).cloned()
    }
}

impl Default for InMemoryProjectionStore {
    fn default() -> Self {
        Self::new()
    }
}

#[async_trait]
impl ProjectionStore for InMemoryProjectionStore {
    async fn save_event_projection(&self, projection: &EventReadProjection) -> Result<(), String> {
        self.events
            .write()
            .await
            .insert(projection.id.clone(), projection.clone());
        Ok(())
    }

    async fn save_dispatch_job_projection(
        &self,
        projection: &DispatchJobReadProjection,
    ) -> Result<(), String> {
        self.jobs
            .write()
            .await
            .insert(projection.id.clone(), projection.clone());
        Ok(())
    }

    async fn update_dispatch_job_projection(
        &self,
        projection: &DispatchJobReadProjection,
    ) -> Result<(), String> {
        self.jobs
            .write()
            .await
            .insert(projection.id.clone(), projection.clone());
        Ok(())
    }

    async fn increment_event_dispatch_count(&self, event_id: &str) -> Result<(), String> {
        if let Some(event) = self.events.write().await.get_mut(event_id) {
            event.dispatch_job_count += 1;
        }
        Ok(())
    }
}

/// In-memory lookup for testing
pub struct InMemoryLookup {
    event_types: std::collections::HashMap<String, String>,
    clients: std::collections::HashMap<String, String>,
    subscriptions: std::collections::HashMap<String, String>,
    pools: std::collections::HashMap<String, String>,
}

impl InMemoryLookup {
    pub fn new() -> Self {
        Self {
            event_types: std::collections::HashMap::new(),
            clients: std::collections::HashMap::new(),
            subscriptions: std::collections::HashMap::new(),
            pools: std::collections::HashMap::new(),
        }
    }

    pub fn add_event_type(&mut self, code: &str, name: &str) {
        self.event_types.insert(code.to_string(), name.to_string());
    }

    pub fn add_client(&mut self, id: &str, name: &str) {
        self.clients.insert(id.to_string(), name.to_string());
    }

    pub fn add_subscription(&mut self, id: &str, name: &str) {
        self.subscriptions.insert(id.to_string(), name.to_string());
    }

    pub fn add_pool(&mut self, id: &str, name: &str) {
        self.pools.insert(id.to_string(), name.to_string());
    }
}

impl Default for InMemoryLookup {
    fn default() -> Self {
        Self::new()
    }
}

#[async_trait]
impl ProjectionLookup for InMemoryLookup {
    async fn get_event_type_name(&self, code: &str) -> Option<String> {
        self.event_types.get(code).cloned()
    }

    async fn get_client_name(&self, id: &str) -> Option<String> {
        self.clients.get(id).cloned()
    }

    async fn get_subscription_name(&self, id: &str) -> Option<String> {
        self.subscriptions.get(id).cloned()
    }

    async fn get_dispatch_pool_name(&self, id: &str) -> Option<String> {
        self.pools.get(id).cloned()
    }
}

// ============================================================================
// ProjectionMapper Framework
// ============================================================================

/// Operation type from a MongoDB change stream
#[derive(Debug, Clone, Copy, PartialEq, Eq)]
pub enum ChangeOperationType {
    Insert,
    Update,
    Replace,
    Delete,
}

impl ChangeOperationType {
    pub fn from_str(s: &str) -> Option<Self> {
        match s {
            "insert" => Some(Self::Insert),
            "update" => Some(Self::Update),
            "replace" => Some(Self::Replace),
            "delete" => Some(Self::Delete),
            _ => None,
        }
    }
}

/// Result of mapping a change event
#[derive(Debug)]
pub enum ProjectionMapResult<T> {
    /// Successfully mapped to a projection
    Mapped(T),
    /// Document should be skipped (e.g., delete operation)
    Skip,
    /// Mapping failed with an error
    Error(String),
}

impl<T> ProjectionMapResult<T> {
    pub fn is_mapped(&self) -> bool {
        matches!(self, Self::Mapped(_))
    }

    pub fn into_option(self) -> Option<T> {
        match self {
            Self::Mapped(t) => Some(t),
            _ => None,
        }
    }
}

/// Trait for mapping MongoDB documents to projection types
#[async_trait]
pub trait ProjectionMapper<T>: Send + Sync {
    /// Map a full document (from insert/replace) to a projection
    async fn map_document(&self, doc: &mongodb::bson::Document) -> ProjectionMapResult<T>;

    /// Map an update description to partial projection fields
    /// Returns None if the mapper doesn't support partial updates
    async fn map_update(
        &self,
        _id: &str,
        _update_description: &mongodb::bson::Document,
    ) -> Option<ProjectionMapResult<T>> {
        None
    }

    /// Get the collection name this mapper watches
    fn source_collection(&self) -> &str;

    /// Get the target projection collection name
    fn target_collection(&self) -> &str;
}

/// Event mapper - transforms Event documents into EventReadProjection
pub struct EventMapper {
    lookup: Arc<dyn ProjectionLookup>,
}

impl EventMapper {
    pub fn new(lookup: Arc<dyn ProjectionLookup>) -> Self {
        Self { lookup }
    }

    fn parse_event_type_code(code: &str) -> (String, String, String, String) {
        let parts: Vec<&str> = code.split(':').collect();
        if parts.len() == 4 {
            (
                parts[0].to_string(),
                parts[1].to_string(),
                parts[2].to_string(),
                parts[3].to_string(),
            )
        } else {
            (
                "unknown".to_string(),
                "unknown".to_string(),
                "unknown".to_string(),
                "unknown".to_string(),
            )
        }
    }
}

#[async_trait]
impl ProjectionMapper<EventReadProjection> for EventMapper {
    async fn map_document(&self, doc: &mongodb::bson::Document) -> ProjectionMapResult<EventReadProjection> {
        use mongodb::bson::Bson;

        // Extract required fields
        let id = match doc.get_str("_id") {
            Ok(id) => id.to_string(),
            Err(_) => return ProjectionMapResult::Error("Missing _id field".to_string()),
        };

        let event_type_code = match doc.get_str("eventTypeCode").or_else(|_| doc.get_str("event_type_code")) {
            Ok(code) => code.to_string(),
            Err(_) => return ProjectionMapResult::Error("Missing eventTypeCode field".to_string()),
        };

        let (application, subdomain, subject, action) = Self::parse_event_type_code(&event_type_code);

        let event_type_name = self
            .lookup
            .get_event_type_name(&event_type_code)
            .await
            .unwrap_or_else(|| event_type_code.clone());

        let client_id = doc.get_str("clientId").or_else(|_| doc.get_str("client_id")).ok().map(String::from);
        let client_name = match &client_id {
            Some(id) => self.lookup.get_client_name(id).await,
            None => None,
        };

        let source_id = doc.get_str("sourceId").or_else(|_| doc.get_str("source_id")).ok().map(String::from);
        let source_type = doc.get_str("sourceType").or_else(|_| doc.get_str("source_type")).ok().map(String::from);
        let correlation_id = doc.get_str("correlationId").or_else(|_| doc.get_str("correlation_id")).ok().map(String::from);

        // Create data summary
        let data_summary = doc.get("data").and_then(|d| {
            let json = serde_json::to_string(d).ok()?;
            if json.len() > 200 {
                Some(format!("{}...", &json[..200]))
            } else if json != "{}" && json != "null" {
                Some(json)
            } else {
                None
            }
        });

        let dispatch_job_count = doc.get_i32("dispatchJobCount").or_else(|_| doc.get_i32("dispatch_job_count")).unwrap_or(0) as u32;

        let created_at = match doc.get("createdAt").or_else(|| doc.get("created_at")) {
            Some(Bson::DateTime(dt)) => dt.to_chrono(),
            _ => Utc::now(),
        };

        ProjectionMapResult::Mapped(EventReadProjection {
            id,
            event_type_code,
            event_type_name,
            application,
            subdomain,
            subject,
            action,
            client_id,
            client_name,
            source_id,
            source_type,
            correlation_id,
            data_summary,
            dispatch_job_count,
            created_at,
        })
    }

    fn source_collection(&self) -> &str {
        "events"
    }

    fn target_collection(&self) -> &str {
        "events_read"
    }
}

/// Dispatch job mapper - transforms DispatchJob documents into DispatchJobReadProjection
pub struct DispatchJobMapper {
    lookup: Arc<dyn ProjectionLookup>,
}

impl DispatchJobMapper {
    pub fn new(lookup: Arc<dyn ProjectionLookup>) -> Self {
        Self { lookup }
    }
}

#[async_trait]
impl ProjectionMapper<DispatchJobReadProjection> for DispatchJobMapper {
    async fn map_document(&self, doc: &mongodb::bson::Document) -> ProjectionMapResult<DispatchJobReadProjection> {
        use mongodb::bson::Bson;

        let id = match doc.get_str("_id") {
            Ok(id) => id.to_string(),
            Err(_) => return ProjectionMapResult::Error("Missing _id field".to_string()),
        };

        let event_id = doc.get_str("eventId").or_else(|_| doc.get_str("event_id"))
            .map(String::from)
            .unwrap_or_default();

        let event_type_code = doc.get_str("eventTypeCode").or_else(|_| doc.get_str("event_type_code"))
            .map(String::from)
            .unwrap_or_default();

        let event_type_name = self
            .lookup
            .get_event_type_name(&event_type_code)
            .await
            .unwrap_or_else(|| event_type_code.clone());

        let subscription_id = doc.get_str("subscriptionId").or_else(|_| doc.get_str("subscription_id"))
            .map(String::from)
            .unwrap_or_default();

        let subscription_name = self
            .lookup
            .get_subscription_name(&subscription_id)
            .await
            .unwrap_or_else(|| subscription_id.clone());

        let client_id = doc.get_str("clientId").or_else(|_| doc.get_str("client_id")).ok().map(String::from);
        let client_name = match &client_id {
            Some(id) => self.lookup.get_client_name(id).await,
            None => None,
        };

        let target = doc.get_str("target").map(String::from).unwrap_or_default();
        let status = doc.get_str("status").map(String::from).unwrap_or_else(|_| "PENDING".to_string());

        let attempt_count = doc.get_i32("attemptCount").or_else(|_| doc.get_i32("attempt_count")).unwrap_or(0) as u32;
        let max_retries = doc.get_i32("maxRetries").or_else(|_| doc.get_i32("max_retries")).unwrap_or(3) as u32;

        let last_error = doc.get_str("lastError").or_else(|_| doc.get_str("last_error")).ok().map(String::from);
        let correlation_id = doc.get_str("correlationId").or_else(|_| doc.get_str("correlation_id")).ok().map(String::from);

        let dispatch_pool_id = doc.get_str("dispatchPoolId").or_else(|_| doc.get_str("dispatch_pool_id")).ok().map(String::from);
        let dispatch_pool_name = match &dispatch_pool_id {
            Some(id) => self.lookup.get_dispatch_pool_name(id).await,
            None => None,
        };

        let parse_datetime = |key1: &str, key2: &str| -> Option<DateTime<Utc>> {
            match doc.get(key1).or_else(|| doc.get(key2)) {
                Some(Bson::DateTime(dt)) => Some(dt.to_chrono()),
                _ => None,
            }
        };

        let last_attempt_at = parse_datetime("lastAttemptAt", "last_attempt_at");
        let next_retry_at = parse_datetime("nextRetryAt", "next_retry_at");
        let completed_at = parse_datetime("completedAt", "completed_at");

        let created_at = match doc.get("createdAt").or_else(|| doc.get("created_at")) {
            Some(Bson::DateTime(dt)) => dt.to_chrono(),
            _ => Utc::now(),
        };

        let updated_at = match doc.get("updatedAt").or_else(|| doc.get("updated_at")) {
            Some(Bson::DateTime(dt)) => dt.to_chrono(),
            _ => Utc::now(),
        };

        ProjectionMapResult::Mapped(DispatchJobReadProjection {
            id,
            event_id,
            event_type_code,
            event_type_name,
            subscription_id,
            subscription_name,
            client_id,
            client_name,
            target,
            status,
            attempt_count,
            max_retries,
            last_error,
            last_attempt_at,
            next_retry_at,
            completed_at,
            correlation_id,
            dispatch_pool_id,
            dispatch_pool_name,
            created_at,
            updated_at,
        })
    }

    fn source_collection(&self) -> &str {
        "dispatch_jobs"
    }

    fn target_collection(&self) -> &str {
        "dispatch_jobs_read"
    }
}

/// Projection processor that combines mapper and store
pub struct ProjectionProcessor<T> {
    mapper: Arc<dyn ProjectionMapper<T>>,
    store: Arc<dyn ProjectionStore>,
}

impl<T> ProjectionProcessor<T>
where
    T: Send + Sync,
{
    pub fn new(mapper: Arc<dyn ProjectionMapper<T>>, store: Arc<dyn ProjectionStore>) -> Self {
        Self { mapper, store }
    }

    pub fn source_collection(&self) -> &str {
        self.mapper.source_collection()
    }

    pub fn target_collection(&self) -> &str {
        self.mapper.target_collection()
    }
}

impl ProjectionProcessor<EventReadProjection> {
    /// Process a batch of event documents
    pub async fn process_batch(&self, docs: &[mongodb::bson::Document]) -> BatchWriteResult {
        let mut projections = Vec::with_capacity(docs.len());

        for doc in docs {
            match self.mapper.map_document(doc).await {
                ProjectionMapResult::Mapped(p) => projections.push(p),
                ProjectionMapResult::Skip => {}
                ProjectionMapResult::Error(e) => {
                    tracing::warn!("Failed to map event document: {}", e);
                }
            }
        }

        self.store.save_event_projections_batch(&projections).await
    }
}

impl ProjectionProcessor<DispatchJobReadProjection> {
    /// Process a batch of dispatch job documents
    pub async fn process_batch(&self, docs: &[mongodb::bson::Document]) -> BatchWriteResult {
        let mut projections = Vec::with_capacity(docs.len());

        for doc in docs {
            match self.mapper.map_document(doc).await {
                ProjectionMapResult::Mapped(p) => projections.push(p),
                ProjectionMapResult::Skip => {}
                ProjectionMapResult::Error(e) => {
                    tracing::warn!("Failed to map dispatch job document: {}", e);
                }
            }
        }

        self.store.save_dispatch_job_projections_batch(&projections).await
    }

    /// Process updates to dispatch jobs
    pub async fn process_updates(&self, docs: &[mongodb::bson::Document]) -> BatchWriteResult {
        let mut projections = Vec::with_capacity(docs.len());

        for doc in docs {
            match self.mapper.map_document(doc).await {
                ProjectionMapResult::Mapped(p) => projections.push(p),
                ProjectionMapResult::Skip => {}
                ProjectionMapResult::Error(e) => {
                    tracing::warn!("Failed to map dispatch job update: {}", e);
                }
            }
        }

        self.store.update_dispatch_job_projections_batch(&projections).await
    }
}

#[cfg(test)]
mod tests {
    use super::*;

    #[tokio::test]
    async fn test_create_event_projection() {
        let mut lookup = InMemoryLookup::new();
        lookup.add_event_type("orders:fulfillment:shipment:shipped", "Shipment Shipped");
        lookup.add_client("client-1", "Acme Corp");

        let store = InMemoryProjectionStore::new();
        let builder = ProjectionBuilder::new(
            Arc::new(lookup),
            Arc::new(store.clone()),
        );

        let event = EventData {
            id: "evt-1".to_string(),
            event_type_code: "orders:fulfillment:shipment:shipped".to_string(),
            client_id: Some("client-1".to_string()),
            source_id: Some("order-123".to_string()),
            source_type: Some("Order".to_string()),
            correlation_id: Some("corr-1".to_string()),
            data: serde_json::json!({"tracking": "ABC123"}),
            created_at: Utc::now(),
        };

        let projection = builder.create_event_projection(&event).await.unwrap();

        assert_eq!(projection.id, "evt-1");
        assert_eq!(projection.event_type_name, "Shipment Shipped");
        assert_eq!(projection.client_name, Some("Acme Corp".to_string()));
        assert_eq!(projection.application, "orders");
        assert_eq!(projection.subdomain, "fulfillment");
        assert_eq!(projection.subject, "shipment");
        assert_eq!(projection.action, "shipped");
    }
}
