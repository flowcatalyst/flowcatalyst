//! Dispatch Scheduler Service
//!
//! Handles polling for pending and stale dispatch jobs.
//! Moves jobs through the dispatch lifecycle.

use std::sync::Arc;
use std::time::Duration;
use chrono::Utc;
use tokio::sync::Mutex;
use tokio::task::JoinHandle;
use tracing::{info, warn, error, debug};

use crate::{DispatchJob, DispatchStatus, ErrorType};
use crate::DispatchJobRepository;
use crate::shared::error::Result;

/// Configuration for the dispatch scheduler
#[derive(Debug, Clone)]
pub struct DispatchConfig {
    /// Interval between polling for pending jobs
    pub pending_poll_interval: Duration,

    /// Interval between polling for stale jobs
    pub stale_poll_interval: Duration,

    /// How long a job can be in-progress before considered stale
    pub stale_threshold: Duration,

    /// How long a job can be in QUEUED status before considered stuck
    pub queued_stale_threshold: Duration,

    /// Maximum number of retry attempts
    pub max_retries: u32,

    /// Batch size for polling
    pub poll_batch_size: i64,

    /// Enable dispatch processing
    pub enabled: bool,

    /// Interval for checking blocked message groups
    pub block_check_interval: Duration,

    /// Interval for checking stale queued jobs
    pub queued_stale_check_interval: Duration,
}

impl Default for DispatchConfig {
    fn default() -> Self {
        Self {
            pending_poll_interval: Duration::from_secs(5),
            stale_poll_interval: Duration::from_secs(30),
            stale_threshold: Duration::from_secs(300), // 5 minutes
            queued_stale_threshold: Duration::from_secs(600), // 10 minutes
            max_retries: 3,
            poll_batch_size: 100,
            enabled: true,
            block_check_interval: Duration::from_secs(60), // 1 minute
            queued_stale_check_interval: Duration::from_secs(120), // 2 minutes
        }
    }
}

/// Dispatch job processor callback type
pub type JobProcessor = Arc<dyn Fn(DispatchJob) -> std::pin::Pin<Box<dyn std::future::Future<Output = Result<()>> + Send>> + Send + Sync>;

/// Dispatch Scheduler - manages polling loops for job processing
pub struct DispatchScheduler {
    config: DispatchConfig,
    job_repo: Arc<DispatchJobRepository>,
    processor: Option<JobProcessor>,
    running: Arc<Mutex<bool>>,
    handles: Arc<Mutex<Vec<JoinHandle<()>>>>,
}

impl DispatchScheduler {
    pub fn new(
        config: DispatchConfig,
        job_repo: Arc<DispatchJobRepository>,
    ) -> Self {
        Self {
            config,
            job_repo,
            processor: None,
            running: Arc::new(Mutex::new(false)),
            handles: Arc::new(Mutex::new(vec![])),
        }
    }

    /// Set the job processor callback
    pub fn with_processor(mut self, processor: JobProcessor) -> Self {
        self.processor = Some(processor);
        self
    }

    /// Start the scheduler polling loops
    pub async fn start(&self) -> Result<()> {
        if !self.config.enabled {
            info!("Dispatch scheduler disabled");
            return Ok(());
        }

        let mut running = self.running.lock().await;
        if *running {
            warn!("Dispatch scheduler already running");
            return Ok(());
        }
        *running = true;
        drop(running);

        info!("Starting dispatch scheduler");

        // Start pending job poller
        let pending_handle = self.start_pending_poller().await;

        // Start stale job poller
        let stale_handle = self.start_stale_poller().await;

        let mut handles = self.handles.lock().await;
        handles.push(pending_handle);
        handles.push(stale_handle);

        info!("Dispatch scheduler started");
        Ok(())
    }

    /// Stop the scheduler
    pub async fn stop(&self) {
        info!("Stopping dispatch scheduler");
        let mut running = self.running.lock().await;
        *running = false;
        drop(running);

        let mut handles = self.handles.lock().await;
        for handle in handles.drain(..) {
            handle.abort();
        }

        info!("Dispatch scheduler stopped");
    }

