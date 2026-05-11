//! Audit Log Repository — PostgreSQL via SQLx

use chrono::{DateTime, Utc};
use sqlx::{PgPool, Postgres, QueryBuilder};

use super::entity::AuditLog;
use crate::shared::error::Result;

#[derive(sqlx::FromRow)]
struct AuditLogRow {
    id: String,
    entity_type: String,
    entity_id: String,
    operation: String,
    operation_json: Option<serde_json::Value>,
    principal_id: Option<String>,
    application_id: Option<String>,
    client_id: Option<String>,
    performed_at: DateTime<Utc>,
}

impl From<AuditLogRow> for AuditLog {
    fn from(r: AuditLogRow) -> Self {
        Self {
            id: r.id,
            entity_type: r.entity_type,
            entity_id: r.entity_id,
            operation: r.operation,
            operation_json: r.operation_json,
            principal_id: r.principal_id,
            principal_name: None,
            application_id: r.application_id,
            client_id: r.client_id,
            performed_at: r.performed_at,
        }
    }
}

fn apply_audit_filters(
    qb: &mut QueryBuilder<Postgres>,
    entity_type: Option<&str>,
    entity_id: Option<&str>,
    operation: Option<&str>,
    principal_id: Option<&str>,
) {
    let mut has_where = false;
    let push_where = |qb: &mut QueryBuilder<Postgres>, has_where: &mut bool| {
        qb.push(if *has_where { " AND " } else { " WHERE " });
        *has_where = true;
    };

    if let Some(et) = entity_type {
        push_where(qb, &mut has_where);
        qb.push("entity_type = ").push_bind(et.to_string());
    }
    if let Some(eid) = entity_id {
        push_where(qb, &mut has_where);
        qb.push("entity_id = ").push_bind(eid.to_string());
    }
    if let Some(op) = operation {
        push_where(qb, &mut has_where);
        qb.push("operation = ").push_bind(op.to_string());
    }
    if let Some(pid) = principal_id {
        push_where(qb, &mut has_where);
        qb.push("principal_id = ").push_bind(pid.to_string());
    }
}

pub struct AuditLogRepository {
    pool: PgPool,
}

impl AuditLogRepository {
    pub fn new(pool: &PgPool) -> Self {
        Self { pool: pool.clone() }
    }

    pub async fn insert(&self, log: &AuditLog) -> Result<()> {
        sqlx::query(
            r#"INSERT INTO aud_logs
                (id, entity_type, entity_id, operation, operation_json,
                 principal_id, application_id, client_id, performed_at)
            VALUES ($1, $2, $3, $4, $5, $6, $7, $8, NOW())"#,
        )
        .bind(&log.id)
        .bind(&log.entity_type)
        .bind(&log.entity_id)
        .bind(&log.operation)
        .bind(&log.operation_json)
        .bind(&log.principal_id)
        .bind(&log.application_id)
        .bind(&log.client_id)
        .execute(&self.pool)
        .await?;
        Ok(())
    }

    pub async fn find_by_id(&self, id: &str) -> Result<Option<AuditLog>> {
        let row = sqlx::query_as::<_, AuditLogRow>("SELECT * FROM aud_logs WHERE id = $1")
            .bind(id)
            .fetch_optional(&self.pool)
            .await?;
        Ok(row.map(AuditLog::from))
    }

    pub async fn find_by_entity(
        &self,
        entity_type: &str,
        entity_id: &str,
        limit: i64,
    ) -> Result<Vec<AuditLog>> {
        let rows = sqlx::query_as::<_, AuditLogRow>(
            "SELECT * FROM aud_logs WHERE entity_type = $1 AND entity_id = $2 \
             ORDER BY performed_at DESC LIMIT $3",
        )
        .bind(entity_type)
        .bind(entity_id)
        .bind(limit)
        .fetch_all(&self.pool)
        .await?;
        Ok(rows.into_iter().map(AuditLog::from).collect())
    }

    pub async fn find_by_principal(&self, principal_id: &str, limit: i64) -> Result<Vec<AuditLog>> {
        let rows = sqlx::query_as::<_, AuditLogRow>(
            "SELECT * FROM aud_logs WHERE principal_id = $1 ORDER BY performed_at DESC LIMIT $2",
        )
        .bind(principal_id)
        .bind(limit)
        .fetch_all(&self.pool)
        .await?;
        Ok(rows.into_iter().map(AuditLog::from).collect())
    }

