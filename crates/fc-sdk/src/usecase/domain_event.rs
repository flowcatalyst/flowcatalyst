//! Domain Event Trait
//!
//! Base trait for all domain events. Events follow the CloudEvents specification
//! with additional fields for distributed tracing and message ordering.
//!
//! # Event Type Format
//!
//! `{app}:{domain}:{aggregate}:{action}` — e.g., `orders:fulfillment:shipment:shipped`
//!
//! # Subject Format
//!
//! `{domain}.{aggregate}.{id}` — e.g., `fulfillment.shipment.0HZXEQ5Y8JY5Z`
//!
//! # Message Group
//!
//! `{domain}:{aggregate}:{id}` — events in the same group are processed in order.

use chrono::{DateTime, Utc};
use serde::{Deserialize, Serialize};

/// Base trait for all domain events.
///
/// Implement this trait for each domain event in your application.
/// Use the [`impl_domain_event!`](crate::impl_domain_event) macro to
/// delegate to an `EventMetadata` field.
pub trait DomainEvent: Send + Sync {
    /// Unique identifier for this event (TSID Crockford Base32 string).
    fn event_id(&self) -> &str;
    /// Event type code: `{app}:{domain}:{aggregate}:{action}`
    fn event_type(&self) -> &str;
    /// Schema version of this event type (e.g., "1.0").
    fn spec_version(&self) -> &str;
    /// Source system that generated this event.
    fn source(&self) -> &str;
    /// Qualified aggregate identifier: `{domain}.{aggregate}.{id}`
    fn subject(&self) -> &str;
    /// When the event occurred.
    fn time(&self) -> DateTime<Utc>;
    /// Execution ID for tracking a single use case execution.
    fn execution_id(&self) -> &str;
    /// Correlation ID for distributed tracing.
    fn correlation_id(&self) -> &str;
    /// ID of the event that caused this event (if any).
    fn causation_id(&self) -> Option<&str>;
    /// Principal who initiated the action that produced this event.
    fn principal_id(&self) -> &str;
    /// Message group for ordering guarantees.
    fn message_group(&self) -> &str;
    /// Serialize the event-specific data payload to JSON.
    fn to_data_json(&self) -> String;
}

/// Common metadata for domain events.
///
/// Include this as a `metadata` field in your event structs and use
/// [`impl_domain_event!`](crate::impl_domain_event) to auto-implement the trait.
///
/// # Example
///
/// ```
/// use fc_sdk::usecase::EventMetadata;
/// use serde::Serialize;
///
/// #[derive(Serialize)]
/// pub struct OrderCreated {
///     pub metadata: EventMetadata,
///     pub order_id: String,
///     pub customer_id: String,
///     pub total: f64,
/// }
///
/// fc_sdk::impl_domain_event!(OrderCreated);
/// ```
#[derive(Debug, Clone, Serialize, Deserialize)]
pub struct EventMetadata {
    pub event_id: String,
    pub event_type: String,
    pub spec_version: String,
    pub source: String,
    pub subject: String,
    pub time: DateTime<Utc>,
    pub execution_id: String,
    pub correlation_id: String,
    #[serde(skip_serializing_if = "Option::is_none")]
    pub causation_id: Option<String>,
    pub principal_id: String,
    pub message_group: String,
}

impl EventMetadata {
    // EventMetadata maps 1:1 to CloudEvents fields — each is mandatory and
    // not naturally groupable. A builder would add ceremony without
    // shrinking the call site (every field is required).
    #[allow(clippy::too_many_arguments)]
    pub fn new(
        event_id: String,
        event_type: &str,
        spec_version: &str,
        source: &str,
        subject: String,
        message_group: String,
        execution_id: String,
        correlation_id: String,
        causation_id: Option<String>,
        principal_id: String,
    ) -> Self {
        Self {
            event_id,
            event_type: event_type.to_string(),
            spec_version: spec_version.to_string(),
            source: source.to_string(),
            subject,
            time: Utc::now(),
            execution_id,
            correlation_id,
            causation_id,
            principal_id,
            message_group,
        }
    }

    /// Create a builder for event metadata.
    pub fn builder() -> EventMetadataBuilder {
        EventMetadataBuilder::new()
    }
}

