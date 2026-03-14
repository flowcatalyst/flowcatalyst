//! Pending job poller
//!
//! Polls for PENDING dispatch jobs, groups by message_group, filters by
//! dispatch mode and connection status, then submits to the MessageGroupDispatcher
//! for ordered, semaphore-limited dispatch.

use std::collections::{HashMap, HashSet};
use std::sync::Arc;

use sea_orm::{
    DatabaseBackend, DatabaseConnection, FromQueryResult, Statement,
};
use tracing::{debug, trace};

use crate::{
    BlockOnErrorChecker, DispatchJob, DispatchMode,
    MessageGroupDispatcher, SchedulerConfig, SchedulerError,
};

const DEFAULT_MESSAGE_GROUP: &str = "default";

#[derive(Clone)]
pub struct PendingJobPoller {
    config: SchedulerConfig,
    db: DatabaseConnection,
    block_checker: Arc<BlockOnErrorChecker>,
    group_dispatcher: Arc<MessageGroupDispatcher>,
}

impl PendingJobPoller {
    pub fn new(
        config: SchedulerConfig,
        db: DatabaseConnection,
        group_dispatcher: Arc<MessageGroupDispatcher>,
    ) -> Self {
        let block_checker = Arc::new(BlockOnErrorChecker::new(db.clone()));
        Self {
            config,
            db,
            block_checker,
            group_dispatcher,
        }
    }

    pub async fn poll(&self) -> Result<(), SchedulerError> {
        let pending_jobs = self.find_pending_jobs().await?;
        if pending_jobs.is_empty() {
            trace!("No pending jobs found");
            return Ok(());
        }

        debug!(count = pending_jobs.len(), "Found pending jobs to process");
        metrics::gauge!("scheduler.pending_jobs").set(pending_jobs.len() as f64);

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
        Ok(())
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

        // Post-query connection filter: batch-resolve connection statuses via subscription IDs
        let sub_ids: Vec<String> = jobs.iter()
            .filter_map(|j| j.subscription_id.clone())
            .collect::<HashSet<_>>()
            .into_iter()
            .collect();

        if sub_ids.is_empty() {
            return Ok(jobs);
        }

        let paused_sub_ids = self.find_paused_subscription_ids(&sub_ids).await?;
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

    /// Batch-query subscription IDs whose connections are PAUSED.
    async fn find_paused_subscription_ids(&self, sub_ids: &[String]) -> Result<HashSet<String>, SchedulerError> {
        if sub_ids.is_empty() {
            return Ok(HashSet::new());
        }

        let placeholders: Vec<String> = (1..=sub_ids.len()).map(|i| format!("${}", i)).collect();
        let sql = format!(
            "SELECT s.id FROM msg_subscriptions s \
             JOIN msg_connections c ON c.id = s.connection_id \
             WHERE s.id IN ({}) AND c.status = 'PAUSED'",
            placeholders.join(", ")
        );

        #[derive(FromQueryResult)]
        struct SubIdRow { id: String }

        let values: Vec<sea_orm::Value> = sub_ids.iter()
            .map(|s| sea_orm::Value::from(s.clone()))
            .collect();

        let rows = SubIdRow::find_by_statement(
            Statement::from_sql_and_values(DatabaseBackend::Postgres, &sql, values),
        )
        .all(&self.db)
        .await?;

        Ok(rows.into_iter().map(|r| r.id).collect())
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
