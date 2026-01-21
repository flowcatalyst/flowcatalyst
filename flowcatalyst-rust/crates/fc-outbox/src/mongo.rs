//! MongoDB Outbox Repository Implementation
//!
//! Implements the OutboxRepository trait for MongoDB with Java-compatible
//! dual-collection support (outbox_events and outbox_dispatch_jobs).

use async_trait::async_trait;
use fc_common::{OutboxItem, OutboxItemType, OutboxStatus};
use crate::repository::{OutboxRepository, OutboxTableConfig};
use anyhow::Result;
use mongodb::{Client, Collection, Database, IndexModel};
use mongodb::bson::{doc, Document};
use mongodb::options::{FindOptions, IndexOptions};
use chrono::{DateTime, Utc};
use futures::stream::TryStreamExt;
use std::time::Duration;
use tracing::{info, debug};

/// MongoDB implementation of OutboxRepository
pub struct MongoOutboxRepository {
    database: Database,
    table_config: OutboxTableConfig,
}

impl MongoOutboxRepository {
    /// Create a new MongoDB outbox repository with default table config
    pub fn new(client: Client, db_name: &str) -> Self {
        let database = client.database(db_name);
        Self {
            database,
            table_config: OutboxTableConfig::default(),
        }
    }

    /// Create with custom table configuration
    pub fn with_config(client: Client, db_name: &str, table_config: OutboxTableConfig) -> Self {
        let database = client.database(db_name);
        Self { database, table_config }
    }

    /// Get the database reference
    pub fn database(&self) -> &Database {
        &self.database
    }

    /// Get collection for item type
    fn collection_for_type(&self, item_type: OutboxItemType) -> Collection<Document> {
        let name = self.table_config.table_for_type(item_type);
        self.database.collection(name)
    }

    /// Parse a document into an OutboxItem
    fn parse_doc(&self, doc: &Document, item_type: OutboxItemType) -> Result<OutboxItem> {
        let created_at_ts = doc.get_i64("created_at")?;
        let created_at = DateTime::from_timestamp_millis(created_at_ts)
            .ok_or_else(|| anyhow::anyhow!("Invalid created_at timestamp"))?;

        let updated_at = doc.get_i64("updated_at").ok()
            .and_then(DateTime::from_timestamp_millis);

        let status_code = doc.get_i32("status").unwrap_or(0);
        let status = OutboxStatus::from_code(status_code);

        let payload_str = doc.get_str("payload")?;
        let payload: serde_json::Value = serde_json::from_str(payload_str)?;

        Ok(OutboxItem {
            id: doc.get_str("id")?.to_string(),
            item_type,
            pool_code: doc.get_str("pool_code").ok().map(String::from),
            mediation_target: doc.get_str("mediation_target").ok().map(String::from),
            message_group: doc.get_str("message_group").ok().map(String::from),
            payload,
            status,
            retry_count: doc.get_i32("retry_count").unwrap_or(0),
            error_message: doc.get_str("error_message").ok().map(String::from),
            created_at,
            updated_at,
        })
    }
}

#[async_trait]
impl OutboxRepository for MongoOutboxRepository {
    async fn fetch_pending_by_type(&self, item_type: OutboxItemType, limit: u32) -> Result<Vec<OutboxItem>> {
        let collection = self.collection_for_type(item_type);
        let filter = doc! { "status": OutboxStatus::PENDING.code() };
        let find_options = FindOptions::builder()
            .sort(doc! { "created_at": 1 })
            .limit(limit as i64)
            .build();

        let mut cursor = collection.find(filter).with_options(find_options).await?;
        let mut items = Vec::new();

        while let Some(doc) = cursor.try_next().await? {
            items.push(self.parse_doc(&doc, item_type)?);
        }

        debug!(
            collection = %self.table_config.table_for_type(item_type),
            count = items.len(),
            "Fetched pending items"
        );

        Ok(items)
    }

    async fn mark_in_progress(&self, item_type: OutboxItemType, ids: Vec<String>) -> Result<()> {
        if ids.is_empty() {
            return Ok(());
        }

        let collection = self.collection_for_type(item_type);
        let now = Utc::now().timestamp_millis();

        let filter = doc! { "id": { "$in": &ids } };
        let update = doc! {
            "$set": {
                "status": OutboxStatus::IN_PROGRESS.code(),
                "updated_at": now
            }
        };

        collection.update_many(filter, update).await?;

        debug!(
            collection = %self.table_config.table_for_type(item_type),
            count = ids.len(),
            "Marked items as IN_PROGRESS"
        );

        Ok(())
    }

    async fn mark_with_status(
        &self,
        item_type: OutboxItemType,
        ids: Vec<String>,
        status: OutboxStatus,
        error_message: Option<String>,
    ) -> Result<()> {
        if ids.is_empty() {
            return Ok(());
        }

        let collection = self.collection_for_type(item_type);
        let now = Utc::now().timestamp_millis();

        let filter = doc! { "id": { "$in": &ids } };
        let mut set_doc = doc! {
            "status": status.code(),
            "updated_at": now
        };

        if let Some(err) = &error_message {
            set_doc.insert("error_message", err);
        }

        let update = doc! { "$set": set_doc };
        collection.update_many(filter, update).await?;

        debug!(
            collection = %self.table_config.table_for_type(item_type),
            status = ?status,
            count = ids.len(),
            "Marked items with status"
        );

        Ok(())
    }