/// Fluent builder for [`EventMetadata`].
///
/// # Example
///
/// ```ignore
/// let metadata = EventMetadata::builder()
///     .from(&ctx)
///     .event_type("orders:fulfillment:shipment:shipped")
///     .spec_version("1.0")
///     .source("orders:fulfillment")
///     .subject(format!("fulfillment.shipment.{}", shipment_id))
///     .message_group(format!("fulfillment:shipment:{}", shipment_id))
///     .build();
/// ```
#[derive(Default)]
pub struct EventMetadataBuilder {
    event_id: Option<String>,
    event_type: Option<String>,
    spec_version: Option<String>,
    source: Option<String>,
    subject: Option<String>,
    message_group: Option<String>,
    execution_id: Option<String>,
    correlation_id: Option<String>,
    causation_id: Option<String>,
    principal_id: Option<String>,
}

impl EventMetadataBuilder {
    pub fn new() -> Self {
        Self::default()
    }

    /// Copy tracing metadata from an [`ExecutionContext`](super::ExecutionContext).
    ///
    /// Sets event_id (new TSID), execution_id, correlation_id, causation_id,
    /// and principal_id from the context.
    pub fn from(mut self, ctx: &super::ExecutionContext) -> Self {
        self.event_id = Some(crate::tsid::TsidGenerator::generate_untyped());
        self.execution_id = Some(ctx.execution_id.clone());
        self.correlation_id = Some(ctx.correlation_id.clone());
        self.causation_id = ctx.causation_id.clone();
        self.principal_id = Some(ctx.principal_id.clone());
        self
    }

    pub fn event_id(mut self, id: impl Into<String>) -> Self {
        self.event_id = Some(id.into());
        self
    }

    pub fn event_type(mut self, event_type: impl Into<String>) -> Self {
        self.event_type = Some(event_type.into());
        self
    }

    pub fn spec_version(mut self, version: impl Into<String>) -> Self {
        self.spec_version = Some(version.into());
        self
    }

    pub fn source(mut self, source: impl Into<String>) -> Self {
        self.source = Some(source.into());
        self
    }

    pub fn subject(mut self, subject: impl Into<String>) -> Self {
        self.subject = Some(subject.into());
        self
    }

    pub fn message_group(mut self, group: impl Into<String>) -> Self {
        self.message_group = Some(group.into());
        self
    }

    pub fn execution_id(mut self, id: impl Into<String>) -> Self {
        self.execution_id = Some(id.into());
        self
    }

    pub fn correlation_id(mut self, id: impl Into<String>) -> Self {
        self.correlation_id = Some(id.into());
        self
    }

    pub fn causation_id(mut self, id: impl Into<String>) -> Self {
        self.causation_id = Some(id.into());
        self
    }

    pub fn principal_id(mut self, id: impl Into<String>) -> Self {
        self.principal_id = Some(id.into());
        self
    }

    /// Build the EventMetadata.
    ///
    /// # Panics
    ///
    /// Panics if required fields are missing: event_type, spec_version,
    /// source, subject, message_group, execution_id, correlation_id, principal_id.
    pub fn build(self) -> EventMetadata {
        EventMetadata {
            event_id: self
                .event_id
                .unwrap_or_else(crate::tsid::TsidGenerator::generate_untyped),
            event_type: self.event_type.expect("event_type is required"),
            spec_version: self.spec_version.expect("spec_version is required"),
            source: self.source.expect("source is required"),
            subject: self.subject.expect("subject is required"),
            time: Utc::now(),
            execution_id: self
                .execution_id
                .expect("execution_id is required (use .from(ctx))"),
            correlation_id: self
                .correlation_id
                .expect("correlation_id is required (use .from(ctx))"),
            causation_id: self.causation_id,
            principal_id: self
                .principal_id
                .expect("principal_id is required (use .from(ctx))"),
            message_group: self.message_group.expect("message_group is required"),
        }
    }

    /// Try to build the EventMetadata, returning an error if fields are missing.
    pub fn try_build(self) -> Result<EventMetadata, &'static str> {
        Ok(EventMetadata {
            event_id: self
                .event_id
                .unwrap_or_else(crate::tsid::TsidGenerator::generate_untyped),
            event_type: self.event_type.ok_or("event_type is required")?,
            spec_version: self.spec_version.ok_or("spec_version is required")?,
            source: self.source.ok_or("source is required")?,
            subject: self.subject.ok_or("subject is required")?,
            time: Utc::now(),
            execution_id: self.execution_id.ok_or("execution_id is required")?,
            correlation_id: self.correlation_id.ok_or("correlation_id is required")?,
            causation_id: self.causation_id,
            principal_id: self.principal_id.ok_or("principal_id is required")?,
            message_group: self.message_group.ok_or("message_group is required")?,
        })
    }
}

