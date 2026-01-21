//! Index Initializer
//!
//! Ensures all required MongoDB indexes exist for the stream processor.
//! Should be called on application startup.

use mongodb::{Database, IndexModel};
use mongodb::bson::doc;
use mongodb::options::IndexOptions;
use tracing::{info, warn};

/// Configuration for index initialization
#[derive(Debug, Clone)]
pub struct IndexConfig {
    /// Events collection name
    pub events_collection: String,
    /// Dispatch jobs collection name
    pub dispatch_jobs_collection: String,
    /// Events read projection collection name
    pub events_read_collection: String,
    /// Dispatch jobs read projection collection name
    pub dispatch_jobs_read_collection: String,
    /// Subscriptions collection name
    pub subscriptions_collection: String,
    /// Checkpoints collection name
    pub checkpoints_collection: String,
}

impl Default for IndexConfig {
    fn default() -> Self {
        Self {
            events_collection: "events".to_string(),
            dispatch_jobs_collection: "dispatch_jobs".to_string(),
            events_read_collection: "events_read".to_string(),
            dispatch_jobs_read_collection: "dispatch_jobs_read".to_string(),
            subscriptions_collection: "subscriptions".to_string(),
            checkpoints_collection: "stream_checkpoints".to_string(),
        }
    }
}

/// Result of index initialization
#[derive(Debug, Default)]
pub struct IndexInitResult {
    /// Total number of indexes created
    pub indexes_created: usize,
    /// Collections that were processed
    pub collections_processed: Vec<String>,
    /// Any errors encountered (non-fatal)
    pub warnings: Vec<String>,
}

impl IndexInitResult {
    pub fn is_success(&self) -> bool {
        self.warnings.is_empty()
    }
}

/// Initialize all required indexes for the stream processor
pub struct IndexInitializer {
    db: Database,
    config: IndexConfig,
}

impl IndexInitializer {
    pub fn new(db: Database) -> Self {
        Self {
            db,
            config: IndexConfig::default(),
        }
    }

    pub fn with_config(db: Database, config: IndexConfig) -> Self {
        Self { db, config }
    }

    /// Initialize all indexes
    pub async fn init_all(&self) -> IndexInitResult {
        let mut result = IndexInitResult::default();

        // Events source collection
        self.init_events_indexes(&mut result).await;

        // Dispatch jobs source collection
        self.init_dispatch_jobs_indexes(&mut result).await;

        // Events read projection collection
        self.init_events_read_indexes(&mut result).await;

        // Dispatch jobs read projection collection
        self.init_dispatch_jobs_read_indexes(&mut result).await;

        // Subscriptions collection
        self.init_subscriptions_indexes(&mut result).await;

        // Checkpoints collection
        self.init_checkpoints_indexes(&mut result).await;

        info!(
            indexes_created = result.indexes_created,
            collections = ?result.collections_processed,
            "Index initialization complete"
        );

        result
    }

    async fn init_events_indexes(&self, result: &mut IndexInitResult) {
        let collection = self.db.collection::<mongodb::bson::Document>(&self.config.events_collection);

        let indexes = vec![
            // Query by client and time
            IndexModel::builder()
                .keys(doc! { "clientId": 1, "createdAt": -1 })
                .options(IndexOptions::builder().name("idx_client_created".to_string()).build())
                .build(),
            // Query by event type
            IndexModel::builder()
                .keys(doc! { "eventTypeCode": 1, "createdAt": -1 })
                .options(IndexOptions::builder().name("idx_event_type".to_string()).build())
                .build(),
            // Query by correlation ID
            IndexModel::builder()
                .keys(doc! { "correlationId": 1 })
                .options(IndexOptions::builder().name("idx_correlation".to_string()).sparse(true).build())
                .build(),
            // Query by source entity
            IndexModel::builder()
                .keys(doc! { "sourceType": 1, "sourceId": 1 })
                .options(IndexOptions::builder().name("idx_source".to_string()).sparse(true).build())
                .build(),
        ];

        match collection.create_indexes(indexes).await {
            Ok(res) => {
                result.indexes_created += res.index_names.len();
                result.collections_processed.push(self.config.events_collection.clone());
                info!("Created {} indexes on {}", res.index_names.len(), self.config.events_collection);
            }
            Err(e) => {
                warn!("Failed to create indexes on {}: {}", self.config.events_collection, e);
                result.warnings.push(format!("{}: {}", self.config.events_collection, e));
            }
        }
    }

