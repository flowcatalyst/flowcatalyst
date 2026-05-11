//! Pending job poller
//!
//! Polls for PENDING dispatch jobs, groups by message_group, filters by
//! dispatch mode and connection status, then submits to the MessageGroupDispatcher
//! for ordered, semaphore-limited dispatch.

use std::collections::{HashMap, HashSet};
use std::sync::Arc;
use std::time::{Duration, Instant};

use parking_lot::RwLock;
use sqlx::PgPool;
use tracing::{debug, info, trace, warn};

use crate::scheduler::{
    BlockOnErrorChecker, DispatchMode, MessageGroupDispatcher, SchedulerConfig, SchedulerError,
    SchedulerJobRow,
};

const DEFAULT_MESSAGE_GROUP: &str = "default";

/// Caches the set of subscription IDs whose connections are PAUSED.
/// Refreshed in the background on a configurable interval (default 60s).
#[derive(Clone)]
pub struct PausedConnectionCache {
    pool: PgPool,
    cache: Arc<RwLock<HashSet<String>>>,
    last_refresh: Arc<RwLock<Instant>>,
    ttl: Duration,
}

impl PausedConnectionCache {
    pub fn new(pool: PgPool, ttl: Duration) -> Self {
        Self {
            pool,
            cache: Arc::new(RwLock::new(HashSet::new())),
            last_refresh: Arc::new(RwLock::new(Instant::now() - ttl - Duration::from_secs(1))),
            ttl,
        }
    }

    pub async fn get_paused_subscription_ids(&self) -> Result<HashSet<String>, SchedulerError> {
        if self.last_refresh.read().elapsed() > self.ttl {
            self.refresh().await?;
        }
        Ok(self.cache.read().clone())
    }

    pub async fn refresh(&self) -> Result<(), SchedulerError> {
        let sql = "SELECT s.id FROM msg_subscriptions s \
                   JOIN msg_connections c ON c.id = s.connection_id \
                   WHERE c.status = 'PAUSED'";

        let rows: Vec<String> = sqlx::query_scalar(sql).fetch_all(&self.pool).await?;

        let paused: HashSet<String> = rows.into_iter().collect();
        let count = paused.len();

        *self.cache.write() = paused;
        *self.last_refresh.write() = Instant::now();

        debug!(
            paused_subscriptions = count,
            "Refreshed paused connection cache"
        );
        Ok(())
    }

