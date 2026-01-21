//! PostgreSQL Outbox Repository Implementation
//!
//! Implements the OutboxRepository trait for PostgreSQL with Java-compatible
//! dual-table support (outbox_events and outbox_dispatch_jobs).

use async_trait::async_trait;
use fc_common::{OutboxItem, OutboxItemType, OutboxStatus};
use crate::repository::{OutboxRepository, OutboxTableConfig};
use anyhow::Result;
use sqlx::{PgPool, Row};
use chrono::{DateTime, Utc};
use std::time::Duration;
use tracing::{info, debug};

/// PostgreSQL implementation of OutboxRepository
pub struct PostgresOutboxRepository {
    pool: PgPool,
    table_config: OutboxTableConfig,
}

impl PostgresOutboxRepository {
    /// Create a new PostgreSQL outbox repository with default table config
    pub fn new(pool: PgPool) -> Self {
        Self {
            pool,
            table_config: OutboxTableConfig::default(),
        }
    }

    /// Create with custom table configuration
    pub fn with_config(pool: PgPool, table_config: OutboxTableConfig) -> Self {
        Self { pool, table_config }
    }

    /// Get the pool reference
    pub fn pool(&self) -> &PgPool {
        &self.pool
    }

    /// Parse a row into an OutboxItem
    fn parse_row(&self, row: &sqlx::postgres::PgRow, item_type: OutboxItemType) -> Result<OutboxItem> {
        let created_at_ts: i64 = row.get("created_at");
        let created_at = DateTime::from_timestamp_millis(created_at_ts)
            .ok_or_else(|| anyhow::anyhow!("Invalid created_at timestamp"))?;

        let updated_at_ts: Option<i64> = row.try_get("updated_at").ok();
        let updated_at = updated_at_ts.and_then(DateTime::from_timestamp_millis);

        let status_code: i32 = row.get("status");
        let status = OutboxStatus::from_code(status_code);

        Ok(OutboxItem {
            id: row.get("id"),
            item_type,
            pool_code: row.try_get("pool_code").ok(),
            mediation_target: row.try_get("mediation_target").ok(),
            message_group: row.try_get("message_group").ok(),
            payload: serde_json::from_str(row.get("payload"))?,
            status,
            retry_count: row.get::<i32, _>("retry_count"),
            error_message: row.try_get("error_message").ok().flatten(),
            created_at,
            updated_at,
        })
    }
}

#[async_trait]
impl OutboxRepository for PostgresOutboxRepository {
    // ========================================================================
    // Core Operations (Java-compatible)
    // ========================================================================

    async fn fetch_pending_by_type(&self, item_type: OutboxItemType, limit: u32) -> Result<Vec<OutboxItem>> {
        let table = self.table_config.table_for_type(item_type);
        let query = format!(
            "SELECT id, pool_code, mediation_target, message_group, payload, status, retry_count, error_message, created_at, updated_at \
             FROM {} WHERE status = $1 ORDER BY created_at ASC LIMIT $2",
            table
        );

        let rows = sqlx::query(&query)
            .bind(OutboxStatus::PENDING.code())
            .bind(limit as i64)
            .fetch_all(&self.pool)
            .await?;

        let mut items = Vec::with_capacity(rows.len());
        for row in &rows {
            items.push(self.parse_row(row, item_type)?);
        }

        debug!(
            table = %table,
            count = items.len(),
            "Fetched pending items"
        );

        Ok(items)
    }

    async fn mark_in_progress(&self, item_type: OutboxItemType, ids: Vec<String>) -> Result<()> {
        if ids.is_empty() {
            return Ok(());
        }

        let table = self.table_config.table_for_type(item_type);
        let now = Utc::now().timestamp_millis();

        let query = format!(
            "UPDATE {} SET status = $1, updated_at = $2 WHERE id = ANY($3)",
            table
        );

        sqlx::query(&query)
            .bind(OutboxStatus::IN_PROGRESS.code())
            .bind(now)
            .bind(&ids)
            .execute(&self.pool)
            .await?;

        debug!(
            table = %table,
            count = ids.len(),
            "Marked items as IN_PROGRESS"
        );

        Ok(())
    }

