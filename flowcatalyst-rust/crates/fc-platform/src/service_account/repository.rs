//! ServiceAccount Repository

use mongodb::{Collection, Database, bson::doc};
use futures::TryStreamExt;
use crate::ServiceAccount;
use crate::shared::error::Result;

pub struct ServiceAccountRepository {
    collection: Collection<ServiceAccount>,
}

impl ServiceAccountRepository {
    pub fn new(db: &Database) -> Self {
        Self {
            collection: db.collection("service_accounts"),
        }
    }

    pub async fn insert(&self, account: &ServiceAccount) -> Result<()> {
        self.collection.insert_one(account).await?;
        Ok(())
    }

    pub async fn find_by_id(&self, id: &str) -> Result<Option<ServiceAccount>> {
        Ok(self.collection.find_one(doc! { "_id": id }).await?)
    }

    pub async fn find_by_code(&self, code: &str) -> Result<Option<ServiceAccount>> {
        Ok(self.collection.find_one(doc! { "code": code }).await?)
    }

    pub async fn find_active(&self) -> Result<Vec<ServiceAccount>> {
        let cursor = self.collection
            .find(doc! { "active": true })
            .await?;
        Ok(cursor.try_collect().await?)
    }

    pub async fn find_by_application(&self, application_id: &str) -> Result<Vec<ServiceAccount>> {
        let cursor = self.collection
            .find(doc! { "applicationId": application_id })
            .await?;
        Ok(cursor.try_collect().await?)
    }

    pub async fn find_by_client(&self, client_id: &str) -> Result<Vec<ServiceAccount>> {
        let cursor = self.collection
            .find(doc! { "clientIds": client_id })
            .await?;
        Ok(cursor.try_collect().await?)
    }

    pub async fn find_with_role(&self, role: &str) -> Result<Vec<ServiceAccount>> {
        let cursor = self.collection
            .find(doc! { "roles.roleName": role })
            .await?;
        Ok(cursor.try_collect().await?)
    }

    pub async fn update(&self, account: &ServiceAccount) -> Result<()> {
        self.collection
            .replace_one(doc! { "_id": &account.id }, account)
            .await?;
        Ok(())
    }

    pub async fn delete(&self, id: &str) -> Result<bool> {
        let result = self.collection.delete_one(doc! { "_id": id }).await?;
        Ok(result.deleted_count > 0)
    }
}