    async fn init_dispatch_jobs_indexes(&self, result: &mut IndexInitResult) {
        let collection = self.db.collection::<mongodb::bson::Document>(&self.config.dispatch_jobs_collection);

        let indexes = vec![
            // Query by event
            IndexModel::builder()
                .keys(doc! { "eventId": 1 })
                .options(IndexOptions::builder().name("idx_event".to_string()).build())
                .build(),
            // Query by status for processing
            IndexModel::builder()
                .keys(doc! { "status": 1, "nextRetryAt": 1 })
                .options(IndexOptions::builder().name("idx_status_retry".to_string()).build())
                .build(),
            // Query by subscription
            IndexModel::builder()
                .keys(doc! { "subscriptionId": 1, "createdAt": -1 })
                .options(IndexOptions::builder().name("idx_subscription".to_string()).build())
                .build(),
            // Query by client
            IndexModel::builder()
                .keys(doc! { "clientId": 1, "createdAt": -1 })
                .options(IndexOptions::builder().name("idx_client_created".to_string()).build())
                .build(),
            // Query by correlation
            IndexModel::builder()
                .keys(doc! { "correlationId": 1 })
                .options(IndexOptions::builder().name("idx_correlation".to_string()).sparse(true).build())
                .build(),
            // Query pending jobs for dispatch
            IndexModel::builder()
                .keys(doc! { "status": 1, "dispatchPoolId": 1, "createdAt": 1 })
                .options(IndexOptions::builder().name("idx_pending_dispatch".to_string()).build())
                .build(),
        ];

        match collection.create_indexes(indexes).await {
            Ok(res) => {
                result.indexes_created += res.index_names.len();
                result.collections_processed.push(self.config.dispatch_jobs_collection.clone());
                info!("Created {} indexes on {}", res.index_names.len(), self.config.dispatch_jobs_collection);
            }
            Err(e) => {
                warn!("Failed to create indexes on {}: {}", self.config.dispatch_jobs_collection, e);
                result.warnings.push(format!("{}: {}", self.config.dispatch_jobs_collection, e));
            }
        }
    }

    async fn init_events_read_indexes(&self, result: &mut IndexInitResult) {
        let collection = self.db.collection::<mongodb::bson::Document>(&self.config.events_read_collection);

        let indexes = vec![
            // Query by client and time (UI list view)
            IndexModel::builder()
                .keys(doc! { "clientId": 1, "createdAt": -1 })
                .options(IndexOptions::builder().name("idx_client_time".to_string()).build())
                .build(),
            // Query by event type
            IndexModel::builder()
                .keys(doc! { "eventTypeCode": 1, "createdAt": -1 })
                .options(IndexOptions::builder().name("idx_event_type".to_string()).build())
                .build(),
            // Query by correlation ID (tracing)
            IndexModel::builder()
                .keys(doc! { "correlationId": 1 })
                .options(IndexOptions::builder().name("idx_correlation".to_string()).sparse(true).build())
                .build(),
            // Query by source (entity-specific views)
            IndexModel::builder()
                .keys(doc! { "sourceType": 1, "sourceId": 1, "createdAt": -1 })
                .options(IndexOptions::builder().name("idx_source".to_string()).sparse(true).build())
                .build(),
            // Query by application/subdomain
            IndexModel::builder()
                .keys(doc! { "application": 1, "subdomain": 1, "createdAt": -1 })
                .options(IndexOptions::builder().name("idx_app_subdomain".to_string()).build())
                .build(),
            // Full-text search on data summary
            IndexModel::builder()
                .keys(doc! { "dataSummary": "text" })
                .options(IndexOptions::builder().name("idx_data_search".to_string()).build())
                .build(),
        ];

        match collection.create_indexes(indexes).await {
            Ok(res) => {
                result.indexes_created += res.index_names.len();
                result.collections_processed.push(self.config.events_read_collection.clone());
                info!("Created {} indexes on {}", res.index_names.len(), self.config.events_read_collection);
            }
            Err(e) => {
                warn!("Failed to create indexes on {}: {}", self.config.events_read_collection, e);
                result.warnings.push(format!("{}: {}", self.config.events_read_collection, e));
            }
        }
    }

