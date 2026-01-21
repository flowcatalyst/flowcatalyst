//! FlowCatalyst Stream Processor
//!
//! Watches MongoDB change streams for new events and:
//! - Matches events to subscriptions
//! - Creates dispatch jobs
//! - Builds read projections
//!
//! ## Environment Variables
//!
//! | Variable | Default | Description |
//! |----------|---------|-------------|
//! | `FC_MONGO_URL` | `mongodb://localhost:27017` | MongoDB connection URL |
//! | `FC_MONGO_DB` | `flowcatalyst` | MongoDB database name |
//! | `FC_METRICS_PORT` | `9090` | Metrics/health port |
//! | `FC_STREAM_BATCH_SIZE` | `100` | Max events to process per batch |
//! | `RUST_LOG` | `info` | Log level |

use std::sync::Arc;
use std::net::SocketAddr;
use std::time::Duration;
use anyhow::Result;
use tracing::{info, warn, error};
use tokio::signal;
use tokio::sync::broadcast;
use futures::StreamExt;
use mongodb::bson::doc;

use fc_platform::repository::{
    EventRepository, EventTypeRepository, DispatchJobRepository,
    SubscriptionRepository, ClientRepository, ApplicationRepository,
};
use fc_platform::domain::{Event, DispatchJob};

fn env_or(key: &str, default: &str) -> String {
    std::env::var(key).unwrap_or_else(|_| default.to_string())
}

fn env_or_parse<T: std::str::FromStr>(key: &str, default: T) -> T {
    std::env::var(key)
        .ok()
        .and_then(|v| v.parse().ok())
        .unwrap_or(default)
}

