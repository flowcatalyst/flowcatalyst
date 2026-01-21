//! Batch Dispatcher
//!
//! Dispatches batches of dispatch jobs to the outbox or message queue.

use std::sync::Arc;
use tokio::sync::RwLock;
use serde::{Deserialize, Serialize};
use chrono::{DateTime, Utc};
use async_trait::async_trait;
use tracing::{debug, info, error, warn};

use crate::subscription_matcher::DispatchJobCreation;

/// Result of dispatching a batch
#[derive(Debug, Clone)]
pub struct BatchDispatchResult {
    pub total: usize,
    pub succeeded: usize,
    pub failed: usize,
    pub errors: Vec<DispatchError>,
}

impl BatchDispatchResult {
    pub fn empty() -> Self {
        Self {
            total: 0,
            succeeded: 0,
            failed: 0,
            errors: vec![],
        }
    }

    pub fn success(count: usize) -> Self {
        Self {
            total: count,
            succeeded: count,
            failed: 0,
            errors: vec![],
        }
    }
}

/// Dispatch error info
#[derive(Debug, Clone)]
pub struct DispatchError {
    pub job_id: String,
    pub error: String,
    pub retryable: bool,
}

/// Target for dispatch jobs
#[async_trait]
pub trait DispatchTarget: Send + Sync {
    /// Dispatch a single job
    async fn dispatch(&self, job: &DispatchJobCreation) -> Result<(), String>;

    /// Dispatch a batch of jobs (default: one by one)
    async fn dispatch_batch(&self, jobs: &[DispatchJobCreation]) -> BatchDispatchResult {
        let mut result = BatchDispatchResult::empty();
        result.total = jobs.len();

        for job in jobs {
            match self.dispatch(job).await {
                Ok(_) => result.succeeded += 1,
                Err(e) => {
                    result.failed += 1;
                    result.errors.push(DispatchError {
                        job_id: job.id.clone(),
                        error: e,
                        retryable: true,
                    });
                }
            }
        }

        result
    }
}

/// Configuration for the batch dispatcher
#[derive(Debug, Clone)]
pub struct BatchDispatcherConfig {
    /// Maximum batch size for dispatching
    pub max_batch_size: usize,
    /// Maximum wait time before flushing a partial batch (ms)
    pub max_batch_wait_ms: u64,
    /// Retry attempts for failed dispatches
    pub retry_attempts: u32,
    /// Delay between retries (ms)
    pub retry_delay_ms: u64,
}

impl Default for BatchDispatcherConfig {
    fn default() -> Self {
        Self {
            max_batch_size: 100,
            max_batch_wait_ms: 1000,
            retry_attempts: 3,
            retry_delay_ms: 1000,
        }
    }
}

/// Statistics for the batch dispatcher
#[derive(Debug, Clone, Default, Serialize, Deserialize)]
pub struct DispatcherStats {
    pub batches_dispatched: u64,
    pub jobs_dispatched: u64,
    pub jobs_failed: u64,
    pub retries: u64,
    pub last_dispatch_at: Option<DateTime<Utc>>,
}

/// Batch dispatcher - dispatches dispatch jobs to targets
pub struct BatchDispatcher {
    config: BatchDispatcherConfig,
    target: Arc<dyn DispatchTarget>,
    stats: Arc<RwLock<DispatcherStats>>,
}

impl BatchDispatcher {
    pub fn new(config: BatchDispatcherConfig, target: Arc<dyn DispatchTarget>) -> Self {
        Self {
            config,
            target,
            stats: Arc::new(RwLock::new(DispatcherStats::default())),
        }
    }

