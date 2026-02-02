package tech.flowcatalyst.platform.admin;

import jakarta.inject.Inject;
import jakarta.validation.Valid;
import jakarta.validation.constraints.NotBlank;
import jakarta.validation.constraints.Size;
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
import tech.flowcatalyst.platform.application.Application;
import tech.flowcatalyst.platform.application.ApplicationClientConfig;
import tech.flowcatalyst.platform.application.ApplicationService;
import tech.flowcatalyst.platform.application.events.ApplicationDisabledForClient;
import tech.flowcatalyst.platform.application.events.ApplicationEnabledForClient;
import tech.flowcatalyst.platform.application.operations.DisableApplicationForClientCommand;
import tech.flowcatalyst.platform.application.operations.EnableApplicationForClientCommand;
import tech.flowcatalyst.platform.audit.AuditContext;
import tech.flowcatalyst.platform.authorization.AuthorizationService;
import tech.flowcatalyst.platform.authorization.platform.PlatformAdminPermissions;
import tech.flowcatalyst.platform.common.Result;
import tech.flowcatalyst.platform.common.api.ApiResponses;
import tech.flowcatalyst.platform.common.api.ClientResponses;
import tech.flowcatalyst.platform.common.api.ClientResponses.*;
import tech.flowcatalyst.platform.authentication.EmbeddedModeOnly;
import tech.flowcatalyst.platform.client.Client;
import tech.flowcatalyst.platform.client.ClientService;
import tech.flowcatalyst.platform.client.ClientStatus;
import tech.flowcatalyst.platform.common.ExecutionContext;
import tech.flowcatalyst.platform.shared.EntityType;
import tech.flowcatalyst.platform.shared.TypedId;
import tech.flowcatalyst.platform.shared.TypedIdParam;

import java.util.List;
import java.util.Set;
import java.util.stream.Collectors;

/**
 * Admin API for client management.
 *
 * Provides CRUD operations for clients including:
 * - Create, read, update clients
 * - Status management (activate, suspend, deactivate)
 * - Audit notes
 *
 * All operations require admin-level permissions.
 */
@Path("/api/admin/clients")
@Tag(name = "BFF - Client Admin", description = "Administrative operations for client management")
@Produces(MediaType.APPLICATION_JSON)
@Consumes(MediaType.APPLICATION_JSON)
@EmbeddedModeOnly
@jakarta.transaction.Transactional
public class ClientAdminResource {

    private static final Logger LOG = Logger.getLogger(ClientAdminResource.class);

    @Inject
    ClientService clientService;

    @Inject
    ApplicationService applicationService;

    @Inject
    AuditContext auditContext;

    @Inject
    AuthorizationService authorizationService;

    // ==================== CRUD Operations ====================

    /**
     * List all clients.
     */
    @GET
    @Operation(operationId = "listClients", summary = "List all clients", description = "Returns all clients regardless of status")
    @APIResponses({
        @APIResponse(responseCode = "200", description = "List of clients",
            content = @Content(schema = @Schema(implementation = ClientListResponse.class))),
        @APIResponse(responseCode = "401", description = "Not authenticated",
            content = @Content(schema = @Schema(implementation = ApiResponses.UnauthorizedResponse.class))),
        @APIResponse(responseCode = "403", description = "Insufficient permissions",
            content = @Content(schema = @Schema(implementation = ApiResponses.ForbiddenResponse.class)))
    })
    public Response listClients(
            @QueryParam("status") @Parameter(description = "Filter by status") ClientStatus status) {

        // Authentication: AuditContext is populated by AuditContextFilter
        String principalId = auditContext.requirePrincipalId();

        // Authorization: Check permission
        authorizationService.requirePermission(principalId, PlatformAdminPermissions.CLIENT_VIEW);

        List<Client> clients;
        if (status != null) {
            clients = clientService.findAll().stream()
                .filter(c -> c.status == status)
                .toList();
        } else {
            clients = clientService.findAll();
        }

        List<ClientDto> dtos = clients.stream()
            .map(ClientDto::from)
            .toList();

        return Response.ok(new ClientListResponse(dtos, dtos.size())).build();
    }

