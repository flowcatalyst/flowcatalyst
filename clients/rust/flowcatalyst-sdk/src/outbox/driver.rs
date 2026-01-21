//! Outbox storage drivers

use super::dto::OutboxMessage;
use crate::error::Result;
use async_trait::async_trait;

/// Trait for outbox storage drivers
#[async_trait]
pub trait OutboxDriver: Send + Sync {
    /// Insert a single message into the outbox
    async fn insert(&self, message: OutboxMessage) -> Result<()>;

    /// Insert multiple messages into the outbox
    async fn insert_batch(&self, messages: Vec<OutboxMessage>) -> Result<()>;
}

/// PostgreSQL outbox driver (stub - requires sqlx feature)
#[derive(Debug)]
pub struct PostgresDriver {
    connection_string: String,
    table_name: String,
}

impl PostgresDriver {
    /// Create a new PostgreSQL driver
    ///
    /// Note: This is a stub implementation. Enable the `postgres` feature
    /// and provide a real database connection for production use.
    pub async fn new(connection_string: impl Into<String>) -> Result<Self> {
        Ok(Self {
            connection_string: connection_string.into(),
            table_name: "outbox_messages".to_string(),
        })
    }

    /// Set the table name
    pub fn with_table_name(mut self, table_name: impl Into<String>) -> Self {
        self.table_name = table_name.into();
        self
    }
}

#[async_trait]
impl OutboxDriver for PostgresDriver {
    async fn insert(&self, message: OutboxMessage) -> Result<()> {
        // Stub implementation - in production, use sqlx to insert
        tracing::debug!(
            id = %message.id,
            table = %self.table_name,
            connection = %self.connection_string,
            "Would insert outbox message (stub)"
        );

        // In a real implementation:
        // sqlx::query!(
        //     "INSERT INTO {} (id, tenant_id, partition_id, type, payload, headers, created_at)
        //      VALUES ($1, $2, $3, $4, $5, $6, $7)",
        //     message.id,
        //     message.tenant_id,
        //     message.partition_id,
        //     message.message_type,
        //     message.payload,
        //     serde_json::to_value(&message.headers)?,
        //     message.created_at,
        // )
        // .execute(&self.pool)
        // .await?;

        Ok(())
    }

    async fn insert_batch(&self, messages: Vec<OutboxMessage>) -> Result<()> {
        for message in messages {
            self.insert(message).await?;
        }
        Ok(())
    }
}

/// MongoDB outbox driver (stub - requires mongodb feature)
#[derive(Debug)]
#[allow(dead_code)]
pub struct MongoDriver {
    connection_string: String,
    database: String,
    collection: String,
}

impl MongoDriver {
    /// Create a new MongoDB driver
    ///
    /// Note: This is a stub implementation. Enable the `mongodb` feature
    /// and provide a real database connection for production use.
    pub async fn new(
        connection_string: impl Into<String>,
        database: impl Into<String>,
    ) -> Result<Self> {
        Ok(Self {
            connection_string: connection_string.into(),
            database: database.into(),
            collection: "outbox_messages".to_string(),
        })
    }

    /// Set the collection name
    pub fn with_collection(mut self, collection: impl Into<String>) -> Self {
        self.collection = collection.into();
        self
    }
}

#[async_trait]
impl OutboxDriver for MongoDriver {
    async fn insert(&self, message: OutboxMessage) -> Result<()> {
        // Stub implementation - in production, use mongodb driver to insert
        tracing::debug!(
            id = %message.id,
            collection = %self.collection,
            database = %self.database,
            "Would insert outbox message to MongoDB (stub)"
        );

        // In a real implementation:
        // let collection = self.client
        //     .database(&self.database)
        //     .collection::<OutboxMessage>(&self.collection);
        // collection.insert_one(message, None).await?;

        Ok(())
    }

    async fn insert_batch(&self, messages: Vec<OutboxMessage>) -> Result<()> {
        // Stub implementation
        tracing::debug!(
            count = messages.len(),
            collection = %self.collection,
            database = %self.database,
            "Would insert batch to MongoDB (stub)"
        );

        // In a real implementation:
        // let collection = self.client
        //     .database(&self.database)
        //     .collection::<OutboxMessage>(&self.collection);
        // collection.insert_many(messages, None).await?;

        Ok(())
    }
}

/// In-memory outbox driver for testing
#[derive(Debug, Default)]
#[allow(dead_code)]
pub struct InMemoryDriver {
    messages: std::sync::Arc<tokio::sync::RwLock<Vec<OutboxMessage>>>,
}

#[allow(dead_code)]
impl InMemoryDriver {
    /// Create a new in-memory driver
    pub fn new() -> Self {
        Self::default()
    }

    /// Get all stored messages
    pub async fn messages(&self) -> Vec<OutboxMessage> {
        self.messages.read().await.clone()
    }

    /// Clear all messages
    pub async fn clear(&self) {
        self.messages.write().await.clear();
    }
}

#[async_trait]
impl OutboxDriver for InMemoryDriver {
    async fn insert(&self, message: OutboxMessage) -> Result<()> {
        self.messages.write().await.push(message);
        Ok(())
    }

    async fn insert_batch(&self, messages: Vec<OutboxMessage>) -> Result<()> {
        self.messages.write().await.extend(messages);
        Ok(())
    }
}
