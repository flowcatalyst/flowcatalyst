use std::sync::Arc;
use std::time::Duration;

use sqlx::PgPool;
use tokio::sync::watch;
use tracing::{debug, error, info};

use crate::health::StreamHealth;

/// Projects events from `msg_events` into `msg_events_read`.
///
/// Reads rows where `projected_at IS NULL`, inserts them into the read model
/// with parsed application/subdomain/aggregate fields, and stamps `projected_at`.
pub struct EventProjectionService {
    pool: PgPool,
    batch_size: u32,
    shutdown_tx: watch::Sender<bool>,
    shutdown_rx: watch::Receiver<bool>,
    health: Arc<StreamHealth>,
}

impl EventProjectionService {
    pub fn new(pool: PgPool, batch_size: u32) -> Self {
        let (shutdown_tx, shutdown_rx) = watch::channel(false);
        Self {
            pool,
            batch_size,
            shutdown_tx,
            shutdown_rx,
            health: Arc::new(StreamHealth::new("event-projection".to_string())),
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
                "Event projection started (batch_size={})",
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
                            debug!("Projected {} events", count);
                        }
                        adaptive_sleep(count, batch_size)
                    }
                    Err(e) => {
                        error!("Event projection error: {}", e);
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
            info!("Event projection stopped");
        })
    }

    /// Signals the projection loop to stop.
    pub fn stop(&self) {
        let _ = self.shutdown_tx.send(true);
    }
}

async fn poll_once(pool: &PgPool, batch_size: u32) -> anyhow::Result<u32> {
    // Select unprojected events, insert into read model, and stamp projected_at
    // in a single atomic CTE. The RETURNING clause gives us the count.
    let rows = sqlx::query_as::<_, (i32,)>(
        r#"
        WITH batch AS (
            SELECT id
            FROM msg_events
            WHERE projected_at IS NULL
            ORDER BY created_at
            LIMIT $1
        ),
        projected AS (
            INSERT INTO msg_events_read (
                id, spec_version, type, source, subject, time, data,
                correlation_id, causation_id, deduplication_id, message_group,
                client_id, application, subdomain, aggregate, projected_at
            )
            SELECT
                e.id,
                e.spec_version,
                e.type,
                e.source,
                e.subject,
                e.time,
                e.data::text,
                e.correlation_id,
                e.causation_id,
                e.deduplication_id,
                e.message_group,
                e.client_id,
                split_part(e.type, ':', 1),
                NULLIF(split_part(e.type, ':', 2), ''),
                NULLIF(split_part(e.type, ':', 3), ''),
                NOW()
            FROM msg_events e
            JOIN batch b ON b.id = e.id
            ON CONFLICT (id) DO NOTHING
        )
        UPDATE msg_events
        SET projected_at = NOW()
        WHERE id IN (SELECT id FROM batch)
        RETURNING 1
        "#,
    )
    .bind(batch_size as i64)
    .fetch_all(pool)
    .await
    .map_err(|e| anyhow::anyhow!("event projection query failed: {}", e))?;

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
        // count == 0 → long sleep (1000ms)
        assert_eq!(adaptive_sleep(0, 100), 1000);
        assert_eq!(adaptive_sleep(0, 1), 1000);
        assert_eq!(adaptive_sleep(0, 500), 1000);
    }

    #[test]
    fn adaptive_sleep_short_when_partial_batch() {
        // 0 < count < batch_size → short sleep (100ms)
        assert_eq!(adaptive_sleep(1, 100), 100);
        assert_eq!(adaptive_sleep(50, 100), 100);
        assert_eq!(adaptive_sleep(99, 100), 100);
    }

    #[test]
    fn adaptive_sleep_immediate_when_full_batch() {
        // count >= batch_size → no sleep (0ms), more rows likely waiting
        assert_eq!(adaptive_sleep(100, 100), 0);
        assert_eq!(adaptive_sleep(200, 100), 0); // over batch_size
        assert_eq!(adaptive_sleep(1, 1), 0);     // exactly batch_size of 1
    }

    #[test]
    fn adaptive_sleep_boundary_at_batch_size() {
        let batch = 50;
        assert_eq!(adaptive_sleep(batch - 1, batch), 100); // just under
        assert_eq!(adaptive_sleep(batch, batch), 0);        // exactly at
        assert_eq!(adaptive_sleep(batch + 1, batch), 0);    // just over
    }

    // --- Service construction tests ---

    #[tokio::test]
    async fn service_health_has_correct_name() {
        let pool = sqlx::PgPool::connect_lazy("postgres://localhost/fake").unwrap();
        let svc = EventProjectionService::new(pool, 200);
        assert_eq!(svc.health().name(), "event-projection");
    }

    #[tokio::test]
    async fn service_health_starts_not_running() {
        let pool = sqlx::PgPool::connect_lazy("postgres://localhost/fake").unwrap();
        let svc = EventProjectionService::new(pool, 100);
        assert!(!svc.health().is_running());
        assert!(!svc.health().is_healthy());
    }

    #[tokio::test]
    async fn service_stop_signals_shutdown() {
        let pool = sqlx::PgPool::connect_lazy("postgres://localhost/fake").unwrap();
        let svc = EventProjectionService::new(pool, 100);

        // Before stop, shutdown_rx should be false
        assert!(!*svc.shutdown_rx.borrow());

        svc.stop();

        // After stop, shutdown_rx should be true
        assert!(*svc.shutdown_rx.borrow());
    }
}
