//! FlowCatalyst Outbox Processor
//!
//! Reads messages from application database outbox tables and dispatches them.
//!
//! ## Modes
//!
//! - **Enhanced Mode (default)**: Sends to FlowCatalyst HTTP API with message group ordering
//! - **SQS Mode**: Publishes directly to SQS (legacy behavior)
//!
//! Supports multiple database backends: SQLite, PostgreSQL, MongoDB.
//!
//! ## Environment Variables
//!
//! | Variable | Default | Description |
//! |----------|---------|-------------|
//! | `FC_OUTBOX_MODE` | `enhanced` | Mode: `enhanced` (HTTP API) or `sqs` (direct SQS) |
//! | `FC_OUTBOX_DB_TYPE` | `postgres` | Database type: `sqlite`, `postgres`, `mongo` |
//! | `FC_OUTBOX_DB_URL` | - | Database connection URL (required) |
//! | `FC_OUTBOX_MONGO_DB` | `flowcatalyst` | MongoDB database name |
//! | `FC_OUTBOX_MONGO_COLLECTION` | `outbox` | MongoDB collection name |
//! | `FC_OUTBOX_POLL_INTERVAL_MS` | `1000` | Poll interval in milliseconds |
//! | `FC_OUTBOX_BATCH_SIZE` | `100` | Max messages per batch (SQS mode) |
//! | `FC_QUEUE_URL` | - | SQS queue URL (required for SQS mode) |
//! | `FC_API_BASE_URL` | `http://localhost:8080` | FlowCatalyst API URL (enhanced mode) |
//! | `FC_API_TOKEN` | - | API Bearer token (optional) |
//! | `FC_MAX_IN_FLIGHT` | `5000` | Max concurrent items (enhanced mode) |
//! | `FC_GLOBAL_BUFFER_SIZE` | `1000` | Buffer capacity (enhanced mode) |
//! | `FC_MAX_CONCURRENT_GROUPS` | `10` | Max concurrent message groups (enhanced mode) |
//! | `FC_METRICS_PORT` | `9090` | Metrics/health port |
//! | `RUST_LOG` | `info` | Log level |

use std::sync::Arc;
use std::time::Duration;
use std::net::SocketAddr;
use anyhow::Result;
use tracing::info;
use tokio::signal;
use tokio::sync::broadcast;
use async_trait::async_trait;

use fc_outbox::{OutboxProcessor, repository::OutboxRepository};
use fc_outbox::{EnhancedOutboxProcessor, EnhancedProcessorConfig};
use fc_outbox::http_dispatcher::HttpDispatcherConfig;
use fc_common::Message;

use sqlx::sqlite::SqlitePoolOptions;
use sqlx::postgres::PgPoolOptions;

fn env_or(key: &str, default: &str) -> String {
    std::env::var(key).unwrap_or_else(|_| default.to_string())
}

fn env_or_parse<T: std::str::FromStr>(key: &str, default: T) -> T {
    std::env::var(key)
        .ok()
        .and_then(|v| v.parse().ok())
        .unwrap_or(default)
}

fn env_required(key: &str) -> Result<String> {
    std::env::var(key).map_err(|_| anyhow::anyhow!("{} environment variable is required", key))
}

