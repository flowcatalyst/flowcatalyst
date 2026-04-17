//! ServiceAccount Repository
//!
//! PostgreSQL persistence for ServiceAccount entities using SQLx.
//! Queries through iam_principals (type=SERVICE) as the source of truth,
//! hydrating webhook credentials from iam_service_accounts.
//! This matches the TypeScript implementation.

use async_trait::async_trait;
use sqlx::PgPool;
use chrono::{DateTime, Utc};

use crate::ServiceAccount;
use crate::service_account::entity::{RoleAssignment, WebhookCredentials, WebhookAuthType};
use crate::shared::error::Result;
use crate::usecase::unit_of_work::HasId;

/// Row mapping for iam_principals table (SERVICE type rows)
#[derive(sqlx::FromRow, Clone)]
struct PrincipalRow {
    id: String,
    #[sqlx(rename = "type")]
    #[allow(dead_code)]
    principal_type: String,
    scope: Option<String>,
    #[allow(dead_code)]
    client_id: Option<String>,
    application_id: Option<String>,
    name: String,
    active: bool,
    service_account_id: Option<String>,
    created_at: DateTime<Utc>,
    updated_at: DateTime<Utc>,
}

/// Row mapping for iam_service_accounts table (webhook credentials side)
#[derive(sqlx::FromRow, Clone)]
struct ServiceAccountRow {
    id: String,
    code: String,
    #[allow(dead_code)]
    name: String,
    description: Option<String>,
    #[allow(dead_code)]
    application_id: Option<String>,
    #[allow(dead_code)]
    active: bool,
    wh_auth_type: Option<String>,
    wh_auth_token_ref: Option<String>,
    wh_signing_secret_ref: Option<String>,
    wh_signing_algorithm: Option<String>,
    last_used_at: Option<DateTime<Utc>>,
    #[allow(dead_code)]
    created_at: DateTime<Utc>,
    #[allow(dead_code)]
    updated_at: DateTime<Utc>,
}

/// Row mapping for iam_principal_roles junction table
#[derive(sqlx::FromRow)]
struct PrincipalRoleRow {
    principal_id: String,
    role_name: String,
    assignment_source: Option<String>,
    assigned_at: DateTime<Utc>,
}

pub struct ServiceAccountRepository {
    pool: PgPool,
}

impl ServiceAccountRepository {
    pub fn new(pool: &PgPool) -> Self {
        Self { pool: pool.clone() }
    }