    pub async fn find_recent(&self, limit: i64) -> Result<Vec<AuditLog>> {
        let rows = sqlx::query_as::<_, AuditLogRow>(
            "SELECT * FROM aud_logs ORDER BY performed_at DESC LIMIT $1",
        )
        .bind(limit)
        .fetch_all(&self.pool)
        .await?;
        Ok(rows.into_iter().map(AuditLog::from).collect())
    }

    pub async fn search(
        &self,
        entity_type: Option<&str>,
        entity_id: Option<&str>,
        operation: Option<&str>,
        principal_id: Option<&str>,
        limit: i64,
        offset: i64,
    ) -> Result<Vec<AuditLog>> {
        let mut qb: QueryBuilder<Postgres> = QueryBuilder::new("SELECT * FROM aud_logs");
        apply_audit_filters(&mut qb, entity_type, entity_id, operation, principal_id);
        qb.push(" ORDER BY performed_at DESC LIMIT ")
            .push_bind(limit)
            .push(" OFFSET ")
            .push_bind(offset);

        let rows: Vec<AuditLogRow> = qb.build_query_as().fetch_all(&self.pool).await?;
        Ok(rows.into_iter().map(AuditLog::from).collect())
    }

    pub async fn count_with_filters(
        &self,
        entity_type: Option<&str>,
        entity_id: Option<&str>,
        operation: Option<&str>,
        principal_id: Option<&str>,
    ) -> Result<i64> {
        let mut qb: QueryBuilder<Postgres> = QueryBuilder::new("SELECT COUNT(*) FROM aud_logs");
        apply_audit_filters(&mut qb, entity_type, entity_id, operation, principal_id);
        let count: i64 = qb.build_query_scalar().fetch_one(&self.pool).await?;
        Ok(count)
    }

    /// Cursor-paginated search. Keyset on `(performed_at, id) DESC`. Returns
    /// `fetch_limit` rows so the caller can detect `hasMore`.
    pub async fn search_with_cursor(
        &self,
        entity_type: Option<&str>,
        entity_id: Option<&str>,
        operation: Option<&str>,
        principal_id: Option<&str>,
        cursor: Option<&crate::shared::api_common::DecodedCursor>,
        fetch_limit: i64,
    ) -> Result<Vec<AuditLog>> {
        let mut qb: QueryBuilder<Postgres> = QueryBuilder::new("SELECT * FROM aud_logs");
        apply_audit_filters(&mut qb, entity_type, entity_id, operation, principal_id);
        if let Some(c) = cursor {
            // apply_audit_filters injects WHERE/AND for any filter; if there
            // were none we need WHERE here, otherwise AND.
            let already_has_where = entity_type.is_some()
                || entity_id.is_some()
                || operation.is_some()
                || principal_id.is_some();
            qb.push(if already_has_where {
                " AND "
            } else {
                " WHERE "
            });
            qb.push("(performed_at, id) < (")
                .push_bind(c.created_at)
                .push(", ")
                .push_bind(c.id.clone())
                .push(")");
        }
        qb.push(" ORDER BY performed_at DESC, id DESC LIMIT ")
            .push_bind(fetch_limit);
        let rows: Vec<AuditLogRow> = qb.build_query_as().fetch_all(&self.pool).await?;
        Ok(rows.into_iter().map(AuditLog::from).collect())
    }

    pub async fn find_distinct_entity_types(&self) -> Result<Vec<String>> {
        let rows = sqlx::query_scalar::<_, String>(
            "SELECT DISTINCT entity_type FROM aud_logs ORDER BY entity_type",
        )
        .fetch_all(&self.pool)
        .await?;
        Ok(rows)
    }

    pub async fn find_distinct_application_ids(&self) -> Result<Vec<String>> {
        let rows = sqlx::query_scalar::<_, String>(
            "SELECT DISTINCT application_id FROM aud_logs \
             WHERE application_id IS NOT NULL ORDER BY application_id",
        )
        .fetch_all(&self.pool)
        .await?;
        Ok(rows)
    }

    pub async fn find_distinct_client_ids(&self) -> Result<Vec<String>> {
        let rows = sqlx::query_scalar::<_, String>(
            "SELECT DISTINCT client_id FROM aud_logs \
             WHERE client_id IS NOT NULL ORDER BY client_id",
        )
        .fetch_all(&self.pool)
        .await?;
        Ok(rows)
    }

    pub async fn find_distinct_operations(&self) -> Result<Vec<String>> {
        let rows = sqlx::query_scalar::<_, String>(
            "SELECT DISTINCT operation FROM aud_logs ORDER BY operation",
        )
        .fetch_all(&self.pool)
        .await?;
        Ok(rows)
    }
}
