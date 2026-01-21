//! Checkpoint Tracker
//!
//! Tracks in-flight batch processing and manages checkpoint advancement.
//! Batches are processed concurrently, but checkpoints only advance when all
//! prior batches have completed. This ensures no events are skipped on restart.

use std::collections::{BTreeMap, HashSet};
use std::sync::Arc;
use tokio::sync::Mutex;
use mongodb::change_stream::event::ResumeToken;
use tracing::{debug, error};

use crate::checkpoint::CheckpointStore;

/// Result of a batch operation
#[derive(Debug, Clone)]
pub struct BatchResult {
    pub resume_token: Option<ResumeToken>,
    pub success: bool,
    pub error: Option<String>,
}

/// Tracks in-flight batches and manages checkpoints
pub struct CheckpointTracker {
    checkpoint_store: Arc<dyn CheckpointStore>,
    stream_name: String,
    checkpoint_key: String,
    state: Mutex<TrackerState>,
}

struct TrackerState {
    /// Completed batches awaiting checkpoint advancement
    batches: BTreeMap<u64, BatchResult>,
    /// Last checkpointed sequence number
    last_checkpointed_seq: u64,
    /// Current batch sequence counter
    next_seq: u64,
    /// Fatal error if one occurred
    fatal_error: Option<String>,
}

impl CheckpointTracker {
    pub fn new(
        checkpoint_store: Arc<dyn CheckpointStore>,
        stream_name: String,
        checkpoint_key: String,
    ) -> Self {
        Self {
            checkpoint_store,
            stream_name,
            checkpoint_key,
            state: Mutex::new(TrackerState {
                batches: BTreeMap::new(),
                last_checkpointed_seq: 0,
                next_seq: 1,
                fatal_error: None,
            }),
        }
    }

    /// Get the next batch sequence number
    pub async fn next_sequence(&self) -> u64 {
        let mut state = self.state.lock().await;
        let seq = state.next_seq;
        state.next_seq += 1;
        seq
    }

    /// Mark a batch as successfully completed
    pub async fn mark_complete(&self, seq: u64, resume_token: Option<ResumeToken>) {
        let mut state = self.state.lock().await;
        state.batches.insert(seq, BatchResult {
            resume_token,
            success: true,
            error: None,
        });
        drop(state);

        self.advance_checkpoint().await;
    }

    /// Mark a batch as failed
    pub async fn mark_failed(&self, seq: u64, error: String) {
        let mut state = self.state.lock().await;
        state.batches.insert(seq, BatchResult {
            resume_token: None,
            success: false,
            error: Some(error.clone()),
        });
        state.fatal_error = Some(error);
    }

    /// Check if a fatal error has occurred
    pub async fn has_fatal_error(&self) -> bool {
        let state = self.state.lock().await;
        state.fatal_error.is_some()
    }

    /// Get the fatal error if one occurred
    pub async fn get_fatal_error(&self) -> Option<String> {
        let state = self.state.lock().await;
        state.fatal_error.clone()
    }

    /// Get the number of in-flight batches
    pub async fn in_flight_count(&self) -> usize {
        let state = self.state.lock().await;
        state.batches.len()
    }

    /// Get the last checkpointed sequence number
    pub async fn last_checkpointed_seq(&self) -> u64 {
        let state = self.state.lock().await;
        state.last_checkpointed_seq
    }

    /// Advance checkpoint to highest contiguous completed batch
    async fn advance_checkpoint(&self) {
        let mut state = self.state.lock().await;

        while let Some(result) = state.batches.get(&(state.last_checkpointed_seq + 1)) {
            if !result.success {
                break; // Stop at failed batch
            }

            let seq = state.last_checkpointed_seq + 1;
            let result = state.batches.remove(&seq).unwrap();
            state.last_checkpointed_seq = seq;

            // Persist checkpoint
            if let Some(token) = result.resume_token {
                if let Ok(token_doc) = mongodb::bson::to_document(&token) {
                    if let Err(e) = self.checkpoint_store.save_checkpoint(&self.checkpoint_key, token_doc).await {
                        error!("[{}] Failed to save checkpoint: {}", self.stream_name, e);
                    } else {
                        debug!("[{}] Checkpoint advanced to batch {}", self.stream_name, seq);
                    }
                }
            }
        }
    }

    /// Reset state (for testing)
    pub async fn reset(&self) {
        let mut state = self.state.lock().await;
        state.batches.clear();
        state.last_checkpointed_seq = 0;
        state.next_seq = 1;
        state.fatal_error = None;
    }
}

/// Tracks which aggregate IDs are currently in-flight to prevent concurrent processing
pub struct AggregateTracker {
    #[allow(dead_code)] // Reserved for future logging/debugging
    stream_name: String,
    state: Mutex<AggregateState>,
}

struct AggregateState {
    /// Aggregate IDs currently being processed (batch_seq -> set of aggregate IDs)
    in_flight: BTreeMap<u64, HashSet<String>>,
    /// Documents waiting for their aggregate to be free
    pending: Vec<PendingDocument>,
}

