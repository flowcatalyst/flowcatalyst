package tech.flowcatalyst.platform.authentication;

import jakarta.inject.Inject;
import jakarta.ws.rs.*;
import jakarta.ws.rs.core.*;
import org.eclipse.microprofile.openapi.annotations.Operation;
import org.eclipse.microprofile.openapi.annotations.media.Content;
import org.eclipse.microprofile.openapi.annotations.media.Schema;
import org.eclipse.microprofile.openapi.annotations.responses.APIResponse;
import org.eclipse.microprofile.openapi.annotations.tags.Tag;
import org.jboss.logging.Logger;
import tech.flowcatalyst.platform.authorization.PermissionRegistry;
import tech.flowcatalyst.platform.principal.Principal;
import tech.flowcatalyst.platform.principal.PrincipalRepository;
import tech.flowcatalyst.platform.client.Client;
import tech.flowcatalyst.platform.client.ClientAccessService;
import tech.flowcatalyst.platform.client.ClientRepository;
import tech.flowcatalyst.platform.shared.EntityType;
import tech.flowcatalyst.platform.shared.TypedId;

import java.util.List;
import java.util.Set;
import java.util.stream.Collectors;

/**
 * Endpoints for client selection and context switching.
 *
 * Users with access to multiple clients can:
 * 1. List accessible clients
 * 2. Switch client context (get new token with client claim)
 *
 * This is particularly useful for:
 * - Multi-client users (consultants, support staff)
 * - Anchor domain users (global access)
 * - Partner users with cross-client access grants
 */
@Path("/auth/client")
@Tag(name = "Client Selection", description = "Client context management")
@Produces(MediaType.APPLICATION_JSON)
@Consumes(MediaType.APPLICATION_JSON)
@EmbeddedModeOnly
public class ClientSelectionResource {

    private static final Logger LOG = Logger.getLogger(ClientSelectionResource.class);

    @Inject
    JwtKeyService jwtKeyService;

    @Inject
    PrincipalRepository principalRepo;

    @Inject
    ClientRepository clientRepo;

    @Inject
    ClientAccessService clientAccessService;

    @Inject
    PermissionRegistry permissionRegistry;

    @Inject
    AuthConfig authConfig;

    /**
     * Get list of clients the current user can access.
     *
     * Returns client information including the user's current client context.
     */
    @GET
    @Path("/accessible")
    @Operation(summary = "List accessible clients for current user")
    @APIResponse(responseCode = "200", description = "List of accessible clients",
        content = @Content(schema = @Schema(implementation = AccessibleClientsResponse.class)))
    @APIResponse(responseCode = "401", description = "Not authenticated")
    public Response getAccessibleClients(
            @CookieParam("fc_session") String sessionToken,
            @HeaderParam("Authorization") String authHeader) {

        var principalIdOpt = jwtKeyService.extractAndValidatePrincipalId(sessionToken, authHeader);
        if (principalIdOpt.isEmpty()) {
            return Response.status(Response.Status.UNAUTHORIZED)
                .entity(new ErrorResponse("Not authenticated"))
                .build();
        }

        Principal principal = principalRepo.findByIdOptional(principalIdOpt.get())
            .orElse(null);

        if (principal == null || !principal.active) {
            return Response.status(Response.Status.UNAUTHORIZED)
                .entity(new ErrorResponse("User not found or inactive"))
                .build();
        }

        // Get accessible client IDs
        Set<String> clientIds = clientAccessService.getAccessibleClients(principal);

        // Load client details
        List<ClientInfo> clients = clientRepo.findByIds(clientIds).stream()
            .map(c -> new ClientInfo(
                TypedId.Ops.serialize(EntityType.CLIENT, c.id),
                c.name,
                c.identifier))
            .toList();

        // Determine if user has global access
        boolean globalAccess = clientAccessService.isAnchorDomainUser(principal);

        return Response.ok(new AccessibleClientsResponse(
            clients,
            TypedId.Ops.serialize(EntityType.CLIENT, principal.clientId),
            globalAccess
        )).build();
    }

