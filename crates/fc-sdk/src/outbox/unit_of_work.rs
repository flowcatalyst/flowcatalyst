//! Unit of Work
//!
//! Atomic commit of entity state changes + domain events (+ optional audit logs)
//! within a single PostgreSQL transaction.
//!
//! The `OutboxUnitOfWork` writes events to the `outbox_messages` table in the
//! consumer's database. The fc-outbox-processor then forwards these to the
//! FlowCatalyst platform API.
//!
//! # Use Case Pattern
//!
//! Consumer apps build use cases that follow the same pattern as the platform:
//!
//! ```ignore
//! pub struct ShipOrderUseCase<U: UnitOfWork> {
//!     order_repo: Arc<OrderRepository>,
//!     unit_of_work: Arc<U>,
//! }
//!
//! impl<U: UnitOfWork> ShipOrderUseCase<U> {
//!     pub async fn execute(
//!         &self,
//!         command: ShipOrderCommand,
//!         ctx: ExecutionContext,
//!     ) -> UseCaseResult<OrderShipped> {
//!         // 1. Validate
//!         if command.tracking_number.is_empty() {
//!             return UseCaseResult::failure(
//!                 UseCaseError::validation("TRACKING_REQUIRED", "Tracking number is required"),
//!             );
//!         }
//!
//!         // 2. Load & check business rules
//!         let order = self.order_repo.find_by_id(&command.order_id).await
//!             .ok_or_else(|| UseCaseError::not_found("ORDER_NOT_FOUND", "Order not found"))?;
//!         if order.status != "confirmed" {
//!             return UseCaseResult::failure(
//!                 UseCaseError::business_rule("NOT_CONFIRMED", "Order must be confirmed to ship"),
//!             );
//!         }
//!
//!         // 3. Build domain event
//!         let event = OrderShipped {
//!             metadata: EventMetadata::builder()
//!                 .from(&ctx)
//!                 .event_type("shop:orders:order:shipped")
//!                 .spec_version("1.0")
//!                 .source("shop:orders")
//!                 .subject(format!("orders.order.{}", order.id))
//!                 .message_group(format!("orders:order:{}", order.id))
//!                 .build(),
//!             order_id: order.id.clone(),
//!             tracking_number: command.tracking_number.clone(),
//!         };
//!
//!         // 4. Atomic commit: entity + event (+ audit log if configured)
//!         self.unit_of_work.commit(&order, event, &command).await
//!     }
//! }
//! ```
//!
//! The handler checks authorization, builds the command, creates an
//! `ExecutionContext`, and calls `use_case.execute(cmd, ctx).await.into_result()?`.

use async_trait::async_trait;
use serde::Serialize;
use sqlx::{PgPool, Postgres, Transaction};
use tracing::{debug, error};

use crate::tsid::TsidGenerator;
use crate::usecase::domain_event::DomainEvent;
use crate::usecase::error::UseCaseError;
use crate::usecase::result::UseCaseResult;

// ─── Traits ──────────────────────────────────────────────────────────────────

/// Trait for entities that have a unique string ID.
pub trait HasId {
    fn id(&self) -> &str;
    /// Legacy collection name. Unused in PostgreSQL implementation.
    fn collection_name() -> &'static str where Self: Sized { "" }
}