    async fn increment_retry_count(&self, item_type: OutboxItemType, ids: Vec<String>) -> Result<()> {
        if ids.is_empty() {
            return Ok(());
        }

        let collection = self.collection_for_type(item_type);
        let now = Utc::now().timestamp_millis();

        let filter = doc! { "id": { "$in": &ids } };
        let update = doc! {
            "$inc": { "retry_count": 1 },
            "$set": {
                "status": OutboxStatus::PENDING.code(),
                "updated_at": now
            }
        };

        collection.update_many(filter, update).await?;

        debug!(
            collection = %self.table_config.table_for_type(item_type),
            count = ids.len(),
            "Incremented retry count"
        );

        Ok(())
    }

    async fn fetch_recoverable_items(
        &self,
        item_type: OutboxItemType,
        timeout: Duration,
        limit: u32,
    ) -> Result<Vec<OutboxItem>> {
        let collection = self.collection_for_type(item_type);
        let timeout_ms = timeout.as_millis() as i64;
        let cutoff = Utc::now().timestamp_millis() - timeout_ms;

        let filter = doc! {
            "status": {
                "$in": [
                    OutboxStatus::IN_PROGRESS.code(),
                    OutboxStatus::BAD_REQUEST.code(),
                    OutboxStatus::INTERNAL_ERROR.code(),
                    OutboxStatus::UNAUTHORIZED.code(),
                    OutboxStatus::FORBIDDEN.code(),
                    OutboxStatus::GATEWAY_ERROR.code(),
                ]
            },
            "updated_at": { "$lt": cutoff }
        };

        let find_options = FindOptions::builder()
            .sort(doc! { "created_at": 1 })
            .limit(limit as i64)
            .build();

        let mut cursor = collection.find(filter).with_options(find_options).await?;
        let mut items = Vec::new();

        while let Some(doc) = cursor.try_next().await? {
            items.push(self.parse_doc(&doc, item_type)?);
        }

        Ok(items)
    }

    async fn reset_recoverable_items(&self, item_type: OutboxItemType, ids: Vec<String>) -> Result<()> {
        if ids.is_empty() {
            return Ok(());
        }

        let collection = self.collection_for_type(item_type);
        let now = Utc::now().timestamp_millis();

        let filter = doc! { "id": { "$in": &ids } };
        let update = doc! {
            "$set": {
                "status": OutboxStatus::PENDING.code(),
                "updated_at": now
            }
        };

        collection.update_many(filter, update).await?;

        info!(
            collection = %self.table_config.table_for_type(item_type),
            count = ids.len(),
            "Reset recoverable items to PENDING"
        );

        Ok(())
    }

    async fn fetch_stuck_items(
        &self,
        item_type: OutboxItemType,
        timeout: Duration,
        limit: u32,
    ) -> Result<Vec<OutboxItem>> {
        let collection = self.collection_for_type(item_type);
        let timeout_ms = timeout.as_millis() as i64;
        let cutoff = Utc::now().timestamp_millis() - timeout_ms;

        let filter = doc! {
            "status": OutboxStatus::IN_PROGRESS.code(),
            "updated_at": { "$lt": cutoff }
        };

        let find_options = FindOptions::builder()
            .sort(doc! { "created_at": 1 })
            .limit(limit as i64)
            .build();

        let mut cursor = collection.find(filter).with_options(find_options).await?;
        let mut items = Vec::new();

        while let Some(doc) = cursor.try_next().await? {
            items.push(self.parse_doc(&doc, item_type)?);
        }

        Ok(items)
    }

    async fn reset_stuck_items(&self, item_type: OutboxItemType, ids: Vec<String>) -> Result<()> {
        self.reset_recoverable_items(item_type, ids).await
    }

    async fn init_schema(&self) -> Result<()> {
        // Create indexes for events collection
        let events_collection = self.collection_for_type(OutboxItemType::EVENT);
        let status_index = IndexModel::builder()
            .keys(doc! { "status": 1 })
            .options(IndexOptions::builder().name("idx_status".to_string()).build())
            .build();
        let created_at_index = IndexModel::builder()
            .keys(doc! { "created_at": 1 })
            .options(IndexOptions::builder().name("idx_created_at".to_string()).build())
            .build();

        events_collection.create_indexes([status_index.clone(), created_at_index.clone()]).await?;

        // Create indexes for dispatch_jobs collection
        let dispatch_jobs_collection = self.collection_for_type(OutboxItemType::DISPATCH_JOB);
        dispatch_jobs_collection.create_indexes([status_index, created_at_index]).await?;

        info!(
            events_collection = %self.table_config.events_table,
            dispatch_jobs_collection = %self.table_config.dispatch_jobs_table,
            "Initialized MongoDB outbox indexes"
        );

        Ok(())
    }

    fn table_config(&self) -> &OutboxTableConfig {
        &self.table_config
    }
}
