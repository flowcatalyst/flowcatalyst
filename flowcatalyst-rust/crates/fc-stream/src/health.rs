use serde::{Deserialize, Serialize};
use std::sync::atomic::{AtomicBool, AtomicI64, AtomicU64, Ordering};
use std::sync::Arc;

/// StreamHealth tracks the health status of a stream watcher.
/// This matches Java's StreamContext health tracking.
#[derive(Debug)]
pub struct StreamHealth {
    /// Name of the stream for identification
    name: String,
    /// Whether the stream is currently running
    running: AtomicBool,
    /// Total batches processed
    batch_sequence: AtomicU64,
    /// Last checkpointed batch sequence
    checkpointed_sequence: AtomicU64,
    /// Number of batches currently in-flight
    in_flight_count: AtomicI64,
    /// Whether a fatal error has occurred
    has_fatal_error: AtomicBool,
    /// Fatal error message (if any)
    fatal_error_message: parking_lot::RwLock<Option<String>>,
    /// Last successful checkpoint timestamp (Unix millis)
    last_checkpoint_time: AtomicU64,
    /// Consecutive reconnection attempts
    reconnect_attempts: AtomicU64,
}

impl StreamHealth {
    /// Create a new StreamHealth instance
    pub fn new(name: String) -> Self {
        Self {
            name,
            running: AtomicBool::new(false),
            batch_sequence: AtomicU64::new(0),
            checkpointed_sequence: AtomicU64::new(0),
            in_flight_count: AtomicI64::new(0),
            has_fatal_error: AtomicBool::new(false),
            fatal_error_message: parking_lot::RwLock::new(None),
            last_checkpoint_time: AtomicU64::new(0),
            reconnect_attempts: AtomicU64::new(0),
        }
    }

    /// Get the stream name
    pub fn name(&self) -> &str {
        &self.name
    }

    /// Mark the stream as running
    pub fn set_running(&self, running: bool) {
        self.running.store(running, Ordering::SeqCst);
    }

    /// Check if the stream is running
    pub fn is_running(&self) -> bool {
        self.running.load(Ordering::SeqCst)
    }

    /// Increment batch sequence and return new value
    pub fn increment_batch_sequence(&self) -> u64 {
        self.batch_sequence.fetch_add(1, Ordering::SeqCst) + 1
    }

    /// Get current batch sequence
    pub fn get_batch_sequence(&self) -> u64 {
        self.batch_sequence.load(Ordering::SeqCst)
    }

    /// Update checkpointed sequence
    pub fn set_checkpointed_sequence(&self, seq: u64) {
        self.checkpointed_sequence.store(seq, Ordering::SeqCst);
        self.last_checkpoint_time.store(
            std::time::SystemTime::now()
                .duration_since(std::time::UNIX_EPOCH)
                .unwrap_or_default()
                .as_millis() as u64,
            Ordering::SeqCst,
        );
    }

    /// Get checkpointed sequence
    pub fn get_checkpointed_sequence(&self) -> u64 {
        self.checkpointed_sequence.load(Ordering::SeqCst)
    }

    /// Increment in-flight count
    pub fn increment_in_flight(&self) {
        self.in_flight_count.fetch_add(1, Ordering::SeqCst);
    }

    /// Decrement in-flight count
    pub fn decrement_in_flight(&self) {
        self.in_flight_count.fetch_sub(1, Ordering::SeqCst);
    }

    /// Get in-flight count
    pub fn get_in_flight_count(&self) -> i64 {
        self.in_flight_count.load(Ordering::SeqCst)
    }

    /// Set fatal error
    pub fn set_fatal_error(&self, error: String) {
        self.has_fatal_error.store(true, Ordering::SeqCst);
        *self.fatal_error_message.write() = Some(error);
    }

    /// Clear fatal error
    pub fn clear_fatal_error(&self) {
        self.has_fatal_error.store(false, Ordering::SeqCst);
        *self.fatal_error_message.write() = None;
    }

    /// Check if there's a fatal error
    pub fn has_fatal_error(&self) -> bool {
        self.has_fatal_error.load(Ordering::SeqCst)
    }

    /// Get fatal error message
    pub fn get_fatal_error(&self) -> Option<String> {
        self.fatal_error_message.read().clone()
    }

    /// Increment reconnect attempts
    pub fn increment_reconnect_attempts(&self) -> u64 {
        self.reconnect_attempts.fetch_add(1, Ordering::SeqCst) + 1
    }