/// Macro for implementing the [`DomainEvent`] trait.
///
/// Delegates all trait methods to a field named `metadata` of type [`EventMetadata`].
///
/// # Example
///
/// ```
/// use fc_sdk::usecase::{DomainEvent, EventMetadata};
/// use serde::Serialize;
///
/// #[derive(Debug, Clone, Serialize)]
/// pub struct OrderShipped {
///     pub metadata: EventMetadata,
///     pub order_id: String,
///     pub tracking_number: String,
/// }
///
/// fc_sdk::impl_domain_event!(OrderShipped);
/// ```
///
#[macro_export]
macro_rules! impl_domain_event {
    ($event_type:ty) => {
        impl $crate::usecase::DomainEvent for $event_type {
            fn event_id(&self) -> &str {
                &self.metadata.event_id
            }

            fn event_type(&self) -> &str {
                &self.metadata.event_type
            }

            fn spec_version(&self) -> &str {
                &self.metadata.spec_version
            }

            fn source(&self) -> &str {
                &self.metadata.source
            }

            fn subject(&self) -> &str {
                &self.metadata.subject
            }

            fn time(&self) -> chrono::DateTime<chrono::Utc> {
                self.metadata.time
            }

            fn execution_id(&self) -> &str {
                &self.metadata.execution_id
            }

            fn correlation_id(&self) -> &str {
                &self.metadata.correlation_id
            }

            fn causation_id(&self) -> Option<&str> {
                self.metadata.causation_id.as_deref()
            }

            fn principal_id(&self) -> &str {
                &self.metadata.principal_id
            }

            fn message_group(&self) -> &str {
                &self.metadata.message_group
            }

            fn to_data_json(&self) -> String {
                serde_json::to_string(self).unwrap_or_else(|_| "{}".to_string())
            }
        }
    };
}

#[cfg(test)]
mod tests {
    use super::*;
    use serde::Serialize;

    // ─── EventMetadata ──────────────────────────────────────────────────

    #[test]
    fn event_metadata_new_sets_all_fields() {
        let meta = EventMetadata::new(
            "evt_1".into(),
            "orders:order:created",
            "1.0",
            "shop:orders",
            "orders.order.42".into(),
            "orders:order:42".into(),
            "exec-1".into(),
            "corr-1".into(),
            Some("cause-1".into()),
            "prn_user".into(),
        );

        assert_eq!(meta.event_id, "evt_1");
        assert_eq!(meta.event_type, "orders:order:created");
        assert_eq!(meta.spec_version, "1.0");
        assert_eq!(meta.source, "shop:orders");
        assert_eq!(meta.subject, "orders.order.42");
        assert_eq!(meta.message_group, "orders:order:42");
        assert_eq!(meta.execution_id, "exec-1");
        assert_eq!(meta.correlation_id, "corr-1");
        assert_eq!(meta.causation_id.as_deref(), Some("cause-1"));
        assert_eq!(meta.principal_id, "prn_user");
        // time is set to Utc::now()
        assert!(meta.time <= Utc::now());
    }

    #[test]
    fn event_metadata_new_without_causation() {
        let meta = EventMetadata::new(
            "evt_2".into(),
            "t",
            "1.0",
            "s",
            "sub".into(),
            "grp".into(),
            "exec".into(),
            "corr".into(),
            None,
            "prn".into(),
        );
        assert!(meta.causation_id.is_none());
    }

    #[test]
    fn event_metadata_serialization_round_trip() {
        let meta = EventMetadata::new(
            "evt_rt".into(),
            "test.event",
            "2.0",
            "test-src",
            "test.sub.1".into(),
            "test:sub:1".into(),
            "exec-rt".into(),
            "corr-rt".into(),
            Some("cause-rt".into()),
            "prn_rt".into(),
        );

        let json = serde_json::to_string(&meta).unwrap();
        let deserialized: EventMetadata = serde_json::from_str(&json).unwrap();

        assert_eq!(meta.event_id, deserialized.event_id);
        assert_eq!(meta.event_type, deserialized.event_type);
        assert_eq!(meta.spec_version, deserialized.spec_version);
        assert_eq!(meta.source, deserialized.source);
        assert_eq!(meta.subject, deserialized.subject);
        assert_eq!(meta.message_group, deserialized.message_group);
        assert_eq!(meta.execution_id, deserialized.execution_id);
        assert_eq!(meta.correlation_id, deserialized.correlation_id);
        assert_eq!(meta.causation_id, deserialized.causation_id);
        assert_eq!(meta.principal_id, deserialized.principal_id);
    }

