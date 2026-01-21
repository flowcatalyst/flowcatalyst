//! Stream Watcher
//!
//! Watches MongoDB change streams and dispatches batches for concurrent processing.
//! Ensures aggregate ID ordering by not allowing the same aggregate in concurrent batches.

use crate::{StreamWatcher, StreamConfig};
use crate::checkpoint::CheckpointStore;
use crate::checkpoint_tracker::{CheckpointTracker, AggregateTracker, PendingDocument};
use async_trait::async_trait;
use mongodb::{Client, Collection};
use mongodb::bson::{doc, Document};
use mongodb::options::ChangeStreamOptions;
use mongodb::change_stream::event::ResumeToken;
use futures::stream::StreamExt;
use anyhow::Result;
use tracing::{info, warn, error, debug};
use std::sync::Arc;
use std::collections::HashSet;
use tokio::sync::Semaphore;
use tokio::time::{timeout, Duration};

/// Reconnection settings
const INITIAL_BACKOFF_MS: u64 = 5000;    // 5 seconds
const MAX_BACKOFF_MS: u64 = 60000;       // 60 seconds
const BACKOFF_MULTIPLIER: f64 = 2.0;

/// Batch processor callback
#[async_trait]
pub trait BatchProcessor: Send + Sync {
    /// Process a batch of documents
    async fn process(&self, documents: Vec<Document>) -> Result<()>;
}

/// Default batch processor that just logs
pub struct LoggingBatchProcessor {
    stream_name: String,
}

impl LoggingBatchProcessor {
    pub fn new(stream_name: &str) -> Self {
        Self {
            stream_name: stream_name.to_string(),
        }
    }
}

#[async_trait]
impl BatchProcessor for LoggingBatchProcessor {
    async fn process(&self, documents: Vec<Document>) -> Result<()> {
        info!("[{}] Processing batch of {} documents", self.stream_name, documents.len());
        Ok(())
    }
}

pub struct MongoStreamWatcher {
    client: Client,
    config: StreamConfig,
    checkpoint_store: Arc<dyn CheckpointStore>,
    checkpoint_tracker: Arc<CheckpointTracker>,
    aggregate_tracker: Arc<AggregateTracker>,
    batch_processor: Arc<dyn BatchProcessor>,
    concurrency_semaphore: Arc<Semaphore>,
}

impl MongoStreamWatcher {
    pub fn new(
        client: Client,
        config: StreamConfig,
        checkpoint_store: Arc<dyn CheckpointStore>,
        batch_processor: Arc<dyn BatchProcessor>,
    ) -> Self {
        let checkpoint_key = format!("checkpoint:{}", config.name);
        let checkpoint_tracker = Arc::new(CheckpointTracker::new(
            checkpoint_store.clone(),
            config.name.clone(),
            checkpoint_key,
        ));
        let aggregate_tracker = Arc::new(AggregateTracker::new(config.name.clone()));
        let concurrency_semaphore = Arc::new(Semaphore::new(config.concurrency as usize));

        Self {
            client,
            config,
            checkpoint_store,
            checkpoint_tracker,
            aggregate_tracker,
            batch_processor,
            concurrency_semaphore,
        }
    }

    /// Create with custom trackers (for testing or advanced usage)
    pub fn with_trackers(
        client: Client,
        config: StreamConfig,
        checkpoint_store: Arc<dyn CheckpointStore>,
        checkpoint_tracker: Arc<CheckpointTracker>,
        aggregate_tracker: Arc<AggregateTracker>,
        batch_processor: Arc<dyn BatchProcessor>,
    ) -> Self {
        let concurrency_semaphore = Arc::new(Semaphore::new(config.concurrency as usize));

        Self {
            client,
            config,
            checkpoint_store,
            checkpoint_tracker,
            aggregate_tracker,
            batch_processor,
            concurrency_semaphore,
        }
    }

