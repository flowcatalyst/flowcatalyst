//! Authorization Service
//!
//! Permission-based access control with role resolution.

use std::collections::HashSet;
use std::sync::Arc;
use std::time::Instant;
use dashmap::DashMap;
use crate::permissions;
use crate::RoleRepository;
use crate::shared::error::{PlatformError, Result};
use crate::AccessTokenClaims;

/// Authorization context for a request
#[derive(Debug, Clone)]
pub struct AuthContext {
    /// Principal ID
    pub principal_id: String,

    /// Principal type (USER or SERVICE)
    pub principal_type: String,

    /// User scope
    pub scope: String,

    /// Email (for users)
    pub email: Option<String>,

    /// Display name
    pub name: String,

    /// Client IDs this principal can access
    pub accessible_clients: Vec<String>,

    /// All permissions (resolved from roles)
    pub permissions: HashSet<String>,

    /// Role codes
    pub roles: Vec<String>,
}

impl AuthContext {
    /// Create from JWT claims with resolved permissions
    pub fn from_claims_with_permissions(
        claims: &AccessTokenClaims,
        permissions: HashSet<String>,
    ) -> Self {
        Self {
            principal_id: claims.sub.clone(),
            principal_type: claims.principal_type.clone(),
            scope: claims.scope.clone(),
            email: claims.email.clone(),
            name: claims.name.clone(),
            accessible_clients: claims.clients.clone(),
            permissions,
            roles: claims.roles.clone(),
        }
    }

    /// Check if this context is for an anchor user
    pub fn is_anchor(&self) -> bool {
        self.scope == "ANCHOR"
    }

    /// Check if this context can access a specific client
    pub fn can_access_client(&self, client_id: &str) -> bool {
        self.accessible_clients.contains(&"*".to_string()) ||
            self.accessible_clients.contains(&client_id.to_string())
    }

    /// Check if this context has a specific permission (4-level pattern matching)
    pub fn has_permission(&self, permission: &str) -> bool {
        // Direct match
        if self.permissions.contains(permission) {
            return true;
        }

        // 4-level wildcard pattern matching
        for pattern in &self.permissions {
            if crate::role::entity::matches_pattern(permission, pattern) {
                return true;
            }
        }

        false
    }

    /// Check if this context has all specified permissions
    pub fn has_all_permissions(&self, required: &[&str]) -> bool {
        required.iter().all(|p| self.has_permission(p))
    }

    /// Check if this context has any of the specified permissions
    pub fn has_any_permission(&self, required: &[&str]) -> bool {
        required.iter().any(|p| self.has_permission(p))
    }

    /// Check if this context has a specific role
    pub fn has_role(&self, role: &str) -> bool {
        self.roles.contains(&role.to_string())
    }
}

/// Cached permission entry with TTL
struct CachedPermissions {
    permissions: HashSet<String>,
    cached_at: Instant,
}

/// Cache TTL for resolved permissions (60 seconds)
const PERMISSION_CACHE_TTL_SECS: u64 = 60;

/// Authorization service for checking permissions
pub struct AuthorizationService {
    role_repo: Arc<RoleRepository>,
    /// Cache: sorted role codes joined by "," → resolved permissions
    permission_cache: DashMap<String, CachedPermissions>,
}

impl AuthorizationService {
    pub fn new(role_repo: Arc<RoleRepository>) -> Self {
        Self {
            role_repo,
            permission_cache: DashMap::new(),
        }
    }

    /// Build an authorization context from JWT claims
    /// Resolves all permissions from roles (cached)
    pub async fn build_context(&self, claims: &AccessTokenClaims) -> Result<AuthContext> {
        let permissions = self.resolve_permissions(&claims.roles).await?;
        Ok(AuthContext::from_claims_with_permissions(claims, permissions))
    }

