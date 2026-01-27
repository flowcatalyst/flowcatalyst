package tech.flowcatalyst.platform.sdk;

import jakarta.inject.Inject;
import jakarta.ws.rs.*;
import jakarta.ws.rs.core.*;
import org.eclipse.microprofile.openapi.annotations.Operation;
import org.eclipse.microprofile.openapi.annotations.media.Content;
import org.eclipse.microprofile.openapi.annotations.media.Schema;
import org.eclipse.microprofile.openapi.annotations.parameters.Parameter;
import org.eclipse.microprofile.openapi.annotations.responses.APIResponse;
import org.eclipse.microprofile.openapi.annotations.responses.APIResponses;
import org.eclipse.microprofile.openapi.annotations.tags.Tag;
import org.jboss.logging.Logger;
import tech.flowcatalyst.platform.authentication.JwtKeyService;
import tech.flowcatalyst.platform.authorization.AuthorizationService;
import tech.flowcatalyst.platform.authorization.platform.PlatformAdminPermissions;
import tech.flowcatalyst.platform.client.Client;
import tech.flowcatalyst.platform.client.ClientService;
import tech.flowcatalyst.platform.client.ClientStatus;
import tech.flowcatalyst.platform.common.errors.UseCaseError;
import tech.flowcatalyst.platform.shared.EntityType;
import tech.flowcatalyst.platform.shared.TypedId;

import java.time.Instant;
import java.util.List;
import java.util.Map;

/**
 * SDK API for managing clients.
 * Uses Bearer token authentication (no BFF session cookie).
 */
@Path("/api/sdk/clients")
@Tag(name = "SDK - Clients", description = "SDK API for client management")
@Produces(MediaType.APPLICATION_JSON)
@Consumes(MediaType.APPLICATION_JSON)
public class SdkClientResource {

    private static final Logger LOG = Logger.getLogger(SdkClientResource.class);

    @Inject
    ClientService clientService;

    @Inject
    JwtKeyService jwtKeyService;

    @Inject
    AuthorizationService authorizationService;

    // ==================== CRUD Operations ====================

    @GET
    @Operation(operationId = "sdkListClients", summary = "List clients")
    @APIResponses({
        @APIResponse(responseCode = "200", description = "List of clients",
            content = @Content(schema = @Schema(implementation = ClientListResponse.class))),
        @APIResponse(responseCode = "401", description = "Not authenticated"),
        @APIResponse(responseCode = "403", description = "Insufficient permissions")
    })
    public Response listClients(
            @QueryParam("status") @Parameter(description = "Filter by status") ClientStatus status,
            @HeaderParam("Authorization") String authHeader) {

        var principalId = requireAuth(authHeader);
        authorizationService.requirePermission(principalId, PlatformAdminPermissions.CLIENT_VIEW);

        List<Client> clients;
        if (status != null) {
            clients = clientService.findAll().stream()
                .filter(c -> c.status == status)
                .toList();
        } else {
            clients = clientService.findAll();
        }

        var dtos = clients.stream().map(this::toDto).toList();
        return Response.ok(new ClientListResponse(dtos, dtos.size())).build();
    }

    @GET
    @Path("/{id}")
    @Operation(operationId = "sdkGetClient", summary = "Get client by ID")
    @APIResponses({
        @APIResponse(responseCode = "200", description = "Client details",
            content = @Content(schema = @Schema(implementation = ClientDto.class))),
        @APIResponse(responseCode = "404", description = "Client not found"),
        @APIResponse(responseCode = "401", description = "Not authenticated"),
        @APIResponse(responseCode = "403", description = "Insufficient permissions")
    })
    public Response getClient(
            @PathParam("id") String id,
            @HeaderParam("Authorization") String authHeader) {

        var principalId = requireAuth(authHeader);
        authorizationService.requirePermission(principalId, PlatformAdminPermissions.CLIENT_VIEW);

        return clientService.findById(id)
            .map(client -> Response.ok(toDto(client)).build())
            .orElse(Response.status(Response.Status.NOT_FOUND)
                .entity(new ErrorResponse("CLIENT_NOT_FOUND", "Client not found"))
                .build());
    }

