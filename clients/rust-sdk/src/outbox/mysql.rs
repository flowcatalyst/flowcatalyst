//! MySQL 8 outbox driver (sqlx).

use sqlx::{MySql, MySqlConnection, MySqlPool, QueryBuilder};

use super::error::OutboxError;
use super::manager::OutboxDriver;
use super::message::OutboxMessage;
use super::schema::{DEFAULT_TABLE, validate_table_name};

/// Rows per `INSERT` (ten bind parameters each, under MySQL's 65 535).
pub const ROWS_PER_STATEMENT: usize = 1000;

/// Writes outbox rows to a MySQL table created from [`super::schema::MYSQL_DDL`].
#[derive(Debug, Clone)]
pub struct MySqlOutboxDriver {
    pool: MySqlPool,
    table: String,
}

impl MySqlOutboxDriver {
    pub fn new(pool: MySqlPool) -> Self {
        Self {
            pool,
            table: DEFAULT_TABLE.to_owned(),
        }
    }

    pub fn with_table(pool: MySqlPool, table: impl Into<String>) -> Result<Self, OutboxError> {
        let table = table.into();
        validate_table_name(&table)?;
        Ok(Self { pool, table })
    }

    /// Insert rows on the caller's connection or transaction.
    pub async fn insert_in(
        &self,
        conn: &mut MySqlConnection,
        messages: &[OutboxMessage],
    ) -> Result<(), OutboxError> {
        for chunk in messages.chunks(ROWS_PER_STATEMENT) {
            let mut headers = Vec::with_capacity(chunk.len());
            for m in chunk {
                headers.push(m.headers_json()?);
            }
            let mut qb: QueryBuilder<'_, MySql> = QueryBuilder::new(format!(
                "INSERT INTO {} (id, type, message_group, payload, status, created_at, updated_at, \
                 client_id, payload_size, headers) ",
                self.table
            ));
            qb.push_values(chunk.iter().zip(headers), |mut row, (m, h)| {
                row.push_bind(m.id.as_str())
                    .push_bind(m.message_type.as_str())
                    .push_bind(m.message_group.as_deref())
                    .push_bind(m.payload.as_str())
                    .push_bind(m.status_code())
                    .push_bind(m.created_at)
                    .push_bind(m.updated_at)
                    .push_bind(m.client_id.as_str())
                    .push_bind(m.payload_size)
                    .push_bind(h);
            });
            qb.build().execute(&mut *conn).await?;
        }
        Ok(())
    }
}

impl OutboxDriver for MySqlOutboxDriver {
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