    #[test]
    fn event_metadata_causation_id_skipped_when_none() {
        let meta = EventMetadata::new(
            "e".into(),
            "t",
            "1",
            "s",
            "sub".into(),
            "grp".into(),
            "exec".into(),
            "corr".into(),
            None,
            "prn".into(),
        );
        let json = serde_json::to_string(&meta).unwrap();
        assert!(!json.contains("causation_id"));
    }

    // ─── EventMetadataBuilder ───────────────────────────────────────────

    #[test]
    fn builder_sets_all_fields() {
        let meta = EventMetadataBuilder::new()
            .event_id("evt_b")
            .event_type("shop:orders:order:created")
            .spec_version("1.0")
            .source("shop:orders")
            .subject("orders.order.1")
            .message_group("orders:order:1")
            .execution_id("exec-b")
            .correlation_id("corr-b")
            .causation_id("cause-b")
            .principal_id("prn_b")
            .build();

        assert_eq!(meta.event_id, "evt_b");
        assert_eq!(meta.event_type, "shop:orders:order:created");
        assert_eq!(meta.spec_version, "1.0");
        assert_eq!(meta.source, "shop:orders");
        assert_eq!(meta.subject, "orders.order.1");
        assert_eq!(meta.message_group, "orders:order:1");
        assert_eq!(meta.execution_id, "exec-b");
        assert_eq!(meta.correlation_id, "corr-b");
        assert_eq!(meta.causation_id.as_deref(), Some("cause-b"));
        assert_eq!(meta.principal_id, "prn_b");
    }

    #[test]
    fn builder_generates_event_id_if_not_set() {
        let meta = EventMetadataBuilder::new()
            .event_type("t")
            .spec_version("1")
            .source("s")
            .subject("sub")
            .message_group("grp")
            .execution_id("exec")
            .correlation_id("corr")
            .principal_id("prn")
            .build();

        assert!(!meta.event_id.is_empty());
    }

    #[test]
    fn builder_causation_id_is_none_by_default() {
        let meta = EventMetadataBuilder::new()
            .event_id("e")
            .event_type("t")
            .spec_version("1")
            .source("s")
            .subject("sub")
            .message_group("grp")
            .execution_id("exec")
            .correlation_id("corr")
            .principal_id("prn")
            .build();

        assert!(meta.causation_id.is_none());
    }

    #[test]
    fn builder_from_execution_context() {
        let ctx = super::super::ExecutionContext::with_correlation("prn_test", "corr_from_ctx");
        let meta = EventMetadataBuilder::new()
            .from(&ctx)
            .event_type("test.event")
            .spec_version("1.0")
            .source("test")
            .subject("sub.1")
            .message_group("grp:1")
            .build();

        assert!(!meta.event_id.is_empty());
        assert_eq!(meta.execution_id, ctx.execution_id);
        assert_eq!(meta.correlation_id, "corr_from_ctx");
        assert_eq!(meta.principal_id, "prn_test");
        assert!(meta.causation_id.is_none());
    }

    #[test]
    fn builder_from_ctx_with_causation() {
        let ctx = super::super::ExecutionContext::with_correlation("prn", "corr");
        let child_ctx = ctx.with_causation("evt_parent");

        let meta = EventMetadataBuilder::new()
            .from(&child_ctx)
            .event_type("t")
            .spec_version("1")
            .source("s")
            .subject("sub")
            .message_group("grp")
            .build();

        assert_eq!(meta.causation_id.as_deref(), Some("evt_parent"));
    }

    #[test]
    #[should_panic(expected = "event_type is required")]
    fn builder_panics_without_event_type() {
        EventMetadataBuilder::new()
            .spec_version("1")
            .source("s")
            .subject("sub")
            .message_group("grp")
            .execution_id("exec")
            .correlation_id("corr")
            .principal_id("prn")
            .build();
    }

