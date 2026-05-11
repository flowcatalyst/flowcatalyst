//! LoginAttempt Repository — PostgreSQL via SQLx

use chrono::{DateTime, Utc};
use sqlx::{PgPool, Postgres, QueryBuilder};

use super::entity::{AttemptType, LoginAttempt, LoginOutcome};
use crate::shared::error::Result;

#[derive(sqlx::FromRow)]
struct LoginAttemptRow {
    id: String,
    attempt_type: String,
    outcome: String,
    failure_reason: Option<String>,
    identifier: Option<String>,
    principal_id: Option<String>,
    ip_address: Option<String>,
    user_agent: Option<String>,
    attempted_at: DateTime<Utc>,
}

impl From<LoginAttemptRow> for LoginAttempt {
    fn from(r: LoginAttemptRow) -> Self {
        Self {
            id: r.id,
            attempt_type: AttemptType::from_str(&r.attempt_type),
            outcome: LoginOutcome::from_str(&r.outcome),
            failure_reason: r.failure_reason,
            identifier: r.identifier,
            principal_id: r.principal_id,
            ip_address: r.ip_address,
            user_agent: r.user_agent,
            attempted_at: r.attempted_at,
        }
    }
}

pub struct LoginAttemptRepository {
    pool: PgPool,
}

impl LoginAttemptRepository {
    pub fn new(pool: &PgPool) -> Self {
        Self { pool: pool.clone() }
    }