#[derive(Debug, Clone)]
pub struct PendingDocument {
    pub aggregate_id: String,
    pub document: mongodb::bson::Document,
    pub resume_token: Option<ResumeToken>,
}

impl AggregateTracker {
    pub fn new(stream_name: String) -> Self {
        Self {
            stream_name,
            state: Mutex::new(AggregateState {
                in_flight: BTreeMap::new(),
                pending: Vec::new(),
            }),
        }
    }

    /// Check if an aggregate ID is currently in-flight
    pub async fn is_in_flight(&self, aggregate_id: &str) -> bool {
        let state = self.state.lock().await;
        state.in_flight.values().any(|ids| ids.contains(aggregate_id))
    }

    /// Register aggregate IDs for a batch
    pub async fn register_batch(&self, batch_seq: u64, aggregate_ids: HashSet<String>) {
        let mut state = self.state.lock().await;
        state.in_flight.insert(batch_seq, aggregate_ids);
    }

    /// Mark a batch as complete, releasing its aggregate IDs
    pub async fn complete_batch(&self, batch_seq: u64) -> Vec<PendingDocument> {
        let mut state = self.state.lock().await;
        state.in_flight.remove(&batch_seq);

        // Check if any pending documents can now be released
        let mut ready = Vec::new();
        let mut still_pending = Vec::new();

        // First drain all pending documents
        let pending_docs: Vec<_> = state.pending.drain(..).collect();

        // Now check each one against the remaining in_flight
        for doc in pending_docs {
            let is_blocked = state.in_flight.values().any(|ids| ids.contains(&doc.aggregate_id));
            if is_blocked {
                still_pending.push(doc);
            } else {
                ready.push(doc);
            }
        }

        state.pending = still_pending;
        ready
    }

    /// Add a document to the pending queue (blocked by in-flight aggregate)
    pub async fn add_pending(&self, doc: PendingDocument) {
        let mut state = self.state.lock().await;
        state.pending.push(doc);
    }

    /// Get count of pending documents
    pub async fn pending_count(&self) -> usize {
        let state = self.state.lock().await;
        state.pending.len()
    }

    /// Get count of in-flight batches
    pub async fn in_flight_batch_count(&self) -> usize {
        let state = self.state.lock().await;
        state.in_flight.len()
    }
}

#[cfg(test)]
mod tests {
    use super::*;
    use crate::checkpoint::MemoryCheckpointStore;

    #[tokio::test]
    async fn test_checkpoint_tracker_contiguous_completion() {
        let store = Arc::new(MemoryCheckpointStore::new());
        let tracker = CheckpointTracker::new(
            store.clone(),
            "test".to_string(),
            "test-checkpoint".to_string(),
        );

        // Get sequences for 3 batches
        let seq1 = tracker.next_sequence().await;
        let seq2 = tracker.next_sequence().await;
        let seq3 = tracker.next_sequence().await;

        assert_eq!(seq1, 1);
        assert_eq!(seq2, 2);
        assert_eq!(seq3, 3);

        // Complete batch 3 first - checkpoint should not advance
        tracker.mark_complete(seq3, None).await;
        assert_eq!(tracker.last_checkpointed_seq().await, 0);

        // Complete batch 1 - checkpoint should advance to 1
        tracker.mark_complete(seq1, None).await;
        assert_eq!(tracker.last_checkpointed_seq().await, 1);

        // Complete batch 2 - checkpoint should advance to 3
        tracker.mark_complete(seq2, None).await;
        assert_eq!(tracker.last_checkpointed_seq().await, 3);
    }

    #[tokio::test]
    async fn test_aggregate_tracker_blocks_duplicates() {
        let tracker = AggregateTracker::new("test".to_string());

        // Register batch 1 with aggregate "agg-1"
        let mut ids = HashSet::new();
        ids.insert("agg-1".to_string());
        tracker.register_batch(1, ids).await;

        // Check that "agg-1" is in-flight
        assert!(tracker.is_in_flight("agg-1").await);
        assert!(!tracker.is_in_flight("agg-2").await);

        // Complete batch 1
        let released = tracker.complete_batch(1).await;
        assert!(released.is_empty());

        // Now "agg-1" should be free
        assert!(!tracker.is_in_flight("agg-1").await);
    }

    #[tokio::test]
    async fn test_aggregate_tracker_pending_release() {
        let tracker = AggregateTracker::new("test".to_string());

        // Register batch 1 with aggregate "agg-1"
        let mut ids = HashSet::new();
        ids.insert("agg-1".to_string());
        tracker.register_batch(1, ids).await;

        // Add pending document for "agg-1"
        tracker.add_pending(PendingDocument {
            aggregate_id: "agg-1".to_string(),
            document: mongodb::bson::doc! { "test": 1 },
            resume_token: None,
        }).await;

        assert_eq!(tracker.pending_count().await, 1);

        // Complete batch 1 - pending doc should be released
        let released = tracker.complete_batch(1).await;
        assert_eq!(released.len(), 1);
        assert_eq!(released[0].aggregate_id, "agg-1");
        assert_eq!(tracker.pending_count().await, 0);
    }
}
