//! Outbox Manager
//!
//! High-level manager for creating outbox messages. This is the simpler
//! alternative to the full [`UnitOfWork`](super::UnitOfWork) pattern —
//! use it when you don't need atomic entity + event commits.
//!
//! Matches the TS and Laravel SDK `OutboxManager` API.
//!
//! # Example
//!
//! ```ignore
//! use fc_sdk::outbox::{OutboxManager, SqlxPgDriver, CreateEventDto, CreateDispatchJobDto};
//!
//! let driver = SqlxPgDriver::new(pool);
//! let outbox = OutboxManager::new(Box::new(driver), "clt_0HZXEQ5Y8JY5Z");
//!
//! // Create a single event
//! let event_id = outbox.create_event(
//!     CreateEventDto::new("user.registered", serde_json::json!({"userId": "123"}))
//!         .source("user-service")
//!         .message_group("users:user:123"),
//! ).await?;
//!
//! // Create a batch of dispatch jobs
//! let job_ids = outbox.create_dispatch_jobs(vec![job1, job2]).await?;
//! ```

use crate::tsid::TsidGenerator;

use super::driver::{MessageType, OutboxDriver, OutboxMessage, OutboxStatus};
use super::dto::{CreateAuditLogDto, CreateDispatchJobDto, CreateEventDto};

/// Manages outbox message creation.
pub struct OutboxManager {
    driver: Box<dyn OutboxDriver>,
    client_id: String,
}

impl OutboxManager {
    pub fn new(driver: Box<dyn OutboxDriver>, client_id: impl Into<String>) -> Self {
        Self {
            driver,
            client_id: client_id.into(),
        }
    }

    /// Create a single event in the outbox. Returns the generated TSID.
    pub async fn create_event(&self, event: CreateEventDto) -> anyhow::Result<String> {
        self.ensure_client_id()?;

        let id = TsidGenerator::generate_untyped();
        let payload = serde_json::to_string(&event.to_payload())?;
        let headers = if event.headers.is_empty() {
            None
        } else {
            Some(event.headers.clone())
        };

        let message = self.build_message(
            id.clone(),
            MessageType::Event,
            &payload,
            event.message_group.as_deref(),
            headers,
        );

        self.driver.insert(message).await?;
        Ok(id)
    }

    /// Create multiple events in the outbox (batch). Returns the generated TSIDs.
    pub async fn create_events(&self, events: Vec<CreateEventDto>) -> anyhow::Result<Vec<String>> {
        if events.is_empty() {
            return Ok(Vec::new());
        }
        self.ensure_client_id()?;

        let mut ids = Vec::with_capacity(events.len());
        let mut messages = Vec::with_capacity(events.len());

        for event in &events {
            let id = TsidGenerator::generate_untyped();
            ids.push(id.clone());
            let payload = serde_json::to_string(&event.to_payload())?;
            let headers = if event.headers.is_empty() {
                None
            } else {
                Some(event.headers.clone())
            };

            messages.push(self.build_message(
                id,
                MessageType::Event,
                &payload,
                event.message_group.as_deref(),
                headers,
            ));
        }

        self.driver.insert_batch(messages).await?;
        Ok(ids)
    }

    /// Create a single dispatch job in the outbox. Returns the generated TSID.
    pub async fn create_dispatch_job(&self, job: CreateDispatchJobDto) -> anyhow::Result<String> {
        self.ensure_client_id()?;

        let id = TsidGenerator::generate_untyped();
        let payload = serde_json::to_string(&job.to_payload())?;

        let message = self.build_message(
            id.clone(),
            MessageType::DispatchJob,
            &payload,
            job.message_group.as_deref(),
            None,
        );

        self.driver.insert(message).await?;
        Ok(id)
    }

    /// Create multiple dispatch jobs in the outbox (batch). Returns the generated TSIDs.
    pub async fn create_dispatch_jobs(
        &self,
        jobs: Vec<CreateDispatchJobDto>,
    ) -> anyhow::Result<Vec<String>> {
        if jobs.is_empty() {
            return Ok(Vec::new());
        }
        self.ensure_client_id()?;

        let mut ids = Vec::with_capacity(jobs.len());
        let mut messages = Vec::with_capacity(jobs.len());

        for job in &jobs {
            let id = TsidGenerator::generate_untyped();
            ids.push(id.clone());
            let payload = serde_json::to_string(&job.to_payload())?;

            messages.push(self.build_message(
                id,
                MessageType::DispatchJob,
                &payload,
                job.message_group.as_deref(),
                None,
            ));
        }

        self.driver.insert_batch(messages).await?;
        Ok(ids)
    }

    /// Create a single audit log in the outbox. Returns the generated TSID.
    pub async fn create_audit_log(&self, audit: CreateAuditLogDto) -> anyhow::Result<String> {
        self.ensure_client_id()?;

        let id = TsidGenerator::generate_untyped();
        let payload = serde_json::to_string(&audit.to_payload())?;
        let headers = if audit.headers.is_empty() {
            None
        } else {
            Some(audit.headers.clone())
        };

        let message = self.build_message(
            id.clone(),
            MessageType::AuditLog,
            &payload,
            None,
            headers,
        );

        self.driver.insert(message).await?;
        Ok(id)
    }

    /// Create multiple audit logs in the outbox (batch). Returns the generated TSIDs.
    pub async fn create_audit_logs(
        &self,
        audits: Vec<CreateAuditLogDto>,
    ) -> anyhow::Result<Vec<String>> {
        if audits.is_empty() {
            return Ok(Vec::new());
        }
        self.ensure_client_id()?;

        let mut ids = Vec::with_capacity(audits.len());
        let mut messages = Vec::with_capacity(audits.len());

        for audit in &audits {
            let id = TsidGenerator::generate_untyped();
            ids.push(id.clone());
            let payload = serde_json::to_string(&audit.to_payload())?;
            let headers = if audit.headers.is_empty() {
                None
            } else {
                Some(audit.headers.clone())
            };

            messages.push(self.build_message(id, MessageType::AuditLog, &payload, None, headers));
        }

        self.driver.insert_batch(messages).await?;
        Ok(ids)
    }

    /// Get the underlying driver.
    pub fn driver(&self) -> &dyn OutboxDriver {
        &*self.driver
    }

    fn build_message(
        &self,
        id: String,
        message_type: MessageType,
        payload: &str,
        message_group: Option<&str>,
        headers: Option<std::collections::HashMap<String, String>>,
    ) -> OutboxMessage {
        let now = chrono::Utc::now().to_rfc3339();
        OutboxMessage {
            id,
            message_type: message_type.as_str().to_string(),
            message_group: message_group.map(|s| s.to_string()),
            payload: payload.to_string(),
            status: OutboxStatus::PENDING,
            created_at: now.clone(),
            updated_at: now,
            client_id: self.client_id.clone(),
            payload_size: payload.len() as i32,
            headers,
        }
    }

    fn ensure_client_id(&self) -> anyhow::Result<()> {
        if self.client_id.is_empty() {
            anyhow::bail!(
                "OutboxManager: client_id is required. Provide a valid client ID when constructing the OutboxManager."
            );
        }
        Ok(())
    }
}