    /// Resolve all permissions for a set of role codes, with in-memory caching
    async fn resolve_permissions(&self, role_codes: &[String]) -> Result<HashSet<String>> {
        if role_codes.is_empty() {
            return Ok(HashSet::new());
        }

        // Build cache key from sorted role codes
        let mut sorted_codes = role_codes.to_vec();
        sorted_codes.sort();
        let cache_key = sorted_codes.join(",");

        // Check cache
        if let Some(entry) = self.permission_cache.get(&cache_key) {
            if entry.cached_at.elapsed().as_secs() < PERMISSION_CACHE_TTL_SECS {
                return Ok(entry.permissions.clone());
            }
        }

        // Cache miss or expired — query DB
        let roles = self.role_repo.find_by_codes(role_codes).await?;
        let mut permissions = HashSet::new();

        for role in roles {
            permissions.extend(role.permissions);
        }

        // Store in cache
        self.permission_cache.insert(cache_key, CachedPermissions {
            permissions: permissions.clone(),
            cached_at: Instant::now(),
        });

        Ok(permissions)
    }

    /// Check if a principal can perform an action on a resource
    pub fn authorize(
        &self,
        context: &AuthContext,
        permission: &str,
        client_id: Option<&str>,
    ) -> Result<()> {
        // Check permission
        if !context.has_permission(permission) {
            return Err(PlatformError::forbidden(format!(
                "Missing permission: {}",
                permission
            )));
        }

        // Check client access if client-specific
        if let Some(cid) = client_id {
            if !context.can_access_client(cid) {
                return Err(PlatformError::forbidden(format!(
                    "No access to client: {}",
                    cid
                )));
            }
        }

        Ok(())
    }

    /// Require anchor scope
    pub fn require_anchor(&self, context: &AuthContext) -> Result<()> {
        if !context.is_anchor() {
            return Err(PlatformError::forbidden("Anchor scope required"));
        }
        Ok(())
    }

    /// Require specific permission
    pub fn require_permission(&self, context: &AuthContext, permission: &str) -> Result<()> {
        if !context.has_permission(permission) {
            return Err(PlatformError::forbidden(format!(
                "Permission required: {}",
                permission
            )));
        }
        Ok(())
    }

    /// Require client access
    pub fn require_client_access(&self, context: &AuthContext, client_id: &str) -> Result<()> {
        if !context.can_access_client(client_id) {
            return Err(PlatformError::forbidden(format!(
                "Client access required: {}",
                client_id
            )));
        }
        Ok(())
    }
}

/// Common authorization checks
pub mod checks {
    use super::*;

    /// Require anchor scope
    pub fn require_anchor(context: &AuthContext) -> Result<()> {
        if context.is_anchor() {
            Ok(())
        } else {
            Err(PlatformError::forbidden("Anchor access required"))
        }
    }

    /// Check read access to events
    pub fn can_read_events(context: &AuthContext) -> Result<()> {
        if context.has_permission(permissions::admin::EVENT_READ) {
            Ok(())
        } else {
            Err(PlatformError::forbidden("Cannot read events"))
        }
    }

    /// Check raw read access to events (includes payload)
    pub fn can_read_events_raw(context: &AuthContext) -> Result<()> {
        if context.has_permission(permissions::admin::EVENT_VIEW_RAW) {
            Ok(())
        } else {
            Err(PlatformError::forbidden("Cannot read raw event data"))
        }
    }

    /// Check read access to event types
    pub fn can_read_event_types(context: &AuthContext) -> Result<()> {
        if context.has_permission(permissions::admin::EVENT_TYPE_READ) {
            Ok(())
        } else {
            Err(PlatformError::forbidden("Cannot read event types"))
        }
    }

    /// Check create access to event types
    pub fn can_create_event_types(context: &AuthContext) -> Result<()> {
        if context.has_permission(permissions::admin::EVENT_TYPE_CREATE) {
            Ok(())
        } else {
            Err(PlatformError::forbidden("Cannot create event types"))
        }
    }

    /// Check update access to event types
    pub fn can_update_event_types(context: &AuthContext) -> Result<()> {
        if context.has_permission(permissions::admin::EVENT_TYPE_UPDATE) {
            Ok(())
        } else {
            Err(PlatformError::forbidden("Cannot update event types"))
        }
    }

    /// Check delete access to event types
    pub fn can_delete_event_types(context: &AuthContext) -> Result<()> {
        if context.has_permission(permissions::admin::EVENT_TYPE_DELETE) {
            Ok(())
        } else {
            Err(PlatformError::forbidden("Cannot delete event types"))
        }
    }

