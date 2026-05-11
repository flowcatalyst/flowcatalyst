//! Tracing Context
//!
//! Thread-local propagation of correlation and causation IDs through
//! async/background jobs. Enables distributed tracing across service boundaries.

use std::cell::RefCell;

thread_local! {
    static TRACING_CONTEXT: RefCell<Option<TracingContext>> = const { RefCell::new(None) };
}

/// Distributed tracing context for correlation and causation tracking.
///
/// Stored in thread-local storage and automatically picked up by
/// [`ExecutionContext::create()`](super::ExecutionContext::create) when available.
///
/// # Examples
///
/// ```
/// use fc_sdk::usecase::TracingContext;
///
/// // Run code with tracing context
/// TracingContext::run_with_context("corr-123", None, || {
///     // ExecutionContext::create() will use these IDs automatically
/// });
/// ```
#[derive(Debug, Clone)]
pub struct TracingContext {
    correlation_id: String,
    causation_id: Option<String>,
}

impl TracingContext {
    pub fn new(correlation_id: String, causation_id: Option<String>) -> Self {
        Self {
            correlation_id,
            causation_id,
        }
    }

    pub fn correlation_id(&self) -> String {
        self.correlation_id.clone()
    }

    pub fn causation_id(&self) -> Option<&str> {
        self.causation_id.as_deref()
    }

    /// Get the current thread-local tracing context (if set).
    pub fn current() -> Option<TracingContext> {
        TRACING_CONTEXT.with(|ctx| ctx.borrow().clone())
    }

    /// Get the current context or panic.
    pub fn require_current() -> TracingContext {
        Self::current().expect("TracingContext not set — use run_with_context or set_current")
    }

    /// Set the thread-local tracing context.
    pub fn set_current(ctx: TracingContext) {
        TRACING_CONTEXT.with(|c| {
            *c.borrow_mut() = Some(ctx);
        });
    }

    /// Clear the thread-local tracing context.
    pub fn clear_current() {
        TRACING_CONTEXT.with(|c| {
            *c.borrow_mut() = None;
        });
    }

    /// Run a closure with a tracing context set, restoring the previous context afterward.
    pub fn run_with_context<F, R>(
        correlation_id: impl Into<String>,
        causation_id: Option<String>,
        f: F,
    ) -> R
    where
        F: FnOnce() -> R,
    {
        let previous = Self::current();
        Self::set_current(TracingContext::new(correlation_id.into(), causation_id));
        let result = f();
        match previous {
            Some(prev) => Self::set_current(prev),
            None => Self::clear_current(),
        }
        result
    }

    /// Run an async closure with a tracing context set.
    pub async fn run_with_context_async<F, Fut, R>(
        correlation_id: impl Into<String>,
        causation_id: Option<String>,
        f: F,
    ) -> R
    where
        F: FnOnce() -> Fut,
        Fut: std::future::Future<Output = R>,
    {
        let previous = Self::current();
        Self::set_current(TracingContext::new(correlation_id.into(), causation_id));
        let result = f().await;
        match previous {
            Some(prev) => Self::set_current(prev),
            None => Self::clear_current(),
        }
        result
    }

    /// Run a closure with context derived from a parent event.
    pub fn run_from_event<F, R>(
        correlation_id: impl Into<String>,
        causing_event_id: impl Into<String>,
        f: F,
    ) -> R
    where
        F: FnOnce() -> R,
    {
        Self::run_with_context(correlation_id, Some(causing_event_id.into()), f)
    }
}

#[cfg(test)]
mod tests {
    use super::*;

    /// Helper to ensure clean thread-local state before each test.
    fn with_clean_state<F: FnOnce()>(f: F) {
        TracingContext::clear_current();
        f();
        TracingContext::clear_current();
    }

    #[test]
    fn new_and_accessors() {
        let tc = TracingContext::new("corr-1".into(), Some("cause-1".into()));
        assert_eq!(tc.correlation_id(), "corr-1");
        assert_eq!(tc.causation_id(), Some("cause-1"));
    }

    #[test]
    fn new_without_causation() {
        let tc = TracingContext::new("corr-2".into(), None);
        assert_eq!(tc.correlation_id(), "corr-2");
        assert!(tc.causation_id().is_none());
    }