    pub async fn create(&self, attempt: &LoginAttempt) -> Result<()> {
        sqlx::query(
            r#"INSERT INTO iam_login_attempts
                (id, attempt_type, outcome, failure_reason, identifier,
                 principal_id, ip_address, user_agent, attempted_at)
            VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9)"#,
        )
        .bind(&attempt.id)
        .bind(attempt.attempt_type.as_str())
        .bind(attempt.outcome.as_str())
        .bind(&attempt.failure_reason)
        .bind(&attempt.identifier)
        .bind(&attempt.principal_id)
        .bind(&attempt.ip_address)
        .bind(&attempt.user_agent)
        .bind(attempt.attempted_at)
        .execute(&self.pool)
        .await?;
        Ok(())
    }

    pub async fn find_paged(
        &self,
        attempt_type: Option<&str>,
        outcome: Option<&str>,
        identifier: Option<&str>,
        principal_id: Option<&str>,
        date_from: Option<&str>,
        date_to: Option<&str>,
        limit: i64,
        offset: i64,
        sort_field: Option<&str>,
        sort_order: Option<&str>,
    ) -> Result<(Vec<LoginAttempt>, u64)> {
        // Parse date bounds up front so failures silently skip that condition.
        let date_from_parsed = date_from
            .and_then(|s| chrono::DateTime::parse_from_rfc3339(s).ok())
            .map(|dt| dt.with_timezone(&Utc));
        let date_to_parsed = date_to
            .and_then(|s| chrono::DateTime::parse_from_rfc3339(s).ok())
            .map(|dt| dt.with_timezone(&Utc));

        fn apply_filters(
            qb: &mut QueryBuilder<Postgres>,
            attempt_type: Option<&str>,
            outcome: Option<&str>,
            identifier: Option<&str>,
            principal_id: Option<&str>,
            date_from: Option<DateTime<Utc>>,
            date_to: Option<DateTime<Utc>>,
        ) {
            let mut has_where = false;
            let push_where = |qb: &mut QueryBuilder<Postgres>, has_where: &mut bool| {
                qb.push(if *has_where { " AND " } else { " WHERE " });
                *has_where = true;
            };

            if let Some(at) = attempt_type {
                push_where(qb, &mut has_where);
                qb.push("attempt_type = ").push_bind(at.to_string());
            }
            if let Some(o) = outcome {
                push_where(qb, &mut has_where);
                qb.push("outcome = ").push_bind(o.to_string());
            }
            if let Some(ident) = identifier {
                push_where(qb, &mut has_where);
                qb.push("identifier = ").push_bind(ident.to_string());
            }
            if let Some(pid) = principal_id {
                push_where(qb, &mut has_where);
                qb.push("principal_id = ").push_bind(pid.to_string());
            }
            if let Some(dt) = date_from {
                push_where(qb, &mut has_where);
                qb.push("attempted_at >= ").push_bind(dt);
            }
            if let Some(dt) = date_to {
                push_where(qb, &mut has_where);
                qb.push("attempted_at <= ").push_bind(dt);
            }
        }

        // Count query
        let mut count_qb: QueryBuilder<Postgres> =
            QueryBuilder::new("SELECT COUNT(*) FROM iam_login_attempts");
        apply_filters(
            &mut count_qb,
            attempt_type,
            outcome,
            identifier,
            principal_id,
            date_from_parsed,
            date_to_parsed,
        );
        let total: i64 = count_qb.build_query_scalar().fetch_one(&self.pool).await?;
        let total = total as u64;

        // Resolve sort column (whitelist to prevent SQL injection — never bound)
        let sort_col = match sort_field {
            Some("identifier") => "identifier",
            Some("outcome") => "outcome",
            Some("attempt_type") => "attempt_type",
            _ => "attempted_at",
        };
        let direction = if matches!(sort_order, Some("asc")) {
            "ASC"
        } else {
            "DESC"
        };

        let mut data_qb: QueryBuilder<Postgres> =
            QueryBuilder::new("SELECT * FROM iam_login_attempts");
        apply_filters(
            &mut data_qb,
            attempt_type,
            outcome,
            identifier,
            principal_id,
            date_from_parsed,
            date_to_parsed,
        );
        data_qb
            .push(" ORDER BY ")
            .push(sort_col)
            .push(" ")
            .push(direction)
            .push(" LIMIT ")
            .push_bind(limit)
            .push(" OFFSET ")
            .push_bind(offset);

        let rows: Vec<LoginAttemptRow> = data_qb.build_query_as().fetch_all(&self.pool).await?;

        Ok((rows.into_iter().map(LoginAttempt::from).collect(), total))
    }

    /// Cursor-paginated variant. Drops the `SELECT COUNT(*)` and the
    /// configurable sort — orders by `(attempted_at, id) DESC` so the keyset
    /// comparison is well-defined. Returns `fetch_limit` rows so the caller
    /// can detect `hasMore`.
    #[allow(clippy::too_many_arguments)]
    pub async fn find_with_cursor(
        &self,
        attempt_type: Option<&str>,
        outcome: Option<&str>,
        identifier: Option<&str>,
        principal_id: Option<&str>,
        date_from: Option<&str>,
        date_to: Option<&str>,
        cursor: Option<&crate::shared::api_common::DecodedCursor>,
        fetch_limit: i64,
    ) -> Result<Vec<LoginAttempt>> {
        let date_from_parsed = date_from
            .and_then(|s| chrono::DateTime::parse_from_rfc3339(s).ok())
            .map(|dt| dt.with_timezone(&Utc));
        let date_to_parsed = date_to
            .and_then(|s| chrono::DateTime::parse_from_rfc3339(s).ok())
            .map(|dt| dt.with_timezone(&Utc));

        let mut qb: QueryBuilder<Postgres> = QueryBuilder::new("SELECT * FROM iam_login_attempts");
        let mut has_where = false;
        let push_where = |qb: &mut QueryBuilder<Postgres>, has_where: &mut bool| {
            qb.push(if *has_where { " AND " } else { " WHERE " });
            *has_where = true;
        };

        if let Some(at) = attempt_type {
            push_where(&mut qb, &mut has_where);
            qb.push("attempt_type = ").push_bind(at.to_string());
        }
        if let Some(o) = outcome {
            push_where(&mut qb, &mut has_where);
            qb.push("outcome = ").push_bind(o.to_string());
        }
        if let Some(ident) = identifier {
            push_where(&mut qb, &mut has_where);
            qb.push("identifier = ").push_bind(ident.to_string());
        }
        if let Some(pid) = principal_id {
            push_where(&mut qb, &mut has_where);
            qb.push("principal_id = ").push_bind(pid.to_string());
        }
        if let Some(dt) = date_from_parsed {
            push_where(&mut qb, &mut has_where);
            qb.push("attempted_at >= ").push_bind(dt);
        }
        if let Some(dt) = date_to_parsed {
            push_where(&mut qb, &mut has_where);
            qb.push("attempted_at <= ").push_bind(dt);
        }
        if let Some(c) = cursor {
            push_where(&mut qb, &mut has_where);
            qb.push("(attempted_at, id) < (")
                .push_bind(c.created_at)
                .push(", ")
                .push_bind(c.id.clone())
                .push(")");
        }
        qb.push(" ORDER BY attempted_at DESC, id DESC LIMIT ")
            .push_bind(fetch_limit);
        let rows: Vec<LoginAttemptRow> = qb.build_query_as().fetch_all(&self.pool).await?;
        Ok(rows.into_iter().map(LoginAttempt::from).collect())
    }

    /// Last successful login attempt timestamp for an identifier. Used by the
    /// backoff helper to compute "failures since the last good login".
    pub async fn last_success_at(&self, identifier: &str) -> Result<Option<DateTime<Utc>>> {
        // MAX(...) over zero rows returns NULL — query_as decodes the single
        // aggregated row, then we treat the NULL as None.
        let (ts,): (Option<DateTime<Utc>>,) = sqlx::query_as(
            "SELECT MAX(attempted_at) FROM iam_login_attempts
             WHERE identifier = $1 AND outcome = 'SUCCESS'",
        )
        .bind(identifier)
        .fetch_one(&self.pool)
        .await?;
        Ok(ts)
    }

    /// Count failures and most-recent failure timestamp for `(identifier, ip)`
    /// strictly after `since`. Returns `(0, None)` when none exist.
    pub async fn failure_stats_by_identifier_ip_since(
        &self,
        identifier: &str,
        ip: &str,
        since: DateTime<Utc>,
    ) -> Result<(i64, Option<DateTime<Utc>>)> {
        let row: (i64, Option<DateTime<Utc>>) = sqlx::query_as(
            "SELECT COUNT(*), MAX(attempted_at) FROM iam_login_attempts
             WHERE identifier = $1 AND ip_address = $2 AND outcome = 'FAILURE'
               AND attempted_at > $3",
        )
        .bind(identifier)
        .bind(ip)
        .bind(since)
        .fetch_one(&self.pool)
        .await?;
        Ok(row)
    }

    /// Count failures across all IPs for `identifier` strictly after `since`.
    /// Used for the per-email global ceiling.
    pub async fn failure_count_by_identifier_since(
        &self,
        identifier: &str,
        since: DateTime<Utc>,
    ) -> Result<i64> {
        let count: (i64,) = sqlx::query_as(
            "SELECT COUNT(*) FROM iam_login_attempts
             WHERE identifier = $1 AND outcome = 'FAILURE' AND attempted_at > $2",
        )
        .bind(identifier)
        .bind(since)
        .fetch_one(&self.pool)
        .await?;
        Ok(count.0)
    }
}