    /// Dispatch a batch of jobs
    pub async fn dispatch(&self, jobs: Vec<DispatchJobCreation>) -> BatchDispatchResult {
        if jobs.is_empty() {
            return BatchDispatchResult::empty();
        }

        let total = jobs.len();
        info!("Dispatching batch of {} jobs", total);

        // Split into chunks if needed
        let chunks: Vec<Vec<DispatchJobCreation>> = jobs
            .chunks(self.config.max_batch_size)
            .map(|c| c.to_vec())
            .collect();

        let mut overall_result = BatchDispatchResult::empty();
        overall_result.total = total;

        for chunk in chunks {
            let chunk_result = self.dispatch_chunk(chunk).await;
            overall_result.succeeded += chunk_result.succeeded;
            overall_result.failed += chunk_result.failed;
            overall_result.errors.extend(chunk_result.errors);
        }

        // Update stats
        {
            let mut stats = self.stats.write().await;
            stats.batches_dispatched += 1;
            stats.jobs_dispatched += overall_result.succeeded as u64;
            stats.jobs_failed += overall_result.failed as u64;
            stats.last_dispatch_at = Some(Utc::now());
        }

        if overall_result.failed > 0 {
            warn!(
                "Batch dispatch completed with {} failures out of {}",
                overall_result.failed, total
            );
        } else {
            debug!("Batch dispatch completed successfully: {} jobs", total);
        }

        overall_result
    }

    /// Dispatch a chunk with retries
    async fn dispatch_chunk(&self, jobs: Vec<DispatchJobCreation>) -> BatchDispatchResult {
        let mut remaining = jobs;
        let mut final_result = BatchDispatchResult::empty();
        final_result.total = remaining.len();

        for attempt in 0..=self.config.retry_attempts {
            if remaining.is_empty() {
                break;
            }

            if attempt > 0 {
                debug!("Retry attempt {} for {} jobs", attempt, remaining.len());
                tokio::time::sleep(tokio::time::Duration::from_millis(
                    self.config.retry_delay_ms * attempt as u64,
                ))
                .await;

                let mut stats = self.stats.write().await;
                stats.retries += 1;
            }

            let result = self.target.dispatch_batch(&remaining).await;
            final_result.succeeded += result.succeeded;

            // Collect jobs that need retry
            let failed_job_ids: std::collections::HashSet<_> =
                result.errors.iter()
                    .filter(|e| e.retryable)
                    .map(|e| e.job_id.clone())
                    .collect();

            // Non-retryable failures are final
            for err in &result.errors {
                if !err.retryable {
                    final_result.failed += 1;
                    final_result.errors.push(err.clone());
                }
            }

            // Keep only retryable failures for next attempt
            remaining.retain(|j| failed_job_ids.contains(&j.id));
        }

        // Any remaining after all retries are failures
        for job in remaining {
            final_result.failed += 1;
            final_result.errors.push(DispatchError {
                job_id: job.id,
                error: "Max retries exceeded".to_string(),
                retryable: false,
            });
        }

        final_result
    }

    /// Get current stats
    pub async fn stats(&self) -> DispatcherStats {
        self.stats.read().await.clone()
    }

    /// Reset stats
    pub async fn reset_stats(&self) {
        let mut stats = self.stats.write().await;
        *stats = DispatcherStats::default();
    }
}

/// MongoDB-based dispatch target that inserts into dispatch_jobs collection
pub struct MongoDispatchTarget {
    collection: mongodb::Collection<mongodb::bson::Document>,
}

impl MongoDispatchTarget {
    pub fn new(db: &mongodb::Database) -> Self {
        Self {
            collection: db.collection("dispatch_jobs"),
        }
    }
}

#[async_trait]
impl DispatchTarget for MongoDispatchTarget {
    async fn dispatch(&self, job: &DispatchJobCreation) -> Result<(), String> {
        let doc = mongodb::bson::to_document(job)
            .map_err(|e| format!("Serialization error: {}", e))?;

        self.collection
            .insert_one(doc)
            .await
            .map_err(|e| format!("MongoDB insert error: {}", e))?;

        Ok(())
    }