    /// Start the pending job poller
    async fn start_pending_poller(&self) -> JoinHandle<()> {
        let running = self.running.clone();
        let job_repo = self.job_repo.clone();
        let processor = self.processor.clone();
        let interval = self.config.pending_poll_interval;
        let batch_size = self.config.poll_batch_size;

        tokio::spawn(async move {
            info!("Pending job poller started");
            loop {
                // Check if we should continue running
                {
                    let is_running = running.lock().await;
                    if !*is_running {
                        break;
                    }
                }

                // Poll for pending jobs
                match job_repo.find_pending_for_dispatch(batch_size).await {
                    Ok(jobs) if !jobs.is_empty() => {
                        debug!("Found {} pending jobs", jobs.len());
                        for job in jobs {
                            if let Some(ref proc) = processor {
                                if let Err(e) = proc(job.clone()).await {
                                    error!("Failed to process job {}: {:?}", job.id, e);
                                }
                            } else {
                                // No processor - just mark as queued
                                if let Err(e) = Self::queue_job(&job_repo, job).await {
                                    error!("Failed to queue job: {:?}", e);
                                }
                            }
                        }
                    }
                    Ok(_) => {
                        debug!("No pending jobs found");
                    }
                    Err(e) => {
                        error!("Error polling for pending jobs: {:?}", e);
                    }
                }

                tokio::time::sleep(interval).await;
            }
            info!("Pending job poller stopped");
        })
    }

    /// Start the stale job poller
    async fn start_stale_poller(&self) -> JoinHandle<()> {
        let running = self.running.clone();
        let job_repo = self.job_repo.clone();
        let interval = self.config.stale_poll_interval;
        let threshold = self.config.stale_threshold;
        let max_retries = self.config.max_retries;
        let batch_size = self.config.poll_batch_size;

        tokio::spawn(async move {
            info!("Stale job poller started");
            loop {
                // Check if we should continue running
                {
                    let is_running = running.lock().await;
                    if !*is_running {
                        break;
                    }
                }

                // Calculate cutoff time
                let cutoff = Utc::now() - chrono::Duration::from_std(threshold)
                    .unwrap_or_else(|_| chrono::Duration::seconds(300));

                // Poll for stale jobs
                match job_repo.find_stale_in_progress(cutoff, batch_size).await {
                    Ok(jobs) if !jobs.is_empty() => {
                        warn!("Found {} stale in-progress jobs", jobs.len());
                        for job in jobs {
                            if let Err(e) = Self::handle_stale_job(&job_repo, job, max_retries).await {
                                error!("Failed to handle stale job: {:?}", e);
                            }
                        }
                    }
                    Ok(_) => {
                        debug!("No stale jobs found");
                    }
                    Err(e) => {
                        error!("Error polling for stale jobs: {:?}", e);
                    }
                }

                tokio::time::sleep(interval).await;
            }
            info!("Stale job poller stopped");
        })
    }

    /// Queue a pending job
    async fn queue_job(repo: &DispatchJobRepository, mut job: DispatchJob) -> Result<()> {
        job.mark_queued();
        repo.update(&job).await?;
        debug!("Queued job {}", job.id);
        Ok(())
    }

    /// Handle a stale job - either retry or fail it
    async fn handle_stale_job(
        repo: &DispatchJobRepository,
        mut job: DispatchJob,
        max_retries: u32,
    ) -> Result<()> {
        if job.attempt_count >= max_retries {
            // Fail the job
            job.record_failure(
                "Job timed out after maximum retries".to_string(),
                ErrorType::Timeout,
                None,
            );
            repo.update(&job).await?;
            warn!("Job {} failed after {} attempts", job.id, job.attempt_count);
        } else {
            // Retry the job - reset to queued status
            job.status = DispatchStatus::Queued;
            job.updated_at = Utc::now();
            repo.update(&job).await?;
            info!("Requeued stale job {} (attempt {})", job.id, job.attempt_count);
        }
        Ok(())
    }
}