    /// Check read access to subscriptions
    pub fn can_read_subscriptions(context: &AuthContext) -> Result<()> {
        if context.has_permission(permissions::admin::SUBSCRIPTION_READ) {
            Ok(())
        } else {
            Err(PlatformError::forbidden("Cannot read subscriptions"))
        }
    }

    /// Check create access to subscriptions
    pub fn can_create_subscriptions(context: &AuthContext) -> Result<()> {
        if context.has_permission(permissions::admin::SUBSCRIPTION_CREATE) {
            Ok(())
        } else {
            Err(PlatformError::forbidden("Cannot create subscriptions"))
        }
    }

    /// Check update access to subscriptions
    pub fn can_update_subscriptions(context: &AuthContext) -> Result<()> {
        if context.has_permission(permissions::admin::SUBSCRIPTION_UPDATE) {
            Ok(())
        } else {
            Err(PlatformError::forbidden("Cannot update subscriptions"))
        }
    }

    /// Check delete access to subscriptions
    pub fn can_delete_subscriptions(context: &AuthContext) -> Result<()> {
        if context.has_permission(permissions::admin::SUBSCRIPTION_DELETE) {
            Ok(())
        } else {
            Err(PlatformError::forbidden("Cannot delete subscriptions"))
        }
    }

    /// Check read access to dispatch jobs
    pub fn can_read_dispatch_jobs(context: &AuthContext) -> Result<()> {
        if context.has_permission(permissions::admin::DISPATCH_JOB_READ) {
            Ok(())
        } else {
            Err(PlatformError::forbidden("Cannot read dispatch jobs"))
        }
    }

    /// Check raw read access to dispatch jobs (includes payload)
    pub fn can_read_dispatch_jobs_raw(context: &AuthContext) -> Result<()> {
        if context.has_permission(permissions::admin::DISPATCH_JOB_VIEW_RAW) {
            Ok(())
        } else {
            Err(PlatformError::forbidden("Cannot read raw dispatch job data"))
        }
    }

    /// Check admin access (any admin permission)
    pub fn is_admin(context: &AuthContext) -> Result<()> {
        if context.is_anchor() || context.has_permission(permissions::ADMIN_ALL) {
            Ok(())
        } else {
            Err(PlatformError::forbidden("Admin access required"))
        }
    }

    /// Check write access to events (create)
    pub fn can_write_events(context: &AuthContext) -> Result<()> {
        if context.has_any_permission(&[
            permissions::admin::BATCH_EVENTS_WRITE,
            permissions::application_service::EVENT_CREATE,
        ]) {
            Ok(())
        } else {
            Err(PlatformError::forbidden("Cannot write events"))
        }
    }

    /// Check write access to event types (create, update, or delete)
    pub fn can_write_event_types(context: &AuthContext) -> Result<()> {
        if context.has_any_permission(&[
            permissions::admin::EVENT_TYPE_CREATE,
            permissions::admin::EVENT_TYPE_UPDATE,
            permissions::admin::EVENT_TYPE_DELETE,
        ]) {
            Ok(())
        } else {
            Err(PlatformError::forbidden("Cannot write event types"))
        }
    }

    /// Check write access to subscriptions (create, update, or delete)
    pub fn can_write_subscriptions(context: &AuthContext) -> Result<()> {
        if context.has_any_permission(&[
            permissions::admin::SUBSCRIPTION_CREATE,
            permissions::admin::SUBSCRIPTION_UPDATE,
            permissions::admin::SUBSCRIPTION_DELETE,
        ]) {
            Ok(())
        } else {
            Err(PlatformError::forbidden("Cannot write subscriptions"))
        }
    }

    /// Check create access to dispatch jobs
    pub fn can_create_dispatch_jobs(context: &AuthContext) -> Result<()> {
        if context.has_permission(permissions::admin::BATCH_DISPATCH_JOBS_WRITE) {
            Ok(())
        } else {
            Err(PlatformError::forbidden("Cannot create dispatch jobs"))
        }
    }

    /// Check retry access to dispatch jobs
    pub fn can_retry_dispatch_jobs(context: &AuthContext) -> Result<()> {
        if context.has_permission(permissions::admin::BATCH_DISPATCH_JOBS_WRITE) {
            Ok(())
        } else {
            Err(PlatformError::forbidden("Cannot retry dispatch jobs"))
        }
    }