#[tokio::main]
async fn main() -> Result<()> {
    fc_common::logging::init_logging("fc-stream-processor");

    info!("Starting FlowCatalyst Stream Processor");

    // Configuration
    let mongo_url = env_or("FC_MONGO_URL", "mongodb://localhost:27017");
    let mongo_db = env_or("FC_MONGO_DB", "flowcatalyst");
    let metrics_port: u16 = env_or_parse("FC_METRICS_PORT", 9090);
    let _batch_size: usize = env_or_parse("FC_STREAM_BATCH_SIZE", 100);

    // Setup shutdown signal
    let (shutdown_tx, _) = broadcast::channel::<()>(1);

    // Connect to MongoDB
    info!("Connecting to MongoDB: {}/{}", mongo_url, mongo_db);
    let mongo_client = mongodb::Client::with_uri_str(&mongo_url).await?;
    let db = mongo_client.database(&mongo_db);

    // Initialize repositories
    let event_repo = Arc::new(EventRepository::new(&db));
    let _event_type_repo = Arc::new(EventTypeRepository::new(&db));
    let dispatch_job_repo = Arc::new(DispatchJobRepository::new(&db));
    let subscription_repo = Arc::new(SubscriptionRepository::new(&db));
    let _client_repo = Arc::new(ClientRepository::new(&db));
    let _application_repo = Arc::new(ApplicationRepository::new(&db));
    info!("Repositories initialized");

    // Start change stream watcher
    let stream_handle = {
        let mut shutdown_rx = shutdown_tx.subscribe();
        let event_repo = event_repo.clone();
        let dispatch_job_repo = dispatch_job_repo.clone();
        let subscription_repo = subscription_repo.clone();
        let db = db.clone();

        tokio::spawn(async move {
            tokio::select! {
                result = watch_events(db, event_repo, dispatch_job_repo, subscription_repo) => {
                    if let Err(e) = result {
                        error!("Change stream error: {}", e);
                    }
                }
                _ = shutdown_rx.recv() => {
                    info!("Stream processor shutting down");
                }
            }
        })
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

    info!("FlowCatalyst Stream Processor started");
    info!("Press Ctrl+C to shutdown");

    // Wait for shutdown
    shutdown_signal().await;
    info!("Shutdown signal received...");

    let _ = shutdown_tx.send(());

    let _ = tokio::time::timeout(Duration::from_secs(30), async {
        let _ = stream_handle.await;
        let _ = metrics_handle.await;
    }).await;

    info!("FlowCatalyst Stream Processor shutdown complete");
    Ok(())
}

async fn watch_events(
    db: mongodb::Database,
    _event_repo: Arc<EventRepository>,
    dispatch_job_repo: Arc<DispatchJobRepository>,
    subscription_repo: Arc<SubscriptionRepository>,
) -> Result<()> {
    let collection = db.collection::<Event>("events");

    // Watch for inserts on events collection
    let pipeline = vec![
        doc! { "$match": { "operationType": "insert" } }
    ];

    info!("Starting change stream on events collection");

    let mut change_stream = collection.watch().pipeline(pipeline).await?;

    while let Some(change) = change_stream.next().await {
        match change {
            Ok(change_event) => {
                if let Some(full_doc) = change_event.full_document {
                    if let Err(e) = process_event(&full_doc, &dispatch_job_repo, &subscription_repo).await {
                        error!("Error processing event {}: {}", full_doc.id, e);
                    }
                }
            }
            Err(e) => {
                warn!("Change stream error: {}", e);
                // Sleep before retry
                tokio::time::sleep(Duration::from_secs(5)).await;
            }
        }
    }

    Ok(())
}

async fn process_event(
    event: &Event,
    dispatch_job_repo: &DispatchJobRepository,
    subscription_repo: &SubscriptionRepository,
) -> Result<()> {
    info!("Processing event: {} (type: {})", event.id, event.event_type);

    // Find matching subscriptions
    let subscriptions = subscription_repo.find_active().await?;

    let mut jobs_created = 0;
    for sub in subscriptions {
        // Check if subscription matches this event type
        let matches = sub.event_types.iter().any(|binding| {
            matches_event_type(&binding.event_type_code, &event.event_type)
        });
        if !matches {
            continue;
        }

        // Check client access
        if let Some(ref event_client) = event.client_id {
            if let Some(ref sub_client) = sub.client_id {
                if event_client != sub_client {
                    continue;
                }
            }
        }

        // Serialize event data as payload
        let payload = serde_json::to_string(&event.data).unwrap_or_default();

        // Create dispatch job using constructor
        let mut job = DispatchJob::for_event(
            &event.id,
            &event.event_type,
            &event.source,
            &sub.target,
            payload,
        );

        // Set optional fields
        job.subscription_id = Some(sub.id.clone());
        job.client_id = event.client_id.clone();
        job.message_group = event.message_group.clone();
        job.correlation_id = event.correlation_id.clone();
        job.subject = event.subject.clone();
        job.dispatch_pool_id = sub.dispatch_pool_id.clone();
        job.max_retries = sub.max_retries;

        dispatch_job_repo.insert(&job).await?;
        jobs_created += 1;
    }

    if jobs_created > 0 {
        info!("Created {} dispatch jobs for event {}", jobs_created, event.id);
    }

    Ok(())
}

/// Check if event type matches subscription pattern (supports wildcards)
fn matches_event_type(pattern: &str, event_type: &str) -> bool {
    let pattern_parts: Vec<&str> = pattern.split(':').collect();
    let event_parts: Vec<&str> = event_type.split(':').collect();

    if pattern_parts.len() != event_parts.len() {
        return false;
    }

    for (p, e) in pattern_parts.iter().zip(event_parts.iter()) {
        if *p != "*" && p != e {
            return false;
        }
    }

    true
}

async fn metrics_handler() -> String {
    "# HELP fc_stream_up Stream processor is up\n# TYPE fc_stream_up gauge\nfc_stream_up 1\n".to_string()
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

#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn test_matches_event_type() {
        assert!(matches_event_type("orders:*:*:*", "orders:fulfillment:shipment:shipped"));
        assert!(matches_event_type("orders:fulfillment:*:*", "orders:fulfillment:shipment:shipped"));
        assert!(matches_event_type("orders:fulfillment:shipment:shipped", "orders:fulfillment:shipment:shipped"));
        assert!(!matches_event_type("orders:fulfillment:shipment:shipped", "orders:fulfillment:shipment:created"));
        assert!(!matches_event_type("payments:*:*:*", "orders:fulfillment:shipment:shipped"));
    }
}