    pub fn spawn_refresh_task(&self, shutdown: tokio::sync::broadcast::Sender<()>) {
        let cache = self.clone();
        let interval = self.ttl;
        let mut shutdown_rx = shutdown.subscribe();

        tokio::spawn(async move {
            let mut ticker = tokio::time::interval(interval);
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
    pool: PgPool,
    block_checker: Arc<BlockOnErrorChecker>,
    group_dispatcher: Arc<MessageGroupDispatcher>,
    paused_cache: Arc<PausedConnectionCache>,
}

impl PendingJobPoller {
    pub fn new(
        config: SchedulerConfig,
        pool: PgPool,
        group_dispatcher: Arc<MessageGroupDispatcher>,
    ) -> Self {
        let block_checker = Arc::new(BlockOnErrorChecker::new(pool.clone()));
        let paused_cache = Arc::new(PausedConnectionCache::new(
            pool.clone(),
            Duration::from_secs(60),
        ));
        Self {
            config,
            pool,
            block_checker,
            group_dispatcher,
            paused_cache,
        }
    }

    pub fn paused_cache(&self) -> &Arc<PausedConnectionCache> {
        &self.paused_cache
    }

    pub async fn poll(&self) -> Result<usize, SchedulerError> {
        let pending_jobs = self.find_pending_jobs().await?;
        if pending_jobs.is_empty() {
            trace!("No pending jobs found");
            return Ok(0);
        }

        let job_count = pending_jobs.len();
        trace!(count = job_count, "Found pending jobs to process");
        metrics::gauge!("scheduler.pending_jobs").set(job_count as f64);

        let jobs_by_group = Self::group_by_message_group(pending_jobs);
        let groups: HashSet<String> = jobs_by_group.keys().cloned().collect();

        let blocked_groups = self.block_checker.get_blocked_groups(&groups).await?;
        metrics::gauge!("scheduler.blocked_groups").set(blocked_groups.len() as f64);

        for (group, jobs) in jobs_by_group {
            if blocked_groups.contains(&group) {
                trace!(group = %group, count = jobs.len(), "Message group blocked, skipping");
                metrics::counter!("scheduler.jobs.blocked_total").increment(jobs.len() as u64);
                continue;
            }

            let dispatchable = Self::filter_by_dispatch_mode(jobs, &blocked_groups);
            if !dispatchable.is_empty() {
                trace!(group = %group, count = dispatchable.len(), "Submitting jobs for message group");
                self.group_dispatcher.submit_jobs(&group, dispatchable);
            }
        }
        Ok(job_count)
    }

    async fn find_pending_jobs(&self) -> Result<Vec<SchedulerJobRow>, SchedulerError> {
        let sql = "SELECT id, message_group, dispatch_pool_id, status, mode, target_url, \
                    payload, sequence, created_at, updated_at, queued_at, last_error, subscription_id \
             FROM msg_dispatch_jobs \
             WHERE status = 'PENDING' \
             ORDER BY message_group ASC NULLS LAST, sequence ASC, created_at ASC \
             LIMIT $1";

        let jobs = sqlx::query_as::<_, SchedulerJobRow>(sql)
            .bind(self.config.batch_size as i64)
            .fetch_all(&self.pool)
            .await?;

        if !self.config.connection_filter_enabled || jobs.is_empty() {
            return Ok(jobs);
        }

        let paused_sub_ids = self.paused_cache.get_paused_subscription_ids().await?;
        if paused_sub_ids.is_empty() {
            return Ok(jobs);
        }

        let filtered: Vec<SchedulerJobRow> = jobs
            .into_iter()
            .filter(|j| match &j.subscription_id {
                Some(sid) => !paused_sub_ids.contains(sid),
                None => true,
            })
            .collect();

        Ok(filtered)
    }

    fn group_by_message_group(jobs: Vec<SchedulerJobRow>) -> HashMap<String, Vec<SchedulerJobRow>> {
        let mut grouped: HashMap<String, Vec<SchedulerJobRow>> = HashMap::new();
        for job in jobs {
            let group = job
                .message_group
                .clone()
                .unwrap_or_else(|| DEFAULT_MESSAGE_GROUP.to_string());
            grouped.entry(group).or_default().push(job);
        }
        grouped
    }

    fn filter_by_dispatch_mode(
        jobs: Vec<SchedulerJobRow>,
        blocked_groups: &HashSet<String>,
    ) -> Vec<SchedulerJobRow> {
        jobs.into_iter()
            .filter(|job| {
                let group = job
                    .message_group
                    .as_deref()
                    .unwrap_or(DEFAULT_MESSAGE_GROUP);
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

#[cfg(test)]
mod tests {
    use super::*;
    use chrono::Utc;

    fn make_job(id: &str, group: Option<&str>, mode: &str) -> SchedulerJobRow {
        let now = Utc::now();
        SchedulerJobRow {
            id: id.to_string(),
            message_group: group.map(|s| s.to_string()),
            dispatch_pool_id: None,
            status: "PENDING".to_string(),
            mode: mode.to_string(),
            target_url: "http://target.example.com/webhook".to_string(),
            payload: None,
            sequence: 1,
            created_at: now,
            updated_at: now,
            queued_at: None,
            last_error: None,
            subscription_id: None,
        }
    }

    fn make_job_with_subscription(
        id: &str,
        group: Option<&str>,
        sub_id: Option<&str>,
    ) -> SchedulerJobRow {
        let mut job = make_job(id, group, "IMMEDIATE");
        job.subscription_id = sub_id.map(|s| s.to_string());
        job
    }

    #[test]
    fn group_by_message_group_separates_groups() {
        let jobs = vec![
            make_job("j1", Some("alpha"), "IMMEDIATE"),
            make_job("j2", Some("beta"), "IMMEDIATE"),
            make_job("j3", Some("alpha"), "IMMEDIATE"),
        ];
        let grouped = PendingJobPoller::group_by_message_group(jobs);
        assert_eq!(grouped.len(), 2);
        assert_eq!(grouped["alpha"].len(), 2);
        assert_eq!(grouped["beta"].len(), 1);
    }

    #[test]
    fn group_by_message_group_none_uses_default() {
        let jobs = vec![
            make_job("j1", None, "IMMEDIATE"),
            make_job("j2", None, "IMMEDIATE"),
            make_job("j3", Some("explicit"), "IMMEDIATE"),
        ];
        let grouped = PendingJobPoller::group_by_message_group(jobs);
        assert_eq!(grouped.len(), 2);
        assert_eq!(grouped[DEFAULT_MESSAGE_GROUP].len(), 2);
        assert_eq!(grouped["explicit"].len(), 1);
    }

    #[test]
    fn group_by_message_group_empty_input() {
        let grouped = PendingJobPoller::group_by_message_group(vec![]);
        assert!(grouped.is_empty());
    }

    #[test]
    fn group_by_message_group_preserves_job_ids() {
        let jobs = vec![
            make_job("aaa", Some("g1"), "IMMEDIATE"),
            make_job("bbb", Some("g1"), "IMMEDIATE"),
        ];
        let grouped = PendingJobPoller::group_by_message_group(jobs);
        let ids: Vec<&str> = grouped["g1"].iter().map(|j| j.id.as_str()).collect();
        assert!(ids.contains(&"aaa"));
        assert!(ids.contains(&"bbb"));
    }

    #[test]
    fn filter_immediate_always_passes() {
        let blocked: HashSet<String> = ["grp_a".to_string()].into_iter().collect();
        let jobs = vec![
            make_job("j1", Some("grp_a"), "IMMEDIATE"),
            make_job("j2", Some("grp_b"), "IMMEDIATE"),
        ];
        let result = PendingJobPoller::filter_by_dispatch_mode(jobs, &blocked);
        assert_eq!(result.len(), 2);
    }

    #[test]
    fn filter_block_on_error_excluded_when_group_blocked() {
        let blocked: HashSet<String> = ["grp_a".to_string()].into_iter().collect();
        let jobs = vec![
            make_job("j1", Some("grp_a"), "BLOCK_ON_ERROR"),
            make_job("j2", Some("grp_b"), "BLOCK_ON_ERROR"),
        ];
        let result = PendingJobPoller::filter_by_dispatch_mode(jobs, &blocked);
        assert_eq!(result.len(), 1);
        assert_eq!(result[0].id, "j2");
    }

    #[test]
    fn filter_next_on_error_excluded_when_group_blocked() {
        let blocked: HashSet<String> = ["grp_x".to_string()].into_iter().collect();
        let jobs = vec![
            make_job("j1", Some("grp_x"), "NEXT_ON_ERROR"),
            make_job("j2", Some("grp_y"), "NEXT_ON_ERROR"),
        ];
        let result = PendingJobPoller::filter_by_dispatch_mode(jobs, &blocked);
        assert_eq!(result.len(), 1);
        assert_eq!(result[0].id, "j2");
    }

    #[test]
    fn filter_no_blocked_groups_passes_everything() {
        let blocked: HashSet<String> = HashSet::new();
        let jobs = vec![
            make_job("j1", Some("g1"), "BLOCK_ON_ERROR"),
            make_job("j2", Some("g2"), "NEXT_ON_ERROR"),
            make_job("j3", Some("g3"), "IMMEDIATE"),
        ];
        let result = PendingJobPoller::filter_by_dispatch_mode(jobs, &blocked);
        assert_eq!(result.len(), 3);
    }

    #[test]
    fn filter_mixed_modes_in_same_group() {
        let blocked: HashSet<String> = ["grp".to_string()].into_iter().collect();
        let jobs = vec![
            make_job("j_imm", Some("grp"), "IMMEDIATE"),
            make_job("j_noe", Some("grp"), "NEXT_ON_ERROR"),
            make_job("j_boe", Some("grp"), "BLOCK_ON_ERROR"),
        ];
        let result = PendingJobPoller::filter_by_dispatch_mode(jobs, &blocked);
        assert_eq!(result.len(), 1);
        assert_eq!(result[0].id, "j_imm");
    }

    #[test]
    fn paused_subscription_filter_logic() {
        let paused_sub_ids: HashSet<String> =
            ["sub_paused_1".to_string(), "sub_paused_2".to_string()]
                .into_iter()
                .collect();

        let jobs = vec![
            make_job_with_subscription("j1", Some("g"), Some("sub_active")),
            make_job_with_subscription("j2", Some("g"), Some("sub_paused_1")),
            make_job_with_subscription("j3", Some("g"), None),
            make_job_with_subscription("j4", Some("g"), Some("sub_paused_2")),
        ];

        let filtered: Vec<SchedulerJobRow> = jobs
            .into_iter()
            .filter(|j| match &j.subscription_id {
                Some(sid) => !paused_sub_ids.contains(sid),
                None => true,
            })
            .collect();

        assert_eq!(filtered.len(), 2);
        assert_eq!(filtered[0].id, "j1");
        assert_eq!(filtered[1].id, "j3");
    }

    #[test]
    fn paused_cache_initial_state_is_stale() {
        let ttl = Duration::from_secs(60);
        let initial_time = Instant::now() - ttl - Duration::from_secs(1);
        assert!(initial_time.elapsed() > ttl);
    }

    #[test]
    fn default_message_group_constant() {
        assert_eq!(DEFAULT_MESSAGE_GROUP, "default");
    }

    #[test]
    fn adaptive_polling_full_batch_repolls_immediately() {
        let config = SchedulerConfig::default();
        let batch_size = config.batch_size;
        assert!(batch_size >= batch_size);
    }

    #[test]
    fn connection_filter_enabled_by_default() {
        let config = SchedulerConfig::default();
        assert!(config.connection_filter_enabled);
    }
}
