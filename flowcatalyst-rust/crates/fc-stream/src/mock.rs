use async_trait::async_trait;
use crate::StreamWatcher;
use anyhow::Result;
use tokio::time::{sleep, Duration};
use tracing::info;

pub struct MockStreamWatcher {
    name: String,
}

impl MockStreamWatcher {
    pub fn new(name: String) -> Self {
        Self { name }
    }
}

#[async_trait]
impl StreamWatcher for MockStreamWatcher {
    async fn watch(&self) -> Result<()> {
        info!("Starting mock stream watcher: {}", self.name);
        loop {
            // Simulate watching changes
            sleep(Duration::from_secs(10)).await;
            info!("Mock stream [{}] heartbeat", self.name);
        }
    }
}
