//! Subscription Matcher
//!
//! Matches events to subscriptions and creates dispatch jobs.

use std::collections::HashMap;
use std::sync::Arc;
use tokio::sync::RwLock;
use serde::{Deserialize, Serialize};
use chrono::{DateTime, Utc};
use tracing::{debug, info};

/// Minimal event representation for matching
#[derive(Debug, Clone, Serialize, Deserialize)]
pub struct MatchableEvent {
    pub id: String,
    pub event_type_code: String,
    pub client_id: Option<String>,
    pub correlation_id: Option<String>,
    pub source_id: Option<String>,
    pub data: serde_json::Value,
    pub created_at: DateTime<Utc>,
}

/// Subscription binding for matching
#[derive(Debug, Clone)]
pub struct SubscriptionBinding {
    /// Event type pattern (supports wildcards with *)
    pub event_type_pattern: String,
    /// Optional filter expression (JSONPath or similar)
    pub filter: Option<String>,
}

impl SubscriptionBinding {
    /// Check if this binding matches an event type code
    pub fn matches_event_type(&self, event_type_code: &str) -> bool {
        let pattern_parts: Vec<&str> = self.event_type_pattern.split(':').collect();
        let event_parts: Vec<&str> = event_type_code.split(':').collect();

        if pattern_parts.len() != event_parts.len() {
            return false;
        }

        pattern_parts.iter().zip(event_parts.iter()).all(|(pattern, event)| {
            *pattern == "*" || pattern == event
        })
    }
}

/// Subscription for matching
#[derive(Debug, Clone)]
pub struct MatchableSubscription {
    pub id: String,
    pub code: String,
    pub client_id: Option<String>,
    pub bindings: Vec<SubscriptionBinding>,
    pub target: String,
    pub dispatch_pool_id: Option<String>,
    pub service_account_id: Option<String>,
    pub mode: String,
    pub delay_seconds: u32,
    pub sequence: i32,
    pub timeout_seconds: u32,
    pub max_retries: u32,
    pub data_only: bool,
    pub is_active: bool,
}

impl MatchableSubscription {
    /// Check if this subscription matches an event
    pub fn matches_event(&self, event: &MatchableEvent) -> bool {
        if !self.is_active {
            return false;
        }

        // Check client matching
        if !self.matches_client(event.client_id.as_deref()) {
            return false;
        }

        // Check event type matching
        self.bindings.iter().any(|b| b.matches_event_type(&event.event_type_code))
    }

    fn matches_client(&self, event_client_id: Option<&str>) -> bool {
        match (&self.client_id, event_client_id) {
            // Anchor-level subscription matches all
            (None, _) => true,
            // Client-specific matches same client
            (Some(sub_client), Some(event_client)) => sub_client == event_client,
            // Client-specific doesn't match anchor-level event
            (Some(_), None) => false,
        }
    }
}

/// Dispatch job to be created
#[derive(Debug, Clone, Serialize, Deserialize)]
pub struct DispatchJobCreation {
    pub id: String,
    pub event_id: String,
    pub subscription_id: String,
    pub subscription_code: String,
    pub client_id: Option<String>,
    pub event_type_code: String,
    pub correlation_id: Option<String>,
    pub source_id: Option<String>,
    pub target: String,
    pub dispatch_pool_id: Option<String>,
    pub service_account_id: Option<String>,
    pub mode: String,
    pub delay_seconds: u32,
    pub sequence: i32,
    pub timeout_seconds: u32,
    pub max_retries: u32,
    pub data_only: bool,
    pub payload: serde_json::Value,
    pub scheduled_for: DateTime<Utc>,
    pub created_at: DateTime<Utc>,
}

/// Subscription cache for fast lookups
pub struct SubscriptionCache {
    /// All subscriptions
    subscriptions: Arc<RwLock<Vec<MatchableSubscription>>>,
    /// Index by event type pattern for faster matching
    by_pattern: Arc<RwLock<HashMap<String, Vec<usize>>>>,
    /// Last refresh time
    last_refresh: Arc<RwLock<Option<DateTime<Utc>>>>,
}