#[tokio::main]
async fn main() -> Result<()> {
    fc_common::logging::init_logging("fc-outbox-processor");

    info!("Starting FlowCatalyst Outbox Processor");

    // Configuration
    let mode = env_or("FC_OUTBOX_MODE", "enhanced");
    let db_type = env_or("FC_OUTBOX_DB_TYPE", "postgres");
    let poll_interval_ms: u64 = env_or_parse("FC_OUTBOX_POLL_INTERVAL_MS", 1000);
    let metrics_port: u16 = env_or_parse("FC_METRICS_PORT", 9090);

    // Setup shutdown signal
    let (shutdown_tx, _) = broadcast::channel::<()>(1);

    // Initialize outbox repository
    let outbox_repo = create_outbox_repository(&db_type).await?;
    info!("Outbox repository initialized ({})", db_type);

    // Start processor based on mode
    let processor_handle = match mode.as_str() {
        "sqs" => {
            // Legacy SQS mode
            let batch_size: u32 = env_or_parse("FC_OUTBOX_BATCH_SIZE", 100);
            let queue_url = env_required("FC_QUEUE_URL")?;

            let config = aws_config::load_defaults(aws_config::BehaviorVersion::latest()).await;
            let sqs_client = aws_sdk_sqs::Client::new(&config);
            let publisher = Arc::new(SqsPublisher::new(sqs_client, queue_url.clone()));
            info!("SQS mode: publishing to {}", queue_url);

            let processor = OutboxProcessor::new(
                outbox_repo,
                publisher,
                Duration::from_millis(poll_interval_ms),
                batch_size,
            );

            let mut shutdown_rx = shutdown_tx.subscribe();
            tokio::spawn(async move {
                tokio::select! {
                    _ = processor.start() => {}
                    _ = shutdown_rx.recv() => {
                        info!("Outbox processor shutting down");
                    }
                }
            })
        }
        _ => {
            // Enhanced mode (HTTP API with message group ordering)
            let api_base_url = env_or("FC_API_BASE_URL", "http://localhost:8080");
            let api_token = std::env::var("FC_API_TOKEN").ok();
            let max_in_flight: u64 = env_or_parse("FC_MAX_IN_FLIGHT", 5000);
            let global_buffer_size: usize = env_or_parse("FC_GLOBAL_BUFFER_SIZE", 1000);
            let max_concurrent_groups: usize = env_or_parse("FC_MAX_CONCURRENT_GROUPS", 10);
            let poll_batch_size: u32 = env_or_parse("FC_OUTBOX_BATCH_SIZE", 500);
            let api_batch_size: usize = env_or_parse("FC_API_BATCH_SIZE", 100);

            info!("Enhanced mode: sending to {} with message group ordering", api_base_url);
            info!("  max_in_flight: {}, buffer_size: {}, concurrent_groups: {}",
                max_in_flight, global_buffer_size, max_concurrent_groups);

            let config = EnhancedProcessorConfig {
                poll_interval: Duration::from_millis(poll_interval_ms),
                poll_batch_size,
                api_batch_size,
                max_concurrent_groups,
                global_buffer_size,
                max_in_flight,
                http_config: HttpDispatcherConfig {
                    api_base_url,
                    api_token,
                    ..Default::default()
                },
                ..Default::default()
            };

            let processor = Arc::new(EnhancedOutboxProcessor::new(config, outbox_repo)?);

            let mut shutdown_rx = shutdown_tx.subscribe();
            let processor_clone = Arc::clone(&processor);
            tokio::spawn(async move {
                tokio::select! {
                    _ = processor_clone.start() => {}
                    _ = shutdown_rx.recv() => {
                        processor_clone.stop();
                        info!("Enhanced outbox processor shutting down");
                    }
                }
            })
        }
    };

    // Start metrics server
    let metrics_addr = SocketAddr::from(([0, 0, 0, 0], metrics_port));
    info!("Metrics server listening on http://{}/metrics", metrics_addr);

    let metrics_app = axum::Router::new()
        .route("/metrics", axum::routing::get(metrics_handler))
        .route("/health", axum::routing::get(health_handler))
        .route("/ready", axum::routing::get(ready_handler));

    let metrics_listener = tokio::net::TcpListener::bind(metrics_addr).await?;
    let metrics_handle = {
        let mut shutdown_rx = shutdown_tx.subscribe();
        tokio::spawn(async move {
            axum::serve(metrics_listener, metrics_app)
                .with_graceful_shutdown(async move {
                    let _ = shutdown_rx.recv().await;
                })
                .await
                .ok();
        })
    };

    info!("FlowCatalyst Outbox Processor started (mode: {})", mode);
    info!("Press Ctrl+C to shutdown");

    // Wait for shutdown
    shutdown_signal().await;
    info!("Shutdown signal received...");

    let _ = shutdown_tx.send(());

    let _ = tokio::time::timeout(Duration::from_secs(30), async {
        let _ = processor_handle.await;
        let _ = metrics_handle.await;
    }).await;

    info!("FlowCatalyst Outbox Processor shutdown complete");
    Ok(())
}

