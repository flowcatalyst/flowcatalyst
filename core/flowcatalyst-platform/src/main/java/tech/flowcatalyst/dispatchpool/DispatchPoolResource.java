package tech.flowcatalyst.dispatchpool;

import jakarta.inject.Inject;
import jakarta.validation.Valid;
import jakarta.validation.constraints.Min;
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
import tech.flowcatalyst.dispatchpool.events.DispatchPoolCreated;
import tech.flowcatalyst.dispatchpool.events.DispatchPoolDeleted;
import tech.flowcatalyst.dispatchpool.events.DispatchPoolUpdated;
import tech.flowcatalyst.dispatchpool.operations.createpool.CreateDispatchPoolCommand;
import tech.flowcatalyst.dispatchpool.operations.deletepool.DeleteDispatchPoolCommand;
import tech.flowcatalyst.dispatchpool.operations.updatepool.UpdateDispatchPoolCommand;
import tech.flowcatalyst.platform.audit.AuditContext;
import tech.flowcatalyst.platform.authentication.EmbeddedModeOnly;
import tech.flowcatalyst.platform.authorization.AuthorizationService;
import tech.flowcatalyst.platform.authorization.platform.PlatformMessagingPermissions;
import tech.flowcatalyst.platform.common.ExecutionContext;
import tech.flowcatalyst.platform.common.Result;

import java.time.Instant;
import java.util.List;

/**
 * Admin API for dispatch pool management.
 *
 * Provides CRUD operations for dispatch pools including:
 * - Create, read, update, delete pools
 * - Status management (suspend, activate)
 * - Filtering by client, application, status
 *
 * All operations require messaging-level permissions.
 */
@Path("/api/admin/dispatch-pools")
@Tag(name = "BFF - Dispatch Pool Admin", description = "Administrative operations for dispatch pool management")
@Produces(MediaType.APPLICATION_JSON)
@Consumes(MediaType.APPLICATION_JSON)
@EmbeddedModeOnly
public class DispatchPoolResource {

    private static final Logger LOG = Logger.getLogger(DispatchPoolResource.class);

    @Inject
    DispatchPoolOperations poolOperations;

    @Inject
    AuditContext auditContext;

    @Inject
    AuthorizationService authorizationService;

    // ==================== List & Get ====================

    /**
     * List dispatch pools with optional filters.
     */
    @GET
    @Operation(summary = "List dispatch pools", description = "Returns dispatch pools with optional filtering")
    @APIResponses({
        @APIResponse(responseCode = "200", description = "List of dispatch pools",
            content = @Content(schema = @Schema(implementation = DispatchPoolListResponse.class))),
        @APIResponse(responseCode = "401", description = "Not authenticated"),
        @APIResponse(responseCode = "403", description = "Insufficient permissions")
    })
    public Response listPools(
            @QueryParam("clientId") @Parameter(description = "Filter by client ID") String clientId,
            @QueryParam("status") @Parameter(description = "Filter by status") DispatchPoolStatus status,
            @QueryParam("anchorLevel") @Parameter(description = "If true, return only anchor-level pools (clientId is null)") Boolean anchorLevel) {

        String principalId = auditContext.requirePrincipalId();
        authorizationService.requirePermission(principalId, PlatformMessagingPermissions.DISPATCH_POOL_VIEW);

        List<DispatchPool> pools;
        if (Boolean.TRUE.equals(anchorLevel)) {
            pools = poolOperations.findAnchorLevel();
            if (status != null) {
                pools = pools.stream().filter(p -> p.status() == status).toList();
            }
        } else {
            pools = poolOperations.findWithFilters(clientId, status);
        }

        List<DispatchPoolDto> dtos = pools.stream()
            .map(this::toDto)
            .toList();

        return Response.ok(new DispatchPoolListResponse(dtos, dtos.size())).build();
    }

