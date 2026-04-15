//! Client Repository — PostgreSQL via SQLx

use async_trait::async_trait;
use sqlx::PgPool;
use sqlx::Postgres;
use chrono::{DateTime, Utc};

use super::entity::{Client, ClientNote, ClientStatus};
use crate::shared::error::Result;
use crate::usecase::unit_of_work::{HasId, PgPersist};

/// Row mapping for tnt_clients table
#[derive(sqlx::FromRow)]
struct ClientRow {
    id: String,
    name: String,
    identifier: String,
    status: String,
    status_reason: Option<String>,
    status_changed_at: Option<DateTime<Utc>>,
    notes: Option<serde_json::Value>,
    created_at: DateTime<Utc>,
    updated_at: DateTime<Utc>,
}

impl From<ClientRow> for Client {
    fn from(r: ClientRow) -> Self {
        let notes: Vec<ClientNote> = r.notes
            .and_then(|v| serde_json::from_value(v).ok())
            .unwrap_or_default();

        Self {
            id: r.id,
            name: r.name,
            identifier: r.identifier,
            status: ClientStatus::from_str(&r.status),
            status_reason: r.status_reason,
            status_changed_at: r.status_changed_at,
            notes,
            created_at: r.created_at,
            updated_at: r.updated_at,
        }
    }
}

pub struct ClientRepository {
    pool: PgPool,
}

impl ClientRepository {
    pub fn new(pool: &PgPool) -> Self {
        Self { pool: pool.clone() }
    }

    pub async fn insert(&self, client: &Client) -> Result<()> {
        let notes_json = serde_json::to_value(&client.notes).unwrap_or_default();
        let now = Utc::now();
        sqlx::query(
            "INSERT INTO tnt_clients (id, name, identifier, status, status_reason, status_changed_at, notes, created_at, updated_at)
             VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9)"
        )
        .bind(&client.id)
        .bind(&client.name)
        .bind(&client.identifier)
        .bind(client.status.as_str())
        .bind(&client.status_reason)
        .bind(client.status_changed_at)
        .bind(&notes_json)
        .bind(now)
        .bind(now)
        .execute(&self.pool)
        .await?;
        Ok(())
    }

    pub async fn find_by_id(&self, id: &str) -> Result<Option<Client>> {
        let row = sqlx::query_as::<_, ClientRow>(
            "SELECT * FROM tnt_clients WHERE id = $1"
        )
        .bind(id)
        .fetch_optional(&self.pool)
        .await?;
        Ok(row.map(Client::from))
    }

    pub async fn find_by_identifier(&self, identifier: &str) -> Result<Option<Client>> {
        let row = sqlx::query_as::<_, ClientRow>(
            "SELECT * FROM tnt_clients WHERE identifier = $1"
        )
        .bind(identifier)
        .fetch_optional(&self.pool)
        .await?;
        Ok(row.map(Client::from))
    }

    pub async fn find_active(&self) -> Result<Vec<Client>> {
        let rows = sqlx::query_as::<_, ClientRow>(
            "SELECT * FROM tnt_clients WHERE status = 'ACTIVE'"
        )
        .fetch_all(&self.pool)
        .await?;
        Ok(rows.into_iter().map(Client::from).collect())
    }

    pub async fn find_all(&self) -> Result<Vec<Client>> {
        let rows = sqlx::query_as::<_, ClientRow>(
            "SELECT * FROM tnt_clients"
        )
        .fetch_all(&self.pool)
        .await?;
        Ok(rows.into_iter().map(Client::from).collect())
    }

    /// Search clients by name or identifier (case-insensitive partial match)
    pub async fn search(&self, term: &str) -> Result<Vec<Client>> {
        let pattern = format!("%{}%", term);
        let rows = sqlx::query_as::<_, ClientRow>(
            "SELECT * FROM tnt_clients WHERE name ILIKE $1 OR identifier ILIKE $1"
        )
        .bind(&pattern)
        .fetch_all(&self.pool)
        .await?;
        Ok(rows.into_iter().map(Client::from).collect())
    }

    pub async fn find_by_status(&self, status: ClientStatus) -> Result<Vec<Client>> {
        let rows = sqlx::query_as::<_, ClientRow>(
            "SELECT * FROM tnt_clients WHERE status = $1"
        )
        .bind(status.as_str())
        .fetch_all(&self.pool)
        .await?;
        Ok(rows.into_iter().map(Client::from).collect())
    }

    pub async fn find_by_ids(&self, ids: &[String]) -> Result<Vec<Client>> {
        if ids.is_empty() {
            return Ok(vec![]);
        }
        let rows = sqlx::query_as::<_, ClientRow>(
            "SELECT * FROM tnt_clients WHERE id = ANY($1)"
        )
        .bind(ids)
        .fetch_all(&self.pool)
        .await?;
        Ok(rows.into_iter().map(Client::from).collect())
    }

    pub async fn exists(&self, id: &str) -> Result<bool> {
        let row: (bool,) = sqlx::query_as(
            "SELECT EXISTS(SELECT 1 FROM tnt_clients WHERE id = $1)"
        )
        .bind(id)
        .fetch_one(&self.pool)
        .await?;
        Ok(row.0)
    }

    pub async fn exists_by_identifier(&self, identifier: &str) -> Result<bool> {
        let row: (bool,) = sqlx::query_as(
            "SELECT EXISTS(SELECT 1 FROM tnt_clients WHERE identifier = $1)"
        )
        .bind(identifier)
        .fetch_one(&self.pool)
        .await?;
        Ok(row.0)
    }

    pub async fn update(&self, client: &Client) -> Result<()> {
        let notes_json = serde_json::to_value(&client.notes).unwrap_or_default();
        sqlx::query(
            "UPDATE tnt_clients SET
                name = $2,
                identifier = $3,
                status = $4,
                status_reason = $5,
                status_changed_at = $6,
                notes = $7,
                updated_at = $8
             WHERE id = $1"
        )
        .bind(&client.id)
        .bind(&client.name)
        .bind(&client.identifier)
        .bind(client.status.as_str())
        .bind(&client.status_reason)
        .bind(client.status_changed_at)
        .bind(&notes_json)
        .bind(Utc::now())
        .execute(&self.pool)
        .await?;
        Ok(())
    }

    pub async fn delete(&self, id: &str) -> Result<bool> {
        let result = sqlx::query("DELETE FROM tnt_clients WHERE id = $1")
            .bind(id)
            .execute(&self.pool)
            .await?;
        Ok(result.rows_affected() > 0)
    }
}

