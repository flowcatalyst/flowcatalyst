//! Principal Repository

use mongodb::{Collection, Database, bson::doc};
use futures::TryStreamExt;
use crate::principal::entity::{Principal, UserScope};
use crate::shared::error::Result;

pub struct PrincipalRepository {
    collection: Collection<Principal>,
}

impl PrincipalRepository {
    pub fn new(db: &Database) -> Self {
        Self {
            collection: db.collection("principals"),
        }
    }

    pub async fn insert(&self, principal: &Principal) -> Result<()> {
        self.collection.insert_one(principal).await?;
        Ok(())
    }

    pub async fn find_by_id(&self, id: &str) -> Result<Option<Principal>> {
        Ok(self.collection.find_one(doc! { "_id": id }).await?)
    }

    pub async fn find_by_email(&self, email: &str) -> Result<Option<Principal>> {
        Ok(self.collection.find_one(doc! {
            "type": "USER",
            "userIdentity.email": email
        }).await?)
    }

    pub async fn find_by_service_account(&self, service_account_id: &str) -> Result<Option<Principal>> {
        Ok(self.collection.find_one(doc! {
            "type": "SERVICE",
            "serviceAccountId": service_account_id
        }).await?)
    }

    pub async fn find_active(&self) -> Result<Vec<Principal>> {
        let cursor = self.collection
            .find(doc! { "active": true })
            .await?;
        Ok(cursor.try_collect().await?)
    }

    pub async fn find_users(&self) -> Result<Vec<Principal>> {
        let cursor = self.collection
            .find(doc! { "type": "USER", "active": true })
            .await?;
        Ok(cursor.try_collect().await?)
    }

    pub async fn find_services(&self) -> Result<Vec<Principal>> {
        let cursor = self.collection
            .find(doc! { "type": "SERVICE", "active": true })
            .await?;
        Ok(cursor.try_collect().await?)
    }

    pub async fn find_by_client(&self, client_id: &str) -> Result<Vec<Principal>> {
        let cursor = self.collection
            .find(doc! {
                "active": true,
                "$or": [
                    { "clientId": client_id },
                    { "assignedClients": client_id }
                ]
            })
            .await?;
        Ok(cursor.try_collect().await?)
    }

    pub async fn find_by_scope(&self, scope: UserScope) -> Result<Vec<Principal>> {
        let scope_str = serde_json::to_string(&scope)
            .unwrap_or_default()
            .trim_matches('"')
            .to_string();
        let cursor = self.collection
            .find(doc! { "scope": scope_str, "active": true })
            .await?;
        Ok(cursor.try_collect().await?)
    }

    pub async fn find_anchors(&self) -> Result<Vec<Principal>> {
        let cursor = self.collection
            .find(doc! { "scope": "ANCHOR", "active": true })
            .await?;
        Ok(cursor.try_collect().await?)
    }

    pub async fn find_by_application(&self, application_id: &str) -> Result<Vec<Principal>> {
        let cursor = self.collection
            .find(doc! { "applicationId": application_id })
            .await?;
        Ok(cursor.try_collect().await?)
    }

    pub async fn find_with_role(&self, role: &str) -> Result<Vec<Principal>> {
        let cursor = self.collection
            .find(doc! { "roles.roleName": role, "active": true })
            .await?;
        Ok(cursor.try_collect().await?)
    }

    pub async fn update(&self, principal: &Principal) -> Result<()> {
        self.collection
            .replace_one(doc! { "_id": &principal.id }, principal)
            .await?;
        Ok(())
    }

    pub async fn delete(&self, id: &str) -> Result<bool> {
        let result = self.collection.delete_one(doc! { "_id": id }).await?;
        Ok(result.deleted_count > 0)
    }

    /// Count principals with email ending in the given domain (matches Java countByEmailDomain)
    pub async fn count_by_email_domain(&self, domain: &str) -> Result<i64> {
        // Match emails ending with @domain (case-insensitive)
        let pattern = format!("@{}$", regex::escape(&domain.to_lowercase()));
        let count = self.collection.count_documents(doc! {
            "type": "USER",
            "userIdentity.email": { "$regex": &pattern, "$options": "i" }
        }).await?;
        Ok(count as i64)
    }
}
