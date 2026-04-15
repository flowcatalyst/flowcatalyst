//! CORS Origin Repository — PostgreSQL via SQLx

use sqlx::PgPool;
use chrono::{DateTime, Utc};

use super::entity::CorsAllowedOrigin;
use crate::shared::error::Result;

#[derive(sqlx::FromRow)]
struct CorsOriginRow {
    id: String,
    origin: String,
    description: Option<String>,
    created_by: Option<String>,
    created_at: DateTime<Utc>,
    updated_at: DateTime<Utc>,
}

impl From<CorsOriginRow> for CorsAllowedOrigin {
    fn from(r: CorsOriginRow) -> Self {
        Self {
            id: r.id,
            origin: r.origin,
            description: r.description,
            created_by: r.created_by,
            created_at: r.created_at,
            updated_at: r.updated_at,
        }
    }
}

pub struct CorsOriginRepository {
    pool: PgPool,
}

impl CorsOriginRepository {
    pub fn new(pool: &PgPool) -> Self {
        Self { pool: pool.clone() }
    }

    pub async fn find_by_id(&self, id: &str) -> Result<Option<CorsAllowedOrigin>> {
        let row = sqlx::query_as::<_, CorsOriginRow>(
            "SELECT * FROM tnt_cors_allowed_origins WHERE id = $1"
        )
        .bind(id)
        .fetch_optional(&self.pool)
        .await?;
        Ok(row.map(CorsAllowedOrigin::from))
    }

    pub async fn find_by_origin(&self, origin: &str) -> Result<Option<CorsAllowedOrigin>> {
        let row = sqlx::query_as::<_, CorsOriginRow>(
            "SELECT * FROM tnt_cors_allowed_origins WHERE origin = $1"
        )
        .bind(origin)
        .fetch_optional(&self.pool)
        .await?;
        Ok(row.map(CorsAllowedOrigin::from))
    }

    pub async fn find_all(&self) -> Result<Vec<CorsAllowedOrigin>> {
        let rows = sqlx::query_as::<_, CorsOriginRow>(
            "SELECT * FROM tnt_cors_allowed_origins ORDER BY origin"
        )
        .fetch_all(&self.pool)
        .await?;
        Ok(rows.into_iter().map(CorsAllowedOrigin::from).collect())
    }

    pub async fn get_allowed_origins(&self) -> Result<Vec<String>> {
        let rows = sqlx::query_scalar::<_, String>(
            "SELECT origin FROM tnt_cors_allowed_origins"
        )
        .fetch_all(&self.pool)
        .await?;
        Ok(rows)
    }

    pub async fn insert(&self, origin: &CorsAllowedOrigin) -> Result<()> {
        sqlx::query(
            r#"INSERT INTO tnt_cors_allowed_origins
                (id, origin, description, created_by, created_at, updated_at)
            VALUES ($1, $2, $3, $4, NOW(), NOW())"#
        )
        .bind(&origin.id)
        .bind(&origin.origin)
        .bind(&origin.description)
        .bind(&origin.created_by)
        .execute(&self.pool)
        .await?;
        Ok(())
    }

    pub async fn delete(&self, id: &str) -> Result<bool> {
        let result = sqlx::query("DELETE FROM tnt_cors_allowed_origins WHERE id = $1")
            .bind(id)
            .execute(&self.pool)
            .await?;
        Ok(result.rows_affected() > 0)
    }
}
