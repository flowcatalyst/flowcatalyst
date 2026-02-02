package tech.flowcatalyst.platform.user;

import jakarta.inject.Inject;
import jakarta.ws.rs.*;
import jakarta.ws.rs.core.*;
import org.eclipse.microprofile.openapi.annotations.Operation;
import org.eclipse.microprofile.openapi.annotations.media.Content;
import org.eclipse.microprofile.openapi.annotations.media.Schema;
import org.eclipse.microprofile.openapi.annotations.responses.APIResponse;
import org.eclipse.microprofile.openapi.annotations.responses.APIResponses;
import org.eclipse.microprofile.openapi.annotations.tags.Tag;
import org.jboss.logging.Logger;
import tech.flowcatalyst.platform.application.Application;
import tech.flowcatalyst.platform.application.ApplicationClientConfig;
import tech.flowcatalyst.platform.application.ApplicationService;
import tech.flowcatalyst.platform.authentication.JwtKeyService;
import tech.flowcatalyst.platform.client.Client;
import tech.flowcatalyst.platform.client.ClientAccessService;
import tech.flowcatalyst.platform.client.ClientRepository;
import tech.flowcatalyst.platform.common.api.ApiResponses;
import tech.flowcatalyst.platform.principal.Principal;
import tech.flowcatalyst.platform.principal.PrincipalRepository;
import tech.flowcatalyst.platform.shared.EntityType;
import tech.flowcatalyst.platform.shared.TypedId;

import java.time.Instant;
import java.util.List;
import java.util.Set;

/**
 * User-facing API for accessing resources the authenticated user has access to.
 *
 * These endpoints do NOT require admin permissions - they return only resources
 * the user can access based on their scope (ANCHOR/PARTNER/CLIENT).
 *
 * Supports both cookie-based sessions and Bearer token authentication.
 */
@Path("/api/me")
@Tag(name = "User", description = "User-scoped resource access")
@Produces(MediaType.APPLICATION_JSON)
@Consumes(MediaType.APPLICATION_JSON)
public class UserResource {

    private static final Logger LOG = Logger.getLogger(UserResource.class);
    private static final String SESSION_COOKIE_NAME = "fc_session";

    @Inject
    JwtKeyService jwtKeyService;

    @Inject
    PrincipalRepository principalRepository;

    @Inject
    ClientRepository clientRepository;

    @Inject
    ClientAccessService clientAccessService;

    @Inject
    ApplicationService applicationService;

    // ==================== Clients ====================

    /**
     * Get clients the authenticated user has access to.
     *
     * Access is determined by user scope:
     * - ANCHOR: All active clients
     * - PARTNER: IDP granted clients + explicit grants
     * - CLIENT: Home client + IDP additional clients + explicit grants
     */
    @GET
    @Path("/clients")
    @Operation(operationId = "getMyClients", summary = "Get my accessible clients",
        description = "Returns clients the authenticated user can access based on their scope")
    @APIResponses({
        @APIResponse(responseCode = "200", description = "List of accessible clients",
            content = @Content(schema = @Schema(implementation = MyClientsResponse.class))),
        @APIResponse(responseCode = "401", description = "Not authenticated",
            content = @Content(schema = @Schema(implementation = ApiResponses.UnauthorizedResponse.class)))
    })
    public Response getMyClients(
            @CookieParam(SESSION_COOKIE_NAME) String sessionToken,
            @HeaderParam("Authorization") String authHeader) {

        Principal principal = requireAuthenticatedPrincipal(sessionToken, authHeader);

        Set<String> accessibleClientIds = clientAccessService.getAccessibleClients(principal);

        List<Client> clients = accessibleClientIds.isEmpty()
            ? List.of()
            : clientRepository.findByIds(accessibleClientIds);

        List<MyClientDto> dtos = clients.stream()
            .map(this::toClientDto)
            .sorted((a, b) -> a.name().compareToIgnoreCase(b.name()))
            .toList();

        return Response.ok(new MyClientsResponse(dtos, dtos.size())).build();
    }

