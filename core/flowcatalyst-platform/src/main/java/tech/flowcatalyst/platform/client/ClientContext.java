package tech.flowcatalyst.platform.client;

import jakarta.enterprise.context.RequestScoped;
import jakarta.inject.Inject;
import jakarta.ws.rs.ForbiddenException;
import tech.flowcatalyst.platform.principal.Principal;
import tech.flowcatalyst.platform.principal.PrincipalRepository;

import java.util.Collection;
import java.util.Set;

/**
 * Request-scoped context for client access validation.
 *
 * Use this in JAX-RS resources to validate and enforce client-scoped access.
 * The principal must be set (typically by a filter) before using access methods.
 *
 * Usage in resources:
 * <pre>
 * {@code
 * @Inject
 * ClientContext clientContext;
 *
 * @POST
 * @Path("/users")
 * public Response createUser(CreateUserRequest request) {
 *     // Validate access before proceeding
 *     clientContext.requireAccessTo(request.clientId());
 *
 *     // Safe to proceed - client access is validated
 *     return userService.create(request);
 * }
 *
 * @GET
 * @Path("/users")
 * public Response listUsers(@QueryParam("clientId") Long clientId) {
 *     if (clientId != null) {
 *         clientContext.requireAccessTo(clientId);
 *         return userService.listByClient(clientId);
 *     } else {
 *         // Return for all accessible clients
 *         return userService.listByClients(clientContext.getAccessibleClientIds());
 *     }
 * }
 * }
 * </pre>
 */
@RequestScoped
public class ClientContext {

    @Inject
    ClientAccessService clientAccessService;

    @Inject
    PrincipalRepository principalRepo;

    private String principalId;
    private Principal principal;
    private Set<String> accessibleClientIds;
    private Boolean isAnchorUser;

    /**
     * Set the principal ID for this request context.
     * Should be called by authentication filter.
     */
    public void setPrincipalId(String principalId) {
        this.principalId = principalId;
        // Reset cached values
        this.principal = null;
        this.accessibleClientIds = null;
        this.isAnchorUser = null;
    }

    /**
     * Get the current principal ID.
     */
    public String getPrincipalId() {
        return principalId;
    }

    /**
     * Get the current principal, loading from DB if needed.
     */
    public Principal getPrincipal() {
        if (principal == null && principalId != null) {
            principal = principalRepo.findById(principalId);
        }
        return principal;
    }

    /**
     * Check if the current user is from an anchor domain (has global client access).
     */
    public boolean isAnchorUser() {
        if (isAnchorUser == null) {
            Principal p = getPrincipal();
            isAnchorUser = p != null && clientAccessService.isAnchorDomainUser(p);
        }
        return isAnchorUser;
    }

    /**
     * Get all client IDs the current principal can access.
     * Results are cached for the duration of the request.
     *
     * @return Set of accessible client IDs (empty if no principal set)
     */
    public Set<String> getAccessibleClientIds() {
        if (accessibleClientIds == null) {
            Principal p = getPrincipal();
            if (p != null) {
                accessibleClientIds = clientAccessService.getAccessibleClients(p);
            } else {
                accessibleClientIds = Set.of();
            }
        }
        return accessibleClientIds;
    }

    /**
     * Check if the current principal can access a specific client.
     *
     * @param clientId The client ID to check
     * @return true if the principal can access the client
     */
    public boolean hasAccessTo(Long clientId) {
        if (clientId == null) {
            return false;
        }
        return getAccessibleClientIds().contains(clientId);
    }

    /**
     * Check if the current principal can access ALL of the specified clients.
     *
     * @param clientIds The client IDs to check
     * @return true if the principal can access all specified clients
     */
    public boolean hasAccessToAll(Collection<Long> clientIds) {
        if (clientIds == null || clientIds.isEmpty()) {
            return true;
        }
        return getAccessibleClientIds().containsAll(clientIds);
    }

    /**
     * Require access to a specific client, throwing ForbiddenException if not allowed.
     * Use this for explicit validation in resource methods.
     *
     * @param clientId The client ID to validate access to
     * @throws ForbiddenException if access is denied
     * @throws IllegalStateException if no principal is set
     */
    public void requireAccessTo(Long clientId) {
        requirePrincipal();

        if (clientId == null) {
            throw new ForbiddenException("Client ID is required");
        }

        if (!hasAccessTo(clientId)) {
            throw new ForbiddenException("Access denied to client: " + clientId);
        }
    }

    /**
     * Require access to all specified clients, throwing ForbiddenException if not allowed.
     *
     * @param clientIds The client IDs to validate access to
     * @throws ForbiddenException if access is denied to any client
     * @throws IllegalStateException if no principal is set
     */
    public void requireAccessToAll(Collection<String> clientIds) {
        requirePrincipal();

        if (clientIds == null || clientIds.isEmpty()) {
            return;
        }

        Set<String> accessible = getAccessibleClientIds();
        for (String clientId : clientIds) {
            if (!accessible.contains(clientId)) {
                throw new ForbiddenException("Access denied to client: " + clientId);
            }
        }
    }

    /**
     * Get the principal's home client ID (may be null for anchor users or partners).
     *
     * @return The home client ID, or null if not set
     */
    public String getHomeClientId() {
        Principal p = getPrincipal();
        return p != null ? p.clientId : null;
    }

    /**
     * Require that a principal is set in the context.
     *
     * @throws IllegalStateException if no principal is set
     */
    public void requirePrincipal() {
        if (principalId == null) {
            throw new IllegalStateException("ClientContext: No principal set - authentication required");
        }
    }

    /**
     * Check if a principal is set in the context.
     */
    public boolean hasPrincipal() {
        return principalId != null;
    }
}
