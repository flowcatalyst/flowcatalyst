//! Client Repository

use mongodb::{Collection, Database, bson::doc};
use futures::TryStreamExt;
use super::entity::{Client, ClientStatus};
use crate::shared::error::Result;

pub struct ClientRepository {
    collection: Collection<Client>,
}

impl ClientRepository {
    pub fn new(db: &Database) -> Self {
        Self {
            collection: db.collection("clients"),
        }
    }

    pub async fn insert(&self, client: &Client) -> Result<()> {
        self.collection.insert_one(client).await?;
        Ok(())
    }

    pub async fn find_by_id(&self, id: &str) -> Result<Option<Client>> {
        Ok(self.collection.find_one(doc! { "_id": id }).await?)
    }

    pub async fn find_by_identifier(&self, identifier: &str) -> Result<Option<Client>> {
        Ok(self.collection.find_one(doc! { "identifier": identifier }).await?)
    }

    pub async fn find_active(&self) -> Result<Vec<Client>> {
        let cursor = self.collection
            .find(doc! { "status": "ACTIVE" })
            .await?;
        Ok(cursor.try_collect().await?)
    }

    pub async fn find_all(&self) -> Result<Vec<Client>> {
        let cursor = self.collection.find(doc! {}).await?;
        Ok(cursor.try_collect().await?)
    }

    /// Search clients by name or identifier (case-insensitive partial match)
    pub async fn search(&self, term: &str) -> Result<Vec<Client>> {
        use mongodb::bson::Regex;
        let pattern = Regex {
            pattern: term.to_string(),
            options: "i".to_string(), // case-insensitive
        };
        let cursor = self.collection
            .find(doc! {
                "$or": [
                    { "name": { "$regex": &pattern } },
                    { "identifier": { "$regex": &pattern } }
                ]
            })
            .await?;
        Ok(cursor.try_collect().await?)
    }

    pub async fn find_by_status(&self, status: ClientStatus) -> Result<Vec<Client>> {
        let status_str = serde_json::to_string(&status)
            .unwrap_or_default()
            .trim_matches('"')
            .to_string();
        let cursor = self.collection
            .find(doc! { "status": status_str })
            .await?;
        Ok(cursor.try_collect().await?)
    }

    pub async fn find_by_ids(&self, ids: &[String]) -> Result<Vec<Client>> {
        let cursor = self.collection
            .find(doc! { "_id": { "$in": ids } })
            .await?;
        Ok(cursor.try_collect().await?)
    }

    pub async fn exists(&self, id: &str) -> Result<bool> {
        let count = self.collection
            .count_documents(doc! { "_id": id })
            .await?;
        Ok(count > 0)
    }

    pub async fn exists_by_identifier(&self, identifier: &str) -> Result<bool> {
        let count = self.collection
            .count_documents(doc! { "identifier": identifier })
            .await?;
        Ok(count > 0)
    }

    pub async fn update(&self, client: &Client) -> Result<()> {
        self.collection
            .replace_one(doc! { "_id": &client.id }, client)
            .await?;
        Ok(())
    }

    pub async fn delete(&self, id: &str) -> Result<bool> {
        let result = self.collection.delete_one(doc! { "_id": id }).await?;
        Ok(result.deleted_count > 0)
    }
}
