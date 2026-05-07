//! Event fan-out service.
//!
//! Polls `msg_events` for rows where `fanned_out_at IS NULL`, matches them
//! against the active subscription set, batch-creates dispatch jobs, and
//! stamps `fanned_out_at` so each event is processed at-least-once.
//!
//! Replaces the synchronous fan-out previously embedded in the
//! `POST /api/events/batch` request path. Trade-off: the request returns as
//! soon as the events are durably stored; dispatch creation lags by at most
//! the poll interval (~ms when busy, up to 1s at idle).
//!
//! Jobs are inserted with status `PENDING`. The scheduler is responsible for
//! transitioning `PENDING → QUEUED` (publishing to the real queue) — that
//! keeps publish ownership in one place and makes the fan-out service the
//! same shape regardless of which queue backend the deployment uses.
//!
//! At-least-once semantics: dispatch job insert and `fanned_out_at` stamp
//! happen in one transaction.
//!
//! Concurrency: uses `FOR UPDATE SKIP LOCKED` on the claim. Any number of
//! pollers can run safely against the same parent; only one will claim each
//! row. The fan-out can therefore be horizontally scaled if needed.

use std::sync::Arc;
use std::sync::atomic::{AtomicBool, AtomicU64, Ordering};
use std::time::Duration;

use chrono::{DateTime, Utc};
use sqlx::{PgPool, Postgres, Transaction};
use tokio::sync::watch;
use tracing::{debug, error, info, warn};

use crate::dispatch_job::entity::{DispatchJob, DispatchStatus};
use crate::dispatch_job::repository::DispatchJobRepository;
use crate::event::entity::Event;
use crate::subscription::entity::Subscription;
use crate::subscription::repository::SubscriptionRepository;

/// Configuration for the fan-out service.
#[derive(Debug, Clone)]
pub struct EventFanOutConfig {
    /// Max events claimed per poll cycle.
    pub batch_size: u32,
    /// How often to refresh the in-memory subscription cache from the DB.
    /// Subs change rarely; a short refresh keeps fan-out coherent without
    /// querying every poll.
    pub subscription_refresh: Duration,
}

/// Lightweight health tracker; mirrors fc-stream::StreamHealth but kept local
/// to avoid pulling fc-stream into fc-platform.
#[derive(Debug)]
pub struct FanOutHealth {
    running: AtomicBool,
    processed_events: AtomicU64,
    created_jobs: AtomicU64,
    error_count: AtomicU64,
}

impl FanOutHealth {
    fn new() -> Self {
        Self {
            running: AtomicBool::new(false),
            processed_events: AtomicU64::new(0),
            created_jobs: AtomicU64::new(0),
            error_count: AtomicU64::new(0),
        }
    }
    pub fn is_running(&self) -> bool { self.running.load(Ordering::SeqCst) }
    pub fn processed_events(&self) -> u64 { self.processed_events.load(Ordering::SeqCst) }
    pub fn created_jobs(&self) -> u64 { self.created_jobs.load(Ordering::SeqCst) }
    pub fn error_count(&self) -> u64 { self.error_count.load(Ordering::SeqCst) }
    fn set_running(&self, v: bool) { self.running.store(v, Ordering::SeqCst); }
    fn add_processed(&self, n: u64) { self.processed_events.fetch_add(n, Ordering::SeqCst); }
    fn add_jobs(&self, n: u64) { self.created_jobs.fetch_add(n, Ordering::SeqCst); }
    fn record_error(&self) { self.error_count.fetch_add(1, Ordering::SeqCst); }
}

pub struct EventFanOutService {
    pool: PgPool,
    subscription_repo: Arc<SubscriptionRepository>,
    dispatch_job_repo: Arc<DispatchJobRepository>,
    config: EventFanOutConfig,
    shutdown_tx: watch::Sender<bool>,
    shutdown_rx: watch::Receiver<bool>,
    health: Arc<FanOutHealth>,
}

impl EventFanOutService {
    pub fn new(
        pool: PgPool,
        subscription_repo: Arc<SubscriptionRepository>,
        dispatch_job_repo: Arc<DispatchJobRepository>,
        config: EventFanOutConfig,
    ) -> Self {
        let (shutdown_tx, shutdown_rx) = watch::channel(false);
        Self {
            pool,
            subscription_repo,
            dispatch_job_repo,
            config,
            shutdown_tx,
            shutdown_rx,
            health: Arc::new(FanOutHealth::new()),
        }
    }

    pub fn health(&self) -> Arc<FanOutHealth> {
        self.health.clone()
    }

