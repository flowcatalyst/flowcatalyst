use std::sync::Arc;
use std::time::Duration;

use sqlx::PgPool;
use tokio::sync::watch;
use tracing::{debug, error, info};

use crate::health::StreamHealth;

/// Projects dispatch jobs from `msg_dispatch_jobs` into `msg_dispatch_jobs_read`.
///
/// Reads rows where `projected_at IS NULL` (new inserts) or where `updated_at > projected_at`
/// (status changes), upserts into the read model, and stamps `projected_at`.
pub struct DispatchJobProjectionService {
    pool: PgPool,
    batch_size: u32,
    shutdown_tx: watch::Sender<bool>,
    shutdown_rx: watch::Receiver<bool>,
    health: Arc<StreamHealth>,
}

impl DispatchJobProjectionService {
    pub fn new(pool: PgPool, batch_size: u32) -> Self {
        let (shutdown_tx, shutdown_rx) = watch::channel(false);
        Self {
            pool,
            batch_size,
            shutdown_tx,
            shutdown_rx,
            health: Arc::new(StreamHealth::new("dispatch-job-projection".to_string())),
        }
    }

    /// Returns the health tracker for this service.
    pub fn health(&self) -> Arc<StreamHealth> {
        self.health.clone()
    }

    /// Starts the projection loop in a background tokio task.
    pub fn start(&self) -> tokio::task::JoinHandle<()> {
        let pool = self.pool.clone();
        let batch_size = self.batch_size;
        let mut shutdown_rx = self.shutdown_rx.clone();
        let health = self.health.clone();

        tokio::spawn(async move {
            health.set_running(true);
            info!(
                "Dispatch job projection started (batch_size={})",
                batch_size
            );

            loop {
                if *shutdown_rx.borrow() {
                    break;
                }

                let sleep_ms = match poll_once(&pool, batch_size).await {
                    Ok(count) => {
                        if count > 0 {
                            health.add_processed(count as u64);
                            debug!("Projected {} dispatch jobs", count);
                        }
                        adaptive_sleep(count, batch_size)
                    }
                    Err(e) => {
                        error!("Dispatch job projection error: {}", e);
                        health.record_error();
                        5000
                    }
                };

                if sleep_ms > 0 {
                    tokio::select! {
                        _ = tokio::time::sleep(Duration::from_millis(sleep_ms)) => {}
                        _ = shutdown_rx.changed() => { break; }
                    }
                }
            }

            health.set_running(false);
            info!("Dispatch job projection stopped");
        })
    }

    /// Signals the projection loop to stop.
    pub fn stop(&self) {
        let _ = self.shutdown_tx.send(true);
    }
}