    /**
     * Get a specific dispatch pool by ID.
     */
    @GET
    @Path("/{id}")
    @Operation(summary = "Get dispatch pool by ID")
    @APIResponses({
        @APIResponse(responseCode = "200", description = "Dispatch pool details",
            content = @Content(schema = @Schema(implementation = DispatchPoolDto.class))),
        @APIResponse(responseCode = "404", description = "Pool not found"),
        @APIResponse(responseCode = "401", description = "Not authenticated"),
        @APIResponse(responseCode = "403", description = "Insufficient permissions")
    })
    public Response getPool(@PathParam("id") String id) {

        String principalId = auditContext.requirePrincipalId();
        authorizationService.requirePermission(principalId, PlatformMessagingPermissions.DISPATCH_POOL_VIEW);

        return poolOperations.findById(id)
            .map(pool -> Response.ok(toDto(pool)).build())
            .orElse(Response.status(Response.Status.NOT_FOUND)
                .entity(new ErrorResponse("Dispatch pool not found"))
                .build());
    }

    // ==================== Create ====================

    /**
     * Create a new dispatch pool.
     */
    @POST
    @Operation(summary = "Create a new dispatch pool")
    @APIResponses({
        @APIResponse(responseCode = "201", description = "Dispatch pool created",
            content = @Content(schema = @Schema(implementation = DispatchPoolDto.class))),
        @APIResponse(responseCode = "400", description = "Invalid request or code already exists"),
        @APIResponse(responseCode = "401", description = "Not authenticated"),
        @APIResponse(responseCode = "403", description = "Insufficient permissions")
    })
    public Response createPool(@Valid CreateDispatchPoolRequest request, @Context UriInfo uriInfo) {

        String principalId = auditContext.requirePrincipalId();
        authorizationService.requirePermission(principalId, PlatformMessagingPermissions.DISPATCH_POOL_CREATE);

        CreateDispatchPoolCommand command = new CreateDispatchPoolCommand(
            request.code(),
            request.name(),
            request.description(),
            request.rateLimit(),
            request.concurrency(),
            request.clientId()
        );

        ExecutionContext ctx = ExecutionContext.create(principalId);
        Result<DispatchPoolCreated> result = poolOperations.createPool(command, ctx);

        if (result instanceof Result.Success<DispatchPoolCreated> success) {
            DispatchPoolCreated event = success.value();
            LOG.infof("Dispatch pool created: %s (%s) by principal %s",
                event.poolId(), event.code(), principalId);

            // Fetch the created pool to return full details
            return poolOperations.findById(event.poolId())
                .map(pool -> Response.status(Response.Status.CREATED)
                    .entity(toDto(pool))
                    .location(uriInfo.getAbsolutePathBuilder().path(pool.id()).build())
                    .build())
                .orElse(Response.status(Response.Status.CREATED)
                    .entity(new StatusResponse("Pool created", event.poolId()))
                    .build());
        } else {
            Result.Failure<DispatchPoolCreated> failure = (Result.Failure<DispatchPoolCreated>) result;
            return Response.status(Response.Status.BAD_REQUEST)
                .entity(new ErrorResponse(failure.error().message()))
                .build();
        }
    }

    // ==================== Update ====================

    /**
     * Update a dispatch pool.
     */
    @PUT
    @Path("/{id}")
    @Operation(summary = "Update dispatch pool")
    @APIResponses({
        @APIResponse(responseCode = "200", description = "Dispatch pool updated",
            content = @Content(schema = @Schema(implementation = DispatchPoolDto.class))),
        @APIResponse(responseCode = "404", description = "Pool not found"),
        @APIResponse(responseCode = "400", description = "Invalid request"),
        @APIResponse(responseCode = "401", description = "Not authenticated"),
        @APIResponse(responseCode = "403", description = "Insufficient permissions")
    })
    public Response updatePool(@PathParam("id") String id, @Valid UpdateDispatchPoolRequest request) {

        String principalId = auditContext.requirePrincipalId();
        authorizationService.requirePermission(principalId, PlatformMessagingPermissions.DISPATCH_POOL_UPDATE);

        UpdateDispatchPoolCommand command = new UpdateDispatchPoolCommand(
            id,
            request.name(),
            request.description(),
            request.rateLimit(),
            request.concurrency(),
            request.status()
        );

        ExecutionContext ctx = ExecutionContext.create(principalId);
        Result<DispatchPoolUpdated> result = poolOperations.updatePool(command, ctx);

        if (result instanceof Result.Success<DispatchPoolUpdated> success) {
            LOG.infof("Dispatch pool updated: %s by principal %s", id, principalId);

            return poolOperations.findById(id)
                .map(pool -> Response.ok(toDto(pool)).build())
                .orElse(Response.ok(new StatusResponse("Pool updated", id)).build());
        } else {
            Result.Failure<DispatchPoolUpdated> failure = (Result.Failure<DispatchPoolUpdated>) result;
            String errorCode = failure.error().code();
            if ("POOL_NOT_FOUND".equals(errorCode)) {
                return Response.status(Response.Status.NOT_FOUND)
                    .entity(new ErrorResponse(failure.error().message()))
                    .build();
            }
            return Response.status(Response.Status.BAD_REQUEST)
                .entity(new ErrorResponse(failure.error().message()))
                .build();
        }
    }