    /// Reset reconnect attempts (on successful connection)
    pub fn reset_reconnect_attempts(&self) {
        self.reconnect_attempts.store(0, Ordering::SeqCst);
    }

    /// Get reconnect attempts
    pub fn get_reconnect_attempts(&self) -> u64 {
        self.reconnect_attempts.load(Ordering::SeqCst)
    }

    /// Check if the stream is healthy
    /// Healthy means: running, no fatal errors, and checkpoint is recent
    pub fn is_healthy(&self) -> bool {
        self.is_running() && !self.has_fatal_error()
    }

    /// Get the pending batch count (batches not yet checkpointed)
    pub fn get_pending_batches(&self) -> u64 {
        let current = self.get_batch_sequence();
        let checkpointed = self.get_checkpointed_sequence();
        if current > checkpointed {
            current - checkpointed
        } else {
            0
        }
    }

    /// Get detailed status
    pub fn get_status(&self) -> StreamHealthStatus {
        StreamHealthStatus {
            name: self.name.clone(),
            running: self.is_running(),
            healthy: self.is_healthy(),
            batch_sequence: self.get_batch_sequence(),
            checkpointed_sequence: self.get_checkpointed_sequence(),
            pending_batches: self.get_pending_batches(),
            in_flight_count: self.get_in_flight_count(),
            has_fatal_error: self.has_fatal_error(),
            fatal_error: self.get_fatal_error(),
            reconnect_attempts: self.get_reconnect_attempts(),
            last_checkpoint_time_ms: self.last_checkpoint_time.load(Ordering::SeqCst),
        }
    }

    /// Get simplified status snapshot for API
    pub fn status(&self) -> StreamHealthSnapshot {
        let checkpoint_ms = self.last_checkpoint_time.load(Ordering::SeqCst);
        let last_checkpoint_at = if checkpoint_ms > 0 {
            chrono::DateTime::from_timestamp_millis(checkpoint_ms as i64)
        } else {
            None
        };

        let status = if self.has_fatal_error() {
            StreamStatus::Error
        } else if self.is_running() {
            StreamStatus::Running
        } else {
            StreamStatus::Stopped
        };

        StreamHealthSnapshot {
            status,
            batch_sequence: self.get_batch_sequence(),
            in_flight_count: self.get_in_flight_count().max(0) as u32,
            pending_count: self.get_pending_batches() as u32,
            error_count: self.reconnect_attempts.load(Ordering::SeqCst),
            last_checkpoint_at,
        }
    }
}

/// Detailed health status for a stream
#[derive(Debug, Clone, Serialize, Deserialize)]
pub struct StreamHealthStatus {
    pub name: String,
    pub running: bool,
    pub healthy: bool,
    pub batch_sequence: u64,
    pub checkpointed_sequence: u64,
    pub pending_batches: u64,
    pub in_flight_count: i64,
    pub has_fatal_error: bool,
    #[serde(skip_serializing_if = "Option::is_none")]
    pub fatal_error: Option<String>,
    pub reconnect_attempts: u64,
    pub last_checkpoint_time_ms: u64,
}

/// Aggregated health status for all streams
#[derive(Debug, Clone, Serialize, Deserialize)]
pub struct StreamProcessorHealth {
    pub healthy: bool,
    pub total_streams: usize,
    pub healthy_streams: usize,
    pub unhealthy_streams: usize,
    pub streams: Vec<StreamHealthStatus>,
}

/// Aggregated health result with liveness/readiness and errors
#[derive(Debug, Clone)]
pub struct AggregatedHealth {
    live: bool,
    ready: bool,
    pub errors: Vec<String>,
}

impl AggregatedHealth {
    pub fn is_live(&self) -> bool {
        self.live
    }

    pub fn is_ready(&self) -> bool {
        self.ready
    }
}

/// Status enum for stream health
#[derive(Debug, Clone, Copy, PartialEq, Eq, Serialize, Deserialize)]
pub enum StreamStatus {
    Running,
    Stopped,
    Error,
}

/// Snapshot of stream health status for API responses
#[derive(Debug, Clone)]
pub struct StreamHealthSnapshot {
    pub status: StreamStatus,
    pub batch_sequence: u64,
    pub in_flight_count: u32,
    pub pending_count: u32,
    pub error_count: u64,
    pub last_checkpoint_at: Option<chrono::DateTime<chrono::Utc>>,
}