async fn poll_once(pool: &PgPool, batch_size: u32) -> anyhow::Result<u32> {
    // Pick up dispatch jobs that are new (projected_at IS NULL) or updated
    // since last projection (updated_at > projected_at). Upsert into read
    // model and stamp projected_at.
    let rows = sqlx::query_as::<_, (i32,)>(
        r#"
        WITH batch AS (
            SELECT id, created_at
            FROM msg_dispatch_jobs
            WHERE projected_at IS NULL
               OR updated_at > projected_at
            ORDER BY created_at
            LIMIT $1
        ),
        projected AS (
            INSERT INTO msg_dispatch_jobs_read (
                id, external_id, source, kind, code, subject, event_id, correlation_id,
                target_url, protocol, service_account_id, client_id, subscription_id,
                mode, dispatch_pool_id, message_group, sequence, timeout_seconds,
                status, max_retries, retry_strategy, scheduled_for, expires_at,
                attempt_count, last_attempt_at, completed_at, duration_millis, last_error,
                idempotency_key, is_completed, is_terminal,
                application, subdomain, aggregate,
                created_at, updated_at, projected_at
            )
            SELECT
                j.id, j.external_id, j.source, j.kind, j.code, j.subject,
                j.event_id, j.correlation_id, j.target_url, j.protocol,
                j.service_account_id, j.client_id, j.subscription_id,
                j.mode, j.dispatch_pool_id, j.message_group,
                j.sequence, j.timeout_seconds, j.status,
                j.max_retries, j.retry_strategy,
                j.scheduled_for, j.expires_at,
                j.attempt_count, j.last_attempt_at, j.completed_at,
                j.duration_millis, j.last_error, j.idempotency_key,
                j.status IN ('SUCCESS', 'FAILED', 'IGNORED', 'CANCELLED', 'EXPIRED') AS is_completed,
                j.status IN ('FAILED', 'IGNORED', 'CANCELLED', 'EXPIRED') AS is_terminal,
                split_part(j.code, ':', 1),
                NULLIF(split_part(j.code, ':', 2), ''),
                NULLIF(split_part(j.code, ':', 3), ''),
                j.created_at, j.updated_at, NOW()
            FROM msg_dispatch_jobs j
            JOIN batch b ON b.id = j.id AND b.created_at = j.created_at
            ON CONFLICT (id, created_at) DO UPDATE SET
                status = EXCLUDED.status,
                attempt_count = EXCLUDED.attempt_count,
                last_attempt_at = EXCLUDED.last_attempt_at,
                completed_at = EXCLUDED.completed_at,
                duration_millis = EXCLUDED.duration_millis,
                last_error = EXCLUDED.last_error,
                is_completed = EXCLUDED.is_completed,
                is_terminal = EXCLUDED.is_terminal,
                updated_at = EXCLUDED.updated_at,
                projected_at = NOW()
        )
        UPDATE msg_dispatch_jobs m
        SET projected_at = NOW()
        FROM batch b
        WHERE m.id = b.id AND m.created_at = b.created_at
        RETURNING 1
        "#,
    )
    .bind(batch_size as i64)
    .fetch_all(pool)
    .await
    .map_err(|e| anyhow::anyhow!("dispatch job projection query failed: {}", e))?;

    Ok(rows.len() as u32)
}

/// Returns how long to sleep (ms) based on how many rows were processed.
fn adaptive_sleep(count: u32, batch_size: u32) -> u64 {
    if count >= batch_size {
        0
    } else if count > 0 {
        100
    } else {
        1000
    }
}

#[cfg(test)]
mod tests {
    use super::*;

    // --- adaptive_sleep tests ---

    #[test]
    fn adaptive_sleep_idle_when_no_rows() {
        assert_eq!(adaptive_sleep(0, 100), 1000);
        assert_eq!(adaptive_sleep(0, 1), 1000);
        assert_eq!(adaptive_sleep(0, 500), 1000);
    }

    #[test]
    fn adaptive_sleep_short_when_partial_batch() {
        assert_eq!(adaptive_sleep(1, 100), 100);
        assert_eq!(adaptive_sleep(50, 100), 100);
        assert_eq!(adaptive_sleep(99, 100), 100);
    }

    #[test]
    fn adaptive_sleep_immediate_when_full_batch() {
        assert_eq!(adaptive_sleep(100, 100), 0);
        assert_eq!(adaptive_sleep(200, 100), 0);
        assert_eq!(adaptive_sleep(1, 1), 0);
    }

    #[test]
    fn adaptive_sleep_boundary_at_batch_size() {
        let batch = 50;
        assert_eq!(adaptive_sleep(batch - 1, batch), 100);
        assert_eq!(adaptive_sleep(batch, batch), 0);
        assert_eq!(adaptive_sleep(batch + 1, batch), 0);
    }

    // --- Service construction tests ---

    #[tokio::test]
    async fn service_health_has_correct_name() {
        let pool = sqlx::PgPool::connect_lazy("postgres://localhost/fake").unwrap();
        let svc = DispatchJobProjectionService::new(pool, 200);
        assert_eq!(svc.health().name(), "dispatch-job-projection");
    }

    #[tokio::test]
    async fn service_health_starts_not_running() {
        let pool = sqlx::PgPool::connect_lazy("postgres://localhost/fake").unwrap();
        let svc = DispatchJobProjectionService::new(pool, 100);
        assert!(!svc.health().is_running());
        assert!(!svc.health().is_healthy());
    }

    #[tokio::test]
    async fn service_stop_signals_shutdown() {
        let pool = sqlx::PgPool::connect_lazy("postgres://localhost/fake").unwrap();
        let svc = DispatchJobProjectionService::new(pool, 100);

        assert!(!*svc.shutdown_rx.borrow());
        svc.stop();
        assert!(*svc.shutdown_rx.borrow());
    }
}