    /**
     * Search clients with text filter.
     * Returns clients matching the search query (name or identifier).
     */
    @GET
    @Path("/search")
    @Operation(operationId = "searchClients", summary = "Search clients", description = "Search clients by name or identifier")
    @APIResponses({
        @APIResponse(responseCode = "200", description = "Matching clients",
            content = @Content(schema = @Schema(implementation = ClientListResponse.class))),
        @APIResponse(responseCode = "401", description = "Not authenticated",
            content = @Content(schema = @Schema(implementation = ApiResponses.UnauthorizedResponse.class))),
        @APIResponse(responseCode = "403", description = "Insufficient permissions",
            content = @Content(schema = @Schema(implementation = ApiResponses.ForbiddenResponse.class)))
    })
    public Response searchClients(
            @QueryParam("q") @Parameter(description = "Search query (name or identifier)") String query,
            @QueryParam("status") @Parameter(description = "Filter by status") ClientStatus status,
            @QueryParam("limit") @Parameter(description = "Max results to return") @DefaultValue("20") int limit) {

        String principalId = auditContext.requirePrincipalId();
        authorizationService.requirePermission(principalId, PlatformAdminPermissions.CLIENT_VIEW);

        List<Client> clients = clientService.findAll();

        // Apply filters
        var stream = clients.stream();

        if (status != null) {
            stream = stream.filter(c -> c.status == status);
        }

        if (query != null && !query.isBlank()) {
            String lowerQuery = query.toLowerCase();
            stream = stream.filter(c ->
                (c.name != null && c.name.toLowerCase().contains(lowerQuery)) ||
                (c.identifier != null && c.identifier.toLowerCase().contains(lowerQuery))
            );
        }

        List<ClientDto> dtos = stream
            .limit(limit)
            .map(ClientDto::from)
            .toList();

        return Response.ok(new ClientListResponse(dtos, dtos.size())).build();
    }

    /**
     * Get a specific client by ID.
     */
    @GET
    @Path("/{id}")
    @Operation(operationId = "getClient", summary = "Get client by ID")
    @APIResponses({
        @APIResponse(responseCode = "200", description = "Client details",
            content = @Content(schema = @Schema(implementation = ClientDto.class))),
        @APIResponse(responseCode = "400", description = "Invalid client ID format",
            content = @Content(schema = @Schema(implementation = ApiResponses.ErrorResponse.class))),
        @APIResponse(responseCode = "404", description = "Client not found",
            content = @Content(schema = @Schema(implementation = ApiResponses.NotFoundResponse.class))),
        @APIResponse(responseCode = "401", description = "Not authenticated",
            content = @Content(schema = @Schema(implementation = ApiResponses.UnauthorizedResponse.class))),
        @APIResponse(responseCode = "403", description = "Insufficient permissions",
            content = @Content(schema = @Schema(implementation = ApiResponses.ForbiddenResponse.class)))
    })
    public Response getClient(
            @TypedIdParam(EntityType.CLIENT) @PathParam("id") String id) {

        String principalId = auditContext.requirePrincipalId();
        authorizationService.requirePermission(principalId, PlatformAdminPermissions.CLIENT_VIEW);

        return clientService.findById(id)
            .map(client -> Response.ok(ClientDto.from(client)).build())
            .orElse(Response.status(Response.Status.NOT_FOUND)
                .entity(new ApiResponses.NotFoundResponse("client", id))
                .build());
    }

    /**
     * Get a client by identifier/slug.
     */
    @GET
    @Path("/by-identifier/{identifier}")
    @Operation(operationId = "getClientByIdentifier", summary = "Get client by identifier")
    @APIResponses({
        @APIResponse(responseCode = "200", description = "Client details",
            content = @Content(schema = @Schema(implementation = ClientDto.class))),
        @APIResponse(responseCode = "404", description = "Client not found",
            content = @Content(schema = @Schema(implementation = ApiResponses.NotFoundResponse.class))),
        @APIResponse(responseCode = "401", description = "Not authenticated",
            content = @Content(schema = @Schema(implementation = ApiResponses.UnauthorizedResponse.class))),
        @APIResponse(responseCode = "403", description = "Insufficient permissions",
            content = @Content(schema = @Schema(implementation = ApiResponses.ForbiddenResponse.class)))
    })
    public Response getClientByIdentifier(@PathParam("identifier") String identifier) {

        String principalId = auditContext.requirePrincipalId();
        authorizationService.requirePermission(principalId, PlatformAdminPermissions.CLIENT_VIEW);

        return clientService.findByIdentifier(identifier)
            .map(client -> Response.ok(ClientDto.from(client)).build())
            .orElse(Response.status(Response.Status.NOT_FOUND)
                .entity(new ApiResponses.NotFoundResponse("client", identifier))
                .build());
    }

