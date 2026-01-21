pub mod repository;
pub mod buffer;
pub mod message_group_processor;
pub mod group_distributor;
pub mod recovery;
pub mod http_dispatcher;
pub mod enhanced_processor;

#[cfg(feature = "sqlite")]
pub mod sqlite;
#[cfg(feature = "postgres")]
pub mod postgres;
#[cfg(feature = "mysql")]
pub mod mysql;
#[cfg(feature = "mongo")]
pub mod mongo;

use std::sync::Arc;
use std::sync::atomic::{AtomicBool, Ordering};
use tokio::time::{sleep, Duration};
use fc_common::{OutboxStatus, OutboxItemType, Message, MediationType};
use anyhow::Result;
use tracing::{info, error, debug, warn};
use async_trait::async_trait;

// Re-export key types
pub use buffer::{GlobalBuffer, GlobalBufferConfig, BufferFullError};
pub use message_group_processor::{
    MessageGroupProcessor, MessageGroupProcessorConfig, MessageDispatcher,
    BatchMessageDispatcher, BatchDispatchResult, BatchItemResult,
    DispatchResult, ProcessorState, TrackedMessage,
};
pub use group_distributor::{GroupDistributor, GroupDistributorConfig, DistributorStats};
pub use recovery::{RecoveryTask, RecoveryConfig};
pub use http_dispatcher::{
    HttpDispatcher, HttpDispatcherConfig, BatchRequest, BatchResponse,
    ItemStatus, OutboxDispatchResult,
};
pub use enhanced_processor::{EnhancedOutboxProcessor, EnhancedProcessorConfig, ProcessorMetrics};
pub use repository::{OutboxRepository, OutboxTableConfig, OutboxRepositoryExt};

/// Configuration for leader election in outbox processor
#[derive(Debug, Clone)]
pub struct LeaderElectionConfig {
    /// Whether leader election is enabled
    pub enabled: bool,
    /// Redis URL for leader election
    pub redis_url: String,
    /// Lock key for this processor
    pub lock_key: String,
    /// Lock TTL in seconds
    pub lock_ttl_seconds: u64,
    /// Heartbeat interval in seconds
    pub heartbeat_interval_seconds: u64,
}

impl Default for LeaderElectionConfig {
    fn default() -> Self {
        Self {
            enabled: false,
            redis_url: "redis://127.0.0.1:6379".to_string(),
            lock_key: "fc:outbox-processor-leader".to_string(),
            lock_ttl_seconds: 30,
            heartbeat_interval_seconds: 10,
        }
    }
}

pub struct OutboxProcessor {
    repository: Arc<dyn OutboxRepository>,
    queue_publisher: Arc<dyn QueuePublisher>,
    poll_interval: Duration,
    batch_size: u32,
    leader_election_config: LeaderElectionConfig,
    is_primary: Arc<AtomicBool>,
}

#[async_trait]
pub trait QueuePublisher: Send + Sync {
    async fn publish(&self, message: Message) -> Result<()>;
}

impl OutboxProcessor {
    pub fn new(
        repository: Arc<dyn OutboxRepository>,
        queue_publisher: Arc<dyn QueuePublisher>,
        poll_interval: Duration,
        batch_size: u32,
    ) -> Self {
        Self {
            repository,
            queue_publisher,
            poll_interval,
            batch_size,
            leader_election_config: LeaderElectionConfig::default(),
            is_primary: Arc::new(AtomicBool::new(true)), // Default to primary (single-instance mode)
        }
    }

    /// Create a new outbox processor with leader election configuration
    pub fn with_leader_election(
        repository: Arc<dyn OutboxRepository>,
        queue_publisher: Arc<dyn QueuePublisher>,
        poll_interval: Duration,
        batch_size: u32,
        leader_election_config: LeaderElectionConfig,
    ) -> Self {
        let is_primary = Arc::new(AtomicBool::new(!leader_election_config.enabled));
        Self {
            repository,
            queue_publisher,
            poll_interval,
            batch_size,
            leader_election_config,
            is_primary,
        }
    }

    /// Check if this processor is the current leader
    pub fn is_primary(&self) -> bool {
        self.is_primary.load(Ordering::SeqCst)
    }

    /// Set the primary status (called by leader election)
    pub fn set_primary(&self, primary: bool) {
        self.is_primary.store(primary, Ordering::SeqCst);
        if primary {
            info!("Outbox processor became primary");
        } else {
            warn!("Outbox processor lost primary status");
        }
    }

    /// Get a clone of the is_primary flag for use by leader election
    pub fn is_primary_flag(&self) -> Arc<AtomicBool> {
        self.is_primary.clone()
    }

    pub async fn start(&self) {
        info!(
            poll_interval_ms = %self.poll_interval.as_millis(),
            batch_size = %self.batch_size,
            leader_election_enabled = %self.leader_election_config.enabled,
            is_primary = %self.is_primary(),
            "Starting Outbox Processor"
        );

        loop {
            // Only process if we're the primary (leader)
            if !self.is_primary() {
                debug!("Skipping poll - not primary");
                sleep(self.poll_interval).await;
                continue;
            }

            if let Err(e) = self.process_batch().await {
                error!("Error processing outbox batch: {}", e);
            }
            sleep(self.poll_interval).await;
        }
    }

    async fn process_batch(&self) -> Result<()> {
        // Process both EVENT and DISPATCH_JOB items
        for item_type in [OutboxItemType::EVENT, OutboxItemType::DISPATCH_JOB] {
            self.process_items_of_type(item_type).await?;
        }
        Ok(())
    }

    async fn process_items_of_type(&self, item_type: OutboxItemType) -> Result<()> {
        let items = self.repository.fetch_pending_by_type(item_type, self.batch_size / 2).await?;
        if items.is_empty() {
            return Ok(());
        }

        let ids: Vec<String> = items.iter().map(|i| i.id.clone()).collect();
        self.repository.mark_in_progress(item_type, ids).await?;

        for item in items {
            debug!("Processing outbox item [{}] type={}", item.id, item_type);

            // Map OutboxItem to Message
            let message = Message {
                id: item.id.clone(),
                pool_code: item.pool_code.clone().unwrap_or_else(|| "DEFAULT".to_string()),
                auth_token: None,
                signing_secret: None,
                mediation_type: MediationType::HTTP,
                mediation_target: item.mediation_target.clone().unwrap_or_else(|| "http://localhost:8080".to_string()),
                message_group_id: item.message_group.clone(),
            };

            match self.queue_publisher.publish(message).await {
                Ok(_) => {
                    self.repository.mark_with_status(
                        item_type,
                        vec![item.id.clone()],
                        OutboxStatus::SUCCESS,
                        None,
                    ).await?;
                }
                Err(e) => {
                    error!("Failed to publish outbox item [{}]: {}", item.id, e);
                    self.repository.mark_with_status(
                        item_type,
                        vec![item.id.clone()],
                        OutboxStatus::INTERNAL_ERROR,
                        Some(e.to_string()),
                    ).await?;
                }
            }
        }

        Ok(())
    }
}