    /// Check write access to dispatch jobs (batch)
    pub fn can_write_dispatch_jobs(context: &AuthContext) -> Result<()> {
        if context.has_permission(permissions::admin::BATCH_DISPATCH_JOBS_WRITE) {
            Ok(())
        } else {
            Err(PlatformError::forbidden("Cannot write dispatch jobs"))
        }
    }
}

#[cfg(test)]
mod tests {
    use super::*;

    fn create_test_context(permissions: Vec<&str>, scope: &str, clients: Vec<&str>) -> AuthContext {
        AuthContext {
            principal_id: "test123".to_string(),
            principal_type: "USER".to_string(),
            scope: scope.to_string(),
            email: Some("test@example.com".to_string()),
            name: "Test User".to_string(),
            accessible_clients: clients.into_iter().map(String::from).collect(),
            permissions: permissions.into_iter().map(String::from).collect(),
            roles: vec!["test:admin".to_string()],
        }
    }

    #[test]
    fn test_direct_permission() {
        let ctx = create_test_context(vec!["platform:admin:event:read"], "CLIENT", vec!["client1"]);
        assert!(ctx.has_permission("platform:admin:event:read"));
        assert!(!ctx.has_permission("platform:admin:event:create"));
    }

    #[test]
    fn test_wildcard_permission_4_level() {
        let ctx = create_test_context(vec!["platform:admin:*:*"], "CLIENT", vec!["client1"]);
        assert!(ctx.has_permission("platform:admin:event:read"));
        assert!(ctx.has_permission("platform:admin:client:create"));
        assert!(!ctx.has_permission("platform:iam:user:read"));
    }

    #[test]
    fn test_superuser_permission() {
        let ctx = create_test_context(vec!["platform:*:*:*"], "ANCHOR", vec!["*"]);
        assert!(ctx.has_permission("platform:admin:event:read"));
        assert!(ctx.has_permission("platform:iam:user:delete"));
        assert!(ctx.has_permission("platform:auth:oauth-client:read"));
    }

    #[test]
    fn test_client_access() {
        let ctx = create_test_context(vec![], "CLIENT", vec!["client1", "client2"]);
        assert!(ctx.can_access_client("client1"));
        assert!(ctx.can_access_client("client2"));
        assert!(!ctx.can_access_client("client3"));
    }

    #[test]
    fn test_anchor_all_clients() {
        let ctx = create_test_context(vec![], "ANCHOR", vec!["*"]);
        assert!(ctx.can_access_client("any_client"));
        assert!(ctx.can_access_client("another_client"));
    }

    // ── Wildcard permission edge cases ────────────────────────────────

    #[test]
    fn test_wildcard_single_level() {
        // Wildcard at only one level
        let ctx = create_test_context(vec!["platform:admin:event:*"], "CLIENT", vec![]);
        assert!(ctx.has_permission("platform:admin:event:read"));
        assert!(ctx.has_permission("platform:admin:event:create"));
        assert!(ctx.has_permission("platform:admin:event:delete"));
        // Different aggregate should not match
        assert!(!ctx.has_permission("platform:admin:client:read"));
    }

    #[test]
    fn test_wildcard_context_level() {
        let ctx = create_test_context(vec!["platform:*:event:read"], "CLIENT", vec![]);
        assert!(ctx.has_permission("platform:admin:event:read"));
        assert!(ctx.has_permission("platform:iam:event:read"));
        assert!(!ctx.has_permission("platform:admin:event:create"));
    }

    #[test]
    fn test_non_four_level_permission_no_match() {
        // Permissions with != 4 parts should never match wildcard patterns
        let ctx = create_test_context(vec!["platform:*:*:*"], "ANCHOR", vec!["*"]);
        assert!(!ctx.has_permission("platform:admin"));
        assert!(!ctx.has_permission("platform:admin:event"));
        assert!(!ctx.has_permission("a:b:c:d:e"));
        assert!(!ctx.has_permission(""));
    }

    #[test]
    fn test_no_wildcard_in_permission_itself() {
        // The permission being checked should not use wildcards — only patterns do
        let ctx = create_test_context(vec!["platform:admin:event:read"], "CLIENT", vec![]);
        // Checking a wildcard as a "permission" — should only match the literal string
        assert!(!ctx.has_permission("platform:admin:*:read"));
    }