    async fn init_dispatch_jobs_read_indexes(&self, result: &mut IndexInitResult) {
        let collection = self.db.collection::<mongodb::bson::Document>(&self.config.dispatch_jobs_read_collection);

        let indexes = vec![
            // Query by client and time (UI list view)
            IndexModel::builder()
                .keys(doc! { "clientId": 1, "createdAt": -1 })
                .options(IndexOptions::builder().name("idx_client_time".to_string()).build())
                .build(),
            // Query by status (filter by pending, failed, etc.)
            IndexModel::builder()
                .keys(doc! { "status": 1, "createdAt": -1 })
                .options(IndexOptions::builder().name("idx_status".to_string()).build())
                .build(),
            // Query by event ID (show dispatch jobs for an event)
            IndexModel::builder()
                .keys(doc! { "eventId": 1 })
                .options(IndexOptions::builder().name("idx_event".to_string()).build())
                .build(),
            // Query by subscription (analytics)
            IndexModel::builder()
                .keys(doc! { "subscriptionId": 1, "createdAt": -1 })
                .options(IndexOptions::builder().name("idx_subscription".to_string()).build())
                .build(),
            // Query by correlation ID (tracing)
            IndexModel::builder()
                .keys(doc! { "correlationId": 1 })
                .options(IndexOptions::builder().name("idx_correlation".to_string()).sparse(true).build())
                .build(),
            // Query pending retries (scheduler)
            IndexModel::builder()
                .keys(doc! { "status": 1, "nextRetryAt": 1 })
                .options(IndexOptions::builder().name("idx_pending_retry".to_string()).build())
                .build(),
            // Query by dispatch pool (analytics)
            IndexModel::builder()
                .keys(doc! { "dispatchPoolId": 1, "createdAt": -1 })
                .options(IndexOptions::builder().name("idx_pool".to_string()).sparse(true).build())
                .build(),
        ];

        match collection.create_indexes(indexes).await {
            Ok(res) => {
                result.indexes_created += res.index_names.len();
                result.collections_processed.push(self.config.dispatch_jobs_read_collection.clone());
                info!("Created {} indexes on {}", res.index_names.len(), self.config.dispatch_jobs_read_collection);
            }
            Err(e) => {
                warn!("Failed to create indexes on {}: {}", self.config.dispatch_jobs_read_collection, e);
                result.warnings.push(format!("{}: {}", self.config.dispatch_jobs_read_collection, e));
            }
        }
    }

    async fn init_subscriptions_indexes(&self, result: &mut IndexInitResult) {
        let collection = self.db.collection::<mongodb::bson::Document>(&self.config.subscriptions_collection);

        let indexes = vec![
            // Query active subscriptions by client
            IndexModel::builder()
                .keys(doc! { "clientId": 1, "active": 1 })
                .options(IndexOptions::builder().name("idx_client_active".to_string()).build())
                .build(),
            // Query by event type pattern
            IndexModel::builder()
                .keys(doc! { "eventTypePattern": 1 })
                .options(IndexOptions::builder().name("idx_event_type_pattern".to_string()).build())
                .build(),
            // Unique constraint on name per client
            IndexModel::builder()
                .keys(doc! { "clientId": 1, "name": 1 })
                .options(IndexOptions::builder().name("idx_client_name".to_string()).unique(true).build())
                .build(),
        ];

        match collection.create_indexes(indexes).await {
            Ok(res) => {
                result.indexes_created += res.index_names.len();
                result.collections_processed.push(self.config.subscriptions_collection.clone());
                info!("Created {} indexes on {}", res.index_names.len(), self.config.subscriptions_collection);
            }
            Err(e) => {
                warn!("Failed to create indexes on {}: {}", self.config.subscriptions_collection, e);
                result.warnings.push(format!("{}: {}", self.config.subscriptions_collection, e));
            }
        }
    }

    async fn init_checkpoints_indexes(&self, result: &mut IndexInitResult) {
        let collection = self.db.collection::<mongodb::bson::Document>(&self.config.checkpoints_collection);

        let indexes = vec![
            // Primary lookup by stream name
            IndexModel::builder()
                .keys(doc! { "streamName": 1 })
                .options(IndexOptions::builder().name("idx_stream_name".to_string()).unique(true).build())
                .build(),
        ];

        match collection.create_indexes(indexes).await {
            Ok(res) => {
                result.indexes_created += res.index_names.len();
                result.collections_processed.push(self.config.checkpoints_collection.clone());
                info!("Created {} indexes on {}", res.index_names.len(), self.config.checkpoints_collection);
            }
            Err(e) => {
                warn!("Failed to create indexes on {}: {}", self.config.checkpoints_collection, e);
                result.warnings.push(format!("{}: {}", self.config.checkpoints_collection, e));
            }
        }
    }
}

/// Convenience function to initialize all indexes
pub async fn ensure_indexes(db: &Database) -> IndexInitResult {
    IndexInitializer::new(db.clone()).init_all().await
}

/// Convenience function to initialize indexes with custom config
pub async fn ensure_indexes_with_config(db: &Database, config: IndexConfig) -> IndexInitResult {
    IndexInitializer::with_config(db.clone(), config).init_all().await
}

#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn test_default_config() {
        let config = IndexConfig::default();
        assert_eq!(config.events_collection, "events");
        assert_eq!(config.dispatch_jobs_collection, "dispatch_jobs");
        assert_eq!(config.events_read_collection, "events_read");
        assert_eq!(config.dispatch_jobs_read_collection, "dispatch_jobs_read");
    }

    #[test]
    fn test_init_result() {
        let mut result = IndexInitResult::default();
        assert!(result.is_success());

        result.warnings.push("test warning".to_string());
        assert!(!result.is_success());
    }
}
