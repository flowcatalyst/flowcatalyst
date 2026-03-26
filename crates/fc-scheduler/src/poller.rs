//! Pending job poller
//!
//! Polls for PENDING dispatch jobs, groups by message_group, filters by
//! dispatch mode and connection status, then submits to the MessageGroupDispatcher
//! for ordered, semaphore-limited dispatch.

use std::collections::{HashMap, HashSet};
use std::sync::Arc;
use std::time::{Duration, Instant};

use parking_lot::RwLock;
use sea_orm::{
    DatabaseBackend, DatabaseConnection, FromQueryResult, Statement,
};
use tracing::{debug, info, trace, warn};

use crate::{
    BlockOnErrorChecker, DispatchJob, DispatchMode,
    MessageGroupDispatcher, SchedulerConfig, SchedulerError,
};

const DEFAULT_MESSAGE_GROUP: &str = "default";

/// Caches the set of subscription IDs whose connections are PAUSED.
/// Refreshed in the background on a configurable interval (default 60s).
/// Avoids a JOIN query on every 5-second poll cycle.
#[derive(Clone)]
pub struct PausedConnectionCache {
    db: DatabaseConnection,
    /// Cached set of paused subscription IDs
    cache: Arc<RwLock<HashSet<String>>>,
    /// When the cache was last refreshed
    last_refresh: Arc<RwLock<Instant>>,
    /// How long before the cache is considered stale
    ttl: Duration,
}

impl PausedConnectionCache {
    pub fn new(db: DatabaseConnection, ttl: Duration) -> Self {
        Self {
            db,
            cache: Arc::new(RwLock::new(HashSet::new())),
            last_refresh: Arc::new(RwLock::new(Instant::now() - ttl - Duration::from_secs(1))), // force initial refresh
            ttl,
        }
    }

    /// Get the cached set of paused subscription IDs.
    /// If the cache is stale, refreshes synchronously before returning.
    pub async fn get_paused_subscription_ids(&self) -> Result<HashSet<String>, SchedulerError> {
        if self.last_refresh.read().elapsed() > self.ttl {
            self.refresh().await?;
        }
        Ok(self.cache.read().clone())
    }

    /// Refresh the cache from the database.
    pub async fn refresh(&self) -> Result<(), SchedulerError> {
        let sql = "SELECT s.id FROM msg_subscriptions s \
                   JOIN msg_connections c ON c.id = s.connection_id \
                   WHERE c.status = 'PAUSED'";

        #[derive(FromQueryResult)]
        struct SubIdRow { id: String }

        let rows = SubIdRow::find_by_statement(
            Statement::from_sql_and_values(DatabaseBackend::Postgres, sql, vec![]),
        )
        .all(&self.db)
        .await?;

        let paused: HashSet<String> = rows.into_iter().map(|r| r.id).collect();
        let count = paused.len();

        *self.cache.write() = paused;
        *self.last_refresh.write() = Instant::now();

        debug!(paused_subscriptions = count, "Refreshed paused connection cache");
        Ok(())
    }

    /// Spawn a background task that refreshes the cache periodically.
    pub fn spawn_refresh_task(&self, shutdown: tokio::sync::broadcast::Sender<()>) {
        let cache = self.clone();
        let interval = self.ttl;
        let mut shutdown_rx = shutdown.subscribe();

        tokio::spawn(async move {
            let mut ticker = tokio::time::interval(interval);
            // Skip first tick (handled by initial get_paused_subscription_ids call)
            ticker.tick().await;

            loop {
                tokio::select! {
                    _ = ticker.tick() => {
                        if let Err(e) = cache.refresh().await {
                            warn!(error = %e, "Failed to refresh paused connection cache");
                        }
                    }
                    _ = shutdown_rx.recv() => {
                        info!("Paused connection cache refresh task shutting down");
                        break;
                    }
                }
            }
        });
    }
}

#[derive(Clone)]
pub struct PendingJobPoller {
    config: SchedulerConfig,
    db: DatabaseConnection,
    block_checker: Arc<BlockOnErrorChecker>,
    group_dispatcher: Arc<MessageGroupDispatcher>,
    paused_cache: Arc<PausedConnectionCache>,
}

impl PendingJobPoller {
    pub fn new(
        config: SchedulerConfig,
        db: DatabaseConnection,
        group_dispatcher: Arc<MessageGroupDispatcher>,
    ) -> Self {
        let block_checker = Arc::new(BlockOnErrorChecker::new(db.clone()));
        let paused_cache = Arc::new(PausedConnectionCache::new(
            db.clone(),
            Duration::from_secs(60), // 60s TTL
        ));
        Self {
            config,
            db,
            block_checker,
            group_dispatcher,
            paused_cache,
        }
    }