    /// Check if the watcher should shutdown due to fatal error
    pub async fn has_fatal_error(&self) -> bool {
        self.checkpoint_tracker.has_fatal_error().await
    }

    /// Get the fatal error if one occurred
    pub async fn get_fatal_error(&self) -> Option<String> {
        self.checkpoint_tracker.get_fatal_error().await
    }

    /// Dispatch a batch for processing
    async fn dispatch_batch(
        &self,
        documents: Vec<Document>,
        aggregate_ids: HashSet<String>,
        resume_token: Option<ResumeToken>,
    ) -> Result<()> {
        if documents.is_empty() {
            return Ok(());
        }

        // Check for fatal error before dispatching
        if self.checkpoint_tracker.has_fatal_error().await {
            warn!("[{}] Skipping batch dispatch - fatal error occurred", self.config.name);
            return Ok(());
        }

        // Get batch sequence
        let seq = self.checkpoint_tracker.next_sequence().await;

        // Register aggregate IDs for this batch
        self.aggregate_tracker.register_batch(seq, aggregate_ids.clone()).await;

        // Acquire concurrency permit - blocks if at max concurrency
        let permit = self.concurrency_semaphore.clone().acquire_owned().await
            .map_err(|e| anyhow::anyhow!("Semaphore closed: {}", e))?;

        // Clone what we need for the spawned task
        let checkpoint_tracker = self.checkpoint_tracker.clone();
        let aggregate_tracker = self.aggregate_tracker.clone();
        let batch_processor = self.batch_processor.clone();
        let stream_name = self.config.name.clone();

        // Spawn task to process the batch
        tokio::spawn(async move {
            let _permit = permit; // Holds permit until task completes

            let result = batch_processor.process(documents).await;

            match result {
                Ok(()) => {
                    checkpoint_tracker.mark_complete(seq, resume_token).await;
                    debug!("[{}] Batch {} completed", stream_name, seq);
                }
                Err(e) => {
                    error!("[{}] Batch {} failed: {}", stream_name, seq, e);
                    checkpoint_tracker.mark_failed(seq, e.to_string()).await;
                }
            }

            // Release aggregate IDs and check for pending documents
            let released = aggregate_tracker.complete_batch(seq).await;
            if !released.is_empty() {
                debug!("[{}] Released {} pending documents after batch {}", stream_name, released.len(), seq);
            }
        });

        Ok(())
    }

    /// Extract aggregate ID from a document
    fn get_aggregate_id(&self, doc: &Document) -> Option<String> {
        // Try to get the aggregate ID field
        let field = &self.config.aggregate_id_field;

        if let Ok(id) = doc.get_str(field) {
            return Some(id.to_string());
        }

        // Try as ObjectId
        if let Ok(id) = doc.get_object_id(field) {
            return Some(id.to_hex());
        }

        // Try nested path (e.g., "_id")
        if field == "_id" {
            if let Some(bson) = doc.get("_id") {
                return Some(format!("{}", bson));
            }
        }

        None
    }
}

