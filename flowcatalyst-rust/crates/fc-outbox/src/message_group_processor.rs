//! Message Group Processor
//!
//! Handles FIFO ordering within a message group.
//! Messages with the same message_group_id are processed sequentially in batches.

use std::collections::VecDeque;
use std::sync::Arc;
use tokio::sync::{Mutex, oneshot};
use fc_common::Message;
use tracing::{debug, error, warn, info};
use async_trait::async_trait;

/// Message dispatch result
#[derive(Debug, Clone)]
pub enum DispatchResult {
    Success,
    Failure { error: String, retryable: bool },
    Blocked { reason: String },
}

/// Result for a single item in a batch
#[derive(Debug, Clone)]
pub struct BatchItemResult {
    pub message_id: String,
    pub result: DispatchResult,
}

/// Batch dispatch result
#[derive(Debug, Clone)]
pub struct BatchDispatchResult {
    pub results: Vec<BatchItemResult>,
}

impl BatchDispatchResult {
    /// Check if all items succeeded
    pub fn all_succeeded(&self) -> bool {
        self.results.iter().all(|r| matches!(r.result, DispatchResult::Success))
    }

    /// Get failed items
    pub fn failed_items(&self) -> Vec<&BatchItemResult> {
        self.results.iter()
            .filter(|r| !matches!(r.result, DispatchResult::Success))
            .collect()
    }
}

/// Message dispatcher trait for single messages
#[async_trait]
pub trait MessageDispatcher: Send + Sync {
    async fn dispatch(&self, message: &Message) -> DispatchResult;
}

/// Batch message dispatcher trait - dispatches multiple messages in one API call
#[async_trait]
pub trait BatchMessageDispatcher: Send + Sync {
    async fn dispatch_batch(&self, messages: &[Message]) -> BatchDispatchResult;
}

/// Configuration for message group processor
#[derive(Debug, Clone)]
pub struct MessageGroupProcessorConfig {
    /// Maximum queue depth before blocking
    pub max_queue_depth: usize,
    /// Whether to block on error (stops processing until resolved)
    pub block_on_error: bool,
    /// Maximum retry attempts before giving up
    pub max_retries: u32,
    /// Batch size for API calls (like Java's apiBatchSize)
    pub batch_size: usize,
}

impl Default for MessageGroupProcessorConfig {
    fn default() -> Self {
        Self {
            max_queue_depth: 1000,
            block_on_error: true,
            max_retries: 3,
            batch_size: 100,
        }
    }
}

/// State of the message group processor
#[derive(Debug, Clone, PartialEq)]
pub enum ProcessorState {
    /// Normal processing
    Running,
    /// Blocked due to error (waiting for resolution)
    Blocked { message_id: String, error: String },
    /// Paused by operator
    Paused,
    /// Stopped
    Stopped,
}

/// Message with tracking info
#[derive(Debug, Clone)]
pub struct TrackedMessage {
    pub message: Message,
    pub attempt: u32,
    pub last_error: Option<String>,
}

impl TrackedMessage {
    pub fn new(message: Message) -> Self {
        Self {
            message,
            attempt: 0,
            last_error: None,
        }
    }

    pub fn increment_attempt(&mut self) {
        self.attempt += 1;
    }
}

/// Message group processor - ensures FIFO ordering within a group
pub struct MessageGroupProcessor {
    /// Group identifier
    group_id: String,
    /// Configuration
    config: MessageGroupProcessorConfig,
    /// Message queue
    queue: Arc<Mutex<VecDeque<TrackedMessage>>>,
    /// Current processor state
    state: Arc<Mutex<ProcessorState>>,
    /// Batch message dispatcher
    dispatcher: Arc<dyn BatchMessageDispatcher>,
    /// Shutdown signal receiver
    shutdown_rx: Arc<Mutex<Option<oneshot::Receiver<()>>>>,
}