async fn create_outbox_repository(db_type: &str) -> Result<Arc<dyn OutboxRepository>> {
    match db_type {
        "sqlite" => {
            let url = env_required("FC_OUTBOX_DB_URL")?;
            let pool = SqlitePoolOptions::new()
                .max_connections(5)
                .connect(&url)
                .await?;
            let repo = fc_outbox::sqlite::SqliteOutboxRepository::new(pool);
            repo.init_schema().await?;
            info!("Using SQLite outbox: {}", url);
            Ok(Arc::new(repo))
        }
        "postgres" => {
            let url = env_required("FC_OUTBOX_DB_URL")?;
            let pool = PgPoolOptions::new()
                .max_connections(10)
                .connect(&url)
                .await?;
            let repo = fc_outbox::postgres::PostgresOutboxRepository::new(pool);
            repo.init_schema().await?;
            info!("Using PostgreSQL outbox");
            Ok(Arc::new(repo))
        }
        "mongo" => {
            let url = env_required("FC_OUTBOX_DB_URL")?;
            let db_name = env_or("FC_OUTBOX_MONGO_DB", "flowcatalyst");
            let client = mongodb::Client::with_uri_str(&url).await?;
            let repo = fc_outbox::mongo::MongoOutboxRepository::new(client, &db_name);
            info!("Using MongoDB outbox: {} (collections: outbox_events, outbox_dispatch_jobs)", db_name);
            Ok(Arc::new(repo))
        }
        other => {
            Err(anyhow::anyhow!("Unknown database type: {}. Use sqlite, postgres, or mongo", other))
        }
    }
}

// SQS Publisher
struct SqsPublisher {
    client: aws_sdk_sqs::Client,
    queue_url: String,
}

impl SqsPublisher {
    fn new(client: aws_sdk_sqs::Client, queue_url: String) -> Self {
        Self { client, queue_url }
    }
}

#[async_trait]
impl fc_outbox::QueuePublisher for SqsPublisher {
    async fn publish(&self, message: Message) -> Result<()> {
        let body = serde_json::to_string(&message)?;

        self.client.send_message()
            .queue_url(&self.queue_url)
            .message_body(body)
            .message_group_id(message.message_group_id.as_deref().unwrap_or("default"))
            .message_deduplication_id(&message.id)
            .send()
            .await
            .map_err(|e| anyhow::anyhow!("SQS send error: {}", e))?;

        Ok(())
    }
}

async fn metrics_handler() -> String {
    "# HELP fc_outbox_up Outbox processor is up\n# TYPE fc_outbox_up gauge\nfc_outbox_up 1\n".to_string()
}

async fn health_handler() -> axum::Json<serde_json::Value> {
    axum::Json(serde_json::json!({
        "status": "UP",
        "version": env!("CARGO_PKG_VERSION")
    }))
}

async fn ready_handler() -> axum::Json<serde_json::Value> {
    axum::Json(serde_json::json!({
        "status": "READY"
    }))
}

async fn shutdown_signal() {
    let ctrl_c = async {
        signal::ctrl_c().await.expect("failed to install Ctrl+C handler");
    };

    #[cfg(unix)]
    let terminate = async {
        signal::unix::signal(signal::unix::SignalKind::terminate())
            .expect("failed to install signal handler")
            .recv()
            .await;
    };

    #[cfg(not(unix))]
    let terminate = std::future::pending::<()>();

    tokio::select! {
        _ = ctrl_c => {},
        _ = terminate => {},
    }
}
