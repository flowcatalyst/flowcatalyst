//! `cargo run --example outbox-postgres --features outbox-postgres`
//!
//! Writes an order and its event, a dispatch job and an audit entry in one
//! transaction. The platform's outbox processor (`fcdev outbox`, or inside
//! `fc-server`) then delivers the three rows with the application's own
//! credentials.
//!
//! ```bash
//! export DATABASE_URL=postgres://localhost/orders   # a database you own
//! cargo run --example outbox-postgres --features outbox-postgres
//! ```

use std::env;
use std::error;

use flowcatalyst_sdk::outbox::schema::init_postgres_schema;
use flowcatalyst_sdk::outbox::{
    CreateAuditLogDto, CreateEventDto, DispatchJob, OutboxManager, PgOutboxDriver,
};
use flowcatalyst_sdk::tsid;
use serde::Serialize;

#[derive(Serialize)]
#[serde(rename_all = "camelCase")]
struct OrderPlaced<'a> {
    order_id: &'a str,
    total_cents: u64,
}

#[tokio::main]
async fn main() -> Result<(), Box<dyn error::Error>> {
    let pool = sqlx::PgPool::connect(&env::var("DATABASE_URL")?).await?;
    init_postgres_schema(&pool).await?;
    sqlx::query(
        "CREATE TABLE IF NOT EXISTS orders (id TEXT PRIMARY KEY, total_cents BIGINT NOT NULL)",
    )
    .execute(&pool)
    .await?;

    let driver = PgOutboxDriver::new(pool.clone());
    let outbox = OutboxManager::new(driver.clone(), "clt_0HZXEQ5Y8JY5Z")?;
    let order_id = tsid::generate_with_prefix("ord");

    let event = CreateEventDto::new(
        "shop:orders:order:placed",
        &OrderPlaced {
            order_id: &order_id,
            total_cents: 1500,
        },
    )?
    .source("shop:orders")
    .subject(format!("orders.order.{order_id}"))
    .message_group(format!("orders:order:{order_id}"))
    .client_code("acme");
    let job = DispatchJob::builder(
        "shop:fulfilment:pick",
        "https://warehouse.example.com/hooks/pick",
    )
    .json_payload(&serde_json::json!({ "orderId": order_id }))?
    .message_group(format!("orders:order:{order_id}"))
    .build()?;
    let audit = CreateAuditLogDto::new("Order", &order_id, "PlaceOrder", "prn_0HZXEQ5Y8JY5A")?
        .application_code("shop")
        .client_code("acme");
    let rows = vec![
        outbox.prepare_event(&event)?,
        outbox.prepare_dispatch_job(&job)?,
        outbox.prepare_audit_log(&audit)?,
    ];

    let mut tx = pool.begin().await?;
    sqlx::query("INSERT INTO orders (id, total_cents) VALUES ($1, $2)")
        .bind(&order_id)
        .bind(1500_i64)
        .execute(&mut *tx)
        .await?;
    driver.insert_in(&mut tx, &rows).await?;
    tx.commit().await?;

    println!("order {order_id}: {} outbox rows written", rows.len());
    Ok(())
}
