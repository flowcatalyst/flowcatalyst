//! Transactional Outbox Pattern
//!
//! The outbox pattern allows applications to write events atomically with their
//! business transactions. Events are stored locally and later processed by FlowCatalyst.
//!
//! ## Supported Drivers
//!
//! - `PostgresDriver` - PostgreSQL database
//! - `MongoDriver` - MongoDB database
//!
//! ## Example
//!
//! ```rust,no_run
//! use flowcatalyst_sdk::outbox::{OutboxManager, CreateEventDto, PostgresDriver};
//!
//! # async fn example() -> Result<(), Box<dyn std::error::Error>> {
//! // Create outbox manager with PostgreSQL driver
//! let driver = PostgresDriver::new("postgres://localhost/mydb").await?;
//! let outbox = OutboxManager::new(driver, "tenant_123");
//!
//! // Create an event in the outbox
//! let event = CreateEventDto::new("order.created", serde_json::json!({
//!     "order_id": "123",
//!     "total": 99.99
//! }), "orders")
//!     .with_correlation_id("corr-123")
//!     .with_source("order-service");
//!
//! let message_id = outbox.create_event(event).await?;
//! println!("Created outbox message: {}", message_id);
//! # Ok(())
//! # }
//! ```

mod dto;
mod driver;

pub use dto::{CreateEventDto, CreateDispatchJobDto, MessageType, OutboxMessage};
pub use driver::{OutboxDriver, PostgresDriver, MongoDriver};

use crate::error::Result;
use crate::tsid::TsidGenerator;

/// Outbox manager for transactional event publishing
pub struct OutboxManager<D: OutboxDriver> {
    driver: D,
    tenant_id: String,
    default_partition: String,
    tsid_generator: TsidGenerator,
}

impl<D: OutboxDriver> OutboxManager<D> {
    /// Create a new outbox manager
    pub fn new(driver: D, tenant_id: impl Into<String>) -> Self {
        Self {
            driver,
            tenant_id: tenant_id.into(),
            default_partition: "default".to_string(),
            tsid_generator: TsidGenerator::new(),
        }
    }

    /// Set the default partition ID
    pub fn with_default_partition(mut self, partition: impl Into<String>) -> Self {
        self.default_partition = partition.into();
        self
    }

    /// Create an event in the outbox
    ///
    /// Returns the message ID.
    pub async fn create_event(&self, event: CreateEventDto) -> Result<String> {
        let message_id = self.tsid_generator.generate();
        let partition_id = if event.partition_id.is_empty() {
            self.default_partition.clone()
        } else {
            event.partition_id.clone()
        };

        let message = OutboxMessage {
            id: message_id.clone(),
            tenant_id: self.tenant_id.clone(),
            partition_id,
            message_type: MessageType::Event,
            payload: event.to_payload(),
            headers: event.headers,
            created_at: chrono::Utc::now(),
        };

        self.driver.insert(message).await?;
        Ok(message_id)
    }

    /// Create multiple events in the outbox
    ///
    /// Returns the message IDs.
    pub async fn create_events(&self, events: Vec<CreateEventDto>) -> Result<Vec<String>> {
        let messages: Vec<OutboxMessage> = events
            .into_iter()
            .map(|event| {
                let message_id = self.tsid_generator.generate();
                let partition_id = if event.partition_id.is_empty() {
                    self.default_partition.clone()
                } else {
                    event.partition_id.clone()
                };

                OutboxMessage {
                    id: message_id,
                    tenant_id: self.tenant_id.clone(),
                    partition_id,
                    message_type: MessageType::Event,
                    payload: event.to_payload(),
                    headers: event.headers,
                    created_at: chrono::Utc::now(),
                }
            })
            .collect();

        let ids: Vec<String> = messages.iter().map(|m| m.id.clone()).collect();
        self.driver.insert_batch(messages).await?;
        Ok(ids)
    }

    /// Create a dispatch job in the outbox
    ///
    /// Returns the message ID.
    pub async fn create_dispatch_job(&self, job: CreateDispatchJobDto) -> Result<String> {
        let message_id = self.tsid_generator.generate();
        let partition_id = if job.partition_id.is_empty() {
            self.default_partition.clone()
        } else {
            job.partition_id.clone()
        };

        let message = OutboxMessage {
            id: message_id.clone(),
            tenant_id: self.tenant_id.clone(),
            partition_id,
            message_type: MessageType::DispatchJob,
            payload: job.to_payload(),
            headers: job.headers,
            created_at: chrono::Utc::now(),
        };

        self.driver.insert(message).await?;
        Ok(message_id)
    }

    /// Create multiple dispatch jobs in the outbox
    ///
    /// Returns the message IDs.
    pub async fn create_dispatch_jobs(&self, jobs: Vec<CreateDispatchJobDto>) -> Result<Vec<String>> {
        let messages: Vec<OutboxMessage> = jobs
            .into_iter()
            .map(|job| {
                let message_id = self.tsid_generator.generate();
                let partition_id = if job.partition_id.is_empty() {
                    self.default_partition.clone()
                } else {
                    job.partition_id.clone()
                };

                OutboxMessage {
                    id: message_id,
                    tenant_id: self.tenant_id.clone(),
                    partition_id,
                    message_type: MessageType::DispatchJob,
                    payload: job.to_payload(),
                    headers: job.headers,
                    created_at: chrono::Utc::now(),
                }
            })
            .collect();

        let ids: Vec<String> = messages.iter().map(|m| m.id.clone()).collect();
        self.driver.insert_batch(messages).await?;
        Ok(ids)
    }
}