// ── PgPersist implementation ──────────────────────────────────────────────────

impl HasId for Client {
    fn id(&self) -> &str { &self.id }
}

#[async_trait]
impl PgPersist for Client {
    async fn pg_upsert(&self, txn: &mut sqlx::Transaction<'_, Postgres>) -> Result<()> {
        let now = Utc::now();
        let notes_json = serde_json::to_value(&self.notes).unwrap_or_default();
        sqlx::query(
            "INSERT INTO tnt_clients (id, name, identifier, status, status_reason, status_changed_at, notes, created_at, updated_at)
             VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9)
             ON CONFLICT (id) DO UPDATE SET
                name = EXCLUDED.name,
                identifier = EXCLUDED.identifier,
                status = EXCLUDED.status,
                status_reason = EXCLUDED.status_reason,
                status_changed_at = EXCLUDED.status_changed_at,
                notes = EXCLUDED.notes,
                updated_at = EXCLUDED.updated_at"
        )
        .bind(&self.id)
        .bind(&self.name)
        .bind(&self.identifier)
        .bind(self.status.as_str())
        .bind(&self.status_reason)
        .bind(self.status_changed_at)
        .bind(&notes_json)
        .bind(now)
        .bind(now)
        .execute(&mut **txn)
        .await?;
        Ok(())
    }

    async fn pg_delete(&self, txn: &mut sqlx::Transaction<'_, Postgres>) -> Result<()> {
        sqlx::query("DELETE FROM tnt_clients WHERE id = $1")
            .bind(&self.id)
            .execute(&mut **txn)
            .await?;
        Ok(())
    }
}
