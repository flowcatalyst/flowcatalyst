//! The canonical `outbox_messages` table.
//!
//! These are the Java SDK's migration files byte for byte
//! (`clients/java-sdk/migrations/{postgresql,mysql}/001_create_outbox_messages.sql`;
//! the Go SDK's `outboxsql` constants describe the same table). The platform's
//! outbox processor reads `id, type, message_group, payload, status,
//! retry_count, created_at, updated_at, error_message` and relies on the two
//! partial indexes; `client_id`, `payload_size` and `headers` are the SDKs'.
//!
//! Run the file with your migration tool, or [`init_postgres_schema`] /
//! [`init_mysql_schema`] for a quick start. An application that already
//! created the table (for example through the Java SDK or the framework's
//! `outbox` module) must not create it again.

/// PostgreSQL DDL, the Java SDK's `001_create_outbox_messages.sql`.
pub const POSTGRES_DDL: &str =
    include_str!("../../migrations/postgresql/001_create_outbox_messages.sql");

/// MySQL 8 DDL, the Java SDK's `001_create_outbox_messages.sql`.
pub const MYSQL_DDL: &str = include_str!("../../migrations/mysql/001_create_outbox_messages.sql");

/// The default table name.
pub const DEFAULT_TABLE: &str = "outbox_messages";

/// Create the table and its indexes unless the table already exists.
#[cfg(feature = "outbox-postgres")]
pub async fn init_postgres_schema(pool: &sqlx::PgPool) -> Result<(), super::OutboxError> {
    let exists: bool = sqlx::query_scalar("SELECT to_regclass('outbox_messages') IS NOT NULL")
        .fetch_one(pool)
        .await?;
    if !exists {
        sqlx::raw_sql(POSTGRES_DDL).execute(pool).await?;
    }
    Ok(())
}

/// Create the table unless it already exists.
#[cfg(feature = "outbox-mysql")]
pub async fn init_mysql_schema(pool: &sqlx::MySqlPool) -> Result<(), super::OutboxError> {
    let count: i64 = sqlx::query_scalar(
        "SELECT COUNT(*) FROM information_schema.tables \
         WHERE table_schema = DATABASE() AND table_name = 'outbox_messages'",
    )
    .fetch_one(pool)
    .await?;
    if count == 0 {
        sqlx::raw_sql(MYSQL_DDL).execute(pool).await?;
    }
    Ok(())
}

/// A plain or schema-qualified SQL identifier (`[A-Za-z_][A-Za-z0-9_]*`, at
/// most one `.`), so a configured table name cannot inject SQL.
#[cfg(any(feature = "outbox-postgres", feature = "outbox-mysql"))]
pub(crate) fn validate_table_name(name: &str) -> Result<(), super::OutboxError> {
    let part_ok = |p: &str| {
        let mut b = p.bytes();
        b.next()
            .is_some_and(|c| c.is_ascii_alphabetic() || c == b'_')
            && b.all(|c| c.is_ascii_alphanumeric() || c == b'_')
            && p.len() <= 63
    };
    let parts: Vec<&str> = name.split('.').collect();
    if parts.len() <= 2 && parts.iter().all(|p| part_ok(p)) {
        Ok(())
    } else {
        Err(super::OutboxError::InvalidTableName(name.to_owned()))
    }
}

#[cfg(test)]
mod tests {
    use super::*;

    /// The column set and the processor's indexes, as the Go `outboxsql`
    /// constants and the Java files have them.
    #[test]
    fn postgres_ddl_is_the_canonical_table() {
        for needle in [
            "CREATE TABLE outbox_messages (",
            "id VARCHAR(26) PRIMARY KEY,",
            "type VARCHAR(20) NOT NULL,",
            "message_group VARCHAR(255),",
            "payload TEXT NOT NULL,",
            "status SMALLINT NOT NULL DEFAULT 0,",
            "retry_count SMALLINT NOT NULL DEFAULT 0,",
            "client_id VARCHAR(26),",
            "payload_size INTEGER,",
            "headers JSONB",
            "CREATE INDEX idx_outbox_messages_pending\n    ON outbox_messages(status, message_group, created_at)\n    WHERE status = 0;",
            "CREATE INDEX idx_outbox_messages_stuck\n    ON outbox_messages(status, created_at)\n    WHERE status = 9;",
            "CREATE INDEX idx_outbox_client_pending\n    ON outbox_messages(client_id, status, created_at);",
        ] {
            assert!(POSTGRES_DDL.contains(needle), "missing {needle:?}");
        }
        assert!(
            !POSTGRES_DDL.contains("JSONB NOT NULL"),
            "payload is TEXT, not JSONB"
        );
    }

    #[test]
    fn mysql_ddl_is_the_canonical_table() {
        for needle in [
            "payload LONGTEXT NOT NULL,",
            "payload_size BIGINT,",
            "headers JSON,",
            "INDEX idx_outbox_messages_pending (status, message_group, created_at),",
        ] {
            assert!(MYSQL_DDL.contains(needle), "missing {needle:?}");
        }
    }

    #[cfg(any(feature = "outbox-postgres", feature = "outbox-mysql"))]
    #[test]
    fn table_names_are_plain_identifiers() {
        for ok in ["outbox_messages", "app.outbox_messages", "_x1"] {
            assert!(validate_table_name(ok).is_ok(), "{ok}");
        }
        for bad in ["", "1x", "a.b.c", "x; DROP TABLE y", "x-y", "\"x\""] {
            assert!(validate_table_name(bad).is_err(), "{bad}");
        }
    }
}
