//! Unit of Work — PostgreSQL via SQLx
//!
//! Atomic commit of entity state changes, domain events, and audit logs
//! within a single PostgreSQL transaction.

use async_trait::async_trait;
use chrono::Utc;
use serde::Serialize;
use sqlx::{PgPool, Postgres, Transaction};
use tracing::{debug, error};

use super::domain_event::DomainEvent;
use super::error::UseCaseError;
use super::result::UseCaseResult;

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
#[async_trait]
pub trait PgPersist: HasId + Send + Sync {
    /// Upsert the entity into the database within the given transaction.
    async fn pg_upsert(&self, txn: &mut Transaction<'_, Postgres>) -> crate::shared::error::Result<()>;

    /// Delete the entity from the database within the given transaction.
    async fn pg_delete(&self, txn: &mut Transaction<'_, Postgres>) -> crate::shared::error::Result<()>;
}

/// Trait for aggregates passed by value to `commit_all`.
/// Same as `PgPersist` but object-safe via `async_trait`.
#[async_trait]
pub trait PgAggregate: Send + Sync {
    fn id(&self) -> &str;
    async fn pg_upsert(&self, txn: &mut Transaction<'_, Postgres>) -> crate::shared::error::Result<()>;
}

// ─── UnitOfWork trait ────────────────────────────────────────────────────────

/// Unit of Work for atomic control plane operations.
///
/// Ensures entity state changes, domain events, and audit logs are committed
/// atomically in a single PostgreSQL transaction.
#[async_trait]
pub trait UnitOfWork: Send + Sync {
    /// Commit an entity upsert with its domain event and audit log.
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

    /// Commit an entity delete with its domain event and audit log.
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

    /// Emit a domain event and audit log without an entity change.
    ///
    /// Used for events that don't modify an entity directly (e.g., UserLoggedIn).
    async fn emit_event<E, C>(
        &self,
        event: E,
        command: &C,
    ) -> UseCaseResult<E>
    where
        E: DomainEvent + Serialize + Send + 'static,
        C: Serialize + Send + Sync;

    /// Commit multiple entity upserts with a single domain event and audit log.
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

// ─── PgUnitOfWork ────────────────────────────────────────────────────────────

/// PostgreSQL implementation of `UnitOfWork` using SQLx transactions.
#[derive(Clone)]
pub struct PgUnitOfWork {
    pool: PgPool,
}

impl PgUnitOfWork {
    pub fn new(pool: PgPool) -> Self {
        Self { pool }
    }

    pub fn from_ref(pool: &PgPool) -> Self {
        Self { pool: pool.clone() }
    }

    // ── Subject parsing helpers ───────────────────────────────

    /// "platform.eventtype.123" -> "Eventtype"
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

    /// "platform.eventtype.123" -> Some("123")
    fn extract_entity_id(subject: &str) -> String {
        subject.split('.').nth(2).unwrap_or("").to_string()
    }

    // ── Persist helpers ──────────────────────────────────────

    async fn persist_event<E: DomainEvent>(
        txn: &mut Transaction<'_, Postgres>,
        event: &E,
    ) -> Result<(), UseCaseError> {
        let data_json: serde_json::Value = serde_json::from_str(&event.to_data_json())
            .unwrap_or(serde_json::json!({}));

        let context_data = serde_json::json!([
            {"key": "principalId", "value": event.principal_id()},
            {"key": "aggregateType", "value": Self::extract_aggregate_type(event.subject())},
        ]);

        let deduplication_id = format!("{}-{}", event.event_type(), event.event_id());
        let now = Utc::now();

        let result = sqlx::query(
            r#"INSERT INTO msg_events
                (id, spec_version, type, source, subject,
                 time, data, correlation_id, causation_id,
                 deduplication_id, message_group, client_id,
                 context_data, created_at)
            VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14)"#,
        )
        .bind(event.event_id())
        .bind(event.spec_version())
        .bind(event.event_type())
        .bind(event.source())
        .bind(event.subject())
        .bind(event.time())
        .bind(&data_json)
        .bind(event.correlation_id())
        .bind(event.causation_id())
        .bind(&deduplication_id)
        .bind(event.message_group())
        .bind(None::<String>) // client_id
        .bind(&context_data)
        .bind(now)
        .execute(&mut **txn)
        .await;

        if let Err(e) = result {
            error!("Failed to insert domain event: {}", e);
            return Err(UseCaseError::commit(format!("Failed to insert domain event: {}", e)));
        }

        Ok(())
    }

    async fn persist_audit_log<E: DomainEvent, C: Serialize>(
        txn: &mut Transaction<'_, Postgres>,
        event: &E,
        command: &C,
    ) -> Result<(), UseCaseError> {
        let command_name = std::any::type_name::<C>()
            .rsplit("::")
            .next()
            .unwrap_or("Unknown")
            .to_string();

        let operation_json: Option<serde_json::Value> = serde_json::to_value(command).ok();

        let result = sqlx::query(
            r#"INSERT INTO aud_logs
                (id, entity_type, entity_id, operation,
                 operation_json, principal_id, application_id,
                 client_id, performed_at)
            VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9)"#,
        )
        .bind(crate::TsidGenerator::generate_untyped())
        .bind(Self::extract_aggregate_type(event.subject()))
        .bind(Self::extract_entity_id(event.subject()))
        .bind(&command_name)
        .bind(&operation_json)
        .bind(event.principal_id())
        .bind(None::<String>) // application_id
        .bind(None::<String>) // client_id
        .bind(event.time())
        .execute(&mut **txn)
        .await;

        if let Err(e) = result {
            error!("Failed to insert audit log: {}", e);
            return Err(UseCaseError::commit(format!("Failed to insert audit log: {}", e)));
        }

        Ok(())
    }

    async fn persist_event_and_audit<E: DomainEvent, C: Serialize>(
        txn: &mut Transaction<'_, Postgres>,
        event: &E,
        command: &C,
    ) -> Result<(), UseCaseError> {
        Self::persist_event(&mut *txn, event).await?;
        Self::persist_audit_log(&mut *txn, event, command).await?;
        Ok(())
    }
}

#[async_trait]
impl UnitOfWork for PgUnitOfWork {
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
                return UseCaseResult::failure(UseCaseError::commit(format!("Failed to start transaction: {}", e)));
            }
        };