/// Trait for domain entities that can be upserted/deleted within a PostgreSQL transaction.
///
/// Implement this for every aggregate that is passed to `UnitOfWork::commit`.
/// This matches the platform's `PgPersist` trait so that SDK consumers follow
/// the same conventions as the platform codebase.
///
/// # Example
///
/// ```ignore
/// use fc_sdk::outbox::{PgPersist, HasId};
/// use sqlx::{Postgres, Transaction};
///
/// struct Order { id: String, customer_id: String, total: f64 }
///
/// impl HasId for Order {
///     fn id(&self) -> &str { &self.id }
/// }
///
/// #[async_trait::async_trait]
/// impl PgPersist for Order {
///     async fn pg_upsert(&self, txn: &mut Transaction<'_, Postgres>) -> anyhow::Result<()> {
///         sqlx::query("INSERT INTO orders (id, customer_id, total) VALUES ($1, $2, $3)
///                      ON CONFLICT (id) DO UPDATE SET customer_id = $2, total = $3")
///             .bind(&self.id)
///             .bind(&self.customer_id)
///             .bind(self.total)
///             .execute(&mut **txn)
///             .await?;
///         Ok(())
///     }
///
///     async fn pg_delete(&self, txn: &mut Transaction<'_, Postgres>) -> anyhow::Result<()> {
///         sqlx::query("DELETE FROM orders WHERE id = $1")
///             .bind(&self.id)
///             .execute(&mut **txn)
///             .await?;
///         Ok(())
///     }
/// }
/// ```
#[async_trait]
pub trait PgPersist: HasId + Send + Sync {
    /// Upsert the entity into the database within the given transaction.
    async fn pg_upsert(&self, txn: &mut Transaction<'_, Postgres>) -> anyhow::Result<()>;

    /// Delete the entity from the database within the given transaction.
    async fn pg_delete(&self, txn: &mut Transaction<'_, Postgres>) -> anyhow::Result<()>;
}

/// Trait for aggregates passed by value to `commit_all`.
/// Same as `PgPersist` but object-safe via `async_trait`.
#[async_trait]
pub trait PgAggregate: Send + Sync {
    fn id(&self) -> &str;
    async fn pg_upsert(&self, txn: &mut Transaction<'_, Postgres>) -> anyhow::Result<()>;
}

// ─── UnitOfWork trait ────────────────────────────────────────────────────────

/// Unit of Work for atomic domain operations.
///
/// Ensures entity state changes and domain events are committed atomically.
/// Audit logs are written when enabled (see [`OutboxConfig::audit_enabled`]).
///
/// Consumer apps use this the same way the platform does:
/// validate → build event → `uow.commit(entity, event, &cmd)`.
#[async_trait]
pub trait UnitOfWork: Send + Sync {
    /// Commit an entity upsert with its domain event (and optional audit log).
    async fn commit<E, T, C>(
        &self,
        aggregate: &T,
        event: E,
        command: &C,
    ) -> UseCaseResult<E>
    where
        E: DomainEvent + Serialize + Send + 'static,
        T: Serialize + HasId + PgPersist + Send + Sync,
        C: Serialize + Send + Sync;

    /// Commit an entity delete with its domain event (and optional audit log).
    async fn commit_delete<E, T, C>(
        &self,
        aggregate: &T,
        event: E,
        command: &C,
    ) -> UseCaseResult<E>
    where
        E: DomainEvent + Serialize + Send + 'static,
        T: Serialize + HasId + PgPersist + Send + Sync,
        C: Serialize + Send + Sync;

    /// Emit a domain event without an entity change (e.g., UserLoggedIn).
    async fn emit_event<E, C>(
        &self,
        event: E,
        command: &C,
    ) -> UseCaseResult<E>
    where
        E: DomainEvent + Serialize + Send + 'static,
        C: Serialize + Send + Sync;

    /// Commit multiple entity upserts with a single domain event.
    async fn commit_all<E, C>(
        &self,
        aggregates: Vec<Box<dyn PgAggregate>>,
        event: E,
        command: &C,
    ) -> UseCaseResult<E>
    where
        E: DomainEvent + Serialize + Send + 'static,
        C: Serialize + Send + Sync;
}

// ─── OutboxConfig ────────────────────────────────────────────────────────────

/// Configuration for the outbox unit of work.
#[derive(Debug, Clone)]
pub struct OutboxConfig {
    /// Table name for outbox messages (default: "outbox_messages")
    pub table_name: String,
    /// Optional client_id for multi-tenant scoping
    pub client_id: Option<String>,
    /// Whether to write audit log entries alongside events (default: false).
    ///
    /// The platform always audits (control plane operations). Consumer apps
    /// should enable this only for admin/human-initiated operations, not for
    /// every transactional event.
    pub audit_enabled: bool,
}

impl Default for OutboxConfig {
    fn default() -> Self {
        Self {
            table_name: "outbox_messages".to_string(),
            client_id: None,
            audit_enabled: false,
        }
    }
}

// ─── OutboxUnitOfWork ────────────────────────────────────────────────────────

/// Outbox-backed implementation of [`UnitOfWork`].
///
/// Atomically persists entity changes and domain events to the `outbox_messages`
/// table. The fc-outbox-processor polls this table and forwards items to the
/// FlowCatalyst platform API.
///
/// # Example
///
/// ```ignore
/// use fc_sdk::outbox::{OutboxUnitOfWork, OutboxConfig};
///
/// // Events only (default for transactional operations)
/// let uow = OutboxUnitOfWork::new(pool.clone());
///
/// // Events + audit logs (for admin operations)
/// let uow = OutboxUnitOfWork::with_config(pool, OutboxConfig {
///     audit_enabled: true,
///     ..Default::default()
/// });
/// ```
#[derive(Clone)]
pub struct OutboxUnitOfWork {
    pool: PgPool,
    config: OutboxConfig,
}

impl OutboxUnitOfWork {
    /// Create a new OutboxUnitOfWork with default configuration (events only, no audit).
    pub fn new(pool: PgPool) -> Self {
        Self {
            pool,
            config: OutboxConfig::default(),
        }
    }

    /// Create a new OutboxUnitOfWork with custom configuration.
    pub fn with_config(pool: PgPool, config: OutboxConfig) -> Self {
        Self { pool, config }
    }

    /// "domain.aggregate.123" → "Aggregate"
    fn extract_aggregate_type(subject: &str) -> String {
        subject
            .split('.')
            .nth(1)
            .map(|s| {
                let mut chars = s.chars();
                match chars.next() {
                    Some(c) => c.to_uppercase().collect::<String>() + chars.as_str(),
                    None => String::new(),
                }
            })
            .unwrap_or_else(|| "Unknown".to_string())
    }

    /// "domain.aggregate.123" → "123"
    fn extract_entity_id(subject: &str) -> String {
        subject.split('.').nth(2).unwrap_or("").to_string()
    }

    /// Write the event outbox item into the transaction.
    async fn write_event_outbox<E: DomainEvent + Serialize>(
        txn: &mut Transaction<'_, Postgres>,
        table: &str,
        event: &E,
        client_id: &Option<String>,
    ) -> Result<(), UseCaseError> {
        let id = TsidGenerator::generate_untyped();
        let data_json: serde_json::Value = serde_json::from_str(&event.to_data_json())
            .unwrap_or(serde_json::json!({}));

        let payload = serde_json::json!({
            "event_type": event.event_type(),
            "spec_version": event.spec_version(),
            "source": event.source(),
            "subject": event.subject(),
            "data": data_json,
            "correlation_id": event.correlation_id(),
            "causation_id": event.causation_id(),
            "deduplication_id": format!("{}-{}", event.event_type(), event.event_id()),
            "message_group": event.message_group(),
            "context_data": [
                {"key": "principalId", "value": event.principal_id()},
                {"key": "aggregateType", "value": Self::extract_aggregate_type(event.subject())},
            ],
        });

        let payload_str = payload.to_string();
        let payload_size = payload_str.len() as i32;

        let query = format!(
            "INSERT INTO {} (id, type, message_group, payload, status, retry_count, created_at, updated_at, client_id, payload_size) \
             VALUES ($1, 'EVENT', $2, $3, 0, 0, NOW(), NOW(), $4, $5)",
            table
        );

        if let Err(e) = sqlx::query(&query)
            .bind(&id)
            .bind(event.message_group())
            .bind(&payload)
            .bind(client_id.as_deref())
            .bind(payload_size)
            .execute(&mut **txn)
            .await
        {
            error!("Failed to write event outbox item: {}", e);
            return Err(UseCaseError::commit(format!(
                "Failed to write event outbox item: {}",
                e
            )));
        }

        Ok(())
    }

    /// Write the audit log outbox item into the transaction.
    async fn write_audit_outbox<E: DomainEvent, C: Serialize>(
        txn: &mut Transaction<'_, Postgres>,
        table: &str,
        event: &E,
        command: &C,
        client_id: &Option<String>,
    ) -> Result<(), UseCaseError> {
        let id = TsidGenerator::generate_untyped();

        let command_name = std::any::type_name::<C>()
            .rsplit("::")
            .next()
            .unwrap_or("Unknown")
            .to_string();

        let operation_json = serde_json::to_value(command).ok();

        let payload = serde_json::json!({
            "entity_type": Self::extract_aggregate_type(event.subject()),
            "entity_id": Self::extract_entity_id(event.subject()),
            "operation": command_name,
            "operation_json": operation_json,
            "principal_id": event.principal_id(),
            "performed_at": event.time().to_rfc3339(),
        });

        let payload_size = payload.to_string().len() as i32;

        let query = format!(
            "INSERT INTO {} (id, type, message_group, payload, status, retry_count, created_at, updated_at, client_id, payload_size) \
             VALUES ($1, 'AUDIT_LOG', $2, $3, 0, 0, NOW(), NOW(), $4, $5)",
            table
        );

        if let Err(e) = sqlx::query(&query)
            .bind(&id)
            .bind(event.message_group())
            .bind(&payload)
            .bind(client_id.as_deref())
            .bind(payload_size)
            .execute(&mut **txn)
            .await
        {
            error!("Failed to write audit outbox item: {}", e);
            return Err(UseCaseError::commit(format!(
                "Failed to write audit outbox item: {}",
                e
            )));
        }

        Ok(())
    }

    async fn persist_outbox_items<E: DomainEvent + Serialize, C: Serialize>(
        txn: &mut Transaction<'_, Postgres>,
        table: &str,
        event: &E,
        command: &C,
        client_id: &Option<String>,
        audit_enabled: bool,
    ) -> Result<(), UseCaseError> {
        Self::write_event_outbox(txn, table, event, client_id).await?;
        if audit_enabled {
            Self::write_audit_outbox(txn, table, event, command, client_id).await?;
        }
        Ok(())
    }
}

#[async_trait]
impl UnitOfWork for OutboxUnitOfWork {
    async fn commit<E, T, C>(
        &self,
        aggregate: &T,
        event: E,
        command: &C,
    ) -> UseCaseResult<E>
    where
        E: DomainEvent + Serialize + Send + 'static,
        T: Serialize + HasId + PgPersist + Send + Sync,
        C: Serialize + Send + Sync,
    {
        let mut txn = match self.pool.begin().await {
            Ok(t) => t,
            Err(e) => {
                error!("Failed to start transaction: {}", e);
                return UseCaseResult::failure(UseCaseError::commit(format!(
                    "Failed to start transaction: {}",
                    e
                )));
            }
        };

        if let Err(e) = aggregate.pg_upsert(&mut txn).await {
            error!("Failed to persist aggregate: {}", e);
            return UseCaseResult::failure(UseCaseError::commit(format!(
                "Failed to persist aggregate: {}",
                e
            )));
        }

        if let Err(e) = Self::persist_outbox_items(
            &mut txn,
            &self.config.table_name,
            &event,
            command,
            &self.config.client_id,
            self.config.audit_enabled,
        )
        .await
        {
            return UseCaseResult::failure(e);
        }

        if let Err(e) = txn.commit().await {
            error!("Failed to commit transaction: {}", e);
            return UseCaseResult::failure(UseCaseError::commit(format!(
                "Failed to commit transaction: {}",
                e
            )));
        }

        debug!(
            event_id = event.event_id(),
            event_type = event.event_type(),
            "Committed entity + outbox event"
        );

        UseCaseResult::success(event)
    }

    async fn commit_delete<E, T, C>(
        &self,
        aggregate: &T,
        event: E,
        command: &C,
    ) -> UseCaseResult<E>
    where
        E: DomainEvent + Serialize + Send + 'static,
        T: Serialize + HasId + PgPersist + Send + Sync,
        C: Serialize + Send + Sync,
    {
        let mut txn = match self.pool.begin().await {
            Ok(t) => t,
            Err(e) => {
                error!("Failed to start transaction: {}", e);
                return UseCaseResult::failure(UseCaseError::commit(format!(
                    "Failed to start transaction: {}",
                    e
                )));
            }
        };

        if let Err(e) = aggregate.pg_delete(&mut txn).await {
            error!("Failed to delete aggregate: {}", e);
            return UseCaseResult::failure(UseCaseError::commit(format!(
                "Failed to delete aggregate: {}",
                e
            )));
        }

        if let Err(e) = Self::persist_outbox_items(
            &mut txn,
            &self.config.table_name,
            &event,
            command,
            &self.config.client_id,
            self.config.audit_enabled,
        )
        .await
        {
            return UseCaseResult::failure(e);
        }

        if let Err(e) = txn.commit().await {
            error!("Failed to commit transaction: {}", e);
            return UseCaseResult::failure(UseCaseError::commit(format!(
                "Failed to commit transaction: {}",
                e
            )));
        }

        debug!(
            event_id = event.event_id(),
            event_type = event.event_type(),
            "Committed delete + outbox event"
        );

        UseCaseResult::success(event)
    }

    async fn emit_event<E, C>(
        &self,
        event: E,
        command: &C,
    ) -> UseCaseResult<E>
    where
        E: DomainEvent + Serialize + Send + 'static,
        C: Serialize + Send + Sync,
    {
        let mut txn = match self.pool.begin().await {
            Ok(t) => t,
            Err(e) => {
                error!("Failed to start transaction: {}", e);
                return UseCaseResult::failure(UseCaseError::commit(format!(
                    "Failed to start transaction: {}",
                    e
                )));
            }
        };

        if let Err(e) = Self::persist_outbox_items(
            &mut txn,
            &self.config.table_name,
            &event,
            command,
            &self.config.client_id,
            self.config.audit_enabled,
        )
        .await
        {
            return UseCaseResult::failure(e);
        }

        if let Err(e) = txn.commit().await {
            error!("Failed to commit transaction: {}", e);
            return UseCaseResult::failure(UseCaseError::commit(format!(
                "Failed to commit transaction: {}",
                e
            )));
        }

        debug!(
            event_id = event.event_id(),
            event_type = event.event_type(),
            "Emitted event via outbox"
        );

        UseCaseResult::success(event)
    }

    async fn commit_all<E, C>(
        &self,
        aggregates: Vec<Box<dyn PgAggregate>>,
        event: E,
        command: &C,
    ) -> UseCaseResult<E>
    where
        E: DomainEvent + Serialize + Send + 'static,
        C: Serialize + Send + Sync,
    {
        let mut txn = match self.pool.begin().await {
            Ok(t) => t,
            Err(e) => {
                error!("Failed to start transaction: {}", e);
                return UseCaseResult::failure(UseCaseError::commit(format!(
                    "Failed to start transaction: {}",
                    e
                )));
            }
        };

        for aggregate in &aggregates {
            if let Err(e) = aggregate.pg_upsert(&mut txn).await {
                error!("Failed to persist aggregate: {}", e);
                return UseCaseResult::failure(UseCaseError::commit(format!(
                    "Failed to persist aggregate: {}",
                    e
                )));
            }
        }

        if let Err(e) = Self::persist_outbox_items(
            &mut txn,
            &self.config.table_name,
            &event,
            command,
            &self.config.client_id,
            self.config.audit_enabled,
        )
        .await
        {
            return UseCaseResult::failure(e);
        }

        if let Err(e) = txn.commit().await {
            error!("Failed to commit transaction: {}", e);
            return UseCaseResult::failure(UseCaseError::commit(format!(
                "Failed to commit transaction: {}",
                e
            )));
        }

        debug!(
            event_id = event.event_id(),
            event_type = event.event_type(),
            aggregate_count = aggregates.len(),
            "Committed multi-aggregate + outbox event"
        );

        UseCaseResult::success(event)
    }
}

// ─── InMemoryUnitOfWork (tests) ──────────────────────────────────────────────

/// In-memory implementation of [`UnitOfWork`] for unit testing use cases.
///
/// Records committed event IDs so tests can assert which events were emitted
/// without needing a database.
///
/// # Example
///
/// ```ignore
/// use fc_sdk::outbox::InMemoryUnitOfWork;
/// use std::sync::Arc;
///
/// let uow = Arc::new(InMemoryUnitOfWork::new());
/// let use_case = ShipOrderUseCase::new(mock_repo, uow.clone());
///
/// let result = use_case.execute(cmd, ctx).await;
/// assert!(result.is_success());
/// assert_eq!(uow.committed_events().len(), 1);
/// ```
pub struct InMemoryUnitOfWork {
    committed_events: std::sync::Mutex<Vec<String>>,
}

impl InMemoryUnitOfWork {
    pub fn new() -> Self {
        Self {
            committed_events: std::sync::Mutex::new(Vec::new()),
        }
    }

    /// Get the list of committed event IDs.
    pub fn committed_events(&self) -> Vec<String> {
        self.committed_events.lock().unwrap().clone()
    }

    /// Check if any events were committed.
    pub fn has_commits(&self) -> bool {
        !self.committed_events.lock().unwrap().is_empty()
    }

    /// Clear all recorded events.
    pub fn clear(&self) {
        self.committed_events.lock().unwrap().clear();
    }
}

impl Default for InMemoryUnitOfWork {
    fn default() -> Self {
        Self::new()
    }
}

#[async_trait]
impl UnitOfWork for InMemoryUnitOfWork {
    async fn commit<E, T, C>(
        &self,
        _aggregate: &T,
        event: E,
        _command: &C,
    ) -> UseCaseResult<E>
    where
        E: DomainEvent + Serialize + Send + 'static,
        T: Serialize + HasId + PgPersist + Send + Sync,
        C: Serialize + Send + Sync,
    {
        self.committed_events
            .lock()
            .unwrap()
            .push(event.event_id().to_string());
        UseCaseResult::success(event)
    }

    async fn commit_delete<E, T, C>(
        &self,
        _aggregate: &T,
        event: E,
        _command: &C,
    ) -> UseCaseResult<E>
    where
        E: DomainEvent + Serialize + Send + 'static,
        T: Serialize + HasId + PgPersist + Send + Sync,
        C: Serialize + Send + Sync,
    {
        self.committed_events
            .lock()
            .unwrap()
            .push(event.event_id().to_string());
        UseCaseResult::success(event)
    }

    async fn emit_event<E, C>(
        &self,
        event: E,
        _command: &C,
    ) -> UseCaseResult<E>
    where
        E: DomainEvent + Serialize + Send + 'static,
        C: Serialize + Send + Sync,
    {
        self.committed_events
            .lock()
            .unwrap()
            .push(event.event_id().to_string());
        UseCaseResult::success(event)
    }

    async fn commit_all<E, C>(
        &self,
        _aggregates: Vec<Box<dyn PgAggregate>>,
        event: E,
        _command: &C,
    ) -> UseCaseResult<E>
    where
        E: DomainEvent + Serialize + Send + 'static,
        C: Serialize + Send + Sync,
    {
        self.committed_events
            .lock()
            .unwrap()
            .push(event.event_id().to_string());
        UseCaseResult::success(event)
    }
}