impl MessageGroupProcessor {
    pub fn new(
        group_id: String,
        config: MessageGroupProcessorConfig,
        dispatcher: Arc<dyn BatchMessageDispatcher>,
    ) -> (Self, oneshot::Sender<()>) {
        let (shutdown_tx, shutdown_rx) = oneshot::channel();

        let processor = Self {
            group_id,
            config,
            queue: Arc::new(Mutex::new(VecDeque::new())),
            state: Arc::new(Mutex::new(ProcessorState::Running)),
            dispatcher,
            shutdown_rx: Arc::new(Mutex::new(Some(shutdown_rx))),
        };

        (processor, shutdown_tx)
    }

    /// Get the group ID
    pub fn group_id(&self) -> &str {
        &self.group_id
    }

    /// Enqueue a message for processing
    pub async fn enqueue(&self, message: Message) -> Result<(), String> {
        let mut queue = self.queue.lock().await;

        if queue.len() >= self.config.max_queue_depth {
            warn!(
                "Queue depth exceeded for group {}, current: {}",
                self.group_id, queue.len()
            );
            return Err("Queue depth exceeded".to_string());
        }

        queue.push_back(TrackedMessage::new(message));
        debug!(
            "Message enqueued for group {}, queue depth: {}",
            self.group_id,
            queue.len()
        );

        Ok(())
    }

    /// Get current queue depth
    pub async fn queue_depth(&self) -> usize {
        let queue = self.queue.lock().await;
        queue.len()
    }

    /// Get current state
    pub async fn state(&self) -> ProcessorState {
        let state = self.state.lock().await;
        state.clone()
    }

    /// Pause processing
    pub async fn pause(&self) {
        let mut state = self.state.lock().await;
        if *state == ProcessorState::Running {
            *state = ProcessorState::Paused;
            info!("Message group processor {} paused", self.group_id);
        }
    }

    /// Resume processing
    pub async fn resume(&self) {
        let mut state = self.state.lock().await;
        if *state == ProcessorState::Paused {
            *state = ProcessorState::Running;
            info!("Message group processor {} resumed", self.group_id);
        }
    }

    /// Unblock the processor (after resolving blocking error)
    pub async fn unblock(&self) {
        let mut state = self.state.lock().await;
        if matches!(*state, ProcessorState::Blocked { .. }) {
            *state = ProcessorState::Running;
            info!("Message group processor {} unblocked", self.group_id);
        }
    }

    /// Skip the blocking message (mark as failed and continue)
    pub async fn skip_blocking_message(&self) -> Option<TrackedMessage> {
        let state_val = self.state().await;
        if !matches!(state_val, ProcessorState::Blocked { .. }) {
            return None;
        }

        let mut queue = self.queue.lock().await;
        let skipped = queue.pop_front();

        let mut state = self.state.lock().await;
        *state = ProcessorState::Running;

        if let Some(ref msg) = skipped {
            info!(
                "Skipped blocking message {} in group {}",
                msg.message.id, self.group_id
            );
        }

        skipped
    }

