//! PostgreSQL outbox driver (sqlx).

use sqlx::{PgConnection, PgPool, Postgres, QueryBuilder};

use super::error::OutboxError;
use super::manager::OutboxDriver;
use super::message::OutboxMessage;
use super::push::PUSH_NOTIFY_CHANNEL;
use super::schema::{DEFAULT_TABLE, validate_table_name};
use super::status::OutboxItemType;

/// Rows per `INSERT` (ten bind parameters each, well under PostgreSQL's 65 535).
pub const ROWS_PER_STATEMENT: usize = 1000;

/// Writes outbox rows with sqlx.
///
/// The payload is bound as `TEXT` (the column's type), so the bytes stored
/// are exactly the payload built, and `payload_size` matches them. (fc-sdk
/// cast it to `jsonb`, which re-formats the JSON on the way in.)
///
/// ```no_run
/// # async fn demo(pool: sqlx::PgPool) -> Result<(), Box<dyn std::error::Error>> {
/// use flowcatalyst_sdk::outbox::{CreateEventDto, OutboxManager, PgOutboxDriver};
///
/// let driver = PgOutboxDriver::new(pool.clone());
/// let outbox = OutboxManager::new(driver.clone(), "clt_0HZXEQ5Y8JY5Z")?;
///
/// // Atomic with the business write: one transaction.
/// let mut tx = pool.begin().await?;
/// sqlx::query("UPDATE orders SET status = 'PLACED' WHERE id = $1").bind("ord_1").execute(&mut *tx).await?;
/// let row = outbox.prepare_event(&CreateEventDto::new("shop:orders:order:placed", &serde_json::json!({"id": "ord_1"}))?)?;
/// driver.insert_in(&mut tx, &[row]).await?;
/// tx.commit().await?;
/// # Ok(()) }
/// ```
#[derive(Debug, Clone)]
pub struct PgOutboxDriver {
    pool: PgPool,
    table: String,
}

impl PgOutboxDriver {
    /// A driver for the `outbox_messages` table.
    pub fn new(pool: PgPool) -> Self {
        Self {
            pool,
            table: DEFAULT_TABLE.to_owned(),
        }
    }

    /// A driver for another table of the same shape.
    pub fn with_table(pool: PgPool, table: impl Into<String>) -> Result<Self, OutboxError> {
        let table = table.into();
        validate_table_name(&table)?;
        Ok(Self { pool, table })
    }

    /// Insert rows on the caller's connection or transaction, so they commit
    /// (or roll back) with the caller's own writes. Large batches are split
    /// into statements of [`ROWS_PER_STATEMENT`] rows; pass a transaction to
    /// keep them atomic.
    ///
    /// A `PUSH` row is written with a `pg_notify` on [`PUSH_NOTIFY_CHANNEL`] in
    /// the same statement ([`push_insert_statement`]); the notification is
    /// delivered at the commit, so a processor that listens on the channel
    /// delivers the push at once instead of at its next poll.
    pub async fn insert_in(
        &self,
        conn: &mut PgConnection,
        messages: &[OutboxMessage],
    ) -> Result<(), OutboxError> {
        let mut run: Vec<&OutboxMessage> = Vec::new();
        for m in messages {
            if m.message_type == OutboxItemType::Push {
                self.flush_run(&mut *conn, &mut run).await?;
                self.insert_push(&mut *conn, m).await?;
            } else {
                run.push(m);
            }
        }
        self.flush_run(conn, &mut run).await
    }

    async fn flush_run(
        &self,
        conn: &mut PgConnection,
        run: &mut Vec<&OutboxMessage>,
    ) -> Result<(), OutboxError> {
        for chunk in run.chunks(ROWS_PER_STATEMENT) {
            self.insert_chunk(&mut *conn, chunk).await?;
        }
        run.clear();
        Ok(())
    }

    async fn insert_push(
        &self,
        conn: &mut PgConnection,
        m: &OutboxMessage,
    ) -> Result<(), OutboxError> {
        let sql = push_insert_statement(&self.table);
        sqlx::query(&sql)
            .bind(m.id.as_str())
            .bind(m.message_type.as_str())
            .bind(m.message_group.as_deref())
            .bind(m.payload.as_str())
            .bind(m.status_code())
            .bind(m.created_at)
            .bind(m.updated_at)
            .bind(m.client_id.as_str())
            .bind(m.payload_size)
            .bind(m.headers_json()?)
            .execute(conn)
            .await?;
        Ok(())
    }

    async fn insert_chunk(
        &self,
        conn: &mut PgConnection,
        messages: &[&OutboxMessage],
    ) -> Result<(), OutboxError> {
        let mut headers = Vec::with_capacity(messages.len());
        for m in messages {
            headers.push(m.headers_json()?);
        }
        let mut qb: QueryBuilder<'_, Postgres> = QueryBuilder::new(format!(
            "INSERT INTO {} (id, type, message_group, payload, status, created_at, updated_at, \
             client_id, payload_size, headers) ",
            self.table
        ));
        qb.push_values(messages.iter().copied().zip(headers), |mut row, (m, h)| {
            row.push_bind(m.id.as_str())
                .push_bind(m.message_type.as_str())
                .push_bind(m.message_group.as_deref())
                .push_bind(m.payload.as_str())
                .push_bind(m.status_code())
                .push_bind(m.created_at)
                .push_bind(m.updated_at)
                .push_bind(m.client_id.as_str())
                .push_bind(m.payload_size)
                .push_bind(h)
                .push_unseparated("::jsonb");
        });
        qb.build().execute(conn).await?;
        Ok(())
    }
}

/// The Postgres `INSERT` of a `PUSH` row with the wake-up notification: the
/// shared insert's columns, written in one statement with
/// `pg_notify('fc_outbox_push', '')` (the Go SDK's `PushInsertStatement`).
/// Arguments are the row's ten columns as `$1` to `$10`. `table` must be a plain
/// or schema-qualified identifier.
pub fn push_insert_statement(table: &str) -> String {
    format!(
        "WITH woken AS (SELECT pg_notify('{PUSH_NOTIFY_CHANNEL}', ''))\n\
         INSERT INTO {table} (\n  \
         id, type, message_group, payload, status, created_at, updated_at,\n  \
         client_id, payload_size, headers\n)\n\
         SELECT $1::text, $2::text, $3::text, $4::text, $5::smallint, $6::timestamptz, $7::timestamptz,\n  \
         $8::text, $9::integer, $10::jsonb\n\
         FROM woken"
    )
}

impl OutboxDriver for PgOutboxDriver {
    /// Insert in a transaction of its own: all the rows or none.
    async fn insert(&self, messages: &[OutboxMessage]) -> Result<(), OutboxError> {
        if messages.is_empty() {
            return Ok(());
        }
        let mut tx = self.pool.begin().await?;
        self.insert_in(&mut tx, messages).await?;
        tx.commit().await?;
        Ok(())
    }
}

#[cfg(test)]
mod tests {
    use super::*;

    /// Byte for byte the Go SDK's `outbox.PushInsertStatement`.
    #[test]
    fn the_push_statement_is_the_go_sdks() {
        assert_eq!(
            push_insert_statement("outbox_messages"),
            "WITH woken AS (SELECT pg_notify('fc_outbox_push', ''))\n\
INSERT INTO outbox_messages (\n  id, type, message_group, payload, status, created_at, updated_at,\n  client_id, payload_size, headers\n)\n\
SELECT $1::text, $2::text, $3::text, $4::text, $5::smallint, $6::timestamptz, $7::timestamptz,\n  $8::text, $9::integer, $10::jsonb\n\
FROM woken"
        );
    }
}