impl SubscriptionCache {
    pub fn new() -> Self {
        Self {
            subscriptions: Arc::new(RwLock::new(vec![])),
            by_pattern: Arc::new(RwLock::new(HashMap::new())),
            last_refresh: Arc::new(RwLock::new(None)),
        }
    }

    /// Load subscriptions into cache
    pub async fn load(&self, subscriptions: Vec<MatchableSubscription>) {
        let mut subs = self.subscriptions.write().await;
        let mut index = self.by_pattern.write().await;

        subs.clear();
        index.clear();

        for (i, sub) in subscriptions.iter().enumerate() {
            for binding in &sub.bindings {
                index
                    .entry(binding.event_type_pattern.clone())
                    .or_insert_with(Vec::new)
                    .push(i);
            }
        }

        *subs = subscriptions;

        let mut last = self.last_refresh.write().await;
        *last = Some(Utc::now());

        info!("Loaded {} subscriptions into cache", subs.len());
    }

    /// Check if cache needs refresh
    pub async fn needs_refresh(&self, max_age_secs: u64) -> bool {
        let last = self.last_refresh.read().await;
        match *last {
            None => true,
            Some(time) => {
                let age = (Utc::now() - time).num_seconds() as u64;
                age > max_age_secs
            }
        }
    }
}

/// Subscription matcher - matches events to subscriptions
pub struct SubscriptionMatcher {
    cache: Arc<SubscriptionCache>,
    id_generator: Arc<dyn Fn() -> String + Send + Sync>,
}

impl SubscriptionMatcher {
    pub fn new<F>(cache: Arc<SubscriptionCache>, id_generator: F) -> Self
    where
        F: Fn() -> String + Send + Sync + 'static,
    {
        Self {
            cache,
            id_generator: Arc::new(id_generator),
        }
    }

    /// Match an event to subscriptions and generate dispatch jobs
    pub async fn match_event(&self, event: &MatchableEvent) -> Vec<DispatchJobCreation> {
        let subscriptions = self.cache.subscriptions.read().await;
        let now = Utc::now();

        let mut jobs = Vec::new();

        for sub in subscriptions.iter() {
            if sub.matches_event(event) {
                let scheduled_for = if sub.delay_seconds > 0 {
                    now + chrono::Duration::seconds(sub.delay_seconds as i64)
                } else {
                    now
                };

                let job = DispatchJobCreation {
                    id: (self.id_generator)(),
                    event_id: event.id.clone(),
                    subscription_id: sub.id.clone(),
                    subscription_code: sub.code.clone(),
                    client_id: event.client_id.clone().or_else(|| sub.client_id.clone()),
                    event_type_code: event.event_type_code.clone(),
                    correlation_id: event.correlation_id.clone(),
                    source_id: event.source_id.clone(),
                    target: sub.target.clone(),
                    dispatch_pool_id: sub.dispatch_pool_id.clone(),
                    service_account_id: sub.service_account_id.clone(),
                    mode: sub.mode.clone(),
                    delay_seconds: sub.delay_seconds,
                    sequence: sub.sequence,
                    timeout_seconds: sub.timeout_seconds,
                    max_retries: sub.max_retries,
                    data_only: sub.data_only,
                    payload: if sub.data_only {
                        event.data.clone()
                    } else {
                        serde_json::json!({
                            "eventId": event.id,
                            "eventType": event.event_type_code,
                            "correlationId": event.correlation_id,
                            "sourceId": event.source_id,
                            "data": event.data,
                            "createdAt": event.created_at.to_rfc3339(),
                        })
                    },
                    scheduled_for,
                    created_at: now,
                };

                debug!(
                    "Event {} matched subscription {} -> job {}",
                    event.id, sub.code, job.id
                );
                jobs.push(job);
            }
        }

        if jobs.is_empty() {
            debug!("Event {} matched no subscriptions", event.id);
        } else {
            debug!(
                "Event {} matched {} subscriptions",
                event.id,
                jobs.len()
            );
        }

        // Sort by sequence (lower = higher priority)
        jobs.sort_by_key(|j| j.sequence);
        jobs
    }

    /// Match a batch of events
    pub async fn match_batch(&self, events: &[MatchableEvent]) -> Vec<DispatchJobCreation> {
        let mut all_jobs = Vec::new();

        for event in events {
            let jobs = self.match_event(event).await;
            all_jobs.extend(jobs);
        }

        info!(
            "Matched batch of {} events -> {} dispatch jobs",
            events.len(),
            all_jobs.len()
        );

        all_jobs
    }
}