    /**
     * Create a new client.
     */
    @POST
    @Operation(operationId = "createClient", summary = "Create a new client")
    @APIResponses({
        @APIResponse(responseCode = "201", description = "Client created",
            content = @Content(schema = @Schema(implementation = ClientDto.class))),
        @APIResponse(responseCode = "400", description = "Invalid request or identifier already exists",
            content = @Content(schema = @Schema(implementation = ApiResponses.ErrorResponse.class))),
        @APIResponse(responseCode = "401", description = "Not authenticated",
            content = @Content(schema = @Schema(implementation = ApiResponses.UnauthorizedResponse.class))),
        @APIResponse(responseCode = "403", description = "Insufficient permissions",
            content = @Content(schema = @Schema(implementation = ApiResponses.ForbiddenResponse.class)))
    })
    public Response createClient(@Valid CreateClientRequest request, @Context UriInfo uriInfo) {

        String principalId = auditContext.requirePrincipalId();
        authorizationService.requirePermission(principalId, PlatformAdminPermissions.CLIENT_CREATE);

        try {
            Client client = clientService.createClient(request.name(), request.identifier());
            LOG.infof("Client created: %s (%s) by principal %s",
                client.name, client.identifier, principalId);

            return Response.status(Response.Status.CREATED)
                .entity(ClientDto.from(client))
                .location(uriInfo.getAbsolutePathBuilder().path(String.valueOf(client.id)).build())
                .build();
        } catch (BadRequestException e) {
            return Response.status(Response.Status.BAD_REQUEST)
                .entity(new ApiResponses.ErrorResponse("VALIDATION_ERROR", e.getMessage()))
                .build();
        }
    }

    /**
     * Update client details.
     */
    @PUT
    @Path("/{id}")
    @Operation(operationId = "updateClient", summary = "Update client details")
    @APIResponses({
        @APIResponse(responseCode = "200", description = "Client updated",
            content = @Content(schema = @Schema(implementation = ClientDto.class))),
        @APIResponse(responseCode = "400", description = "Invalid request or client ID format",
            content = @Content(schema = @Schema(implementation = ApiResponses.ErrorResponse.class))),
        @APIResponse(responseCode = "404", description = "Client not found",
            content = @Content(schema = @Schema(implementation = ApiResponses.NotFoundResponse.class))),
        @APIResponse(responseCode = "401", description = "Not authenticated",
            content = @Content(schema = @Schema(implementation = ApiResponses.UnauthorizedResponse.class))),
        @APIResponse(responseCode = "403", description = "Insufficient permissions",
            content = @Content(schema = @Schema(implementation = ApiResponses.ForbiddenResponse.class)))
    })
    public Response updateClient(
            @TypedIdParam(EntityType.CLIENT) @PathParam("id") String id,
            @Valid UpdateClientRequest request) {

        String principalId = auditContext.requirePrincipalId();
        authorizationService.requirePermission(principalId, PlatformAdminPermissions.CLIENT_UPDATE);

        try {
            Client client = clientService.updateClient(id, request.name());
            LOG.infof("Client updated: %s by principal %s", id, principalId);
            return Response.ok(ClientDto.from(client)).build();
        } catch (NotFoundException e) {
            return Response.status(Response.Status.NOT_FOUND)
                .entity(new ApiResponses.NotFoundResponse("client", id))
                .build();
        }
    }

    // ==================== Status Management ====================