/// Event dispatcher - creates dispatch jobs for subscriptions
pub struct EventDispatcher {
    job_repo: Arc<DispatchJobRepository>,
}

impl EventDispatcher {
    pub fn new(job_repo: Arc<DispatchJobRepository>) -> Self {
        Self { job_repo }
    }

    /// Dispatch an event to matching subscriptions
    /// Returns the created dispatch job IDs
    pub async fn dispatch(
        &self,
        event_id: &str,
        event_type: &str,
        source: &str,
        subject: Option<&str>,
        data: serde_json::Value,
        correlation_id: Option<&str>,
        message_group: Option<&str>,
        client_id: Option<&str>,
        subscriptions: Vec<crate::Subscription>,
    ) -> Result<Vec<String>> {
        let mut job_ids = Vec::new();

        // Serialize payload once
        let payload = serde_json::to_string(&data).unwrap_or_default();

        for subscription in subscriptions {
            // Skip if subscription doesn't match the event
            if !subscription.matches_event_type(event_type) {
                continue;
            }

            // Skip if subscription doesn't match the client
            if !subscription.matches_client(client_id) {
                continue;
            }

            // Create dispatch job using for_event constructor
            let mut job = DispatchJob::for_event(
                event_id,
                event_type,
                source,
                &subscription.target,
                &payload,
            );

            // Set subject
            if let Some(sub) = subject {
                job.subject = Some(sub.to_string());
            }

            // Set correlation ID
            if let Some(corr) = correlation_id {
                job = job.with_correlation_id(corr);
            }

            // Set message group
            if let Some(group) = message_group {
                job = job.with_message_group(group);
            }

            // Set client ID
            if let Some(cid) = client_id {
                job = job.with_client_id(cid);
            }

            // Set subscription details
            job = job
                .with_subscription_id(&subscription.id)
                .with_mode(subscription.mode.clone())
                .with_data_only(subscription.data_only);

            // Set dispatch pool if configured
            if let Some(ref pool_id) = subscription.dispatch_pool_id {
                job = job.with_dispatch_pool_id(pool_id);
            }

            // Set service account if configured
            if let Some(ref sa_id) = subscription.service_account_id {
                job = job.with_service_account_id(sa_id);
            }

            job.max_retries = subscription.max_retries;
            job.timeout_seconds = subscription.timeout_seconds;

            job_ids.push(job.id.clone());
            self.job_repo.insert(&job).await?;

            debug!(
                "Created dispatch job {} for event {} to subscription {}",
                job.id, event_id, subscription.id
            );
        }

        info!(
            "Created {} dispatch jobs for event {}",
            job_ids.len(),
            event_id
        );

        Ok(job_ids)
    }
}

/// Blocked message group info
#[derive(Debug, Clone)]
pub struct BlockedMessageGroup {
    pub message_group_id: String,
    pub blocked_job_id: String,
    pub error_message: String,
    pub blocked_since: chrono::DateTime<Utc>,
    pub pending_jobs_count: u32,
}

/// Block on error checker - monitors message groups that are blocked due to errors
pub struct BlockOnErrorChecker {
    job_repo: Arc<DispatchJobRepository>,
    config: DispatchConfig,
    running: Arc<Mutex<bool>>,
}

impl BlockOnErrorChecker {
    pub fn new(job_repo: Arc<DispatchJobRepository>, config: DispatchConfig) -> Self {
        Self {
            job_repo,
            config,
            running: Arc::new(Mutex::new(false)),
        }
    }

