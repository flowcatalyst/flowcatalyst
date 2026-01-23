package tech.flowcatalyst.platform.application;

import jakarta.inject.Inject;
import jakarta.ws.rs.*;
import jakarta.ws.rs.core.*;
import org.eclipse.microprofile.openapi.annotations.Operation;
import org.eclipse.microprofile.openapi.annotations.media.Content;
import org.eclipse.microprofile.openapi.annotations.media.Schema;
import org.eclipse.microprofile.openapi.annotations.responses.APIResponse;
import org.eclipse.microprofile.openapi.annotations.responses.APIResponses;
import org.eclipse.microprofile.openapi.annotations.tags.Tag;
import tech.flowcatalyst.dispatchpool.DispatchPool;
import tech.flowcatalyst.dispatchpool.DispatchPoolOperations;
import tech.flowcatalyst.dispatchpool.DispatchPoolStatus;
import tech.flowcatalyst.dispatchpool.events.DispatchPoolsSynced;
import tech.flowcatalyst.dispatchpool.operations.syncpools.SyncDispatchPoolsCommand;
import tech.flowcatalyst.platform.audit.AuditContext;
import tech.flowcatalyst.platform.authentication.JwtKeyService;
import tech.flowcatalyst.platform.common.ExecutionContext;
import tech.flowcatalyst.platform.common.Result;
import tech.flowcatalyst.platform.common.TracingContext;
import tech.flowcatalyst.platform.common.errors.UseCaseError;

import java.util.List;
import java.util.Map;

/**
 * SDK API for external applications to manage dispatch pools.
 *
 * <p>External applications using the FlowCatalyst SDK can:
 * <ul>
 *   <li>List anchor-level dispatch pools</li>
 *   <li>Sync dispatch pools (bulk create/update/delete)</li>
 * </ul>
 *
 * <p>Pools are created as anchor-level (not client-scoped).
 */
@Path("/api/applications/{appCode}/dispatch-pools")
@Tag(name = "Application Dispatch Pools SDK", description = "SDK API for external applications to manage dispatch pools")
@Produces(MediaType.APPLICATION_JSON)
@Consumes(MediaType.APPLICATION_JSON)
public class ApplicationDispatchPoolResource {

    @Inject
    ApplicationRepository applicationRepository;

    @Inject
    DispatchPoolOperations poolOperations;

    @Inject
    JwtKeyService jwtKeyService;

    @Inject
    AuditContext auditContext;

    @Inject
    TracingContext tracingContext;

    /**
     * List all anchor-level dispatch pools.
     */
    @GET
    @Operation(summary = "List anchor-level dispatch pools",
        description = "Returns all anchor-level dispatch pools (clientId = null).")
    @APIResponses({
        @APIResponse(responseCode = "200", description = "List of dispatch pools",
            content = @Content(schema = @Schema(implementation = PoolListResponse.class))),
        @APIResponse(responseCode = "404", description = "Application not found")
    })
    public Response listPools(
            @PathParam("appCode") String appCode,
            @QueryParam("status") String status,
            @CookieParam("fc_session") String sessionToken,
            @HeaderParam("Authorization") String authHeader) {

        var principalIdOpt = jwtKeyService.extractAndValidatePrincipalId(sessionToken, authHeader);
        if (principalIdOpt.isEmpty()) {
            return Response.status(Response.Status.UNAUTHORIZED)
                .entity(new ErrorResponse("UNAUTHORIZED", "Not authenticated"))
                .build();
        }

        Application app = applicationRepository.findByCode(appCode).orElse(null);
        if (app == null) {
            return Response.status(Response.Status.NOT_FOUND)
                .entity(new ErrorResponse("APPLICATION_NOT_FOUND", "Application not found: " + appCode))
                .build();
        }

        // Get anchor-level pools
        List<DispatchPool> pools = poolOperations.findAnchorLevel();

        // Filter by status if provided
        if (status != null && !status.isBlank()) {
            try {
                DispatchPoolStatus statusEnum = DispatchPoolStatus.valueOf(status.toUpperCase());
                pools = pools.stream()
                    .filter(p -> p.status() == statusEnum)
                    .toList();
            } catch (IllegalArgumentException e) {
                return Response.status(Response.Status.BAD_REQUEST)
                    .entity(new ErrorResponse("INVALID_STATUS", "Invalid status. Must be ACTIVE or ARCHIVED"))
                    .build();
            }
        } else {
            // By default, exclude archived
            pools = pools.stream()
                .filter(p -> p.status() != DispatchPoolStatus.ARCHIVED)
                .toList();
        }

        List<PoolDto> dtos = pools.stream()
            .map(this::toPoolDto)
            .toList();

        return Response.ok(new PoolListResponse(dtos, dtos.size())).build();
    }

