package tech.flowcatalyst.platform.audit;

import jakarta.enterprise.context.RequestScoped;
import jakarta.inject.Inject;
import jakarta.ws.rs.NotAuthorizedException;
import jakarta.ws.rs.core.Response;
import tech.flowcatalyst.platform.cache.PrincipalCacheService;
import tech.flowcatalyst.platform.principal.Principal;
import tech.flowcatalyst.platform.principal.PrincipalRepository;
import tech.flowcatalyst.platform.principal.PrincipalType;
import tech.flowcatalyst.platform.principal.UserScope;
import tech.flowcatalyst.platform.shared.TsidGenerator;

import java.util.List;
import java.util.Optional;
import java.util.Set;

/**
 * Request-scoped context holding the current principal for audit logging and authorization.
 *
 * Can be populated:
 * - Automatically via AuditContextFilter (for HTTP requests)
 * - Manually via setPrincipalId() for tests
 * - Via setSystemPrincipal() for background jobs, CLI tools, and startup tasks
 *
 * The SYSTEM principal is a special service account used for automated operations
 * that occur outside of a user request context.
 *
 * <p>Authorization data (roles, client access) is loaded from the database (with caching),
 * not from token claims. This ensures the platform has real-time control over access
 * and prevents stale token claims.
 */
@RequestScoped
public class AuditContext {

    public static final String SYSTEM_PRINCIPAL_CODE = "SYSTEM";
    public static final String SYSTEM_PRINCIPAL_NAME = "System";

    @Inject
    PrincipalRepository principalRepo;

    @Inject
    PrincipalCacheService principalCache;

    private String principalId;
    private String principalType; // "USER", "SERVICE", or "SYSTEM"
    private Principal cachedPrincipal; // Lazily loaded from cache/database

    /**
     * Set principal ID manually (for tests, background jobs, CLI tools).
     */
    public void setPrincipalId(String principalId) {
        this.principalId = principalId;
        this.principalType = "USER";
    }

    /**
     * Set the context to use the SYSTEM principal for automated operations.
     * Creates the SYSTEM principal if it doesn't exist.
     */
    public void setSystemPrincipal() {
        Principal systemPrincipal = getOrCreateSystemPrincipal();
        this.principalId = systemPrincipal.id;
        this.principalType = "SYSTEM";
    }

    /**
     * Get the current principal ID, or null if not set.
     */
    public String getPrincipalId() {
        return principalId;
    }

    /**
     * Get the current principal type ("USER", "SERVICE", or "SYSTEM").
     */
    public String getPrincipalType() {
        return principalType;
    }

    /**
     * Get the current principal ID, throwing if not set.
     * Use this when audit context is required.
     * Throws NotAuthorizedException (401) if not authenticated.
     */
    public String requirePrincipalId() {
        if (principalId == null) {
            throw new NotAuthorizedException(
                Response.status(Response.Status.UNAUTHORIZED)
                    .entity("{\"error\":\"Authentication required\"}")
                    .type("application/json")
                    .build()
            );
        }
        return principalId;
    }

    /**
     * Check if principal ID is set.
     */
    public boolean isSet() {
        return principalId != null;
    }

    /**
     * Check if this is the system principal.
     */
    public boolean isSystemPrincipal() {
        return "SYSTEM".equals(principalType);
    }

    // ========================================================================
    // Authorization Methods (loaded from database, cached)
    // ========================================================================

    /**
     * Get the current principal entity, loaded from database with caching.
     *
     * @return The principal, or empty if not found or not authenticated
     */
    public Optional<Principal> getPrincipal() {
        if (principalId == null) {
            return Optional.empty();
        }
        if (cachedPrincipal == null) {
            cachedPrincipal = principalCache.getById(principalId).orElse(null);
        }
        return Optional.ofNullable(cachedPrincipal);
    }

    /**
     * Get the current principal, throwing if not authenticated.
     *
     * @return The principal
     * @throws NotAuthorizedException if not authenticated
     */
    public Principal requirePrincipal() {
        return getPrincipal().orElseThrow(() -> new NotAuthorizedException(
            Response.status(Response.Status.UNAUTHORIZED)
                .entity("{\"error\":\"Authentication required\"}")
                .type("application/json")
                .build()
        ));
    }

    /**
     * Check if the current principal has access to all clients (ANCHOR scope).
     *
     * @return true if the principal can access all clients
     */
    public boolean hasAccessToAllClients() {
        return getPrincipal()
            .map(p -> p.scope == UserScope.ANCHOR)
            .orElse(false);
    }

    /**
     * Check if the current principal has access to a specific client.
     *
     * @param clientId The client ID to check
     * @return true if access is allowed
     */
    public boolean hasAccessToClient(String clientId) {
        Optional<Principal> principal = getPrincipal();
        if (principal.isEmpty()) {
            return false;
        }

        Principal p = principal.get();

        // ANCHOR scope has access to all clients
        if (p.scope == UserScope.ANCHOR) {
            return true;
        }

        // CLIENT scope only has access to their home client
        if (p.scope == UserScope.CLIENT) {
            return clientId != null && clientId.equals(p.clientId);
        }

        // PARTNER scope - would need to check partner_client_access table
        // For now, allow access to home client
        if (p.scope == UserScope.PARTNER) {
            return clientId != null && clientId.equals(p.clientId);
            // TODO: Check partner_client_access table for additional clients
        }

        return false;
    }

    /**
     * Get the roles assigned to the current principal.
     *
     * @return Set of role names, or empty set if not authenticated
     */
    public Set<String> getRoles() {
        return getPrincipal()
            .map(Principal::getRoleNames)
            .orElse(Set.of());
    }

    /**
     * Check if the current principal has a specific role.
     *
     * @param roleName The role name to check
     * @return true if the principal has the role
     */
    public boolean hasRole(String roleName) {
        return getRoles().contains(roleName);
    }

    /**
     * Get the home client ID for the current principal.
     *
     * @return The client ID, or empty if no home client
     */
    public Optional<String> getHomeClientId() {
        return getPrincipal().map(p -> p.clientId);
    }

    /**
     * Get or create the SYSTEM service principal.
     */
    private Principal getOrCreateSystemPrincipal() {
        // Look for existing SYSTEM principal by service account code
        Optional<Principal> existing = principalRepo.findByServiceAccountCode(SYSTEM_PRINCIPAL_CODE);
        if (existing.isPresent()) {
            return existing.get();
        }

        // Create the SYSTEM principal
        Principal system = new Principal();
        system.id = TsidGenerator.generate();
        system.type = PrincipalType.SERVICE;
        system.name = SYSTEM_PRINCIPAL_NAME;
        system.active = true;
        system.clientId = null; // Platform-level, no client

        system.serviceAccount = new tech.flowcatalyst.platform.principal.ServiceAccount();
        system.serviceAccount.code = SYSTEM_PRINCIPAL_CODE;
        system.serviceAccount.description = "System principal for automated operations (background jobs, startup tasks, CLI tools)";

        principalRepo.persist(system);
        return system;
    }
}
