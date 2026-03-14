//! ServiceAccount Repository
//!
//! PostgreSQL persistence for ServiceAccount entities using SeaORM.
//! Queries through iam_principals (type=SERVICE) as the source of truth,
//! hydrating webhook credentials from iam_service_accounts.
//! This matches the TypeScript implementation.

use async_trait::async_trait;
use sea_orm::*;
use sea_orm::sea_query::OnConflict;
use chrono::Utc;

use crate::ServiceAccount;
use crate::service_account::entity::RoleAssignment;
use crate::entities::{iam_service_accounts, iam_principals, iam_principal_roles};
use crate::shared::error::Result;
use crate::usecase::unit_of_work::{HasId, PgPersist};
use crate::principal::repository::PrincipalRepository;

pub struct ServiceAccountRepository {
    db: DatabaseConnection,
}

impl ServiceAccountRepository {
    pub fn new(db: &DatabaseConnection) -> Self {
        Self { db: db.clone() }
    }

    pub async fn insert(&self, account: &ServiceAccount) -> Result<()> {
        let model = Self::build_sa_active_model(account, true);
        iam_service_accounts::Entity::insert(model)
            .exec(&self.db)
            .await?;
        Ok(())
    }

    /// Find by principal ID (the ID returned in API responses).
    pub async fn find_by_id(&self, id: &str) -> Result<Option<ServiceAccount>> {
        let principal = iam_principals::Entity::find_by_id(id)
            .one(&self.db)
            .await?;
        match principal {
            Some(p) => self.hydrate(p).await.map(Some),
            None => Ok(None),
        }
    }

    /// Find by service account code.
    pub async fn find_by_code(&self, code: &str) -> Result<Option<ServiceAccount>> {
        // Look up the service_account_id from iam_service_accounts, then find the principal
        let sa = iam_service_accounts::Entity::find()
            .filter(iam_service_accounts::Column::Code.eq(code))
            .one(&self.db)
            .await?;
        match sa {
            Some(sa_model) => {
                let principal = iam_principals::Entity::find()
                    .filter(iam_principals::Column::ServiceAccountId.eq(&sa_model.id))
                    .one(&self.db)
                    .await?;
                match principal {
                    Some(p) => self.hydrate_with_sa(p, sa_model).await.map(Some),
                    None => Ok(None),
                }
            }
            None => Ok(None),
        }
    }

    /// Find all active service account principals.
    pub async fn find_active(&self) -> Result<Vec<ServiceAccount>> {
        let principals = iam_principals::Entity::find()
            .filter(iam_principals::Column::PrincipalType.eq("SERVICE"))
            .filter(iam_principals::Column::Active.eq(true))
            .all(&self.db)
            .await?;
        self.hydrate_many(principals).await
    }

    /// Find service accounts by application ID.
    pub async fn find_by_application(&self, application_id: &str) -> Result<Vec<ServiceAccount>> {
        let principals = iam_principals::Entity::find()
            .filter(iam_principals::Column::PrincipalType.eq("SERVICE"))
            .filter(iam_principals::Column::ApplicationId.eq(application_id))
            .all(&self.db)
            .await?;
        self.hydrate_many(principals).await
    }

    /// Find service accounts by client ID.
    pub async fn find_by_client(&self, client_id: &str) -> Result<Vec<ServiceAccount>> {
        let principals = iam_principals::Entity::find()
            .filter(iam_principals::Column::PrincipalType.eq("SERVICE"))
            .filter(iam_principals::Column::ClientId.eq(client_id))
            .filter(iam_principals::Column::Active.eq(true))
            .all(&self.db)
            .await?;
        self.hydrate_many(principals).await
    }

    /// Find service accounts with a specific role.
    pub async fn find_with_role(&self, role: &str) -> Result<Vec<ServiceAccount>> {
        let principal_ids: Vec<String> = iam_principal_roles::Entity::find()
            .filter(iam_principal_roles::Column::RoleName.eq(role))
            .all(&self.db)
            .await?
            .into_iter()
            .map(|pr| pr.principal_id)
            .collect();

        if principal_ids.is_empty() {
            return Ok(vec![]);
        }

        let principals = iam_principals::Entity::find()
            .filter(iam_principals::Column::Id.is_in(principal_ids))
            .filter(iam_principals::Column::PrincipalType.eq("SERVICE"))
            .filter(iam_principals::Column::Active.eq(true))
            .all(&self.db)
            .await?;
        self.hydrate_many(principals).await
    }