    /**
     * Sync dispatch pools from an external application.
     * Creates new pools, updates existing pools, and optionally removes unlisted pools.
     */
    @POST
    @Path("/sync")
    @Operation(summary = "Sync dispatch pools",
        description = "Bulk sync dispatch pools. Creates new pools, updates existing ones. " +
                      "Set removeUnlisted=true to archive pools not in the sync list.")
    @APIResponses({
        @APIResponse(responseCode = "200", description = "Sync complete",
            content = @Content(schema = @Schema(implementation = SyncResponse.class))),
        @APIResponse(responseCode = "404", description = "Application not found")
    })
    public Response syncPools(
            @PathParam("appCode") String appCode,
            @QueryParam("removeUnlisted") @DefaultValue("false") boolean removeUnlisted,
            SyncPoolsRequest request,
            @CookieParam("fc_session") String sessionToken,
            @HeaderParam("Authorization") String authHeader) {

        var principalIdOpt = jwtKeyService.extractAndValidatePrincipalId(sessionToken, authHeader);
        if (principalIdOpt.isEmpty()) {
            return Response.status(Response.Status.UNAUTHORIZED)
                .entity(new ErrorResponse("UNAUTHORIZED", "Not authenticated"))
                .build();
        }
        String principalId = principalIdOpt.get();

        Application app = applicationRepository.findByCode(appCode).orElse(null);
        if (app == null) {
            return Response.status(Response.Status.NOT_FOUND)
                .entity(new ErrorResponse("APPLICATION_NOT_FOUND", "Application not found: " + appCode))
                .build();
        }

        if (request.pools() == null || request.pools().isEmpty()) {
            return Response.status(Response.Status.BAD_REQUEST)
                .entity(new ErrorResponse("POOLS_REQUIRED", "pools list is required"))
                .build();
        }

        // Set audit context and create execution context
        auditContext.setPrincipalId(principalId);
        ExecutionContext context = ExecutionContext.from(tracingContext, principalId);

        // Convert request to internal format
        List<SyncDispatchPoolsCommand.SyncPoolItem> poolItems = request.pools().stream()
            .map(p -> new SyncDispatchPoolsCommand.SyncPoolItem(
                p.code(),
                p.name(),
                p.description(),
                p.rateLimit(),
                p.concurrency()
            ))
            .toList();

        SyncDispatchPoolsCommand command = new SyncDispatchPoolsCommand(appCode, poolItems, removeUnlisted);
        Result<DispatchPoolsSynced> result = poolOperations.syncPools(command, context);

        return switch (result) {
            case Result.Success<DispatchPoolsSynced> s -> {
                // Return updated pool list
                List<DispatchPool> pools = poolOperations.findAnchorLevel().stream()
                    .filter(p -> p.status() != DispatchPoolStatus.ARCHIVED)
                    .toList();
                List<PoolDto> dtos = pools.stream()
                    .map(this::toPoolDto)
                    .toList();
                yield Response.ok(new SyncResponse(
                    s.value().poolsCreated(),
                    s.value().poolsUpdated(),
                    s.value().poolsDeleted(),
                    dtos
                )).build();
            }
            case Result.Failure<DispatchPoolsSynced> f -> mapErrorToResponse(f.error());
        };
    }

    // ==================== Helper Methods ====================

    private Response mapErrorToResponse(UseCaseError error) {
        Response.Status status = switch (error) {
            case UseCaseError.ValidationError v -> Response.Status.BAD_REQUEST;
            case UseCaseError.NotFoundError n -> Response.Status.NOT_FOUND;
            case UseCaseError.BusinessRuleViolation b -> Response.Status.CONFLICT;
            case UseCaseError.ConcurrencyError c -> Response.Status.CONFLICT;
            case UseCaseError.AuthorizationError a -> Response.Status.FORBIDDEN;
        };

        return Response.status(status)
            .entity(new ErrorResponse(error.code(), error.message(), error.details()))
            .build();
    }

    private PoolDto toPoolDto(DispatchPool pool) {
        return new PoolDto(
            pool.id(),
            pool.code(),
            pool.name(),
            pool.description(),
            pool.rateLimit(),
            pool.concurrency(),
            pool.status().name()
        );
    }

    // ==================== DTOs ====================

    public record PoolDto(
        String id,
        String code,
        String name,
        String description,
        int rateLimit,
        int concurrency,
        String status
    ) {}

    public record PoolListResponse(
        List<PoolDto> pools,
        int total
    ) {}

    public record SyncPoolsRequest(
        List<SyncPoolItem> pools
    ) {}

    /**
     * Pool item for sync request.
     */
    public record SyncPoolItem(
        String code,
        String name,
        String description,
        Integer rateLimit,
        Integer concurrency
    ) {}

    public record SyncResponse(
        int created,
        int updated,
        int deleted,
        List<PoolDto> pools
    ) {}

    public record ErrorResponse(String code, String message, Map<String, Object> details) {
        public ErrorResponse(String code, String message) {
            this(code, message, Map.of());
        }
    }
}