    /// Process a batch of messages from the queue (up to batch_size)
    /// This matches the Java behavior of processing multiple messages per API call
    pub async fn process_batch(&self) -> Option<BatchDispatchResult> {
        // Check state
        let current_state = self.state().await;
        match current_state {
            ProcessorState::Stopped => return None,
            ProcessorState::Paused => return None,
            ProcessorState::Blocked { .. } => return None,
            ProcessorState::Running => {}
        }

        // Drain up to batch_size messages
        let mut batch: Vec<TrackedMessage> = {
            let mut queue = self.queue.lock().await;
            let count = queue.len().min(self.config.batch_size);
            if count == 0 {
                return None;
            }
            queue.drain(..count).collect()
        };

        // Increment attempt count for all
        for tracked in &mut batch {
            tracked.increment_attempt();
        }

        debug!(
            "Processing batch of {} messages in group {}",
            batch.len(), self.group_id
        );

        // Extract messages for dispatch
        let messages: Vec<Message> = batch.iter().map(|t| t.message.clone()).collect();

        // Dispatch batch
        let result = self.dispatcher.dispatch_batch(&messages).await;

        // Handle results - process in order to maintain FIFO guarantees
        let mut failed_to_requeue: Vec<TrackedMessage> = Vec::new();
        let mut should_block = false;
        let mut block_message_id = String::new();
        let mut block_error = String::new();

        for (tracked, item_result) in batch.into_iter().zip(result.results.iter()) {
            match &item_result.result {
                DispatchResult::Success => {
                    debug!(
                        "Message {} dispatched successfully in group {}",
                        tracked.message.id, self.group_id
                    );
                }
                DispatchResult::Failure { error, retryable } => {
                    let mut tracked = tracked;
                    tracked.last_error = Some(error.clone());

                    if *retryable && tracked.attempt < self.config.max_retries {
                        // Re-queue for retry
                        failed_to_requeue.push(tracked);
                        debug!(
                            "Message {} will be re-queued for retry in group {}",
                            item_result.message_id, self.group_id
                        );
                    } else if self.config.block_on_error {
                        // Block on first non-retryable failure
                        if !should_block {
                            should_block = true;
                            block_message_id = item_result.message_id.clone();
                            block_error = error.clone();
                        }
                        failed_to_requeue.push(tracked);
                    } else {
                        error!(
                            "Message {} failed permanently in group {}: {}",
                            item_result.message_id, self.group_id, error
                        );
                    }
                }
                DispatchResult::Blocked { reason } => {
                    if !should_block {
                        should_block = true;
                        block_message_id = item_result.message_id.clone();
                        block_error = reason.clone();
                    }
                    failed_to_requeue.push(tracked);
                }
            }
        }

        // Re-queue failed messages at the front (in reverse order to maintain FIFO)
        if !failed_to_requeue.is_empty() {
            let mut queue = self.queue.lock().await;
            for tracked in failed_to_requeue.into_iter().rev() {
                queue.push_front(tracked);
            }
        }

        // Set blocked state if needed
        if should_block {
            let mut state = self.state.lock().await;
            *state = ProcessorState::Blocked {
                message_id: block_message_id.clone(),
                error: block_error.clone(),
            };
            error!(
                "Message group processor {} blocked on message {}",
                self.group_id, block_message_id
            );
        }

        Some(result)
    }

    /// Process one message from the queue (for backwards compatibility)
    pub async fn process_one(&self) -> Option<DispatchResult> {
        let batch_result = self.process_batch().await?;
        batch_result.results.into_iter().next().map(|r| r.result)
    }

    /// Run the processor loop
    pub async fn run(&self) {
        info!("Starting message group processor for {}", self.group_id);

        let mut shutdown_rx = {
            let mut rx = self.shutdown_rx.lock().await;
            rx.take()
        };

        loop {
            // Check for shutdown
            if let Some(ref mut rx) = shutdown_rx {
                if rx.try_recv().is_ok() {
                    let mut state = self.state.lock().await;
                    *state = ProcessorState::Stopped;
                    break;
                }
            }

            // Process batch of messages
            match self.process_batch().await {
                Some(_) => {
                    // Continue immediately if we processed something
                }
                None => {
                    // Nothing to process, wait a bit
                    tokio::time::sleep(tokio::time::Duration::from_millis(10)).await;
                }
            }
        }

        info!("Message group processor {} stopped", self.group_id);
    }
}

#[cfg(test)]
mod tests {
    use super::*;
    use fc_common::MediationType;
    use std::sync::atomic::{AtomicUsize, Ordering};

    struct MockBatchDispatcher {
        success_count: AtomicUsize,
        fail_until: AtomicUsize,
    }

    impl MockBatchDispatcher {
        fn new(fail_until: usize) -> Self {
            Self {
                success_count: AtomicUsize::new(0),
                fail_until: AtomicUsize::new(fail_until),
            }
        }
    }

    #[async_trait]
    impl BatchMessageDispatcher for MockBatchDispatcher {
        async fn dispatch_batch(&self, messages: &[Message]) -> BatchDispatchResult {
            let results = messages.iter().map(|msg| {
                let current = self.success_count.fetch_add(1, Ordering::SeqCst);
                let result = if current < self.fail_until.load(Ordering::SeqCst) {
                    DispatchResult::Failure {
                        error: "Mock failure".to_string(),
                        retryable: true,
                    }
                } else {
                    DispatchResult::Success
                };
                BatchItemResult {
                    message_id: msg.id.clone(),
                    result,
                }
            }).collect();
            BatchDispatchResult { results }
        }
    }