    pub async fn insert(&self, account: &ServiceAccount) -> Result<()> {
        let now = Utc::now();
        let wh = &account.webhook_credentials;
        let sa_id = account.service_account_table_id.as_ref().unwrap_or(&account.id);

        sqlx::query(
            "INSERT INTO iam_service_accounts
                (id, code, name, description, application_id, active,
                 wh_auth_type, wh_auth_token_ref, wh_signing_secret_ref, wh_signing_algorithm,
                 wh_credentials_created_at, wh_credentials_regenerated_at,
                 last_used_at, created_at, updated_at)
             VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, NULL, $12, $13, $14)"
        )
        .bind(sa_id)
        .bind(&account.code)
        .bind(&account.name)
        .bind(&account.description)
        .bind(&account.application_id)
        .bind(account.active)
        .bind(Some(wh.auth_type.as_str()))
        .bind(&wh.token)
        .bind(&wh.signing_secret)
        .bind(&wh.signing_algorithm)
        .bind(Some(now)) // wh_credentials_created_at
        .bind(account.last_used_at)
        .bind(now)
        .bind(now)
        .execute(&self.pool)
        .await?;
        Ok(())
    }

    /// Find by principal ID (the ID returned in API responses).
    pub async fn find_by_id(&self, id: &str) -> Result<Option<ServiceAccount>> {
        let principal = sqlx::query_as::<_, PrincipalRow>(
            "SELECT id, type, scope, client_id, application_id, name, active, \
             service_account_id, created_at, updated_at \
             FROM iam_principals WHERE id = $1"
        )
        .bind(id)
        .fetch_optional(&self.pool)
        .await?;

        match principal {
            Some(p) => self.hydrate(p).await.map(Some),
            None => Ok(None),
        }
    }

    /// Find by service account code.
    pub async fn find_by_code(&self, code: &str) -> Result<Option<ServiceAccount>> {
        // Look up the service_account_id from iam_service_accounts, then find the principal
        let sa = sqlx::query_as::<_, ServiceAccountRow>(
            "SELECT id, code, name, description, application_id, active, \
             wh_auth_type, wh_auth_token_ref, wh_signing_secret_ref, wh_signing_algorithm, \
             last_used_at, created_at, updated_at \
             FROM iam_service_accounts WHERE code = $1"
        )
        .bind(code)
        .fetch_optional(&self.pool)
        .await?;

        match sa {
            Some(sa_row) => {
                let principal = sqlx::query_as::<_, PrincipalRow>(
                    "SELECT id, type, scope, client_id, application_id, name, active, \
                     service_account_id, created_at, updated_at \
                     FROM iam_principals WHERE service_account_id = $1"
                )
                .bind(&sa_row.id)
                .fetch_optional(&self.pool)
                .await?;
                match principal {
                    Some(p) => self.hydrate_with_sa(p, sa_row).await.map(Some),
                    None => Ok(None),
                }
            }
            None => Ok(None),
        }
    }

    /// Find all active service account principals.
    pub async fn find_active(&self) -> Result<Vec<ServiceAccount>> {
        let principals = sqlx::query_as::<_, PrincipalRow>(
            "SELECT id, type, scope, client_id, application_id, name, active, \
             service_account_id, created_at, updated_at \
             FROM iam_principals WHERE type = 'SERVICE' AND active = true"
        )
        .fetch_all(&self.pool)
        .await?;
        self.hydrate_many(principals).await
    }

    /// Find service accounts by application ID.
    pub async fn find_by_application(&self, application_id: &str) -> Result<Vec<ServiceAccount>> {
        let principals = sqlx::query_as::<_, PrincipalRow>(
            "SELECT id, type, scope, client_id, application_id, name, active, \
             service_account_id, created_at, updated_at \
             FROM iam_principals WHERE type = 'SERVICE' AND application_id = $1"
        )
        .bind(application_id)
        .fetch_all(&self.pool)
        .await?;
        self.hydrate_many(principals).await
    }

    /// Find service accounts by client ID.
    pub async fn find_by_client(&self, client_id: &str) -> Result<Vec<ServiceAccount>> {
        let principals = sqlx::query_as::<_, PrincipalRow>(
            "SELECT id, type, scope, client_id, application_id, name, active, \
             service_account_id, created_at, updated_at \
             FROM iam_principals WHERE type = 'SERVICE' AND client_id = $1 AND active = true"
        )
        .bind(client_id)
        .fetch_all(&self.pool)
        .await?;
        self.hydrate_many(principals).await
    }

    /// Find service accounts with a specific role.
    pub async fn find_with_role(&self, role: &str) -> Result<Vec<ServiceAccount>> {
        let principals = sqlx::query_as::<_, PrincipalRow>(
            "SELECT p.id, p.type, p.scope, p.client_id, p.application_id, p.name, p.active, \
             p.service_account_id, p.created_at, p.updated_at \
             FROM iam_principals p
             INNER JOIN iam_principal_roles pr ON pr.principal_id = p.id
             WHERE p.type = 'SERVICE' AND p.active = true AND pr.role_name = $1"
        )
        .bind(role)
        .fetch_all(&self.pool)
        .await?;

        self.hydrate_many(principals).await
    }

    pub async fn update(&self, account: &ServiceAccount) -> Result<()> {
        let now = Utc::now();
        if let Some(ref sa_table_id) = account.service_account_table_id {
            let wh = &account.webhook_credentials;
            sqlx::query(
                "UPDATE iam_service_accounts SET
                    code = $2, name = $3, description = $4, application_id = $5, active = $6,
                    wh_auth_type = $7, wh_auth_token_ref = $8, wh_signing_secret_ref = $9,
                    wh_signing_algorithm = $10, last_used_at = $11, updated_at = $12
                 WHERE id = $1"
            )
            .bind(sa_table_id)
            .bind(&account.code)
            .bind(&account.name)
            .bind(&account.description)
            .bind(&account.application_id)
            .bind(account.active)
            .bind(Some(wh.auth_type.as_str()))
            .bind(&wh.token)
            .bind(&wh.signing_secret)
            .bind(&wh.signing_algorithm)
            .bind(account.last_used_at)
            .bind(now)
            .execute(&self.pool)
            .await?;
        }
        Ok(())
    }

    pub async fn delete(&self, id: &str) -> Result<bool> {
        // Delete the principal (CASCADE will clean up roles)
        let result = sqlx::query("DELETE FROM iam_principals WHERE id = $1")
            .bind(id)
            .execute(&self.pool)
            .await?;
        Ok(result.rows_affected() > 0)
    }

    // ── Hydration ──────────────────────────────────────────────

    /// Hydrate a single principal into a ServiceAccount by loading
    /// webhook credentials from iam_service_accounts and roles from iam_principal_roles.
    async fn hydrate(&self, principal: PrincipalRow) -> Result<ServiceAccount> {
        let sa_row = if let Some(ref sa_id) = principal.service_account_id {
            sqlx::query_as::<_, ServiceAccountRow>(
                "SELECT id, code, name, description, application_id, active, \
                 wh_auth_type, wh_auth_token_ref, wh_signing_secret_ref, wh_signing_algorithm, \
                 last_used_at, created_at, updated_at \
                 FROM iam_service_accounts WHERE id = $1"
            )
            .bind(sa_id)
            .fetch_optional(&self.pool)
            .await?
        } else {
            None
        };
        let roles = self.load_roles(&principal.id).await?;
        Ok(Self::build_service_account_sync(principal, sa_row.as_ref(), roles))
    }

    /// Hydrate when we already have both rows.
    async fn hydrate_with_sa(
        &self,
        principal: PrincipalRow,
        sa_row: ServiceAccountRow,
    ) -> Result<ServiceAccount> {
        let roles = self.load_roles(&principal.id).await?;
        Ok(Self::build_service_account_sync(principal, Some(&sa_row), roles))
    }

    /// Hydrate multiple principals into ServiceAccounts (batch).
    async fn hydrate_many(&self, principals: Vec<PrincipalRow>) -> Result<Vec<ServiceAccount>> {
        if principals.is_empty() {
            return Ok(vec![]);
        }

        let principal_ids: Vec<String> = principals.iter().map(|p| p.id.clone()).collect();

        // Batch-load service account details
        let sa_ids: Vec<String> = principals.iter()
            .filter_map(|p| p.service_account_id.clone())
            .collect();

        let sa_rows: std::collections::HashMap<String, ServiceAccountRow> = if !sa_ids.is_empty() {
            sqlx::query_as::<_, ServiceAccountRow>(
                "SELECT id, code, name, description, application_id, active, \
                 wh_auth_type, wh_auth_token_ref, wh_signing_secret_ref, wh_signing_algorithm, \
                 last_used_at, created_at, updated_at \
                 FROM iam_service_accounts WHERE id = ANY($1)"
            )
            .bind(&sa_ids)
            .fetch_all(&self.pool)
            .await?
            .into_iter()
            .map(|r| (r.id.clone(), r))
            .collect()
        } else {
            std::collections::HashMap::new()
        };

        // Batch-load roles
        let all_roles = sqlx::query_as::<_, PrincipalRoleRow>(
            "SELECT principal_id, role_name, assignment_source, assigned_at \
             FROM iam_principal_roles WHERE principal_id = ANY($1)"
        )
        .bind(&principal_ids)
        .fetch_all(&self.pool)
        .await?;

        let mut role_map: std::collections::HashMap<String, Vec<RoleAssignment>> =
            std::collections::HashMap::new();
        for r in all_roles {
            role_map.entry(r.principal_id.clone()).or_default().push(RoleAssignment {
                role: r.role_name,
                client_id: None,
                assignment_source: r.assignment_source,
                assigned_at: r.assigned_at,
                assigned_by: None,
            });
        }

        // Build ServiceAccount entities
        let results = principals.into_iter().map(|p| {
            let id = p.id.clone();
            let sa_row = p.service_account_id.as_ref()
                .and_then(|sa_id| sa_rows.get(sa_id));
            let roles = role_map.remove(&id).unwrap_or_default();

            Self::build_service_account_sync(p, sa_row, roles)
        }).collect();

        Ok(results)
    }

    /// Synchronous builder (no DB calls).
    fn build_service_account_sync(
        principal: PrincipalRow,
        sa_row: Option<&ServiceAccountRow>,
        roles: Vec<RoleAssignment>,
    ) -> ServiceAccount {
        let webhook_credentials = sa_row.map(|sa| WebhookCredentials {
            auth_type: sa.wh_auth_type.as_deref().map(WebhookAuthType::from_str).unwrap_or_default(),
            token: sa.wh_auth_token_ref.clone(),
            username: None,
            password: None,
            header_name: None,
            signing_secret: sa.wh_signing_secret_ref.clone(),
            signing_algorithm: sa.wh_signing_algorithm.clone(),
            signature_header: None,
        }).unwrap_or_default();

        let code = sa_row.map(|sa| sa.code.clone())
            .unwrap_or_else(|| principal.name.clone());

        ServiceAccount {
            // The principal ID is what gets returned to clients
            id: principal.id,
            code,
            name: principal.name,
            description: sa_row.and_then(|sa| sa.description.clone()),
            active: principal.active,
            client_ids: vec![], // Loaded via iam_client_access_grants if needed
            application_id: principal.application_id,
            scope: principal.scope,
            webhook_credentials,
            roles,
            service_account_table_id: principal.service_account_id,
            last_used_at: sa_row.and_then(|sa| sa.last_used_at),
            created_at: principal.created_at,
            updated_at: principal.updated_at,
        }
    }

    /// Load roles for a principal from the junction table.
    async fn load_roles(&self, principal_id: &str) -> Result<Vec<RoleAssignment>> {
        let rows = sqlx::query_as::<_, PrincipalRoleRow>(
            "SELECT principal_id, role_name, assignment_source, assigned_at \
             FROM iam_principal_roles WHERE principal_id = $1"
        )
        .bind(principal_id)
        .fetch_all(&self.pool)
        .await?;

        Ok(rows.into_iter().map(|m| RoleAssignment {
            role: m.role_name,
            client_id: None,
            assignment_source: m.assignment_source,
            assigned_at: m.assigned_at,
            assigned_by: None,
        }).collect())
    }
}