    pub fn start(&self) -> tokio::task::JoinHandle<()> {
        let pool = self.pool.clone();
        let sub_repo = self.subscription_repo.clone();
        let job_repo = self.dispatch_job_repo.clone();
        let config = self.config.clone();
        let mut shutdown_rx = self.shutdown_rx.clone();
        let health = self.health.clone();

        tokio::spawn(async move {
            health.set_running(true);
            info!(batch_size = config.batch_size, "Event fan-out started");

            let mut subs_cache = SubscriptionCache::new(config.subscription_refresh);

            loop {
                if *shutdown_rx.borrow() {
                    break;
                }

                // Refresh the subscription cache if stale.
                if subs_cache.needs_refresh() {
                    match sub_repo.find_active().await {
                        Ok(subs) => subs_cache.replace(subs),
                        Err(e) => {
                            warn!(error = %e, "Failed to refresh subscriptions; using stale cache");
                        }
                    }
                }

                let sleep_ms = match poll_once(
                    &pool,
                    job_repo.as_ref(),
                    subs_cache.subs(),
                    &config,
                )
                .await
                {
                    Ok(report) => {
                        if report.events > 0 {
                            health.add_processed(report.events as u64);
                            health.add_jobs(report.jobs as u64);
                            debug!(events = report.events, jobs = report.jobs, "Fan-out cycle");
                        }
                        adaptive_sleep(report.events, config.batch_size)
                    }
                    Err(e) => {
                        error!(error = %e, "Event fan-out cycle failed");
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
            info!("Event fan-out stopped");
        })
    }

    pub fn stop(&self) {
        let _ = self.shutdown_tx.send(true);
    }
}

struct CycleReport {
    events: u32,
    jobs: u32,
}

/// One poll cycle: claim a batch of unfanned events, build dispatch jobs in
/// memory, persist jobs + stamp fanned_out_at in a single transaction. The
/// scheduler will pick the PENDING jobs up and handle the publish step.
async fn poll_once(
    pool: &PgPool,
    job_repo: &DispatchJobRepository,
    subscriptions: &[Subscription],
    config: &EventFanOutConfig,
) -> anyhow::Result<CycleReport> {
    if subscriptions.is_empty() {
        // Nothing to fan out to. Still claim and stamp events so they don't
        // accumulate forever in the partial index.
        let claimed = claim_events_no_subs(pool, config.batch_size).await?;
        return Ok(CycleReport { events: claimed, jobs: 0 });
    }

    let mut tx: Transaction<'_, Postgres> = pool.begin().await?;

    let claimed = claim_events(&mut tx, config.batch_size).await?;
    if claimed.is_empty() {
        tx.rollback().await.ok();
        return Ok(CycleReport { events: 0, jobs: 0 });
    }

    let mut dispatch_jobs: Vec<DispatchJob> = Vec::new();

    for event in &claimed {
        for sub in subscriptions {
            if !sub.matches_event_type(&event.event_type) {
                continue;
            }
            if !sub.matches_client(event.client_id.as_deref()) {
                continue;
            }

            let payload = serde_json::to_string(&event.data).unwrap_or_default();
            let mut job = DispatchJob::for_event(
                &event.id,
                &event.event_type,
                &event.source,
                &sub.endpoint,
                &payload,
            );

            // Status remains PENDING (the entity default). The scheduler
            // will transition PENDING → QUEUED and publish to the real queue.
            debug_assert_eq!(job.status, DispatchStatus::Pending);

            if let Some(s) = &event.subject {
                job.subject = Some(s.clone());
            }
            if let Some(c) = &event.correlation_id {
                job = job.with_correlation_id(c);
            }
            if let Some(g) = &event.message_group {
                job = job.with_message_group(g);
            }
            if let Some(c) = &event.client_id {
                job = job.with_client_id(c);
            }

            job = job
                .with_subscription_id(&sub.id)
                .with_mode(sub.mode)
                .with_data_only(sub.data_only);

            if let Some(pool_id) = &sub.dispatch_pool_id {
                job = job.with_dispatch_pool_id(pool_id);
            }
            if let Some(sa_id) = &sub.service_account_id {
                job = job.with_service_account_id(sa_id);
            }

            job.max_retries = sub.max_retries as u32;
            job.timeout_seconds = sub.timeout_seconds as u32;

            // Inherit the source event's created_at so the scheduler's
            // ORDER BY created_at preserves source order within a
            // message_group, and so events and their dispatch jobs land
            // in the same monthly partition.
            job.created_at = event.created_at;
            job.updated_at = event.created_at;

            dispatch_jobs.push(job);
        }
    }

    let job_count = dispatch_jobs.len();
    if !dispatch_jobs.is_empty() {
        job_repo.insert_many_tx(&mut tx, &dispatch_jobs).await?;
    }
    tx.commit().await?;

    Ok(CycleReport {
        events: claimed.len() as u32,
        jobs: job_count as u32,
    })
}

/// Claim a batch of events for fan-out. `UPDATE ... RETURNING` is the CAS:
/// any concurrent poller will see a flipped `fanned_out_at` and skip this row.
async fn claim_events(
    tx: &mut Transaction<'_, Postgres>,
    batch_size: u32,
) -> anyhow::Result<Vec<Event>> {
    let rows: Vec<EventClaimRow> = sqlx::query_as::<_, EventClaimRow>(
        r#"
        WITH batch AS (
            SELECT id, created_at
            FROM msg_events
            WHERE fanned_out_at IS NULL
            ORDER BY created_at
            LIMIT $1
            FOR UPDATE SKIP LOCKED
        )
        UPDATE msg_events e
        SET fanned_out_at = NOW()
        FROM batch b
        WHERE e.id = b.id AND e.created_at = b.created_at
        RETURNING
            e.id, e.spec_version, e.type, e.source, e.subject, e.time, e.data,
            e.correlation_id, e.causation_id, e.deduplication_id,
            e.message_group, e.client_id, e.context_data, e.created_at
        "#,
    )
    .bind(batch_size as i64)
    .fetch_all(&mut **tx)
    .await?;

    Ok(rows.into_iter().map(Event::from).collect())
}

/// No-subs fast path: stamp events as fanned-out without producing any jobs.
/// Avoids holding a transaction open when there's nothing to do.
async fn claim_events_no_subs(pool: &PgPool, batch_size: u32) -> anyhow::Result<u32> {
    let row: (i64,) = sqlx::query_as(
        r#"
        WITH batch AS (
            SELECT id, created_at
            FROM msg_events
            WHERE fanned_out_at IS NULL
            ORDER BY created_at
            LIMIT $1
        )
        UPDATE msg_events e
        SET fanned_out_at = NOW()
        FROM batch b
        WHERE e.id = b.id AND e.created_at = b.created_at
        RETURNING (SELECT COUNT(*) FROM batch)
        "#,
    )
    .bind(batch_size as i64)
    .fetch_optional(pool)
    .await?
    .unwrap_or((0,));
    Ok(row.0 as u32)
}

#[derive(sqlx::FromRow)]
struct EventClaimRow {
    id: String,
    spec_version: Option<String>,
    #[sqlx(rename = "type")]
    event_type: String,
    source: String,
    subject: Option<String>,
    time: DateTime<Utc>,
    data: Option<serde_json::Value>,
    correlation_id: Option<String>,
    causation_id: Option<String>,
    deduplication_id: Option<String>,
    message_group: Option<String>,
    client_id: Option<String>,
    context_data: Option<serde_json::Value>,
    created_at: DateTime<Utc>,
}

impl From<EventClaimRow> for Event {
    fn from(r: EventClaimRow) -> Self {
        let context_data = r
            .context_data
            .and_then(|v| serde_json::from_value(v).ok())
            .unwrap_or_default();

        Event {
            id: r.id,
            event_type: r.event_type,
            source: r.source,
            subject: r.subject,
            time: r.time,
            data: r.data.unwrap_or(serde_json::Value::Null),
            spec_version: r
                .spec_version
                .unwrap_or_else(|| crate::event::entity::CLOUDEVENTS_SPEC_VERSION.to_string()),
            message_group: r.message_group,
            correlation_id: r.correlation_id,
            causation_id: r.causation_id,
            deduplication_id: r.deduplication_id,
            client_id: r.client_id,
            context_data,
            created_at: r.created_at,
        }
    }
}

/// Tiny TTL cache for the active subscription set. Re-fetched periodically
/// rather than per-cycle to amortise the round-trip.
struct SubscriptionCache {
    subscriptions: Vec<Subscription>,
    last_refreshed: std::time::Instant,
    ttl: Duration,
}

impl SubscriptionCache {
    fn new(ttl: Duration) -> Self {
        Self {
            subscriptions: Vec::new(),
            // Force initial refresh.
            last_refreshed: std::time::Instant::now() - ttl - Duration::from_millis(1),
            ttl,
        }
    }
    fn needs_refresh(&self) -> bool {
        self.last_refreshed.elapsed() >= self.ttl
    }
    fn replace(&mut self, subs: Vec<Subscription>) {
        self.subscriptions = subs;
        self.last_refreshed = std::time::Instant::now();
    }
    fn subs(&self) -> &[Subscription] {
        &self.subscriptions
    }
}

/// Sleep duration based on cycle yield: 0 if the batch was full (more rows
/// likely waiting), 100ms for a partial batch, 1s when idle.
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

    #[test]
    fn adaptive_sleep_idle() {
        assert_eq!(adaptive_sleep(0, 100), 1000);
    }
    #[test]
    fn adaptive_sleep_partial() {
        assert_eq!(adaptive_sleep(50, 100), 100);
    }
    #[test]
    fn adaptive_sleep_full() {
        assert_eq!(adaptive_sleep(100, 100), 0);
        assert_eq!(adaptive_sleep(150, 100), 0);
    }
}