    async fn mark_with_status(
        &self,
        item_type: OutboxItemType,
        ids: Vec<String>,
        status: OutboxStatus,
        error_message: Option<String>,
    ) -> Result<()> {
        if ids.is_empty() {
            return Ok(());
        }

        let table = self.table_config.table_for_type(item_type);
        let now = Utc::now().timestamp_millis();

        let query = format!(
            "UPDATE {} SET status = $1, error_message = $2, updated_at = $3 WHERE id = ANY($4)",
            table
        );

        sqlx::query(&query)
            .bind(status.code())
            .bind(&error_message)
            .bind(now)
            .bind(&ids)
            .execute(&self.pool)
            .await?;

        debug!(
            table = %table,
            status = ?status,
            count = ids.len(),
            "Marked items with status"
        );

        Ok(())
    }

    async fn increment_retry_count(&self, item_type: OutboxItemType, ids: Vec<String>) -> Result<()> {
        if ids.is_empty() {
            return Ok(());
        }

        let table = self.table_config.table_for_type(item_type);
        let now = Utc::now().timestamp_millis();

        // Increment retry count and reset to PENDING for retry
        let query = format!(
            "UPDATE {} SET retry_count = retry_count + 1, status = $1, updated_at = $2 WHERE id = ANY($3)",
            table
        );

        sqlx::query(&query)
            .bind(OutboxStatus::PENDING.code())
            .bind(now)
            .bind(&ids)
            .execute(&self.pool)
            .await?;

        debug!(
            table = %table,
            count = ids.len(),
            "Incremented retry count and reset to PENDING"
        );

        Ok(())
    }

    async fn fetch_recoverable_items(
        &self,
        item_type: OutboxItemType,
        timeout: Duration,
        limit: u32,
    ) -> Result<Vec<OutboxItem>> {
        let table = self.table_config.table_for_type(item_type);
        let timeout_ms = timeout.as_millis() as i64;
        let cutoff = Utc::now().timestamp_millis() - timeout_ms;

        // Recoverable items: IN_PROGRESS or error states that have been stuck
        let query = format!(
            "SELECT id, pool_code, mediation_target, message_group, payload, status, retry_count, error_message, created_at, updated_at \
             FROM {} WHERE (status = $1 OR status = $2 OR status = $3 OR status = $4 OR status = $5 OR status = $6) \
             AND updated_at < $7 ORDER BY created_at ASC LIMIT $8",
            table
        );

        let rows = sqlx::query(&query)
            .bind(OutboxStatus::IN_PROGRESS.code())
            .bind(OutboxStatus::BAD_REQUEST.code())
            .bind(OutboxStatus::INTERNAL_ERROR.code())
            .bind(OutboxStatus::UNAUTHORIZED.code())
            .bind(OutboxStatus::FORBIDDEN.code())
            .bind(OutboxStatus::GATEWAY_ERROR.code())
            .bind(cutoff)
            .bind(limit as i64)
            .fetch_all(&self.pool)
            .await?;

        let mut items = Vec::with_capacity(rows.len());
        for row in &rows {
            items.push(self.parse_row(row, item_type)?);
        }

        Ok(items)
    }

    async fn reset_recoverable_items(&self, item_type: OutboxItemType, ids: Vec<String>) -> Result<()> {
        if ids.is_empty() {
            return Ok(());
        }

        let table = self.table_config.table_for_type(item_type);
        let now = Utc::now().timestamp_millis();

        let query = format!(
            "UPDATE {} SET status = $1, updated_at = $2 WHERE id = ANY($3)",
            table
        );

        sqlx::query(&query)
            .bind(OutboxStatus::PENDING.code())
            .bind(now)
            .bind(&ids)
            .execute(&self.pool)
            .await?;

        info!(
            table = %table,
            count = ids.len(),
            "Reset recoverable items to PENDING"
        );

        Ok(())
    }