    /**
     * Switch to a different client context.
     *
     * Issues a new token with the selected client in the claims.
     * The token will include:
     * - client_id: The selected client
     * - roles: User's roles
     * - permissions: Resolved permissions from roles
     */
    @POST
    @Path("/switch")
    @Operation(summary = "Switch to a different client context")
    @APIResponse(responseCode = "200", description = "New token issued with client context",
        content = @Content(schema = @Schema(implementation = SwitchClientResponse.class)))
    @APIResponse(responseCode = "401", description = "Not authenticated")
    @APIResponse(responseCode = "403", description = "Access denied to client")
    public Response switchClient(
            SwitchClientRequest request,
            @CookieParam("fc_session") String sessionToken,
            @HeaderParam("Authorization") String authHeader) {

        var principalIdOpt = jwtKeyService.extractAndValidatePrincipalId(sessionToken, authHeader);
        if (principalIdOpt.isEmpty()) {
            return Response.status(Response.Status.UNAUTHORIZED)
                .entity(new ErrorResponse("Not authenticated"))
                .build();
        }
        String principalId = principalIdOpt.get();

        Principal principal = principalRepo.findByIdOptional(principalId)
            .orElse(null);

        if (principal == null || !principal.active) {
            return Response.status(Response.Status.UNAUTHORIZED)
                .entity(new ErrorResponse("User not found or inactive"))
                .build();
        }

        // Deserialize typed client ID
        String internalClientId = TypedId.Ops.deserialize(EntityType.CLIENT, request.clientId());

        // Verify access to requested client
        Set<String> accessibleClients = clientAccessService.getAccessibleClients(principal);
        if (!accessibleClients.contains(internalClientId)) {
            LOG.warnf("Principal %s attempted to switch to unauthorized client %s",
                principalId, internalClientId);
            return Response.status(Response.Status.FORBIDDEN)
                .entity(new ErrorResponse("Access denied to client"))
                .build();
        }

        // Get client info
        Client client = clientRepo.findByIdOptional(internalClientId)
            .orElse(null);

        if (client == null) {
            return Response.status(Response.Status.NOT_FOUND)
                .entity(new ErrorResponse("Client not found"))
                .build();
        }

        // Load roles from embedded Principal.roles
        Set<String> roles = principal.getRoleNames();

        Set<String> permissions = permissionRegistry.getPermissionsForRoles(roles);

        // Issue new token with client context (uses internal ID)
        String newToken = jwtKeyService.issueSessionTokenWithClient(
            principalId,
            principal.userIdentity != null ? principal.userIdentity.email : null,
            roles,
            permissions,
            internalClientId
        );

        LOG.infof("Principal %s switched to client %s (%s)",
            principalId, client.id, client.identifier);

        // Build response with optional cookie
        Response.ResponseBuilder response = Response.ok(new SwitchClientResponse(
            newToken,
            new ClientInfo(
                TypedId.Ops.serialize(EntityType.CLIENT, client.id),
                client.name,
                client.identifier),
            roles,
            permissions
        ));

        // Also set session cookie if configured
        if (sessionToken != null) {
            NewCookie sessionCookie = new NewCookie.Builder(authConfig.session().cookieName())
                .value(newToken)
                .path("/")
                .maxAge((int) authConfig.jwt().sessionTokenExpiry().toSeconds())
                .httpOnly(true)
                .secure(authConfig.session().secure())
                .sameSite(NewCookie.SameSite.valueOf(authConfig.session().sameSite().toUpperCase()))
                .build();
            response.cookie(sessionCookie);
        }

        return response.build();
    }

    /**
     * Get current client context from the token.
     */
    @GET
    @Path("/current")
    @Operation(summary = "Get current client context")
    @APIResponse(responseCode = "200", description = "Current client info")
    @APIResponse(responseCode = "401", description = "Not authenticated")
    public Response getCurrentClient(
            @CookieParam("fc_session") String sessionToken,
            @HeaderParam("Authorization") String authHeader) {

        var principalIdOpt = jwtKeyService.extractAndValidatePrincipalId(sessionToken, authHeader);
        if (principalIdOpt.isEmpty()) {
            return Response.status(Response.Status.UNAUTHORIZED)
                .entity(new ErrorResponse("Not authenticated"))
                .build();
        }

        Principal principal = principalRepo.findByIdOptional(principalIdOpt.get())
            .orElse(null);

        if (principal == null) {
            return Response.status(Response.Status.UNAUTHORIZED)
                .entity(new ErrorResponse("User not found"))
                .build();
        }

        // Get current client from token or principal's home client
        String currentClientId = jwtKeyService.extractClientId(sessionToken != null ? sessionToken : extractBearerToken(authHeader));

        if (currentClientId == null) {
            currentClientId = principal.clientId;
        }

        ClientInfo clientInfo = null;
        if (currentClientId != null) {
            Client client = clientRepo.findByIdOptional(currentClientId).orElse(null);
            if (client != null) {
                clientInfo = new ClientInfo(
                    TypedId.Ops.serialize(EntityType.CLIENT, client.id),
                    client.name,
                    client.identifier);
            }
        }

        return Response.ok(new CurrentClientResponse(
            clientInfo,
            currentClientId == null
        )).build();
    }

    // ==================== Helper Methods ====================

    private String extractBearerToken(String authHeader) {
        if (authHeader != null && authHeader.startsWith("Bearer ")) {
            return authHeader.substring("Bearer ".length());
        }
        return null;
    }

    // ==================== DTOs ====================

    public record ClientInfo(
        String id,
        String name,
        String identifier
    ) {}

    public record AccessibleClientsResponse(
        List<ClientInfo> clients,
        String currentClientId,
        boolean globalAccess
    ) {}

    public record SwitchClientRequest(
        String clientId
    ) {}

    public record SwitchClientResponse(
        String token,
        ClientInfo client,
        Set<String> roles,
        Set<String> permissions
    ) {}

    public record CurrentClientResponse(
        ClientInfo client,
        boolean noClientContext
    ) {}

    public record ErrorResponse(
        String error
    ) {}
}