// ── Persist<ServiceAccount> ──────────────────────────────────────────────────

impl HasId for ServiceAccount {
    fn id(&self) -> &str { &self.id }
}

#[async_trait]
impl crate::usecase::Persist<ServiceAccount> for ServiceAccountRepository {
    async fn persist(&self, sa: &ServiceAccount, tx: &mut crate::usecase::DbTx<'_>) -> Result<()> {
        let now = Utc::now();
        let scope = sa.scope.clone().unwrap_or_else(|| "ANCHOR".to_string());
        let sa_table_id = sa.service_account_table_id.clone().unwrap_or_else(|| sa.id.clone());
        let wh = &sa.webhook_credentials;

        // 1. Upsert iam_principals (SERVICE type principal)
        sqlx::query(
            "INSERT INTO iam_principals (id, principal_type, scope, client_id, application_id, name, active, email, email_domain, idp_type, external_idp_id, password_hash, last_login_at, service_account_id, created_at, updated_at)
             VALUES ($1, 'SERVICE', $2, $3, $4, $5, $6, NULL, NULL, NULL, NULL, NULL, NULL, $7, $8, $9)
             ON CONFLICT (id) DO UPDATE SET
                name = EXCLUDED.name,
                active = EXCLUDED.active,
                client_id = EXCLUDED.client_id,
                application_id = EXCLUDED.application_id,
                updated_at = EXCLUDED.updated_at"
        )
        .bind(&sa.id)
        .bind(&scope)
        .bind(sa.client_ids.first())
        .bind(&sa.application_id)
        .bind(&sa.name)
        .bind(sa.active)
        .bind(Some(&sa.id))
        .bind(sa.created_at)
        .bind(now)
        .execute(&mut **tx.inner).await?;

        // 2. Upsert iam_service_accounts (webhook credentials)
        sqlx::query(
            "INSERT INTO iam_service_accounts (id, code, name, description, application_id, active, wh_auth_type, wh_auth_token_ref, wh_signing_secret_ref, wh_signing_algorithm, wh_credentials_created_at, last_used_at, created_at, updated_at)
             VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14)
             ON CONFLICT (id) DO UPDATE SET
                code = EXCLUDED.code,
                name = EXCLUDED.name,
                description = EXCLUDED.description,
                application_id = EXCLUDED.application_id,
                active = EXCLUDED.active,
                wh_auth_type = EXCLUDED.wh_auth_type,
                wh_auth_token_ref = EXCLUDED.wh_auth_token_ref,
                wh_signing_secret_ref = EXCLUDED.wh_signing_secret_ref,
                wh_signing_algorithm = EXCLUDED.wh_signing_algorithm,
                last_used_at = EXCLUDED.last_used_at,
                updated_at = EXCLUDED.updated_at"
        )
        .bind(&sa_table_id)
        .bind(&sa.code)
        .bind(&sa.name)
        .bind(&sa.description)
        .bind(&sa.application_id)
        .bind(sa.active)
        .bind(Some(wh.auth_type.as_str()))
        .bind(&wh.token)
        .bind(&wh.signing_secret)
        .bind(&wh.signing_algorithm)
        .bind(Some(now))
        .bind(sa.last_used_at)
        .bind(now)
        .bind(now)
        .execute(&mut **tx.inner).await?;

        // 3. Sync roles to iam_principal_roles using the principal ID
        sqlx::query("DELETE FROM iam_principal_roles WHERE principal_id = $1")
            .bind(&sa.id)
            .execute(&mut **tx.inner).await?;
        for r in &sa.roles {
            sqlx::query(
                "INSERT INTO iam_principal_roles (principal_id, role_name, assignment_source, assigned_at)
                 VALUES ($1, $2, $3, $4)"
            )
            .bind(&sa.id)
            .bind(&r.role)
            .bind(&r.assignment_source)
            .bind(r.assigned_at)
            .execute(&mut **tx.inner).await?;
        }

        Ok(())
    }

    async fn delete(&self, sa: &ServiceAccount, tx: &mut crate::usecase::DbTx<'_>) -> Result<()> {
        if let Some(ref sa_id) = sa.service_account_table_id {
            sqlx::query("DELETE FROM iam_service_accounts WHERE id = $1")
                .bind(sa_id)
                .execute(&mut **tx.inner).await?;
        }
        sqlx::query("DELETE FROM iam_principals WHERE id = $1")
            .bind(&sa.id)
            .execute(&mut **tx.inner).await?;
        Ok(())
    }
}