    fn create_test_message(id: &str) -> Message {
        Message {
            id: id.to_string(),
            pool_code: "default".to_string(),
            auth_token: None,
            signing_secret: None,
            mediation_type: MediationType::HTTP,
            mediation_target: "http://localhost".to_string(),
            message_group_id: Some("group-1".to_string()),
        }
    }

    #[tokio::test]
    async fn test_enqueue_and_process() {
        let dispatcher = Arc::new(MockBatchDispatcher::new(0));
        let config = MessageGroupProcessorConfig {
            batch_size: 1, // Process one at a time for this test
            ..Default::default()
        };
        let (processor, _shutdown) = MessageGroupProcessor::new(
            "test-group".to_string(),
            config,
            dispatcher,
        );

        // Enqueue messages
        processor.enqueue(create_test_message("msg-1")).await.unwrap();
        processor.enqueue(create_test_message("msg-2")).await.unwrap();

        assert_eq!(processor.queue_depth().await, 2);

        // Process
        let result1 = processor.process_one().await;
        assert!(matches!(result1, Some(DispatchResult::Success)));
        assert_eq!(processor.queue_depth().await, 1);

        let result2 = processor.process_one().await;
        assert!(matches!(result2, Some(DispatchResult::Success)));
        assert_eq!(processor.queue_depth().await, 0);
    }

    #[tokio::test]
    async fn test_batch_processing() {
        let dispatcher = Arc::new(MockBatchDispatcher::new(0));
        let config = MessageGroupProcessorConfig {
            batch_size: 10, // Process up to 10 at a time
            ..Default::default()
        };
        let (processor, _shutdown) = MessageGroupProcessor::new(
            "test-group".to_string(),
            config,
            dispatcher,
        );

        // Enqueue 5 messages
        for i in 0..5 {
            processor.enqueue(create_test_message(&format!("msg-{}", i))).await.unwrap();
        }
        assert_eq!(processor.queue_depth().await, 5);

        // Process batch - should process all 5 in one call
        let result = processor.process_batch().await;
        assert!(result.is_some());
        let batch_result = result.unwrap();
        assert_eq!(batch_result.results.len(), 5);
        assert!(batch_result.all_succeeded());
        assert_eq!(processor.queue_depth().await, 0);
    }

    #[tokio::test]
    async fn test_block_on_error() {
        let dispatcher = Arc::new(MockBatchDispatcher::new(10)); // Fail 10 times
        let config = MessageGroupProcessorConfig {
            max_retries: 2,
            block_on_error: true,
            batch_size: 1,
            ..Default::default()
        };
        let (processor, _shutdown) = MessageGroupProcessor::new(
            "test-group".to_string(),
            config,
            dispatcher,
        );

        processor.enqueue(create_test_message("msg-1")).await.unwrap();

        // Process should fail and retry
        for _ in 0..2 {
            let _ = processor.process_one().await;
        }

        // Should now be blocked
        let state = processor.state().await;
        assert!(matches!(state, ProcessorState::Blocked { .. }));

        // Unblock
        processor.unblock().await;
        assert_eq!(processor.state().await, ProcessorState::Running);
    }

    #[tokio::test]
    async fn test_pause_resume() {
        let dispatcher = Arc::new(MockBatchDispatcher::new(0));
        let config = MessageGroupProcessorConfig {
            batch_size: 1,
            ..Default::default()
        };
        let (processor, _shutdown) = MessageGroupProcessor::new(
            "test-group".to_string(),
            config,
            dispatcher,
        );

        processor.enqueue(create_test_message("msg-1")).await.unwrap();

        // Pause
        processor.pause().await;
        assert_eq!(processor.state().await, ProcessorState::Paused);

        // Process should return None when paused
        let result = processor.process_one().await;
        assert!(result.is_none());
        assert_eq!(processor.queue_depth().await, 1); // Message still in queue

        // Resume
        processor.resume().await;
        assert_eq!(processor.state().await, ProcessorState::Running);

        // Now process should work
        let result = processor.process_one().await;
        assert!(matches!(result, Some(DispatchResult::Success)));
    }
}