    #[test]
    #[should_panic(expected = "spec_version is required")]
    fn builder_panics_without_spec_version() {
        EventMetadataBuilder::new()
            .event_type("t")
            .source("s")
            .subject("sub")
            .message_group("grp")
            .execution_id("exec")
            .correlation_id("corr")
            .principal_id("prn")
            .build();
    }

    #[test]
    #[should_panic(expected = "source is required")]
    fn builder_panics_without_source() {
        EventMetadataBuilder::new()
            .event_type("t")
            .spec_version("1")
            .subject("sub")
            .message_group("grp")
            .execution_id("exec")
            .correlation_id("corr")
            .principal_id("prn")
            .build();
    }

    #[test]
    #[should_panic(expected = "message_group is required")]
    fn builder_panics_without_message_group() {
        EventMetadataBuilder::new()
            .event_type("t")
            .spec_version("1")
            .source("s")
            .subject("sub")
            .execution_id("exec")
            .correlation_id("corr")
            .principal_id("prn")
            .build();
    }

    #[test]
    fn try_build_returns_error_for_missing_fields() {
        let result = EventMetadataBuilder::new().try_build();
        assert!(result.is_err());
        assert_eq!(result.unwrap_err(), "event_type is required");
    }

    #[test]
    fn try_build_succeeds_with_all_fields() {
        let result = EventMetadataBuilder::new()
            .event_type("t")
            .spec_version("1")
            .source("s")
            .subject("sub")
            .message_group("grp")
            .execution_id("exec")
            .correlation_id("corr")
            .principal_id("prn")
            .try_build();

        assert!(result.is_ok());
        let meta = result.unwrap();
        assert_eq!(meta.event_type, "t");
    }

    // ─── impl_domain_event! macro ───────────────────────────────────────

    #[derive(Debug, Clone, Serialize)]
    struct TestEvent {
        pub metadata: EventMetadata,
        pub order_id: String,
        pub amount: f64,
    }

    crate::impl_domain_event!(TestEvent);

    #[test]
    fn impl_domain_event_delegates_to_metadata() {
        let meta = EventMetadata::new(
            "evt_macro".into(),
            "test:event",
            "1.0",
            "test-src",
            "sub.1".into(),
            "grp:1".into(),
            "exec-m".into(),
            "corr-m".into(),
            Some("cause-m".into()),
            "prn_m".into(),
        );

        let event = TestEvent {
            metadata: meta,
            order_id: "ord_1".into(),
            amount: 99.99,
        };

        assert_eq!(event.event_id(), "evt_macro");
        assert_eq!(event.event_type(), "test:event");
        assert_eq!(event.spec_version(), "1.0");
        assert_eq!(event.source(), "test-src");
        assert_eq!(event.subject(), "sub.1");
        assert_eq!(event.execution_id(), "exec-m");
        assert_eq!(event.correlation_id(), "corr-m");
        assert_eq!(event.causation_id(), Some("cause-m"));
        assert_eq!(event.principal_id(), "prn_m");
        assert_eq!(event.message_group(), "grp:1");
    }

    #[test]
    fn impl_domain_event_to_data_json() {
        let meta = EventMetadata::new(
            "e".into(),
            "t",
            "1",
            "s",
            "sub".into(),
            "grp".into(),
            "exec".into(),
            "corr".into(),
            None,
            "prn".into(),
        );
        let event = TestEvent {
            metadata: meta,
            order_id: "ord_42".into(),
            amount: 123.45,
        };

        let json = event.to_data_json();
        let parsed: serde_json::Value = serde_json::from_str(&json).unwrap();
        assert_eq!(parsed["order_id"], "ord_42");
        assert_eq!(parsed["amount"], 123.45);
        // Metadata is also serialized
        assert!(parsed["metadata"].is_object());
    }

    #[test]
    fn impl_domain_event_no_causation() {
        let meta = EventMetadata::new(
            "e".into(),
            "t",
            "1",
            "s",
            "sub".into(),
            "grp".into(),
            "exec".into(),
            "corr".into(),
            None,
            "prn".into(),
        );
        let event = TestEvent {
            metadata: meta,
            order_id: "x".into(),
            amount: 0.0,
        };
        assert!(event.causation_id().is_none());
    }
}