    pub async fn update(&self, account: &ServiceAccount) -> Result<()> {
        if let Some(ref sa_table_id) = account.service_account_table_id {
            let model = Self::build_sa_active_model(account, false);
            // Use the iam_service_accounts.id for the update
            let mut model = model;
            model.id = Set(sa_table_id.clone());
            iam_service_accounts::Entity::update(model)
                .exec(&self.db)
                .await?;
        }
        Ok(())
    }

    pub async fn delete(&self, id: &str) -> Result<bool> {
        // Delete the principal (CASCADE will clean up roles)
        let result = iam_principals::Entity::delete_by_id(id)
            .exec(&self.db)
            .await?;
        Ok(result.rows_affected > 0)
    }

    // ── Hydration ──────────────────────────────────────────────

    /// Hydrate a single principal into a ServiceAccount by loading
    /// webhook credentials from iam_service_accounts and roles from iam_principal_roles.
    async fn hydrate(&self, principal: iam_principals::Model) -> Result<ServiceAccount> {
        let sa_model = if let Some(ref sa_id) = principal.service_account_id {
            iam_service_accounts::Entity::find_by_id(sa_id)
                .one(&self.db)
                .await?
        } else {
            None
        };
        self.build_service_account(principal, sa_model).await
    }

    /// Hydrate when we already have both models.
    async fn hydrate_with_sa(
        &self,
        principal: iam_principals::Model,
        sa_model: iam_service_accounts::Model,
    ) -> Result<ServiceAccount> {
        self.build_service_account(principal, Some(sa_model)).await
    }

    /// Hydrate multiple principals into ServiceAccounts (batch).
    async fn hydrate_many(&self, principals: Vec<iam_principals::Model>) -> Result<Vec<ServiceAccount>> {
        if principals.is_empty() {
            return Ok(vec![]);
        }

        let principal_ids: Vec<String> = principals.iter().map(|p| p.id.clone()).collect();

        // Batch-load service account details
        let sa_ids: Vec<String> = principals.iter()
            .filter_map(|p| p.service_account_id.clone())
            .collect();

        let sa_models: std::collections::HashMap<String, iam_service_accounts::Model> = if !sa_ids.is_empty() {
            iam_service_accounts::Entity::find()
                .filter(iam_service_accounts::Column::Id.is_in(sa_ids))
                .all(&self.db)
                .await?
                .into_iter()
                .map(|m| (m.id.clone(), m))
                .collect()
        } else {
            std::collections::HashMap::new()
        };

        // Batch-load roles
        let all_roles = iam_principal_roles::Entity::find()
            .filter(iam_principal_roles::Column::PrincipalId.is_in(principal_ids))
            .all(&self.db)
            .await?;

        let mut role_map: std::collections::HashMap<String, Vec<RoleAssignment>> =
            std::collections::HashMap::new();
        for r in all_roles {
            role_map.entry(r.principal_id.clone()).or_default().push(RoleAssignment {
                role: r.role_name,
                client_id: None,
                assignment_source: r.assignment_source,
                assigned_at: r.assigned_at.naive_utc().and_utc(),
                assigned_by: None,
            });
        }

        // Build ServiceAccount entities
        let results = principals.into_iter().map(|p| {
            let id = p.id.clone();
            let sa_model = p.service_account_id.as_ref()
                .and_then(|sa_id| sa_models.get(sa_id));
            let roles = role_map.remove(&id).unwrap_or_default();

            Self::build_service_account_sync(p, sa_model, roles)
        }).collect();

        Ok(results)
    }

    /// Build a ServiceAccount from principal + optional service account model + roles.
    async fn build_service_account(
        &self,
        principal: iam_principals::Model,
        sa_model: Option<iam_service_accounts::Model>,
    ) -> Result<ServiceAccount> {
        let roles = self.load_roles(&principal.id).await?;
        Ok(Self::build_service_account_sync(principal, sa_model.as_ref(), roles))
    }

    /// Synchronous builder (no DB calls).
    fn build_service_account_sync(
        principal: iam_principals::Model,
        sa_model: Option<&iam_service_accounts::Model>,
        roles: Vec<RoleAssignment>,
    ) -> ServiceAccount {
        use crate::service_account::entity::{WebhookCredentials, WebhookAuthType};

        let webhook_credentials = sa_model.map(|sa| WebhookCredentials {
            auth_type: sa.wh_auth_type.as_deref().map(WebhookAuthType::from_str).unwrap_or_default(),
            token: sa.wh_auth_token_ref.clone(),
            username: None,
            password: None,
            header_name: None,
            signing_secret: sa.wh_signing_secret_ref.clone(),
            signing_algorithm: sa.wh_signing_algorithm.clone(),
            signature_header: None,
        }).unwrap_or_default();

        let code = sa_model.map(|sa| sa.code.clone())
            .unwrap_or_else(|| principal.name.clone());

        ServiceAccount {
            // The principal ID is what gets returned to clients
            id: principal.id,
            code,
            name: principal.name,
            description: sa_model.and_then(|sa| sa.description.clone()),
            active: principal.active,
            client_ids: vec![], // Loaded via iam_client_access_grants if needed
            application_id: principal.application_id,
            scope: principal.scope,
            webhook_credentials,
            roles,
            service_account_table_id: principal.service_account_id,
            last_used_at: sa_model.and_then(|sa| sa.last_used_at.map(|dt| dt.naive_utc().and_utc())),
            created_at: principal.created_at.naive_utc().and_utc(),
            updated_at: principal.updated_at.naive_utc().and_utc(),
        }
    }