    // ==================== Delete ====================

    /**
     * Delete (archive) a dispatch pool.
     */
    @DELETE
    @Path("/{id}")
    @Operation(summary = "Delete dispatch pool", description = "Archives the pool (soft delete)")
    @APIResponses({
        @APIResponse(responseCode = "200", description = "Dispatch pool deleted"),
        @APIResponse(responseCode = "404", description = "Pool not found"),
        @APIResponse(responseCode = "400", description = "Cannot delete pool with active subscriptions"),
        @APIResponse(responseCode = "401", description = "Not authenticated"),
        @APIResponse(responseCode = "403", description = "Insufficient permissions")
    })
    public Response deletePool(@PathParam("id") String id) {

        String principalId = auditContext.requirePrincipalId();
        authorizationService.requirePermission(principalId, PlatformMessagingPermissions.DISPATCH_POOL_DELETE);

        DeleteDispatchPoolCommand command = new DeleteDispatchPoolCommand(id);
        ExecutionContext ctx = ExecutionContext.create(principalId);
        Result<DispatchPoolDeleted> result = poolOperations.deletePool(command, ctx);

        if (result instanceof Result.Success<DispatchPoolDeleted>) {
            LOG.infof("Dispatch pool deleted: %s by principal %s", id, principalId);
            return Response.ok(new StatusResponse("Pool deleted", id)).build();
        } else {
            Result.Failure<DispatchPoolDeleted> failure = (Result.Failure<DispatchPoolDeleted>) result;
            String errorCode = failure.error().code();
            if ("POOL_NOT_FOUND".equals(errorCode)) {
                return Response.status(Response.Status.NOT_FOUND)
                    .entity(new ErrorResponse(failure.error().message()))
                    .build();
            }
            return Response.status(Response.Status.BAD_REQUEST)
                .entity(new ErrorResponse(failure.error().message()))
                .build();
        }
    }

    // ==================== Status Management ====================

    /**
     * Suspend a dispatch pool.
     */
    @POST
    @Path("/{id}/suspend")
    @Operation(summary = "Suspend dispatch pool", description = "Temporarily disable the pool")
    @APIResponses({
        @APIResponse(responseCode = "200", description = "Pool suspended"),
        @APIResponse(responseCode = "404", description = "Pool not found"),
        @APIResponse(responseCode = "401", description = "Not authenticated"),
        @APIResponse(responseCode = "403", description = "Insufficient permissions")
    })
    public Response suspendPool(@PathParam("id") String id) {

        String principalId = auditContext.requirePrincipalId();
        authorizationService.requirePermission(principalId, PlatformMessagingPermissions.DISPATCH_POOL_UPDATE);

        UpdateDispatchPoolCommand command = new UpdateDispatchPoolCommand(
            id, null, null, null, null, DispatchPoolStatus.SUSPENDED
        );

        ExecutionContext ctx = ExecutionContext.create(principalId);
        Result<DispatchPoolUpdated> result = poolOperations.updatePool(command, ctx);

        if (result instanceof Result.Success<DispatchPoolUpdated>) {
            LOG.infof("Dispatch pool suspended: %s by principal %s", id, principalId);
            return Response.ok(new StatusResponse("Pool suspended", id)).build();
        } else {
            Result.Failure<DispatchPoolUpdated> failure = (Result.Failure<DispatchPoolUpdated>) result;
            String errorCode = failure.error().code();
            if ("POOL_NOT_FOUND".equals(errorCode)) {
                return Response.status(Response.Status.NOT_FOUND)
                    .entity(new ErrorResponse(failure.error().message()))
                    .build();
            }
            return Response.status(Response.Status.BAD_REQUEST)
                .entity(new ErrorResponse(failure.error().message()))
                .build();
        }
    }

