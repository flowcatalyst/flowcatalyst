//! Result types returned by `DefinitionSynchronizer::sync`.
//!
//! Category order: roles, event types, dispatch pools, connections,
//! subscriptions, processes, scheduled jobs, principals, openapi, docs.
//!
//! **Failures are data, not `Err`:** a failed category does not stop the
//! sync, so always check the result — call [`SyncResult::into_result`] to
//! turn any failed category into a [`DefinitionSyncError`].
//!
//! Per-category outcomes are stored as `Option<CategorySyncResult>` so callers
//! can distinguish "skipped" (`None`) from "ran with zero rows changed"
//! (`Some(..)` with all counts at 0). Failures don't propagate as `Err` —
//! they're captured on the per-category result's `error` field so the
//! caller can see what partially succeeded.

/// Per-category outcome counts plus an optional error message.
///
/// `synced_codes` carries the platform's `syncedCodes` field for the
/// categories that return it (all except processes, which returns just
/// the counts). The orchestrator normalises a missing list to an empty
/// `Vec`.
#[derive(Debug, Clone, Default)]
pub struct CategorySyncResult {
    pub created: u32,
    pub updated: u32,
    pub deleted: u32,
    pub synced_codes: Vec<String>,
    /// Principals only: the existing users (emails) whose `password_hash`
    /// the platform did not apply.
    pub password_hash_ignored: Vec<String>,
    pub error: Option<String>,
}

impl CategorySyncResult {
    /// Construct a result from a transport failure. Counts are zero and
    /// `error` carries the message.
    pub fn from_error(error: impl Into<String>) -> Self {
        Self {
            error: Some(error.into()),
            ..Self::default()
        }
    }

    pub fn is_error(&self) -> bool {
        self.error.is_some()
    }

    pub fn touched(&self) -> u32 {
        self.created + self.updated + self.deleted
    }
}

/// Aggregate result of syncing a full `DefinitionSet`. Each category is
/// either `Some(..)` (the orchestrator hit that endpoint) or `None`
/// (skipped because the category was empty or disabled via `SyncOptions`).
///
/// A category failure is recorded here, not returned as an error. Callers
/// should use [`into_result`](Self::into_result) (or at least
/// [`has_errors`](Self::has_errors)) so a failed sync is never ignored.
#[must_use = "a failed category is only recorded on the result; call `into_result()` or check `has_errors()`"]
#[derive(Debug, Clone, Default)]
pub struct SyncResult {
    pub application_code: String,
    pub roles: Option<CategorySyncResult>,
    pub event_types: Option<CategorySyncResult>,
    pub subscriptions: Option<CategorySyncResult>,
    pub dispatch_pools: Option<CategorySyncResult>,
    pub connections: Option<CategorySyncResult>,
    pub principals: Option<CategorySyncResult>,
    pub processes: Option<CategorySyncResult>,
    pub scheduled_jobs: Option<CategorySyncResult>,
    /// OpenAPI sync is a single-spec upload, not a per-row list — the
    /// `synced_codes` field always contains exactly the publish-version
    /// string on success.
    pub openapi: Option<CategorySyncResult>,
    pub docs: Option<CategorySyncResult>,
}

/// One or more categories of a sync failed. Carries the full result so the
/// categories that succeeded are still visible.
#[derive(Debug, Clone)]
pub struct DefinitionSyncError {
    /// The complete result, including successful categories.
    pub result: Box<SyncResult>,
    /// `(category, message)` for each failed category, in sync order.
    pub failed: Vec<(String, String)>,
}

impl std::fmt::Display for DefinitionSyncError {
    fn fmt(&self, f: &mut std::fmt::Formatter<'_>) -> std::fmt::Result {
        write!(
            f,
            "definition sync for application \"{}\" failed in {} categor{}: ",
            self.result.application_code,
            self.failed.len(),
            if self.failed.len() == 1 { "y" } else { "ies" }
        )?;
        for (i, (cat, msg)) in self.failed.iter().enumerate() {
            if i > 0 {
                f.write_str("; ")?;
            }
            write!(f, "{cat}: {msg}")?;
        }
        Ok(())
    }
}

impl std::error::Error for DefinitionSyncError {}

