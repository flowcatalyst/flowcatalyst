//! LoginAttempt Repository — PostgreSQL via SQLx

use sqlx::{PgPool, Postgres, QueryBuilder};
use chrono::{DateTime, Utc};

use super::entity::{LoginAttempt, AttemptType, LoginOutcome};
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
            VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9)"#
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
        let direction = if matches!(sort_order, Some("asc")) { "ASC" } else { "DESC" };

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
}