    /**
     * Activate a client.
     */
    @POST
    @Path("/{id}/activate")
    @Operation(operationId = "activateClient", summary = "Activate a client")
    @APIResponses({
        @APIResponse(responseCode = "200", description = "Client activated",
            content = @Content(schema = @Schema(implementation = ClientStatusResponse.class))),
        @APIResponse(responseCode = "400", description = "Invalid client ID format",
            content = @Content(schema = @Schema(implementation = ApiResponses.ErrorResponse.class))),
        @APIResponse(responseCode = "404", description = "Client not found",
            content = @Content(schema = @Schema(implementation = ApiResponses.NotFoundResponse.class))),
        @APIResponse(responseCode = "401", description = "Not authenticated",
            content = @Content(schema = @Schema(implementation = ApiResponses.UnauthorizedResponse.class))),
        @APIResponse(responseCode = "403", description = "Insufficient permissions",
            content = @Content(schema = @Schema(implementation = ApiResponses.ForbiddenResponse.class)))
    })
    public Response activateClient(
            @TypedIdParam(EntityType.CLIENT) @PathParam("id") String id) {

        String principalId = auditContext.requirePrincipalId();
        authorizationService.requirePermission(principalId, PlatformAdminPermissions.CLIENT_UPDATE);

        try {
            clientService.activateClient(id, principalId);
            LOG.infof("Client %s activated by principal %s", id, principalId);
            String serializedId = TypedId.Ops.serialize(EntityType.CLIENT, id);
            return Response.ok(ClientStatusResponse.activated(serializedId)).build();
        } catch (NotFoundException e) {
            return Response.status(Response.Status.NOT_FOUND)
                .entity(new ApiResponses.NotFoundResponse("client", id))
                .build();
        }
    }

    /**
     * Suspend a client.
     */
    @POST
    @Path("/{id}/suspend")
    @Operation(operationId = "suspendClient", summary = "Suspend a client")
    @APIResponses({
        @APIResponse(responseCode = "200", description = "Client suspended",
            content = @Content(schema = @Schema(implementation = ClientStatusResponse.class))),
        @APIResponse(responseCode = "400", description = "Invalid client ID format",
            content = @Content(schema = @Schema(implementation = ApiResponses.ErrorResponse.class))),
        @APIResponse(responseCode = "404", description = "Client not found",
            content = @Content(schema = @Schema(implementation = ApiResponses.NotFoundResponse.class))),
        @APIResponse(responseCode = "401", description = "Not authenticated",
            content = @Content(schema = @Schema(implementation = ApiResponses.UnauthorizedResponse.class))),
        @APIResponse(responseCode = "403", description = "Insufficient permissions",
            content = @Content(schema = @Schema(implementation = ApiResponses.ForbiddenResponse.class)))
    })
    public Response suspendClient(
            @TypedIdParam(EntityType.CLIENT) @PathParam("id") String id,
            @Valid StatusChangeRequest request) {

        String principalId = auditContext.requirePrincipalId();
        authorizationService.requirePermission(principalId, PlatformAdminPermissions.CLIENT_UPDATE);

        try {
            clientService.suspendClient(id, request.reason(), principalId);
            LOG.infof("Client %s suspended by principal %s: %s", id, principalId, request.reason());
            String serializedId = TypedId.Ops.serialize(EntityType.CLIENT, id);
            return Response.ok(ClientStatusResponse.suspended(serializedId)).build();
        } catch (NotFoundException e) {
            return Response.status(Response.Status.NOT_FOUND)
                .entity(new ApiResponses.NotFoundResponse("client", id))
                .build();
        }
    }