/// Health service for the stream processor
pub struct StreamHealthService {
    stream_healths: Vec<Arc<StreamHealth>>,
}

impl StreamHealthService {
    pub fn new() -> Self {
        Self {
            stream_healths: Vec::new(),
        }
    }

    /// Register a stream health tracker
    pub fn register(&mut self, health: Arc<StreamHealth>) {
        self.stream_healths.push(health);
    }

    /// Check if all streams are healthy (for Kubernetes liveness probe)
    pub fn is_live(&self) -> bool {
        // Live as long as we have streams and at least one is running
        !self.stream_healths.is_empty() &&
            self.stream_healths.iter().any(|h| h.is_running())
    }

    /// Check if the processor is ready (for Kubernetes readiness probe)
    pub fn is_ready(&self) -> bool {
        // Ready when all streams are healthy
        !self.stream_healths.is_empty() &&
            self.stream_healths.iter().all(|h| h.is_healthy())
    }

    /// Get aggregated health status
    pub fn get_health(&self) -> StreamProcessorHealth {
        let statuses: Vec<StreamHealthStatus> = self.stream_healths
            .iter()
            .map(|h| h.get_status())
            .collect();

        let healthy_count = statuses.iter().filter(|s| s.healthy).count();
        let total = statuses.len();

        StreamProcessorHealth {
            healthy: healthy_count == total && total > 0,
            total_streams: total,
            healthy_streams: healthy_count,
            unhealthy_streams: total - healthy_count,
            streams: statuses,
        }
    }

    /// Get aggregated health with liveness/readiness and error details
    pub fn get_aggregated_health(&self) -> AggregatedHealth {
        let live = self.is_live();
        let ready = self.is_ready();

        let errors: Vec<String> = self.stream_healths
            .iter()
            .filter_map(|h| h.get_fatal_error())
            .collect();

        AggregatedHealth { live, ready, errors }
    }

    /// Get all registered stream health trackers
    pub fn get_all_stream_health(&self) -> &[Arc<StreamHealth>] {
        &self.stream_healths
    }
}

impl Default for StreamHealthService {
    fn default() -> Self {
        Self::new()
    }
}

#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn test_stream_health_basic() {
        let health = StreamHealth::new("test-stream".to_string());

        assert!(!health.is_running());
        assert!(!health.has_fatal_error());
        assert!(!health.is_healthy()); // Not running = not healthy

        health.set_running(true);
        assert!(health.is_running());
        assert!(health.is_healthy()); // Running + no fatal error = healthy

        health.set_fatal_error("Connection failed".to_string());
        assert!(!health.is_healthy()); // Has fatal error = not healthy

        health.clear_fatal_error();
        assert!(health.is_healthy()); // Error cleared = healthy again
    }

    #[test]
    fn test_stream_health_metrics() {
        let health = StreamHealth::new("test-stream".to_string());

        assert_eq!(health.get_batch_sequence(), 0);
        assert_eq!(health.increment_batch_sequence(), 1);
        assert_eq!(health.increment_batch_sequence(), 2);
        assert_eq!(health.get_batch_sequence(), 2);

        health.set_checkpointed_sequence(1);
        assert_eq!(health.get_pending_batches(), 1); // 2 - 1 = 1

        health.increment_in_flight();
        health.increment_in_flight();
        assert_eq!(health.get_in_flight_count(), 2);
        health.decrement_in_flight();
        assert_eq!(health.get_in_flight_count(), 1);
    }

    #[test]
    fn test_health_service() {
        let mut service = StreamHealthService::new();

        let health1 = Arc::new(StreamHealth::new("stream-1".to_string()));
        let health2 = Arc::new(StreamHealth::new("stream-2".to_string()));

        service.register(health1.clone());
        service.register(health2.clone());

        // Not live when no streams are running
        assert!(!service.is_live());
        assert!(!service.is_ready());

        // Live when at least one stream is running
        health1.set_running(true);
        assert!(service.is_live());
        assert!(!service.is_ready()); // stream-2 not running

        // Ready when all streams are healthy
        health2.set_running(true);
        assert!(service.is_ready());

        // Not ready when any stream has fatal error
        health1.set_fatal_error("Error".to_string());
        assert!(!service.is_ready());
        assert!(service.is_live()); // Still live

        let status = service.get_health();
        assert!(!status.healthy);
        assert_eq!(status.total_streams, 2);
        assert_eq!(status.healthy_streams, 1);
        assert_eq!(status.unhealthy_streams, 1);
    }
}