#[async_trait]
impl StreamWatcher for MongoStreamWatcher {
    async fn watch(&self) -> Result<()> {
        let db = self.client.database(&self.config.source_database);
        let collection: Collection<Document> = db.collection(&self.config.source_collection);
        let checkpoint_key = format!("checkpoint:{}", self.config.name);

        let mut consecutive_failures = 0u32;
        let mut backoff_ms = INITIAL_BACKOFF_MS;

        // Outer reconnection loop
        loop {
            // Check for fatal error
            if self.checkpoint_tracker.has_fatal_error().await {
                error!("[{}] Fatal error detected - stopping watcher", self.config.name);
                return Err(anyhow::anyhow!("Fatal error in batch processing"));
            }

            // Load checkpoint
            let resume_token_doc = match self.checkpoint_store.get_checkpoint(&checkpoint_key).await {
                Ok(doc) => doc,
                Err(e) => {
                    warn!("[{}] Failed to load checkpoint, starting from current: {}", self.config.name, e);
                    None
                }
            };

            let mut options = ChangeStreamOptions::builder()
                .full_document(Some(mongodb::options::FullDocumentType::UpdateLookup))
                .build();

            if let Some(doc) = resume_token_doc {
                info!("[{}] Resuming from checkpoint", self.config.name);
                if let Ok(token) = mongodb::bson::from_document::<ResumeToken>(doc) {
                    options.resume_after = Some(token);
                }
            } else {
                info!("[{}] Starting from current position (no checkpoint)", self.config.name);
            }

            let pipeline = vec![
                doc! { "$match": { "operationType": { "$in": &self.config.watch_operations } } }
            ];

            // Try to open the change stream
            let stream_result = collection.watch().pipeline(pipeline).with_options(options).await;
            let mut stream = match stream_result {
                Ok(s) => {
                    consecutive_failures = 0;
                    backoff_ms = INITIAL_BACKOFF_MS;
                    info!("[{}] Change stream opened on {}.{} (concurrency: {})",
                        self.config.name, self.config.source_database, self.config.source_collection,
                        self.config.concurrency);
                    s
                }
                Err(e) => {
                    consecutive_failures += 1;

                    if is_stale_resume_token_error(&e) {
                        error!("[{}] Resume token expired - clearing checkpoint. EVENTS MAY BE MISSED.", self.config.name);
                        let _ = self.checkpoint_store.clear_checkpoint(&checkpoint_key).await;
                        backoff_ms = INITIAL_BACKOFF_MS;
                        continue;
                    }

                    error!("[{}] Failed to open change stream (attempt {}), retrying in {}ms: {}",
                        self.config.name, consecutive_failures, backoff_ms, e);

                    tokio::time::sleep(Duration::from_millis(backoff_ms)).await;
                    backoff_ms = (backoff_ms as f64 * BACKOFF_MULTIPLIER) as u64;
                    if backoff_ms > MAX_BACKOFF_MS {
                        backoff_ms = MAX_BACKOFF_MS;
                    }
                    continue;
                }
            };

            // Process stream events
            let stream_error = self.process_stream_events(&mut stream).await;

            match stream_error {
                Ok(()) => {
                    info!("[{}] Change stream ended cleanly", self.config.name);
                    return Ok(());
                }
                Err(e) => {
                    // Check if this is a fatal error from batch processing
                    if self.checkpoint_tracker.has_fatal_error().await {
                        error!("[{}] Fatal error in batch processing - stopping", self.config.name);
                        return Err(e);
                    }

                    consecutive_failures += 1;

                    if is_stale_resume_token_error(&e) {
                        error!("[{}] Resume token expired - clearing checkpoint. EVENTS MAY BE MISSED.", self.config.name);
                        let _ = self.checkpoint_store.clear_checkpoint(&checkpoint_key).await;
                        backoff_ms = INITIAL_BACKOFF_MS;
                        continue;
                    }

                    warn!("[{}] Change stream error (attempt {}), reconnecting in {}ms: {}",
                        self.config.name, consecutive_failures, backoff_ms, e);

                    tokio::time::sleep(Duration::from_millis(backoff_ms)).await;
                    backoff_ms = (backoff_ms as f64 * BACKOFF_MULTIPLIER) as u64;
                    if backoff_ms > MAX_BACKOFF_MS {
                        backoff_ms = MAX_BACKOFF_MS;
                    }
                }
            }
        }
    }
}

