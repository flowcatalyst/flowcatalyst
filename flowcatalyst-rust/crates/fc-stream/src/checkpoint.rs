use async_trait::async_trait;
use mongodb::bson::Document;
use anyhow::Result;
use std::collections::HashMap;
use std::sync::Arc;
use parking_lot::RwLock;

#[async_trait]
pub trait CheckpointStore: Send + Sync {
    async fn get_checkpoint(&self, key: &str) -> Result<Option<Document>>;
    async fn save_checkpoint(&self, key: &str, token: Document) -> Result<()>;
    /// Clear a checkpoint (for recovery from stale resume token)
    async fn clear_checkpoint(&self, key: &str) -> Result<()>;
}

// ============================================================================
// MongoDB Checkpoint Store
// ============================================================================

pub struct MongoCheckpointStore {
    collection: mongodb::Collection<Document>,
}

impl MongoCheckpointStore {
    pub fn new(client: mongodb::Client, db_name: &str, collection_name: &str) -> Self {
        let db = client.database(db_name);
        Self {
            collection: db.collection(collection_name),
        }
    }
}

#[async_trait]
impl CheckpointStore for MongoCheckpointStore {
    async fn get_checkpoint(&self, key: &str) -> Result<Option<Document>> {
        let filter = mongodb::bson::doc! { "_id": key };
        let doc = self.collection.find_one(filter).await?;
        Ok(doc.and_then(|d| d.get_document("token").ok().cloned()))
    }

    async fn save_checkpoint(&self, key: &str, token: Document) -> Result<()> {
        let filter = mongodb::bson::doc! { "_id": key };
        let update = mongodb::bson::doc! {
            "$set": { "token": token, "updated_at": mongodb::bson::DateTime::now() }
        };
        let options = mongodb::options::UpdateOptions::builder().upsert(true).build();

        self.collection.update_one(filter, update).with_options(options).await?;
        Ok(())
    }

    async fn clear_checkpoint(&self, key: &str) -> Result<()> {
        let filter = mongodb::bson::doc! { "_id": key };
        self.collection.delete_one(filter).await?;
        Ok(())
    }
}

// ============================================================================
// Redis Checkpoint Store
// ============================================================================

pub struct RedisCheckpointStore {
    client: redis::Client,
    prefix: String,
}

impl RedisCheckpointStore {
    pub fn new(redis_url: &str, prefix: &str) -> Result<Self> {
        let client = redis::Client::open(redis_url)?;
        Ok(Self {
            client,
            prefix: prefix.to_string(),
        })
    }

    fn key(&self, stream_key: &str) -> String {
        format!("{}:{}", self.prefix, stream_key)
    }
}

#[async_trait]
impl CheckpointStore for RedisCheckpointStore {
    async fn get_checkpoint(&self, key: &str) -> Result<Option<Document>> {
        let mut conn = self.client.get_multiplexed_async_connection().await?;
        let redis_key = self.key(key);

        let data: Option<String> = redis::AsyncCommands::get(&mut conn, &redis_key).await?;

        match data {
            Some(json) => {
                let doc: Document = serde_json::from_str(&json)?;
                Ok(Some(doc))
            }
            None => Ok(None),
        }
    }

    async fn save_checkpoint(&self, key: &str, token: Document) -> Result<()> {
        let mut conn = self.client.get_multiplexed_async_connection().await?;
        let redis_key = self.key(key);
        let json = serde_json::to_string(&token)?;

        redis::AsyncCommands::set::<_, _, ()>(&mut conn, &redis_key, json).await?;
        Ok(())
    }

    async fn clear_checkpoint(&self, key: &str) -> Result<()> {
        let mut conn = self.client.get_multiplexed_async_connection().await?;
        let redis_key = self.key(key);

        redis::AsyncCommands::del::<_, ()>(&mut conn, &redis_key).await?;
        Ok(())
    }
}

// ============================================================================
// In-Memory Checkpoint Store (for testing/development)
// ============================================================================

pub struct MemoryCheckpointStore {
    checkpoints: Arc<RwLock<HashMap<String, Document>>>,
}

impl MemoryCheckpointStore {
    pub fn new() -> Self {
        Self {
            checkpoints: Arc::new(RwLock::new(HashMap::new())),
        }
    }
}

impl Default for MemoryCheckpointStore {
    fn default() -> Self {
        Self::new()
    }
}

#[async_trait]
impl CheckpointStore for MemoryCheckpointStore {
    async fn get_checkpoint(&self, key: &str) -> Result<Option<Document>> {
        let checkpoints = self.checkpoints.read();
        Ok(checkpoints.get(key).cloned())
    }

    async fn save_checkpoint(&self, key: &str, token: Document) -> Result<()> {
        let mut checkpoints = self.checkpoints.write();
        checkpoints.insert(key.to_string(), token);
        Ok(())
    }

    async fn clear_checkpoint(&self, key: &str) -> Result<()> {
        let mut checkpoints = self.checkpoints.write();
        checkpoints.remove(key);
        Ok(())
    }
}
