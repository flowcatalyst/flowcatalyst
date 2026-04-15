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

/// Leader election configuration. Re-exported from `fc_common` — a single
/// unified type replacing the previous per-crate duplicates in fc-outbox and fc-standby.
pub use fc_common::LeaderElectionConfig;