    /**
     * Deactivate a client (soft delete).
     */
    @POST
    @Path("/{id}/deactivate")
    @Operation(operationId = "deactivateClient", summary = "Deactivate a client")
    @APIResponses({
        @APIResponse(responseCode = "200", description = "Client deactivated",
            content = @Content(schema = @Schema(implementation = ClientStatusResponse.class))),
        @APIResponse(responseCode = "400", description = "Invalid client ID format",
            content = @Content(schema = @Schema(implementation = ApiResponses.ErrorResponse.class))),
        @APIResponse(responseCode = "404", description = "Client not found",
            content = @Content(schema = @Schema(implementation = ApiResponses.NotFoundResponse.class))),
        @APIResponse(responseCode = "401", description = "Not authenticated",
            content = @Content(schema = @Schema(implementation = ApiResponses.UnauthorizedResponse.class))),
        @APIResponse(responseCode = "403", description = "Insufficient permissions",
            content = @Content(schema = @Schema(implementation = ApiResponses.ForbiddenResponse.class)))
    })
    public Response deactivateClient(
            @TypedIdParam(EntityType.CLIENT) @PathParam("id") String id,
            @Valid StatusChangeRequest request) {

        String principalId = auditContext.requirePrincipalId();
        authorizationService.requirePermission(principalId, PlatformAdminPermissions.CLIENT_DELETE);

        try {
            clientService.deactivateClient(id, request.reason(), principalId);
            LOG.infof("Client %s deactivated by principal %s: %s", id, principalId, request.reason());
            String serializedId = TypedId.Ops.serialize(EntityType.CLIENT, id);
            return Response.ok(ClientStatusResponse.deactivated(serializedId)).build();
        } catch (NotFoundException e) {
            return Response.status(Response.Status.NOT_FOUND)
                .entity(new ApiResponses.NotFoundResponse("client", id))
                .build();
        }
    }

    // ==================== Audit Notes ====================

    /**
     * Add a note to a client's audit trail.
     */
    @POST
    @Path("/{id}/notes")
    @Operation(operationId = "addClientAuditNote", summary = "Add audit note to client")
    @APIResponses({
        @APIResponse(responseCode = "201", description = "Note added",
            content = @Content(schema = @Schema(implementation = NoteAddedResponse.class))),
        @APIResponse(responseCode = "400", description = "Invalid client ID format",
            content = @Content(schema = @Schema(implementation = ApiResponses.ErrorResponse.class))),
        @APIResponse(responseCode = "404", description = "Client not found",
            content = @Content(schema = @Schema(implementation = ApiResponses.NotFoundResponse.class))),
        @APIResponse(responseCode = "401", description = "Not authenticated",
            content = @Content(schema = @Schema(implementation = ApiResponses.UnauthorizedResponse.class))),
        @APIResponse(responseCode = "403", description = "Insufficient permissions",
            content = @Content(schema = @Schema(implementation = ApiResponses.ForbiddenResponse.class)))
    })
    public Response addNote(
            @TypedIdParam(EntityType.CLIENT) @PathParam("id") String id,
            @Valid AddNoteRequest request) {

        String principalId = auditContext.requirePrincipalId();
        authorizationService.requirePermission(principalId, PlatformAdminPermissions.CLIENT_UPDATE);

        try {
            clientService.addNote(id, request.category(), request.text(), principalId);
            String serializedId = TypedId.Ops.serialize(EntityType.CLIENT, id);
            return Response.status(Response.Status.CREATED)
                .entity(NoteAddedResponse.created(serializedId, request.category()))
                .build();
        } catch (NotFoundException e) {
            return Response.status(Response.Status.NOT_FOUND)
                .entity(new ApiResponses.NotFoundResponse("client", id))
                .build();
        }
    }

    // ==================== Application Management ====================