    // ── Empty roles / permissions ─────────────────────────────────────

    #[test]
    fn test_empty_permissions_denies_all() {
        let ctx = create_test_context(vec![], "CLIENT", vec!["client1"]);
        assert!(!ctx.has_permission("platform:admin:event:read"));
        assert!(!ctx.has_permission("anything"));
    }

    #[test]
    fn test_empty_roles_list() {
        let ctx = AuthContext {
            principal_id: "p1".to_string(),
            principal_type: "USER".to_string(),
            scope: "CLIENT".to_string(),
            email: None,
            name: "No Roles".to_string(),
            accessible_clients: vec![],
            permissions: HashSet::new(),
            roles: vec![],
        };
        assert!(!ctx.has_role("admin"));
        assert!(!ctx.has_permission("anything"));
    }

    // ── has_all_permissions / has_any_permission ──────────────────────

    #[test]
    fn test_has_all_permissions_all_present() {
        let ctx = create_test_context(
            vec!["platform:admin:event:read", "platform:admin:client:read"],
            "CLIENT",
            vec![],
        );
        assert!(ctx.has_all_permissions(&[
            "platform:admin:event:read",
            "platform:admin:client:read",
        ]));
    }

    #[test]
    fn test_has_all_permissions_one_missing() {
        let ctx = create_test_context(
            vec!["platform:admin:event:read"],
            "CLIENT",
            vec![],
        );
        assert!(!ctx.has_all_permissions(&[
            "platform:admin:event:read",
            "platform:admin:client:read",
        ]));
    }

    #[test]
    fn test_has_all_permissions_empty_required() {
        let ctx = create_test_context(vec![], "CLIENT", vec![]);
        // Empty required set — trivially true
        assert!(ctx.has_all_permissions(&[]));
    }

    #[test]
    fn test_has_any_permission_one_present() {
        let ctx = create_test_context(
            vec!["platform:admin:event:read"],
            "CLIENT",
            vec![],
        );
        assert!(ctx.has_any_permission(&[
            "platform:admin:event:read",
            "platform:admin:client:read",
        ]));
    }

    #[test]
    fn test_has_any_permission_none_present() {
        let ctx = create_test_context(vec![], "CLIENT", vec![]);
        assert!(!ctx.has_any_permission(&[
            "platform:admin:event:read",
        ]));
    }

    #[test]
    fn test_has_any_permission_empty_required() {
        let ctx = create_test_context(vec![], "CLIENT", vec![]);
        // Empty required set — none match
        assert!(!ctx.has_any_permission(&[]));
    }

    // ── has_role ──────────────────────────────────────────────────────

    #[test]
    fn test_has_role_present() {
        let ctx = create_test_context(vec![], "CLIENT", vec![]);
        assert!(ctx.has_role("test:admin"));
    }

    #[test]
    fn test_has_role_absent() {
        let ctx = create_test_context(vec![], "CLIENT", vec![]);
        assert!(!ctx.has_role("nonexistent:role"));
    }

    // ── is_anchor ─────────────────────────────────────────────────────

    #[test]
    fn test_is_anchor_true() {
        let ctx = create_test_context(vec![], "ANCHOR", vec!["*"]);
        assert!(ctx.is_anchor());
    }

    #[test]
    fn test_is_anchor_false_for_client_scope() {
        let ctx = create_test_context(vec![], "CLIENT", vec![]);
        assert!(!ctx.is_anchor());
    }

    #[test]
    fn test_is_anchor_false_for_partner_scope() {
        let ctx = create_test_context(vec![], "PARTNER", vec![]);
        assert!(!ctx.is_anchor());
    }

    // ── Client access edge cases ──────────────────────────────────────

    #[test]
    fn test_no_clients_denies_all() {
        let ctx = create_test_context(vec![], "CLIENT", vec![]);
        assert!(!ctx.can_access_client("anything"));
    }

    #[test]
    fn test_client_access_exact_match_only() {
        let ctx = create_test_context(vec![], "CLIENT", vec!["client1"]);
        assert!(ctx.can_access_client("client1"));
        assert!(!ctx.can_access_client("client10")); // no prefix matching
        assert!(!ctx.can_access_client("client")); // no partial matching
    }