        if let Err(e) = aggregate.pg_upsert(&mut txn).await {
            let _ = txn.rollback().await;
            error!("Failed to persist aggregate: {}", e);
            return UseCaseResult::failure(UseCaseError::commit(format!("Failed to persist aggregate: {}", e)));
        }

        if let Err(e) = Self::persist_event_and_audit(&mut txn, &event, command).await {
            let _ = txn.rollback().await;
            return UseCaseResult::failure(e);
        }

        if let Err(e) = txn.commit().await {
            error!("Failed to commit transaction: {}", e);
            return UseCaseResult::failure(UseCaseError::commit(format!("Failed to commit transaction: {}", e)));
        }

        debug!(
            event_id = event.event_id(),
            event_type = event.event_type(),
            "Successfully committed transaction"
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
                return UseCaseResult::failure(UseCaseError::commit(format!("Failed to start transaction: {}", e)));
            }
        };

        if let Err(e) = aggregate.pg_delete(&mut txn).await {
            let _ = txn.rollback().await;
            error!("Failed to delete aggregate: {}", e);
            return UseCaseResult::failure(UseCaseError::commit(format!("Failed to delete aggregate: {}", e)));
        }

        if let Err(e) = Self::persist_event_and_audit(&mut txn, &event, command).await {
            let _ = txn.rollback().await;
            return UseCaseResult::failure(e);
        }

        if let Err(e) = txn.commit().await {
            error!("Failed to commit transaction: {}", e);
            return UseCaseResult::failure(UseCaseError::commit(format!("Failed to commit transaction: {}", e)));
        }

        debug!(
            event_id = event.event_id(),
            event_type = event.event_type(),
            "Successfully committed delete transaction"
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
                return UseCaseResult::failure(UseCaseError::commit(format!("Failed to start transaction: {}", e)));
            }
        };

        if let Err(e) = Self::persist_event_and_audit(&mut txn, &event, command).await {
            let _ = txn.rollback().await;
            return UseCaseResult::failure(e);
        }

        if let Err(e) = txn.commit().await {
            error!("Failed to commit transaction: {}", e);
            return UseCaseResult::failure(UseCaseError::commit(format!("Failed to commit transaction: {}", e)));
        }

        debug!(
            event_id = event.event_id(),
            event_type = event.event_type(),
            "Successfully emitted domain event"
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
                return UseCaseResult::failure(UseCaseError::commit(format!("Failed to start transaction: {}", e)));
            }
        };

        for aggregate in &aggregates {
            if let Err(e) = aggregate.pg_upsert(&mut txn).await {
                let _ = txn.rollback().await;
                error!("Failed to persist aggregate: {}", e);
                return UseCaseResult::failure(UseCaseError::commit(format!("Failed to persist aggregate: {}", e)));
            }
        }

        if let Err(e) = Self::persist_event_and_audit(&mut txn, &event, command).await {
            let _ = txn.rollback().await;
            return UseCaseResult::failure(e);
        }

        if let Err(e) = txn.commit().await {
            error!("Failed to commit transaction: {}", e);
            return UseCaseResult::failure(UseCaseError::commit(format!("Failed to commit transaction: {}", e)));
        }

        debug!(
            event_id = event.event_id(),
            event_type = event.event_type(),
            aggregate_count = aggregates.len(),
            "Successfully committed multi-aggregate transaction"
        );

        UseCaseResult::success(event)
    }
}

