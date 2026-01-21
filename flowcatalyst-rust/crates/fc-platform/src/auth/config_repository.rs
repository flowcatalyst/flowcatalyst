//! Authentication Configuration Repositories

use mongodb::{Collection, Database, bson::doc};
use futures::TryStreamExt;
use crate::auth::config_entity::{AnchorDomain, ClientAuthConfig, ClientAccessGrant, IdpRoleMapping};
use crate::shared::error::Result;

/// Anchor Domain Repository
pub struct AnchorDomainRepository {
    collection: Collection<AnchorDomain>,
}

impl AnchorDomainRepository {
    pub fn new(db: &Database) -> Self {
        Self {
            collection: db.collection("anchor_domains"),
        }
    }

    pub async fn insert(&self, domain: &AnchorDomain) -> Result<()> {
        self.collection.insert_one(domain).await?;
        Ok(())
    }

    pub async fn find_by_id(&self, id: &str) -> Result<Option<AnchorDomain>> {
        Ok(self.collection.find_one(doc! { "_id": id }).await?)
    }

    pub async fn find_by_domain(&self, domain: &str) -> Result<Option<AnchorDomain>> {
        Ok(self.collection.find_one(doc! { "domain": domain.to_lowercase() }).await?)
    }

    pub async fn find_all(&self) -> Result<Vec<AnchorDomain>> {
        let cursor = self.collection.find(doc! {}).await?;
        Ok(cursor.try_collect().await?)
    }

    pub async fn is_anchor_domain(&self, domain: &str) -> Result<bool> {
        let count = self.collection
            .count_documents(doc! { "domain": domain.to_lowercase() })
            .await?;
        Ok(count > 0)
    }

    pub async fn delete(&self, id: &str) -> Result<bool> {
        let result = self.collection.delete_one(doc! { "_id": id }).await?;
        Ok(result.deleted_count > 0)
    }
}

/// Client Auth Config Repository
pub struct ClientAuthConfigRepository {
    collection: Collection<ClientAuthConfig>,
}

impl ClientAuthConfigRepository {
    pub fn new(db: &Database) -> Self {
        Self {
            collection: db.collection("auth_configs"),
        }
    }

    pub async fn insert(&self, config: &ClientAuthConfig) -> Result<()> {
        self.collection.insert_one(config).await?;
        Ok(())
    }

    pub async fn find_by_id(&self, id: &str) -> Result<Option<ClientAuthConfig>> {
        Ok(self.collection.find_one(doc! { "_id": id }).await?)
    }

    pub async fn find_by_email_domain(&self, domain: &str) -> Result<Option<ClientAuthConfig>> {
        Ok(self.collection
            .find_one(doc! { "emailDomain": domain.to_lowercase() })
            .await?)
    }

    pub async fn find_by_client_id(&self, client_id: &str) -> Result<Vec<ClientAuthConfig>> {
        let cursor = self.collection
            .find(doc! { "primaryClientId": client_id })
            .await?;
        Ok(cursor.try_collect().await?)
    }

    pub async fn find_all(&self) -> Result<Vec<ClientAuthConfig>> {
        let cursor = self.collection.find(doc! {}).await?;
        Ok(cursor.try_collect().await?)
    }

    pub async fn update(&self, config: &ClientAuthConfig) -> Result<()> {
        self.collection
            .replace_one(doc! { "_id": &config.id }, config)
            .await?;
        Ok(())
    }

    pub async fn delete(&self, id: &str) -> Result<bool> {
        let result = self.collection.delete_one(doc! { "_id": id }).await?;
        Ok(result.deleted_count > 0)
    }
}

/// Client Access Grant Repository
pub struct ClientAccessGrantRepository {
    collection: Collection<ClientAccessGrant>,
}

impl ClientAccessGrantRepository {
    pub fn new(db: &Database) -> Self {
        Self {
            collection: db.collection("client_access_grants"),
        }
    }