    async fn fetch_stuck_items(
        &self,
        item_type: OutboxItemType,
        timeout: Duration,
        limit: u32,
    ) -> Result<Vec<OutboxItem>> {
        let table = self.table_config.table_for_type(item_type);
        let timeout_ms = timeout.as_millis() as i64;
        let cutoff = Utc::now().timestamp_millis() - timeout_ms;

        // Stuck items: only IN_PROGRESS that have been stuck
        let query = format!(
            "SELECT id, pool_code, mediation_target, message_group, payload, status, retry_count, error_message, created_at, updated_at \
             FROM {} WHERE status = $1 AND updated_at < $2 ORDER BY created_at ASC LIMIT $3",
            table
        );

        let rows = sqlx::query(&query)
            .bind(OutboxStatus::IN_PROGRESS.code())
            .bind(cutoff)
            .bind(limit as i64)
            .fetch_all(&self.pool)
            .await?;

        let mut items = Vec::with_capacity(rows.len());
        for row in &rows {
            items.push(self.parse_row(row, item_type)?);
        }

        Ok(items)
    }

    async fn reset_stuck_items(&self, item_type: OutboxItemType, ids: Vec<String>) -> Result<()> {
        // Same as reset_recoverable_items
        self.reset_recoverable_items(item_type, ids).await
    }

    // ========================================================================
    // Schema Management
    // ========================================================================

    async fn init_schema(&self) -> Result<()> {
        // Create events table
        let events_schema = format!(
            r#"
            CREATE TABLE IF NOT EXISTS {} (
                id TEXT PRIMARY KEY,
                pool_code TEXT,
                mediation_target TEXT,
                message_group TEXT,
                payload JSONB NOT NULL,
                status INTEGER NOT NULL DEFAULT 0,
                retry_count INTEGER NOT NULL DEFAULT 0,
                error_message TEXT,
                created_at BIGINT NOT NULL,
                updated_at BIGINT
            );
            CREATE INDEX IF NOT EXISTS idx_{}_status ON {}(status);
            CREATE INDEX IF NOT EXISTS idx_{}_created_at ON {}(created_at);
            "#,
            self.table_config.events_table,
            self.table_config.events_table.replace('.', "_"),
            self.table_config.events_table,
            self.table_config.events_table.replace('.', "_"),
            self.table_config.events_table,
        );

        sqlx::query(&events_schema)
            .execute(&self.pool)
            .await?;

        // Create dispatch_jobs table
        let dispatch_jobs_schema = format!(
            r#"
            CREATE TABLE IF NOT EXISTS {} (
                id TEXT PRIMARY KEY,
                pool_code TEXT,
                mediation_target TEXT,
                message_group TEXT,
                payload JSONB NOT NULL,
                status INTEGER NOT NULL DEFAULT 0,
                retry_count INTEGER NOT NULL DEFAULT 0,
                error_message TEXT,
                created_at BIGINT NOT NULL,
                updated_at BIGINT
            );
            CREATE INDEX IF NOT EXISTS idx_{}_status ON {}(status);
            CREATE INDEX IF NOT EXISTS idx_{}_created_at ON {}(created_at);
            "#,
            self.table_config.dispatch_jobs_table,
            self.table_config.dispatch_jobs_table.replace('.', "_"),
            self.table_config.dispatch_jobs_table,
            self.table_config.dispatch_jobs_table.replace('.', "_"),
            self.table_config.dispatch_jobs_table,
        );

        sqlx::query(&dispatch_jobs_schema)
            .execute(&self.pool)
            .await?;

        info!(
            events_table = %self.table_config.events_table,
            dispatch_jobs_table = %self.table_config.dispatch_jobs_table,
            "Initialized PostgreSQL outbox schema"
        );

        Ok(())
    }

    fn table_config(&self) -> &OutboxTableConfig {
        &self.table_config
    }
}

#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn test_table_config_default() {
        let config = OutboxTableConfig::default();
        assert_eq!(config.events_table, "outbox_events");
        assert_eq!(config.dispatch_jobs_table, "outbox_dispatch_jobs");
    }

    #[test]
    fn test_table_for_type() {
        let config = OutboxTableConfig::default();
        assert_eq!(config.table_for_type(OutboxItemType::EVENT), "outbox_events");
        assert_eq!(config.table_for_type(OutboxItemType::DISPATCH_JOB), "outbox_dispatch_jobs");
    }
}
