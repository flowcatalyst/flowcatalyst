package tech.flowcatalyst.platform.client;

import jakarta.inject.Inject;
import jakarta.ws.rs.*;
import jakarta.ws.rs.core.MediaType;
import jakarta.ws.rs.core.Response;
import org.eclipse.microprofile.openapi.annotations.Operation;
import org.eclipse.microprofile.openapi.annotations.media.Content;
import org.eclipse.microprofile.openapi.annotations.media.Schema;
import org.eclipse.microprofile.openapi.annotations.responses.APIResponse;
import org.eclipse.microprofile.openapi.annotations.responses.APIResponses;
import org.eclipse.microprofile.openapi.annotations.tags.Tag;
import tech.flowcatalyst.platform.application.Application;
import tech.flowcatalyst.platform.application.ApplicationClientConfig;
import tech.flowcatalyst.platform.application.ApplicationService;
import tech.flowcatalyst.platform.audit.AuditContext;
import tech.flowcatalyst.platform.shared.EntityType;
import tech.flowcatalyst.platform.shared.TypedId;
import tech.flowcatalyst.platform.shared.TypedIdParam;

import java.util.List;
import java.util.Map;
import java.util.Set;
import java.util.stream.Collectors;

/**
 * Public API for clients.
 *
 * Returns clients based on the caller's database-stored permissions.
 * Authorization is determined by the principal's scope (ANCHOR, PARTNER, CLIENT).
 */
@Path("/api/clients")
@Produces(MediaType.APPLICATION_JSON)
@Consumes(MediaType.APPLICATION_JSON)
@Tag(name = "Clients", description = "Client access API")
public class ClientResource {

    @Inject
    ClientRepository clientRepository;

    @Inject
    ApplicationService applicationService;

    @Inject
    AuditContext auditContext;

    @GET
    @Operation(
        summary = "Get accessible clients",
        description = "Returns the list of clients the authenticated principal has access to, based on their scope."
    )
    @APIResponses({
        @APIResponse(
            responseCode = "200",
            description = "List of accessible clients",
            content = @Content(mediaType = MediaType.APPLICATION_JSON,
                schema = @Schema(implementation = ClientListResponse.class))
        ),
        @APIResponse(responseCode = "401", description = "Not authenticated")
    })
    public Response getClients() {
        // Require authentication (throws 401 if not authenticated)
        auditContext.requirePrincipalId();

        List<Client> clients;
        if (auditContext.hasAccessToAllClients()) {
            // ANCHOR scope: access to all clients
            clients = clientRepository.findAllActive();
        } else {
            // CLIENT/PARTNER scope: access to home client only (for now)
            // TODO: For PARTNER scope, also include clients from partner_client_access table
            clients = auditContext.getHomeClientId()
                .flatMap(clientRepository::findByIdOptional)
                .filter(c -> c.status == ClientStatus.ACTIVE)
                .map(List::of)
                .orElse(List.of());
        }

        List<ClientResponse> responses = clients.stream()
            .map(ClientResponse::from)
            .toList();

        return Response.ok(new ClientListResponse(responses)).build();
    }

    @GET
    @Path("/{id}")
    @Operation(
        summary = "Get client by ID",
        description = "Returns a specific client if the caller has access to it."
    )
    @APIResponses({
        @APIResponse(responseCode = "200", description = "Client found"),
        @APIResponse(responseCode = "400", description = "Invalid client ID format"),
        @APIResponse(responseCode = "401", description = "Not authenticated"),
        @APIResponse(responseCode = "403", description = "Access denied"),
        @APIResponse(responseCode = "404", description = "Client not found")
    })
    public Response getClient(
            @TypedIdParam(EntityType.CLIENT) @PathParam("id") String id) {
        // Require authentication (throws 401 if not authenticated)
        auditContext.requirePrincipalId();

        // Check access based on principal's scope
        if (!auditContext.hasAccessToClient(id)) {
            return Response.status(403)
                .entity(Map.of("error", "Access denied to this client"))
                .build();
        }

        // Find client
        return clientRepository.findByIdOptional(id)
            .filter(c -> c.status == ClientStatus.ACTIVE)
            .map(c -> Response.ok(ClientResponse.from(c)).build())
            .orElse(Response.status(404)
                .entity(Map.of("error", "Client not found"))
                .build());
    }

    @GET
    @Path("/{id}/applications")
    @Operation(
        summary = "Get applications for client",
        description = "Returns applications enabled for the specified client."
    )
    @APIResponses({
        @APIResponse(responseCode = "200", description = "List of applications",
            content = @Content(mediaType = MediaType.APPLICATION_JSON,
                schema = @Schema(implementation = ApplicationListResponse.class))),
        @APIResponse(responseCode = "400", description = "Invalid client ID format"),
        @APIResponse(responseCode = "401", description = "Not authenticated"),
        @APIResponse(responseCode = "403", description = "Access denied"),
        @APIResponse(responseCode = "404", description = "Client not found")
    })
    public Response getClientApplications(
            @TypedIdParam(EntityType.CLIENT) @PathParam("id") String id) {
        // Require authentication
        auditContext.requirePrincipalId();

        // Check access based on principal's scope
        if (!auditContext.hasAccessToClient(id)) {
            return Response.status(403)
                .entity(Map.of("error", "Access denied to this client"))
                .build();
        }

        // Verify client exists
        var clientOpt = clientRepository.findByIdOptional(id);
        if (clientOpt.isEmpty() || clientOpt.get().status != ClientStatus.ACTIVE) {
            return Response.status(404)
                .entity(Map.of("error", "Client not found"))
                .build();
        }

        // Get enabled applications for this client
        List<ApplicationClientConfig> configs = applicationService.getConfigsForClient(id);
        Set<String> enabledAppIds = configs.stream()
            .filter(c -> c.enabled)
            .map(c -> c.applicationId)
            .collect(Collectors.toSet());

        // Get all active applications and filter to enabled ones
        List<ApplicationResponse> applications = applicationService.findAll().stream()
            .filter(app -> app.active && enabledAppIds.contains(app.id))
            .map(app -> {
                // Check for website override in config
                String effectiveWebsite = configs.stream()
                    .filter(c -> c.applicationId.equals(app.id))
                    .findFirst()
                    .map(c -> c.websiteOverride != null && !c.websiteOverride.isBlank()
                        ? c.websiteOverride
                        : app.website)
                    .orElse(app.website);

                return new ApplicationResponse(
                    TypedId.Ops.serialize(EntityType.APPLICATION, app.id),
                    app.name,
                    app.code,
                    effectiveWebsite,
                    app.logo
                );
            })
            .toList();

        return Response.ok(new ApplicationListResponse(applications)).build();
    }

    // ========================================================================
    // DTOs
    // ========================================================================

    public record ClientListResponse(List<ClientResponse> items) {}

    public record ApplicationListResponse(List<ApplicationResponse> items) {}

    public record ApplicationResponse(
        String id,
        String name,
        String code,
        String website,
        String logo
    ) {}

    public record ClientResponse(
        String id,
        String name,
        String identifier,
        String status
    ) {
        /**
         * Creates a ClientResponse from a Client entity.
         * Serializes the ID with type prefix (e.g., "client_0HZXEQ5Y8JY5Z").
         */
        public static ClientResponse from(Client client) {
            return new ClientResponse(
                TypedId.Ops.serialize(EntityType.CLIENT, client.id),
                client.name,
                client.identifier,
                client.status != null ? client.status.name() : null
            );
        }
    }
}