impl Default for SubscriptionCache {
    fn default() -> Self {
        Self::new()
    }
}

#[cfg(test)]
mod tests {
    use super::*;

    fn create_test_subscription(id: &str, pattern: &str) -> MatchableSubscription {
        MatchableSubscription {
            id: id.to_string(),
            code: id.to_string(),
            client_id: None,
            bindings: vec![SubscriptionBinding {
                event_type_pattern: pattern.to_string(),
                filter: None,
            }],
            target: "http://example.com/webhook".to_string(),
            dispatch_pool_id: None,
            service_account_id: None,
            mode: "IMMEDIATE".to_string(),
            delay_seconds: 0,
            sequence: 99,
            timeout_seconds: 30,
            max_retries: 3,
            data_only: false,
            is_active: true,
        }
    }

    fn create_test_event(id: &str, event_type: &str) -> MatchableEvent {
        MatchableEvent {
            id: id.to_string(),
            event_type_code: event_type.to_string(),
            client_id: None,
            correlation_id: None,
            source_id: None,
            data: serde_json::json!({"test": "data"}),
            created_at: Utc::now(),
        }
    }

    #[tokio::test]
    async fn test_exact_match() {
        let cache = Arc::new(SubscriptionCache::new());
        cache.load(vec![
            create_test_subscription("sub1", "orders:fulfillment:shipment:shipped"),
        ]).await;

        let counter = std::sync::atomic::AtomicUsize::new(0);
        let matcher = SubscriptionMatcher::new(cache, move || {
            format!("job-{}", counter.fetch_add(1, std::sync::atomic::Ordering::SeqCst))
        });

        let event = create_test_event("evt1", "orders:fulfillment:shipment:shipped");
        let jobs = matcher.match_event(&event).await;

        assert_eq!(jobs.len(), 1);
        assert_eq!(jobs[0].subscription_id, "sub1");
    }

    #[tokio::test]
    async fn test_wildcard_match() {
        let cache = Arc::new(SubscriptionCache::new());
        cache.load(vec![
            create_test_subscription("sub1", "orders:*:*:*"),
        ]).await;

        let counter = std::sync::atomic::AtomicUsize::new(0);
        let matcher = SubscriptionMatcher::new(cache, move || {
            format!("job-{}", counter.fetch_add(1, std::sync::atomic::Ordering::SeqCst))
        });

        let event = create_test_event("evt1", "orders:fulfillment:shipment:shipped");
        let jobs = matcher.match_event(&event).await;

        assert_eq!(jobs.len(), 1);
    }

    #[tokio::test]
    async fn test_no_match() {
        let cache = Arc::new(SubscriptionCache::new());
        cache.load(vec![
            create_test_subscription("sub1", "payments:*:*:*"),
        ]).await;

        let counter = std::sync::atomic::AtomicUsize::new(0);
        let matcher = SubscriptionMatcher::new(cache, move || {
            format!("job-{}", counter.fetch_add(1, std::sync::atomic::Ordering::SeqCst))
        });

        let event = create_test_event("evt1", "orders:fulfillment:shipment:shipped");
        let jobs = matcher.match_event(&event).await;

        assert_eq!(jobs.len(), 0);
    }

    #[tokio::test]
    async fn test_multiple_subscriptions() {
        let cache = Arc::new(SubscriptionCache::new());
        cache.load(vec![
            create_test_subscription("sub1", "orders:*:*:*"),
            create_test_subscription("sub2", "orders:fulfillment:*:*"),
            create_test_subscription("sub3", "payments:*:*:*"),
        ]).await;

        let counter = std::sync::atomic::AtomicUsize::new(0);
        let matcher = SubscriptionMatcher::new(cache, move || {
            format!("job-{}", counter.fetch_add(1, std::sync::atomic::Ordering::SeqCst))
        });

        let event = create_test_event("evt1", "orders:fulfillment:shipment:shipped");
        let jobs = matcher.match_event(&event).await;

        assert_eq!(jobs.len(), 2); // sub1 and sub2 match
    }
}