// ─── InMemory (tests) ─────────────────────────────────────────────────────────

#[cfg(test)]
pub struct InMemoryUnitOfWork {
    pub committed_events: std::sync::Mutex<Vec<String>>,
}

#[cfg(test)]
impl InMemoryUnitOfWork {
    pub fn new() -> Self {
        Self { committed_events: std::sync::Mutex::new(Vec::new()) }
    }
}

#[cfg(test)]
#[async_trait]
impl UnitOfWork for InMemoryUnitOfWork {
    async fn commit<E, T, C>(&self, _aggregate: &T, event: E, _command: &C) -> UseCaseResult<E>
    where
        E: DomainEvent + Serialize + Send + 'static,
        T: Serialize + HasId + PgPersist + Send + Sync,
        C: Serialize + Send + Sync,
    {
        self.committed_events.lock().unwrap().push(event.event_id().to_string());
        UseCaseResult::success(event)
    }

    async fn commit_delete<E, T, C>(&self, _aggregate: &T, event: E, _command: &C) -> UseCaseResult<E>
    where
        E: DomainEvent + Serialize + Send + 'static,
        T: Serialize + HasId + PgPersist + Send + Sync,
        C: Serialize + Send + Sync,
    {
        self.committed_events.lock().unwrap().push(event.event_id().to_string());
        UseCaseResult::success(event)
    }

    async fn emit_event<E, C>(&self, event: E, _command: &C) -> UseCaseResult<E>
    where
        E: DomainEvent + Serialize + Send + 'static,
        C: Serialize + Send + Sync,
    {
        self.committed_events.lock().unwrap().push(event.event_id().to_string());
        UseCaseResult::success(event)
    }

    async fn commit_all<E, C>(&self, _aggregates: Vec<Box<dyn PgAggregate>>, event: E, _command: &C) -> UseCaseResult<E>
    where
        E: DomainEvent + Serialize + Send + 'static,
        C: Serialize + Send + Sync,
    {
        self.committed_events.lock().unwrap().push(event.event_id().to_string());
        UseCaseResult::success(event)
    }
}

#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn test_extract_aggregate_type() {
        assert_eq!(PgUnitOfWork::extract_aggregate_type("platform.eventtype.123"), "Eventtype");
        assert_eq!(PgUnitOfWork::extract_aggregate_type("platform.user.abc"), "User");
        assert_eq!(PgUnitOfWork::extract_aggregate_type(""), "Unknown");
    }

    #[test]
    fn test_extract_entity_id() {
        assert_eq!(PgUnitOfWork::extract_entity_id("platform.user.123"), "123");
        assert_eq!(PgUnitOfWork::extract_entity_id("platform.user"), "");
    }
}