    /// Load roles for a principal from the junction table.
    async fn load_roles(&self, principal_id: &str) -> Result<Vec<RoleAssignment>> {
        let role_models = iam_principal_roles::Entity::find()
            .filter(iam_principal_roles::Column::PrincipalId.eq(principal_id))
            .all(&self.db)
            .await?;

        Ok(role_models.into_iter().map(|m| RoleAssignment {
            role: m.role_name,
            client_id: None,
            assignment_source: m.assignment_source,
            assigned_at: m.assigned_at.naive_utc().and_utc(),
            assigned_by: None,
        }).collect())
    }

    fn build_sa_active_model(account: &ServiceAccount, is_insert: bool) -> iam_service_accounts::ActiveModel {
        let wh = &account.webhook_credentials;
        let sa_id = account.service_account_table_id.as_ref()
            .unwrap_or(&account.id);

        iam_service_accounts::ActiveModel {
            id: Set(sa_id.clone()),
            code: Set(account.code.clone()),
            name: Set(account.name.clone()),
            description: Set(account.description.clone()),
            application_id: Set(account.application_id.clone()),
            active: Set(account.active),
            wh_auth_type: Set(Some(wh.auth_type.as_str().to_string())),
            wh_auth_token_ref: Set(wh.token.clone()),
            wh_signing_secret_ref: Set(wh.signing_secret.clone()),
            wh_signing_algorithm: Set(wh.signing_algorithm.clone()),
            wh_credentials_created_at: if is_insert {
                Set(Some(Utc::now().into()))
            } else {
                NotSet
            },
            wh_credentials_regenerated_at: NotSet,
            last_used_at: Set(account.last_used_at.map(|dt| dt.into())),
            created_at: if is_insert { Set(Utc::now().into()) } else { NotSet },
            updated_at: Set(Utc::now().into()),
        }
    }
}

// ── PgPersist implementation ──────────────────────────────────────────────────

impl HasId for ServiceAccount {
    fn id(&self) -> &str { &self.id }
}

#[async_trait]
impl PgPersist for ServiceAccount {
    async fn pg_upsert(&self, txn: &sea_orm::DatabaseTransaction) -> Result<()> {
        // Upsert iam_service_accounts (webhook credentials)
        if self.service_account_table_id.is_some() {
            let model = ServiceAccountRepository::build_sa_active_model(self, true);
            iam_service_accounts::Entity::insert(model)
                .on_conflict(
                    OnConflict::column(iam_service_accounts::Column::Id)
                        .update_columns([
                            iam_service_accounts::Column::Code,
                            iam_service_accounts::Column::Name,
                            iam_service_accounts::Column::Description,
                            iam_service_accounts::Column::ApplicationId,
                            iam_service_accounts::Column::Active,
                            iam_service_accounts::Column::WhAuthType,
                            iam_service_accounts::Column::WhAuthTokenRef,
                            iam_service_accounts::Column::WhSigningSecretRef,
                            iam_service_accounts::Column::WhSigningAlgorithm,
                            iam_service_accounts::Column::LastUsedAt,
                            iam_service_accounts::Column::UpdatedAt,
                        ])
                        .to_owned(),
                )
                .exec(txn)
                .await?;
        }

        // Sync roles to iam_principal_roles using the principal ID (self.id)
        iam_principal_roles::Entity::delete_many()
            .filter(iam_principal_roles::Column::PrincipalId.eq(&self.id))
            .exec(txn)
            .await?;
        PrincipalRepository::insert_roles_txn(&self.id, &self.roles, txn).await?;

        Ok(())
    }

    async fn pg_delete(&self, txn: &sea_orm::DatabaseTransaction) -> Result<()> {
        // Delete service account details
        if let Some(ref sa_id) = self.service_account_table_id {
            iam_service_accounts::Entity::delete_by_id(sa_id).exec(txn).await?;
        }
        // Delete principal (CASCADE handles roles)
        iam_principals::Entity::delete_by_id(&self.id).exec(txn).await?;
        Ok(())
    }
}