    /**
     * Get applications for a client.
     * Returns all applications with their enabled status for this client.
     */
    @GET
    @Path("/{id}/applications")
    @Operation(operationId = "getClientApplications", summary = "Get applications for client", description = "Returns all applications with their enabled/disabled status for this client")
    @APIResponses({
        @APIResponse(responseCode = "200", description = "List of applications with status",
            content = @Content(schema = @Schema(implementation = ClientApplicationsResponse.class))),
        @APIResponse(responseCode = "400", description = "Invalid client ID format",
            content = @Content(schema = @Schema(implementation = ApiResponses.ErrorResponse.class))),
        @APIResponse(responseCode = "404", description = "Client not found",
            content = @Content(schema = @Schema(implementation = ApiResponses.NotFoundResponse.class))),
        @APIResponse(responseCode = "401", description = "Not authenticated",
            content = @Content(schema = @Schema(implementation = ApiResponses.UnauthorizedResponse.class))),
        @APIResponse(responseCode = "403", description = "Insufficient permissions",
            content = @Content(schema = @Schema(implementation = ApiResponses.ForbiddenResponse.class)))
    })
    public Response getClientApplications(
            @TypedIdParam(EntityType.CLIENT) @PathParam("id") String clientId) {

        String principalId = auditContext.requirePrincipalId();
        // Viewing client applications requires both client view and application view permissions
        authorizationService.requirePermission(principalId, PlatformAdminPermissions.CLIENT_VIEW);
        authorizationService.requirePermission(principalId, PlatformAdminPermissions.APPLICATION_VIEW);

        // Verify client exists
        var clientOpt = clientService.findById(clientId);
        if (clientOpt.isEmpty()) {
            return Response.status(Response.Status.NOT_FOUND)
                .entity(new ApiResponses.NotFoundResponse("client", clientId))
                .build();
        }

        // Get all applications
        List<Application> allApps = applicationService.findAll();

        // Get configs for this client
        List<ApplicationClientConfig> configs = applicationService.getConfigsForClient(clientId);
        Set<String> enabledAppIds = configs.stream()
            .filter(c -> c.enabled)
            .map(c -> c.applicationId)
            .collect(Collectors.toSet());

        // Build response with enabled status and effective website
        List<ClientApplicationDto> appDtos = allApps.stream()
            .map(app -> {
                // Find config for this app to get website override
                ApplicationClientConfig config = configs.stream()
                    .filter(c -> c.applicationId.equals(app.id))
                    .findFirst()
                    .orElse(null);

                String effectiveWebsite = (config != null && config.websiteOverride != null && !config.websiteOverride.isBlank())
                    ? config.websiteOverride
                    : app.website;

                return new ClientApplicationDto(
                    TypedId.Ops.serialize(EntityType.APPLICATION, app.id),
                    app.code,
                    app.name,
                    app.description,
                    app.iconUrl,
                    app.website,
                    effectiveWebsite,
                    app.logoMimeType,
                    app.active,
                    enabledAppIds.contains(app.id)
                );
            })
            .toList();

        return Response.ok(new ClientApplicationsResponse(appDtos, appDtos.size())).build();
    }

    /**
     * Enable an application for a client.
     */
    @POST
    @Path("/{id}/applications/{applicationId}/enable")
    @Operation(operationId = "enableClientApplication", summary = "Enable application for client")
    @APIResponses({
        @APIResponse(responseCode = "200", description = "Application enabled",
            content = @Content(schema = @Schema(implementation = ClientApplicationStatusResponse.class))),
        @APIResponse(responseCode = "400", description = "Invalid ID format",
            content = @Content(schema = @Schema(implementation = ApiResponses.ErrorResponse.class))),
        @APIResponse(responseCode = "404", description = "Client or application not found",
            content = @Content(schema = @Schema(implementation = ApiResponses.NotFoundResponse.class))),
        @APIResponse(responseCode = "401", description = "Not authenticated",
            content = @Content(schema = @Schema(implementation = ApiResponses.UnauthorizedResponse.class))),
        @APIResponse(responseCode = "403", description = "Insufficient permissions",
            content = @Content(schema = @Schema(implementation = ApiResponses.ForbiddenResponse.class)))
    })
    public Response enableApplicationForClient(
            @TypedIdParam(EntityType.CLIENT) @PathParam("id") String clientId,
            @TypedIdParam(EntityType.APPLICATION) @PathParam("applicationId") String applicationId) {

        String principalId = auditContext.requirePrincipalId();
        // Enabling application for client requires both client update and application update
        authorizationService.requirePermission(principalId, PlatformAdminPermissions.CLIENT_UPDATE);
        authorizationService.requirePermission(principalId, PlatformAdminPermissions.APPLICATION_UPDATE);

        try {
            ExecutionContext ctx = ExecutionContext.create(principalId);
            var cmd = new EnableApplicationForClientCommand(applicationId, clientId, null);
            var result = applicationService.enableForClient(ctx, cmd);

            if (result instanceof Result.Failure<ApplicationEnabledForClient> f) {
                return Response.status(Response.Status.BAD_REQUEST)
                    .entity(new ApiResponses.ErrorResponse("ENABLE_FAILED", f.error().message()))
                    .build();
            }

            LOG.infof("Application %s enabled for client %s by principal %s",
                applicationId, clientId, principalId);
            String serializedClientId = TypedId.Ops.serialize(EntityType.CLIENT, clientId);
            String serializedAppId = TypedId.Ops.serialize(EntityType.APPLICATION, applicationId);
            return Response.ok(ClientApplicationStatusResponse.enabled(serializedClientId, serializedAppId)).build();
        } catch (NotFoundException e) {
            return Response.status(Response.Status.NOT_FOUND)
                .entity(new ApiResponses.NotFoundResponse("NOT_FOUND", e.getMessage(), "resource", null))
                .build();
        }
    }

