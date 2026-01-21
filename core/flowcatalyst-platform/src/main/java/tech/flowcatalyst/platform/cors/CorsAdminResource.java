package tech.flowcatalyst.platform.cors;

import jakarta.inject.Inject;
import jakarta.ws.rs.*;
import jakarta.ws.rs.core.MediaType;
import jakarta.ws.rs.core.Response;
import org.eclipse.microprofile.openapi.annotations.Operation;
import org.eclipse.microprofile.openapi.annotations.tags.Tag;
import tech.flowcatalyst.platform.audit.AuditContext;
import tech.flowcatalyst.platform.common.ExecutionContext;
import tech.flowcatalyst.platform.common.Result;
import tech.flowcatalyst.platform.common.errors.UseCaseError;
import tech.flowcatalyst.platform.cors.events.CorsOriginAdded;
import tech.flowcatalyst.platform.cors.events.CorsOriginDeleted;
import tech.flowcatalyst.platform.cors.operations.addorigin.AddCorsOriginCommand;
import tech.flowcatalyst.platform.cors.operations.deleteorigin.DeleteCorsOriginCommand;

import java.util.List;
import java.util.Map;
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
    @Operation(summary = "List all CORS origins")
    public Response listOrigins() {
        auditContext.requirePrincipalId();

        List<CorsAllowedOrigin> origins = corsOperations.listAll();
        List<CorsOriginResponse> responses = origins.stream()
            .map(CorsOriginResponse::from)
            .toList();

        return Response.ok(Map.of(
            "items", responses,
            "total", responses.size()
        )).build();
    }

    @GET
    @Path("/allowed")
    @Operation(summary = "Get allowed origins (cached)")
    public Response getAllowedOrigins() {
        auditContext.requirePrincipalId();

        Set<String> origins = corsOperations.getAllowedOrigins();
        return Response.ok(Map.of("origins", origins)).build();
    }

    @GET
    @Path("/{id}")
    @Operation(summary = "Get a CORS origin by ID")
    public Response getOrigin(@PathParam("id") String id) {
        auditContext.requirePrincipalId();

        return corsOperations.findById(id)
            .map(origin -> Response.ok(CorsOriginResponse.from(origin)).build())
            .orElse(Response.status(404)
                .entity(Map.of("error", "CORS entry not found"))
                .build());
    }

    @POST
    @Operation(summary = "Add a new allowed origin")
    public Response addOrigin(CreateCorsOriginRequest request) {
        String principalId = auditContext.requirePrincipalId();
        ExecutionContext ctx = ExecutionContext.create(principalId);

        var command = new AddCorsOriginCommand(request.origin(), request.description());
        Result<CorsOriginAdded> result = corsOperations.addOrigin(command, ctx);

        return switch (result) {
            case Result.Success<CorsOriginAdded> s -> {
                // Fetch the created entry
                CorsAllowedOrigin origin = corsOperations.findById(s.value().originId()).orElse(null);
                yield Response.status(201).entity(CorsOriginResponse.from(origin)).build();
            }
            case Result.Failure<CorsOriginAdded> f -> {
                int status = switch (f.error()) {
                    case UseCaseError.ValidationError ignored -> 400;
                    case UseCaseError.BusinessRuleViolation ignored -> 409;
                    case UseCaseError.NotFoundError ignored -> 404;
                    default -> 400;
                };
                yield Response.status(status)
                    .entity(Map.of("error", f.error().message()))
                    .build();
            }
        };
    }

    @DELETE
    @Path("/{id}")
    @Operation(summary = "Delete a CORS origin")
    public Response deleteOrigin(@PathParam("id") String id) {
        String principalId = auditContext.requirePrincipalId();
        ExecutionContext ctx = ExecutionContext.create(principalId);

        var command = new DeleteCorsOriginCommand(id);
        Result<CorsOriginDeleted> result = corsOperations.deleteOrigin(command, ctx);

        return switch (result) {
            case Result.Success<CorsOriginDeleted> ignored -> Response.noContent().build();
            case Result.Failure<CorsOriginDeleted> f -> {
                int status = switch (f.error()) {
                    case UseCaseError.NotFoundError ignored -> 404;
                    case UseCaseError.ValidationError ignored -> 400;
                    default -> 400;
                };
                yield Response.status(status)
                    .entity(Map.of("error", f.error().message()))
                    .build();
            }
        };
    }

    // DTOs

    public record CreateCorsOriginRequest(String origin, String description) {}

    public record CorsOriginResponse(
        String id,
        String origin,
        String description,
        String createdBy,
        String createdAt
    ) {
        public static CorsOriginResponse from(CorsAllowedOrigin o) {
            return new CorsOriginResponse(
                o.id,
                o.origin,
                o.description,
                o.createdBy,
                o.createdAt != null ? o.createdAt.toString() : null
            );
        }
    }
}
