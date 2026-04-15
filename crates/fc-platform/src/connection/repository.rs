//! Connection Repository — PostgreSQL via SQLx

use async_trait::async_trait;
use sqlx::{PgPool, Postgres, QueryBuilder};
use chrono::{DateTime, Utc};

use super::entity::{Connection, ConnectionStatus};
use crate::shared::error::Result;
use crate::usecase::unit_of_work::{HasId, PgPersist};

/// Row mapping for msg_connections table
#[derive(sqlx::FromRow)]
struct ConnectionRow {
    id: String,
    code: String,
    name: String,
    description: Option<String>,
    external_id: Option<String>,
    status: String,
    service_account_id: String,
    client_id: Option<String>,
    client_identifier: Option<String>,
    created_at: DateTime<Utc>,
    updated_at: DateTime<Utc>,
}

impl From<ConnectionRow> for Connection {
    fn from(r: ConnectionRow) -> Self {
        Self {
            id: r.id,
            code: r.code,
            name: r.name,
            description: r.description,
            external_id: r.external_id,
            status: ConnectionStatus::from_str(&r.status),
            service_account_id: r.service_account_id,
            client_id: r.client_id,
            client_identifier: r.client_identifier,
            created_at: r.created_at,
            updated_at: r.updated_at,
        }
    }
}

pub struct ConnectionRepository {
    pool: PgPool,
}

impl ConnectionRepository {
    pub fn new(pool: &PgPool) -> Self {
        Self { pool: pool.clone() }
    }

    pub async fn insert(&self, conn: &Connection) -> Result<()> {
        let now = Utc::now();
        sqlx::query(
            "INSERT INTO msg_connections (id, code, name, description, external_id, status, service_account_id, client_id, client_identifier, created_at, updated_at)
             VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11)"
        )
        .bind(&conn.id)
        .bind(&conn.code)
        .bind(&conn.name)
        .bind(&conn.description)
        .bind(&conn.external_id)
        .bind(conn.status.as_str())
        .bind(&conn.service_account_id)
        .bind(&conn.client_id)
        .bind(&conn.client_identifier)
        .bind(now)
        .bind(now)
        .execute(&self.pool)
        .await?;
        Ok(())
    }

    pub async fn find_by_id(&self, id: &str) -> Result<Option<Connection>> {
        let row = sqlx::query_as::<_, ConnectionRow>(
            "SELECT * FROM msg_connections WHERE id = $1"
        )
        .bind(id)
        .fetch_optional(&self.pool)
        .await?;
        Ok(row.map(Connection::from))
    }

    pub async fn find_by_code_and_client(&self, code: &str, client_id: Option<&str>) -> Result<Option<Connection>> {
        let row = if let Some(cid) = client_id {
            sqlx::query_as::<_, ConnectionRow>(
                "SELECT * FROM msg_connections WHERE code = $1 AND client_id = $2"
            )
            .bind(code)
            .bind(cid)
            .fetch_optional(&self.pool)
            .await?
        } else {
            sqlx::query_as::<_, ConnectionRow>(
                "SELECT * FROM msg_connections WHERE code = $1 AND client_id IS NULL"
            )
            .bind(code)
            .fetch_optional(&self.pool)
            .await?
        };
        Ok(row.map(Connection::from))
    }

    pub async fn find_all(&self) -> Result<Vec<Connection>> {
        let rows = sqlx::query_as::<_, ConnectionRow>(
            "SELECT * FROM msg_connections ORDER BY code ASC"
        )
        .fetch_all(&self.pool)
        .await?;
        Ok(rows.into_iter().map(Connection::from).collect())
    }

    pub async fn find_with_filters(
        &self,
        client_id: Option<&str>,
        status: Option<&str>,
        service_account_id: Option<&str>,
    ) -> Result<Vec<Connection>> {
        let mut qb: QueryBuilder<Postgres> = QueryBuilder::new("SELECT * FROM msg_connections");
        let mut has_where = false;
        let push_where = |qb: &mut QueryBuilder<Postgres>, has_where: &mut bool| {
            qb.push(if *has_where { " AND " } else { " WHERE " });
            *has_where = true;
        };

        if let Some(v) = client_id {
            push_where(&mut qb, &mut has_where);
            qb.push("client_id = ").push_bind(v.to_string());
        }
        if let Some(v) = status {
            push_where(&mut qb, &mut has_where);
            qb.push("status = ").push_bind(v.to_string());
        }
        if let Some(v) = service_account_id {
            push_where(&mut qb, &mut has_where);
            qb.push("service_account_id = ").push_bind(v.to_string());
        }

        qb.push(" ORDER BY code ASC");
        let rows: Vec<ConnectionRow> = qb.build_query_as().fetch_all(&self.pool).await?;
        Ok(rows.into_iter().map(Connection::from).collect())
    }

    pub async fn find_by_status(&self, status: &str) -> Result<Vec<Connection>> {
        let rows = sqlx::query_as::<_, ConnectionRow>(
            "SELECT * FROM msg_connections WHERE status = $1 ORDER BY code ASC"
        )
        .bind(status)
        .fetch_all(&self.pool)
        .await?;
        Ok(rows.into_iter().map(Connection::from).collect())
    }

    pub async fn find_by_client_id(&self, client_id: &str) -> Result<Vec<Connection>> {
        let rows = sqlx::query_as::<_, ConnectionRow>(
            "SELECT * FROM msg_connections WHERE client_id = $1 ORDER BY code ASC"
        )
        .bind(client_id)
        .fetch_all(&self.pool)
        .await?;
        Ok(rows.into_iter().map(Connection::from).collect())
    }

    pub async fn find_by_service_account(&self, service_account_id: &str) -> Result<Vec<Connection>> {
        let rows = sqlx::query_as::<_, ConnectionRow>(
            "SELECT * FROM msg_connections WHERE service_account_id = $1"
        )
        .bind(service_account_id)
        .fetch_all(&self.pool)
        .await?;
        Ok(rows.into_iter().map(Connection::from).collect())
    }

    pub async fn update(&self, conn: &Connection) -> Result<()> {
        sqlx::query(
            "UPDATE msg_connections SET
                code = $2,
                name = $3,
                description = $4,
                external_id = $5,
                status = $6,
                service_account_id = $7,
                client_id = $8,
                client_identifier = $9,
                updated_at = $10
             WHERE id = $1"
        )
        .bind(&conn.id)
        .bind(&conn.code)
        .bind(&conn.name)
        .bind(&conn.description)
        .bind(&conn.external_id)
        .bind(conn.status.as_str())
        .bind(&conn.service_account_id)
        .bind(&conn.client_id)
        .bind(&conn.client_identifier)
        .bind(Utc::now())
        .execute(&self.pool)
        .await?;
        Ok(())
    }

    pub async fn delete(&self, id: &str) -> Result<bool> {
        let result = sqlx::query("DELETE FROM msg_connections WHERE id = $1")
            .bind(id)
            .execute(&self.pool)
            .await?;
        Ok(result.rows_affected() > 0)
    }
}