impl SyncResult {
    /// Stable category iteration order matching the orchestrator's call
    /// sequence — roles → event_types → dispatch_pools → connections →
    /// subscriptions → processes → scheduled_jobs → principals → openapi →
    /// docs.
    fn categories(&self) -> [(&'static str, Option<&CategorySyncResult>); 10] {
        [
            ("roles", self.roles.as_ref()),
            ("eventTypes", self.event_types.as_ref()),
            ("dispatchPools", self.dispatch_pools.as_ref()),
            ("connections", self.connections.as_ref()),
            ("subscriptions", self.subscriptions.as_ref()),
            ("processes", self.processes.as_ref()),
            ("scheduledJobs", self.scheduled_jobs.as_ref()),
            ("principals", self.principals.as_ref()),
            ("openapi", self.openapi.as_ref()),
            ("docs", self.docs.as_ref()),
        ]
    }

    /// `Ok(self)` when no category failed, otherwise a
    /// [`DefinitionSyncError`] listing the failed categories (and carrying
    /// the full result). Prefer this over ignoring the result.
    pub fn into_result(self) -> Result<SyncResult, DefinitionSyncError> {
        let failed: Vec<(String, String)> = self
            .errors()
            .into_iter()
            .map(|(c, m)| (c.to_string(), m.to_string()))
            .collect();
        if failed.is_empty() {
            Ok(self)
        } else {
            Err(DefinitionSyncError {
                result: Box::new(self),
                failed,
            })
        }
    }

    /// True when at least one category recorded a non-zero count.
    pub fn has_changes(&self) -> bool {
        self.categories()
            .iter()
            .any(|(_, r)| matches!(r, Some(c) if c.touched() > 0))
    }

    /// True when at least one category captured an error.
    pub fn has_errors(&self) -> bool {
        self.categories()
            .iter()
            .any(|(_, r)| matches!(r, Some(c) if c.is_error()))
    }

    /// Per-category errors as (category_name, message) pairs.
    pub fn errors(&self) -> Vec<(&'static str, &str)> {
        self.categories()
            .iter()
            .filter_map(|(name, r)| match r {
                Some(c) => c.error.as_deref().map(|msg| (*name, msg)),
                None => None,
            })
            .collect()
    }

    /// Summed `(created, updated, deleted)` counts across every category
    /// that ran.
    pub fn totals(&self) -> (u32, u32, u32) {
        self.categories()
            .iter()
            .fold((0, 0, 0), |(c, u, d), (_, r)| match r {
                Some(cat) => (c + cat.created, u + cat.updated, d + cat.deleted),
                None => (c, u, d),
            })
    }
}

#[cfg(test)]
mod tests {
    use super::*;

    fn cat(created: u32, updated: u32, deleted: u32) -> CategorySyncResult {
        CategorySyncResult {
            created,
            updated,
            deleted,
            synced_codes: Vec::new(),
            password_hash_ignored: Vec::new(),
            error: None,
        }
    }

    #[test]
    fn empty_result_has_no_changes_or_errors() {
        let r = SyncResult::default();
        assert!(!r.has_changes());
        assert!(!r.has_errors());
        assert_eq!(r.totals(), (0, 0, 0));
        assert!(r.errors().is_empty());
    }

    #[test]
    fn totals_sums_across_categories() {
        let r = SyncResult {
            roles: Some(cat(1, 2, 0)),
            event_types: Some(cat(0, 0, 3)),
            processes: Some(cat(5, 1, 0)),
            ..SyncResult::default()
        };
        assert_eq!(r.totals(), (6, 3, 3));
        assert!(r.has_changes());
        assert!(!r.has_errors());
    }

    #[test]
    fn errors_collected_by_category_name() {
        let r = SyncResult {
            roles: Some(cat(1, 0, 0)),
            event_types: Some(CategorySyncResult::from_error("validation failed")),
            dispatch_pools: Some(CategorySyncResult::from_error("network timeout")),
            ..SyncResult::default()
        };
        assert!(r.has_errors());
        let errors = r.errors();
        assert_eq!(errors.len(), 2);
        assert_eq!(errors[0].0, "eventTypes");
        assert_eq!(errors[0].1, "validation failed");
        assert_eq!(errors[1].0, "dispatchPools");
    }

    #[test]
    fn into_result_is_ok_without_errors_and_err_listing_failed_categories() {
        let ok = SyncResult {
            roles: Some(cat(1, 0, 0)),
            ..SyncResult::default()
        };
        assert!(ok.into_result().is_ok());

        let bad = SyncResult {
            application_code: "orders".into(),
            roles: Some(cat(1, 0, 0)),
            connections: Some(CategorySyncResult::from_error("boom")),
            docs: Some(CategorySyncResult::from_error("bad slug")),
            ..SyncResult::default()
        };
        let err = bad.into_result().unwrap_err();
        assert_eq!(err.failed.len(), 2);
        assert_eq!(err.failed[0].0, "connections");
        assert_eq!(err.result.roles.as_ref().map(|r| r.created), Some(1));
        let msg = err.to_string();
        assert!(msg.contains("connections: boom") && msg.contains("docs: bad slug"));
    }

    #[test]
    fn category_from_error_zeroes_counts() {
        let c = CategorySyncResult::from_error("boom");
        assert_eq!(c.created, 0);
        assert_eq!(c.touched(), 0);
        assert!(c.is_error());
    }
}