impl MongoStreamWatcher {
    /// Process events from an active change stream
    async fn process_stream_events(
        &self,
        stream: &mut mongodb::change_stream::ChangeStream<mongodb::change_stream::event::ChangeStreamEvent<Document>>,
    ) -> Result<()> {
        let mut batch: Vec<Document> = Vec::new();
        let mut batch_aggregate_ids: HashSet<String> = HashSet::new();
        let mut last_token: Option<ResumeToken> = None;
        let batch_timeout = Duration::from_millis(self.config.batch_max_wait_ms);

        loop {
            // Check for fatal error
            if self.checkpoint_tracker.has_fatal_error().await {
                error!("[{}] Fatal error detected - stopping stream processing", self.config.name);
                // Flush current batch before returning
                if !batch.is_empty() {
                    let _ = self.dispatch_batch(
                        std::mem::take(&mut batch),
                        std::mem::take(&mut batch_aggregate_ids),
                        last_token.clone(),
                    ).await;
                }
                return Err(anyhow::anyhow!("Fatal error in batch processing"));
            }

            let event_result = timeout(batch_timeout, stream.next()).await;

            match event_result {
                Ok(Some(Ok(event))) => {
                    if let Some(doc) = event.full_document {
                        // Get aggregate ID
                        let aggregate_id = self.get_aggregate_id(&doc);

                        if let Some(ref agg_id) = aggregate_id {
                            // Check if this aggregate is already in-flight
                            if self.aggregate_tracker.is_in_flight(agg_id).await {
                                // Add to pending queue
                                debug!("[{}] Aggregate {} is in-flight, queueing document", self.config.name, agg_id);
                                self.aggregate_tracker.add_pending(PendingDocument {
                                    aggregate_id: agg_id.clone(),
                                    document: doc,
                                    resume_token: stream.resume_token(),
                                }).await;
                                continue;
                            }

                            // Check if this aggregate is already in current batch
                            if batch_aggregate_ids.contains(agg_id) {
                                // Flush current batch first, then add this to next batch
                                if !batch.is_empty() {
                                    self.dispatch_batch(
                                        std::mem::take(&mut batch),
                                        std::mem::take(&mut batch_aggregate_ids),
                                        last_token.clone(),
                                    ).await?;
                                }
                            }

                            batch_aggregate_ids.insert(agg_id.clone());
                        }

                        batch.push(doc);
                        last_token = stream.resume_token();
                    }

                    // Check if batch is full
                    if batch.len() >= self.config.batch_max_size as usize {
                        self.dispatch_batch(
                            std::mem::take(&mut batch),
                            std::mem::take(&mut batch_aggregate_ids),
                            last_token.clone(),
                        ).await?;
                    }
                }
                Ok(Some(Err(e))) => {
                    // Stream error - flush batch and return for reconnection
                    if !batch.is_empty() {
                        let _ = self.dispatch_batch(
                            std::mem::take(&mut batch),
                            std::mem::take(&mut batch_aggregate_ids),
                            last_token.clone(),
                        ).await;
                    }
                    return Err(e.into());
                }
                Ok(None) => {
                    // Stream closed
                    if !batch.is_empty() {
                        let _ = self.dispatch_batch(
                            std::mem::take(&mut batch),
                            std::mem::take(&mut batch_aggregate_ids),
                            last_token.clone(),
                        ).await;
                    }
                    return Err(anyhow::anyhow!("Change stream closed unexpectedly"));
                }
                Err(_) => {
                    // Timeout - flush batch if any
                    if !batch.is_empty() {
                        self.dispatch_batch(
                            std::mem::take(&mut batch),
                            std::mem::take(&mut batch_aggregate_ids),
                            last_token.clone(),
                        ).await?;
                    }
                }
            }
        }
    }
}

/// Check if an error is due to a stale/expired resume token
fn is_stale_resume_token_error<E: std::fmt::Display>(e: &E) -> bool {
    let err_str = e.to_string().to_lowercase();
    err_str.contains("changestream") && err_str.contains("history") ||
    err_str.contains("resume token") ||
    err_str.contains("oplog") ||
    err_str.contains("invalidate")
}

#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn test_stale_resume_token_detection() {
        assert!(is_stale_resume_token_error(&"ChangeStream history lost"));
        assert!(is_stale_resume_token_error(&"resume token expired"));
        assert!(is_stale_resume_token_error(&"oplog entry not found"));
        assert!(!is_stale_resume_token_error(&"connection refused"));
    }
}