    /// Start the checker loop
    pub async fn start(&self) -> JoinHandle<()> {
        let running = self.running.clone();
        let job_repo = self.job_repo.clone();
        let interval = self.config.block_check_interval;
        let batch_size = self.config.poll_batch_size;

        {
            let mut r = running.lock().await;
            *r = true;
        }

        tokio::spawn(async move {
            info!("Block on error checker started");
            loop {
                {
                    let is_running = running.lock().await;
                    if !*is_running {
                        break;
                    }
                }

                // Find jobs that have failed - some may be blocking message groups
                match job_repo.find_by_status(DispatchStatus::Failed, batch_size).await {
                    Ok(failed_jobs) if !failed_jobs.is_empty() => {
                        // Filter for jobs that have message groups and recent failures
                        let recent_cutoff = Utc::now() - chrono::Duration::hours(1);
                        let blocking_jobs: Vec<_> = failed_jobs
                            .iter()
                            .filter(|j| j.message_group.is_some() && j.updated_at > recent_cutoff)
                            .collect();

                        if !blocking_jobs.is_empty() {
                            warn!("Found {} potentially blocking failed jobs", blocking_jobs.len());

                            // Group by message_group
                            let mut groups: std::collections::HashMap<String, Vec<&&DispatchJob>> =
                                std::collections::HashMap::new();

                            for job in &blocking_jobs {
                                if let Some(ref group_id) = job.message_group {
                                    groups.entry(group_id.clone()).or_default().push(job);
                                }
                            }

                            for (group_id, jobs) in groups {
                                let oldest = jobs.iter().min_by_key(|j| j.updated_at);
                                if let Some(oldest_job) = oldest {
                                    info!(
                                        "Message group {} may be blocked by job {} since {}: {}",
                                        group_id,
                                        oldest_job.id,
                                        oldest_job.updated_at,
                                        oldest_job.last_error.as_deref().unwrap_or("unknown error")
                                    );
                                }
                            }
                        }
                    }
                    Ok(_) => {
                        debug!("No failed jobs found");
                    }
                    Err(e) => {
                        error!("Error checking failed jobs: {:?}", e);
                    }
                }

                tokio::time::sleep(interval).await;
            }
            info!("Block on error checker stopped");
        })
    }

    /// Stop the checker
    pub async fn stop(&self) {
        let mut running = self.running.lock().await;
        *running = false;
    }

    /// Get currently blocked message groups (failed jobs with message groups)
    pub async fn get_blocked_groups(&self) -> Result<Vec<BlockedMessageGroup>> {
        let failed_jobs = self.job_repo
            .find_by_status(DispatchStatus::Failed, self.config.poll_batch_size)
            .await?;

        // Filter for recent failures with message groups
        let recent_cutoff = Utc::now() - chrono::Duration::hours(1);
        let blocking_jobs: Vec<_> = failed_jobs
            .iter()
            .filter(|j| j.message_group.is_some() && j.updated_at > recent_cutoff)
            .collect();

        let mut groups: std::collections::HashMap<String, Vec<&&DispatchJob>> =
            std::collections::HashMap::new();

        for job in &blocking_jobs {
            if let Some(ref group_id) = job.message_group {
                groups.entry(group_id.clone()).or_default().push(job);
            }
        }

        let result: Vec<BlockedMessageGroup> = groups
            .into_iter()
            .filter_map(|(group_id, jobs)| {
                let oldest = jobs.iter().min_by_key(|j| j.updated_at)?;
                Some(BlockedMessageGroup {
                    message_group_id: group_id,
                    blocked_job_id: oldest.id.clone(),
                    error_message: oldest.last_error.clone().unwrap_or_default(),
                    blocked_since: oldest.updated_at,
                    pending_jobs_count: jobs.len() as u32,
                })
            })
            .collect();

        Ok(result)
    }

    /// Mark a failed job as acknowledged (won't show as blocking anymore)
    pub async fn acknowledge_failed_job(&self, job_id: &str) -> Result<()> {
        if let Some(mut job) = self.job_repo.find_by_id(job_id).await? {
            if job.status == DispatchStatus::Failed {
                // Update the timestamp so it falls outside the recent window
                job.updated_at = Utc::now() - chrono::Duration::hours(2);
                self.job_repo.update(&job).await?;
                info!("Acknowledged failed job {}", job_id);
            }
        }
        Ok(())
    }

