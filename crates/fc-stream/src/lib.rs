pub mod config;
pub mod event_projection;
pub mod dispatch_job_projection;
pub mod health;
pub mod partition_manager;

pub use config::StreamProcessorConfig;
pub use event_projection::EventProjectionService;
pub use dispatch_job_projection::DispatchJobProjectionService;
pub use partition_manager::{PartitionManagerConfig, PartitionManagerService};
pub use health::{
    StreamHealth, StreamHealthService, StreamHealthSnapshot,
    StreamHealthStatus, StreamProcessorHealth, StreamStatus,
    AggregatedHealth,
};

/// Handle returned by `start_stream_processor` to control the running projections.
pub struct StreamProcessorHandle {
    event_handle: Option<tokio::task::JoinHandle<()>>,
    dispatch_handle: Option<tokio::task::JoinHandle<()>>,
    partition_handle: Option<tokio::task::JoinHandle<()>>,
    event_service: Option<EventProjectionService>,
    dispatch_service: Option<DispatchJobProjectionService>,
    partition_service: Option<PartitionManagerService>,
}

impl StreamProcessorHandle {
    /// Signal all projection loops to stop and wait for them to finish.
    pub async fn stop(self) {
        if let Some(svc) = &self.event_service {
            svc.stop();
        }
        if let Some(svc) = &self.dispatch_service {
            svc.stop();
        }
        if let Some(svc) = &self.partition_service {
            svc.stop();
        }
        if let Some(h) = self.event_handle {
            let _ = h.await;
        }
        if let Some(h) = self.dispatch_handle {
            let _ = h.await;
        }
        if let Some(h) = self.partition_handle {
            let _ = h.await;
        }
    }
}

/// Start the stream processor projection loops.
///
/// Returns a `StreamProcessorHandle` to stop them, and a `StreamHealthService`
/// pre-populated with health trackers for all enabled projections.
pub fn start_stream_processor(
    pool: sqlx::PgPool,
    config: StreamProcessorConfig,
) -> (StreamProcessorHandle, StreamHealthService) {
    let mut health_service = StreamHealthService::new();

    let (event_service, event_handle) = if config.events_enabled {
        let svc = EventProjectionService::new(pool.clone(), config.events_batch_size);
        health_service.register(svc.health());
        let handle = svc.start();
        (Some(svc), Some(handle))
    } else {
        (None, None)
    };

    let (dispatch_service, dispatch_handle) = if config.dispatch_jobs_enabled {
        let svc = DispatchJobProjectionService::new(pool.clone(), config.dispatch_jobs_batch_size);
        health_service.register(svc.health());
        let handle = svc.start();
        (Some(svc), Some(handle))
    } else {
        (None, None)
    };

    let (partition_service, partition_handle) = if config.partition_manager_enabled {
        let svc = PartitionManagerService::new(pool, PartitionManagerConfig::default());
        health_service.register(svc.health());
        let handle = svc.start();
        (Some(svc), Some(handle))
    } else {
        (None, None)
    };

    let handle = StreamProcessorHandle {
        event_handle,
        dispatch_handle,
        partition_handle,
        event_service,
        dispatch_service,
        partition_service,
    };

    (handle, health_service)
}