    async fn dispatch_batch(&self, jobs: &[DispatchJobCreation]) -> BatchDispatchResult {
        if jobs.is_empty() {
            return BatchDispatchResult::empty();
        }

        let docs: Result<Vec<_>, _> = jobs
            .iter()
            .map(|j| mongodb::bson::to_document(j))
            .collect();

        let docs = match docs {
            Ok(d) => d,
            Err(e) => {
                return BatchDispatchResult {
                    total: jobs.len(),
                    succeeded: 0,
                    failed: jobs.len(),
                    errors: jobs
                        .iter()
                        .map(|j| DispatchError {
                            job_id: j.id.clone(),
                            error: format!("Serialization error: {}", e),
                            retryable: false,
                        })
                        .collect(),
                };
            }
        };

        match self.collection.insert_many(docs).await {
            Ok(_) => BatchDispatchResult::success(jobs.len()),
            Err(e) => {
                error!("MongoDB batch insert error: {}", e);
                // On batch failure, we don't know which succeeded
                // Conservative approach: mark all as failed but retryable
                BatchDispatchResult {
                    total: jobs.len(),
                    succeeded: 0,
                    failed: jobs.len(),
                    errors: jobs
                        .iter()
                        .map(|j| DispatchError {
                            job_id: j.id.clone(),
                            error: format!("MongoDB insert error: {}", e),
                            retryable: true,
                        })
                        .collect(),
                }
            }
        }
    }
}

/// In-memory dispatch target for testing
pub struct InMemoryDispatchTarget {
    jobs: Arc<RwLock<Vec<DispatchJobCreation>>>,
}

impl InMemoryDispatchTarget {
    pub fn new() -> Self {
        Self {
            jobs: Arc::new(RwLock::new(vec![])),
        }
    }

    pub async fn jobs(&self) -> Vec<DispatchJobCreation> {
        self.jobs.read().await.clone()
    }

    pub async fn clear(&self) {
        self.jobs.write().await.clear();
    }
}

impl Default for InMemoryDispatchTarget {
    fn default() -> Self {
        Self::new()
    }
}

#[async_trait]
impl DispatchTarget for InMemoryDispatchTarget {
    async fn dispatch(&self, job: &DispatchJobCreation) -> Result<(), String> {
        self.jobs.write().await.push(job.clone());
        Ok(())
    }
}

#[cfg(test)]
mod tests {
    use super::*;

    fn create_test_job(id: &str) -> DispatchJobCreation {
        DispatchJobCreation {
            id: id.to_string(),
            event_id: "evt-1".to_string(),
            subscription_id: "sub-1".to_string(),
            subscription_code: "test-sub".to_string(),
            client_id: None,
            event_type_code: "orders:*:*:*".to_string(),
            correlation_id: None,
            source_id: None,
            target: "http://example.com/webhook".to_string(),
            dispatch_pool_id: None,
            service_account_id: None,
            mode: "IMMEDIATE".to_string(),
            delay_seconds: 0,
            sequence: 99,
            timeout_seconds: 30,
            max_retries: 3,
            data_only: false,
            payload: serde_json::json!({"test": "data"}),
            scheduled_for: Utc::now(),
            created_at: Utc::now(),
        }
    }

    #[tokio::test]
    async fn test_dispatch_batch() {
        let target = Arc::new(InMemoryDispatchTarget::new());
        let dispatcher = BatchDispatcher::new(
            BatchDispatcherConfig::default(),
            target.clone(),
        );

        let jobs = vec![
            create_test_job("job-1"),
            create_test_job("job-2"),
            create_test_job("job-3"),
        ];

        let result = dispatcher.dispatch(jobs).await;

        assert_eq!(result.total, 3);
        assert_eq!(result.succeeded, 3);
        assert_eq!(result.failed, 0);

        let stored = target.jobs().await;
        assert_eq!(stored.len(), 3);
    }

    #[tokio::test]
    async fn test_dispatch_stats() {
        let target = Arc::new(InMemoryDispatchTarget::new());
        let dispatcher = BatchDispatcher::new(
            BatchDispatcherConfig::default(),
            target.clone(),
        );

        dispatcher.dispatch(vec![create_test_job("job-1")]).await;
        dispatcher.dispatch(vec![create_test_job("job-2"), create_test_job("job-3")]).await;

        let stats = dispatcher.stats().await;
        assert_eq!(stats.batches_dispatched, 2);
        assert_eq!(stats.jobs_dispatched, 3);
    }
}