    /**
     * Disable an application for a client.
     */
    @POST
    @Path("/{id}/applications/{applicationId}/disable")
    @Operation(operationId = "disableClientApplication", summary = "Disable application for client")
    @APIResponses({
        @APIResponse(responseCode = "200", description = "Application disabled",
            content = @Content(schema = @Schema(implementation = ClientApplicationStatusResponse.class))),
        @APIResponse(responseCode = "400", description = "Invalid ID format",
            content = @Content(schema = @Schema(implementation = ApiResponses.ErrorResponse.class))),
        @APIResponse(responseCode = "404", description = "Client or application not found",
            content = @Content(schema = @Schema(implementation = ApiResponses.NotFoundResponse.class))),
        @APIResponse(responseCode = "401", description = "Not authenticated",
            content = @Content(schema = @Schema(implementation = ApiResponses.UnauthorizedResponse.class))),
        @APIResponse(responseCode = "403", description = "Insufficient permissions",
            content = @Content(schema = @Schema(implementation = ApiResponses.ForbiddenResponse.class)))
    })
    public Response disableApplicationForClient(
            @TypedIdParam(EntityType.CLIENT) @PathParam("id") String clientId,
            @TypedIdParam(EntityType.APPLICATION) @PathParam("applicationId") String applicationId) {

        String principalId = auditContext.requirePrincipalId();
        // Disabling application for client requires both client update and application update
        authorizationService.requirePermission(principalId, PlatformAdminPermissions.CLIENT_UPDATE);
        authorizationService.requirePermission(principalId, PlatformAdminPermissions.APPLICATION_UPDATE);

        try {
            ExecutionContext ctx = ExecutionContext.create(principalId);
            var cmd = new DisableApplicationForClientCommand(applicationId, clientId);
            var result = applicationService.disableForClient(ctx, cmd);

            if (result instanceof Result.Failure<ApplicationDisabledForClient> f) {
                return Response.status(Response.Status.BAD_REQUEST)
                    .entity(new ApiResponses.ErrorResponse("DISABLE_FAILED", f.error().message()))
                    .build();
            }

            LOG.infof("Application %s disabled for client %s by principal %s",
                applicationId, clientId, principalId);
            String serializedClientId = TypedId.Ops.serialize(EntityType.CLIENT, clientId);
            String serializedAppId = TypedId.Ops.serialize(EntityType.APPLICATION, applicationId);
            return Response.ok(ClientApplicationStatusResponse.disabled(serializedClientId, serializedAppId)).build();
        } catch (NotFoundException e) {
            return Response.status(Response.Status.NOT_FOUND)
                .entity(new ApiResponses.NotFoundResponse("NOT_FOUND", e.getMessage(), "resource", null))
                .build();
        }
    }

