//! Execution Context
//!
//! Context for a use case execution. Carries tracing IDs and principal
//! information through the execution of a use case.

use chrono::{DateTime, Utc};
use super::domain_event::DomainEvent;
use super::tracing_context::TracingContext;
use crate::tsid::TsidGenerator;

/// Context for a use case execution.
///
/// Carries tracing IDs and principal information through the execution
/// of a use case. This context is used to populate domain event metadata.
///
/// # Examples
///
/// ```
/// use fc_sdk::usecase::ExecutionContext;
///
/// // Fresh request (generates new IDs)
/// let ctx = ExecutionContext::create("user-123");
///
/// // With specific correlation ID from upstream
/// let ctx = ExecutionContext::with_correlation("user-123", "trace-from-gateway");
///
/// // Child context within same execution
/// let child = ctx.with_causation("evt-456");
/// ```
#[derive(Debug, Clone)]
pub struct ExecutionContext {
    /// Unique ID for this execution (generated)
    pub execution_id: String,
    /// ID for distributed tracing (usually from original request)
    pub correlation_id: String,
    /// ID of the parent event that caused this execution (if any)
    pub causation_id: Option<String>,
    /// ID of the principal performing the action
    pub principal_id: String,
    /// When the execution was initiated
    pub initiated_at: DateTime<Utc>,
}

impl ExecutionContext {
    /// Create a new execution context for a fresh request.
    ///
    /// Automatically picks up thread-local [`TracingContext`] if available.
    pub fn create(principal_id: impl Into<String>) -> Self {
        if let Some(tracing_ctx) = TracingContext::current() {
            return Self::from_tracing_context(&tracing_ctx, principal_id);
        }

        let exec_id = format!("exec-{}", TsidGenerator::generate_untyped());
        Self {
            execution_id: exec_id.clone(),
            correlation_id: exec_id,
            causation_id: None,
            principal_id: principal_id.into(),
            initiated_at: Utc::now(),
        }
    }

    /// Create an execution context from a [`TracingContext`].
    ///
    /// Preferred when running within an HTTP request where TracingContext
    /// has been populated from headers.
    pub fn from_tracing_context(
        tracing_context: &TracingContext,
        principal_id: impl Into<String>,
    ) -> Self {
        let exec_id = format!("exec-{}", TsidGenerator::generate_untyped());
        Self {
            execution_id: exec_id,
            correlation_id: tracing_context.correlation_id(),
            causation_id: tracing_context.causation_id().map(|s| s.to_string()),
            principal_id: principal_id.into(),
            initiated_at: Utc::now(),
        }
    }

    /// Create a new execution context with a specific correlation ID.
    pub fn with_correlation(
        principal_id: impl Into<String>,
        correlation_id: impl Into<String>,
    ) -> Self {
        Self {
            execution_id: format!("exec-{}", TsidGenerator::generate_untyped()),
            correlation_id: correlation_id.into(),
            causation_id: None,
            principal_id: principal_id.into(),
            initiated_at: Utc::now(),
        }
    }

    /// Create a new execution context from a parent event.
    ///
    /// The parent event's ID becomes the causation_id, and the
    /// correlation_id is preserved.
    pub fn from_parent_event<E: DomainEvent>(
        parent: &E,
        principal_id: impl Into<String>,
    ) -> Self {
        Self {
            execution_id: format!("exec-{}", TsidGenerator::generate_untyped()),
            correlation_id: parent.correlation_id().to_string(),
            causation_id: Some(parent.event_id().to_string()),
            principal_id: principal_id.into(),
            initiated_at: Utc::now(),
        }
    }

    /// Create an execution context from an [`AuthContext`](crate::auth::AuthContext).
    ///
    /// Bridges the auth layer to the use case layer by extracting the
    /// principal_id from the validated token claims.
    ///
    /// # Example
    ///
    /// ```ignore
    /// use fc_sdk::usecase::ExecutionContext;
    ///
    /// let ctx = ExecutionContext::from_auth(&auth_context);
    /// let result = use_case.execute(command, ctx).await;
    /// ```
    #[cfg(feature = "auth")]
    pub fn from_auth(auth: &crate::auth::AuthContext) -> Self {
        Self::create(auth.principal_id())
    }

    /// Create an execution context from [`AccessTokenClaims`](crate::auth::AccessTokenClaims).
    ///
    /// Use this when you have the raw claims without a full AuthContext.
    #[cfg(feature = "auth")]
    pub fn from_claims(claims: &crate::auth::AccessTokenClaims) -> Self {
        Self::create(claims.principal_id())
    }

    /// Create a child context within the same execution.
    pub fn with_causation(&self, causing_event_id: impl Into<String>) -> Self {
        Self {
            execution_id: self.execution_id.clone(),
            correlation_id: self.correlation_id.clone(),
            causation_id: Some(causing_event_id.into()),
            principal_id: self.principal_id.clone(),
            initiated_at: Utc::now(),
        }
    }

    /// Create a new context with a different principal.
    pub fn with_principal(&self, principal_id: impl Into<String>) -> Self {
        Self {
            execution_id: self.execution_id.clone(),
            correlation_id: self.correlation_id.clone(),
            causation_id: self.causation_id.clone(),
            principal_id: principal_id.into(),
            initiated_at: self.initiated_at,
        }
    }
}
