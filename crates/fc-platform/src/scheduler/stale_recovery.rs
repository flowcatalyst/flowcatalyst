//! Stale job recovery - finds jobs stuck in QUEUED status and resets them

use chrono::Utc;
use sqlx::PgPool;
use tracing::{debug, info, warn};

use crate::scheduler::{SchedulerConfig, SchedulerError};

#[derive(Clone)]
pub struct StaleQueuedJobPoller {
    config: SchedulerConfig,
    pool: PgPool,
}

impl StaleQueuedJobPoller {
    pub fn new(config: SchedulerConfig, pool: PgPool) -> Self {
        Self { config, pool }
    }

    pub async fn recover_stale_jobs(&self) -> Result<usize, SchedulerError> {
        let threshold = Utc::now()
            - chrono::Duration::from_std(self.config.stale_threshold)
                .unwrap_or_else(|_| chrono::Duration::minutes(15));

        let sql = "UPDATE msg_dispatch_jobs SET status = 'PENDING', queued_at = NULL, updated_at = NOW() \
                    WHERE status = 'QUEUED' AND queued_at < $1";

        let result = sqlx::query(sql).bind(threshold).execute(&self.pool).await?;

        let count = result.rows_affected() as usize;

        metrics::counter!("scheduler.stale_jobs.recovered_total").increment(count as u64);
        metrics::gauge!("scheduler.stale_jobs.last_recovery_count").set(count as f64);

        if count > 0 {
            info!(
                count = count,
                threshold_mins = self.config.stale_threshold.as_secs() / 60,
                "Recovered stale QUEUED jobs"
            );
        } else {
            debug!("No stale jobs to recover");
        }

        Ok(count)
    }

    pub async fn count_queued_jobs(&self) -> Result<u64, SchedulerError> {
        let sql = "SELECT COUNT(*) FROM msg_dispatch_jobs WHERE status = 'QUEUED'";

        let count: i64 = sqlx::query_scalar(sql).fetch_one(&self.pool).await?;

        let count = count as u64;
        metrics::gauge!("scheduler.queued_jobs_total").set(count as f64);
        Ok(count)
    }

    pub async fn count_near_stale_jobs(&self) -> Result<u64, SchedulerError> {
        let warning_threshold = Utc::now()
            - chrono::Duration::from_std(self.config.stale_threshold / 2)
                .unwrap_or_else(|_| chrono::Duration::minutes(7));

        let sql =
            "SELECT COUNT(*) FROM msg_dispatch_jobs WHERE status = 'QUEUED' AND queued_at < $1";

        let count: i64 = sqlx::query_scalar(sql)
            .bind(warning_threshold)
            .fetch_one(&self.pool)
            .await?;

        let count = count as u64;

        if count > 0 {
            warn!(count = count, "Jobs approaching stale threshold");
            metrics::gauge!("scheduler.near_stale_jobs").set(count as f64);
        }

        Ok(count)
    }
}

#[cfg(test)]
mod tests {
    use super::*;
    use std::time::Duration;

    #[test]
    fn test_stale_threshold_conversion() {
        let config = SchedulerConfig {
            stale_threshold: Duration::from_secs(15 * 60),
            ..Default::default()
        };
        assert_eq!(config.stale_threshold.as_secs(), 15 * 60);
    }
}