    @POST
    @Operation(operationId = "sdkCreateClient", summary = "Create a new client")
    @APIResponses({
        @APIResponse(responseCode = "201", description = "Client created",
            content = @Content(schema = @Schema(implementation = ClientDto.class))),
        @APIResponse(responseCode = "400", description = "Invalid request"),
        @APIResponse(responseCode = "401", description = "Not authenticated"),
        @APIResponse(responseCode = "403", description = "Insufficient permissions")
    })
    public Response createClient(
            CreateClientRequest request,
            @HeaderParam("Authorization") String authHeader) {

        var principalId = requireAuth(authHeader);
        authorizationService.requirePermission(principalId, PlatformAdminPermissions.CLIENT_CREATE);

        try {
            var client = clientService.createClient(request.name(), request.identifier());
            LOG.infof("SDK: Client created: %s (%s) by principal %s",
                client.name, client.identifier, principalId);
            return Response.status(Response.Status.CREATED).entity(toDto(client)).build();
        } catch (BadRequestException e) {
            return Response.status(Response.Status.BAD_REQUEST)
                .entity(new ErrorResponse("INVALID_REQUEST", e.getMessage()))
                .build();
        }
    }

    @PUT
    @Path("/{id}")
    @Operation(operationId = "sdkUpdateClient", summary = "Update client details")
    @APIResponses({
        @APIResponse(responseCode = "200", description = "Client updated",
            content = @Content(schema = @Schema(implementation = ClientDto.class))),
        @APIResponse(responseCode = "404", description = "Client not found"),
        @APIResponse(responseCode = "401", description = "Not authenticated"),
        @APIResponse(responseCode = "403", description = "Insufficient permissions")
    })
    public Response updateClient(
            @PathParam("id") String id,
            UpdateClientRequest request,
            @HeaderParam("Authorization") String authHeader) {

        var principalId = requireAuth(authHeader);
        authorizationService.requirePermission(principalId, PlatformAdminPermissions.CLIENT_UPDATE);

        try {
            var client = clientService.updateClient(id, request.name());
            return Response.ok(toDto(client)).build();
        } catch (NotFoundException e) {
            return Response.status(Response.Status.NOT_FOUND)
                .entity(new ErrorResponse("CLIENT_NOT_FOUND", "Client not found"))
                .build();
        }
    }

    // ==================== Status Management ====================

    @POST
    @Path("/{id}/activate")
    @Operation(operationId = "sdkActivateClient", summary = "Activate a client")
    @APIResponses({
        @APIResponse(responseCode = "200", description = "Client activated",
            content = @Content(schema = @Schema(implementation = StatusResponse.class))),
        @APIResponse(responseCode = "404", description = "Client not found"),
        @APIResponse(responseCode = "401", description = "Not authenticated"),
        @APIResponse(responseCode = "403", description = "Insufficient permissions")
    })
    public Response activateClient(
            @PathParam("id") String id,
            @HeaderParam("Authorization") String authHeader) {

        var principalId = requireAuth(authHeader);
        authorizationService.requirePermission(principalId, PlatformAdminPermissions.CLIENT_UPDATE);

        try {
            clientService.activateClient(id, principalId);
            return Response.ok(new StatusResponse("Client activated")).build();
        } catch (NotFoundException e) {
            return Response.status(Response.Status.NOT_FOUND)
                .entity(new ErrorResponse("CLIENT_NOT_FOUND", "Client not found"))
                .build();
        }
    }