    /**
     * Get a specific client the user has access to.
     */
    @GET
    @Path("/clients/{clientId}")
    @Operation(operationId = "getMyClient", summary = "Get a specific accessible client",
        description = "Returns client details if the user has access to it")
    @APIResponses({
        @APIResponse(responseCode = "200", description = "Client details",
            content = @Content(schema = @Schema(implementation = MyClientDto.class))),
        @APIResponse(responseCode = "401", description = "Not authenticated",
            content = @Content(schema = @Schema(implementation = ApiResponses.UnauthorizedResponse.class))),
        @APIResponse(responseCode = "403", description = "No access to this client",
            content = @Content(schema = @Schema(implementation = ApiResponses.ForbiddenResponse.class))),
        @APIResponse(responseCode = "404", description = "Client not found",
            content = @Content(schema = @Schema(implementation = ApiResponses.NotFoundResponse.class)))
    })
    public Response getMyClient(
            @PathParam("clientId") String clientId,
            @CookieParam(SESSION_COOKIE_NAME) String sessionToken,
            @HeaderParam("Authorization") String authHeader) {

        Principal principal = requireAuthenticatedPrincipal(sessionToken, authHeader);

        // Validate the client ID format (IDs are stored with prefixes)
        TypedId.Ops.validate(EntityType.CLIENT, clientId);

        // Resource-level authorization: check if user can access this client
        if (!clientAccessService.canAccessClient(principal, clientId)) {
            return Response.status(Response.Status.FORBIDDEN)
                .entity(new ApiResponses.ForbiddenResponse("You do not have access to this client"))
                .build();
        }

        return clientRepository.findByIdOptional(clientId)
            .map(client -> Response.ok(toClientDto(client)).build())
            .orElse(Response.status(Response.Status.NOT_FOUND)
                .entity(new ApiResponses.NotFoundResponse("client", clientId))
                .build());
    }

    // ==================== Client Applications ====================

    /**
     * Get applications enabled for a client that the user has access to.
     *
     * Resource-level authorization:
     * - User must have access to the client
     * - Only returns applications enabled for that client
     */
    @GET
    @Path("/clients/{clientId}/applications")
    @Operation(operationId = "getMyClientApplications", summary = "Get applications for an accessible client",
        description = "Returns applications enabled for a client the user has access to")
    @APIResponses({
        @APIResponse(responseCode = "200", description = "List of applications",
            content = @Content(schema = @Schema(implementation = MyApplicationsResponse.class))),
        @APIResponse(responseCode = "401", description = "Not authenticated",
            content = @Content(schema = @Schema(implementation = ApiResponses.UnauthorizedResponse.class))),
        @APIResponse(responseCode = "403", description = "No access to this client",
            content = @Content(schema = @Schema(implementation = ApiResponses.ForbiddenResponse.class))),
        @APIResponse(responseCode = "404", description = "Client not found",
            content = @Content(schema = @Schema(implementation = ApiResponses.NotFoundResponse.class)))
    })
    public Response getMyClientApplications(
            @PathParam("clientId") String clientId,
            @CookieParam(SESSION_COOKIE_NAME) String sessionToken,
            @HeaderParam("Authorization") String authHeader) {

        Principal principal = requireAuthenticatedPrincipal(sessionToken, authHeader);

        // Validate the client ID format (IDs are stored with prefixes)
        TypedId.Ops.validate(EntityType.CLIENT, clientId);

        // Resource-level authorization: check if user can access this client
        if (!clientAccessService.canAccessClient(principal, clientId)) {
            return Response.status(Response.Status.FORBIDDEN)
                .entity(new ApiResponses.ForbiddenResponse("You do not have access to this client"))
                .build();
        }

        // Verify client exists
        var clientOpt = clientRepository.findByIdOptional(clientId);
        if (clientOpt.isEmpty()) {
            return Response.status(Response.Status.NOT_FOUND)
                .entity(new ApiResponses.NotFoundResponse("client", clientId))
                .build();
        }

        Client client = clientOpt.get();

        // Get enabled applications for this client
        List<ApplicationClientConfig> configs = applicationService.getConfigsForClient(clientId);
        Set<String> enabledAppIds = configs.stream()
            .filter(c -> c.enabled)
            .map(c -> c.applicationId)
            .collect(java.util.stream.Collectors.toSet());

        // Get application details for enabled apps
        List<Application> enabledApps = applicationService.findAll().stream()
            .filter(app -> enabledAppIds.contains(app.id))
            .filter(app -> app.active)
            .toList();

        // Build response with effective URLs
        List<MyApplicationDto> dtos = enabledApps.stream()
            .map(app -> {
                ApplicationClientConfig config = configs.stream()
                    .filter(c -> c.applicationId.equals(app.id))
                    .findFirst()
                    .orElse(null);

                String effectiveBaseUrl = (config != null && config.baseUrlOverride != null && !config.baseUrlOverride.isBlank())
                    ? config.baseUrlOverride
                    : app.defaultBaseUrl;

                String effectiveWebsite = (config != null && config.websiteOverride != null && !config.websiteOverride.isBlank())
                    ? config.websiteOverride
                    : app.website;

                return new MyApplicationDto(
                    TypedId.Ops.serialize(EntityType.APPLICATION, app.id),
                    app.code,
                    app.name,
                    app.description,
                    app.iconUrl,
                    effectiveBaseUrl,
                    effectiveWebsite,
                    app.logoMimeType
                );
            })
            .sorted((a, b) -> a.name().compareToIgnoreCase(b.name()))
            .toList();

        return Response.ok(new MyApplicationsResponse(dtos, dtos.size(), clientId)).build();
    }