    pub async fn insert(&self, grant: &ClientAccessGrant) -> Result<()> {
        self.collection.insert_one(grant).await?;
        Ok(())
    }

    pub async fn find_by_id(&self, id: &str) -> Result<Option<ClientAccessGrant>> {
        Ok(self.collection.find_one(doc! { "_id": id }).await?)
    }

    pub async fn find_by_principal(&self, principal_id: &str) -> Result<Vec<ClientAccessGrant>> {
        let cursor = self.collection
            .find(doc! { "principalId": principal_id })
            .await?;
        Ok(cursor.try_collect().await?)
    }

    pub async fn find_by_client(&self, client_id: &str) -> Result<Vec<ClientAccessGrant>> {
        let cursor = self.collection
            .find(doc! { "clientId": client_id })
            .await?;
        Ok(cursor.try_collect().await?)
    }

    pub async fn find_by_principal_and_client(
        &self,
        principal_id: &str,
        client_id: &str,
    ) -> Result<Option<ClientAccessGrant>> {
        Ok(self.collection
            .find_one(doc! { "principalId": principal_id, "clientId": client_id })
            .await?)
    }

    pub async fn find_active_by_principal(&self, principal_id: &str) -> Result<Vec<ClientAccessGrant>> {
        use chrono::Utc;
        let now = mongodb::bson::DateTime::from_chrono(Utc::now());
        let cursor = self.collection
            .find(doc! {
                "principalId": principal_id,
                "$or": [
                    { "expiresAt": { "$exists": false } },
                    { "expiresAt": null },
                    { "expiresAt": { "$gt": now } }
                ]
            })
            .await?;
        Ok(cursor.try_collect().await?)
    }

    pub async fn delete(&self, id: &str) -> Result<bool> {
        let result = self.collection.delete_one(doc! { "_id": id }).await?;
        Ok(result.deleted_count > 0)
    }

    pub async fn delete_by_principal_and_client(
        &self,
        principal_id: &str,
        client_id: &str,
    ) -> Result<bool> {
        let result = self.collection
            .delete_one(doc! { "principalId": principal_id, "clientId": client_id })
            .await?;
        Ok(result.deleted_count > 0)
    }
}

/// IDP Role Mapping Repository
pub struct IdpRoleMappingRepository {
    collection: Collection<IdpRoleMapping>,
}

impl IdpRoleMappingRepository {
    pub fn new(db: &Database) -> Self {
        Self {
            collection: db.collection("idp_role_mappings"),
        }
    }

    pub async fn insert(&self, mapping: &IdpRoleMapping) -> Result<()> {
        self.collection.insert_one(mapping).await?;
        Ok(())
    }

    pub async fn find_by_id(&self, id: &str) -> Result<Option<IdpRoleMapping>> {
        Ok(self.collection.find_one(doc! { "_id": id }).await?)
    }

    pub async fn find_by_idp_type(&self, idp_type: &str) -> Result<Vec<IdpRoleMapping>> {
        let cursor = self.collection
            .find(doc! { "idpType": idp_type })
            .await?;
        Ok(cursor.try_collect().await?)
    }

    pub async fn find_by_idp_role(
        &self,
        idp_type: &str,
        idp_role_name: &str,
    ) -> Result<Option<IdpRoleMapping>> {
        Ok(self.collection
            .find_one(doc! { "idpType": idp_type, "idpRoleName": idp_role_name })
            .await?)
    }

    pub async fn find_all(&self) -> Result<Vec<IdpRoleMapping>> {
        let cursor = self.collection.find(doc! {}).await?;
        Ok(cursor.try_collect().await?)
    }

    pub async fn update(&self, mapping: &IdpRoleMapping) -> Result<()> {
        self.collection
            .replace_one(doc! { "_id": &mapping.id }, mapping)
            .await?;
        Ok(())
    }

    pub async fn delete(&self, id: &str) -> Result<bool> {
        let result = self.collection.delete_one(doc! { "_id": id }).await?;
        Ok(result.deleted_count > 0)
    }
}
