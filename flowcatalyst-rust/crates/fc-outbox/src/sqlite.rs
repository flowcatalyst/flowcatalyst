//! SQLite Outbox Repository Implementation
//!
//! Implements the OutboxRepository trait for SQLite with Java-compatible
//! dual-table support (outbox_events and outbox_dispatch_jobs).

use async_trait::async_trait;
use fc_common::{OutboxItem, OutboxItemType, OutboxStatus};
use crate::repository::{OutboxRepository, OutboxTableConfig};
use anyhow::Result;
use sqlx::{SqlitePool, Row};
use chrono::{DateTime, Utc};
use std::time::Duration;
use tracing::{info, debug};

/// SQLite implementation of OutboxRepository
pub struct SqliteOutboxRepository {
    pool: SqlitePool,
    table_config: OutboxTableConfig,
}

impl SqliteOutboxRepository {
    /// Create a new SQLite outbox repository with default table config
    pub fn new(pool: SqlitePool) -> Self {
        Self {
            pool,
            table_config: OutboxTableConfig::default(),
        }
    }

    /// Create with custom table configuration
    pub fn with_config(pool: SqlitePool, table_config: OutboxTableConfig) -> Self {
        Self { pool, table_config }
    }

    /// Get the pool reference
    pub fn pool(&self) -> &SqlitePool {
        &self.pool
    }

    /// Build a query with the appropriate number of placeholders for IN clause
    fn build_in_clause(count: usize) -> String {
        let placeholders: Vec<&str> = (0..count).map(|_| "?").collect();
        placeholders.join(", ")
    }

    /// Parse a row into an OutboxItem
    fn parse_row(&self, row: &sqlx::sqlite::SqliteRow, item_type: OutboxItemType) -> Result<OutboxItem> {
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
impl OutboxRepository for SqliteOutboxRepository {
    async fn fetch_pending_by_type(&self, item_type: OutboxItemType, limit: u32) -> Result<Vec<OutboxItem>> {
        let table = self.table_config.table_for_type(item_type);
        let query = format!(
            "SELECT id, pool_code, mediation_target, message_group, payload, status, retry_count, error_message, created_at, updated_at \
             FROM {} WHERE status = ? ORDER BY created_at ASC LIMIT ?",
            table
        );

        let rows = sqlx::query(&query)
            .bind(OutboxStatus::PENDING.code())
            .bind(limit)
            .fetch_all(&self.pool)
            .await?;

        let mut items = Vec::with_capacity(rows.len());
        for row in &rows {
            items.push(self.parse_row(row, item_type)?);
        }

        debug!(table = %table, count = items.len(), "Fetched pending items");
        Ok(items)
    }

    async fn mark_in_progress(&self, item_type: OutboxItemType, ids: Vec<String>) -> Result<()> {
        if ids.is_empty() {
            return Ok(());
        }

        let table = self.table_config.table_for_type(item_type);
        let now = Utc::now().timestamp_millis();
        let in_clause = Self::build_in_clause(ids.len());

        let query = format!(
            "UPDATE {} SET status = ?, updated_at = ? WHERE id IN ({})",
            table, in_clause
        );

        let mut q = sqlx::query(&query)
            .bind(OutboxStatus::IN_PROGRESS.code())
            .bind(now);
        for id in &ids {
            q = q.bind(id);
        }
        q.execute(&self.pool).await?;

        debug!(table = %table, count = ids.len(), "Marked items as IN_PROGRESS");
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
        let in_clause = Self::build_in_clause(ids.len());

        let query = format!(
            "UPDATE {} SET status = ?, error_message = ?, updated_at = ? WHERE id IN ({})",
            table, in_clause
        );

        let mut q = sqlx::query(&query)
            .bind(status.code())
            .bind(&error_message)
            .bind(now);
        for id in &ids {
            q = q.bind(id);
        }
        q.execute(&self.pool).await?;

        debug!(table = %table, status = ?status, count = ids.len(), "Marked items with status");
        Ok(())
    }

    async fn increment_retry_count(&self, item_type: OutboxItemType, ids: Vec<String>) -> Result<()> {
        if ids.is_empty() {
            return Ok(());
        }

        let table = self.table_config.table_for_type(item_type);
        let now = Utc::now().timestamp_millis();
        let in_clause = Self::build_in_clause(ids.len());

        let query = format!(
            "UPDATE {} SET retry_count = retry_count + 1, status = ?, updated_at = ? WHERE id IN ({})",
            table, in_clause
        );

        let mut q = sqlx::query(&query)
            .bind(OutboxStatus::PENDING.code())
            .bind(now);
        for id in &ids {
            q = q.bind(id);
        }
        q.execute(&self.pool).await?;

        debug!(table = %table, count = ids.len(), "Incremented retry count");
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

        let query = format!(
            "SELECT id, pool_code, mediation_target, message_group, payload, status, retry_count, error_message, created_at, updated_at \
             FROM {} WHERE status IN (?, ?, ?, ?, ?, ?) AND updated_at < ? ORDER BY created_at ASC LIMIT ?",
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
            .bind(limit)
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
        let in_clause = Self::build_in_clause(ids.len());

        let query = format!(
            "UPDATE {} SET status = ?, updated_at = ? WHERE id IN ({})",
            table, in_clause
        );

        let mut q = sqlx::query(&query)
            .bind(OutboxStatus::PENDING.code())
            .bind(now);
        for id in &ids {
            q = q.bind(id);
        }
        q.execute(&self.pool).await?;

        info!(table = %table, count = ids.len(), "Reset recoverable items to PENDING");
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

        let query = format!(
            "SELECT id, pool_code, mediation_target, message_group, payload, status, retry_count, error_message, created_at, updated_at \
             FROM {} WHERE status = ? AND updated_at < ? ORDER BY created_at ASC LIMIT ?",
            table
        );

        let rows = sqlx::query(&query)
            .bind(OutboxStatus::IN_PROGRESS.code())
            .bind(cutoff)
            .bind(limit)
            .fetch_all(&self.pool)
            .await?;

        let mut items = Vec::with_capacity(rows.len());
        for row in &rows {
            items.push(self.parse_row(row, item_type)?);
        }
        Ok(items)
    }

    async fn reset_stuck_items(&self, item_type: OutboxItemType, ids: Vec<String>) -> Result<()> {
        self.reset_recoverable_items(item_type, ids).await
    }

    async fn init_schema(&self) -> Result<()> {
        // Create events table
        let events_schema = format!(
            r#"
            CREATE TABLE IF NOT EXISTS {} (
                id TEXT PRIMARY KEY,
                pool_code TEXT,
                mediation_target TEXT,
                message_group TEXT,
                payload TEXT NOT NULL,
                status INTEGER NOT NULL DEFAULT 0,
                retry_count INTEGER NOT NULL DEFAULT 0,
                error_message TEXT,
                created_at INTEGER NOT NULL,
                updated_at INTEGER
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
                payload TEXT NOT NULL,
                status INTEGER NOT NULL DEFAULT 0,
                retry_count INTEGER NOT NULL DEFAULT 0,
                error_message TEXT,
                created_at INTEGER NOT NULL,
                updated_at INTEGER
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
            "Initialized SQLite outbox schema"
        );

        Ok(())
    }

    fn table_config(&self) -> &OutboxTableConfig {
        &self.table_config
    }
}