    @POST
    @Path("/{id}/suspend")
    @Operation(operationId = "sdkSuspendClient", summary = "Suspend a client")
    @APIResponses({
        @APIResponse(responseCode = "200", description = "Client suspended",
            content = @Content(schema = @Schema(implementation = StatusResponse.class))),
        @APIResponse(responseCode = "404", description = "Client not found"),
        @APIResponse(responseCode = "401", description = "Not authenticated"),
        @APIResponse(responseCode = "403", description = "Insufficient permissions")
    })
    public Response suspendClient(
            @PathParam("id") String id,
            StatusChangeRequest request,
            @HeaderParam("Authorization") String authHeader) {

        var principalId = requireAuth(authHeader);
        authorizationService.requirePermission(principalId, PlatformAdminPermissions.CLIENT_UPDATE);

        try {
            clientService.suspendClient(id, request != null ? request.reason() : null, principalId);
            return Response.ok(new StatusResponse("Client suspended")).build();
        } catch (NotFoundException e) {
            return Response.status(Response.Status.NOT_FOUND)
                .entity(new ErrorResponse("CLIENT_NOT_FOUND", "Client not found"))
                .build();
        }
    }

    @POST
    @Path("/{id}/deactivate")
    @Operation(operationId = "sdkDeactivateClient", summary = "Deactivate a client")
    @APIResponses({
        @APIResponse(responseCode = "200", description = "Client deactivated",
            content = @Content(schema = @Schema(implementation = StatusResponse.class))),
        @APIResponse(responseCode = "404", description = "Client not found"),
        @APIResponse(responseCode = "401", description = "Not authenticated"),
        @APIResponse(responseCode = "403", description = "Insufficient permissions")
    })
    public Response deactivateClient(
            @PathParam("id") String id,
            StatusChangeRequest request,
            @HeaderParam("Authorization") String authHeader) {

        var principalId = requireAuth(authHeader);
        authorizationService.requirePermission(principalId, PlatformAdminPermissions.CLIENT_DELETE);

        try {
            clientService.deactivateClient(id, request != null ? request.reason() : null, principalId);
            return Response.ok(new StatusResponse("Client deactivated")).build();
        } catch (NotFoundException e) {
            return Response.status(Response.Status.NOT_FOUND)
                .entity(new ErrorResponse("CLIENT_NOT_FOUND", "Client not found"))
                .build();
        }
    }

    // ==================== Helpers ====================

    private String requireAuth(String authHeader) {
        var principalId = jwtKeyService.extractAndValidatePrincipalId(null, authHeader);
        if (principalId.isEmpty()) {
            throw new NotAuthorizedException("Bearer");
        }
        return principalId.get();
    }

    private ClientDto toDto(Client client) {
        return new ClientDto(
            TypedId.Ops.serialize(EntityType.CLIENT, client.id),
            client.name,
            client.identifier,
            client.status != null ? client.status.name() : null,
            client.statusReason,
            client.statusChangedAt,
            client.createdAt,
            client.updatedAt
        );
    }

    // ==================== DTOs ====================

    @Schema(description = "Client details")
    public record ClientDto(
        @Schema(description = "Client ID") String id,
        @Schema(description = "Client name") String name,
        @Schema(description = "Client identifier/slug") String identifier,
        @Schema(description = "Client status (ACTIVE, SUSPENDED, INACTIVE)") String status,
        @Schema(description = "Reason for current status") String statusReason,
        @Schema(description = "When status was last changed") Instant statusChangedAt,
        @Schema(description = "Creation timestamp") Instant createdAt,
        @Schema(description = "Last update timestamp") Instant updatedAt
    ) {}

    @Schema(description = "List of clients")
    public record ClientListResponse(
        @Schema(description = "Client list") List<ClientDto> clients,
        @Schema(description = "Total count") int total
    ) {}

    @Schema(description = "Create client request")
    public record CreateClientRequest(
        @Schema(required = true, description = "Client name") String name,
        @Schema(required = true, description = "Unique identifier/slug") String identifier
    ) {}

    @Schema(description = "Update client request")
    public record UpdateClientRequest(
        @Schema(required = true, description = "Client name") String name
    ) {}

    @Schema(description = "Status change request")
    public record StatusChangeRequest(
        @Schema(description = "Reason for status change") String reason
    ) {}

    @Schema(description = "Status change response")
    public record StatusResponse(
        @Schema(description = "Status message") String message
    ) {}

    @Schema(description = "Error response")
    public record ErrorResponse(
        @Schema(description = "Error code") String code,
        @Schema(description = "Error message") String message
    ) {}
}