    /**
     * Activate a dispatch pool.
     */
    @POST
    @Path("/{id}/activate")
    @Operation(summary = "Activate dispatch pool", description = "Re-enable a suspended pool")
    @APIResponses({
        @APIResponse(responseCode = "200", description = "Pool activated"),
        @APIResponse(responseCode = "404", description = "Pool not found"),
        @APIResponse(responseCode = "401", description = "Not authenticated"),
        @APIResponse(responseCode = "403", description = "Insufficient permissions")
    })
    public Response activatePool(@PathParam("id") String id) {

        String principalId = auditContext.requirePrincipalId();
        authorizationService.requirePermission(principalId, PlatformMessagingPermissions.DISPATCH_POOL_UPDATE);

        UpdateDispatchPoolCommand command = new UpdateDispatchPoolCommand(
            id, null, null, null, null, DispatchPoolStatus.ACTIVE
        );

        ExecutionContext ctx = ExecutionContext.create(principalId);
        Result<DispatchPoolUpdated> result = poolOperations.updatePool(command, ctx);

        if (result instanceof Result.Success<DispatchPoolUpdated>) {
            LOG.infof("Dispatch pool activated: %s by principal %s", id, principalId);
            return Response.ok(new StatusResponse("Pool activated", id)).build();
        } else {
            Result.Failure<DispatchPoolUpdated> failure = (Result.Failure<DispatchPoolUpdated>) result;
            String errorCode = failure.error().code();
            if ("POOL_NOT_FOUND".equals(errorCode)) {
                return Response.status(Response.Status.NOT_FOUND)
                    .entity(new ErrorResponse(failure.error().message()))
                    .build();
            }
            return Response.status(Response.Status.BAD_REQUEST)
                .entity(new ErrorResponse(failure.error().message()))
                .build();
        }
    }

    // ==================== Helper Methods ====================

    private DispatchPoolDto toDto(DispatchPool pool) {
        return new DispatchPoolDto(
            pool.id(),
            pool.code(),
            pool.name(),
            pool.description(),
            pool.rateLimit(),
            pool.concurrency(),
            pool.clientId(),
            pool.clientIdentifier(),
            pool.status(),
            pool.createdAt(),
            pool.updatedAt()
        );
    }

    // ==================== DTOs ====================

    public record DispatchPoolDto(
        String id,
        String code,
        String name,
        String description,
        int rateLimit,
        int concurrency,
        String clientId,
        String clientIdentifier,
        DispatchPoolStatus status,
        Instant createdAt,
        Instant updatedAt
    ) {}

    public record DispatchPoolListResponse(
        List<DispatchPoolDto> pools,
        int total
    ) {}

    public record CreateDispatchPoolRequest(
        @NotBlank(message = "Code is required")
        @Size(min = 2, max = 100, message = "Code must be 2-100 characters")
        String code,

        @NotBlank(message = "Name is required")
        @Size(max = 255, message = "Name must be less than 255 characters")
        String name,

        String description,

        @Min(value = 1, message = "Rate limit must be at least 1")
        int rateLimit,

        @Min(value = 1, message = "Concurrency must be at least 1")
        int concurrency,

        String clientId
    ) {}

    public record UpdateDispatchPoolRequest(
        @Size(max = 255, message = "Name must be less than 255 characters")
        String name,

        String description,

        @Min(value = 1, message = "Rate limit must be at least 1")
        Integer rateLimit,

        @Min(value = 1, message = "Concurrency must be at least 1")
        Integer concurrency,

        DispatchPoolStatus status
    ) {}

    public record StatusResponse(
        String message,
        String poolId
    ) {}

    public record ErrorResponse(
        String error
    ) {}
}