    /// Retry a failed job by resetting it to pending
    pub async fn retry_failed_job(&self, job_id: &str) -> Result<()> {
        if let Some(mut job) = self.job_repo.find_by_id(job_id).await? {
            if job.status == DispatchStatus::Failed {
                job.status = DispatchStatus::Pending;
                job.last_error = None;
                job.updated_at = Utc::now();
                self.job_repo.update(&job).await?;
                info!("Retrying failed job {}", job_id);
            }
        }
        Ok(())
    }
}

/// Stale queued job poller - handles jobs stuck in QUEUED status
pub struct StaleQueuedJobPoller {
    job_repo: Arc<DispatchJobRepository>,
    config: DispatchConfig,
    running: Arc<Mutex<bool>>,
}

impl StaleQueuedJobPoller {
    pub fn new(job_repo: Arc<DispatchJobRepository>, config: DispatchConfig) -> Self {
        Self {
            job_repo,
            config,
            running: Arc::new(Mutex::new(false)),
        }
    }

    /// Start the poller loop
    pub async fn start(&self) -> JoinHandle<()> {
        let running = self.running.clone();
        let job_repo = self.job_repo.clone();
        let interval = self.config.queued_stale_check_interval;
        let threshold = self.config.queued_stale_threshold;
        let batch_size = self.config.poll_batch_size;
        let max_retries = self.config.max_retries;

        {
            let mut r = running.lock().await;
            *r = true;
        }

        tokio::spawn(async move {
            info!("Stale queued job poller started");
            loop {
                {
                    let is_running = running.lock().await;
                    if !*is_running {
                        break;
                    }
                }

                let cutoff = Utc::now() - chrono::Duration::from_std(threshold)
                    .unwrap_or_else(|_| chrono::Duration::seconds(600));

                // Find jobs stuck in QUEUED status
                match job_repo.find_by_status(DispatchStatus::Queued, batch_size).await {
                    Ok(queued_jobs) => {
                        let stale_jobs: Vec<_> = queued_jobs
                            .into_iter()
                            .filter(|j| j.updated_at < cutoff)
                            .collect();

                        if !stale_jobs.is_empty() {
                            warn!("Found {} stale queued jobs", stale_jobs.len());

                            for mut job in stale_jobs {
                                if job.attempt_count >= max_retries {
                                    // Mark as failed
                                    job.record_failure(
                                        "Job stuck in queued state after maximum retries".to_string(),
                                        ErrorType::Unknown,
                                        None,
                                    );
                                    warn!("Failed stale queued job {}", job.id);
                                } else {
                                    // Reset to pending for re-processing
                                    job.status = DispatchStatus::Pending;
                                    job.attempt_count += 1;
                                    job.updated_at = Utc::now();
                                    info!("Requeued stale job {} (attempt {})", job.id, job.attempt_count);
                                }

                                if let Err(e) = job_repo.update(&job).await {
                                    error!("Failed to update stale job {}: {:?}", job.id, e);
                                }
                            }
                        }
                    }
                    Err(e) => {
                        error!("Error polling for stale queued jobs: {:?}", e);
                    }
                }

                tokio::time::sleep(interval).await;
            }
            info!("Stale queued job poller stopped");
        })
    }

    /// Stop the poller
    pub async fn stop(&self) {
        let mut running = self.running.lock().await;
        *running = false;
    }

    /// Get count of stale queued jobs
    pub async fn count_stale_queued(&self) -> Result<usize> {
        let cutoff = Utc::now() - chrono::Duration::from_std(self.config.queued_stale_threshold)
            .unwrap_or_else(|_| chrono::Duration::seconds(600));

        let queued_jobs = self.job_repo
            .find_by_status(DispatchStatus::Queued, self.config.poll_batch_size)
            .await?;

        let stale_count = queued_jobs
            .into_iter()
            .filter(|j| j.updated_at < cutoff)
            .count();

        Ok(stale_count)
    }
}

#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn test_dispatch_config_default() {
        let config = DispatchConfig::default();
        assert!(config.enabled);
        assert_eq!(config.max_retries, 3);
        assert_eq!(config.poll_batch_size, 100);
    }
}