impl HasId for Connection {
    fn id(&self) -> &str { &self.id }
}

#[async_trait]
impl PgPersist for Connection {
    async fn pg_upsert(&self, txn: &mut sqlx::Transaction<'_, Postgres>) -> Result<()> {
        let now = Utc::now();
        sqlx::query(
            "INSERT INTO msg_connections (id, code, name, description, external_id, status, service_account_id, client_id, client_identifier, created_at, updated_at)
             VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11)
             ON CONFLICT (id) DO UPDATE SET
                code = EXCLUDED.code,
                name = EXCLUDED.name,
                description = EXCLUDED.description,
                external_id = EXCLUDED.external_id,
                status = EXCLUDED.status,
                service_account_id = EXCLUDED.service_account_id,
                client_id = EXCLUDED.client_id,
                client_identifier = EXCLUDED.client_identifier,
                updated_at = EXCLUDED.updated_at"
        )
        .bind(&self.id)
        .bind(&self.code)
        .bind(&self.name)
        .bind(&self.description)
        .bind(&self.external_id)
        .bind(self.status.as_str())
        .bind(&self.service_account_id)
        .bind(&self.client_id)
        .bind(&self.client_identifier)
        .bind(now)
        .bind(now)
        .execute(&mut **txn)
        .await?;
        Ok(())
    }

    async fn pg_delete(&self, txn: &mut sqlx::Transaction<'_, Postgres>) -> Result<()> {
        sqlx::query("DELETE FROM msg_connections WHERE id = $1")
            .bind(&self.id)
            .execute(&mut **txn)
            .await?;
        Ok(())
    }
}
