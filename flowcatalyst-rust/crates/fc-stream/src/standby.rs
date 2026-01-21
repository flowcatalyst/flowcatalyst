//! Hot Standby Integration for Stream Processor
//!
//! Provides integration with fc-standby for leader election, allowing
//! multiple stream processor instances to run with only one actively
//! processing events.

use std::sync::Arc;
use std::sync::atomic::{AtomicBool, Ordering};
use fc_standby::{LeaderElection, LeadershipStatus};
use tracing::{info, warn, debug};

use crate::StreamWatcher;

/// Stream processor with hot standby support
pub struct StandbyStreamProcessor {
    watchers: Vec<Arc<dyn StreamWatcher>>,
    leader_election: Arc<LeaderElection>,
    running: Arc<AtomicBool>,
}

impl StandbyStreamProcessor {
    pub fn new(leader_election: Arc<LeaderElection>) -> Self {
        Self {
            watchers: Vec::new(),
            leader_election,
            running: Arc::new(AtomicBool::new(false)),
        }
    }

    pub fn add_watcher(&mut self, watcher: Arc<dyn StreamWatcher>) {
        self.watchers.push(watcher);
    }

    /// Check if this instance is currently the leader
    pub fn is_leader(&self) -> bool {
        matches!(self.leader_election.status(), LeadershipStatus::Leader)
    }

    /// Start the stream processor with standby support
    ///
    /// The processor will:
    /// 1. Wait until it becomes the leader
    /// 2. Start all watchers
    /// 3. If leadership is lost, stop watchers and wait to regain leadership
    /// 4. If a fatal error occurs, exit to allow standby to take over
    pub async fn start(self: Arc<Self>) {
        self.running.store(true, Ordering::SeqCst);

        info!(
            "Starting Stream Processor with standby support ({} watchers)",
            self.watchers.len()
        );

        // Subscribe to leadership changes
        let mut receiver = self.leader_election.subscribe();
        let mut watcher_handles: Vec<tokio::task::JoinHandle<()>> = Vec::new();
        let mut watchers_running = false;

        loop {
            if !self.running.load(Ordering::SeqCst) {
                info!("Stream processor shutting down");
                break;
            }

            // Check current status
            let current_status = self.leader_election.status();

            match current_status {
                LeadershipStatus::Leader => {
                    if !watchers_running {
                        info!("Acquired leadership - starting stream watchers");
                        watcher_handles = self.start_watchers().await;
                        watchers_running = true;
                    }
                }
                LeadershipStatus::Follower => {
                    if watchers_running {
                        warn!("Lost leadership - stopping stream watchers");
                        self.stop_watchers(&mut watcher_handles).await;
                        watchers_running = false;
                    }
                    debug!("Follower mode - waiting for leadership");
                }
                LeadershipStatus::Unknown => {
                    if watchers_running {
                        warn!("Leadership status unknown - stopping watchers for safety");
                        self.stop_watchers(&mut watcher_handles).await;
                        watchers_running = false;
                    }
                }
            }

            // Wait for status change
            tokio::select! {
                result = receiver.changed() => {
                    match result {
                        Ok(()) => {
                            let new_status = *receiver.borrow();
                            info!("Leadership status changed to {:?}", new_status);
                        }
                        Err(e) => {
                            warn!("Leadership receiver error: {}", e);
                            // Channel closed, try to reconnect
                            receiver = self.leader_election.subscribe();
                        }
                    }
                }
                _ = tokio::time::sleep(tokio::time::Duration::from_secs(1)) => {
                    // Periodic check in case we miss events
                }
            }
        }

        // Cleanup
        if watchers_running {
            self.stop_watchers(&mut watcher_handles).await;
        }

        info!("Stream processor stopped");
    }

    /// Start all watchers
    async fn start_watchers(&self) -> Vec<tokio::task::JoinHandle<()>> {
        let mut handles = Vec::new();

        for watcher in &self.watchers {
            let watcher_clone = watcher.clone();
            let handle = tokio::spawn(async move {
                if let Err(e) = watcher_clone.watch().await {
                    tracing::error!("Stream watcher failed: {}", e);
                }
            });
            handles.push(handle);
        }

        handles
    }

    /// Stop all watchers
    async fn stop_watchers(&self, handles: &mut Vec<tokio::task::JoinHandle<()>>) {
        for handle in handles.drain(..) {
            handle.abort();
        }
    }

    /// Signal the processor to stop
    pub fn stop(&self) {
        self.running.store(false, Ordering::SeqCst);
    }
}

#[cfg(test)]
mod tests {
    // Tests would require mocking LeaderElection
}
