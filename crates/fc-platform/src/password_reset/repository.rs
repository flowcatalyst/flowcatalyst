//! PasswordResetToken Repository — PostgreSQL via SQLx

use sqlx::PgPool;
use chrono::{DateTime, Utc};

use super::entity::PasswordResetToken;
use crate::shared::error::Result;

#[derive(sqlx::FromRow)]
struct PasswordResetTokenRow {
    id: String,
    principal_id: String,
    token_hash: String,
    expires_at: DateTime<Utc>,
    created_at: DateTime<Utc>,
}

impl From<PasswordResetTokenRow> for PasswordResetToken {
    fn from(r: PasswordResetTokenRow) -> Self {
        Self {
            id: r.id,
            principal_id: r.principal_id,
            token_hash: r.token_hash,
            expires_at: r.expires_at,
            created_at: r.created_at,
        }
    }
}

pub struct PasswordResetTokenRepository {
    pool: PgPool,
}

impl PasswordResetTokenRepository {
    pub fn new(pool: &PgPool) -> Self {
        Self { pool: pool.clone() }
    }

    pub async fn create(&self, token: &PasswordResetToken) -> Result<()> {
        sqlx::query(
            r#"INSERT INTO iam_password_reset_tokens
                (id, principal_id, token_hash, expires_at, created_at)
            VALUES ($1, $2, $3, $4, NOW())"#
        )
        .bind(&token.id)
        .bind(&token.principal_id)
        .bind(&token.token_hash)
        .bind(token.expires_at)
        .execute(&self.pool)
        .await?;
        Ok(())
    }

    pub async fn find_by_token_hash(&self, hash: &str) -> Result<Option<PasswordResetToken>> {
        let row = sqlx::query_as::<_, PasswordResetTokenRow>(
            "SELECT * FROM iam_password_reset_tokens WHERE token_hash = $1"
        )
        .bind(hash)
        .fetch_optional(&self.pool)
        .await?;
        Ok(row.map(PasswordResetToken::from))
    }

    pub async fn delete_by_principal_id(&self, principal_id: &str) -> Result<()> {
        sqlx::query("DELETE FROM iam_password_reset_tokens WHERE principal_id = $1")
            .bind(principal_id)
            .execute(&self.pool)
            .await?;
        Ok(())
    }

    pub async fn delete_expired(&self) -> Result<u64> {
        let result = sqlx::query("DELETE FROM iam_password_reset_tokens WHERE expires_at < NOW()")
            .execute(&self.pool)
            .await?;
        Ok(result.rows_affected())
    }

    pub async fn delete_by_id(&self, id: &str) -> Result<()> {
        sqlx::query("DELETE FROM iam_password_reset_tokens WHERE id = $1")
            .bind(id)
            .execute(&self.pool)
            .await?;
        Ok(())
    }
}