    // ==================== Authentication ====================

    private Principal requireAuthenticatedPrincipal(String sessionToken, String authHeader) {
        String principalId = null;

        // Try Bearer token first
        if (authHeader != null && authHeader.startsWith("Bearer ")) {
            var extracted = jwtKeyService.extractAndValidatePrincipalId(null, authHeader);
            if (extracted.isPresent()) {
                principalId = extracted.get();
            }
        }

        // Fall back to session cookie
        if (principalId == null && sessionToken != null && !sessionToken.isEmpty()) {
            principalId = jwtKeyService.validateAndGetPrincipalId(sessionToken);
        }

        if (principalId == null) {
            throw new NotAuthorizedException("Bearer");
        }

        return principalRepository.findByIdOptional(principalId)
            .orElseThrow(() -> new NotAuthorizedException("Bearer"));
    }

    // ==================== Mappers ====================

    private MyClientDto toClientDto(Client client) {
        return new MyClientDto(
            TypedId.Ops.serialize(EntityType.CLIENT, client.id),
            client.name,
            client.identifier,
            client.status != null ? client.status.name() : null,
            client.createdAt,
            client.updatedAt
        );
    }

    // ==================== DTOs ====================

    @Schema(description = "Client the user has access to")
    public record MyClientDto(
        @Schema(description = "Client ID", example = "cli_0ABC123DEF456")
        String id,
        @Schema(description = "Client name", example = "Acme Corporation")
        String name,
        @Schema(description = "Client identifier/slug", example = "acme-corp")
        String identifier,
        @Schema(description = "Client status (ACTIVE, SUSPENDED, INACTIVE)")
        String status,
        @Schema(description = "Creation timestamp")
        Instant createdAt,
        @Schema(description = "Last update timestamp")
        Instant updatedAt
    ) {}

    @Schema(description = "List of accessible clients")
    public record MyClientsResponse(
        @Schema(description = "Clients the user can access")
        List<MyClientDto> clients,
        @Schema(description = "Total count")
        int total
    ) {}

    @Schema(description = "Application enabled for a client")
    public record MyApplicationDto(
        @Schema(description = "Application ID", example = "app_0ABC123DEF456")
        String id,
        @Schema(description = "Application code", example = "my-app")
        String code,
        @Schema(description = "Application name", example = "My Application")
        String name,
        @Schema(description = "Application description")
        String description,
        @Schema(description = "Icon URL")
        String iconUrl,
        @Schema(description = "Effective base URL for this client")
        String baseUrl,
        @Schema(description = "Effective website URL for this client")
        String website,
        @Schema(description = "Logo MIME type")
        String logoMimeType
    ) {}

    @Schema(description = "List of applications for a client")
    public record MyApplicationsResponse(
        @Schema(description = "Applications enabled for this client")
        List<MyApplicationDto> applications,
        @Schema(description = "Total count")
        int total,
        @Schema(description = "Client ID these applications belong to")
        String clientId
    ) {}
}