    // ── from_claims_with_permissions ──────────────────────────────────

    #[test]
    fn test_from_claims_preserves_all_fields() {
        let claims = AccessTokenClaims {
            sub: "principal_1".to_string(),
            iss: "https://auth.example.com".to_string(),
            aud: "api".to_string(),
            exp: 1700000000,
            iat: 1699996400,
            nbf: 1699996400,
            jti: "jwt-id-1".to_string(),
            principal_type: "SERVICE".to_string(),
            scope: "ANCHOR".to_string(),
            email: Some("svc@test.com".to_string()),
            name: "Service Account".to_string(),
            clients: vec!["*".to_string()],
            roles: vec!["platform:super-admin".to_string()],
            applications: vec!["app1".to_string()],
        };
        let mut perms = HashSet::new();
        perms.insert("platform:*:*:*".to_string());

        let ctx = AuthContext::from_claims_with_permissions(&claims, perms);
        assert_eq!(ctx.principal_id, "principal_1");
        assert_eq!(ctx.principal_type, "SERVICE");
        assert_eq!(ctx.scope, "ANCHOR");
        assert_eq!(ctx.email, Some("svc@test.com".to_string()));
        assert_eq!(ctx.name, "Service Account");
        assert!(ctx.can_access_client("any_client"));
        assert!(ctx.is_anchor());
        assert!(ctx.has_permission("platform:admin:event:read"));
    }

    // ── Authorization checks module ───────────────────────────────────

    #[test]
    fn test_check_require_anchor_passes() {
        let ctx = create_test_context(vec![], "ANCHOR", vec!["*"]);
        assert!(checks::require_anchor(&ctx).is_ok());
    }

    #[test]
    fn test_check_require_anchor_fails() {
        let ctx = create_test_context(vec![], "CLIENT", vec![]);
        assert!(checks::require_anchor(&ctx).is_err());
    }

    #[test]
    fn test_check_is_admin_with_superuser() {
        let ctx = create_test_context(vec![permissions::ADMIN_ALL], "ANCHOR", vec!["*"]);
        assert!(checks::is_admin(&ctx).is_ok());
    }

    #[test]
    fn test_check_is_admin_anchor_scope_only() {
        // Anchor scope alone is sufficient for is_admin
        let ctx = create_test_context(vec![], "ANCHOR", vec!["*"]);
        assert!(checks::is_admin(&ctx).is_ok());
    }

    #[test]
    fn test_check_is_admin_fails_for_normal_user() {
        let ctx = create_test_context(vec!["platform:admin:event:read"], "CLIENT", vec!["c1"]);
        assert!(checks::is_admin(&ctx).is_err());
    }

    #[test]
    fn test_can_read_events_with_permission() {
        let ctx = create_test_context(
            vec![permissions::admin::EVENT_READ],
            "CLIENT",
            vec!["c1"],
        );
        assert!(checks::can_read_events(&ctx).is_ok());
    }

    #[test]
    fn test_can_read_events_without_permission() {
        let ctx = create_test_context(vec![], "CLIENT", vec!["c1"]);
        assert!(checks::can_read_events(&ctx).is_err());
    }

    #[test]
    fn test_can_write_events_with_batch_permission() {
        let ctx = create_test_context(
            vec![permissions::admin::BATCH_EVENTS_WRITE],
            "CLIENT",
            vec!["c1"],
        );
        assert!(checks::can_write_events(&ctx).is_ok());
    }

    #[test]
    fn test_can_write_events_with_app_permission() {
        let ctx = create_test_context(
            vec![permissions::application_service::EVENT_CREATE],
            "CLIENT",
            vec!["c1"],
        );
        assert!(checks::can_write_events(&ctx).is_ok());
    }

    #[test]
    fn test_wildcard_permission_satisfies_check() {
        // platform:*:*:* should satisfy any specific permission check
        let ctx = create_test_context(vec!["platform:*:*:*"], "ANCHOR", vec!["*"]);
        assert!(checks::can_read_events(&ctx).is_ok());
        assert!(checks::can_read_event_types(&ctx).is_ok());
        assert!(checks::can_read_subscriptions(&ctx).is_ok());
        assert!(checks::can_read_dispatch_jobs(&ctx).is_ok());
    }
}