    /**
     * Update applications for a client (bulk enable/disable).
     * Enables the specified applications and disables all others.
     */
    @PUT
    @Path("/{id}/applications")
    @Operation(operationId = "updateClientApplications", summary = "Update applications for client", description = "Sets which applications are enabled for this client")
    @APIResponses({
        @APIResponse(responseCode = "200", description = "Applications updated",
            content = @Content(schema = @Schema(implementation = ApplicationsUpdatedResponse.class))),
        @APIResponse(responseCode = "400", description = "Invalid ID format",
            content = @Content(schema = @Schema(implementation = ApiResponses.ErrorResponse.class))),
        @APIResponse(responseCode = "404", description = "Client not found",
            content = @Content(schema = @Schema(implementation = ApiResponses.NotFoundResponse.class))),
        @APIResponse(responseCode = "401", description = "Not authenticated",
            content = @Content(schema = @Schema(implementation = ApiResponses.UnauthorizedResponse.class))),
        @APIResponse(responseCode = "403", description = "Insufficient permissions",
            content = @Content(schema = @Schema(implementation = ApiResponses.ForbiddenResponse.class)))
    })
    public Response updateClientApplications(
            @TypedIdParam(EntityType.CLIENT) @PathParam("id") String clientId,
            @Valid UpdateClientApplicationsRequest request) {

        String principalId = auditContext.requirePrincipalId();
        // Bulk update requires both client update and application update
        authorizationService.requirePermission(principalId, PlatformAdminPermissions.CLIENT_UPDATE);
        authorizationService.requirePermission(principalId, PlatformAdminPermissions.APPLICATION_UPDATE);

        // Verify client exists
        var clientOpt = clientService.findById(clientId);
        if (clientOpt.isEmpty()) {
            return Response.status(Response.Status.NOT_FOUND)
                .entity(new ApiResponses.NotFoundResponse("client", clientId))
                .build();
        }

        // Validate all application IDs from the request (IDs are stored with prefixes)
        Set<String> enabledAppIds = request.enabledApplicationIds().stream()
            .peek(id -> TypedId.Ops.validate(EntityType.APPLICATION, id))
            .collect(Collectors.toSet());

        // Get all applications
        List<Application> allApps = applicationService.findAll();

        // Enable/disable each application
        for (Application app : allApps) {
            try {
                ExecutionContext ctx = ExecutionContext.create(principalId);
                if (enabledAppIds.contains(app.id)) {
                    var cmd = new EnableApplicationForClientCommand(app.id, clientId, null);
                    applicationService.enableForClient(ctx, cmd);
                } else {
                    var cmd = new DisableApplicationForClientCommand(app.id, clientId);
                    applicationService.disableForClient(ctx, cmd);
                }
            } catch (Exception e) {
                LOG.warnf("Failed to update application %s for client %s: %s",
                    app.id, clientId, e.getMessage());
            }
        }

        LOG.infof("Updated applications for client %s by principal %s: %d enabled",
            clientId, principalId, enabledAppIds.size());
        String serializedClientId = TypedId.Ops.serialize(EntityType.CLIENT, clientId);
        return Response.ok(ApplicationsUpdatedResponse.success(serializedClientId, enabledAppIds.size())).build();
    }

    // ==================== Request DTOs ====================

    @Schema(description = "Request to create a new client")
    public record CreateClientRequest(
        @NotBlank(message = "Name is required")
        @Size(max = 255, message = "Name must be less than 255 characters")
        @Schema(description = "Client display name", example = "Acme Corporation")
        String name,

        @NotBlank(message = "Identifier is required")
        @Size(min = 2, max = 100, message = "Identifier must be 2-100 characters")
        @Schema(description = "Unique client identifier/slug", example = "acme-corp")
        String identifier
    ) {}

    @Schema(description = "Request to update a client")
    public record UpdateClientRequest(
        @NotBlank(message = "Name is required")
        @Size(max = 255, message = "Name must be less than 255 characters")
        @Schema(description = "Client display name", example = "Acme Corporation")
        String name
    ) {}

    @Schema(description = "Request for status change with reason")
    public record StatusChangeRequest(
        @NotBlank(message = "Reason is required")
        @Schema(description = "Reason for status change", example = "Account suspended due to non-payment")
        String reason
    ) {}

    @Schema(description = "Request to add an audit note")
    public record AddNoteRequest(
        @NotBlank(message = "Category is required")
        @Schema(description = "Note category", example = "SUPPORT")
        String category,

        @NotBlank(message = "Text is required")
        @Schema(description = "Note text", example = "Customer requested feature X")
        String text
    ) {}

    @Schema(description = "Request to update client applications")
    public record UpdateClientApplicationsRequest(
        @Schema(description = "List of application IDs to enable for this client")
        List<String> enabledApplicationIds
    ) {}
}