    #[test]
    fn current_returns_none_when_not_set() {
        with_clean_state(|| {
            assert!(TracingContext::current().is_none());
        });
    }

    #[test]
    fn set_and_get_current() {
        with_clean_state(|| {
            let tc = TracingContext::new("corr-set".into(), Some("cause-set".into()));
            TracingContext::set_current(tc);

            let current = TracingContext::current().unwrap();
            assert_eq!(current.correlation_id(), "corr-set");
            assert_eq!(current.causation_id(), Some("cause-set"));
        });
    }

    #[test]
    fn clear_current_removes_context() {
        with_clean_state(|| {
            TracingContext::set_current(TracingContext::new("x".into(), None));
            assert!(TracingContext::current().is_some());

            TracingContext::clear_current();
            assert!(TracingContext::current().is_none());
        });
    }

    #[test]
    #[should_panic(expected = "TracingContext not set")]
    fn require_current_panics_when_not_set() {
        with_clean_state(|| {
            TracingContext::require_current();
        });
    }

    #[test]
    fn require_current_returns_when_set() {
        with_clean_state(|| {
            TracingContext::set_current(TracingContext::new("corr-req".into(), None));
            let tc = TracingContext::require_current();
            assert_eq!(tc.correlation_id(), "corr-req");
        });
    }

    #[test]
    fn run_with_context_sets_and_restores() {
        with_clean_state(|| {
            // Set an initial context
            TracingContext::set_current(TracingContext::new("original".into(), None));

            let result = TracingContext::run_with_context("inner-corr", Some("inner-cause".into()), || {
                let tc = TracingContext::current().unwrap();
                assert_eq!(tc.correlation_id(), "inner-corr");
                assert_eq!(tc.causation_id(), Some("inner-cause"));
                42
            });

            assert_eq!(result, 42);

            // Original context restored
            let restored = TracingContext::current().unwrap();
            assert_eq!(restored.correlation_id(), "original");
        });
    }

    #[test]
    fn run_with_context_restores_none() {
        with_clean_state(|| {
            // No initial context
            assert!(TracingContext::current().is_none());

            TracingContext::run_with_context("temp", None, || {
                assert!(TracingContext::current().is_some());
            });

            // Restored to None
            assert!(TracingContext::current().is_none());
        });
    }

    #[test]
    fn run_from_event_sets_causation() {
        with_clean_state(|| {
            TracingContext::run_from_event("corr-evt", "evt_parent_id", || {
                let tc = TracingContext::current().unwrap();
                assert_eq!(tc.correlation_id(), "corr-evt");
                assert_eq!(tc.causation_id(), Some("evt_parent_id"));
            });
        });
    }

    #[test]
    fn nested_run_with_context() {
        with_clean_state(|| {
            TracingContext::run_with_context("outer", None, || {
                assert_eq!(TracingContext::current().unwrap().correlation_id(), "outer");

                TracingContext::run_with_context("inner", Some("cause".into()), || {
                    let tc = TracingContext::current().unwrap();
                    assert_eq!(tc.correlation_id(), "inner");
                    assert_eq!(tc.causation_id(), Some("cause"));
                });

                // Outer restored
                assert_eq!(TracingContext::current().unwrap().correlation_id(), "outer");
            });

            // Fully cleared
            assert!(TracingContext::current().is_none());
        });
    }

    #[test]
    fn clone_is_independent() {
        let tc = TracingContext::new("corr-clone".into(), Some("cause-clone".into()));
        let cloned = tc.clone();

        assert_eq!(cloned.correlation_id(), "corr-clone");
        assert_eq!(cloned.causation_id(), Some("cause-clone"));
    }

    #[tokio::test]
    async fn run_with_context_async_sets_and_restores() {
        with_clean_state(|| {});

        let result = TracingContext::run_with_context_async("async-corr", None, || async {
            let tc = TracingContext::current().unwrap();
            assert_eq!(tc.correlation_id(), "async-corr");
            "async-result"
        })
        .await;

        assert_eq!(result, "async-result");
        assert!(TracingContext::current().is_none());
    }
}
