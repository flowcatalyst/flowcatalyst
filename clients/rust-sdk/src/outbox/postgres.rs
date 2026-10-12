//! PostgreSQL outbox driver (sqlx).

use sqlx::{PgConnection, PgPool, Postgres, QueryBuilder};

use super::error::OutboxError;
use super::manager::OutboxDriver;
use super::message::OutboxMessage;
use super::schema::{DEFAULT_TABLE, validate_table_name};

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
    pub async fn insert_in(
        &self,
        conn: &mut PgConnection,
        messages: &[OutboxMessage],
    ) -> Result<(), OutboxError> {
        for chunk in messages.chunks(ROWS_PER_STATEMENT) {
            self.insert_chunk(&mut *conn, chunk).await?;
        }
        Ok(())
    }

    async fn insert_chunk(
        &self,
        conn: &mut PgConnection,
        messages: &[OutboxMessage],
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
        qb.push_values(messages.iter().zip(headers), |mut row, (m, h)| {
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
