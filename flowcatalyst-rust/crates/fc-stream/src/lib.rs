pub mod config;
pub mod checkpoint;
pub mod checkpoint_tracker;
pub mod watcher;
pub mod mock;
pub mod subscription_matcher;
pub mod batch_dispatcher;
pub mod projection;
pub mod health;
pub mod index_initializer;

#[cfg(feature = "standby")]
pub mod standby;

use async_trait::async_trait;
use anyhow::Result;
pub use config::StreamConfig;
pub use checkpoint_tracker::{CheckpointTracker, AggregateTracker, PendingDocument};
pub use watcher::{MongoStreamWatcher, BatchProcessor, LoggingBatchProcessor};

// Re-export key types
pub use subscription_matcher::{
    MatchableEvent, MatchableSubscription, SubscriptionBinding,
    SubscriptionCache, SubscriptionMatcher, DispatchJobCreation,
};
pub use batch_dispatcher::{
    BatchDispatcher, BatchDispatcherConfig, BatchDispatchResult,
    DispatchTarget, DispatchError, InMemoryDispatchTarget, MongoDispatchTarget,
};
pub use projection::{
    EventReadProjection, DispatchJobReadProjection, ProjectionBuilder,
    ProjectionLookup, ProjectionStore, EventData, DispatchJobData,
    MongoProjectionStore, InMemoryProjectionStore, InMemoryLookup,
    BatchWriteResult, ChangeOperationType, ProjectionMapResult,
    ProjectionMapper, EventMapper, DispatchJobMapper, ProjectionProcessor,
};
pub use health::{
    StreamHealth, StreamHealthStatus, StreamProcessorHealth, StreamHealthService,
    AggregatedHealth, StreamStatus, StreamHealthSnapshot,
};
pub use index_initializer::{
    IndexInitializer, IndexConfig, IndexInitResult, ensure_indexes, ensure_indexes_with_config,
};

#[cfg(feature = "standby")]
pub use standby::StandbyStreamProcessor;

#[async_trait]
pub trait StreamWatcher: Send + Sync {
    async fn watch(&self) -> Result<()>;
}

pub struct StreamProcessor {
    watchers: Vec<Box<dyn StreamWatcher>>,
}

impl StreamProcessor {
    pub fn new() -> Self {
        Self { watchers: Vec::new() }
    }

    pub fn add_watcher(&mut self, watcher: Box<dyn StreamWatcher>) {
        self.watchers.push(watcher);
    }

    pub async fn start(self) {
        tracing::info!("Starting Stream Processor with {} watchers", self.watchers.len());
        let mut handles = Vec::new();
        for watcher in self.watchers {
            handles.push(tokio::spawn(async move {
                if let Err(e) = watcher.watch().await {
                    tracing::error!("Stream watcher failed: {}", e);
                }
            }));
        }

        for handle in handles {
            let _ = handle.await;
        }
    }
}