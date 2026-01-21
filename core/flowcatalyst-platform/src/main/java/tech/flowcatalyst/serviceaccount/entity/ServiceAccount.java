package tech.flowcatalyst.serviceaccount.entity;

import tech.flowcatalyst.platform.principal.Principal;

import java.time.Instant;
import java.util.ArrayList;
import java.util.List;
import java.util.Set;
import java.util.stream.Collectors;

/**
 * Service account for machine-to-machine authentication and webhook signing.
 *
 * <p>Service accounts are standalone entities that can be:</p>
 * <ul>
 *   <li>Associated with an Application (for integration service accounts)</li>
 *   <li>Standalone (for custom service accounts)</li>
 *   <li>Scoped to a specific client (for multi-tenant deployments)</li>
 * </ul>
 *
 * <p>Each service account has embedded webhook credentials for:</p>
 * <ul>
 *   <li>Authentication (Bearer or Basic)</li>
 *   <li>Signing outbound webhooks (HMAC-SHA256)</li>
 * </ul>
 */

public class ServiceAccount {

    public String id;

    /**
     * Unique code identifier (e.g., "tms-service", "sf-integration").
     * Used for lookups and display.
     */
    public String code;

    /**
     * Human-readable display name.
     */
    public String name;

    /**
     * Optional description of what this service account is used for.
     */
    public String description;

    /**
     * List of client IDs this service account has access to.
     * When empty, the service account has no client restrictions.
     * When populated, the service account can only operate within these clients' scopes.
     */
    public List<String> clientIds = new ArrayList<>();

    /**
     * Optional application ID if this service account was created for an application.
     * When set, this service account is managed by the application lifecycle.
     */
    public String applicationId;

    /**
     * Whether this service account is active and can be used.
     */
    public boolean active = true;

    /**
     * Embedded webhook credentials for authentication and signing.
     */
    public WebhookCredentials webhookCredentials;

    /**
     * Denormalized role assignments (same pattern as Principal).
     */
    public List<Principal.RoleAssignment> roles = new ArrayList<>();

    /**
     * When this service account was last used for authentication or webhook dispatch.
     */
    public Instant lastUsedAt;

    public Instant createdAt;

    public Instant updatedAt;

    public ServiceAccount() {
    }

    /**
     * Get role names as a set for quick lookup.
     */
    public Set<String> getRoleNames() {
        return roles.stream()
            .map(r -> r.roleName)
            .collect(Collectors.toSet());
    }

    /**
     * Check if service account has a specific role.
     */
    public boolean hasRole(String roleName) {
        return roles.stream().anyMatch(r -> r.roleName.equals(roleName));
    }
}
