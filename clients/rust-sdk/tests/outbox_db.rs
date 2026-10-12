//! The sqlx outbox drivers against real databases.
//!
//! Skipped unless `FC_SDK_TEST_POSTGRES_URL` / `FC_SDK_TEST_MYSQL_URL` name a
//! **throwaway** database (the tests create and drop `outbox_messages`), e.g.
//! the containers `make test-db` starts.

#![cfg(any(feature = "outbox-postgres", feature = "outbox-mysql"))]
#![allow(
    clippy::unwrap_used,
    clippy::expect_used,
    clippy::panic,
    reason = "test helpers"
)]

use flowcatalyst_sdk::outbox::{
    CreateAuditLogDto, CreateEventDto, DispatchJob, OutboxDriver, OutboxManager, OutboxMessage,
};

fn rows<D: OutboxDriver>(m: &OutboxManager<D>) -> Vec<OutboxMessage> {
    let event = CreateEventDto::new(
        "orders:sales:order:created",
        &serde_json::json!({"name": "Zoë"}),
    )
    .unwrap()
    .message_group("orders:order:1")
    .header("traceparent", "00-abc-01");
    let job = DispatchJob::builder("orders:billing:export", "https://billing.example.com/x")
        .metadata("k", "v")
        .build()
        .unwrap();
    let audit = CreateAuditLogDto::new("Order", "1", "Place", "prn_1").unwrap();
    vec![
        m.prepare_event(&event).unwrap(),
        m.prepare_dispatch_job(&job).unwrap(),
        m.prepare_audit_log(&audit).unwrap(),
    ]
}

#[cfg(feature = "outbox-postgres")]
mod postgres {
    use super::*;
    use flowcatalyst_sdk::outbox::PgOutboxDriver;
    use flowcatalyst_sdk::outbox::schema::init_postgres_schema;
    use sqlx::{PgPool, Row};

    async fn pool() -> Option<PgPool> {
        let url = std::env::var("FC_SDK_TEST_POSTGRES_URL").ok()?;
        let pool = PgPool::connect(&url).await.unwrap();
        sqlx::query("DROP TABLE IF EXISTS outbox_messages")
            .execute(&pool)
            .await
            .unwrap();
        Some(pool)
    }

    #[tokio::test]
    async fn writes_canonical_rows_and_respects_the_callers_transaction() {
        let Some(pool) = pool().await else {
            eprintln!("FC_SDK_TEST_POSTGRES_URL not set; skipped");
            return;
        };
        init_postgres_schema(&pool).await.unwrap();
        init_postgres_schema(&pool).await.unwrap(); // idempotent

        // The column types are the canonical table's.
        let types: Vec<(String, String)> = sqlx::query_as(
            "SELECT column_name::text, data_type::text FROM information_schema.columns \
             WHERE table_name = 'outbox_messages' ORDER BY ordinal_position",
        )
        .fetch_all(&pool)
        .await
        .unwrap();
        let get = |c: &str| types.iter().find(|(n, _)| n == c).map(|(_, t)| t.as_str());
        assert_eq!(get("payload"), Some("text"));
        assert_eq!(get("status"), Some("smallint"));
        assert_eq!(get("id"), Some("character varying"));
        assert_eq!(get("headers"), Some("jsonb"));

        let driver = PgOutboxDriver::new(pool.clone());
        let m = OutboxManager::new(driver.clone(), "clt_0HZXEQ5Y8JY5Z").unwrap();

        // Rolled back with the caller's transaction: nothing written.
        let mut tx = pool.begin().await.unwrap();
        driver.insert_in(&mut tx, &rows(&m)).await.unwrap();
        tx.rollback().await.unwrap();
        let n: i64 = sqlx::query_scalar("SELECT count(*) FROM outbox_messages")
            .fetch_one(&pool)
            .await
            .unwrap();
        assert_eq!(n, 0);

        let written = rows(&m);
        driver.insert(&written).await.unwrap();
        for w in &written {
            let r = sqlx::query(
                "SELECT type, message_group, payload, status, retry_count, client_id, \
                 payload_size, headers::text AS headers FROM outbox_messages WHERE id = $1",
            )
            .bind(w.id.as_str())
            .fetch_one(&pool)
            .await
            .unwrap();
            assert_eq!(r.get::<String, _>("type"), w.message_type.as_str());
            assert_eq!(r.get::<Option<String>, _>("message_group"), w.message_group);
            // TEXT, so the stored bytes are the payload built, unchanged.
            assert_eq!(r.get::<String, _>("payload"), w.payload);
            assert_eq!(r.get::<i16, _>("status"), 0);
            assert_eq!(r.get::<i16, _>("retry_count"), 0);
            assert_eq!(r.get::<String, _>("client_id"), "clt_0HZXEQ5Y8JY5Z");
            assert_eq!(r.get::<i32, _>("payload_size") as usize, w.payload.len());
            assert_eq!(
                r.get::<Option<String>, _>("headers").is_some(),
                w.headers.is_some()
            );
        }
        sqlx::query("DROP TABLE outbox_messages")
            .execute(&pool)
            .await
            .unwrap();
    }

    #[tokio::test]
    async fn table_names_are_validated() {
        // Lazy pool: never connects.
        let pool = PgPool::connect_lazy("postgres://localhost/none").unwrap();
        assert!(PgOutboxDriver::with_table(pool, "x; DROP TABLE y").is_err());
    }
}

#[cfg(feature = "outbox-mysql")]
mod mysql {
    use super::*;
    use flowcatalyst_sdk::outbox::MySqlOutboxDriver;
    use flowcatalyst_sdk::outbox::schema::init_mysql_schema;
    use sqlx::{MySqlPool, Row};

    #[tokio::test]
    async fn writes_canonical_rows() {
        let Ok(url) = std::env::var("FC_SDK_TEST_MYSQL_URL") else {
            eprintln!("FC_SDK_TEST_MYSQL_URL not set; skipped");
            return;
        };
        let pool = MySqlPool::connect(&url).await.unwrap();
        sqlx::query("DROP TABLE IF EXISTS outbox_messages")
            .execute(&pool)
            .await
            .unwrap();
        init_mysql_schema(&pool).await.unwrap();
        init_mysql_schema(&pool).await.unwrap();

        let driver = MySqlOutboxDriver::new(pool.clone());
        let m = OutboxManager::new(driver.clone(), "clt_0HZXEQ5Y8JY5Z").unwrap();
        let mut tx = pool.begin().await.unwrap();
        driver.insert_in(&mut tx, &rows(&m)).await.unwrap();
        tx.rollback().await.unwrap();
        let written = rows(&m);
        driver.insert(&written).await.unwrap();
        let n: i64 = sqlx::query_scalar("SELECT COUNT(*) FROM outbox_messages")
            .fetch_one(&pool)
            .await
            .unwrap();
        assert_eq!(n, 3);
        for w in &written {
            let r = sqlx::query(
                "SELECT type, payload, status, payload_size FROM outbox_messages WHERE id = ?",
            )
            .bind(w.id.as_str())
            .fetch_one(&pool)
            .await
            .unwrap();
            assert_eq!(r.get::<String, _>("type"), w.message_type.as_str());
            assert_eq!(r.get::<String, _>("payload"), w.payload);
            assert_eq!(r.get::<i16, _>("status"), 0);
            assert_eq!(r.get::<i64, _>("payload_size") as usize, w.payload.len());
        }
        sqlx::query("DROP TABLE outbox_messages")
            .execute(&pool)
            .await
            .unwrap();
    }
}
