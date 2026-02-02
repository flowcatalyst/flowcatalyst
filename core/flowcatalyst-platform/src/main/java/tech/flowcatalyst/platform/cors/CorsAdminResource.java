package tech.flowcatalyst.platform.cors;

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
import tech.flowcatalyst.platform.audit.AuditContext;
import tech.flowcatalyst.platform.common.ExecutionContext;
import tech.flowcatalyst.platform.common.Result;
import tech.flowcatalyst.platform.common.api.ApiResponses;
import tech.flowcatalyst.platform.common.api.CorsResponses.*;
import tech.flowcatalyst.platform.common.errors.UseCaseError;
import tech.flowcatalyst.platform.cors.events.CorsOriginAdded;
import tech.flowcatalyst.platform.cors.events.CorsOriginDeleted;
import tech.flowcatalyst.platform.cors.operations.addorigin.AddCorsOriginCommand;
import tech.flowcatalyst.platform.cors.operations.deleteorigin.DeleteCorsOriginCommand;

import java.util.List;
import java.util.Set;

/**
 * Admin API for managing CORS allowed origins.
 *
 * Requires platform:super-admin or platform:iam-admin role.
 * Located under Platform Identity & Access.
 */
@Path("/api/admin/platform/cors")
@Produces(MediaType.APPLICATION_JSON)
@Consumes(MediaType.APPLICATION_JSON)
@Tag(name = "BFF - Platform IAM - CORS", description = "Manage allowed CORS origins")
public class CorsAdminResource {

    @Inject
    CorsOperations corsOperations;

    @Inject
    AuditContext auditContext;

    @GET
    @Operation(operationId = "listCorsOrigins", summary = "List all CORS origins")
    @APIResponses({
        @APIResponse(responseCode = "200", description = "CORS origins retrieved",
            content = @Content(schema = @Schema(implementation = CorsOriginListResponse.class))),
        @APIResponse(responseCode = "401", description = "Not authenticated",
            content = @Content(schema = @Schema(implementation = ApiResponses.UnauthorizedResponse.class))),
        @APIResponse(responseCode = "403", description = "Insufficient permissions",
            content = @Content(schema = @Schema(implementation = ApiResponses.ForbiddenResponse.class)))
    })
    public Response listOrigins() {
        auditContext.requirePrincipalId();

        List<CorsAllowedOrigin> origins = corsOperations.listAll();
        List<CorsOriginDto> dtos = origins.stream()
            .map(this::toDto)
            .toList();

        return Response.ok(new CorsOriginListResponse(dtos, dtos.size())).build();
    }

    @GET
    @Path("/allowed")
    @Operation(operationId = "getAllowedCorsOrigins", summary = "Get allowed origins (cached)")
    @APIResponses({
        @APIResponse(responseCode = "200", description = "Allowed origins retrieved",
            content = @Content(schema = @Schema(implementation = AllowedOriginsResponse.class))),
        @APIResponse(responseCode = "401", description = "Not authenticated",
            content = @Content(schema = @Schema(implementation = ApiResponses.UnauthorizedResponse.class))),
        @APIResponse(responseCode = "403", description = "Insufficient permissions",
            content = @Content(schema = @Schema(implementation = ApiResponses.ForbiddenResponse.class)))
    })
    public Response getAllowedOrigins() {
        auditContext.requirePrincipalId();

        Set<String> origins = corsOperations.getAllowedOrigins();
        return Response.ok(new AllowedOriginsResponse(origins)).build();
    }

    @GET
    @Path("/{id}")
    @Operation(operationId = "getCorsOrigin", summary = "Get a CORS origin by ID")
    @APIResponses({
        @APIResponse(responseCode = "200", description = "CORS origin retrieved",
            content = @Content(schema = @Schema(implementation = CorsOriginDto.class))),
        @APIResponse(responseCode = "404", description = "CORS origin not found",
            content = @Content(schema = @Schema(implementation = ApiResponses.NotFoundResponse.class))),
        @APIResponse(responseCode = "401", description = "Not authenticated",
            content = @Content(schema = @Schema(implementation = ApiResponses.UnauthorizedResponse.class))),
        @APIResponse(responseCode = "403", description = "Insufficient permissions",
            content = @Content(schema = @Schema(implementation = ApiResponses.ForbiddenResponse.class)))
    })
    public Response getOrigin(@PathParam("id") String id) {
        auditContext.requirePrincipalId();

        return corsOperations.findById(id)
            .map(origin -> Response.ok(toDto(origin)).build())
            .orElse(Response.status(Response.Status.NOT_FOUND)
                .entity(new ApiResponses.NotFoundResponse("corsOrigin", id))
                .build());
    }