    /// Get the paused connection cache (for spawning the background refresh task)
    pub fn paused_cache(&self) -> &Arc<PausedConnectionCache> {
        &self.paused_cache
    }

    /// Poll for pending jobs and submit them for dispatch.
    /// Returns the number of jobs found (used for adaptive polling).
    pub async fn poll(&self) -> Result<usize, SchedulerError> {
        let pending_jobs = self.find_pending_jobs().await?;
        if pending_jobs.is_empty() {
            trace!("No pending jobs found");
            return Ok(0);
        }

        let job_count = pending_jobs.len();
        debug!(count = job_count, "Found pending jobs to process");
        metrics::gauge!("scheduler.pending_jobs").set(job_count as f64);

        // Group by message_group
        let jobs_by_group = Self::group_by_message_group(pending_jobs);
        let groups: HashSet<String> = jobs_by_group.keys().cloned().collect();

        // Batch check for blocked groups (single query)
        let blocked_groups = self.block_checker.get_blocked_groups(&groups).await?;
        metrics::gauge!("scheduler.blocked_groups").set(blocked_groups.len() as f64);

        // Process each group
        for (group, jobs) in jobs_by_group {
            if blocked_groups.contains(&group) {
                debug!(group = %group, count = jobs.len(), "Message group blocked, skipping");
                metrics::counter!("scheduler.jobs.blocked_total").increment(jobs.len() as u64);
                continue;
            }

            // Filter by dispatch mode
            let dispatchable = Self::filter_by_dispatch_mode(jobs, &blocked_groups);
            if !dispatchable.is_empty() {
                debug!(group = %group, count = dispatchable.len(), "Submitting jobs for message group");
                // Submit to the group dispatcher (1-in-flight per group, semaphore-limited)
                self.group_dispatcher.submit_jobs(&group, dispatchable);
            }
        }
        Ok(job_count)
    }

    /// Query PENDING jobs with proper ordering: message_group, sequence, created_at.
    /// Connection filtering is applied post-query (matching TS ConnectionCache pattern).
    async fn find_pending_jobs(&self) -> Result<Vec<DispatchJob>, SchedulerError> {
        let sql = "SELECT id, message_group, dispatch_pool_id, status, mode, target_url, \
                    payload, sequence, created_at, updated_at, queued_at, last_error, subscription_id \
             FROM msg_dispatch_jobs \
             WHERE status = 'PENDING' \
             ORDER BY message_group ASC NULLS LAST, sequence ASC, created_at ASC \
             LIMIT $1";

        let jobs = DispatchJob::find_by_statement(
            Statement::from_sql_and_values(
                DatabaseBackend::Postgres,
                sql,
                vec![sea_orm::Value::from(self.config.batch_size as i64)],
            ),
        )
        .all(&self.db)
        .await?;

        if !self.config.connection_filter_enabled || jobs.is_empty() {
            return Ok(jobs);
        }

        // Filter using cached paused connection set (refreshed every 60s)
        let paused_sub_ids = self.paused_cache.get_paused_subscription_ids().await?;
        if paused_sub_ids.is_empty() {
            return Ok(jobs);
        }

        let filtered: Vec<DispatchJob> = jobs.into_iter()
            .filter(|j| {
                match &j.subscription_id {
                    Some(sid) => !paused_sub_ids.contains(sid),
                    None => true,
                }
            })
            .collect();

        Ok(filtered)
    }

    fn group_by_message_group(jobs: Vec<DispatchJob>) -> HashMap<String, Vec<DispatchJob>> {
        let mut grouped: HashMap<String, Vec<DispatchJob>> = HashMap::new();
        for job in jobs {
            let group = job.message_group.clone().unwrap_or_else(|| DEFAULT_MESSAGE_GROUP.to_string());
            grouped.entry(group).or_default().push(job);
        }
        grouped
    }

    fn filter_by_dispatch_mode(jobs: Vec<DispatchJob>, blocked_groups: &HashSet<String>) -> Vec<DispatchJob> {
        jobs.into_iter()
            .filter(|job| {
                let group = job.message_group.as_deref().unwrap_or(DEFAULT_MESSAGE_GROUP);
                match job.dispatch_mode() {
                    DispatchMode::Immediate => true,
                    DispatchMode::NextOnError | DispatchMode::BlockOnError => {
                        !blocked_groups.contains(group)
                    }
                }
            })
            .collect()
    }
}