    @POST
    @Operation(operationId = "addCorsOrigin", summary = "Add a new allowed origin")
    @APIResponses({
        @APIResponse(responseCode = "201", description = "CORS origin created",
            content = @Content(schema = @Schema(implementation = CorsOriginDto.class))),
        @APIResponse(responseCode = "400", description = "Invalid request",
            content = @Content(schema = @Schema(implementation = ApiResponses.ErrorResponse.class))),
        @APIResponse(responseCode = "409", description = "Origin already exists",
            content = @Content(schema = @Schema(implementation = ApiResponses.ConflictResponse.class))),
        @APIResponse(responseCode = "401", description = "Not authenticated",
            content = @Content(schema = @Schema(implementation = ApiResponses.UnauthorizedResponse.class))),
        @APIResponse(responseCode = "403", description = "Insufficient permissions",
            content = @Content(schema = @Schema(implementation = ApiResponses.ForbiddenResponse.class)))
    })
    public Response addOrigin(CreateCorsOriginRequest request) {
        String principalId = auditContext.requirePrincipalId();
        ExecutionContext ctx = ExecutionContext.create(principalId);

        var command = new AddCorsOriginCommand(request.origin(), request.description());
        Result<CorsOriginAdded> result = corsOperations.addOrigin(command, ctx);

        return switch (result) {
            case Result.Success<CorsOriginAdded> s -> {
                // Fetch the created entry
                CorsAllowedOrigin origin = corsOperations.findById(s.value().originId()).orElse(null);
                yield Response.status(Response.Status.CREATED).entity(toDto(origin)).build();
            }
            case Result.Failure<CorsOriginAdded> f -> {
                yield switch (f.error()) {
                    case UseCaseError.ValidationError v -> Response.status(Response.Status.BAD_REQUEST)
                        .entity(new ApiResponses.ErrorResponse(v.code(), v.message()))
                        .build();
                    case UseCaseError.BusinessRuleViolation b -> Response.status(Response.Status.CONFLICT)
                        .entity(new ApiResponses.ConflictResponse(b.code(), b.message()))
                        .build();
                    case UseCaseError.NotFoundError n -> Response.status(Response.Status.NOT_FOUND)
                        .entity(new ApiResponses.NotFoundResponse(n.code(), n.message(), "resource", null))
                        .build();
                    default -> Response.status(Response.Status.BAD_REQUEST)
                        .entity(new ApiResponses.ErrorResponse(f.error().code(), f.error().message()))
                        .build();
                };
            }
        };
    }

    @DELETE
    @Path("/{id}")
    @Operation(operationId = "deleteCorsOrigin", summary = "Delete a CORS origin")
    @APIResponses({
        @APIResponse(responseCode = "200", description = "CORS origin deleted",
            content = @Content(schema = @Schema(implementation = CorsOriginDeletedResponse.class))),
        @APIResponse(responseCode = "400", description = "Invalid request",
            content = @Content(schema = @Schema(implementation = ApiResponses.ErrorResponse.class))),
        @APIResponse(responseCode = "404", description = "CORS origin not found",
            content = @Content(schema = @Schema(implementation = ApiResponses.NotFoundResponse.class))),
        @APIResponse(responseCode = "401", description = "Not authenticated",
            content = @Content(schema = @Schema(implementation = ApiResponses.UnauthorizedResponse.class))),
        @APIResponse(responseCode = "403", description = "Insufficient permissions",
            content = @Content(schema = @Schema(implementation = ApiResponses.ForbiddenResponse.class)))
    })
    public Response deleteOrigin(@PathParam("id") String id) {
        String principalId = auditContext.requirePrincipalId();
        ExecutionContext ctx = ExecutionContext.create(principalId);

        var command = new DeleteCorsOriginCommand(id);
        Result<CorsOriginDeleted> result = corsOperations.deleteOrigin(command, ctx);

        return switch (result) {
            case Result.Success<CorsOriginDeleted> s ->
                Response.ok(CorsOriginDeletedResponse.success(s.value().originId())).build();
            case Result.Failure<CorsOriginDeleted> f -> {
                yield switch (f.error()) {
                    case UseCaseError.NotFoundError n -> Response.status(Response.Status.NOT_FOUND)
                        .entity(new ApiResponses.NotFoundResponse("corsOrigin", id))
                        .build();
                    case UseCaseError.ValidationError v -> Response.status(Response.Status.BAD_REQUEST)
                        .entity(new ApiResponses.ErrorResponse(v.code(), v.message()))
                        .build();
                    default -> Response.status(Response.Status.BAD_REQUEST)
                        .entity(new ApiResponses.ErrorResponse(f.error().code(), f.error().message()))
                        .build();
                };
            }
        };
    }

    // ==================== Helper Methods ====================

    private CorsOriginDto toDto(CorsAllowedOrigin o) {
        return new CorsOriginDto(
            o.id,
            o.origin,
            o.description,
            o.createdBy,
            o.createdAt != null ? o.createdAt.toString() : null
        );
    }

    // ==================== Request DTOs ====================

    @Schema(description = "Request to create a new CORS origin")
    public record CreateCorsOriginRequest(
        @Schema(description = "Origin URL", example = "https://example.com")
        String origin,
        @Schema(description = "Description of why this origin is allowed")
        String description
    ) {}
}
